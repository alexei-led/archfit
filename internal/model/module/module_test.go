package module_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/model/module"
)

const kernelModule = "kernel"

func TestModuleFor_MostSpecificWins(t *testing.T) {
	mm := module.BuildMap(map[string]module.ModuleDef{
		"catchall":   {Paths: []string{"internal/**"}},
		kernelModule: {Paths: []string{"internal/model/**"}},
	})

	tests := []struct {
		path   string
		want   string
		wantOK bool
	}{
		{"internal/model/graph/graph.go", kernelModule, true},
		{"internal/config/config.go", "catchall", true},
		{"cmd/main.go", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := mm.ModuleFor(tt.path)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("ModuleFor(%q) = (%q, %v), want (%q, %v)", tt.path, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestValidRole(t *testing.T) {
	tests := []struct {
		role module.Role
		want bool
	}{
		{"", true}, // absent role is allowed
		{module.RoleCompositionRoot, true},
		{module.RoleGenerated, true},
		{"banana", false},
	}
	for _, tt := range tests {
		if got := module.ValidRole(tt.role); got != tt.want {
			t.Errorf("ValidRole(%q) = %v, want %v", tt.role, got, tt.want)
		}
	}
}

func TestIsModuleRoot(t *testing.T) {
	mm := module.BuildMap(map[string]module.ModuleDef{
		kernelModule: {Paths: []string{"internal/model/**"}},
	})
	if !mm.IsModuleRoot("internal/model") {
		t.Error("IsModuleRoot(internal/model) = false, want true")
	}
	if mm.IsModuleRoot("internal/model/graph") {
		t.Error("IsModuleRoot(internal/model/graph) = true, want false (nested dir)")
	}
}
