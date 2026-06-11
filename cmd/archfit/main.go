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
	"github.com/alexei-led/archfit/internal/extract/clones"
	"github.com/alexei-led/archfit/internal/extract/gitnexus"
	"github.com/alexei-led/archfit/internal/extract/golang"
	"github.com/alexei-led/archfit/internal/extract/py"
	"github.com/alexei-led/archfit/internal/extract/scip"
	"github.com/alexei-led/archfit/internal/extract/ts"
	"github.com/alexei-led/archfit/internal/fitness"
	"github.com/alexei-led/archfit/internal/history/git"
	"github.com/alexei-led/archfit/internal/initcfg"
	"github.com/alexei-led/archfit/internal/metrics"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/output/console"
	"github.com/alexei-led/archfit/internal/output/jsonout"
	"github.com/alexei-led/archfit/internal/output/markdown"
	"github.com/alexei-led/archfit/internal/ownership"
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
	defaultConfigPath   = ".archfit.yaml"          // fallback to config.Default() when absent
	defaultBaselinePath = ".archfit-baseline.json" // on-disk path for the baseline file
)

// cli is the top-level kong command struct.
type cli struct {
	Check    CheckCmd    `cmd:"" help:"Check architecture constraints."`
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
// CheckCmd
// ---------------------------------------------------------------------------

// CheckCmd runs the full archfit analysis pipeline.
type CheckCmd struct {
	Config   string   `short:"c" help:"Path to config file (optional; built-in defaults used if absent)." default:".archfit.yaml"`
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

	mode := engine.Mode{
		Base:       c.Base,
		Full:       c.Full,
		Advisory:   c.Advisory,
		ReportOnly: c.Report,
		Formats:    c.Format,
	}

	diag, err := runPipeline(ctx, deps, cfg, configDir, mode, base)
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
		case "md", "markdown":
			renderErr = markdown.New().Render(diag, deps.Stdout)
		}
		if renderErr != nil {
			return &exitError{code: 3, msg: fmt.Sprintf("render %s: %v", format, renderErr)}
		}
	}

	// --report promises "never exit with a failure code": the verdict is still
	// rendered, but findings and metric regressions do not affect the exit.
	if c.Report {
		return nil
	}
	return verdictToError(diag.Verdict)
}

