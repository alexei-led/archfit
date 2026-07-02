package engine

import (
	"testing"

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
// needed. See V1 in reports/eval-2026-07-02-v1.1.2/00-FINDINGS.md: today
// computeVerdict treats any Delta < 0 as a regression regardless of whether
// the metric is a ratio (higher is better) or a count (higher is worse). The
// cycle-metric cases below assert TODAY's inverted behavior (marked
// TODO(wave1)); Task 2 flips them to the book-correct expectation once
// direction-aware deltas land.
func TestComputeVerdict(t *testing.T) {
	tests := []struct {
		name             string
		gateFindings     []finding.Finding
		metrics          []diagnostic.MetricResult
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
			// TODO(wave1): a new cycle is a count-metric regression (more
			// cycles is worse) and must WARN/FAIL. computeVerdict only checks
			// Delta < 0, so a positive count delta is silently ignored today.
			// Flip `want` to VerdictWarn once Task 2's direction fix lands.
			name: "new cycle (count metric, Delta=+1) currently silently PASSes (V1 bug)",
			metrics: []diagnostic.MetricResult{
				{Name: metricCycle, Delta: new(1.0)},
			},
			want: diagnostic.VerdictPass,
		},
		{
			// TODO(wave1): fixing a cycle (fewer cycles, Delta=-1) is an
			// improvement and must PASS. computeVerdict flags any negative
			// delta as a regression regardless of metric direction, producing
			// a false WARN today. Flip `want` to VerdictPass once Task 2's
			// direction fix lands.
			name: "fixed cycle (count metric, Delta=-1) currently false-WARNs (V1 bug)",
			metrics: []diagnostic.MetricResult{
				{Name: metricCycle, Delta: new(-1.0)},
			},
			want: diagnostic.VerdictWarn,
		},
		{
			name: "encapsulation drop (ratio metric, Delta=-0.1) warns",
			metrics: []diagnostic.MetricResult{
				{Name: metricEncapsulation, Delta: new(-0.1)},
			},
			want: diagnostic.VerdictWarn,
		},
		{
			name: "encapsulation rise (ratio metric, Delta=+0.1) passes",
			metrics: []diagnostic.MetricResult{
				{Name: metricEncapsulation, Delta: new(0.1)},
			},
			want: diagnostic.VerdictPass,
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
			name: "metric absent on either side (nil delta) contributes no warn",
			metrics: []diagnostic.MetricResult{
				{Name: metricCycle, Delta: nil},
				{Name: metricEncapsulation, Delta: nil},
			},
			want: diagnostic.VerdictPass,
		},
		{
			name:             "active rule advisory warns",
			activeAdvisories: 1,
			want:             diagnostic.VerdictWarn,
		},
		{
			name:         "gate fail outranks metric warn and advisories",
			gateFindings: []finding.Finding{newTestFinding(finding.StatusNew)},
			metrics: []diagnostic.MetricResult{
				{Name: metricEncapsulation, Delta: new(-0.1)},
			},
			activeAdvisories: 1,
			want:             diagnostic.VerdictFail,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeVerdict(tt.gateFindings, tt.metrics, tt.activeAdvisories)
			if got != tt.want {
				t.Errorf("computeVerdict() = %v, want %v", got, tt.want)
			}
		})
	}
}
