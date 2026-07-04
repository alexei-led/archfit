package golang_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	goextract "github.com/alexei-led/archfit/internal/extract/golang"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
)

// Constants used in strength-hint tests.
const (
	hintContract   = "contract"
	hintModel      = "model"
	hintFunctional = "functional"
	hintDTO        = "dto"

	pkgB = "pkg/b" // repo-relative path of the test helper package

	statusPartial = "partial"
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

func TestExtract_NonGoDir_Absent(t *testing.T) {
	// A directory with no Go source files: go/packages finds nothing. The
	// extractor must report Status "absent" (not "partial"/"ok") so the coverage
	// metric reads n/a instead of a false-green 100% over an empty file set.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# docs only\n"), 0o600); err != nil {
		t.Fatalf("write README: %v", err)
	}
	ext := goextract.New(config.ExtractConfig{})
	s := scope.Scope{Root: dir, Mode: scope.ModeFull}

	_, cov, err := ext.Extract(context.Background(), s)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if cov.Status != "absent" {
		t.Errorf("non-Go dir: cov.Status = %q, want %q", cov.Status, "absent")
	}
	if cov.FilesSeen != 0 {
		t.Errorf("non-Go dir: FilesSeen = %d, want 0", cov.FilesSeen)
	}
}

// TestExtract_MemberLoadFailure: when go/packages.Load fails for a workspace
// member (e.g. a go.work "use" directive pointing at a directory that no longer
// exists), auto mode degrades to a partial coverage gap instead of failing the
// whole run; ModeOn still hard-errors so a required analyzer surfaces the problem.
func TestExtract_MemberLoadFailure(t *testing.T) {
	dir := t.TempDir()
	goWork := "go 1.21\n\nuse ./missing\n"
	if err := os.WriteFile(filepath.Join(dir, "go.work"), []byte(goWork), 0o600); err != nil {
		t.Fatalf("write go.work: %v", err)
	}
	s := scope.Scope{Root: dir, Mode: scope.ModeFull}

	t.Run("auto degrades to partial coverage", func(t *testing.T) {
		ext := goextract.New(config.ExtractConfig{Mode: config.ModeAuto})
		facts, cov, err := ext.Extract(context.Background(), s)
		if err != nil {
			t.Fatalf("auto mode must not error on member load failure; got %v", err)
		}
		if cov.Status != statusPartial {
			t.Errorf("Status = %q, want %q", cov.Status, statusPartial)
		}
		if len(facts.Nodes) != 0 || len(facts.Edges) != 0 {
			t.Errorf("expected no nodes/edges on degraded run; got %d nodes, %d edges", len(facts.Nodes), len(facts.Edges))
		}
	})

	t.Run("on hard-errors", func(t *testing.T) {
		ext := goextract.New(config.ExtractConfig{Mode: config.ModeOn})
		if _, _, err := ext.Extract(context.Background(), s); err == nil {
			t.Error("ModeOn must hard-error on member load failure")
		}
	})
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
	if !hasEdge(facts.Edges, "pkg/a/a.go", pkgB, graph.EdgeKindImports) {
		t.Errorf("expected imports edge from pkg/a/a.go to pkg/b; edges: %v", facts.Edges)
	}
}

func TestExtract_InternalAccess(t *testing.T) {
	root := testdataRoot(t)
	// violator.go carries //go:build extractortest so it is excluded from normal
	// go test runs (keeping testdata/golang/pkg/a compilable). The build tag
	// must be passed here so go/packages includes the file during extraction.
	ext := goextract.New(config.ExtractConfig{
		BuildFlags: []string{"-tags", "extractortest"},
	})
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
		if containsSuffix(e.To, pkgB) || containsSuffix(e.To, "pkg/b/internal/impl") {
			t.Errorf("expected no edges to pkg/b after exclusion, got: %v", e)
		}
	}
}

// edgeStrengthHint returns the StrengthHint for the first edge matching
// fromSuffix, toSuffix, and kind, or "" if no such edge exists.
func edgeStrengthHint(edges []graph.Edge, fromSuffix, toSuffix string, kind graph.EdgeKind) string {
	for _, e := range edges {
		if containsSuffix(e.From, fromSuffix) && containsSuffix(e.To, toSuffix) && e.Kind == kind {
			return e.StrengthHint
		}
	}
	return ""
}

