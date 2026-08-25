package main

import (
	"context"
	"path/filepath"

	apppipeline "github.com/alexei-led/archfit/internal/analysispipeline"
	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/labels/labelsio"
)

func enrichmentLabelStore() application.EnrichmentLabelStore { return labelsio.ApplicationStore{} }

func newUseCaseAnalyzer(configPath, root string, cfg config.Config, deps *appDeps) *apppipeline.Analyzer {
	return apppipeline.NewAnalyzer(configPath, root, cfg, &apppipeline.Deps{
		Runner: deps.Runner, LabelLoader: labelsio.Loader{}, LabelsPath: filepath.Join(filepath.Dir(configPath), defaultLabelsPath),
		Stderr: deps.stderr(), Progress: deps.progress, WarnLabel: deps.warnLabel, Refresh: deps.refresh,
	})
}

type baselineWriterAdapter struct{}

func (baselineWriterAdapter) Save(ctx context.Context, path string, in application.BaselineSnapshot) error {
	b := baseline.Baseline{Metrics: in.Metrics}
	for _, f := range in.Accepted {
		b.Accepted = append(b.Accepted, baseline.AcceptedFinding{Fingerprint: f.Fingerprint, RuleID: f.RuleID, Kind: f.Kind, Severity: f.Severity})
	}
	if in.Score != nil {
		b.Score = &baseline.ScoreSnapshot{CouplingBalance: in.Score.CouplingBalance, Band: in.Score.Band, ScoreVersion: in.Score.ScoreVersion, RubricVersion: in.Score.RubricVersion}
	}
	return baseline.Save(ctx, path, b)
}
