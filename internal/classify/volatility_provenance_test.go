package classify_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/classify"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/graph"
)

// Crate/node id constants shared by the provenance and override fixtures
// (herdr shape: a declared crate module plus cargo-modules submodule nodes).
const (
	crateName    = "mycrate"
	crateNodeA   = "package:mycrate::a"
	crateSubA    = "mycrate::a"
	crateSubB    = "mycrate::b"
	crateSubKeyS = "mycrate::state"
	crateNodeS   = "package:mycrate::state"
)

// TestComputeVolatilityProvenance verifies the module-level volatility source
// counts behind the coupling_balance triage disclosure: config-declared vs
// ancestor-inherited (synthetic modules) vs cascade-raised, with undeclared as
// the honest remainder. Disclosure-only — the counts never touch the balance.
func TestComputeVolatilityProvenance(t *testing.T) {
	t.Run("herdr shape: declared parent fans out to inherited synthetics", func(t *testing.T) {
		// One declared crate module (volatility high) whose cargo-modules
		// submodule nodes are NOT in config, plus one declared module with no
		// volatility from any source.
		declared := map[string]config.ModuleDef{
			crateName: {Paths: []string{crateName}, Owner: ownerTeamX, Volatility: extVolHigh},
			"tools":   {Paths: []string{"tools/**"}, Owner: ownerTeamX}, // no volatility, no subdomain
		}
		e := graph.Edge{
			From:         crateNodeA,
			To:           "package:" + crateSubB,
			Kind:         graph.EdgeKindDependsOn,
			Language:     langRust,
			StrengthHint: hintFunctional,
		}
		g := makeGraph([]graph.Edge{e})
		augmented := classify.AugmentModulesFromGraph(g, declared)

		vp := classify.ComputeVolatilityProvenance(g, declared, config.ClassifyConfig{Modules: augmented})
		if vp == nil {
			t.Fatal("ComputeVolatilityProvenance = nil for a non-empty module map")
		}
		if vp.Declared != 1 || vp.Inherited != 2 || vp.Cascade != 0 || vp.Undeclared != 1 {
			t.Errorf("counts = declared %d, inherited %d, cascade %d, undeclared %d; want 1, 2, 0, 1",
				vp.Declared, vp.Inherited, vp.Cascade, vp.Undeclared)
		}
	})

	t.Run("cascade-raised module counted on top of its base source", func(t *testing.T) {
		declared := map[string]config.ModuleDef{
			"caller": {Paths: []string{pathsA}, Owner: ownerTeamX, Volatility: cfgVolLow},
			"core":   {Paths: []string{pathsB}, Owner: ownerTeamX, Volatility: extVolHigh},
		}
		e := graph.Edge{
			From:         "package:services/a/x",
			To:           "package:services/b/y",
			Kind:         graph.EdgeKindDependsOn,
			StrengthHint: hintFunctional,
		}
		g := makeGraph([]graph.Edge{e})

		vp := classify.ComputeVolatilityProvenance(g, declared, config.ClassifyConfig{
			Modules:                  declared,
			VolatilityCascadeEnabled: true,
		})
		if vp == nil {
			t.Fatal("ComputeVolatilityProvenance = nil for a non-empty module map")
		}
		if vp.Declared != 2 || vp.Inherited != 0 || vp.Cascade != 1 || vp.Undeclared != 0 {
			t.Errorf("counts = declared %d, inherited %d, cascade %d, undeclared %d; want 2, 0, 1, 0",
				vp.Declared, vp.Inherited, vp.Cascade, vp.Undeclared)
		}
	})

	t.Run("empty module map discloses nothing", func(t *testing.T) {
		if vp := classify.ComputeVolatilityProvenance(nil, nil, config.ClassifyConfig{}); vp != nil {
			t.Errorf("ComputeVolatilityProvenance = %+v, want nil for an empty module map", vp)
		}
	})

	t.Run("synthetic module with no base volatility counts as undeclared, not inherited", func(t *testing.T) {
		// A synthetic module (absent from declared) whose own volatility and
		// subdomain are both unset has base volatility Undeclared. The switch in
		// ComputeVolatilityProvenance must test base==Undeclared BEFORE
		// isDeclared, so this module lands in vp.Undeclared even though it is
		// exactly the "not declared" shape that would otherwise fall to Inherited.
		declared := map[string]config.ModuleDef{
			modNameA: {Paths: []string{pathsA}, Owner: ownerTeamX, Volatility: extVolHigh},
		}
		modules := map[string]config.ModuleDef{
			modNameA:    declared[modNameA],
			"synthetic": {Paths: []string{"synthetic/**"}}, // not in declared, no volatility, no subdomain
		}
		vp := classify.ComputeVolatilityProvenance(nil, declared, config.ClassifyConfig{Modules: modules})
		if vp == nil {
			t.Fatal("ComputeVolatilityProvenance = nil for a non-empty module map")
		}
		if vp.Declared != 1 || vp.Inherited != 0 || vp.Undeclared != 1 {
			t.Errorf("counts = declared %d, inherited %d, undeclared %d; want 1, 0, 1 (undeclared wins over inherited)",
				vp.Declared, vp.Inherited, vp.Undeclared)
		}
	})

	t.Run("cascade enabled with nil graph does not panic and raises no cascade count", func(t *testing.T) {
		modules := map[string]config.ModuleDef{
			modNameA: {Paths: []string{pathsA}, Volatility: extVolHigh},
		}
		vp := classify.ComputeVolatilityProvenance(nil, nil, config.ClassifyConfig{
			Modules:                  modules,
			VolatilityCascadeEnabled: true,
		})
		if vp == nil {
			t.Fatal("ComputeVolatilityProvenance = nil for a non-empty module map")
		}
		if vp.Cascade != 0 {
			t.Errorf("Cascade = %d, want 0 (nil graph guards the cascade pass)", vp.Cascade)
		}
	})
}

