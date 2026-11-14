package evaluation

import (
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	modevidence "github.com/alexei-led/archfit/v3/internal/model/evidence"
)

// project evaluates the assessment stages and assembles the diagnostic. Every
// report-only block it copies was already derived by its owning capability.
// Assessment re-derives none of them.
func project(in AssessInput, metrics Metricset) result.Result {
	syntaxFacts := in.Facts.SyntaxFacts
	coverage := in.Facts.Coverage

	// Metric calculation and diagnostic advisories are owned here.
	// The relationship contract and the acquired signals are the only inputs.
	assessed := evaluate(Input{
		Relationships:      in.Relationships,
		Metrics:            metrics,
		Signals:            runSignals(in.Facts),
		Symbols:            in.Facts.Symbols,
		Coverage:           coverage,
		ChangedFiles:       in.Scope.Changed,
		Policy:             in.Policy.Assessment,
		Now:                in.Now,
		AdvisoryCandidates: in.RelationshipSignals.AdvisoryCandidates,
		IncludeAdvisories:  in.Advisory,
	})
	resolvedFindings := assessed.Findings
	metricResults := assessed.Metrics
	warnings := assessed.Warnings

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
		SchemaVersion:         result.SchemaVersion,
		Base:                  in.BaseRef,
		Head:                  in.Head,
		ConfigHash:            in.ConfigHash,
		ModelHash:             in.ModelHash,
		ClassificationHash:    in.ClassificationHash,
		PrimaryExtractorTools: in.PrimaryExtractorTools,
		Metrics:               metricResults,
		Findings:              resolvedFindings,
		SyntaxFacts:           syntaxFacts,
		FileFacts:             fileFacts,
		DeprecatedDeps:        in.Facts.DeprecatedDeps,
		ToolCoverage:          coverage,
		ClassifiedEdges:       classifiedEdges,
		ModuleGraphComplexity: graphComplexity,
		Seams:                 projectSeams(in.RelationshipSignals.Seams),
		Summary: result.Summary{
			Warnings: warnings,
		},
	}

	return d
}
