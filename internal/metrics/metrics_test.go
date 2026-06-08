package metrics_test

import (
	"math"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/metrics"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
)

// Node path constants used across tests.
const (
	pathA = "pkg/a/a.go"
	pathB = "pkg/b/b.go"
	pathC = "pkg/c/c.go"
	pathD = "pkg/d/d.go"
	pathE = "pkg/e/e.go"
)

// buildGraph builds a graph from a single Facts value for test convenience.
func buildGraph(nodes []graph.Node, edges []graph.Edge) *graph.Graph {
	return graph.Build([]graph.Facts{{Nodes: nodes, Edges: edges}})
}

// importKey returns the coupling index key for an imports edge.
func importKey(from, to string) string {
	return from + "\x00" + to + "\x00" + string(graph.EdgeKindImports)
}

// approxEqual reports whether two float64 values are within 1e-9 of each other.
func approxEqual(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9
}

// ---------------------------------------------------------------------------
// EncapsulationMetric
// ---------------------------------------------------------------------------

func TestEncapsulation_ZeroDenominator(t *testing.T) {
	// Graph with no cross-boundary edges → value must be 1.0, no panic.
	g := buildGraph(
		[]graph.Node{{Kind: graph.NodeKindFile, Path: pathA}},
		nil,
	)
	m := metrics.EncapsulationMetric{}
	result := m.Calculate(metrics.MetricInput{Graph: g})
	if result.Value != 1.0 {
		t.Errorf("expected value 1.0 got %v", result.Value)
	}
}

func TestEncapsulation_NilGraph(t *testing.T) {
	m := metrics.EncapsulationMetric{}
	result := m.Calculate(metrics.MetricInput{Graph: nil})
	if result.Value != 1.0 {
		t.Errorf("expected value 1.0 for nil graph, got %v", result.Value)
	}
}

func TestEncapsulation_KnownRatio(t *testing.T) {
	// 1 contract + 1 intrusive cross-boundary edge → value 0.5
	nodeA := graph.Node{Kind: graph.NodeKindFile, Path: pathA}
	nodeB := graph.Node{Kind: graph.NodeKindFile, Path: pathB}
	nodeC := graph.Node{Kind: graph.NodeKindFile, Path: pathC}

	edgeAB := graph.Edge{From: nodeA.ID(), To: nodeB.ID(), Kind: graph.EdgeKindImports}
	edgeAC := graph.Edge{From: nodeA.ID(), To: nodeC.ID(), Kind: graph.EdgeKindImports}

	g := buildGraph([]graph.Node{nodeA, nodeB, nodeC}, []graph.Edge{edgeAB, edgeAC})

	idx := coupling.Index{
		importKey(nodeA.ID(), nodeB.ID()): coupling.Classification{
			Strength: coupling.StrengthContract,
			Distance: coupling.DistanceCrossModuleDiffOwner,
		},
		importKey(nodeA.ID(), nodeC.ID()): coupling.Classification{
			Strength: coupling.StrengthIntrusive,
			Distance: coupling.DistanceCrossModuleDiffOwner,
		},
	}

	m := metrics.EncapsulationMetric{}
	result := m.Calculate(metrics.MetricInput{Graph: g, Classifications: idx})

	if !approxEqual(result.Value, 0.5) {
		t.Errorf("expected value 0.5 got %v", result.Value)
	}
	// Score 5.0 → band "mixed"
	if result.Band != "mixed" {
		t.Errorf("expected band mixed got %q", result.Band)
	}
}

