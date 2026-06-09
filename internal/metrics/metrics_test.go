package metrics_test

import (
	"math"
	"strconv"
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

// Band/confidence string constants used across tests.
const (
	bandMixed = "mixed"
	bandNAStr = "n/a"
	confLow   = "low"
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
	if result.Band != bandMixed {
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

func TestEncapsulation_AllUnknownIsNA(t *testing.T) {
	// Cross-boundary edges exist but every strength is unknown (the common case
	// for a project with no public/internal API surface declared). The metric
	// must report n/a — never a high-confidence 0/critical.
	nodeA := graph.Node{Kind: graph.NodeKindFile, Path: pathA}
	nodeB := graph.Node{Kind: graph.NodeKindFile, Path: pathB}
	nodeC := graph.Node{Kind: graph.NodeKindFile, Path: pathC}

	edges := []graph.Edge{
		{From: nodeA.ID(), To: nodeB.ID(), Kind: graph.EdgeKindImports},
		{From: nodeA.ID(), To: nodeC.ID(), Kind: graph.EdgeKindImports},
	}
	g := buildGraph([]graph.Node{nodeA, nodeB, nodeC}, edges)

	idx := coupling.Index{
		importKey(nodeA.ID(), nodeB.ID()): coupling.Classification{
			Strength: coupling.StrengthUnknown,
			Distance: coupling.DistanceCrossModuleDiffOwner,
		},
		importKey(nodeA.ID(), nodeC.ID()): coupling.Classification{
			Strength: coupling.StrengthUnknown,
			Distance: coupling.DistanceCrossModuleDiffOwner,
		},
	}

	m := metrics.EncapsulationMetric{}
	result := m.Calculate(metrics.MetricInput{Graph: g, Classifications: idx})

	if result.Band != bandNAStr {
		t.Errorf("expected band n/a got %q", result.Band)
	}
	if result.Display != bandNAStr {
		t.Errorf("expected display n/a got %q", result.Display)
	}
	if result.Confidence != confLow {
		t.Errorf("expected confidence low got %q", result.Confidence)
	}
	if result.Delta != nil {
		t.Errorf("expected nil delta for n/a, got %v", *result.Delta)
	}
}

func TestEncapsulation_UnknownExcludedFromDenominator(t *testing.T) {
	// 1 contract + 1 intrusive + 2 unknown cross-boundary edges.
	// Unknown is absence of evidence: value must be 1/2 = 0.5 (over classified
	// edges only), not 1/4 = 0.25 (which would count unknowns against the score).
	nodeA := graph.Node{Kind: graph.NodeKindFile, Path: pathA}
	nodeB := graph.Node{Kind: graph.NodeKindFile, Path: pathB}
	nodeC := graph.Node{Kind: graph.NodeKindFile, Path: pathC}
	nodeD := graph.Node{Kind: graph.NodeKindFile, Path: pathD}
	nodeE := graph.Node{Kind: graph.NodeKindFile, Path: pathE}

	edges := []graph.Edge{
		{From: nodeA.ID(), To: nodeB.ID(), Kind: graph.EdgeKindImports},
		{From: nodeA.ID(), To: nodeC.ID(), Kind: graph.EdgeKindImports},
		{From: nodeA.ID(), To: nodeD.ID(), Kind: graph.EdgeKindImports},
		{From: nodeA.ID(), To: nodeE.ID(), Kind: graph.EdgeKindImports},
	}
	g := buildGraph([]graph.Node{nodeA, nodeB, nodeC, nodeD, nodeE}, edges)

	cross := coupling.DistanceCrossModuleDiffOwner
	idx := coupling.Index{
		importKey(nodeA.ID(), nodeB.ID()): {Strength: coupling.StrengthContract, Distance: cross},
		importKey(nodeA.ID(), nodeC.ID()): {Strength: coupling.StrengthIntrusive, Distance: cross},
		importKey(nodeA.ID(), nodeD.ID()): {Strength: coupling.StrengthUnknown, Distance: cross},
		importKey(nodeA.ID(), nodeE.ID()): {Strength: coupling.StrengthUnknown, Distance: cross},
	}

	m := metrics.EncapsulationMetric{}
	result := m.Calculate(metrics.MetricInput{Graph: g, Classifications: idx})

	if !approxEqual(result.Value, 0.5) {
		t.Errorf("expected value 0.5 (unknown excluded) got %v", result.Value)
	}
	// Only 2 of 4 cross edges classified → 50% coverage → medium confidence,
	// which caps the band at serviceable. value 0.5 → mixed (below the cap), so mixed.
	if result.Confidence != "medium" {
		t.Errorf("expected confidence medium (50%% classified) got %q", result.Confidence)
	}
	if result.Band != bandMixed {
		t.Errorf("expected band mixed got %q", result.Band)
	}
}

func TestEncapsulation_FunctionalAndModelExcluded(t *testing.T) {
	// 1 contract + 1 intrusive + 2 functional + 1 model cross-boundary edges.
	// Functional and model are normal public coupling, not boundary verdicts, so
	// they are excluded: value must be contract/(contract+intrusive) = 1/2 = 0.5,
	// not 1/5 = 0.2 (which would treat calling a public function as a leak).
	nodes := []graph.Node{
		{Kind: graph.NodeKindFile, Path: pathA},
		{Kind: graph.NodeKindFile, Path: pathB},
		{Kind: graph.NodeKindFile, Path: pathC},
		{Kind: graph.NodeKindFile, Path: pathD},
		{Kind: graph.NodeKindFile, Path: pathE},
	}
	from := nodes[0].ID()
	var edges []graph.Edge
	for i := 1; i < 5; i++ {
		edges = append(edges, graph.Edge{From: from, To: nodes[i].ID(), Kind: graph.EdgeKindImports})
	}
	// pathE reused as a 5th target via a second source to get a 5th edge.
	edges = append(edges, graph.Edge{From: nodes[1].ID(), To: nodes[4].ID(), Kind: graph.EdgeKindImports})
	g := buildGraph(nodes, edges)

	cross := coupling.DistanceCrossModuleDiffOwner
	idx := coupling.Index{
		importKey(from, nodes[1].ID()):          {Strength: coupling.StrengthContract, Distance: cross},
		importKey(from, nodes[2].ID()):          {Strength: coupling.StrengthIntrusive, Distance: cross},
		importKey(from, nodes[3].ID()):          {Strength: coupling.StrengthFunctional, Distance: cross},
		importKey(from, nodes[4].ID()):          {Strength: coupling.StrengthFunctional, Distance: cross},
		importKey(nodes[1].ID(), nodes[4].ID()): {Strength: coupling.StrengthModel, Distance: cross},
	}

	m := metrics.EncapsulationMetric{}
	result := m.Calculate(metrics.MetricInput{Graph: g, Classifications: idx})

	if !approxEqual(result.Value, 0.5) {
		t.Errorf("expected value 0.5 (functional/model excluded) got %v", result.Value)
	}
	// 2 of 5 cross edges take a boundary stance → 40% → low confidence → band capped to mixed.
	if result.Confidence != confLow {
		t.Errorf("expected confidence low got %q", result.Confidence)
	}
}

func TestEncapsulation_LowCoverageCapsGoodBand(t *testing.T) {
	// value 0.75 (3 contract + 1 intrusive → would be "serviceable"), but only 4 of
	// 10 cross edges are classified (40% coverage → low confidence). The band cap
	// must downgrade to mixed so the tool does not over-claim on thin evidence.
	const ncross = 10
	nodes := make([]graph.Node, ncross+1)
	for i := range nodes {
		nodes[i] = graph.Node{Kind: graph.NodeKindFile, Path: "pkg/m" + strconv.Itoa(i) + "/f.go"}
	}
	from := nodes[0].ID()
	cross := coupling.DistanceCrossModuleDiffOwner
	var edges []graph.Edge
	idx := coupling.Index{}
	for i := 1; i <= ncross; i++ {
		edges = append(edges, graph.Edge{From: from, To: nodes[i].ID(), Kind: graph.EdgeKindImports})
		var st coupling.Strength
		switch {
		case i <= 3:
			st = coupling.StrengthContract
		case i == 4:
			st = coupling.StrengthIntrusive
		default:
			st = coupling.StrengthUnknown
		}
		idx[importKey(from, nodes[i].ID())] = coupling.Classification{Strength: st, Distance: cross}
	}

	g := buildGraph(nodes, edges)
	result := metrics.EncapsulationMetric{}.Calculate(metrics.MetricInput{Graph: g, Classifications: idx})

	if !approxEqual(result.Value, 0.75) {
		t.Errorf("expected value 0.75 got %v", result.Value)
	}
	if result.Confidence != confLow {
		t.Errorf("expected confidence low (40%% classified) got %q", result.Confidence)
	}
	if result.Band != bandMixed {
		t.Errorf("expected band capped to mixed got %q", result.Band)
	}
}

func TestEncapsulation_NoIntrusiveIsNA(t *testing.T) {
	// Contract edges but ZERO intrusive: the ratio is trivially 1.0 and cannot tell
	// earned encapsulation from the compiler-boundary case (Go/TS, where every
	// cross-package import is forced through a public API). Report n/a, not 10/10.
	nodeA := graph.Node{Kind: graph.NodeKindFile, Path: pathA}
	nodeB := graph.Node{Kind: graph.NodeKindFile, Path: pathB}
	nodeC := graph.Node{Kind: graph.NodeKindFile, Path: pathC}
	edges := []graph.Edge{
		{From: nodeA.ID(), To: nodeB.ID(), Kind: graph.EdgeKindImports},
		{From: nodeA.ID(), To: nodeC.ID(), Kind: graph.EdgeKindImports},
	}
	g := buildGraph([]graph.Node{nodeA, nodeB, nodeC}, edges)
	cross := coupling.DistanceCrossModuleDiffOwner
	idx := coupling.Index{
		importKey(nodeA.ID(), nodeB.ID()): {Strength: coupling.StrengthContract, Distance: cross},
		importKey(nodeA.ID(), nodeC.ID()): {Strength: coupling.StrengthContract, Distance: cross},
	}

	result := metrics.EncapsulationMetric{}.Calculate(metrics.MetricInput{Graph: g, Classifications: idx})
	if result.Band != bandNAStr {
		t.Errorf("expected band n/a (no intrusive to contrast) got %q", result.Band)
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

func TestUnbalancedEdge_UnknownVolatilityIsNA(t *testing.T) {
	// An intrusive cross-module edge IS a candidate, but its volatility is unknown
	// (no churn data, no subdomain config). The high-volatility test cannot be
	// evaluated, so the metric is n/a — not a false "strong" (absence of evidence
	// is not evidence of balance).
	nodeA := graph.Node{Kind: graph.NodeKindFile, Path: pathA}
	nodeB := graph.Node{Kind: graph.NodeKindFile, Path: pathB}
	e := graph.Edge{From: nodeA.ID(), To: nodeB.ID(), Kind: graph.EdgeKindImports}
	g := buildGraph([]graph.Node{nodeA, nodeB}, []graph.Edge{e})

	idx := coupling.Index{
		importKey(nodeA.ID(), nodeB.ID()): coupling.Classification{
			Strength:   coupling.StrengthIntrusive,
			Distance:   coupling.DistanceCrossModuleDiffOwner,
			Volatility: coupling.VolatilityUnknown,
		},
	}

	m := metrics.UnbalancedEdgeMetric{}
	result := m.Calculate(metrics.MetricInput{Graph: g, Classifications: idx})

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

	if result.Confidence != confLow {
		t.Errorf("expected confidence low got %q", result.Confidence)
	}
	if result.Band != bandMixed {
		t.Errorf("expected band mixed (capped from strong) got %q", result.Band)
	}
}

// ---------------------------------------------------------------------------
// Metric interface and New()
// ---------------------------------------------------------------------------

func TestNew_ReturnsAllMetrics(t *testing.T) {
	ms := metrics.New(config.Config{})
	if len(ms) != 11 {
		t.Errorf("expected 11 metrics got %d", len(ms))
	}
	names := make(map[string]bool)
	for _, m := range ms {
		names[m.Name()] = true
	}
	for _, want := range []string{"encapsulation", "unbalanced_edge", "cycle", "coverage", "blast_radius", "change_amplification", "hidden_coupling", "structural_weight", "complexity", "risk_hub"} {
		if !names[want] {
			t.Errorf("missing metric %q", want)
		}
	}
}
