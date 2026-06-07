package baseline_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
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
			{Fingerprint: "aabbcc", RuleID: "r1"},
			{Fingerprint: "ddeeff", RuleID: "r2"},
		},
	}

	tests := []struct {
		fp   string
		want bool
	}{
		{"aabbcc", true},
		{"ddeeff", true},
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
