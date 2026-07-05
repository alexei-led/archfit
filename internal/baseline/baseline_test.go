package baseline_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/status"
)

// Shared test fingerprints (goconst).
const (
	fpA = "aabbcc"
	fpB = "ddeeff"

	bandMixed = "mixed"
)

func TestLoad_MissingFile(t *testing.T) {
	ctx := context.Background()
	b, err := baseline.Load(ctx, filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if b.SchemaVersion != "" {
		t.Errorf("expected empty SchemaVersion, got %q", b.SchemaVersion)
	}
	if len(b.Accepted) != 0 {
		t.Errorf("expected no accepted findings, got %d", len(b.Accepted))
	}
}

func TestLoad_SchemaMismatch(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")

	data, _ := json.Marshal(map[string]any{
		"schema_version": "archfit.baseline.v0",
		"accepted":       []any{},
	})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := baseline.Load(ctx, path)
	if err == nil {
		t.Fatal("expected error on schema version mismatch, got nil")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")

	if err := os.WriteFile(path, []byte("not json{{{"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := baseline.Load(ctx, path)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")

	want := baseline.Baseline{
		Accepted: []baseline.AcceptedFinding{
			{Fingerprint: "abc123", RuleID: "forbidden_dependency"},
			{Fingerprint: "def456", RuleID: "public_api_only"},
		},
		Metrics: diagnostic.MetricSnapshot{
			"encapsulation": {Value: 8.5, Version: "encapsulation.v1"},
		},
	}

	if err := baseline.Save(ctx, path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := baseline.Load(ctx, path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.SchemaVersion != baseline.SchemaVersion {
		t.Errorf("SchemaVersion: got %q, want %q", got.SchemaVersion, baseline.SchemaVersion)
	}
	if len(got.Accepted) != len(want.Accepted) {
		t.Fatalf("Accepted len: got %d, want %d", len(got.Accepted), len(want.Accepted))
	}
	for i, a := range got.Accepted {
		if a.Fingerprint != want.Accepted[i].Fingerprint {
			t.Errorf("Accepted[%d].Fingerprint: got %q, want %q", i, a.Fingerprint, want.Accepted[i].Fingerprint)
		}
		if a.RuleID != want.Accepted[i].RuleID {
			t.Errorf("Accepted[%d].RuleID: got %q, want %q", i, a.RuleID, want.Accepted[i].RuleID)
		}
	}
	snap := got.Metrics["encapsulation"]
	if snap.Value != 8.5 {
		t.Errorf("Metrics[encapsulation].Value: got %v, want 8.5", snap.Value)
	}
}

func TestSave_SetsSchemaVersion(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")

	// Save a baseline with no SchemaVersion set — Save must inject it.
	b := baseline.Baseline{}
	if err := baseline.Save(ctx, path, b); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, err := os.ReadFile(path) //nolint:gosec // path from t.TempDir(), trusted in tests
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["schema_version"] != baseline.SchemaVersion {
		t.Errorf("schema_version in file: got %v, want %q", m["schema_version"], baseline.SchemaVersion)
	}
}

func TestSave_EmptySlicesNotNull(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")

	if err := baseline.Save(ctx, path, baseline.Baseline{}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, err := os.ReadFile(path) //nolint:gosec // path from t.TempDir(), trusted in tests
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}

	accepted, ok := m["accepted"]
	if !ok {
		t.Fatal("missing accepted field")
	}
	if accepted == nil {
		t.Error("accepted must not be null")
	}
}

func TestHasFingerprint(t *testing.T) {
	b := baseline.Baseline{
		Accepted: []baseline.AcceptedFinding{
			{Fingerprint: fpA, RuleID: "r1"},
			{Fingerprint: fpB, RuleID: "r2"},
		},
	}

	tests := []struct {
		fp   string
		want bool
	}{
		{fpA, true},
		{fpB, true},
		{"000000", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.fp, func(t *testing.T) {
			if got := b.HasFingerprint(tc.fp); got != tc.want {
				t.Errorf("HasFingerprint(%q) = %v, want %v", tc.fp, got, tc.want)
			}
		})
	}
}

func TestEntries(t *testing.T) {
	b := baseline.Baseline{
		Accepted: []baseline.AcceptedFinding{
			{Fingerprint: fpA, RuleID: "r1", Kind: "gate"},
			{Fingerprint: fpB, RuleID: "r2", Kind: "advisory"},
		},
	}

	got := b.Entries()
	want := []status.AcceptedEntry{
		{Fingerprint: fpA, RuleID: "r1", Kind: "gate"},
		{Fingerprint: fpB, RuleID: "r2", Kind: "advisory"},
	}
	if len(got) != len(want) {
		t.Fatalf("Entries() = %d entries, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Entries()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestCouplingScore locks the max_drop anchor contract: only a snapshot
// recorded under the CURRENT scorer version anchors a drop — no snapshot,
// a pre-version-tracking snapshot (empty version), or a different version
// all return nil (a cross-version drop is a methodology change, not a
// regression).
func TestCouplingScore(t *testing.T) {
	tests := []struct {
		name      string
		b         baseline.Baseline
		want      *int
		wantStale bool
	}{
		{
			name: "no snapshot",
			b:    baseline.Baseline{},
		},
		{
			name:      "legacy snapshot without score_version is stale",
			b:         baseline.Baseline{Score: &baseline.ScoreSnapshot{CouplingBalance: 42, Band: bandMixed}},
			wantStale: true,
		},
		{
			name:      "snapshot from a different scorer version is stale",
			b:         baseline.Baseline{Score: &baseline.ScoreSnapshot{CouplingBalance: 42, Band: bandMixed, ScoreVersion: "bc_score.v3"}},
			wantStale: true,
		},
		{
			name: "current-version snapshot anchors",
			b:    baseline.Baseline{Score: &baseline.ScoreSnapshot{CouplingBalance: 42, Band: bandMixed, ScoreVersion: coupling.ScoreVersion}},
			want: func() *int { v := 42; return &v }(),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.b.CouplingScore()
			switch {
			case tc.want == nil && got != nil:
				t.Errorf("CouplingScore() = %d, want nil", *got)
			case tc.want != nil && (got == nil || *got != *tc.want):
				t.Errorf("CouplingScore() = %v, want %d", got, *tc.want)
			}
			if stale := tc.b.ScoreVersionStale(); stale != tc.wantStale {
				t.Errorf("ScoreVersionStale() = %v, want %v", stale, tc.wantStale)
			}
		})
	}
}
