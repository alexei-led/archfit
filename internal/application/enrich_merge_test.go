package application

import "testing"

const (
	mergeModA       = "a"
	mergeModB       = "b"
	mergeModC       = "c"
	mergeContract   = "contract"
	mergeModel      = "model"
	mergeFunctional = "functional"
	mergeIntrusive  = "intrusive"
	staleEvidence   = "stale"
	currentEvidence = "current"
)

// TestMergeEnrichmentLabelsKeepsFreshApprovals pins the merge rule: a draft may
// never clobber an approval the gate still honours, but it does replace an
// existing draft and adds new pairs. Output is sorted so a re-run rewrites the
// file byte-identically.
func TestMergeEnrichmentLabelsKeepsFreshApprovals(t *testing.T) {
	t.Parallel()
	existing := []EnrichmentLabel{
		{From: mergeModA, To: mergeModB, Strength: mergeModel, Status: EnrichmentLabelStatusApproved},
		{From: mergeModB, To: mergeModA, Strength: mergeFunctional, Status: EnrichmentLabelStatusDraft},
	}
	drafts := []EnrichmentLabel{
		{From: mergeModA, To: mergeModB, Strength: mergeIntrusive, Status: EnrichmentLabelStatusDraft},
		{From: mergeModB, To: mergeModA, Strength: mergeModel, Status: EnrichmentLabelStatusDraft},
		{From: mergeModC, To: mergeModA, Strength: mergeContract, Status: EnrichmentLabelStatusDraft},
	}

	// Nil evidence is a delta run over a partial graph: nothing was hashed, so
	// no approval can be shown stale and every approval survives.
	merged := mergeEnrichmentLabels(existing, drafts, nil)
	if len(merged) != 3 {
		t.Fatalf("merged = %+v, want 3", merged)
	}
	byKey := map[string]EnrichmentLabel{}
	for _, l := range merged {
		byKey[EnrichmentPairKey(l.From, l.To)] = l
	}
	if got := byKey[EnrichmentPairKey(mergeModA, mergeModB)]; got.Status != EnrichmentLabelStatusApproved || got.Strength != mergeModel {
		t.Errorf("unhashed approval was clobbered: %+v", got)
	}
	if got := byKey[EnrichmentPairKey(mergeModB, mergeModA)]; got.Strength != mergeModel {
		t.Errorf("existing draft not replaced: %+v", got)
	}
	if merged[0].From > merged[1].From || merged[1].From > merged[2].From {
		t.Errorf("not sorted: %+v", merged)
	}
}

// TestMergeEnrichmentLabelsKeepsHandAuthoredApproval covers the case the gate
// supports and enrich must not undo: an approval a human wrote by hand carries
// no evidence_hash, yet the pair IS measured this run. labels.isEffective
// treats an empty hash as effective, so the LLM draft must not overwrite it.
func TestMergeEnrichmentLabelsKeepsHandAuthoredApproval(t *testing.T) {
	t.Parallel()
	key := EnrichmentPairKey(mergeModA, mergeModB)
	existing := []EnrichmentLabel{{
		From: mergeModA, To: mergeModB, Strength: mergeContract,
		Status: EnrichmentLabelStatusApproved, Provenance: EnrichmentLabelProvenanceHuman,
	}}
	drafts := []EnrichmentLabel{{
		From: mergeModA, To: mergeModB, Strength: mergeIntrusive,
		EvidenceHash: currentEvidence, Status: EnrichmentLabelStatusDraft,
	}}

	merged := mergeEnrichmentLabels(existing, drafts, map[string]string{key: currentEvidence})
	if len(merged) != 1 {
		t.Fatalf("merged = %+v, want one entry", merged)
	}
	if merged[0].Status != EnrichmentLabelStatusApproved || merged[0].Strength != mergeContract {
		t.Fatalf("hand-authored approval was overwritten by a draft: %+v", merged[0])
	}
}

func TestMergeEnrichmentLabelsReplacesStaleApproved(t *testing.T) {
	t.Parallel()
	key := EnrichmentPairKey(mergeModA, mergeModB)
	existing := []EnrichmentLabel{{
		From: mergeModA, To: mergeModB, Strength: mergeModel,
		EvidenceHash: staleEvidence, Status: EnrichmentLabelStatusApproved,
	}}
	drafts := []EnrichmentLabel{{
		From: mergeModA, To: mergeModB, Strength: mergeIntrusive,
		EvidenceHash: currentEvidence, Status: EnrichmentLabelStatusDraft,
	}}

	merged := mergeEnrichmentLabels(existing, drafts, map[string]string{key: currentEvidence})
	if len(merged) != 1 {
		t.Fatalf("merged = %+v, want one replacement", merged)
	}
	if merged[0].Status != EnrichmentLabelStatusDraft || merged[0].Strength != mergeIntrusive {
		t.Fatalf("stale approved label was not replaced: %+v", merged[0])
	}
}

// TestMergeEnrichmentLabelsKeepsApprovalWhenEvidenceMatches is the other half of
// the rule: the same pair, but the stored hash still matches the code.
func TestMergeEnrichmentLabelsKeepsApprovalWhenEvidenceMatches(t *testing.T) {
	t.Parallel()
	key := EnrichmentPairKey(mergeModA, mergeModB)
	existing := []EnrichmentLabel{{
		From: mergeModA, To: mergeModB, Strength: mergeModel,
		EvidenceHash: currentEvidence, Status: EnrichmentLabelStatusApproved,
	}}
	drafts := []EnrichmentLabel{{From: mergeModA, To: mergeModB, Strength: mergeIntrusive, Status: EnrichmentLabelStatusDraft}}

	merged := mergeEnrichmentLabels(existing, drafts, map[string]string{key: currentEvidence})
	if len(merged) != 1 || merged[0].Strength != mergeModel || merged[0].Status != EnrichmentLabelStatusApproved {
		t.Fatalf("a fresh approval was clobbered: %+v", merged)
	}
}
