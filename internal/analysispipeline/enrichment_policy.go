package pipeline

import (
	"github.com/alexei-led/archfit/internal/application"
	relationshipanalysis "github.com/alexei-led/archfit/internal/relationship/analysis"
)

// EnrichmentPolicy implements technical candidate selection and evidence hashing.
type EnrichmentPolicy struct{}

// PairEvidence computes technical evidence hashes.
func (EnrichmentPolicy) PairEvidence(in application.EnrichmentEvidence, wanted map[string]struct{}) map[string]string {
	return relationshipanalysis.PairEvidence(enrichmentSet(in), wanted)
}

// SelectCandidates selects regular enrichment candidates.
func (EnrichmentPolicy) SelectCandidates(in application.EnrichmentEvidence, approved map[string]struct{}) []application.EnrichmentCandidatePair {
	pairs := relationshipanalysis.RefinablePairs(enrichmentSet(in), approved)
	out := make([]application.EnrichmentCandidatePair, len(pairs))
	for i, p := range pairs {
		out[i] = application.EnrichmentCandidatePair{From: p.From, To: p.To, Strength: p.Strength, EdgeCount: p.EdgeCount, SamplePaths: append([]string(nil), p.SamplePaths...)}
	}
	return out
}

// SelectAbstained selects unknown-strength cross-module candidates.
func (EnrichmentPolicy) SelectAbstained(in application.EnrichmentEvidence, approved map[string]struct{}, edgeCap, sampleCap int) ([]application.EnrichmentAbstainedPair, int) {
	pairs, total := relationshipanalysis.AbstainedPairs(enrichmentSet(in), approved, edgeCap, sampleCap)
	out := make([]application.EnrichmentAbstainedPair, len(pairs))
	for i, p := range pairs {
		out[i] = application.EnrichmentAbstainedPair{From: p.From, To: p.To, EdgeCount: p.EdgeCount}
		for _, s := range p.Samples {
			out[i].Samples = append(out[i].Samples, application.EnrichmentSample{FromPath: s.FromPath, ToPath: s.ToPath, File: s.File, Line: s.Line})
		}
	}
	return out, total
}

var _ application.EnrichmentSelectionPolicy = EnrichmentPolicy{}
