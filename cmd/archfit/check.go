package main

import "context"

// CheckCmd is the CI gate command. It runs the same scan pipeline as analyze
// but returns non-zero exit codes for violations and warnings.
type CheckCmd struct {
	Config string `short:"c" help:"Path to config file." default:".archfit.yaml"`
	Root   string `help:"Repository root to analyze (default: directory of --config). Use this when a CI policy config lives outside the checked-out repo." type:"path"`
	Base   string `help:"Git ref to compare against for scorecard delta (e.g. main, HEAD~1). When set, runs a before/after delta table instead of a single-run render."`

	NoAdvisories bool   `name:"no-advisories" help:"Hide informational Balanced-Coupling advisories from the output."`
	MinSeverity  string `name:"min-severity" help:"Minimum advisory severity to show: low, medium, high, critical." enum:"low,medium,high,critical," default:""`
	Refresh      bool   `name:"refresh" help:"Re-run all extractors and refresh the cache. Use after installing or updating analyzer tools."`
	RequireTools bool   `name:"require-tools" help:"Exit non-zero when any required analyzer tool is missing."`

	JSON     bool     `name:"json" help:"Output format: JSON (shorthand for --format json)."`
	Markdown bool     `name:"markdown" help:"Output format: Markdown (shorthand for --format markdown)."`
	Sarif    bool     `name:"sarif" help:"Output format: SARIF (shorthand for --format sarif)."`
	Format   []string `name:"format" help:"Output format: json, text, markdown, md, sarif, scorecard. Repeatable." enum:"json,text,markdown,md,sarif,scorecard"`

	Progress string `name:"progress" help:"Progress reporting on stderr: auto, plain, none." enum:"auto,plain,none," default:""`
	Quiet    bool   `short:"q" help:"Suppress progress output."`
}

func (*CheckCmd) Help() string {
	return `Use this command in CI and agent validation loops.

Common runs:
  archfit check -c .archfit.yaml
  archfit check --json -c .archfit.yaml
  archfit check --base origin/main --format sarif > archfit.sarif
  archfit check --require-tools -c .archfit.yaml`
}

func (c *CheckCmd) Run(deps *appDeps) error {
	ctx := context.Background()
	return runScan(ctx, deps, scanRequest{
		configPath:   c.Config,
		root:         c.Root,
		baseRef:      c.Base,
		json:         c.JSON,
		markdown:     c.Markdown,
		sarif:        c.Sarif,
		formats:      c.Format,
		noAdvisories: c.NoAdvisories,
		minSeverity:  c.MinSeverity,
		requireTools: c.RequireTools,
		progress:     c.Progress,
		quiet:        c.Quiet,
		refresh:      c.Refresh,
		reportOnly:   false,
	})
}
