package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexei-led/archfit/v3/internal/config"
	"github.com/alexei-led/archfit/v3/internal/extract/registry"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const installSubcmd = "install"

// DoctorCmd checks toolchain availability and prints a status table. With --fix
// it also installs the analyzer toolchains archfit can install automatically
// (the former `install` command, folded in so `doctor` both diagnoses and fixes).
type DoctorCmd struct {
	Fix    bool     `name:"fix" aliases:"install" help:"Install missing analyzer toolchains (archfit installs what it can)."`
	Lang   []string `name:"lang" help:"Toolchains to install with --fix (default: all). Repeatable. .NET has no automatic install: install the .NET SDK and build the helper with make dotnet-roslyn." enum:"go,ts,py,rust"`
	DryRun bool     `name:"dry-run" short:"n" help:"With --fix: print install commands without running them."`
	Format string   `name:"format" help:"Output format: text or json. json prints the tool rows and the agent loop (archfit.doctor.v1)." enum:"text,json" default:"text"`
}

func (*DoctorCmd) Help() string {
	return "Checks local analyzer tools and prints a status table, then the agent loop: which harness is here, which archfit hooks it would run (an uninstalled or untrusted hook is inactive), the target branch and scope a check resolves, and whether a policy file differs from the target. With --fix, installs the toolchains archfit can install automatically. For complete setup notes, see " + languagesDocsURL + "."
}

