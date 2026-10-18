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

func enrichmentLabelStore() application.EnrichmentLabelStore { return labelsio.ApplicationStore{} }

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

// newConfigLintService composes `config lint`: the acquisition walk is its
// source-inventory port, under the projections of the config being linted.
func newConfigLintService(configPath string, cfg config.Config, deps *appDeps) application.ConfigLintService {
	return application.ConfigLintService{Inventory: acquisition.Inventory{
		ConfigPath: configPath, Options: cfg.RunOptions(), Runner: deps.Runner,
	}}
}

// newAnalyzeStages composes the analyze/check run. It is the only composition
// that owns the user's stderr conversation, so it is the only one that
// discloses the config-quality block.
func newAnalyzeStages(configPath, root string, cfg config.Config, deps *appDeps) application.StageExecutor {
	stages := newAnalysisStages(configPath, root, cfg, deps)
	stages.Preparer = config.Preparer{Config: cfg, Stderr: deps.stderr(), DiscloseLint: true}
	return stages
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

type baselineWriterAdapter struct{}

func (baselineWriterAdapter) Save(ctx context.Context, path string, in application.BaselineSnapshot) error {
	b := baseline.Baseline{Metrics: in.Metrics}
	for _, f := range in.Accepted {
		b.Accepted = append(b.Accepted, baseline.AcceptedFinding{Fingerprint: f.Fingerprint, RuleID: f.RuleID, Kind: f.Kind, Severity: f.Severity})
	}
	if in.State != nil {
		b.State = &baseline.StateSnapshot{
			ConfigHash: in.State.ConfigHash, ClassificationHash: in.State.ClassificationHash, ModelHash: in.State.ModelHash,
			LabelsHash: in.State.LabelsHash, RubricVersion: in.State.RubricVersion,
			MeasurementProfile: in.State.MeasurementProfile,
			QualifyingSeamIDs:  in.State.QualifyingSeamIDs,
		}
	}
	return baseline.Save(ctx, path, b)
}

// bundleDirOf is the directory that holds a config bundle: the config file's.
func bundleDirOf(configPath string) string { return filepath.Dir(configPath) }

// newPolicyQuery composes the tool-free acquisition behind `archfit policy`:
// the analysis root resolved as check resolves it, under the config's run
// projections.
func newPolicyQuery(ctx context.Context, configPath, root string, cfg config.Config, deps *appDeps) (acquisition.Query, error) {
	return acquisition.NewQuery(ctx, configPath, root, cfg.RunOptions(), deps.Runner)
}

// newPolicyQueryService composes `archfit policy can-import`: the tool-free
// edge query and the persisted baseline. No analyzer and no fact cache.
func newPolicyQueryService(q acquisition.Query) application.PolicyQueryService {
	return application.PolicyQueryService{Edges: q, Baseline: baselineLoaderAdapter{}}
}
