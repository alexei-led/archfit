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
// (count metrics) is the allowed increase. Both default to 0 when unset (any
// worsening move trips). validate() rejects a knob on a metric of the wrong
// kind — pointers so a zero-valued wrong-kind knob (e.g. `max_new: 0` on a
// ratio metric) is still seen as present and rejected, not silently inert.
type MetricEntry struct {
	// Metrics run by default: a knob-only entry (e.g. only `gate: warn`)
	// stays enabled, and only an explicit `enabled: false` disables the
	// metric. (Pointer so absent and false decode differently.)
	Enabled  *bool    `yaml:"enabled"`
	Gate     string   `yaml:"gate"`
	MinDelta *float64 `yaml:"min_delta"`
	MaxNew   *int     `yaml:"max_new"`
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

// DuplicatedKnowledgePolicy controls whether clone-only duplicated knowledge
// enters the headline coupling_balance score or remains advisory-only.
type DuplicatedKnowledgePolicy string

const (
	// DuplicatedKnowledgePolicyScore includes clone-only cross-module pairs in
	// coupling_balance as symmetric-strength coupling facts. This is the default
	// v5 policy: the book's Ch7 duplicated-knowledge case affects the flagship score.
	DuplicatedKnowledgePolicyScore DuplicatedKnowledgePolicy = "score"
	// DuplicatedKnowledgePolicyAdvisory preserves the v4 behavior: clone-only
	// pairs emit bc/duplicated_knowledge advisories but stay out of coupling_balance.
	DuplicatedKnowledgePolicyAdvisory DuplicatedKnowledgePolicy = "advisory"
)

// NormalizeDuplicatedKnowledgePolicy applies the default policy for omitted YAML
// and direct ClassifyConfig literals. Empty means score.
func NormalizeDuplicatedKnowledgePolicy(p DuplicatedKnowledgePolicy) DuplicatedKnowledgePolicy {
	if p == "" {
		return DuplicatedKnowledgePolicyScore
	}
	return p
}

// CouplingConfig tunes the Balanced-Coupling advisory pass (`coupling:`).
type CouplingConfig struct {
	// MinSeverity is the minimum severity for a coupling advisory to appear:
	// low | medium | high | critical (default medium). low is noisy on
	// well-designed codebases; high/critical surface only intrusive/functional
	// coupling across large boundaries.
	MinSeverity string `yaml:"min_severity,omitempty"`
	// DuplicatedKnowledge controls clone-only duplicated knowledge (cross-module
	// clone pairs with no import edge). "score" (default) includes those pairs in
	// coupling_balance; "advisory" preserves the v4 report-only behavior.
	DuplicatedKnowledge DuplicatedKnowledgePolicy `yaml:"duplicated_knowledge,omitempty"`
	// VolatilityCascade enables the book Ch9 propagation pass: a module strongly
	// coupled to a high-effective-volatility module inherits raised effective
	// volatility. The pass runs to a deterministic fixpoint and never lowers values.
	VolatilityCascade bool `yaml:"volatility_cascade,omitempty"`
	// Gate makes the synthesised coupling_balance score gate the verdict.
	// Absent (nil) = coupling stays advisory, today's behavior. An unmeasured
	// score (band n/a) never trips the gate regardless of these knobs.
	Gate *CouplingGateDef `yaml:"gate,omitempty"`
}

// ExternalSystemDef declares one external integration seam (`external_systems:`),
// per book Ch10 Example 1 (cross-vendor integration). Edges whose target matches
// a Targets glob enter coupling_balance scoring at the distance ladder's far end
// (declared_external, D=10) instead of the disclosed external exclusion.
// Scoring EVERY library import at D=10 would flood the metric with vendor noise —
// only a declared seam (a vendor SDK, a generated client) is an integration the
// architect chose to measure.
type ExternalSystemDef struct {
	// Targets are glob patterns matched against the classified edge target —
	// the same node identity the language extractor emits: a Go import path
	// ("github.com/aws/aws-sdk-go-v2/**"), a TS resolved package path
	// ("node_modules/@aws-sdk/**") or bare specifier, a Python dotted module
	// ("boto3.**"), or a Rust crate name ("aws_sdk_s3").
	Targets []string `yaml:"targets"`
	// Volatility of the external system: high | medium | low | frozen.
	// Default low — the book's generic-subdomain guidance (an external vendor
	// system is a generic capability, presumed stable unless declared otherwise).
	Volatility string `yaml:"volatility,omitempty"`
}

// CouplingGateDef configures the coupling_balance verdict gate
// (`coupling.gate:`). At least one knob must be set; validate() rejects an
// empty block.
type CouplingGateDef struct {
	// MinBand is the band floor: poor | mixed | serviceable | strong. The
	// verdict fails when the current coupling_balance band ranks below it.
	// critical is rejected — no band ranks below it, so it could never trip.
	MinBand string `yaml:"min_band,omitempty"`
	// MaxDrop is the tolerated point drop of the coupling_balance value against
	// the score stored in .archfit-baseline.json. Unset = no drop check;
	// 0 = any drop fails. Skipped when the baseline carries no stored score.
	MaxDrop *int `yaml:"max_drop,omitempty"`
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
	// by from+"\x00"+to (labels.Key). Human-approved enrich output with
	// human/tool provenance, validated for freshness by the engine before
	// injection. Precedence in classify: config globs > approved labels >
	// extractor hint.
	ApprovedLabels map[string]string
	// LLMLabels pins integration strength for approved labels whose judgment
	// came from an LLM (provenance: llm), same keying as ApprovedLabels.
	// Weaker precedence: an llm label only fills a cell every static source
	// left unknown (no config glob, no extractor/type-info/SCIP hint) — it
	// never displaces a static classification (compiler-grade beats LLM, the
	// same rule as SCIP-for-Go).
	LLMLabels map[string]string
	// LLMLabelConfidence records the confidence value for entries in LLMLabels,
	// keyed the same way. Missing or non-"high" values are treated as uncertain
	// when an LLM label actually fills an edge.
	LLMLabelConfidence map[string]string
	// Scorer is the coupling scorer applied to each cross-boundary edge.
	// When nil, classify.Run uses coupling.DefaultScorer() (MultiplicativeScorer, locked Task 16).
	Scorer coupling.Scorer
	// CrossModuleClonePairs is the set of canonical module-pair keys
	// ("[a]\x00[b]" with a≤b) that share duplicated code blocks, derived
	// from the clone-detection signal. Consumed by the Symmetric-strength
	// upgrade, the volatility-cascade clone exclusion, and duplicated-knowledge
	// pairing. Empty when clone detection is disabled or produced no results.
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
	// VolatilityCascadeEnabled enables inferred volatility propagation: a module
	// strongly coupled (strength ≥ functional) to a high-effective-volatility
	// module inherits high effective volatility. The cascade runs to a deterministic
	// fixpoint and only raises volatility; it never lowers configured values.
	VolatilityCascadeEnabled bool
	// ExternalSystems are the declared external integration seams
	// (`external_systems:`). An edge whose target resolves to no module but
	// matches an entry's target glob classifies at DistanceExternal (D=10)
	// with the entry's volatility (default low) and enters scoring.
	ExternalSystems map[string]ExternalSystemDef
	// DuplicatedKnowledgePolicy controls whether clone-only duplicated knowledge
	// is score-bearing or advisory-only.
	DuplicatedKnowledgePolicy DuplicatedKnowledgePolicy
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
