package decision_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/decision"
	"github.com/alexei-led/archfit/internal/assessment/result"
)

const (
	comparisonBaseRef = "main"
	// driftedHash stands in for any fingerprint that moved between the two runs.
	driftedHash = "other"
)

func fingerprints() decision.Fingerprints {
	return decision.Fingerprints{
		ConfigHash: "cfg", ModelHash: "model", LabelsHash: "labels", RubricVersion: "bc_score.v6",
	}
}

// TestCompareFingerprints pins the strictness: any one of the four inputs
// moving makes the comparison inadmissible, and the reason names which one. A
// policy change that moves a number is not a code change that moves a number.
func TestCompareFingerprints(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*decision.Fingerprints)
		wantStatus string
		wantReason string
	}{
		{name: "identical fingerprints compare", mutate: func(*decision.Fingerprints) {}, wantStatus: result.StateComparisonComparable},
		{
			name: "a config edit is not comparable", mutate: func(f *decision.Fingerprints) { f.ConfigHash = driftedHash },
			wantStatus: result.StateComparisonNonComparable, wantReason: "config_hash",
		},
		{
			// Seam identity comes from module NAMES, so a rename would read as
			// one resolved seam plus one new seam without this check.
			name: "a module rename is not comparable", mutate: func(f *decision.Fingerprints) { f.ModelHash = driftedHash },
			wantStatus: result.StateComparisonNonComparable, wantReason: "model_hash",
		},
		{
			name: "a label change is not comparable", mutate: func(f *decision.Fingerprints) { f.LabelsHash = driftedHash },
			wantStatus: result.StateComparisonNonComparable, wantReason: "labels_hash",
		},
		{
			name: "a rubric change is not comparable", mutate: func(f *decision.Fingerprints) { f.RubricVersion = "bc_score.v5" },
			wantStatus: result.StateComparisonNonComparable, wantReason: "rubric_version",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := fingerprints()
			tc.mutate(&base)
			got := decision.CompareFingerprints(comparisonBaseRef, fingerprints(), base)

			if got.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q (reasons %v)", got.Status, tc.wantStatus, got.Reasons)
			}
			if got.BaseRef != comparisonBaseRef {
				t.Errorf("base_ref = %q, want %q", got.BaseRef, comparisonBaseRef)
			}
			if tc.wantReason == "" {
				if len(got.Reasons) != 0 {
					t.Errorf("reasons = %v, want none on a clean comparison", got.Reasons)
				}
				return
			}
			if !strings.Contains(strings.Join(got.Reasons, "; "), tc.wantReason) {
				t.Errorf("reasons = %v, want the mismatched input %q named", got.Reasons, tc.wantReason)
			}
		})
	}
}

// TestCompareFingerprintsReportsEveryMismatch pins that a run with several
// drifted inputs names all of them: fixing one and finding the comparison still
// refused, with no new information, is the failure mode.
func TestCompareFingerprintsReportsEveryMismatch(t *testing.T) {
	base := decision.Fingerprints{ConfigHash: "x", ModelHash: "y", LabelsHash: "z", RubricVersion: "w"}

	got := decision.CompareFingerprints(comparisonBaseRef, fingerprints(), base)
	if len(got.Reasons) != 4 {
		t.Errorf("reasons = %v, want one per drifted input", got.Reasons)
	}
}

// TestCompareFingerprintsDistinguishesUnsetFromDigest pins the reason text: an
// absent fingerprint and a real digest are different facts and must not print
// the same, or "unset vs unset" would read as a mismatch nobody can chase.
func TestCompareFingerprintsDistinguishesUnsetFromDigest(t *testing.T) {
	head := fingerprints()
	base := head
	base.LabelsHash = ""

	got := decision.CompareFingerprints(comparisonBaseRef, head, base)
	joined := strings.Join(got.Reasons, "; ")
	if !strings.Contains(joined, "unset") {
		t.Errorf("reasons = %q, want an absent fingerprint named as unset", joined)
	}
}

// TestNonComparableStateCarriesTheCallerReason pins that a comparison that
// could not be attempted still explains itself.
func TestNonComparableStateCarriesTheCallerReason(t *testing.T) {
	const reason = "legacy_score_snapshot_ignored"

	got := decision.NonComparableState(comparisonBaseRef, reason)
	if got.Status != result.StateComparisonNonComparable {
		t.Errorf("status = %q, want non_comparable", got.Status)
	}
	if len(got.Reasons) != 1 || got.Reasons[0] != reason {
		t.Errorf("reasons = %v, want exactly the caller's reason", got.Reasons)
	}
}
