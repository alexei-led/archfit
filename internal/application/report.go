package application

import (
	"strconv"

	"github.com/alexei-led/archfit/internal/assessment/decision"
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/score"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/report"
)

// ProjectReport converts an assessment result plus its synthesised scorecard into
// the single stable external report contract renderers consume. baseScore is nil
// unless a --base delta was requested; hardGate forces a FAIL decision band even
// when the verdict itself is not fail (a tripped opt-in tool gate).
func ProjectReport(r result.Result, sc score.Scorecard, baseScore *score.Scorecard, hardGate bool) report.Document {
	doc := report.Document{
		SchemaVersion: r.SchemaVersion, Verdict: report.Verdict(r.Verdict), Base: r.Base, Head: r.Head,
		ConfigHash: r.ConfigHash, Metrics: projectMetrics(r.Metrics), Findings: projectFindings(r.Findings),
		FileFacts: projectFileFacts(r.FileFacts), DynamicImports: projectDynamicImports(r.DynamicImports), Connascence: projectConnascence(r.Connascence),
		DynamicConnascenceSignals: projectDynamicConnascenceSignals(r.DynamicConnascenceSignals), RuntimeAsync: projectRuntimeAsyncModules(r.RuntimeAsync),
		RuntimeAsyncEdges: projectRuntimeAsyncEdges(r.RuntimeAsyncEdges), DeprecatedDeps: projectDeprecatedDeps(r.DeprecatedDeps),
		SemanticStrengthOverlay: projectSemanticStrengthOverlay(r.SemanticStrengthOverlay), SyntaxFacts: projectSyntaxFacts(r.SyntaxFacts),
		AgentTasks: projectAgentTasks(r.AgentTasks), AdvisoryTasks: projectAdvisoryTasks(r.AdvisoryTasks),
		ToolCoverage: projectCoverage(r.ToolCoverage), CoverageGaps: projectCoverageGaps(r.CoverageGaps), OwnerSource: r.OwnerSource,
		PrimaryExtractorTools: r.PrimaryExtractorTools, ConfigWarnings: r.ConfigWarnings,
		ClassifiedEdges: projectClassifiedEdges(r.ClassifiedEdges), DistanceContext: projectDistanceContext(r.DistanceContext),
		DistanceConfigCandidates: projectDistanceConfigCandidates(r.DistanceConfigCandidates), VolatilityCorroboration: projectVolatilityCorroboration(r.VolatilityCorroboration),
		LocalCoupling: projectLocalCoupling(r.LocalCoupling), GitFindingDelta: projectGitFindingDelta(r.GitFindingDelta), Delta: projectDelta(r.Delta), Summary: report.Summary(r.Summary),
	}
	doc.Score = projectScorecard(sc)
	if baseScore != nil {
		b := projectScorecard(*baseScore)
		doc.BaseScore = &b
	}
	doc.Decision = projectDecision(decision.Build(r, sc, baseScore, hardGate))
	doc.State = projectArchitectureState(r, doc, hardGate)
	return doc
}

// projectArchitectureState builds the shadow archfit.architecture-state.v1
// contract from facts the assessment result already carries.
//
// This is the contract-freeze stage of the migration, not the measurement
// stage: every dimension reports `unmeasured` with a named owner, because the
// collectors that fill the nine envelopes land in the next task. Reporting them
// as measured-and-empty would be exactly the implicit green result the contract
// exists to prevent.
//
// The verdict rule here is deliberately minimal and is replaced by the
// assessment-owned aggregator: `blocked` when an active hard-gate finding or a
// tripped tool gate exists, otherwise `needs_attention`. `healthy` is
// unreachable by construction while any dimension is unmeasured.
func projectArchitectureState(r result.Result, doc report.Document, hardGate bool) report.ArchitectureState {
	state := report.NewArchitectureState()
	state.Findings = doc.Findings
	state.AgentTasks = doc.AgentTasks
	state.Measurement.SourceRef = r.Head
	state.Measurement.ToolVersions, state.Coverage.Tools = projectStateToolCoverage(r.ToolCoverage)
	if h := r.VolatilityCorroboration; h != nil {
		state.Measurement.HistoryDepth = h.CommitsScanned
		state.Measurement.HistoryWindow = historyWindow(h.FullHistory, h.CommitWindow)
	}
	markDimensionsPending(&state.Dimensions)

	state.Comparison.ConfigHash = r.ConfigHash
	state.Comparison.RubricVersion = report.ScoreVersion
	if r.Base != "" {
		state.Comparison.Status = report.ComparisonNonComparable
		state.Comparison.BaseRef = r.Base
		state.Comparison.Reasons = []string{"state_comparison_unimplemented"}
	}

	blockers := r.Summary.GateFindings
	if hardGate && blockers == 0 {
		blockers = 1
	}
	state.Decision.ActiveBlockers = blockers
	if blockers > 0 {
		state.Decision.HardGates = report.HardGateFail
		state.Verdict = report.StateBlocked
	} else {
		state.Verdict = report.StateNeedsAttention
	}

	measured, partial, unmeasured := state.Dimensions.CountStatuses()
	state.Coverage.Measured, state.Coverage.Partial, state.Coverage.Unmeasured = measured, partial, unmeasured
	state.Decision.UnknownDimensions = partial + unmeasured
	return state
}

