package modularity_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/metricstest"
	"github.com/alexei-led/archfit/internal/metrics/modularity"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/signal"
)

const (
	modPathA      = "pkg.a"
	modPathB      = "pkg.b"
	modPathC      = "pkg.c"
	modPathX      = "pkg.x"
	confidenceLow = "low"
)

// chainABC builds the graph A→B→C and returns nodes a, b, c and the graph.
func chainABC() (graph.Node, graph.Node, graph.Node, *graph.Graph) {
	a := graph.Node{Kind: graph.NodeKindModule, Path: modPathA}
	b := graph.Node{Kind: graph.NodeKindModule, Path: modPathB}
	c := graph.Node{Kind: graph.NodeKindModule, Path: modPathC}
	g := metricstest.BuildGraph([]graph.Node{a, b, c}, []graph.Edge{
		{From: a.ID(), To: b.ID(), Kind: graph.EdgeKindImports},
		{From: b.ID(), To: c.ID(), Kind: graph.EdgeKindImports},
	})
	return a, b, c, g
}

// TestComputeInstability verifies the I=Ce/(Ca+Ce) formula on A→B→C.
// A: Ce=1 (imports B), Ca=0        → I=1.0
// B: Ce=1 (imports C), Ca=1 (A)    → I=0.5
// C: Ce=0,             Ca=1 (B)    → I=0.0
func TestComputeInstability(t *testing.T) {
	_, _, _, g := chainABC()
	inst := modularity.ComputeInstability(g)
	if inst == nil {
		t.Fatal("expected non-nil instability map")
	}

	cases := []struct {
		path string
		want float64
	}{
		{modPathA, 1.0},
		{modPathB, 0.5},
		{modPathC, 0.0},
	}
	for _, tc := range cases {
		got, ok := inst[tc.path]
		if !ok {
			t.Errorf("module %q missing from instability map", tc.path)
			continue
		}
		if !metricstest.ApproxEqual(got, tc.want) {
			t.Errorf("I(%q)=%.4f, want %.4f", tc.path, got, tc.want)
		}
	}
}

// TestInstabilityMetric_Basic verifies band, name, confidence, and display
// content on A→B→C. Only A (I=1.0) exceeds the 0.7 threshold.
func TestInstabilityMetric_Basic(t *testing.T) {
	_, _, _, g := chainABC()
	res := modularity.InstabilityMetric{}.Calculate(signal.CommonInput{Graph: g})

	if res.Name != "instability" {
		t.Errorf("name=%q want instability", res.Name)
	}
	if res.Band != bandInfo {
		t.Errorf("band=%q want %s", res.Band, bandInfo)
	}
	// 3 modules < ModularitySmallN (15) → low confidence
	if res.Confidence != confidenceLow {
		t.Errorf("confidence=%q want %s", res.Confidence, confidenceLow)
	}
	// Only pkg.a has I=1.0 > 0.7
	if res.Value != 1 {
		t.Errorf("value=%.0f want 1 (only %s is unstable)", res.Value, modPathA)
	}
	if !strings.Contains(res.Display, modPathA) {
		t.Errorf("display should mention %s; got %q", modPathA, res.Display)
	}
}

// TestInstabilityMetric_NilGraph verifies n/a is returned for a nil graph.
func TestInstabilityMetric_NilGraph(t *testing.T) {
	res := modularity.InstabilityMetric{}.Calculate(signal.CommonInput{Graph: nil})
	if res.Band != bandNAStr {
		t.Errorf("band=%q want %s for nil graph", res.Band, bandNAStr)
	}
}

