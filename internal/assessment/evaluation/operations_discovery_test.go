package evaluation_test

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/v3/internal/assessment/state"
	modevidence "github.com/alexei-led/archfit/v3/internal/model/evidence"
)

func TestOperationsRequiresCompletedDeployDiscovery(t *testing.T) {
	for _, producerStatus := range []string{"", modevidence.StatusOK, modevidence.StatusPartial, "timeout", modevidence.StatusAbsent} {
		t.Run(producerStatus, func(t *testing.T) {
			diag, in := dimensionsFixture()
			diag.ToolCoverage = nil
			if producerStatus != "" {
				diag.ToolCoverage = append(diag.ToolCoverage, modevidence.Coverage{Tool: toolDeployDiscovery, Status: producerStatus, Reason: "Go main discovery incomplete"})
			}
			got := evaluation.BuildDimensions(diag, in, nil).Operations
			want := state.Partial
			if producerStatus == modevidence.StatusOK {
				want = state.Measured
			}
			if got.Status != want {
				t.Fatalf("operations = %+v, want %s", got, want)
			}
			if producerStatus != modevidence.StatusOK {
				found := false
				for _, unknown := range got.Unknown {
					if unknown.Fact == state.FactTopologyReconciliation {
						found = true
					}
				}
				if !found {
					t.Fatalf("missing discovery gap: %+v", got.Unknown)
				}
			}
		})
	}
}
