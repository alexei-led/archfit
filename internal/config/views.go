package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/model/module"
	"github.com/alexei-led/archfit/internal/syntax"
	"github.com/alexei-led/archfit/internal/view"
)

// Config-quality lint field tokens, reported in ConfigWarning.Missing.
const (
	// lintFieldOwner marks a module with no `owner:`. Distance then falls back to
	// code structure, so cross-module edges that share a single (or git-author
	// degenerate) owner collapse to the same distance — the cause of near-identical
	// BC advisory floods.
	lintFieldOwner = "owner"
	// lintFieldVolatility marks a module with neither `subdomain:` nor `volatility:`.
	// Volatility is then undeclared, so coupling advice cannot recommend lowering
	// volatility; declaring either closes the gap (core→high, supporting→low,
	// generic→low).
	lintFieldVolatility = "subdomain/volatility"
)

// LintWarning is one config-quality lint finding: a configured module that
// omits a field archfit relies on for accurate Balanced-Coupling classification.
// Missing is non-empty and in fixed order (owner before subdomain/volatility).
// Advisory only — Lint never affects the verdict.
type LintWarning struct {
	Module  string
	Missing []string
}

// String renders the warning as one deterministic line.
func (w LintWarning) String() string {
	return fmt.Sprintf("module %q omits %s", w.Module, strings.Join(w.Missing, ", "))
}

