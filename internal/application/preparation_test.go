package application

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
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
	p.calls = append(p.calls, acquireStageCall)
	return Acquired{}, nil
}

func (p *preparationRecorder) stages() StageExecutor {
	return StageExecutor{Preparer: p, Evidence: p, Stderr: io.Discard}
}

type preparationWriter struct{ calls *[]string }

func (w preparationWriter) Save(context.Context, string, BaselineSnapshot) error {
	*w.calls = append(*w.calls, "save")
	return nil
}

func TestBaselineServicePreparesBeforeAnalysis(t *testing.T) {
	stage := &preparationRecorder{}
	calls := []string{}
	_, err := (BaselineService{Stages: stage.stages(), Writer: preparationWriter{calls: &calls}}).
		Execute(t.Context(), BaselineRequest{Path: "baseline.json"})
	if err != nil {
		t.Fatal(err)
	}
	calls = append(stage.calls, calls...)
	if got, want := strings.Join(calls, ","), "prepare,acquire,save"; got != want {
		t.Fatalf("stage calls = %q, want %q", got, want)
	}
}

func TestBaselineServicePreparationFailureStopsAnalysis(t *testing.T) {
	stage := &preparationRecorder{err: errors.New("invalid policy")}
	calls := []string{}
	_, err := (BaselineService{Stages: stage.stages(), Writer: preparationWriter{calls: &calls}}).
		Execute(t.Context(), BaselineRequest{Path: "baseline.json"})
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
	_, err := (ExplainService{Stages: stage.stages()}).Execute(t.Context(), ExplainRequest{Fingerprint: "abc"})
	// An empty measurement has no findings, so the fingerprint cannot resolve —
	// what this test pins is that preparation and acquisition ran in order first.
	if err == nil || !strings.Contains(err.Error(), "no finding with fingerprint prefix") {
		t.Fatalf("Execute error = %v, want an unresolved-fingerprint error", err)
	}
	if got, want := strings.Join(stage.calls, ","), "prepare,acquire"; got != want {
		t.Fatalf("stage calls = %q, want %q", got, want)
	}
}

func TestExplainServicePreparationFailureStopsAnalysis(t *testing.T) {
	stage := &preparationRecorder{err: errors.New("invalid policy")}
	_, err := (ExplainService{Stages: stage.stages()}).Execute(t.Context(), ExplainRequest{Fingerprint: "abc"})
	if err == nil || !strings.Contains(err.Error(), "invalid policy") {
		t.Fatalf("Execute error = %v, want preparation failure", err)
	}
	if got := strings.Join(stage.calls, ","); got != prepareCall {
		t.Fatalf("stage calls = %q, want prepare only", got)
	}
}
