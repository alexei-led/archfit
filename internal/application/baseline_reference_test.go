package application

import (
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/report"
)

func headContext() AnalysisContext {
	return AnalysisContext{ConfigHash: "cfg", ModelHash: "mod", LabelsHash: "lbl"}
}

func matchingSnapshot() *BaselineStateSnapshot {
	return &BaselineStateSnapshot{
		ConfigHash: "cfg", ModelHash: "mod", LabelsHash: "lbl", RubricVersion: report.ScoreVersion,
		QualifyingSeamIDs: []string{"seam-1"},
	}
}

// TestSeamAnchor pins the comparison contract: a stored reference is admissible
// only when all four fingerprints still match, and every refusal names its
// cause. "Not comparable" with no reason is indistinguishable from a bug.
func TestSeamAnchor(t *testing.T) {
	drifted := func(mutate func(*BaselineStateSnapshot)) *BaselineStateSnapshot {
		s := matchingSnapshot()
		mutate(s)
		return s
	}

	tests := []struct {
		name           string
		base           Baseline
		wantComparable bool
		wantReasonHas  string
	}{
		{
			name:          "a pre-state baseline names the ignored scalar snapshot",
			base:          Baseline{Legacy: true},
			wantReasonHas: LegacyScoreIgnored,
		},
		{
			name:           "all four fingerprints match",
			base:           Baseline{State: matchingSnapshot()},
			wantComparable: true,
		},
		{
			name:          "config drift names config_hash",
			base:          Baseline{State: drifted(func(s *BaselineStateSnapshot) { s.ConfigHash = driftedFingerprint })},
			wantReasonHas: keyConfigHash,
		},
		{
			name:          "module rename names model_hash",
			base:          Baseline{State: drifted(func(s *BaselineStateSnapshot) { s.ModelHash = driftedFingerprint })},
			wantReasonHas: keyModelHash,
		},
		{
			name:          "label change names labels_hash",
			base:          Baseline{State: drifted(func(s *BaselineStateSnapshot) { s.LabelsHash = driftedFingerprint })},
			wantReasonHas: keyLabelsHash,
		},
		{
			name:          "rubric change names rubric_version",
			base:          Baseline{State: drifted(func(s *BaselineStateSnapshot) { s.RubricVersion = "bc_score.v1" })},
			wantReasonHas: keyRubricVersion,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			anchor := seamAnchor(tc.base, headContext())
			if anchor.SeamsComparable != tc.wantComparable {
				t.Fatalf("SeamsComparable = %v, want %v", anchor.SeamsComparable, tc.wantComparable)
			}
			if tc.wantComparable {
				if !slices.Equal(anchor.QualifyingSeamIDs, tc.base.State.QualifyingSeamIDs) {
					t.Errorf("seam IDs = %v, want %v", anchor.QualifyingSeamIDs, tc.base.State.QualifyingSeamIDs)
				}
				return
			}
			joined := anchor.NonComparableReason + " " + strings.Join(anchor.SnapshotMismatches, " ")
			if !strings.Contains(joined, tc.wantReasonHas) {
				t.Errorf("reason %q does not name %q", joined, tc.wantReasonHas)
			}
		})
	}
}

// TestSeamAnchor_NoBaselineCarriesNoFalseComparison guards the dangerous
// direction: with no stored reference the gate must never claim comparability,
// because "no seams recorded" and "there were no seams" are different facts.
func TestSeamAnchor_NoBaselineCarriesNoFalseComparison(t *testing.T) {
	anchor := seamAnchor(Baseline{}, headContext())
	if anchor.SeamsComparable {
		t.Error("an absent baseline was treated as a comparable reference")
	}
	if len(anchor.QualifyingSeamIDs) != 0 {
		t.Errorf("an absent baseline supplied seam IDs: %v", anchor.QualifyingSeamIDs)
	}
}
