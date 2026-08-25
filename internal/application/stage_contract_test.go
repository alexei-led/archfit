package application

import (
	"context"
	"testing"

	"github.com/alexei-led/archfit/internal/evidence"
	"github.com/alexei-led/archfit/internal/relationship"
)

const acquiredConfigHash = "acquired"

type explicitStageFake struct {
	prepared bool
	gotReq   AnalysisRequest
	gotFacts evidence.AssessmentFacts
	gotCtx   AnalysisContext
	gotRel   relationship.AnalysisResult
}

func (f *explicitStageFake) Prepare(context.Context) error { f.prepared = true; return nil }
func (f *explicitStageFake) Acquire(_ context.Context, req AnalysisRequest) (Acquired, error) {
	f.gotReq = req
	return Acquired{
		Facts:   evidence.Facts{FileLOC: map[string]int{"a.go": 1}},
		Context: AnalysisContext{ConfigHash: acquiredConfigHash, OwnerSource: "codeowners"},
	}, nil
}
func (f *explicitStageFake) Relate(_ context.Context, in Acquired) (relationship.AnalysisResult, error) {
	if in.Context.ConfigHash != acquiredConfigHash {
		return relationship.AnalysisResult{}, nil
	}
	return relationship.AnalysisResult{Relationships: relationship.Set{Nodes: []relationship.Node{{ID: "passed"}}}}, nil
}
func (f *explicitStageFake) Assess(_ context.Context, req AnalysisRequest, facts evidence.AssessmentFacts, runCtx AnalysisContext, rel relationship.AnalysisResult) (AnalysisResult, error) {
	f.gotReq, f.gotFacts, f.gotCtx, f.gotRel = req, facts, runCtx, rel
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
	// Assessment receives the acquisition-time context verbatim: the ownership
	// resolved once during Acquire must not be re-derived downstream.
	if fake.gotCtx.OwnerSource != "codeowners" || fake.gotCtx.ConfigHash != acquiredConfigHash {
		t.Fatalf("assessment did not receive the acquisition context: %+v", fake.gotCtx)
	}
	if fake.gotFacts.FileLOC["a.go"] != 1 {
		t.Fatalf("assessment did not receive the neutral facts projection: %+v", fake.gotFacts)
	}
}