// TestExtract_StrengthHint verifies that the Go extractor sets StrengthHint on
// edges using the SCIP reader mapping: interface TypeName → contract, concrete
// TypeName → model, Func → functional, and that the strongest rank wins when a
// file has multiple cross-package references.
func TestExtract_StrengthHint(t *testing.T) {
	root := testdataRoot(t)
	ext := goextract.New(config.ExtractConfig{})
	s := scope.Scope{Root: root, Mode: scope.ModeFull}

	facts, _, err := ext.Extract(context.Background(), s)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	cases := []struct {
		name     string
		from     string // file suffix
		to       string // package suffix
		kind     graph.EdgeKind
		wantHint string
	}{
		// a.go calls b.Hello() (a function) → functional.
		{"function call → functional", "pkg/a/a.go", pkgB, graph.EdgeKindImports, hintFunctional},
		// contract_cons.go only takes b.Greeter as a parameter type (interface TypeName) → contract.
		{"interface type → contract", "pkg/a/contract_cons.go", pkgB, graph.EdgeKindImports, hintContract},
		// model_cons.go only returns b.Config{} (concrete TypeName, no field access) → model.
		{"concrete type → model", "pkg/a/model_cons.go", pkgB, graph.EdgeKindImports, hintModel},
		// const_cons.go only reads b.MaxRetries — pure data sharing (book Ch7) → model, not functional.
		{"const reference → model", "pkg/a/const_cons.go", pkgB, graph.EdgeKindImports, hintModel},
		// var_cons.go only reads b.DefaultName — pure data sharing (book Ch7) → model, not functional.
		{"var reference → model", "pkg/a/var_cons.go", pkgB, graph.EdgeKindImports, hintModel},
		// max_cons.go uses b.Greeter (contract, rank 1) AND b.Hello() (functional, rank 4) → functional wins.
		{"max rank wins", "pkg/a/max_cons.go", pkgB, graph.EdgeKindImports, hintFunctional},
		// dto_cons.go only references b.UserDTO — exported struct, only exported
		// data fields, empty method set → dto (the book's canonical Contract when
		// the edge crosses a declared public boundary; classify resolves which).
		{"pure-data struct → dto", "pkg/a/dto_cons.go", pkgB, graph.EdgeKindImports, hintDTO},
		// entity_cons.go references b.Entity — a struct WITH a method → model.
		{"struct with method → model", "pkg/a/entity_cons.go", pkgB, graph.EdgeKindImports, hintModel},
		// partial_cons.go references b.Partial — unexported field → model.
		{"struct with unexported field → model", "pkg/a/partial_cons.go", pkgB, graph.EdgeKindImports, hintModel},
		// callback_cons.go references b.CallbackHolder — func-typed field → model.
		{"struct with func field → model", "pkg/a/callback_cons.go", pkgB, graph.EdgeKindImports, hintModel},
		// dto_rank_cons.go uses b.Greeter (contract, rank 1) AND b.UserDTO (dto, rank 2) → dto wins.
		{"dto outranks contract", "pkg/a/dto_rank_cons.go", pkgB, graph.EdgeKindImports, hintDTO},
		// funcvar_cons.go calls b.DefaultHandler() — a func-typed var is stored behavior → functional.
		{"func-valued var → functional", "pkg/a/funcvar_cons.go", pkgB, graph.EdgeKindImports, hintFunctional},
		// callback_fire_cons.go invokes h.OnDone() — a func-typed field call outranks the holder's model hint.
		{"func-typed field invocation → functional", "pkg/a/callback_fire_cons.go", pkgB, graph.EdgeKindImports, hintFunctional},
		// dto_read_cons.go reads u.ID via a plain selector — the DTO field stays dto.
		{"dto field selector read → dto", "pkg/a/dto_read_cons.go", pkgB, graph.EdgeKindImports, hintDTO},
		// var_set_cons.go WRITES b.DefaultName — reads and writes classify alike → model.
		{"var write → model", "pkg/a/var_set_cons.go", pkgB, graph.EdgeKindImports, hintModel},
		// chan_cons.go references b.ChanHolder — chan-typed field disqualifies the DTO upgrade → model.
		{"struct with chan field → model", "pkg/a/chan_cons.go", pkgB, graph.EdgeKindImports, hintModel},
		// iface_cons.go references b.IfaceHolder — interface-typed field disqualifies the DTO upgrade → model.
		{"struct with interface field → model", "pkg/a/iface_cons.go", pkgB, graph.EdgeKindImports, hintModel},
		// marker_cons.go references b.Marker — zero-field structs carry no data model → model, not dto.
		{"zero-field marker struct → model", "pkg/a/marker_cons.go", pkgB, graph.EdgeKindImports, hintModel},
		// composite_carrier_cons.go references b.SliceCallbackHolder — a []func()
		// field smuggles behavior through a composite element type → model, not dto.
		{"struct with []func field → model", "pkg/a/composite_carrier_cons.go", pkgB, graph.EdgeKindImports, hintModel},
		// chanmap_carrier_cons.go references b.ChanMapHolder — map[string]chan int → model.
		{"struct with map-of-chan field → model", "pkg/a/chanmap_carrier_cons.go", pkgB, graph.EdgeKindImports, hintModel},
		// ptriface_carrier_cons.go references b.PtrIfaceHolder — *Greeter (pointer to interface) → model.
		{"struct with pointer-to-interface field → model", "pkg/a/ptriface_carrier_cons.go", pkgB, graph.EdgeKindImports, hintModel},
		// nested_carrier_cons.go references b.NestedHolder — nested struct holds a func field → model.
		{"struct with nested behavior carrier → model", "pkg/a/nested_carrier_cons.go", pkgB, graph.EdgeKindImports, hintModel},
		// linked_dto_cons.go references b.LinkedNode — a self-referential pure-data
		// struct: the *LinkedNode cycle is not a behavior carrier → dto.
		{"self-referential pure data → dto", "pkg/a/linked_dto_cons.go", pkgB, graph.EdgeKindImports, hintDTO},
		// external_cons.go calls strings.Repeat — an EXTERNAL (stdlib) target must
		// carry a real type-info hint so a declared external_systems seam can score
		// at D=10 instead of abstaining (no manual hint injection anywhere here).
		{"external stdlib function call → functional", "pkg/a/external_cons.go", "strings", graph.EdgeKindImports, hintFunctional},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := edgeStrengthHint(facts.Edges, tc.from, tc.to, tc.kind)
			if got != tc.wantHint {
				t.Errorf("StrengthHint = %q, want %q (edges: %v)", got, tc.wantHint, facts.Edges)
			}
		})
	}
}

