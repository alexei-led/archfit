package agentout

import (
	"math"

	"github.com/alexei-led/archfit/internal/model/report"
)

// metricRegression is one metric that worsened against the accepted baseline
// in a run that a metric ratchet blocked.
type metricRegression struct {
	name          string
	before, after float64
	dimension     string
}

// ratchetRegressions names the metrics behind a metric-ratchet block. It is
// the twin of the console and Markdown helpers of the same name: a report
// adapter may not import another adapter, and a tripped ratchet produces no
// finding the state could point at. See console.ratchetRegressions for the
// proof rules; the three copies must agree.
func ratchetRegressions(d report.Document) []metricRegression {
	s := d.State
	if s.Verdict != report.StateBlocked {
		return nil
	}
	analyzerGateFailed := false
	for _, gap := range d.CoverageGaps {
		if gap.Gate == string(report.GateFail) {
			analyzerGateFailed = true
		}
	}
	listAll := s.Decision.ActiveBlockers == 0 && !analyzerGateFailed
	proven := ratchetProvenDimensions(s.Dimensions, analyzerGateFailed)
	var out []metricRegression
	for _, m := range d.Metrics {
		if !worsened(m) {
			continue
		}
		r := metricRegression{name: m.Name, before: m.Value - *m.Delta, after: m.Value}
		r.dimension = owningDimension(s.Dimensions, m.Name)
		if _, ok := proven[r.dimension]; !listAll && !ok {
			continue
		}
		out = append(out, r)
	}
	return out
}

// ratchetProvenDimensions names the dimensions whose failing gate only a
// tripped ratchet can explain.
func ratchetProvenDimensions(dims report.Dimensions, analyzerGateFailed bool) map[string]struct{} {
	out := map[string]struct{}{}
	for _, dim := range dims.All() {
		if dim.Gate != report.GateFail || (analyzerGateFailed && dim.Name == report.DimensionOperations) {
			continue
		}
		explained := false
		for _, ref := range dim.Findings {
			if ref.Kind == report.FindingKindGate {
				explained = true
			}
		}
		if !explained {
			out[dim.Name] = struct{}{}
		}
	}
	return out
}

// worsened reports whether a metric moved the wrong way against the accepted
// baseline. A metric with no delta was never compared.
func worsened(m report.MetricResult) bool {
	if m.Delta == nil {
		return false
	}
	if m.Direction == report.DirectionHigherIsWorse {
		return *m.Delta > 0
	}
	return *m.Delta < 0
}

// owningDimension finds the envelope that publishes the metric.
func owningDimension(dims report.Dimensions, name string) string {
	for _, dim := range dims.All() {
		for _, m := range dim.Metrics {
			if m.Name == name {
				return dim.Name
			}
		}
	}
	return ""
}

// roundValue drops float noise: the baseline value is rebuilt as current minus
// delta, which need not reproduce the stored float.
func roundValue(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}
