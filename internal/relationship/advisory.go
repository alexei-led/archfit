package relationship

import "github.com/alexei-led/archfit/internal/model/evidence"

// AdvisoryCandidate is relationship-owned evidence for an assessment advisory.
// It deliberately contains no finding lifecycle or report types.
type AdvisoryCandidate struct {
	ID         string
	RuleID     string
	Kind       string
	Severity   Severity
	From       string
	To         string
	FromModule string
	ToModule   string
	EdgeKind   string
	Locations  []Location
	Why        string
	MatchedBy  map[string]string
}

// ClassifiedEdgeSummary is the report-neutral distribution of classified edges.
type ClassifiedEdgeSummary struct {
	Total                 int
	Scored                int
	Abstained             int
	SameModule            int
	MeanBalance           float64
	TailRisk              *CouplingTailRiskSummary
	ByStrength            map[string]int
	ByDistance            map[string]int
	ByDistanceBasis       map[string]int
	ByVolatility          map[string]int
	BySeverity            map[string]int
	ByBalanceDriver       map[string]int
	ByCriticalDriver      map[string]int
	ByModulePair          map[string]int
	DistributedMonolith   int
	External              int
	DeclaredExternal      int
	ConnectedModules      int
	CloneOnlyScored       int
	CloneOnlyAdvisory     int
	LLMApproved           int
	LabeledLLM            int
	LLMLowConfidenceEdges int
	VolatilityProvenance  *VolatilityProvenance
	DistanceCompression   *DistanceCompressionSummary
}

// CouplingTailRiskSummary records lower-tail relationship statistics.
type CouplingTailRiskSummary struct {
	WorstBalance              int
	LowerDecileBalance        int
	HighOrWorseEdges          int
	HighOrWorseSharePct       int
	CriticalEdges             int
	DistributedMonolithEdges  int
	CloneOnlyScored           int
	CloneOnlyHighOrWorseEdges int
	CloneOnlyWorstBalance     int
}

// DistanceCompressionSummary records deterministic distance-ladder coverage.
type DistanceCompressionSummary struct {
	CompressedMiddleRungs       bool
	ImplementedRungs            []int
	OmittedRungs                []int
	OmittedRungReasons          []DistanceOmittedRungReason
	DeterministicSplits         []string
	CodeStructureBoundaryCounts []DistanceCount
	CodeStructureAncestorDepths []DistanceCount
	Rationale                   string
}

// DistanceCount is one distance-evidence histogram bucket.
type DistanceCount struct {
	Value int
	Count int
}

// DistanceOmittedRungReason explains why a distance rung remains compressed.
type DistanceOmittedRungReason struct {
	Rung   int
	Reason string
}

// AnalysisResult is the complete relationship-owned output of classification.
type AnalysisResult struct {
	Relationships            Set
	StaleLabelKeys           []string
	LLMApprovedCount         int
	RuntimeSignals           []RuntimeSignal
	RuntimeRelations         []RuntimeRelationship
	CloneOnly                []CloneOnlyPair
	AdvisoryCandidates       []AdvisoryCandidate
	ClassifiedEdges          *ClassifiedEdgeSummary
	Connascence              *evidence.ConnascenceReport
	DistanceConfigCandidates []evidence.DistanceConfigCandidate
	LocalCoupling            []evidence.LocalCouplingModule
	VolatilityProvenance     *VolatilityProvenance
}
