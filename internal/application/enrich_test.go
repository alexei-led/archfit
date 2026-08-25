package application

import (
	"context"
	"testing"
)

// TestEnrichServiceValidatesAndProjectsEvidence pins the request validation the
// enrichment use case owns, and that the technical evidence the capture stage
// produced reaches the application DTO unchanged.
func TestEnrichServiceValidatesAndProjectsEvidence(t *testing.T) {
	order := []string{}
	ev := workflowEvidence{order: &order}
	store := &workflowStore{order: &order}
	service := workflowService(ev, store, workflowJudge{order: &order})

	if _, err := service.Execute(context.Background(), EnrichmentRequest{LabelsPath: workflowLabels}); err == nil {
		t.Fatal("missing config path accepted")
	}
	if _, err := service.Execute(context.Background(), EnrichmentRequest{ConfigPath: workflowConfig}); err == nil {
		t.Fatal("missing labels path accepted")
	}

	got, err := service.Execute(context.Background(), EnrichmentRequest{ConfigPath: workflowConfig, LabelsPath: workflowLabels})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Evidence.Edges) != 2 || got.Evidence.Edges[0].Strength != "unknown" {
		t.Fatalf("evidence = %+v", got.Evidence)
	}

	service.Stages.Evidence = nil
	if _, err := service.Execute(context.Background(), EnrichmentRequest{ConfigPath: workflowConfig, LabelsPath: workflowLabels}); err == nil {
		t.Fatal("nil evidence stage accepted")
	}
	service.Stages.Evidence = ev
	service.Judge = nil
	if _, err := service.Execute(context.Background(), EnrichmentRequest{ConfigPath: workflowConfig, LabelsPath: workflowLabels}); err == nil {
		t.Fatal("partially configured workflow accepted")
	}
}
