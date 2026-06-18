package modularity_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/metricstest"
	"github.com/alexei-led/archfit/internal/metrics/modularity"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/signal"
)

// Hand-computed PC for the linear chain A→B→C (N=3):
//
//	reach(A) = 2  (A→B→C transitively)
//	reach(B) = 1  (B→C only)
//	reach(C) = 0  (no outgoing edges)
//	reachable_pairs = 3
//	denom = N*(N-1) = 3*2 = 6
//	PC = 3/6 = 0.5
const wantChainPC = 0.5

func TestComputePropagation_LinearChain(t *testing.T) {
	_, _, _, g := chainABC()

	reach, n := modularity.ComputePropagation(g)
	if n != 3 {
		t.Fatalf("n=%d want 3", n)
	}

	cases := []struct {
		path      string
		wantReach int
	}{
		{modPathA, 2}, // A reaches B and C
		{modPathB, 1}, // B reaches C only
		{modPathC, 0}, // C reaches nobody
	}
	for _, tc := range cases {
		got, ok := reach[tc.path]
		if !ok {
			t.Errorf("module %q missing from reach map", tc.path)
			continue
		}
		if got != tc.wantReach {
			t.Errorf("reach(%q)=%d want %d", tc.path, got, tc.wantReach)
		}
	}
}

// TestComputePropagation_FullyConnected verifies a 3-node fully-connected DAG:
// A→B, A→C, B→C. Reach: A=2, B=1, C=0. Same as linear chain = PC=0.5.
// (Adding A→C doesn't create new reachability beyond the chain.)
func TestComputePropagation_FullyConnectedDAG(t *testing.T) {
	a := graph.Node{Kind: graph.NodeKindModule, Path: modPathA}
	b := graph.Node{Kind: graph.NodeKindModule, Path: modPathB}
	c := graph.Node{Kind: graph.NodeKindModule, Path: modPathC}
	g := metricstest.BuildGraph([]graph.Node{a, b, c}, []graph.Edge{
		{From: a.ID(), To: b.ID(), Kind: graph.EdgeKindImports},
		{From: a.ID(), To: c.ID(), Kind: graph.EdgeKindImports},
		{From: b.ID(), To: c.ID(), Kind: graph.EdgeKindImports},
	})

	reach, n := modularity.ComputePropagation(g)
	if n != 3 {
		t.Fatalf("n=%d want 3", n)
	}
	if reach[modPathA] != 2 {
		t.Errorf("reach(A)=%d want 2", reach[modPathA])
	}
	if reach[modPathB] != 1 {
		t.Errorf("reach(B)=%d want 1", reach[modPathB])
	}
	if reach[modPathC] != 0 {
		t.Errorf("reach(C)=%d want 0", reach[modPathC])
	}
}

// TestComputePropagation_Disconnected verifies isolated modules contribute 0 reach.
// Graph: A→B only; C is isolated.
// reach(A)=1, reach(B)=0, reach(C)=0; reachable_pairs=1; denom=6; PC=1/6≈0.167.
func TestComputePropagation_Disconnected(t *testing.T) {
	a := graph.Node{Kind: graph.NodeKindModule, Path: modPathA}
	b := graph.Node{Kind: graph.NodeKindModule, Path: modPathB}
	c := graph.Node{Kind: graph.NodeKindModule, Path: modPathC}
	g := metricstest.BuildGraph([]graph.Node{a, b, c}, []graph.Edge{
		{From: a.ID(), To: b.ID(), Kind: graph.EdgeKindImports},
	})

	reach, n := modularity.ComputePropagation(g)
	if n != 3 {
		t.Fatalf("n=%d want 3", n)
	}
	if reach[modPathA] != 1 {
		t.Errorf("reach(A)=%d want 1", reach[modPathA])
	}
	if reach[modPathB] != 0 {
		t.Errorf("reach(B)=%d want 0", reach[modPathB])
	}
	if reach[modPathC] != 0 {
		t.Errorf("reach(C)=%d want 0 (isolated)", reach[modPathC])
	}
}

