package result

// Verdict is the top-level assessment outcome.
type Verdict string

const (
	// VerdictPass means all blocking checks passed.
	VerdictPass Verdict = "pass"
	// VerdictFail means at least one blocking check failed.
	VerdictFail Verdict = "fail"
	// VerdictWarn means checks passed with warnings.
	VerdictWarn Verdict = "warn"
)

// Direction records whether a rising metric is an improvement or regression.
type Direction string

const (
	// DirectionHigherIsBetter means a larger metric value is better.
	DirectionHigherIsBetter Direction = "higher_is_better"
	// DirectionHigherIsWorse means a larger metric value is worse.
	DirectionHigherIsWorse Direction = "higher_is_worse"
)

// MetricResult is one computed assessment metric.
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

// MetricSnapshot stores baseline metric values by name.
type MetricSnapshot map[string]struct {
	Value   float64 `json:"value"`
	Version string  `json:"version"`
}

// Summary holds gate, warning, and waiver counts.
type Summary struct {
	GateFindings int `json:"gate_findings"`
	Warnings     int `json:"warnings"`
	WaiversUsed  int `json:"waivers_used"`
}

// DeltaReport groups finding IDs by their relationship to the baseline.
type DeltaReport struct {
	New             []string `json:"new,omitempty"`
	Existing        []string `json:"existing,omitempty"`
	Resolved        []string `json:"resolved,omitempty"`
	SeverityChanged []string `json:"severity_changed,omitempty"`
	TouchedByDelta  []string `json:"touched_by_delta,omitempty"`
}

// ModuleGraphComplexity is the architecture-level distribution over the
// declared module graph. It stays internal to assessment; the architecture-state
// envelope publishes the individual metrics.
type ModuleGraphComplexity struct {
	Modules            int
	MaxDependencyChain int
	FanInP90           int
	FanOutP90          int
}

// ClassifiedEdgeSummary is the assessment distribution over classified relationships.
type ClassifiedEdgeSummary struct {
	Total                          int                         `json:"total"`
	Scored                         int                         `json:"scored"`
	Abstained                      int                         `json:"abstained"`
	SameModule                     int                         `json:"same_module"`
	DependencyEdges                int                         `json:"-"`
	InternalDependencies           int                         `json:"-"`
	ClassifiedInternalDependencies int                         `json:"-"`
	SameModuleDependencies         int                         `json:"-"`
	DependencyModules              int                         `json:"-"`
	FirstPartyNodes                int                         `json:"-"`
	AttributedFirstPartyNodes      int                         `json:"-"`
	MeanBalance                    float64                     `json:"mean_balance"`
	TailRisk                       *CouplingTailRiskSummary    `json:"tail_risk,omitempty"`
	ByStrength                     map[string]int              `json:"by_strength,omitempty"`
	ByDistance                     map[string]int              `json:"by_distance,omitempty"`
	ByDistanceBasis                map[string]int              `json:"by_distance_basis,omitempty"`
	ByVolatility                   map[string]int              `json:"by_volatility,omitempty"`
	BySeverity                     map[string]int              `json:"by_severity,omitempty"`
	ByBalanceDriver                map[string]int              `json:"by_balance_driver,omitempty"`
	ByCriticalDriver               map[string]int              `json:"by_critical_driver,omitempty"`
	ByModulePair                   map[string]int              `json:"by_module_pair,omitempty"`
	DistributedMonolith            int                         `json:"distributed_monolith,omitempty"`
	External                       int                         `json:"external,omitempty"`
	DeclaredExternal               int                         `json:"declared_external,omitempty"`
	ConnectedModules               int                         `json:"connected_modules,omitempty"`
	CloneOnlyScored                int                         `json:"clone_only_scored,omitempty"`
	CloneOnlyAdvisory              int                         `json:"clone_only_advisory,omitempty"`
	LLMApproved                    int                         `json:"llm_approved,omitempty"`
	LabeledLLM                     int                         `json:"labeled_llm,omitempty"`
	LLMLowConfidenceEdges          int                         `json:"-"`
	VolatilityProvenance           *VolatilityProvenance       `json:"volatility_provenance,omitempty"`
	DistanceCompression            *DistanceCompressionSummary `json:"distance_compression,omitempty"`
}

