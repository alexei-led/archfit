package boundary_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/boundary"
	"github.com/alexei-led/archfit/internal/metrics/metricstest"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
)

func TestCoverage_Ratio(t *testing.T) {
	// FilesSeen=8, FilesApplicable=10 → value 0.8
	m := boundary.CoverageMetric{}
	result := m.Calculate(signal.CommonInput{
		ToolCoverage: []diagnostic.Coverage{
			{FilesSeen: 8, FilesApplicable: 10},
		},
	})

	if !metricstest.ApproxEqual(result.Value, 0.8) {
		t.Errorf("expected value 0.8 got %v", result.Value)
	}
	// Direction drives computeVerdict's delta-sign handling (V1 fix): a ratio
	// metric regresses when it FALLS. A wrong stamp silently inverts gating.
	if result.Direction != diagnostic.DirectionHigherIsBetter {
		t.Errorf("direction = %q, want %q", result.Direction, diagnostic.DirectionHigherIsBetter)
	}
}

func TestCoverage_NoExtractors(t *testing.T) {
	// No tool contributed any coverage record: the repo was not analysed. The
	// metric must report n/a (low confidence), never a false-green 100%.
	m := boundary.CoverageMetric{}
	result := m.Calculate(signal.CommonInput{})
	if result.Band != bandNAStr {
		t.Errorf("expected band %q for no extractors, got %q", bandNAStr, result.Band)
	}
	if result.Confidence != confLow {
		t.Errorf("expected confidence low for no extractors, got %q", result.Confidence)
	}
	if result.Value == 1.0 {
		t.Errorf("no extractors must not yield value 1.0 (false-green)")
	}
}

func TestCoverage_ZeroApplicable(t *testing.T) {
	// A record present but with zero applicable files is no evidence of coverage:
	// "100% of an empty file set" is the false-green this gate prevents. It must
	// read n/a, not 1.0 (corrects the earlier zero-applicable→1.0 behaviour; the
	// negative-case acceptance run surfaced it as a residual false-green).
	m := boundary.CoverageMetric{}
	result := m.Calculate(signal.CommonInput{
		ToolCoverage: []diagnostic.Coverage{
			{FilesSeen: 0, FilesApplicable: 0, Status: diagnostic.StatusOK},
		},
	})
	if result.Band != bandNAStr {
		t.Errorf("expected band %q for zero applicable, got %q", bandNAStr, result.Band)
	}
	if result.Value == 1.0 {
		t.Errorf("zero applicable must not yield value 1.0 (false-green)")
	}
}

func TestCoverage_AllAbsent(t *testing.T) {
	// Every structural extractor reported absent (e.g. go/packages on a non-Go
	// repo): records exist but none contributed. The metric must report n/a, not
	// a false-green 100% over an empty file set.
	m := boundary.CoverageMetric{}
	result := m.Calculate(signal.CommonInput{
		ToolCoverage: []diagnostic.Coverage{
			{Tool: "go/packages", Status: diagnostic.StatusAbsent},
			{Tool: "dependency-cruiser", Status: diagnostic.StatusAbsent},
		},
	})
	if result.Band != bandNAStr {
		t.Errorf("expected band %q for all-absent, got %q", bandNAStr, result.Band)
	}
	if result.Value == 1.0 {
		t.Errorf("all-absent must not yield value 1.0 (false-green)")
	}
}

func TestCoverage_AbsentRecordSkipped(t *testing.T) {
	// An absent record alongside a real one must not dilute the ratio: only the
	// contributing extractor (8/10) counts.
	m := boundary.CoverageMetric{}
	result := m.Calculate(signal.CommonInput{
		ToolCoverage: []diagnostic.Coverage{
			{Tool: "go/packages", FilesSeen: 8, FilesApplicable: 10, Status: diagnostic.StatusOK},
			{Tool: "grimp", Status: diagnostic.StatusAbsent},
		},
	})
	if !metricstest.ApproxEqual(result.Value, 0.8) {
		t.Errorf("expected value 0.8 (absent record skipped) got %v", result.Value)
	}
}

func TestCoverage_AuxiliaryToolSkipped(t *testing.T) {
	// An auxiliary tool (e.g. ast-grep syntax pass) may report FilesSeen > 0
	// but FilesApplicable == 0 because it does not define a first-party file
	// scope. Adding its FilesSeen to the numerator without a denominator
	// contribution inflates the ratio above 1.0, which is impossible.
	// The fix: skip tools with FilesApplicable == 0.
	m := boundary.CoverageMetric{}
	result := m.Calculate(signal.CommonInput{
		ToolCoverage: []diagnostic.Coverage{
			{Tool: "dependency-cruiser", FilesSeen: 118, FilesApplicable: 118, Status: diagnostic.StatusOK},
			{Tool: "loc", FilesSeen: 131, FilesApplicable: 131, Status: diagnostic.StatusOK},
			// ast-grep aux: seen=136 but applicable=0 — must not inflate ratio
			{Tool: "ast-grep", FilesSeen: 136, FilesApplicable: 0, Status: diagnostic.StatusOK},
		},
	})
	if result.Value > 1.0 {
		t.Errorf("coverage ratio = %.4f, want <= 1.0 (auxiliary tool with FilesApplicable=0 must not inflate numerator)", result.Value)
	}
	// dc+loc: 249/249 = 1.0
	if !metricstest.ApproxEqual(result.Value, 1.0) {
		t.Errorf("coverage ratio = %.4f, want 1.0 (dc+loc only)", result.Value)
	}
}

func TestBandModel_LowConfidenceCap(t *testing.T) {
	// FilesSeen=10, FilesApplicable=10, Unresolved=9 → ratio=0.9 → low confidence
	// value = 10/10 = 1.0 → score 10 → band would be "strong" → capped to "mixed"
	m := boundary.CoverageMetric{}
	result := m.Calculate(signal.CommonInput{
		ToolCoverage: []diagnostic.Coverage{
			{FilesSeen: 10, FilesApplicable: 10, Unresolved: 9},
		},
	})

	if result.Confidence != confLow {
		t.Errorf("expected confidence low got %q", result.Confidence)
	}
	if result.Band != bandMixed {
		t.Errorf("expected band mixed (capped from strong) got %q", result.Band)
	}
}
