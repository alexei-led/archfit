package diagnostic

import (
	"sort"

	"github.com/alexei-led/archfit/internal/model/finding"
)

// SyntaxFact holds one syntactic declaration or route registration extracted
// by ast-grep from a source file. It is a neutral, score-free, off-gate fact
// (like FileFact) that surfaces in scan/review output and agent_tasks evidence.
// Fields per design §3.
type SyntaxFact struct {
	Language           string `json:"language"`         // go|typescript|python|rust
	File               string `json:"file"`             // repo-relative, slash
	Module             string `json:"module,omitempty"` // module-map key this file belongs to; empty when outside declared modules
	Kind               string `json:"kind"`             // function|method|class|struct|interface|trait|enum|type_alias|annotation|route
	Name               string `json:"name"`
	Exported           bool   `json:"exported,omitempty"`
	StartLine          int    `json:"start_line"`
	EndLine            int    `json:"end_line,omitempty"`
	Count              int    `json:"count,omitempty"`     // for kindStructField: estimated field count from line range (endLine - startLine - 1)
	Framework          string `json:"framework,omitempty"` // for routes: gin|fastapi|express|axum|…
	FrameworkConfirmed bool   `json:"-"`                   // true when the file imports the route framework; set by the adapter, never serialised
}

// SortSyntaxFacts sorts in place by (File, StartLine, Kind, Name).
// sort.SliceStable preserves input order among facts that are equal on all
// four keys, guaranteeing deterministic golden output.
func SortSyntaxFacts(facts []SyntaxFact) {
	sort.SliceStable(facts, func(i, j int) bool {
		a, b := facts[i], facts[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.StartLine != b.StartLine {
			return a.StartLine < b.StartLine
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Name < b.Name
	})
}

// Verdict is the top-level pass/fail/warn outcome of an archfit run (spec §12).
type Verdict string

// Verdict constants.
const (
	VerdictPass Verdict = "pass"
	VerdictFail Verdict = "fail"
	VerdictWarn Verdict = "warn"
)

// Direction records whether a rising metric value is an improvement or a
// regression. It is a property of the metric's definition, not a user choice
// (Technical Details, docs/plans/completed/20260702-wave1-gate-integrity.md): the metric
// that produces a MetricResult stamps its own Direction, and computeVerdict
// reads it to interpret Delta's sign instead of assuming ratio semantics for
// every metric.
type Direction string

// Direction constants.
const (
	DirectionHigherIsBetter Direction = "higher_is_better"
	DirectionHigherIsWorse  Direction = "higher_is_worse"
)

// MetricResult holds the computed value and metadata for a single metric (spec §10).
// JSON tags match spec §10 field names exactly.
type MetricResult struct {
	Name       string    `json:"name"`
	Value      float64   `json:"value"`
	Display    string    `json:"display"`
	Band       string    `json:"band"`
	Confidence string    `json:"confidence"`
	Version    string    `json:"metric_version"`
	Mode       string    `json:"mode"`
	Definition string    `json:"definition"`
	Delta      *float64  `json:"delta,omitempty"`
	Direction  Direction `json:"direction,omitempty"`
}

// MetricSnapshot is the baseline snapshot of metric values keyed by metric name.
// Stored in the baseline file so delta can be computed on the next run.
type MetricSnapshot map[string]struct {
	Value   float64 `json:"value"`
	Version string  `json:"version"`
}

// Summary holds the gate/warning/exception counts for the top-level summary block (spec §12).
type Summary struct {
	GateFindings int `json:"gate_findings"`
	Warnings     int `json:"warnings"`
	WaiversUsed  int `json:"waivers_used"`
}

// Coverage records what a single tool extracted (spec §12 tool_coverage entry).
type Coverage struct {
	Tool            string `json:"tool"`
	Version         string `json:"version"`
	FilesSeen       int    `json:"files_seen"`
	FilesApplicable int    `json:"files_applicable"`
	Unresolved      int    `json:"unresolved"`
	// SpecifiersSeen is the total import-specifier count the extractor
	// examined — the denominator that makes Unresolved a ratio. Only
	// specifier-granular extractors (dependency-cruiser) set it; 0 means
	// "not tracked", never "no specifiers".
	SpecifiersSeen int    `json:"specifiers_seen,omitempty"`
	Status         string `json:"status"`
	// Reason explains why a headline metric is absent or partial — a missing
	// tool, an opt-in-off setting, or an uninstalled dependency — and how to
	// enable it (the actionable next step). Empty when status is ok. A static,
	// deterministic string so a double-run stays byte-identical.
	Reason string `json:"reason,omitempty"`
}

// SemanticStrengthOverlay records how often SCIP semantic strength refined
// extractor edges. It is report-only evidence: gates and scores consume the
// resulting edge strengths through classification, never these counters.
type SemanticStrengthOverlay struct {
	ByLanguage map[string]SemanticStrengthOverlayStats `json:"by_language,omitempty"`
}

// SemanticStrengthOverlayStats is the per-language SCIP overlay hit/miss summary.
type SemanticStrengthOverlayStats struct {
	CandidateEdges int            `json:"candidate_edges"`
	Applied        int            `json:"applied"`
	Missed         int            `json:"missed"`
	Before         map[string]int `json:"before,omitempty"`
	After          map[string]int `json:"after,omitempty"`
}

// AgentTask is the structured repair-task block (spec §13): one per ACTIVE gate
// finding (status new/expired_waiver), derived deterministically from the
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
	// Declarations holds the syntax declarations found in the referenced files
	// (name, kind, exported, role, file:line). Populated only when syntax facts
	// are present (analyzers.syntax.enabled: true); absent otherwise — no empty slice,
	// no JSON key emitted (omitempty ensures byte-for-byte parity with prior runs).
	Declarations []SyntaxFact `json:"declarations,omitempty"`
}

// AdvisoryTask is a report-only remediation prompt for grouped advisory
// findings. Unlike AgentTask it never gates and never changes verdict status;
// it just turns a deterministic rollup into a smaller human/agent work item.
type AdvisoryTask struct {
	// FindingID joins the task back to its grouped findings[] advisory.
	FindingID string           `json:"finding_id"`
	RuleID    string           `json:"rule_id"`
	Status    finding.Status   `json:"status"`
	Severity  finding.Severity `json:"severity"`
	// GroupCount is the true number of advisory edges represented by the rollup.
	GroupCount int `json:"group_count"`
	// GroupMembers is the capped representative member ID list from matched_by.
	GroupMembers []string `json:"group_members,omitempty"`
	// Goal is a deterministic advisory objective, not a gate repair order.
	Goal string `json:"goal"`
	// CheapestMove carries the scorer's lowest-cost improvement hint when known.
	CheapestMove string `json:"cheapest_move,omitempty"`
	// ScoreValue carries the 1-10 Balanced-Coupling effort/risk score when known.
	ScoreValue int `json:"score_value,omitempty"`
	// TopFiles are representative repo-relative files from the rolled-up locations.
	TopFiles []string `json:"top_files"`
	// Constraints keep the advisory task inside report-only semantics.
	Constraints []string `json:"constraints"`
	// Validation are the commands that confirm the report stayed healthy.
	Validation []string `json:"validation"`
}

// FileFact holds neutral per-module structural facts assembled from the symbol
// graph and file LOC.
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
}

