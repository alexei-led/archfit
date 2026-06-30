package metrics_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/metrics"
)

// ---------------------------------------------------------------------------
// Metric interface and New()
// ---------------------------------------------------------------------------

const (
	metricBlastRadius = "blast_radius"
	metricCoverage    = "coverage"
)

func TestNew_ReturnsAllMetrics(t *testing.T) {
	ms := metrics.New(config.Config{})
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
// removes the metric while unconfigured metrics stay enabled.
func TestNew_ExplicitDisableHonored(t *testing.T) {
	cfg := config.Config{Metrics: map[string]config.MetricEntry{
		"blast_radius": {Enabled: false},
		"coverage":     {Enabled: false},
		"cycle":        {Enabled: true},
	}}
	ms := metrics.New(cfg)
	if len(ms) != 3 {
		t.Fatalf("expected 3 metrics (5 - 2 disabled), got %d", len(ms))
	}
	for _, m := range ms {
		if m.Name() == "blast_radius" || m.Name() == "coverage" {
			t.Errorf("disabled metric %q still registered", m.Name())
		}
	}
}
