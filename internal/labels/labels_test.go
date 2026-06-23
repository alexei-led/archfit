package labels_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/labels"
)

// TestLabel_ConfidenceProvenance_Fields verifies round-trip of new fields via
// the Label struct (YAML serialisation is covered by labelsio tests).
func TestLabel_ConfidenceProvenance_Fields(t *testing.T) {
	l := labels.Label{
		From:       modA,
		To:         modB,
		Strength:   strengthModel,
		Status:     labels.StatusApproved,
		Confidence: labels.ConfidenceMedium,
		Provenance: labels.ProvenanceLLM,
	}
	if l.Confidence != labels.ConfidenceMedium {
		t.Errorf("Confidence = %q, want %q", l.Confidence, labels.ConfidenceMedium)
	}
	if l.Provenance != labels.ProvenanceLLM {
		t.Errorf("Provenance = %q, want %q", l.Provenance, labels.ProvenanceLLM)
	}
}

func TestLLMApprovedCount(t *testing.T) {
	freshHash := labels.HashItems([]string{"a.go\x00b.go\x00imports"})
	evidence := map[string]string{
		labels.Key(modA, modB): freshHash,
		labels.Key(modC, modD): labels.HashItems([]string{"c.go\x00d.go\x00imports"}),
	}

	cases := []struct {
		name    string
		lbls    []labels.Label
		nilEvid bool
		want    int
	}{
		{
			name: "no labels",
			lbls: nil,
			want: 0,
		},
		{
			name: "draft llm label not counted",
			lbls: []labels.Label{
				{From: modA, To: modB, Strength: strengthModel, EvidenceHash: freshHash,
					Status: labels.StatusDraft, Provenance: labels.ProvenanceLLM, Confidence: labels.ConfidenceMedium},
			},
			want: 0,
		},
		{
			name: "approved llm medium-confidence counted",
			lbls: []labels.Label{
				{From: modA, To: modB, Strength: strengthModel, EvidenceHash: freshHash,
					Status: labels.StatusApproved, Provenance: labels.ProvenanceLLM, Confidence: labels.ConfidenceMedium},
			},
			want: 1,
		},
		{
			name: "approved llm low-confidence counted",
			lbls: []labels.Label{
				{From: modA, To: modB, Strength: strengthModel, EvidenceHash: freshHash,
					Status: labels.StatusApproved, Provenance: labels.ProvenanceLLM, Confidence: labels.ConfidenceLow},
			},
			want: 1,
		},
		{
			name: "approved llm high-confidence NOT counted (user confirmed)",
			lbls: []labels.Label{
				{From: modA, To: modB, Strength: strengthModel, EvidenceHash: freshHash,
					Status: labels.StatusApproved, Provenance: labels.ProvenanceLLM, Confidence: labels.ConfidenceHigh},
			},
			want: 0,
		},
		{
			name: "approved human label not counted",
			lbls: []labels.Label{
				{From: modA, To: modB, Strength: strengthModel, EvidenceHash: freshHash,
					Status: labels.StatusApproved, Provenance: labels.ProvenanceHuman},
			},
			want: 0,
		},
		{
			name: "stale llm label excluded",
			lbls: []labels.Label{
				{From: modA, To: modB, Strength: strengthModel, EvidenceHash: "stale",
					Status: labels.StatusApproved, Provenance: labels.ProvenanceLLM, Confidence: labels.ConfidenceMedium},
			},
			want: 0,
		},
		{
			name:    "nil evidence: llm labels counted regardless (delta run)",
			nilEvid: true,
			lbls: []labels.Label{
				{From: modA, To: modB, Strength: strengthModel,
					Status: labels.StatusApproved, Provenance: labels.ProvenanceLLM, Confidence: labels.ConfidenceMedium},
			},
			want: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ev map[string]string
			if !tc.nilEvid {
				ev = evidence
			}
			got := labels.LLMApprovedCount(tc.lbls, ev)
			if got != tc.want {
				t.Errorf("LLMApprovedCount = %d, want %d", got, tc.want)
			}
		})
	}
}

const (
	modStore      = "app.store"
	modHandlers   = "app.handlers"
	strengthModel = "model"
	modA          = "app.a"
	modB          = "app.b"
	modC          = "app.c"
	modD          = "app.d"
)

func TestHashItems_DeterministicAndChangeSensitive(t *testing.T) {
	itemA := "pkg/h1.go\x00pkg/s1.go\x00imports"
	itemB := "pkg/h2.go\x00pkg/s2.go\x00imports"

	h1 := labels.HashItems([]string{itemA, itemB})
	if h2 := labels.HashItems([]string{itemB, itemA}); h2 != h1 {
		t.Error("hash must be order-independent (sorted before hashing)")
	}
	if labels.HashItems([]string{itemA, itemB, "pkg/h1.go\x00pkg/s2.go\x00imports"}) == h1 {
		t.Error("hash unchanged after evidence changed")
	}
	if labels.HashItems(nil) == h1 {
		t.Error("empty evidence must hash differently")
	}
}

func TestApproved(t *testing.T) {
	freshHash := labels.HashItems([]string{"a.go\x00b.go\x00imports"})
	evidence := map[string]string{
		labels.Key(modHandlers, modStore): freshHash,
		labels.Key(modStore, modHandlers): labels.HashItems([]string{"other"}),
	}

	in := []labels.Label{
		{From: modHandlers, To: modStore, Strength: strengthModel, EvidenceHash: freshHash, Status: labels.StatusApproved},
		{From: modHandlers, To: "app.util", Strength: "functional", Status: labels.StatusDraft},                         // draft → inert
		{From: modStore, To: modHandlers, Strength: strengthModel, EvidenceHash: "dead", Status: labels.StatusApproved}, // stale: mismatch
		{From: "app.a", To: "app.b", Strength: "intrusive", Status: labels.StatusApproved},                              // no hash → applies
		{From: "app.x", To: "app.y", Strength: "contract", EvidenceHash: "ghost", Status: labels.StatusApproved},        // pair has no evidence → applies (moot)
	}

	approved, stale := labels.Approved(in, evidence)
	if len(approved) != 3 {
		t.Fatalf("approved = %v, want 3 entries", approved)
	}
	if approved[labels.Key(modHandlers, modStore)] != strengthModel {
		t.Errorf("fresh approved label missing: %v", approved)
	}
	if approved[labels.Key("app.a", "app.b")] != "intrusive" {
		t.Errorf("hash-less approved label must apply: %v", approved)
	}
	if approved[labels.Key("app.x", "app.y")] != "contract" {
		t.Errorf("no-current-evidence label must apply (moot): %v", approved)
	}
	if len(stale) != 1 || stale[0].From != modStore {
		t.Errorf("stale = %+v, want only the dead-hash label", stale)
	}

	// Nil evidence (delta run): freshness unverifiable → approved labels all apply.
	approvedNil, staleNil := labels.Approved(in, nil)
	if len(approvedNil) != 4 || staleNil != nil {
		t.Errorf("nil evidence: approved=%v stale=%v, want 4 applied / none stale", approvedNil, staleNil)
	}
}
