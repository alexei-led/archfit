package main

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/decision"
	"github.com/alexei-led/archfit/internal/engine"
	"github.com/alexei-led/archfit/internal/llm"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/output/console"
	"github.com/alexei-led/archfit/internal/output/jsonout"
	"github.com/alexei-led/archfit/internal/output/markdown"
	"github.com/alexei-led/archfit/internal/output/sarif"
	"github.com/alexei-led/archfit/internal/output/scorecard"
	"github.com/alexei-led/archfit/internal/score"
)

// AnalyzeCmd is the single unified analysis command. It is also the default
// command: bare `archfit`, `archfit --json`, `archfit --gate` all route here.
//
// Without --gate the command is report-only (exit 0 on success, exit 3 on
// config/tool error). With --gate the exit codes match the old check command:
// 0 = clean, 1 = architecture policy failed, 2 = warnings, 3 = config/tool error.
//
// With --base <ref> the command runs a scorecard delta (old diff) instead of a
// single-run render.
//
// With --llm the command appends an off-gate LLM narrative section after the
// normal render (old review).
type AnalyzeCmd struct {
	Config string `short:"c" help:"Path to config file (optional; built-in defaults used if absent)." default:".archfit.yaml"`
	Root   string `help:"Repository root to analyze (default: directory of --config). Use this when a CI policy config lives outside the checked-out repo." type:"path"`
	Base   string `help:"Git ref to compare against for scorecard delta (e.g. main, HEAD~1). When set, runs a before/after delta table instead of a single-run render."`

	Gate bool `help:"CI exit behavior: 0=clean, 1=architecture violation, 2=warnings, 3=config/tool error. Without --gate the command is always report-only (exit 0 on success)."`
	LLM  bool `help:"Append an off-gate LLM narrative review after the normal render. Requires ai configured in the config file."`

	NoCache bool `name:"no-cache" help:"With --llm: bypass the LLM response cache."`

	// Format shorthand flags — mutually exclusive; error if more than one set.
	// These map onto Format below.
	JSON     bool `name:"json"     help:"Output format: JSON (shorthand for --format json)."`
	Markdown bool `name:"markdown" help:"Output format: Markdown (shorthand for --format markdown)."`
	Sarif    bool `name:"sarif"    help:"Output format: SARIF (shorthand for --format sarif)."`

	// Format is the advanced repeatable form. Default is text when no shorthand
	// flag is set. Valid values: json, text, markdown, md, sarif, scorecard.
	Format []string `name:"format" help:"Output format: json, text, markdown, md, sarif, scorecard. Repeatable." enum:"json,text,markdown,md,sarif,scorecard"`

	Full     bool `help:"Scan all files instead of only files changed since --base." default:"true"`
	Advisory bool `help:"Include informational Balanced-Coupling advisories in output." default:"true"`

	Severity     string   `name:"severity"      help:"Minimum advisory severity to show: low, medium, high, critical." enum:"low,medium,high,critical," default:""`
	Lang         []string `name:"lang"          help:"Analyzer name to force on. Repeatable. See analyzer setup docs for valid names."`
	NoConfig     bool     `name:"no-config"     help:"Skip the config file and use built-in defaults. Combine with --root for an ad-hoc scoring run."`
	RequireTools bool     `name:"require-tools" help:"Exit non-zero when any required analyzer tool is missing (opt-in hard gate)."`

	// Progress controls live phase reporting on stderr; Quiet suppresses it.
	Progress string `name:"progress" help:"Progress reporting on stderr: auto, plain, none." enum:"auto,plain,none," default:""`
	Quiet    bool   `name:"quiet"    short:"q" help:"Suppress progress output."`

	// providerOverride is a test seam — set directly on the struct to inject a
	// fake LLM provider (mirrors the old ReviewCmd seam). nil in production.
	providerOverride llm.Provider
}