// historyWindow renders the bounded git-history window as the deterministic
// string the measurement block publishes. An unbounded scan says so; a zero
// window is left empty rather than reported as "0 commits", which would read as
// a measured-and-empty history.
func historyWindow(fullHistory bool, commitWindow int) string {
	switch {
	case fullHistory:
		return "full history"
	case commitWindow > 0:
		return strconv.Itoa(commitWindow) + " commits"
	default:
		return ""
	}
}

// markDimensionsPending stamps every envelope with the reason it is unmeasured.
// The contract-freeze stage ships the nine envelopes before their collectors,
// and an unmeasured dimension with no stated reason is indistinguishable from
// one nobody bothered to explain. Each collector deletes its own entry as it
// lands.
func markDimensionsPending(d *report.Dimensions) {
	for _, dim := range []*report.DimensionState{
		&d.Intent, &d.Structure, &d.Modularity, &d.Coupling, &d.ChangeLocality,
		&d.Complexity, &d.Testability, &d.Operations, &d.Drift,
	} {
		dim.Unknown = []report.UnknownFact{{
			Fact:   dim.Name,
			Reason: "collector not wired: the architecture-state contract ships before its measurements",
			Owner:  dim.Owner,
		}}
	}
}

// projectStateToolCoverage splits the existing coverage rows into the two views
// the state contract keeps separate: deterministic tool versions (a measurement
// fact) and per-tool status (an evidence-coverage fact). Rows stay in the order
// acquisition produced them, and a version is recorded only when the tool
// reported one.
func projectStateToolCoverage(in []evidence.Coverage) (map[string]string, []report.StateToolCoverage) {
	versions := make(map[string]string, len(in))
	tools := make([]report.StateToolCoverage, 0, len(in))
	for _, c := range in {
		if c.Version != "" {
			versions[c.Tool] = c.Version
		}
		tools = append(tools, report.StateToolCoverage{Tool: c.Tool, Status: c.Status, Reason: c.Reason})
	}
	return versions, tools
}

func projectScorecard(in score.Scorecard) report.Scorecard {
	out := report.Scorecard{RubricVersion: in.RubricVersion, Overall: in.Overall, OverallBand: report.ScoreBand(in.OverallBand), Dimensions: make([]report.Dimension, len(in.Dimensions))}
	for i, d := range in.Dimensions {
		out.Dimensions[i] = report.Dimension{Name: d.Name, Value: d.Value, Band: report.ScoreBand(d.Band), Confidence: report.Confidence(d.Confidence), Evidence: d.Evidence, Summary: d.Summary, RawValue: d.RawValue, CapApplied: d.CapApplied, Meta: d.Meta}
	}
	return out
}

func projectDecision(in decision.Report) report.Report {
	out := report.Report{Band: report.DecisionBand(in.Band), Headline: in.Headline, Blocking: in.Blocking, Advisory: in.Advisory, Overall: in.Overall, OverallBand: report.ScoreBand(in.OverallBand), Dimensions: make([]report.DimReport, len(in.Dimensions)), Recommendations: report.Recommendations{MustFix: projectRecs(in.Recommendations.MustFix), ShouldFix: projectRecs(in.Recommendations.ShouldFix), Watch: projectRecs(in.Recommendations.Watch), Calibrate: projectRecs(in.Recommendations.Calibrate), Ignore: projectRecs(in.Recommendations.Ignore)}}
	for i, d := range in.Dimensions {
		out.Dimensions[i] = report.DimReport{Name: d.Name, Value: d.Value, Band: report.ScoreBand(d.Band), Confidence: report.Confidence(d.Confidence), RawValue: d.RawValue, CapApplied: d.CapApplied, Meta: d.Meta, Why: d.Why, WhatMoves: d.WhatMoves}
	}
	if in.Delta != nil {
		out.Delta = &report.Delta{Overall: in.Delta.Overall, Dimensions: make([]report.DimDelta, len(in.Delta.Dimensions))}
		for i, d := range in.Delta.Dimensions {
			out.Delta.Dimensions[i] = report.DimDelta{Name: d.Name, Before: d.Before, After: d.After, Change: d.Change}
		}
	}
	return out
}

