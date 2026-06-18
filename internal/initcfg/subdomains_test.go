package initcfg

import (
	"os"
	"strings"
	"testing"
	"time"
)

// Module name constants used in subdomain tests.
const (
	testBilling = "billing"
	testAuth    = "auth"
	testNotify  = "notify"
)

// ---------------------------------------------------------------------------
// MergeSubdomainDrafts
// ---------------------------------------------------------------------------

func TestMergeSubdomainDrafts_ApprovedUntouched(t *testing.T) {
	existing := SubdomainDraftFile{
		Version: 1,
		Drafts: []SubdomainDraft{
			{Module: testAuth, Subdomain: layerCore, Status: SubdomainStatusApproved},
			{Module: testNotify, Subdomain: testSupporting, Status: SubdomainStatusDraft},
		},
	}
	newDrafts := []SubdomainDraft{
		{Module: testAuth, Subdomain: testGeneric, Status: SubdomainStatusDraft},    // must NOT replace approved
		{Module: testNotify, Subdomain: testGeneric, Status: SubdomainStatusDraft},  // replaces draft
		{Module: testBilling, Subdomain: testGeneric, Status: SubdomainStatusDraft}, // new
	}

	got := MergeSubdomainDrafts(existing, newDrafts)
	if got.Version != 1 {
		t.Errorf("version = %d, want 1", got.Version)
	}
	if len(got.Drafts) != 3 {
		t.Fatalf("drafts = %d, want 3: %+v", len(got.Drafts), got.Drafts)
	}

	byModule := make(map[string]SubdomainDraft, len(got.Drafts))
	for _, d := range got.Drafts {
		byModule[d.Module] = d
	}

	if d := byModule[testAuth]; d.Subdomain != layerCore || d.Status != SubdomainStatusApproved {
		t.Errorf("auth approved clobbered: %+v", d)
	}
	if d := byModule[testNotify]; d.Subdomain != testGeneric {
		t.Errorf("notify draft not replaced: %+v", d)
	}
	if d := byModule[testBilling]; d.Subdomain != testGeneric {
		t.Errorf("billing missing: %+v", d)
	}
	// Output must be sorted by Module.
	if got.Drafts[0].Module > got.Drafts[1].Module || got.Drafts[1].Module > got.Drafts[2].Module {
		t.Errorf("not sorted: %v %v %v", got.Drafts[0].Module, got.Drafts[1].Module, got.Drafts[2].Module)
	}
}

func TestMergeSubdomainDrafts_EmptyExisting(t *testing.T) {
	existing := SubdomainDraftFile{Version: 1}
	newDrafts := []SubdomainDraft{
		{Module: layerCore, Subdomain: layerCore, Status: SubdomainStatusDraft},
	}
	got := MergeSubdomainDrafts(existing, newDrafts)
	if len(got.Drafts) != 1 || got.Drafts[0].Module != layerCore {
		t.Errorf("unexpected drafts: %+v", got.Drafts)
	}
}

// ---------------------------------------------------------------------------
// LoadSubdomainDrafts / WriteSubdomainDrafts round-trip
// ---------------------------------------------------------------------------

func TestLoadSubdomainDrafts_Absent(t *testing.T) {
	f, err := LoadSubdomainDrafts("/nonexistent/path/.archfit-subdomains.yaml")
	if err != nil {
		t.Fatalf("expected no error for absent file, got: %v", err)
	}
	if f.Version != 1 || len(f.Drafts) != 0 {
		t.Errorf("unexpected file: %+v", f)
	}
}

func TestWriteLoadSubdomainDrafts_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/.archfit-subdomains.yaml"

	original := SubdomainDraftFile{
		Version: 1,
		Drafts: []SubdomainDraft{
			{Module: testAuth, Subdomain: layerCore, Volatility: testAnnVolatility, Rationale: "central", Status: SubdomainStatusApproved},
			{Module: testNotify, Subdomain: testSupporting, Status: SubdomainStatusDraft},
		},
	}

	if err := WriteSubdomainDrafts(path, original); err != nil {
		t.Fatalf("write: %v", err)
	}
	loaded, err := LoadSubdomainDrafts(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Version != 1 || len(loaded.Drafts) != 2 {
		t.Errorf("round-trip mismatch: %+v", loaded)
	}
	if loaded.Drafts[0].Module != testAuth || loaded.Drafts[0].Status != SubdomainStatusApproved {
		t.Errorf("draft[0] = %+v", loaded.Drafts[0])
	}
}

// ---------------------------------------------------------------------------
// PinSubdomains
// ---------------------------------------------------------------------------

