package report

import "github.com/alexei-led/archfit/internal/model/evidence"

// SchemaVersion identifies the external report document contract.
const SchemaVersion = "archfit.diagnostic.v2"

// AgentTask is the structured repair task for an active gate finding.
type AgentTask struct {
	FindingID    string                `json:"finding_id"`
	RuleID       string                `json:"rule_id"`
	Goal         string                `json:"goal"`
	Constraints  []string              `json:"constraints"`
	Files        []string              `json:"files"`
	Validation   []string              `json:"validation"`
	Declarations []evidence.SyntaxFact `json:"declarations,omitempty"`
}

// FindingStatus values describe the lifecycle state of a report finding.
const (
	FindingStatusNew           = "new"
	FindingStatusBaseline      = "baseline"
	FindingStatusWaived        = "waived"
	FindingStatusExpiredWaiver = "expired_waiver"
	FindingStatusFixed         = "fixed"
)

// FindingKind values classify blocking gates and non-blocking advisories.
const (
	FindingKindGate     = "gate"
	FindingKindAdvisory = "advisory"
)

// FindingSeverity values describe report finding severity.
const (
	FindingSeverityCritical = "critical"
	FindingSeverityHigh     = "high"
	FindingSeverityMedium   = "medium"
	FindingSeverityLow      = "low"
)

// FindingEndpoint identifies one side of a report finding edge.
type FindingEndpoint struct {
	Module string `json:"module"`
	Path   string `json:"path"`
}

// FindingEdge is the report representation of an assessed edge.
type FindingEdge struct {
	From FindingEndpoint `json:"from"`
	To   FindingEndpoint `json:"to"`
	Kind string          `json:"kind"`
}

// Finding is the stable report view of an assessment finding.
type Finding struct {
	ID           string            `json:"id"`
	Kind         string            `json:"kind"`
	RuleID       string            `json:"rule_id"`
	Status       string            `json:"status"`
	Severity     string            `json:"severity"`
	Confidence   string            `json:"confidence"`
	Edge         FindingEdge       `json:"edge"`
	MatchedBy    map[string]string `json:"matched_by"`
	Locations    []Location        `json:"locations"`
	Why          string            `json:"why"`
	Constraint   string            `json:"constraint"`
	Alternatives []string          `json:"allowed_alternatives,omitempty"`
}

// Location identifies a source location in a report finding.
type Location struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// AdvisoryTask is a report-only remediation prompt for grouped advisories.
type AdvisoryTask struct {
	FindingID    string   `json:"finding_id"`
	RuleID       string   `json:"rule_id"`
	Status       string   `json:"status"`
	Severity     string   `json:"severity"`
	GroupCount   int      `json:"group_count"`
	GroupMembers []string `json:"group_members,omitempty"`
	Goal         string   `json:"goal"`
	CheapestMove string   `json:"cheapest_move,omitempty"`
	ScoreValue   int      `json:"score_value,omitempty"`
	TopFiles     []string `json:"top_files"`
	Constraints  []string `json:"constraints"`
	Validation   []string `json:"validation"`
}

// Document is the versioned output contract consumed by report adapters.
type Document struct {
	SchemaVersion             string                              `json:"schema_version"`
	Verdict                   Verdict                             `json:"verdict"`
	Base                      string                              `json:"base"`
	Head                      string                              `json:"head"`
	ConfigHash                string                              `json:"config_hash,omitempty"`
	Metrics                   []MetricResult                      `json:"metrics"`
	Findings                  []Finding                           `json:"findings"`
	FileFacts                 []evidence.FileFact                 `json:"file_facts"`
	DynamicImports            []evidence.DynamicImport            `json:"dynamic_imports"`
	Connascence               *evidence.ConnascenceReport         `json:"connascence,omitempty"`
	DynamicConnascenceSignals *evidence.DynamicConnascenceSignals `json:"dynamic_connascence_signals,omitempty"`
	RuntimeAsync              []evidence.RuntimeAsyncModule       `json:"runtime_async,omitempty"`
	RuntimeAsyncEdges         []evidence.RuntimeAsyncEdge         `json:"runtime_async_edges,omitempty"`
	DeprecatedDeps            []evidence.DeprecatedDep            `json:"deprecated_deps,omitempty"`
	SemanticStrengthOverlay   *evidence.SemanticStrengthOverlay   `json:"semantic_strength_overlay,omitempty"`
	SyntaxFacts               []evidence.SyntaxFact               `json:"syntax_facts,omitempty"`
	AgentTasks                []AgentTask                         `json:"agent_tasks"`
	AdvisoryTasks             []AdvisoryTask                      `json:"advisory_tasks"`
	ToolCoverage              []evidence.Coverage                 `json:"tool_coverage"`
	CoverageGaps              []evidence.CoverageGap              `json:"coverage_gaps,omitempty"`
	OwnerSource               string                              `json:"owner_source,omitempty"`
	PrimaryExtractorTools     []string                            `json:"primary_extractor_tools,omitempty"`
	ConfigWarnings            []string                            `json:"config_warnings,omitempty"`
	ClassifiedEdges           *ClassifiedEdgeSummary              `json:"classified_edges,omitempty"`
	DistanceContext           *evidence.DistanceContext           `json:"distance_context,omitempty"`
	DistanceConfigCandidates  []evidence.DistanceConfigCandidate  `json:"distance_config_candidates,omitempty"`
	VolatilityCorroboration   *evidence.VolatilityCorroboration   `json:"volatility_corroboration,omitempty"`
	LocalCoupling             []evidence.LocalCouplingModule      `json:"local_coupling,omitempty"`
	GitFindingDelta           *GitFindingDelta                    `json:"git_finding_delta,omitempty"`
	Delta                     *DeltaReport                        `json:"delta,omitempty"`
	Summary                   Summary                             `json:"summary"`
}

// Git comparison statuses describe whether every current repair task has a
// definite origin relative to the selected base.
const (
	GitComparisonComparable = "comparable"
	GitComparisonUnknown    = "unknown"
)

// NewDocument returns an empty document with deterministic non-nil collections.
func NewDocument() Document {
	return Document{
		SchemaVersion:  SchemaVersion,
		Metrics:        []MetricResult{},
		Findings:       []Finding{},
		FileFacts:      []evidence.FileFact{},
		DynamicImports: []evidence.DynamicImport{},
		AgentTasks:     []AgentTask{},
		AdvisoryTasks:  []AdvisoryTask{},
		ToolCoverage:   []evidence.Coverage{},
	}
}
