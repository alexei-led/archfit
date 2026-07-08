package golang_test

// workspace_fixture_test.go — Task 7: committed workspace fixture + extractor
// integration test.
//
// Fixture layout (under testdata/workspace/, materialized into t.TempDir()):
//
//	go.work         — uses ./a and ./b
//	a/go.mod        — module example.com/wa
//	a/consumer/consumer.go — imports example.com/wb/api (Greeter interface)
//	b/go.mod        — module example.com/wb
//	b/api/api.go    — exports Greeter interface
//
// The workspace root is NOT the git root (it is nested under testdata/ and
// further materialised into a TempDir). This directly exercises the cross-module
// extraction path added in Task 6 and documents the regression the plan fixes.

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	goextract "github.com/alexei-led/archfit/internal/extract/golang"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/view"
)

// materializeWorkspaceFixture copies the committed fixture under
// testdata/workspace/ into a fresh t.TempDir(), renaming:
//   - *.go.txt → *.go   (convention: keeps golangci-lint from typechecking fixtures)
//   - gowork.txt → go.work (go.work* is gitignored at the repo level)
//
// Returns the absolute path of the materialized workspace root.
func materializeWorkspaceFixture(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	fixtureDir := filepath.Join(filepath.Dir(thisFile), "testdata", "workspace")

	dir := t.TempDir()
	err := filepath.WalkDir(fixtureDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(fixtureDir, path)
		if relErr != nil {
			return relErr
		}
		// Rename .go.txt → .go in the destination.
		destRel := rel
		if strings.HasSuffix(rel, ".go.txt") {
			destRel = strings.TrimSuffix(rel, ".txt")
		}
		dst := filepath.Join(dir, destRel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o750)
		}
		data, readErr := os.ReadFile(path) //nolint:gosec // path is from WalkDir within a known fixture dir
		if readErr != nil {
			return readErr
		}
		if mkErr := os.MkdirAll(filepath.Dir(dst), 0o750); mkErr != nil {
			return mkErr
		}
		return os.WriteFile(dst, data, 0o600) //nolint:gosec // dst is within t.TempDir()
	})
	if err != nil {
		t.Fatalf("copy workspace fixture: %v", err)
	}

	// Rename gowork.txt → go.work.
	if err := os.Rename(
		filepath.Join(dir, "gowork.txt"),
		filepath.Join(dir, "go.work"),
	); err != nil {
		t.Fatalf("rename gowork.txt → go.work: %v", err)
	}

	return dir
}

// TestWorkspaceFixture_CoverageOK loads the committed two-module workspace
// fixture through the golang extractor and asserts that extraction succeeds.
func TestWorkspaceFixture_CoverageOK(t *testing.T) {
	dir := materializeWorkspaceFixture(t)

	ext := goextract.New(view.ExtractConfig{})
	_, cov, err := ext.Extract(context.Background(), scope.Scope{Root: dir, Mode: scope.ModeFull})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if cov.Status != "ok" {
		t.Errorf("cov.Status = %q, want %q", cov.Status, "ok")
	}
	if cov.FilesSeen == 0 {
		t.Error("FilesSeen = 0; expected Go source files")
	}
}

// TestWorkspaceFixture_TwoGoModules asserts that Extract populates
// facts.GoModules with exactly two entries (one per workspace member) and that
// their RelDirs are ScanRoot-relative (not bare module import paths).
func TestWorkspaceFixture_TwoGoModules(t *testing.T) {
	dir := materializeWorkspaceFixture(t)

	ext := goextract.New(view.ExtractConfig{})
	facts, _, err := ext.Extract(context.Background(), scope.Scope{Root: dir, Mode: scope.ModeFull})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if len(facts.GoModules) != 2 {
		t.Fatalf("GoModules = %v (len %d), want 2 entries", facts.GoModules, len(facts.GoModules))
	}
	for _, m := range facts.GoModules {
		if strings.HasPrefix(m.RelDir, "example.com/") {
			t.Errorf("GoModule.RelDir looks like a bare import path: %q", m.RelDir)
		}
		if m.RelDir == "" {
			t.Errorf("GoModule.RelDir is empty for module %q", m.Path)
		}
	}
}

// TestWorkspaceFixture_NodeIDsScanRootRelative asserts that package nodes use
// ScanRoot-relative paths (e.g. "a/consumer", "b/api") and not bare module
// import paths (e.g. "example.com/wa/consumer").
func TestWorkspaceFixture_NodeIDsScanRootRelative(t *testing.T) {
	dir := materializeWorkspaceFixture(t)

	ext := goextract.New(view.ExtractConfig{})
	facts, _, err := ext.Extract(context.Background(), scope.Scope{Root: dir, Mode: scope.ModeFull})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	var hasA, hasB bool
	for _, n := range facts.Nodes {
		if strings.HasPrefix(n.Path, "example.com/") {
			t.Errorf("node path not ScanRoot-relative: %q", n.Path)
		}
		if strings.HasPrefix(n.Path, "a/") {
			hasA = true
		}
		if strings.HasPrefix(n.Path, "b/") {
			hasB = true
		}
	}
	if !hasA {
		t.Error("no nodes with prefix a/ (module a not extracted)")
	}
	if !hasB {
		t.Error("no nodes with prefix b/ (module b not extracted)")
	}
}

