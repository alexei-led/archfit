package golang_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	goextract "github.com/alexei-led/archfit/internal/extract/golang"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
)

// testdataRoot returns the absolute path to testdata/golang.
func testdataRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// file is .../internal/extract/golang/golang_test.go
	// go up three directories to reach the repo root, then into testdata/golang
	root := filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "golang")
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	return abs
}

// hasEdge reports whether edges contains an edge whose From ends with fromSuffix,
// To ends with toSuffix, and whose Kind matches.
func hasEdge(edges []graph.Edge, fromSuffix, toSuffix string, kind graph.EdgeKind) bool {
	for _, e := range edges {
		if containsSuffix(e.From, fromSuffix) && containsSuffix(e.To, toSuffix) && e.Kind == kind {
			return true
		}
	}
	return false
}

func containsSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func TestExtract_SimpleImport(t *testing.T) {
	root := testdataRoot(t)
	ext := goextract.New(config.ExtractConfig{})
	s := scope.Scope{Root: root, Mode: scope.ModeFull}

	facts, cov, err := ext.Extract(context.Background(), s)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if cov.FilesSeen == 0 {
		t.Error("expected FilesSeen > 0")
	}

	// pkg/a/a.go imports pkg/b — expect an "imports" edge (repo-relative path after module prefix strip)
	if !hasEdge(facts.Edges, "pkg/a/a.go", "pkg/b", graph.EdgeKindImports) {
		t.Errorf("expected imports edge from pkg/a/a.go to pkg/b; edges: %v", facts.Edges)
	}
}

func TestExtract_InternalAccess(t *testing.T) {
	root := testdataRoot(t)
	ext := goextract.New(config.ExtractConfig{})
	s := scope.Scope{Root: root, Mode: scope.ModeFull}

	facts, _, err := ext.Extract(context.Background(), s)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// pkg/a/violator.go imports pkg/b/internal/impl — uses_internal (repo-relative path)
	if !hasEdge(facts.Edges, "pkg/a/violator.go", "pkg/b/internal/impl", graph.EdgeKindUsesInternal) {
		t.Errorf("expected uses_internal edge from pkg/a/violator.go to pkg/b/internal/impl; edges: %v", facts.Edges)
	}
}

func TestExtract_ExcludedPath(t *testing.T) {
	root := testdataRoot(t)
	// Exclude any path that matches pkg/b — import targets containing pkg/b should be dropped.
	ext := goextract.New(config.ExtractConfig{
		Exclusions: []string{"**/pkg/b/**", "pkg/b/**"},
	})
	s := scope.Scope{Root: root, Mode: scope.ModeFull}

	facts, _, err := ext.Extract(context.Background(), s)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// No edges should target pkg/b (regular import) or pkg/b/internal/impl.
	for _, e := range facts.Edges {
		if containsSuffix(e.To, "pkg/b") || containsSuffix(e.To, "pkg/b/internal/impl") {
			t.Errorf("expected no edges to pkg/b after exclusion, got: %v", e)
		}
	}
}

func TestExtract_MissingPackage(t *testing.T) {
	root := testdataRoot(t)
	ext := goextract.New(config.ExtractConfig{})
	s := scope.Scope{Root: root, Mode: scope.ModeFull}

	// This test validates that extraction completes without error even when
	// packages have resolution issues. The testdata module is self-contained
	// so this primarily verifies no panic and correct coverage reporting.
	facts, cov, err := ext.Extract(context.Background(), s)
	if err != nil {
		t.Fatalf("Extract returned unexpected error: %v", err)
	}
	// Coverage status should be "ok" for a well-formed module.
	if cov.Status != "ok" && cov.Status != "partial" {
		t.Errorf("unexpected coverage status: %q", cov.Status)
	}
	// Unresolved in facts and coverage should agree.
	if facts.Unresolved != cov.Unresolved {
		t.Errorf("facts.Unresolved=%d != cov.Unresolved=%d", facts.Unresolved, cov.Unresolved)
	}
}
