package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/toolrun"
)

const installSubcmd = "install"

// InstallCmd installs external tools required for language analysis.
type InstallCmd struct {
	Lang   []string `name:"lang" help:"Languages to install tools for: py, ts, go. Repeatable. Default: py." enum:"go,ts,py" default:"py"`
	DryRun bool     `name:"dry-run" short:"n" help:"Print install commands without running them."`
}

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
