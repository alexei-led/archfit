package boundary

import (
	"fmt"

	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
)

// ---------------------------------------------------------------------------
// CoverageMetric (coverage.v1)
// ---------------------------------------------------------------------------

// CoverageMetric reports extracted_files / applicable_files from ToolCoverage.
// Confidence degrades as the unresolved count grows relative to total files seen.
type CoverageMetric struct{}

// Name returns "coverage".
func (m CoverageMetric) Name() string { return "coverage" }

// Version returns "coverage.v1".
func (m CoverageMetric) Version() string { return "coverage.v1" }

// Calculate computes the coverage ratio and applies confidence-based band capping.
func (m CoverageMetric) Calculate(in signal.CommonInput) diagnostic.MetricResult {
	var totalApplicable, totalExtracted, totalUnresolved int
	for _, c := range in.ToolCoverage {
		totalApplicable += c.FilesApplicable
		totalExtracted += c.FilesSeen
		totalUnresolved += c.Unresolved
	}

	var value float64
	if totalApplicable == 0 {
		value = 1.0
	} else {
		value = float64(totalExtracted) / float64(totalApplicable)
	}

	// Confidence: based on unresolved ratio vs files seen.
	confidence := coverageConfidence(totalUnresolved, totalExtracted)

	score := value * 10.0
	band := result.ApplyConfidenceCap(result.BandScore(score), confidence)
	display := fmt.Sprintf("%.0f%% coverage", value*100)
	delta := result.ComputeDelta(value, in.Baseline, m.Name(), m.Version())

	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      value,
		Display:    display,
		Band:       band,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       result.ModeRatio,
		Definition: "extracted_files / applicable_files",
		Delta:      delta,
	}
}

// coverageConfidence derives a confidence level from the unresolved/total ratio.
//
//	unresolved / total <= 0.05 → high
//	unresolved / total <= 0.20 → medium
//	otherwise                  → low
func coverageConfidence(unresolved, total int) string {
	if total == 0 {
		return result.ConfidenceHigh
	}
	ratio := float64(unresolved) / float64(total)
	switch {
	case ratio <= 0.05:
		return result.ConfidenceHigh
	case ratio <= 0.20:
		return result.ConfidenceMedium
	default:
		return result.ConfidenceLow
	}
}
