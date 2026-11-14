// Package report defines the stable data-only contract shared by scoring,
// decision synthesis, persistence, and output adapters.
package report

// RubricVersion is the architect scorecard rubric this contract represents.
// Scorecards are comparable only when their rubric versions match.
const RubricVersion = 1

// ScoreVersion identifies the published Balanced Coupling measurement contract.
// Bump it when scorer ordinals, normalization, or severity mapping changes.
const ScoreVersion = "bc_score.v7"

// ScoreBand is a qualitative label for a 0-100 dimension value.
type ScoreBand string

// ScoreBandCritical through ScoreBandStrong define measured score bands.
const (
	ScoreBandCritical    ScoreBand = "critical"
	ScoreBandPoor        ScoreBand = "poor"
	ScoreBandMixed       ScoreBand = "mixed"
	ScoreBandServiceable ScoreBand = "serviceable"
	ScoreBandStrong      ScoreBand = "strong"
	// ScoreBandNA marks a dimension that could not be measured.
	ScoreBandNA ScoreBand = "n/a"
)

// Unmeasured reports whether b marks an unmeasured dimension.
func (b ScoreBand) Unmeasured() bool { return b == ScoreBandNA }

// Confidence describes how trustworthy a dimension assessment is.
type Confidence string

// ConfidenceLow through ConfidenceHigh define evidence confidence levels.
const (
	ConfidenceLow    Confidence = "low"
	ConfidenceMedium Confidence = "medium"
	ConfidenceHigh   Confidence = "high"
)

// DimCouplingBalance names the coupling score dimension.
const (
	DimCouplingBalance = "coupling_balance"
)

// Dimension is one scored axis of the architecture.
type Dimension struct {
	Name       string     `json:"name"`
	Value      int        `json:"value"`
	Band       ScoreBand  `json:"band"`
	Confidence Confidence `json:"confidence"`
	Evidence   []string   `json:"evidence"`
	Summary    string     `json:"summary"`
	RawValue   int        `json:"raw_value,omitempty"`
	CapApplied string     `json:"cap_applied,omitempty"`
	// Meta marks a review-process dimension.
	Meta bool `json:"meta,omitempty"`
}

// Scorecard is the synthesised banded assessment across all dimensions.
type Scorecard struct {
	RubricVersion int         `json:"rubric_version"`
	Overall       int         `json:"overall"`
	OverallBand   ScoreBand   `json:"overall_band"`
	Dimensions    []Dimension `json:"dimensions"`
}

// Direction records whether a rising metric value is an improvement or a
// regression. It is a property of the metric's definition, not a user choice
// (Technical Details, docs/archived/plans/20260702-wave1-gate-integrity.md): the metric
// that produces a MetricResult stamps its own Direction for report readers.
type Direction string

// Direction constants.
const (
	DirectionHigherIsBetter Direction = "higher_is_better"
	DirectionHigherIsWorse  Direction = "higher_is_worse"
)

// MetricResult holds the computed value and metadata for a single metric (spec §10).
// JSON tags match spec §10 field names exactly.
type MetricResult struct {
	Name       string    `json:"name"`
	Value      float64   `json:"value"`
	Display    string    `json:"display"`
	Band       string    `json:"band"`
	Confidence string    `json:"confidence"`
	Version    string    `json:"metric_version"`
	Mode       string    `json:"mode"`
	Definition string    `json:"definition"`
	Direction  Direction `json:"direction,omitempty"`
}

// Summary counts report diagnostic warnings.
type Summary struct {
	Warnings int `json:"warnings"`
}

