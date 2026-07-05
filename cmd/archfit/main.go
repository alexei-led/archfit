// Package main is the entry point for the archfit binary.
// main is a thin wrapper: it calls Run and delegates os.Exit.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"

	"github.com/alexei-led/archfit/internal/toolrun"
)

// Build-time variables injected by -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const (
	flagHelp              = "--help"
	defaultConfigPath     = ".archfit.yaml"            // fallback to config.Default() when absent
	defaultBaselinePath   = ".archfit-baseline.json"   // on-disk path for the baseline file
	defaultLabelsPath     = ".archfit-labels.yaml"     // pinned coupling labels (enrich output)
	defaultSubdomainsPath = ".archfit-subdomains.yaml" // subdomain draft/apply file (config enrich subdomain)
	defaultOwnersPath     = ".archfit-owners.yaml"     // owner draft/apply file (config enrich owner)
	defaultVolatilityPath = ".archfit-volatility.yaml" // volatility draft/apply file (config enrich volatility)
)

const (
	docsURL          = "https://github.com/alexei-led/archfit/tree/main/docs/guide"
	ciDocsURL        = "https://github.com/alexei-led/archfit/blob/main/docs/guide/ci.md"
	agentDocsURL     = "https://github.com/alexei-led/archfit/blob/main/docs/guide/agent-feedback.md"
	languagesDocsURL = "https://github.com/alexei-led/archfit/blob/main/docs/guide/languages.md"
)

const (
	commandGroupAnalysis = "analysis"
	commandGroupSetup    = "setup"
)

// cli is the top-level kong command struct. Config-consuming commands
// (analyze/baseline/explain) stay flat at the top level; config-authoring
// commands live under `config`; `doctor` both checks and (with --fix) installs
// analyzer tools.
type cli struct {
	Analyze  AnalyzeCmd  `cmd:"" default:"withargs" group:"analysis" help:"Analyze architecture: decision, score, findings (default command)."`
	Baseline BaselineCmd `cmd:"" group:"analysis" help:"Accept current findings as the gate baseline."`
	Explain  ExplainCmd  `cmd:"" group:"analysis" help:"Explain one finding by fingerprint prefix."`

	Doctor DoctorCmd `cmd:"" group:"setup" help:"Check analyzer/tool availability (use --fix to install missing tools)."`
	Config ConfigCmd `cmd:"" group:"setup" help:"Create, sync, and enrich the .archfit.yaml config."`

	Version versionFlag `short:"v" help:"Print version and exit."`
}

// ConfigCmd groups the config-authoring subcommands. init scaffolds a config (or,
// with --llm, drafts a full LLM-classified config); update syncs the config with
// the project structure; enrich drafts per-dimension LLM annotations for review.
type ConfigCmd struct {
	Init   InitCmd   `cmd:"" help:"Create a starter config (use --llm for an LLM-classified draft)."`
	Update UpdateCmd `cmd:"" help:"Sync the config with current project structure."`
	Enrich EnrichCmd `cmd:"" help:"Draft LLM annotations (labels/owner/volatility/subdomain) for review."`
}

func (cli) Help() string {
	return `archfit keeps code changes aligned with the architecture you intended. It turns dependency facts into deterministic gates, scorecards, SARIF, and agent repair tasks so CI can catch architecture drift before review.

First run:
  archfit doctor                                   # check analyzers (doctor --fix installs them)
  archfit config init --root .                      # scaffold .archfit.yaml (config init --llm to draft via LLM)
  archfit --gate --config .archfit.yaml --full      # the gate
  archfit baseline --config .archfit.yaml --full    # accept current findings as the baseline

CI / agent loop:
  archfit --gate --config .archfit.yaml --base origin/main --format json
  # on exit 1, read agent_tasks[] and rerun the validation command
  archfit --markdown --config .archfit.yaml > archfit-report.md

Off-gate LLM (review-only; never affects the gate):
  archfit analyze --llm -c .archfit.yaml            # cited architect review after deterministic output
  archfit config enrich owner                       # draft → review → config enrich owner --apply
  archfit config init --llm -o draft.yaml           # full LLM-classified config draft for review

Docs:
  ` + docsURL + `
  CI: ` + ciDocsURL + `
  Agent feedback: ` + agentDocsURL + `
  Analyzer setup: ` + languagesDocsURL + `

Optional LLM commands are review-only. They never decide whether the gate passes.`
}

func commandGroups() []kong.Group {
	return []kong.Group{
		{
			Key:         commandGroupAnalysis,
			Title:       "Analysis",
			Description: "Run these locally, in CI, and in AI-agent validation steps.",
		},
		{
			Key:         commandGroupSetup,
			Title:       "Setup & config",
			Description: "Create configs (config init), sync structure (config update), install analyzers (doctor --fix), and draft off-gate LLM annotations (config enrich).",
		},
	}
}

// versionFlag prints the version and exits cleanly.
type versionFlag bool

