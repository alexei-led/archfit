package boundary_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/boundary"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/signal"
)

const (
	clFileA = "pkg/a/a.go"
	clFileB = "pkg/b/b.go"
	clFileC = "pkg/c/c.go"
	clNodeA = "file:pkg/a/a.go"
	clNodeB = "file:pkg/b/b.go"
	clNodeC = "file:pkg/c/c.go"
)

// clGraph builds a → b → c (a, b, c in different modules), plus a same-module
// edge a → a2.
func clGraph() *graph.Graph {
	return graph.Build([]graph.Facts{{
		Language: "go",
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: clFileA},
			{Kind: graph.NodeKindFile, Path: clFileB},
			{Kind: graph.NodeKindFile, Path: clFileC},
			{Kind: graph.NodeKindFile, Path: "pkg/a/a2.go"},
		},
		Edges: []graph.Edge{
			{From: clNodeA, To: clNodeB, Kind: graph.EdgeKindImports, Language: "go"},
			{From: clNodeB, To: clNodeC, Kind: graph.EdgeKindImports, Language: "go"},
			{From: clNodeA, To: "file:pkg/a/a2.go", Kind: graph.EdgeKindImports, Language: "go"},
		},
	}})
}

// clIndex classifies a→b as cross-module, a→a2 as same-module.
func clIndex() coupling.Index {
	return coupling.Index{
		clNodeA + "\x00" + clNodeB + "\x00imports": {
			Strength: coupling.StrengthFunctional, Distance: coupling.DistanceCrossModuleDiffOwner,
		},
		clNodeB + "\x00" + clNodeC + "\x00imports": {
			Strength: coupling.StrengthFunctional, Distance: coupling.DistanceCrossModuleDiffOwner,
		},
		clNodeA + "\x00file:pkg/a/a2.go\x00imports": {
			Strength: coupling.StrengthFunctional, Distance: coupling.DistanceSameModule,
		},
	}
}

func TestChangeLocality_CountsCrossModuleEdgesFromChangedFiles(t *testing.T) {
	m := boundary.ChangeLocalityMetric{}
	res := m.Calculate(signal.CommonInput{
		Graph:           clGraph(),
		Classifications: clIndex(),
		ChangedFiles:    []string{clFileA},
	})

	// Only a's edges count: a→b is cross-module (1); a→a2 same-module (not
	// counted); b→c is cross-module but b is unchanged.
	if res.Value != 1 {
		t.Errorf("value = %v, want 1 (a→b only)", res.Value)
	}
	// Forward reach from a: b, a2, then c via b → 3 files.
	want := "1 cross-module edge(s) from 1 changed file(s); forward reach 3 file(s)"
	if res.Display != want {
		t.Errorf("display = %q, want %q", res.Display, want)
	}
	if res.Band != "info" {
		t.Errorf("band = %q, want info (report-only)", res.Band)
	}
	if res.Confidence != "high" {
		t.Errorf("confidence = %q, want high", res.Confidence)
	}
}

func TestChangeLocality_NAWithoutDiff(t *testing.T) {
	m := boundary.ChangeLocalityMetric{}
	res := m.Calculate(signal.CommonInput{Graph: clGraph(), Classifications: clIndex()})
	if res.Band != "n/a" {
		t.Errorf("band = %q, want n/a in full mode (no changed files)", res.Band)
	}
}

func TestChangeLocality_LowConfidenceWithoutClassifications(t *testing.T) {
	m := boundary.ChangeLocalityMetric{}
	res := m.Calculate(signal.CommonInput{
		Graph:        clGraph(),
		ChangedFiles: []string{clFileA},
	})
	if res.Confidence != "low" {
		t.Errorf("confidence = %q, want low without classifications", res.Confidence)
	}
	if res.Value != 0 {
		t.Errorf("value = %v, want 0 (no classified cross-module edges)", res.Value)
	}
}

// pyGraph builds a Python import graph with dotted "module:" node IDs:
// app.handlers → app.core → app.db (each a distinct module). Mirrors the
// internal/extract/py scheme where node Path is the dotted module name.
func pyGraph() *graph.Graph {
	const (
		modHandlers = "module:app.handlers"
		modCore     = "module:app.core"
		modDB       = "module:app.db"
	)
	return graph.Build([]graph.Facts{{
		Language: graph.LangPython,
		Nodes: []graph.Node{
			{Kind: graph.NodeKindModule, Path: "app.handlers"},
			{Kind: graph.NodeKindModule, Path: "app.core"},
			{Kind: graph.NodeKindModule, Path: "app.db"},
		},
		Edges: []graph.Edge{
			{From: modHandlers, To: modCore, Kind: graph.EdgeKindImports, Language: graph.LangPython},
			{From: modCore, To: modDB, Kind: graph.EdgeKindImports, Language: graph.LangPython},
		},
	}})
}

