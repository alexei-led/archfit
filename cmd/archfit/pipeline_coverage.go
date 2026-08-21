package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/extract/golang"
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
// whose real discovery is not root-only. Two languages need it:
//
//   - Go: go/packages discovers members by walking for nested go.mod dirs
//     (CLAUDE.md, "Go workspace loading") and then applies the
//     languages.go.modules include/exclude filter. Without the override a
//     services/api/go.mod repo answers "no Go here".
//   - Rust: languages.rust.manifest can point cargo at a sub-crate manifest with
//     no root Cargo.toml at all, so the root-only check answers "no Rust here"
//     for a project the extractor happily analyses.
//
// Getting either wrong turns a real analyzer failure into "language not
// present" — the one absent shape both --base and `config compare` read as
// safely comparable. dependency-cruiser (package.json) and grimp (pyproject and
// friends) do resolve from a root manifest, so they keep the plain check.
var primaryToolProjectProbe = map[string]func(root string, cfg config.Config, exclusions []string) bool{
	toolGoPackages: goProjectPresent,
	toolCargo:      rustProjectPresent,
}

// primaryProjectPresent reports whether the language behind a primary coverage
// tool is present under root, using the analyzer's own discovery shape.
// exclusions are the run's EFFECTIVE exclusion globs (defaults merged with
// config) — the same set the extractors filter on.
func primaryProjectPresent(tool, root string, cfg config.Config, markers, exclusions []string) bool {
	if probe, ok := primaryToolProjectProbe[tool]; ok {
		return probe(root, cfg, exclusions)
	}
	return projectMarkerPresent(root, markers)
}

// goProjectPresent reports whether any go.mod the Go extractor would actually
// load exists at or under root. A go.work above root is deliberately NOT
// evidence: when it lists members inside root those members carry their own
// go.mod (the walk finds them), and when it lists none, Go is not in this scan
// root at all.
//
// The languages.go.modules include/exclude filter is applied through the
// extractor's own golang.FilterMembers, not a re-implementation of it: the
// extractor reports absent when the filter removes every member, so a probe
// that ignored the filter would call that deliberate scoping a coverage gap.
// FilterMembers short-circuits when neither list is configured, so the common
// case costs nothing.
func goProjectPresent(root string, cfg config.Config, exclusions []string) bool {
	ec := cfg.ForExtract(config.LangGo)
	return markerInTree(root, markerGoMod, exclusions, func(dir string) bool {
		return len(golang.FilterMembers([]string{dir}, root, ec.GoModuleInclude, ec.GoModuleExclude)) == 1
	})
}

// rustProjectPresent mirrors the Rust extractor's applicability marker
// (internal/extract/rust, Extract): a configured languages.rust.manifest
// resolved against root exactly like cargo's --manifest-path when set, else the
// root Cargo.toml. Exclusions play no part — the extractor stats the manifest
// directly, and a probe that filtered it would disagree with the run.
func rustProjectPresent(root string, cfg config.Config, _ []string) bool {
	manifest := cfg.ForExtract(config.LangRust).CargoManifest
	if manifest == "" {
		return projectMarkerPresent(root, []string{markerCargoToml})
	}
	if !filepath.IsAbs(manifest) {
		manifest = filepath.Join(root, manifest)
	}
	_, err := os.Stat(manifest)
	return err == nil
}

