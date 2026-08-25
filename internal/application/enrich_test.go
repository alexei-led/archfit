package application

import (
	"context"
	"testing"
)

type enrichmentAnalyzerStub struct{}

func (enrichmentAnalyzerStub) AnalyzeEnrichment(context.Context, EnrichmentRequest) (EnrichmentResult, error) {
	return EnrichmentResult{Evidence: EnrichmentEvidence{Edges: []EnrichmentEdge{{FromModule: "a", ToModule: "b", Strength: "unknown"}}}}, nil
}

func TestEnrichServiceValidatesAndPreservesApplicationDTO(t *testing.T) {
	service := EnrichService{Analyzer: enrichmentAnalyzerStub{}}
	if _, err := service.Execute(context.Background(), EnrichmentRequest{}); err == nil {
		t.Fatal("missing config path accepted")
	}
	got, err := service.Execute(context.Background(), EnrichmentRequest{ConfigPath: ".archfit.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Evidence.Edges) != 1 || got.Evidence.Edges[0].Strength != "unknown" {
		t.Fatalf("evidence = %+v", got.Evidence)
	}
	service.Analyzer = nil
	if _, err := service.Execute(context.Background(), EnrichmentRequest{ConfigPath: ".archfit.yaml"}); err == nil {
		t.Fatal("nil analyzer accepted")
	}
}
