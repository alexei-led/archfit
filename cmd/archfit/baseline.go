package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/engine"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
)

// BaselineCmd runs the engine and saves findings as the new baseline.
type BaselineCmd struct {
	Config   string `short:"c" help:"Config file." default:".archfit.yaml"`
	Root     string `short:"r" help:"Repository root to analyze (default: directory of --config)." type:"path"`
	Full     bool   `help:"Scan all files before saving the accepted baseline."`
	Advisory bool   `help:"Include advisory findings in the baseline."`
	Base     string `help:"Git ref to compare against when baselining a delta run."`
}

func (*BaselineCmd) Help() string {
	return `Use baseline after reviewing current findings so CI can block only new architecture drift.

Typical calibration:
  archfit --gate --config .archfit.yaml --full
  archfit baseline --config .archfit.yaml --full
  archfit --gate --config .archfit.yaml --base origin/main`
}

func (c *BaselineCmd) Run(deps *appDeps) error {
	ctx := context.Background()

	cfg, err := loadConfig(ctx, c.Config, false)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	configDir := filepath.Dir(c.Config)
	existingBase, err := baseline.Load(ctx, filepath.Join(configDir, defaultBaselinePath))
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	// Baseline runs the exact same pipeline as check (same change history,
	// pattern provider, and SCIP resolver): snapshot values recorded from
	// different inputs would surface as phantom deltas on the next check.
	mode := engine.Mode{Full: c.Full, Advisory: c.Advisory, Base: c.Base}
	diag, err := runPipeline(ctx, deps, cfg, c.Config, c.Root, false, mode, existingBase)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	newBase := baseline.Baseline{}
	for _, f := range diag.Findings {
		if f.Status == finding.StatusFixed {
			continue // fixed = no longer detected; don't carry into new baseline
		}
		newBase.Accepted = append(newBase.Accepted, baseline.AcceptedFinding{
			Fingerprint: f.ID,
			RuleID:      f.RuleID,
			Kind:        f.Kind,
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

	bPath := filepath.Join(configDir, defaultBaselinePath)
	if err := baseline.Save(ctx, bPath, newBase); err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	_, _ = fmt.Fprintf(deps.Stdout, "baseline saved: %s\n", bPath)
	return nil
}
