// Package state owns the architecture-state contract as assessment sees it:
// the nine dimension envelopes, the explicit finding classification, and the
// deterministic healthy/needs_attention/blocked decision.
//
// It is deliberately dependency-free. Assessment must not import the external
// report DTOs (`assessment_no_report_dtos`), so this package mirrors the wire
// contract the same way `result` mirrors `report.Document`; the application
// projects one onto the other. It must also not import `internal/assessment/
// score` (`assessment_no_score_decision`): the architecture decision is made
// from explicit gate results and finding classifications, never from an
// averaged repository scalar.
package state

// Verdict is the primary architecture decision. No averaged score may produce
// or move it.
type Verdict string

// Architecture verdicts.
const (
	// Healthy requires every dimension measured, every hard gate passing, and
	// no active diagnostic. It is never produced by treating an unsupported
	// collector as zero.
	Healthy Verdict = "healthy"
	// NeedsAttention means no blocker, but at least one active diagnostic
	// exists or at least one dimension is partial/unmeasured.
	NeedsAttention Verdict = "needs_attention"
	// Blocked means at least one active hard-gate finding, or a reportable
	// required-tool policy failure.
	Blocked Verdict = "blocked"
)

// MeasurementStatus records whether a dimension's evidence was actually
// gathered. Missing evidence is explicit, never an implicit green result.
type MeasurementStatus string

// Measurement statuses for one dimension envelope.
const (
	Measured   MeasurementStatus = "measured"
	Partial    MeasurementStatus = "partial"
	Unmeasured MeasurementStatus = "unmeasured"
)

// Confidence is a dimension's confidence in its own evidence.
type Confidence string

// Confidence rungs. Unrated is the abstain rung: the evidence supports no
// confidence claim at all.
const (
	ConfidenceHigh    Confidence = "high"
	ConfidenceMedium  Confidence = "medium"
	ConfidenceLow     Confidence = "low"
	ConfidenceUnrated Confidence = "unrated"
)

// GateState is one dimension's gate posture. It is a different enum from
// HardGateState on purpose: a dimension may warn, the repository decision may
// not.
type GateState string

// Per-dimension gate postures.
const (
	GatePass          GateState = "pass"
	GateWarn          GateState = "warn"
	GateFail          GateState = "fail"
	GateNotApplicable GateState = "not_applicable"
)

// HardGateState is the repository-level hard-gate result. Only a named hard
// rule or a required-tool policy failure can set it to fail.
type HardGateState string

// Repository hard-gate results.
const (
	HardGatePass       HardGateState = "pass"
	HardGateFail       HardGateState = "fail"
	HardGateUnmeasured HardGateState = "unmeasured"
)

// ComparisonStatus records whether this run may be compared numerically against
// a reference. Comparison is strict: any config/model/labels/rubric mismatch is
// non_comparable, never a numerical delta.
type ComparisonStatus string

// Comparison statuses.
const (
	ComparisonComparable    ComparisonStatus = "comparable"
	ComparisonNonComparable ComparisonStatus = "non_comparable"
	ComparisonNotRequested  ComparisonStatus = "not_requested"
)

// DimensionCount is the fixed number of architecture-state dimensions. Coverage
// counts must sum to it.
const DimensionCount = 9

// Dimension names. They are the envelope's Name field and the report's JSON
// keys.
const (
	DimensionIntent         = "intent"
	DimensionStructure      = "structure"
	DimensionModularity     = "modularity"
	DimensionCoupling       = "coupling"
	DimensionChangeLocality = "change_locality"
	DimensionComplexity     = "complexity"
	DimensionTestability    = "testability"
	DimensionOperations     = "operations"
	DimensionDrift          = "drift"
)

// Evidence owners for each dimension. Every dimension names the capability that
// produces its facts; an envelope with no owner cannot be measured by anyone.
const (
	OwnerIntent         = "policy+assessment/evaluation"
	OwnerStructure      = "relationship/facts"
	OwnerModularity     = "assessment/metrics"
	OwnerCoupling       = "relationship/analysis"
	OwnerChangeLocality = "history/git"
	OwnerComplexity     = "syntax+evidence/acquisition"
	OwnerTestability    = "syntax/fileclass"
	OwnerOperations     = "policy+evidence/acquisition"
	OwnerDrift          = "assessment/decision"
)

// MetricDenominator is the observed-over-total coverage of one metric. It makes
// a zero value distinguishable from an unmeasured one.
type MetricDenominator struct {
	Observed int
	Total    int
}

// MetricValue is one typed metric record. A dimension's metrics are always a
// slice of these, never a free-form map: the envelope is a reporting contract,
// not a property bag.
type MetricValue struct {
	Name        string
	Value       float64
	Unit        string
	Denominator *MetricDenominator
	Provenance  []string
}

// FindingRef points a dimension envelope at a finding the diagnostic already
// owns. Findings are never duplicated per dimension, so identity, status, and
// ordering keep exactly one owner.
type FindingRef struct {
	ID       string
	RuleID   string
	Kind     string
	Severity string
	Status   string
}

// UnknownFact names one thing a dimension could not observe, why, and which
// capability would have to observe it. It is how partial/unmeasured states stay
// honest instead of silently reading as zero.
type UnknownFact struct {
	Fact   string
	Reason string
	Owner  string
}

// MetricDelta is a signed change in one dimension metric against a comparable
// reference.
type MetricDelta struct {
	Name   string
	Before float64
	After  float64
	Change float64
}

// Delta is one dimension's change against the comparison reference. A
// non-comparable reference yields reasons and no numbers.
type Delta struct {
	Status           ComparisonStatus
	Reasons          []string
	Metrics          []MetricDelta
	NewFindings      []string
	ResolvedFindings []string
}

