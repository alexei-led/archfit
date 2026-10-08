package evaluation_test

import (
	"maps"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/assessment/state"
	modevidence "github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

const (
	selShipping  = "internal/shipping/**"
	selBilling   = "internal/billing/**"
	selPyCore    = "app.core"
	selJava      = "src/**/*.java"
	selMissingGo = "missing/**/*.go"
	pyCoreFile   = "src/app/core.py"
	tsAppFile    = "web/src/app.ts"
	rustLibFile  = "crates/core/src/lib.rs"
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
	diag.ToolCoverage = append(diag.ToolCoverage, modevidence.Coverage{Tool: toolAstGrep, Status: modevidence.StatusOK})
	for file, loc := range map[string]int{
		"internal/billing/domain/order.go": 10, "internal/billing/api/api.go": 10,
		"internal/shipping/ship.go": 10, pyCoreFile: 10, rustLibFile: 10,
		tsAppFile: 10, "go/cmd/server/main.go": 10,
	} {
		in.Facts.FileLOC[file] = loc
	}
	in.Facts.SourceSelectors = map[string]string{pyCoreFile: selPyCore, rustLibFile: ""}
	in.Facts.GoModulePaths = []string{"example.com/shop"}
	return diag, in
}

// TestVacuousSelectorsNeverCountAsConformance is the vacuity table: a gated
// rule whose selector matches nothing the rule can see is listed as not
// evaluated with the selector named, while external targets, guards, and
// selectors the inventory cannot judge keep their existing outcome. A
// dependency rule sees graph edge endpoints, spelled per language: a Go edge
// starts at a file and ends at a package directory, a TypeScript edge joins
// files, and Python and Rust edges join dotted modules and crates.
func TestVacuousSelectorsNeverCountAsConformance(t *testing.T) {
	const genericReason = "rule scope cannot be established from the supported source inventory"
	for _, tc := range []struct {
		name       string
		rule       policy.RuleDef
		crate      string // the Rust crate name cargo metadata resolved; "" leaves it unknown
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
		{name: "domain-prefixed target beside a go/ directory", rule: policy.RuleDef{From: selShipping, To: "go.uber.org/**"}},
		{name: "typo under the go/ directory", rule: policy.RuleDef{From: selShipping, To: "go/cmd/sevrer"},
			wantReason: "selector matches nothing: to go/cmd/sevrer"},
		{name: "go file-path target", rule: policy.RuleDef{From: selShipping, To: "internal/billing/domain/*.go"},
			wantReason: "selector matches nothing: to internal/billing/domain/*.go"},
		{name: "go package-directory source", rule: policy.RuleDef{From: "internal/shipping", To: selBilling},
			wantReason: "selector matches nothing: from internal/shipping"},
		{name: "go file-path source", rule: policy.RuleDef{From: "internal/shipping/*.go", To: selBilling}},
		{name: "typescript file target", rule: policy.RuleDef{From: selShipping, To: "web/src/*.ts"}},
		{name: "python file-path target", rule: policy.RuleDef{From: selShipping, To: "src/app/*.py"},
			wantReason: "selector matches nothing: to src/app/*.py"},
		{name: "python file-path source", rule: policy.RuleDef{From: "src/app/*.py", To: selBilling},
			wantReason: "selector matches nothing: from src/app/*.py"},
		{name: "rust crate-directory source", rule: policy.RuleDef{From: "crates/core/**", To: selBilling}, crate: assessCore,
			wantReason: "selector matches nothing: from crates/core/**"},
		{name: "rust module target below a known crate", rule: policy.RuleDef{From: selShipping, To: "core::domain::**"},
			crate: assessCore},
		{name: "rust module source below an unknown crate", rule: policy.RuleDef{From: "coer::domain::**", To: selBilling},
			crate: assessCore, wantReason: "selector matches nothing: from coer::domain::**"},
		{name: "forbidden pattern over a go package directory", rule: policy.RuleDef{Type: ruleTypePattern, From: "internal/shipping"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diag, in := vacuityFixture()
			if tc.crate != "" {
				in.Facts.SourceSelectors[rustLibFile] = tc.crate
			}
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

// TestGuardWithVacuousTargetIgnoresProducerPartialness pins the guard
// semantics: while the guarded target matches no scanned source, no edge can
// reach it, so the guard holds whatever state the dependency producer is in
// (a partial run is the steady state on TypeScript). Once the guarded path
// exists again the guard is an ordinary rule and waits for complete evidence.
func TestGuardWithVacuousTargetIgnoresProducerPartialness(t *testing.T) {
	for _, tc := range []struct {
		name       string
		to         string
		wantListed bool
	}{
		{name: "guarded target absent", to: "internal/legacy/**"},
		{name: "guarded target exists again", to: selBilling, wantListed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diag, in := vacuityFixture()
			diag.ToolCoverage[0].Status = modevidence.StatusPartial
			in.Policy.Gates.Rules.Rules = []policy.RuleDef{{
				ID: "guard", Type: ruleForbidden, Gate: string(policy.GateFail), From: selShipping, To: tc.to, Guard: true,
			}}
			listed := len(evaluation.BuildState(diag, in).Decision.UnevaluatedRequiredRules) > 0
			if listed != tc.wantListed {
				t.Fatalf("guard listed in unevaluated_required_rules = %v, want %v", listed, tc.wantListed)
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