// TestAbstractnessMetric_FromClassifications builds A→B→C with:
//   - A→B edge classified as StrengthContract
//   - B→C edge classified as StrengthModel
//
// B has 1 contract inbound (from A), 0 concrete inbound → A(B)=1.0 > 0.5
// C has 0 contract inbound, 1 concrete inbound (from B) → A(C)=0.0 (not flagged)
func TestAbstractnessMetric_FromClassifications(t *testing.T) {
	a, b, c, g := chainABC()

	idx := coupling.Index{
		metricstest.ImportKey(a.ID(), b.ID()): {Strength: coupling.StrengthContract},
		metricstest.ImportKey(b.ID(), c.ID()): {Strength: coupling.StrengthModel},
	}

	res := modularity.AbstractnessMetric{}.Calculate(signal.CommonInput{
		Graph:           g,
		Classifications: idx,
	})

	if res.Band != bandInfo {
		t.Errorf("band=%q want %s", res.Band, bandInfo)
	}
	// Always low confidence (proxy metric, not SCIP type kinds)
	if res.Confidence != confidenceLow {
		t.Errorf("confidence=%q want %s", res.Confidence, confidenceLow)
	}
	// pkg.b should be flagged (A=1.0); pkg.c should not (A=0.0)
	if res.Value != 1 {
		t.Errorf("value=%.0f want 1 (only %s is abstract)", res.Value, modPathB)
	}
	if !strings.Contains(res.Display, modPathB) {
		t.Errorf("display should mention %s; got %q", modPathB, res.Display)
	}
	if strings.Contains(res.Display, modPathC) {
		t.Errorf("display must not mention %s; got %q", modPathC, res.Display)
	}
}

// TestUnstableDependency verifies the Designite-smell check.
func TestUnstableDependency(t *testing.T) {
	inst := map[string]float64{
		modPathA: 0.3,
		modPathB: 0.8,
	}
	cases := []struct {
		from, to string
		want     bool
		label    string
	}{
		{modPathA, modPathB, true, "caller depends on more-unstable target"},
		{modPathB, modPathA, false, "caller more unstable than target"},
		{modPathA, modPathX, false, "to absent from map → conservative false"},
		{modPathX, modPathB, false, "from absent from map → conservative false"},
	}
	for _, tc := range cases {
		got := modularity.UnstableDependency(tc.from, tc.to, inst)
		if got != tc.want {
			t.Errorf("[%s] UnstableDependency(%q→%q)=%v want %v", tc.label, tc.from, tc.to, got, tc.want)
		}
	}
}

// TestMartinDistanceMetric_ZoneOfPain builds a graph where pkg.c has I≈0.0
// (stable: nobody imports it and no outgoing) — but wait, in a chain A→B→C,
// C is depended upon by B but has no outgoing. I(C)=Ce/(Ca+Ce)=0/1=0.
// With no classified inbound edges, A(C)=0. Dms = |0+0-1| = 1.0 > 0.5.
// So pkg.c is in the zone of pain.
func TestMartinDistanceMetric_ZoneOfPain(t *testing.T) {
	_, _, _, g := chainABC()
	// No classifications → A=0 for all modules.
	res := modularity.MartinDistanceMetric{}.Calculate(signal.CommonInput{Graph: g})

	if res.Band != bandInfo {
		t.Errorf("band=%q want %s", res.Band, bandInfo)
	}
	if res.Confidence != confidenceLow {
		t.Errorf("confidence=%q want %s", res.Confidence, confidenceLow)
	}
	// pkg.c: I=0.0, A=0.0, Dms=1.0 → zone of pain
	// pkg.b: I=0.5, A=0.0, Dms=0.5 → not > 0.5 (exactly on boundary, excluded)
	// pkg.a: I=1.0, A=0.0, Dms=0.0 → not flagged
	if res.Value < 1 {
		t.Errorf("value=%.0f want ≥1 (%s in zone of pain)", res.Value, modPathC)
	}
	if !strings.Contains(res.Display, modPathC) {
		t.Errorf("display should mention %s; got %q", modPathC, res.Display)
	}
}

