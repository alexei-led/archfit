// Package evidence defines extractor-facing, score-free facts.
package evidence

import "sort"

// Coverage records what a single tool extracted.
type Coverage struct {
	Tool                    string `json:"tool"`
	Version                 string `json:"version"`
	FilesSeen               int    `json:"files_seen"`
	FilesApplicable         int    `json:"files_applicable"`
	Unresolved              int    `json:"unresolved"`
	SpecifiersSeen          int    `json:"specifiers_seen,omitempty"`
	UnresolvedInputsMissing int    `json:"unresolved_inputs_missing,omitempty"`
	UnresolvedPrecisionOnly int    `json:"unresolved_precision_only,omitempty"`
	Status                  string `json:"status"`
	Reason                  string `json:"reason,omitempty"`
}

// Coverage status constants are shared by extractor adapters.
const (
	StatusOK       = "ok"
	StatusPartial  = "partial"
	StatusAbsent   = "absent"
	StatusDisabled = "disabled"
	StatusTimedOut = "timed out"
)

// TopologySource is the closed provenance vocabulary for declared operational
// topology and the repository evidence that corroborates it.
type TopologySource string

// Operational-topology evidence sources.
const (
	TopologySourceDockerfile TopologySource = "dockerfile"
	TopologySourceK8s        TopologySource = "k8s_manifest"
	TopologySourceWorkspace  TopologySource = "workspace"
	TopologySourcePyproject  TopologySource = "pyproject"
	TopologySourceGoMain     TopologySource = "go_main"
	TopologySourceCodeowners TopologySource = "codeowners"
	TopologySourceGitAuthor  TopologySource = "git_author"
	TopologySourceDeclared   TopologySource = "declared"
)

// CorroboratedDeployUnit is one independently detected deploy-unit declaration.
// Path is repository-relative; Unit is the detected name, not a config value.
type CorroboratedDeployUnit struct {
	Path   string         `json:"path"`
	Unit   string         `json:"unit"`
	Source TopologySource `json:"source"`
}

// OwnerProvenance records which ownership statement or fallback produced one
// module's resolved owner. A git-author fallback is evidence but is not an
// ownership statement.
type OwnerProvenance struct {
	Module string         `json:"module"`
	Owner  string         `json:"owner"`
	Source TopologySource `json:"source"`
}

// DeprecatedDep is one locally-declared deprecation or retraction marker.
type DeprecatedDep struct {
	File    string `json:"file"`
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Note    string `json:"note,omitempty"`
}

// DynamicImport groups dynamic or lazy import sites by owning module.
type DynamicImport struct {
	Module string              `json:"module"`
	Count  int                 `json:"count"`
	Sites  []DynamicImportSite `json:"sites"`
}

// Dynamic import kind constants are shared by extractor adapters.
const (
	DynamicImportKindLazyImport    = "lazy_import"
	DynamicImportKindImportlib     = "importlib"
	DynamicImportKindRequire       = "require"
	DynamicImportKindDynamicImport = "dynamic_import"
)

// DynamicImportSite is one dynamic or lazy import occurrence.
type DynamicImportSite struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Kind     string `json:"kind"`
	Language string `json:"language"`
}

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

// DistanceConfigCandidate is a report-only hint that static external, runtime,
// or dynamic evidence may justify reviewing distance config. It never feeds
// classification, scoring, findings, baselines, or gates.
type DistanceConfigCandidate struct {
	SourceBlock           string                       `json:"source_block"`
	Module                string                       `json:"module"`
	Target                string                       `json:"target"`
	IntegrationKind       string                       `json:"integration_kind"`
	Count                 int                          `json:"count"`
	EvidenceSites         []DistanceConfigEvidenceSite `json:"evidence_sites,omitempty"`
	SuggestedReviewAction string                       `json:"suggested_review_action"`
}

// DistanceConfigEvidenceSite is a capped source location behind a
// DistanceConfigCandidate.
type DistanceConfigEvidenceSite struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Kind     string `json:"kind"`
	Language string `json:"language"`
	Target   string `json:"target,omitempty"`
}

// VolatilityCorroboration reports source-control touch frequency for declared
// modules. It is supporting evidence only: git history may reflect both domain
// volatility and accidental design churn, so this block never affects scoring.
type VolatilityCorroboration struct {
	Source         string            `json:"source"`
	Status         string            `json:"status"`
	CommitWindow   int               `json:"commit_window,omitempty"`
	FullHistory    bool              `json:"full_history,omitempty"`
	CommitsScanned int               `json:"commits_scanned,omitempty"`
	ModulesTouched int               `json:"modules_touched,omitempty"`
	TopTouched     []VolatilityTouch `json:"top_touched,omitempty"`
	Caveat         string            `json:"caveat,omitempty"`
}

// VolatilityTouch is one module's touch frequency sample from git history.
type VolatilityTouch struct {
	Module             string `json:"module"`
	TouchCommits       int    `json:"touch_commits"`
	DeclaredVolatility string `json:"declared_volatility,omitempty"`
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
	// StrengthInferredEdges counts classified edges whose strength was refined by
	// deterministic static connascence evidence rather than a direct strength hint.
	StrengthInferredEdges int `json:"strength_inferred_edges,omitempty"`
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

// DistanceContext explains how distance evidence should be read for this run.
// It is disclosure-only: the scorer consumes per-edge Distance and DistanceBasis,
// never this rollup. The block keeps single-owner repositories honest by saying
// "low socio-technical distance" instead of implying missing evidence.
type DistanceContext struct {
	OwnerModel                string         `json:"owner_model"`
	DistanceBasis             map[string]int `json:"distance_basis,omitempty"`
	DeployUnitDetectedModules int            `json:"deploy_unit_detected_modules,omitempty"`
	DeclaredExternalSystems   int            `json:"declared_external_systems,omitempty"`
	RuntimeAsyncRelations     int            `json:"runtime_async_relations,omitempty"`
	RuntimeAsyncKinds         map[string]int `json:"runtime_async_kinds,omitempty"`
	Interpretation            string         `json:"interpretation"`
	RuntimeInterpretation     string         `json:"runtime_interpretation,omitempty"`
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