// TestExtract_StrengthHint_UsesInternal verifies that StrengthHint is also set
// on EdgeKindUsesInternal edges (not only imports).
func TestExtract_StrengthHint_UsesInternal(t *testing.T) {
	root := testdataRoot(t)
	ext := goextract.New(config.ExtractConfig{
		BuildFlags: []string{"-tags", "extractortest"},
	})
	s := scope.Scope{Root: root, Mode: scope.ModeFull}

	facts, _, err := ext.Extract(context.Background(), s)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// violator.go calls impl.Secret() (a function) → uses_internal edge with functional hint.
	hint := edgeStrengthHint(facts.Edges, "pkg/a/violator.go", "pkg/b/internal/impl", graph.EdgeKindUsesInternal)
	if hint != "functional" {
		t.Errorf("uses_internal StrengthHint = %q, want %q", hint, "functional")
	}
}

// TestExtract_StrengthHint_NoHintForExcluded verifies that excluded paths do not
// contribute to StrengthHints (the edge itself is dropped, so the hint is irrelevant,
// but the map must not be keyed on excluded files).
func TestExtract_StrengthHint_NoHintForExcluded(t *testing.T) {
	root := testdataRoot(t)
	ext := goextract.New(config.ExtractConfig{
		Exclusions: []string{"**/pkg/b/**", "pkg/b/**"},
	})
	s := scope.Scope{Root: root, Mode: scope.ModeFull}

	facts, _, err := ext.Extract(context.Background(), s)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// With pkg/b excluded, no edges to pkg/b should exist (hint is moot but assert no edge).
	for _, e := range facts.Edges {
		if containsSuffix(e.To, pkgB) {
			t.Errorf("expected no edges to pkg/b after exclusion, got: %+v", e)
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
	if cov.Status != "ok" && cov.Status != statusPartial {
		t.Errorf("unexpected coverage status: %q", cov.Status)
	}
	// Unresolved in facts and coverage should agree.
	if facts.Unresolved != cov.Unresolved {
		t.Errorf("facts.Unresolved=%d != cov.Unresolved=%d", facts.Unresolved, cov.Unresolved)
	}
}
