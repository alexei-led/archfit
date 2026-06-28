package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"slices"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/engine"
	"github.com/alexei-led/archfit/internal/output/console"
	"github.com/alexei-led/archfit/internal/output/jsonout"
	"github.com/alexei-led/archfit/internal/output/markdown"
	"github.com/alexei-led/archfit/internal/output/sarif"
	"github.com/alexei-led/archfit/internal/output/scorecard"
)

// CheckCmd runs the full archfit analysis pipeline.
type CheckCmd struct {
	Config   string   `short:"c" help:"Path to config file (optional; built-in defaults used if absent)." default:".archfit.yaml"`
	Root     string   `help:"Repository root to analyze (default: directory of --config). Use this when a CI policy config lives outside the checked-out repo." type:"path"`
	Base     string   `help:"Git ref to compare against for PR/delta mode (e.g. origin/main, HEAD~1)."`
	Full     bool     `help:"Scan all files instead of only files changed since --base."`
	Format   []string `help:"Output format for stdout: text, json, markdown, md, sarif, scorecard. Repeatable." enum:"json,text,markdown,md,sarif,scorecard" default:"text"`
	Advisory bool     `help:"Include informational Balanced-Coupling advisories in output."`
	Report   bool     `help:"Render findings but never fail for architecture violations; useful for report artifacts."`
	NoConfig bool     `name:"no-config" help:"Skip the config file and use built-in defaults. Combine with --lang and --severity for an ad-hoc scan."`

	RequireTools bool `name:"require-tools" help:"Fail when analyzer coverage is incomplete. Default is warn-loud: report n/a coverage gaps without blocking."`

	// Overrides — each flag overrides the equivalent setting from the config file.
	Severity string   `name:"severity" help:"Minimum advisory severity to show: low, medium, high, critical. Default: medium." enum:"low,medium,high,critical," default:""`
	Lang     []string `name:"lang"     help:"Analyzer name to force on. Repeatable. See analyzer setup docs for valid names."`
}

func (*CheckCmd) Help() string {
	return `Use check as the merge gate. Exit codes are stable for scripts: 0 means clean, 1 means an architecture policy failed, and 3 means a config or tool error.

Common runs:
  archfit check --config .archfit.yaml --full
  archfit check --config .archfit.yaml --base origin/main --format json
  archfit check --config .archfit.yaml --full --format sarif > archfit.sarif

AI agents should read agent_tasks[] from JSON on exit 1, make the constrained repair, then rerun the validation command in that task.

Docs:
  CI: ` + ciDocsURL + `
  Agent feedback: ` + agentDocsURL + `
  Analyzer setup: ` + languagesDocsURL
}

func (c *CheckCmd) Run(deps *appDeps) error {
	ctx := context.Background()

	cfg, err := loadConfig(ctx, c.Config, c.NoConfig)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	if err := applyFlagOverrides(&cfg, c.Severity, c.Lang); err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	// Config-quality lint → stderr (advisory; never gates, never pollutes the
	// stdout JSON/markdown contract that the determinism double-run diffs).
	printConfigLint(deps.stderr(), cfg.Lint())

	configDir := filepath.Dir(c.Config)
	base, err := baseline.Load(ctx, filepath.Join(configDir, defaultBaselinePath))
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	// The scorecard's coupling_balance dimension reads the Balanced-Coupling
	// advisory edges; force advisory on when that format is requested so the BC
	// edges reach the synthesis (advisory is additive — other formats are
	// unaffected except for the extra advisory findings they already opt into).
	advisory := c.Advisory
	if slices.Contains(c.Format, "scorecard") {
		advisory = true
	}

	mode := engine.Mode{
		Base:       c.Base,
		Full:       c.Full,
		Advisory:   advisory,
		ReportOnly: c.Report,
		Formats:    c.Format,
	}

	diag, err := runPipeline(ctx, deps, cfg, c.Config, c.Root, c.NoConfig, mode, base)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	// Warn when --base was given but no baseline file exists: every finding
	// appears "new" without this notice, silently misleading the reader.
	if c.Base != "" && base.SchemaVersion == "" {
		_, _ = fmt.Fprintf(deps.stderr(),
			"no baseline found at %s — all %d findings are untracked; run `archfit baseline` to enable drift detection\n",
			c.Base, len(diag.Findings))
	}

	// Resolve the opt-in hard gate before rendering so the output shows the
	// effective gate per coverage gap. A tripped gate is a policy failure (exit 1),
	// distinct from a tool/config error (exit 3), and is NOT suppressed by --report:
	// --report waives architecture findings, but a tool the user explicitly required
	// (--require-tools / tools.<x>.gate: fail) is a deliberate CI block.
	hardGate := applyToolGate(&diag, c.RequireTools)

	for _, format := range c.Format {
		var renderErr error
		switch format {
		case "json":
			renderErr = jsonout.New().Render(diag, deps.Stdout)
		case "text":
			renderErr = console.New().Render(diag, deps.Stdout)
		case "md", "markdown":
			renderErr = markdown.New().Render(diag, deps.Stdout)
		case "sarif":
			renderErr = sarif.New().Render(diag, deps.Stdout)
		case "scorecard":
			renderErr = scorecard.New().Render(diag, deps.Stdout)
		}
		if renderErr != nil {
			return &exitError{code: 3, msg: fmt.Sprintf("render %s: %v", format, renderErr)}
		}
	}

	// A tripped hard tool-gate exits 1 regardless of --report (see applyToolGate).
	if hardGate {
		return &exitError{code: 1}
	}

	// --report promises "never exit with a failure code": the verdict is still
	// rendered, but findings and metric regressions do not affect the exit.
	if c.Report {
		return nil
	}
	return verdictToError(diag.Verdict)
}

// printConfigLint writes config-quality warnings to w (stderr). It is silent
// when there are none. The header explains why under-specified modules matter,
// then one line per module names the omitted fields. Deterministic; advisory.
func printConfigLint(w io.Writer, warnings []config.LintWarning) {
	if len(warnings) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "config-quality: %d module(s) under-specified — "+
		"degrades distance/volatility classification (can cause BC advisory floods):\n",
		len(warnings))
	for _, warn := range warnings {
		_, _ = fmt.Fprintf(w, "  - %s\n", warn.String())
	}
}

// ScanCmd is a convenience alias for a full advisory Markdown audit.
// Always runs: check --full --advisory --report --format markdown.
// For custom combinations (e.g. without advisory), use the check command directly.
type ScanCmd struct {
	Config       string `short:"c" help:"Config file." default:".archfit.yaml"`
	Root         string `help:"Repository root to analyze (default: directory of --config)." type:"path"`
	RequireTools bool   `name:"require-tools" help:"Exit non-zero when any required analyzer tool is missing (opt-in hard gate)."`
}

func (c *ScanCmd) Run(deps *appDeps) error {
	check := CheckCmd{
		Config:       c.Config,
		Root:         c.Root,
		Full:         true,
		Advisory:     true,
		Report:       true,
		Format:       []string{"markdown"},
		RequireTools: c.RequireTools,
	}
	return check.Run(deps)
}
