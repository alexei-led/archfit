// Package main is the entry point for the archfit binary.
// main is a thin wrapper: it calls Run and delegates os.Exit.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/alecthomas/kong"
)

// Build-time variables injected by -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// cli is the top-level kong command struct. Commands will be added as tagged
// fields in later tasks.
type cli struct {
	Version versionFlag `short:"v" help:"Print version and exit."`
}

// versionFlag is a custom flag type so we can implement BeforeReset to print
// the version and exit cleanly without triggering kong's usage error path.
type versionFlag bool

func (v versionFlag) BeforeReset(ctx *kong.Context) error {
	_, err := fmt.Fprintf(ctx.Stdout, "archfit version %s (commit %s, built %s)\n", version, commit, date)
	if err != nil {
		return err
	}
	ctx.Exit(0)
	return nil
}

// exitCode is used to capture the exit code when running in test mode.
// A panic with this type signals a controlled exit.
type exitCode int

// Run parses args, runs the selected command, and returns the process exit code.
// Separating this from main makes the logic testable without os.Exit.
func Run(args []string, stdout io.Writer) (exitStatus int) {
	// Capture controlled exits (from --version / --help) via panic+recover.
	defer func() {
		if r := recover(); r != nil {
			if code, ok := r.(exitCode); ok {
				exitStatus = int(code)
				return
			}
			panic(r) // re-panic for unexpected panics
		}
	}()

	var c cli
	parser, err := kong.New(&c,
		kong.Name("archfit"),
		kong.Description("Architecture fitness checker for Go, TypeScript, and Python repositories."),
		kong.Writers(stdout, stdout),
		kong.Exit(func(code int) { panic(exitCode(code)) }),
	)
	if err != nil {
		_, _ = fmt.Fprintf(stdout, "error: %v\n", err)
		return 3
	}

	_, err = parser.Parse(args)
	if err != nil {
		// kong has already written the error + usage; treat as config error.
		return 3
	}

	return 0
}

func main() {
	os.Exit(Run(os.Args[1:], os.Stdout))
}
