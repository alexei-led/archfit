package pipeline

import (
	"context"

	"github.com/alexei-led/archfit/internal/application"
)

// AnalyzeEnrichment captures classified relationship evidence for review.
func (a *Analyzer) AnalyzeEnrichment(ctx context.Context, req application.EnrichmentRequest) (application.EnrichmentResult, error) {
	snapshot, err := a.Acquire(ctx, application.AnalysisRequest{ConfigSource: req.ConfigPath, Root: req.Root, CaptureRelationships: true})
	if err != nil {
		return application.EnrichmentResult{}, err
	}
	relationships, err := a.Relate(ctx, snapshot)
	if err != nil {
		return application.EnrichmentResult{}, err
	}
	out, err := a.Assess(ctx, application.AnalysisRequest{ConfigSource: req.ConfigPath, Root: req.Root, CaptureRelationships: true}, snapshot.Facts.ForAssessment(), snapshot.Context, relationships)
	if err != nil {
		return application.EnrichmentResult{}, err
	}
	if out.EnrichmentEvidence == nil {
		return application.EnrichmentResult{}, nil
	}
	return application.EnrichmentResult{Evidence: *out.EnrichmentEvidence}, nil
}

var _ application.EnrichmentAnalyzer = (*Analyzer)(nil)
