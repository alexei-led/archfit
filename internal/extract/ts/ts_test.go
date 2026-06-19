package ts_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/extract/ts"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// fixtureDir is the testdata/ts directory with package.json and JSON fixtures.
var fixtureDir = filepath.Join("..", "..", "..", "testdata", "ts")

// loadFixture reads a JSON fixture from testdata/ts.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtureDir, name)) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatalf("load fixture %q: %v", name, err)
	}
	return data
}

// mockRunner builds a RunnerMock that:
//   - DetectFunc: returns (ToolInfo{Name: tool}, true) for bunx; false for anything else.
//   - RunFunc: when args contain "--version" returns "14.0.0"; otherwise returns fixtureData.
func mockRunner(fixtureData []byte) *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == "bunx" {
				return toolrun.ToolInfo{Name: "bunx", Path: "/usr/bin/bunx"}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if slices.Contains(cmd.Args, "--version") {
				return toolrun.Output{Stdout: []byte("14.0.0\n")}, nil
			}
			return toolrun.Output{Stdout: fixtureData}, nil
		},
	}
}

func TestExtract_Parse(t *testing.T) {
	data := loadFixture(t, "depcruise_output.json")
	runner := mockRunner(data)

	cfg := config.ExtractConfig{
		Mode:     config.ModeAuto,
		Internal: []string{"src/b/internal/**"},
	}
	extractor := ts.New(runner, cfg)

	s := scope.Scope{Root: fixtureDir}
	facts, cov, err := extractor.Extract(context.Background(), s)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Coverage checks.
	if cov.FilesSeen != 2 {
		t.Errorf("FilesSeen = %d, want 2", cov.FilesSeen)
	}
	if cov.Unresolved != 0 {
		t.Errorf("Unresolved = %d, want 0", cov.Unresolved)
	}
	if cov.Status != "ok" {
		t.Errorf("Status = %q, want %q", cov.Status, "ok")
	}
	if cov.Version != "14.0.0" {
		t.Errorf("Version = %q, want %q", cov.Version, "14.0.0")
	}

	// Edge assertions.
	findEdge := func(from, to string, kind graph.EdgeKind) *graph.Edge {
		for i := range facts.Edges {
			e := &facts.Edges[i]
			if e.From == from && e.To == to && e.Kind == kind {
				return e
			}
		}
		return nil
	}

	// a.ts → b/index.ts: imports
	e1 := findEdge("file:src/a.ts", "file:src/b/index.ts", graph.EdgeKindImports)
	if e1 == nil {
		t.Error("expected edge file:src/a.ts → file:src/b/index.ts (imports), not found")
	}

	// a.ts → b/internal/impl.ts: uses_internal (matches internal glob)
	e2 := findEdge("file:src/a.ts", "file:src/b/internal/impl.ts", graph.EdgeKindUsesInternal)
	if e2 == nil {
		t.Error("expected edge file:src/a.ts → file:src/b/internal/impl.ts (uses_internal), not found")
	}

	// fs (coreModule=true) must NOT appear as an edge.
	for _, e := range facts.Edges {
		if e.To == "file:fs" {
			t.Errorf("core module edge should not be emitted: %+v", e)
		}
	}
}

func TestExtract_CouldNotResolve(t *testing.T) {
	data := loadFixture(t, "depcruise_unresolved.json")
	runner := mockRunner(data)

	cfg := config.ExtractConfig{Mode: config.ModeAuto}
	extractor := ts.New(runner, cfg)

	s := scope.Scope{Root: fixtureDir}
	facts, cov, err := extractor.Extract(context.Background(), s)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if cov.Unresolved != 1 {
		t.Errorf("Unresolved = %d, want 1", cov.Unresolved)
	}
	if cov.Status != "partial" {
		t.Errorf("Status = %q, want %q", cov.Status, "partial")
	}
	if facts.Unresolved != 1 {
		t.Errorf("facts.Unresolved = %d, want 1", facts.Unresolved)
	}

	// The unresolved edge must still be emitted with low confidence, pointing at
	// an external node (not a first-party file).
	if len(facts.Edges) == 0 {
		t.Fatal("expected at least one edge for couldNotResolve case")
	}
	edge := facts.Edges[0]
	if edge.Confidence != "low" {
		t.Errorf("edge.Confidence = %q, want %q", edge.Confidence, "low")
	}
	if edge.To != "external:src/missing.ts" {
		t.Errorf("edge.To = %q, want %q (unresolved target marked external)", edge.To, "external:src/missing.ts")
	}
}

