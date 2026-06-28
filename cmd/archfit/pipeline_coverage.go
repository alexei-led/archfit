package main

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
)

// gateWarn / gateFail are the coverage-gap gate strings stamped on each gap.
// warn (default) degrades a missing tool's metrics to n/a (never green) and reports
// it, but does not fail the build; fail is the opt-in hard gate (tools.<x>.gate: fail
// / --require-tools). Sourced from config.GateMode so the two never drift.
const (
	gateWarn = string(config.GateWarn)
	gateFail = string(config.GateFail)
)

// coverageToolConfigKey maps a coverage tool name (as it appears in ToolCoverage,
// e.g. "go/packages") to the config Tools map key whose gate: governs it (e.g.
// "go"). Lets a user write tools.go.gate: fail to gate on the go/packages analyzer
// without knowing the internal coverage name. Tools absent here fall back to warn.
// The per-language primary analyzers come from the language registry; the
// cross-language optional tools stay literal. Built once at init.
var coverageToolConfigKey = buildCoverageToolConfigKey()

func buildCoverageToolConfigKey() map[string]string {
	m := map[string]string{
		toolLizard:       config.ToolComplexity,
		toolAstGrep:      config.ToolComplexity, // auto backend absent-both path
		toolJscpd:        config.ToolClones,
		toolCargoModules: config.ToolCargoModules,
	}
	for _, lang := range languageRegistry {
		m[lang.PrimaryTool] = lang.ID
	}
	return m
}

// primaryGraphMetrics are the metrics the dependency-graph extractors
// (go/packages, dependency-cruiser, grimp) unlock; absent any of them, all of
// these drop to n/a. Shared (read-only) across those per-language table entries.
var primaryGraphMetrics = []string{"coverage", "coupling_balance", "encapsulation", "cycle", "blast_radius"}

// affectedMetrics carries an absent analyzer's one-line install hint and the
// metrics its absence leaves unmeasured.
type affectedMetrics struct {
	install string
	metrics []string
}

// toolAffectedMetrics maps an absent analyzer's coverage name to its one-line
// install hint and the metrics its absence leaves unmeasured. Only tools listed
// here produce a CoverageGap — an absent coverage entry with no actionable
// install path is not a gap a user can close. Per-language analyzers come from
// the registry; cross-language optional tools stay literal. Built once at init.
var toolAffectedMetrics = buildToolAffectedMetrics()

func buildToolAffectedMetrics() map[string]affectedMetrics {
	m := map[string]affectedMetrics{
		toolLizard:       {"uv tool install lizard / pip install lizard", []string{"complexity"}},
		toolAstGrep:      {"cargo install ast-grep / brew install ast-grep (then optionally: go install github.com/fzipp/gocyclo/cmd/gocyclo@latest for exact Go CCN)", []string{"complexity"}},
		toolJscpd:        {"npm install -g jscpd", []string{"functional_candidates"}},
		toolCargoModules: {"cargo install cargo-modules (tools.cargo-modules.enabled: on)", []string{"cycle", "blast_radius", "cohesion", "encapsulation"}},
	}
	for _, lang := range languageRegistry {
		m[lang.PrimaryTool] = affectedMetrics{lang.InstallHint, primaryGraphMetrics}
	}
	return m
}

// primaryToolLanguage maps a language's primary-tool coverage name back to its
// config language key, so a coverage gap for a disabled language can be suppressed
// (a Rust-only repo should not be told to install go/ts/py analyzers). Built once.
var primaryToolLanguage = buildPrimaryToolLanguage()

func buildPrimaryToolLanguage() map[string]string {
	m := make(map[string]string, len(languageRegistry))
	for _, lang := range languageRegistry {
		m[lang.PrimaryTool] = lang.ID
	}
	return m
}

// primaryToolProjectMarkers maps a language's primary-tool coverage name to the
// project-marker filenames that signal the language is present in a repo root
// (e.g. "go/packages" → ["go.mod"], "cargo" → ["Cargo.toml"]). Used by
// buildCoverageGaps to suppress gaps for languages whose project is absent from
// the scan root. Built once at init.
var primaryToolProjectMarkers = buildPrimaryToolProjectMarkers()

func buildPrimaryToolProjectMarkers() map[string][]string {
	m := make(map[string][]string, len(languageRegistry))
	for _, lang := range languageRegistry {
		m[lang.PrimaryTool] = lang.ProjectMarkers
	}
	return m
}

// projectMarkerPresent reports whether any of the given project-marker filenames
// exist in root. Checks only the root dir (not recursive) — markers like go.mod
// and Cargo.toml are always at the repo root. Returns true when markers is empty
// (no marker = cannot determine absence, so don't suppress).
func projectMarkerPresent(root string, markers []string) bool {
	if len(markers) == 0 {
		return true
	}
	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(root, m)); err == nil {
			return true
		}
	}
	return false
}

