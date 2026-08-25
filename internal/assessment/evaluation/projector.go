package evaluation

import (
	"github.com/alexei-led/archfit/internal/assessment/result"
	modevidence "github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/scope"
)

// project evaluates the assessment stages and assembles the diagnostic. Every
// report-only block it copies was already derived by its owning capability: the
// relationship stage keyed the runtime, dynamic-import, and connascence rollups
// against the module map it classified with, and acquisition built the file
// facts. Assessment re-derives none of them.
func project(in AssessInput, rules Ruleset, metrics Metricset) (result.Result, relationship.Set) {
	classified := in.Relationships
	syntaxFacts := in.Facts.SyntaxFacts
	runtimeAsync := classified.Evidence.RuntimeModules
	runtimeAsyncEdges := classified.Evidence.RuntimeEdges
	coverage := in.Facts.Coverage

	// --- Stages 4–6: assessment ---
	// Rule evaluation, lifecycle status, and metric calculation are owned here.
	// The relationship contract and the acquired signals are the only inputs.
	assessed := evaluate(Input{
		Relationships:      classified.Relationships,
		Evidence:           RuleEvidence{PatternMatches: in.Facts.PatternMatches, SyntaxFacts: syntaxFacts},
		Rules:              rules,
		Metrics:            metrics,
		Signals:            runSignals(in.Facts),
		Symbols:            in.Facts.Symbols,
		Coverage:           coverage,
		ChangedFiles:       in.Scope.Changed,
		Baseline:           in.BaseMetrics,
		Accepted:           in.Accepted,
		Policy:             in.Policy.Assessment,
		Gates:              in.Policy.Gates.Metrics,
		Now:                in.Now,
		AdvisoryCandidates: classified.Assessment.AdvisoryCandidates,
		StaleLabelKeys:     classified.Assessment.StaleLabelKeys,
		IncludeAdvisories:  in.Advisory,
		Delta:              in.Scope.Mode == scope.ModeDelta,

		CaptureRelationships: in.CaptureRelationships,
	})
	resolvedFindings := assessed.Findings
	metricResults := assessed.Metrics
	verdict := assessed.Verdict
	gateNew, warnings, waiversUsed := assessed.GateFindings, assessed.Warnings, assessed.WaiversUsed

	if metricResults == nil {
		metricResults = []result.MetricResult{}
	}
	if coverage == nil {
		coverage = []modevidence.Coverage{}
	}

	// Neutral structural-facts block: assembled by acquisition from the symbol
	// graph and file LOC, attached here as report-only evidence. Never read by
	// the verdict or any gate. Empty when SCIP is off/absent.
	fileFacts := in.Facts.FileFacts

	// Dynamic/lazy-import risk: report-only evidence rolled up per module by the
	// relationship stage, against the same augmented module map it classified
	// with. Dynamic imports are invisible to the static graph, so they hide
	// cycles and undercount coupling. Never read by the verdict or any gate.
	dynamicImports := classified.Evidence.DynamicImports

	// Delta bucketing (Task 3c): in delta mode, group findings by how they relate
	// to the baseline and the changed-file set so the report does not read like a
	// full-repo dump. Report-only; never enters the verdict. Nil outside delta mode.
	delta := assessed.Delta

	classifiedEdges := projectRelationshipSummary(classified.Evidence.ClassifiedEdges)
	if classifiedEdges != nil {
		classifiedEdges.LLMApproved = classified.Evidence.LLMApprovedCount
	}
	connascenceReport := classified.Evidence.Connascence
	if connascenceReport == nil {
		connascenceReport = &modevidence.ConnascenceReport{}
	}
	dynamicConnascenceSignals := classified.Evidence.DynamicConnascenceSignals
	distanceConfigCandidates := classified.Evidence.DistanceConfigCandidates
	if classified.Evidence.VolatilityProvenance != nil && classifiedEdges != nil {
		vp := classified.Evidence.VolatilityProvenance
		classifiedEdges.VolatilityProvenance = &result.VolatilityProvenance{Declared: vp.Declared, Inherited: vp.Inherited, Cascade: vp.Cascade, Undeclared: vp.Undeclared}
	}
	localCoupling := classified.Evidence.LocalCoupling

	d := result.Result{
		SchemaVersion:             result.SchemaVersion,
		Verdict:                   verdict,
		Base:                      in.BaseRef,
		Head:                      in.Head,
		ConfigHash:                in.ConfigHash,
		PrimaryExtractorTools:     in.PrimaryExtractorTools,
		Metrics:                   metricResults,
		Findings:                  resolvedFindings,
		SyntaxFacts:               syntaxFacts,
		FileFacts:                 fileFacts,
		DynamicImports:            dynamicImports,
		Connascence:               connascenceReport,
		DynamicConnascenceSignals: dynamicConnascenceSignals,
		RuntimeAsync:              runtimeAsync,
		RuntimeAsyncEdges:         runtimeAsyncEdges,
		DeprecatedDeps:            in.Facts.DeprecatedDeps,
		SemanticStrengthOverlay:   in.Facts.SemanticStrengthOverlay,
		AgentTasks:                []result.AgentTask{},
		AdvisoryTasks:             []result.AdvisoryTask{},
		ToolCoverage:              coverage,
		ClassifiedEdges:           classifiedEdges,
		DistanceConfigCandidates:  distanceConfigCandidates,
		LocalCoupling:             localCoupling,
		Delta:                     delta,
		Summary: result.Summary{
			GateFindings: gateNew,
			Warnings:     warnings,
			WaiversUsed:  waiversUsed,
		},
	}

	return d, assessed.Captured
}
