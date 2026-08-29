package baseline_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/status"
	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/model/report"
)

// Shared test fingerprints (goconst).
const (
	fpA = "aabbcc"
	fpB = "ddeeff"
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
	if !strings.Contains(err.Error(), "regenerate with `archfit baseline`") {
		t.Fatalf("schema mismatch is not actionable: %v", err)
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
		Metrics: report.MetricSnapshot{
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

// TestRoundTrip_StateSnapshot: the architecture-state reference survives a
// write/read cycle intact — the fingerprints and the facts they qualify travel
// together or a later delta cannot be trusted.
func TestRoundTrip_StateSnapshot(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "baseline.json")
	want := &baseline.StateSnapshot{
		ConfigHash: "cfg", ModelHash: "mod", LabelsHash: "lbl", RubricVersion: report.ScoreVersion,
		HardGateFindingIDs: []string{fpA},
		QualifyingSeamIDs:  []string{"seam-1", "seam-2"},
		Dimensions: []baseline.DimensionSnapshot{{
			Name: "coupling", Status: "measured", Gate: "warn",
			Coverage: baseline.CoverageSnapshot{Basis: "scored edges", Observed: 3, Total: 4},
			Metrics:  []baseline.MetricSnapshotValue{{Name: "critical_edges", Value: 2, Unit: "count"}},
		}},
	}

	if err := baseline.Save(ctx, path, baseline.Baseline{State: want}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := baseline.Load(ctx, path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.State == nil {
		t.Fatal("state snapshot missing after round trip")
	}
	if got.State.ConfigHash != want.ConfigHash || got.State.ModelHash != want.ModelHash ||
		got.State.LabelsHash != want.LabelsHash || got.State.RubricVersion != want.RubricVersion {
		t.Errorf("fingerprints changed: got %+v", got.State)
	}
	if !slices.Equal(got.State.QualifyingSeamIDs, want.QualifyingSeamIDs) {
		t.Errorf("seam IDs = %v, want %v", got.State.QualifyingSeamIDs, want.QualifyingSeamIDs)
	}
	if !slices.Equal(got.State.HardGateFindingIDs, want.HardGateFindingIDs) {
		t.Errorf("hard-gate IDs = %v, want %v", got.State.HardGateFindingIDs, want.HardGateFindingIDs)
	}
	if len(got.State.Dimensions) != 1 {
		t.Fatalf("dimensions = %d, want 1", len(got.State.Dimensions))
	}
	gotDim, wantDim := got.State.Dimensions[0], want.Dimensions[0]
	if gotDim.Name != wantDim.Name || gotDim.Status != wantDim.Status ||
		gotDim.Gate != wantDim.Gate || gotDim.Coverage != wantDim.Coverage ||
		!slices.Equal(gotDim.Metrics, wantDim.Metrics) {
		t.Errorf("dimension = %+v, want %+v", gotDim, wantDim)
	}
}
