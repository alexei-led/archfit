package boundary_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/metrics/boundary"
	signal "github.com/alexei-led/archfit/internal/assessment/signals"
	"github.com/alexei-led/archfit/internal/relationship"
)

func TestBoundaryMetricsConsumeRelationshipContract(t *testing.T) {
	fromID := "file:" + pathA
	toID := "file:" + pathB
	rels := relationship.Set{
		Nodes: []relationship.Node{
			{ID: fromID, Path: pathA, Kind: "file", Module: "pkg/a", FirstParty: true},
			{ID: toID, Path: pathB, Kind: "file", Module: "pkg/b", FirstParty: true},
		},
		Edges: []relationship.Edge{
			{
				FromID: fromID, ToID: toID, FromPath: pathA, ToPath: pathB,
				FromModule: "pkg/a", ToModule: "pkg/b", Kind: "imports", Strength: relationship.StrengthIntrusive,
				Distance: relationship.DistanceCrossModuleDiffOwner, Volatility: relationship.VolatilityHigh,
				Provenance: relationship.Provenance{ClassificationKey: fromID + "\x00" + toID + "\x00imports", DistanceBasis: "ownership"},
			},
		},
	}
	findings := []finding.Finding{{
		Edge:   finding.EdgeEvidence{From: finding.Endpoint{Path: pathA}, To: finding.Endpoint{Path: pathB}, Kind: "imports"},
		Status: finding.StatusNew,
	}}
	in := signal.CommonInput{Relationships: rels, Findings: findings}

	enc := boundary.EncapsulationMetric{}.Calculate(in)
	if enc.Band == bandNAStr || enc.Value != 0 {
		t.Fatalf("encapsulation from relationship.Set = value %.2f band %s, want real zero intrusive measurement", enc.Value, enc.Band)
	}
	unbalanced := boundary.UnbalancedEdgeMetric{}.Calculate(in)
	if unbalanced.Value != 1 || unbalanced.Band != bandCritical {
		t.Fatalf("unbalanced_edge from relationship.Set = value %.2f band %s, want 1 critical", unbalanced.Value, unbalanced.Band)
	}
}