func TestEncapsulation_DeltaComputed(t *testing.T) {
	// Baseline has encapsulation=0.8; current value=0.6 → delta=-0.2
	baseline := diagnostic.MetricSnapshot{
		"encapsulation": {Value: 0.8, Version: "encapsulation.v1"},
	}

	nodeA := graph.Node{Kind: graph.NodeKindFile, Path: pathA}
	nodeB := graph.Node{Kind: graph.NodeKindFile, Path: pathB}
	nodeC := graph.Node{Kind: graph.NodeKindFile, Path: pathC}
	nodeD := graph.Node{Kind: graph.NodeKindFile, Path: pathD}
	nodeE := graph.Node{Kind: graph.NodeKindFile, Path: pathE}

	// 3 contract + 2 intrusive = value 0.6
	edges := []graph.Edge{
		{From: nodeA.ID(), To: nodeB.ID(), Kind: graph.EdgeKindImports},
		{From: nodeA.ID(), To: nodeC.ID(), Kind: graph.EdgeKindImports},
		{From: nodeA.ID(), To: nodeD.ID(), Kind: graph.EdgeKindImports},
		{From: nodeA.ID(), To: nodeE.ID(), Kind: graph.EdgeKindImports},
		{From: nodeB.ID(), To: nodeC.ID(), Kind: graph.EdgeKindImports},
	}
	g := buildGraph([]graph.Node{nodeA, nodeB, nodeC, nodeD, nodeE}, edges)

	idx := coupling.Index{
		importKey(nodeA.ID(), nodeB.ID()): coupling.Classification{
			Strength: coupling.StrengthContract,
			Distance: coupling.DistanceCrossModuleDiffOwner,
		},
		importKey(nodeA.ID(), nodeC.ID()): coupling.Classification{
			Strength: coupling.StrengthContract,
			Distance: coupling.DistanceCrossModuleDiffOwner,
		},
		importKey(nodeA.ID(), nodeD.ID()): coupling.Classification{
			Strength: coupling.StrengthContract,
			Distance: coupling.DistanceCrossModuleDiffOwner,
		},
		importKey(nodeA.ID(), nodeE.ID()): coupling.Classification{
			Strength: coupling.StrengthIntrusive,
			Distance: coupling.DistanceCrossModuleDiffOwner,
		},
		importKey(nodeB.ID(), nodeC.ID()): coupling.Classification{
			Strength: coupling.StrengthIntrusive,
			Distance: coupling.DistanceCrossModuleDiffOwner,
		},
	}

	m := metrics.EncapsulationMetric{}
	result := m.Calculate(metrics.MetricInput{
		Graph:           g,
		Classifications: idx,
		Baseline:        baseline,
	})

	if !approxEqual(result.Value, 0.6) {
		t.Errorf("expected value 0.6 got %v", result.Value)
	}
	if result.Delta == nil {
		t.Fatal("expected non-nil delta")
	}
	if !approxEqual(*result.Delta, -0.2) {
		t.Errorf("expected delta -0.2 got %v", *result.Delta)
	}
}

// ---------------------------------------------------------------------------
// UnbalancedEdgeMetric
// ---------------------------------------------------------------------------

func TestUnbalancedEdge_Count(t *testing.T) {
	// 1 edge: intrusive + cross_module_different_owner + high volatility → count 1
	nodeA := graph.Node{Kind: graph.NodeKindFile, Path: pathA}
	nodeB := graph.Node{Kind: graph.NodeKindFile, Path: pathB}
	e := graph.Edge{From: nodeA.ID(), To: nodeB.ID(), Kind: graph.EdgeKindImports}
	g := buildGraph([]graph.Node{nodeA, nodeB}, []graph.Edge{e})

	idx := coupling.Index{
		importKey(nodeA.ID(), nodeB.ID()): coupling.Classification{
			Strength:   coupling.StrengthIntrusive,
			Distance:   coupling.DistanceCrossModuleDiffOwner,
			Volatility: coupling.VolatilityHigh,
		},
	}

	m := metrics.UnbalancedEdgeMetric{}
	result := m.Calculate(metrics.MetricInput{Graph: g, Classifications: idx})

	if result.Value != 1 {
		t.Errorf("expected value 1 got %v", result.Value)
	}
	if result.Band != "critical" {
		t.Errorf("expected band critical got %q", result.Band)
	}
}

func TestUnbalancedEdge_ZeroCount(t *testing.T) {
	// Contract edge → not counted
	nodeA := graph.Node{Kind: graph.NodeKindFile, Path: pathA}
	nodeB := graph.Node{Kind: graph.NodeKindFile, Path: pathB}
	e := graph.Edge{From: nodeA.ID(), To: nodeB.ID(), Kind: graph.EdgeKindImports}
	g := buildGraph([]graph.Node{nodeA, nodeB}, []graph.Edge{e})

	idx := coupling.Index{
		importKey(nodeA.ID(), nodeB.ID()): coupling.Classification{
			Strength:   coupling.StrengthContract,
			Distance:   coupling.DistanceCrossModuleDiffOwner,
			Volatility: coupling.VolatilityHigh,
		},
	}

	m := metrics.UnbalancedEdgeMetric{}
	result := m.Calculate(metrics.MetricInput{Graph: g, Classifications: idx})

	if result.Value != 0 {
		t.Errorf("expected value 0 got %v", result.Value)
	}
	if result.Band != "strong" {
		t.Errorf("expected band strong got %q", result.Band)
	}
}

