package rules_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/rules"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
)

const (
	typeModuleCycle   = "module_cycle"
	ruleIDModuleCycle = "no_module_cycles"
	modBilling        = "billing"
	modShipping       = "shipping"
	modCatalog        = "catalog"
	kindModuleDep     = "module_dependency"

	pairBillingShipping = "billing->shipping"
	pairShippingBilling = "shipping->billing"
	nodeShopBilling     = "package:shop::billing"
	nodeBillingAGo      = "file:billing/a.go"
	nodeShippingBGo     = "file:shipping/b.go"
	nodePkgBilling      = "package:billing"
	nodePkgShipping     = "package:shipping"
)

// moduleTestEdge is a dependency edge whose endpoints relationship analysis
// already resolved to modules — the shape module rules read.
type moduleTestEdge struct {
	from, to             string
	fromModule, toModule string
	locs                 []relationship.Location
}

func moduleSet(edges ...moduleTestEdge) relationship.Set {
	set := relationship.Set{}
	for _, e := range edges {
		set.Edges = append(set.Edges, relationship.Edge{
			FromID: e.from, ToID: e.to,
			FromPath: relationship.NodePath(e.from), ToPath: relationship.NodePath(e.to),
			FromModule: e.fromModule, ToModule: e.toModule,
			Kind: edgeKindImports, Locations: e.locs,
		})
	}
	return set
}

