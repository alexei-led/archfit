package diagnostic

import "github.com/alexei-led/archfit/internal/model/finding"

// Verdict is the top-level pass/fail/warn outcome of an archfit run (spec §12).
type Verdict string

// Verdict constants.
const (
	VerdictPass Verdict = "pass"
	VerdictFail Verdict = "fail"
	VerdictWarn Verdict = "warn"
)

// MetricResult holds the computed value and metadata for a single metric (spec §10).
// JSON tags match spec §10 field names exactly.
type MetricResult struct {
	Name       string   `json:"name"`
	Value      float64  `json:"value"`
	Display    string   `json:"display"`
	Band       string   `json:"band"`
	Confidence string   `json:"confidence"`
	Version    string   `json:"metric_version"`
	Mode       string   `json:"mode"`
	Definition string   `json:"definition"`
	Delta      *float64 `json:"delta,omitempty"`
}

// MetricSnapshot is the baseline snapshot of metric values keyed by metric name.
// Stored in the baseline file so delta can be computed on the next run.
type MetricSnapshot map[string]struct {
	Value   float64 `json:"value"`
	Version string  `json:"version"`
}

// Summary holds the gate/warning/exception counts for the top-level summary block (spec §12).
type Summary struct {
	GateFindings   int `json:"gate_findings"`
	Warnings       int `json:"warnings"`
	ExceptionsUsed int `json:"exceptions_used"`
}

// Coverage records what a single tool extracted (spec §12 tool_coverage entry).
type Coverage struct {
	Tool            string `json:"tool"`
	Version         string `json:"version"`
	FilesSeen       int    `json:"files_seen"`
	FilesApplicable int    `json:"files_applicable"`
	Unresolved      int    `json:"unresolved"`
	Status          string `json:"status"`
}

// AgentTask is a placeholder for the structured repair-task block (spec §13 / Phase 4).
// Phase 1 emits an empty typed slice so that agent_tasks serializes as [] not null.
type AgentTask struct{}

// Coverage status constants used across all extractor adapters.
const (
	StatusOK      = "ok"
	StatusPartial = "partial"
	StatusAbsent  = "absent"
)

// SchemaVersion is the fixed schema_version value emitted in every diagnostic.
const SchemaVersion = "archfit.diagnostic.v1"

// Diagnostic is the top-level output contract for archfit check (spec §12).
// JSON tags match spec §12 field names exactly.
type Diagnostic struct {
	SchemaVersion string            `json:"schema_version"`
	Verdict       Verdict           `json:"verdict"`
	Base          string            `json:"base"`
	Head          string            `json:"head"`
	Metrics       []MetricResult    `json:"metrics"`
	Findings      []finding.Finding `json:"findings"`
	AgentTasks    []AgentTask       `json:"agent_tasks"`
	ToolCoverage  []Coverage        `json:"tool_coverage"`
	Summary       Summary           `json:"summary"`
}

// New returns a zero-value Diagnostic with all required fields initialised to their
// empty (non-null) forms: schema_version set, slices allocated as empty (not nil).
func New() Diagnostic {
	return Diagnostic{
		SchemaVersion: SchemaVersion,
		Metrics:       []MetricResult{},
		Findings:      []finding.Finding{},
		AgentTasks:    []AgentTask{},
		ToolCoverage:  []Coverage{},
	}
}
