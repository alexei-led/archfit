package application

import (
	"context"
	"errors"
	"strings"
	"testing"
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
			got, err := resolveFormats(test.json, test.markdown, test.sarif, test.formats)
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
	s.calls = append(s.calls, prepareCall)
	return nil
}

func (s *orderedAnalysisStage) Acquire(context.Context, AnalysisRequest) (Acquired, error) {
	s.calls = append(s.calls, acquireStageCall)
	return Acquired{}, nil
}

const acquireStageCall = "acquire"

// TestStageExecutorStopsAfterEachFailedStage pins the short-circuit: a failed
// stage stops the sequence and the error names the stage that failed, so a
// caller never sees a partially measured result reported as a real one.
func TestStageExecutorStopsAfterEachFailedStage(t *testing.T) {
	for _, failed := range []string{prepareCall, acquireStageCall} {
		t.Run(failed, func(t *testing.T) {
			stage := &failureAnalysisStage{failed: failed}
			_, err := (StageExecutor{Preparer: stage, Evidence: stage}).Execute(t.Context(), AnalysisRequest{})
			if err == nil {
				t.Fatal("Execute error = nil")
			}
			want := map[string][]string{
				prepareCall:      {prepareCall},
				acquireStageCall: {prepareCall, acquireStageCall},
			}[failed]
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
	s.calls = append(s.calls, prepareCall)
	if s.failed == prepareCall {
		return errors.New("prepare failed")
	}
	return nil
}

func (s *failureAnalysisStage) Acquire(context.Context, AnalysisRequest) (Acquired, error) {
	s.calls = append(s.calls, acquireStageCall)
	if s.failed == acquireStageCall {
		return Acquired{}, errors.New("acquire failed")
	}
	return Acquired{}, errors.New("stop after acquire")
}

func TestServicePreparesPolicyBeforeAnalysis(t *testing.T) {
	stage := &orderedAnalysisStage{}
	if _, err := (Service{Stages: StageExecutor{Preparer: stage, Evidence: stage}}).Execute(t.Context(), Request{}); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(stage.calls, ","), prepareCall+","+acquireStageCall; got != want {
		t.Fatalf("stage calls = %q, want %q", got, want)
	}
}
