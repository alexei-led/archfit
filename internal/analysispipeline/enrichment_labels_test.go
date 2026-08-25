package pipeline_test

import (
	"testing"

	pipeline "github.com/alexei-led/archfit/internal/analysispipeline"
	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/relationship/labels"
)

func TestEnrichmentLabelProjectionPreservesReviewMetadata(t *testing.T) {
	in := []labels.Label{{
		From: "a", To: "b", Strength: "model", Rationale: "types cross",
		EvidenceRefs: []string{"api:a"}, Basis: "semantic_judgment", EvidenceHash: "hash",
		Status: labels.StatusDraft, Confidence: labels.ConfidenceMedium, Provenance: labels.ProvenanceLLM,
	}}
	got := pipeline.RelationshipLabelsToApplication(in)
	if len(got) != 1 || got[0].EvidenceHash != "hash" || got[0].Provenance != application.EnrichmentLabelProvenanceLLM || got[0].Confidence != application.EnrichmentLabelConfidenceMedium {
		t.Fatalf("projection = %+v", got)
	}
	back := pipeline.ApplicationLabelsToRelationship(got)
	if len(back) != 1 || back[0].Rationale != in[0].Rationale || back[0].EvidenceRefs[0] != "api:a" {
		t.Fatalf("round trip = %+v", back)
	}
}

func TestMergeEnrichmentDraftsHonorsFreshApprovalAndReplacesStale(t *testing.T) {
	key := pipeline.EnrichmentLabelKey("a", "b")
	existing := []application.EnrichmentLabel{{From: "a", To: "b", Strength: "model", Status: application.EnrichmentLabelStatusApproved, EvidenceHash: "old"}}
	drafts := []application.EnrichmentLabel{{From: "a", To: "b", Strength: "intrusive", Status: application.EnrichmentLabelStatusDraft, EvidenceHash: "new"}}
	got := pipeline.MergeEnrichmentDrafts(existing, drafts, map[string]string{key: "new"})
	if len(got) != 1 || got[0].Strength != "intrusive" {
		t.Fatalf("stale approval was not replaced: %+v", got)
	}
}
