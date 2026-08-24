package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/decision"
	"github.com/alexei-led/archfit/internal/engine"
	"github.com/alexei-led/archfit/internal/llm"
	"github.com/alexei-led/archfit/internal/model/scan"
	"github.com/alexei-led/archfit/internal/output/console"
	"github.com/alexei-led/archfit/internal/output/jsonout"
	"github.com/alexei-led/archfit/internal/output/markdown"
	"github.com/alexei-led/archfit/internal/output/sarif"
	"github.com/alexei-led/archfit/internal/output/scorecard"
	"github.com/alexei-led/archfit/internal/score"
)

// AnalyzeCmd is the local, report-only analysis command. It runs the same scan
// pipeline as `archfit check` but always exits 0 on a successful run, even when
// findings or warnings exist.
type AnalyzeCmd struct {
	Config string `short:"c" help:"Path to config file." default:".archfit.yaml"`
	Root   string `help:"Repository root to analyze (default: directory of --config). Use this when a CI policy config lives outside the checked-out repo." type:"path"`
	Base   string `help:"Git ref to compare against (e.g. main, HEAD~1). When set, the normal output gains a base-vs-head delta."`

	AISummary bool `name:"ai-summary" help:"Append an off-gate AI narrative review after the normal render. Requires ai configured in the config file."`
	Refresh   bool `name:"refresh" help:"Re-run all extractors and refresh the cache. Use after installing or updating analyzer tools."`

	// Format shorthand flags — mutually exclusive; also mutually exclusive with
	// the repeatable --format flag.
	JSON     bool `name:"json"     help:"Output format: JSON (shorthand for --format json)."`
	Markdown bool `name:"markdown" help:"Output format: Markdown (shorthand for --format markdown)."`
	Sarif    bool `name:"sarif"    help:"Output format: SARIF (shorthand for --format sarif)."`

	// Format is the advanced repeatable form. Default is text when no shorthand
	// flag is set. Valid values: json, text, markdown, md, sarif, scorecard.
	Format []string `name:"format" help:"Output format: json, text, markdown, md, sarif, scorecard. Repeatable." enum:"json,text,markdown,md,sarif,scorecard"`

	NoAdvisories bool     `name:"no-advisories" help:"Hide informational Balanced-Coupling advisories from the output."`
	MinSeverity  string   `name:"min-severity" help:"Minimum advisory severity to show: low, medium, high, critical." enum:"low,medium,high,critical," default:""`
	Lang         []string `name:"lang" help:"Analyzer name to force on. Repeatable. See analyzer setup docs for valid names."`
	RequireTools bool     `name:"require-tools" help:"Mark missing required analyzer tools as fail in the rendered verdict."`

	// Progress controls live phase reporting on stderr; Quiet suppresses it.
	Progress string `name:"progress" help:"Progress reporting on stderr: auto, plain, none." enum:"auto,plain,none," default:""`
	Quiet    bool   `name:"quiet" short:"q" help:"Suppress progress output."`

	// providerOverride is a test seam — set directly on the struct to inject a
	// fake AI provider. nil in production.
	providerOverride llm.Provider
}

func (*AnalyzeCmd) Help() string {
	return `Analyze architecture locally: run the full pipeline, print a verdict, scorecard, and findings.

This command is report-only: it always exits 0 on success. Use ` + "`archfit check`" + ` in CI when findings or warnings should fail the run.

Common runs:
  archfit analyze                               # local text report
  archfit analyze --markdown -c .archfit.yaml   # Markdown audit report
  archfit analyze --json -c .archfit.yaml | jq .
  archfit analyze --format sarif > archfit.sarif
  archfit analyze --format scorecard            # banded scorecard only
  archfit analyze --base origin/main            # add a base-vs-head delta
  archfit analyze --ai-summary -c .archfit.yaml # add AI narrative section

AI agents should read agent_tasks[] from JSON output, make the constrained
repair, then rerun the validation command in that task.

Docs:
  CI: ` + ciDocsURL + `
  Agent feedback: ` + agentDocsURL + `
  Analyzer setup: ` + languagesDocsURL
}

