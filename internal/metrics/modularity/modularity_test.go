package modularity_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/metricstest"
	"github.com/alexei-led/archfit/internal/metrics/modularity"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/signal"
)

const bandNAStr = "n/a"

// Chain A -> B -> C (A imports B, B imports C). Reverse-deps: C has {A,B}=2,
// B has {A}=1, A has 0.
func TestBlastRadius_TransitiveReverseDeps(t *testing.T) {
	a := graph.Node{Kind: graph.NodeKindModule, Path: "pkg.a"}
	b := graph.Node{Kind: graph.NodeKindModule, Path: "pkg.b"}
	c := graph.Node{Kind: graph.NodeKindModule, Path: "pkg.c"}
	edges := []graph.Edge{
		{From: a.ID(), To: b.ID(), Kind: graph.EdgeKindImports},
		{From: b.ID(), To: c.ID(), Kind: graph.EdgeKindImports},
	}
	g := metricstest.BuildGraph([]graph.Node{a, b, c}, edges)

	res := modularity.BlastRadiusMetric{}.Calculate(signal.CommonInput{Graph: g})
	if res.Name != "blast_radius" {
		t.Fatalf("name = %q", res.Name)
	}
	// 3 modules, threshold 30%: C reaches 2/2=100% -> a hub; B 1/2=50% -> a hub; A 0.
	if res.Value != 2 {
		t.Errorf("expected 2 hubs (C,B) got %v; display=%q", res.Value, res.Display)
	}
	// small N -> low confidence (not a quality verdict), never gating.
	if res.Confidence != confidenceLow {
		t.Errorf("expected low confidence for small N, got %q", res.Confidence)
	}
}

func TestStructuralWeight_GodModuleBySize(t *testing.T) {
	// Go graph (sets language) + per-file LOC: one module far over 4x the median
	// and the absolute floor is a god-module; a large-but-cohesive core just under
	// the multiple is not.
	a := graph.Node{Kind: graph.NodeKindPackage, Path: "internal/big"}
	b := graph.Node{Kind: graph.NodeKindPackage, Path: "internal/small"}
	g := metricstest.BuildGraph([]graph.Node{a, b}, []graph.Edge{
		{From: a.ID(), To: b.ID(), Kind: graph.EdgeKindImports, Language: "go"},
	})
	fileLOC := map[string]int{
		"internal/big/a.go":   1600, // 1600 LOC -> god (>=4x median 200, >=400 floor)
		"internal/small/a.go": 200,
		"internal/small/b.go": 150,
		"internal/mid/a.go":   250,
	}

	res := modularity.StructuralWeightMetric{}.Calculate(signal.SizeInput{
		CommonInput: signal.CommonInput{Graph: g},
		Size:        signal.SizeSignals{FileLOC: fileLOC},
	})
	if res.Value != 1 {
		t.Errorf("expected 1 god-module got %v; display=%q", res.Value, res.Display)
	}
	if !strings.Contains(res.Display, "internal/big") {
		t.Errorf("expected internal/big flagged; display=%q", res.Display)
	}
	if strings.Contains(res.Display, "internal/small") || strings.Contains(res.Display, "internal/mid") {
		t.Errorf("small/mid modules must not be god-modules; display=%q", res.Display)
	}
}

func TestStructuralWeight_NoLOCIsNA(t *testing.T) {
	g := metricstest.BuildGraph([]graph.Node{{Kind: graph.NodeKindPackage, Path: "x"}}, nil)
	res := modularity.StructuralWeightMetric{}.Calculate(signal.SizeInput{
		CommonInput: signal.CommonInput{Graph: g},
	})
	if res.Band != bandNAStr {
		t.Errorf("expected n/a without LOC data, got %q", res.Band)
	}
}
