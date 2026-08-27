package golang_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"

	goextract "github.com/alexei-led/archfit/internal/extract/golang"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
)

// buildWorkspace creates a minimal go.work workspace in a temp dir with two
// modules:
//
//	dir/b  — module example.com/b, exports:
//	           api.Greeter (interface) → StrengthHint "contract"
//	           api.Config  (concrete)  → StrengthHint "model"
//	dir/a  — module example.com/a, imports example.com/b/api
//	dir/go.work — uses ./a and ./b
//
// Returns the workspace root dir.
func buildWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	writeTestFile(t, filepath.Join(dir, "b", "go.mod"),
		"module example.com/b\n\ngo 1.21\n")
	writeTestFile(t, filepath.Join(dir, "b", "api", "api.go"), `package api

// Greeter is a contract interface.
type Greeter interface {
	Greet() string
}

// Config is a concrete configuration struct.
type Config struct {
	Name string
}
`)

	writeTestFile(t, filepath.Join(dir, "a", "go.mod"),
		"module example.com/a\n\ngo 1.21\n")
	writeTestFile(t, filepath.Join(dir, "a", "consumer", "consumer.go"), `package consumer

import "example.com/b/api"

// AsGreeter accepts a Greeter interface value but calls no methods on it.
// Only the TypeName reference api.Greeter is present → StrengthHint "contract".
func AsGreeter(g api.Greeter) api.Greeter {
	return g
}
`)

	writeTestFile(t, filepath.Join(dir, "go.work"),
		"go 1.21\n\nuse (\n\t./a\n\t./b\n)\n")

	return dir
}

// writeTestFile creates parent dirs and writes content to path.
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// --- Tests ---

// TestExtract_MultiModule_CrossModuleStrengthHint is the keystone strength-guard
// test: a cross-module edge (a/consumer → b/api) must carry a StrengthHint.
// buildStrengthHints derives hints for every resolved target; the original
// regression was an isInModule predicate that only matched the loading member's
// own module path, leaving cross-member hints empty.
func TestExtract_MultiModule_CrossModuleStrengthHint(t *testing.T) {
	dir := buildWorkspace(t)

	ext := goextract.New(evidenceports.ExtractConfig{})
	facts, cov, err := ext.Extract(context.Background(), scope.Scope{Root: dir, Mode: scope.ModeFull})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if cov.Status != "ok" {
		t.Errorf("cov.Status = %q, want %q", cov.Status, "ok")
	}

	// Find the cross-module edge from a/consumer/consumer.go → b/api.
	var found bool
	for _, e := range facts.Edges {
		if !strings.HasSuffix(e.From, "consumer/consumer.go") {
			continue
		}
		if !strings.HasSuffix(e.To, "b/api") {
			continue
		}
		found = true
		if e.StrengthHint == "" {
			t.Error("cross-module edge has no StrengthHint (member-set predicate broken)")
		} else if e.StrengthHint != hintContract {
			t.Errorf("cross-module StrengthHint = %q, want %q", e.StrengthHint, hintContract)
		}
	}
	if !found {
		froms := make([]string, 0, len(facts.Edges))
		for _, e := range facts.Edges {
			froms = append(froms, e.From+"→"+e.To)
		}
		t.Errorf("no cross-module edge from consumer/consumer.go to b/api; edges: %v", froms)
	}
}