func projectRecs(in []decision.Rec) []report.Rec {
	out := make([]report.Rec, len(in))
	for i, r := range in {
		out[i] = report.Rec{Title: r.Title, Detail: r.Detail, RuleID: r.RuleID}
	}
	return out
}

func projectCoverage(in []evidence.Coverage) []report.Coverage {
	out := make([]report.Coverage, len(in))
	for i, v := range in {
		out[i] = report.Coverage{Tool: v.Tool, Version: v.Version, FilesSeen: v.FilesSeen, FilesApplicable: v.FilesApplicable, Unresolved: v.Unresolved, SpecifiersSeen: v.SpecifiersSeen, UnresolvedInputsMissing: v.UnresolvedInputsMissing, UnresolvedPrecisionOnly: v.UnresolvedPrecisionOnly, Status: v.Status, Reason: v.Reason}
	}
	return out
}

func projectCoverageGaps(in []evidence.CoverageGap) []report.CoverageGap {
	out := make([]report.CoverageGap, len(in))
	for i, v := range in {
		out[i] = report.CoverageGap{Tool: v.Tool, InstallCmd: v.InstallCmd, AffectedMetrics: v.AffectedMetrics, Gate: v.Gate}
	}
	return out
}

func projectFileFacts(in []evidence.FileFact) []report.FileFact {
	out := make([]report.FileFact, len(in))
	for i, v := range in {
		out[i] = report.FileFact{Module: v.Module, Files: v.Files, InboundModuleFanIn: v.InboundModuleFanIn, OutboundDestinations: v.OutboundDestinations, LOC: v.LOC}
	}
	return out
}

func projectDynamicImports(in []evidence.DynamicImport) []report.DynamicImport {
	out := make([]report.DynamicImport, len(in))
	for i, v := range in {
		sites := make([]report.DynamicImportSite, len(v.Sites))
		for j, s := range v.Sites {
			sites[j] = report.DynamicImportSite{File: s.File, Line: s.Line, Kind: s.Kind, Language: s.Language}
		}
		out[i] = report.DynamicImport{Module: v.Module, Count: v.Count, Sites: sites}
	}
	return out
}

func projectSyntaxFacts(in []evidence.SyntaxFact) []report.SyntaxFact {
	out := make([]report.SyntaxFact, len(in))
	for i, v := range in {
		out[i] = report.SyntaxFact{Language: v.Language, File: v.File, Module: v.Module, Kind: v.Kind, Name: v.Name, Exported: v.Exported, StartLine: v.StartLine, EndLine: v.EndLine, Count: v.Count, Framework: v.Framework, FrameworkConfirmed: v.FrameworkConfirmed}
	}
	return out
}

func projectDeprecatedDeps(in []evidence.DeprecatedDep) []report.DeprecatedDep {
	out := make([]report.DeprecatedDep, len(in))
	for i, v := range in {
		out[i] = report.DeprecatedDep{File: v.File, Kind: v.Kind, Subject: v.Subject, Note: v.Note}
	}
	return out
}

func projectSemanticStrengthOverlay(in *evidence.SemanticStrengthOverlay) *report.SemanticStrengthOverlay {
	if in == nil {
		return nil
	}
	out := &report.SemanticStrengthOverlay{ByLanguage: make(map[string]report.SemanticStrengthOverlayStats, len(in.ByLanguage))}
	for lang, v := range in.ByLanguage {
		out.ByLanguage[lang] = report.SemanticStrengthOverlayStats{CandidateEdges: v.CandidateEdges, Applied: v.Applied, Missed: v.Missed, Before: v.Before, After: v.After}
	}
	return out
}