func (c *DoctorCmd) Run(deps *appDeps) error {
	ctx := deps.runContext()
	if c.Format == formatJSON && c.Fix {
		return &exitError{code: 3, msg: "error: doctor --format json only reports; run doctor --fix without it"}
	}

	// Cross-language tools stay literal here; per-language tools (compilers,
	// scip indexers) come from the language registry so adding a language adds
	// its doctor probes in one place.
	tools := []registry.Tool{
		{Name: "git", Command: "git", InstallHint: "https://git-scm.com/downloads"},
		{Name: "uv", Command: "uv", InstallHint: "https://docs.astral.sh/uv/getting-started/installation"},
		{Name: "ast-grep", Command: "ast-grep", InstallHint: languagesDocsURL + "#install-analyzers"},
		// Optional semantic depth tools — their absence degrades coupling_balance precision.
		{Name: toolJscpd, Command: toolJscpd, InstallHint: languagesDocsURL + "#install-analyzers"},
	}
	for _, lang := range registry.All() {
		tools = append(tools, lang.DoctorTools...)
	}

	// Versions are probed where analysis runs: here, where a global.json
	// selects the .NET SDK.
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	// A tool configured to an unusable value fails doctor after the full
	// report, as it fails analysis: a missing tool is not an error, a wrong one is.
	var invalid []error
	rows := make([]toolRow, 0, len(tools))
	for _, t := range tools {
		path, found, err := locateTool(ctx, deps.Runner, t)
		if err == nil && found && t.Version != nil {
			var version string
			if version, err = t.Version(ctx, deps.Runner, path, dir); err == nil {
				path += " (" + version + ")"
			}
		}
		switch {
		case err != nil:
			invalid = append(invalid, fmt.Errorf("%s: %w", t.Name, err))
			rows = append(rows, toolRow{Name: t.Name, Status: "invalid", Error: err.Error()})
		case found:
			rows = append(rows, toolRow{Name: t.Name, Status: "ok", Path: path})
		default:
			rows = append(rows, toolRow{Name: t.Name, Status: "missing", InstallHint: t.InstallHint})
		}
	}
	cfg, cfgErr := loadConfig(ctx, defaultConfigPath)
	if c.Format == formatJSON {
		if err := writeDoctorJSON(deps, doctorReport{
			SchemaVersion: doctorSchemaVersion, Tools: rows, AgentLoop: agentLoopStatus(ctx, deps, cfg.TargetBranch),
		}); err != nil {
			return err
		}
		return errors.Join(invalid...)
	}

	nameWidth := len("TOOL")
	for _, t := range tools {
		nameWidth = max(nameWidth, len(t.Name))
	}
	row := func(name, status, detail string) {
		_, _ = fmt.Fprintf(deps.Stdout, "%-*s %-8s %s\n", nameWidth, name, status, detail)
	}
	row("TOOL", "STATUS", "PATH / INSTALL")
	_, _ = fmt.Fprintf(deps.Stdout, "%s\n", strings.Repeat("-", 60))
	for _, r := range rows {
		row(r.Name, r.Status, r.Path+r.InstallHint+r.Error)
	}

	// Off-gate LLM setup (config init/update/enrich, analyze/explain --ai-summary): provider config + key + cache.
	_, _ = fmt.Fprintf(deps.Stdout, "\nLLM (off-gate; config init/update/enrich, analyze/explain --ai-summary — never used by the gate):\n")
	// cfgErr is non-nil only when the file exists but fails to load (an absent
	// default config falls back to config.Default()) — surface it, or doctor
	// misreports a broken config as "not configured" and skips config checks.
	if cfgErr != nil {
		_, _ = fmt.Fprintf(deps.Stdout, "  %s failed to load: %v (LLM status and config checks skipped)\n", defaultConfigPath, cfgErr)
	} else if llmCfg, ok := cfg.LLM(); ok {
		_, _ = fmt.Fprintf(deps.Stdout, "  provider: %s  model: %s\n", llmCfg.Provider, llmCfg.Model)
		switch llmCfg.Provider {
		case providerAnthropic:
			_, _ = fmt.Fprintf(deps.Stdout, "  ANTHROPIC_API_KEY: %s\n", keyStatus(os.Getenv("ANTHROPIC_API_KEY")))
		case "openai":
			_, _ = fmt.Fprintf(deps.Stdout, "  OPENAI_API_KEY: %s\n", keyStatus(os.Getenv("OPENAI_API_KEY")))
		case providerDeepSeek:
			_, _ = fmt.Fprintf(deps.Stdout, "  DEEPSEEK_API_KEY: %s\n", keyStatus(os.Getenv("DEEPSEEK_API_KEY")))
		case providerCompatible:
			if llmCfg.APIKeyEnv != "" {
				_, _ = fmt.Fprintf(deps.Stdout, "  %s: %s\n", llmCfg.APIKeyEnv, keyStatus(os.Getenv(llmCfg.APIKeyEnv)))
			} else {
				_, _ = fmt.Fprintln(deps.Stdout, "  authentication: none")
			}
		case providerOllama:
			_, _ = fmt.Fprintln(deps.Stdout, "  authentication: none")
		}
		if llmCfg.BaseURL != "" {
			_, _ = fmt.Fprintf(deps.Stdout, "  base_url: %s\n", llmCfg.BaseURL)
		}
		if entries, err := filepath.Glob(filepath.Join(llmCacheDir(""), "*.json")); err == nil {
			_, _ = fmt.Fprintf(deps.Stdout, "  cache: %d entries in .archfit-cache/llm\n", len(entries))
		}
	} else {
		_, _ = fmt.Fprintln(deps.Stdout, "  not configured (set ai provider + model to enable LLM commands)")
	}

	writeAgentLoop(deps.Stdout, agentLoopStatus(ctx, deps, cfg.TargetBranch))

	if c.Fix {
		if err := c.runFix(ctx, deps); err != nil {
			return err
		}
	}
	return errors.Join(invalid...)
}

// locateTool finds one doctor tool: through its own probe when it has one,
// else on PATH.
func locateTool(ctx context.Context, runner toolrun.Runner, t registry.Tool) (string, bool, error) {
	if t.Locate != nil {
		return t.Locate()
	}
	info, ok := runner.Detect(ctx, t.Command)
	return info.Path, ok, nil
}

// writeDoctorJSON writes the doctor report as one indented JSON document.
func writeDoctorJSON(deps *appDeps, doc doctorReport) error {
	enc := json.NewEncoder(deps.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: write the doctor report: %v", err)}
	}
	return nil
}

// keyStatus renders the presence of an API key without leaking it.
func keyStatus(v string) string {
	if v == "" {
		return "missing"
	}
	return "set"
}

