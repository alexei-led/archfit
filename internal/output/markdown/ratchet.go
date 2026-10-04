package markdown

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/internal/model/report"
)

// metricRegression is one metric that worsened against the accepted baseline
// in a run that a metric ratchet blocked.
type metricRegression struct {
	name          string
	before, after float64
	dimension     string
	gate          report.GateState
}

// ratchetRegressions names the metrics behind a metric-ratchet block.
//
// A tripped ratchet produces no finding, so the state cannot point at it; the
// document's metric deltas can. They are read only when the contract proves a
// ratchet blocked. The verdict must be blocked. With no active blocker and no
// required analyzer failing its gate, nothing else can block, so every
// worsened metric is listed. Otherwise the proof is per dimension: a
// dimension's gate fails only from a hard-gate finding routed to it, from a
// tripped ratchet on a metric it owns, or (operations only) from a required
// analyzer failing its gate. A failing dimension with no hard-gate finding ref
// — and, for operations, no failing analyzer gate — therefore holds a tripped
// ratchet, and only its worsened metrics are listed. A ratchet in a dimension a
// blocker also explains cannot be told apart and stays unnamed.
//
// The thresholds (metrics.<name>.max_new / min_delta) and the per-metric gate
// are not in the contract, so every worsened metric of a proven dimension is
// listed, one still inside its threshold included; the section says
// "worsened", never "tripped". The console renderer keeps an identical twin.
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
		r.dimension, r.gate = owningDimension(s.Dimensions, m.Name)
		if _, ok := proven[r.dimension]; !listAll && !ok {
			continue
		}
		out = append(out, r)
	}
	return out
}

// ratchetProvenDimensions names the dimensions whose failing gate only a
// tripped ratchet can explain: no hard-gate finding is routed to them, and the
// operations gate is not also explained by a required analyzer failing.
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
// baseline. A metric with no delta was never compared. The direction default
// mirrors the gate's: anything not higher-is-worse reads as higher-is-better.
func worsened(m report.MetricResult) bool {
	if m.Delta == nil {
		return false
	}
	if m.Direction == report.DirectionHigherIsWorse {
		return *m.Delta > 0
	}
	return *m.Delta < 0
}

// owningDimension finds the envelope that publishes the metric and its gate. A
// metric with an n/a band is in no envelope, so it names none.
func owningDimension(dims report.Dimensions, name string) (string, report.GateState) {
	for _, dim := range dims.All() {
		for _, m := range dim.Metrics {
			if m.Name == name {
				return dim.Name, dim.Gate
			}
		}
	}
	return "", ""
}

// writeMetricRatchet names each worsened metric with its accepted-baseline and
// current values, the gate reference the ratchet compared against, and the
// next step. It writes nothing unless a ratchet provably blocked.
func writeMetricRatchet(b *strings.Builder, ref *report.StateComparison, regressions []metricRegression) {
	if len(regressions) == 0 {
		return
	}
	b.WriteString("\n## Metric ratchet\n\n")
	b.WriteString("Blocked by a metric ratchet, which produces no finding and no agent task. " +
		"Metrics worsened against the accepted baseline:\n\n")
	b.WriteString("| Metric | Accepted baseline | Current | Dimension gate |\n| --- | ---: | ---: | --- |\n")
	for _, r := range regressions {
		gate := "—"
		if r.dimension != "" {
			gate = fmt.Sprintf("%s: %s", r.dimension, r.gate)
		}
		fmt.Fprintf(b, "| %s | %s | %s | %s |\n", mdTableCell(r.name), ratchetValue(r.before), ratchetValue(r.after), gate)
	}
	b.WriteString("\n")
	if ref != nil {
		fmt.Fprintf(b, "- **Gate reference:** %s (`%s`)\n", ref.Status, ref.BaseRef)
		for _, reason := range ref.Reasons {
			fmt.Fprintf(b, "  - %s\n", strings.TrimSpace(reason))
		}
	}
	b.WriteString("- **Next step:** fix the regression; if the new value is intended, an owner reviews it " +
		"and re-runs `archfit baseline` to accept it.\n")
}

// ratchetValue prints a metric value without float noise: the baseline value is
// rebuilt as current minus delta, which need not reproduce the stored float.
func ratchetValue(v float64) string {
	return strconv.FormatFloat(math.Round(v*1e6)/1e6, 'f', -1, 64)
}
