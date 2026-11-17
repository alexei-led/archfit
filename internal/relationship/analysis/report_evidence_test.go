package analysis

import (
	"testing"

	"github.com/alexei-led/archfit/v4/internal/policy"
	"github.com/alexei-led/archfit/v4/internal/relationship"
)

const (
	reportTestSource = "a/one.go"
	reportTestName   = "name"
	reportTestType   = "type"
)

func TestReportRelationshipsKeepDistinctPopulationsAndEdgeKinds(t *testing.T) {
	t.Parallel()
	items := []relationship.ConnascenceEvidence{{Kind: reportTestName, Source: connascenceSourceGoTypes}, {Kind: reportTestType, Source: connascenceSourceGoTypes}, {Kind: reportTestName, Source: connascenceSourceGoTypes}, {Kind: reportTestType, Source: connascenceSourceGoTypes}, {Kind: reportTestName, Source: connascenceSourceGoTypes}, {Kind: reportTestType, Source: connascenceSourceGoTypes}}
	set := relationship.Set{Edges: []relationship.Edge{
		{FromID: reportTestSource, ToID: "b/two.go", FromModule: "a", ToModule: "b", Distance: relationship.DistanceCrossModule, Kind: "dependency", Locations: []relationship.Location{{File: reportTestSource, Line: 3}, {File: reportTestSource, Line: 3}}, Classified: relationship.Classification{Connascence: items}},
		{FromID: "a/other.go", ToID: "b/two.go", FromModule: "a", ToModule: "b", Distance: relationship.DistanceCrossModule, Kind: "type_use"},
		{FromID: reportTestSource, ToID: "a/two.go", FromModule: "a", ToModule: "a", Distance: relationship.DistanceSameModule},
		{FromID: reportTestSource, ToID: "external", FromModule: "a", Distance: relationship.DistanceUnknown},
		{FromID: "b/one.go", ToID: "external", FromModule: "b", Distance: relationship.DistanceUnknown},
	}}
	out := reportRelationships(set, map[string]policy.ModuleDef{"a": {}, "b": {}})
	summary := buildConnascenceSummary(set)
	if out.TotalEdges != 5 || out.PairEdges != 2 || len(out.Pairs) != 1 || len(out.Pairs[0].Observations) != 2 {
		t.Fatalf("populations: %+v", out)
	}
	if summary.EdgesWithEvidence+summary.AbstainedEdges != out.TotalEdges || summary.TotalEvidence != 6 || summary.TotalEvidence <= out.TotalEdges {
		t.Fatalf("summary: %+v", summary)
	}
	pair := out.Pairs[0]
	if pair.PairID != relationship.SeamID("a", "b") || pair.PairID == relationship.SeamID("b", "a") || pair.PairID == relationship.SeamID("a", "c") {
		t.Fatalf("pair identity: %s", pair.PairID)
	}
	var retained bool
	for _, o := range pair.Observations {
		if o.EdgeKind == "type_use" {
			retained = true
		}
		if o.EdgeKind == "dependency" && len(o.Sites) != 1 {
			t.Fatalf("sites not deduplicated: %+v", o.Sites)
		}
	}
	if !retained {
		t.Fatalf("non-dependency kind lost: %+v", pair)
	}
	if len(pair.Unknowns) == 0 {
		t.Fatal("missing location gap not disclosed")
	}
}
