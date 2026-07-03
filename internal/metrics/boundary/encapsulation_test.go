package boundary_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/boundary"
	"github.com/alexei-led/archfit/internal/metrics/metricstest"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/signal"
)

// Node path constants used across boundary metric tests.
const (
	pathA = "pkg/a/a.go"
	pathB = "pkg/b/b.go"
	pathC = "pkg/c/c.go"
	pathD = "pkg/d/d.go"
	pathE = "pkg/e/e.go"
)

// Band literals reused across boundary metric tests (deduplicated for goconst).
const bandCritical = "critical"

// Band/confidence string constants used across boundary metric tests.
const (
	bandMixed = "mixed"
	bandNAStr = "n/a"
	confLow   = "low"
)

func TestEncapsulation_NoCrossBoundaryIsNA(t *testing.T) {
	// Dependency edges exist but none cross a module boundary (all same-module).
	// This is not earned encapsulation — it is absence of a boundary signal (often
	// just unclassified edge distance), so it must read n/a, never a vacuous 1.0 that
	// would let an unclassified module graph look perfectly encapsulated.
	nodeA := graph.Node{Kind: graph.NodeKindFile, Path: pathA}
	nodeB := graph.Node{Kind: graph.NodeKindFile, Path: pathB}
	edge := graph.Edge{From: nodeA.ID(), To: nodeB.ID(), Kind: graph.EdgeKindImports}
	g := metricstest.BuildGraph([]graph.Node{nodeA, nodeB}, []graph.Edge{edge})
	idx := coupling.Index{
		metricstest.ImportKey(nodeA.ID(), nodeB.ID()): coupling.Classification{
			Strength: coupling.StrengthContract,
			Distance: coupling.DistanceSameModule,
		},
	}
	m := boundary.EncapsulationMetric{}
	result := m.Calculate(signal.CommonInput{Graph: g, Classifications: idx})
	if result.Band != "n/a" {
		t.Errorf("expected n/a band, got %q (value %v)", result.Band, result.Value)
	}
	if result.Value == 1.0 {
		t.Errorf("no cross-boundary edges must not yield value 1.0 (false-green)")
	}
}

func TestEncapsulation_EmptyGraphIsNA(t *testing.T) {
	// A graph with nodes but no dependency edges is absence of evidence, not an
	// encapsulated codebase — report n/a, never a false-green 1.0.
	g := metricstest.BuildGraph(
		[]graph.Node{{Kind: graph.NodeKindFile, Path: pathA}},
		nil,
	)
	m := boundary.EncapsulationMetric{}
	result := m.Calculate(signal.CommonInput{Graph: g})
	if result.Band != bandNAStr {
		t.Errorf("expected band %q for empty graph, got %q", bandNAStr, result.Band)
	}
	if result.Value == 1.0 {
		t.Errorf("empty graph must not yield value 1.0 (false-green)")
	}
}

func TestEncapsulation_NilGraph(t *testing.T) {
	// A nil graph is absent evidence — n/a, not a false-green 1.0.
	m := boundary.EncapsulationMetric{}
	result := m.Calculate(signal.CommonInput{Graph: nil})
	if result.Band != bandNAStr {
		t.Errorf("expected band %q for nil graph, got %q", bandNAStr, result.Band)
	}
	if result.Value == 1.0 {
		t.Errorf("nil graph must not yield value 1.0 (false-green)")
	}
}

func TestEncapsulation_KnownRatio(t *testing.T) {
	// 1 contract + 1 intrusive cross-boundary edge → value 0.5
	nodeA := graph.Node{Kind: graph.NodeKindFile, Path: pathA}
	nodeB := graph.Node{Kind: graph.NodeKindFile, Path: pathB}
	nodeC := graph.Node{Kind: graph.NodeKindFile, Path: pathC}

	edgeAB := graph.Edge{From: nodeA.ID(), To: nodeB.ID(), Kind: graph.EdgeKindImports}
	edgeAC := graph.Edge{From: nodeA.ID(), To: nodeC.ID(), Kind: graph.EdgeKindImports}

	g := metricstest.BuildGraph([]graph.Node{nodeA, nodeB, nodeC}, []graph.Edge{edgeAB, edgeAC})

	idx := coupling.Index{
		metricstest.ImportKey(nodeA.ID(), nodeB.ID()): coupling.Classification{
			Strength: coupling.StrengthContract,
			Distance: coupling.DistanceCrossModuleDiffOwner,
		},
		metricstest.ImportKey(nodeA.ID(), nodeC.ID()): coupling.Classification{
			Strength: coupling.StrengthIntrusive,
			Distance: coupling.DistanceCrossModuleDiffOwner,
		},
	}

	m := boundary.EncapsulationMetric{}
	result := m.Calculate(signal.CommonInput{Graph: g, Classifications: idx})

	if !metricstest.ApproxEqual(result.Value, 0.5) {
		t.Errorf("expected value 0.5 got %v", result.Value)
	}
	// Score 5.0 → band "mixed"
	if result.Band != bandMixed {
		t.Errorf("expected band mixed got %q", result.Band)
	}
	// Direction drives computeVerdict's delta-sign handling (V1 fix): a ratio
	// metric regresses when it FALLS. A wrong stamp silently inverts gating.
	if result.Direction != diagnostic.DirectionHigherIsBetter {
		t.Errorf("direction = %q, want %q", result.Direction, diagnostic.DirectionHigherIsBetter)
	}
}

