package application

import (
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/model/report"
)

// ProjectReport converts an assessment result into the stable external report contract.
func ProjectReport(r result.Result) report.Document {
	return report.Document{
		SchemaVersion: r.SchemaVersion, Verdict: r.Verdict, Base: r.Base, Head: r.Head,
		ConfigHash: r.ConfigHash, Metrics: r.Metrics, Findings: r.Findings,
		FileFacts: r.FileFacts, DynamicImports: r.DynamicImports, Connascence: r.Connascence,
		DynamicConnascenceSignals: r.DynamicConnascenceSignals, RuntimeAsync: r.RuntimeAsync,
		RuntimeAsyncEdges: r.RuntimeAsyncEdges, DeprecatedDeps: r.DeprecatedDeps,
		SemanticStrengthOverlay: r.SemanticStrengthOverlay, SyntaxFacts: r.SyntaxFacts,
		AgentTasks: projectAgentTasks(r.AgentTasks), AdvisoryTasks: projectAdvisoryTasks(r.AdvisoryTasks),
		ToolCoverage: r.ToolCoverage, CoverageGaps: r.CoverageGaps, OwnerSource: r.OwnerSource,
		PrimaryExtractorTools: r.PrimaryExtractorTools, ConfigWarnings: r.ConfigWarnings,
		ClassifiedEdges: r.ClassifiedEdges, DistanceContext: r.DistanceContext,
		DistanceConfigCandidates: r.DistanceConfigCandidates, VolatilityCorroboration: r.VolatilityCorroboration,
		LocalCoupling: r.LocalCoupling, GitFindingDelta: r.GitFindingDelta, Delta: r.Delta, Summary: r.Summary,
	}
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
		out = append(out, report.AdvisoryTask{FindingID: t.FindingID, RuleID: t.RuleID, Status: t.Status, Severity: t.Severity, GroupCount: t.GroupCount, GroupMembers: t.GroupMembers, Goal: t.Goal, CheapestMove: t.CheapestMove, ScoreValue: t.ScoreValue, TopFiles: t.TopFiles, Constraints: t.Constraints, Validation: t.Validation})
	}
	return out
}
