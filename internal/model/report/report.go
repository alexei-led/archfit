// Package report defines the stable data-only contract shared by scoring,
// decision synthesis, persistence, and output adapters.
package report

// RubricVersion is the architect scorecard rubric this contract represents.
// Scorecards are comparable only when their rubric versions match.
const RubricVersion = 1

// ScoreVersion identifies the published Balanced Coupling measurement contract.
// Bump it when scorer ordinals, normalization, or severity mapping changes.
const ScoreVersion = "bc_score.v6"

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

// DecisionBand is the top-level human decision for a report.
type DecisionBand string

// Decision bands combine the scorecard and active gate state.
const (
	DecisionBandFail           DecisionBand = "FAIL"
	DecisionBandNeedsAttention DecisionBand = "NEEDS_ATTENTION"
	DecisionBandHealthy        DecisionBand = "HEALTHY"
	DecisionBandAcceptable     DecisionBand = "ACCEPTABLE_WITH_WATCH_ITEMS"
)

// Report is the human-decision view model formatted by output adapters.
type Report struct {
	Band            DecisionBand
	Headline        string
	Blocking        int
	Advisory        int
	Overall         int
	OverallBand     ScoreBand
	Dimensions      []DimReport
	Recommendations Recommendations
	Delta           *Delta
}

// DimReport is the presentation view of one scorecard dimension.
type DimReport struct {
	Name       string
	Value      int
	Band       ScoreBand
	Confidence Confidence
	RawValue   int
	CapApplied string
	Meta       bool
	Why        string
	WhatMoves  string
}

// Rec is one actionable recommendation.
type Rec struct {
	Title  string
	Detail string
	RuleID string
}

// Recommendations groups findings by urgency tier.
type Recommendations struct {
	MustFix   []Rec
	ShouldFix []Rec
	Watch     []Rec
	Calibrate []Rec
	Ignore    []Rec
}

// Delta holds signed score changes between a base and current scorecard.
type Delta struct {
	Overall    int
	Dimensions []DimDelta
}

// DimDelta is the before, after, and signed change for one dimension.
type DimDelta struct {
	Name   string
	Before int
	After  int
	Change int
}

// Verdict is the top-level pass/fail/warn outcome of an archfit run (spec §12).
type Verdict string

// Verdict constants.
const (
	VerdictPass Verdict = "pass"
	VerdictFail Verdict = "fail"
	VerdictWarn Verdict = "warn"
)

// Direction records whether a rising metric value is an improvement or a
// regression. It is a property of the metric's definition, not a user choice
// (Technical Details, docs/plans/completed/20260702-wave1-gate-integrity.md): the metric
// that produces a MetricResult stamps its own Direction, and computeVerdict
// reads it to interpret Delta's sign instead of assuming ratio semantics for
// every metric.
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
	Delta      *float64  `json:"delta,omitempty"`
	Direction  Direction `json:"direction,omitempty"`
}

// MetricSnapshot is the baseline snapshot of metric values keyed by metric name.
// Stored in the baseline file so delta can be computed on the next run.
type MetricSnapshot map[string]struct {
	Value   float64 `json:"value"`
	Version string  `json:"version"`
}

// Summary holds the gate/warning/exception counts for the top-level summary block (spec §12).
type Summary struct {
	GateFindings int `json:"gate_findings"`
	Warnings     int `json:"warnings"`
	WaiversUsed  int `json:"waivers_used"`
}

// DeltaReport groups a delta run's findings by how they relate to the baseline
// and the changed-file set, so a reviewer can separate what this change
// introduced, resolved, or merely touched from pre-existing issues — instead of
// a delta run reading like a full-repo dump. Each slice holds finding IDs that
// join back to findings[]; buckets are mutually exclusive. Populated in delta
// mode only; the whole block is omitted otherwise (pointer + omitempty).
type DeltaReport struct {
	// New holds findings absent from the baseline (introduced by this change).
	New []string `json:"new,omitempty"`
	// Existing holds baseline findings still present and not on a changed file.
	Existing []string `json:"existing,omitempty"`
	// Resolved holds baseline findings no longer detected (status fixed).
	Resolved []string `json:"resolved,omitempty"`
	// SeverityChanged holds baseline findings whose severity differs from the
	// severity recorded in the baseline.
	SeverityChanged []string `json:"severity_changed,omitempty"`
	// TouchedByDelta holds pre-existing findings on a file this change touched —
	// debt a reviewer is well-placed to clear while already in the file.
	TouchedByDelta []string `json:"touched_by_delta,omitempty"`
}

