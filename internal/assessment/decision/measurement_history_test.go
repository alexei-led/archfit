package decision_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/decision"
	"github.com/alexei-led/archfit/internal/model/evidence"
)

func TestHistoryAlgorithmAndConfigurationStillGuardComparison(t *testing.T) {
	profile := func() *evidence.MeasurementProfile {
		semantics, _ := evidence.MeasurementContract("git-history")
		p := measurementFixture()
		p.Producers = []evidence.MeasurementProducer{{Tool: "git-history", SemanticsVersion: semantics, ToolVersion: "git 2.49.0", Status: evidence.StatusOK}}
		return p
	}
	head, base := profile(), profile()
	if reasons := decision.CompareMeasurementProfiles(head, base); len(reasons) != 0 {
		t.Fatalf("same history algorithm must compare: %v", reasons)
	}
	base.Producers[0].SemanticsVersion += ".changed"
	if reasons := decision.CompareMeasurementProfiles(head, base); len(reasons) == 0 {
		t.Fatal("changed history algorithm remained comparable")
	}
	base = profile()
	base.SettingsHash = "changed-configured-settings"
	if reasons := decision.CompareMeasurementProfiles(head, base); len(reasons) == 0 {
		t.Fatal("changed configured settings remained comparable")
	}
}
