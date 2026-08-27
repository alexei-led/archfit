package acquisition

import (
	"sort"

	"github.com/alexei-led/archfit/internal/extract/registry"
	"github.com/alexei-led/archfit/internal/model/evidence"
)

// gateWarn / gateFail are the coverage-gap gate strings stamped on each gap.
// warn (default) degrades a missing tool's metrics to n/a (never green) and reports
// it, but does not fail the build; fail is the opt-in hard gate (tools.<x>.gate: fail
// / --require-tools). Sourced from config.GateMode so the two never drift.
const (
	gateOff  = "off"
	gateWarn = "warn"
	gateFail = "fail"
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
		toolJscpd:                 "clones",
		registry.ToolCargoModules: "cargo-modules",
	}
	for _, lang := range registry.All() {
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
		toolJscpd:                 {"npm install -g jscpd", []string{"coupling_balance"}},
		registry.ToolCargoModules: {"cargo install cargo-modules (analyzers.cargo_modules.enabled: true)", []string{metricCycle, metricBlastRadius, metricEncapsulation}},
	}
	for _, lang := range registry.All() {
		m[lang.PrimaryTool] = affectedMetrics{lang.InstallHint, primaryGraphMetrics}
	}
	return m
}

// primaryToolLanguage maps a language's primary-tool coverage name back to its
// config language key, so a coverage gap for a disabled language can be suppressed
// (a Rust-only repo should not be told to install go/ts/py analyzers). Built once.
var primaryToolLanguage = buildPrimaryToolLanguage()

func buildPrimaryToolLanguage() map[string]string {
	m := make(map[string]string, len(registry.All()))
	for _, lang := range registry.All() {
		m[lang.PrimaryTool] = lang.ID
	}
	return m
}

// primaryToolProjectProbe maps a language's primary-tool coverage name to the
// registry row's applicability probe, so buildCoverageGaps and
// markDisabledPrimaries can ask "is this language present?" by coverage name.
// Built once at init.
var primaryToolProjectProbe = buildPrimaryToolProjectProbe()

func buildPrimaryToolProjectProbe() map[string]func(root string, cfg CoverageOptions) bool {
	m := make(map[string]func(root string, cfg CoverageOptions) bool, len(registry.All()))
	for _, lang := range registry.All() {
		m[lang.PrimaryTool] = func(root string, cfg CoverageOptions) bool {
			probe, ok := cfg.ProjectPresent[lang.PrimaryTool]
			if !ok || probe == nil {
				// No probe projected: absence cannot be established, so disclose.
				return true
			}
			return probe(root)
		}
	}
	return m
}

