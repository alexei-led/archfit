package evaluation

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/metrics"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/state"
	"github.com/alexei-led/archfit/internal/policy"
)

// Matched_by keys of a metric ratchet finding. Values are plain numbers, so the
// text a reader sees and the text an agent task quotes cannot drift.
const (
	matchedByMetric    = "metric"
	matchedByBefore    = "before"
	matchedByAfter     = "after"
	matchedByDelta     = "delta"
	matchedByThreshold = "threshold"
)

// metricBreach reports whether a metric's accepted-baseline delta worsened past
// its configured threshold. It is the single breach predicate.
//
// Direction is the metric's own: a count metric worsens upward past `max_new`,
// a ratio metric worsens downward past `min_delta`. A metric with no delta was
// never compared (no baseline, a baseline that does not compare, or a metric
// version that moved), and `gate: off` opted out entirely.
func metricBreach(m result.MetricResult, c policy.MetricConfig) bool {
	if m.Delta == nil || c.Gate == string(policy.GateOff) {
		return false
	}
	if m.Direction == result.DirectionHigherIsWorse {
		return *m.Delta > float64(metricMaxNew(c))
	}
	return *m.Delta < -metricMinDelta(c)
}

// ratchetFindings turns every tripped metric ratchet into one finding.
//
// A ratchet used to block without a finding, which left an agent with nothing
// to repair and the App with no blocker to name. The finding is keyed by the
// metric alone, never by its values, so a waiver keeps matching while the
// number moves. `gate: warn` yields an advisory; the default and `fail` yield a
// gate. Findings come back in metric registration order.
func ratchetFindings(ms []result.MetricResult, cfg map[string]policy.MetricConfig) []finding.Finding {
	var out []finding.Finding
	for _, m := range ms {
		c := cfg[m.Name]
		if !metricBreach(m, c) {
			continue
		}
		before, after := m.Value-*m.Delta, m.Value
		kind, severity := finding.KindGate, finding.SeverityHigh
		if c.Gate == string(policy.GateWarn) {
			kind, severity = finding.KindAdvisory, finding.SeverityMedium
		}
		f := finding.NewKeyed(finding.RuleIDMetricPrefix+m.Name, "metric_ratchet", m.Name)
		f.Kind, f.Severity, f.Confidence = kind, severity, "high"
		f.Why = fmt.Sprintf("metric %s worsened from %s to %s against the accepted baseline (%s)",
			m.Name, ratchetNumber(before), ratchetNumber(after), ratchetThreshold(m, c))
		f.Constraint = "do not accept this by re-running `archfit baseline`: restore the metric, or ask the owner to review and re-baseline"
		f.MatchedBy = map[string]string{
			matchedByMetric: m.Name, matchedByBefore: ratchetNumber(before), matchedByAfter: ratchetNumber(after),
			matchedByDelta: ratchetNumber(*m.Delta), matchedByThreshold: ratchetThreshold(m, c),
		}
		out = append(out, f)
	}
	return out
}

// ratchetThreshold names the configured limit the delta passed.
func ratchetThreshold(m result.MetricResult, c policy.MetricConfig) string {
	if m.Direction == result.DirectionHigherIsWorse {
		return "max_new " + strconv.Itoa(metricMaxNew(c))
	}
	return "min_delta " + ratchetNumber(metricMinDelta(c))
}

// ratchetNumber prints a metric value rounded to six places, so a delta never
// shows subtraction noise ("-0.20000000000000007").
func ratchetNumber(v float64) string {
	return strconv.FormatFloat(math.Round(v*1e6)/1e6, 'f', -1, 64) // six places hide subtraction noise
}

// ratchetMetricNames lists the metrics a blocking ratchet would compare: they
// can ratchet at all (blast_radius cannot), are enabled, hold a value in the
// stored snapshot, and are neither switched off nor warn-only.
func ratchetMetricNames(snapshot result.MetricSnapshot, cfg map[string]policy.MetricConfig) []string {
	names := make([]string, 0, len(snapshot))
	for _, name := range metrics.RatchetNames(cfg) {
		if _, stored := snapshot[name]; !stored {
			continue
		}
		if g := cfg[name].Gate; g == string(policy.GateOff) || g == string(policy.GateWarn) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// RatchetReference is the stored baseline as the metric ratchets read it.
type RatchetReference struct {
	// Present reports that a baseline file was loaded.
	Present bool
	// Comparable reports that its reference compares with this run.
	Comparable bool
	// Metrics is the stored metric snapshot.
	Metrics result.MetricSnapshot
	// Drift names the input classes that made it non-comparable.
	Drift []string
}

// RatchetsRuleID is the unevaluated-rule ID under which every blocking ratchet
// that could not be decided is reported once. One entry, not one per metric:
// the cause is the reference, and thirty identical lines would bury it.
const RatchetsRuleID = "metric_ratchets"

// unevaluated reports the blocking ratchets that cannot be decided because the
// stored reference does not compare. No baseline file means no ratchet exists,
// and a comparable reference decides them, so both report nothing.
func (r RatchetReference) unevaluated(cfg map[string]policy.MetricConfig) (state.UnevaluatedRule, bool) {
	if !r.Present || r.Comparable {
		return state.UnevaluatedRule{}, false
	}
	names := ratchetMetricNames(r.Metrics, cfg)
	if len(names) == 0 {
		return state.UnevaluatedRule{}, false
	}
	cause := "its inputs changed"
	if len(r.Drift) > 0 {
		cause = "drift: " + strings.Join(r.Drift, ", ")
	}
	return state.UnevaluatedRule{RuleID: RatchetsRuleID, Reason: fmt.Sprintf(
		"reference not comparable (%s): %d metric ratchets cannot be evaluated", cause, len(names))}, true
}