func newModuleCycleRule(t *testing.T, gate string, declared ...string) rules.Rule {
	t.Helper()
	modules := make(map[string]policy.ModuleDef, len(declared))
	for _, name := range declared {
		modules[name] = policy.ModuleDef{Paths: []string{name + "/**"}}
	}
	ruleSet, err := rules.New(policy.RuleConfig{
		Rules:     []policy.RuleDef{{ID: ruleIDModuleCycle, Type: typeModuleCycle, Gate: gate}},
		ModuleMap: policy.BuildModuleMap(modules),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return ruleSet[0]
}

func loc(file string, line int) relationship.Location {
	return relationship.Location{File: file, Line: line}
}

// productionEvidence inventories every file an edge of set starts in as a
// Production source file.
func productionEvidence(set relationship.Set) rules.Evidence {
	classes := map[string]fileclass.FileClass{}
	for _, e := range set.Edges {
		if strings.HasPrefix(e.FromID, "file:") {
			classes[e.FromPath] = fileclass.Production
		}
		for _, l := range e.Locations {
			classes[l.File] = fileclass.Production
		}
	}
	return rules.Evidence{FileClasses: classes}
}

func checkProduction(r rules.Rule, set relationship.Set) []finding.Finding {
	return r.Check(set, productionEvidence(set))
}

// TestModuleCycle_CountsOnlyProductionSources pins that a module cycle is a
// production fact: an edge counts only when its importing source file is a
// Production file in the source inventory. A cycle that closes only through
// test or generated code, or through a file declared out of scope (an
// exclude: glob), is not reported. A file the source walk never visited (a Go
// package under a directory named target/, which go/packages still loads) has
// no class to prove it non-production, so its edge counts: a fail-gated cycle
// never passes silently. An edge with no source-file attribution (a Rust crate
// dependency located at Cargo.toml, a cargo-modules crate::mod edge) keeps
// counting: only its manifest kind could say otherwise, and the extractor
// already applied it.
func TestModuleCycle_CountsOnlyProductionSources(t *testing.T) {
	const (
		billingProd  = "billing/app/notify.go"
		shippingProd = "shipping/app/bill.go"
		shippingTest = "shipping/app/bill_test.go"
	)
	back := func(from string, locs ...relationship.Location) moduleTestEdge {
		return moduleTestEdge{from, "package:billing/app", modShipping, modBilling, locs}
	}
	forward := moduleTestEdge{"file:" + billingProd, nodePkgShipping, modBilling, modShipping,
		[]relationship.Location{loc(billingProd, 3)}}
	classes := map[string]fileclass.FileClass{
		billingProd: fileclass.Production, shippingProd: fileclass.Production, shippingTest: fileclass.Test,
		"shipping/src/x.stories.gen.ts": fileclass.Generated, "shop/shipping/test_app.py": fileclass.Test,
		"shop/shipping/app.py": fileclass.Production,
	}
	tests := []struct {
		name  string
		back  moduleTestEdge
		cycle bool
	}{
		{"go production import closes the cycle", back("file:"+shippingProd, loc(shippingProd, 3)), true},
		{"go test import does not", back("file:"+shippingTest, loc(shippingTest, 3)), false},
		{"typescript generated file does not", back("file:shipping/src/x.stories.gen.ts"), false},
		{"file declared out of scope does not",
			back("file:shipping/legacy/old.go", loc("shipping/legacy/old.go", 3)), false},
		{"file the walk never visited does",
			back("file:shipping/target/x.go", loc("shipping/target/x.go", 3)), true},
		{"unwalked file classified non-production does not",
			back("file:shipping/.storybook/preview.tsx"), false},
		{"unwalked file with no class does",
			back("file:shipping/.config/x.ts"), true},
		{"python test module does not",
			back("module:shop.shipping.test_app", loc("shop/shipping/test_app.py", 2)), false},
		{"python production module does",
			back("module:shop.shipping.app", loc("shop/shipping/app.py", 2)), true},
		{"rust crate dependency located at its manifest does",
			back("package:shipping", loc("Cargo.toml", 0)), true},
		{"rust crate::mod edge without sites does", back("package:shop::shipping"), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newModuleCycleRule(t, "", modBilling, modShipping)
			findings := r.Check(moduleSet(forward, tc.back), rules.Evidence{
				FileClasses: classes, OutOfScopeFiles: map[string]struct{}{"shipping/legacy/old.go": {}},
				UnwalkedSourceProduction: map[string]bool{"shipping/target/x.go": true, "shipping/.storybook/preview.tsx": false},
			})
			if got := len(findings) == 2; got != tc.cycle {
				t.Fatalf("cycle reported = %v (%d findings), want %v", got, len(findings), tc.cycle)
			}
		})
	}
}

// TestModuleCycle_FindsCyclesInEveryNodeVocabulary pins one finding per
// ordered module pair inside a cycle, whatever node vocabulary the language's
// extractor uses: Go file -> package, TypeScript file -> file, Python dotted
// modules, Rust crate::mod.
func TestModuleCycle_FindsCyclesInEveryNodeVocabulary(t *testing.T) {
	tests := []struct {
		name      string
		set       relationship.Set
		wantLocs  map[string][]relationship.Location // "from->to" -> locations
		wantTotal map[string]string
	}{
		{
			name: "go file to package edges through different files",
			set: moduleSet(
				moduleTestEdge{"file:billing/app/notify.go", "package:shipping/api", modBilling, modShipping,
					[]relationship.Location{loc("billing/app/notify.go", 3)}},
				moduleTestEdge{"file:shipping/app/bill.go", "package:billing/app", modShipping, modBilling,
					[]relationship.Location{loc("shipping/app/bill.go", 3)}},
			),
			wantLocs: map[string][]relationship.Location{
				pairBillingShipping: {loc("billing/app/notify.go", 3)},
				pairShippingBilling: {loc("shipping/app/bill.go", 3)},
			},
		},
		{
			name: "typescript file edges without sites locate the importing file",
			set: moduleSet(
				moduleTestEdge{"file:billing/src/a.ts", "file:shipping/src/b.ts", modBilling, modShipping, nil},
				moduleTestEdge{"file:shipping/src/c.ts", "file:billing/src/d.ts", modShipping, modBilling, nil},
			),
			wantLocs: map[string][]relationship.Location{
				pairBillingShipping: {loc("billing/src/a.ts", 0)},
				pairShippingBilling: {loc("shipping/src/c.ts", 0)},
			},
		},
		{
			name: "python dotted module edges",
			set: moduleSet(
				moduleTestEdge{"module:shop.billing.app", "module:shop.shipping.api", modBilling, modShipping,
					[]relationship.Location{loc("shop/billing/app.py", 4)}},
				moduleTestEdge{"module:shop.shipping.app", "module:shop.billing.app", modShipping, modBilling,
					[]relationship.Location{loc("shop/shipping/app.py", 7)}},
			),
			wantLocs: map[string][]relationship.Location{
				pairBillingShipping: {loc("shop/billing/app.py", 4)},
				pairShippingBilling: {loc("shop/shipping/app.py", 7)},
			},
		},
		{
			name: "rust crate::mod edges without sites carry no fabricated path",
			set: moduleSet(
				moduleTestEdge{nodeShopBilling, "package:shop::shipping", modBilling, modShipping, nil},
				moduleTestEdge{"package:shop::shipping::rates", nodeShopBilling, modShipping, modBilling, nil},
			),
			wantLocs: map[string][]relationship.Location{
				pairBillingShipping: {},
				pairShippingBilling: {},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newModuleCycleRule(t, "", modBilling, modShipping)
			findings := checkProduction(r, tc.set)
			if len(findings) != 2 {
				t.Fatalf("got %d findings, want one per direction: %+v", len(findings), findings)
			}
			for i, want := range []string{pairBillingShipping, pairShippingBilling} {
				f := findings[i]
				from, to, _ := strings.Cut(want, "->")
				if f.Edge.From != (finding.Endpoint{Module: from}) || f.Edge.To != (finding.Endpoint{Module: to}) || f.Edge.Kind != kindModuleDep {
					t.Errorf("finding %d edge = %+v, want %s -> %s with empty paths", i, f.Edge, from, to)
				}
				if f.ID != finding.NewKeyed(ruleIDModuleCycle, kindModuleDep, from, to).ID {
					t.Errorf("finding %d ID %q is not keyed on (rule, kind, %s, %s)", i, f.ID, from, to)
				}
				if f.Kind != kindGate || f.Severity != finding.SeverityHigh {
					t.Errorf("finding %d kind/severity = %s/%s, want gate/high", i, f.Kind, f.Severity)
				}
				if f.MatchedBy["cycle_modules"] != "billing, shipping" || f.MatchedBy["cycle_size"] != "2" {
					t.Errorf("finding %d matched_by = %v, want the cycle members", i, f.MatchedBy)
				}
				if got, wantLocs := fmt.Sprint(f.Locations), fmt.Sprint(tc.wantLocs[want]); got != wantLocs {
					t.Errorf("finding %d locations = %s, want %s", i, got, wantLocs)
				}
				if f.Why == "" || f.Constraint == "" {
					t.Errorf("finding %d has empty why/constraint", i)
				}
			}
		})
	}
}

func TestModuleCycle_FingerprintIsPinned(t *testing.T) {
	r := newModuleCycleRule(t, "", modBilling, modShipping)
	findings := checkProduction(r, moduleSet(
		moduleTestEdge{nodeBillingAGo, nodePkgShipping, modBilling, modShipping, nil},
		moduleTestEdge{nodeShippingBGo, nodePkgBilling, modShipping, modBilling, nil},
	))
	if len(findings) != 2 {
		t.Fatalf("got %d findings, want 2", len(findings))
	}
	// sha256("no_module_cycles\x00module_dependency\x00billing\x00shipping")[:16]:
	// accepted baseline entries depend on this staying put.
	if findings[0].ID != "c569fa89305a0875e0c8e48596cd99d6" {
		t.Errorf("billing -> shipping ID = %q, want the pinned c569fa89305a0875e0c8e48596cd99d6", findings[0].ID)
	}
}

func TestModuleCycle_NoFindingWithoutADeclaredModuleCycle(t *testing.T) {
	tests := []struct {
		name string
		set  relationship.Set
	}{
		{
			name: "acyclic module graph",
			set: moduleSet(
				moduleTestEdge{nodeBillingAGo, "package:shipping/api", modBilling, modShipping, nil},
				moduleTestEdge{nodeShippingBGo, "package:catalog/api", modShipping, modCatalog, nil},
			),
		},
		{
			name: "cycle inside one module",
			set: moduleSet(
				moduleTestEdge{"file:billing/a/a.go", "package:billing/b", modBilling, modBilling, nil},
				moduleTestEdge{"file:billing/b/b.go", "package:billing/a", modBilling, modBilling, nil},
			),
		},
		{
			name: "unowned endpoint",
			set: moduleSet(
				moduleTestEdge{nodeBillingAGo, "package:tools/gen", modBilling, "", nil},
				moduleTestEdge{"file:tools/gen/g.go", nodePkgBilling, "", modBilling, nil},
			),
		},
		{
			name: "cycle through an undeclared synthetic module",
			set: moduleSet(
				moduleTestEdge{nodeShopBilling, "package:shop::auto", modBilling, "shop::auto", nil},
				moduleTestEdge{"package:shop::auto", nodeShopBilling, "shop::auto", modBilling, nil},
			),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newModuleCycleRule(t, "", modBilling, modShipping, modCatalog)
			if findings := checkProduction(r, tc.set); len(findings) != 0 {
				t.Fatalf("got %d findings, want 0: %+v", len(findings), findings)
			}
		})
	}
}

