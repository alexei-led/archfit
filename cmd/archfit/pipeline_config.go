package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/labels"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
)

// buildConfigWarnings assembles the advisory ConfigWarnings block: under-specified
// modules from cfg.Lint() (deterministic order) followed by any swallowed
// optional-tool errors. Returns nil when both are empty.
func buildConfigWarnings(cfg config.Config, toolWarnings []string) []string {
	lint := cfg.Lint()
	out := make([]string, 0, len(lint)+len(toolWarnings))
	for _, w := range lint {
		out = append(out, w.String())
	}
	out = append(out, toolWarnings...)
	if len(out) == 0 {
		return nil
	}
	return out
}

// buildJudgmentDecisionTasks returns actionable decision-task strings for
// undeclared judgment inputs that force the scorer to abstain:
//
//  1. Modules with no subdomain AND no volatility declared — the scorer cannot
//     place their edges on the book's volatility scale. Tells the user to edit
//     .archfit.yaml and add subdomain: or volatility:.
//  2. Approved labels whose strength came from an LLM (provenance: llm) —
//     notifies the user they can upgrade provenance to "human" after code review.
//
// These are advisory strings appended to ConfigWarnings, not gate findings.
// Sorted for deterministic output.
func buildJudgmentDecisionTasks(cfg config.Config, lbls []labels.Label, configPath string) []string {
	var out []string

	// 1. Modules missing subdomain and volatility — scorer abstains on volatility.
	names := make([]string, 0, len(cfg.Modules))
	for name := range cfg.Modules {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		def := cfg.Modules[name]
		if def.Subdomain == "" && def.Volatility == "" {
			out = append(out,
				"decision needed: module "+name+" has no subdomain or volatility declared — "+
					"scorer abstains on volatility for its edges; "+
					"add `subdomain: core|supporting|generic` or `volatility: high|medium|low` "+
					"to modules."+name+" in "+configPath)
		}
	}

	// 2. LLM-provenance approved labels — inform the user they can promote to human.
	for _, l := range lbls {
		if l.Status == labels.StatusApproved && l.Provenance == labels.ProvenanceLLM {
			out = append(out,
				"decision needed: label "+l.From+" → "+l.To+
					" approved but provenance is llm — "+
					"if you have reviewed the code, set `provenance: human` in .archfit-labels.yaml "+
					"to restore full confidence in coupling_balance")
		}
	}

	return out
}

// outputInsideRootWarning reports whether dir (an absolute config/output
// directory) resolves strictly inside the absolute analyzed root. Returns a
// warning string in that case, "" when dir is the root itself or lies outside
// it. Path-only — no filesystem access — so it stays deterministic and testable.
func outputInsideRootWarning(root, dir string) string {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return ""
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return ""
	}
	return "output written inside analyzed root (" + rel + ") — exclude it or " +
		"use a path outside --root to keep scans deterministic"
}

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

// applyFlagOverrides applies non-empty CLI flag values onto cfg, overriding
// whatever the config file (or Default) provided.
func applyFlagOverrides(cfg *config.Config, severity string, lang []string) error {
	if severity != "" {
		cfg.BCAdvisoryMinSeverity = severity
	}
	for _, key := range lang {
		canonical := languageByAlias(key)
		if canonical == "" {
			return fmt.Errorf("--lang: unknown analyzer %q; see %s", key, languagesDocsURL)
		}
		if cfg.Tools == nil {
			cfg.Tools = make(map[string]config.ToolConfig)
		}
		cfg.Tools[canonical] = config.ToolConfig{Enabled: config.ModeOn}
	}
	return nil
}

// effectiveConfigHash returns the config hash that governed this run: the
// sha256 of the on-disk config file, or "" when --no-config ignored that file
// (built-in defaults were used). Hashing an ignored file would make the reported
// hash misleading and non-reproducible — it would change when a file the run
// never read changes.
func effectiveConfigHash(path string, noConfig bool) string {
	if noConfig {
		return ""
	}
	return computeConfigHash(path)
}

// computeConfigHash returns the sha256 hex digest of the raw config file bytes
// at path. Returns "" when the file cannot be read (absent, --no-config, etc.)
// so callers never fail on a missing config; they just get no hash.
func computeConfigHash(path string) string {
	b, err := os.ReadFile(path) //#nosec G304 -- path comes from the --config CLI flag, not arbitrary user input
	if err != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
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
