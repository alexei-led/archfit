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
	result := m.Calculate(signal.MetricInput{
		ToolCoverage: []diagnostic.Coverage{
			{FilesSeen: 8, FilesApplicable: 10},
		},
	})

	if !metricstest.ApproxEqual(result.Value, 0.8) {
		t.Errorf("expected value 0.8 got %v", result.Value)
	}
}

func TestCoverage_ZeroApplicable(t *testing.T) {
	m := boundary.CoverageMetric{}
	result := m.Calculate(signal.MetricInput{
		ToolCoverage: []diagnostic.Coverage{
			{FilesSeen: 0, FilesApplicable: 0},
		},
	})
	if result.Value != 1.0 {
		t.Errorf("expected value 1.0 for zero applicable, got %v", result.Value)
	}
}

func TestBandModel_LowConfidenceCap(t *testing.T) {
	// FilesSeen=10, FilesApplicable=10, Unresolved=9 → ratio=0.9 → low confidence
	// value = 10/10 = 1.0 → score 10 → band would be "strong" → capped to "mixed"
	m := boundary.CoverageMetric{}
	result := m.Calculate(signal.MetricInput{
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
