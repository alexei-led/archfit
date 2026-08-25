package boundary

import (
	"fmt"

	assessmentresult "github.com/alexei-led/archfit/internal/assessment/result"

	"github.com/alexei-led/archfit/internal/assessment/metrics/internal/result"
	signal "github.com/alexei-led/archfit/internal/assessment/signals"
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
func (m CoverageMetric) Calculate(in signal.CommonInput) assessmentresult.MetricResult {
	var totalApplicable, totalExtracted, totalUnresolved int
	for _, c := range in.Coverage {
		// An "absent" record means the extractor did not run or found nothing of
		// its language (e.g. go/packages on a non-Go repo). It is not evidence of
		// coverage, so it must not count toward the totals.
		if c.Status == "absent" {
			continue
		}
		// Auxiliary tools (e.g. ast-grep syntax pass) report FilesSeen > 0 but
		// FilesApplicable == 0 because they do not define a first-party file
		// scope — they annotate whatever they match. Counting their FilesSeen
		// without a matching FilesApplicable inflates the ratio above 1.0, which
		// is definitionally impossible. Skip them from both sides.
		if c.FilesApplicable == 0 {
			continue
		}
		totalApplicable += c.FilesApplicable
		totalExtracted += c.FilesSeen
		totalUnresolved += c.Unresolved
	}

	// No applicable files among any contributing extractor: the repo was not
	// analysed at all (no records, every structural extractor absent, or only
	// auxiliary tools like loc/ast-grep that report ok over zero source files).
	// Report n/a (low confidence) — "100% of an empty file set" is the false-green
	// this gate exists to prevent, not evidence of full coverage.
	if totalApplicable == 0 {
		res := result.NACount(m.Name(), m.Version(), "extracted_files / applicable_files")
		res.Direction = assessmentresult.DirectionHigherIsBetter
		return res
	}

	value := float64(totalExtracted) / float64(totalApplicable)

	// Confidence: based on unresolved ratio vs files seen.
	confidence := coverageConfidence(totalUnresolved, totalExtracted)

	score := value * 10.0
	band := result.ApplyConfidenceCap(result.BandScore(score), confidence)
	display := fmt.Sprintf("%.0f%% coverage", value*100)
	delta := result.ComputeDelta(value, in.Baseline, m.Name(), m.Version())

	return assessmentresult.MetricResult{
		Name:       m.Name(),
		Value:      value,
		Display:    display,
		Band:       band,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       result.ModeRatio,
		Definition: "extracted_files / applicable_files",
		Delta:      delta,
		Direction:  assessmentresult.DirectionHigherIsBetter,
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
