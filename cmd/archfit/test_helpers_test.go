package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	apppipeline "github.com/alexei-led/archfit/internal/analysispipeline"
	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/assessment/metrics"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/score"
	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/labels/labelsio"
)

const (
	gateOff  = string(config.GateOff)
	gateWarn = string(config.GateWarn)
	gateFail = string(config.GateFail)
)

func runPipeline(ctx context.Context, deps *appDeps, cfg config.Config, rc apppipeline.RunContext, mode apppipeline.Mode, _ baseline.Baseline, _ ...metrics.Metric) (result.Result, score.Scorecard, error) {
	pipelineDeps := &apppipeline.Deps{Runner: deps.Runner, LabelLoader: labelsio.Loader{}, LabelsPath: filepath.Join(rc.BundleDir, defaultLabelsPath), Stderr: deps.stderr(), Progress: deps.progress, WarnLabel: deps.warnLabel, Refresh: deps.refresh}
	a := apppipeline.NewAnalyzer(rc.ConfigSource, rc.ScanRoot, cfg, pipelineDeps)
	a.PreparedOptions = apppipeline.Options(cfg)
	a.PreparedSnapshot = apppipeline.PolicySnapshot(cfg)
	if err := a.Prepare(ctx); err != nil {
		return result.Result{}, score.Scorecard{}, err
	}
	// Characterization helpers exercise the same application-owned stage calls
	// as production; the compatibility Run adapter is intentionally not used.
	snapshot, err := a.Acquire(ctx, application.AnalysisRequest{ConfigSource: rc.ConfigSource, BundleDir: rc.BundleDir, Root: rc.ScanRoot, BaseRef: mode.Base, Formats: mode.Formats, ReportOnly: mode.ReportOnly, NoAdvisories: !mode.Advisory, EmptyBaseline: true})
	if err != nil {
		return result.Result{}, score.Scorecard{}, err
	}
	relationships, err := a.Relate(ctx, snapshot)
	if err != nil {
		return result.Result{}, score.Scorecard{}, err
	}
	out, err := a.Assess(ctx, application.AnalysisRequest{ConfigSource: rc.ConfigSource, BundleDir: rc.BundleDir, Root: rc.ScanRoot, BaseRef: mode.Base, Formats: mode.Formats, ReportOnly: mode.ReportOnly, NoAdvisories: !mode.Advisory, EmptyBaseline: true}, snapshot.Facts.ForAssessment(), snapshot.Context, relationships)
	if err != nil {
		return result.Result{}, score.Scorecard{}, err
	}
	return out.Diagnostic, out.Score, nil
}

func writeFileAt(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func newRunContext(path, root string) apppipeline.RunContext {
	return apppipeline.NewRunContext(path, root)
}
