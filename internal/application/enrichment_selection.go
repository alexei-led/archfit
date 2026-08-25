package application

import (
	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/relationship/analysis"
)

// enrichmentSet projects captured enrichment evidence back into the
// relationship contract the selection rules read. It is the one adapter between
// the application DTO and the relationship domain.
func enrichmentSet(in EnrichmentEvidence) relationship.Set {
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

// pairEvidence computes the per-pair evidence hashes a stored label is checked
// against. Selection is a pure relationship decision, so the application calls
// it directly: a port here would only hide the call.
func pairEvidence(in EnrichmentEvidence, wanted map[string]struct{}) map[string]string {
	return analysis.PairEvidence(enrichmentSet(in), wanted)
}

// selectCandidates selects regular enrichment candidates.
func selectCandidates(in EnrichmentEvidence, approved map[string]struct{}) []EnrichmentCandidatePair {
	pairs := analysis.RefinablePairs(enrichmentSet(in), approved)
	out := make([]EnrichmentCandidatePair, len(pairs))
	for i, p := range pairs {
		out[i] = EnrichmentCandidatePair{From: p.From, To: p.To, Strength: p.Strength, EdgeCount: p.EdgeCount, SamplePaths: append([]string(nil), p.SamplePaths...)}
	}
	return out
}

// selectAbstained selects unknown-strength cross-module candidates.
func selectAbstained(in EnrichmentEvidence, approved map[string]struct{}, edgeCap, sampleCap int) ([]EnrichmentAbstainedPair, int) {
	pairs, total := analysis.AbstainedPairs(enrichmentSet(in), approved, edgeCap, sampleCap)
	out := make([]EnrichmentAbstainedPair, len(pairs))
	for i, p := range pairs {
		out[i] = EnrichmentAbstainedPair{From: p.From, To: p.To, EdgeCount: p.EdgeCount}
		for _, s := range p.Samples {
			out[i].Samples = append(out[i].Samples, EnrichmentSample{FromPath: s.FromPath, ToPath: s.ToPath, File: s.File, Line: s.Line})
		}
	}
	return out, total
}
