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
	defaultConfigPath   = ".archfit.yaml"          // fallback to config.Default() when absent
	defaultBaselinePath = ".archfit-baseline.json" // on-disk path for the baseline file
	defaultLabelsPath   = ".archfit-labels.yaml"   // pinned coupling labels (enrich output)
)

// cli is the top-level kong command struct.
type cli struct {
	Check    CheckCmd    `cmd:"" help:"Check architecture constraints."`
	Enrich   EnrichCmd   `cmd:"" help:"Draft LLM coupling-label refinements for human review (off-gate)."`
	Scan     ScanCmd     `cmd:"" help:"Full architecture audit report (scan ≡ check --full --advisory --report --format markdown)."`
	Baseline BaselineCmd `cmd:"" help:"Save current findings as baseline."`
	Explain  ExplainCmd  `cmd:"" help:"Explain a specific finding."`
	Doctor   DoctorCmd   `cmd:"" help:"Check toolchain availability."`
	Install  InstallCmd  `cmd:"" help:"Install external tools required for language analysis."`
	Init     InitCmd     `cmd:"" help:"Initialize .archfit.yaml."`
	Version  versionFlag `short:"v" help:"Print version and exit."`
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
		kong.Description("Architecture fitness checker for Go, TypeScript, and Python."),
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
		// kong has already written the error + usage.
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
	os.Exit(Run(os.Args[1:], os.Stdout))
}
