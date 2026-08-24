package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/engine"
	"github.com/alexei-led/archfit/internal/extract/registry"
	"github.com/alexei-led/archfit/internal/model/evidence"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/ownership"
	"github.com/alexei-led/archfit/internal/relationship/labels"
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

// ownerDegradationWarning returns a disclosure message when ownership
// resolution did not produce usable module owners despite a signal that it
// should have — a CODEOWNERS file existed but matched none of the configured
// modules, or the git-author history walk timed out before finishing. Both
// cases silently degrade coupling distance to code_structure without ever
// telling the caller. Plain SourceNone (no CODEOWNERS, no git data) is
// deliberately excluded — the ownership package documents that as a clean
// "nothing to attribute" result, not a degradation, and SourceGit (the
// designed CODEOWNERS→git fallback) is expected behaviour, not a defect.
// Returns "" when src needs no disclosure.
func ownerDegradationWarning(src ownership.Source) string {
	switch src {
	case ownership.SourceCodeownersNoMatch:
		return "owner resolution: a CODEOWNERS file was found but matched none of the configured " +
			"modules (owner_source=codeowners_no_match) — its rules may simply not cover any module " +
			"path (benign), or the --root/subtree case or module path globs are wrong; coupling " +
			"distance falls back to code_structure"
	case ownership.SourceGitTimeout:
		return "owner resolution: the git-author history walk timed out before resolving any owner " +
			"(owner_source=git_timeout) — coupling distance falls back to code_structure"
	default:
		return ""
	}
}

// tsUnresolvedWarning returns a disclosure message when the TypeScript
// extractor (dependency-cruiser) reported partial coverage with a non-empty
// Reason — e.g. a high unresolved-import-specifier count from a missing
// tsconfig path alias or an uninstalled dependency. Those edges silently land
// in the external bucket, excluded from coupling_balance's denominator, so the
// gap must not be stderr-silent (mirrors ownerDegradationWarning). Returns ""
// when no such coverage record is present.
func tsUnresolvedWarning(cov []evidence.Coverage) string {
	for _, c := range cov {
		if c.Tool == toolDepCruiser && c.Status == evidence.StatusPartial && c.Reason != "" {
			return toolDepCruiser + ": " + c.Reason
		}
	}
	return ""
}

// pyUnresolvedWarning returns a disclosure message when grimp reported
// unresolved Python imports. Those imports are emitted as low-confidence
// external edges, so partial coverage should not be stderr-silent.
func pyUnresolvedWarning(cov []evidence.Coverage) string {
	for _, c := range cov {
		if c.Tool == toolGrimp && c.Unresolved > 0 {
			if strings.Contains(c.Reason, "imports unresolved") {
				return toolGrimp + ": " + c.Reason
			}
			return fmt.Sprintf("%s: %d imports unresolved — check languages.python.package and src layout", toolGrimp, c.Unresolved)
		}
	}
	return ""
}

// loadConfig loads the config file at path. When path equals the default
// ".archfit.yaml" and the file is absent, it returns config.Default() so that
// commands which tolerate a missing config (doctor, explain) can still run.
// Do NOT call this from analyze or check — use loadAnalysisConfig, which
// requires the config to exist and returns a clear error when it does not.
// An explicit non-default --config path that is missing always returns an error.
func loadConfig(ctx context.Context, path string) (config.Config, error) {
	cfg, err := config.Load(ctx, path)
	if err != nil {
		if path == defaultConfigPath && errors.Is(err, os.ErrNotExist) {
			return config.Default(), nil
		}
		return config.Config{}, err
	}
	if err := validateConfigRules(cfg); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

// validateConfigRules constructs the rules block to surface rule-type errors
// (unknown type:, public_api_max without max:) at load time. config.Load
// cannot do this itself — config is a support layer and must not import
// rules — so without this check `doctor` and the init/update revalidation
// would pass a config that later hard-fails at analyze time.
func validateConfigRules(cfg config.Config) error {
	_, err := engine.BuildRules(cfg.ForRules())
	return err
}

// applyFlagOverrides applies non-empty CLI flag values onto cfg, overriding
// whatever the config file (or Default) provided.
func applyFlagOverrides(cfg *config.Config, severity string, lang []string) error {
	if severity != "" {
		cfg.Coupling.MinSeverity = severity
	}
	for _, key := range lang {
		canonical := registry.ByAlias(key)
		if canonical == "" {
			return fmt.Errorf("--lang: unknown analyzer %q; see %s", key, languagesDocsURL)
		}
		cfg.SetToolMode(canonical, config.ModeOn)
	}
	return nil
}

// effectiveConfigHash returns the config hash that governed this run: the
// sha256 of the on-disk config file, or "" when --no-config ignored that file
// (built-in defaults were used). Hashing an ignored file would make the reported
// hash misleading and non-reproducible — it would change when a file the run
// never read changes.
func effectiveConfigHash(path string) string {
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
func verdictToError(v result.Verdict) error {
	switch v {
	case result.VerdictFail:
		return &exitError{code: 1}
	case result.VerdictWarn:
		return &exitError{code: 2}
	default:
		return nil
	}
}

// printConfigLint writes config-quality warnings to w (stderr). It is silent
// when there are none. The header explains why under-specified modules matter,
// then one line per module names the omitted fields. Deterministic; advisory.
func printConfigLint(w io.Writer, warnings []config.LintWarning) {
	if len(warnings) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "config-quality: %d module(s) under-specified — "+
		"degrades distance/volatility classification (can cause BC advisory floods):\n",
		len(warnings))
	for _, warn := range warnings {
		_, _ = fmt.Fprintf(w, "  - %s\n", warn.String())
	}
}
