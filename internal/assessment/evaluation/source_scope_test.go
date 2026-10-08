package evaluation_test

import (
	"slices"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/v3/internal/assessment/state"
	modevidence "github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

func TestRequiredPythonDottedRuleScope(t *testing.T) {
	primaryTools := []string{"", toolDepCruiser, assessGrimp, toolCargo}
	for _, producerStatus := range []string{modevidence.StatusOK, modevidence.StatusAbsent, modevidence.StatusPartial} {
		for _, target := range []string{"app.adapter", "app.adapter.**", "app.adapter**"} {
			t.Run(producerStatus+"/"+target, func(t *testing.T) {
				diag, in := dimensionsFixture()
				diag.ToolCoverage = []modevidence.Coverage{{Tool: primaryTools[2], Status: producerStatus}}
				diag.PrimaryExtractorTools = primaryTools
				in.Facts = evaluation.Observations{FileLOC: map[string]int{
					pyCoreFile: 10, "src/app/adapter/__init__.py": 10, "src/app/adapter/http.py": 10,
				}}
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

// TestRuleScopeSkipsDeclaredOutOfScopeSources pins that a source file the
// configuration declared outside the analysis scope (an exclude: glob or a
// switched-off language) cannot hold a rule unevaluated for want of the
// producer of its language: a stray tooling script must not keep a Go-only
// rule waiting for dependency-cruiser.
func TestRuleScopeSkipsDeclaredOutOfScopeSources(t *testing.T) {
	const helper = "a/tool/helper.cjs"
	rules := []policy.RuleDef{
		{ID: "module_wide", Type: "forbidden_layer_direction", Gate: string(policy.GateFail)},
		{ID: "go_only", Type: ruleForbidden, Gate: string(policy.GateFail), From: assessPathsA, To: assessPathsB},
	}
	for _, tc := range []struct {
		name        string
		outOfScope  map[string]struct{}
		want        state.HardGateState
		unevaluated []string
	}{
		{name: "stray helper in scope", want: state.HardGateUnmeasured, unevaluated: []string{"go_only", "module_wide"}},
		{name: "helper declared out of scope", outOfScope: map[string]struct{}{helper: {}}, want: state.HardGatePass},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diag, in := dimensionsFixture()
			diag.PrimaryExtractorTools = []string{diag.PrimaryExtractorTools[0], toolDepCruiser, assessGrimp, toolCargo}
			diag.ToolCoverage = append(diag.ToolCoverage, modevidence.Coverage{Tool: toolDepCruiser, Status: modevidence.StatusAbsent})
			in.Facts.FileLOC[helper] = 3
			in.Facts.OutOfScopeFiles = tc.outOfScope
			in.Policy.Gates.Rules.Rules = rules
			got := evaluation.BuildState(diag, in)
			if got.Decision.HardGates != tc.want {
				t.Fatalf("hard gates = %s, want %s: %+v", got.Decision.HardGates, tc.want, got.Decision)
			}
			ids := make([]string, 0, len(got.Decision.UnevaluatedRequiredRules))
			for _, rule := range got.Decision.UnevaluatedRequiredRules {
				ids = append(ids, rule.RuleID)
			}
			if !slices.Equal(ids, tc.unevaluated) {
				t.Fatalf("unevaluated rules = %v, want %v", ids, tc.unevaluated)
			}
		})
	}
}
