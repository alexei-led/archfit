package result

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
	Direction  Direction `json:"direction,omitempty"`
}

// Summary counts diagnostic warnings.
type Summary struct {
	Warnings int `json:"warnings"`
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
	LibraryDependencies            int                         `json:"-"`
	UnmappedFirstPartyDependencies int                         `json:"-"`
	DependencyModules              int                         `json:"-"`
	FirstPartyNodes                int                         `json:"-"`
	AttributedFirstPartyNodes      int                         `json:"-"`
	UnratedVolatilityEdges         int                         `json:"-"`
	UnratedVolatilityModules       []string                    `json:"-"`
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
	VolatilityProvenance           *VolatilityProvenance       `json:"volatility_provenance,omitempty"`
	DistanceCompression            *DistanceCompressionSummary `json:"distance_compression,omitempty"`
}

// CouplingTailRiskSummary records lower-tail relationship statistics.
type CouplingTailRiskSummary struct {
	WorstBalance             int `json:"worst_balance"`
	LowerDecileBalance       int `json:"lower_decile_balance"`
	HighOrWorseEdges         int `json:"high_or_worse_edges"`
	HighOrWorseSharePct      int `json:"high_or_worse_share_pct"`
	CriticalEdges            int `json:"critical_edges"`
	DistributedMonolithEdges int `json:"distributed_monolith_edges"`
}

// DistanceCompressionSummary records deterministic distance-ladder coverage.
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

// Seam is the assessment-side mirror of one logical coupling seam: an ordered
// module pair with its measured edge denominator, score distribution, raw
// distance context, and role expectation.
//
// The seam replaces the repository coupling scalar as the unit of coupling
// reporting. Every field is a fact about that one pair; there is deliberately
// no repository-wide roll-up here, because averaging seams would recreate the
// scalar architecture decision this model replaced.
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

// SeamDistance is the raw distance evidence behind a seam's collapsed rung.
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