func pyIndex() coupling.Index {
	const (
		modHandlers = "module:app.handlers"
		modCore     = "module:app.core"
		modDB       = "module:app.db"
	)
	return coupling.Index{
		modHandlers + "\x00" + modCore + "\x00imports": {
			Strength: coupling.StrengthFunctional, Distance: coupling.DistanceCrossModuleDiffOwner,
		},
		modCore + "\x00" + modDB + "\x00imports": {
			Strength: coupling.StrengthFunctional, Distance: coupling.DistanceCrossModuleDiffOwner,
		},
	}
}

// TestChangeLocality_PythonModuleNodeIDs is the regression test for the false
// zero: changed files arrive as "app/handlers.py" file paths, but Python node
// IDs are dotted "module:app.handlers". The metric must bridge the two schemes.
func TestChangeLocality_PythonModuleNodeIDs(t *testing.T) {
	m := boundary.ChangeLocalityMetric{}
	res := m.Calculate(signal.CommonInput{
		Graph:           pyGraph(),
		Classifications: pyIndex(),
		ChangedFiles:    []string{"app/handlers.py"},
	})

	// app.handlers → app.core is the one cross-module edge from the changed
	// module; before the fix this was a false 0.
	if res.Value != 1 {
		t.Errorf("value = %v, want 1 (app.handlers→app.core)", res.Value)
	}
	// Forward reach from app.handlers: app.core, then app.db → 2 modules.
	want := "1 cross-module edge(s) from 1 changed file(s); forward reach 2 file(s)"
	if res.Display != want {
		t.Errorf("display = %q, want %q", res.Display, want)
	}
}

// TestChangeLocality_PythonPackageInit maps an "app/db/__init__.py" change to
// the "module:app.db" node (package modules have no .py file of their own name).
func TestChangeLocality_PythonPackageInit(t *testing.T) {
	const (
		modHandlers = "module:app.handlers"
		modDB       = "module:app.db"
	)
	g := graph.Build([]graph.Facts{{
		Language: graph.LangPython,
		Nodes: []graph.Node{
			{Kind: graph.NodeKindModule, Path: "app.handlers"},
			{Kind: graph.NodeKindModule, Path: "app.db"},
		},
		Edges: []graph.Edge{
			{From: modDB, To: modHandlers, Kind: graph.EdgeKindImports, Language: graph.LangPython},
		},
	}})
	idx := coupling.Index{
		modDB + "\x00" + modHandlers + "\x00imports": {
			Strength: coupling.StrengthFunctional, Distance: coupling.DistanceCrossModuleDiffOwner,
		},
	}
	m := boundary.ChangeLocalityMetric{}
	res := m.Calculate(signal.CommonInput{
		Graph:           g,
		Classifications: idx,
		ChangedFiles:    []string{"app/db/__init__.py"},
	})
	if res.Value != 1 {
		t.Errorf("value = %v, want 1 (app.db→app.handlers via __init__.py)", res.Value)
	}
}

// TestChangeLocality_PythonSrcLayout is the ccgram regression: a src-layout
// project reports changed files as "src/app/handlers.py", but grimp's module
// node drops the source root ("module:app.handlers"). Stripping the single
// leading source-root segment must bridge the two so the metric is not a false
// zero on real src-layout repos.
func TestChangeLocality_PythonSrcLayout(t *testing.T) {
	m := boundary.ChangeLocalityMetric{}
	res := m.Calculate(signal.CommonInput{
		Graph:           pyGraph(),
		Classifications: pyIndex(),
		ChangedFiles:    []string{"src/app/handlers.py"},
	})
	if res.Value != 1 {
		t.Errorf("value = %v, want 1 (src/app/handlers.py → module:app.handlers)", res.Value)
	}
	want := "1 cross-module edge(s) from 1 changed file(s); forward reach 2 file(s)"
	if res.Display != want {
		t.Errorf("display = %q, want %q", res.Display, want)
	}
}

// TestChangeLocality_NoBasenameOverMatch guards the source-root stripping: a file
// that shares only the basename of a module ("lib/handlers.py" vs module
// "app.handlers") must NOT be attributed to that module. Stripping never
// collapses a path to a bare basename, so this stays a genuine 0.
func TestChangeLocality_NoBasenameOverMatch(t *testing.T) {
	m := boundary.ChangeLocalityMetric{}
	res := m.Calculate(signal.CommonInput{
		Graph:           pyGraph(),
		Classifications: pyIndex(),
		ChangedFiles:    []string{"lib/handlers.py"},
	})
	if res.Value != 0 {
		t.Errorf("value = %v, want 0 (lib/handlers.py must not match module:app.handlers)", res.Value)
	}
}