func (v versionFlag) BeforeReset(ctx *kong.Context) error {
	_, err := fmt.Fprintf(ctx.Stdout, "archfit version %s (commit %s, built %s)\n", version, commit, date)
	if err != nil {
		return err
	}
	ctx.Exit(0)
	return nil
}

// appDeps is the composition root passed via kong.Bind.
type appDeps struct {
	Runner toolrun.Runner
	Stdout io.Writer
	Stderr io.Writer // nil → os.Stderr

	// progress, when non-nil, is invoked at pipeline phase boundaries so a long
	// analyze run reports progress to stderr. Set by AnalyzeCmd; nil (no-op) for
	// every other command.
	progress func(stage string)

	// warnLabel prefixes pipeline stderr warnings. Empty for normal runs; the
	// --base worktree sub-scan sets "[base] " so its owner/TS-coverage warnings
	// aren't misread as head-side regressions (both sides share one stderr).
	warnLabel string

	// noCache disables the extractor fact cache (fact-cache.md D2: one
	// --no-cache flag bypasses ALL archfit caches, reads AND writes). Set by
	// each pipeline command from its --no-cache flag before runPipeline.
	noCache bool
}

// warn writes a labeled pipeline warning to stderr.
func (d *appDeps) warn(w string) {
	_, _ = fmt.Fprintln(d.stderr(), "warning: "+d.warnLabel+w)
}

// stderr returns the configured stderr writer, falling back to os.Stderr.
func (d *appDeps) stderr() io.Writer {
	if d.Stderr != nil {
		return d.Stderr
	}
	return os.Stderr
}

// reportPhase advances the attached progress reporter, if any (no-op otherwise).
// The pipeline calls this at phase boundaries; only AnalyzeCmd attaches a reporter.
func (d *appDeps) reportPhase(stage string) {
	if d.progress != nil {
		d.progress(stage)
	}
}

// exitError carries an exit code through the Run return path.
type exitError struct {
	code int
	msg  string
}

func (e *exitError) Error() string {
	if e.msg != "" {
		return e.msg
	}
	return fmt.Sprintf("exit %d", e.code)
}

func (e *exitError) ExitCode() int { return e.code }

// exitCode is used to capture controlled exits (--version, --help) via panic+recover.
type exitCode int

// ---------------------------------------------------------------------------
// Run — testable entry point
// ---------------------------------------------------------------------------

// Run parses args, runs the selected command, and returns the process exit code.
// Errors and parser diagnostics go to os.Stderr; normal output to stdout. Tests
// that need to capture stderr use RunWithStderr.
func Run(args []string, stdout io.Writer) int {
	return RunWithStderr(args, stdout, os.Stderr)
}

// RunWithStderr is Run with an explicit stderr writer. Diagnostics — config/tool
// errors, parser errors, and exitError messages — are written to stderr (not
// stdout) so `archfit --json 2>/dev/null` yields clean JSON and CI logs separate
// data from errors.
func RunWithStderr(args []string, stdout, stderr io.Writer) (exitStatus int) {
	// Capture controlled exits (--version, --help) via panic+recover.
	defer func() {
		if r := recover(); r != nil {
			if code, ok := r.(exitCode); ok {
				exitStatus = int(code)
				return
			}
			panic(r)
		}
	}()

	deps := &appDeps{Runner: toolrun.New(), Stdout: stdout, Stderr: stderr}

	var c cli
	parser, err := kong.New(&c,
		kong.Name("archfit"),
		kong.Description("Architecture drift feedback for CI and AI agents."),
		kong.ExplicitGroups(commandGroups()),
		kong.ConfigureHelp(kong.HelpOptions{FlagsLast: true, WrapUpperBound: 100}),
		// Help → stdout; usage/errors → stderr (kong's two-writer convention).
		kong.Writers(stdout, stderr),
		kong.Exit(func(code int) { panic(exitCode(code)) }),
		kong.Bind(deps),
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return 3
	}

	kctx, err := parser.Parse(args)
	if err != nil {
		// Manual parser.Parse (unlike the kong.Parse helper) does not print the
		// error itself, so surface it — otherwise a bad flag exits 3 silently.
		_, _ = fmt.Fprintf(stderr, "archfit: %v\n", err)
		return 3
	}

	runErr := kctx.Run(deps)
	if runErr == nil {
		return 0
	}

	var ee *exitError
	if errors.As(runErr, &ee) {
		if ee.msg != "" {
			_, _ = fmt.Fprintln(stderr, ee.msg)
		}
		return ee.ExitCode()
	}

	_, _ = fmt.Fprintf(stderr, "error: %v\n", runErr)
	return 3
}

func main() {
	// Best-effort .env autoload at startup so the LLM commands pick up a key from
	// a local .env without polluting the shell. Real env / CI secrets always win.
	// CWD first, then the analyzed repo (--root) and the config's directory, so a
	// key kept alongside the target repo or config is found too (F3).
	loadDotEnv(".env")
	for _, dir := range envSearchDirs(os.Args[1:]) {
		loadDotEnv(filepath.Join(dir, ".env"))
	}
	os.Exit(Run(os.Args[1:], os.Stdout))
}
