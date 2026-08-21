package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/view"
)

// gateWarn / gateFail are the coverage-gap gate strings stamped on each gap.
// warn (default) degrades a missing tool's metrics to n/a (never green) and reports
// it, but does not fail the build; fail is the opt-in hard gate (tools.<x>.gate: fail
// / --require-tools). Sourced from config.GateMode so the two never drift.
const (
	gateOff  = string(config.GateOff)
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
var primaryGraphMetrics = []string{"coverage", "coupling_balance", metricEncapsulation, metricCycle, metricBlastRadius}

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
		toolJscpd:        {"npm install -g jscpd", []string{"coupling_balance"}},
		toolCargoModules: {"cargo install cargo-modules (analyzers.cargo_modules.enabled: true)", []string{metricCycle, metricBlastRadius, metricEncapsulation}},
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

// primaryToolProjectProbe overrides the root-only marker check for analyzers
// whose real discovery is not root-only. Only Go needs it: dependency-cruiser
// requires package.json in the project root, grimp and cargo probe their own
// root manifests, but go/packages discovers members by walking for nested
// go.mod dirs (CLAUDE.md, "Go workspace loading"). Without the override a
// services/api/go.mod repo answers "no Go here", which turns a real analyzer
// failure into "language not present" — the one absent shape both --base and
// `config compare` read as safely comparable.
var primaryToolProjectProbe = map[string]func(root string, exclusions []string) bool{
	toolGoPackages: goProjectPresent,
}

// primaryProjectPresent reports whether the language behind a primary coverage
// tool is present under root, using the analyzer's own discovery shape.
// exclusions are the run's EFFECTIVE exclusion globs (defaults merged with
// config) — the same set the extractors filter on.
func primaryProjectPresent(tool, root string, markers, exclusions []string) bool {
	if probe, ok := primaryToolProjectProbe[tool]; ok {
		return probe(root, exclusions)
	}
	return projectMarkerPresent(root, markers)
}

// goProjectPresent reports whether any go.mod the Go extractor would actually
// load exists at or under root. A go.work above root is deliberately NOT
// evidence: when it lists members inside root those members carry their own
// go.mod (the walk finds them), and when it lists none, Go is not in this scan
// root at all.
func goProjectPresent(root string, exclusions []string) bool {
	return markerInTree(root, markerGoMod, exclusions)
}

// markerInTree reports whether marker exists in root or any directory under it
// that the run's exclusions do not remove. Stops at the first hit.
//
// The exclusion check is a CORRECTNESS filter, not a bound: a marker the
// extractor excludes but this walk counts makes the analyzer's "absent" read as
// a coverage gap instead of "the language is not here", which turns the whole
// origin delta inert (`--base`) or not-comparable (`config compare`). It is
// applied to the marker's repo-relative path with the same doublestar matcher
// the extractors use, so a `go.mod` under `testdata/` or an excluded subtree
// cannot be counted. Directory pruning is derived from the same globs and is
// only a walk bound.
func markerInTree(root, marker string, exclusions []string) bool {
	prune := scope.ExcludedDirNames(exclusions)
	found := false
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil //nolint:nilerr // an unreadable entry is skipped, not fatal
		}
		if path != root {
			name := d.Name()
			if _, pruned := prune[name]; pruned || strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
		}
		if _, statErr := os.Stat(filepath.Join(path, marker)); statErr != nil {
			return nil
		}
		rel, relErr := filepath.Rel(root, filepath.Join(path, marker))
		if relErr != nil || matchesAnyGlob(exclusions, filepath.ToSlash(rel)) {
			return nil
		}
		found = true
		return fs.SkipAll
	})
	return found
}

// matchesAnyGlob reports whether path matches any of the exclusion globs, using
// the same matcher the extractors apply to their own relative paths.
func matchesAnyGlob(globs []string, path string) bool {
	for _, g := range globs {
		if matched, _ := doublestar.Match(g, path); matched {
			return true
		}
	}
	return false
}

// projectMarkerPresent reports whether any of the given project-marker filenames
// exist in root. Checks only the root dir (not recursive) — the analyzers that
// use it (dependency-cruiser, grimp, cargo) resolve their project from a root
// manifest. Go goes through primaryToolProjectProbe instead. Returns true when
// markers is empty (no marker = cannot determine absence, so don't suppress).
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
// gate on that tool overrides the suppression — except gate: off, which is a
// deliberate opt-out and never produces an install prompt.
func buildCoverageGaps(cov []diagnostic.Coverage, cfg config.Config, root string) []diagnostic.CoverageGap {
	// The EFFECTIVE exclusions, so the marker probe agrees with the extractors:
	// a marker they never see must not become evidence the language is present.
	// MergeExclusions is set-based and idempotent, so re-merging an already
	// merged cfg.Exclude is a no-op.
	exclusions := scope.MergeExclusions(cfg.Exclude)
	var gaps []diagnostic.CoverageGap
	for _, c := range cov {
		// Only truly absent tools produce a gap. Disabled-by-config tools are an
		// intentional opt-out; partial coverage is informational (not actionable).
		if c.Status != diagnostic.StatusAbsent {
			continue
		}
		if configToolGate(cfg, c.Tool) == gateOff {
			continue
		}
		// A disabled language's primary tool is not a gap the user needs to close —
		// don't tell a Rust-only repo to install dependency-cruiser/grimp/go-packages.
		// Explicit warn/fail gates keep the gap even if the language is absent.
		if lang, isPrimary := primaryToolLanguage[c.Tool]; isPrimary &&
			cfg.ToolMode(lang) == view.ModeOff && cfg.ToolGate(lang) == "" {
			continue
		}
		// Suppress the gap when the language's project marker is absent from the
		// scan root — the language simply isn't present in this repo, so the missing
		// tool is not actionable. An explicit warn/fail gate overrides this (same
		// carve-out as the disabled-language check above). cargo-modules (opt-in
		// intra-crate tool, not a language primary) is also suppressed when no
		// Cargo.toml is present.
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
					if cfg.ToolGate(lang) == "" && !primaryProjectPresent(c.Tool, root, markers, exclusions) {
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
	if g := cfg.ToolGate(key); g != "" {
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
