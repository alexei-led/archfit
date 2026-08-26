package report

// StateSchemaVersion identifies the architecture-state report contract. It is
// versioned independently of SchemaVersion (the diagnostic envelope) because
// the two cut over on different schedules: the state contract is introduced
// alongside the diagnostic envelope and becomes the primary output later in the
// migration (docs/design/architecture-state-reporting.md).
const StateSchemaVersion = "archfit.architecture-state.v1"

// StateVerdict is the primary architecture decision. It replaces the banded
// repository scalar: no averaged score may produce or move it.
type StateVerdict string

// Architecture-state verdicts. JSON stores lower case; human formats display
// upper case.
const (
	// StateHealthy requires every dimension measured, every hard gate passing,
	// and no active diagnostic. It is never produced by treating an unsupported
	// collector as zero.
	StateHealthy StateVerdict = "healthy"
	// StateNeedsAttention means no blocker, but at least one active diagnostic
	// exists or at least one dimension is partial/unmeasured.
	StateNeedsAttention StateVerdict = "needs_attention"
	// StateBlocked means at least one active hard-gate finding is reportable.
	StateBlocked StateVerdict = "blocked"
)

// MeasurementStatus records whether a dimension's evidence was actually
// gathered. Missing evidence is explicit, never an implicit green result.
type MeasurementStatus string

// Measurement statuses for one dimension envelope.
const (
	MeasurementMeasured   MeasurementStatus = "measured"
	MeasurementPartial    MeasurementStatus = "partial"
	MeasurementUnmeasured MeasurementStatus = "unmeasured"
)

// GateState is one dimension's gate posture. It is deliberately a different
// enum from HardGateState: a dimension may warn, the repository decision may
// not.
type GateState string

// Per-dimension gate postures.
const (
	GatePass          GateState = "pass"
	GateWarn          GateState = "warn"
	GateFail          GateState = "fail"
	GateNotApplicable GateState = "not_applicable"
)

// HardGateState is the repository-level hard-gate result carried by
// StateDecision. Only a named hard rule can set it to fail.
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

// ConfidenceUnrated marks a dimension whose evidence does not support any
// confidence claim — the abstain rung of the existing Confidence enum.
const ConfidenceUnrated Confidence = "unrated"

// DimensionCount is the fixed number of architecture-state dimensions. Coverage
// counts must sum to it.
const DimensionCount = 9

// Architecture-state dimension names. They are the JSON keys of Dimensions and
// the Name field of each envelope.
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
	Observed int `json:"observed"`
	Total    int `json:"total"`
}

// MetricValue is one typed metric record. On the wire a dimension's metrics are
// always an array of these, never an object or a free-form map.
type MetricValue struct {
	Name        string             `json:"name"`
	Value       float64            `json:"value"`
	Unit        string             `json:"unit"`
	Denominator *MetricDenominator `json:"denominator,omitempty"`
	Provenance  []string           `json:"provenance,omitempty"`
}

// FindingRef points a dimension envelope at a finding in the state's findings
// list. Findings themselves are never duplicated per dimension, so identity,
// status, and ordering keep exactly one owner.
type FindingRef struct {
	ID       string `json:"id"`
	RuleID   string `json:"rule_id"`
	Kind     string `json:"kind"`
	Severity string `json:"severity"`
	Status   string `json:"status"`
}

// UnknownFact names one thing a dimension could not observe, why, and which
// capability would have to observe it. It is how partial/unmeasured states stay
// honest instead of silently reading as zero.
type UnknownFact struct {
	Fact   string `json:"fact"`
	Reason string `json:"reason"`
	Owner  string `json:"owner"`
}

// MetricDelta is a signed change in one dimension metric between a comparable
// reference and this run.
type MetricDelta struct {
	Name   string  `json:"name"`
	Before float64 `json:"before"`
	After  float64 `json:"after"`
	Change float64 `json:"change"`
}

