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
	res := m.Calculate(signal.MetricInput{
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
	res := m.Calculate(signal.MetricInput{Graph: clGraph(), Classifications: clIndex()})
	if res.Band != "n/a" {
		t.Errorf("band = %q, want n/a in full mode (no changed files)", res.Band)
	}
}

func TestChangeLocality_LowConfidenceWithoutClassifications(t *testing.T) {
	m := boundary.ChangeLocalityMetric{}
	res := m.Calculate(signal.MetricInput{
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

func TestChangeLocality_Deterministic(t *testing.T) {
	m := boundary.ChangeLocalityMetric{}
	in := signal.MetricInput{
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
