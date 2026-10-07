package decision_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/decision"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/model/evidence"
)

const (
	comparisonBaseRef = "main"
	// driftedHash stands in for any fingerprint that moved between the two runs.
	driftedHash = "other"
)

func fingerprints() decision.Fingerprints {
	return decision.Fingerprints{
		ClassificationHash: "cls", ModelHash: "model", LabelsHash: "labels", RubricVersion: "bc_score.v6",
		MeasurementProfile: measurementFixture(),
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
			name: "a classification edit is not comparable", mutate: func(f *decision.Fingerprints) { f.ClassificationHash = driftedHash },
			wantStatus: result.StateComparisonNonComparable, wantReason: "classification_hash",
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
	base := decision.Fingerprints{ClassificationHash: "x", ModelHash: "y", LabelsHash: "z", RubricVersion: "w", MeasurementProfile: measurementFixture()}

	got := decision.CompareFingerprints(comparisonBaseRef, fingerprints(), base)
	if len(got.Reasons) != 4 {
		t.Errorf("reasons = %v, want one per drifted input", got.Reasons)
	}
}

func measurementFixture() *evidence.MeasurementProfile {
	return &evidence.MeasurementProfile{Version: evidence.MeasurementProfileVersion, SettingsHash: "settings", Producers: []evidence.MeasurementProducer{{Tool: "loc", SemanticsVersion: "loc.v1", Status: evidence.StatusOK}}}
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

func TestCompareFingerprintsNamesDriftClasses(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*decision.Fingerprints)
		wantDrift []string
	}{
		{"nothing moved", func(*decision.Fingerprints) {}, nil},
		{"classification", func(f *decision.Fingerprints) { f.ClassificationHash = driftedHash }, []string{"classification_hash"}},
		{"reference predates classification_hash", func(f *decision.Fingerprints) { f.ClassificationHash = "" }, []string{"reference_incomplete"}},
		{"model and rubric", func(f *decision.Fingerprints) { f.ModelHash, f.RubricVersion = driftedHash, "bc_score.v5" }, []string{"model_hash", "rubric_version"}},
		{"profile", func(f *decision.Fingerprints) { f.MeasurementProfile = nil }, []string{"measurement_profile"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := fingerprints()
			tc.mutate(&base)
			got := decision.CompareFingerprints(comparisonBaseRef, fingerprints(), base)
			if strings.Join(got.Drift, ",") != strings.Join(tc.wantDrift, ",") {
				t.Errorf("Drift = %v, want %v (reasons %v)", got.Drift, tc.wantDrift, got.Reasons)
			}
		})
	}
}

func TestOnlyDrift(t *testing.T) {
	drift := func(status string, classes ...string) *result.StateComparison {
		return &result.StateComparison{Status: status, Drift: classes}
	}
	rubricAndProfile := []decision.DriftClass{decision.DriftRubric, decision.DriftProfile}
	tests := []struct {
		name string
		cmp  *result.StateComparison
		want bool
	}{
		{"comparable has nothing to cross", drift(result.StateComparisonComparable), false},
		{"nil", nil, false},
		{"allowed classes only", drift(result.StateComparisonNonComparable, "rubric_version", "measurement_profile"), true},
		{"one class outside the allowed set", drift(result.StateComparisonNonComparable, "rubric_version", "labels_hash"), false},
		{"incomplete reference is never implied", drift(result.StateComparisonNonComparable, "reference_incomplete"), false},
		{"non-comparable with no class", drift(result.StateComparisonNonComparable), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := decision.OnlyDrift(tc.cmp, rubricAndProfile...); got != tc.want {
				t.Errorf("OnlyDrift = %v, want %v", got, tc.want)
			}
		})
	}
}
