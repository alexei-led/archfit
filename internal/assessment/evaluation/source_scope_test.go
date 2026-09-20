package evaluation_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/assessment/state"
	modevidence "github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/policy"
)

func TestRequiredPythonDottedRuleScope(t *testing.T) {
	primaryTools := []string{"", toolDepCruiser, assessGrimp, toolCargo}
	for _, producerStatus := range []string{modevidence.StatusOK, modevidence.StatusAbsent, modevidence.StatusPartial} {
		for _, target := range []string{"app.adapter", "app.adapter.**", "app.adapter**"} {
			t.Run(producerStatus+"/"+target, func(t *testing.T) {
				diag, in := dimensionsFixture()
				diag.ToolCoverage = []modevidence.Coverage{{Tool: primaryTools[2], Status: producerStatus}}
				diag.PrimaryExtractorTools = primaryTools
				in.Facts = evaluation.Observations{FileLOC: map[string]int{"src/app/core.py": 10, "src/app/adapter/__init__.py": 10}}
				in.Policy.Gates.Rules.Rules = []policy.RuleDef{{ID: "python", Type: ruleForbidden, Gate: string(policy.GateFail), From: "app.core**", To: target}}
				got := evaluation.BuildState(diag, in)
				want := state.HardGateUnmeasured
				if producerStatus == modevidence.StatusOK {
					want = state.HardGatePass
				}
				if got.Decision.HardGates != want {
					t.Fatalf("decision = %+v, want %s", got.Decision, want)
				}
			})
		}
	}
}

func TestRequiredRustRuleScopeUsesProducerIdentities(t *testing.T) {
	for _, producerStatus := range []string{modevidence.StatusOK, modevidence.StatusAbsent, modevidence.StatusPartial} {
		t.Run(producerStatus, func(t *testing.T) {
			diag, in := dimensionsFixture()
			diag.PrimaryExtractorTools = []string{"", toolDepCruiser, assessGrimp, toolCargo}
			diag.ToolCoverage = []modevidence.Coverage{{Tool: toolCargo, Status: producerStatus}}
			in.Facts = evaluation.Observations{
				FileLOC:         map[string]int{"core/src/lib.rs": 10, "adapter/src/lib.rs": 10},
				SourceSelectors: map[string]string{"core/src/lib.rs": "smoke-core", "adapter/src/lib.rs": "smoke-adapter"},
			}
			in.Policy.Gates.Rules.Rules = []policy.RuleDef{{ID: "rust", Type: ruleForbidden, Gate: string(policy.GateFail), From: "smoke-core", To: "smoke-adapter"}}
			got := evaluation.BuildState(diag, in)
			want := state.HardGateUnmeasured
			if producerStatus == modevidence.StatusOK {
				want = state.HardGatePass
			}
			if got.Decision.HardGates != want {
				t.Fatalf("decision = %+v, want %s", got.Decision, want)
			}
		})
	}
}
