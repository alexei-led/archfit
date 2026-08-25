package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/alexei-led/archfit/internal/application"
)

// BaselineCmd runs the engine and saves findings as the new baseline.
type BaselineCmd struct {
	Config       string `short:"c" help:"Config file." default:".archfit.yaml"`
	Root         string `short:"r" help:"Repository root to analyze (default: directory of --config)." type:"path"`
	NoAdvisories bool   `name:"no-advisories" help:"Exclude informational Balanced-Coupling advisories from the baseline."`
	Refresh      bool   `name:"refresh" help:"Re-run all extractors and refresh the cache. Use after installing or updating analyzer tools."`
}

func (*BaselineCmd) Help() string {
	return `Use baseline after reviewing current findings so CI can block only new architecture drift.

It records one full baseline of the tree as checked out. There is no git-base mode; compare against a ref with ` + "`archfit check --base`" + ` instead.

Typical calibration:
  archfit check --config .archfit.yaml
  archfit baseline --config .archfit.yaml
  archfit check --config .archfit.yaml --base origin/main`
}

func (c *BaselineCmd) Run(deps *appDeps) error {
	ctx := context.Background()

	cfg, err := loadAnalysisConfig(ctx, c.Config)
	if err != nil {
		return configLoadError(err)
	}
	deps.refresh = c.Refresh
	bPath := filepath.Join(filepath.Dir(c.Config), defaultBaselinePath)
	service := application.BaselineService{Stages: newAnalysisStages(c.Config, c.Root, cfg, deps), Writer: baselineWriterAdapter{}}
	if _, err := service.Execute(ctx, application.BaselineRequest{ConfigPath: c.Config, Root: c.Root, Path: bPath, NoAdvisories: c.NoAdvisories}); err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	_, _ = fmt.Fprintf(deps.Stdout, "baseline saved: %s\n", bPath)
	return nil
}
