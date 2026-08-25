package application

import (
	"context"
	"testing"

	"github.com/alexei-led/archfit/internal/evidence"
	"github.com/alexei-led/archfit/internal/relationship"
)

type explicitStageFake struct {
	prepared bool
	gotReq   AnalysisRequest
	gotView  evidence.AssessmentView
	gotRel   relationship.AnalysisResult
}

func (f *explicitStageFake) Prepare(context.Context) error { f.prepared = true; return nil }
func (f *explicitStageFake) Acquire(_ context.Context, req AnalysisRequest) (evidence.Snapshot, error) {
	f.gotReq = req
	return evidence.Snapshot{ConfigHash: "acquired"}, nil
}
func (f *explicitStageFake) Relate(_ context.Context, in evidence.Snapshot) (relationship.AnalysisResult, error) {
	if in.ConfigHash != "acquired" {
		return relationship.AnalysisResult{}, nil
	}
	return relationship.AnalysisResult{Relationships: relationship.Set{Nodes: []relationship.Node{{ID: "passed"}}}}, nil
}
func (f *explicitStageFake) Assess(_ context.Context, req AnalysisRequest, view evidence.AssessmentView, rel relationship.AnalysisResult) (AnalysisResult, error) {
	f.gotReq, f.gotView, f.gotRel = req, view, rel
	return AnalysisResult{}, nil
}

func TestStageExecutorPassesExplicitStageValues(t *testing.T) {
	fake := &explicitStageFake{}
	_, err := (StageExecutor{Preparer: fake, Evidence: fake, Relationship: fake, Assessment: fake}).Execute(t.Context(), AnalysisRequest{BaseRef: lifecycleBaseRef})
	if err != nil {
		t.Fatal(err)
	}
	if !fake.prepared || fake.gotReq.BaseRef != lifecycleBaseRef || fake.gotRel.Relationships.Nodes[0].ID != "passed" {
		t.Fatalf("stage values were not passed: prepared=%v req=%+v rel=%+v", fake.prepared, fake.gotReq, fake.gotRel)
	}
}
