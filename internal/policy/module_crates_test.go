package policy

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/graph"
)

const (
	crateYaziShared = "yazi_shared"
	globURLURL      = "yazi_shared::url::url"
	layerBase       = "base"
	modSem          = "sem"
	crateLinter     = "ruff_linter"
	modShared       = "shared"
	modYaziShared   = "yazi-shared"
	modYaziFM       = "yazi-fm"
	modLinter       = "linter"
	modTypes        = "sem_types"
	modWeb          = "web"
	modPy           = "py"
)

// crateModuleMap declares Rust crates the three ways a config spells them: by
// package name (yazi), by directory glob (ruff-style crates/<crate>/**), and by
// an explicit crate::mod glob carved out of a crate. web and py are the
// non-Rust neighbours whose nodes must never take a crate's module.
func crateModuleMap() ModuleMap {
	mm := BuildModuleMap(map[string]ModuleDef{
		modYaziShared: {
			Paths:    []string{modYaziShared},
			Public:   []string{globURLURL},
			Internal: []string{"yazi_shared::url::**"},
			Layer:    layerBase,
		},
		modYaziFM: {Paths: []string{modYaziFM}, Layer: policyTestApp},
		modLinter: {Paths: []string{"crates/ruff_linter/**"}},
		modSem:    {Paths: []string{"crates/sem/**"}, Layer: layerBase},
		modTypes:  {Paths: []string{"sem::types", "sem::types::**"}, Layer: policyTestApp},
		modWeb:    {Paths: []string{"web/**"}},
		modPy:     {Paths: []string{"ruff_py.**"}},
	})
	return mm.WithCrateOwners(mm.CrateOwners([]graph.CrateRoot{
		{Dir: modYaziShared, Name: modYaziShared, Crate: crateYaziShared},
		{Dir: modYaziFM, Name: modYaziFM, Crate: "yazi"},
		{Dir: "crates/ruff_linter", Name: crateLinter, Crate: crateLinter},
		{Dir: "crates/sem", Name: modSem, Crate: modSem},
		{Dir: "crates/ruff", Name: "ruff", Crate: "ruff"},
		{Dir: "", Name: policyTestApp, Crate: policyTestApp},
	}))
}

func TestModuleForNode_ResolvesRustNodesToTheDeclaredCrateModule(t *testing.T) {
	mm := crateModuleMap()
	tests := []struct {
		name, path, language string
		want                 string
		wantOK               bool
	}{
		{"rust package node by its declared name", modYaziShared, graph.LangRust, modYaziShared, true},
		{"rust crate identifier", crateYaziShared, graph.LangRust, modYaziShared, true},
		{"rust crate::mod node", "yazi_shared::url::buf", graph.LangRust, modYaziShared, true},
		{"rust binary crate::mod node", "yazi::app::run", graph.LangRust, modYaziFM, true},
		{"rust crate node under a directory glob", crateLinter, graph.LangRust, modLinter, true},
		{"rust crate::mod node under a directory glob", "ruff_linter::rules::pyflakes", graph.LangRust, modLinter, true},
		{"explicit crate::mod glob wins over the crate", "sem::types::infer", graph.LangRust, modTypes, true},
		{"sibling of an explicit crate::mod module takes the crate", "sem::place", graph.LangRust, modSem, true},
		{"crate no module declares stays unowned", "ruff::cli", graph.LangRust, "", false},
		{"root crate is never claimed by its directory", "app::main", graph.LangRust, "", false},
		{"external crate stays unowned", "serde", graph.LangRust, "", false},
		{"go slash path keeps glob resolution", "web/handler", graph.LangGo, modWeb, true},
		{"typescript path keeps glob resolution", "web/src/app.ts", graph.LangTypeScript, modWeb, true},
		{"python dotted path keeps glob resolution", "ruff_py.cli", graph.LangPython, modPy, true},
		{"python node named like a crate is not a crate", crateLinter, graph.LangPython, "", false},
		{"go node matching a glob keeps glob resolution", modYaziShared, graph.LangGo, modYaziShared, true},
		{"go node named like a crate identifier is not a crate", crateYaziShared, graph.LangGo, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := mm.ModuleForNode(tc.path, tc.language)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("ModuleForNode(%q, %q) = (%q, %v), want (%q, %v)", tc.path, tc.language, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestModuleForNode_WithoutCrateOwnersIsModuleFor(t *testing.T) {
	mm := BuildModuleMap(map[string]ModuleDef{modYaziShared: {Paths: []string{modYaziShared}}})
	if got, ok := mm.ModuleForNode("yazi_shared::url::buf", graph.LangRust); ok {
		t.Errorf("ModuleForNode without crate owners = %q, want unresolved", got)
	}
	if got, _ := mm.ModuleForNode(modYaziShared, graph.LangRust); got != modYaziShared {
		t.Errorf("ModuleForNode(yazi-shared) = %q, want %q", got, modYaziShared)
	}
}

func TestMatchesInternal_RustOwnerIsTheDeclaredCrateModule(t *testing.T) {
	mm := crateModuleMap()
	tests := []struct {
		name, path   string
		wantInternal bool
		wantGlob     string
	}{
		{"the crate module's public glob exempts its crate::mod node", globURLURL, false, globURLURL},
		{"the crate module's internal glob still marks the rest", "yazi_shared::url::buf", true, "yazi_shared::url::**"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			internal, glob := mm.MatchesInternal(tc.path, graph.LangRust)
			if internal != tc.wantInternal || glob != tc.wantGlob {
				t.Errorf("MatchesInternal(%q) = (%v, %q), want (%v, %q)", tc.path, internal, glob, tc.wantInternal, tc.wantGlob)
			}
		})
	}
}

func TestLayerFor_RustCrateModNodeTakesTheCrateModuleLayer(t *testing.T) {
	mm := crateModuleMap()
	if got, ok := mm.LayerFor("sem::place", graph.LangRust); !ok || got != layerBase {
		t.Errorf("LayerFor(sem::place) = (%q, %v), want (base, true)", got, ok)
	}
	if got, ok := mm.LayerFor("sem::types::infer", graph.LangRust); !ok || got != policyTestApp {
		t.Errorf("LayerFor(sem::types::infer) = (%q, %v), want (app, true)", got, ok)
	}
}

// TestCrateOwners_PackageNameWinsACollision pins determinism when one crate's
// identifier spells another crate's package name.
func TestCrateOwners_PackageNameWinsACollision(t *testing.T) {
	modules := map[string]ModuleDef{
		"tools":   {Paths: []string{"tools/**"}},
		modShared: {Paths: []string{"crates/shared/**"}},
	}
	roots := []graph.CrateRoot{
		{Dir: "tools/a", Name: "a-cli", Crate: modShared},
		{Dir: "crates/shared", Name: modShared, Crate: modShared},
	}
	for _, order := range [][]graph.CrateRoot{roots, {roots[1], roots[0]}} {
		if got := BuildModuleMap(modules).CrateOwners(order)[modShared]; got != modShared {
			t.Errorf(`CrateOwners["shared"] = %q, want the module of the package named shared`, got)
		}
	}
}
