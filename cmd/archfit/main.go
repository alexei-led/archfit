// Package main is the entry point for the archfit binary.
// main is a thin wrapper: it calls Run and delegates os.Exit.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/alecthomas/kong"

	gitpkg "github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// Build-time variables injected by -ldflags.
var (
	version = devVersion
	commit  = "none"
	date    = "unknown"
)

// init lets a `go install` build report its module version. Release builds
// set version through -ldflags and skip this; `go install` passes no ldflags
// but records the module version in the build info. Commit and date stay unset.
func init() {
	if version != devVersion {
		return
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		version = releaseVersion(info.Main.Version)
	}
}

// releaseVersion maps build info's main module version to a display version.
// A local build reports "(devel)" or nothing, which stays "dev".
func releaseVersion(moduleVersion string) string {
	if moduleVersion == "" || moduleVersion == "(devel)" {
		return devVersion
	}
	return moduleVersion
}

const devVersion = "dev"

const (
	flagHelp              = "--help"
	defaultConfigPath     = ".archfit.yaml"            // fallback to config.Default() when absent
	defaultLabelsPath     = ".archfit-labels.yaml"     // pinned coupling labels (enrich output)
	defaultSubdomainsPath = ".archfit-subdomains.yaml" // subdomain draft/apply file (config enrich subdomain)
	defaultOwnersPath     = ".archfit-owners.yaml"     // owner draft/apply file (config enrich owner)
	defaultVolatilityPath = ".archfit-volatility.yaml" // volatility draft/apply file (config enrich volatility)
)

const (
	docsURL          = "https://github.com/alexei-led/archfit/blob/main/docs/getting-started.md"
	ciDocsURL        = "https://github.com/alexei-led/archfit/blob/main/docs/ci.md"
	agentDocsURL     = "https://github.com/alexei-led/archfit/blob/main/docs/agents.md"
	languagesDocsURL = "https://github.com/alexei-led/archfit/blob/main/docs/languages.md"
)

// cli exposes the seven public commands and the hidden hook entry point.
type cli struct {
	Check   CheckCmd    `cmd:"" help:"The status of the change: pass (0), fail (1), or unknown (2)."`
	Can     CanCmd      `cmd:"" help:"Before writing code: may this file, directory or module depend on that one, or use a library."`
	Rules   RulesCmd    `cmd:"" help:"The rules that apply to paths or to the change, one rule in full, or the module graph."`
	Request RequestCmd  `cmd:"" help:"Ask an owner to change the policy for the errors of the last check."`
	Init    RepoInitCmd `cmd:"" help:"Set up a repository or convert an Archfit 3 policy."`
	Report  ReportCmd   `cmd:"" help:"The repository-wide picture for owners. Exits 0 whatever it shows."`
	Doctor  DoctorCmd   `cmd:"" help:"This checkout: the tools and hooks. --fix installs what is missing."`
	Hook    HookCmd     `cmd:"" hidden:"" help:"The hook entry points installed by doctor --fix."`
	Version versionFlag `short:"v" help:"Print version and exit."`
}

func (cli) Help() string {
	return `Check a change against the repository's architecture policy.

First run:
  archfit doctor
  archfit init
  # Read .archfit.yaml, then open a pull request with it.

Worker loop:
  archfit rules
  archfit can internal/a/a.go internal/b
  archfit check
  archfit request -m "why the policy needs to change"

For owners:
  archfit report
  archfit rules --graph
  archfit doctor --fix

Exit codes: 0 pass or done; 1 fail or no; 2 unknown, wrong usage, or a tool error.

Docs:
  ` + docsURL + `
  CI: ` + ciDocsURL + `
  Agent feedback: ` + agentDocsURL + `
  Analyzer setup: ` + languagesDocsURL
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
	Stdin  io.Reader // nil → os.Stdin
	Stdout io.Writer
	Stderr io.Writer // nil → os.Stderr

	// requestClient overrides the GitHub transport (nil uses the standard client).
	requestClient *http.Client

	// progress, when non-nil, is invoked at pipeline phase boundaries so a long
	// analyze/check run reports progress to stderr. Set by AnalyzeCmd/CheckCmd;
	// nil (no-op) for every other command.
	progress func(stage string)

	// warnLabel prefixes pipeline stderr warnings. Empty for normal runs; the
	// --base worktree sub-scan sets "[base] " so its owner/TS-coverage warnings
	// aren't misread as head-side regressions (both sides share one stderr).
	warnLabel string

	// refresh bypasses cache reads but still records fresh extractor facts.
	// Set by each pipeline command from its --refresh flag before the analysis pipeline.
	refresh bool

	// now is the clock of the version 3 commands: exceptions are judged on
	// its date. nil → time.Now (tests set a fixed date).
	now func() time.Time

	// ctx is cancelled on SIGINT or SIGTERM, so tool runs stop and extractor
	// cleanup (the .NET snapshot) runs before the process exits. nil →
	// context.Background() (tests that build appDeps directly).
	ctx context.Context
}

