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
	// Reason explains why a headline metric is absent or partial — a missing
	// tool, an opt-in-off setting, or an uninstalled dependency — and how to
	// enable it (the actionable next step). Empty when status is ok. A static,
	// deterministic string so a double-run stays byte-identical.
	Reason string `json:"reason,omitempty"`
}

// AgentTask is the structured repair-task block (spec §13): one per ACTIVE gate
// finding (status new/expired_exception), derived deterministically from the
// finding + rule configuration — no fabrication. It tells a coding agent what
// to achieve, within which constraints, where, and how to verify.
type AgentTask struct {
	// FindingID joins the task back to its findings[] entry.
	FindingID string `json:"finding_id"`
	RuleID    string `json:"rule_id"`
	// Goal is the repair objective, instantiated from the rule type's template
	// with the finding's edge endpoints.
	Goal string `json:"goal"`
	// Constraints are hard boundaries for the fix: the rule's constraint text,
	// allowed alternatives, and the target module's public surface.
	Constraints []string `json:"constraints"`
	// Files are the repo-relative files involved (edge endpoints + locations).
	Files []string `json:"files"`
	// Validation are the exact commands that must pass after the fix.
	Validation []string `json:"validation"`
}

// FileFact holds neutral per-module structural facts assembled from collected
// data (symbol graph, file LOC, co-change history, optional gitnexus impact).
//
// The facts block is report-only evidence for the Tranche-2 LLM: it carries no
// band, no score, no risk label, never sets delta, and never enters the verdict
// or gate logic. Ranking and judgment are the LLM's job.
type FileFact struct {
	// Module is the symbol-graph module key (dotted for Python, package dir for Go).
	Module string `json:"module"`
	// Files lists the repo-relative source files that define this module's
	// symbols, sorted. Empty when the symbol graph carries no path data.
	Files []string `json:"files"`
	// InboundModuleFanIn counts distinct OTHER modules whose symbols reference
	// this module's symbols. Read-only config scores high here too — separating
	// benign config from mutable shared state is the LLM's job.
	InboundModuleFanIn int `json:"inbound_module_fanin"`
	// OutboundDestinations counts distinct destination modules this module's
	// symbols reference, at raw module granularity (no parent-package collapse).
	OutboundDestinations int `json:"outbound_destinations"`
	// LOC is the summed line count of Files (exact join against FileLOC keys).
	LOC int `json:"loc"`
	// CoChangePartners lists files outside this module most frequently committed
	// together with this module's files, count descending, capped.
	CoChangePartners []string `json:"cochange_partners"`
	// GitnexusImpact is the historical change-impact count for this module when
	// gitnexus enrichment ran and covered it; nil otherwise (never fabricated).
	GitnexusImpact *int `json:"gitnexus_impact,omitempty"`
}

// DynamicImport is the report-only dynamic/lazy-import risk signal for one module
// (Task 9). Dynamic/lazy imports — Python non-top-level (in-function) imports,
// importlib.import_module / __import__, and TS require() / dynamic import() — are
// invisible to the static dependency graph, so they hide cycles and undercount
// coupling. This block is evidence only: it carries no band, no score, never
// enters the verdict or any gate, and never modifies the dependency graph or any
// metric. Ranking and judgment are the off-gate LLM's job.
type DynamicImport struct {
	// Module is the module-map key that owns the sites, or the file's directory
	// when the module map does not cover them.
	Module string `json:"module"`
	// Count is the total dynamic/lazy import sites found in this module.
	Count int `json:"count"`
	// Sites is a deterministic, capped sample of the underlying sites.
	Sites []DynamicImportSite `json:"sites"`
}

// DynamicImportSite is one dynamic/lazy import occurrence at a file location.
// Kind is one of: lazy_import, importlib, require, dynamic_import.
type DynamicImportSite struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Kind     string `json:"kind"`
	Language string `json:"language"`
}

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
	SchemaVersion string  `json:"schema_version"`
	Verdict       Verdict `json:"verdict"`
	Base          string  `json:"base"`
	Head          string  `json:"head"`
	// ConfigHash is the sha256 hex digest of the loaded .archfit.yaml bytes.
	// Empty when no config file was loaded (--no-config or default built-in).
	// Reproducibility: same config + same repo state → same ConfigHash.
	ConfigHash string            `json:"config_hash,omitempty"`
	Metrics    []MetricResult    `json:"metrics"`
	Findings   []finding.Finding `json:"findings"`
	// FileFacts is the neutral per-module structural-facts block (Tranche 1.5).
	// Report-only evidence — never consumed by verdict or gate logic. Empty when
	// no symbol graph was collected (SCIP off/absent).
	FileFacts []FileFact `json:"file_facts"`
	// DynamicImports is the report-only dynamic/lazy-import risk block (Task 9).
	// Evidence only — never consumed by verdict or gate logic, never alters the
	// dependency graph or any metric. Empty when no dynamic imports were found.
	DynamicImports []DynamicImport `json:"dynamic_imports"`
	AgentTasks     []AgentTask     `json:"agent_tasks"`
	ToolCoverage   []Coverage      `json:"tool_coverage"`
	Summary        Summary         `json:"summary"`
}

// New returns a zero-value Diagnostic with all required fields initialised to their
// empty (non-null) forms: schema_version set, slices allocated as empty (not nil).
func New() Diagnostic {
	return Diagnostic{
		SchemaVersion:  SchemaVersion,
		Metrics:        []MetricResult{},
		Findings:       []finding.Finding{},
		FileFacts:      []FileFact{},
		DynamicImports: []DynamicImport{},
		AgentTasks:     []AgentTask{},
		ToolCoverage:   []Coverage{},
	}
}
