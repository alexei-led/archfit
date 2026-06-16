package boundary_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/boundary"
	"github.com/alexei-led/archfit/internal/metrics/metricstest"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/signal"
)

func TestCycle_NoCycles(t *testing.T) {
	// A→B, B→C (DAG, no cycle)
	nodeA := graph.Node{Kind: graph.NodeKindFile, Path: pathA}
	nodeB := graph.Node{Kind: graph.NodeKindFile, Path: pathB}
	nodeC := graph.Node{Kind: graph.NodeKindFile, Path: pathC}
	edges := []graph.Edge{
		{From: nodeA.ID(), To: nodeB.ID(), Kind: graph.EdgeKindImports},
		{From: nodeB.ID(), To: nodeC.ID(), Kind: graph.EdgeKindImports},
	}
	g := metricstest.BuildGraph([]graph.Node{nodeA, nodeB, nodeC}, edges)

	m := boundary.CycleMetric{}
	result := m.Calculate(signal.CommonInput{Graph: g})

	if result.Value != 0 {
		t.Errorf("expected 0 cycles got %v", result.Value)
	}
	if result.Band != "strong" {
		t.Errorf("expected band strong got %q", result.Band)
	}
}

func TestCycle_WithCycle(t *testing.T) {
	// A→B, B→A (cycle)
	nodeA := graph.Node{Kind: graph.NodeKindFile, Path: pathA}
	nodeB := graph.Node{Kind: graph.NodeKindFile, Path: pathB}
	edges := []graph.Edge{
		{From: nodeA.ID(), To: nodeB.ID(), Kind: graph.EdgeKindImports},
		{From: nodeB.ID(), To: nodeA.ID(), Kind: graph.EdgeKindImports},
	}
	g := metricstest.BuildGraph([]graph.Node{nodeA, nodeB}, edges)

	m := boundary.CycleMetric{}
	result := m.Calculate(signal.CommonInput{Graph: g})

	if result.Value <= 0 {
		t.Errorf("expected cycle count > 0 got %v", result.Value)
	}
	if result.Band != "critical" {
		t.Errorf("expected band critical got %q", result.Band)
	}
}
