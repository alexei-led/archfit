package report

// StateSchemaVersion identifies the canonical architecture-state JSON contract.
// Renderers publish this state directly; the broader Document remains an
// internal adapter input rather than a second wire format.
const StateSchemaVersion = "archfit.architecture-state.v1"

// MeasurementStatus records whether a dimension's evidence was actually
// gathered. Missing evidence is explicit, never an implicit green result.
type MeasurementStatus string

// Measurement statuses for one dimension envelope.
const (
	MeasurementMeasured   MeasurementStatus = "measured"
	MeasurementPartial    MeasurementStatus = "partial"
	MeasurementUnmeasured MeasurementStatus = "unmeasured"
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
	OwnerComplexity     = "relationship/analysis+syntax+evidence/acquisition"
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
	Coverage   DimensionCoverage `json:"coverage"`
	Metrics    []MetricValue     `json:"metrics"`
	Findings   []FindingRef      `json:"findings"`
	Unknown    []UnknownFact     `json:"unknown"`
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
// mechanical — no policy or thresholds participate.
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

// StateMeasurement holds deterministic source and tool facts only. It
// deliberately excludes wall-clock evaluation time, absolute local paths, and
// process IDs so two identical runs stay byte-identical.
type StateMeasurement struct {
	SourceRef    string            `json:"source_ref"`
	ToolVersions map[string]string `json:"tool_versions"`
}

// StateToolCoverage is one analyzer's coverage row as the state reports it.
type StateToolCoverage struct {
	Tool   string `json:"tool"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
	// Details is the per-unit table behind Reason: one line per project, target
	// or package. The row keeps one shortened reason, so this is where a
	// consumer reads each unit without re-running the producer.
	Details []string `json:"details,omitempty"`
}

// StateCoverage summarises evidence coverage across the nine dimensions. The
// three status counts always sum to DimensionCount.
type StateCoverage struct {
	Measured   int                 `json:"measured"`
	Partial    int                 `json:"partial"`
	Unmeasured int                 `json:"unmeasured"`
	Tools      []StateToolCoverage `json:"tools"`
}

// SeamDistance is the raw distance evidence behind a seam's collapsed rung. The
// facts and the analysis level are published together: a reader must be able to
// see WHY a pair reads as distant, not just that it does.
type SeamDistance struct {
	Level             string `json:"level"`
	Basis             string `json:"basis,omitempty"`
	FromOwner         string `json:"from_owner,omitempty"`
	ToOwner           string `json:"to_owner,omitempty"`
	SameOwner         *bool  `json:"same_owner"` // null unless both owners are known
	FromDeployUnit    string `json:"from_deploy_unit,omitempty"`
	ToDeployUnit      string `json:"to_deploy_unit,omitempty"`
	SameDeployUnit    *bool  `json:"same_deploy_unit"` // null unless both deploy units are declared
	BoundaryCrossings int    `json:"boundary_crossings"`
	SharedAncestor    int    `json:"shared_ancestor"`
}

// SeamScoreDistribution is the per-seam spread of book balance scores. P10 and
// P90 are null below ten samples: a decile over fewer samples is invented, and
// a null is the only honest way to say so.
type SeamScoreDistribution struct {
	N      int     `json:"n"`
	Min    int     `json:"min"`
	Median int     `json:"median"`
	Max    int     `json:"max"`
	P10    *int    `json:"p10"`
	P90    *int    `json:"p90"`
	Mean   float64 `json:"mean"`
}

// Seam is one logical ordered module pair — however many imports express it —
// with the evidence behind its classification. The ledger is the coupling
// dimension's evidence: the dimension envelope keeps the common shape, and the
// per-seam facts live here rather than being flattened into a repository mean.
type Seam struct {
	ID                   string                `json:"id"`
	FromModule           string                `json:"from_module"`
	ToModule             string                `json:"to_module"`
	Edges                int                   `json:"edges"`
	ScoredEdges          int                   `json:"scored_edges"`
	AbstainedEdges       int                   `json:"abstained_edges"`
	Strength             string                `json:"strength"`
	Distance             string                `json:"distance"`
	Volatility           string                `json:"volatility"`
	VolatilityProvenance string                `json:"volatility_provenance,omitempty"`
	Severity             string                `json:"severity,omitempty"`
	RawDistance          SeamDistance          `json:"raw_distance"`
	Quadrant             string                `json:"quadrant,omitempty"`
	Scores               SeamScoreDistribution `json:"scores"`
	CriticalEdges        int                   `json:"critical_edges"`
	HighOrWorseEdges     int                   `json:"high_or_worse_edges"`
	CriticalSharePct     int                   `json:"critical_share_pct"`
	HighOrWorseSharePct  int                   `json:"high_or_worse_share_pct"`
	Confidence           string                `json:"confidence"`
	RoleExpectation      string                `json:"role_expectation,omitempty"`
	Hypothesis           string                `json:"hypothesis,omitempty"`
	DistributedMonolith  bool                  `json:"distributed_monolith,omitempty"`
}

// ArchitectureState is the versioned architecture-state report contract. It has
// no verdict: measurement completeness and repository state remain separate.
type ArchitectureState struct {
	SchemaVersion string `json:"schema_version"`
	// Identity names what the run was judged under.
	Identity    ResultIdentity   `json:"identity"`
	Measurement StateMeasurement `json:"measurement"`
	Dimensions  Dimensions       `json:"dimensions"`
	Coverage    StateCoverage    `json:"coverage"`
	// Findings carries the run's findings unchanged; dimension envelopes point
	// into it by ID so finding identity has exactly one owner.
	Findings []Finding `json:"findings"`
	// Seams is the coupling dimension's evidence: one record per ordered module
	// pair. It is published beside the dimensions, not inside one, for the same
	// reason findings are — the ledger has exactly one owner and the coupling
	// envelope keeps the common shape every dimension uses.
	Seams []Seam `json:"seams"`
	// Repository is the state of the repository for owners. Only `archfit
	// report` on a version 3 policy sets it.
	Repository *RepositoryState `json:"repository,omitempty"`
}

// NewArchitectureState returns a state whose nine envelopes are named, owned,
// and honestly unmeasured, with deterministic non-nil collections. A caller
// that measures nothing therefore reports nothing measured — never an implicit
// green result.
func NewArchitectureState() ArchitectureState {
	return ArchitectureState{
		SchemaVersion: StateSchemaVersion,
		Identity:      ResultIdentity{SchemaVersion: IdentitySchemaVersion},
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
		Coverage: StateCoverage{Unmeasured: DimensionCount, Tools: []StateToolCoverage{}},
		Findings: []Finding{},
		Seams:    []Seam{},
	}
}

func newDimension(name, owner string) DimensionState {
	return DimensionState{
		Name: name, Owner: owner,
		Status: MeasurementUnmeasured, Confidence: ConfidenceUnrated,
		Metrics: []MetricValue{}, Findings: []FindingRef{}, Unknown: []UnknownFact{},
	}
}
