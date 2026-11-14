package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	Fix    bool     `name:"fix" aliases:"install" help:"Install the hooks of the harnesses found here (Claude Code, Codex, git) and the missing toolchains archfit can install."`
	Lang   []string `name:"lang" help:"Toolchains to install with --fix (default: all). Repeatable. .NET has no automatic install: install the .NET SDK and build the helper with make dotnet-roslyn." enum:"go,ts,py,rust"`
	DryRun bool     `name:"dry-run" short:"n" help:"With --fix: print the install commands and the hook files without changing anything."`
	Format string   `name:"format" help:"Output format: text or json. json prints the tool rows and the agent loop (archfit.doctor.v1)." enum:"text,json" default:"text"`
}

func (*DoctorCmd) Help() string {
	return `This checkout: the tools archfit uses and the hooks of the agent loop.

The tools come in two groups: needed for the check (git, and ast-grep, cargo
or dotnet for the languages that use them) and optional, for the report. A
missing tool of the first group makes the check unknown for the code it
cannot read.

Then the hooks: for Claude Code, Codex and git, whether the harness is here
and whether each archfit hook runs. A settings file that git ignores is
named: other checkouts and worktrees do not get its hooks.

With --fix, doctor writes the hooks of each harness it finds, at the root of
the repository: .claude/settings.json, .codex/hooks.json, and the pre-commit
hook. It keeps every other setting and hook, and never replaces a pre-commit
hook that archfit did not write. Running it twice changes nothing. Outside a
git repository it writes nothing and says so. It also installs the
toolchains archfit can install (--lang).

For setup notes, see ` + languagesDocsURL + `.`
}

func (c *DoctorCmd) Run(deps *appDeps) error {
	ctx := deps.runContext()
	if c.Format == formatJSON && c.Fix {
		return &exitError{code: 3, msg: "error: doctor --format json only reports; run doctor --fix without it"}
	}

	check, report := registry.ToolGroups()

	// Versions are probed where analysis runs: here, where a global.json
	// selects the .NET SDK.
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	// A tool configured to an unusable value fails doctor after the full
	// report, as it fails analysis: a missing tool is not an error, a wrong one is.
	var invalid []error
	rows := make([]toolRow, 0, len(check)+len(report))
	for _, group := range []struct {
		name  string
		tools []registry.ToolUse
	}{{toolGroupCheck, check}, {toolGroupReport, report}} {
		for _, t := range group.tools {
			row, err := probeTool(ctx, deps.Runner, t, dir)
			if err != nil {
				invalid = append(invalid, fmt.Errorf("%s: %w", t.Name, err))
			}
			row.Group = group.name
			rows = append(rows, row)
		}
	}
	cfg, cfgErr := loadConfig(ctx, defaultConfigPath)
	loop := agentLoopStatus(ctx, deps, cfg.TargetBranch)
	// The Claude harness config is resolved at the repository root by agentLoopStatus.
	plugin := claudePluginStatus(filepath.Dir(filepath.Dir(loop.Harnesses[0].Config)))
	if c.Format == formatJSON {
		if err := writeDoctorJSON(deps, struct {
			doctorReport
			ClaudePlugin harnessStatus `json:"claude_plugin"`
		}{doctorReport{
			SchemaVersion: doctorSchemaVersion, Tools: rows, AgentLoop: loop,
		}, plugin}); err != nil {
			return err
		}
		return errors.Join(invalid...)
	}

	writeToolGroups(deps.Stdout, rows)
	if cfgErr != nil {
		_, _ = fmt.Fprintf(deps.Stdout, "\n%s failed to load: %v (config checks skipped)\n", defaultConfigPath, cfgErr)
	}

	writeAgentLoop(deps.Stdout, loop)
	writeClaudePlugin(deps.Stdout, plugin)

	if c.Fix {
		// The hooks come first: a toolchain install that fails (brew offline)
		// must not cost a checkout its hooks.
		hooksErr := c.installHooks(ctx, deps)
		if err := errors.Join(hooksErr, c.runFix(ctx, deps)); err != nil {
			return err
		}
	}
	return errors.Join(invalid...)
}

// The two tool groups of design 11.8.
const (
	toolGroupCheck  = "check"
	toolGroupReport = "report"
)

// toolGroupTitles head the two tables of the text report.
var toolGroupTitles = map[string]string{
	toolGroupCheck:  "Needed for the check:",
	toolGroupReport: "\nOptional, for the report:",
}

// probeTool finds one tool and reads its version. err is a configured value
// the tool cannot use; the row says so too.
func probeTool(ctx context.Context, runner toolrun.Runner, t registry.ToolUse, dir string) (toolRow, error) {
	row := toolRow{Name: t.Name, Languages: t.Languages}
	path, found, err := locateTool(ctx, runner, t.Tool)
	if err == nil && found && t.Version != nil {
		var version string
		if version, err = t.Version(ctx, runner, path, dir); err == nil {
			path += " (" + version + ")"
		}
	}
	switch {
	case err != nil:
		row.Status, row.Error = "invalid", err.Error()
	case found:
		row.Status, row.Path = "ok", path
	default:
		row.Status, row.InstallHint = "missing", t.InstallHint
	}
	return row, err
}

// toolLanguages is the FOR column of a row: the languages that use the tool.
func toolLanguages(r toolRow) string {
	if len(r.Languages) == 0 {
		return "all"
	}
	return strings.Join(r.Languages, ", ")
}

// writeToolGroups prints one table per group, with the columns aligned across
// both tables.
func writeToolGroups(w io.Writer, rows []toolRow) {
	nameWidth, forWidth := len("TOOL"), len("FOR")
	for _, r := range rows {
		nameWidth, forWidth = max(nameWidth, len(r.Name)), max(forWidth, len(toolLanguages(r)))
	}
	row := func(name, status, langs, detail string) {
		_, _ = fmt.Fprintf(w, "%-*s %-8s %-*s %s\n", nameWidth, name, status, forWidth, langs, detail)
	}
	group := ""
	for _, r := range rows {
		if r.Group != group {
			group = r.Group
			_, _ = fmt.Fprintln(w, toolGroupTitles[group])
			row("TOOL", "STATUS", "FOR", "PATH / INSTALL")
			_, _ = fmt.Fprintln(w, strings.Repeat("-", 60))
		}
		row(r.Name, r.Status, toolLanguages(r), r.Path+r.InstallHint+r.Error)
	}
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
func writeDoctorJSON(deps *appDeps, doc any) error {
	enc := json.NewEncoder(deps.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: write the doctor report: %v", err)}
	}
	return nil
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
	// Only the optional SCIP reader needs Python through uv. Import extraction
	// uses ast-grep and never starts a Python runtime.
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
