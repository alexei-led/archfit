package rules_test

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/rules"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

// The yazi shape: crates declared by package name, a crate::mod internal: glob
// and a crate::mod public: glob on the shared crate, and cargo-modules nodes
// spelled with the crate identifier (yazi_shared), not the package name.
const (
	layerBase     = "base"
	layerCore     = "core"
	modFFI        = "ffi"
	crateFFI      = "yazi-ffi"
	typeLayerDir  = "forbidden_layer_direction"
	modShared     = "yazi-shared"
	modFS         = "yazi-fs"
	globSharedURL = "yazi_shared::url::**"
	nodeURLBuf    = "package:yazi_shared::url::buf"
	nodeURLCov    = "package:yazi_shared::url::cov"
	nodeURLURL    = "package:yazi_shared::url::url"
	nodeDataData  = "package:yazi_shared::data::data"
	nodeFSPath    = "package:yazi_fs::path"
)

func rustCrateRuleConfig(ruleType string) policy.RuleConfig {
	mm := policy.BuildModuleMap(map[string]policy.ModuleDef{
		modShared: {
			Paths:    []string{"yazi-shared"},
			Public:   []string{"yazi_shared::url::url"},
			Internal: []string{globSharedURL},
			Layer:    layerBase,
		},
		modFS:  {Paths: []string{"yazi-fs"}, Layer: layerCore},
		modFFI: {Paths: []string{crateFFI}, Internal: []string{crateFFI}, Layer: layerBase},
	}).WithCrateOwners(map[string]string{
		"yazi-shared": modShared, "yazi_shared": modShared,
		"yazi-fs": modFS, "yazi_fs": modFS,
		crateFFI: modFFI, "yazi_ffi": modFFI,
	})
	return policy.RuleConfig{
		Rules:     []policy.RuleDef{{ID: "r", Type: ruleType}},
		Layers:    []string{layerBase, layerCore},
		ModuleMap: mm,
	}
}

func TestInternalAccessRules_RustCrateModResolvesToTheDeclaredCrate(t *testing.T) {
	tests := []struct {
		name string
		edge testEdge
		want int
	}{
		{"same-crate access into a declared internal crate::mod does not fire",
			testEdge{From: nodeURLBuf, To: nodeURLCov, Kind: edgeKindDependsOn, Language: langRust}, 0},
		{"same-crate access from another crate::mod does not fire",
			testEdge{From: nodeDataData, To: nodeURLBuf, Kind: edgeKindDependsOn, Language: langRust}, 0},
		{"the owner's public: glob exempts its crate::mod target",
			testEdge{From: nodeFSPath, To: nodeURLURL, Kind: edgeKindDependsOn, Language: langRust}, 0},
		{"cross-crate access into a declared internal crate::mod fires",
			testEdge{From: nodeFSPath, To: nodeURLBuf, Kind: edgeKindDependsOn, Language: langRust}, 1},
		{"cross-crate dependency on a declared internal crate fires",
			testEdge{From: "package:yazi-fs", To: "package:yazi-ffi", Kind: edgeKindDependsOn, Language: langRust}, 1},
		{"a non-Rust edge never takes a crate's module",
			testEdge{From: "package:yazi_fs", To: "package:yazi_shared::url::buf", Kind: edgeKindDependsOn, Language: langPython}, 1},
	}
	for _, ruleType := range []string{typePublicAPIOnly, typeInternalAPIAccess} {
		ruleSet, err := rules.New(rustCrateRuleConfig(ruleType))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		for _, tc := range tests {
			t.Run(ruleType+"/"+tc.name, func(t *testing.T) {
				findings := ruleSet[0].Check(makeGraph([]testEdge{tc.edge}), rules.Evidence{})
				if len(findings) != tc.want {
					t.Fatalf("got %d findings, want %d: %+v", len(findings), tc.want, findings)
				}
			})
		}
	}
}

func TestPublicAPIOnly_RustCrossCrateWhyNamesTheDeclaredModules(t *testing.T) {
	ruleSet, err := rules.New(rustCrateRuleConfig(typePublicAPIOnly))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	findings := ruleSet[0].Check(makeGraph([]testEdge{{From: nodeFSPath, To: nodeURLBuf, Kind: edgeKindDependsOn, Language: langRust}}), rules.Evidence{})
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1", len(findings))
	}
	want := `Cross-module access from "yazi_fs::path" (yazi-fs) to internal path "yazi_shared::url::buf" (yazi-shared)`
	if findings[0].Why != want {
		t.Errorf("why = %q, want %q", findings[0].Why, want)
	}
}

func TestModuleRules_RustCrateModResolvesToTheDeclaredCrate(t *testing.T) {
	tests := []struct {
		name, ruleType string
		edge           testEdge
		want           int
	}{
		{"new_cross_module_dependency ignores a same-crate crate::mod edge", typeNewCrossModuleDependency,
			testEdge{From: nodeDataData, To: nodeURLBuf, Kind: edgeKindDependsOn, Language: langRust}, 0},
		{"new_cross_module_dependency sees a cross-crate crate::mod edge", typeNewCrossModuleDependency,
			testEdge{From: nodeFSPath, To: nodeURLBuf, Kind: edgeKindDependsOn, Language: langRust}, 1},
		{"forbidden_layer_direction reads the crate module's layer", typeLayerDir,
			testEdge{From: nodeURLBuf, To: nodeFSPath, Kind: edgeKindDependsOn, Language: langRust}, 1},
		{"forbidden_layer_direction allows the inward crate::mod edge", typeLayerDir,
			testEdge{From: nodeFSPath, To: nodeURLBuf, Kind: edgeKindDependsOn, Language: langRust}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ruleSet, err := rules.New(rustCrateRuleConfig(tc.ruleType))
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			findings := ruleSet[0].Check(makeGraph([]testEdge{tc.edge}), rules.Evidence{})
			if len(findings) != tc.want {
				t.Fatalf("got %d findings, want %d: %+v", len(findings), tc.want, findings)
			}
		})
	}
}