// buildCoverageGaps derives the CoverageGaps block from the absent tool-coverage
// records. Each gap's Gate is the configured posture for that tool (tools.<x>.gate,
// default warn) — the --require-tools override is applied later by applyToolGate so
// the non-check callers of runPipeline are unaffected. Gaps are sorted by tool name
// so a double-run stays byte-identical regardless of upstream coverage order.
// Returns nil when no known tool is absent (omitempty keeps clean output unchanged).
//
// StatusDisabled entries (tools present but turned off in config) are intentionally
// excluded — the user does not need an "install" prompt for a deliberate opt-out.
// The tool_coverage block already carries the reason for any reader who wants it.
//
// Gaps are also suppressed when the language's project marker is absent from root
// (e.g. no Cargo.toml → Rust is not present → cargo gap is noise). An explicit
// gate on that tool overrides the suppression — it is an intentional "require it"
// even in repos that don't currently use that language.
func buildCoverageGaps(cov []diagnostic.Coverage, cfg config.Config, root string) []diagnostic.CoverageGap {
	var gaps []diagnostic.CoverageGap
	for _, c := range cov {
		// Only truly absent tools produce a gap. Disabled-by-config tools are an
		// intentional opt-out; partial coverage is informational (not actionable).
		if c.Status != diagnostic.StatusAbsent {
			continue
		}
		// A disabled language's primary tool is not a gap the user needs to close —
		// don't tell a Rust-only repo to install dependency-cruiser/grimp/go-packages.
		// An explicit gate on that tool (tools.<lang>.gate) is an intentional
		// "require it anyway" override and is preserved.
		if lang, isPrimary := primaryToolLanguage[c.Tool]; isPrimary &&
			cfg.Tools[lang].Enabled == config.ModeOff && cfg.Tools[lang].Gate == "" {
			continue
		}
		// Suppress the gap when the language's project marker is absent from the
		// scan root — the language simply isn't present in this repo, so the missing
		// tool is not actionable. An explicit gate overrides this (same carve-out as
		// the disabled-language check above). cargo-modules (opt-in intra-crate tool,
		// not a language primary) is also suppressed when no Cargo.toml is present.
		if root != "" {
			switch c.Tool {
			case toolCargoModules:
				// cargo-modules is Rust-specific but not a primary tool; use Cargo.toml.
				if configToolGate(cfg, c.Tool) == gateWarn && !projectMarkerPresent(root, []string{markerCargoToml}) {
					continue
				}
			default:
				if markers, ok := primaryToolProjectMarkers[c.Tool]; ok {
					lang := primaryToolLanguage[c.Tool]
					if cfg.Tools[lang].Gate == "" && !projectMarkerPresent(root, markers) {
						continue
					}
				}
			}
		}
		info, ok := toolAffectedMetrics[c.Tool]
		if !ok {
			continue
		}
		gaps = append(gaps, diagnostic.CoverageGap{
			Tool:            c.Tool,
			InstallCmd:      info.install,
			AffectedMetrics: info.metrics,
			Gate:            configToolGate(cfg, c.Tool),
		})
	}
	sort.Slice(gaps, func(i, j int) bool { return gaps[i].Tool < gaps[j].Tool })
	return gaps
}

// configToolGate resolves the configured gate for the analyzer behind a coverage
// tool name, defaulting to warn. An unmapped tool or an empty gate: yields warn.
func configToolGate(cfg config.Config, tool string) string {
	key, ok := coverageToolConfigKey[tool]
	if !ok {
		return gateWarn
	}
	if g := cfg.Tools[key].Gate; g != "" {
		return string(g)
	}
	return gateWarn
}

// applyToolGate finalises the hard-gate decision for a check/scan run: --require-tools
// raises every coverage gap to fail, and any gap that gates fail stamps the verdict
// fail so the rendered output reflects the policy failure. Returns true when the run
// must exit 1. The policy decision lives here in cmd/ (the layering invariant) — the
// core ring never sees tool names or gate config. Idempotent and render-order safe:
// callers invoke it before rendering so the output shows the effective gate.
func applyToolGate(diag *diagnostic.Diagnostic, requireTools bool) bool {
	failed := false
	for i := range diag.CoverageGaps {
		if requireTools {
			diag.CoverageGaps[i].Gate = gateFail
		}
		if diag.CoverageGaps[i].Gate == gateFail {
			failed = true
		}
	}
	if failed {
		diag.Verdict = diagnostic.VerdictFail
	}
	return failed
}
