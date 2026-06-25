package config

import (
	"time"

	"github.com/alexei-led/archfit/internal/model/coupling"
)

// MetricEntry holds the settings for a single metric inside the metrics map.
type MetricEntry struct {
	Enabled    bool    `yaml:"enabled"`
	Gate       string  `yaml:"gate"`
	MinDelta   float64 `yaml:"min_delta"`
	MaxNewHigh int     `yaml:"max_new_high"`
	MaxNew     int     `yaml:"max_new"`
}

// MetricsConfig holds settings for all metrics, keyed by metric name.
type MetricsConfig map[string]MetricEntry

// MapReviewConfig configures architecture-map staleness gating.
type MapReviewConfig struct {
	StaleAfter string `yaml:"stale_after"`
	Gate       string `yaml:"gate"`
}

// OutputsConfig controls which output formats are produced.
type OutputsConfig struct {
	JSON     bool `yaml:"json"`
	Markdown bool `yaml:"markdown"`
	SARIF    bool `yaml:"sarif"`
}

// ---------------------------------------------------------------------------
// View types — narrow projections of Config passed to each pipeline stage.
// ---------------------------------------------------------------------------

// ScopeConfig is the view passed to the scope resolution stage.
type ScopeConfig struct {
	Base       string // git ref to diff against (empty = none)
	Full       bool   // if true, full-repo mode (no diff)
	Exclusions []string
	WorkDir    string // working directory for git commands; empty = process cwd
}

// ExtractConfig is the view passed to a language extractor.
// Built via ForExtract(lang); holds only what the extractor needs.
type ExtractConfig struct {
	// Common fields.
	Src        string   // source root (first path of first module, or ".")
	Paths      []string // all module paths
	Exclusions []string
	Internal   []string // all internal globs across modules
	Mode       ToolMode // derived from Tools map for the given language/tool

	// Go-specific.
	BuildFlags []string // extra build flags passed to go/packages (e.g. ["-tags", "extractortest"])

	// TypeScript-specific.
	TSConfig string // path to tsconfig.json (empty = auto)

	// Python-specific.
	PyPackage   string // top-level package name
	ProjectRoot string // project root for uv --directory

	// Rust-specific.
	CargoManifest  string   // path to Cargo.toml (empty = auto, root manifest)
	CargoFeatures  []string // cargo features to activate (empty = default)
	IncludeDevDeps bool     // include dev-dependencies as edges
	ModuleGraph    bool     // run cargo-modules to build intra-crate module graph (opt-in)
}

// ClassifyConfig is the view passed to the classify stage.
type ClassifyConfig struct {
	Modules               map[string]ModuleDef
	Layers                []string
	ModuleMap             ModuleMap
	BCAdvisoryMinSeverity string // minimum severity to emit BC coupling advisories
	// ApprovedLabels pins integration strength per ordered module pair, keyed
	// by from+"\x00"+to (labels.Key). Human-approved enrich output, validated
	// for freshness by the engine before injection. Precedence in classify:
	// config globs > approved labels > extractor hint.
	ApprovedLabels map[string]string
	// Scorer is the coupling scorer applied to each cross-boundary edge.
	// When nil, classify.Run uses coupling.DefaultScorer() (MultiplicativeScorer, locked Task 16).
	Scorer coupling.Scorer
	// CrossModuleClonePairs is the set of canonical module-pair keys
	// ("[a]\x00[b]" with a≤b) that share duplicated code blocks, derived
	// from the clone-detection signal. Used to tag CoA (connascence of
	// algorithm) on cross-module edges. Empty when clone detection is
	// disabled or produced no results.
	CrossModuleClonePairs map[string]struct{}
	// ExplicitOwners marks modules whose `owner:` was hand-authored in YAML.
	// classifyDistance treats explicit ownership as authoritative, so an explicit
	// `owner: same-team` is not overridden by the code-structure fallback even in
	// a single-author (degenerate) repo.
	ExplicitOwners map[string]bool
	// VolatilityCascadeEnabled enables a single-hop volatility propagation pass:
	// a module strongly coupled (strength ≥ functional) to a high-volatility
	// module inherits high effective volatility. Config-declared volatility always
	// takes precedence over the inferred result.
	VolatilityCascadeEnabled bool
}

// RuleConfig is the view passed to the rules stage.
type RuleConfig struct {
	Rules     []RuleDef
	Layers    []string
	ModuleMap ModuleMap
}

// MetricConfig is the per-metric view returned by ForMetric.
type MetricConfig = MetricEntry

// ExceptionSet is the view passed to the status stage.
type ExceptionSet struct {
	Exceptions []ExceptionDef
}

// OutputConfig is the view passed to the output stage.
type OutputConfig struct {
	JSON     bool
	Markdown bool
	SARIF    bool
}

// PatternDef defines a single structural pattern rule for ast-grep.
type PatternDef struct {
	ID   string `yaml:"id"`
	Lang string `yaml:"lang"`
	Rule string `yaml:"rule"`
}

// PatternConfig is the list of pattern definitions passed to a PatternProvider.
type PatternConfig []PatternDef

// StalenessConfig is the view passed to the staleness check stage.
type StalenessConfig struct {
	Enabled   bool
	Threshold time.Duration // zero value defaults to 90*24*time.Hour in Check
	Modules   map[string]ModuleDef
}
