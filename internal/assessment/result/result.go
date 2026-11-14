// Package result contains assessment output before external report projection.
package result

import (
	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/assessment/state"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
)

// SchemaVersion identifies the current output-compatible result schema.
// It is duplicated at the application/report boundary so assessment does not
// depend on the external report DTO package.
const SchemaVersion = "archfit.diagnostic.v2"

// Result is the assessment result before report projection.
type Result struct {
	SchemaVersion             string                              `json:"schema_version"`
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
	SyntaxFacts               []evidence.SyntaxFact               `json:"syntax_facts,omitempty"`
	ToolCoverage              []evidence.Coverage                 `json:"tool_coverage"`
	CoverageGaps              []evidence.CoverageGap              `json:"coverage_gaps,omitempty"`
	OwnerSource               string                              `json:"owner_source,omitempty"`
	PrimaryExtractorTools     []string                            `json:"primary_extractor_tools,omitempty"`
	ConfigWarnings            []string                            `json:"config_warnings,omitempty"`
	// ModelHash fingerprints the module model.
	ModelHash          string `json:"-"`
	ClassificationHash string `json:"-"`

	ClassifiedEdges *ClassifiedEdgeSummary `json:"classified_edges,omitempty"`
	// ModuleGraphComplexity is an assessment-internal summary of the relationship
	// graph. The architecture-state complexity envelope is its only wire form.
	ModuleGraphComplexity *ModuleGraphComplexity `json:"-"`
	// Seams is the logical coupling seam ledger, one record per ordered module
	// pair. It reaches the wire only through the architecture-state contract.
	Seams                    []Seam                             `json:"-"`
	DistanceContext          *evidence.DistanceContext          `json:"distance_context,omitempty"`
	DistanceConfigCandidates []evidence.DistanceConfigCandidate `json:"distance_config_candidates,omitempty"`
	LocalCoupling            []evidence.LocalCouplingModule     `json:"local_coupling,omitempty"`
	Summary                  Summary                            `json:"summary"`
	// State contains measurement envelopes for the canonical report projection.
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
		ToolCoverage:   []evidence.Coverage{},
		State:          state.New(),
	}
}