// ConnascenceRoadmapItem explains one Ch6 connascence category's measurement
// posture. It is report-only documentation in the machine output: a stable
// checklist of what archfit measures deterministically today, what remains
// unmeasured, and which separate report-only signals may inform human review.
type ConnascenceRoadmapItem struct {
	// Kind is a book connascence category: name, type, meaning, algorithm,
	// position, execution, timing, value, or identity.
	Kind string `json:"kind"`
	// CurrentStatus is deterministic_static, unmeasured_static, or
	// unmeasured_dynamic. It is descriptive only; no scorer or gate consumes it.
	CurrentStatus string `json:"current_status"`
	// Sources names deterministic sources that can support this category today.
	Sources []string `json:"sources,omitempty"`
	// RelatedSignals names separate report-only blocks that may help a human
	// review this category but are not connascence measurements.
	RelatedSignals []string `json:"related_signals,omitempty"`
	// UpgradeTrigger names the evidence needed before the category can become a
	// deterministic measurement.
	UpgradeTrigger string `json:"upgrade_trigger,omitempty"`
}

// ConnascenceReport summarizes deterministic static connascence evidence from
// classified dependency edges. It is report-only: never consumed by scoring,
// rules, baseline deltas, or gate verdicts.
type ConnascenceReport struct {
	// EdgesWithEvidence counts classified edges with at least one deterministic
	// connascence fact.
	EdgesWithEvidence int `json:"edges_with_evidence"`
	// AbstainedEdges counts classified edges with no deterministic connascence fact.
	AbstainedEdges int `json:"abstained_edges"`
	// TotalEvidence counts individual connascence facts. A single edge may carry
	// multiple facts, e.g. name + type.
	TotalEvidence int `json:"total_evidence"`
	// ByKind counts evidence by Ch6 static category: name, type, meaning,
	// algorithm, and position when deterministically measured.
	ByKind map[string]int `json:"by_kind,omitempty"`
	// BySource counts evidence by deterministic source, such as go/types,
	// dependency-cruiser, grimp, or scip.
	BySource map[string]int `json:"by_source,omitempty"`
	// Unmeasured names book categories not measured by deterministic evidence in
	// this run. These are disclosed rather than guessed.
	Unmeasured []string `json:"unmeasured,omitempty"`
	// Roadmap is the dynamic connascence roadmap: deterministic static categories,
	// unmeasured categories, and review-only related signals. It is disclosure
	// only and never feeds score, findings, baselines, or gates.
	Roadmap []ConnascenceRoadmapItem `json:"roadmap,omitempty"`
}