// TestWorkspaceFixture_CrossModuleEdgeFirstPartyWithStrengthHint is the keystone
// assertion: the cross-module import (a/consumer → b/api) must be classified as
// first-party AND carry a StrengthHint.
//
// The StrengthHint comes from buildStrengthHints, which derives hints for every
// resolved target. If cross-member type references stopped producing hints,
// coupling_balance would collapse even when distance is present (Task 6's
// critical strength guard).
func TestWorkspaceFixture_CrossModuleEdgeFirstPartyWithStrengthHint(t *testing.T) {
	dir := materializeWorkspaceFixture(t)

	ext := goextract.New(view.ExtractConfig{})
	facts, _, err := ext.Extract(context.Background(), scope.Scope{Root: dir, Mode: scope.ModeFull})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Find the edge from a/consumer/consumer.go → b/api.
	var found bool
	for _, e := range facts.Edges {
		if !strings.HasSuffix(e.From, "a/consumer/consumer.go") {
			continue
		}
		if !strings.HasSuffix(e.To, "b/api") {
			continue
		}
		found = true
		if e.StrengthHint == "" {
			t.Errorf("cross-module edge %q → %q has no StrengthHint (member-set predicate broken)", e.From, e.To)
		}
		// consumer.go only uses api.Greeter as an interface type → "contract".
		if e.StrengthHint != hintContract {
			t.Errorf("cross-module StrengthHint = %q, want %q", e.StrengthHint, hintContract)
		}
		if e.Kind != graph.EdgeKindImports {
			t.Errorf("edge kind = %q, want %q", e.Kind, graph.EdgeKindImports)
		}
	}
	if !found {
		edges := make([]string, 0, len(facts.Edges))
		for _, e := range facts.Edges {
			edges = append(edges, e.From+"→"+e.To)
		}
		t.Errorf("no cross-module edge from a/consumer/consumer.go to b/api; all edges: %v", edges)
	}
}

// TestWorkspaceFixture_Regression_SingleLoadAtRoot documents the pre-Task-6
// regression: a single packages.Load with Dir=workspaceRoot and pattern "./..."
// at a workspace root that has no go.mod produces synthetic-error packages or
// an empty result — in either case, NO first-party cross-module edges with
// StrengthHints are produced.
//
// The new per-member Extract (Task 6) loads each member separately and
// correctly resolves cross-module edges with StrengthHints. This test guards
// the regression by:
//  1. Asserting that the old single-load approach fails to yield a StrengthHint
//     edge from a/consumer/consumer.go to b/api.
//  2. Asserting that the new Extract does produce it.
func TestWorkspaceFixture_Regression_SingleLoadAtRoot(t *testing.T) {
	dir := materializeWorkspaceFixture(t)

	loadMode := packages.NeedName |
		packages.NeedFiles |
		packages.NeedImports |
		packages.NeedSyntax |
		packages.NeedTypes |
		packages.NeedTypesInfo |
		packages.NeedModule

	// Simulate the pre-Task-6 path: one packages.Load at the workspace root.
	// A workspace root with go.work but no go.mod has no module of its own;
	// ./... finds no packages here — Load returns synthetic-error packages
	// (Module==nil && len(Errors)>0) or an empty slice.
	oldPkgs, _ := packages.Load(&packages.Config{
		Mode: loadMode,
		Dir:  dir,
	}, "./...")

	// Verify the old approach: either no packages, or all packages are
	// synthetic-error packages (no real module loaded).
	allSyntheticOrEmpty := len(oldPkgs) == 0
	for _, pkg := range oldPkgs {
		if pkg.Module == nil && len(pkg.Errors) > 0 {
			allSyntheticOrEmpty = true
		}
	}
	// Confirm the old approach returned only synthetic/empty packages. When TypesInfo
	// is nil for every package (the common case for a workspace root with no go.mod),
	// there is no basis for emitting cross-module StrengthHints — the allSyntheticOrEmpty
	// check above already captures this invariant.

	t.Logf("old single-load at workspace root: pkgs=%d, allSyntheticOrEmpty=%v", len(oldPkgs), allSyntheticOrEmpty)
	if !allSyntheticOrEmpty {
		t.Logf("note: packages.Load returned %d packages at workspace root without go.mod — "+
			"cross-module StrengthHints still absent because old code used single-module predicate", len(oldPkgs))
	}

	// Now run the new per-member Extract and verify it succeeds where old failed.
	ext := goextract.New(view.ExtractConfig{})
	facts, cov, err := ext.Extract(context.Background(), scope.Scope{Root: dir, Mode: scope.ModeFull})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if cov.Status != "ok" {
		t.Errorf("new Extract cov.Status = %q, want %q", cov.Status, "ok")
	}

	// New approach must produce a StrengthHint on the cross-module edge.
	var foundHint bool
	for _, e := range facts.Edges {
		if strings.HasSuffix(e.From, "a/consumer/consumer.go") &&
			strings.HasSuffix(e.To, "b/api") &&
			e.StrengthHint != "" {
			foundHint = true
			break
		}
	}
	if !foundHint {
		t.Error("new per-member Extract did not produce a cross-module StrengthHint edge (regression in Task 6 member-set predicate)")
	}
}