// Coverage is a dimension's denominator: what was counted, how much of it was
// observed, and out of how much.
type Coverage struct {
	Basis    string
	Observed int
	Total    int
}

// Dimension is the one common envelope every architecture-state dimension uses.
type Dimension struct {
	Name       string
	Owner      string
	Status     MeasurementStatus
	Confidence Confidence
	Gate       GateState
	Coverage   Coverage
	Metrics    []MetricValue
	Findings   []FindingRef
	Unknown    []UnknownFact
	Delta      *Delta
}

// Dimensions holds the nine envelopes as named fields rather than a map: each
// dimension has a compile-time name and owner, and the contract order is fixed
// by declaration order instead of by map iteration.
type Dimensions struct {
	Intent         Dimension
	Structure      Dimension
	Modularity     Dimension
	Coupling       Dimension
	ChangeLocality Dimension
	Complexity     Dimension
	Testability    Dimension
	Operations     Dimension
	Drift          Dimension
}

// All returns the nine envelopes in their fixed contract order.
func (d Dimensions) All() []Dimension {
	return []Dimension{
		d.Intent, d.Structure, d.Modularity, d.Coupling, d.ChangeLocality,
		d.Complexity, d.Testability, d.Operations, d.Drift,
	}
}

// Each returns pointers to the nine envelopes in contract order so collectors
// can fill them in place without repeating the field list.
func (d *Dimensions) Each() []*Dimension {
	return []*Dimension{
		&d.Intent, &d.Structure, &d.Modularity, &d.Coupling, &d.ChangeLocality,
		&d.Complexity, &d.Testability, &d.Operations, &d.Drift,
	}
}

// CountStatuses tallies the nine envelopes by measurement status. It is purely
// mechanical — no policy, no thresholds — so the coverage block and the
// decision counters cannot disagree about what was measured.
func (d Dimensions) CountStatuses() (measured, partial, unmeasured int) {
	for _, dim := range d.All() {
		switch dim.Status {
		case Measured:
			measured++
		case Partial:
			partial++
		default:
			unmeasured++
		}
	}
	return measured, partial, unmeasured
}

// Decision holds the explicit inputs to the verdict. The aggregator reads only
// these classifications plus dimension status; it may never inspect a
// dimension's metrics or derive an implicit threshold from them.
type Decision struct {
	HardGates           HardGateState
	ActiveBlockers      int
	AttentionDimensions int
	UnknownDimensions   int
}

// Architecture is the assessment-owned architecture state. It has no
// repository-level scalar by construction.
//
// Blockers and Diagnostics are the two finding populations the decision reads.
// Both hold references into the diagnostic's own findings slice, which stays
// the single owner of finding identity, status, and order.
type Architecture struct {
	Verdict    Verdict
	Decision   Decision
	Dimensions Dimensions
	// Blockers are active hard-gate findings: kind gate, status new or
	// expired_waiver. A baseline-accepted or waived gate finding is not one.
	Blockers []FindingRef
	// Diagnostics are active advisories: kind advisory, status new or
	// expired_waiver. Accepted and waived advisories are excluded — they were
	// decided already and must not re-open the verdict.
	Diagnostics []FindingRef
	// RequiredToolFailure records a reportable required-tool policy failure. It
	// blocks without producing a finding, so it is carried separately rather
	// than faked as one.
	RequiredToolFailure bool
}

// ConfidenceFor is the confidence a measurement status supports on its own.
//
// V1 derives confidence from status because status is the one evidence-quality
// fact all nine dimensions share. A dimension with a sharper signal — a
// low-confidence contributing metric, a truncated tool run — lowers it further
// from its own evidence; nothing may raise it above this rung.
func ConfidenceFor(s MeasurementStatus) Confidence {
	switch s {
	case Measured:
		return ConfidenceHigh
	case Partial:
		return ConfidenceMedium
	default:
		return ConfidenceUnrated
	}
}

// NewDimension returns one honestly unmeasured envelope with deterministic
// non-nil collections. Collectors start here and fill in what they observed.
func NewDimension(name, owner string) Dimension {
	return Dimension{
		Name: name, Owner: owner,
		Status: Unmeasured, Confidence: ConfidenceUnrated, Gate: GateNotApplicable,
		Metrics: []MetricValue{}, Findings: []FindingRef{}, Unknown: []UnknownFact{},
	}
}

// New returns a state whose nine envelopes are named, owned, and honestly
// unmeasured, with deterministic non-nil collections. A caller that measures
// nothing therefore reports nothing measured — never an implicit green result.
func New() Architecture {
	return Architecture{
		Decision: Decision{HardGates: HardGateUnmeasured, UnknownDimensions: DimensionCount},
		Dimensions: Dimensions{
			Intent:         NewDimension(DimensionIntent, OwnerIntent),
			Structure:      NewDimension(DimensionStructure, OwnerStructure),
			Modularity:     NewDimension(DimensionModularity, OwnerModularity),
			Coupling:       NewDimension(DimensionCoupling, OwnerCoupling),
			ChangeLocality: NewDimension(DimensionChangeLocality, OwnerChangeLocality),
			Complexity:     NewDimension(DimensionComplexity, OwnerComplexity),
			Testability:    NewDimension(DimensionTestability, OwnerTestability),
			Operations:     NewDimension(DimensionOperations, OwnerOperations),
			Drift:          NewDimension(DimensionDrift, OwnerDrift),
		},
		Blockers:    []FindingRef{},
		Diagnostics: []FindingRef{},
	}
}
