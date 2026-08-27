// The label evidence hash is computed twice per label lifecycle: once when
// `config enrich` stamps a draft, once when the gate verifies freshness. The
// two must hash the same edge set or every label reads permanently stale.
package analysis_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/relationship/analysis"
	"github.com/alexei-led/archfit/internal/relationship/labels"
)

// TestPairEvidenceMatchesTheHashAnalysisVerifies pins that the enrich-side
// PairEvidence hashes exactly the edges the gate-side hash covers, including
// non-dependency kinds. A Rust `belongs_to` edge between two modules gave the
// two sides different hashes, so the label was ignored on every run and a
// labels/stale advisory fired forever.
func TestPairEvidenceMatchesTheHashAnalysisVerifies(t *testing.T) {
	const fileOutside = "vendor/x/x.go"
	g := graph.Build([]graph.Facts{{
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: fileA, Language: graph.LangGo},
			{Kind: graph.NodeKindFile, Path: fileB, Language: graph.LangGo},
			{Kind: graph.NodeKindFile, Path: fileOutside, Language: graph.LangGo},
		},
		Edges: []graph.Edge{
			{From: nodeA, To: nodeB, Kind: graph.EdgeKindImports, Language: graph.LangGo,
				StrengthHint: string(relationship.StrengthFunctional)},
			// A containment edge: not a dependency kind, still part of the pair's
			// dependency surface as far as the gate-side hash is concerned.
			{From: nodeA, To: nodeB, Kind: graph.EdgeKindBelongsTo, Language: graph.LangGo},
			// A duplicate of the import edge: both sides must hash it the same
			// number of times, so a de-duplication added to one side is caught.
			{From: nodeA, To: nodeB, Kind: graph.EdgeKindImports, Language: graph.LangGo,
				StrengthHint: string(relationship.StrengthFunctional)},
			// An endpoint outside every declared module: the gate side drops it on
			// ModuleFor, the enrich side on the projected Set's empty ToModule.
			// Both must drop it, or the pair's two hashes diverge.
			{From: nodeA, To: "file:" + fileOutside, Kind: graph.EdgeKindImports, Language: graph.LangGo},
		},
	}})
	key := labels.Key(moduleA, moduleB)

	// A label with no stored hash is always effective, so analysis applies it.
	// Re-stamping it with the enrich-side hash must keep it effective.
	stamped := analysis.PairEvidence(
		analysis.Analyze(analysis.Input{Graph: g, Policy: relationshipPolicy(twoModules())}).Relationships,
		map[string]struct{}{key: {}},
	)
	if stamped[key] == "" {
		t.Fatal("enrich-side evidence hash is empty, want a hash for the fixture pair")
	}

	got := analysis.Analyze(analysis.Input{
		Graph:  g,
		Policy: relationshipPolicy(twoModules()),
		Labels: []labels.Label{{
			From: moduleA, To: moduleB, Strength: string(relationship.StrengthIntrusive),
			Status: labels.StatusApproved, EvidenceHash: stamped[key],
		}},
	})
	if len(got.Assessment.StaleLabelKeys) != 0 {
		t.Errorf("stale label keys = %v, want none: the enrich hash must match the gate hash",
			got.Assessment.StaleLabelKeys)
	}
	for _, e := range got.Relationships.Edges {
		if e.FromModule != moduleA || e.ToModule != moduleB {
			continue // the vendor edge resolves to no module and stays unknown.
		}
		if e.Kind == string(graph.EdgeKindImports) && e.Strength != relationship.StrengthIntrusive {
			t.Errorf("strength = %q, want the freshly stamped label applied", e.Strength)
		}
	}
}