// runFix installs the analyzer toolchains archfit can install automatically. With
// no --lang it covers every supported language; --lang scopes it; --dry-run prints.
func (c *DoctorCmd) runFix(ctx context.Context, deps *appDeps) error {
	langs := c.Lang
	if len(langs) == 0 {
		langs = []string{"go", "ts", "py", "rust"}
	}
	_, _ = fmt.Fprintln(deps.Stdout, "\ninstalling analyzer toolchains:")
	for _, lang := range langs {
		switch registry.ByAlias(lang) {
		case config.LangPython:
			if err := c.installPy(ctx, deps); err != nil {
				return err
			}
		case config.LangTypeScript:
			if err := c.installTS(ctx, deps); err != nil {
				return err
			}
		case config.LangGo:
			if err := c.installGo(ctx, deps); err != nil {
				return err
			}
		case config.LangRust:
			c.installRust(ctx, deps)
		}
	}
	return nil
}

func (c *DoctorCmd) installPy(ctx context.Context, deps *appDeps) error {
	_, _ = fmt.Fprintln(deps.Stdout, "python tools:")
	// The extractor selects Python 3.14 and grimp==3.17 through uv. When Python
	// downloads are disabled, that interpreter must already be available;
	// discovering uv alone does not prove the analyzer environment is ready.
	return c.ensureTool(ctx, deps, "uv", "uv", "https://docs.astral.sh/uv/getting-started/installation/")
}

func (c *DoctorCmd) installTS(ctx context.Context, deps *appDeps) error {
	_, _ = fmt.Fprintln(deps.Stdout, "typescript tools:")
	return c.ensureTool(ctx, deps, "node", "node", "https://nodejs.org/")
}

func (c *DoctorCmd) installGo(ctx context.Context, deps *appDeps) error {
	_, _ = fmt.Fprintln(deps.Stdout, "go tools:")
	if _, ok := deps.Runner.Detect(ctx, "go"); ok {
		_, _ = fmt.Fprintf(deps.Stdout, "  %-16s ok\n", "go")
	} else {
		_, _ = fmt.Fprintf(deps.Stdout, "  go: missing — install from https://go.dev/dl/\n")
	}
	return nil
}

func (c *DoctorCmd) installRust(ctx context.Context, deps *appDeps) {
	_, _ = fmt.Fprintln(deps.Stdout, "rust tools:")
	if _, ok := deps.Runner.Detect(ctx, "cargo"); ok {
		_, _ = fmt.Fprintf(deps.Stdout, "  %-16s ok\n", "cargo")
	} else {
		_, _ = fmt.Fprintf(deps.Stdout, "  cargo: missing — install Rust from https://rustup.rs/\n")
	}
}

// ensureTool checks whether tool is present; if not, tries brew install formula.
func (c *DoctorCmd) ensureTool(ctx context.Context, deps *appDeps, tool, formula, url string) error {
	if _, ok := deps.Runner.Detect(ctx, tool); ok {
		_, _ = fmt.Fprintf(deps.Stdout, "  %-16s ok\n", tool)
		return nil
	}
	_, _ = fmt.Fprintf(deps.Stdout, "  %-16s missing\n", tool)
	if _, ok := deps.Runner.Detect(ctx, "brew"); ok {
		return c.runOrPrint(ctx, deps, "brew", []string{installSubcmd, formula})
	}
	_, _ = fmt.Fprintf(deps.Stdout, "    install from: %s\n", url)
	return nil
}

// runOrPrint prints (dry-run) or executes the install command.
func (c *DoctorCmd) runOrPrint(ctx context.Context, deps *appDeps, cmd string, args []string) error {
	parts := make([]string, 0, 1+len(args))
	parts = append(parts, cmd)
	parts = append(parts, args...)
	line := strings.Join(parts, " ")
	if c.DryRun {
		_, _ = fmt.Fprintf(deps.Stdout, "  [dry-run] %s\n", line)
		return nil
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
		return fmt.Errorf("%s: %s", line, msg)
	}
	_, _ = fmt.Fprintln(deps.Stdout, "ok")
	return nil
}