func (c *AnalyzeCmd) Run(deps *appDeps) error {
	ctx := context.Background()
	return runScan(ctx, deps, scanRequest{
		configPath:       c.Config,
		root:             c.Root,
		baseRef:          c.Base,
		json:             c.JSON,
		markdown:         c.Markdown,
		sarif:            c.Sarif,
		formats:          c.Format,
		noAdvisories:     c.NoAdvisories,
		minSeverity:      c.MinSeverity,
		lang:             c.Lang,
		requireTools:     c.RequireTools,
		progress:         c.Progress,
		quiet:            c.Quiet,
		refresh:          c.Refresh,
		aiSummary:        c.AISummary,
		providerOverride: c.providerOverride,
		reportOnly:       true,
	})
}

type scanRequest struct {
	configPath string
	root       string
	baseRef    string

	json     bool
	markdown bool
	sarif    bool
	formats  []string

	noAdvisories bool
	minSeverity  string
	lang         []string
	requireTools bool
	progress     string
	quiet        bool
	refresh      bool
	aiSummary    bool
	reportOnly   bool

	providerOverride llm.Provider
}

// resolveScanFormats merges the shorthand flags (--json/--markdown/--sarif)
// with the repeatable --format flag. Returns an error when multiple
// shorthands are set or shorthand and --format are mixed. Falls back to
// ["text"] when nothing is specified.
func resolveScanFormats(jsonFlag, markdownFlag, sarifFlag bool, formats []string) ([]string, error) {
	shorthands := 0
	if jsonFlag {
		shorthands++
	}
	if markdownFlag {
		shorthands++
	}
	if sarifFlag {
		shorthands++
	}
	if shorthands > 1 {
		return nil, &exitError{code: 3, msg: "error: --json, --markdown, and --sarif are mutually exclusive; use at most one"}
	}
	if shorthands > 0 && len(formats) > 0 {
		return nil, &exitError{code: 3, msg: "error: --json/--markdown/--sarif and --format are mutually exclusive"}
	}
	switch {
	case jsonFlag:
		return []string{formatJSON}, nil
	case markdownFlag:
		return []string{formatMarkdown}, nil
	case sarifFlag:
		return []string{formatSarif}, nil
	case len(formats) > 0:
		return formats, nil
	default:
		return []string{formatText}, nil
	}
}