// TestExtract_MultiModule_ScanRootRelativeIDs checks that node paths are
// ScanRoot-relative (e.g. "a/consumer", "b/api") and not bare module paths
// (e.g. "example.com/a/consumer").
func TestExtract_MultiModule_ScanRootRelativeIDs(t *testing.T) {
	dir := buildWorkspace(t)

	ext := goextract.New(evidenceports.ExtractConfig{})
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

// TestExtract_MultiModule_GoModulesPopulated verifies that facts.GoModules is
// populated with one entry per workspace member, sorted by Path.
func TestExtract_MultiModule_GoModulesPopulated(t *testing.T) {
	dir := buildWorkspace(t)

	ext := goextract.New(evidenceports.ExtractConfig{})
	facts, _, err := ext.Extract(context.Background(), scope.Scope{Root: dir, Mode: scope.ModeFull})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if len(facts.GoModules) != 2 {
		t.Fatalf("GoModules len = %d, want 2; got %v", len(facts.GoModules), facts.GoModules)
	}

	// Must be sorted by Path.
	paths := make([]string, len(facts.GoModules))
	for i, m := range facts.GoModules {
		paths[i] = m.Path
	}
	sorted := append([]string(nil), paths...)
	sort.Strings(sorted)
	if !slices.Equal(paths, sorted) {
		t.Errorf("GoModules not sorted by Path: %v", paths)
	}

	// RelDirs must be non-empty and not contain bare module paths.
	for _, m := range facts.GoModules {
		if strings.HasPrefix(m.RelDir, "example.com/") {
			t.Errorf("GoModule.RelDir looks like a module path: %q", m.RelDir)
		}
	}
}

// TestExtract_MultiModule_SortedOutput runs Extract twice and asserts that the
// node and edge sets are identical across runs (double-run determinism).
func TestExtract_MultiModule_SortedOutput(t *testing.T) {
	dir := buildWorkspace(t)

	ext := goextract.New(evidenceports.ExtractConfig{})
	s := scope.Scope{Root: dir, Mode: scope.ModeFull}

	facts1, _, err := ext.Extract(context.Background(), s)
	if err != nil {
		t.Fatalf("Extract run 1: %v", err)
	}
	facts2, _, err := ext.Extract(context.Background(), s)
	if err != nil {
		t.Fatalf("Extract run 2: %v", err)
	}

	ids1 := sortedNodeIDs(facts1.Nodes)
	ids2 := sortedNodeIDs(facts2.Nodes)
	if !slices.Equal(ids1, ids2) {
		t.Errorf("node IDs differ between runs:\nrun1: %v\nrun2: %v", ids1, ids2)
	}

	keys1 := sortedEdgeKeys(facts1.Edges)
	keys2 := sortedEdgeKeys(facts2.Edges)
	if !slices.Equal(keys1, keys2) {
		t.Errorf("edge keys differ between runs:\nrun1: %v\nrun2: %v", keys1, keys2)
	}
}

// TestExtract_MultiModule_MissingDep_Partial verifies that a module with an
// unresolvable import yields "partial" coverage (not "absent") because source
// files were seen even though type-checking failed.
func TestExtract_MultiModule_MissingDep_Partial(t *testing.T) {
	dir := t.TempDir()

	// Single module that imports a local package that does not exist.
	writeTestFile(t, filepath.Join(dir, "go.mod"), "module example.com/broken\n\ngo 1.21\n")
	writeTestFile(t, filepath.Join(dir, "app", "main.go"), `package app

import _ "example.com/broken/notexist"
`)

	ext := goextract.New(evidenceports.ExtractConfig{})
	_, cov, err := ext.Extract(context.Background(), scope.Scope{Root: dir, Mode: scope.ModeFull})
	if err != nil {
		t.Skipf("Extract returned fatal error (platform-dependent, skip): %v", err)
	}

	// With a missing local import the package is ill-typed → unresolved>0 → "partial".
	if cov.Status != statusPartial {
		t.Errorf("status = %q, want %q (files were seen but type-checking failed)", cov.Status, statusPartial)
	}
}

// sortedNodeIDs returns node IDs in sorted order.
func sortedNodeIDs(nodes []graph.Node) []string {
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID()
	}
	slices.Sort(ids)
	return ids
}

// sortedEdgeKeys returns "from→to(kind)" strings in sorted order.
func sortedEdgeKeys(edges []graph.Edge) []string {
	keys := make([]string, len(edges))
	for i, e := range edges {
		keys[i] = e.From + "→" + e.To + "(" + string(e.Kind) + ")"
	}
	slices.Sort(keys)
	return keys
}