// CouplingTailRiskSummary records lower-tail relationship statistics.
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

// DistanceCompressionSummary records deterministic distance-ladder coverage.
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

// DistanceCount is one distance-evidence histogram bucket.
type DistanceCount struct {
	Value int `json:"value"`
	Count int `json:"count"`
}

// DistanceOmittedRungReason explains why a distance rung remains compressed.
type DistanceOmittedRungReason struct {
	Rung   int    `json:"rung"`
	Reason string `json:"reason"`
}

// VolatilityProvenance counts modules by volatility source.
type VolatilityProvenance struct {
	Declared   int `json:"declared"`
	Inherited  int `json:"inherited"`
	Cascade    int `json:"cascade"`
	Undeclared int `json:"undeclared"`
}

// GitFindingDelta records repair-task origin relative to a git base.
type GitFindingDelta struct {
	BaseRef                 string   `json:"base_ref"`
	ComparisonStatus        string   `json:"comparison_status"`
	IntroducedFindingIDs    []string `json:"introduced_finding_ids"`
	PreExistingFindingIDs   []string `json:"pre_existing_finding_ids"`
	UnknownOriginFindingIDs []string `json:"unknown_origin_finding_ids"`
	ComparisonReasons       []string `json:"comparison_reasons"`
}

const (
	// GitComparisonComparable means task origins were established.
	GitComparisonComparable = "comparable"
	// GitComparisonUnknown means at least one task origin was uncertain.
	GitComparisonUnknown = "unknown"
)

// Seam is the assessment-side mirror of one logical coupling seam: an ordered
// module pair with its measured edge denominator, score distribution, raw
// distance context, and role expectation.
//
// The seam replaces the repository coupling scalar as the unit of coupling
// reporting. Every field is a fact about that one pair; there is deliberately
// no repository-wide roll-up here, because averaging seams is exactly the
// aggregation the migration removes.
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
	Labels               []string              `json:"labels,omitempty"`
	LabelEvidenceHash    string                `json:"label_evidence_hash,omitempty"`
	Confidence           string                `json:"confidence"`
	RoleExpectation      string                `json:"role_expectation,omitempty"`
	Hypothesis           string                `json:"hypothesis,omitempty"`
	DistributedMonolith  bool                  `json:"distributed_monolith,omitempty"`
}

// SeamDistance is the raw distance evidence behind a seam's collapsed rung.
type SeamDistance struct {
	Level             string `json:"level"`
	Basis             string `json:"basis,omitempty"`
	FromOwner         string `json:"from_owner,omitempty"`
	ToOwner           string `json:"to_owner,omitempty"`
	SameOwner         bool   `json:"same_owner"`
	FromDeployUnit    string `json:"from_deploy_unit,omitempty"`
	ToDeployUnit      string `json:"to_deploy_unit,omitempty"`
	SameDeployUnit    bool   `json:"same_deploy_unit"`
	BoundaryCrossings int    `json:"boundary_crossings"`
	SharedAncestor    int    `json:"shared_ancestor"`
}

// SeamScoreDistribution is the per-seam spread of book balance scores. P10 and
// P90 are null below ten samples: a decile over fewer samples is invented.
type SeamScoreDistribution struct {
	N      int     `json:"n"`
	Min    int     `json:"min"`
	Median int     `json:"median"`
	Max    int     `json:"max"`
	P10    *int    `json:"p10"`
	P90    *int    `json:"p90"`
	Mean   float64 `json:"mean"`
}

// State comparison statuses. They mirror the architecture-state contract
// without importing the report DTOs.
const (
	StateComparisonComparable    = "comparable"
	StateComparisonNonComparable = "non_comparable"
	StateComparisonNotRequested  = "not_requested"
)

// StateComparison records what a run was compared against and whether the
// comparison is admissible at all. A non-comparable result always carries at
// least one reason: "not comparable" with no explanation is indistinguishable
// from a bug.
type StateComparison struct {
	Status  string   `json:"status"`
	BaseRef string   `json:"base_ref,omitempty"`
	Reasons []string `json:"reasons"`
}