// runScan runs the shared analysis pipeline for both `archfit analyze` and
// `archfit check`. reportOnly=true keeps the run local/reporting-only: exit 0 on
// success. reportOnly=false applies CI gate exit codes: 0 clean, 1 violations,
// 2 warnings, 3 config/tool error.
func runScan(ctx context.Context, deps *appDeps, req scanRequest) error {
	formats, err := resolveScanFormats(req.json, req.markdown, req.sarif, req.formats)
	if err != nil {
		return err
	}

	rep := newProgressReporter(deps.stderr(), analyzePhaseTotal(req.aiSummary, req.baseRef != ""), req.progress, req.quiet, time.Now())
	rep.banner("Archfit analyzing " + analyzeTarget(req.configPath, req.root))
	deps.progress = rep.advance
	deps.refresh = req.refresh
	defer rep.finish()

	rep.advance("Loading config")
	cfg, err := loadAnalysisConfig(ctx, req.configPath)
	if err != nil {
		return configLoadError(err)
	}
	if err := applyFlagOverrides(&cfg, req.minSeverity, req.lang); err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	printConfigLint(deps.stderr(), cfg.Lint())

	// Snapshot the EFFECTIVE config for the --base sub-run before the head
	// pipeline touches it. runPipeline's owner and deploy-unit backfill
	// (FillMissingOwners / FillMissingDeployUnits) writes through the shared
	// cfg.Modules map, so without an independent map the base side would inherit
	// head-tree owners, skip its own resolution, and classify distance against
	// evidence its own tree never produced.
	baseCfg := cfg
	if req.baseRef != "" {
		baseCfg = withIndependentModules(cfg)
	}

	configDir := filepath.Dir(req.configPath)
	deps.scanRoot = req.root
	if deps.scanRoot == "" {
		deps.scanRoot = configDir
	}
	defer func() { deps.scanRoot = "" }()

	base, err := baseline.Load(ctx, filepath.Join(configDir, defaultBaselinePath))
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	// --no-advisories has one meaning for every requested format: the scorecard
	// value is synthesised from ClassifiedEdges (before advisory filtering), so
	// suppressing advisory findings never moves the score and the format no
	// longer needs to force them back on.
	advisory := !req.noAdvisories

	mode := engine.Mode{
		Base:       req.baseRef,
		Full:       true,
		Advisory:   advisory,
		ReportOnly: req.reportOnly,
		Formats:    formats,
	}

	// One run context for the whole comparison: the head run and the --base
	// sub-run share the config source, the config bundle directory, and one
	// sampled evaluation instant. Only the scan root differs (the base side
	// swaps in its worktree), so a finding never ages differently between the
	// two sides just because the second pipeline started later.
	rc := newRunContext(req.configPath, req.root)
	rc.EvaluatedAt = time.Now()

	diag, sc, err := runPipeline(ctx, deps, cfg, rc, mode, base)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	emitHealthWarnings(deps, diag, cfg, deps.scanRoot, req.configPath)

	gateView := couplingGateView(cfg)
	for _, reason := range score.EvaluateCouplingGate(sc, gateView, base.CouplingScore()).Reasons {
		_, _ = fmt.Fprintln(deps.stderr(), "coupling gate: "+reason)
	}
	if gateView.Enabled && gateView.MaxDrop != nil {
		if mismatches := base.ScoreSnapshotMismatches(); len(mismatches) > 0 {
			_, _ = fmt.Fprintf(deps.stderr(),
				"coupling gate: max_drop skipped — baseline score snapshot is incompatible (%s); run `archfit baseline` to re-anchor\n",
				strings.Join(scoreSnapshotMismatchDetails(base, mismatches), ", "))
		}
	}

	hardGate := applyToolGate(&diag, req.requireTools)

	var baseSC *score.Scorecard
	if req.baseRef != "" {
		rep.advance("Comparing against base")
		bsc, bev, berr := scoreBaseRef(ctx, deps, req.baseRef, rc, baseCfg, advisory)
		if berr != nil {
			return berr
		}
		baseSC = &bsc
		// Report-only: the origin block is attached after the verdict and the
		// tool gate are final, so it can never change either.
		diag.GitFindingDelta = buildGitFindingDelta(gitDeltaInput{
			BaseRef:        req.baseRef,
			Tasks:          diag.AgentTasks,
			BaseFindingIDs: bev.FindingIDs,
			Head:           analyzerEvidence{Coverage: diag.ToolCoverage, Gaps: diag.CoverageGaps, Hash: diag.ConfigHash},
			Base:           analyzerEvidence{Coverage: bev.Coverage, Gaps: bev.CoverageGaps, Hash: bev.ConfigHash},
			Families:       analyzerFamilies(cfg),
		})
	}

	if err := analyzeRender(deps, diag, sc, baseSC, formats, hardGate); err != nil {
		return err
	}

	if req.aiSummary {
		rep.advance("Asking AI for interpretation")
		if err := appendAISummary(ctx, deps, cfg, req.configPath, req.root, req.refresh, req.providerOverride, diag, sc, formats); err != nil {
			_, _ = fmt.Fprintf(deps.stderr(), "archfit: AI narrative unavailable (off-gate, ignored): %v\n", err)
		}
	}

	if req.reportOnly {
		return nil
	}
	if hardGate {
		return &exitError{code: 1}
	}
	return verdictToError(diag.Verdict)
}