// TestSyntheticModuleVolatilityOverride_HerdrShape locks the per-submodule
// volatility override path: a config `modules:` entry whose paths glob matches
// a synthetic module key ("mycrate::state") pre-empts synthetic registration
// (most-specific glob wins) and its volatility applies to edges targeting that
// submodule, while sibling submodules keep the ancestor-inherited volatility.
func TestSyntheticModuleVolatilityOverride_HerdrShape(t *testing.T) {
	configMods := map[string]config.ModuleDef{
		crateName:       {Paths: []string{crateName}, Owner: ownerTeamX, Volatility: extVolHigh},
		"mycrate-state": {Paths: []string{crateSubKeyS}, Owner: ownerTeamX, Volatility: cfgVolLow},
	}
	eToState := graph.Edge{
		From:         crateNodeA,
		To:           crateNodeS,
		Kind:         graph.EdgeKindDependsOn,
		Language:     langRust,
		StrengthHint: hintFunctional,
	}
	eToSibling := graph.Edge{
		From:         crateNodeS,
		To:           crateNodeA,
		Kind:         graph.EdgeKindDependsOn,
		Language:     langRust,
		StrengthHint: hintFunctional,
	}
	g := makeGraph([]graph.Edge{eToState, eToSibling})

	augmented := classify.AugmentModulesFromGraph(g, configMods)
	if _, exists := augmented[crateSubKeyS]; exists {
		t.Fatal("mycrate::state must bind to the config override, not a synthetic module")
	}
	if _, exists := augmented[crateSubA]; !exists {
		t.Fatal("mycrate::a must still register as a synthetic module")
	}

	idx := classify.Run(g, config.ClassifyConfig{Modules: augmented})
	if cl := idx[edgeKey(eToState)]; cl.Volatility != coupling.VolatilityLow {
		t.Errorf("edge → overridden submodule: Volatility = %q, want low (config override)", cl.Volatility)
	}
	if cl := idx[edgeKey(eToSibling)]; cl.Volatility != coupling.VolatilityHigh {
		t.Errorf("edge → sibling submodule: Volatility = %q, want high (ancestor-inherited)", cl.Volatility)
	}
}
