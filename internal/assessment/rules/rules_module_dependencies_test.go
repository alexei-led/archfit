package rules_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/rules"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
)

const (
	typeModuleDependencies = "module_dependencies"
	ruleIDBoundaries       = "boundaries"
	modKernel              = "shared"
	violatesDependsOn      = "depends_on"
	violatesVisibleTo      = "visible_to"
	violatesBoth           = "depends_on,visible_to"
	globBilling            = "billing/**"
	modCoreCrate           = "core-mod"
	modAppCrate            = "app-mod"
)

// allowlistModules is the module map of the allowlist tests: billing is
// visible only to shipping, shipping may depend only on billing and shared,
// shared may depend on no first-party module, and catalog declares nothing.
func allowlistModules() map[string]policy.ModuleDef {
	return map[string]policy.ModuleDef{
		modBilling:  {Paths: []string{globBilling}, Public: []string{"billing/api"}, VisibleTo: []string{modShipping}},
		modShipping: {Paths: []string{"shipping/**"}, DependsOn: []string{modBilling, modKernel}},
		modKernel:   {Paths: []string{"shared/**"}, DependsOn: []string{}},
		modCatalog:  {Paths: []string{"catalog/**"}},
	}
}

func newModuleDependenciesRule(t *testing.T, modules map[string]policy.ModuleDef) rules.Rule {
	t.Helper()
	ruleSet, err := rules.New(policy.RuleConfig{
		Rules:     []policy.RuleDef{{ID: ruleIDBoundaries, Type: typeModuleDependencies, Gate: "fail"}},
		ModuleMap: policy.BuildModuleMap(modules),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return ruleSet[0]
}

// goImport is a Go import edge from a file to a package, located at line 3.
func goImport(file, pkg string) moduleTestEdge {
	return moduleTestEdge{from: "file:" + file, to: "package:" + pkg, locs: []relationship.Location{loc(file, 3)}}
}

// TestModuleDependencies_DecidesEachPair pins the allowlist predicate per edge
// shape: an absent list is unconstrained, an empty depends_on allows no
// first-party module, a pair both lists deny is one finding naming both keys,
// an importer no module owns is denied by visible_to only, and external
// targets and same-module edges are out of scope.
func TestModuleDependencies_DecidesEachPair(t *testing.T) {
	tests := []struct {
		name         string
		edge         moduleTestEdge
		wantFrom     string // from module, or "" for an unowned importer
		wantFromPath string
		wantTo       string
		wantViolates string // "" means no finding
	}{
		{name: "listed depends_on and visible_to", edge: goImport("shipping/app/ship.go", "billing/api")},
		{name: "depends_on denies", edge: goImport("shipping/app/ship.go", "catalog/api"),
			wantFrom: modShipping, wantTo: modCatalog, wantViolates: violatesDependsOn},
		{name: "visible_to denies", edge: goImport("catalog/app/list.go", "billing/api"),
			wantFrom: modCatalog, wantTo: modBilling, wantViolates: violatesVisibleTo},
		{name: "both deny", edge: goImport("shared/util/x.go", "billing/api"),
			wantFrom: modKernel, wantTo: modBilling, wantViolates: violatesBoth},
		{name: "empty depends_on denies any module", edge: goImport("shared/util/x.go", "catalog/api"),
			wantFrom: modKernel, wantTo: modCatalog, wantViolates: violatesDependsOn},
		{name: "unowned importer of a visible_to module", edge: goImport("tools/gen/main.go", "billing/api"),
			wantFromPath: "tools/gen", wantTo: modBilling, wantViolates: violatesVisibleTo},
		{name: "unowned importer of an unconstrained module", edge: goImport("tools/gen/main.go", "catalog/api")},
		{name: "unconstrained importer", edge: goImport("catalog/app/list.go", "shipping/api")},
		{name: "external target", edge: goImport("shared/util/x.go", "net/http")},
		{name: "unowned target", edge: goImport("shipping/app/ship.go", "tools/gen")},
		{name: "same module", edge: goImport("shared/util/x.go", "shared/model")},
	}
	r := newModuleDependenciesRule(t, allowlistModules())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkProduction(r, moduleSet(tt.edge))
			if tt.wantViolates == "" {
				if len(got) != 0 {
					t.Fatalf("findings = %+v, want none", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("findings = %+v, want one", got)
			}
			f := got[0]
			if f.Edge.From.Module != tt.wantFrom || f.Edge.From.Path != tt.wantFromPath ||
				f.Edge.To.Module != tt.wantTo || f.Edge.To.Path != "" || f.Edge.Kind != kindModuleDep {
				t.Errorf("edge = %+v, want %q (path %q) -> %q, kind %s", f.Edge, tt.wantFrom, tt.wantFromPath, tt.wantTo, kindModuleDep)
			}
			if f.MatchedBy["violates"] != tt.wantViolates {
				t.Errorf("matched_by.violates = %q, want %q", f.MatchedBy["violates"], tt.wantViolates)
			}
			if f.Kind != finding.KindGate || f.Severity != finding.SeverityHigh {
				t.Errorf("kind/severity = %s/%s, want gate/high", f.Kind, f.Severity)
			}
			if len(f.Locations) != 1 || f.Locations[0] != tt.edge.locs[0] || f.MatchedBy["locations_total"] != "1" {
				t.Errorf("locations = %+v (total %s), want the import site", f.Locations, f.MatchedBy["locations_total"])
			}
			if strings.Contains(f.Constraint, "public API of") {
				t.Errorf("constraint = %q routes through the target's public API", f.Constraint)
			}
			for _, key := range strings.Split(tt.wantViolates, ",") {
				if !strings.Contains(f.Constraint, key) {
					t.Errorf("constraint = %q, want it to name %s", f.Constraint, key)
				}
			}
		})
	}
}

// TestModuleDependencies_WhyNamesTheDenyingLists pins the reason text: it names
// the pair and every list that denies it.
func TestModuleDependencies_WhyNamesTheDenyingLists(t *testing.T) {
	r := newModuleDependenciesRule(t, allowlistModules())
	tests := []struct {
		edge moduleTestEdge
		want string
	}{
		{goImport("shared/util/x.go", "billing/api"),
			"Module shared depends on module billing, which the module allowlist denies: shared's depends_on does not list billing; billing's visible_to does not list shared"},
		{goImport("tools/gen/main.go", "billing/api"),
			"tools/gen, which no declared module owns, depends on module billing, whose visible_to admits only the modules it lists"},
	}
	for _, tt := range tests {
		got := checkProduction(r, moduleSet(tt.edge))
		if len(got) != 1 || got[0].Why != tt.want {
			t.Errorf("why = %+v, want %q", got, tt.want)
		}
	}
}

// TestModuleDependencies_KeyedByModulePair pins finding identity: every
// import on one denied pair joins one finding, and adding or moving a file on
// that pair keeps its ID. An unowned importer is keyed by its package, so a
// second unowned package is a second finding.
func TestModuleDependencies_KeyedByModulePair(t *testing.T) {
	r := newModuleDependenciesRule(t, allowlistModules())
	before := checkProduction(r, moduleSet(goImport("catalog/app/list.go", "billing/api")))
	after := checkProduction(r, moduleSet(
		goImport("catalog/web/handler.go", "billing/api"),
		goImport("catalog/app/moved.go", "billing/model"),
	))
	if len(before) != 1 || len(after) != 1 {
		t.Fatalf("findings before = %d, after = %d, want one each", len(before), len(after))
	}
	if before[0].ID != after[0].ID {
		t.Errorf("finding ID moved with the files: %s -> %s", before[0].ID, after[0].ID)
	}
	if after[0].MatchedBy["locations_total"] != "2" || len(after[0].Locations) != 2 {
		t.Errorf("locations = %+v, want both import sites", after[0].Locations)
	}

	unowned := checkProduction(r, moduleSet(
		goImport("tools/gen/main.go", "billing/api"),
		goImport("tools/gen/flags.go", "billing/api"),
		goImport("tools/lint/main.go", "billing/api"),
	))
	if len(unowned) != 2 || unowned[0].ID == unowned[1].ID {
		t.Fatalf("unowned findings = %+v, want one per importing package", unowned)
	}
	for _, f := range unowned {
		if f.ID == before[0].ID {
			t.Errorf("unowned finding %s collides with the module-pair finding", f.ID)
		}
	}
}

// TestModuleDependencies_CountsOnlyProductionSources pins the module_cycle
// production scope: an import from a test file is not an architecture
// dependency.
func TestModuleDependencies_CountsOnlyProductionSources(t *testing.T) {
	const testFile = "catalog/app/list_test.go"
	r := newModuleDependenciesRule(t, allowlistModules())
	ev := rules.Evidence{FileClasses: map[string]fileclass.FileClass{testFile: fileclass.Test}}
	if got := r.Check(moduleSet(goImport(testFile, "billing/api")), ev); len(got) != 0 {
		t.Errorf("findings = %+v, want none from a test file", got)
	}
}

// TestModuleDependencies_ResolvesRustNodesToTheDeclaredCrateModule pins the
// declared-module resolution: a cargo-modules node of a crate declared by
// package name belongs to that module, so a crate-internal edge is not a
// cross-module dependency, and an edge to another crate is judged by its lists.
func TestModuleDependencies_ResolvesRustNodesToTheDeclaredCrateModule(t *testing.T) {
	mm := policy.BuildModuleMap(map[string]policy.ModuleDef{
		modCoreCrate: {Paths: []string{"my-core"}, VisibleTo: []string{}},
		modAppCrate:  {Paths: []string{"my-app"}},
	}).WithCrateOwners(map[string]string{"my-core": modCoreCrate, "my_core": modCoreCrate, "my-app": modAppCrate, "my_app": modAppCrate})
	ruleSet, err := rules.New(policy.RuleConfig{
		Rules:     []policy.RuleDef{{ID: ruleIDBoundaries, Type: typeModuleDependencies}},
		ModuleMap: mm,
	})
	if err != nil {
		t.Fatal(err)
	}
	rust := func(from, to string) relationship.Edge {
		return relationship.Edge{FromID: "module:" + from, ToID: "module:" + to, FromPath: from, ToPath: to, Kind: edgeKindImports, Language: "rust"}
	}
	set := relationship.Set{Edges: []relationship.Edge{
		rust("my_core::url", "my_core::fs"),
		rust("my_app::cli", "my_core::url"),
	}}
	got := ruleSet[0].Check(set, rules.Evidence{})
	if len(got) != 1 || got[0].Edge.From.Module != modAppCrate || got[0].Edge.To.Module != modCoreCrate {
		t.Errorf("findings = %+v, want only app-mod -> core-mod", got)
	}
}

func TestModuleDependencies_RejectsScopeSelectors(t *testing.T) {
	for _, def := range []policy.RuleDef{
		{ID: "a", Type: typeModuleDependencies, From: globBilling},
		{ID: "b", Type: typeModuleDependencies, To: globBilling},
	} {
		if _, err := rules.New(policy.RuleConfig{Rules: []policy.RuleDef{def}}); err == nil {
			t.Errorf("New accepted %+v, want a config error: module_dependencies takes no from/to", def)
		}
	}
}

// TestModuleDependencies_ResolvesGoFilesThroughTheirPackage pins importer and
// target resolution against a module declared by its package directory or a
// one-segment glob: a Go file belongs to the module that owns its package, as
// in relationship analysis, so that module's depends_on is checked and its
// visible_to admissions hold.
func TestModuleDependencies_ResolvesGoFilesThroughTheirPackage(t *testing.T) {
	modules := map[string]policy.ModuleDef{
		modBilling:  {Paths: []string{globBilling}, VisibleTo: []string{modKernel}},
		modKernel:   {Paths: []string{"kernel"}, DependsOn: []string{}},
		modShipping: {Paths: []string{"cmd/*"}},
	}
	r := newModuleDependenciesRule(t, modules)
	tests := []struct {
		name         string
		edge         moduleTestEdge
		wantFrom     string
		wantViolates string
	}{
		{name: "package-dir module's empty depends_on", edge: goImport("kernel/x.go", "billing/api"),
			wantFrom: modKernel, wantViolates: violatesDependsOn},
		{name: "one-segment glob module importing a visible_to module", edge: goImport("cmd/tool/main.go", "billing/api"),
			wantFrom: modShipping, wantViolates: violatesVisibleTo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkProduction(r, moduleSet(tt.edge))
			if len(got) != 1 || got[0].Edge.From.Module != tt.wantFrom || got[0].MatchedBy["violates"] != tt.wantViolates {
				t.Fatalf("findings = %+v, want %s -> billing violating %s", got, tt.wantFrom, tt.wantViolates)
			}
		})
	}
}

// TestModuleDependencies_KeysRootUnownedImportersByPackage pins that two files
// of an unowned root package are one finding, keyed on the package ".", not on
// either file name.
func TestModuleDependencies_KeysRootUnownedImportersByPackage(t *testing.T) {
	r := newModuleDependenciesRule(t, allowlistModules())
	got := checkProduction(r, moduleSet(goImport("main.go", "billing/api"), goImport("flags.go", "billing/api")))
	if len(got) != 1 || got[0].Edge.From.Path != "." || len(got[0].Locations) != 2 {
		t.Fatalf("findings = %+v, want one from the root package with both sites", got)
	}
	renamed := checkProduction(r, moduleSet(goImport("cli.go", "billing/api")))
	if len(renamed) != 1 || renamed[0].ID != got[0].ID {
		t.Errorf("finding ID moved with a root file rename: %+v vs %s", renamed, got[0].ID)
	}
}
