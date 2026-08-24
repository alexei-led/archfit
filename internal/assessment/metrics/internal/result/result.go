// Package result provides shared scoring helpers for the metrics layer:
// band names, confidence constants, mode constants, and the utility functions
// that map scores to bands, apply confidence caps, compute deltas, and build
// indeterminate (n/a) count results.
package result

import (
	"strings"

	"github.com/alexei-led/archfit/internal/model/report"
)

// Band name constants (spec §10.1).
const (
	BandStrong      = "strong"
	BandServiceable = "serviceable"
	BandMixed       = "mixed"
	BandPoor        = "poor"
	BandCritical    = "critical"
	// BandNA marks a metric that cannot be scored because the underlying signal
	// is absent (e.g. encapsulation when no cross-boundary edge has a classified
	// strength). It is neither good nor bad — it means "no evidence", and must
	// not be conflated with BandCritical (a measured, bad value).
	BandNA = "n/a"
	// BandInformational marks a report-only metric that surfaces facts (hubs,
	// change-impact) without asserting a 0–10 quality verdict. Never gates.
	BandInformational = "info"
)

// Confidence level constants.
const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

// Metric mode constants (the "mode" JSON field, spec §10).
const (
	ModeRatio = "ratio"
	ModeCount = "count"
)

// ModularitySmallN is the node count below which concentration metrics are not
// meaningful (a handful of packages always has one depended-upon core). Below it
// the metric reports low confidence (same discipline as encapsulation coverage).
const ModularitySmallN = 15

// BandScore maps a 0–10 score to a band name.
//
//	strong:      9.0–10.0
//	serviceable: 7.0–8.9
//	mixed:       5.0–6.9
//	poor:        3.0–4.9
//	critical:    0.0–2.9
func BandScore(score float64) string {
	switch {
	case score >= 9.0:
		return BandStrong
	case score >= 7.0:
		return BandServiceable
	case score >= 5.0:
		return BandMixed
	case score >= 3.0:
		return BandPoor
	default:
		return BandCritical
	}
}

// bandRank maps a band name to a comparable rank (higher = better).
func bandRank(band string) int {
	switch band {
	case BandStrong:
		return 4
	case BandServiceable:
		return 3
	case BandMixed:
		return 2
	case BandPoor:
		return 1
	default: // BandCritical
		return 0
	}
}

// bandByRank maps a rank back to a band name.
func bandByRank(rank int) string {
	switch rank {
	case 4:
		return BandStrong
	case 3:
		return BandServiceable
	case 2:
		return BandMixed
	case 1:
		return BandPoor
	default:
		return BandCritical
	}
}

// confidenceCapRank returns the maximum allowed band rank for a given confidence level.
//
//	high:   max "strong"      (rank 4)
//	medium: max "serviceable" (rank 3)
//	low:    max "mixed"       (rank 2)
func confidenceCapRank(confidence string) int {
	switch confidence {
	case ConfidenceHigh:
		return 4
	case ConfidenceMedium:
		return 3
	default: // ConfidenceLow or unknown
		return 2
	}
}

// ApplyConfidenceCap clamps band to the maximum allowed by confidence.
func ApplyConfidenceCap(band, confidence string) string {
	computed := bandRank(band)
	maxRank := confidenceCapRank(confidence)
	if computed > maxRank {
		return bandByRank(maxRank)
	}
	return band
}

// ComputeDelta returns the delta between current and baseline for the given
// metric name and version. Returns nil if no matching baseline entry exists.
func ComputeDelta(current float64, baseline report.MetricSnapshot, name, version string) *float64 {
	if baseline == nil {
		return nil
	}
	snap, ok := baseline[name]
	if !ok || snap.Version != version {
		return nil
	}
	d := current - snap.Value
	return &d
}

// NACount is the indeterminate result for a count-mode informational metric.
func NACount(name, version, def string) report.MetricResult {
	return report.MetricResult{
		Name: name, Value: 0, Display: BandNA, Band: BandNA,
		Confidence: ConfidenceLow, Version: version, Mode: ModeCount, Definition: def,
	}
}

// ShortModule trims a module path to its last two segments for compact display.
func ShortModule(mod string) string {
	parts := strings.Split(strings.ReplaceAll(mod, ".", "/"), "/")
	if len(parts) <= 2 {
		return mod
	}
	return ".../" + strings.Join(parts[len(parts)-2:], "/")
}