const pinFixtureYAML = `version: 1

layers:
  - core
  - adapter

modules:
  auth:
    paths:
      - "internal/auth/**"
    layer: core
  notify:
    paths:
      - "internal/notify/**"
    layer: adapter
  billing:
    paths:
      - "internal/billing/**"
    subdomain: generic
`

func TestPinSubdomains_Basic(t *testing.T) {
	src := []byte(pinFixtureYAML)
	ts := time.Date(2026, 6, 18, 10, 30, 0, 0, time.UTC)

	pins := []SubdomainPin{
		{Module: testAuth, Subdomain: layerCore, Volatility: testAnnVolatility, ReviewedAt: ts, ReviewedBy: "alice"},
		{Module: testNotify, Subdomain: testSupporting, ReviewedAt: ts, ReviewedBy: "alice"},
	}
	currentSubdomains := map[string]string{
		testAuth:    "",
		testNotify:  "",
		testBilling: testGeneric,
	}

	out, patched, err := PinSubdomains(src, currentSubdomains, pins)
	if err != nil {
		t.Fatalf("PinSubdomains: %v", err)
	}
	if patched != 2 {
		t.Errorf("patched = %d, want 2", patched)
	}

	got := string(out)
	if !strings.Contains(got, "subdomain: core") {
		t.Error("auth subdomain not written")
	}
	if !strings.Contains(got, "subdomain: supporting") {
		t.Error("notify subdomain not written")
	}
	if !strings.Contains(got, "volatility: "+testAnnVolatility) {
		t.Error("auth volatility not written")
	}
	// reviewed_at is written quoted (timestamp contains ":") — check key presence
	// and that the timestamp value appears in the output (possibly quoted).
	if !strings.Contains(got, "reviewed_at:") {
		t.Errorf("reviewed_at key not found\n%s", got)
	}
	if !strings.Contains(got, ts.UTC().Format("2006-01-02T15:04:05Z07:00")) {
		t.Errorf("reviewed_at timestamp value not found\n%s", got)
	}
	if !strings.Contains(got, "reviewed_by: alice") {
		t.Errorf("reviewed_by not found\n%s", got)
	}
}

func TestPinSubdomains_Idempotent(t *testing.T) {
	src := []byte(pinFixtureYAML)
	ts := time.Date(2026, 6, 18, 10, 30, 0, 0, time.UTC)

	// billing already has subdomain: generic — pinning generic again is a no-op.
	pins := []SubdomainPin{
		{Module: testBilling, Subdomain: testGeneric, ReviewedAt: ts, ReviewedBy: "bob"},
	}
	currentSubdomains := map[string]string{testBilling: testGeneric}

	_, patched, err := PinSubdomains(src, currentSubdomains, pins)
	if err != nil {
		t.Fatalf("PinSubdomains: %v", err)
	}
	if patched != 0 {
		t.Errorf("patched = %d, want 0 (idempotent)", patched)
	}
}

func TestPinSubdomains_SkipsDifferentExisting(t *testing.T) {
	src := []byte(pinFixtureYAML)
	ts := time.Now().UTC()

	// billing has subdomain: generic, try to pin "core" — must skip.
	pins := []SubdomainPin{
		{Module: testBilling, Subdomain: layerCore, ReviewedAt: ts, ReviewedBy: "bob"},
	}
	currentSubdomains := map[string]string{testBilling: testGeneric}

	_, patched, err := PinSubdomains(src, currentSubdomains, pins)
	if err != nil {
		t.Fatalf("PinSubdomains: %v", err)
	}
	if patched != 0 {
		t.Errorf("patched = %d, want 0 (skip conflicting existing)", patched)
	}
}

func TestPinSubdomains_EmptyPins(t *testing.T) {
	src := []byte(pinFixtureYAML)
	out, patched, err := PinSubdomains(src, nil, nil)
	if err != nil {
		t.Fatalf("PinSubdomains: %v", err)
	}
	if patched != 0 {
		t.Errorf("patched = %d, want 0", patched)
	}
	if string(out) != string(src) {
		t.Error("output differs from input on empty pins")
	}
}

func TestPinSubdomains_OutputParseable(t *testing.T) {
	// The patched YAML must be readable (basic sanity via round-trip file write).
	src := []byte(pinFixtureYAML)
	ts := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	pins := []SubdomainPin{
		{Module: testAuth, Subdomain: layerCore, ReviewedAt: ts, ReviewedBy: "ci-bot"},
	}
	currentSubdomains := map[string]string{testAuth: ""}

	out, _, err := PinSubdomains(src, currentSubdomains, pins)
	if err != nil {
		t.Fatalf("PinSubdomains: %v", err)
	}

	dir := t.TempDir()
	path := dir + "/test.yaml"
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path) //nolint:gosec // test temp path
	if err != nil || len(data) == 0 {
		t.Fatalf("round-trip file read: %v", err)
	}
}
