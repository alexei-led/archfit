package engine

import (
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
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
		metrics          []diagnostic.MetricResult
		gates            map[string]config.MetricConfig
		activeAdvisories int
		want             diagnostic.Verdict
	}{
		{
			name:         "new gate finding fails",
			gateFindings: []finding.Finding{newTestFinding(finding.StatusNew)},
			want:         diagnostic.VerdictFail,
		},
		{
			name:         "expired waiver fails",
			gateFindings: []finding.Finding{newTestFinding(finding.StatusExpiredWaiver)},
			want:         diagnostic.VerdictFail,
		},
		{
			name:         "fixed gate finding alone passes",
			gateFindings: []finding.Finding{newTestFinding(finding.StatusFixed)},
			want:         diagnostic.VerdictPass,
		},
		{
			// Waived findings reach computeVerdict unfiltered in production
			// (status.Assign stamps them, it does not drop them) — the gate
			// loop must not treat a waived finding as a breach.
			name:         "waived gate finding alone passes",
			gateFindings: []finding.Finding{newTestFinding(finding.StatusWaived)},
			want:         diagnostic.VerdictPass,
		},
		{
			// Baseline findings likewise reach computeVerdict unfiltered — an
			// accepted pre-existing finding must not flip the verdict.
			name:         "baseline gate finding alone passes",
			gateFindings: []finding.Finding{newTestFinding(finding.StatusBaseline)},
			want:         diagnostic.VerdictPass,
		},
		{
			// A new cycle is a count-metric regression (more cycles is worse)
			// and must block — this was the V1 bug: computeVerdict only
			// checked Delta < 0, so a positive count delta was silently
			// ignored. With no metrics.cycle config the gate is unset, and
			// unset = blocking (rule-gate convention).
			name: "new cycle (count metric, Delta=+1, unset gate) fails",
			metrics: []diagnostic.MetricResult{
				{Name: metricCycle, Delta: new(1.0), Direction: diagnostic.DirectionHigherIsWorse},
			},
			want: diagnostic.VerdictFail,
		},
		{
			// Fixing a cycle (fewer cycles, Delta=-1) is an improvement and
			// must PASS — this was the V1 bug: computeVerdict flagged any
			// negative delta as a regression regardless of metric direction,
			// producing a false WARN.
			name: "fixed cycle (count metric, Delta=-1) passes",
			metrics: []diagnostic.MetricResult{
				{Name: metricCycle, Delta: new(-1.0), Direction: diagnostic.DirectionHigherIsWorse},
			},
			want: diagnostic.VerdictPass,
		},
		{
			name: "encapsulation drop (ratio metric, Delta=-0.1, unset gate) fails",
			metrics: []diagnostic.MetricResult{
				{Name: metricEncapsulation, Delta: new(-0.1), Direction: diagnostic.DirectionHigherIsBetter},
			},
			want: diagnostic.VerdictFail,
		},
		{
			name: "encapsulation rise (ratio metric, Delta=+0.1) passes",
			metrics: []diagnostic.MetricResult{
				{Name: metricEncapsulation, Delta: new(0.1), Direction: diagnostic.DirectionHigherIsBetter},
			},
			want: diagnostic.VerdictPass,
		},
		{
			// Direction unset (zero value) must still behave as ratio
			// semantics (breach on negative delta) — the safe default for
			// any MetricResult that predates or omits Direction.
			name: "unset Direction defaults to ratio semantics (Delta=-0.1) fails",
			metrics: []diagnostic.MetricResult{
				{Name: metricEncapsulation, Delta: new(-0.1)},
			},
			want: diagnostic.VerdictFail,
		},
		{
			name: "unchanged metrics pass",
			metrics: []diagnostic.MetricResult{
				{Name: metricCycle, Delta: new(0.0)},
				{Name: metricEncapsulation, Delta: new(0.0)},
			},
			want: diagnostic.VerdictPass,
		},
		{
			name: "metric absent on either side (nil delta) contributes no verdict flip",
			metrics: []diagnostic.MetricResult{
				{Name: metricCycle, Delta: nil},
				{Name: metricEncapsulation, Delta: nil},
			},
			want: diagnostic.VerdictPass,
		},
		{
			name: "gate off skips count breach",
			metrics: []diagnostic.MetricResult{
				{Name: metricCycle, Delta: new(3.0), Direction: diagnostic.DirectionHigherIsWorse},
			},
			gates: map[string]config.MetricConfig{
				metricCycle: {Gate: string(config.GateOff)},
			},
			want: diagnostic.VerdictPass,
		},
		{
			name: "gate off skips ratio breach",
			metrics: []diagnostic.MetricResult{
				{Name: metricEncapsulation, Delta: new(-0.5), Direction: diagnostic.DirectionHigherIsBetter},
			},
			gates: map[string]config.MetricConfig{
				metricEncapsulation: {Gate: string(config.GateOff)},
			},
			want: diagnostic.VerdictPass,
		},
		{
			name: "gate warn caps count breach at warn",
			metrics: []diagnostic.MetricResult{
				{Name: metricCycle, Delta: new(1.0), Direction: diagnostic.DirectionHigherIsWorse},
			},
			gates: map[string]config.MetricConfig{
				metricCycle: {Gate: string(config.GateWarn)},
			},
			want: diagnostic.VerdictWarn,
		},
		{
			name: "gate warn caps ratio breach at warn",
			metrics: []diagnostic.MetricResult{
				{Name: metricEncapsulation, Delta: new(-0.1), Direction: diagnostic.DirectionHigherIsBetter},
			},
			gates: map[string]config.MetricConfig{
				metricEncapsulation: {Gate: string(config.GateWarn)},
			},
			want: diagnostic.VerdictWarn,
		},
		{
			name: "gate fail on count breach fails",
			metrics: []diagnostic.MetricResult{
				{Name: metricCycle, Delta: new(1.0), Direction: diagnostic.DirectionHigherIsWorse},
			},
			gates: map[string]config.MetricConfig{
				metricCycle: {Gate: string(config.GateFail)},
			},
			want: diagnostic.VerdictFail,
		},
		{
			name: "max_new tolerates increase up to threshold",
			metrics: []diagnostic.MetricResult{
				{Name: metricCycle, Delta: new(2.0), Direction: diagnostic.DirectionHigherIsWorse},
			},
			gates: map[string]config.MetricConfig{
				metricCycle: {Gate: string(config.GateFail), MaxNew: new(2)},
			},
			want: diagnostic.VerdictPass,
		},
		{
			name: "max_new trips above threshold",
			metrics: []diagnostic.MetricResult{
				{Name: metricCycle, Delta: new(3.0), Direction: diagnostic.DirectionHigherIsWorse},
			},
			gates: map[string]config.MetricConfig{
				metricCycle: {Gate: string(config.GateWarn), MaxNew: new(2)},
			},
			want: diagnostic.VerdictWarn,
		},
		{
			name: "min_delta tolerates drop up to threshold",
			metrics: []diagnostic.MetricResult{
				{Name: metricEncapsulation, Delta: new(-0.05), Direction: diagnostic.DirectionHigherIsBetter},
			},
			gates: map[string]config.MetricConfig{
				metricEncapsulation: {Gate: string(config.GateFail), MinDelta: new(0.05)},
			},
			want: diagnostic.VerdictPass,
		},
		{
			name: "min_delta trips below threshold",
			metrics: []diagnostic.MetricResult{
				{Name: metricEncapsulation, Delta: new(-0.06), Direction: diagnostic.DirectionHigherIsBetter},
			},
			gates: map[string]config.MetricConfig{
				metricEncapsulation: {MinDelta: new(0.05)},
			},
			want: diagnostic.VerdictFail,
		},
		{
			// A warn-gated breach must not mask a later fail-gated breach.
			name: "fail-gated breach outranks earlier warn-gated breach",
			metrics: []diagnostic.MetricResult{
				{Name: metricEncapsulation, Delta: new(-0.1), Direction: diagnostic.DirectionHigherIsBetter},
				{Name: metricCycle, Delta: new(1.0), Direction: diagnostic.DirectionHigherIsWorse},
			},
			gates: map[string]config.MetricConfig{
				metricEncapsulation: {Gate: string(config.GateWarn)},
				metricCycle:         {Gate: string(config.GateFail)},
			},
			want: diagnostic.VerdictFail,
		},
		{
			name:             "active rule advisory warns",
			activeAdvisories: 1,
			want:             diagnostic.VerdictWarn,
		},
		{
			name: "warn-gated breach and advisories stay warn",
			metrics: []diagnostic.MetricResult{
				{Name: metricEncapsulation, Delta: new(-0.1), Direction: diagnostic.DirectionHigherIsBetter},
			},
			gates: map[string]config.MetricConfig{
				metricEncapsulation: {Gate: string(config.GateWarn)},
			},
			activeAdvisories: 1,
			want:             diagnostic.VerdictWarn,
		},
		{
			name:         "gate fail outranks metric warn and advisories",
			gateFindings: []finding.Finding{newTestFinding(finding.StatusNew)},
			metrics: []diagnostic.MetricResult{
				{Name: metricEncapsulation, Delta: new(-0.1)},
			},
			gates: map[string]config.MetricConfig{
				metricEncapsulation: {Gate: string(config.GateWarn)},
			},
			activeAdvisories: 1,
			want:             diagnostic.VerdictFail,
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