// DynamicConnascenceSignals is a report-only bridge from deterministic static
// sites (dynamic imports and runtime async integrations) to the Ch6 dynamic
// connascence categories they may help humans inspect. The signals are not
// measurements: they never feed coupling_balance, findings, baselines, or gates.
type DynamicConnascenceSignals struct {
	Signals          []DynamicConnascenceSignal `json:"signals"`
	Unmeasured       []string                   `json:"unmeasured,omitempty"`
	ReportOnlyReason string                     `json:"report_only_reason"`
}

// DynamicConnascenceSignal is one module/site rollup that points at a possible
// dynamic connascence review area while explicitly marking it as unmeasured.
type DynamicConnascenceSignal struct {
	Kind               string                   `json:"kind"`
	RelatedConnascence []string                 `json:"related_connascence"`
	Measured           bool                     `json:"measured"`
	ReportOnlyReason   string                   `json:"report_only_reason"`
	Module             string                   `json:"module"`
	Target             string                   `json:"target,omitempty"`
	IntegrationKind    string                   `json:"integration_kind,omitempty"`
	Count              int                      `json:"count"`
	Sites              []DynamicConnascenceSite `json:"sites,omitempty"`
}

// DynamicConnascenceSite is a capped sample location behind a dynamic
// connascence signal. Kind is the dynamic-import kind or runtime integration kind.
type DynamicConnascenceSite struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Kind     string `json:"kind"`
	Language string `json:"language"`
	Target   string `json:"target,omitempty"`
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

// RuntimeAsyncEdge is a relationship-level async integration fact from one
// module to one runtime target (library, decorator, or async primitive).
// Report-only prerequisite evidence for a future runtime-distance model; never
// consumed by verdict, gate, classify, score, or baseline logic.
type RuntimeAsyncEdge struct {
	FromModule      string             `json:"from_module"`
	Target          string             `json:"target"`
	IntegrationKind string             `json:"integration_kind"` // "message_queue" | "event_bus" | "async_task"
	Count           int                `json:"count"`            // number of detected signals for this module→target relation
	Confidence      string             `json:"confidence"`       // "low" | "medium"
	Sites           []RuntimeAsyncSite `json:"sites,omitempty"`  // capped deterministic sample; Count is the true total
}

