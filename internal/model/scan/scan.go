// Package scan defines the stable top-level scan contract.
package scan

import (
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/report"
)

// SchemaVersion identifies the JSON scan contract.
const SchemaVersion = "archfit.diagnostic.v2"

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
	Declarations []evidence.SyntaxFact `json:"declarations,omitempty"`
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

// Diagnostic is the top-level output contract for archfit analyze (spec §12).
// JSON tags match spec §12 field names exactly.
type Diagnostic struct {
	SchemaVersion string         `json:"schema_version"`
	Verdict       report.Verdict `json:"verdict"`
	Base          string         `json:"base"`
	Head          string         `json:"head"`
	// ConfigHash is the sha256 hex digest of the loaded .archfit.yaml bytes.
	// Empty when no config file was loaded (--no-config or default built-in).
	// Reproducibility: same config + same repo state → same ConfigHash.
	ConfigHash string                `json:"config_hash,omitempty"`
	Metrics    []report.MetricResult `json:"metrics"`
	Findings   []finding.Finding     `json:"findings"`
	// FileFacts is the neutral per-module structural-facts block (Tranche 1.5).
	// Report-only evidence — never consumed by verdict or gate logic. Empty when
	// no symbol graph was collected (SCIP off/absent).
	FileFacts []evidence.FileFact `json:"file_facts"`
	// DynamicImports is the report-only dynamic/lazy-import risk block (Task 9).
	// Evidence only — never consumed by verdict or gate logic, never alters the
	// dependency graph or any metric. Empty when no dynamic imports were found.
	DynamicImports []evidence.DynamicImport `json:"dynamic_imports"`
	// Connascence summarizes deterministic static connascence evidence. Report-only;
	// semantic/dynamic categories without a deterministic source are listed as
	// unmeasured rather than guessed. Omitted only when classification did not run.
	Connascence *evidence.ConnascenceReport `json:"connascence,omitempty"`
	// DynamicConnascenceSignals maps dynamic/lazy imports and runtime async facts
	// to possible Ch6 dynamic connascence categories for human review. Report-only;
	// all entries are measured=false and never change score, findings, or verdicts.
	DynamicConnascenceSignals *evidence.DynamicConnascenceSignals `json:"dynamic_connascence_signals,omitempty"`
	// RuntimeAsync is the report-only async-bridge detection block.
	// Evidence only — never consumed by classify, score, or gate logic; never
	// annotates graph edges and never affects distance, score, or verdict.
	// Empty when no async patterns were detected.
	RuntimeAsync []evidence.RuntimeAsyncModule `json:"runtime_async,omitempty"`
	// RuntimeAsyncEdges is the relationship-level async-bridge evidence block.
	// It groups concrete sites by source module and runtime target so later work
	// can review runtime distance without re-scanning raw files. Evidence only —
	// never consumed by classify, score, baseline deltas, or gate verdicts.
	RuntimeAsyncEdges []evidence.RuntimeAsyncEdge `json:"runtime_async_edges,omitempty"`
	// DeprecatedDeps is the report-only locally-declared deprecation/retraction
	// marker block. Evidence only — never consumed by verdict or gate logic, never
	// alters the dependency graph or any metric. Omitted when no markers were found.
	// Ceiling: cargo yanked and live-version EOL require external registry queries
	// and are routed to the LLM path (archfit analyze --ai-summary / config enrich), not here.
	DeprecatedDeps []evidence.DeprecatedDep `json:"deprecated_deps,omitempty"`
	// SemanticStrengthOverlay reports SCIP semantic-strength overlay coverage by
	// language. Report-only visibility for the refinement layer; never consumed by
	// verdict, gates, score synthesis, or baseline deltas.
	SemanticStrengthOverlay *evidence.SemanticStrengthOverlay `json:"semantic_strength_overlay,omitempty"`
	// SyntaxFacts is the report-only syntactic declaration/route block extracted
	// by ast-grep (design §3). Neutral, off-gate evidence — never consumed by
	// verdict or gate logic. Omitted (omitempty) when analyzers.syntax is off or sg
	// is absent, so absent sg never emits a null/empty block (no false green).
	SyntaxFacts   []evidence.SyntaxFact `json:"syntax_facts,omitempty"`
	AgentTasks    []AgentTask           `json:"agent_tasks"`
	AdvisoryTasks []AdvisoryTask        `json:"advisory_tasks"`
	ToolCoverage  []evidence.Coverage   `json:"tool_coverage"`
	// CoverageGaps lists analyzers that did not run, the metrics their absence
	// leaves unmeasured, and how to install them (warn-loud coverage reporting).
	// Omitted when every required tool ran. Populated in cmd/, never the core ring.
	CoverageGaps []evidence.CoverageGap `json:"coverage_gaps,omitempty"`
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
	ClassifiedEdges *report.ClassifiedEdgeSummary `json:"classified_edges,omitempty"`
	// DistanceContext is a human-readable rollup of the basis behind the distance
	// dimension (owner model, basis counts, deploy/external evidence). Report-only.
	DistanceContext *evidence.DistanceContext `json:"distance_context,omitempty"`
	// DistanceConfigCandidates are review-only hints derived from static external,
	// runtime, and dynamic evidence for possible external_systems or deploy_unit
	// config entries. They never alter distance classification, scoring, or gate
	// verdicts.
	DistanceConfigCandidates []evidence.DistanceConfigCandidate `json:"distance_config_candidates,omitempty"`
	// VolatilityCorroboration carries source-control touch frequency as book Ch9
	// supporting evidence for declared volatility. Report-only: never consumed by
	// scoring, findings, baselines, or gate verdicts.
	VolatilityCorroboration *evidence.VolatilityCorroboration `json:"volatility_corroboration,omitempty"`
	// LocalCoupling is the report-only per-module summary of scored same-module
	// edges — the book Ch10 local-complexity quadrant. Same-module edges never
	// enter coupling_balance's denominator (see LocalCouplingModule). Never
	// consumed by verdict or gate logic; omitted when no module has a
	// same-module edge.
	LocalCoupling []evidence.LocalCouplingModule `json:"local_coupling,omitempty"`
	// GitFindingDelta classifies the current repair tasks by git origin
	// (introduced / pre-existing / unknown) against `--base <ref>`. Nil
	// (omitted) unless --base was set; report-only — never changes the verdict
	// or the exit code.
	GitFindingDelta *report.GitFindingDelta `json:"git_finding_delta,omitempty"`
	// Delta groups findings by lifecycle bucket (new/existing/resolved/
	// severity_changed/touched_by_delta) for a delta run. Nil (omitted) outside
	// delta mode and when the run produced no findings to bucket.
	Delta   *report.DeltaReport `json:"delta,omitempty"`
	Summary report.Summary      `json:"summary"`
}

// New returns an empty scan with deterministic non-nil collections.
func New() Diagnostic {
	return Diagnostic{
		SchemaVersion:  SchemaVersion,
		Metrics:        []report.MetricResult{},
		Findings:       []finding.Finding{},
		FileFacts:      []evidence.FileFact{},
		DynamicImports: []evidence.DynamicImport{},
		AgentTasks:     []AgentTask{},
		AdvisoryTasks:  []AdvisoryTask{},
		ToolCoverage:   []evidence.Coverage{},
	}
}
