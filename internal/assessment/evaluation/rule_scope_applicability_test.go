package evaluation_test

import (
	"maps"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/assessment/result"
	modevidence "github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/policy"
)

const (
	ruleIDScoped = "rule"
	webUIFile    = "web/ui/src/app.ts"
	webUIGlob    = "web/ui/**"
	rustCrate    = "core"
	rustModPlace = "core::place"
	rustModTypes = "core::types"
	modBilling   = "billing"
	modWeb       = "web"
	typeModCycle = "module_cycle"
	typeLayerDir = "forbidden_layer_direction"
)

// unevaluatedReasons maps each listed required rule to its reason.
func unevaluatedReasons(diag *result.Result, in evaluation.StateInput) map[string]string {
	got := map[string]string{}
	for _, rule := range evaluation.BuildState(diag, in).Decision.UnevaluatedRequiredRules {
		got[rule.RuleID] = rule.Reason
	}
	return got
}

func withModules(in *evaluation.StateInput, modules map[string]policy.ModuleDef) {
	topology := policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)}
	in.Policy = policy.New(topology, policy.RelationshipPolicy{}, policy.AssessmentPolicy{}, in.Policy.Gates, nil, nil)
}

func withRule(in *evaluation.StateInput, rule policy.RuleDef) {
	rule.ID = ruleIDScoped
	if rule.Gate == "" {
		rule.Gate = string(policy.GateFail)
	}
	in.Policy.Gates.Rules.Rules = []policy.RuleDef{rule}
}

// typescriptAbsentFixture is a Go repo with TypeScript under web/ui and no
// root package.json: dependency-cruiser's own applicability probe finds no
// project, so its row is gapless absent and acquisition reports the files as
// unanalysed.
func typescriptAbsentFixture() (*result.Result, evaluation.StateInput) {
	diag, in := vacuityFixture()
	for i := range diag.ToolCoverage {
		if diag.ToolCoverage[i].Tool == toolDepCruiser {
			diag.ToolCoverage[i].Status = modevidence.StatusAbsent
		}
	}
	delete(in.Facts.FileLOC, tsAppFile)
	in.Facts.FileLOC[webUIFile] = 10
	in.Facts.UnanalysedFiles = map[string]struct{}{webUIFile: {}}
	return diag, in
}

// TestRuleScopeSkipsLanguagesTheExtractorSaysAreAbsent pins rule scope to the
// extractor's applicability: TypeScript the dependency-cruiser probe calls
// absent never holds a dependency or module rule unevaluated, while an absent
// producer for TypeScript that IS present still does, and forbidden_pattern,
// which reads the ast-grep pattern pass, keeps those files.
func TestRuleScopeSkipsLanguagesTheExtractorSaysAreAbsent(t *testing.T) {
	webModules := map[string]policy.ModuleDef{
		modBilling: {Paths: []string{selBilling}},
		modWeb:     {Paths: []string{"web/**"}},
	}
	tsOnlyModules := map[string]policy.ModuleDef{
		modBilling: {Paths: []string{selBilling}},
		"web_ui":   {Paths: []string{webUIGlob}},
	}
	const onlyUnanalysed = "selector matches only source no dependency producer analyses: from " + webUIGlob
	for _, tc := range []struct {
		name       string
		modules    map[string]policy.ModuleDef
		rule       policy.RuleDef
		present    bool // the TypeScript project is present, its producer absent
		wantReason string
	}{
		{name: "module_cycle over a module mixing go and absent typescript", modules: webModules,
			rule: policy.RuleDef{Type: typeModCycle}},
		{name: "module_cycle over a module owning only absent typescript", modules: tsOnlyModules,
			rule: policy.RuleDef{Type: typeModCycle}},
		{name: "forbidden_layer_direction over absent typescript", modules: tsOnlyModules,
			rule: policy.RuleDef{Type: typeLayerDir}},
		{name: "module_cycle with the typescript producer missing", modules: tsOnlyModules, present: true,
			rule: policy.RuleDef{Type: typeModCycle}, wantReason: "dependency-cruiser evidence is absent"},
		{name: "dependency rule aimed only at absent typescript",
			rule:       policy.RuleDef{Type: ruleForbidden, From: webUIGlob, To: selBilling},
			wantReason: onlyUnanalysed},
		{name: "guard aimed only at absent typescript",
			rule:       policy.RuleDef{Type: ruleForbidden, From: webUIGlob, To: selBilling, Guard: true},
			wantReason: onlyUnanalysed},
		{name: "forbidden_pattern keeps the files", rule: policy.RuleDef{Type: ruleTypePattern, From: webUIGlob}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diag, in := typescriptAbsentFixture()
			if tc.present {
				diag.CoverageGaps = []modevidence.CoverageGap{{Tool: toolDepCruiser}}
				in.Facts.UnanalysedFiles = nil
			}
			if tc.modules != nil {
				withModules(&in, tc.modules)
			}
			withRule(&in, tc.rule)
			want := map[string]string{}
			if tc.wantReason != "" {
				want[ruleIDScoped] = tc.wantReason
			}
			if got := unevaluatedReasons(diag, in); !maps.Equal(got, want) {
				t.Fatalf("unevaluated_required_rules = %v, want %v", got, want)
			}
		})
	}
}