// Lint reports configured modules that omit fields archfit uses to classify
// Balanced-Coupling distance (owner) and volatility (subdomain/volatility).
// Under-specified modules degrade those classifications and explain symptoms a
// user often blames on the tool: missing owners collapse distance and flood BC
// advisories; missing subdomain+volatility leave volatility undeclared.
//
// Only modules with at least one path are linted — a pathless entry classifies
// nothing. Results are returned in deterministic module order; the Missing slice
// within each is in fixed order. Lint is advisory and never gates.
func (c Config) Lint() []LintWarning {
	var out []LintWarning
	for _, name := range sortedKeys(c.Modules) {
		def := c.Modules[name]
		if len(def.Paths) == 0 {
			continue
		}
		var missing []string
		if def.Owner == "" {
			missing = append(missing, lintFieldOwner)
		}
		if def.Subdomain == "" && def.Volatility == "" {
			missing = append(missing, lintFieldVolatility)
		}
		if len(missing) > 0 {
			out = append(out, LintWarning{Module: name, Missing: missing})
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Projection methods on Config.
// ---------------------------------------------------------------------------

// ForScope returns the ScopeConfig view.
func (c Config) ForScope() view.ScopeConfig {
	return view.ScopeConfig{
		Exclusions: c.Exclude,
	}
}

// ForExtract returns an ExtractConfig for the given language.
// Mode is derived from the Tools map (defaults to ModeAuto when absent).
func (c Config) ForExtract(lang string) view.ExtractConfig {
	ec := view.ExtractConfig{
		Src:        ".",
		Exclusions: c.Exclude,
	}

	// Collect all module paths and internal globs. Src is deliberately NOT
	// derived from Modules: a module's Paths glob classifies graph nodes (it
	// can be a dotted Python id or a Rust crate name), not a filesystem
	// directory the TypeScript extractor can scan — see
	// docs/plans/completed/20260701-multilang-reliability-fixes.md Task 4.3. Src stays
	// at the "." default set above; the TS extractor falls back to "src"
	// when it sees "." (internal/extract/ts/ts.go).
	var paths, internal []string
	for _, name := range sortedKeys(c.Modules) {
		def := c.Modules[name]
		paths = append(paths, def.Paths...)
		internal = append(internal, def.Internal...)
	}
	ec.Paths = paths
	ec.Internal = internal

	// Go-specific fields.
	if lang == LangGo {
		ec.GoModuleInclude = c.Languages.Go.Modules.Include
		ec.GoModuleExclude = c.Languages.Go.Modules.Exclude
	}

	// Python-specific fields.
	if lang == LangPython {
		ec.PyPackage = c.Languages.Python.Package
	}

	// Rust-specific fields.
	if lang == LangRust {
		ec.CargoManifest = c.Languages.Rust.Manifest
		ec.CargoFeatures = c.Languages.Rust.Features
		ec.IncludeDevDeps = c.Languages.Rust.IncludeDevDeps
		ec.ModuleGraph = c.CargoModulesEnabled()
	}

	// Derive Mode from the language's enabled state. An unset (empty) mode
	// defaults to auto — the same default the old absent-tool-key path produced.
	ec.Mode = c.ToolMode(lang)
	if ec.Mode == "" {
		ec.Mode = view.ModeAuto
	}

	return ec
}

// ForClassify returns the ClassifyConfig view. Classification sees only
// hand-authored module definitions — explicit volatility and subdomain fields
// only. Git-churn-derived volatility is intentionally excluded: Balanced Coupling
// forbids commit-history volatility on the gate path.
func (c Config) ForClassify() view.ClassifyConfig {
	return view.ClassifyConfig{
		Modules:                   c.Modules,
		Layers:                    c.Layers,
		ModuleMap:                 module.BuildMap(c.Modules),
		BCAdvisoryMinSeverity:     c.Coupling.MinSeverity,
		ExplicitOwners:            c.explicitOwners,
		VolatilityCascadeEnabled:  c.Coupling.VolatilityCascade,
		ExternalSystems:           c.ExternalSystems,
		DuplicatedKnowledgePolicy: view.NormalizeDuplicatedKnowledgePolicy(c.Coupling.DuplicatedKnowledge),
	}
}

// ForRules returns the RuleConfig view.
func (c Config) ForRules() view.RuleConfig {
	return view.RuleConfig{
		Rules:     c.Rules,
		Layers:    c.Layers,
		ModuleMap: module.BuildMap(c.Modules),
	}
}

// ForMetric returns the MetricConfig for the named metric.
// Returns a zero MetricConfig (all knobs unset) if the metric is not
// configured — the metric stays enabled; only an explicit `enabled: false`
// disables it (Enabled is *bool, nil means default-on; see metrics.New).
func (c Config) ForMetric(name string) view.MetricConfig {
	if c.Metrics == nil {
		return view.MetricConfig{}
	}
	return c.Metrics[name]
}

// ForWaivers returns the WaiverSet view consumed by the status stage.
func (c Config) ForWaivers() view.WaiverSet {
	return view.WaiverSet{Waivers: c.Waivers}
}

// ForOutput returns the OutputConfig view.
func (c Config) ForOutput() view.OutputConfig {
	return view.OutputConfig{
		JSON:     c.Outputs.JSON,
		Markdown: c.Outputs.Markdown,
		SARIF:    c.Outputs.SARIF,
	}
}

// ModuleMapView returns a module.Map built from this Config's Modules.
func (c Config) ModuleMapView() module.Map {
	return module.BuildMap(c.Modules)
}

// ForPatterns returns the PatternConfig view: all PatternDef values collected
// from rules that declare a patterns: block.
func (c Config) ForPatterns() view.PatternConfig {
	var out view.PatternConfig
	for _, r := range c.Rules {
		out = append(out, r.Patterns...)
	}
	return out
}

// ForSyntax returns the syntax-stage view. Languages are derived from the four
// known language keys; an unset mode is enabled.
func (c Config) ForSyntax() view.SyntaxConfig {
	enabled := c.SyntaxEnabled()
	var langs []string
	for _, lang := range []string{LangGo, LangTypeScript, LangPython, LangRust} {
		if c.ToolMode(lang) == view.ModeOff {
			continue
		}
		langs = append(langs, lang)
	}
	return view.SyntaxConfig{
		Enabled:   enabled,
		Languages: langs,
	}
}

// ForFileClass returns the file-class configuration for the classifier.
// Auto-detection rules are not represented here; this carries only the
// user-supplied extension patterns. A zero value is valid — auto-detection
// still runs when all slices are empty.
func (c Config) ForFileClass() syntax.FileClassConfig {
	return syntax.FileClassConfig{
		GeneratedGlobs: c.FileClass.GeneratedGlobs,
		TestGlobs:      c.FileClass.TestGlobs,
		MockFrameworks: c.FileClass.MockFrameworks,
	}
}

// ForStaleness returns the StalenessConfig view.
func (c Config) ForStaleness() view.StalenessConfig {
	var threshold time.Duration
	if c.ModuleReview.StaleAfter != "" {
		if d, err := time.ParseDuration(c.ModuleReview.StaleAfter); err == nil {
			threshold = d
		}
	}
	// gate: off explicitly disables module review, matching gate semantics
	// everywhere else (off = disabled). Any other configured signal —
	// stale_after, or a warn/fail gate — enables the advisory pass.
	gate := c.ModuleReview.Gate
	enabled := gate != string(view.ModeOff) && (c.ModuleReview.StaleAfter != "" || gate != "")
	return view.StalenessConfig{
		Enabled:   enabled,
		Threshold: threshold,
		Modules:   c.Modules,
	}
}
