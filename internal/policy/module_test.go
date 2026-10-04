package policy

import (
	"testing"
)

const kernelModule = "kernel"

func TestModuleFor_MostSpecificWins(t *testing.T) {
	mm := BuildModuleMap(map[string]ModuleDef{
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
		role Role
		want bool
	}{
		{"", true}, // absent role is allowed
		{RoleCompositionRoot, true},
		{RoleGenerated, true},
		{"banana", false},
	}
	for _, tt := range tests {
		if got := ValidRole(tt.role); got != tt.want {
			t.Errorf("ValidRole(%q) = %v, want %v", tt.role, got, tt.want)
		}
	}
}

func TestIsModuleRoot(t *testing.T) {
	mm := BuildModuleMap(map[string]ModuleDef{
		kernelModule: {Paths: []string{"internal/model/**"}},
	})
	if !mm.IsModuleRoot("internal/model") {
		t.Error("IsModuleRoot(internal/model) = false, want true")
	}
	if mm.IsModuleRoot("internal/model/graph") {
		t.Error("IsModuleRoot(internal/model/graph) = true, want false (nested dir)")
	}
}

func TestMatchesInternal_DeclaredSurfacePrecedence(t *testing.T) {
	const (
		billingAll  = "internal/billing/**"
		billingAPI  = "internal/billing/api"
		pyDomain    = "myapp.domain"
		pyDomainAll = "myapp.domain.**"
		rustLedger  = "shop::billing::ledger"
	)
	mm := BuildModuleMap(map[string]ModuleDef{
		"billing": {
			Paths:    []string{billingAll},
			Public:   []string{billingAPI},
			Internal: []string{billingAll},
		},
		"shipping": {Paths: []string{"internal/shipping/**"}},
		"ledger": {
			// More specific than billing, and its public: glob names a path it
			// does not own: that declaration cannot open billing's surface.
			Paths:  []string{"internal/billing/ledger/**"},
			Public: []string{"internal/billing/domain"},
		},
		"py": {
			Paths:    []string{pyDomain, pyDomainAll},
			Public:   []string{pyDomain},
			Internal: []string{pyDomainAll},
		},
		"rs": {
			Paths:    []string{"shop::billing", "shop::billing::**"},
			Internal: []string{rustLedger},
		},
	})

	tests := []struct {
		name         string
		path         string
		wantInternal bool
		wantGlob     string
	}{
		{"owner public glob exempts", billingAPI, false, billingAPI},
		{"owner internal glob marks internal", "internal/billing/domain/store", true, billingAll},
		{"another module's public glob cannot open a surface", "internal/billing/domain", true, billingAll},
		{"internal glob of a less specific module still applies", "internal/billing/ledger/book", true, billingAll},
		{"no declaration is undecided", "internal/shipping/api", false, ""},
		{"unowned path is undecided", "cmd/server", false, ""},
		{"python dotted public wins", pyDomain, false, pyDomain},
		{"python dotted internal", "myapp.domain.store", true, pyDomainAll},
		{"rust crate::mod internal", rustLedger, true, rustLedger},
		{"rust crate::mod undeclared", "shop::billing::api", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			internal, glob := mm.MatchesInternal(tt.path)
			if internal != tt.wantInternal || glob != tt.wantGlob {
				t.Errorf("MatchesInternal(%q) = (%v, %q), want (%v, %q)", tt.path, internal, glob, tt.wantInternal, tt.wantGlob)
			}
		})
	}
}
