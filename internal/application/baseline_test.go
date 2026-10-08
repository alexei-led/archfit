package application

import (
	"context"
	"testing"
	"time"

	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/v3/internal/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

const (
	baselineTestFrom = "a.go"
	baselineTestTo   = "b.go"
	baselineTestPath = "baseline.json"
)

type baselineTestPreparer struct{}

func (baselineTestPreparer) Prepare(context.Context) error { return nil }

type baselineTestEvidence struct {
	policy policy.PolicySnapshot
}

func (s baselineTestEvidence) Acquire(context.Context, AnalysisRequest) (Acquired, error) {
	return Acquired{
		Facts: evidence.Facts{Graph: graph.Build([]graph.Facts{{
			Nodes: []graph.Node{
				{Kind: graph.NodeKindFile, Path: baselineTestFrom},
				{Kind: graph.NodeKindFile, Path: baselineTestTo},
			},
			Edges: []graph.Edge{{From: "file:" + baselineTestFrom, To: "file:" + baselineTestTo, Kind: graph.EdgeKindImports}},
		}})},
		Observations: evaluation.Observations{},
		Context: AnalysisContext{
			Now:    time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC),
			Policy: s.policy,
		},
	}, nil
}

type baselineTestWriter struct {
	snapshot BaselineSnapshot
}

func (w *baselineTestWriter) Save(_ context.Context, _ string, snapshot BaselineSnapshot) error {
	w.snapshot = snapshot
	return nil
}

func baselineTestPolicy(expiry string) policy.PolicySnapshot {
	rule := policy.RuleDef{ID: "edge_rule", Type: "forbidden_dependency", From: baselineTestFrom, To: baselineTestTo}
	return policy.New(
		policy.TopologyView{},
		policy.RelationshipPolicy{},
		policy.AssessmentPolicy{Waivers: policy.WaiverSet{Waivers: []policy.WaiverDef{{
			Rule: rule.ID, From: baselineTestFrom, To: baselineTestTo, Expires: expiry,
		}}}},
		policy.GatePolicy{Rules: policy.RuleConfig{Rules: []policy.RuleDef{rule}}},
		nil, nil,
	)
}

func baselineTestService(expiry string, writer *baselineTestWriter) BaselineService {
	return BaselineService{
		Stages: StageExecutor{Preparer: baselineTestPreparer{}, Evidence: baselineTestEvidence{policy: baselineTestPolicy(expiry)}},
		Writer: writer,
	}
}

func TestBaselineSkipsWaivedFindings(t *testing.T) {
	writer := &baselineTestWriter{}
	service := baselineTestService("2099-01-01", writer)
	response, err := service.Execute(context.Background(), BaselineRequest{Path: baselineTestPath})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if response.SkippedWaived != 1 {
		t.Fatalf("SkippedWaived = %d, want 1", response.SkippedWaived)
	}
	if len(writer.snapshot.Accepted) != 0 {
		t.Fatalf("accepted findings = %+v, want none", writer.snapshot.Accepted)
	}
}

func TestBaselineSkipsExpiredWaiverFindings(t *testing.T) {
	writer := &baselineTestWriter{}
	response, err := baselineTestService("2020-01-01", writer).Execute(context.Background(), BaselineRequest{Path: baselineTestPath})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if response.SkippedWaived != 1 {
		t.Fatalf("SkippedWaived = %d, want 1", response.SkippedWaived)
	}
	if len(writer.snapshot.Accepted) != 0 {
		t.Fatalf("accepted findings = %+v, want none", writer.snapshot.Accepted)
	}
}