// TestComputePropagation_Cycle verifies a cycle A→B→A is handled without
// infinite loops. Both modules reach each other.
// Graph: A→B, B→A; reach(A)=1, reach(B)=1; reachable_pairs=2; denom=2; PC=1.0.
func TestComputePropagation_Cycle(t *testing.T) {
	a := graph.Node{Kind: graph.NodeKindModule, Path: modPathA}
	b := graph.Node{Kind: graph.NodeKindModule, Path: modPathB}
	g := metricstest.BuildGraph([]graph.Node{a, b}, []graph.Edge{
		{From: a.ID(), To: b.ID(), Kind: graph.EdgeKindImports},
		{From: b.ID(), To: a.ID(), Kind: graph.EdgeKindImports},
	})

	reach, n := modularity.ComputePropagation(g)
	if n != 2 {
		t.Fatalf("n=%d want 2", n)
	}
	if reach[modPathA] != 1 {
		t.Errorf("reach(A)=%d want 1", reach[modPathA])
	}
	if reach[modPathB] != 1 {
		t.Errorf("reach(B)=%d want 1", reach[modPathB])
	}
}

// ---------------------------------------------------------------------------
// PropagationCostMetric
// ---------------------------------------------------------------------------

// TestPropagationCostMetric_LinearChain checks band, name, value on A→B→C.
// PC = 0.5 (hand-computed above). N=3 < ModularitySmallN → low confidence.
// reach(A)=2 → per-mod PC=2/2=1.0 > 50% → A is a wide-blast contributor.
func TestPropagationCostMetric_LinearChain(t *testing.T) {
	_, _, _, g := chainABC()

	res := modularity.PropagationCostMetric{}.Calculate(signal.CommonInput{Graph: g})

	if res.Name != "propagation_cost" {
		t.Errorf("name=%q want propagation_cost", res.Name)
	}
	if res.Band != bandInfo {
		t.Errorf("band=%q want %s", res.Band, bandInfo)
	}
	if res.Confidence != confidenceLow {
		t.Errorf("confidence=%q want %s (N<15)", res.Confidence, confidenceLow)
	}
	if !metricstest.ApproxEqual(res.Value, wantChainPC) {
		t.Errorf("PC=%.4f want %.4f", res.Value, wantChainPC)
	}
	// A reaches 100% of remaining modules → must appear in display.
	if !strings.Contains(res.Display, modPathA) {
		t.Errorf("display should mention wide-blast %s; got %q", modPathA, res.Display)
	}
}

// TestPropagationCostMetric_NilGraph returns n/a for a nil graph.
func TestPropagationCostMetric_NilGraph(t *testing.T) {
	res := modularity.PropagationCostMetric{}.Calculate(signal.CommonInput{Graph: nil})
	if res.Band != bandNAStr {
		t.Errorf("band=%q want %s for nil graph", res.Band, bandNAStr)
	}
}

// TestPropagationCostMetric_SingleNode returns n/a when fewer than 2 modules exist.
func TestPropagationCostMetric_SingleNode(t *testing.T) {
	solo := graph.Node{Kind: graph.NodeKindModule, Path: "pkg.solo"}
	g := metricstest.BuildGraph([]graph.Node{solo}, nil)

	res := modularity.PropagationCostMetric{}.Calculate(signal.CommonInput{Graph: g})
	if res.Band != bandNAStr {
		t.Errorf("band=%q want %s for single-node graph", res.Band, bandNAStr)
	}
}

// TestPropagationCostMetric_NeverGates confirms band is always "info" (report-only).
func TestPropagationCostMetric_NeverGates(t *testing.T) {
	_, _, _, g := chainABC()
	res := modularity.PropagationCostMetric{}.Calculate(signal.CommonInput{Graph: g})
	if res.Band == "low" || res.Band == "medium" || res.Band == "high" || res.Band == "critical" {
		t.Errorf("band=%q: propagation_cost must never be a gate band", res.Band)
	}
}
