package application

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/assessment/state"
	"github.com/alexei-led/archfit/v3/internal/model/report"
)

func TestProjectArchitectureStatePreservesRequiredRuleEvidence(t *testing.T) {
	diagnostic := result.New()
	diagnostic.State = state.New()
	diagnostic.State.Decision.HardGates = state.HardGateUnmeasured
	diagnostic.State.Decision.UnevaluatedRequiredRules = []state.UnevaluatedRule{{RuleID: "no_cycle", Reason: "dependency producer failed"}}
	projected := projectArchitectureState(diagnostic, report.Document{})
	if projected.Decision.HardGates != report.HardGateUnmeasured || len(projected.Decision.UnevaluatedRequiredRules) != 1 {
		t.Fatalf("decision = %+v", projected.Decision)
	}
	encoded, err := json.Marshal(projected.Decision)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"unevaluated_required_rules":[{"rule_id":"no_cycle","reason":"dependency producer failed"}]`) {
		t.Fatalf("required rule evidence missing from JSON: %s", encoded)
	}
	diagnostic.State.Decision.UnevaluatedRequiredRules[0].Reason = "mutated"
	if projected.Decision.UnevaluatedRequiredRules[0].Reason != "dependency producer failed" {
		t.Fatal("projection shares the mutable assessment list")
	}
}
