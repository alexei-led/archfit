package engine

import (
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/view"
)

const (
	metricCycle         = "cycle"
	metricEncapsulation = "encapsulation"
)

func newTestFinding(status finding.Status) finding.Finding {
	f := finding.New("test_rule", graph.Edge{
		From: "file:pkg/a/a.go",
		To:   "file:pkg/b/b.go",
		Kind: graph.EdgeKindUsesInternal,
	}, nil)
	f.Status = status
	return f
}

// TestComputeVerdict exercises computeVerdict (engine.go) in isolation — it is
// a pure function over constructed Diagnostic/finding inputs, no fixtures
// needed. See V1/V3 in docs/archived/reports/eval-2026-07-02-v1.1.2/00-FINDINGS.md:
// computeVerdict used to treat any Delta < 0 as a regression regardless of
// metric direction (V1, fixed by stamping Direction on MetricResult), and the
// documented metrics.<name> gate/max_new/min_delta knobs were schema-validated
// but never consumed (V3, fixed by passing MetricGates in). Gate semantics
// follow the rule-gate convention: off skips, warn caps at WARN, fail/unset
// blocks. The cycle-metric cases set Direction explicitly to mirror what the
// real cycle metric (internal/metrics/boundary/cycle.go) produces.
func TestComputeVerdict(t *testing.T) {
	tests := []struct {
		name             string
		gateFindings     []finding.Finding
		metrics          []result.MetricResult
		gates            map[string]view.MetricConfig
		activeAdvisories int
		want             result.Verdict
	}{
		{
			name:         "new gate finding fails",
			gateFindings: []finding.Finding{newTestFinding(finding.StatusNew)},
			want:         result.VerdictFail,
		},
		{
			name:         "expired waiver fails",
			gateFindings: []finding.Finding{newTestFinding(finding.StatusExpiredWaiver)},
			want:         result.VerdictFail,
		},
		{
			name:         "fixed gate finding alone passes",
			gateFindings: []finding.Finding{newTestFinding(finding.StatusFixed)},
			want:         result.VerdictPass,
		},
		{
			// Waived findings reach computeVerdict unfiltered in production
			// (status.Assign stamps them, it does not drop them) — the gate
			// loop must not treat a waived finding as a breach.
			name:         "waived gate finding alone passes",
			gateFindings: []finding.Finding{newTestFinding(finding.StatusWaived)},
			want:         result.VerdictPass,
		},
		{
			// Baseline findings likewise reach computeVerdict unfiltered — an
			// accepted pre-existing finding must not flip the verdict.
			name:         "baseline gate finding alone passes",
			gateFindings: []finding.Finding{newTestFinding(finding.StatusBaseline)},
			want:         result.VerdictPass,
		},
		{
			// A new cycle is a count-metric regression (more cycles is worse)
			// and must block — this was the V1 bug: computeVerdict only
			// checked Delta < 0, so a positive count delta was silently
			// ignored. With no metrics.cycle config the gate is unset, and
			// unset = blocking (rule-gate convention).
			name: "new cycle (count metric, Delta=+1, unset gate) fails",
			metrics: []result.MetricResult{
				{Name: metricCycle, Delta: new(1.0), Direction: result.DirectionHigherIsWorse},
			},
			want: result.VerdictFail,
		},
		{
			// Fixing a cycle (fewer cycles, Delta=-1) is an improvement and
			// must PASS — this was the V1 bug: computeVerdict flagged any
			// negative delta as a regression regardless of metric direction,
			// producing a false WARN.
			name: "fixed cycle (count metric, Delta=-1) passes",
			metrics: []result.MetricResult{
				{Name: metricCycle, Delta: new(-1.0), Direction: result.DirectionHigherIsWorse},
			},
			want: result.VerdictPass,
		},
		{
			name: "encapsulation drop (ratio metric, Delta=-0.1, unset gate) fails",
			metrics: []result.MetricResult{
				{Name: metricEncapsulation, Delta: new(-0.1), Direction: result.DirectionHigherIsBetter},
			},
			want: result.VerdictFail,
		},
		{
			name: "encapsulation rise (ratio metric, Delta=+0.1) passes",
			metrics: []result.MetricResult{
				{Name: metricEncapsulation, Delta: new(0.1), Direction: result.DirectionHigherIsBetter},
			},
			want: result.VerdictPass,
		},
		{
			// Direction unset (zero value) must still behave as ratio
			// semantics (breach on negative delta) — the safe default for
			// any MetricResult that predates or omits Direction.
			name: "unset Direction defaults to ratio semantics (Delta=-0.1) fails",
			metrics: []result.MetricResult{
				{Name: metricEncapsulation, Delta: new(-0.1)},
			},
			want: result.VerdictFail,
		},
		{
			name: "unchanged metrics pass",
			metrics: []result.MetricResult{
				{Name: metricCycle, Delta: new(0.0)},
				{Name: metricEncapsulation, Delta: new(0.0)},
			},
			want: result.VerdictPass,
		},
		{
			name: "metric absent on either side (nil delta) contributes no verdict flip",
			metrics: []result.MetricResult{
				{Name: metricCycle, Delta: nil},
				{Name: metricEncapsulation, Delta: nil},
			},
			want: result.VerdictPass,
		},
		{
			name: "gate off skips count breach",
			metrics: []result.MetricResult{
				{Name: metricCycle, Delta: new(3.0), Direction: result.DirectionHigherIsWorse},
			},
			gates: map[string]view.MetricConfig{
				metricCycle: {Gate: string(config.GateOff)},
			},
			want: result.VerdictPass,
		},
		{
			name: "gate off skips ratio breach",
			metrics: []result.MetricResult{
				{Name: metricEncapsulation, Delta: new(-0.5), Direction: result.DirectionHigherIsBetter},
			},
			gates: map[string]view.MetricConfig{
				metricEncapsulation: {Gate: string(config.GateOff)},
			},
			want: result.VerdictPass,
		},
		{
			name: "gate warn caps count breach at warn",
			metrics: []result.MetricResult{
				{Name: metricCycle, Delta: new(1.0), Direction: result.DirectionHigherIsWorse},
			},
			gates: map[string]view.MetricConfig{
				metricCycle: {Gate: string(config.GateWarn)},
			},
			want: result.VerdictWarn,
		},
		{
			name: "gate warn caps ratio breach at warn",
			metrics: []result.MetricResult{
				{Name: metricEncapsulation, Delta: new(-0.1), Direction: result.DirectionHigherIsBetter},
			},
			gates: map[string]view.MetricConfig{
				metricEncapsulation: {Gate: string(config.GateWarn)},
			},
			want: result.VerdictWarn,
		},
		{
			name: "gate fail on count breach fails",
			metrics: []result.MetricResult{
				{Name: metricCycle, Delta: new(1.0), Direction: result.DirectionHigherIsWorse},
			},
			gates: map[string]view.MetricConfig{
				metricCycle: {Gate: string(config.GateFail)},
			},
			want: result.VerdictFail,
		},
		{
			name: "max_new tolerates increase up to threshold",
			metrics: []result.MetricResult{
				{Name: metricCycle, Delta: new(2.0), Direction: result.DirectionHigherIsWorse},
			},
			gates: map[string]view.MetricConfig{
				metricCycle: {Gate: string(config.GateFail), MaxNew: new(2)},
			},
			want: result.VerdictPass,
		},
		{
			name: "max_new trips above threshold",
			metrics: []result.MetricResult{
				{Name: metricCycle, Delta: new(3.0), Direction: result.DirectionHigherIsWorse},
			},
			gates: map[string]view.MetricConfig{
				metricCycle: {Gate: string(config.GateWarn), MaxNew: new(2)},
			},
			want: result.VerdictWarn,
		},
		{
			name: "min_delta tolerates drop up to threshold",
			metrics: []result.MetricResult{
				{Name: metricEncapsulation, Delta: new(-0.05), Direction: result.DirectionHigherIsBetter},
			},
			gates: map[string]view.MetricConfig{
				metricEncapsulation: {Gate: string(config.GateFail), MinDelta: new(0.05)},
			},
			want: result.VerdictPass,
		},
		{
			name: "min_delta trips below threshold",
			metrics: []result.MetricResult{
				{Name: metricEncapsulation, Delta: new(-0.06), Direction: result.DirectionHigherIsBetter},
			},
			gates: map[string]view.MetricConfig{
				metricEncapsulation: {MinDelta: new(0.05)},
			},
			want: result.VerdictFail,
		},
		{
			// A warn-gated breach must not mask a later fail-gated breach.
			name: "fail-gated breach outranks earlier warn-gated breach",
			metrics: []result.MetricResult{
				{Name: metricEncapsulation, Delta: new(-0.1), Direction: result.DirectionHigherIsBetter},
				{Name: metricCycle, Delta: new(1.0), Direction: result.DirectionHigherIsWorse},
			},
			gates: map[string]view.MetricConfig{
				metricEncapsulation: {Gate: string(config.GateWarn)},
				metricCycle:         {Gate: string(config.GateFail)},
			},
			want: result.VerdictFail,
		},
		{
			name:             "active rule advisory warns",
			activeAdvisories: 1,
			want:             result.VerdictWarn,
		},
		{
			name: "warn-gated breach and advisories stay warn",
			metrics: []result.MetricResult{
				{Name: metricEncapsulation, Delta: new(-0.1), Direction: result.DirectionHigherIsBetter},
			},
			gates: map[string]view.MetricConfig{
				metricEncapsulation: {Gate: string(config.GateWarn)},
			},
			activeAdvisories: 1,
			want:             result.VerdictWarn,
		},
		{
			name:         "gate fail outranks metric warn and advisories",
			gateFindings: []finding.Finding{newTestFinding(finding.StatusNew)},
			metrics: []result.MetricResult{
				{Name: metricEncapsulation, Delta: new(-0.1)},
			},
			gates: map[string]view.MetricConfig{
				metricEncapsulation: {Gate: string(config.GateWarn)},
			},
			activeAdvisories: 1,
			want:             result.VerdictFail,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeVerdict(tt.gateFindings, tt.metrics, tt.gates, tt.activeAdvisories)
			if got != tt.want {
				t.Errorf("computeVerdict() = %v, want %v", got, tt.want)
			}
		})
	}
}