// TestMartinDistance_SharedDTOTrap documents the shared-DTO trap: high Ca + low I
// is NOT automatically "good". A shared data hub reached via concrete coupling
// (model/functional/intrusive strength, not contract) has A≈0 and I≈0, giving
// Dms=|0+0-1|=1.0 — flagged as "zone of pain" (maximally rigid: stable yet concrete).
//
// By contrast, the same topology with CONTRACT-strength inbound edges has A=1.0 and
// I=0, giving Dms=|1+0-1|=0.0 — NOT flagged (correctly: stable+abstract is on the
// main sequence). This is why these metrics are report-only: raw Ca/Ce alone cannot
// determine whether stability is healthy — volatility and strength must moderate it.
func TestMartinDistance_SharedDTOTrap(t *testing.T) {
	const (
		modHub      = "pkg.hub"
		modContrHub = "pkg.contr_hub"
		modY        = "pkg.y"
		modZ        = "pkg.z"
	)

	// --- Concrete hub (shared DTO trap) ---
	// hub reached via model-strength: A=0, I=0 → Dms=1.0 → flagged (zone of pain).
	hub := graph.Node{Kind: graph.NodeKindModule, Path: modHub}
	x := graph.Node{Kind: graph.NodeKindModule, Path: modPathX}
	y := graph.Node{Kind: graph.NodeKindModule, Path: modY}
	z := graph.Node{Kind: graph.NodeKindModule, Path: modZ}
	g := metricstest.BuildGraph([]graph.Node{hub, x, y, z}, []graph.Edge{
		{From: x.ID(), To: hub.ID(), Kind: graph.EdgeKindImports},
		{From: y.ID(), To: hub.ID(), Kind: graph.EdgeKindImports},
		{From: z.ID(), To: hub.ID(), Kind: graph.EdgeKindImports},
	})

	// All inbound edges are model-strength (concrete) → A(hub)=0.0
	concreteIdx := coupling.Index{
		metricstest.ImportKey(x.ID(), hub.ID()): {Strength: coupling.StrengthModel},
		metricstest.ImportKey(y.ID(), hub.ID()): {Strength: coupling.StrengthModel},
		metricstest.ImportKey(z.ID(), hub.ID()): {Strength: coupling.StrengthModel},
	}
	res := modularity.MartinDistanceMetric{}.Calculate(signal.CommonInput{
		Graph:           g,
		Classifications: concreteIdx,
	})
	// hub: I=0 (Ca=3, Ce=0), A=0 (all model inbound), Dms=|0+0-1|=1.0 → FLAGGED.
	// Raw I says "stable" but Dms reveals rigidity (zone of pain). Report-only.
	if !strings.Contains(res.Display, modHub) {
		t.Errorf("concrete hub %s must appear in Dms display (Dms=1.0, zone-of-pain); got %q", modHub, res.Display)
	}
	if res.Band != bandInfo {
		t.Errorf("band=%q want %s (report-only, never gates)", res.Band, bandInfo)
	}

	// --- Contract hub (well-designed stable interface) ---
	// Same topology but all inbound are contract-strength → A=1.0, I=0 → Dms=0.0 → NOT flagged.
	ch := graph.Node{Kind: graph.NodeKindModule, Path: modContrHub}
	cx := graph.Node{Kind: graph.NodeKindModule, Path: "pkg.cx"}
	cy := graph.Node{Kind: graph.NodeKindModule, Path: "pkg.cy"}
	g2 := metricstest.BuildGraph([]graph.Node{ch, cx, cy}, []graph.Edge{
		{From: cx.ID(), To: ch.ID(), Kind: graph.EdgeKindImports},
		{From: cy.ID(), To: ch.ID(), Kind: graph.EdgeKindImports},
	})
	contractIdx := coupling.Index{
		metricstest.ImportKey(cx.ID(), ch.ID()): {Strength: coupling.StrengthContract},
		metricstest.ImportKey(cy.ID(), ch.ID()): {Strength: coupling.StrengthContract},
	}
	res2 := modularity.MartinDistanceMetric{}.Calculate(signal.CommonInput{
		Graph:           g2,
		Classifications: contractIdx,
	})
	// ch: I=0, A=1.0, Dms=|1+0-1|=0.0 → NOT flagged (correctly stable+abstract).
	if strings.Contains(res2.Display, modContrHub) {
		t.Errorf("contract hub %s must not appear in Dms display (Dms=0.0); got %q", modContrHub, res2.Display)
	}
	if res2.Band != bandInfo {
		t.Errorf("band=%q want %s", res2.Band, bandInfo)
	}
}
