package application

import (
	"encoding/json"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

func TestReportKeepsMetricsAndSeamsWithoutStoredReferences(t *testing.T) {
	const metricName, metricMode, primaryTool = "cycle", "count", "go/packages"
	diagnostic := result.New()
	diagnostic.Metrics = []result.MetricResult{{Name: metricName, Value: 2, Display: "2", Mode: metricMode, Confidence: "high"}}
	diagnostic.Seams = []result.Seam{{ID: "orders-payments", FromModule: "orders", ToModule: "payments", DistributedMonolith: true}}
	diagnostic.PrimaryExtractorTools = []string{primaryTool}
	diagnostic.ClassifiedEdges = &result.ClassifiedEdgeSummary{}
	scored := evaluation.Score(&diagnostic, evaluation.ScoreInput{
		Policy:         policy.PolicySnapshot{},
		MarkedCoverage: []evidence.Coverage{{Tool: primaryTool, Status: evidence.StatusOK, FilesApplicable: 1, FilesSeen: 1}},
	})
	document := projectReport(diagnostic, scored.Score)
	if len(document.Metrics) != 1 || document.Metrics[0].Value != 2 {
		t.Fatalf("metric measurement lost: %+v", document.Metrics)
	}
	if len(document.State.Seams) != 1 || document.State.Seams[0].ID != "orders-payments" || !document.State.Seams[0].DistributedMonolith {
		t.Fatalf("seam measurement lost: %+v", document.State.Seams)
	}
	found := false
	for _, metric := range document.State.Dimensions.Structure.Metrics {
		if metric.Name == metricName && metric.Value == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("state metric measurement lost: %+v", document.State.Dimensions.Structure.Metrics)
	}
	if document.State.Decision.HardGates != "pass" || len(document.Findings) != 0 {
		t.Fatalf("measurements became blockers: %+v", document.State.Decision)
	}
	raw, err := json.Marshal(document.State)
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"gate_reasons", "gate_reference", "comparison"} {
		if _, exists := state[key]; exists {
			t.Errorf("retired reference field %q remains", key)
		}
	}
	for name, value := range state["dimensions"].(map[string]any) {
		if _, exists := value.(map[string]any)["delta"]; exists {
			t.Errorf("%s retains a stored-reference delta", name)
		}
	}
}
