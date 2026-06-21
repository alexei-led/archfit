// Package main is the entry point for the archfit binary.
// main is a thin wrapper: it calls Run and delegates os.Exit.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

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
	defaultSubdomainsPath = ".archfit-subdomains.yaml" // subdomain draft/pin file (enrich --subdomains)
	defaultOwnersPath     = ".archfit-owners.yaml"     // owner draft/pin file (enrich --owner)
	defaultVolatilityPath = ".archfit-volatility.yaml" // volatility draft/pin file (enrich --volatility)
	defaultAutopilotPath  = ".archfit-autopilot.yaml"  // full-config draft (autopilot, review-only)
)

const (
	docsURL          = "https://github.com/alexei-led/archfit/tree/main/docs/guide"
	ciDocsURL        = "https://github.com/alexei-led/archfit/blob/main/docs/guide/ci.md"
	agentDocsURL     = "https://github.com/alexei-led/archfit/blob/main/docs/guide/agent-feedback.md"
	languagesDocsURL = "https://github.com/alexei-led/archfit/blob/main/docs/guide/languages.md"
)

const (
	commandGroupCore        = "core"
	commandGroupReports     = "reports"
	commandGroupSetup       = "setup"
	commandGroupLLM         = "llm"
	commandGroupMaintenance = "maintenance"
)

// cli is the top-level kong command struct.
type cli struct {
	Doctor   DoctorCmd   `cmd:"" group:"core" help:"Check analyzer/tool availability."`
	Init     InitCmd     `cmd:"" group:"core" help:"Create a starter architecture config."`
	Check    CheckCmd    `cmd:"" group:"core" help:"Run the architecture drift gate."`
	Baseline BaselineCmd `cmd:"" group:"core" help:"Accept current findings as the baseline."`

	Score   ScoreCmd   `cmd:"" group:"reports" help:"Print the banded architecture scorecard."`
	Scan    ScanCmd    `cmd:"" group:"reports" help:"Write a full Markdown architecture audit report."`
	Explain ExplainCmd `cmd:"" group:"reports" help:"Explain one finding by fingerprint prefix."`

	Install InstallCmd `cmd:"" group:"setup" help:"Install optional analyzer tools."`
	Update  UpdateCmd  `cmd:"" group:"setup" help:"Sync .archfit.yaml with current project structure."`

	Review    ReviewCmd    `cmd:"" group:"llm" help:"Off-gate LLM narrative review of collected evidence."`
	Enrich    EnrichCmd    `cmd:"" group:"llm" help:"Draft human-reviewed LLM coupling labels and metadata."`
	Autopilot AutopilotCmd `cmd:"" group:"llm" help:"Draft a full review-only config via LLM."`

	Calibrate CalibrateCmd `cmd:"" group:"maintenance" help:"Compare scorers over real repos (dev tool; off-gate)."`
	Version   versionFlag  `short:"v" help:"Print version and exit."`
}

func (cli) Help() string {
	return `archfit keeps code changes aligned with the architecture you intended. It turns dependency facts into deterministic gates, scorecards, SARIF, and agent repair tasks so CI can catch architecture drift before review.

First run:
  archfit doctor
  archfit init --root .
  archfit check --config .archfit.yaml --full
  archfit baseline --config .archfit.yaml --full

CI / agent loop:
  archfit check --config .archfit.yaml --base origin/main --format json
  # on exit 1, read agent_tasks[] and rerun the validation command
  archfit scan --config .archfit.yaml > archfit-report.md

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
			Key:         commandGroupCore,
			Title:       "Core feedback loop",
			Description: "Run these locally, in CI, and in AI-agent validation steps.",
		},
		{
			Key:         commandGroupReports,
			Title:       "Reports and explanations",
			Description: "Use these to inspect evidence without changing the gate verdict.",
		},
		{
			Key:         commandGroupSetup,
			Title:       "Setup and config maintenance",
			Description: "Create configs and keep analyzer coverage honest as the repo changes.",
		},
		{
			Key:         commandGroupLLM,
			Title:       "Off-gate LLM helpers",
			Description: "Draft reviews and labels for humans. The deterministic gate ignores LLM judgment.",
		},
		{
			Key:         commandGroupMaintenance,
			Title:       "Maintainer tools",
			Description: "Project development helpers; not part of normal CI policy.",
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
func Run(args []string, stdout io.Writer) (exitStatus int) {
	if len(args) == 0 {
		args = []string{flagHelp}
	}

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

	deps := &appDeps{Runner: toolrun.New(), Stdout: stdout}

	var c cli
	parser, err := kong.New(&c,
		kong.Name("archfit"),
		kong.Description("Architecture drift feedback for CI and AI agents."),
		kong.ExplicitGroups(commandGroups()),
		kong.ConfigureHelp(kong.HelpOptions{FlagsLast: true, WrapUpperBound: 100}),
		kong.Writers(stdout, stdout),
		kong.Exit(func(code int) { panic(exitCode(code)) }),
		kong.Bind(deps),
	)
	if err != nil {
		_, _ = fmt.Fprintf(stdout, "error: %v\n", err)
		return 3
	}

	kctx, err := parser.Parse(args)
	if err != nil {
		// Manual parser.Parse (unlike the kong.Parse helper) does not print the
		// error itself, so surface it — otherwise a bad flag exits 3 silently.
		_, _ = fmt.Fprintf(stdout, "archfit: %v\n", err)
		return 3
	}

	runErr := kctx.Run(deps)
	if runErr == nil {
		return 0
	}

	var ee *exitError
	if errors.As(runErr, &ee) {
		if ee.msg != "" {
			_, _ = fmt.Fprintln(stdout, ee.msg)
		}
		return ee.ExitCode()
	}

	_, _ = fmt.Fprintf(stdout, "error: %v\n", runErr)
	return 3
}

func main() {
	// Best-effort .env autoload at startup so the LLM commands pick up a key from
	// a local .env without polluting the shell. Real env / CI secrets always win.
	loadDotEnv(".env")
	os.Exit(Run(os.Args[1:], os.Stdout))
}