// TestExtract_ExternalNodes mirrors the codegraph case: a CLI importing the
// uninstalled npm package "commander" (couldNotResolve) and the node builtin
// "fs" (coreModule), with no node_modules. dependency-cruiser lists both as
// their own module.source entries (not just as dependencies), so the fixture
// includes those source entries. The unresolved package must become an external
// node — never a first-party file: node that pollutes martin metrics — and the
// core module must be dropped entirely, whether it appears as a dependency or as
// a source.
func TestExtract_ExternalNodes(t *testing.T) {
	data := loadFixture(t, "depcruise_external.json")
	runner := mockRunner(data)

	cfg := config.ExtractConfig{Mode: config.ModeAuto}
	extractor := ts.New(runner, cfg)

	facts, cov, err := extractor.Extract(context.Background(), scope.Scope{Root: fixtureDir})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// commander appears both as a module entry and as a dependency edge; it must
	// be counted once, not twice (no double-count of the same unresolved package).
	if cov.Unresolved != 1 {
		t.Errorf("cov.Unresolved = %d, want 1 (commander counted once)", cov.Unresolved)
	}
	// FilesSeen counts first-party source files only: src/cli.ts and
	// src/commands/index.ts — not the unresolved (commander) or core (fs) entries.
	if cov.FilesSeen != 2 {
		t.Errorf("cov.FilesSeen = %d, want 2 (first-party files only, excludes core/unresolved)", cov.FilesSeen)
	}
	if facts.Unresolved != 1 {
		t.Errorf("facts.Unresolved = %d, want 1 (commander counted once)", facts.Unresolved)
	}

	hasNode := func(id string) bool {
		for _, n := range facts.Nodes {
			if n.ID() == id {
				return true
			}
		}
		return false
	}

	// commander is unresolved → external node, NOT a first-party file: node.
	if hasNode("file:commander") {
		t.Error("unresolved npm package must not appear as a first-party file: node")
	}
	if !hasNode("external:commander") {
		t.Error("expected external:commander node for the unresolved npm package")
	}
	// fs is a core module → dropped entirely (no node, no edge).
	if hasNode("file:fs") || hasNode("external:fs") {
		t.Error("core module fs must not appear as a node")
	}
	// First-party files keep their file: nodes.
	if !hasNode("file:src/cli.ts") || !hasNode("file:src/commands/index.ts") {
		t.Error("first-party source files must keep file: nodes")
	}
	// Exactly the two first-party files plus external:commander — the builtin/
	// unresolved source entries must not add any first-party module nodes.
	if len(facts.Nodes) != 3 {
		t.Errorf("node count = %d, want 3 (2 first-party files + external:commander); nodes=%v", len(facts.Nodes), facts.Nodes)
	}

	// The edge to commander is kept (fan-out stays complete) and points external.
	var found bool
	for _, e := range facts.Edges {
		if e.To == "external:commander" {
			found = true
			if e.From != "file:src/cli.ts" {
				t.Errorf("commander edge From = %q, want file:src/cli.ts", e.From)
			}
			if e.Confidence != "low" {
				t.Errorf("commander edge Confidence = %q, want low", e.Confidence)
			}
		}
		if e.To == "file:fs" || e.To == "external:fs" {
			t.Errorf("core module fs must not produce an edge: %+v", e)
		}
	}
	if !found {
		t.Error("expected edge to external:commander")
	}
}

// TestExtract_EdgeTypes asserts dependency-cruiser dependencyTypes drive the
// Balanced Coupling integration-strength hint: a type-only (`import type`) edge
// shares only the type shape and vanishes at runtime → Contract (weakest); a
// value/runtime import and a dynamic `import()` both bind to exported
// names/signatures → Functional.
func TestExtract_EdgeTypes(t *testing.T) {
	data := loadFixture(t, "depcruise_edgetypes.json")
	runner := mockRunner(data)

	extractor := ts.New(runner, config.ExtractConfig{Mode: config.ModeAuto})

	facts, _, err := extractor.Extract(context.Background(), scope.Scope{Root: fixtureDir})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	hintFor := func(to string) string {
		for _, e := range facts.Edges {
			if e.From == "file:src/a.ts" && e.To == to {
				return e.StrengthHint
			}
		}
		t.Fatalf("edge file:src/a.ts → %s not found", to)
		return ""
	}

	cases := []struct {
		name string
		to   string
		want string
	}{
		{"type-only import → contract", "file:src/types.ts", "contract"},
		{"value import → functional", "file:src/b/index.ts", "functional"},
		{"dynamic import → functional", "file:src/lazy.ts", "functional"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hintFor(tc.to); got != tc.want {
				t.Errorf("StrengthHint(%s) = %q, want %q", tc.to, got, tc.want)
			}
		})
	}
}

func TestExtract_ToolAbsentAuto(t *testing.T) {
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, false
		},
		// RunFunc must not be called; leave nil to panic if it is.
	}

	cfg := config.ExtractConfig{Mode: config.ModeAuto}
	extractor := ts.New(runner, cfg)

	s := scope.Scope{Root: fixtureDir}
	facts, cov, err := extractor.Extract(context.Background(), s)
	if err != nil {
		t.Fatalf("Extract: expected nil error for absent tool in auto mode, got %v", err)
	}
	if len(facts.Edges) != 0 {
		t.Errorf("expected empty facts, got %d edges", len(facts.Edges))
	}
	if cov.Status != "absent" {
		t.Errorf("Coverage.Status = %q, want %q", cov.Status, "absent")
	}
	if cov.Tool != "dependency-cruiser" {
		t.Errorf("Coverage.Tool = %q, want %q", cov.Tool, "dependency-cruiser")
	}
}
