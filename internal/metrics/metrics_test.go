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
	metricChangeLocality = "change_locality"
	metricRiskHub        = "risk_hub"
)

func TestNew_ReturnsAllMetrics(t *testing.T) {
	ms := metrics.New(config.Config{})
	if len(ms) != 13 {
		t.Errorf("expected 13 metrics got %d", len(ms))
	}
	names := make(map[string]bool)
	for _, m := range ms {
		names[m.Name()] = true
	}
	for _, want := range []string{"encapsulation", "unbalanced_edge", "cycle", "coverage", "blast_radius", "change_amplification", "hidden_coupling", "structural_weight", "complexity", metricRiskHub, "architecture_fitness", "functional_candidates", metricChangeLocality} {
		if !names[want] {
			t.Errorf("missing metric %q", want)
		}
	}
}

// TestNew_ExplicitDisableHonored verifies that metrics.<name>.enabled: false
// removes the metric while unconfigured metrics stay enabled.
func TestNew_ExplicitDisableHonored(t *testing.T) {
	cfg := config.Config{Metrics: map[string]config.MetricEntry{
		"risk_hub":           {Enabled: false},
		metricChangeLocality: {Enabled: false},
		"cycle":              {Enabled: true},
	}}
	ms := metrics.New(cfg)
	if len(ms) != 11 {
		t.Fatalf("expected 11 metrics (13 - 2 disabled), got %d", len(ms))
	}
	for _, m := range ms {
		if m.Name() == metricRiskHub || m.Name() == metricChangeLocality {
			t.Errorf("disabled metric %q still registered", m.Name())
		}
	}
}