// runPipeline resolves scope, builds extractors/rules/metrics, collects change
// history and optional tool inputs, and executes the engine. check, scan, and
// baseline all run through this single path: baseline snapshots must be computed
// from exactly the same inputs as check verdicts, or every post-baseline check
// reports phantom metric regressions and unmatched finding fingerprints.
func runPipeline(ctx context.Context, deps *appDeps, cfg config.Config, configDir string, mode engine.Mode, base baseline.Baseline) (diagnostic.Diagnostic, error) {
	sc := cfg.ForScope()
	sc.WorkDir = configDir
	sc.Base = mode.Base
	sc.Full = mode.Full
	s, err := scope.Resolve(ctx, sc, deps.Runner)
	if err != nil {
		return diagnostic.Diagnostic{}, err
	}

	extractors := []engine.Extractor{
		golang.New(cfg.ForExtract("go")),
		ts.New(deps.Runner, cfg.ForExtract("typescript")),
		py.New(deps.Runner, cfg.ForExtract("python")),
	}

	rs := rules.New(cfg.ForRules())
	// metrics.New must run BEFORE ApplyVolatility below: risk_hub captures only
	// hand-authored config volatility, never churn-derived values.
	ms := metrics.New(cfg)

	// Recent git history (cheap; runs by default): per-file churn drives module
	// volatility (unbalanced_edge, BC severity) and the modularity metrics
	// (change_amplification, hidden_coupling). Hand-authored volatility/subdomain
	// config always wins; a non-git repo leaves these signals empty.
	change := metrics.ChangeHistory{}
	if churn, coChange, _, herr := git.History(ctx, s.Root, deps.Runner); herr == nil {
		cfg.ApplyVolatility(config.DeriveVolatility(cfg.Modules, churn))
		change.FileChurn, change.CoChange = churn, coChange
	}
	change.FileLOC = sourceFileLOC(s.Root)

	// Architecture-fitness enforcement signals (deterministic FS scan; always runs).
	change.FitnessSignals = fitness.Detect(s.Root)

	// Ownership resolution: fills module owner gaps from CODEOWNERS or git-author
	// history. Explicit config owner always wins; resolver only fills empty slots.
	// Absent CODEOWNERS and non-git repos yield an empty map — no fabrication.
	cfg.FillMissingOwners(ownership.Resolve(ctx, s.Root, cfg.ModuleMapView(), deps.Runner))

	// Cyclomatic complexity via an external multi-language tool (lizard) — opt-in
	// (tools.complexity.enabled: on) like SCIP, since it shells out and adds cost.
	if cfg.ComplexityEnabled() {
		change.Complexity = complexityFuncs(ctx, deps.Runner, s.Root)
	}

	// Clone detection — opt-in (tools.clones.enabled: on). Run returns empty+absent
	// when disabled or the tool is missing; the metric reports n/a in that case.
	var clonesCov diagnostic.Coverage
	change.CloneClusters, clonesCov, _ = clones.Run(ctx, deps.Runner, s.Root, cfg.ClonesEnabled())
	change.ExtraCoverage = append(change.ExtraCoverage, clonesCov)

	// gitnexus optional symbol-impact enrichment — opt-in (tools.gitnexus.enabled: on).
	// Never auto: gitnexus may require network access. Returns empty+absent when
	// disabled or tool absent; risk_hub falls back to surface-breadth-only in that case.
	// Coverage is appended to ExtraCoverage so the engine includes it in the diagnostic.
	var gitnexusCov diagnostic.Coverage
	change.GitnexusImpact, gitnexusCov, _ = gitnexus.Run(ctx, deps.Runner, s.Root, cfg.GitnexusEnabled())
	change.ExtraCoverage = append(change.ExtraCoverage, gitnexusCov)

	// SCIP symbol-level strength is opt-in (tools.scip.enabled: on): the indexer is
	// whole-repo and slow, so it must not run on the default check path, and the
	// decision must live in config (not PATH presence) to keep metrics deterministic.
	var resolver engine.SymbolResolver = engine.NopSymbolResolver{}
	if cfg.ScipEnabled() {
		resolver = scip.New(deps.Runner)
	}

	patternCfg := cfg.ForPatterns()
	return engine.Run(ctx, mode, s, cfg.ForClassify(), cfg.ForStaleness(), cfg.ForStatus(), extractors, astgrep.New(deps.Runner), resolver, patternCfg, rs, ms, base, change, time.Now())
}

// ---------------------------------------------------------------------------
// BaselineCmd
// ---------------------------------------------------------------------------

