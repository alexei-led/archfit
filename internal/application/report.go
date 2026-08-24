package application

import (
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/model/report"
)

// ProjectReport converts an assessment result into the stable external report contract.
func ProjectReport(r result.Result) report.Document {
	return report.Document{
		SchemaVersion: r.SchemaVersion, Verdict: report.Verdict(r.Verdict), Base: r.Base, Head: r.Head,
		ConfigHash: r.ConfigHash, Metrics: projectMetrics(r.Metrics), Findings: projectFindings(r.Findings),
		FileFacts: r.FileFacts, DynamicImports: r.DynamicImports, Connascence: r.Connascence,
		DynamicConnascenceSignals: r.DynamicConnascenceSignals, RuntimeAsync: r.RuntimeAsync,
		RuntimeAsyncEdges: r.RuntimeAsyncEdges, DeprecatedDeps: r.DeprecatedDeps,
		SemanticStrengthOverlay: r.SemanticStrengthOverlay, SyntaxFacts: r.SyntaxFacts,
		AgentTasks: projectAgentTasks(r.AgentTasks), AdvisoryTasks: projectAdvisoryTasks(r.AdvisoryTasks),
		ToolCoverage: r.ToolCoverage, CoverageGaps: r.CoverageGaps, OwnerSource: r.OwnerSource,
		PrimaryExtractorTools: r.PrimaryExtractorTools, ConfigWarnings: r.ConfigWarnings,
		ClassifiedEdges: projectClassifiedEdges(r.ClassifiedEdges), DistanceContext: r.DistanceContext,
		DistanceConfigCandidates: r.DistanceConfigCandidates, VolatilityCorroboration: r.VolatilityCorroboration,
		LocalCoupling: r.LocalCoupling, GitFindingDelta: projectGitFindingDelta(r.GitFindingDelta), Delta: projectDelta(r.Delta), Summary: report.Summary(r.Summary),
	}
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
		locations := make([]report.Location, 0, len(f.Locations))
		for _, loc := range f.Locations {
			locations = append(locations, report.Location{File: loc.File, Line: loc.Line})
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
		out = append(out, report.AgentTask{FindingID: t.FindingID, RuleID: t.RuleID, Goal: t.Goal, Constraints: t.Constraints, Files: t.Files, Validation: t.Validation, Declarations: t.Declarations})
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
