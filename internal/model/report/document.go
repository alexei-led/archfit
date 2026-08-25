package report

// SchemaVersion identifies the external report document contract.
const SchemaVersion = "archfit.diagnostic.v2"

// AgentTask is the structured repair task for an active gate finding.
type AgentTask struct {
	FindingID    string       `json:"finding_id"`
	RuleID       string       `json:"rule_id"`
	Goal         string       `json:"goal"`
	Constraints  []string     `json:"constraints"`
	Files        []string     `json:"files"`
	Validation   []string     `json:"validation"`
	Declarations []SyntaxFact `json:"declarations,omitempty"`
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
	SchemaVersion             string                     `json:"schema_version"`
	Verdict                   Verdict                    `json:"verdict"`
	Base                      string                     `json:"base"`
	Head                      string                     `json:"head"`
	ConfigHash                string                     `json:"config_hash,omitempty"`
	Metrics                   []MetricResult             `json:"metrics"`
	Findings                  []Finding                  `json:"findings"`
	FileFacts                 []FileFact                 `json:"file_facts"`
	DynamicImports            []DynamicImport            `json:"dynamic_imports"`
	Connascence               *ConnascenceReport         `json:"connascence,omitempty"`
	DynamicConnascenceSignals *DynamicConnascenceSignals `json:"dynamic_connascence_signals,omitempty"`
	RuntimeAsync              []RuntimeAsyncModule       `json:"runtime_async,omitempty"`
	RuntimeAsyncEdges         []RuntimeAsyncEdge         `json:"runtime_async_edges,omitempty"`
	DeprecatedDeps            []DeprecatedDep            `json:"deprecated_deps,omitempty"`
	SemanticStrengthOverlay   *SemanticStrengthOverlay   `json:"semantic_strength_overlay,omitempty"`
	SyntaxFacts               []SyntaxFact               `json:"syntax_facts,omitempty"`
	AgentTasks                []AgentTask                `json:"agent_tasks"`
	AdvisoryTasks             []AdvisoryTask             `json:"advisory_tasks"`
	ToolCoverage              []Coverage                 `json:"tool_coverage"`
	CoverageGaps              []CoverageGap              `json:"coverage_gaps,omitempty"`
	OwnerSource               string                     `json:"owner_source,omitempty"`
	PrimaryExtractorTools     []string                   `json:"primary_extractor_tools,omitempty"`
	ConfigWarnings            []string                   `json:"config_warnings,omitempty"`
	ClassifiedEdges           *ClassifiedEdgeSummary     `json:"classified_edges,omitempty"`
	DistanceContext           *DistanceContext           `json:"distance_context,omitempty"`
	DistanceConfigCandidates  []DistanceConfigCandidate  `json:"distance_config_candidates,omitempty"`
	VolatilityCorroboration   *VolatilityCorroboration   `json:"volatility_corroboration,omitempty"`
	LocalCoupling             []LocalCouplingModule      `json:"local_coupling,omitempty"`
	GitFindingDelta           *GitFindingDelta           `json:"git_finding_delta,omitempty"`
	Delta                     *DeltaReport               `json:"delta,omitempty"`
	Summary                   Summary                    `json:"summary"`

	// Score is the projected scorecard, consumed by renderers. It is not part of
	// the raw JSON diagnostic envelope (jsonout re-emits it under its own "score"
	// key), so it is tagged json:"-".
	Score Scorecard `json:"-"`
	// BaseScore is the optional --base scorecard used to compute the score delta.
	BaseScore *Scorecard `json:"-"`
	// Decision is the projected human-decision view model consumed by the console
	// and markdown decision renderers. It is not part of the raw JSON envelope.
	Decision Report `json:"-"`
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
		FileFacts:      []FileFact{},
		DynamicImports: []DynamicImport{},
		AgentTasks:     []AgentTask{},
		AdvisoryTasks:  []AdvisoryTask{},
		ToolCoverage:   []Coverage{},
	}
}
