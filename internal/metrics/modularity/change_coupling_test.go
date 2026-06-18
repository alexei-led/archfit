package modularity_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/metricstest"
	"github.com/alexei-led/archfit/internal/metrics/modularity"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/signal"
)

// Go package paths used by change_coupling tests (slash-separated, not the
// dot-separated module keys used by the module-kind nodes in martin_test.go).
const (
	ccModA = "pkg/a"
	ccModB = "pkg/b"
	ccModC = "pkg/c"
)

// ccGoFile returns the representative Go file path for a module key.
func ccGoFile(mod string) string { return mod + "/x.go" }

// buildCCInput constructs a HistoryInput from explicit module-level commit
// counts.  mods: ordered list of module paths; modChurn: module→commit-count;
// pairs: (modA,modB)→co-change-count.
// The graph uses language "go" edges so DominantLanguage returns "go".
func buildCCInput(mods []string, modChurn map[string]int, pairs map[[2]string]int) signal.HistoryInput {
	nodes := make([]graph.Node, len(mods))
	for i, m := range mods {
		nodes[i] = graph.Node{Kind: graph.NodeKindPackage, Path: m}
	}
	edges := make([]graph.Edge, 0, len(mods))
	for i := 1; i < len(mods); i++ {
		edges = append(edges, graph.Edge{
			From:     nodes[0].ID(),
			To:       nodes[i].ID(),
			Kind:     graph.EdgeKindImports,
			Language: "go",
		})
	}
	g := metricstest.BuildGraph(nodes, edges)

	fileChurn := make(map[string]int, len(modChurn))
	for mod, c := range modChurn {
		fileChurn[ccGoFile(mod)] = c
	}

	coChange := make(map[[2]string]int, len(pairs))
	for p, c := range pairs {
		a, b := ccGoFile(p[0]), ccGoFile(p[1])
		if a > b {
			a, b = b, a
		}
		coChange[[2]string{a, b}] = c
	}

	return signal.HistoryInput{
		CommonInput: signal.CommonInput{Graph: g},
		History: signal.HistorySignals{
			FileChurn: fileChurn,
			CoChange:  coChange,
		},
	}
}

// ---------------------------------------------------------------------------
// CC formula hand-computed test cases.
//
// CC(A,B) = C_AB / min(C_A, C_B)
//
//	Case 1: C_A=10, C_B=8,  C_AB=7  → CC=7/8=0.875  ≥0.65 → flagged
//	Case 2: C_A=10, C_B=10, C_AB=6  → CC=6/10=0.60  <0.65 → not flagged
//	Case 3: C_A=20, C_B=4,  C_AB=3  → CC=3/4=0.75   ≥0.65 → flagged (min wins)
//	Case 4: C_A=10, C_B=10, C_AB=0  → CC=0           → not flagged
// ---------------------------------------------------------------------------

// TestChangeCoupling_Formula_Flagged verifies CC ≥ 65% is flagged.
// C_A=10, C_B=8, C_AB=7 → CC=7/8=0.875 ≥ 0.65.
func TestChangeCoupling_Formula_Flagged(t *testing.T) {
	in := buildCCInput(
		[]string{ccModA, ccModB},
		map[string]int{ccModA: 10, ccModB: 8},
		map[[2]string]int{{ccModA, ccModB}: 7},
	)

	res := modularity.ChangeCouplingMetric{}.Calculate(in)

	if res.Name != "change_coupling" {
		t.Errorf("name=%q want change_coupling", res.Name)
	}
	if res.Band != bandInfo {
		t.Errorf("band=%q want %s", res.Band, bandInfo)
	}
	if res.Value != 1 {
		t.Errorf("value=%.0f want 1 (one flagged pair)", res.Value)
	}
	if !strings.Contains(res.Display, "CC=") {
		t.Errorf("display should show CC value; got %q", res.Display)
	}
}

// TestChangeCoupling_Formula_BelowThreshold verifies CC < 65% is not flagged.
// C_A=10, C_B=10, C_AB=6 → CC=6/10=0.60 < 0.65.
func TestChangeCoupling_Formula_BelowThreshold(t *testing.T) {
	in := buildCCInput(
		[]string{ccModA, ccModB},
		map[string]int{ccModA: 10, ccModB: 10},
		map[[2]string]int{{ccModA, ccModB}: 6},
	)

	res := modularity.ChangeCouplingMetric{}.Calculate(in)

	if res.Value != 0 {
		t.Errorf("value=%.0f want 0 (no pair reaches 65%%)", res.Value)
	}
}

// TestChangeCoupling_Formula_MinChurnDominates verifies min(C_A,C_B) is used.
// C_A=20, C_B=5, C_AB=4 → CC=4/min(20,5)=4/5=0.80 ≥ 0.65 (not 4/20=0.20).
// C_AB=4 satisfies the minimum-support requirement.
func TestChangeCoupling_Formula_MinChurnDominates(t *testing.T) {
	in := buildCCInput(
		[]string{ccModA, ccModB},
		map[string]int{ccModA: 20, ccModB: 5},
		map[[2]string]int{{ccModA, ccModB}: 4},
	)

	res := modularity.ChangeCouplingMetric{}.Calculate(in)

	if res.Value != 1 {
		t.Errorf("value=%.0f want 1; min(20,5)=5 → CC=4/5=0.80 ≥ 0.65", res.Value)
	}
}

