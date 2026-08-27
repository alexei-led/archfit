package application

import (
	"github.com/alexei-led/archfit/internal/relationship"
)

// projectEnrichmentEvidence narrows the captured relationship set to the
// enrichment contract the application use case consumes.
func projectEnrichmentEvidence(in relationship.Set) *EnrichmentEvidence {
	out := &EnrichmentEvidence{Edges: make([]EnrichmentEdge, 0, len(in.Edges))}
	for _, e := range in.Edges {
		p := EnrichmentEdge{FromPath: e.FromPath, ToPath: e.ToPath, FromModule: e.FromModule, ToModule: e.ToModule, Kind: e.Kind, Strength: string(e.Strength), Distance: string(e.Distance)}
		for _, l := range e.Locations {
			p.Locations = append(p.Locations, Location{File: l.File, Line: l.Line})
		}
		out.Edges = append(out.Edges, p)
	}
	return out
}
