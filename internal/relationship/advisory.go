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

// AssessmentSignals is the explicit relationship-to-assessment contract: the
// classification facts assessment needs to raise advisories, disclose health
// hints, and synthesize metrics. It deliberately carries no report evidence and
// no finding lifecycle.
type AssessmentSignals struct {
	AdvisoryCandidates []AdvisoryCandidate
	ClassifiedEdges    *ClassifiedEdgeSummary
	StaleLabelKeys     []string
}

// AnalysisEvidence is report-only relationship provenance. Assessment never
// reads it; the application projects it into the external report contract.
type AnalysisEvidence struct {
	LLMApprovedCount int
	// RuntimeModules and RuntimeEdges are the async-bridge rollups keyed by the
	// module map classification actually used. Report-only: they never annotate
	// an edge, never move distance, and never gate.
	RuntimeModules []evidence.RuntimeAsyncModule
	RuntimeEdges   []evidence.RuntimeAsyncEdge
	// DynamicImports and DynamicConnascenceSignals roll up acquired dynamic and
	// lazy import sites against the same module map. They are report-only and
	// keyed here so no consumer has to re-derive the augmented module map.
	DynamicImports            []evidence.DynamicImport
	DynamicConnascenceSignals *evidence.DynamicConnascenceSignals
	CloneOnly                 []CloneOnlyPair
	ClassifiedEdges           *ClassifiedEdgeSummary
	Connascence               *evidence.ConnascenceReport
	DistanceConfigCandidates  []evidence.DistanceConfigCandidate
	LocalCoupling             []evidence.LocalCouplingModule
	VolatilityProvenance      *VolatilityProvenance
}

// AnalysisResult is the relationship stage output. The three members are
// separate on purpose: Relationships is the immutable set every consumer may
// hold, Assessment is the narrow judgment contract, and Evidence is report-only
// provenance no decision may read.
type AnalysisResult struct {
	Relationships Set
	Assessment    AssessmentSignals
	Evidence      AnalysisEvidence
}
