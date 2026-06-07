// Package main is the entry point for the archfit binary.
// main is a thin wrapper: it calls Run and delegates os.Exit.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alecthomas/kong"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/engine"
	"github.com/alexei-led/archfit/internal/extract/golang"
	"github.com/alexei-led/archfit/internal/extract/py"
	"github.com/alexei-led/archfit/internal/extract/ts"
	"github.com/alexei-led/archfit/internal/metrics"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/output/console"
	"github.com/alexei-led/archfit/internal/output/jsonout"
	"github.com/alexei-led/archfit/internal/rules"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// Build-time variables injected by -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// defaultBaselinePath is the on-disk path for the baseline file.
const defaultBaselinePath = ".archfit-baseline.json"

// cli is the top-level kong command struct.
type cli struct {
	Check    CheckCmd    `cmd:"" help:"Check architecture constraints."`
	Baseline BaselineCmd `cmd:"" help:"Save current findings as baseline."`
	Explain  ExplainCmd  `cmd:"" help:"Explain a specific finding."`
	Doctor   DoctorCmd   `cmd:"" help:"Check toolchain availability."`
	Init     InitCmd     `cmd:"" help:"Initialize archfit.yaml."`
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
// CheckCmd
// ---------------------------------------------------------------------------

// CheckCmd runs the full archfit analysis pipeline.
type CheckCmd struct {
	Config   string   `short:"c" help:"Config file." default:"archfit.yaml"`
	Base     string   `help:"Base ref for delta mode."`
	Full     bool     `help:"Full scan (not delta)."`
	Format   []string `help:"Output format." enum:"json,console" default:"console"`
	Advisory bool     `help:"Include advisory findings (no-op Phase 1)."`
	Report   bool     `help:"Report only, don't fail on regressions (no-op Phase 1)."`
}

func (c *CheckCmd) Run(deps *appDeps) error {
	ctx := context.Background()

	cfg, err := config.Load(ctx, c.Config)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	base, err := baseline.Load(ctx, defaultBaselinePath)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	sc := cfg.ForScope()
	sc.WorkDir = filepath.Dir(c.Config)
	sc.Base = c.Base
	sc.Full = c.Full
	s, err := scope.Resolve(ctx, sc, deps.Runner)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	extractors := []engine.Extractor{
		golang.New(cfg.ForExtract("go")),
		ts.New(deps.Runner, cfg.ForExtract("typescript")),
		py.New(deps.Runner, cfg.ForExtract("python")),
	}

	rs := rules.New(cfg.ForRules())
	ms := metrics.New(cfg)

	mode := engine.Mode{
		Base:       c.Base,
		Full:       c.Full,
		Advisory:   c.Advisory,
		ReportOnly: c.Report,
		Formats:    c.Format,
	}

	// Pass nil renderers — we render to deps.Stdout below for testability.
	diag, err := engine.Run(ctx, mode, s, cfg.ForClassify(), cfg.ForStatus(), extractors, rs, ms, nil, base, time.Now())
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	// Render to deps.Stdout.
	for _, fmt := range c.Format {
		var renderErr error
		switch fmt {
		case "json":
			renderErr = jsonout.New().Render(diag, deps.Stdout)
		case "console":
			renderErr = console.New().Render(diag, deps.Stdout)
		}
		if renderErr != nil {
			return &exitError{code: 3, msg: fmt}
		}
	}

	return verdictToError(diag.Verdict)
}

// ---------------------------------------------------------------------------
// BaselineCmd
// ---------------------------------------------------------------------------

// BaselineCmd runs the engine and saves findings as the new baseline.
type BaselineCmd struct {
	Config string `short:"c" default:"archfit.yaml"`
	Full   bool
	Base   string
}

func (c *BaselineCmd) Run(deps *appDeps) error {
	ctx := context.Background()

	cfg, err := config.Load(ctx, c.Config)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	existingBase, err := baseline.Load(ctx, defaultBaselinePath)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	sc := cfg.ForScope()
	sc.WorkDir = filepath.Dir(c.Config)
	sc.Base = c.Base
	sc.Full = c.Full
	s, err := scope.Resolve(ctx, sc, deps.Runner)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	extractors := []engine.Extractor{
		golang.New(cfg.ForExtract("go")),
		ts.New(deps.Runner, cfg.ForExtract("typescript")),
		py.New(deps.Runner, cfg.ForExtract("python")),
	}

	rs := rules.New(cfg.ForRules())
	ms := metrics.New(cfg)

	diag, err := engine.Run(ctx, engine.Mode{Full: c.Full, Base: c.Base}, s, cfg.ForClassify(), cfg.ForStatus(), extractors, rs, ms, nil, existingBase, time.Now())
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	// Build baseline from current findings.
	newBase := baseline.Baseline{}
	for _, f := range diag.Findings {
		newBase.Accepted = append(newBase.Accepted, baseline.AcceptedFinding{
			Fingerprint: f.ID,
			RuleID:      f.RuleID,
		})
	}
	newBase.Metrics = make(diagnostic.MetricSnapshot)
	for _, m := range diag.Metrics {
		newBase.Metrics[m.Name] = struct {
			Value   float64 `json:"value"`
			Version string  `json:"version"`
		}{Value: m.Value, Version: m.Version}
	}

	if err := baseline.Save(ctx, defaultBaselinePath, newBase); err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	_, _ = fmt.Fprintf(deps.Stdout, "baseline saved: %s\n", defaultBaselinePath)
	return nil
}

// ---------------------------------------------------------------------------
// ExplainCmd
// ---------------------------------------------------------------------------

// ExplainCmd re-runs the engine and prints the details of a single finding.
type ExplainCmd struct {
	Config      string `short:"c" default:"archfit.yaml"`
	Fingerprint string `arg:"" help:"Finding fingerprint prefix."`
}

func (c *ExplainCmd) Run(deps *appDeps) error {
	ctx := context.Background()

	cfg, err := config.Load(ctx, c.Config)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	existingBase, err := baseline.Load(ctx, defaultBaselinePath)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	sc := cfg.ForScope()
	sc.WorkDir = filepath.Dir(c.Config)
	sc.Full = true
	s, err := scope.Resolve(ctx, sc, deps.Runner)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	extractors := []engine.Extractor{
		golang.New(cfg.ForExtract("go")),
		ts.New(deps.Runner, cfg.ForExtract("typescript")),
		py.New(deps.Runner, cfg.ForExtract("python")),
	}

	rs := rules.New(cfg.ForRules())
	ms := metrics.New(cfg)

	diag, err := engine.Run(ctx, engine.Mode{Full: true}, s, cfg.ForClassify(), cfg.ForStatus(), extractors, rs, ms, nil, existingBase, time.Now())
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	for _, f := range diag.Findings {
		if strings.HasPrefix(f.ID, c.Fingerprint) {
			_, _ = fmt.Fprintf(deps.Stdout, "id:         %s\n", f.ID)
			_, _ = fmt.Fprintf(deps.Stdout, "rule:       %s\n", f.RuleID)
			_, _ = fmt.Fprintf(deps.Stdout, "status:     %s\n", f.Status)
			_, _ = fmt.Fprintf(deps.Stdout, "severity:   %s\n", f.Severity)
			_, _ = fmt.Fprintf(deps.Stdout, "edge:       %s -> %s (%s)\n", f.Edge.From.Path, f.Edge.To.Path, f.Edge.Kind)
			_, _ = fmt.Fprintf(deps.Stdout, "why:        %s\n", f.Why)
			_, _ = fmt.Fprintf(deps.Stdout, "constraint: %s\n", f.Constraint)
			return nil
		}
	}

	return &exitError{code: 3, msg: fmt.Sprintf("error: no finding with fingerprint prefix %q", c.Fingerprint)}
}

// ---------------------------------------------------------------------------
// DoctorCmd
// ---------------------------------------------------------------------------

// DoctorCmd checks toolchain availability and prints a table.
type DoctorCmd struct{}

func (c *DoctorCmd) Run(deps *appDeps) error { //nolint:unparam // satisfies kong command interface; future versions may return errors
	ctx := context.Background()

	tools := []struct {
		name string
		cmd  string
	}{
		{"go", "go"},
		{"git", "git"},
		{"node", "node"},
		{"bunx", "bunx"},
		{"npx", "npx"},
		{"uv", "uv"},
		{"python3", "python3"},
	}

	_, _ = fmt.Fprintf(deps.Stdout, "%-12s %-8s %s\n", "TOOL", "STATUS", "PATH")
	_, _ = fmt.Fprintf(deps.Stdout, "%s\n", strings.Repeat("-", 50))

	for _, t := range tools {
		if info, ok := deps.Runner.Detect(ctx, t.cmd); ok {
			_, _ = fmt.Fprintf(deps.Stdout, "%-12s %-8s %s\n", t.name, "ok", info.Path)
		} else {
			_, _ = fmt.Fprintf(deps.Stdout, "%-12s %-8s %s\n", t.name, "missing", "not found")
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// InitCmd
// ---------------------------------------------------------------------------

// InitCmd is a stub for initializing archfit.yaml.
type InitCmd struct{}

func (c *InitCmd) Run(deps *appDeps) error { //nolint:unparam // satisfies kong command interface; future versions may return errors
	_, _ = fmt.Fprintln(deps.Stdout, "not yet implemented")
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// verdictToError maps a diagnostic verdict to an exit error (nil = exit 0).
func verdictToError(v diagnostic.Verdict) error {
	switch v {
	case diagnostic.VerdictFail:
		return &exitError{code: 1}
	case diagnostic.VerdictWarn:
		return &exitError{code: 2}
	default:
		return nil
	}
}

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
