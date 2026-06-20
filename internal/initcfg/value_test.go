package initcfg

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Owner/volatility draft test fixtures. vtReviewer/vtOwnerAuth are unique to this
// file so they each appear once (goconst-safe); the owner field key is a local
// var, not a const, because yamledit.go already holds the bare "owner" literal.
const (
	vtReviewer  = "tester"
	vtOwnerAuth = "@team-auth"
)

func TestValueDrafts_RoundTrip(t *testing.T) {
	field := "owner"
	dir := t.TempDir()
	path := filepath.Join(dir, ".archfit-owners.yaml")

	// Absent file → empty container with the requested field.
	empty, err := LoadValueDrafts(path, field)
	if err != nil {
		t.Fatalf("load absent: %v", err)
	}
	if empty.Version != 1 || empty.Field != field || len(empty.Drafts) != 0 {
		t.Fatalf("absent load = %+v, want version 1, field owner, no drafts", empty)
	}

	in := ValueDraftFile{
		Version: 1,
		Field:   field,
		Drafts: []ValueDraft{
			{Module: testAuth, Value: vtOwnerAuth, Rationale: "owns login", Status: DraftStatusDraft},
		},
	}
	if err := WriteValueDrafts(path, in); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := LoadValueDrafts(path, field)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(got.Drafts) != 1 || got.Drafts[0].Value != vtOwnerAuth {
		t.Fatalf("reload = %+v, want one draft %s", got, vtOwnerAuth)
	}
}

func TestMergeValueDrafts_ApprovedUntouchableAndSorted(t *testing.T) {
	existing := ValueDraftFile{
		Version: 1,
		Drafts: []ValueDraft{
			{Module: testAuth, Value: "@old", Status: DraftStatusApproved},
			{Module: testNotify, Value: "@draft-old", Status: DraftStatusDraft},
		},
	}
	merged := MergeValueDrafts(existing, []ValueDraft{
		{Module: testAuth, Value: "@new", Status: DraftStatusDraft},          // ignored (approved)
		{Module: testNotify, Value: "@new-notify", Status: DraftStatusDraft}, // replaces existing draft
		{Module: testBilling, Value: "@billing", Status: DraftStatusDraft},   // appended
	})

	byMod := map[string]ValueDraft{}
	for _, d := range merged.Drafts {
		byMod[d.Module] = d
	}
	if byMod[testAuth].Value != "@old" {
		t.Errorf("approved auth was overwritten: %+v", byMod[testAuth])
	}
	if byMod[testNotify].Value != "@new-notify" {
		t.Errorf("notify draft not replaced: %+v", byMod[testNotify])
	}
	if byMod[testBilling].Value != "@billing" {
		t.Errorf("billing not appended: %+v", byMod[testBilling])
	}
	// Deterministic sort by module: auth, billing, notify.
	want := []string{testAuth, testBilling, testNotify}
	for i, d := range merged.Drafts {
		if d.Module != want[i] {
			t.Fatalf("order = %v, want %v", modNames(merged.Drafts), want)
		}
	}
}

func modNames(ds []ValueDraft) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.Module
	}
	return out
}

const valuePinSrc = `version: 1
modules:
  auth:
    paths:
      - "internal/auth/**"
  notify:
    paths:
      - "internal/notify/**"
    owner: "@already"
`

func TestPinModuleValues_Owner(t *testing.T) {
	at := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	current := map[string]string{testAuth: "", testNotify: "@already"}
	pins := []ValuePin{
		{Module: testAuth, Value: vtOwnerAuth, ReviewedAt: at, ReviewedBy: vtReviewer},
		{Module: testNotify, Value: "@override", ReviewedAt: at, ReviewedBy: vtReviewer}, // skipped: already set
	}

	out, patched, err := PinModuleValues([]byte(valuePinSrc), FieldOwner, current, pins)
	if err != nil {
		t.Fatalf("pin: %v", err)
	}
	if patched != 1 {
		t.Fatalf("patched = %d, want 1", patched)
	}
	got := string(out)
	if !strings.Contains(got, "owner:") || !strings.Contains(got, vtOwnerAuth) {
		t.Errorf("auth owner not written:\n%s", got)
	}
	if !strings.Contains(got, "reviewed_by: "+vtReviewer) {
		t.Errorf("reviewed_by not written:\n%s", got)
	}
	if strings.Contains(got, "@override") {
		t.Errorf("existing notify owner was overwritten:\n%s", got)
	}
}

func TestPinModuleValues_VolatilityField(t *testing.T) {
	at := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	current := map[string]string{testAuth: ""}
	// testAnnVolatility ("low") avoids introducing another shared "high"/"medium" literal.
	pins := []ValuePin{{Module: testAuth, Value: testAnnVolatility, ReviewedAt: at, ReviewedBy: vtReviewer}}

	out, patched, err := PinModuleValues([]byte(valuePinSrc), FieldVolatility, current, pins)
	if err != nil {
		t.Fatalf("pin: %v", err)
	}
	if patched != 1 {
		t.Fatalf("patched = %d, want 1", patched)
	}
	if !strings.Contains(string(out), "volatility: "+testAnnVolatility) {
		t.Errorf("volatility not written:\n%s", string(out))
	}
}

func TestPinModuleValues_NoPinsIsNoop(t *testing.T) {
	out, patched, err := PinModuleValues([]byte(valuePinSrc), FieldOwner, map[string]string{}, nil)
	if err != nil {
		t.Fatalf("pin: %v", err)
	}
	if patched != 0 {
		t.Errorf("patched = %d, want 0", patched)
	}
	if string(out) != valuePinSrc {
		t.Errorf("source mutated on no-op pin")
	}
}