// DeprecatedDep is one locally-declared deprecation or retraction marker found
// in a manifest file. Report-only evidence — never consumed by verdict or gate
// logic, never alters the dependency graph or any metric.
// Ceiling: cargo yanked and live-version EOL require external registry queries
// and are routed to the LLM path (archfit analyze --llm / enrich), not this detector.
type DeprecatedDep struct {
	File    string `json:"file"`           // repo-relative manifest file path
	Kind    string `json:"kind"`           // "retract" | "deprecated"
	Subject string `json:"subject"`        // version range (retract) or package name (deprecated)
	Note    string `json:"note,omitempty"` // optional annotation text from the marker
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

// Dynamic import kind constants are shared vocabulary for dynamic-import
// extractors, reports, and eval fixtures.
const (
	DynamicImportKindLazyImport    = "lazy_import"
	DynamicImportKindImportlib     = "importlib"
	DynamicImportKindRequire       = "require"
	DynamicImportKindDynamicImport = "dynamic_import"
)

// DynamicImportSite is one dynamic/lazy import occurrence at a file location.
// Kind is one of the DynamicImportKind* constants.
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
	// Tool is the absent analyzer's coverage name (e.g. "go/packages", "scip").
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

// DistanceContext explains how distance evidence should be read for this run.
// It is disclosure-only: the scorer consumes per-edge Distance and DistanceBasis,
// never this rollup. The block keeps single-owner repositories honest by saying
// "low socio-technical distance" instead of implying missing evidence.
type DistanceContext struct {
	OwnerModel                string         `json:"owner_model"`
	DistanceBasis             map[string]int `json:"distance_basis,omitempty"`
	DeployUnitDetectedModules int            `json:"deploy_unit_detected_modules,omitempty"`
	DeclaredExternalSystems   int            `json:"declared_external_systems,omitempty"`
	Interpretation            string         `json:"interpretation"`
}

// ClassifiedEdgeSummary holds aggregate distribution counts over the
// coupling.Index produced by classify.Run plus score-bearing clone-only
// duplicated-knowledge pairs when that policy is enabled. Stdlib-only (no
// coupling imports). Populated in engine.go; consumed by score.go to drive
// coupling_balance.
type ClassifiedEdgeSummary struct {
	// Total is the total classified coupling fact count (all graph edges,
	// including same_module, plus scored clone-only pairs when enabled).
	Total int `json:"total"`
	// Scored is the count of cross-boundary edges with a concrete book balance
	// (Scored=true on EdgeScore, i.e. strength and distance both known).
	Scored int `json:"scored"`
	// Abstained is the count of cross-boundary edges where the scorer abstained
	// (strength or distance unknown — excluded from MeanBalance).
	Abstained int `json:"abstained"`
	// SameModule is the count of same_module edges (excluded from the balance aggregate).
	SameModule int `json:"same_module"`
	// MeanBalance is the arithmetic mean of the book balance (1..10) over scored
	// cross-boundary edges. 0.0 when Scored == 0.
	MeanBalance float64 `json:"mean_balance"`
	// TailRisk summarizes the lower tail of scored cross-boundary coupling facts.
	// It sits beside MeanBalance so a healthy average cannot hide a concentrated
	// set of high/critical edges. Nil when no scored cross-boundary facts exist.
	TailRisk *CouplingTailRiskSummary `json:"tail_risk,omitempty"`
	// ByStrength counts cross-boundary edges by strength label (string keys, coupling package values).
	ByStrength map[string]int `json:"by_strength,omitempty"`
	// ByDistance counts cross-boundary edges by distance label.
	ByDistance map[string]int `json:"by_distance,omitempty"`
	// ByDistanceBasis counts cross-boundary edges by the deterministic signal that
	// selected their distance rung: code_structure, ownership, deploy_unit, or
	// declared_external. Unknown/same-module edges have no basis and are omitted.
	ByDistanceBasis map[string]int `json:"by_distance_basis,omitempty"`
	// ByVolatility counts cross-boundary edges by volatility label.
	ByVolatility map[string]int `json:"by_volatility,omitempty"`
	// BySeverity counts cross-boundary edges by score band (severity label).
	BySeverity map[string]int `json:"by_severity,omitempty"`
	// DistributedMonolith counts the genuine distributed-monolith edges: those in
	// the critical band AND at high distance (different owner or deploy unit). The
	// critical band alone is NOT distributed-monolith — a critical edge at
	// cross_module_same_owner is local high-strength/high-volatility coupling whose
	// cascade is cheap (one owner, one binary). Only this count may be framed as
	// "distributed-monolith risk"; it never changes the balance value.
	DistributedMonolith int `json:"distributed_monolith,omitempty"`
	// External is the count of cross-boundary edges whose target is NOT a declared
	// module (Distance == unknown: stdlib, third-party, undeclared packages). These
	// are EXCLUDED from the Scored/Abstained distribution that drives coupling_balance
	// — the book measures coupling among YOUR components, not your libraries.
	// External dependency hygiene is tracked separately and does not affect coupling_balance.
	// This field is language-agnostic: it keys on DistanceUnknown, which classifyDistance
	// sets for all languages (Go stdlib/3p, Rust dependency crates, TS node_modules,
	// Python external imports). Zero means no external edges were detected.
	External int `json:"external,omitempty"`
	// DeclaredExternal is the count of edges whose target matched a config-declared
	// `external_systems:` entry (Distance == declared_external, D=10 — book Ch10
	// Example 1). Unlike External, these edges ENTER the Scored/Abstained
	// distribution: the architect declared the seam, so it is measured. The count
	// keeps the disclosed-exclusion arithmetic honest — External covers only the
	// UNDECLARED remainder.
	DeclaredExternal int `json:"declared_external,omitempty"`
	// ConnectedModules is the number of distinct first-party modules participating
	// in scored/abstained cross-boundary coupling facts. It feeds confidence only:
	// a tiny connected module sample cannot claim high-confidence architecture health.
	ConnectedModules int `json:"connected_modules,omitempty"`
	// CloneOnlyScored counts clone-only duplicated-knowledge pairs included in
	// coupling_balance by coupling.duplicated_knowledge: score. They are
	// score-bearing coupling facts, not graph import edges.
	CloneOnlyScored int `json:"clone_only_scored,omitempty"`
	// CloneOnlyAdvisory counts clone-only duplicated-knowledge pairs held out of
	// coupling_balance by coupling.duplicated_knowledge: advisory. They may still
	// emit bc/duplicated_knowledge advisories after severity/status filtering.
	CloneOnlyAdvisory int `json:"clone_only_advisory,omitempty"`
	// LLMApproved is the count of approved cross-boundary labels whose provenance
	// is "llm" and confidence is not "high". These lower the coupling_balance
	// dimension confidence by one band — they are human-approved but not human-judged.
	// Zero means no LLM-provenance labels are in effect.
	LLMApproved int `json:"llm_approved,omitempty"`
	// LabeledLLM is the count of cross-boundary EDGES whose strength came from
	// an approved llm-provenance label filling a cell every static source left
	// unknown (one label covers all edges of its module pair). It attributes
	// the scored-fraction increase to the semantic layer — disclosure only,
	// never fed into the balance value.
	LabeledLLM int `json:"labeled_llm,omitempty"`
	// LLMLowConfidenceEdges counts cross-boundary edges whose strength was filled
	// by an applied non-high-confidence LLM label. It is intentionally not emitted:
	// score confidence consumes it while labeled_llm remains the user-facing
	// attribution bucket.
	LLMLowConfidenceEdges int `json:"-"`
	// VolatilityProvenance counts MODULES (not edges) by the source of their
	// volatility. Nil when no modules were resolved.
	VolatilityProvenance *VolatilityProvenance `json:"volatility_provenance,omitempty"`
	// DistanceCompression discloses which book distance rungs this deterministic
	// instrumentation implements and which middle rungs remain compressed instead
	// of being guessed. Report-only; never consumed by scoring or gates.
	DistanceCompression *DistanceCompressionSummary `json:"distance_compression,omitempty"`
}

// CouplingTailRiskSummary records lower-tail statistics over scored
// cross-boundary coupling facts. Lower book balance is worse, so WorstBalance
// and LowerDecileBalance expose concentrated hot spots that MeanBalance can hide.
type CouplingTailRiskSummary struct {
	WorstBalance              int `json:"worst_balance"`
	LowerDecileBalance        int `json:"lower_decile_balance"`
	HighOrWorseEdges          int `json:"high_or_worse_edges"`
	HighOrWorseSharePct       int `json:"high_or_worse_share_pct"`
	CriticalEdges             int `json:"critical_edges"`
	DistributedMonolithEdges  int `json:"distributed_monolith_edges"`
	CloneOnlyScored           int `json:"clone_only_scored,omitempty"`
	CloneOnlyHighOrWorseEdges int `json:"clone_only_high_or_worse_edges,omitempty"`
	CloneOnlyWorstBalance     int `json:"clone_only_worst_balance,omitempty"`
}

// DistanceCompressionSummary records archfit's deterministic distance-ladder
// coverage. It makes compressed Ch8 middle rungs visible in JSON/Markdown so a
// D=4/D=7 result is not mistaken for full book precision.
type DistanceCompressionSummary struct {
	CompressedMiddleRungs bool                        `json:"compressed_middle_rungs"`
	ImplementedRungs      []int                       `json:"implemented_rungs,omitempty"`
	OmittedRungs          []int                       `json:"omitted_rungs,omitempty"`
	OmittedRungReasons    []DistanceOmittedRungReason `json:"omitted_rung_reasons,omitempty"`
	DeterministicSplits   []string                    `json:"deterministic_splits,omitempty"`
	Rationale             string                      `json:"rationale,omitempty"`
}

// DistanceOmittedRungReason explains why a book distance rung remains compressed.
type DistanceOmittedRungReason struct {
	Rung   int    `json:"rung"`
	Reason string `json:"reason"`
}

// VolatilityProvenance counts modules by where their volatility came from:
// config-declared (an explicit `volatility:` field or subdomain mapping),
// inherited by an auto-registered synthetic module from its nearest declared
// ancestor, or raised by the opt-in volatility cascade (an overlay on top of
// the base source). Undeclared is the honest remainder — no volatility from
// any source. Disclosure-only: a repo whose edges all carry the same
// volatility must be visibly uniform-by-inheritance (one declared ancestor
// fanned out to N synthetic submodules), not mistaken for N measured
// judgments. Volatility comes from the domain (book Ch9), never from commit
// history, so archfit never derives differentiation — it discloses where the
// labels came from. Never consumed by the balance value or the gate.
type VolatilityProvenance struct {
	// Declared counts config-declared modules whose volatility comes from their
	// own `volatility:` field or subdomain mapping.
	Declared int `json:"declared"`
	// Inherited counts synthetic (auto-registered) modules whose volatility was
	// inherited from the nearest config-declared ancestor.
	Inherited int `json:"inherited"`
	// Cascade counts modules whose EFFECTIVE volatility the opt-in cascade pass
	// raised above their base level. Overlays Declared/Inherited/Undeclared.
	Cascade int `json:"cascade"`
	// Undeclared counts modules with no volatility from any source (scored
	// conservatively by the book scorer, never guessed).
	Undeclared int `json:"undeclared"`
}

// LocalCouplingModule summarises the scored same-module edges of one module —
// the book Ch10 "local complexity" quadrant surface (low strength at low
// distance = low cohesion). Same-module edges score with the book formula at
// the same-module distance rung but NEVER enter coupling_balance's
// Scored/Abstained denominator: cross-module coupling and intra-module
// cohesion are different fractal levels and stay separate reported numbers.
// Report-only — never consumed by verdict or gate logic.
type LocalCouplingModule struct {
	// Module is the module-map key that owns both edge endpoints.
	Module string `json:"module"`
	// ScoredEdges is the count of same-module edges with a concrete book balance.
	ScoredEdges int `json:"scored_edges"`
	// AbstainedEdges counts same-module edges the scorer abstained on (unknown
	// strength) — abstain-not-fake applies at this fractal level too.
	AbstainedEdges int `json:"abstained_edges,omitempty"`
	// ComplexityEdges is the count of scored edges in the local-complexity
	// quadrant (contract/model strength at same-module distance — the
	// "ball of mud" corner).
	ComplexityEdges int `json:"complexity_edges"`
	// ComplexitySharePct is 100×ComplexityEdges/ScoredEdges (0 when unscored).
	ComplexitySharePct int `json:"complexity_share_pct"`
	// MeanBalance is the arithmetic mean book balance (1..10) over scored edges.
	MeanBalance float64 `json:"mean_balance"`
	// WorstOffenders is a deterministic, capped sample of the lowest-balance
	// scored edges (band below none), with a representative source location.
	WorstOffenders []LocalCouplingEdge `json:"worst_offenders,omitempty"`
}

// LocalCouplingEdge is one same-module edge sampled into WorstOffenders.
type LocalCouplingEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Strength string `json:"strength"`
	Balance  int    `json:"balance"`
	Band     string `json:"band"`
	// File/Line is the edge's first source location (e.g. the import site).
	// Omitted when the extractor recorded no location (e.g. TS edges).
	File string `json:"file,omitempty"`
	Line int    `json:"line,omitempty"`
}