// ClassifiedEdgeSummary holds aggregate distribution counts over the
// coupling.Index produced by classify.Run. Stdlib-only (no
// coupling imports). Populated in engine.go; consumed by score.go to drive
// coupling_balance.
type ClassifiedEdgeSummary struct {
	// Total is the total classified coupling fact count (all graph edges,
	// including same_module).
	Total int `json:"total"`
	// Scored is the count of cross-boundary edges with a concrete book balance
	// (Scored=true on EdgeScore, i.e. strength and distance both known).
	Scored int `json:"scored"`
	// Abstained is the count of cross-boundary edges where the scorer abstained
	// (strength or distance unknown — excluded from MeanBalance).
	Abstained int `json:"abstained"`
	// SameModule is the count of same_module edges (excluded from the balance aggregate).
	SameModule int `json:"same_module"`
	// MeanBalance is the arithmetic mean of the book balance (1..10) over scored
	// cross-boundary edges. 0.0 when Scored == 0.
	MeanBalance float64 `json:"mean_balance"`
	// TailRisk summarizes the lower tail of scored cross-boundary coupling facts.
	// It sits beside MeanBalance so a healthy average cannot hide a concentrated
	// set of high/critical edges. Nil when no scored cross-boundary facts exist.
	TailRisk *CouplingTailRiskSummary `json:"tail_risk,omitempty"`
	// ByStrength counts cross-boundary edges by strength label (string keys, coupling package values).
	ByStrength map[string]int `json:"by_strength,omitempty"`
	// ByDistance counts cross-boundary edges by distance label.
	ByDistance map[string]int `json:"by_distance,omitempty"`
	// ByDistanceBasis counts cross-boundary edges by the deterministic signal that
	// named their distance token: module_boundary, ownership, deploy_unit, or
	// declared_external. Unknown/same-module edges have no basis and are omitted.
	ByDistanceBasis map[string]int `json:"by_distance_basis,omitempty"`
	// ByVolatility counts cross-boundary edges by volatility label.
	ByVolatility map[string]int `json:"by_volatility,omitempty"`
	// BySeverity counts cross-boundary edges by score band (severity label).
	BySeverity map[string]int `json:"by_severity,omitempty"`
	// ByBalanceDriver counts the formula term that determined each scored edge.
	ByBalanceDriver map[string]int `json:"by_balance_driver,omitempty"`
	// ByCriticalDriver counts the formula term that determined each critical edge.
	ByCriticalDriver map[string]int `json:"by_critical_driver,omitempty"`
	// ByModulePair counts scored edges by resolved source and target module.
	ByModulePair map[string]int `json:"by_module_pair,omitempty"`
	// DistributedMonolith counts critical edges across a module boundary. Under
	// bc_score.v7 every boundary is D=9, so this is the transitional count that the
	// strict v7 qualification (strong strength, declared volatility, no cohesive
	// role) replaces. It never changes the balance value.
	DistributedMonolith int `json:"distributed_monolith,omitempty"`
	// External is the count of cross-boundary edges whose target is NOT a declared
	// module (Distance == unknown: stdlib, third-party, undeclared packages). These
	// are EXCLUDED from the Scored/Abstained distribution that drives coupling_balance
	// — the book measures coupling among YOUR components, not your libraries.
	// External dependency hygiene is tracked separately and does not affect coupling_balance.
	// This field is language-agnostic: it keys on DistanceUnknown, which classifyDistance
	// sets for all languages (Go stdlib/3p, Rust dependency crates, TS node_modules,
	// Python external imports). Zero means no external edges were detected.
	External int `json:"external,omitempty"`
	// DeclaredExternal is the count of edges whose target matched a config-declared
	// `external_systems:` entry (Distance == declared_external, D=10 — book Ch10
	// Example 1). Unlike External, these edges ENTER the Scored/Abstained
	// distribution: the architect declared the seam, so it is measured. The count
	// keeps the disclosed-exclusion arithmetic honest — External covers only the
	// UNDECLARED remainder.
	DeclaredExternal int `json:"declared_external,omitempty"`
	// ConnectedModules is the number of distinct first-party modules participating
	// in scored/abstained cross-boundary coupling facts. It feeds confidence only:
	// a tiny connected module sample cannot claim high-confidence architecture health.
	ConnectedModules int `json:"connected_modules,omitempty"`
	// VolatilityProvenance counts modules by the source of their volatility.
	VolatilityProvenance *VolatilityProvenance `json:"volatility_provenance,omitempty"`
	// DistanceCompression discloses which book distance rungs this deterministic
	// instrumentation implements and which middle rungs remain compressed instead
	// of being guessed. Report-only; never consumed by scoring or gates.
	DistanceCompression *DistanceCompressionSummary `json:"distance_compression,omitempty"`
}