// TestChangeLocality_TestMirrorNoOverMatch guards the source-root stripping
// against test mirrors: only "src/" is an authoritative root, so a changed
// "tests/app/handlers.py" must NOT be attributed to module "app.handlers".
// Stripping any leading segment would over-match and inflate the count.
func TestChangeLocality_TestMirrorNoOverMatch(t *testing.T) {
	m := boundary.ChangeLocalityMetric{}
	res := m.Calculate(signal.CommonInput{
		Graph:           pyGraph(),
		Classifications: pyIndex(),
		ChangedFiles:    []string{"tests/app/handlers.py"},
	})
	if res.Value != 0 {
		t.Errorf("value = %v, want 0 (tests/app/handlers.py must not match module:app.handlers)", res.Value)
	}
}

// TestChangeLocality_TSFileNodeIDs confirms the file-path scheme (Go/TS) still
// resolves after the rework.
func TestChangeLocality_TSFileNodeIDs(t *testing.T) {
	const (
		tsA = "file:src/a.ts"
		tsB = "file:src/b.ts"
	)
	g := graph.Build([]graph.Facts{{
		Language: graph.LangTypeScript,
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: "src/a.ts"},
			{Kind: graph.NodeKindFile, Path: "src/b.ts"},
		},
		Edges: []graph.Edge{
			{From: tsA, To: tsB, Kind: graph.EdgeKindImports, Language: graph.LangTypeScript},
		},
	}})
	idx := coupling.Index{
		tsA + "\x00" + tsB + "\x00imports": {
			Strength: coupling.StrengthFunctional, Distance: coupling.DistanceCrossModuleDiffOwner,
		},
	}
	m := boundary.ChangeLocalityMetric{}
	res := m.Calculate(signal.CommonInput{
		Graph:           g,
		Classifications: idx,
		ChangedFiles:    []string{"src/a.ts"},
	})
	if res.Value != 1 {
		t.Errorf("value = %v, want 1 (src/a.ts→src/b.ts)", res.Value)
	}
	if want := "1 cross-module edge(s) from 1 changed file(s); forward reach 1 file(s)"; res.Display != want {
		t.Errorf("display = %q, want %q", res.Display, want)
	}
}

// TestChangeLocality_ChangedFileNoEdges distinguishes a genuine 0 from the old
// false 0. The changed file maps to a real node (module:app.db) that is a leaf —
// it has incoming edges but no outgoing ones — so 0 cross-edges/0 reach is
// correct, unlike the scheme-mismatch bug which zeroed every Python change.
func TestChangeLocality_ChangedFileNoEdges(t *testing.T) {
	m := boundary.ChangeLocalityMetric{}
	res := m.Calculate(signal.CommonInput{
		Graph:           pyGraph(),
		Classifications: pyIndex(),
		ChangedFiles:    []string{"app/db.py"}, // resolves to leaf module:app.db
	})
	if res.Value != 0 {
		t.Errorf("value = %v, want 0 (leaf module: no outgoing edges)", res.Value)
	}
	if want := "0 cross-module edge(s) from 1 changed file(s); forward reach 0 file(s)"; res.Display != want {
		t.Errorf("display = %q, want %q", res.Display, want)
	}
	if res.Confidence != "high" {
		t.Errorf("confidence = %q, want high (classifications present, genuine 0)", res.Confidence)
	}
}

// TestChangeLocality_UnmatchedChangedFile covers a changed file that maps to no
// graph node at all (not part of the import graph). It contributes nothing, but
// the metric still reports against the full changed-file count — no panic, no
// false positive.
func TestChangeLocality_UnmatchedChangedFile(t *testing.T) {
	m := boundary.ChangeLocalityMetric{}
	res := m.Calculate(signal.CommonInput{
		Graph:           pyGraph(),
		Classifications: pyIndex(),
		ChangedFiles:    []string{"README.md"},
	})
	if res.Value != 0 {
		t.Errorf("value = %v, want 0 (changed file not in graph)", res.Value)
	}
	if want := "0 cross-module edge(s) from 1 changed file(s); forward reach 0 file(s)"; res.Display != want {
		t.Errorf("display = %q, want %q", res.Display, want)
	}
}

func TestChangeLocality_Deterministic(t *testing.T) {
	m := boundary.ChangeLocalityMetric{}
	in := signal.CommonInput{
		Graph:           clGraph(),
		Classifications: clIndex(),
		ChangedFiles:    []string{clFileA, clFileB},
	}
	first := m.Calculate(in)
	second := m.Calculate(in)
	if first.Display != second.Display || first.Value != second.Value {
		t.Errorf("two runs differ: %q vs %q", first.Display, second.Display)
	}
	// Both a and b changed: a→b + b→c = 2 cross-module edges.
	if first.Value != 2 {
		t.Errorf("value = %v, want 2", first.Value)
	}
}