func projectDynamicConnascenceSignals(in *evidence.DynamicConnascenceSignals) *report.DynamicConnascenceSignals {
	if in == nil {
		return nil
	}
	out := &report.DynamicConnascenceSignals{Unmeasured: in.Unmeasured, ReportOnlyReason: in.ReportOnlyReason, Signals: make([]report.DynamicConnascenceSignal, len(in.Signals))}
	for i, v := range in.Signals {
		sites := make([]report.DynamicConnascenceSite, len(v.Sites))
		for j, s := range v.Sites {
			sites[j] = report.DynamicConnascenceSite{File: s.File, Line: s.Line, Kind: s.Kind, Language: s.Language, Target: s.Target}
		}
		out.Signals[i] = report.DynamicConnascenceSignal{Kind: v.Kind, RelatedConnascence: v.RelatedConnascence, Measured: v.Measured, ReportOnlyReason: v.ReportOnlyReason, Module: v.Module, Target: v.Target, IntegrationKind: v.IntegrationKind, Count: v.Count, Sites: sites}
	}
	return out
}

func projectRuntimeAsyncModules(in []evidence.RuntimeAsyncModule) []report.RuntimeAsyncModule {
	out := make([]report.RuntimeAsyncModule, len(in))
	for i, v := range in {
		out[i] = report.RuntimeAsyncModule{Module: v.Module, IntegrationKind: v.IntegrationKind, Count: v.Count, Confidence: v.Confidence}
	}
	return out
}

func projectRuntimeAsyncEdges(in []evidence.RuntimeAsyncEdge) []report.RuntimeAsyncEdge {
	out := make([]report.RuntimeAsyncEdge, len(in))
	for i, v := range in {
		sites := make([]report.RuntimeAsyncSite, len(v.Sites))
		for j, s := range v.Sites {
			sites[j] = report.RuntimeAsyncSite{File: s.File, Line: s.Line, Library: s.Library, IntegrationKind: s.IntegrationKind, Language: s.Language}
		}
		out[i] = report.RuntimeAsyncEdge{FromModule: v.FromModule, Target: v.Target, IntegrationKind: v.IntegrationKind, Count: v.Count, Confidence: v.Confidence, Sites: sites}
	}
	return out
}

func projectConnascence(in *evidence.ConnascenceReport) *report.ConnascenceReport {
	if in == nil {
		return nil
	}
	out := &report.ConnascenceReport{EdgesWithEvidence: in.EdgesWithEvidence, AbstainedEdges: in.AbstainedEdges, TotalEvidence: in.TotalEvidence, StrengthInferredEdges: in.StrengthInferredEdges, ByKind: in.ByKind, BySource: in.BySource, Unmeasured: in.Unmeasured, Roadmap: make([]report.ConnascenceRoadmapItem, len(in.Roadmap))}
	for i, v := range in.Roadmap {
		out.Roadmap[i] = report.ConnascenceRoadmapItem{Kind: v.Kind, CurrentStatus: v.CurrentStatus, Sources: v.Sources, RelatedSignals: v.RelatedSignals, UpgradeTrigger: v.UpgradeTrigger}
	}
	return out
}

func projectDistanceContext(in *evidence.DistanceContext) *report.DistanceContext {
	if in == nil {
		return nil
	}
	return &report.DistanceContext{OwnerModel: in.OwnerModel, DistanceBasis: in.DistanceBasis, DeployUnitDetectedModules: in.DeployUnitDetectedModules, DeclaredExternalSystems: in.DeclaredExternalSystems, RuntimeAsyncRelations: in.RuntimeAsyncRelations, RuntimeAsyncKinds: in.RuntimeAsyncKinds, Interpretation: in.Interpretation, RuntimeInterpretation: in.RuntimeInterpretation}
}

func projectDistanceConfigCandidates(in []evidence.DistanceConfigCandidate) []report.DistanceConfigCandidate {
	out := make([]report.DistanceConfigCandidate, len(in))
	for i, v := range in {
		sites := make([]report.DistanceConfigEvidenceSite, len(v.EvidenceSites))
		for j, s := range v.EvidenceSites {
			sites[j] = report.DistanceConfigEvidenceSite{File: s.File, Line: s.Line, Kind: s.Kind, Language: s.Language, Target: s.Target}
		}
		out[i] = report.DistanceConfigCandidate{SourceBlock: v.SourceBlock, Module: v.Module, Target: v.Target, IntegrationKind: v.IntegrationKind, Count: v.Count, EvidenceSites: sites, SuggestedReviewAction: v.SuggestedReviewAction}
	}
	return out
}

