package boundary_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/metrics/boundary"
	assessmentresult "github.com/alexei-led/archfit/internal/assessment/result"
	signal "github.com/alexei-led/archfit/internal/assessment/signals"
	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/testutil/metricstest"
)

func TestUnbalancedEdge_Count(t *testing.T) {
	// 1 edge: intrusive + cross_module_different_owner + high volatility → count 1
	nodeA := metricstest.Node{Kind: metricstest.NodeKindFile, Path: pathA}
	nodeB := metricstest.Node{Kind: metricstest.NodeKindFile, Path: pathB}
	e := metricstest.Edge{From: nodeA.ID(), To: nodeB.ID(), Kind: metricstest.EdgeKindImports}

	idx := metricstest.Index{
		metricstest.ImportKey(nodeA.ID(), nodeB.ID()): metricstest.Classification{
			Strength:   relationship.StrengthIntrusive,
			Distance:   relationship.DistanceCrossModuleDiffOwner,
			Volatility: relationship.VolatilityHigh,
		},
	}

	m := boundary.UnbalancedEdgeMetric{}
	result := m.Calculate(signal.CommonInput{Relationships: metricstest.BuildRelationships([]metricstest.Node{nodeA, nodeB}, []metricstest.Edge{e}, idx)})

	if result.Value != 1 {
		t.Errorf("expected value 1 got %v", result.Value)
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

func TestUnbalancedEdge_ZeroCount(t *testing.T) {
	// Contract edge → not counted
	nodeA := metricstest.Node{Kind: metricstest.NodeKindFile, Path: pathA}
	nodeB := metricstest.Node{Kind: metricstest.NodeKindFile, Path: pathB}
	e := metricstest.Edge{From: nodeA.ID(), To: nodeB.ID(), Kind: metricstest.EdgeKindImports}

	idx := metricstest.Index{
		metricstest.ImportKey(nodeA.ID(), nodeB.ID()): metricstest.Classification{
			Strength:   relationship.StrengthContract,
			Distance:   relationship.DistanceCrossModuleDiffOwner,
			Volatility: relationship.VolatilityHigh,
		},
	}

	m := boundary.UnbalancedEdgeMetric{}
	result := m.Calculate(signal.CommonInput{Relationships: metricstest.BuildRelationships([]metricstest.Node{nodeA, nodeB}, []metricstest.Edge{e}, idx)})

	if result.Value != 0 {
		t.Errorf("expected value 0 got %v", result.Value)
	}
	if result.Band != bandStrong {
		t.Errorf("expected band strong got %q", result.Band)
	}
}

func TestUnbalancedEdge_DeclaredExternalNotCounted(t *testing.T) {
	// intrusive + high volatility, but distance=declared_external. distanceRank
	// routes DistanceExternal to the default (excluded) case by design (frozen v2
	// metric — declared external seams stay out of it), so this must NOT count as
	// an unbalanced edge even though strength and volatility both qualify.
	nodeA := metricstest.Node{Kind: metricstest.NodeKindFile, Path: pathA}
	nodeB := metricstest.Node{Kind: metricstest.NodeKindFile, Path: pathB}
	e := metricstest.Edge{From: nodeA.ID(), To: nodeB.ID(), Kind: metricstest.EdgeKindImports}

	idx := metricstest.Index{
		metricstest.ImportKey(nodeA.ID(), nodeB.ID()): metricstest.Classification{
			Strength:   relationship.StrengthIntrusive,
			Distance:   relationship.DistanceExternal,
			Volatility: relationship.VolatilityHigh,
		},
	}

	m := boundary.UnbalancedEdgeMetric{}
	result := m.Calculate(signal.CommonInput{Relationships: metricstest.BuildRelationships([]metricstest.Node{nodeA, nodeB}, []metricstest.Edge{e}, idx)})

	if result.Value != 0 {
		t.Errorf("expected value 0 (declared_external excluded, not counted as unbalanced) got %v", result.Value)
	}
}

func TestUnbalancedEdge_UnknownVolatilityIsNA(t *testing.T) {
	// An intrusive cross-module edge IS a candidate, but its volatility is unknown
	// (no churn data, no subdomain config). The high-volatility test cannot be
	// evaluated, so the metric is n/a — not a false "strong" (absence of evidence
	// is not evidence of balance).
	nodeA := metricstest.Node{Kind: metricstest.NodeKindFile, Path: pathA}
	nodeB := metricstest.Node{Kind: metricstest.NodeKindFile, Path: pathB}
	e := metricstest.Edge{From: nodeA.ID(), To: nodeB.ID(), Kind: metricstest.EdgeKindImports}

	idx := metricstest.Index{
		metricstest.ImportKey(nodeA.ID(), nodeB.ID()): metricstest.Classification{
			Strength:   relationship.StrengthIntrusive,
			Distance:   relationship.DistanceCrossModuleDiffOwner,
			Volatility: relationship.VolatilityUnknown,
		},
	}

	m := boundary.UnbalancedEdgeMetric{}
	result := m.Calculate(signal.CommonInput{Relationships: metricstest.BuildRelationships([]metricstest.Node{nodeA, nodeB}, []metricstest.Edge{e}, idx)})

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

func TestUnbalancedEdge_BaselinedFindingSuppressesCount(t *testing.T) {
	// Qualifying edge (intrusive + far + volatile), but the matching finding
	// is already baselined — StatusNew/"" are the only statuses that count as
	// new_high, so a baselined edge must not add to the count even though it
	// still qualifies structurally.
	nodeA := metricstest.Node{Kind: metricstest.NodeKindFile, Path: pathA}
	nodeB := metricstest.Node{Kind: metricstest.NodeKindFile, Path: pathB}
	e := metricstest.Edge{From: nodeA.ID(), To: nodeB.ID(), Kind: metricstest.EdgeKindImports}

	idx := metricstest.Index{
		metricstest.ImportKey(nodeA.ID(), nodeB.ID()): metricstest.Classification{
			Strength:   relationship.StrengthIntrusive,
			Distance:   relationship.DistanceCrossModuleDiffOwner,
			Volatility: relationship.VolatilityHigh,
		},
	}
	findings := []finding.Finding{
		{
			Edge:   finding.EdgeEvidence{From: finding.Endpoint{Path: pathA}, To: finding.Endpoint{Path: pathB}},
			Status: finding.StatusBaseline,
		},
	}

	m := boundary.UnbalancedEdgeMetric{}
	result := m.Calculate(signal.CommonInput{Relationships: metricstest.BuildRelationships([]metricstest.Node{nodeA, nodeB}, []metricstest.Edge{e}, idx), Findings: findings})

	if result.Value != 0 {
		t.Errorf("expected value 0 (baselined edge must not count as new_high) got %v", result.Value)
	}
	if result.Band != bandStrong {
		t.Errorf("expected band strong got %q", result.Band)
	}
}

func TestUnbalancedEdge_HigherPriorityStatusWins(t *testing.T) {
	// Two findings share the same edge pair with different statuses. The
	// first (StatusNew, priority 4) is listed before the second
	// (StatusBaseline, priority 1) — a naive "last write wins" index would
	// keep StatusBaseline and drop the count to 0; the actual statusPriority
	// logic must keep the higher-priority StatusNew regardless of order.
	nodeA := metricstest.Node{Kind: metricstest.NodeKindFile, Path: pathA}
	nodeB := metricstest.Node{Kind: metricstest.NodeKindFile, Path: pathB}
	e := metricstest.Edge{From: nodeA.ID(), To: nodeB.ID(), Kind: metricstest.EdgeKindImports}

	idx := metricstest.Index{
		metricstest.ImportKey(nodeA.ID(), nodeB.ID()): metricstest.Classification{
			Strength:   relationship.StrengthIntrusive,
			Distance:   relationship.DistanceCrossModuleDiffOwner,
			Volatility: relationship.VolatilityHigh,
		},
	}
	edgeEv := finding.EdgeEvidence{From: finding.Endpoint{Path: pathA}, To: finding.Endpoint{Path: pathB}}
	findings := []finding.Finding{
		{Edge: edgeEv, Status: finding.StatusNew},
		{Edge: edgeEv, Status: finding.StatusBaseline},
	}

	m := boundary.UnbalancedEdgeMetric{}
	result := m.Calculate(signal.CommonInput{Relationships: metricstest.BuildRelationships([]metricstest.Node{nodeA, nodeB}, []metricstest.Edge{e}, idx), Findings: findings})

	if result.Value != 1 {
		t.Errorf("expected value 1 (higher-priority StatusNew must win over StatusBaseline) got %v", result.Value)
	}
	if result.Band != bandCritical {
		t.Errorf("expected band critical got %q", result.Band)
	}
}
