package evaluation

import (
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	modevidence "github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/fileclass"
)

// project evaluates the assessment stages and assembles the diagnostic. Every
// report-only block it copies was already derived by its owning capability.
// Assessment re-derives none of them.
func project(in AssessInput, rules Ruleset, metrics Metricset) result.Result {
	syntaxFacts := in.Facts.SyntaxFacts
	coverage := in.Facts.Coverage

	// --- Stages 4–6: assessment ---
	// Rule evaluation, lifecycle status, and metric calculation are owned here.
	// The relationship contract and the acquired signals are the only inputs.
	assessed := evaluate(Input{
		Relationships: in.Relationships,
		Evidence: RuleEvidence{
			PatternMatches: in.Facts.PatternMatches, SyntaxFacts: syntaxFacts,
			FileClasses: inScopeFileClasses(in.Facts), OutOfScopeFiles: in.Facts.OutOfScopeFiles,
			UnwalkedSourceProduction: in.Facts.UnwalkedSourceProduction,
			UnanalysedFiles:          in.Facts.UnanalysedFiles, SourceSelectors: in.Facts.SourceSelectors,
			CrateOwners: in.Facts.CrateOwners,
		},
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
		ModuleReview:       in.Policy.Gates.ModuleReview,
		Now:                in.Now,
		AdvisoryCandidates: in.RelationshipSignals.AdvisoryCandidates,
		StaleLabelKeys:     in.RelationshipSignals.StaleLabelKeys,
		IncludeAdvisories:  in.Advisory,
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

	classifiedEdges := projectRelationshipSummary(in.RelationshipSignals.ClassifiedEdges)
	graphComplexity := moduleGraphComplexity(in.Policy.Topology.Modules, in.Relationships)

	d := result.Result{
		SchemaVersion:           result.SchemaVersion,
		Verdict:                 verdict,
		Base:                    in.BaseRef,
		Head:                    in.Head,
		ConfigHash:              in.ConfigHash,
		ModelHash:               in.ModelHash,
		ClassificationHash:      in.ClassificationHash,
		LabelsHash:              in.LabelsHash,
		PrimaryExtractorTools:   in.PrimaryExtractorTools,
		Metrics:                 metricResults,
		Findings:                resolvedFindings,
		SyntaxFacts:             syntaxFacts,
		FileFacts:               fileFacts,
		DeprecatedDeps:          in.Facts.DeprecatedDeps,
		SemanticStrengthOverlay: in.Facts.SemanticStrengthOverlay,
		AgentTasks:              []result.AgentTask{},
		AdvisoryTasks:           []result.AdvisoryTask{},
		ToolCoverage:            coverage,
		ClassifiedEdges:         classifiedEdges,
		ModuleGraphComplexity:   graphComplexity,
		Seams:                   projectSeams(in.RelationshipSignals.Seams),
		Summary: result.Summary{
			GateFindings: gateNew,
			Warnings:     warnings,
			WaiversUsed:  waiversUsed,
		},
	}

	return d
}

// inScopeFileClasses is the file classification rules read: the source
// inventory minus the files the configuration declared out of scope. The LOC
// walk and the ast-grep scan ignore exclude: and switched-off languages, so
// without this a forbidden_pattern hit in an excluded tree still fired. Only
// forbidden_pattern and module_cycle read it, to keep to production files.
// Metrics and dimensions keep the full index, so a fresh map is built rather
// than deleting from the shared one.
func inScopeFileClasses(f Observations) map[string]fileclass.FileClass {
	if len(f.OutOfScopeFiles) == 0 {
		return f.FileClassIndex
	}
	out := make(map[string]fileclass.FileClass, len(f.FileClassIndex))
	for file, class := range f.FileClassIndex {
		if _, outOfScope := f.OutOfScopeFiles[file]; !outOfScope {
			out[file] = class
		}
	}
	return out
}