func projectVolatilityCorroboration(in *evidence.VolatilityCorroboration) *report.VolatilityCorroboration {
	if in == nil {
		return nil
	}
	out := &report.VolatilityCorroboration{Source: in.Source, Status: in.Status, CommitWindow: in.CommitWindow, FullHistory: in.FullHistory, CommitsScanned: in.CommitsScanned, ModulesTouched: in.ModulesTouched, Caveat: in.Caveat, TopTouched: make([]report.VolatilityTouch, len(in.TopTouched))}
	for i, v := range in.TopTouched {
		out.TopTouched[i] = report.VolatilityTouch{Module: v.Module, TouchCommits: v.TouchCommits, DeclaredVolatility: v.DeclaredVolatility}
	}
	return out
}

func projectLocalCoupling(in []evidence.LocalCouplingModule) []report.LocalCouplingModule {
	out := make([]report.LocalCouplingModule, len(in))
	for i, v := range in {
		edges := make([]report.LocalCouplingEdge, len(v.WorstOffenders))
		for j, e := range v.WorstOffenders {
			edges[j] = report.LocalCouplingEdge{From: e.From, To: e.To, Strength: e.Strength, Balance: e.Balance, Band: e.Band, File: e.File, Line: e.Line}
		}
		out[i] = report.LocalCouplingModule{Module: v.Module, ScoredEdges: v.ScoredEdges, AbstainedEdges: v.AbstainedEdges, ComplexityEdges: v.ComplexityEdges, ComplexitySharePct: v.ComplexitySharePct, MeanBalance: v.MeanBalance, WorstOffenders: edges}
	}
	return out
}

func projectMetrics(in []result.MetricResult) []report.MetricResult {
	out := make([]report.MetricResult, 0, len(in))
	for _, m := range in {
		out = append(out, report.MetricResult{Name: m.Name, Value: m.Value, Display: m.Display, Band: m.Band, Confidence: m.Confidence, Version: m.Version, Mode: m.Mode, Definition: m.Definition, Delta: m.Delta, Direction: report.Direction(m.Direction)})
	}
	return out
}

func projectDelta(in *result.DeltaReport) *report.DeltaReport {
	if in == nil {
		return nil
	}
	return &report.DeltaReport{New: in.New, Existing: in.Existing, Resolved: in.Resolved, SeverityChanged: in.SeverityChanged, TouchedByDelta: in.TouchedByDelta}
}

func projectGitFindingDelta(in *result.GitFindingDelta) *report.GitFindingDelta {
	if in == nil {
		return nil
	}
	return &report.GitFindingDelta{BaseRef: in.BaseRef, ComparisonStatus: in.ComparisonStatus, IntroducedFindingIDs: in.IntroducedFindingIDs, PreExistingFindingIDs: in.PreExistingFindingIDs, UnknownOriginFindingIDs: in.UnknownOriginFindingIDs, ComparisonReasons: in.ComparisonReasons}
}

func projectClassifiedEdges(in *result.ClassifiedEdgeSummary) *report.ClassifiedEdgeSummary {
	if in == nil {
		return nil
	}
	out := &report.ClassifiedEdgeSummary{
		Total: in.Total, Scored: in.Scored, Abstained: in.Abstained, SameModule: in.SameModule, MeanBalance: in.MeanBalance,
		ByStrength: in.ByStrength, ByDistance: in.ByDistance, ByDistanceBasis: in.ByDistanceBasis, ByVolatility: in.ByVolatility,
		BySeverity: in.BySeverity, ByBalanceDriver: in.ByBalanceDriver, ByCriticalDriver: in.ByCriticalDriver, ByModulePair: in.ByModulePair,
		DistributedMonolith: in.DistributedMonolith, External: in.External, DeclaredExternal: in.DeclaredExternal,
		ConnectedModules: in.ConnectedModules, CloneOnlyScored: in.CloneOnlyScored, CloneOnlyAdvisory: in.CloneOnlyAdvisory,
		LLMApproved: in.LLMApproved, LabeledLLM: in.LabeledLLM, LLMLowConfidenceEdges: in.LLMLowConfidenceEdges,
		VolatilityProvenance: projectVolatilityProvenance(in.VolatilityProvenance), DistanceCompression: projectDistanceCompression(in.DistanceCompression),
	}
	if in.TailRisk != nil {
		out.TailRisk = &report.CouplingTailRiskSummary{WorstBalance: in.TailRisk.WorstBalance, LowerDecileBalance: in.TailRisk.LowerDecileBalance, HighOrWorseEdges: in.TailRisk.HighOrWorseEdges, HighOrWorseSharePct: in.TailRisk.HighOrWorseSharePct, CriticalEdges: in.TailRisk.CriticalEdges, DistributedMonolithEdges: in.TailRisk.DistributedMonolithEdges, CloneOnlyScored: in.TailRisk.CloneOnlyScored, CloneOnlyHighOrWorseEdges: in.TailRisk.CloneOnlyHighOrWorseEdges, CloneOnlyWorstBalance: in.TailRisk.CloneOnlyWorstBalance}
	}
	return out
}

