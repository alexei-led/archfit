package evaluation_test

import (
	"maps"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	modevidence "github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/policy"
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
	modWebUI     = "web_ui"

	reasonDepCruiserAbsent = "dependency-cruiser evidence is absent"
	selLayerDomain         = "layer:domain"
	selLayerNowhere        = "layer:nowhere"
	modWarehouse           = "warehouse"
	layerNameDomain        = "domain"
	layerNameApp           = "app"
	pkgHTTP                = "net/http"
	modPlace               = "place"
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
		modWebUI:   {Paths: []string{webUIGlob}},
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
			rule: policy.RuleDef{Type: typeModCycle}, wantReason: reasonDepCruiserAbsent},
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

// TestRustModuleRuleScopeKnowsBinaryTargetCrates pins that a loaded crate
// whose target name differs from its package name (package yazi-fm, binary
// target yazi) is a known crate: with no module graph, modules declared as
// yazi::a and yazi::b leave a fail-gated module_cycle unevaluated instead of
// reading as absent and passing with no findings.
func TestRustModuleRuleScopeKnowsBinaryTargetCrates(t *testing.T) {
	diag, in := rustModuleFixture()
	in.Facts.SourceSelectors[rustLibFile] = "yazi-fm"
	in.Facts.RustCrates = []string{"yazi", "yazi_fm"}
	in.Facts.RustModuleNodes = nil
	withModules(&in, map[string]policy.ModuleDef{
		"a": {Paths: []string{"yazi::a"}},
		"b": {Paths: []string{"yazi::b"}},
	})
	withRule(&in, policy.RuleDef{Type: typeModCycle})
	if got := unevaluatedReasons(diag, in); !strings.Contains(got[ruleIDScoped], "module graph") {
		t.Fatalf("unevaluated_required_rules = %v, want %s unevaluated for the missing module graph", got, ruleIDScoped)
	}
}

const (
	crateTool    = "tool"
	crateToolCLI = "tool_cli"
)

// TestRustModuleRuleScopeKnowsEveryLoadedTarget pins the inventory a lib+bin
// package (package tool, lib tool, bin tool-cli) feeds rule scope: once
// cargo metadata names the binary target tool_cli, modules declared under it
// with no module graph leave a fail-gated module_cycle unevaluated. A lib-only
// package names no tool_cli, so the same modules stay absent and the rule
// stays out of scope.
func TestRustModuleRuleScopeKnowsEveryLoadedTarget(t *testing.T) {
	for _, tc := range []struct {
		name            string
		crates          []string
		wantUnevaluated bool
	}{
		{name: "lib and bin", crates: []string{crateTool, crateToolCLI}, wantUnevaluated: true},
		{name: "lib only", crates: []string{crateTool}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diag, in := rustModuleFixture()
			in.Facts.SourceSelectors[rustLibFile] = crateTool
			in.Facts.RustCrates = tc.crates
			in.Facts.RustModuleNodes = nil
			withModules(&in, map[string]policy.ModuleDef{
				"a": {Paths: []string{crateToolCLI + "::a"}},
				"b": {Paths: []string{crateToolCLI + "::b"}},
			})
			withRule(&in, policy.RuleDef{Type: typeModCycle})
			if _, listed := unevaluatedReasons(diag, in)[ruleIDScoped]; listed != tc.wantUnevaluated {
				t.Fatalf("%s listed in unevaluated_required_rules = %v, want %v", ruleIDScoped, listed, tc.wantUnevaluated)
			}
		})
	}
}

