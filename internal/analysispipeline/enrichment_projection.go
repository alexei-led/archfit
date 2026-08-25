package pipeline

import (
	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/relationship"
)

// enrichmentSet projects captured enrichment evidence back into the
// relationship contract the selection rules read. It is the one adapter between
// the application DTO and the relationship domain.
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
