package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/engine"
	"github.com/alexei-led/archfit/internal/output/console"
	"github.com/alexei-led/archfit/internal/output/jsonout"
	"github.com/alexei-led/archfit/internal/output/markdown"
	"github.com/alexei-led/archfit/internal/output/sarif"
)

// CheckCmd runs the full archfit analysis pipeline.
type CheckCmd struct {
	Config   string   `short:"c" help:"Path to config file (optional; built-in defaults used if absent)." default:".archfit.yaml"`
	Base     string   `help:"Git ref to compare against for incremental mode (e.g. main, HEAD~1)."`
	Full     bool     `help:"Scan all files, not just files changed since --base."`
	Format   []string `help:"Output format: text (human-readable), json, markdown, md, sarif. Repeatable." enum:"json,text,markdown,md,sarif" default:"text"`
	Advisory bool     `help:"Include informational findings (coupling advisories) in output."`
	Report   bool     `help:"Never exit with a failure code, even when violations are found."`
	NoConfig bool     `name:"no-config" help:"Skip config file entirely; use built-in defaults. Combine with --lang and --severity to run without any config file."`

	// Overrides — each flag overrides the equivalent setting from the config file.
	Severity string   `name:"severity" help:"Show only coupling advisories at or above this level: low, medium, high, critical. Default: medium." enum:"low,medium,high,critical," default:""`
	Lang     []string `name:"lang"     help:"Languages to analyze: go, ts, py. Repeatable: --lang go --lang ts. Sets each to 'on'; unspecified languages follow config or auto-detect."`
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
	printConfigLint(os.Stderr, cfg.Lint())

	configDir := filepath.Dir(c.Config)
	base, err := baseline.Load(ctx, filepath.Join(configDir, defaultBaselinePath))
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	mode := engine.Mode{
		Base:       c.Base,
		Full:       c.Full,
		Advisory:   c.Advisory,
		ReportOnly: c.Report,
		Formats:    c.Format,
	}

	diag, err := runPipeline(ctx, deps, cfg, c.Config, c.NoConfig, mode, base)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

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
		}
		if renderErr != nil {
			return &exitError{code: 3, msg: fmt.Sprintf("render %s: %v", format, renderErr)}
		}
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
	Config string `short:"c" help:"Config file." default:".archfit.yaml"`
}

func (c *ScanCmd) Run(deps *appDeps) error {
	check := CheckCmd{
		Config:   c.Config,
		Full:     true,
		Advisory: true,
		Report:   true,
		Format:   []string{"markdown"},
	}
	return check.Run(deps)
}
