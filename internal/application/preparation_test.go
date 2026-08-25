package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/evidence"
	"github.com/alexei-led/archfit/internal/relationship"
)

const prepareCall = "prepare"

type preparationRecorder struct {
	calls []string
	err   error
}

func (p *preparationRecorder) Prepare(context.Context) error {
	p.calls = append(p.calls, prepareCall)
	return p.err
}

func (p *preparationRecorder) Acquire(context.Context, AnalysisRequest) (Acquired, error) {
	p.calls = append(p.calls, "acquire")
	return Acquired{}, nil
}
func (p *preparationRecorder) Relate(context.Context, Acquired) (relationship.AnalysisResult, error) {
	p.calls = append(p.calls, "relate")
	return relationship.AnalysisResult{}, nil
}
func (p *preparationRecorder) Assess(context.Context, AnalysisRequest, evidence.AssessmentFacts, AnalysisContext, relationship.AnalysisResult) (AnalysisResult, error) {
	p.calls = append(p.calls, "assess")
	diagnostic := result.New()
	diagnostic.Findings = append(diagnostic.Findings, finding.Finding{
		ID: "abc123", RuleID: "rule/test", Kind: finding.KindGate,
		Status: finding.StatusNew, Severity: finding.SeverityMedium,
	})
	return AnalysisResult{Diagnostic: diagnostic}, nil
}

type preparationWriter struct{ calls *[]string }

func (w preparationWriter) Save(context.Context, string, BaselineSnapshot) error {
	*w.calls = append(*w.calls, "save")
	return nil
}

func TestBaselineServicePreparesBeforeAnalysis(t *testing.T) {
	stage := &preparationRecorder{}
	calls := []string{}
	_, err := (BaselineService{
		Preparer: stage, Evidence: stage, Relationship: stage, Assessment: stage, Writer: preparationWriter{calls: &calls},
	}).Execute(t.Context(), BaselineRequest{Path: "baseline.json"})
	if err != nil {
		t.Fatal(err)
	}
	calls = append(stage.calls, calls...)
	if got, want := strings.Join(calls, ","), "prepare,acquire,relate,assess,save"; got != want {
		t.Fatalf("stage calls = %q, want %q", got, want)
	}
}

func TestBaselineServicePreparationFailureStopsAnalysis(t *testing.T) {
	stage := &preparationRecorder{err: errors.New("invalid policy")}
	calls := []string{}
	_, err := (BaselineService{
		Preparer: stage, Evidence: stage, Relationship: stage, Assessment: stage, Writer: preparationWriter{calls: &calls},
	}).Execute(t.Context(), BaselineRequest{Path: "baseline.json"})
	if err == nil || !strings.Contains(err.Error(), "invalid policy") {
		t.Fatalf("Execute error = %v, want preparation failure", err)
	}
	if got := strings.Join(stage.calls, ","); got != prepareCall {
		t.Fatalf("stage calls = %q, want prepare only", got)
	}
	if len(calls) != 0 {
		t.Fatalf("writer calls = %v, want none", calls)
	}
}

func TestExplainServicePreparesBeforeAnalysis(t *testing.T) {
	stage := &preparationRecorder{}
	_, err := (ExplainService{Preparer: stage, Evidence: stage, Relationship: stage, Assessment: stage}).Execute(t.Context(), ExplainRequest{Fingerprint: "abc"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(stage.calls, ","), "prepare,acquire,relate,assess"; got != want {
		t.Fatalf("stage calls = %q, want %q", got, want)
	}
}

func TestExplainServicePreparationFailureStopsAnalysis(t *testing.T) {
	stage := &preparationRecorder{err: errors.New("invalid policy")}
	_, err := (ExplainService{Preparer: stage, Evidence: stage, Relationship: stage, Assessment: stage}).Execute(t.Context(), ExplainRequest{Fingerprint: "abc"})
	if err == nil || !strings.Contains(err.Error(), "invalid policy") {
		t.Fatalf("Execute error = %v, want preparation failure", err)
	}
	if got := strings.Join(stage.calls, ","); got != prepareCall {
		t.Fatalf("stage calls = %q, want prepare only", got)
	}
}
