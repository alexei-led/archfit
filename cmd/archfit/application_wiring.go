package main

import (
	"github.com/alexei-led/archfit/v3/internal/application"
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
// acquisition stage and the base-tree comparison
// adapters. Every field is a constructed value — no branching decision lives
// here, only the choice of concrete implementation.
func newAnalysisStages(configPath, root string, cfg config.Config, deps *appDeps) application.StageExecutor {
	return application.StageExecutor{
		Preparer:  config.Preparer{Config: cfg, Stderr: deps.stderr()},
		Evidence:  newEvidenceService(configPath, root, cfg, deps),
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