func (*AnalyzeCmd) Help() string {
	return `Analyze architecture: run the full pipeline, print a verdict, scorecard, and findings.

Without --gate the command is report-only (always exit 0 on success). With --gate
it becomes a CI merge gate: 0 clean, 1 architecture violation, 2 warnings, 3 error.

Common runs:
  archfit                                         # report-only, text output
  archfit --gate -c .archfit.yaml --full          # CI gate
  archfit --markdown -c .archfit.yaml             # Markdown audit report
  archfit --json -c .archfit.yaml | jq .          # machine-readable JSON
  archfit --gate --format sarif > archfit.sarif   # SARIF for GitHub code scanning
  archfit --format scorecard                      # banded scorecard only
  archfit --base origin/main                      # scorecard delta vs base ref
  archfit --llm -c .archfit.yaml                  # add LLM narrative section

AI agents should read agent_tasks[] from JSON on exit 1 (requires --gate), make the
constrained repair, then rerun the validation command in that task.

Docs:
  CI: ` + ciDocsURL + `
  Agent feedback: ` + agentDocsURL + `
  Analyzer setup: ` + languagesDocsURL
}

func (c *AnalyzeCmd) Run(deps *appDeps) error {
	ctx := context.Background()

	formats, err := c.resolveFormats()
	if err != nil {
		return err
	}
	return c.runScan(ctx, deps, formats)
}

// resolveFormats merges the shorthand flags (--json/--markdown/--sarif) with
// the repeatable --format flag. Returns an error when multiple shorthands are set.
// Falls back to ["text"] when nothing is specified.
func (c *AnalyzeCmd) resolveFormats() ([]string, error) {
	shorthands := 0
	if c.JSON {
		shorthands++
	}
	if c.Markdown {
		shorthands++
	}
	if c.Sarif {
		shorthands++
	}
	if shorthands > 1 {
		return nil, &exitError{code: 3, msg: "error: --json, --markdown, and --sarif are mutually exclusive; use at most one"}
	}
	switch {
	case c.JSON:
		return []string{formatJSON}, nil
	case c.Markdown:
		return []string{formatMarkdown}, nil
	case c.Sarif:
		return []string{formatSarif}, nil
	case len(c.Format) > 0:
		return c.Format, nil
	default:
		return []string{formatText}, nil
	}
}

// runScan runs the pipeline for HEAD, optionally scores a --base ref for a
// before/after delta, and renders in each requested format. --base goes through
// this same path (not a separate flow) so the output format, JSON schema, and
// --gate / --require-tools / --advisory behavior are identical with or without it.
func (c *AnalyzeCmd) runScan(ctx context.Context, deps *appDeps, formats []string) error {
	rep := newProgressReporter(deps.stderr(), analyzePhaseTotal(c.LLM, c.Base != ""), c.Progress, c.Quiet, time.Now())
	rep.banner("Archfit analyzing " + analyzeTarget(c.Config, c.Root))
	deps.progress = rep.advance // runPipeline advances the discover/facts/graph phases
	defer rep.finish()

	rep.advance("Loading config")
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
	if slices.Contains(formats, formatScorecard) {
		advisory = true
	}

	// --base passes the ref so the HEAD scan also computes the changed-file set
	// for per-change metrics. Mode stays full, so the scorecard is a full scan
	// and no finding-delta buckets are produced (deltaReport gates on delta mode);
	// the before/after scorecard delta is computed separately by scoreBaseRef.
	mode := engine.Mode{
		Base:       c.Base,
		Full:       c.Full || c.Base != "",
		Advisory:   advisory,
		ReportOnly: !c.Gate,
		Formats:    formats,
	}

	// runPipeline synthesises the scorecard and applies the coupling gate
	// internally (before the verdict and agent_tasks freeze — V2 fix); the
	// "Scoring architecture" phase is reported from inside the pipeline.
	diag, sc, err := runPipeline(ctx, deps, cfg, c.Config, c.Root, c.NoConfig, mode, base)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	// Echo coupling-gate trip reasons from analyze only: runPipeline is shared
	// with baseline/enrich/explain/--base scoring, where an enforcement-sounding
	// stderr line is noise (and --base would print it twice). The gate decision
	// is pure, so re-evaluating here reproduces exactly what applyCouplingGate
	// applied to the verdict inside the pipeline.
	for _, r := range score.EvaluateCouplingGate(sc, couplingGateView(cfg), base.CouplingScore()).Reasons {
		_, _ = fmt.Fprintln(deps.stderr(), "coupling gate: "+r)
	}

	// Apply the opt-in hard gate before rendering so the output shows the
	// effective gate per coverage gap.
	hardGate := applyToolGate(&diag, c.RequireTools)

	// --base: score the ref in a worktree and attach a before/after delta.
	var baseSC *score.Scorecard
	if c.Base != "" {
		rep.advance("Comparing against base")
		bsc, berr := scoreBaseRef(ctx, deps, c.Base, c.Config, configDir, c.Root, c.NoConfig, advisory)
		if berr != nil {
			return berr
		}
		baseSC = &bsc
	}

	if err := analyzeRender(deps, diag, sc, baseSC, formats, hardGate); err != nil {
		return err
	}

	// With --llm: append the off-gate LLM narrative after the deterministic output.
	// The LLM is advisory and off-gate: a narration failure must never change the
	// exit code or mask the gate verdict (a failing policy must still exit 1, not
	// the LLM error's 3). The deterministic report is already rendered above; warn
	// to stderr and fall through to the gate decision.
	if c.LLM {
		rep.advance("Asking LLM for interpretation")
		if err := c.appendLLMNarrative(ctx, deps, cfg, configDir, diag, sc); err != nil {
			_, _ = fmt.Fprintf(deps.stderr(), "archfit: LLM narration unavailable (off-gate, ignored): %v\n", err)
		}
	}

	// Hard tool-gates (--require-tools or per-tool gate:fail in config) always
	// exit 1 when tripped, regardless of --gate. They are explicit opt-in gates
	// on tool presence, not suppressible by report-only mode.
	if hardGate {
		return &exitError{code: 1}
	}
	// Without --gate: always exit 0 on success (report-only mode).
	if !c.Gate {
		return nil
	}
	return verdictToError(diag.Verdict)
}

