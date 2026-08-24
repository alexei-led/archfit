package metrics_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/metrics"
	"github.com/alexei-led/archfit/internal/view"
)

// ---------------------------------------------------------------------------
// Metric interface and New()
// ---------------------------------------------------------------------------

const (
	metricBlastRadius = "blast_radius"
	metricCoverage    = "coverage"
)

func TestNew_ReturnsAllMetrics(t *testing.T) {
	ms := metrics.New(nil)
	if len(ms) != 5 {
		t.Errorf("expected 5 metrics got %d", len(ms))
	}
	names := make(map[string]bool)
	for _, m := range ms {
		names[m.Name()] = true
	}
	for _, want := range []string{"encapsulation", "unbalanced_edge", "cycle", metricCoverage, metricBlastRadius} {
		if !names[want] {
			t.Errorf("missing metric %q", want)
		}
	}
}

// TestNew_ExplicitDisableHonored verifies that metrics.<name>.enabled: false
// removes the metric while unconfigured metrics stay enabled — and that a
// knob-only entry (gate set, enabled absent) does NOT disable the metric.
func TestNew_ExplicitDisableHonored(t *testing.T) {
	cfg := map[string]view.MetricEntry{
		"blast_radius":    {Enabled: new(false)},
		"coverage":        {Enabled: new(false)},
		"cycle":           {Enabled: new(true)},
		"unbalanced_edge": {Gate: "warn"}, // knob-only: stays enabled
	}
	ms := metrics.New(cfg)
	if len(ms) != 3 {
		t.Fatalf("expected 3 metrics (5 - 2 disabled), got %d", len(ms))
	}
	names := make(map[string]bool, len(ms))
	for _, m := range ms {
		names[m.Name()] = true
	}
	if names["blast_radius"] || names["coverage"] {
		t.Errorf("explicitly disabled metric still registered: %v", names)
	}
	if !names["unbalanced_edge"] {
		t.Error("knob-only entry ({gate: warn}) disabled the metric — enabled must default to true")
	}
}
