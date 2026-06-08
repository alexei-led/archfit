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
	"github.com/alexei-led/archfit/internal/extract/astgrep"
	"github.com/alexei-led/archfit/internal/extract/golang"
	"github.com/alexei-led/archfit/internal/extract/py"
	"github.com/alexei-led/archfit/internal/extract/scip"
	"github.com/alexei-led/archfit/internal/extract/ts"
	"github.com/alexei-led/archfit/internal/initcfg"
	"github.com/alexei-led/archfit/internal/metrics"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/output/console"
	"github.com/alexei-led/archfit/internal/output/jsonout"
	"github.com/alexei-led/archfit/internal/output/markdown"
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

const (
	defaultConfigPath   = "archfit.yaml"           // fallback to config.Default() when absent
	defaultBaselinePath = ".archfit-baseline.json" // on-disk path for the baseline file
)

// cli is the top-level kong command struct.
type cli struct {
	Check    CheckCmd    `cmd:"" help:"Check architecture constraints."`
	Scan     ScanCmd     `cmd:"" help:"Full architecture audit report (scan ≡ check --full --advisory --report --format markdown)."`
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
	Config   string   `short:"c" help:"Path to config file (optional; built-in defaults used if absent)." default:"archfit.yaml"`
	Base     string   `help:"Git ref to compare against for incremental mode (e.g. main, HEAD~1)."`
	Full     bool     `help:"Scan all files, not just files changed since --base."`
	Format   []string `help:"Output format: text (human-readable), json, markdown, md. Repeatable." enum:"json,text,markdown,md" default:"text"`
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

	configDir := filepath.Dir(c.Config)
	base, err := baseline.Load(ctx, filepath.Join(configDir, defaultBaselinePath))
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	sc := cfg.ForScope()
	sc.WorkDir = configDir
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

	patternCfg := cfg.ForPatterns()
	diag, err := engine.Run(ctx, mode, s, cfg.ForClassify(), cfg.ForStaleness(), cfg.ForStatus(), extractors, astgrep.New(deps.Runner), scip.New(deps.Runner), patternCfg, rs, ms, base, time.Now())
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	// Render to deps.Stdout.
	for _, format := range c.Format {
		var renderErr error
		switch format {
		case "json":
			renderErr = jsonout.New().Render(diag, deps.Stdout)
		case "text":
			renderErr = console.New().Render(diag, deps.Stdout)
		case "md":
			renderErr = markdown.New().Render(diag, deps.Stdout)
		case "markdown":
			renderErr = markdown.New().Render(diag, deps.Stdout)
		}
		if renderErr != nil {
			return &exitError{code: 3, msg: fmt.Sprintf("render %s: %v", format, renderErr)}
		}
	}

	return verdictToError(diag.Verdict)
}

// ---------------------------------------------------------------------------
// BaselineCmd
// ---------------------------------------------------------------------------

// BaselineCmd runs the engine and saves findings as the new baseline.
type BaselineCmd struct {
	Config   string `short:"c" default:"archfit.yaml"`
	Full     bool
	Advisory bool `help:"Include advisory findings in the baseline."`
	Base     string
}

