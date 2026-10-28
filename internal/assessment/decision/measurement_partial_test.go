package decision_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/decision"
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
)

const partialGoVersion = "go1.26.1"

func TestCompletedPartialMeasurementRemainsComparable(t *testing.T) {
	for _, tc := range []struct {
		tool, version string
		basis         evidence.MeasurementPartialBasis
		row           evidence.Coverage
	}{
		{"dependency-cruiser", "18.3.1", evidence.PartialUnresolvedSpecifiers, evidence.Coverage{Unresolved: 3}},
		{primaryTool, partialGoVersion, evidence.PartialDegradedPrecision, evidence.Coverage{Unresolved: 3, UnresolvedPrecisionOnly: 3}},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			p := measurementFixture()
			semantics, _ := evidence.MeasurementContract(tc.tool)
			p.Producers = []evidence.MeasurementProducer{{Tool: tc.tool, SemanticsVersion: semantics, ToolVersion: tc.version, Status: evidence.StatusPartial, PartialBasis: tc.basis}}
			f := fingerprints()
			f.MeasurementProfile = p
			if got := decision.CompareFingerprints("base", f, f); got.Status != result.StateComparisonComparable {
				t.Fatalf("complete input profile abstained: %+v", got)
			}
			row := tc.row
			row.Tool, row.Version, row.Status = tc.tool, tc.version, evidence.StatusPartial
			cur, cand := side([]evidence.Coverage{row}, nil), side([]evidence.Coverage{row}, nil)
			cur.Diag.MeasurementProfile, cand.Diag.MeasurementProfile = p, p
			cand.Diag.ToolCoverage[0].Unresolved++
			got := decision.CompareConfigs(decision.ConfigCompareInput{Current: cur, Candidate: cand})
			if got.Coverage.Status != decision.CoverageComparableWithGaps || got.ScoreDelta == nil || *got.ScoreDelta != 0 {
				t.Fatalf("same complete inputs must compare with gaps: %+v", got)
			}
		})
	}
}

func TestMeasurementIncompleteOrOpaqueEvidenceStillAbstains(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*evidence.MeasurementProfile)
	}{
		{"missing inputs", func(p *evidence.MeasurementProfile) { p.Producers[0].PartialBasis = "" }},
		{"timeout", func(p *evidence.MeasurementProfile) { p.Producers[0].Status = evidence.StatusTimedOut }},
		{"unknown version", func(p *evidence.MeasurementProfile) { p.Producers[0].ToolVersion = "" }},
		{"opaque configuration", func(p *evidence.MeasurementProfile) { p.Unknowns = []string{"dynamic configuration is unresolved"} }},
		{"unsupported partial basis", func(p *evidence.MeasurementProfile) { p.Producers[0].PartialBasis = "future" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := measurementFixture()
			p.Producers = []evidence.MeasurementProducer{{Tool: primaryTool, SemanticsVersion: primarySemantics(), ToolVersion: partialGoVersion, Status: evidence.StatusPartial, PartialBasis: evidence.PartialDegradedPrecision}}
			tc.mutate(p)
			if reasons := decision.CompareMeasurementProfiles(p, p); len(reasons) == 0 {
				t.Fatal("equal incomplete evidence was treated as proof")
			}
		})
	}
	partial := measurementFixture()
	partial.Producers = []evidence.MeasurementProducer{{Tool: primaryTool, SemanticsVersion: primarySemantics(), ToolVersion: partialGoVersion, Status: evidence.StatusPartial, PartialBasis: evidence.PartialDegradedPrecision}}
	complete := measurementFixture()
	complete.Producers = append([]evidence.MeasurementProducer(nil), partial.Producers...)
	complete.Producers[0].Status, complete.Producers[0].PartialBasis = evidence.StatusOK, ""
	if reasons := decision.CompareMeasurementProfiles(complete, partial); !strings.Contains(strings.Join(reasons, ";"), "availability differs") {
		t.Fatalf("asymmetric evidence compared: %v", reasons)
	}
}

func primarySemantics() string {
	v, _ := evidence.MeasurementContract(primaryTool)
	return v
}

func TestPythonPartialMeasurementNeverClaimsCompleteInputs(t *testing.T) {
	for _, basis := range []evidence.MeasurementPartialBasis{"", evidence.PartialUnresolvedSpecifiers, evidence.PartialDegradedPrecision} {
		p := measurementFixture()
		semantics, _ := evidence.MeasurementContract("grimp")
		p.Producers = []evidence.MeasurementProducer{{Tool: "grimp", SemanticsVersion: semantics, ToolVersion: "grimp 3.17; python 3.14.3", Status: evidence.StatusPartial, PartialBasis: basis}}
		if reasons := decision.CompareMeasurementProfiles(p, p); len(reasons) == 0 {
			t.Fatalf("Python source gaps compared as complete under basis %q", basis)
		}
	}
}

const roslynVersion = "archfit-roslyn 1.0.0; Microsoft.CodeAnalysis 5.6.0; SDK 10.0.402"

// Unresolved Roslyn symbols are missing candidates, not lower precision: an
// unresolved reference may hide a cross-boundary edge, so no partial basis
// makes a partial Roslyn run comparable.
func TestRoslynPartialMeasurementIsNotComparable(t *testing.T) {
	for _, basis := range []evidence.MeasurementPartialBasis{"", evidence.PartialUnresolvedSpecifiers, evidence.PartialDegradedPrecision} {
		p := measurementFixture()
		semantics, _ := evidence.MeasurementContract("roslyn")
		p.Producers = []evidence.MeasurementProducer{{Tool: "roslyn", SemanticsVersion: semantics, ToolVersion: roslynVersion, Status: evidence.StatusPartial, PartialBasis: basis}}
		reasons := decision.CompareMeasurementProfiles(p, p)
		if !strings.Contains(strings.Join(reasons, ";"), "producer roslyn has incomplete evidence") {
			t.Fatalf("partial Roslyn run compared under basis %q: %v", basis, reasons)
		}
	}
}

func TestRoslynProducerVersionIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, version string
		known         bool
	}{
		{"helper identity", roslynVersion, true},
		{"empty", "", false},
		{"unknown", "unknown", false},
		{"helper version only", "archfit-roslyn 1.0.0", false},
		{"no SDK", "archfit-roslyn 1.0.0; Microsoft.CodeAnalysis 5.6.0", false},
		{"no Roslyn release", "archfit-roslyn 1.0.0; SDK 10.0.402", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := measurementFixture()
			semantics, _ := evidence.MeasurementContract("roslyn")
			p.Producers = []evidence.MeasurementProducer{{Tool: "roslyn", SemanticsVersion: semantics, ToolVersion: tc.version, Status: evidence.StatusOK}}
			unknown := strings.Contains(strings.Join(decision.CompareMeasurementProfiles(p, p), ";"), "producer roslyn tool_version is unknown")
			if unknown == tc.known {
				t.Fatalf("version %q known = %v, want %v", tc.version, !unknown, tc.known)
			}
		})
	}
}
