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

// ClassifiedEdgeSummary is the assessment distribution over classified relationships.
type ClassifiedEdgeSummary struct {
	Total                 int                         `json:"total"`
	Scored                int                         `json:"scored"`
	Abstained             int                         `json:"abstained"`
	SameModule            int                         `json:"same_module"`
	MeanBalance           float64                     `json:"mean_balance"`
	TailRisk              *CouplingTailRiskSummary    `json:"tail_risk,omitempty"`
	ByStrength            map[string]int              `json:"by_strength,omitempty"`
	ByDistance            map[string]int              `json:"by_distance,omitempty"`
	ByDistanceBasis       map[string]int              `json:"by_distance_basis,omitempty"`
	ByVolatility          map[string]int              `json:"by_volatility,omitempty"`
	BySeverity            map[string]int              `json:"by_severity,omitempty"`
	ByBalanceDriver       map[string]int              `json:"by_balance_driver,omitempty"`
	ByCriticalDriver      map[string]int              `json:"by_critical_driver,omitempty"`
	ByModulePair          map[string]int              `json:"by_module_pair,omitempty"`
	DistributedMonolith   int                         `json:"distributed_monolith,omitempty"`
	External              int                         `json:"external,omitempty"`
	DeclaredExternal      int                         `json:"declared_external,omitempty"`
	ConnectedModules      int                         `json:"connected_modules,omitempty"`
	CloneOnlyScored       int                         `json:"clone_only_scored,omitempty"`
	CloneOnlyAdvisory     int                         `json:"clone_only_advisory,omitempty"`
	LLMApproved           int                         `json:"llm_approved,omitempty"`
	LabeledLLM            int                         `json:"labeled_llm,omitempty"`
	LLMLowConfidenceEdges int                         `json:"-"`
	VolatilityProvenance  *VolatilityProvenance       `json:"volatility_provenance,omitempty"`
	DistanceCompression   *DistanceCompressionSummary `json:"distance_compression,omitempty"`
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
