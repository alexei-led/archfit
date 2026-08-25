package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/module"

	"github.com/alexei-led/archfit/internal/ownership"
	"github.com/alexei-led/archfit/internal/relationship/labels"
)

// BuildConfigWarnings assembles the advisory ConfigWarnings block: under-specified
// modules from cfg.Lint() (deterministic order) followed by any swallowed
// optional-tool errors. Returns nil when both are empty.
func BuildConfigWarnings(lint []string, toolWarnings []string) []string {
	out := make([]string, 0, len(lint)+len(toolWarnings))
	out = append(out, lint...)
	out = append(out, toolWarnings...)
	if len(out) == 0 {
		return nil
	}
	return out
}

// BuildJudgmentDecisionTasks returns actionable decision-task strings for
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
func BuildJudgmentDecisionTasks(modules map[string]module.ModuleDef, lbls []labels.Label, configPath string) []string {
	var out []string

	// 1. Modules missing subdomain and volatility — scorer abstains on volatility.
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		def := modules[name]
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

// OutputInsideRootWarning reports whether dir (an absolute config/output
// directory) resolves strictly inside the absolute analyzed root. Returns a
// warning string in that case, "" when dir is the root itself or lies outside
// it. Path-only — no filesystem access — so it stays deterministic and testable.
func OutputInsideRootWarning(root, dir string) string {
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

// OwnerDegradationWarning returns a disclosure message when ownership
// resolution did not produce usable module owners despite a signal that it
// should have — a CODEOWNERS file existed but matched none of the configured
// modules, or the git-author history walk timed out before finishing. Both
// cases silently degrade coupling distance to code_structure without ever
// telling the caller. Plain SourceNone (no CODEOWNERS, no git data) is
// deliberately excluded — the ownership package documents that as a clean
// "nothing to attribute" result, not a degradation, and SourceGit (the
// designed CODEOWNERS→git fallback) is expected behaviour, not a defect.
// Returns "" when src needs no disclosure.
func OwnerDegradationWarning(src ownership.Source) string {
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

// TSUnresolvedWarning returns a disclosure message when the TypeScript
// extractor (dependency-cruiser) reported partial coverage with a non-empty
// Reason — e.g. a high unresolved-import-specifier count from a missing
// tsconfig path alias or an uninstalled dependency. Those edges silently land
// in the external bucket, excluded from coupling_balance's denominator, so the
// gap must not be stderr-silent (mirrors ownerDegradationWarning). Returns ""
// when no such coverage record is present.
func TSUnresolvedWarning(cov []evidence.Coverage) string {
	for _, c := range cov {
		if c.Tool == toolDepCruiser && c.Status == evidence.StatusPartial && c.Reason != "" {
			return toolDepCruiser + ": " + c.Reason
		}
	}
	return ""
}

// PyUnresolvedWarning returns a disclosure message when grimp reported
// unresolved Python imports. Those imports are emitted as low-confidence
// external edges, so partial coverage should not be stderr-silent.
func PyUnresolvedWarning(cov []evidence.Coverage) string {
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

// EffectiveConfigHash returns the config hash that governed this run: the
// sha256 of the on-disk config file, or "" when the file cannot be read.
func EffectiveConfigHash(path string) string {
	return ComputeConfigHash(path)
}

func effectiveConfigHash(path string) string {
	return EffectiveConfigHash(path)
}

// ComputeConfigHash returns the sha256 hex digest of the raw config file bytes
// at path. Returns "" when the file cannot be read.
func ComputeConfigHash(path string) string {
	b, err := os.ReadFile(path) //#nosec G304 -- path comes from the --config CLI flag, not arbitrary user input
	if err != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
