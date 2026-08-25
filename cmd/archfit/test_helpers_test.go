package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/score"
	"github.com/alexei-led/archfit/internal/config"
)

const (
	gateOff  = string(config.GateOff)
	gateWarn = string(config.GateWarn)
	gateFail = string(config.GateFail)
)

// runPipeline drives the same application-owned stage sequence production uses,
// against an empty baseline, and returns the diagnostic and scorecard the
// characterization tests assert on.
func runPipeline(ctx context.Context, deps *appDeps, cfg config.Config, configPath, root string) (result.Result, score.Scorecard, error) {
	stages := newAnalysisStages(configPath, root, cfg, deps)
	out, err := stages.Execute(ctx, application.AnalysisRequest{
		ConfigSource: configPath, Root: root, EmptyBaseline: true, ApplyToolGate: true,
	})
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

// Published rule IDs the CLI contract tests assert against. They are spelled
// out rather than imported from assessment so a rename inside the domain cannot
// silently rewrite what the external JSON contract is expected to carry.
const (
	ruleIDBCImbalanced = "bc/imbalanced_coupling"
	ruleIDCouplingGate = "bc/coupling_gate"
)
