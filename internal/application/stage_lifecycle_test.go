// Sequencing tests for the application stage lifecycle: the context each stage
// receives, the number of times each stage runs, how a stage error is reported,
// and how a completed analysis becomes a use-case outcome.
package application

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/score"
	"github.com/alexei-led/archfit/internal/policy"
)

// lifecycleStage counts stage invocations and records the context each stage
// was handed, so cancellation and one-use sequencing are directly observable.
type lifecycleStage struct {
	counts   map[string]int
	ctxErrs  map[string]error
	failStep string
	failErr  error
	policy   policy.PolicySnapshot
}

func newLifecycleStage() *lifecycleStage {
	return &lifecycleStage{counts: map[string]int{}, ctxErrs: map[string]error{}}
}

func (s *lifecycleStage) record(ctx context.Context, step string) error {
	s.counts[step]++
	s.ctxErrs[step] = ctx.Err()
	if s.failStep == step {
		return s.failErr
	}
	return nil
}

func (s *lifecycleStage) Prepare(ctx context.Context) error { return s.record(ctx, prepareCall) }
func (s *lifecycleStage) Acquire(ctx context.Context, _ AnalysisRequest) (Acquired, error) {
	return Acquired{Context: AnalysisContext{Policy: s.policy}}, s.record(ctx, acquireStageCall)
}

func executor(s *lifecycleStage) StageExecutor {
	return StageExecutor{Preparer: s, Evidence: s, Stderr: io.Discard}
}

var stageOrder = []string{prepareCall, acquireStageCall}

func TestStageExecutorRunsEachStageExactlyOnce(t *testing.T) {
	stage := newLifecycleStage()
	if _, err := executor(stage).Execute(t.Context(), AnalysisRequest{}); err != nil {
		t.Fatal(err)
	}
	for _, step := range stageOrder {
		if stage.counts[step] != 1 {
			t.Errorf("%s ran %d times, want exactly 1: evidence must not be re-acquired", step, stage.counts[step])
		}
	}
}

// The caller's context reaches every stage, so a cancellation the CLI issues is
// observable inside acquisition, not just at the top.
func TestStageExecutorPropagatesCallerContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	stage := newLifecycleStage()
	if _, err := executor(stage).Execute(ctx, AnalysisRequest{}); err != nil {
		t.Fatalf("Execute error = %v; stages own cancellation handling, the executor does not short-circuit", err)
	}
	for _, step := range stageOrder {
		if !errors.Is(stage.ctxErrs[step], context.Canceled) {
			t.Errorf("%s saw ctx.Err() = %v, want context.Canceled", step, stage.ctxErrs[step])
		}
	}
}

// A stage error is wrapped with the stage that produced it and stays unwrappable
// so callers can inspect the cause.
func TestStageExecutorWrapsStageErrorsWithStageName(t *testing.T) {
	sentinel := errors.New("boom")
	for _, test := range []struct {
		step       string
		wantPrefix string
	}{
		{step: prepareCall, wantPrefix: "policy preparation: "},
		{step: acquireStageCall, wantPrefix: "evidence acquisition: "},
	} {
		t.Run(test.step, func(t *testing.T) {
			stage := newLifecycleStage()
			stage.failStep, stage.failErr = test.step, sentinel
			_, err := executor(stage).Execute(t.Context(), AnalysisRequest{})
			if err == nil {
				t.Fatal("Execute error = nil")
			}
			if got, want := err.Error(), test.wantPrefix+sentinel.Error(); got != want {
				t.Errorf("error = %q, want %q", got, want)
			}
			if !errors.Is(err, sentinel) {
				t.Error("wrapped error does not unwrap to the stage cause")
			}
		})
	}
}

// An invalid rule definition surfaces from the assessment stage, named as such.
func TestStageExecutorWrapsAssessmentErrors(t *testing.T) {
	stage := newLifecycleStage()
	stage.policy = policy.New(policy.TopologyView{}, policy.RelationshipPolicy{}, policy.AssessmentPolicy{},
		policy.GatePolicy{Rules: policy.RuleConfig{Rules: []policy.RuleDef{{ID: "bad", Type: "bogus_type"}}}}, nil, nil)
	_, err := executor(stage).Execute(t.Context(), AnalysisRequest{})
	if err == nil || !strings.HasPrefix(err.Error(), "assessment: ") {
		t.Fatalf("Execute error = %v, want an assessment-stage error", err)
	}
}

