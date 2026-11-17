package application

import (
	"testing"

	"github.com/alexei-led/archfit/v4/internal/model/evidence"
)

const (
	testSnapshotChanged = "changed"
	testSnapshotUnknown = "unknown"
)

func TestReportSnapshotBindingAndReferenceStates(t *testing.T) {
	t.Parallel()
	a, b := "sha256:a", "sha256:b"
	file := func(d *string) evidence.ReportFileSnapshot {
		return evidence.ReportFileSnapshot{Digest: d, PolicyDigest: d, Files: map[string]string{"x.go": *d}, Cleanliness: "dirty"}
	}
	for _, tc := range []struct {
		name                       string
		before, after              evidence.ReportFileSnapshot
		consumed                   *string
		binding, stability, status string
	}{
		{"unchanged", file(&a), file(&a), &a, "match", "stable_by_pre_post_check", "captured"},
		{"stable disk B", file(&b), file(&b), &a, "mismatch", testSnapshotChanged, "captured"},
		{"changed file", file(&a), file(&b), &a, "mismatch", testSnapshotChanged, testSnapshotChanged},
		{"missing boundary", file(&a), evidence.ReportFileSnapshot{}, &a, testSnapshotUnknown, testSnapshotUnknown, testSnapshotUnknown},
		{"incomplete subset", evidence.ReportFileSnapshot{PolicyDigest: &a}, evidence.ReportFileSnapshot{PolicyDigest: &a}, &a, "match", testSnapshotUnknown, testSnapshotUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			snap := projectEvidenceSnapshot(tc.before, tc.after, tc.consumed)
			if snap.PolicyBinding != tc.binding || snap.Stability != tc.stability {
				t.Fatalf("snapshot: %+v", snap)
			}
			ref := projectEvidenceRef(evidence.ReportRef{Path: "x.go"}, tc.before, tc.after)
			if ref.Status != tc.status {
				t.Fatalf("ref: %+v", ref)
			}
		})
	}
}
