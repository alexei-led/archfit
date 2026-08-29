package main

import "context"

// CheckCmd is the CI gate command. It runs the same scan pipeline as analyze
// but maps the architecture verdict onto the process exit code: healthy 0,
// needs_attention 2, blocked 1.
type CheckCmd struct {
	Config string `short:"c" help:"Path to config file." default:".archfit.yaml"`
	Root   string `help:"Repository root to analyze (default: directory of --config). Use this when a CI policy config lives outside the checked-out repo." type:"path"`
	Base   string `help:"Git ref to compare against (e.g. main, HEAD~1). When set, the normal output gains a base-vs-head delta."`

	NoAdvisories bool     `name:"no-advisories" help:"Hide informational Balanced-Coupling advisories from the output."`
	MinSeverity  string   `name:"min-severity" help:"Minimum advisory severity to show: low, medium, high, critical." enum:"low,medium,high,critical," default:""`
	Refresh      bool     `name:"refresh" help:"Re-run all extractors and refresh the cache. Use after installing or updating analyzer tools."`
	RequireTools bool     `name:"require-tools" help:"Exit non-zero when any required analyzer tool is missing."`
	Lang         []string `name:"lang" help:"Analyzer name to force on. Repeatable. See analyzer setup docs for valid names."`

	JSON     bool     `name:"json" help:"Output format: JSON (shorthand for --format json)."`
	Markdown bool     `name:"markdown" help:"Output format: Markdown (shorthand for --format markdown)."`
	Sarif    bool     `name:"sarif" help:"Output format: SARIF (shorthand for --format sarif)."`
	Format   []string `name:"format" help:"Output format: json, text, markdown, md, sarif, scorecard. Repeatable." enum:"json,text,markdown,md,sarif,scorecard"`

	Progress string `name:"progress" help:"Progress reporting on stderr: auto, plain, none." enum:"auto,plain,none," default:""`
	Quiet    bool   `short:"q" help:"Suppress progress output."`
}

func (*CheckCmd) Help() string {
	return `Use this command in CI and agent validation loops.

Exit codes follow the architecture verdict:
  0  healthy         — every dimension measured, every hard gate passing, nothing active
  2  needs_attention — no blocker, but an active diagnostic or an unmeasured dimension
  1  blocked         — an active hard-gate finding, or a required analyzer that did not run
  3  usage, config, or tool error — no valid report was produced

A coupling advisory is a diagnostic, never a blocker: it can reach exit 2, never
exit 1. The only coupling gate is coupling.gate.distributed_monolith, and it
blocks only in mode: fail against a comparable reference.

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
		lang:         c.Lang,
		requireTools: c.RequireTools,
		progress:     c.Progress,
		quiet:        c.Quiet,
		refresh:      c.Refresh,
		reportOnly:   false,
	})
}