// ---------------------------------------------------------------------------
// CycleMetric
// ---------------------------------------------------------------------------

func TestCycle_NoCycles(t *testing.T) {
	// A→B, B→C (DAG, no cycle)
	nodeA := graph.Node{Kind: graph.NodeKindFile, Path: pathA}
	nodeB := graph.Node{Kind: graph.NodeKindFile, Path: pathB}
	nodeC := graph.Node{Kind: graph.NodeKindFile, Path: pathC}
	edges := []graph.Edge{
		{From: nodeA.ID(), To: nodeB.ID(), Kind: graph.EdgeKindImports},
		{From: nodeB.ID(), To: nodeC.ID(), Kind: graph.EdgeKindImports},
	}
	g := buildGraph([]graph.Node{nodeA, nodeB, nodeC}, edges)

	m := metrics.CycleMetric{}
	result := m.Calculate(metrics.MetricInput{Graph: g})

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
	g := buildGraph([]graph.Node{nodeA, nodeB}, edges)

	m := metrics.CycleMetric{}
	result := m.Calculate(metrics.MetricInput{Graph: g})

	if result.Value <= 0 {
		t.Errorf("expected cycle count > 0 got %v", result.Value)
	}
	if result.Band != "critical" {
		t.Errorf("expected band critical got %q", result.Band)
	}
}

// ---------------------------------------------------------------------------
// CoverageMetric
// ---------------------------------------------------------------------------

func TestCoverage_Ratio(t *testing.T) {
	// FilesSeen=8, FilesApplicable=10 → value 0.8
	m := metrics.CoverageMetric{}
	result := m.Calculate(metrics.MetricInput{
		ToolCoverage: []diagnostic.Coverage{
			{FilesSeen: 8, FilesApplicable: 10},
		},
	})

	if !approxEqual(result.Value, 0.8) {
		t.Errorf("expected value 0.8 got %v", result.Value)
	}
}

func TestCoverage_ZeroApplicable(t *testing.T) {
	m := metrics.CoverageMetric{}
	result := m.Calculate(metrics.MetricInput{
		ToolCoverage: []diagnostic.Coverage{
			{FilesSeen: 0, FilesApplicable: 0},
		},
	})
	if result.Value != 1.0 {
		t.Errorf("expected value 1.0 for zero applicable, got %v", result.Value)
	}
}

// ---------------------------------------------------------------------------
// Band model and confidence cap
// ---------------------------------------------------------------------------

func TestBandModel_LowConfidenceCap(t *testing.T) {
	// FilesSeen=10, FilesApplicable=10, Unresolved=9 → ratio=0.9 → low confidence
	// value = 10/10 = 1.0 → score 10 → band would be "strong" → capped to "mixed"
	m := metrics.CoverageMetric{}
	result := m.Calculate(metrics.MetricInput{
		ToolCoverage: []diagnostic.Coverage{
			{FilesSeen: 10, FilesApplicable: 10, Unresolved: 9},
		},
	})

	if result.Confidence != "low" {
		t.Errorf("expected confidence low got %q", result.Confidence)
	}
	if result.Band != "mixed" {
		t.Errorf("expected band mixed (capped from strong) got %q", result.Band)
	}
}

// ---------------------------------------------------------------------------
// Metric interface and New()
// ---------------------------------------------------------------------------

func TestNew_ReturnsAllMetrics(t *testing.T) {
	ms := metrics.New(config.Config{})
	if len(ms) != 4 {
		t.Errorf("expected 4 metrics got %d", len(ms))
	}
	names := make(map[string]bool)
	for _, m := range ms {
		names[m.Name()] = true
	}
	for _, want := range []string{"encapsulation", "unbalanced_edge", "cycle", "coverage"} {
		if !names[want] {
			t.Errorf("missing metric %q", want)
		}
	}
}
