package evaluation_test

import (
	"maps"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/state"
	modevidence "github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/policy"
)

const (
	selShipping  = "internal/shipping/**"
	selBilling   = "internal/billing/**"
	selPyCore    = "app.core"
	selJava      = "src/**/*.java"
	selMissingGo = "missing/**/*.go"
	pyCoreFile   = "src/app/core.py"
)

// vacuityFixture is a Go repo with a Python package and a Rust crate whose
// crate identity the producer withheld (no cargo metadata). Every primary
// producer completed, so only rule scope can leave a rule unevaluated.
func vacuityFixture() (*result.Result, evaluation.StateInput) {
	diag, in := dimensionsFixture()
	diag.PrimaryExtractorTools = []string{diag.PrimaryExtractorTools[0], toolDepCruiser, assessGrimp, toolCargo}
	for _, tool := range diag.PrimaryExtractorTools[1:] {
		diag.ToolCoverage = append(diag.ToolCoverage, modevidence.Coverage{Tool: tool, Status: modevidence.StatusOK})
	}
	for file, loc := range map[string]int{
		"internal/billing/domain/order.go": 10, "internal/billing/api/api.go": 10,
		"internal/shipping/ship.go": 10, pyCoreFile: 10, "crates/core/src/lib.rs": 10,
	} {
		in.Facts.FileLOC[file] = loc
	}
	in.Facts.SourceSelectors = map[string]string{pyCoreFile: selPyCore, "crates/core/src/lib.rs": ""}
	in.Facts.GoModulePaths = []string{"example.com/shop"}
	return diag, in
}

// TestVacuousSelectorsNeverCountAsConformance is the vacuity table: a gated
// rule whose selector matches nothing the rule can see is listed as not
// evaluated with the selector named, while external targets, guards, and
// selectors the inventory cannot judge keep their existing outcome.
func TestVacuousSelectorsNeverCountAsConformance(t *testing.T) {
	const genericReason = "rule scope cannot be established from the supported source inventory"
	for _, tc := range []struct {
		name       string
		rule       policy.RuleDef
		wantReason string // "" means the rule is evaluated
	}{
		{name: "live selectors", rule: policy.RuleDef{From: selShipping, To: "internal/billing/domain"}},
		{name: "typo in target", rule: policy.RuleDef{From: selShipping, To: "internal/biling/domain"},
			wantReason: "selector matches nothing: to internal/biling/domain"},
		{name: "renamed source directory", rule: policy.RuleDef{From: selCatalog, To: selBilling},
			wantReason: "selector matches nothing: from internal/catalog/**"},
		{name: "source with explicit extension", rule: policy.RuleDef{From: "internal/catalog/*.go", To: selBilling},
			wantReason: "selector matches nothing: from internal/catalog/*.go"},
		{name: "go.mod-prefixed target", rule: policy.RuleDef{From: selShipping, To: "example.com/shop/internal/billing/domain"},
			wantReason: "selector matches nothing: to example.com/shop/internal/billing/domain"},
		{name: "extglob negation", rule: policy.RuleDef{From: selShipping, To: "!(internal/billing/api)"},
			wantReason: "selector matches nothing: to !(internal/billing/api)"},
		{name: "dot-relative source", rule: policy.RuleDef{From: "./internal/shipping/**", To: selBilling},
			wantReason: "selector matches nothing: from ./internal/shipping/**"},
		{name: "python dotted typo", rule: policy.RuleDef{From: selPyCore, To: "app.adaptr"},
			wantReason: "selector matches nothing: to app.adaptr"},
		{name: "stdlib target", rule: policy.RuleDef{From: selShipping, To: "net/http"}},
		{name: "third-party target", rule: policy.RuleDef{From: selShipping, To: "github.com/sirupsen/logrus"}},
		{name: "module path prefix shorter than the module", rule: policy.RuleDef{From: selShipping, To: "example.com/**"}},
		{name: "unsupported source file type", rule: policy.RuleDef{From: selJava, To: selBilling},
			wantReason: genericReason},
		{name: "unresolved crate identity", rule: policy.RuleDef{From: assessCore, To: selBilling},
			wantReason: genericReason},
		{name: "guard with a vacuous target", rule: policy.RuleDef{From: selShipping, To: "internal/view/**", Guard: true}},
		{name: "guard with a vacuous source", rule: policy.RuleDef{From: "internal/view/**", To: selBilling, Guard: true}},
		{name: "rule type that ignores selectors", rule: policy.RuleDef{Type: metricCycle, From: "internal/nowhere/**"}},
		{name: "forbidden pattern over a renamed directory", rule: policy.RuleDef{Type: ruleTypePattern, From: selCatalog},
			wantReason: "selector matches nothing: from internal/catalog/**"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diag, in := vacuityFixture()
			rule := tc.rule
			rule.ID = "rule"
			rule.Gate = string(policy.GateFail)
			if rule.Type == "" {
				rule.Type = ruleForbidden
			}
			in.Policy.Gates.Rules.Rules = []policy.RuleDef{rule}

			got := map[string]string{}
			for _, unevaluated := range evaluation.BuildState(diag, in).Decision.UnevaluatedRequiredRules {
				got[unevaluated.RuleID] = unevaluated.Reason
			}
			want := map[string]string{}
			if tc.wantReason != "" {
				want["rule"] = tc.wantReason
			}
			if !maps.Equal(got, want) {
				t.Fatalf("unevaluated_required_rules = %v, want %v", got, want)
			}
		})
	}
}

// TestVacuousWarnRuleIsNotConformance pins the advisory half: a warn-gated
// rule with a vacuous selector is not a required rule, so it never reaches the
// hard-gate decision, but intent still refuses to count it as evaluated.
func TestVacuousWarnRuleIsNotConformance(t *testing.T) {
	diag, in := dimensionsFixture()
	in.Policy.Gates.Rules.Rules = []policy.RuleDef{{
		ID: "warn_typo", Type: ruleForbidden, Gate: gateWarnPosture, From: assessPathsB, To: "a/missing/**",
	}}
	st := evaluation.BuildState(diag, in)
	if st.Decision.HardGates != state.HardGatePass || len(st.Decision.UnevaluatedRequiredRules) != 0 {
		t.Fatalf("decision = %+v, want a passing hard gate", st.Decision)
	}
	if st.Dimensions.Intent.Status != state.Partial || st.Dimensions.Intent.Coverage.Observed != 0 {
		t.Fatalf("intent = %+v, want the vacuous warn rule unevaluated", st.Dimensions.Intent)
	}
	if diag.ToolCoverage[0].Status != modevidence.StatusOK {
		t.Fatal("fixture producer must be complete, so only vacuity can leave the rule unevaluated")
	}
}
