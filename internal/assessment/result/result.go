// Package result contains assessment output before external report projection.
package result

import (
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/state"
	"github.com/alexei-led/archfit/internal/model/evidence"
)

// SchemaVersion identifies the current output-compatible result schema.
// It is duplicated at the application/report boundary so assessment does not
// depend on the external report DTO package.
const SchemaVersion = "archfit.diagnostic.v2"

// AgentTask is the assessment repair task before report projection.
type AgentTask struct {
	FindingID    string                `json:"finding_id"`
	RuleID       string                `json:"rule_id"`
	Goal         string                `json:"goal"`
	Constraints  []string              `json:"constraints"`
	Files        []string              `json:"files"`
	Validation   []string              `json:"validation"`
	Declarations []evidence.SyntaxFact `json:"declarations,omitempty"`
}

// AdvisoryTask is the assessment advisory task before report projection.
type AdvisoryTask struct {
	FindingID    string           `json:"finding_id"`
	RuleID       string           `json:"rule_id"`
	Status       finding.Status   `json:"status"`
	Severity     finding.Severity `json:"severity"`
	GroupCount   int              `json:"group_count"`
	GroupMembers []string         `json:"group_members,omitempty"`
	Goal         string           `json:"goal"`
	CheapestMove string           `json:"cheapest_move,omitempty"`
	ScoreValue   int              `json:"score_value,omitempty"`
	TopFiles     []string         `json:"top_files"`
	Constraints  []string         `json:"constraints"`
	Validation   []string         `json:"validation"`
}

// Result is the assessment result before report projection.
type Result struct {
	SchemaVersion             string                              `json:"schema_version"`
	Verdict                   Verdict                             `json:"verdict"`
	Base                      string                              `json:"base"`
	Head                      string                              `json:"head"`
	ConfigHash                string                              `json:"config_hash,omitempty"`
	Metrics                   []MetricResult                      `json:"metrics"`
	Findings                  []finding.Finding                   `json:"findings"`
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
	// Seams is the logical coupling seam ledger, one record per ordered module
	// pair. It is `json:"-"` for the same reason State is: the diagnostic
	// schema is frozen, and the seam ledger reaches the wire through the
	// architecture-state contract.
	Seams                    []Seam                             `json:"-"`
	DistanceContext          *evidence.DistanceContext          `json:"distance_context,omitempty"`
	DistanceConfigCandidates []evidence.DistanceConfigCandidate `json:"distance_config_candidates,omitempty"`
	VolatilityCorroboration  *evidence.VolatilityCorroboration  `json:"volatility_corroboration,omitempty"`
	LocalCoupling            []evidence.LocalCouplingModule     `json:"local_coupling,omitempty"`
	GitFindingDelta          *GitFindingDelta                   `json:"git_finding_delta,omitempty"`
	Delta                    *DeltaReport                       `json:"delta,omitempty"`
	Summary                  Summary                            `json:"summary"`
	// State is the architecture-state result. It rides the diagnostic rather
	// than a parallel return value so every stage that already carries the
	// diagnostic carries the state too. It is `json:"-"` because the diagnostic
	// schema is frozen: the state reaches the wire through its own contract in
	// the report projection, not by widening this one.
	State state.Architecture `json:"-"`
}

// New returns an empty assessment result with deterministic collections.
func New() Result {
	return Result{
		SchemaVersion:  SchemaVersion,
		Metrics:        []MetricResult{},
		Findings:       []finding.Finding{},
		FileFacts:      []evidence.FileFact{},
		DynamicImports: []evidence.DynamicImport{},
		AgentTasks:     []AgentTask{},
		AdvisoryTasks:  []AdvisoryTask{},
		ToolCoverage:   []evidence.Coverage{},
		State:          state.New(),
	}
}
