package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/evidence"
	"github.com/alexei-led/archfit/internal/relationship"
)

func TestResolveFormats(t *testing.T) {
	tests := []struct {
		name     string
		json     bool
		markdown bool
		sarif    bool
		formats  []string
		want     []string
		wantErr  bool
	}{
		{name: "default", want: []string{FormatText}},
		{name: "json shorthand", json: true, want: []string{FormatJSON}},
		{name: "markdown shorthand", markdown: true, want: []string{FormatMarkdown}},
		{name: "sarif shorthand", sarif: true, want: []string{FormatSARIF}},
		{name: "repeatable", formats: []string{FormatJSON, FormatScorecard}, want: []string{FormatJSON, FormatScorecard}},
		{name: "multiple shorthands", json: true, sarif: true, wantErr: true},
		{name: "mixed shorthand and repeatable", markdown: true, formats: []string{FormatJSON}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveFormats(test.json, test.markdown, test.sarif, test.formats)
			if test.wantErr {
				if err == nil {
					t.Fatal("ResolveFormats error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveFormats: %v", err)
			}
			if len(got) != len(test.want) {
				t.Fatalf("formats = %v, want %v", got, test.want)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Fatalf("formats = %v, want %v", got, test.want)
				}
			}
		})
	}
}

func TestServiceExecuteValidatesFormatsBeforeRunner(t *testing.T) {
	_, err := (Service{}).Execute(t.Context(), Request{JSON: true, SARIF: true})
	if err == nil {
		t.Fatal("Execute error = nil")
	}
	var invalid *InvalidFormatsError
	if !errors.As(err, &invalid) {
		t.Fatalf("Execute error = %T, want *InvalidFormatsError", err)
	}
}

func TestServiceExecuteRequiresAnalysisStageAfterValidation(t *testing.T) {
	_, err := (Service{}).Execute(t.Context(), Request{})
	if err == nil || err.Error() != "analysis stages are required" {
		t.Fatalf("Execute error = %v, want analysis stages are required", err)
	}
}

type orderedAnalysisStage struct {
	calls []string
}

func (s *orderedAnalysisStage) Prepare(context.Context) error {
	s.calls = append(s.calls, "prepare")
	return nil
}

func (s *orderedAnalysisStage) Acquire(context.Context, AnalysisRequest) (evidence.Snapshot, error) {
	s.calls = append(s.calls, "acquire")
	return evidence.Snapshot{}, nil
}
func (s *orderedAnalysisStage) Relate(context.Context, evidence.Snapshot) (relationship.AnalysisResult, error) {
	s.calls = append(s.calls, "relate")
	return relationship.AnalysisResult{}, nil
}
func (s *orderedAnalysisStage) Assess(context.Context, AnalysisRequest, evidence.AssessmentView, relationship.AnalysisResult) (AnalysisResult, error) {
	s.calls = append(s.calls, "assess")
	return AnalysisResult{}, nil
}

const (
	acquireStageCall = "acquire"
	relateStageCall  = "relate"
	assessStageCall  = "assess"
)

func TestStageExecutorStopsAfterEachFailedStage(t *testing.T) {
	for _, failed := range []string{prepareCall, acquireStageCall, relateStageCall, assessStageCall} {
		t.Run(failed, func(t *testing.T) {
			stage := &failureAnalysisStage{failed: failed}
			_, err := (StageExecutor{Preparer: stage, Evidence: stage, Relationship: stage, Assessment: stage}).Execute(t.Context(), AnalysisRequest{})
			if err == nil {
				t.Fatal("Execute error = nil")
			}
			want := map[string][]string{prepareCall: {prepareCall}, acquireStageCall: {prepareCall, acquireStageCall}, relateStageCall: {prepareCall, acquireStageCall, relateStageCall}, assessStageCall: {prepareCall, acquireStageCall, relateStageCall, assessStageCall}}[failed]
			if got := strings.Join(stage.calls, ","); got != strings.Join(want, ",") {
				t.Fatalf("calls = %q, want %q", got, strings.Join(want, ","))
			}
		})
	}
}

type failureAnalysisStage struct {
	calls  []string
	failed string
}

func (s *failureAnalysisStage) Prepare(context.Context) error {
	s.calls = append(s.calls, "prepare")
	if s.failed == "prepare" {
		return errors.New("prepare failed")
	}
	return nil
}
func (s *failureAnalysisStage) Acquire(context.Context, AnalysisRequest) (evidence.Snapshot, error) {
	s.calls = append(s.calls, "acquire")
	if s.failed == "acquire" {
		return evidence.Snapshot{}, errors.New("acquire failed")
	}
	return evidence.Snapshot{}, nil
}
func (s *failureAnalysisStage) Relate(context.Context, evidence.Snapshot) (relationship.AnalysisResult, error) {
	s.calls = append(s.calls, "relate")
	if s.failed == "relate" {
		return relationship.AnalysisResult{}, errors.New("relate failed")
	}
	return relationship.AnalysisResult{}, nil
}
func (s *failureAnalysisStage) Assess(context.Context, AnalysisRequest, evidence.AssessmentView, relationship.AnalysisResult) (AnalysisResult, error) {
	s.calls = append(s.calls, "assess")
	if s.failed == "assess" {
		return AnalysisResult{}, errors.New("assess failed")
	}
	return AnalysisResult{}, nil
}

func TestServicePreparesPolicyBeforeAnalysis(t *testing.T) {
	stage := &orderedAnalysisStage{}
	if _, err := (Service{Preparer: stage, Evidence: stage, Relationship: stage, Assessment: stage}).Execute(t.Context(), Request{}); err != nil {
		t.Fatal(err)
	}
	if got, want := stage.calls, []string{"prepare", "acquire", "relate", "assess"}; len(got) != len(want) || strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("stage calls = %v, want %v", got, want)
	}
}
