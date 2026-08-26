package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/llm"
	"github.com/alexei-led/archfit/internal/model/report"
	"github.com/alexei-led/archfit/internal/output/console"
	"github.com/alexei-led/archfit/internal/output/jsonout"
	"github.com/alexei-led/archfit/internal/output/markdown"
	"github.com/alexei-led/archfit/internal/output/sarif"
	"github.com/alexei-led/archfit/internal/output/scorecard"
	reportports "github.com/alexei-led/archfit/internal/report/ports"
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

// runScan invokes the application-owned Analyze/Check use case, then renders
// the returned report document and maps the application exit decision to the
// CLI error contract.
func runScan(ctx context.Context, deps *appDeps, req scanRequest) error {
	// A conflicting output-format combination is a usage error: decide it before
	// the progress banner and the config read, so `--json --markdown -c
	// missing.yaml` names the flag conflict instead of a config the run never
	// needed. Format semantics stay owned by the application; cmd only decides
	// when the check runs.
	if err := application.ValidateFormats(req.json, req.markdown, req.sarif, req.formats); err != nil {
		return applicationExitError(err)
	}

	rep := newProgressReporter(deps.stderr(), analyzePhaseTotal(req.aiSummary, req.baseRef != ""), req.progress, req.quiet, time.Now())
	rep.banner("Archfit analyzing " + analyzeTarget(req.configPath, req.root))
	defer rep.finish()

	// Runtime flags and stage progress belong to this invocation. Wire them
	// before config preparation so the technical stage cannot silently fall
	// back to cache reads or a nil progress callback.
	deps.refresh = req.refresh
	deps.progress = rep.advance

	rep.advance("Loading config")
	cfg, err := loadAnalysisConfig(ctx, req.configPath)
	if err != nil {
		return configLoadError(err)
	}
	if err := config.ApplyFlagOverrides(&cfg, req.minSeverity, req.lang); err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	resp, err := application.Service{Stages: newAnalyzeStages(req.configPath, req.root, cfg, deps)}.Execute(ctx, application.Request{
		BaseRef:      req.baseRef,
		JSON:         req.json,
		Markdown:     req.markdown,
		SARIF:        req.sarif,
		Formats:      req.formats,
		NoAdvisories: req.noAdvisories,
		RequireTools: req.requireTools,
	})
	if err != nil {
		return applicationExitError(err)
	}

	if err := analyzeRender(deps, resp); err != nil {
		return err
	}

	if req.aiSummary {
		rep.advance("Asking AI for interpretation")
		if err := appendAISummary(ctx, deps, cfg, req.configPath, req.root, req.refresh, req.providerOverride, resp.Document, resp.Formats); err != nil {
			_, _ = fmt.Fprintf(deps.stderr(), "archfit: AI narrative unavailable (off-gate, ignored): %v\n", err)
		}
	}

	if req.reportOnly {
		return nil
	}
	if code := outcomeExitCode(resp.Outcome); code != 0 {
		return &exitError{code: code}
	}
	return nil
}

// applicationExitError maps a controlled use-case failure onto the CLI exit
// contract. The use case names the failure; cmd owns the exit code and the one
// "error: " prefix, so main.go's uncoded fallback cannot add a second one.
func applicationExitError(err error) error {
	var formats *application.InvalidFormatsError
	var exec *application.ExecutionError
	if errors.As(err, &formats) || errors.As(err, &exec) {
		return &exitError{code: 3, msg: "error: " + err.Error()}
	}
	return err
}

func outcomeExitCode(outcome application.Outcome) int {
	switch outcome {
	case application.OutcomeFail:
		return 1
	case application.OutcomeWarn:
		return 2
	default:
		return 0
	}
}

// analyzeRender writes the document to deps.Stdout in each requested format.
// Every adapter implements the report rendering port (Document + io.Writer);
// cmd only selects the adapter for the resolved format name.
func analyzeRender(deps *appDeps, resp application.Response) error {
	renderers := map[string]reportports.Renderer{
		formatJSON:      jsonout.New(),
		formatText:      console.New(),
		formatMarkdown:  markdown.New(),
		formatMD:        markdown.New(),
		formatSarif:     sarif.New(),
		formatScorecard: scorecard.New(),
	}
	for _, format := range resp.Formats {
		r, ok := renderers[format]
		if !ok {
			return &exitError{code: 3, msg: fmt.Sprintf("render %s: unknown format", format)}
		}
		if err := r.Render(resp.Document, deps.Stdout); err != nil {
			return &exitError{code: 3, msg: fmt.Sprintf("render %s: %v", format, err)}
		}
	}
	return nil
}

// appendAISummary emits the off-gate AI narrative after the deterministic
// report. Text/Markdown runs append to stdout; machine-only formats route the
// review to stderr so stdout remains parseable JSON/SARIF/scorecard output.
func appendAISummary(ctx context.Context, deps *appDeps, cfg config.Config, configPath, root string, refresh bool, providerOverride llm.Provider, doc report.Document, formats []string) error {
	reviewDeps := deps
	if !llmReviewCanUseStdout(formats) {
		copyDeps := *deps
		copyDeps.Stdout = deps.stderr()
		reviewDeps = &copyDeps
	}
	return runLLMReview(ctx, reviewDeps, cfg, configPath, root, refresh, providerOverride, doc)
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