func TestModuleCycle_ThreeModuleCycleAndOutsiders(t *testing.T) {
	r := newModuleCycleRule(t, "", modBilling, modShipping, modCatalog, "audit")
	findings := checkProduction(r, moduleSet(
		moduleTestEdge{nodeBillingAGo, nodePkgShipping, modBilling, modShipping, nil},
		moduleTestEdge{nodeShippingBGo, "package:catalog", modShipping, modCatalog, nil},
		moduleTestEdge{"file:catalog/c.go", nodePkgBilling, modCatalog, modBilling, nil},
		// audit depends on the cycle but is not part of it.
		moduleTestEdge{"file:audit/d.go", nodePkgBilling, "audit", modBilling, nil},
	))
	got := make([]string, 0, len(findings))
	for _, f := range findings {
		got = append(got, f.Edge.From.Module+"->"+f.Edge.To.Module)
		if f.MatchedBy["cycle_size"] != "3" || f.MatchedBy["cycle_modules"] != "billing, catalog, shipping" {
			t.Errorf("matched_by = %v, want the three-member cycle", f.MatchedBy)
		}
	}
	if want := "[billing->shipping catalog->billing shipping->catalog]"; fmt.Sprint(got) != want {
		t.Errorf("findings = %v, want %s in deterministic order", got, want)
	}
}

