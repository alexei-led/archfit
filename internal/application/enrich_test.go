package application

import (
	"context"
	"io"
	"testing"
)

func TestEnrichServiceValidatesAndPreservesApplicationDTO(t *testing.T) {
	order := []string{}
	service := EnrichService{Stages: StageExecutor{
		Preparer: noopPrepare{}, Evidence: workflowEvidence{order: &order}, Stderr: io.Discard,
	}}
	if _, err := service.Execute(context.Background(), EnrichmentRequest{}); err == nil {
		t.Fatal("missing config path accepted")
	}
	got, err := service.Execute(context.Background(), EnrichmentRequest{ConfigPath: ".archfit.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Evidence.Edges) != 2 || got.Evidence.Edges[0].Strength != "unknown" {
		t.Fatalf("evidence = %+v", got.Evidence)
	}
	service.Stages.Evidence = nil
	if _, err := service.Execute(context.Background(), EnrichmentRequest{ConfigPath: ".archfit.yaml"}); err == nil {
		t.Fatal("nil evidence stage accepted")
	}
}