// ClassifiedEdgeSummary holds aggregate distribution counts over the
// coupling.Index produced by classify.Run plus score-bearing clone-only
// duplicated-knowledge pairs when that policy is enabled. Stdlib-only (no
// coupling imports). Populated in engine.go; consumed by score.go to drive
// coupling_balance.
type ClassifiedEdgeSummary struct {
	// Total is the total classified coupling fact count (all graph edges,
	// including same_module, plus scored clone-only pairs when enabled).
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
	// selected their distance rung: code_structure, ownership, deploy_unit, or
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
	// DistributedMonolith counts the genuine distributed-monolith edges: those in
	// the critical band AND at high distance (different owner or deploy unit). The
	// critical band alone is NOT distributed-monolith — a critical edge at
	// cross_module_same_owner is local coupling; inspect its strength and volatility
	// drivers separately. Only this count may be framed as "distributed-monolith
	// risk"; it never changes the balance value.
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
	// CloneOnlyScored counts clone-only duplicated-knowledge pairs included in
	// coupling_balance by coupling.duplicated_knowledge: score. They are
	// score-bearing coupling facts, not graph import edges.
	CloneOnlyScored int `json:"clone_only_scored,omitempty"`
	// CloneOnlyAdvisory counts clone-only duplicated-knowledge pairs held out of
	// coupling_balance by coupling.duplicated_knowledge: advisory. They may still
	// emit bc/duplicated_knowledge advisories after severity/status filtering.
	CloneOnlyAdvisory int `json:"clone_only_advisory,omitempty"`
	// LLMApproved is the count of approved cross-boundary labels whose provenance
	// is "llm" and confidence is not "high". These lower the coupling_balance
	// dimension confidence by one band — they are human-approved but not human-judged.
	// Zero means no LLM-provenance labels are in effect.
	LLMApproved int `json:"llm_approved,omitempty"`
	// LabeledLLM is the count of cross-boundary EDGES whose strength came from
	// an approved llm-provenance label filling a cell every static source left
	// unknown (one label covers all edges of its module pair). It attributes
	// the scored-fraction increase to the semantic layer — disclosure only,
	// never fed into the balance value.
	LabeledLLM int `json:"labeled_llm,omitempty"`
	// LLMLowConfidenceEdges counts cross-boundary edges whose strength was filled
	// by an applied non-high-confidence LLM label. It is intentionally not emitted:
	// score confidence consumes it while labeled_llm remains the user-facing
	// attribution bucket.
	LLMLowConfidenceEdges int `json:"-"`
	// VolatilityProvenance counts MODULES (not edges) by the source of their
	// volatility. Nil when no modules were resolved.
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
	WorstBalance              int `json:"worst_balance"`
	LowerDecileBalance        int `json:"lower_decile_balance"`
	HighOrWorseEdges          int `json:"high_or_worse_edges"`
	HighOrWorseSharePct       int `json:"high_or_worse_share_pct"`
	CriticalEdges             int `json:"critical_edges"`
	DistributedMonolithEdges  int `json:"distributed_monolith_edges"`
	CloneOnlyScored           int `json:"clone_only_scored,omitempty"`
	CloneOnlyHighOrWorseEdges int `json:"clone_only_high_or_worse_edges,omitempty"`
	CloneOnlyWorstBalance     int `json:"clone_only_worst_balance,omitempty"`
}

// DistanceCompressionSummary records archfit's deterministic distance-ladder
// coverage. It makes compressed Ch8 middle rungs visible in JSON/Markdown so a
// D=4/D=7 result is not mistaken for full book precision.
type DistanceCompressionSummary struct {
	CompressedMiddleRungs       bool                        `json:"compressed_middle_rungs"`
	ImplementedRungs            []int                       `json:"implemented_rungs,omitempty"`
	OmittedRungs                []int                       `json:"omitted_rungs,omitempty"`
	OmittedRungReasons          []DistanceOmittedRungReason `json:"omitted_rung_reasons,omitempty"`
	DeterministicSplits         []string                    `json:"deterministic_splits,omitempty"`
	CodeStructureBoundaryCounts []DistanceCount             `json:"code_structure_boundary_counts,omitempty"`
	CodeStructureAncestorDepths []DistanceCount             `json:"code_structure_ancestor_depths,omitempty"`
	Rationale                   string                      `json:"rationale,omitempty"`
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

// GitFindingDelta records where each of the current run's repair tasks came
// from relative to a git base ref (`--base`). It answers one question for a
// coding agent: "which of these tasks did my change introduce?" — without
// re-deriving anything from the base tree.
//
// It is report-only: the verdict, the exit code, and every non-JSON renderer
// are unchanged by this block. The classification is deliberately conservative
// — a task lands in IntroducedFindingIDs only when the base run's
// finding-producing analyzers covered the same ground as the current run.
// Missing, partial, or asymmetric analyzer evidence moves the task to
// UnknownOriginFindingIDs instead of inventing a "new" task.
//
// Only finding IDs, coverage, and the config hash cross over from the base
// sub-run. Base paths, locations, and validation commands never do: the base
// side is scored inside a temporary worktree that is deleted before this block
// is read.
type GitFindingDelta struct {
	// BaseRef is the git ref the current run was compared against.
	BaseRef string `json:"base_ref"`
	// ComparisonStatus is GitComparisonComparable when every task has a definite
	// origin, GitComparisonUnknown when at least one does not.
	ComparisonStatus string `json:"comparison_status"`
	// IntroducedFindingIDs are current repair tasks with no matching base
	// finding, established against comparable analyzer evidence on both sides.
	IntroducedFindingIDs []string `json:"introduced_finding_ids"`
	// PreExistingFindingIDs are current repair tasks whose finding ID was also
	// observed on the base ref (lifecycle labels and gate/advisory promotion are
	// ignored — only the stable ID matters).
	PreExistingFindingIDs []string `json:"pre_existing_finding_ids"`
	// UnknownOriginFindingIDs are current repair tasks whose origin could not be
	// established: incomplete analyzer evidence, a config-hash mismatch, or a
	// synthetic per-run task that has no stable base counterpart.
	UnknownOriginFindingIDs []string `json:"unknown_origin_finding_ids"`
	// ComparisonReasons names each analyzer family whose evidence was not
	// comparable, one sorted entry per family. Empty when nothing blocked the
	// comparison.
	ComparisonReasons []string `json:"comparison_reasons"`
}
