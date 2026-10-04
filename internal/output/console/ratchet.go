package console

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
// ratchet blocked: the verdict is blocked, no active blocker finding exists,
// and no required analyzer failed its gate — the only other causes of a
// blocked verdict. The thresholds (metrics.<name>.max_new / min_delta) and the
// per-metric gate are not in the contract, so every metric that worsened is
// listed, one still inside its threshold included; the section says
// "worsened", never "tripped". The Markdown renderer keeps an identical twin.
func ratchetRegressions(d report.Document) []metricRegression {
	s := d.State
	if s.Verdict != report.StateBlocked || s.Decision.ActiveBlockers > 0 {
		return nil
	}
	for _, gap := range d.CoverageGaps {
		if gap.Gate == string(report.GateFail) {
			return nil
		}
	}
	var out []metricRegression
	for _, m := range d.Metrics {
		if !worsened(m) {
			continue
		}
		r := metricRegression{name: m.Name, before: m.Value - *m.Delta, after: m.Value}
		r.dimension, r.gate = owningDimension(s.Dimensions, m.Name)
		out = append(out, r)
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
	fmt.Fprintf(b, "\nMETRIC RATCHET (%d) — worsened against the accepted baseline\n\n", len(regressions))
	for _, r := range regressions {
		fmt.Fprintf(b, "  %s: %s → %s", r.name, ratchetValue(r.before), ratchetValue(r.after))
		if r.dimension != "" {
			fmt.Fprintf(b, "  ·  %s gate: %s", r.dimension, r.gate)
		}
		b.WriteString("\n")
	}
	if ref != nil {
		fmt.Fprintf(b, "  gate reference: %s  ·  reference: %s\n", ref.Status, ref.BaseRef)
		for _, reason := range ref.Reasons {
			fmt.Fprintf(b, "    %s\n", condense(reason, 140))
		}
	}
	b.WriteString("  next: fix the regression; if the new value is intended, an owner reviews it\n" +
		"        and re-runs `archfit baseline` to accept it\n")
}

// ratchetValue prints a metric value without float noise: the baseline value is
// rebuilt as current minus delta, which need not reproduce the stored float.
func ratchetValue(v float64) string {
	return strconv.FormatFloat(math.Round(v*1e6)/1e6, 'f', -1, 64)
}
