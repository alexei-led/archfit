package modularity_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/metrics/metricstest"
	"github.com/alexei-led/archfit/internal/metrics/modularity"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/signal"
)

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
	if res.Confidence != result.ConfidenceLow {
		t.Errorf("expected low confidence for small N, got %q", res.Confidence)
	}
	// Direction drives computeVerdict's delta-sign handling (V1 fix): a count
	// metric regresses when it RISES. A wrong stamp silently inverts gating.
	if res.Direction != diagnostic.DirectionHigherIsWorse {
		t.Errorf("direction = %q, want %q", res.Direction, diagnostic.DirectionHigherIsWorse)
	}
}