// TestModuleRuleScopeSkipsModulesOwningOnlyOutOfScopeSource pins that a
// declared module whose walked files are all declared out of scope (a
// switched-off language) contributes nothing to a module rule's scope instead
// of making it undeterminable.
func TestModuleRuleScopeSkipsModulesOwningOnlyOutOfScopeSource(t *testing.T) {
	diag, in := vacuityFixture()
	in.Facts.OutOfScopeFiles = map[string]struct{}{tsAppFile: {}}
	withModules(&in, map[string]policy.ModuleDef{
		modBilling: {Paths: []string{selBilling}},
		modWeb:     {Paths: []string{"web/src/**"}},
	})
	withRule(&in, policy.RuleDef{Type: typeModCycle})
	if got := unevaluatedReasons(diag, in); len(got) != 0 {
		t.Fatalf("unevaluated_required_rules = %v, want none", got)
	}
}

// rustModuleFixture is a Go repo beside a Rust workspace member "core" whose
// cargo-modules graph has core::place and core::types, plus a fuzz target
// outside every workspace member.
func rustModuleFixture() (*result.Result, evaluation.StateInput) {
	diag, in := vacuityFixture()
	in.Facts.SourceSelectors[rustLibFile] = rustCrate
	in.Facts.FileLOC["fuzz/fuzz_targets/parse.rs"] = 5
	in.Facts.SourceSelectors["fuzz/fuzz_targets/parse.rs"] = ""
	in.Facts.UnanalysedFiles = map[string]struct{}{"fuzz/fuzz_targets/parse.rs": {}}
	in.Facts.RustModuleNodes = []string{rustModPlace, rustModTypes, "core::types::narrow"}
	return diag, in
}

// TestRustRuleScopeJudgesCrateModulesOverTheModuleGraph pins the Rust scope
// judgment: crate and crate::mod selectors over loaded members are decidable
// once the module graph is in hand, a crate without one stays undecidable, a
// file outside every workspace member is out of rule scope, and guard: true
// works on Rust selectors.
func TestRustRuleScopeJudgesCrateModulesOverTheModuleGraph(t *testing.T) {
	const generic = "rule scope cannot be established from the supported source inventory"
	rustModules := map[string]policy.ModuleDef{
		rustCrate:  {Paths: []string{rustCrate}},
		"types":    {Paths: []string{rustModTypes, rustModTypes + "::*"}},
		"place":    {Paths: []string{rustModPlace, rustModPlace + "::*"}},
		modBilling: {Paths: []string{selBilling}},
	}
	for _, tc := range []struct {
		name        string
		modules     map[string]policy.ModuleDef
		rule        policy.RuleDef
		noModGraph  bool
		wantReason  string
		wantPrefix  bool
		wantContain string
	}{
		{name: "module_cycle over crate::mod modules", modules: rustModules, rule: policy.RuleDef{Type: typeModCycle}},
		{name: "module_cycle without the crate's module graph", modules: rustModules, noModGraph: true,
			rule: policy.RuleDef{Type: typeModCycle}, wantContain: "module graph"},
		{name: "live crate::mod dependency rule", rule: policy.RuleDef{Type: ruleForbidden, From: rustModPlace, To: "core::types::narrow"}},
		{name: "dead crate::mod source", rule: policy.RuleDef{Type: ruleForbidden, From: "core::gone", To: rustModTypes},
			wantReason: "selector matches nothing: from core::gone"},
		{name: "dead crate::mod target", rule: policy.RuleDef{Type: ruleForbidden, From: rustModPlace, To: "core::gone::**"},
			wantReason: "selector matches nothing: to core::gone::**"},
		{name: "guard on a dead crate::mod target", rule: policy.RuleDef{Type: ruleForbidden, From: rustModPlace, To: "core::gone::**", Guard: true}},
		{name: "dead crate source", rule: policy.RuleDef{Type: ruleForbidden, From: "nonexistent", To: rustCrate},
			wantReason: "selector matches nothing: from nonexistent"},
		{name: "guard on a dead crate source", rule: policy.RuleDef{Type: ruleForbidden, From: "nonexistent", To: rustCrate, Guard: true}},
		{name: "crate::mod selector without the crate's module graph", noModGraph: true,
			rule: policy.RuleDef{Type: ruleForbidden, From: rustModPlace, To: rustModTypes}, wantReason: generic},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diag, in := rustModuleFixture()
			if tc.noModGraph {
				in.Facts.RustModuleNodes = nil
			}
			if tc.modules != nil {
				withModules(&in, tc.modules)
			}
			withRule(&in, tc.rule)
			got := unevaluatedReasons(diag, in)
			if tc.wantContain != "" {
				if !strings.Contains(got[ruleIDScoped], tc.wantContain) {
					t.Fatalf("unevaluated_required_rules = %v, want a reason containing %q", got, tc.wantContain)
				}
				return
			}
			want := map[string]string{}
			if tc.wantReason != "" {
				want[ruleIDScoped] = tc.wantReason
			}
			if !maps.Equal(got, want) {
				t.Fatalf("unevaluated_required_rules = %v, want %v", got, want)
			}
		})
	}
}

// TestRustCrateModuleSelectorsMatchUnderscoredCrateNames pins the two
// spellings of one crate: cargo names the package "my-core" while module
// graph node IDs use the library name my_core, so a dead my_core:: target is a
// dead first-party selector, never an external ban.
func TestRustCrateModuleSelectorsMatchUnderscoredCrateNames(t *testing.T) {
	diag, in := rustModuleFixture()
	in.Facts.SourceSelectors[rustLibFile] = "my-core"
	in.Facts.RustModuleNodes = []string{"my_core::api"}
	withRule(&in, policy.RuleDef{Type: ruleForbidden, From: selShipping, To: "my_core::gone"})
	want := map[string]string{ruleIDScoped: "selector matches nothing: to my_core::gone"}
	if got := unevaluatedReasons(diag, in); !maps.Equal(got, want) {
		t.Fatalf("unevaluated_required_rules = %v, want %v", got, want)
	}
}