func TestStageExecutorRequiresEveryStage(t *testing.T) {
	stage := newLifecycleStage()
	for _, test := range []struct {
		name string
		exec StageExecutor
	}{
		{name: "no stages", exec: StageExecutor{}},
		{name: "missing preparer", exec: StageExecutor{Evidence: stage}},
		{name: "missing evidence", exec: StageExecutor{Preparer: stage}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.exec.Execute(t.Context(), AnalysisRequest{})
			if err == nil || err.Error() != "analysis stages are required" {
				t.Fatalf("Execute error = %v, want analysis stages are required", err)
			}
		})
	}
}

// The verdict and the hard coupling gate map onto the use-case outcome; the CLI
// owns the exit code, not this layer.
func TestServiceOutcomeFromVerdictAndHardGate(t *testing.T) {
	for _, test := range []struct {
		name     string
		verdict  result.Verdict
		hardGate bool
		want     Outcome
	}{
		{name: "pass", verdict: result.VerdictPass, want: OutcomePass},
		{name: string(OutcomeWarn), verdict: result.VerdictWarn, want: OutcomeWarn},
		{name: "fail", verdict: result.VerdictFail, want: OutcomeFail},
		{name: "hard gate over a pass verdict", verdict: result.VerdictPass, hardGate: true, want: OutcomeFail},
		{name: "hard gate over a warn verdict", verdict: result.VerdictWarn, hardGate: true, want: OutcomeFail},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := outcomeFor(AnalysisResult{
				Diagnostic: result.Result{Verdict: test.verdict},
				Score:      score.Scorecard{},
				HardGate:   test.hardGate,
			})
			if got != test.want {
				t.Errorf("Outcome = %q, want %q", got, test.want)
			}
		})
	}
}

// The stage-bound request flags reach the stage, the resolved formats stay on
// the response (no stage reads them), and a stage failure is reported as a
// controlled use-case error, not a raw stage error.
func TestServiceForwardsRequestAndWrapsStageFailure(t *testing.T) {
	t.Run("forwards resolved request", func(t *testing.T) {
		var got AnalysisRequest
		stage := &requestCaptureStage{lifecycleStage: newLifecycleStage(), captured: &got}
		resp, err := (Service{Stages: StageExecutor{Preparer: stage, Evidence: stage, Stderr: io.Discard}}).
			Execute(t.Context(), Request{JSON: true, NoAdvisories: true, RequireTools: true})
		if err != nil {
			t.Fatal(err)
		}
		if !got.NoAdvisories || !got.RequireTools {
			t.Errorf("stage request = %+v, want the user intent forwarded", got)
		}
		// Analyze/check are the only use cases that may hard-gate on a missing
		// required analyzer; baseline/explain/enrich/compare must not.
		if !got.ApplyToolGate {
			t.Errorf("stage request = %+v, want the tool gate enabled for analyze/check", got)
		}
		if got.SuppressGateReasons {
			t.Errorf("stage request = %+v, want analyze to disclose coupling-gate reasons", got)
		}
		if len(resp.Formats) != 1 || resp.Formats[0] != FormatJSON {
			t.Errorf("response formats = %v, want the resolved json format", resp.Formats)
		}
	})

	t.Run("wraps stage failure as an execution error", func(t *testing.T) {
		stage := newLifecycleStage()
		stage.failStep, stage.failErr = acquireStageCall, errors.New("no tools")
		_, err := (Service{Stages: StageExecutor{Preparer: stage, Evidence: stage, Stderr: io.Discard}}).
			Execute(t.Context(), Request{})
		var exec *ExecutionError
		if !errors.As(err, &exec) {
			t.Fatalf("Execute error = %T (%v), want *ExecutionError", err, err)
		}
		if exec.Message != "evidence acquisition: no tools" {
			t.Errorf("message = %q, want the stage cause disclosed", exec.Message)
		}
	})
}

type requestCaptureStage struct {
	*lifecycleStage
	captured *AnalysisRequest
}

func (s *requestCaptureStage) Acquire(ctx context.Context, req AnalysisRequest) (Acquired, error) {
	*s.captured = req
	return s.lifecycleStage.Acquire(ctx, req)
}