// TestRustCrateWithEmptyModuleGraphDecidesSelectors pins that a crate
// cargo-modules graphed with no submodule has an empty module graph, not a
// missing one: a core::legacy selector under it matches nothing, and a guard
// on it holds.
func TestRustCrateWithEmptyModuleGraphDecidesSelectors(t *testing.T) {
	for _, tc := range []struct {
		name       string
		guard      bool
		wantReason string
	}{
		{name: "dead selector", wantReason: "selector matches nothing: from core::legacy"},
		{name: "guard on the empty module graph", guard: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diag, in := rustModuleFixture()
			in.Facts.RustModuleNodes = nil
			in.Facts.RustModuleGraphCrates = []string{rustCrate}
			withRule(&in, policy.RuleDef{Type: ruleForbidden, From: "core::legacy", To: selBilling, Guard: tc.guard})
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

// TestModuleDependenciesScopeIsTheDeclaringModules pins the per-module
// evaluability of module_dependencies: its scope is the languages of the
// modules that declare depends_on or visible_to, so a Go-only allowlist stays
// evaluated while the TypeScript producer is missing, an allowlist on the
// TypeScript module waits for it, and a rule no module gives a list is a
// policy defect.
func TestModuleDependenciesScopeIsTheDeclaringModules(t *testing.T) {
	const typeModDeps = "module_dependencies"
	for _, tc := range []struct {
		name       string
		modules    map[string]policy.ModuleDef
		wantReason string
	}{
		{name: "allowlist on a Go module", modules: map[string]policy.ModuleDef{
			modBilling: {Paths: []string{selBilling}, VisibleTo: []string{}},
			modWebUI:   {Paths: []string{webUIGlob}},
		}},
		{name: "allowlist on the TypeScript module", modules: map[string]policy.ModuleDef{
			modBilling: {Paths: []string{selBilling}},
			modWebUI:   {Paths: []string{webUIGlob}, DependsOn: []string{modBilling}},
		}, wantReason: reasonDepCruiserAbsent},
		{name: "no module declares an allowlist", modules: map[string]policy.ModuleDef{
			modBilling: {Paths: []string{selBilling}},
			modWebUI:   {Paths: []string{webUIGlob}},
		}, wantReason: "selector matches nothing: no module declares depends_on or visible_to"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diag, in := typescriptAbsentFixture()
			diag.CoverageGaps = []modevidence.CoverageGap{{Tool: toolDepCruiser}}
			in.Facts.UnanalysedFiles = nil
			withModules(&in, tc.modules)
			withRule(&in, policy.RuleDef{Type: typeModDeps})
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

// TestModuleSelectorScopeAndVacuity pins a forbidden_dependency rule with
// module selectors: a from_module that selects a Go module stays evaluated
// while the TypeScript producer is missing, a from_module that selects the
// TypeScript module waits for it unless its to_module speaks another language
// (no producer emits a cross-language edge), a selector that selects no module is a
// policy defect named by side, and a guard over such a selector holds.
func TestModuleSelectorScopeAndVacuity(t *testing.T) {
	modules := map[string]policy.ModuleDef{
		modBilling: {Paths: []string{selBilling}, Layer: layerNameDomain},
		modWebUI:   {Paths: []string{webUIGlob}, Layer: "ui"},
	}
	for _, tc := range []struct {
		name       string
		rule       policy.RuleDef
		wantReason string
	}{
		{name: "from_module on a Go module", rule: policy.RuleDef{Type: ruleForbidden, FromModule: selLayerDomain, To: pkgHTTP}},
		{name: "from_module on the TypeScript module", rule: policy.RuleDef{Type: ruleForbidden, FromModule: "layer:ui", To: "react"},
			wantReason: reasonDepCruiserAbsent},
		{name: "TypeScript module to a Go module cannot be an edge", rule: policy.RuleDef{Type: ruleForbidden, FromModule: "layer:ui", ToModule: modBilling}},
		{name: "from_module selects nothing", rule: policy.RuleDef{Type: ruleForbidden, FromModule: selLayerNowhere, To: pkgHTTP},
			wantReason: "selector matches nothing: from_module " + selLayerNowhere},
		{name: "to_module selects nothing", rule: policy.RuleDef{Type: ruleForbidden, FromModule: modBilling, ToModule: modWarehouse},
			wantReason: "selector matches nothing: to_module " + modWarehouse},
		{name: "guard over a module selector", rule: policy.RuleDef{Type: ruleForbidden, FromModule: "**", ToModule: "legacy", Guard: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diag, in := typescriptAbsentFixture()
			diag.CoverageGaps = []modevidence.CoverageGap{{Tool: toolDepCruiser}}
			in.Facts.UnanalysedFiles = nil
			withModules(&in, modules)
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

// TestToModuleScopeNeverFallsBackToTheSourceSide pins the target side of a
// module-selector rule: a to_module whose modules the inventory cannot judge
// (a crate::mod path without the module graph) keeps the rule unevaluated with
// that reason; a to_module whose modules own only source no producer analyses,
// or no source at all, is reported like a path glob that matches the same
// source. None counts as an evaluated rule.
func TestToModuleScopeNeverFallsBackToTheSourceSide(t *testing.T) {
	t.Run("target the inventory cannot judge", func(t *testing.T) {
		diag, in := rustModuleFixture()
		in.Facts.RustModuleNodes = nil
		withModules(&in, map[string]policy.ModuleDef{
			modBilling: {Paths: []string{selBilling}},
			modPlace:   {Paths: []string{rustModPlace}},
		})
		withRule(&in, policy.RuleDef{Type: ruleForbidden, FromModule: modBilling, ToModule: modPlace})
		if got := unevaluatedReasons(diag, in)[ruleIDScoped]; !strings.Contains(got, "module graph") {
			t.Errorf("unevaluated reason = %q, want the missing module graph named", got)
		}
	})
	t.Run("target with no analysed source", func(t *testing.T) {
		diag, in := typescriptAbsentFixture()
		withModules(&in, map[string]policy.ModuleDef{
			modBilling: {Paths: []string{selBilling}},
			modWebUI:   {Paths: []string{webUIGlob}},
		})
		withRule(&in, policy.RuleDef{Type: ruleForbidden, FromModule: modBilling, ToModule: modWebUI})
		want := "selector matches only source no dependency producer analyses: to_module " + modWebUI
		if got := unevaluatedReasons(diag, in)[ruleIDScoped]; got != want {
			t.Errorf("unevaluated reason = %q, want %q", got, want)
		}
	})
	const generic = "rule scope cannot be established from the supported source inventory"
	for _, tc := range []struct {
		name, path, want string
	}{
		{"target that provably owns no source", "internal/moved/x.go", "selector matches nothing: to_module moved"},
		{"target the inventory cannot judge from a directory glob", "internal/moved/**", generic},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diag, in := typescriptAbsentFixture()
			withModules(&in, map[string]policy.ModuleDef{
				modBilling: {Paths: []string{selBilling}},
				"moved":    {Paths: []string{tc.path}},
			})
			withRule(&in, policy.RuleDef{Type: ruleForbidden, FromModule: modBilling, ToModule: "moved"})
			if got := unevaluatedReasons(diag, in)[ruleIDScoped]; got != tc.want {
				t.Errorf("unevaluated reason = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRustModuleSelectorsFollowModuleRuleScope pins module selectors over Rust
// modules: a crate::mod path the module graph lacks, or whose every node a
// more specific module owns, matches nothing; a module
// declared by package name, whose files have no crate identity without cargo
// metadata, abstains rather than reading as empty.
func TestRustModuleSelectorsFollowModuleRuleScope(t *testing.T) {
	const generic = "rule scope cannot be established from the supported source inventory"
	t.Run("crate::mod path missing from the module graph", func(t *testing.T) {
		diag, in := rustModuleFixture()
		withModules(&in, map[string]policy.ModuleDef{
			modPlace: {Paths: []string{rustModPlace}},
			"gone":   {Paths: []string{"core::gone"}},
		})
		withRule(&in, policy.RuleDef{Type: ruleForbidden, FromModule: modPlace, ToModule: "gone"})
		if got := unevaluatedReasons(diag, in)[ruleIDScoped]; got != "selector matches nothing: to_module gone" {
			t.Errorf("unevaluated reason = %q, want the dead crate::mod module named", got)
		}
	})
	t.Run("crate-wide catch-all every node of which a specific module owns", func(t *testing.T) {
		diag, in := rustModuleFixture()
		in.Facts.RustModuleNodes = []string{rustModPlace, rustModTypes}
		withModules(&in, map[string]policy.ModuleDef{
			"remainder": {Paths: []string{"core::**"}},
			modPlace:    {Paths: []string{rustModPlace}},
			"types":     {Paths: []string{rustModTypes}},
		})
		withRule(&in, policy.RuleDef{Type: ruleForbidden, FromModule: modPlace, ToModule: "remainder"})
		if got := unevaluatedReasons(diag, in)[ruleIDScoped]; got != "selector matches nothing: to_module remainder" {
			t.Errorf("unevaluated reason = %q, want the fully shadowed module named", got)
		}
	})
	t.Run("package-name module without crate roots", func(t *testing.T) {
		diag, in := rustModuleFixture()
		in.Facts.SourceSelectors[rustLibFile] = ""
		withModules(&in, map[string]policy.ModuleDef{
			rustCrate:  {Paths: []string{rustCrate}, Layer: layerNameDomain},
			modBilling: {Paths: []string{selBilling}},
		})
		withRule(&in, policy.RuleDef{Type: ruleForbidden, FromModule: selLayerDomain, ToModule: modBilling})
		if got := unevaluatedReasons(diag, in)[ruleIDScoped]; got != generic {
			t.Errorf("unevaluated reason = %q, want the generic abstention", got)
		}
		for _, d := range evaluation.LintPolicy(in.Policy, in.Facts) {
			if strings.HasPrefix(d.Path, "rules[") {
				t.Errorf("lint reports %+v, want a module selector it cannot judge left alone", d)
			}
		}
	})
}

// TestModuleRuleScopeOwnsRustFilesThroughCrateOwners pins one ownership for
// rule scope and map completeness: a crate declared by a spelling no file
// selector carries still owns its files through the observed crate owners, so
// a module-wide rule over it is evaluated rather than held unknown.
func TestModuleRuleScopeOwnsRustFilesThroughCrateOwners(t *testing.T) {
	diag, in := rustModuleFixture()
	withModules(&in, map[string]policy.ModuleDef{
		"engine":   {Paths: []string{"core_engine"}},
		modBilling: {Paths: []string{selBilling}},
	})
	in.Facts.CrateOwners = map[string]string{rustCrate: "engine"}
	withRule(&in, policy.RuleDef{Type: typeModCycle})
	if got := unevaluatedReasons(diag, in); len(got) != 0 {
		t.Fatalf("unevaluated_required_rules = %v, want the module-wide rule evaluated", got)
	}
}