func (d *appDeps) runContext() context.Context {
	if d.ctx == nil {
		return context.Background()
	}
	return d.ctx
}

// clock returns the clock of the version 3 commands.
func (d *appDeps) clock() func() time.Time {
	if d.now != nil {
		return d.now
	}
	return time.Now
}

// stdin returns the configured stdin reader, falling back to os.Stdin.
func (d *appDeps) stdin() io.Reader {
	if d.Stdin != nil {
		return d.Stdin
	}
	return os.Stdin
}

// stderr returns the configured stderr writer, falling back to os.Stderr.
func (d *appDeps) stderr() io.Writer {
	if d.Stderr != nil {
		return d.Stderr
	}
	return os.Stderr
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
func RunWithStderr(args []string, stdout, stderr io.Writer) int {
	return runWithIO(args, nil, stdout, stderr)
}

// runWithIO is RunWithStderr with an explicit stdin reader; nil reads os.Stdin.
// The hook commands read their event from stdin.
func runWithIO(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	// The first SIGINT or SIGTERM cancels ctx: running tools are killed and
	// deferred cleanup runs. stop then restores the default handling, so a
	// second signal exits at once.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		stop()
	}()

	return runDeps(args, &appDeps{Runner: toolrun.New(), Stdin: stdin, Stdout: stdout, Stderr: stderr, ctx: ctx})
}

// runDeps parses args and runs the selected command with deps, which carry
// the writers and the context. Tests call it to set deps the binary does
// not let a user set, such as the clock.
func runDeps(args []string, deps *appDeps) (exitStatus int) {
	// Capture controlled exits (--version, --help) via panic+recover.
	defer func() {
		if r := recover(); r != nil {
			if code, ok := r.(exitCode); ok {
				exitStatus = int(code)
			}
			if _, ok := r.(exitCode); !ok {
				panic(r)
			}
		}
		if exitStatus < 0 || exitStatus > exitUnknown {
			exitStatus = exitUnknown
		}
	}()
	stdout, stderr := deps.Stdout, deps.stderr()
	ctx := deps.runContext()
	objects, err := gitpkg.NewObjectStore(deps.Runner)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "archfit: %v\n", err)
		return 3
	}
	defer func() { _ = objects.Close() }()
	scoped := *deps
	scoped.Runner = objects
	deps = &scoped

	var c cli
	parser, err := kong.New(&c,
		kong.Name("archfit"),
		kong.Description("Architecture drift feedback for CI and AI agents."),
		kong.ConfigureHelp(kong.HelpOptions{FlagsLast: true, WrapUpperBound: 100}),
		// Help → stdout; usage/errors → stderr (kong's two-writer convention).
		kong.Writers(stdout, stderr),
		kong.Exit(func(code int) { panic(exitCode(code)) }),
		kong.Bind(deps),
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return exitUnknown
	}

	kctx, err := parser.Parse(args)
	if err != nil {
		// Manual parser.Parse (unlike the kong.Parse helper) does not print the
		// error itself, so surface it — otherwise a bad flag exits silently.
		_, _ = fmt.Fprintf(stderr, "archfit: %v\n", err)
		return exitUnknown
	}

	runErr := kctx.Run(deps)
	if ctx.Err() != nil {
		// A signal cut tool runs short: whatever the command printed is
		// incomplete and must not pass as a verdict.
		_, _ = fmt.Fprintln(stderr, "archfit: interrupted by a signal; the results are incomplete")
		return exitUnknown
	}
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
	var coded interface{ ExitCode() int }
	if errors.As(runErr, &coded) {
		msg := runErr.Error()
		if msg != fmt.Sprintf("exit %d", coded.ExitCode()) {
			_, _ = fmt.Fprintln(stderr, msg)
		}
		return coded.ExitCode()
	}

	_, _ = fmt.Fprintf(stderr, "error: %v\n", runErr)
	return exitUnknown
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
