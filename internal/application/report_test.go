package application

import (
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/decision"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/score"
	"github.com/alexei-led/archfit/internal/model/evidence"
)

func TestProjectReportPreservesAssessmentAndScorecardContracts(t *testing.T) {
	diagnostic := result.New()
	diagnostic.Verdict = result.VerdictWarn
	diagnostic.Base = "main"
	diagnostic.Head = "HEAD"
	diagnostic.Summary = result.Summary{Warnings: 2, WaiversUsed: 1}
	diagnostic.ToolCoverage = []evidence.Coverage{{Tool: "scip", Status: evidence.StatusPartial, FilesSeen: 4, FilesApplicable: 5}}
	diagnostic.ClassifiedEdges = &result.ClassifiedEdgeSummary{Total: 7, Scored: 5, MeanBalance: 4.5}

	scorecard := score.Scorecard{
		RubricVersion: score.RubricVersion,
		Overall:       46,
		OverallBand:   score.BandMixed,
		Dimensions: []score.Dimension{{
			Name: score.DimCouplingBalance, Value: 46, Band: score.BandMixed, Confidence: score.ConfidenceHigh,
		}},
	}
	wantDecision := decision.Build(diagnostic, scorecard, nil, false)

	document := ProjectReport(diagnostic, scorecard, nil, false)
	if string(document.Verdict) != string(OutcomeWarn) || document.Base != "main" || document.Head != "HEAD" {
		t.Fatalf("assessment identity projection lost data: %+v", document)
	}
	if document.Summary.Warnings != 2 || document.Summary.WaiversUsed != 1 {
		t.Fatalf("summary projection = %+v", document.Summary)
	}
	if len(document.ToolCoverage) != 1 || document.ToolCoverage[0].Tool != "scip" || document.ToolCoverage[0].FilesApplicable != 5 {
		t.Fatalf("coverage projection = %+v", document.ToolCoverage)
	}
	if document.ClassifiedEdges == nil || document.ClassifiedEdges.Scored != 5 || document.ClassifiedEdges.MeanBalance != 4.5 {
		t.Fatalf("classified edge projection = %+v", document.ClassifiedEdges)
	}
	if document.Score.Overall != 46 || len(document.Score.Dimensions) != 1 {
		t.Fatalf("score projection = %+v", document.Score)
	}
	if string(document.Decision.Band) != string(wantDecision.Band) || document.Decision.Headline != wantDecision.Headline {
		t.Fatalf("decision projection = %+v", document.Decision)
	}
}