// DimensionDelta is one dimension's change against the comparison reference.
// A non-comparable reference yields reasons and no numbers.
type DimensionDelta struct {
	Status           ComparisonStatus `json:"status"`
	Reasons          []string         `json:"reasons,omitempty"`
	Metrics          []MetricDelta    `json:"metrics,omitempty"`
	NewFindings      []string         `json:"new_findings,omitempty"`
	ResolvedFindings []string         `json:"resolved_findings,omitempty"`
}

// DimensionCoverage is a dimension's denominator: what was counted, how much of
// it was observed, and out of how much. A dimension cannot be measured without
// a nonzero or explicitly empty denominator.
type DimensionCoverage struct {
	Basis    string `json:"basis"`
	Observed int    `json:"observed"`
	Total    int    `json:"total"`
}

// DimensionState is the one common envelope every architecture-state dimension
// uses. Dimension-specific facts stay in typed Metrics records; this struct
// never becomes a generic property bag.
type DimensionState struct {
	Name       string            `json:"name"`
	Owner      string            `json:"owner"`
	Status     MeasurementStatus `json:"status"`
	Confidence Confidence        `json:"confidence"`
	Gate       GateState         `json:"gate"`
	Coverage   DimensionCoverage `json:"coverage"`
	Metrics    []MetricValue     `json:"metrics"`
	Findings   []FindingRef      `json:"findings"`
	Unknown    []UnknownFact     `json:"unknown"`
	Delta      *DimensionDelta   `json:"delta,omitempty"`
}

// Dimensions holds the nine architecture-state envelopes as named fields rather
// than a map: each dimension has a compile-time name and owner, and the wire
// order is fixed by declaration order instead of by map iteration.
type Dimensions struct {
	Intent         DimensionState `json:"intent"`
	Structure      DimensionState `json:"structure"`
	Modularity     DimensionState `json:"modularity"`
	Coupling       DimensionState `json:"coupling"`
	ChangeLocality DimensionState `json:"change_locality"`
	Complexity     DimensionState `json:"complexity"`
	Testability    DimensionState `json:"testability"`
	Operations     DimensionState `json:"operations"`
	Drift          DimensionState `json:"drift"`
}

// All returns the nine envelopes in their fixed contract order.
func (d Dimensions) All() []DimensionState {
	return []DimensionState{
		d.Intent, d.Structure, d.Modularity, d.Coupling, d.ChangeLocality,
		d.Complexity, d.Testability, d.Operations, d.Drift,
	}
}

// CountStatuses tallies the nine envelopes by measurement status. It is purely
// mechanical — no policy, no thresholds — so the coverage block and the
// decision counters cannot disagree about what was measured.
func (d Dimensions) CountStatuses() (measured, partial, unmeasured int) {
	for _, dim := range d.All() {
		switch dim.Status {
		case MeasurementMeasured:
			measured++
		case MeasurementPartial:
			partial++
		default:
			unmeasured++
		}
	}
	return measured, partial, unmeasured
}

// StateDecision holds the explicit inputs to the verdict. The aggregator reads
// only these classifications plus dimension status; it may never inspect a
// dimension's metrics or derive an implicit threshold from them.
type StateDecision struct {
	HardGates           HardGateState `json:"hard_gates"`
	ActiveBlockers      int           `json:"active_blockers"`
	AttentionDimensions int           `json:"attention_dimensions"`
	UnknownDimensions   int           `json:"unknown_dimensions"`
}

// StateComparison records what this run was compared against and whether the
// comparison is admissible at all.
type StateComparison struct {
	Status        ComparisonStatus `json:"status"`
	BaseRef       string           `json:"base_ref,omitempty"`
	ConfigHash    string           `json:"config_hash,omitempty"`
	ModelHash     string           `json:"model_hash,omitempty"`
	LabelsHash    string           `json:"labels_hash,omitempty"`
	RubricVersion string           `json:"rubric_version,omitempty"`
	Reasons       []string         `json:"reasons"`
}

