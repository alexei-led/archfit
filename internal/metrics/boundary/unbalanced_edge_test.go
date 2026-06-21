package boundary_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/boundary"
	"github.com/alexei-led/archfit/internal/metrics/metricstest"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/signal"
)

func TestUnbalancedEdge_Count(t *testing.T) {
	// 1 edge: intrusive + cross_module_different_owner + high volatility → count 1
	nodeA := graph.Node{Kind: graph.NodeKindFile, Path: pathA}
	nodeB := graph.Node{Kind: graph.NodeKindFile, Path: pathB}
	e := graph.Edge{From: nodeA.ID(), To: nodeB.ID(), Kind: graph.EdgeKindImports}
	g := metricstest.BuildGraph([]graph.Node{nodeA, nodeB}, []graph.Edge{e})

	idx := coupling.Index{
		metricstest.ImportKey(nodeA.ID(), nodeB.ID()): coupling.Classification{
			Strength:   coupling.StrengthIntrusive,
			Distance:   coupling.DistanceCrossModuleDiffOwner,
			Volatility: coupling.VolatilityHigh,
		},
	}

	m := boundary.UnbalancedEdgeMetric{}
	result := m.Calculate(signal.CommonInput{Graph: g, Classifications: idx})

	if result.Value != 1 {
		t.Errorf("expected value 1 got %v", result.Value)
	}
	if result.Band != bandCritical {
		t.Errorf("expected band critical got %q", result.Band)
	}
}

func TestUnbalancedEdge_ZeroCount(t *testing.T) {
	// Contract edge → not counted
	nodeA := graph.Node{Kind: graph.NodeKindFile, Path: pathA}
	nodeB := graph.Node{Kind: graph.NodeKindFile, Path: pathB}
	e := graph.Edge{From: nodeA.ID(), To: nodeB.ID(), Kind: graph.EdgeKindImports}
	g := metricstest.BuildGraph([]graph.Node{nodeA, nodeB}, []graph.Edge{e})

	idx := coupling.Index{
		metricstest.ImportKey(nodeA.ID(), nodeB.ID()): coupling.Classification{
			Strength:   coupling.StrengthContract,
			Distance:   coupling.DistanceCrossModuleDiffOwner,
			Volatility: coupling.VolatilityHigh,
		},
	}

	m := boundary.UnbalancedEdgeMetric{}
	result := m.Calculate(signal.CommonInput{Graph: g, Classifications: idx})

	if result.Value != 0 {
		t.Errorf("expected value 0 got %v", result.Value)
	}
	if result.Band != "strong" {
		t.Errorf("expected band strong got %q", result.Band)
	}
}

func TestUnbalancedEdge_UnknownVolatilityIsNA(t *testing.T) {
	// An intrusive cross-module edge IS a candidate, but its volatility is unknown
	// (no churn data, no subdomain config). The high-volatility test cannot be
	// evaluated, so the metric is n/a — not a false "strong" (absence of evidence
	// is not evidence of balance).
	nodeA := graph.Node{Kind: graph.NodeKindFile, Path: pathA}
	nodeB := graph.Node{Kind: graph.NodeKindFile, Path: pathB}
	e := graph.Edge{From: nodeA.ID(), To: nodeB.ID(), Kind: graph.EdgeKindImports}
	g := metricstest.BuildGraph([]graph.Node{nodeA, nodeB}, []graph.Edge{e})

	idx := coupling.Index{
		metricstest.ImportKey(nodeA.ID(), nodeB.ID()): coupling.Classification{
			Strength:   coupling.StrengthIntrusive,
			Distance:   coupling.DistanceCrossModuleDiffOwner,
			Volatility: coupling.VolatilityUnknown,
		},
	}

	m := boundary.UnbalancedEdgeMetric{}
	result := m.Calculate(signal.CommonInput{Graph: g, Classifications: idx})

	if result.Band != bandNAStr {
		t.Errorf("expected band n/a got %q", result.Band)
	}
	if result.Confidence != confLow {
		t.Errorf("expected confidence low got %q", result.Confidence)
	}
	if result.Delta != nil {
		t.Errorf("expected nil delta for n/a, got %v", *result.Delta)
	}
}