func TestModuleCycle_CapsLocationsAndReportsTheTotal(t *testing.T) {
	var sites []relationship.Location
	for i := 60; i > 0; i-- { // reverse order: the rule sorts
		sites = append(sites, loc(fmt.Sprintf("billing/f%02d.go", i), 3))
	}
	sites = append(sites, loc("billing/f01.go", 3)) // duplicate site
	r := newModuleCycleRule(t, "", modBilling, modShipping)
	findings := checkProduction(r, moduleSet(
		moduleTestEdge{nodeBillingAGo, nodePkgShipping, modBilling, modShipping, sites},
		moduleTestEdge{nodeShippingBGo, nodePkgBilling, modShipping, modBilling, nil},
	))
	if len(findings) != 2 {
		t.Fatalf("got %d findings, want 2", len(findings))
	}
	f := findings[0]
	if len(f.Locations) != 50 || f.MatchedBy["locations_total"] != "60" {
		t.Fatalf("locations = %d, total = %s, want 50 kept of 60 distinct", len(f.Locations), f.MatchedBy["locations_total"])
	}
	if f.Locations[0].File != "billing/f01.go" || f.Locations[49].File != "billing/f50.go" {
		t.Errorf("locations not sorted: first %v last %v", f.Locations[0], f.Locations[49])
	}
}

