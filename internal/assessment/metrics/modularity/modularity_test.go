package modularity_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/assessment/metrics/metricstest"
	"github.com/alexei-led/archfit/internal/assessment/metrics/modularity"
	assessmentresult "github.com/alexei-led/archfit/internal/assessment/result"
	signal "github.com/alexei-led/archfit/internal/assessment/signals"
	"github.com/alexei-led/archfit/internal/model/graph"
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
	if res.Direction != assessmentresult.DirectionHigherIsWorse {
		t.Errorf("direction = %q, want %q", res.Direction, assessmentresult.DirectionHigherIsWorse)
	}
}

// TestBlastRadius_NAResult covers the naResult path (nil graph): the n/a path
// must stamp Direction like the measured path does — an unset Direction
// silently falls into computeVerdict's default branch.
func TestBlastRadius_NAResult(t *testing.T) {
	res := modularity.BlastRadiusMetric{}.Calculate(signal.CommonInput{Graph: nil})
	if res.Band != result.BandNA {
		t.Errorf("expected band %q for nil graph, got %q", result.BandNA, res.Band)
	}
	if res.Direction != assessmentresult.DirectionHigherIsWorse {
		t.Errorf("n/a direction = %q, want %q", res.Direction, assessmentresult.DirectionHigherIsWorse)
	}
}
