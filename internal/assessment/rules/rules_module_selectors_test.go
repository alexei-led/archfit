package rules_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/rules"
	"github.com/alexei-led/archfit/internal/policy"
)

const (
	typeForbiddenDep = "forbidden_dependency"
	ruleIDSelector   = "domain_rule"
	selDomainLayer   = "layer:domain"
	pkgNetHTTP       = "net/http"
	globTools        = "tools/**"
)

// selectorModules is the module map of the module-selector tests: billing and
// catalog are the domain layer, shipping is an adapter with role adapter.
func selectorModules() map[string]policy.ModuleDef {
	return map[string]policy.ModuleDef{
		modBilling:  {Paths: []string{globBilling}, Layer: modDomain, Role: policy.RoleCore},
		modCatalog:  {Paths: []string{globCatalog}, Layer: modDomain},
		modShipping: {Paths: []string{globShipping}, Layer: "adapter", Role: policy.RoleAdapter},
	}
}

func newSelectorRule(t *testing.T, def policy.RuleDef) rules.Rule {
	t.Helper()
	def.ID, def.Type = ruleIDSelector, typeForbiddenDep
	ruleSet, err := rules.New(policy.RuleConfig{Rules: []policy.RuleDef{def}, ModuleMap: policy.BuildModuleMap(selectorModules())})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return ruleSet[0]
}