func (c *BaselineCmd) Run(deps *appDeps) error {
	ctx := context.Background()

	cfg, err := loadConfig(ctx, c.Config, false)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	configDir := filepath.Dir(c.Config)
	existingBase, err := baseline.Load(ctx, filepath.Join(configDir, defaultBaselinePath))
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	sc := cfg.ForScope()
	sc.WorkDir = configDir
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

	diag, err := engine.Run(ctx, engine.Mode{Full: c.Full, Advisory: c.Advisory, Base: c.Base}, s, cfg.ForClassify(), cfg.ForStaleness(), cfg.ForStatus(), extractors, engine.NopPatternProvider{}, engine.NopSymbolResolver{}, config.PatternConfig{}, rs, ms, existingBase, time.Now())
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	// Build baseline from current findings.
	newBase := baseline.Baseline{}
	for _, f := range diag.Findings {
		if f.Status == finding.StatusFixed {
			continue // fixed = no longer detected; don't carry into new baseline
		}
		newBase.Accepted = append(newBase.Accepted, baseline.AcceptedFinding{
			Fingerprint: f.ID,
			RuleID:      f.RuleID,
			Kind:        f.Kind,
		})
	}
	newBase.Metrics = make(diagnostic.MetricSnapshot)
	for _, m := range diag.Metrics {
		newBase.Metrics[m.Name] = struct {
			Value   float64 `json:"value"`
			Version string  `json:"version"`
		}{Value: m.Value, Version: m.Version}
	}

	bPath := filepath.Join(configDir, defaultBaselinePath)
	if err := baseline.Save(ctx, bPath, newBase); err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	_, _ = fmt.Fprintf(deps.Stdout, "baseline saved: %s\n", bPath)
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

	cfg, err := loadConfig(ctx, c.Config, false)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	configDir := filepath.Dir(c.Config)
	existingBase, err := baseline.Load(ctx, filepath.Join(configDir, defaultBaselinePath))
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	sc := cfg.ForScope()
	sc.WorkDir = configDir
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

	diag, err := engine.Run(ctx, engine.Mode{Full: true}, s, cfg.ForClassify(), cfg.ForStaleness(), cfg.ForStatus(), extractors, engine.NopPatternProvider{}, engine.NopSymbolResolver{}, config.PatternConfig{}, rs, ms, existingBase, time.Now())
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
		{"sg (ast-grep)", "sg"},
		{"scip-typescript", "scip-typescript"},
		{"scip-python", "scip-python"},
		{"scip-go", "scip-go"},
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

// InitCmd discovers project structure and writes a starter archfit.yaml.
type InitCmd struct {
	Root   string `short:"r" help:"Project root directory." default:"."`
	Output string `short:"o" help:"Output file (use '-' for stdout)." default:"archfit.yaml"`
}

func (c *InitCmd) Run(deps *appDeps) error {
	root := c.Root
	if !filepath.IsAbs(root) {
		var err error
		root, err = filepath.Abs(root)
		if err != nil {
			return fmt.Errorf("resolving root: %w", err)
		}
	}
	ctx := context.Background()
	cfg, err := initcfg.Discover(ctx, root, deps.Runner)
	if err != nil {
		return fmt.Errorf("discovering project structure: %w", err)
	}
	yaml := initcfg.Render(cfg)
	if c.Output == "-" {
		_, _ = fmt.Fprint(deps.Stdout, yaml)
		return nil
	}
	if err := os.WriteFile(c.Output, []byte(yaml), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", c.Output, err)
	}
	_, _ = fmt.Fprintf(deps.Stdout, "wrote %s\n", c.Output)
	return nil
}

// ---------------------------------------------------------------------------
// ScanCmd
// ---------------------------------------------------------------------------

// ScanCmd is a convenience alias for a full advisory Markdown audit.
// Always runs: check --full --advisory --report --format markdown.
// For custom combinations (e.g. without advisory), use the check command directly.
type ScanCmd struct {
	Config string `short:"c" help:"Config file." default:"archfit.yaml"`
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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// loadConfig loads the config file at path. When path equals the default
// "archfit.yaml" and the file is absent, it returns config.Default() so the
// tool works without a config file. An explicit --config path that is missing
// always returns an error. noConfig=true skips file loading entirely.
func loadConfig(ctx context.Context, path string, noConfig bool) (config.Config, error) {
	if noConfig {
		return config.Default(), nil
	}
	cfg, err := config.Load(ctx, path)
	if err == nil {
		return cfg, nil
	}
	if path == defaultConfigPath && errors.Is(err, os.ErrNotExist) {
		return config.Default(), nil
	}
	return config.Config{}, err
}

// canonicalLang maps a --lang key (go, ts, py, or full name) to the config
// language name. Returns "" for unknown keys.
func canonicalLang(key string) string {
	switch key {
	case "go":
		return config.LangGo
	case "ts", config.LangTypeScript:
		return config.LangTypeScript
	case "py", config.LangPython:
		return config.LangPython
	default:
		return ""
	}
}

// applyFlagOverrides applies non-empty CLI flag values onto cfg, overriding
// whatever the config file (or Default) provided.
func applyFlagOverrides(cfg *config.Config, severity string, lang []string) error {
	if severity != "" {
		cfg.BCAdvisoryMinSeverity = severity
	}
	for _, key := range lang {
		canonical := canonicalLang(key)
		if canonical == "" {
			return fmt.Errorf("--lang: unknown language %q; use go, ts, or py", key)
		}
		if cfg.Tools == nil {
			cfg.Tools = make(map[string]config.ToolConfig)
		}
		cfg.Tools[canonical] = config.ToolConfig{Enabled: config.ModeOn}
	}
	return nil
}

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
