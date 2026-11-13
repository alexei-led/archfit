package main

import (
	"context"
	"path/filepath"

	"github.com/alexei-led/archfit/v3/internal/application"
	"github.com/alexei-led/archfit/v3/internal/baseline"
	"github.com/alexei-led/archfit/v3/internal/config"
	"github.com/alexei-led/archfit/v3/internal/evidence/acquisition"
	historygit "github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/labels/labelsio"
)

// newEvidenceService builds the concrete acquisition stage for one tree.
func newEvidenceService(configPath, root string, cfg config.Config, deps *appDeps) *acquisition.Service {
	return &acquisition.Service{
		ConfigPath: configPath, Root: root,
		Options: cfg.RunOptions(), Policy: cfg.PolicySnapshot(),
		Runner: deps.Runner, Labels: labelsio.Loader{},
		Stderr: deps.stderr(), Progress: deps.progress, WarnLabel: deps.warnLabel, Refresh: deps.refresh,
	}
}

// newAnalysisStages composes one analysis run: the policy preparer, the
// acquisition stage, the persistence ports, and the base-tree comparison
// adapters. Every field is a constructed value — no branching decision lives
// here, only the choice of concrete implementation.
func newAnalysisStages(configPath, root string, cfg config.Config, deps *appDeps) application.StageExecutor {
	return application.StageExecutor{
		Preparer:  config.Preparer{Config: cfg, Stderr: deps.stderr()},
		Evidence:  newEvidenceService(configPath, root, cfg, deps),
		Baseline:  baselineLoaderAdapter{},
		Stderr:    deps.stderr(),
		Progress:  deps.progress,
		Worktree:  historygit.Worktree{Runner: deps.Runner},
		Analyzers: cfg.AnalyzerFamilies(),
		// The base tree is measured with the caller's EFFECTIVE head config, so
		// config drift can never masquerade as code drift. Its module map is an
		// independent copy: the run's owner/deploy-unit backfill writes through
		// that map, and a shared one would leak head-tree owners into the base.
		NewBaseEvidence: func(baseRoot string) application.EvidenceStage {
			baseCfg := cfg.WithIndependentModules()
			quiet := *deps
			quiet.progress = nil
			return newEvidenceService(configPath, baseRoot, baseCfg, &quiet)
		},
	}
}

// newReportOnlyStages composes a run that never consults the persisted
// baseline: config comparison and the base sub-run measure trees, not accepted
// history.
func newReportOnlyStages(configPath, root string, cfg config.Config, deps *appDeps) application.StageExecutor {
	stages := newAnalysisStages(configPath, root, cfg, deps)
	stages.Baseline = nil
	return stages
}

// baselineLoaderAdapter reads the persisted baseline for a config bundle.
type baselineLoaderAdapter struct{}

func (baselineLoaderAdapter) Load(ctx context.Context, bundleDir string) (application.Baseline, error) {
	b, err := baseline.Load(ctx, filepath.Join(bundleDir, defaultBaselinePath))
	if err != nil {
		return application.Baseline{}, err
	}
	return applicationBaseline(b), nil
}

// applicationBaseline converts a baseline file into the application view.
func applicationBaseline(b baseline.Baseline) application.Baseline {
	out := application.Baseline{Present: b.SchemaVersion != "", Accepted: b, Metrics: b.Metrics}
	if b.State != nil {
		out.State = &application.BaselineStateSnapshot{
			ConfigHash: b.State.ConfigHash, ClassificationHash: b.State.ClassificationHash, ModelHash: b.State.ModelHash,
			LabelsHash: b.State.LabelsHash, RubricVersion: b.State.RubricVersion,
			MeasurementProfile: b.State.MeasurementProfile,
			QualifyingSeamIDs:  b.State.QualifyingSeamIDs,
		}
	}
	return out
}