// loadAnalysisConfig loads the config for analyze/check. Config is always
// required: unlike loadConfig, it does NOT fall back to config.Default() when
// .archfit.yaml is absent. Call loadConfig directly only for commands that
// tolerate a missing config (doctor, explain).
func loadAnalysisConfig(ctx context.Context, path string) (config.Config, error) {
	cfg, err := config.Load(ctx, path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return config.Config{}, &exitError{code: 3, msg: fmt.Sprintf("error: config not found: %s\n\u2192 run: archfit config init --root .", path)}
		}
		return config.Config{}, err
	}
	if err := validateConfigRules(cfg); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

// configLoadError normalises a config-load failure into an exit-3 error.
// loadAnalysisConfig already returns an *exitError for the missing-config case,
// and that message carries its own "error: " prefix plus a next-step hint —
// wrapping it a second time prints the prefix twice.
func configLoadError(err error) error {
	var already *exitError
	if errors.As(err, &already) {
		return already
	}
	return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
}

// analyzeRender writes the diagnostic to deps.Stdout in each requested format.
// The text format is the terminal-native decision report (decision.Build →
// console.RenderReport); JSON/SARIF/markdown/scorecard keep their existing
// machine/artifact renderers (unchanged contract). hardGate feeds the decision
// band so a tripped tool-gate reads as FAIL even in report-only mode.
func analyzeRender(deps *appDeps, diag scan.Diagnostic, sc score.Scorecard, base *score.Scorecard, formats []string, hardGate bool) error {
	for _, format := range formats {
		var renderErr error
		switch format {
		case formatJSON:
			renderErr = jsonout.New().Render(diag, sc, base, deps.Stdout)
		case formatText:
			renderErr = console.RenderReport(decision.Build(diag, sc, base, hardGate), deps.Stdout)
		case formatMD, formatMarkdown:
			if rerr := markdown.RenderReport(decision.Build(diag, sc, base, hardGate), deps.Stdout); rerr != nil {
				renderErr = rerr
			} else {
				renderErr = markdown.New().Render(diag, deps.Stdout)
			}
		case formatSarif:
			renderErr = sarif.New().Render(diag, deps.Stdout)
		case formatScorecard:
			renderErr = scorecard.New().Render(diag, sc, deps.Stdout)
		}
		if renderErr != nil {
			return &exitError{code: 3, msg: fmt.Sprintf("render %s: %v", format, renderErr)}
		}
	}
	return nil
}

// appendAISummary emits the off-gate AI narrative after the deterministic
// report. Text/Markdown runs append to stdout; machine-only formats route the
// review to stderr so stdout remains parseable JSON/SARIF/scorecard output.
func appendAISummary(ctx context.Context, deps *appDeps, cfg config.Config, configPath, root string, refresh bool, providerOverride llm.Provider, diag scan.Diagnostic, sc score.Scorecard, formats []string) error {
	reviewDeps := deps
	if !llmReviewCanUseStdout(formats) {
		copyDeps := *deps
		copyDeps.Stdout = deps.stderr()
		reviewDeps = &copyDeps
	}
	return runLLMReview(ctx, reviewDeps, cfg, configPath, root, refresh, providerOverride, diag, sc)
}

func llmReviewCanUseStdout(formats []string) bool {
	for _, format := range formats {
		switch format {
		case formatJSON, formatSarif, formatScorecard:
			return false
		}
	}
	return true
}

// analyzePhaseTotal returns the number of progress phases for this run. Base
// phases: load config, discover project, collect facts, analyze dependencies,
// score. --ai-summary adds an interpretation phase.
func analyzePhaseTotal(aiSummary, base bool) int {
	total := 5
	if base {
		total++
	}
	if aiSummary {
		total++
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
