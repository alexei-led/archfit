package config

import (
	"fmt"
	"strings"
	"time"
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
	// volatility; declaring either closes the gap (core→high, supporting→medium,
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
func (c Config) ForScope() ScopeConfig {
	return ScopeConfig{
		Exclusions: c.Exclusions,
	}
}

// ForExtract returns an ExtractConfig for the given language.
// Mode is derived from the Tools map (defaults to ModeAuto when absent).
func (c Config) ForExtract(lang string) ExtractConfig {
	ec := ExtractConfig{
		Src:        ".",
		Exclusions: c.Exclusions,
	}

	// Collect all module paths and internal globs.
	var paths, internal []string
	first := true
	for _, name := range sortedKeys(c.Modules) {
		def := c.Modules[name]
		paths = append(paths, def.Paths...)
		internal = append(internal, def.Internal...)
		if first && len(def.Paths) > 0 {
			ec.Src = def.Paths[0]
			first = false
		}
	}
	ec.Paths = paths
	ec.Internal = internal

	// Python-specific fields.
	if lang == LangPython {
		ec.PyPackage = c.PythonPackage
	}

	// Derive Mode from the Tools map. The key is the language name itself.
	if tc, ok := c.Tools[lang]; ok {
		ec.Mode = tc.Enabled
	} else {
		ec.Mode = ModeAuto
	}

	return ec
}

// ForClassify returns the ClassifyConfig view. Classification sees only
// hand-authored module definitions — explicit volatility and subdomain fields
// only. Git-churn-derived volatility is intentionally excluded: Balanced Coupling
// forbids commit-history volatility on the gate path.
func (c Config) ForClassify() ClassifyConfig {
	return ClassifyConfig{
		Modules:               c.Modules,
		Layers:                c.Layers,
		ModuleMap:             buildModuleMap(c.Modules),
		BCAdvisoryMinSeverity: c.BCAdvisoryMinSeverity,
		ExplicitOwners:        c.explicitOwners,
	}
}

// ForRules returns the RuleConfig view.
func (c Config) ForRules() RuleConfig {
	return RuleConfig{
		Rules:     c.Rules,
		Layers:    c.Layers,
		ModuleMap: buildModuleMap(c.Modules),
	}
}

// ForMetric returns the MetricConfig for the named metric.
// Returns a zero MetricConfig (Enabled=false) if the metric is not configured.
func (c Config) ForMetric(name string) MetricConfig {
	if c.Metrics == nil {
		return MetricConfig{}
	}
	return c.Metrics[name]
}

// ForStatus returns the ExceptionSet view.
func (c Config) ForStatus() ExceptionSet {
	return ExceptionSet{Exceptions: c.Exceptions}
}

// ForOutput returns the OutputConfig view.
func (c Config) ForOutput() OutputConfig {
	return OutputConfig{
		JSON:     c.Outputs.JSON,
		Markdown: c.Outputs.Markdown,
		SARIF:    c.Outputs.SARIF,
	}
}

// ModuleMapView returns a ModuleMap built from this Config's Modules.
func (c Config) ModuleMapView() ModuleMap {
	return buildModuleMap(c.Modules)
}

// ForPatterns returns the PatternConfig view: all PatternDef values collected
// from rules that declare a patterns: block.
func (c Config) ForPatterns() PatternConfig {
	var out PatternConfig
	for _, r := range c.Rules {
		out = append(out, r.Patterns...)
	}
	return out
}

// ForStaleness returns the StalenessConfig view.
func (c Config) ForStaleness() StalenessConfig {
	var threshold time.Duration
	if c.MapReview.StaleAfter != "" {
		if d, err := time.ParseDuration(c.MapReview.StaleAfter); err == nil {
			threshold = d
		}
	}
	// gate: off explicitly disables map review, matching gate semantics
	// everywhere else (off = disabled). Any other configured signal —
	// stale_after, or a warn/fail gate — enables the advisory pass.
	gate := c.MapReview.Gate
	enabled := gate != string(ModeOff) && (c.MapReview.StaleAfter != "" || gate != "")
	return StalenessConfig{
		Enabled:   enabled,
		Threshold: threshold,
		Modules:   c.Modules,
	}
}