// Coverage status constants used across all extractor adapters.
const (
	StatusOK       = "ok"
	StatusPartial  = "partial"
	StatusAbsent   = "absent"
	StatusDisabled = "disabled"  // tool is present but turned off in config
	StatusTimedOut = "timed out" // per-analyzer watchdog fired; result is n/a
)

// SchemaVersion is the fixed schema_version value emitted in every diagnostic.
// The schema is additive within a major version: new optional fields (e.g. the
// delta block) are introduced without a version bump, so JSON consumers must
// ignore unknown fields — do not decode a v1 payload with DisallowUnknownFields.
const SchemaVersion = "archfit.diagnostic.v2"

// Diagnostic is the top-level output contract for archfit analyze (spec §12).
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
	// Connascence summarizes deterministic static connascence evidence. Report-only;
	// semantic/dynamic categories without a deterministic source are listed as
	// unmeasured rather than guessed. Omitted only when classification did not run.
	Connascence *ConnascenceReport `json:"connascence,omitempty"`
	// DynamicConnascenceSignals maps dynamic/lazy imports and runtime async facts
	// to possible Ch6 dynamic connascence categories for human review. Report-only;
	// all entries are measured=false and never change score, findings, or verdicts.
	DynamicConnascenceSignals *DynamicConnascenceSignals `json:"dynamic_connascence_signals,omitempty"`
	// RuntimeAsync is the report-only async-bridge detection block.
	// Evidence only — never consumed by classify, score, or gate logic; never
	// annotates graph edges and never affects distance, score, or verdict.
	// Empty when no async patterns were detected.
	RuntimeAsync []RuntimeAsyncModule `json:"runtime_async,omitempty"`
	// RuntimeAsyncEdges is the relationship-level async-bridge evidence block.
	// It groups concrete sites by source module and runtime target so later work
	// can review runtime distance without re-scanning raw files. Evidence only —
	// never consumed by classify, score, baseline deltas, or gate verdicts.
	RuntimeAsyncEdges []RuntimeAsyncEdge `json:"runtime_async_edges,omitempty"`
	// DeprecatedDeps is the report-only locally-declared deprecation/retraction
	// marker block. Evidence only — never consumed by verdict or gate logic, never
	// alters the dependency graph or any metric. Omitted when no markers were found.
	// Ceiling: cargo yanked and live-version EOL require external registry queries
	// and are routed to the LLM path (archfit analyze --llm / enrich), not here.
	DeprecatedDeps []DeprecatedDep `json:"deprecated_deps,omitempty"`
	// SemanticStrengthOverlay reports SCIP semantic-strength overlay coverage by
	// language. Report-only visibility for the refinement layer; never consumed by
	// verdict, gates, score synthesis, or baseline deltas.
	SemanticStrengthOverlay *SemanticStrengthOverlay `json:"semantic_strength_overlay,omitempty"`
	// SyntaxFacts is the report-only syntactic declaration/route block extracted
	// by ast-grep (design §3). Neutral, off-gate evidence — never consumed by
	// verdict or gate logic. Omitted (omitempty) when analyzers.syntax is off or sg
	// is absent, so absent sg never emits a null/empty block (no false green).
	SyntaxFacts   []SyntaxFact   `json:"syntax_facts,omitempty"`
	AgentTasks    []AgentTask    `json:"agent_tasks"`
	AdvisoryTasks []AdvisoryTask `json:"advisory_tasks"`
	ToolCoverage  []Coverage     `json:"tool_coverage"`
	// CoverageGaps lists analyzers that did not run, the metrics their absence
	// leaves unmeasured, and how to install them (warn-loud coverage reporting).
	// Omitted when every required tool ran. Populated in cmd/, never the core ring.
	CoverageGaps []CoverageGap `json:"coverage_gaps,omitempty"`
	// OwnerSource records where module owners came from for the owner-distance
	// signal: "config" (declared in .archfit.yaml), "codeowners", "git" (author
	// history), or "none" (no owner signal — owner-distance falls back to
	// code-structure). Populated in cmd/. Omitted when empty.
	OwnerSource string `json:"owner_source,omitempty"`
	// PrimaryExtractorTools names the per-language file extractors whose coverage
	// the scorecard treats as load-bearing: their absence (when coverage is n/a)
	// means the repo was not analysed at all. Injected by the composition root from
	// the language registry so the score package holds no hardcoded tool list.
	// Omitted when empty; score then falls back to its built-in default set.
	PrimaryExtractorTools []string `json:"primary_extractor_tools,omitempty"`
	// ConfigWarnings carries advisory config-quality messages (under-specified
	// modules, swallowed optional-tool errors) so they reach md/json/CI instead
	// of being stderr-only. Omitted when empty. Advisory — never gates.
	ConfigWarnings []string `json:"config_warnings,omitempty"`
	// ClassifiedEdges is the aggregate distribution of all classified coupling
	// edges from this run. Populated from the full coupling.Index (before advisory
	// filtering) so coupling_balance sees every edge, not just the noise-controlled
	// advisory subset. Nil when classification did not run (backward compatible).
	ClassifiedEdges *ClassifiedEdgeSummary `json:"classified_edges,omitempty"`
	// DistanceContext is a human-readable rollup of the basis behind the distance
	// dimension (owner model, basis counts, deploy/external evidence). Report-only.
	DistanceContext *DistanceContext `json:"distance_context,omitempty"`
	// LocalCoupling is the report-only per-module summary of scored same-module
	// edges — the book Ch10 local-complexity quadrant. Same-module edges never
	// enter coupling_balance's denominator (see LocalCouplingModule). Never
	// consumed by verdict or gate logic; omitted when no module has a
	// same-module edge.
	LocalCoupling []LocalCouplingModule `json:"local_coupling,omitempty"`
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
		AdvisoryTasks:  []AdvisoryTask{},
		ToolCoverage:   []Coverage{},
	}
}