// primaryProjectPresent reports whether the language behind a primary coverage
// tool is present under root, using the analyzer's own applicability code.
//
// An analyzer with no probe answers "present": absence cannot be established,
// and the safe direction is to disclose the gap rather than silently call the
// language missing.
func primaryProjectPresent(tool, root string, cfg CoverageOptions) bool {
	probe, ok := primaryToolProjectProbe[tool]
	if !ok || probe == nil {
		return true
	}
	return probe(root, cfg)
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
// warn/fail gate on that tool overrides the suppression; gate: off does not,
// because a deliberate opt-out over a language that is not even here needs no
// install prompt.
//
// gate: off does NOT suppress the gap over a language that IS here. That skip
// used to run before the presence check, and the silence it produced was read
// downstream as a fact about the TREE: both comparison paths take a PRIMARY
// analyzer that is absent with no gap as "this language is not present", so a
// gated-off analyzer over a repo full of that language dropped out of the
// comparison entirely, unmeasured and unmentioned. The gap is the only channel
// that carries the difference, so it has to be emitted; applyToolGate is where
// gate: off keeps its meaning, by refusing to escalate even under
// --require-tools.
func buildCoverageGaps(cov []evidence.Coverage, cfg CoverageOptions, root string) []evidence.CoverageGap {
	var gaps []evidence.CoverageGap
	for _, c := range cov {
		// Only truly absent tools produce a gap. Disabled-by-config tools are an
		// intentional opt-out; partial coverage is informational (not actionable).
		if c.Status != evidence.StatusAbsent {
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
		// Suppress the gap when the language's project is absent from the scan root
		// — the language simply isn't present in this repo, so the missing tool is
		// not actionable. Only an explicit warn/fail gate overrides this: a demand
		// to be told about a tool is a demand about a tool, whereas gate: off asks
		// for silence and gets it here, where silence claims nothing. An
		// unprobeable root (root == "") answers "present" and discloses, the same
		// abstain-toward-disclosure choice primaryLanguagePresent makes.
		// cargo-modules (opt-in intra-crate tool, not a language primary) is
		// suppressed on the same marker as Rust itself.
		if root != "" && !toolGateDemands(cfg, c.Tool) {
			switch c.Tool {
			case registry.ToolCargoModules:
				// cargo-modules is Rust-specific but not a primary tool. It runs inside
				// the Rust extractor and behind the SAME applicability marker, so it
				// reads the marker through rustProjectPresent — a configured
				// languages.rust.manifest points both at the sub-crate manifest.
				if probe := cfg.ProjectPresent[registry.ToolCargoModules]; probe != nil && !probe(root) {
					continue
				}
			default:
				if _, isPrimary := primaryToolLanguage[c.Tool]; isPrimary {
					if !primaryProjectPresent(c.Tool, root, cfg) {
						continue
					}
				}
			}
		}
		info, ok := toolAffectedMetrics[c.Tool]
		if !ok {
			continue
		}
		gaps = append(gaps, evidence.CoverageGap{
			Tool:            c.Tool,
			InstallCmd:      info.install,
			AffectedMetrics: info.metrics,
			Gate:            configToolGate(cfg, c.Tool),
		})
	}
	sort.Slice(gaps, func(i, j int) bool { return gaps[i].Tool < gaps[j].Tool })
	return gaps
}

// toolGateDemands reports whether the config explicitly asked to be told about
// this tool — an explicit warn or fail gate. An unset gate defaults to warn but
// demands nothing; gate: off demands the opposite. Only a demand overrides the
// marker suppression, so an explicit gate cannot be answered with silence and an
// opt-out cannot be answered with an install prompt for a language that is not
// in the tree.
func toolGateDemands(cfg CoverageOptions, tool string) bool {
	key, ok := coverageToolConfigKey[tool]
	if !ok {
		return false
	}
	switch cfg.Gates[key] {
	case gateWarn, gateFail:
		return true
	default:
		return false
	}
}

// primaryDisabledByConfig reports whether tool is a language primary analyzer
// that the config switched OFF (languages.<id>.enabled: false) without also
// pinning an explicit gate on it. The explicit-gate carve-out is what lets a
// user say "I turned this off but still want to be told it did not run".
//
// Single source for that question: markDisabledPrimaries rewrites exactly these
// rows, and buildCoverageGaps suppresses exactly these gaps. Two readings of one
// predicate would let the row and the gap disagree.
func primaryDisabledByConfig(cfg CoverageOptions, tool string) bool {
	lang, isPrimary := primaryToolLanguage[tool]
	return isPrimary && cfg.Modes[lang] == "off" && cfg.Gates[lang] == ""
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
// After this pass, absent-without-a-gap has exactly ONE cause left — the
// extractor's own applicability probe says the language is not in the tree —
// which is what both consumers' rules already assume. A
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
func markDisabledPrimaries(cov []evidence.Coverage, cfg CoverageOptions, root string) []evidence.Coverage {
	for i, c := range cov {
		if c.Status != evidence.StatusAbsent || !primaryDisabledByConfig(cfg, c.Tool) {
			continue
		}
		if !primaryLanguagePresent(cfg, c.Tool, root) {
			continue
		}
		cov[i].Status = evidence.StatusDisabled
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
func primaryLanguagePresent(cfg CoverageOptions, tool, root string) bool {
	if root == "" {
		return true
	}
	return primaryProjectPresent(tool, root, cfg)
}

// languageDisabledReason is the reason text stamped on a disabled primary row,
// naming the exact config key that switched it off.
func languageDisabledReason(lang string) string {
	return "language analysis disabled by config — set `languages." + lang + ".enabled: true` in .archfit.yaml to enable"
}

// configToolGate resolves the configured gate for the analyzer behind a coverage
// tool name, defaulting to warn. An unmapped tool or an empty gate: yields warn.
func configToolGate(cfg CoverageOptions, tool string) string {
	key, ok := coverageToolConfigKey[tool]
	if !ok {
		return gateWarn
	}
	if g := cfg.Gates[key]; g != "" {
		return g
	}
	return gateWarn
}
