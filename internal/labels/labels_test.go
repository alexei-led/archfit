package labels_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/labels"
)

const (
	modStore      = "app.store"
	modHandlers   = "app.handlers"
	strengthModel = "model"
)

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".archfit-labels.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantN   int
		wantErr string
	}{
		{
			name: "valid file",
			yaml: `version: 1
labels:
  - from: app.handlers
    to: app.store
    strength: model
    rationale: "store types cross the boundary"
    evidence_hash: abc
    status: approved
  - from: app.handlers
    to: app.util
    strength: functional
    status: draft
`,
			wantN: 2,
		},
		{
			name:    "invalid strength rejected",
			yaml:    "labels:\n  - {from: a, to: b, strength: strong, status: approved}\n",
			wantErr: "invalid strength",
		},
		{
			name:    "invalid status rejected",
			yaml:    "labels:\n  - {from: a, to: b, strength: model, status: pinned}\n",
			wantErr: "invalid status",
		},
		{
			name:    "missing endpoints rejected",
			yaml:    "labels:\n  - {strength: model, status: approved}\n",
			wantErr: "from and to are required",
		},
		{
			name:    "malformed yaml rejected",
			yaml:    "labels: [unclosed",
			wantErr: "parse",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := labels.Load(write(t, tc.yaml))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want substring %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tc.wantN {
				t.Errorf("labels = %d, want %d", len(got), tc.wantN)
			}
		})
	}
}

func TestLoad_MissingFileIsNotAnError(t *testing.T) {
	got, err := labels.Load(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil || got != nil {
		t.Errorf("Load(absent) = (%v, %v), want (nil, nil)", got, err)
	}
}

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