// TestForbiddenDependency_ModuleSelectors pins each selector form on each side:
// a layer, a role, a glob or exact name selects declared modules; a path glob
// keeps matching paths; an edge inside one module never matches; and a module
// side never matches an endpoint no declared module owns.
func TestForbiddenDependency_ModuleSelectors(t *testing.T) {
	tests := []struct {
		name     string
		def      policy.RuleDef
		edge     moduleTestEdge
		wantFrom finding.Endpoint
		wantTo   finding.Endpoint
	}{
		{name: "layer to an external package", def: policy.RuleDef{FromModule: selDomainLayer, To: pkgNetHTTP},
			edge:     goImport("billing/app/pay.go", pkgNetHTTP),
			wantFrom: finding.Endpoint{Module: modBilling}, wantTo: finding.Endpoint{Path: pkgNetHTTP}},
		{name: "exact module to exact module", def: policy.RuleDef{FromModule: modBilling, ToModule: modCatalog},
			edge:     goImport("billing/app/pay.go", "catalog/api"),
			wantFrom: finding.Endpoint{Module: modBilling}, wantTo: finding.Endpoint{Module: modCatalog}},
		{name: "role to a glob of modules", def: policy.RuleDef{FromModule: "role:adapter", ToModule: "cat*"},
			edge:     goImport("shipping/app/ship.go", "catalog/api"),
			wantFrom: finding.Endpoint{Module: modShipping}, wantTo: finding.Endpoint{Module: modCatalog}},
		{name: "path glob to a layer", def: policy.RuleDef{From: globTools, ToModule: selDomainLayer},
			edge:     goImport("tools/gen/main.go", "billing/api"),
			wantFrom: finding.Endpoint{Path: "tools/gen"}, wantTo: finding.Endpoint{Module: modBilling}},
		{name: "edge inside one module", def: policy.RuleDef{FromModule: selDomainLayer, ToModule: selDomainLayer},
			edge: goImport("billing/app/pay.go", "billing/api")},
		{name: "unselected importer", def: policy.RuleDef{FromModule: selDomainLayer, To: pkgNetHTTP},
			edge: goImport("shipping/app/ship.go", pkgNetHTTP)},
		{name: "unowned importer on a module side", def: policy.RuleDef{FromModule: "**", To: pkgNetHTTP},
			edge: goImport("tools/gen/main.go", pkgNetHTTP)},
		{name: "external target on a module side", def: policy.RuleDef{FromModule: modBilling, ToModule: "**"},
			edge: goImport("billing/app/pay.go", pkgNetHTTP)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newSelectorRule(t, tt.def).Check(moduleSet(tt.edge), rules.Evidence{})
			if tt.wantFrom == (finding.Endpoint{}) {
				if len(got) != 0 {
					t.Fatalf("findings = %+v, want none", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("findings = %+v, want one", got)
			}
			f := got[0]
			if f.Edge.From != tt.wantFrom || f.Edge.To != tt.wantTo || f.Edge.Kind != kindModuleDep {
				t.Errorf("edge = %+v, want %+v -> %+v (%s)", f.Edge, tt.wantFrom, tt.wantTo, kindModuleDep)
			}
			if len(f.Locations) != 1 || f.MatchedBy["locations_total"] != "1" || !strings.Contains(f.Why, "explicitly forbidden") {
				t.Errorf("finding = %+v, want the import site and a why", f)
			}
		})
	}
}

// TestForbiddenDependency_ModuleSelectorFindingsAreKeyedByPair pins identity:
// every import on one forbidden module pair joins one finding, and a new or
// moved file keeps its ID.
func TestForbiddenDependency_ModuleSelectorFindingsAreKeyedByPair(t *testing.T) {
	r := newSelectorRule(t, policy.RuleDef{FromModule: modBilling, ToModule: modCatalog})
	before := r.Check(moduleSet(goImport("billing/app/pay.go", "catalog/api")), rules.Evidence{})
	after := r.Check(moduleSet(goImport("billing/web/h.go", "catalog/api"), goImport("billing/app/moved.go", "catalog/model")), rules.Evidence{})
	if len(before) != 1 || len(after) != 1 || before[0].ID != after[0].ID || len(after[0].Locations) != 2 {
		t.Fatalf("before = %+v, after = %+v, want one finding with a stable ID", before, after)
	}
}

// TestForbiddenDependency_SeparateWays pins the "separate ways" pattern: two
// rules, one for each direction, each fire on their own direction only.
func TestForbiddenDependency_SeparateWays(t *testing.T) {
	forward := newSelectorRule(t, policy.RuleDef{FromModule: modBilling, ToModule: modCatalog})
	back := newSelectorRule(t, policy.RuleDef{FromModule: modCatalog, ToModule: modBilling})
	set := moduleSet(goImport("billing/app/pay.go", "catalog/api"))
	if got := forward.Check(set, rules.Evidence{}); len(got) != 1 {
		t.Errorf("forward findings = %+v, want one", got)
	}
	if got := back.Check(set, rules.Evidence{}); len(got) != 0 {
		t.Errorf("back findings = %+v, want none", got)
	}
}

func TestForbiddenDependency_ValidatesSelectors(t *testing.T) {
	mm := policy.BuildModuleMap(selectorModules())
	tests := []policy.RuleDef{
		{Type: typeForbiddenDep, To: pkgNetHTTP},
		{Type: typeForbiddenDep, From: globTools},
		{Type: typeForbiddenDep, From: globTools, FromModule: modBilling, To: pkgNetHTTP},
		{Type: typeForbiddenDep, FromModule: modBilling, To: pkgNetHTTP, ToModule: modCatalog},
		{Type: typeForbiddenDep, FromModule: "layer:", To: pkgNetHTTP},
		{Type: typeForbiddenDep, FromModule: "bill[", To: pkgNetHTTP},
		{Type: typeModuleDependencies, FromModule: modBilling},
		{Type: "module_cycle", ToModule: modBilling},
	}
	for _, def := range tests {
		def.ID = "r"
		if _, err := rules.New(policy.RuleConfig{Rules: []policy.RuleDef{def}, ModuleMap: mm}); err == nil {
			t.Errorf("New accepted %+v, want a config error", def)
		}
	}
	unknown := policy.RuleDef{ID: "r", Type: typeForbiddenDep, FromModule: "layer:nowhere", To: pkgNetHTTP}
	if _, err := rules.New(policy.RuleConfig{Rules: []policy.RuleDef{unknown}, ModuleMap: mm}); err != nil {
		t.Errorf("New rejected a well-formed selector that selects nothing: %v; it must load and be reported", err)
	}
}

// TestRationaleFieldsExplainEveryFinding pins rationale, alternatives, and
// docs on every rule type: the rationale ends the why, the alternatives
// become allowed alternatives, the docs end the why and the constraint (so
// SARIF, which prints the why, carries them), and none of them
// moves a finding ID. A rule without them leaves its findings unchanged.
func TestRationaleFieldsExplainEveryFinding(t *testing.T) {
	const (
		rationale = "Domain code stays free of I/O"
		docs      = "docs/adr/003.md"
		alt       = "Depend on a port in the application layer"
	)
	edge := goImport("billing/app/pay.go", "catalog/api")
	defs := []policy.RuleDef{
		{Type: typeForbiddenDep, From: "billing/**", To: "catalog/**"},
		{Type: typeForbiddenDep, FromModule: modBilling, ToModule: modCatalog},
		{Type: "new_cross_module_dependency"},
	}
	for _, def := range defs {
		t.Run(def.Type+def.FromModule, func(t *testing.T) {
			def.ID = ruleIDSelector
			plain := checkWith(t, def, edge)
			def.Rationale, def.Docs, def.Alternatives = rationale, docs, []string{alt}
			explained := checkWith(t, def, edge)
			if len(plain) != 1 || len(explained) != 1 {
				t.Fatalf("findings = %+v / %+v, want one each", plain, explained)
			}
			p, e := plain[0], explained[0]
			if e.ID != p.ID {
				t.Errorf("rationale moved the finding ID: %s -> %s", p.ID, e.ID)
			}
			if e.Why != p.Why+" — "+rationale+" (see "+docs+")" || e.Rationale != rationale || p.Rationale != "" {
				t.Errorf("why = %q, rationale = %q; want %q + rationale + docs", e.Why, e.Rationale, p.Why)
			}
			if e.Constraint != p.Constraint+" (see "+docs+")" {
				t.Errorf("constraint = %q, want the docs reference appended to %q", e.Constraint, p.Constraint)
			}
			if !slices.Equal(e.Alternatives, []string{alt}) || p.Alternatives != nil {
				t.Errorf("alternatives = %v / %v, want [%s] only when declared", e.Alternatives, p.Alternatives, alt)
			}
		})
	}
}

func checkWith(t *testing.T, def policy.RuleDef, edge moduleTestEdge) []finding.Finding {
	t.Helper()
	ruleSet, err := rules.New(policy.RuleConfig{Rules: []policy.RuleDef{def}, ModuleMap: policy.BuildModuleMap(selectorModules())})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return ruleSet[0].Check(moduleSet(edge), rules.Evidence{})
}