// CouplingTailRiskSummary records lower-tail statistics over scored
// cross-boundary coupling facts. Lower book balance is worse, so WorstBalance
// and LowerDecileBalance expose concentrated hot spots that MeanBalance can hide.
type CouplingTailRiskSummary struct {
	WorstBalance             int `json:"worst_balance"`
	LowerDecileBalance       int `json:"lower_decile_balance"`
	HighOrWorseEdges         int `json:"high_or_worse_edges"`
	HighOrWorseSharePct      int `json:"high_or_worse_share_pct"`
	CriticalEdges            int `json:"critical_edges"`
	DistributedMonolithEdges int `json:"distributed_monolith_edges"`
}

// DistanceCompressionSummary records archfit's deterministic distance-ladder
// coverage. It makes compressed Ch8 middle rungs visible in JSON/Markdown so a
// D=2/D=9/D=10 ladder is not mistaken for full book precision.
type DistanceCompressionSummary struct {
	CompressedMiddleRungs     bool                        `json:"compressed_middle_rungs"`
	ImplementedRungs          []int                       `json:"implemented_rungs,omitempty"`
	OmittedRungs              []int                       `json:"omitted_rungs,omitempty"`
	OmittedRungReasons        []DistanceOmittedRungReason `json:"omitted_rung_reasons,omitempty"`
	DeterministicSplits       []string                    `json:"deterministic_splits,omitempty"`
	ContainmentBoundaryCounts []DistanceCount             `json:"containment_boundary_counts,omitempty"`
	ContainmentAncestorDepths []DistanceCount             `json:"containment_ancestor_depths,omitempty"`
	Rationale                 string                      `json:"rationale,omitempty"`
}

// DistanceCount is one deterministic distance-evidence histogram bucket.
type DistanceCount struct {
	Value int `json:"value"`
	Count int `json:"count"`
}

// DistanceOmittedRungReason explains why a book distance rung remains compressed.
type DistanceOmittedRungReason struct {
	Rung   int    `json:"rung"`
	Reason string `json:"reason"`
}

// VolatilityProvenance counts modules by where their volatility came from:
// config-declared (an explicit `volatility:` field or subdomain mapping),
// inherited by an auto-registered synthetic module from its nearest declared
// ancestor, or raised by the opt-in volatility cascade (an overlay on top of
// the base source). Undeclared is the honest remainder — no volatility from
// any source. Disclosure-only: a repo whose edges all carry the same
// volatility must be visibly uniform-by-inheritance (one declared ancestor
// fanned out to N synthetic submodules), not mistaken for N measured
// judgments. Volatility comes from the domain (book Ch9), never from commit
// history, so archfit never derives differentiation — it discloses where the
// labels came from. Never consumed by the balance value or the gate.
type VolatilityProvenance struct {
	// Declared counts config-declared modules whose volatility comes from their
	// own `volatility:` field or subdomain mapping.
	Declared int `json:"declared"`
	// Inherited counts synthetic (auto-registered) modules whose volatility was
	// inherited from the nearest config-declared ancestor.
	Inherited int `json:"inherited"`
	// Cascade counts modules whose EFFECTIVE volatility the opt-in cascade pass
	// raised above their base level. Overlays Declared/Inherited/Undeclared.
	Cascade int `json:"cascade"`
	// Undeclared counts modules with no volatility from any source (scored
	// conservatively by the book scorer, never guessed).
	Undeclared int `json:"undeclared"`
}
