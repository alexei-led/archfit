package view

import (
	"testing"

	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/module"
)

const (
	policyTestModuleCore = "core"
	policyTestLayerModel = "model"
	policyTestHigh       = "high"
	policyTestPairKey    = "a\x00b"
	policyTestCorePath   = "internal/core/**"
	policyTestVendorPath = "vendor/**"
)

func TestClassifyConfigStaticPolicyProjectsStaticFieldsOnly(t *testing.T) {
	modules := map[string]module.ModuleDef{
		policyTestModuleCore: {Paths: []string{policyTestCorePath}, Owner: "team-a"},
	}
	cfg := ClassifyConfig{
		Modules:                   modules,
		Layers:                    []string{policyTestLayerModel, policyTestModuleCore},
		ModuleMap:                 module.BuildMap(modules),
		BCAdvisoryMinSeverity:     policyTestHigh,
		ExplicitOwners:            map[string]bool{policyTestModuleCore: true},
		VolatilityCascadeEnabled:  true,
		ExternalSystems:           map[string]ExternalSystemDef{"vendor": {Targets: []string{policyTestVendorPath}}},
		DuplicatedKnowledgePolicy: DuplicatedKnowledgePolicyAdvisory,
		ApprovedLabels:            map[string]string{policyTestPairKey: "contract"},
		LLMLabels:                 map[string]string{policyTestPairKey: "model"},
		LLMLabelConfidence:        map[string]string{policyTestPairKey: policyTestHigh},
		CrossModuleClonePairs:     map[string]struct{}{"[a]\x00[b]": {}},
		CloneEvidence:             map[string][]graph.Location{"[a]\x00[b]": {{File: "x.go", Line: 1}}},
	}

	got := cfg.StaticPolicy()

	if len(got.Modules) != 1 || got.Modules[policyTestModuleCore].Owner != "team-a" {
		t.Fatalf("StaticPolicy modules not projected: %+v", got.Modules)
	}
	if len(got.Layers) != 2 || got.Layers[0] != policyTestLayerModel {
		t.Fatalf("StaticPolicy layers not projected: %+v", got.Layers)
	}
	if got.BCAdvisoryMinSeverity != policyTestHigh {
		t.Fatalf("StaticPolicy BCAdvisoryMinSeverity = %q, want %q", got.BCAdvisoryMinSeverity, policyTestHigh)
	}
	if !got.ExplicitOwners[policyTestModuleCore] {
		t.Fatalf("StaticPolicy ExplicitOwners not projected: %+v", got.ExplicitOwners)
	}
	if !got.VolatilityCascadeEnabled {
		t.Fatal("StaticPolicy VolatilityCascadeEnabled not projected")
	}
	if len(got.ExternalSystems) != 1 {
		t.Fatalf("StaticPolicy ExternalSystems not projected: %+v", got.ExternalSystems)
	}
	if got.DuplicatedKnowledgePolicy != DuplicatedKnowledgePolicyAdvisory {
		t.Fatalf("StaticPolicy DuplicatedKnowledgePolicy = %q, want advisory", got.DuplicatedKnowledgePolicy)
	}
	if _, ok := any(got).(PolicyConfig); !ok {
		t.Fatalf("StaticPolicy returned %T, want PolicyConfig", got)
	}
}

func TestClassifyConfigStaticPolicyDoesNotAliasMutableState(t *testing.T) {
	modules := map[string]module.ModuleDef{
		policyTestModuleCore: {Paths: []string{policyTestCorePath}, Public: []string{"internal/core/api/**"}},
	}
	cfg := ClassifyConfig{
		Modules:         modules,
		Layers:          []string{policyTestLayerModel, policyTestModuleCore},
		ModuleMap:       module.BuildMap(modules),
		ExplicitOwners:  map[string]bool{policyTestModuleCore: true},
		ExternalSystems: map[string]ExternalSystemDef{"vendor": {Targets: []string{policyTestVendorPath}}},
	}
	got := cfg.StaticPolicy()

	modules[policyTestModuleCore] = module.ModuleDef{Paths: []string{"changed/**"}}
	cfg.Layers[0] = "changed"
	cfg.ExplicitOwners[policyTestModuleCore] = false
	vendor := cfg.ExternalSystems["vendor"]
	vendor.Targets[0] = "changed/**"
	cfg.ExternalSystems["vendor"] = vendor

	if got.Modules[policyTestModuleCore].Paths[0] != policyTestCorePath {
		t.Fatalf("policy modules alias source: %+v", got.Modules)
	}
	if got.Layers[0] != policyTestLayerModel || !got.ExplicitOwners[policyTestModuleCore] {
		t.Fatalf("policy slices/maps alias source: layers=%v owners=%v", got.Layers, got.ExplicitOwners)
	}
	if got.ExternalSystems["vendor"].Targets[0] != policyTestVendorPath {
		t.Fatalf("policy external targets alias source: %+v", got.ExternalSystems)
	}
	if moduleName, ok := got.ModuleMap.ModuleFor("internal/core/file.go"); !ok || moduleName != policyTestModuleCore {
		t.Fatalf("policy module map did not rebuild from copied modules: module=%q ok=%t", moduleName, ok)
	}
}
