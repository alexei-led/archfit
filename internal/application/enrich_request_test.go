package application

import (
	"context"
	"io"
	"testing"
)

// captureEvidence records the AnalysisRequest the enrichment stage received.
type captureEvidence struct {
	workflowEvidence
	got *AnalysisRequest
}

func (e captureEvidence) Acquire(ctx context.Context, req AnalysisRequest) (Acquired, error) {
	*e.got = req
	return e.workflowEvidence.Acquire(ctx, req)
}

func TestEnrichCapturesRelationships(t *testing.T) {
	order := []string{}
	var got AnalysisRequest
	ev := captureEvidence{workflowEvidence: workflowEvidence{order: &order}, got: &got}
	service := EnrichService{
		Stages: StageExecutor{Preparer: noopPrepare{}, Evidence: ev, Stderr: io.Discard},
		Labels: &workflowStore{order: &order}, Judge: workflowJudge{order: &order},
	}
	if _, err := service.Execute(context.Background(), EnrichmentRequest{ConfigPath: workflowConfig, LabelsPath: workflowLabels}); err != nil {
		t.Fatal(err)
	}
	if !got.CaptureRelationships {
		t.Errorf("enrichment stage request = %+v, want the relationship capture enabled", got)
	}
}
