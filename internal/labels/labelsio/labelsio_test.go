package labelsio_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/labels/labelsio"
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
			got, err := labelsio.Load(write(t, tc.yaml))
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
	got, err := labelsio.Load(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil || got != nil {
		t.Errorf("Load(absent) = (%v, %v), want (nil, nil)", got, err)
	}
}

func TestLoad_ConfidenceProvenance(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		wantErr  string
		wantConf string
		wantProv string
	}{
		{
			name: "valid confidence and provenance round-trip",
			yaml: `version: 1
labels:
  - from: app.a
    to: app.b
    strength: model
    status: approved
    confidence: medium
    provenance: llm
`,
			wantConf: "medium",
			wantProv: "llm",
		},
		{
			name: "omitted confidence and provenance accepted (backward compat)",
			yaml: `version: 1
labels:
  - from: app.a
    to: app.b
    strength: model
    status: approved
`,
			wantConf: "",
			wantProv: "",
		},
		{
			name:    "invalid confidence rejected",
			yaml:    "labels:\n  - {from: a, to: b, strength: model, status: approved, confidence: certain}\n",
			wantErr: "invalid confidence",
		},
		{
			name:    "invalid provenance rejected",
			yaml:    "labels:\n  - {from: a, to: b, strength: model, status: approved, provenance: oracle}\n",
			wantErr: "invalid provenance",
		},
		{
			name: "all valid provenance values accepted",
			yaml: `version: 1
labels:
  - {from: a, to: b, strength: model, status: approved, provenance: human}
  - {from: b, to: c, strength: model, status: approved, provenance: llm}
  - {from: c, to: d, strength: model, status: approved, provenance: tool}
`,
		},
		{
			name: "all valid confidence values accepted",
			yaml: `version: 1
labels:
  - {from: a, to: b, strength: model, status: approved, confidence: high}
  - {from: b, to: c, strength: model, status: approved, confidence: medium}
  - {from: c, to: d, strength: model, status: approved, confidence: low}
`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := labelsio.Load(write(t, tc.yaml))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want substring %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantConf != "" || tc.wantProv != "" {
				if len(got) == 0 {
					t.Fatal("expected at least one label")
				}
				if got[0].Confidence != tc.wantConf {
					t.Errorf("Confidence = %q, want %q", got[0].Confidence, tc.wantConf)
				}
				if got[0].Provenance != tc.wantProv {
					t.Errorf("Provenance = %q, want %q", got[0].Provenance, tc.wantProv)
				}
			}
		})
	}
}
