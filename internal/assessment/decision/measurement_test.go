package decision_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/decision"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/model/evidence"
)

func TestMeasurementProfileCompatibility(t *testing.T) {
	profile := func() *evidence.MeasurementProfile {
		p := measurementFixture()
		p.Producers = append(p.Producers, evidence.MeasurementProducer{Tool: "scip", SemanticsVersion: "scip.v1", ToolVersion: "scip-go 0.2.7", Status: evidence.StatusOK})
		return p
	}
	for _, tc := range []struct {
		name   string
		change func(*evidence.MeasurementProfile) *evidence.MeasurementProfile
		reason string
	}{
		{"identical", func(p *evidence.MeasurementProfile) *evidence.MeasurementProfile { return p }, ""},
		{"missing", func(*evidence.MeasurementProfile) *evidence.MeasurementProfile { return nil }, "missing"},
		{"unknown contract", func(p *evidence.MeasurementProfile) *evidence.MeasurementProfile { p.Version = "future"; return p }, "unsupported"},
		{"settings", func(p *evidence.MeasurementProfile) *evidence.MeasurementProfile {
			p.SettingsHash = "changed"
			return p
		}, "settings_hash"},
		{"producer missing", func(p *evidence.MeasurementProfile) *evidence.MeasurementProfile {
			p.Producers = p.Producers[:1]
			return p
		}, "scip is missing"},
		{"tool unavailable", func(p *evidence.MeasurementProfile) *evidence.MeasurementProfile {
			p.Producers[1].Status = evidence.StatusAbsent
			p.Producers[1].ToolVersion = ""
			return p
		}, "availability"},
		{"tool upgrade", func(p *evidence.MeasurementProfile) *evidence.MeasurementProfile {
			p.Producers[1].ToolVersion = "scip-go 0.2.8"
			return p
		}, "tool_version differs"},
		{"unknown tool version", func(p *evidence.MeasurementProfile) *evidence.MeasurementProfile {
			p.Producers[1].ToolVersion = "scip-go"
			return p
		}, "tool_version is unknown"},
		{"partial", func(p *evidence.MeasurementProfile) *evidence.MeasurementProfile {
			p.Producers[1].Status = evidence.StatusPartial
			return p
		}, "incomplete evidence"},
		{"duplicate", func(p *evidence.MeasurementProfile) *evidence.MeasurementProfile {
			p.Producers = append(p.Producers, p.Producers[0])
			return p
		}, "duplicated"},
		{"producer order", func(p *evidence.MeasurementProfile) *evidence.MeasurementProfile {
			p.Producers[0], p.Producers[1] = p.Producers[1], p.Producers[0]
			return p
		}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reasons := decision.CompareMeasurementProfiles(profile(), tc.change(profile()))
			if tc.reason == "" && len(reasons) != 0 {
				t.Fatalf("unexpected reasons: %v", reasons)
			}
			if tc.reason != "" && !strings.Contains(strings.Join(reasons, "; "), tc.reason) {
				t.Fatalf("reasons %v do not name %q", reasons, tc.reason)
			}
		})
	}
}

func TestConfigCompareSuppressesDeltaWithoutComparableMeasurement(t *testing.T) {
	in := decision.ConfigCompareInput{Current: findingsSide(), Candidate: findingsSide()}
	in.Candidate.Score = measuredCard(90)
	in.Candidate.Diag.MeasurementProfile = nil
	got := decision.CompareConfigs(in)
	if got.ScoreDelta != nil || got.Coverage.Status != decision.CoverageNotComparable {
		t.Fatalf("comparison asserted a delta: %+v", got)
	}
	for _, f := range []decision.Fingerprints{{}, {MeasurementProfile: measurementFixture()}} {
		cmp := decision.CompareFingerprints("base", f, decision.Fingerprints{})
		if cmp.Status != result.StateComparisonNonComparable {
			t.Fatalf("missing profile compared: %+v", cmp)
		}
	}
}