// TestEncapsulation_ZeroContractIsRealNotNA is the 6.3 regression: an
// all-intrusive, zero-contract boundary (reproduces prefect's real 0
// contract / 126 intrusive result) must score numerically — not fall back to
// n/a — since it has real structural evidence (intrusiveCross > 0), and the
// definition string must say so explicitly rather than leaving a 0.0/critical
// result to be misread as an unmeasured config gap.
func TestEncapsulation_ZeroContractIsRealNotNA(t *testing.T) {
	nodeA := graph.Node{Kind: graph.NodeKindFile, Path: pathA}
	nodeB := graph.Node{Kind: graph.NodeKindFile, Path: pathB}

	edgeAB := graph.Edge{From: nodeA.ID(), To: nodeB.ID(), Kind: graph.EdgeKindImports}
	g := metricstest.BuildGraph([]graph.Node{nodeA, nodeB}, []graph.Edge{edgeAB})

	idx := coupling.Index{
		metricstest.ImportKey(nodeA.ID(), nodeB.ID()): coupling.Classification{
			Strength: coupling.StrengthIntrusive,
			Distance: coupling.DistanceCrossModuleDiffOwner,
		},
	}

	m := boundary.EncapsulationMetric{}
	result := m.Calculate(signal.CommonInput{Graph: g, Classifications: idx})

	if result.Band == bandNAStr {
		t.Fatalf("expected a real numeric band, got n/a — real intrusive evidence must not be suppressed")
	}
	if !metricstest.ApproxEqual(result.Value, 0.0) {
		t.Errorf("expected value 0.0 (0 contract / 1 intrusive) got %v", result.Value)
	}
	if !strings.Contains(result.Definition, "not necessarily a config gap") {
		t.Errorf("Definition = %q, want it to clarify 0 contract is a real measurement here", result.Definition)
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
	g := metricstest.BuildGraph([]graph.Node{nodeA, nodeB, nodeC, nodeD, nodeE}, edges)

	idx := coupling.Index{
		metricstest.ImportKey(nodeA.ID(), nodeB.ID()): coupling.Classification{
			Strength: coupling.StrengthContract,
			Distance: coupling.DistanceCrossModuleDiffOwner,
		},
		metricstest.ImportKey(nodeA.ID(), nodeC.ID()): coupling.Classification{
			Strength: coupling.StrengthContract,
			Distance: coupling.DistanceCrossModuleDiffOwner,
		},
		metricstest.ImportKey(nodeA.ID(), nodeD.ID()): coupling.Classification{
			Strength: coupling.StrengthContract,
			Distance: coupling.DistanceCrossModuleDiffOwner,
		},
		metricstest.ImportKey(nodeA.ID(), nodeE.ID()): coupling.Classification{
			Strength: coupling.StrengthIntrusive,
			Distance: coupling.DistanceCrossModuleDiffOwner,
		},
		metricstest.ImportKey(nodeB.ID(), nodeC.ID()): coupling.Classification{
			Strength: coupling.StrengthIntrusive,
			Distance: coupling.DistanceCrossModuleDiffOwner,
		},
	}

	m := boundary.EncapsulationMetric{}
	result := m.Calculate(signal.CommonInput{
		Graph:           g,
		Classifications: idx,
		Baseline:        baseline,
	})

	if !metricstest.ApproxEqual(result.Value, 0.6) {
		t.Errorf("expected value 0.6 got %v", result.Value)
	}
	if result.Delta == nil {
		t.Fatal("expected non-nil delta")
	}
	if !metricstest.ApproxEqual(*result.Delta, -0.2) {
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
	g := metricstest.BuildGraph([]graph.Node{nodeA, nodeB, nodeC}, edges)

	idx := coupling.Index{
		metricstest.ImportKey(nodeA.ID(), nodeB.ID()): coupling.Classification{
			Strength: coupling.StrengthUnknown,
			Distance: coupling.DistanceCrossModuleDiffOwner,
		},
		metricstest.ImportKey(nodeA.ID(), nodeC.ID()): coupling.Classification{
			Strength: coupling.StrengthUnknown,
			Distance: coupling.DistanceCrossModuleDiffOwner,
		},
	}

	m := boundary.EncapsulationMetric{}
	result := m.Calculate(signal.CommonInput{Graph: g, Classifications: idx})

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
	g := metricstest.BuildGraph([]graph.Node{nodeA, nodeB, nodeC, nodeD, nodeE}, edges)

	cross := coupling.DistanceCrossModuleDiffOwner
	idx := coupling.Index{
		metricstest.ImportKey(nodeA.ID(), nodeB.ID()): {Strength: coupling.StrengthContract, Distance: cross},
		metricstest.ImportKey(nodeA.ID(), nodeC.ID()): {Strength: coupling.StrengthIntrusive, Distance: cross},
		metricstest.ImportKey(nodeA.ID(), nodeD.ID()): {Strength: coupling.StrengthUnknown, Distance: cross},
		metricstest.ImportKey(nodeA.ID(), nodeE.ID()): {Strength: coupling.StrengthUnknown, Distance: cross},
	}

	m := boundary.EncapsulationMetric{}
	result := m.Calculate(signal.CommonInput{Graph: g, Classifications: idx})

	if !metricstest.ApproxEqual(result.Value, 0.5) {
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

func TestEncapsulation_DeclaredExternalExcludedFromDenominator(t *testing.T) {
	// 1 contract + 1 intrusive cross-boundary edge, plus 1 declared-external edge
	// (DistanceExternal). A vendor seam has no public/internal glob boundary to
	// honor, so it must not count toward the cross-boundary denominator: value
	// must stay 1/2 = 0.5, not 2/3 (which would count the external edge as
	// contract-classified boundary evidence).
	nodeA := graph.Node{Kind: graph.NodeKindFile, Path: pathA}
	nodeB := graph.Node{Kind: graph.NodeKindFile, Path: pathB}
	nodeC := graph.Node{Kind: graph.NodeKindFile, Path: pathC}
	nodeD := graph.Node{Kind: graph.NodeKindFile, Path: pathD}

	edges := []graph.Edge{
		{From: nodeA.ID(), To: nodeB.ID(), Kind: graph.EdgeKindImports},
		{From: nodeA.ID(), To: nodeC.ID(), Kind: graph.EdgeKindImports},
		{From: nodeA.ID(), To: nodeD.ID(), Kind: graph.EdgeKindImports},
	}
	g := metricstest.BuildGraph([]graph.Node{nodeA, nodeB, nodeC, nodeD}, edges)

	cross := coupling.DistanceCrossModuleDiffOwner
	idx := coupling.Index{
		metricstest.ImportKey(nodeA.ID(), nodeB.ID()): {Strength: coupling.StrengthContract, Distance: cross},
		metricstest.ImportKey(nodeA.ID(), nodeC.ID()): {Strength: coupling.StrengthIntrusive, Distance: cross},
		metricstest.ImportKey(nodeA.ID(), nodeD.ID()): {Strength: coupling.StrengthContract, Distance: coupling.DistanceExternal},
	}

	m := boundary.EncapsulationMetric{}
	result := m.Calculate(signal.CommonInput{Graph: g, Classifications: idx})

	if !metricstest.ApproxEqual(result.Value, 0.5) {
		t.Errorf("expected value 0.5 (declared-external excluded) got %v", result.Value)
	}
	if result.Confidence != "high" {
		t.Errorf("expected confidence high (2 of 2 real cross edges classified) got %q", result.Confidence)
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
	g := metricstest.BuildGraph(nodes, edges)

	cross := coupling.DistanceCrossModuleDiffOwner
	idx := coupling.Index{
		metricstest.ImportKey(from, nodes[1].ID()):          {Strength: coupling.StrengthContract, Distance: cross},
		metricstest.ImportKey(from, nodes[2].ID()):          {Strength: coupling.StrengthIntrusive, Distance: cross},
		metricstest.ImportKey(from, nodes[3].ID()):          {Strength: coupling.StrengthFunctional, Distance: cross},
		metricstest.ImportKey(from, nodes[4].ID()):          {Strength: coupling.StrengthFunctional, Distance: cross},
		metricstest.ImportKey(nodes[1].ID(), nodes[4].ID()): {Strength: coupling.StrengthModel, Distance: cross},
	}

	m := boundary.EncapsulationMetric{}
	result := m.Calculate(signal.CommonInput{Graph: g, Classifications: idx})

	if !metricstest.ApproxEqual(result.Value, 0.5) {
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
		idx[metricstest.ImportKey(from, nodes[i].ID())] = coupling.Classification{Strength: st, Distance: cross}
	}

	g := metricstest.BuildGraph(nodes, edges)
	result := boundary.EncapsulationMetric{}.Calculate(signal.CommonInput{Graph: g, Classifications: idx})

	if !metricstest.ApproxEqual(result.Value, 0.75) {
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
	g := metricstest.BuildGraph([]graph.Node{nodeA, nodeB, nodeC}, edges)
	cross := coupling.DistanceCrossModuleDiffOwner
	idx := coupling.Index{
		metricstest.ImportKey(nodeA.ID(), nodeB.ID()): {Strength: coupling.StrengthContract, Distance: cross},
		metricstest.ImportKey(nodeA.ID(), nodeC.ID()): {Strength: coupling.StrengthContract, Distance: cross},
	}

	result := boundary.EncapsulationMetric{}.Calculate(signal.CommonInput{Graph: g, Classifications: idx})
	if result.Band != bandNAStr {
		t.Errorf("expected band n/a (no intrusive to contrast) got %q", result.Band)
	}
}