func projectVolatilityProvenance(in *result.VolatilityProvenance) *report.VolatilityProvenance {
	if in == nil {
		return nil
	}
	return &report.VolatilityProvenance{Declared: in.Declared, Inherited: in.Inherited, Cascade: in.Cascade, Undeclared: in.Undeclared}
}

func projectDistanceCompression(in *result.DistanceCompressionSummary) *report.DistanceCompressionSummary {
	if in == nil {
		return nil
	}
	out := &report.DistanceCompressionSummary{CompressedMiddleRungs: in.CompressedMiddleRungs, ImplementedRungs: in.ImplementedRungs, OmittedRungs: in.OmittedRungs, Rationale: in.Rationale}
	for _, r := range in.OmittedRungReasons {
		out.OmittedRungReasons = append(out.OmittedRungReasons, report.DistanceOmittedRungReason{Rung: r.Rung, Reason: r.Reason})
	}
	out.DeterministicSplits = append(out.DeterministicSplits, in.DeterministicSplits...)
	for _, c := range in.CodeStructureBoundaryCounts {
		out.CodeStructureBoundaryCounts = append(out.CodeStructureBoundaryCounts, report.DistanceCount{Value: c.Value, Count: c.Count})
	}
	for _, c := range in.CodeStructureAncestorDepths {
		out.CodeStructureAncestorDepths = append(out.CodeStructureAncestorDepths, report.DistanceCount{Value: c.Value, Count: c.Count})
	}
	return out
}

func projectFindings(in []finding.Finding) []report.Finding {
	out := make([]report.Finding, 0, len(in))
	for _, f := range in {
		// nil in, nil out: report.Finding.Locations has no omitempty, so a
		// zero-length slice would publish `[]` where the schema has always
		// carried `null` for a finding with no locations.
		var locations []report.Location
		if len(f.Locations) > 0 {
			locations = make([]report.Location, 0, len(f.Locations))
			for _, loc := range f.Locations {
				locations = append(locations, report.Location{File: loc.File, Line: loc.Line})
			}
		}
		out = append(out, report.Finding{
			ID: f.ID, Kind: f.Kind, RuleID: f.RuleID, Status: string(f.Status), Severity: string(f.Severity),
			Confidence: f.Confidence,
			Edge: report.FindingEdge{
				From: report.FindingEndpoint{Module: f.Edge.From.Module, Path: f.Edge.From.Path},
				To:   report.FindingEndpoint{Module: f.Edge.To.Module, Path: f.Edge.To.Path}, Kind: f.Edge.Kind,
			},
			MatchedBy: f.MatchedBy, Locations: locations, Why: f.Why, Constraint: f.Constraint, Alternatives: f.Alternatives,
		})
	}
	return out
}

func projectAgentTasks(in []result.AgentTask) []report.AgentTask {
	out := make([]report.AgentTask, 0, len(in))
	for _, t := range in {
		out = append(out, report.AgentTask{FindingID: t.FindingID, RuleID: t.RuleID, Goal: t.Goal, Constraints: t.Constraints, Files: t.Files, Validation: t.Validation, Declarations: projectSyntaxFacts(t.Declarations)})
	}
	return out
}

func projectAdvisoryTasks(in []result.AdvisoryTask) []report.AdvisoryTask {
	out := make([]report.AdvisoryTask, 0, len(in))
	for _, t := range in {
		out = append(out, report.AdvisoryTask{FindingID: t.FindingID, RuleID: t.RuleID, Status: string(t.Status), Severity: string(t.Severity), GroupCount: t.GroupCount, GroupMembers: t.GroupMembers, Goal: t.Goal, CheapestMove: t.CheapestMove, ScoreValue: t.ScoreValue, TopFiles: t.TopFiles, Constraints: t.Constraints, Validation: t.Validation})
	}
	return out
}
