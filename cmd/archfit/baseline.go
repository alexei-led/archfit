package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/engine"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
)

// BaselineCmd runs the engine and saves findings as the new baseline.
type BaselineCmd struct {
	Config       string `short:"c" help:"Config file." default:".archfit.yaml"`
	Root         string `short:"r" help:"Repository root to analyze (default: directory of --config)." type:"path"`
	NoAdvisories bool   `name:"no-advisories" help:"Exclude informational Balanced-Coupling advisories from the baseline."`
	Base         string `help:"Git ref to compare against when baselining a delta run."`
	Refresh      bool   `name:"refresh" help:"Re-run all extractors and refresh the cache. Use after installing or updating analyzer tools."`
}

func (*BaselineCmd) Help() string {
	return `Use baseline after reviewing current findings so CI can block only new architecture drift.

Typical calibration:
  archfit check --config .archfit.yaml
  archfit baseline --config .archfit.yaml
  archfit check --config .archfit.yaml --base origin/main`
}

func (c *BaselineCmd) Run(deps *appDeps) error {
	ctx := context.Background()

	cfg, err := loadConfig(ctx, c.Config)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	configDir := filepath.Dir(c.Config)
	existingBase, err := baseline.Load(ctx, filepath.Join(configDir, defaultBaselinePath))
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	advisory := !c.NoAdvisories
	mode := engine.Mode{Full: true, Advisory: advisory, Base: c.Base}
	deps.refresh = c.Refresh
	diag, sc, err := runPipeline(ctx, deps, cfg, c.Config, c.Root, mode, existingBase)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	newBase := baseline.Baseline{}
	for _, f := range diag.Findings {
		if f.Status == finding.StatusFixed {
			continue
		}
		if f.RuleID == ruleIDBCCouplingGate {
			continue
		}
		kind := f.Kind
		if f.RuleID == engine.RuleIDBCImbalanced {
			kind = finding.KindAdvisory
		}
		newBase.Accepted = append(newBase.Accepted, baseline.AcceptedFinding{
			Fingerprint: f.ID,
			RuleID:      f.RuleID,
			Kind:        kind,
			Severity:    string(f.Severity),
		})
	}
	newBase.Metrics = make(diagnostic.MetricSnapshot)
	for _, m := range diag.Metrics {
		newBase.Metrics[m.Name] = struct {
			Value   float64 `json:"value"`
			Version string  `json:"version"`
		}{Value: m.Value, Version: m.Version}
	}
	if !sc.OverallBand.Unmeasured() {
		newBase.Score = &baseline.ScoreSnapshot{
			CouplingBalance: sc.Overall,
			Band:            string(sc.OverallBand),
			ScoreVersion:    coupling.ScoreVersion,
		}
	}

	bPath := filepath.Join(configDir, defaultBaselinePath)
	if err := baseline.Save(ctx, bPath, newBase); err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	_, _ = fmt.Fprintf(deps.Stdout, "baseline saved: %s\n", bPath)
	return nil
}