// analyzeRender writes the diagnostic to deps.Stdout in each requested format.
// The text format is the terminal-native decision report (decision.Build →
// console.RenderReport); JSON/SARIF/markdown/scorecard keep their existing
// machine/artifact renderers (unchanged contract). hardGate feeds the decision
// band so a tripped tool-gate reads as FAIL even in report-only mode.
func analyzeRender(deps *appDeps, diag diagnostic.Diagnostic, sc score.Scorecard, base *score.Scorecard, formats []string, hardGate bool) error {
	for _, format := range formats {
		var renderErr error
		switch format {
		case formatJSON:
			renderErr = jsonout.New().Render(diag, sc, base, deps.Stdout)
		case formatText:
			renderErr = console.RenderReport(decision.Build(diag, sc, base, hardGate), deps.Stdout)
		case formatMD, formatMarkdown:
			// Lead with the decision summary, then the detailed audit.
			if rerr := markdown.RenderReport(decision.Build(diag, sc, base, hardGate), deps.Stdout); rerr != nil {
				renderErr = rerr
			} else {
				renderErr = markdown.New().Render(diag, deps.Stdout)
			}
		case formatSarif:
			renderErr = sarif.New().Render(diag, deps.Stdout)
		case formatScorecard:
			renderErr = scorecard.New().Render(diag, deps.Stdout)
		}
		if renderErr != nil {
			return &exitError{code: 3, msg: fmt.Sprintf("render %s: %v", format, renderErr)}
		}
	}
	return nil
}

// appendLLMNarrative appends the off-gate LLM narrative to stdout, after the
// deterministic report. runLLMReview owns the ai check and provider
// build (exit 3 when unconfigured); we only thread the test-seam override.
func (c *AnalyzeCmd) appendLLMNarrative(ctx context.Context, deps *appDeps, cfg config.Config, configDir string, diag diagnostic.Diagnostic, sc score.Scorecard) error {
	return runLLMReview(ctx, deps, cfg, configDir, c.NoCache, c.providerOverride, diag, sc)
}

// analyzePhaseTotal returns the number of progress phases for this run. Base
// phases: load config, discover project, collect facts, analyze dependencies,
// score. --llm adds an interpretation phase.
func analyzePhaseTotal(llm, base bool) int {
	total := 5 // config, discover, facts, analyze, score
	if base {
		total++ // compare against base
	}
	if llm {
		total++ // LLM interpretation
	}
	return total
}

// analyzeTarget returns a short repo label for the progress banner: the base
// name of --root, or of the config file's directory.
func analyzeTarget(configPath, root string) string {
	dir := root
	if dir == "" {
		dir = filepath.Dir(configPath)
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	return filepath.Base(dir)
}