// TestChangeCoupling_ZeroCoChange verifies pairs with zero co-change are not flagged.
func TestChangeCoupling_ZeroCoChange(t *testing.T) {
	in := buildCCInput(
		[]string{ccModA, ccModB},
		map[string]int{ccModA: 10, ccModB: 10},
		map[[2]string]int{{ccModA, ccModB}: 0},
	)

	res := modularity.ChangeCouplingMetric{}.Calculate(in)

	if res.Value != 0 {
		t.Errorf("value=%.0f want 0 for zero co-change", res.Value)
	}
}

// TestChangeCoupling_BelowMinSupport verifies pairs with fewer than 4 co-change
// commits are skipped regardless of ratio (CC=1.0 but support=3 < 4 → skipped).
func TestChangeCoupling_BelowMinSupport(t *testing.T) {
	in := buildCCInput(
		[]string{ccModA, ccModB},
		map[string]int{ccModA: 3, ccModB: 3},
		map[[2]string]int{{ccModA, ccModB}: 3},
	)

	res := modularity.ChangeCouplingMetric{}.Calculate(in)

	if res.Value != 0 {
		t.Errorf("value=%.0f want 0; pair below minSupport=4 must be skipped", res.Value)
	}
}

// TestChangeCoupling_ExactThreshold verifies CC exactly at 65% is flagged (≥ not >).
// C_A=20, C_B=20, C_AB=13 → CC=13/20=0.65 → flagged.
func TestChangeCoupling_ExactThreshold(t *testing.T) {
	in := buildCCInput(
		[]string{ccModA, ccModB},
		map[string]int{ccModA: 20, ccModB: 20},
		map[[2]string]int{{ccModA, ccModB}: 13},
	)

	res := modularity.ChangeCouplingMetric{}.Calculate(in)

	if res.Value != 1 {
		t.Errorf("value=%.0f want 1; CC=13/20=0.65 is exactly at threshold → flagged", res.Value)
	}
}

// TestChangeCoupling_NilGraph returns n/a for a nil graph.
func TestChangeCoupling_NilGraph(t *testing.T) {
	res := modularity.ChangeCouplingMetric{}.Calculate(signal.HistoryInput{})
	if res.Band != bandNAStr {
		t.Errorf("band=%q want %s for nil graph", res.Band, bandNAStr)
	}
}

// TestChangeCoupling_NoCoChangeData returns n/a when history is empty.
func TestChangeCoupling_NoCoChangeData(t *testing.T) {
	a := graph.Node{Kind: graph.NodeKindModule, Path: modPathA}
	b := graph.Node{Kind: graph.NodeKindModule, Path: modPathB}
	g := metricstest.BuildGraph([]graph.Node{a, b}, []graph.Edge{
		{From: a.ID(), To: b.ID(), Kind: graph.EdgeKindImports, Language: "go"},
	})

	res := modularity.ChangeCouplingMetric{}.Calculate(signal.HistoryInput{
		CommonInput: signal.CommonInput{Graph: g},
		History:     signal.HistorySignals{},
	})
	if res.Band != bandNAStr {
		t.Errorf("band=%q want %s for empty history", res.Band, bandNAStr)
	}
}

// TestChangeCoupling_NeverGates confirms band is always "info" (report-only).
func TestChangeCoupling_NeverGates(t *testing.T) {
	in := buildCCInput(
		[]string{ccModA, ccModB},
		map[string]int{ccModA: 10, ccModB: 10},
		map[[2]string]int{{ccModA, ccModB}: 8},
	)

	res := modularity.ChangeCouplingMetric{}.Calculate(in)

	switch res.Band {
	case "low", "medium", "high", "critical":
		t.Errorf("band=%q: change_coupling must never be a gate band", res.Band)
	}
}

// TestChangeCoupling_MultiplePairs verifies correct count when several pairs are flagged.
// Three modules A,B,C: A↔B and A↔C both CC≥65%; B↔C below threshold.
func TestChangeCoupling_MultiplePairs(t *testing.T) {
	in := buildCCInput(
		[]string{ccModA, ccModB, ccModC},
		map[string]int{ccModA: 10, ccModB: 10, ccModC: 10},
		map[[2]string]int{
			{ccModA, ccModB}: 8, // CC=0.80 → flagged
			{ccModA, ccModC}: 7, // CC=0.70 → flagged
			{ccModB, ccModC}: 5, // CC=0.50 → not flagged
		},
	)

	res := modularity.ChangeCouplingMetric{}.Calculate(in)

	if res.Value != 2 {
		t.Errorf("value=%.0f want 2 flagged pairs (A↔B, A↔C)", res.Value)
	}
}
