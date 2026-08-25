package pipeline

import (
	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/relationship"
)

func enrichmentSet(in application.EnrichmentEvidence) relationship.Set {
	set := relationship.Set{Edges: make([]relationship.Edge, 0, len(in.Edges))}
	for _, e := range in.Edges {
		x := relationship.Edge{FromPath: e.FromPath, ToPath: e.ToPath, FromModule: e.FromModule, ToModule: e.ToModule, Kind: e.Kind, Strength: relationship.Strength(e.Strength), Distance: relationship.Distance(e.Distance)}
		for _, l := range e.Locations {
			x.Locations = append(x.Locations, relationship.Location{File: l.File, Line: l.Line})
		}
		set.Edges = append(set.Edges, x)
	}
	return set
}

// PairEvidenceFromEnrichment computes evidence hashes from application DTOs.
func PairEvidenceFromEnrichment(in application.EnrichmentEvidence, wanted map[string]struct{}) map[string]string {
	return PairEvidenceFromRelationships(enrichmentSet(in), wanted)
}

// SelectRefinableEnrichmentPairs selects prompt-ready review candidates.
func SelectRefinableEnrichmentPairs(in application.EnrichmentEvidence, approved map[string]struct{}) []RefinablePair {
	return SelectRefinableRelationshipPairs(enrichmentSet(in), approved)
}
