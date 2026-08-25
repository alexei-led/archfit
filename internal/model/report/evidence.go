package report

import "sort"

// This file owns report-facing projection DTOs. They are distinct from the
// extractor-neutral facts in internal/model/evidence; application.ProjectReport
// performs the explicit conversion between the two contracts.

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
// that surfaces in scan/review output and agent_tasks evidence.
type SyntaxFact struct {
	Language           string `json:"language"`
	File               string `json:"file"`
	Module             string `json:"module,omitempty"`
	Kind               string `json:"kind"`
	Name               string `json:"name"`
	Exported           bool   `json:"exported,omitempty"`
	StartLine          int    `json:"start_line"`
	EndLine            int    `json:"end_line,omitempty"`
	Count              int    `json:"count,omitempty"`
	Framework          string `json:"framework,omitempty"`
	FrameworkConfirmed bool   `json:"-"`
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
// extractor edges. It is report-only evidence.
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
// graph and file LOC. Report-only evidence for the off-gate LLM narrative.
type FileFact struct {
	Module string `json:"module"`
	// Files lists the repo-relative source files that define this module's symbols.
	Files []string `json:"files"`
	// InboundModuleFanIn counts distinct OTHER modules whose symbols reference
	// this module's symbols.
	InboundModuleFanIn int `json:"inbound_module_fanin"`
	// OutboundDestinations counts distinct destination modules this module's
	// symbols reference, at raw module granularity.
	OutboundDestinations int `json:"outbound_destinations"`
	// LOC is the summed line count of Files.
	LOC int `json:"loc"`
}

// DynamicConnascenceSignals is a report-only bridge from deterministic static
// sites to the Ch6 dynamic connascence categories they may help humans inspect.
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
// connascence signal.
type DynamicConnascenceSite struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Kind     string `json:"kind"`
	Language string `json:"language"`
	Target   string `json:"target,omitempty"`
}

// RuntimeAsyncSite is one detected async integration pattern location.
type RuntimeAsyncSite struct {
	File            string `json:"file"`
	Line            int    `json:"line"`
	Library         string `json:"library"`
	IntegrationKind string `json:"integration_kind"`
	Language        string `json:"language"`
}

// RuntimeAsyncModule is a per-module rollup of detected async integration patterns.
type RuntimeAsyncModule struct {
	Module          string `json:"module"`
	IntegrationKind string `json:"integration_kind"`
	Count           int    `json:"count"`
	Confidence      string `json:"confidence"`
}

// RuntimeAsyncEdge is a relationship-level async integration fact from one
// module to one runtime target.
type RuntimeAsyncEdge struct {
	FromModule      string             `json:"from_module"`
	Target          string             `json:"target"`
	IntegrationKind string             `json:"integration_kind"`
	Count           int                `json:"count"`
	Confidence      string             `json:"confidence"`
	Sites           []RuntimeAsyncSite `json:"sites,omitempty"`
}

// CoverageGap is a machine-readable record of one analyzer that did not run.
type CoverageGap struct {
	Tool            string   `json:"tool"`
	InstallCmd      string   `json:"install_cmd"`
	AffectedMetrics []string `json:"affected_metrics"`
	Gate            string   `json:"gate"`
}

// DistanceConfigCandidate is a report-only hint that static external, runtime,
// or dynamic evidence may justify reviewing distance config.
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
// modules. Supporting evidence only.
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
// posture. Report-only documentation.
type ConnascenceRoadmapItem struct {
	Kind           string   `json:"kind"`
	CurrentStatus  string   `json:"current_status"`
	Sources        []string `json:"sources,omitempty"`
	RelatedSignals []string `json:"related_signals,omitempty"`
	UpgradeTrigger string   `json:"upgrade_trigger,omitempty"`
}

// ConnascenceReport summarizes deterministic static connascence evidence from
// classified dependency edges. Report-only.
type ConnascenceReport struct {
	EdgesWithEvidence     int                      `json:"edges_with_evidence"`
	AbstainedEdges        int                      `json:"abstained_edges"`
	TotalEvidence         int                      `json:"total_evidence"`
	StrengthInferredEdges int                      `json:"strength_inferred_edges,omitempty"`
	ByKind                map[string]int           `json:"by_kind,omitempty"`
	BySource              map[string]int           `json:"by_source,omitempty"`
	Unmeasured            []string                 `json:"unmeasured,omitempty"`
	Roadmap               []ConnascenceRoadmapItem `json:"roadmap,omitempty"`
}

// DistanceContext explains how distance evidence should be read for this run.
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

// LocalCouplingModule summarises the scored same-module edges of one module.
// Report-only — never consumed by verdict or gate logic.
type LocalCouplingModule struct {
	Module             string              `json:"module"`
	ScoredEdges        int                 `json:"scored_edges"`
	AbstainedEdges     int                 `json:"abstained_edges,omitempty"`
	ComplexityEdges    int                 `json:"complexity_edges"`
	ComplexitySharePct int                 `json:"complexity_share_pct"`
	MeanBalance        float64             `json:"mean_balance"`
	WorstOffenders     []LocalCouplingEdge `json:"worst_offenders,omitempty"`
}

// LocalCouplingEdge is one same-module edge sampled into WorstOffenders.
type LocalCouplingEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Strength string `json:"strength"`
	Balance  int    `json:"balance"`
	Band     string `json:"band"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
}