// StateMeasurement holds deterministic source, history, and tool facts only. It
// deliberately excludes wall-clock evaluation time, absolute local paths, and
// process IDs so two identical runs stay byte-identical.
type StateMeasurement struct {
	SourceRef     string            `json:"source_ref"`
	HistoryDepth  int               `json:"history_depth"`
	HistoryWindow string            `json:"history_window"`
	ToolVersions  map[string]string `json:"tool_versions"`
}

// StateToolCoverage is one analyzer's coverage row as the state reports it.
type StateToolCoverage struct {
	Tool   string `json:"tool"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// StateCoverage summarises evidence coverage across the nine dimensions. The
// three status counts always sum to DimensionCount.
type StateCoverage struct {
	Measured   int                 `json:"measured"`
	Partial    int                 `json:"partial"`
	Unmeasured int                 `json:"unmeasured"`
	Tools      []StateToolCoverage `json:"tools"`
}

// ArchitectureState is the versioned architecture-state report contract. It has
// no repository-level scalar by construction: the verdict comes from explicit
// hard gates, diagnostics, and evidence coverage.
type ArchitectureState struct {
	SchemaVersion string           `json:"schema_version"`
	Verdict       StateVerdict     `json:"verdict"`
	Decision      StateDecision    `json:"decision"`
	Comparison    StateComparison  `json:"comparison"`
	Measurement   StateMeasurement `json:"measurement"`
	Dimensions    Dimensions       `json:"dimensions"`
	Coverage      StateCoverage    `json:"coverage"`
	// Findings carries the run's findings unchanged; dimension envelopes point
	// into it by ID so finding identity has exactly one owner.
	Findings []Finding `json:"findings"`
	// AgentTasks is projected from the Assessment-owned agent-task result. State
	// aggregation never derives a second task list.
	AgentTasks []AgentTask `json:"agent_tasks"`
}

// NewArchitectureState returns a state whose nine envelopes are named, owned,
// and honestly unmeasured, with deterministic non-nil collections. A caller
// that measures nothing therefore reports nothing measured — never an implicit
// green result.
func NewArchitectureState() ArchitectureState {
	return ArchitectureState{
		SchemaVersion: StateSchemaVersion,
		Decision:      StateDecision{HardGates: HardGateUnmeasured, UnknownDimensions: DimensionCount},
		Comparison:    StateComparison{Status: ComparisonNotRequested, Reasons: []string{}},
		Measurement:   StateMeasurement{ToolVersions: map[string]string{}},
		Dimensions: Dimensions{
			Intent:         newDimension(DimensionIntent, OwnerIntent),
			Structure:      newDimension(DimensionStructure, OwnerStructure),
			Modularity:     newDimension(DimensionModularity, OwnerModularity),
			Coupling:       newDimension(DimensionCoupling, OwnerCoupling),
			ChangeLocality: newDimension(DimensionChangeLocality, OwnerChangeLocality),
			Complexity:     newDimension(DimensionComplexity, OwnerComplexity),
			Testability:    newDimension(DimensionTestability, OwnerTestability),
			Operations:     newDimension(DimensionOperations, OwnerOperations),
			Drift:          newDimension(DimensionDrift, OwnerDrift),
		},
		Coverage:   StateCoverage{Unmeasured: DimensionCount, Tools: []StateToolCoverage{}},
		Findings:   []Finding{},
		AgentTasks: []AgentTask{},
	}
}

func newDimension(name, owner string) DimensionState {
	return DimensionState{
		Name: name, Owner: owner,
		Status: MeasurementUnmeasured, Confidence: ConfidenceUnrated, Gate: GateNotApplicable,
		Metrics: []MetricValue{}, Findings: []FindingRef{}, Unknown: []UnknownFact{},
	}
}
