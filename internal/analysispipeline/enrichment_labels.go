package pipeline

import (
	"sort"

	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/relationship/labels"
)

// RelationshipLabelsToApplication projects domain labels into the application
// review DTO. The application layer never carries relationship label types.
func RelationshipLabelsToApplication(in []labels.Label) []application.EnrichmentLabel {
	out := make([]application.EnrichmentLabel, len(in))
	for i, l := range in {
		out[i] = application.EnrichmentLabel{
			From: l.From, To: l.To, Strength: l.Strength, Rationale: l.Rationale,
			EvidenceRefs: append([]string(nil), l.EvidenceRefs...), Basis: l.Basis,
			EvidenceHash: l.EvidenceHash, Status: l.Status, Confidence: l.Confidence,
			Provenance: l.Provenance,
		}
	}
	return out
}

// ApplicationLabelsToRelationship projects application review DTOs into the
// domain labels consumed by classification and labelsio.
func ApplicationLabelsToRelationship(in []application.EnrichmentLabel) []labels.Label {
	out := make([]labels.Label, len(in))
	for i, l := range in {
		out[i] = labels.Label{
			From: l.From, To: l.To, Strength: l.Strength, Rationale: l.Rationale,
			EvidenceRefs: append([]string(nil), l.EvidenceRefs...), Basis: l.Basis,
			EvidenceHash: l.EvidenceHash, Status: l.Status, Confidence: l.Confidence,
			Provenance: l.Provenance,
		}
	}
	return out
}

// EnrichmentLabelKey returns the stable ordered module-pair key.
func EnrichmentLabelKey(from, to string) string { return labels.Key(from, to) }

// ValidEnrichmentStrength validates a provider-proposed strength.
func ValidEnrichmentStrength(s string) bool { return labels.ValidStrength(s) }

// EffectiveApprovedEnrichmentPairs returns approved, non-stale pair keys for
// the review workflow without exposing relationship label types to callers.
func EffectiveApprovedEnrichmentPairs(in []application.EnrichmentLabel, evidence map[string]string) map[string]struct{} {
	approved, llmApproved, _ := labels.Approved(ApplicationLabelsToRelationship(in), evidence)
	out := make(map[string]struct{}, len(approved)+len(llmApproved))
	for key := range approved {
		out[key] = struct{}{}
	}
	for key := range llmApproved {
		out[key] = struct{}{}
	}
	return out
}

// MergeEnrichmentDrafts preserves fresh approved labels, replaces stale
// approved and existing draft entries, and returns deterministic output.
func MergeEnrichmentDrafts(existing, drafts []application.EnrichmentLabel, evidence map[string]string) []application.EnrichmentLabel {
	byKey := map[string]application.EnrichmentLabel{}
	approved := EffectiveApprovedEnrichmentPairs(existing, evidence)
	for _, l := range existing {
		byKey[EnrichmentLabelKey(l.From, l.To)] = l
	}
	for _, d := range drafts {
		key := EnrichmentLabelKey(d.From, d.To)
		if _, ok := approved[key]; ok {
			continue
		}
		byKey[key] = d
	}
	out := make([]application.EnrichmentLabel, 0, len(byKey))
	for _, l := range byKey {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out
}
