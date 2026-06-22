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

// RuntimeAsyncSite is one detected async integration pattern location.
// Produced by the runtime detector (internal/extract/runtime); translated to this
// model type in cmd so the core ring never imports an adapter package.
type RuntimeAsyncSite struct {
	File            string `json:"file"`
	Line            int    `json:"line"`
	Library         string `json:"library"`
	IntegrationKind string `json:"integration_kind"` // "message_queue" | "event_bus" | "async_task"
	Language        string `json:"language"`
}

// RuntimeAsyncModule is a per-module rollup of detected async integration patterns.
// Report-only evidence — never consumed by verdict or gate logic.
type RuntimeAsyncModule struct {
	Module          string `json:"module"`
	IntegrationKind string `json:"integration_kind"` // "message_queue" | "event_bus" | "async_task"
	Count           int    `json:"count"`            // number of detected signals
	Confidence      string `json:"confidence"`       // "low" | "medium"
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

// CoverageGap is a machine-readable record of one analyzer that did not run,
// the metrics its absence leaves unmeasured, and the command to install it.
// Populated in cmd/ from the absent ToolCoverage entries plus a static
// tool→metrics map — the core ring never sees tool names beyond coverage facts.
// It is the warn-loud counterpart to a Coverage{Status:"absent"} entry: it
// turns "this tool is missing" into "here is what you lose and how to fix it".
type CoverageGap struct {
	// Tool is the absent analyzer's coverage name (e.g. "go/packages", "lizard").
	Tool string `json:"tool"`
	// InstallCmd is a one-line install hint for the tool.
	InstallCmd string `json:"install_cmd"`
	// AffectedMetrics names the metrics that drop to n/a (or lose confidence)
	// because this tool did not run. Deterministic, fixed order.
	AffectedMetrics []string `json:"affected_metrics"`
	// Gate is the effective gate posture for this gap: "off", "warn", or "fail".
	// Defaults to warn (warn-loud); a "fail" gate is what an opt-in hard gate
	// (tools.<x>.gate: fail / --require-tools) sets to block CI on the gap.
	Gate string `json:"gate"`
}

// DeltaReport groups a delta run's findings by how they relate to the baseline
// and the changed-file set, so a reviewer can separate what this change
// introduced, resolved, or merely touched from pre-existing issues — instead of
// a delta run reading like a full-repo dump. Each slice holds finding IDs that
// join back to findings[]; buckets are mutually exclusive. Populated in delta
// mode only; the whole block is omitted otherwise (pointer + omitempty).
type DeltaReport struct {
	// New holds findings absent from the baseline (introduced by this change).
	New []string `json:"new,omitempty"`
	// Existing holds baseline findings still present and not on a changed file.
	Existing []string `json:"existing,omitempty"`
	// Resolved holds baseline findings no longer detected (status fixed).
	Resolved []string `json:"resolved,omitempty"`
	// SeverityChanged holds baseline findings whose severity differs from the
	// severity recorded in the baseline.
	SeverityChanged []string `json:"severity_changed,omitempty"`
	// TouchedByDelta holds pre-existing findings on a file this change touched —
	// debt a reviewer is well-placed to clear while already in the file.
	TouchedByDelta []string `json:"touched_by_delta,omitempty"`
}

// Coverage status constants used across all extractor adapters.
const (
	StatusOK       = "ok"
	StatusPartial  = "partial"
	StatusAbsent   = "absent"
	StatusDisabled = "disabled" // tool is present but turned off in config
)

// SchemaVersion is the fixed schema_version value emitted in every diagnostic.
// The schema is additive within a major version: new optional fields (e.g. the
// delta block) are introduced without a version bump, so JSON consumers must
// ignore unknown fields — do not decode a v1 payload with DisallowUnknownFields.
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
	// RuntimeAsync is the report-only async-bridge detection block.
	// Evidence only — never consumed by classify, score, or gate logic; never
	// annotates graph edges and never affects distance, score, or verdict.
	// Empty when no async patterns were detected.
	RuntimeAsync []RuntimeAsyncModule `json:"runtime_async,omitempty"`
	AgentTasks   []AgentTask          `json:"agent_tasks"`
	ToolCoverage []Coverage           `json:"tool_coverage"`
	// CoverageGaps lists analyzers that did not run, the metrics their absence
	// leaves unmeasured, and how to install them (warn-loud coverage reporting).
	// Omitted when every required tool ran. Populated in cmd/, never the core ring.
	CoverageGaps []CoverageGap `json:"coverage_gaps,omitempty"`
	// PrimaryExtractorTools names the per-language file extractors whose coverage
	// the scorecard treats as load-bearing: their absence (when coverage is n/a)
	// means the repo was not analysed at all and drives analysis_confidence toward
	// critical. Injected by the composition root from the language registry so the
	// score package holds no hardcoded tool list. Omitted when empty; score then
	// falls back to its built-in default set.
	PrimaryExtractorTools []string `json:"primary_extractor_tools,omitempty"`
	// ConfigWarnings carries advisory config-quality messages (under-specified
	// modules, swallowed optional-tool errors) so they reach md/json/CI instead
	// of being stderr-only. Omitted when empty. Advisory — never gates.
	ConfigWarnings []string `json:"config_warnings,omitempty"`
	// Delta groups findings by lifecycle bucket (new/existing/resolved/
	// severity_changed/touched_by_delta) for a delta run. Nil (omitted) outside
	// delta mode and when the run produced no findings to bucket.
	Delta   *DeltaReport `json:"delta,omitempty"`
	Summary Summary      `json:"summary"`
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