// BaselineCmd runs the engine and saves findings as the new baseline.
type BaselineCmd struct {
	Config   string `short:"c" default:".archfit.yaml"`
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

	// Baseline runs the exact same pipeline as check (same change history,
	// pattern provider, and SCIP resolver): snapshot values recorded from
	// different inputs would surface as phantom deltas on the next check.
	mode := engine.Mode{Full: c.Full, Advisory: c.Advisory, Base: c.Base}
	diag, err := runPipeline(ctx, deps, cfg, configDir, mode, existingBase)
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
	Config      string `short:"c" default:".archfit.yaml"`
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

	// Same pipeline as check/scan: explain must resolve the finding from the
	// same evidence (providers, change history) that produced it.
	diag, err := runPipeline(ctx, deps, cfg, configDir, engine.Mode{Full: true, Advisory: true}, existingBase)
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
			if f.Edge.From.Module != "" || f.Edge.To.Module != "" {
				_, _ = fmt.Fprintf(deps.Stdout, "modules:    %s -> %s\n", f.Edge.From.Module, f.Edge.To.Module)
			}
			for _, loc := range f.Locations {
				_, _ = fmt.Fprintf(deps.Stdout, "location:   %s:%d\n", loc.File, loc.Line)
			}
			_, _ = fmt.Fprintf(deps.Stdout, "why:        %s\n", f.Why)
			_, _ = fmt.Fprintf(deps.Stdout, "constraint: %s\n", f.Constraint)
			for _, alt := range f.Alternatives {
				_, _ = fmt.Fprintf(deps.Stdout, "allowed:    %s\n", alt)
			}
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
	Output string `short:"o" help:"Output file (use '-' for stdout)." default:".archfit.yaml"`
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
	Config string `short:"c" help:"Config file." default:".archfit.yaml"`
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
// ".archfit.yaml" and the file is absent, it returns config.Default() so the
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

// ---------------------------------------------------------------------------
// InstallCmd
// ---------------------------------------------------------------------------

// InstallCmd installs external tools required for language analysis.
type InstallCmd struct {
	Lang   []string `name:"lang" help:"Languages to install tools for: py, ts, go. Repeatable. Default: py." enum:"go,ts,py" default:"py"`
	DryRun bool     `name:"dry-run" short:"n" help:"Print install commands without running them."`
}

const installSubcmd = "install"

func (c *InstallCmd) Run(deps *appDeps) error { //nolint:unparam // satisfies kong command interface; future versions may return errors
	ctx := context.Background()
	for _, lang := range c.Lang {
		switch lang {
		case "py":
			c.installPy(ctx, deps)
		case "ts":
			c.installTS(ctx, deps)
		case "go":
			c.installGo(ctx, deps)
		}
	}
	return nil
}

func (c *InstallCmd) installPy(ctx context.Context, deps *appDeps) {
	_, _ = fmt.Fprintln(deps.Stdout, "python tools:")
	// uv is the only tool archfit needs for Python analysis: the extractor injects
	// grimp transiently at run time via `uv run --with grimp`, so no separate
	// grimp install step is required.
	c.ensureTool(ctx, deps, "uv", "uv", "https://docs.astral.sh/uv/getting-started/installation/")
}

func (c *InstallCmd) installTS(ctx context.Context, deps *appDeps) {
	_, _ = fmt.Fprintln(deps.Stdout, "typescript tools:")
	c.ensureTool(ctx, deps, "node", "node", "https://nodejs.org/")
}

func (c *InstallCmd) installGo(ctx context.Context, deps *appDeps) {
	_, _ = fmt.Fprintln(deps.Stdout, "go tools:")
	if _, ok := deps.Runner.Detect(ctx, "go"); ok {
		_, _ = fmt.Fprintf(deps.Stdout, "  %-16s ok\n", "go")
	} else {
		_, _ = fmt.Fprintf(deps.Stdout, "  go: missing — install from https://go.dev/dl/\n")
	}
}

// ensureTool checks whether tool is present; if not, tries brew install formula.
func (c *InstallCmd) ensureTool(ctx context.Context, deps *appDeps, tool, formula, url string) {
	if _, ok := deps.Runner.Detect(ctx, tool); ok {
		_, _ = fmt.Fprintf(deps.Stdout, "  %-16s ok\n", tool)
		return
	}
	_, _ = fmt.Fprintf(deps.Stdout, "  %-16s missing\n", tool)
	if _, ok := deps.Runner.Detect(ctx, "brew"); ok {
		c.runOrPrint(ctx, deps, "brew", []string{installSubcmd, formula})
	} else {
		_, _ = fmt.Fprintf(deps.Stdout, "    install from: %s\n", url)
	}
}

// runOrPrint prints (dry-run) or executes the install command.
func (c *InstallCmd) runOrPrint(ctx context.Context, deps *appDeps, cmd string, args []string) {
	parts := make([]string, 0, 1+len(args))
	parts = append(parts, cmd)
	parts = append(parts, args...)
	line := strings.Join(parts, " ")
	if c.DryRun {
		_, _ = fmt.Fprintf(deps.Stdout, "  [dry-run] %s\n", line)
		return
	}
	_, _ = fmt.Fprintf(deps.Stdout, "  %s ... ", line)
	out, err := deps.Runner.Run(ctx, toolrun.ToolCmd{
		Name:    cmd,
		Args:    args,
		Timeout: 5 * time.Minute,
	})
	if err != nil || out.ExitCode != 0 {
		msg := strings.TrimSpace(string(out.Stderr))
		if err != nil {
			msg = err.Error()
		}
		_, _ = fmt.Fprintf(deps.Stdout, "failed: %s\n", msg)
		return
	}
	_, _ = fmt.Fprintln(deps.Stdout, "ok")
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
