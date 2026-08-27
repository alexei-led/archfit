package main

import (
	"context"

	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/llm"
)

type labelJudgeAdapter struct {
	provider         llm.Provider
	cfg              config.Config
	configPath, root string
}

func (j labelJudgeAdapter) Judge(ctx context.Context, req application.EnrichmentJudgmentRequest) ([]application.EnrichmentLabel, error) {
	pairs := make([]refinablePair, len(req.Candidates))
	for i, p := range req.Candidates {
		pairs[i] = application.EnrichmentCandidatePair{From: p.From, To: p.To, Strength: p.Strength, EdgeCount: p.EdgeCount, SamplePaths: append([]string(nil), p.SamplePaths...)}
	}
	evidence := architectureEvidenceLines(j.root, configModulesForEvidence(j.cfg), j.configPath, enrichEvidenceDiagnostics("enrich-labels", len(pairs)))
	return draftLabels(ctx, j.provider, j.cfg, pairs, evidence)
}

type abstainedJudgeAdapter struct {
	provider         llm.Provider
	cfg              config.Config
	configPath, root string
}

func (j abstainedJudgeAdapter) Judge(ctx context.Context, req application.EnrichmentJudgmentRequest) ([]application.EnrichmentLabel, error) {
	pairs := make([]abstainedPair, len(req.Abstained))
	for i, p := range req.Abstained {
		pairs[i] = abstainedPair{From: p.From, To: p.To, EdgeCount: p.EdgeCount}
		for _, s := range p.Samples {
			pairs[i].Samples = append(pairs[i].Samples, abstainedSample{FromPath: s.FromPath, ToPath: s.ToPath, File: s.File, Line: s.Line, Snippet: s.Snippet})
		}
	}
	evidence := architectureEvidenceLines(j.root, configModulesForEvidence(j.cfg), j.configPath, enrichEvidenceDiagnostics("enrich-abstained", len(pairs)))
	return draftAbstainedLabels(ctx, j.provider, j.cfg, pairs, evidence)
}

type filesystemSnippetAdapter struct{}

func (filesystemSnippetAdapter) Snippet(root, file string, line int) string {
	return loadSnippet(root, file, line)
}
