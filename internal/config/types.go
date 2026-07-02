package config

import (
	"time"

	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/graph"
)

// MetricEntry holds the settings for a single metric inside the metrics map.
// Gate sets what a baseline regression does to the verdict: off skips the
// check, warn caps at WARN, fail/unset blocks — the rule-gate convention.
// MinDelta (ratio metrics) is the tolerated drop below the baseline; MaxNew
// (count metrics) is the allowed increase. Both default to 0 (any worsening
// move trips). validate() rejects a knob on a metric of the wrong kind.
type MetricEntry struct {
	Enabled  bool    `yaml:"enabled"`
	Gate     string  `yaml:"gate"`
	MinDelta float64 `yaml:"min_delta"`
	MaxNew   int     `yaml:"max_new"`
}

// MetricsConfig holds settings for all metrics, keyed by metric name.
type MetricsConfig map[string]MetricEntry

// ModuleReviewConfig configures staleness gating of the module declarations:
// archfit warns (or fails) when a module's `reviewed_at` is older than
// stale_after, nudging a periodic re-check of the architecture map.
type ModuleReviewConfig struct {
	StaleAfter string `yaml:"stale_after,omitempty"`
	Gate       string `yaml:"gate,omitempty"`
}

// CouplingConfig tunes the Balanced-Coupling advisory pass (`coupling:`).
type CouplingConfig struct {
	// MinSeverity is the minimum severity for a coupling advisory to appear:
	// low | medium | high | critical (default medium). low is noisy on
	// well-designed codebases; high/critical surface only intrusive/functional
	// coupling across large boundaries.
	MinSeverity string `yaml:"min_severity,omitempty"`
	// VolatilityCascade enables the book Ch9 single-hop propagation pass: a
	// module strongly coupled to a high-volatility module inherits raised
	// effective volatility. Config-declared volatility always takes precedence.
	VolatilityCascade bool `yaml:"volatility_cascade,omitempty"`
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
	// Root is the absolute path of the analysis boundary (ScanRoot). When
	// non-empty it overrides the git toplevel so archfit scans a subdirectory
	// of a monorepo. Empty means "use the git toplevel (or WorkDir as a
	// last resort)". Callers (cmd) are responsible for making this absolute.
	Root       string
	Base       string // git ref to diff against (empty = none)
	Full       bool   // if true, full-repo mode (no diff)
	Exclusions []string
	WorkDir    string // working directory for git commands; empty = process cwd
}

// ExtractConfig is the view passed to a language extractor.
// Built via ForExtract(lang); holds only what the extractor needs.
type ExtractConfig struct {
	// Common fields.
	Src        string   // scan-root for extractors that need one (TS); always "." — never derived from Modules paths, which are classification globs, not filesystem dirs
	Paths      []string // all module paths
	Exclusions []string
	Internal   []string // all internal globs across modules
	Mode       ToolMode // derived from Tools map for the given language/tool

	// Go-specific.
	BuildFlags      []string // extra build flags passed to go/packages (e.g. ["-tags", "extractortest"])
	GoModuleInclude []string // tools.go.modules.include: keep only members matching these ScanRoot-relative globs
	GoModuleExclude []string // tools.go.modules.exclude: drop members matching these ScanRoot-relative globs

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
	// CloneEvidence maps each canonical module-pair key (same keying as
	// CrossModuleClonePairs) to the real duplicated-code locations — both sides,
	// as reported by the clone detector — that produced the pairing. classify
	// attaches these onto a Classification when it performs the Symmetric-strength
	// upgrade for that pair, so the downstream finding can cite the actual
	// duplicated file:line instead of only the edge's baseline provenance (e.g. a
	// Rust crate's Cargo.toml:0). Empty when clone detection produced no
	// line-location data, or is disabled.
	CloneEvidence map[string][]graph.Location
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

// WaiverSet is the view passed to the status stage: the approved rule deviations
// (`waivers:`) that suppress matching gate findings until they expire.
type WaiverSet struct {
	Waivers []WaiverDef
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

// FileClassDef holds optional user-supplied patterns for source-file
// classification. Auto-detection (generated-header sniff, naming conventions,
// language test patterns) runs first; these fields extend or fine-tune it for
// custom mock frameworks and project-specific conventions.
type FileClassDef struct {
	// GeneratedGlobs are glob patterns (matched against the repo-relative
	// slash path) that classify a file as Generated. Supports doublestar (`**`)
	// semantics (github.com/bmatcuk/doublestar); `**` matches across path
	// separators. Example: ["**/generated/**", "*.pb.go"]
	GeneratedGlobs []string `yaml:"generated_globs,omitempty"`
	// TestGlobs are glob patterns that classify a file as Test.
	// Example: ["*_helpers_test.go", "testutil/**"]
	TestGlobs []string `yaml:"test_globs,omitempty"`
	// MockFrameworks contains filename prefix/suffix patterns (matched against
	// the base filename only) that identify mock files as Generated. Used for
	// custom mock code-generators not covered by built-in naming conventions.
	// Example: ["fake_", "_double"]
	MockFrameworks []string `yaml:"mock_frameworks,omitempty"`
}

// StalenessConfig is the view passed to the staleness check stage.
type StalenessConfig struct {
	Enabled   bool
	Threshold time.Duration // zero value defaults to 90*24*time.Hour in Check
	Modules   map[string]ModuleDef
}
