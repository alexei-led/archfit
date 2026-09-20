package decision_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/decision"
	"github.com/alexei-led/archfit/internal/model/evidence"
)

func TestCorrectedMeasurementRejectsPrePatchSemantics(t *testing.T) {
	for _, tc := range []struct {
		tool, previous, version string
	}{
		{"deploy-unit", "deploy-unit.v1", ""},
		{"git-history", "git-history.recent500-full-fallback.v1", "git version 2.55.0"},
		{"grimp", "grimp.v1", "grimp 3.14; python 3.13.0"},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			semantics, _ := evidence.MeasurementContract(tc.tool)
			current := &evidence.MeasurementProfile{
				Version: evidence.MeasurementProfileVersion, SettingsHash: "same-settings",
				Producers: []evidence.MeasurementProducer{{Tool: tc.tool, SemanticsVersion: semantics, ToolVersion: tc.version, Status: evidence.StatusOK}},
			}
			prior := *current
			prior.Producers = append([]evidence.MeasurementProducer(nil), current.Producers...)
			prior.Producers[0].SemanticsVersion = tc.previous
			if reasons := decision.CompareMeasurementProfiles(current, current); len(reasons) != 0 {
				t.Fatalf("same corrected measurement must compare: %v", reasons)
			}
			reasons := strings.Join(decision.CompareMeasurementProfiles(current, &prior), "; ")
			if !strings.Contains(reasons, tc.tool) || !strings.Contains(reasons, "semantics_version") {
				t.Fatalf("pre-patch normalization was accepted or unnamed: %q", reasons)
			}
		})
	}
}
