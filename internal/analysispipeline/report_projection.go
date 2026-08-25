package pipeline

import (
	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/assessment/result"
	modevidence "github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/relationship/facts"
	"github.com/alexei-led/archfit/internal/scope"
)

func projectAssessment(in StageInput, acquired acquiredStage, relationships relationshipStage) (result.Result, error) {
	ex := acquired.acquired
	ruleEv := acquired.ruleEvidence
	syntaxFacts := ruleEv.evidence.SyntaxFacts
	classifyCfg := in.Policy.Relationship.ClassifyConfig()
	classifyCfg = AugmentClassifyConfig(ex.Graph, classifyCfg)
	classified := relationships.classified
	runtimeAsync := runtimeModules(classified.RuntimeSignals)
	runtimeAsyncEdges := runtimeEdges(classified.RuntimeRelations)

	// --- Stages 4–6: assessment ---
	// Rule evaluation, lifecycle status, and metric calculation are owned by the
	// assessment service. The engine only supplies the relationship contract and
	// acquired signals, then assembles report-only projections below.
	assessed := evaluation.Evaluate(evaluation.Input{
		Relationships:      relationships.relationships,
		Evidence:           ruleEv.evidence,
		Rules:              in.Rules,
		Metrics:            in.Metrics,
		Signals:            in.Signals,
		Symbols:            ex.SCIPSymbols,
		Coverage:           ex.Coverages,
		ChangedFiles:       in.Scope.Changed,
		Baseline:           result.MetricSnapshot(in.BaseMetrics),
		Accepted:           in.Accepted,
		Policy:             in.Policy.Assessment,
		Gates:              in.Policy.Gates.Metrics,
		Now:                in.Now,
		AdvisoryCandidates: classified.AdvisoryCandidates,
		StaleLabelKeys:     classified.StaleLabelKeys,
		IncludeAdvisories:  in.Mode.Advisory,
		Delta:              in.Scope.Mode == scope.ModeDelta,
	})
	resolvedFindings := assessed.Findings
	metricResults := assessed.Metrics
	verdict := assessed.Verdict
	gateNew, warnings, waiversUsed := assessed.GateFindings, assessed.Warnings, assessed.WaiversUsed

	if metricResults == nil {
		metricResults = []result.MetricResult{}
	}
	if ex.Coverages == nil {
		ex.Coverages = []modevidence.Coverage{}
	}

	// Neutral structural-facts block (Tranche 1.5): assembled from the symbol
	// graph and file LOC, attached as report-only evidence. Never read by
	// computeVerdict or any gate logic. Empty when SCIP is off/absent.
	fileFacts := facts.Build(ex.SCIPSymbols, in.Signals.Size.FileLOC)

	// Dynamic/lazy-import risk (Task 9): report-only evidence rolled up per module.
	// Dynamic imports are invisible to the static graph, so they hide cycles and
	// undercount coupling. Never read by computeVerdict or any gate, and never
	// alters ex.g or any metric — the sites are scanned in cmd, not from the graph.
	dynamicImports := buildDynamicImports(in.Signals.DynamicImports.Sites, classifyCfg.ModuleMap)

	// Delta bucketing (Task 3c): in delta mode, group findings by how they relate
	// to the baseline and the changed-file set so the report does not read like a
	// full-repo dump. Report-only; never enters the verdict. Nil outside delta mode.
	delta := assessed.Delta

	classifiedEdges := projectRelationshipSummary(classified.ClassifiedEdges)
	if classifiedEdges != nil {
		classifiedEdges.LLMApproved = classified.LLMApprovedCount
	}
	connascenceReport := classified.Connascence
	if connascenceReport == nil {
		connascenceReport = &modevidence.ConnascenceReport{}
	}
	dynamicConnascenceSignals := buildDynamicConnascenceSignals(dynamicImports, runtimeAsyncEdges, connascenceReport.Unmeasured)
	distanceConfigCandidates := append(append([]modevidence.DistanceConfigCandidate(nil), classified.DistanceConfigCandidates...), BuildDistanceConfigCandidates(dynamicImports, runtimeAsyncEdges, dynamicConnascenceSignals)...)
	sortDistanceConfigCandidates(distanceConfigCandidates)
	if classified.VolatilityProvenance != nil && classifiedEdges != nil {
		vp := classified.VolatilityProvenance
		classifiedEdges.VolatilityProvenance = &result.VolatilityProvenance{Declared: vp.Declared, Inherited: vp.Inherited, Cascade: vp.Cascade, Undeclared: vp.Undeclared}
	}
	localCoupling := classified.LocalCoupling

	d := result.Result{
		SchemaVersion:             result.SchemaVersion,
		Verdict:                   verdict,
		Base:                      in.Mode.Base,
		Head:                      in.Mode.Head,
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
		DeprecatedDeps:            in.Signals.DeprecatedDeps,
		SemanticStrengthOverlay:   ex.SemanticStrengthOverlay,
		AgentTasks:                []result.AgentTask{},
		AdvisoryTasks:             []result.AdvisoryTask{},
		ToolCoverage:              ex.Coverages,
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

	return d, nil
}