// markerInTree reports whether marker exists in root or any directory under it
// that the run's exclusions do not remove and accept approves. accept receives
// the absolute directory holding the marker. Stops at the first ACCEPTED hit —
// a rejected candidate must not end the walk, or a filtered member shadows an
// analysed one deeper in the tree.
//
// The exclusion check is a CORRECTNESS filter, not a bound: a marker the
// extractor excludes but this walk counts makes the analyzer's "absent" read as
// a coverage gap instead of "the language is not here", which turns the whole
// origin delta inert (`--base`) or not-comparable (`config compare`). It is
// applied to the marker's repo-relative path with the same doublestar matcher
// the extractors use, so a `go.mod` under `testdata/` or an excluded subtree
// cannot be counted. Directory pruning is derived from the same globs and is
// only a walk bound.
func markerInTree(root, marker string, exclusions []string, accept func(dir string) bool) bool {
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
		if !accept(path) {
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
	// cfg.Exclude is ALREADY the run's effective exclusion set: runPipeline merges
	// the defaults in once, at setup, before projecting any view, so the marker
	// probe reads exactly what the extractors filtered on. Never re-merge here.
	// scope.MergeExclusions is not idempotent — it CONSUMES `!` re-includes, so a
	// second pass re-seeds the very defaults the user removed (pinned by
	// scope.TestMergeExclusions). Re-merging made the probe skip trees the
	// extractors analysed, suppressing real coverage gaps.
	exclusions := cfg.Exclude
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
		// markDisabledPrimaries normally rewrites these rows to StatusDisabled
		// before this loop sees them (so the status check above already skipped
		// them); the check stays because this function must derive the right gaps
		// from ANY coverage slice, marked or not.
		if primaryDisabledByConfig(cfg, c.Tool) {
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
				// cargo-modules is Rust-specific but not a primary tool. It runs inside
				// the Rust extractor and behind the SAME applicability marker, so it
				// reads the marker through rustProjectPresent — a configured
				// languages.rust.manifest points both at the sub-crate manifest.
				if configToolGate(cfg, c.Tool) == gateWarn && !rustProjectPresent(root, cfg, exclusions) {
					continue
				}
			default:
				if markers, ok := primaryToolProjectMarkers[c.Tool]; ok {
					lang := primaryToolLanguage[c.Tool]
					if cfg.ToolGate(lang) == "" && !primaryProjectPresent(c.Tool, root, cfg, markers, exclusions) {
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

// primaryDisabledByConfig reports whether tool is a language primary analyzer
// that the config switched OFF (languages.<id>.enabled: false) without also
// pinning an explicit gate on it. The explicit-gate carve-out is what lets a
// user say "I turned this off but still want to be told it did not run".
//
// Single source for that question: markDisabledPrimaries rewrites exactly these
// rows, and buildCoverageGaps suppresses exactly these gaps. Two readings of one
// predicate would let the row and the gap disagree.
func primaryDisabledByConfig(cfg config.Config, tool string) bool {
	lang, isPrimary := primaryToolLanguage[tool]
	return isPrimary && cfg.ToolMode(lang) == view.ModeOff && cfg.ToolGate(lang) == ""
}

// markDisabledPrimaries rewrites the coverage row of every language primary
// analyzer the config switched off OVER A LANGUAGE THAT IS PRESENT into an
// explicit StatusDisabled row.
//
// The extractors report ModeOff as StatusAbsent, which is indistinguishable from
// "this language is not in the tree". Both comparison paths read that shape —
// primary + absent + no coverage gap — as "the language is not present here" and
// drop the analyzer from the comparison entirely (decision.gradeTool,
// normalizeCoverage). Two configs that BOTH disabled Go over a Go repo then
// graded fully comparable while neither had looked at a line of it.
//
// After this pass, absent-without-a-gap has exactly ONE cause left — project
// markers missing — which is what both consumers' rules already assume. A
// disabled row lands on their existing StatusDisabled arm and reports as shared,
// declared blindness.
//
// The presence probe is load-bearing in BOTH directions. Without it the rewrite
// is the mirror image of the bug it fixes: a repo with no TypeScript would be
// told "TypeScript analysis is switched off", and `python: {enabled: false}` on a
// Go-only repo would grade not_comparable against a config that merely left
// python unset — two configurations that measured the tree identically. It is
// the same probe buildCoverageGaps suppresses on, so the row and the gap cannot
// disagree.
//
// Deliberately narrow: rows with an explicit gate keep StatusAbsent so they
// still raise a gap and still fail --require-tools. Non-primary analyzers are
// untouched; their absence is never read as a statement about the tree.
func markDisabledPrimaries(cov []diagnostic.Coverage, cfg config.Config, root string) []diagnostic.Coverage {
	for i, c := range cov {
		if c.Status != diagnostic.StatusAbsent || !primaryDisabledByConfig(cfg, c.Tool) {
			continue
		}
		if !primaryLanguagePresent(cfg, c.Tool, root) {
			continue
		}
		cov[i].Status = diagnostic.StatusDisabled
		cov[i].Reason = languageDisabledReason(primaryToolLanguage[c.Tool])
	}
	return cov
}

// primaryLanguagePresent reports whether the language behind a primary coverage
// tool has a project in root, using the analyzer's own discovery shape.
//
// An empty root cannot be probed. It answers "present", which discloses the
// opt-out rather than hiding it — the same abstain-toward-disclosure choice
// buildCoverageGaps makes when it skips marker suppression for an empty root.
func primaryLanguagePresent(cfg config.Config, tool, root string) bool {
	if root == "" {
		return true
	}
	return primaryProjectPresent(tool, root, cfg, primaryToolProjectMarkers[tool], cfg.Exclude)
}

// languageDisabledReason is the reason text stamped on a disabled primary row,
// naming the exact config key that switched it off.
func languageDisabledReason(lang string) string {
	return "language analysis disabled by config — set `languages." + lang + ".enabled: true` in .archfit.yaml to enable"
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
