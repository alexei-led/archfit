package application

import (
	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/relationship/analysis"
)

// pairEvidence computes the per-pair evidence hashes a stored label is checked
// against. Selection is a pure relationship decision, so the application calls
// it directly: a port here would only hide the call.
func pairEvidence(in relationship.Set, wanted map[string]struct{}) map[string]string {
	return analysis.PairEvidence(in, wanted)
}

// selectCandidates selects regular enrichment candidates.
func selectCandidates(in relationship.Set, approved map[string]struct{}) []EnrichmentCandidatePair {
	pairs := analysis.RefinablePairs(in, approved)
	out := make([]EnrichmentCandidatePair, len(pairs))
	for i, p := range pairs {
		out[i] = EnrichmentCandidatePair{From: p.From, To: p.To, Strength: p.Strength, EdgeCount: p.EdgeCount, SamplePaths: append([]string(nil), p.SamplePaths...)}
	}
	return out
}

// selectAbstained selects unknown-strength cross-module candidates.
func selectAbstained(in relationship.Set, approved map[string]struct{}, edgeCap, sampleCap int) ([]EnrichmentAbstainedPair, int) {
	pairs, total := analysis.AbstainedPairs(in, approved, edgeCap, sampleCap)
	out := make([]EnrichmentAbstainedPair, len(pairs))
	for i, p := range pairs {
		out[i] = EnrichmentAbstainedPair{From: p.From, To: p.To, EdgeCount: p.EdgeCount}
		for _, s := range p.Samples {
			out[i].Samples = append(out[i].Samples, EnrichmentSample{FromPath: s.FromPath, ToPath: s.ToPath, File: s.File, Line: s.Line})
		}
	}
	return out, total
}