func TestModuleCycle_GateAndValidation(t *testing.T) {
	cyclic := moduleSet(
		moduleTestEdge{nodeBillingAGo, nodePkgShipping, modBilling, modShipping, nil},
		moduleTestEdge{nodeShippingBGo, nodePkgBilling, modShipping, modBilling, nil},
	)
	if findings := newModuleCycleRule(t, "off", modBilling, modShipping).Check(cyclic, productionEvidence(cyclic)); len(findings) != 0 {
		t.Errorf("gate off: got %d findings, want 0", len(findings))
	}
	for _, f := range newModuleCycleRule(t, gateWarn, modBilling, modShipping).Check(cyclic, productionEvidence(cyclic)) {
		if f.Kind != kindAdvisory {
			t.Errorf("gate warn: kind = %q, want advisory", f.Kind)
		}
	}

	for _, def := range []policy.RuleDef{
		{ID: "scoped_from", Type: typeModuleCycle, From: globBilling},
		{ID: "scoped_to", Type: typeModuleCycle, To: "shipping/**"},
	} {
		if _, err := rules.New(policy.RuleConfig{Rules: []policy.RuleDef{def}}); err == nil || !strings.Contains(err.Error(), def.ID) {
			t.Errorf("%s: err = %v, want a config error naming the rule", def.ID, err)
		}
	}
}

// meshEdges returns one edge for every ordered pair of the named modules: a
// fully connected strongly-connected component.
func meshEdges(modules []string) []moduleTestEdge {
	var edges []moduleTestEdge
	for _, from := range modules {
		for _, to := range modules {
			if from != to {
				edges = append(edges, moduleTestEdge{"file:" + from + "/a.go", "package:" + to, from, to, nil})
			}
		}
	}
	return edges
}

// TestModuleCycle_CapsPairsPerCycleAndKeepsTheirIDs bounds a large cycle's
// findings: a fully meshed 16-module component has 240 ordered pairs and
// reports the first 200 in (from, to) order, each naming the full count. A
// small cycle elsewhere is unaffected, and every kept pair keeps the ID it has
// without the cap.
func TestModuleCycle_CapsPairsPerCycleAndKeepsTheirIDs(t *testing.T) {
	const reported = 200
	mesh := make([]string, 0, 16)
	for i := range 16 {
		mesh = append(mesh, fmt.Sprintf("m%02d", i))
	}
	declared := append([]string{modBilling, modShipping}, mesh...)
	edges := append(meshEdges(mesh),
		moduleTestEdge{nodeBillingAGo, nodePkgShipping, modBilling, modShipping, nil},
		moduleTestEdge{nodeShippingBGo, nodePkgBilling, modShipping, modBilling, nil},
	)
	findings := newModuleCycleRule(t, "", declared...).Check(moduleSet(edges...), rules.Evidence{})

	var meshPairs []string
	small := 0
	for _, f := range findings {
		pair := f.Edge.From.Module + "->" + f.Edge.To.Module
		if f.MatchedBy["cycle_size"] == "2" {
			small++
			if f.MatchedBy["cycle_pairs_total"] != "2" || strings.Contains(f.Why, "reported") {
				t.Errorf("%s: matched_by = %v, why = %q, want the uncapped two-pair cycle", pair, f.MatchedBy, f.Why)
			}
			continue
		}
		meshPairs = append(meshPairs, pair)
		if f.MatchedBy["cycle_pairs_total"] != "240" || !strings.Contains(f.Why, "200 of the cycle's 240 module pairs are reported") {
			t.Errorf("%s: matched_by = %v, why = %q, want the reported share of 240 pairs", pair, f.MatchedBy, f.Why)
		}
		if want := finding.NewKeyed(ruleIDModuleCycle, kindModuleDep, f.Edge.From.Module, f.Edge.To.Module).ID; f.ID != want {
			t.Errorf("%s: ID = %s, want the pair key %s", pair, f.ID, want)
		}
	}
	if small != 2 || len(meshPairs) != reported {
		t.Fatalf("findings: %d in the small cycle, %d in the mesh; want 2 and %d", small, len(meshPairs), reported)
	}
	if meshPairs[0] != "m00->m01" || meshPairs[reported-1] != "m13->m04" {
		t.Errorf("kept pairs run %s .. %s, want the first %d in (from, to) order: m00->m01 .. m13->m04", meshPairs[0], meshPairs[reported-1], reported)
	}
}
