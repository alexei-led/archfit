package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DoctorCmd checks toolchain availability and prints a table.
type DoctorCmd struct{}

func (c *DoctorCmd) Run(deps *appDeps) error { //nolint:unparam // satisfies kong command interface; future versions may return errors
	ctx := context.Background()

	tools := []struct {
		name    string
		cmd     string
		install string // one-line hint shown when the tool is missing
	}{
		{"go", "go", "https://go.dev/dl"},
		{"git", "git", "https://git-scm.com/downloads"},
		{"node", "node", "https://nodejs.org"},
		{"bunx", "bunx", "https://bun.sh"},
		{"npx", "npx", "ships with node"},
		{"uv", "uv", "https://docs.astral.sh/uv/getting-started/installation"},
		{"python3", "python3", "https://www.python.org/downloads"},
		{"sg (ast-grep)", "sg", "cargo install ast-grep / brew install ast-grep"},
		{"scip-typescript", "scip-typescript", "npm install -g @sourcegraph/scip-typescript"},
		{"scip-python", "scip-python", "npm install -g @sourcegraph/scip-python"},
		{"scip-go", "scip-go", "go install github.com/sourcegraph/scip-go/cmd/scip-go@latest"},
		// Semantic depth tools — their absence lowers analysis_confidence.
		{toolLizard, toolLizard, "uv tool install lizard / pip install lizard"},
		{toolJscpd, toolJscpd, "npm install -g jscpd"},
		{toolGitnexus, toolGitnexus, "see docs/guide — git-history change-coupling index"},
	}

	_, _ = fmt.Fprintf(deps.Stdout, "%-16s %-8s %s\n", "TOOL", "STATUS", "PATH / INSTALL")
	_, _ = fmt.Fprintf(deps.Stdout, "%s\n", strings.Repeat("-", 60))

	for _, t := range tools {
		if info, ok := deps.Runner.Detect(ctx, t.cmd); ok {
			_, _ = fmt.Fprintf(deps.Stdout, "%-16s %-8s %s\n", t.name, "ok", info.Path)
		} else {
			_, _ = fmt.Fprintf(deps.Stdout, "%-16s %-8s %s\n", t.name, "missing", t.install)
		}
	}

	// Off-gate LLM setup (enrich / explain --llm): provider config + key + cache.
	_, _ = fmt.Fprintf(deps.Stdout, "\nLLM (off-gate; enrich/explain only — never used by check):\n")
	cfg, cfgErr := loadConfig(ctx, defaultConfigPath, false)
	if llmCfg, ok := cfg.LLM(); cfgErr == nil && ok {
		_, _ = fmt.Fprintf(deps.Stdout, "  provider: %s  model: %s\n", llmCfg.Provider, llmCfg.Model)
		switch llmCfg.Provider {
		case providerAnthropic:
			_, _ = fmt.Fprintf(deps.Stdout, "  ANTHROPIC_API_KEY: %s\n", keyStatus(os.Getenv("ANTHROPIC_API_KEY")))
		case "openai":
			_, _ = fmt.Fprintf(deps.Stdout, "  OPENAI_API_KEY: %s\n", keyStatus(os.Getenv("OPENAI_API_KEY")))
		case "ollama":
			_, _ = fmt.Fprintf(deps.Stdout, "  base_url: %s (no key needed)\n", llmCfg.BaseURL)
		}
		if entries, err := filepath.Glob(filepath.Join(".archfit-cache", "llm", "*.json")); err == nil {
			_, _ = fmt.Fprintf(deps.Stdout, "  cache: %d entries in .archfit-cache/llm\n", len(entries))
		}
	} else {
		_, _ = fmt.Fprintln(deps.Stdout, "  not configured (set tools.llm provider + model to enable enrich)")
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
