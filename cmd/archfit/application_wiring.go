package main

import (
	"context"
	"path/filepath"

	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/evidence/acquisition"
	historygit "github.com/alexei-led/archfit/internal/history/git"
	"github.com/alexei-led/archfit/internal/labels/labelsio"
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
	out := application.Baseline{Accepted: b, Metrics: b.Metrics}
	if b.State != nil {
		out.State = &application.BaselineStateSnapshot{
			ConfigHash: b.State.ConfigHash, ModelHash: b.State.ModelHash,
			LabelsHash: b.State.LabelsHash, RubricVersion: b.State.RubricVersion,
			HardGateFindingIDs: b.State.HardGateFindingIDs,
			QualifyingSeamIDs:  b.State.QualifyingSeamIDs,
			Dimensions:         loadedDimensions(b.State.Dimensions),
		}
	}
	return out, nil
}

func loadedDimensions(in []baseline.DimensionSnapshot) []application.BaselineDimension {
	out := make([]application.BaselineDimension, 0, len(in))
	for _, d := range in {
		dim := application.BaselineDimension{
			Name: d.Name, Status: d.Status, Gate: d.Gate,
			CoverageBasis: d.Coverage.Basis, CoverageObserved: d.Coverage.Observed, CoverageTotal: d.Coverage.Total,
			Metrics: make([]application.BaselineMetric, 0, len(d.Metrics)),
		}
		for _, m := range d.Metrics {
			dim.Metrics = append(dim.Metrics, application.BaselineMetric{Name: m.Name, Value: m.Value, Unit: m.Unit})
		}
		out = append(out, dim)
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
			ConfigHash: in.State.ConfigHash, ModelHash: in.State.ModelHash,
			LabelsHash: in.State.LabelsHash, RubricVersion: in.State.RubricVersion,
			HardGateFindingIDs: in.State.HardGateFindingIDs,
			QualifyingSeamIDs:  in.State.QualifyingSeamIDs,
			Dimensions:         savedDimensions(in.State.Dimensions),
		}
	}
	return baseline.Save(ctx, path, b)
}

func savedDimensions(in []application.BaselineDimension) []baseline.DimensionSnapshot {
	out := make([]baseline.DimensionSnapshot, 0, len(in))
	for _, d := range in {
		dim := baseline.DimensionSnapshot{
			Name: d.Name, Status: d.Status, Gate: d.Gate,
			Coverage: baseline.CoverageSnapshot{Basis: d.CoverageBasis, Observed: d.CoverageObserved, Total: d.CoverageTotal},
			Metrics:  make([]baseline.MetricSnapshotValue, 0, len(d.Metrics)),
		}
		for _, m := range d.Metrics {
			dim.Metrics = append(dim.Metrics, baseline.MetricSnapshotValue{Name: m.Name, Value: m.Value, Unit: m.Unit})
		}
		out = append(out, dim)
	}
	return out
}
