package boundary_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/metrics/boundary"
	assessmentresult "github.com/alexei-led/archfit/internal/assessment/result"
	signal "github.com/alexei-led/archfit/internal/assessment/signals"
	"github.com/alexei-led/archfit/internal/testutil/metricstest"
)

func TestCycle_NoCycles(t *testing.T) {
	// A→B, B→C (DAG, no cycle)
	nodeA := metricstest.Node{Kind: metricstest.NodeKindFile, Path: pathA}
	nodeB := metricstest.Node{Kind: metricstest.NodeKindFile, Path: pathB}
	nodeC := metricstest.Node{Kind: metricstest.NodeKindFile, Path: pathC}
	edges := []metricstest.Edge{
		{From: nodeA.ID(), To: nodeB.ID(), Kind: metricstest.EdgeKindImports},
		{From: nodeB.ID(), To: nodeC.ID(), Kind: metricstest.EdgeKindImports},
	}
	g := metricstest.NewFixture([]metricstest.Node{nodeA, nodeB, nodeC}, edges)

	m := boundary.CycleMetric{}
	result := m.Calculate(signal.CommonInput{Relationships: metricstest.Classify(g, nil)})

	if result.Value != 0 {
		t.Errorf("expected 0 cycles got %v", result.Value)
	}
	if result.Band != bandStrong {
		t.Errorf("expected band strong got %q", result.Band)
	}
}

func TestCycle_WithCycle(t *testing.T) {
	// A→B, B→A (cycle)
	nodeA := metricstest.Node{Kind: metricstest.NodeKindFile, Path: pathA}
	nodeB := metricstest.Node{Kind: metricstest.NodeKindFile, Path: pathB}
	edges := []metricstest.Edge{
		{From: nodeA.ID(), To: nodeB.ID(), Kind: metricstest.EdgeKindImports},
		{From: nodeB.ID(), To: nodeA.ID(), Kind: metricstest.EdgeKindImports},
	}
	g := metricstest.NewFixture([]metricstest.Node{nodeA, nodeB}, edges)

	m := boundary.CycleMetric{}
	result := m.Calculate(signal.CommonInput{Relationships: metricstest.Classify(g, nil)})

	if result.Value <= 0 {
		t.Errorf("expected cycle count > 0 got %v", result.Value)
	}
	if result.Band != bandCritical {
		t.Errorf("expected band critical got %q", result.Band)
	}
	// Direction drives computeVerdict's delta-sign handling (V1 fix): a count
	// metric regresses when it RISES. A wrong stamp silently inverts gating.
	if result.Direction != assessmentresult.DirectionHigherIsWorse {
		t.Errorf("direction = %q, want %q", result.Direction, assessmentresult.DirectionHigherIsWorse)
	}
}

// TestCycle_RustModuleCycleIsPoor: a cycle built only from Rust edges is an
// intra-crate module cycle (cargo forbids crate cycles), softened to poor.
func TestCycle_RustModuleCycleIsPoor(t *testing.T) {
	a := metricstest.Node{Kind: metricstest.NodeKindPackage, Path: "demo::a"}
	b := metricstest.Node{Kind: metricstest.NodeKindPackage, Path: "demo::b"}
	edges := []metricstest.Edge{
		{From: a.ID(), To: b.ID(), Kind: metricstest.EdgeKindDependsOn, Language: metricstest.LangRust},
		{From: b.ID(), To: a.ID(), Kind: metricstest.EdgeKindDependsOn, Language: metricstest.LangRust},
	}
	g := metricstest.NewFixture([]metricstest.Node{a, b}, edges)
	res := boundary.CycleMetric{}.Calculate(signal.CommonInput{Relationships: metricstest.Classify(g, nil)})
	if res.Value <= 0 {
		t.Fatalf("expected a cycle, got %v", res.Value)
	}
	if res.Band != "poor" {
		t.Errorf("a Rust module cycle should be poor, got %q", res.Band)
	}
}

// TestCycle_PolyglotGoCycleStaysCritical is the H-1 regression: a Go import cycle
// must stay critical even when Rust crate-dep edges dominate the graph by count.
// The old global-majority heuristic wrongly softened it to poor.
func TestCycle_PolyglotGoCycleStaysCritical(t *testing.T) {
	ga := metricstest.Node{Kind: metricstest.NodeKindFile, Path: "go/a.go"}
	gb := metricstest.Node{Kind: metricstest.NodeKindFile, Path: "go/b.go"}
	r1 := metricstest.Node{Kind: metricstest.NodeKindPackage, Path: "rc1"}
	r2 := metricstest.Node{Kind: metricstest.NodeKindPackage, Path: "rc2"}
	r3 := metricstest.Node{Kind: metricstest.NodeKindPackage, Path: "rc3"}
	ext := metricstest.Node{Kind: metricstest.NodeKindExternal, Path: "serde"}
	edges := []metricstest.Edge{
		// The only cycle: ga <-> gb (Go edges).
		{From: ga.ID(), To: gb.ID(), Kind: metricstest.EdgeKindImports, Language: metricstest.LangGo},
		{From: gb.ID(), To: ga.ID(), Kind: metricstest.EdgeKindImports, Language: metricstest.LangGo},
		// Non-cycling Rust edges that outnumber the Go ones.
		{From: r1.ID(), To: ext.ID(), Kind: metricstest.EdgeKindDependsOn, Language: metricstest.LangRust},
		{From: r2.ID(), To: ext.ID(), Kind: metricstest.EdgeKindDependsOn, Language: metricstest.LangRust},
		{From: r3.ID(), To: ext.ID(), Kind: metricstest.EdgeKindDependsOn, Language: metricstest.LangRust},
	}
	g := metricstest.NewFixture([]metricstest.Node{ga, gb, r1, r2, r3, ext}, edges)
	res := boundary.CycleMetric{}.Calculate(signal.CommonInput{Relationships: metricstest.Classify(g, nil)})
	if res.Value <= 0 {
		t.Fatalf("expected the Go cycle to be counted, got %v", res.Value)
	}
	if res.Band != bandCritical {
		t.Errorf("a Go import cycle must stay critical despite dominant Rust edges, got %q", res.Band)
	}
}
