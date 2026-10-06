package policy

import (
	"slices"
	"testing"
)

const (
	kernelModule   = "kernel"
	catchallModule = "catchall"
	kernelGlob     = "internal/model/**"

	modAllowBilling  = "billing"
	modAllowShipping = "shipping"
	modAllowOpen     = "open"
	modAllowSealed   = "sealed"
	modAllowAbsent   = "absent"
	modBillingAPI    = "billing-api"
	layerDomainName  = "domain"
)

func TestModuleFor_MostSpecificWins(t *testing.T) {
	mm := BuildModuleMap(map[string]ModuleDef{
		catchallModule: {Paths: []string{"internal/**"}},
		kernelModule:   {Paths: []string{kernelGlob}},
	})

	tests := []struct {
		path   string
		want   string
		wantOK bool
	}{
		{"internal/model/graph/graph.go", kernelModule, true},
		{"internal/config/config.go", catchallModule, true},
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

func TestOwnershipTie(t *testing.T) {
	mm := BuildModuleMap(map[string]ModuleDef{
		catchallModule: {Paths: []string{"internal/**"}},
		kernelModule:   {Paths: []string{kernelGlob}},
		"twin":         {Paths: []string{kernelGlob}},
		"exact":        {Paths: []string{"cmd/tool", "cmd/tool/**"}},
		"other":        {Paths: []string{"cmd/**"}},
	})
	for _, tc := range []struct {
		path string
		want []string
	}{
		{"internal/model/graph", []string{kernelModule, "twin"}},
		{"internal/config", nil},
		{"cmd/tool", nil},
		{"cmd/other", nil},
		{"docs", nil},
	} {
		if got := mm.OwnershipTie(tc.path); !slices.Equal(got, tc.want) {
			t.Errorf("OwnershipTie(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestUnknownModuleEnums(t *testing.T) {
	for _, tc := range []struct {
		def                   ModuleDef
		volatility, subdomain bool
	}{
		{def: ModuleDef{}},
		{def: ModuleDef{Volatility: "High", Subdomain: "Core"}},
		{def: ModuleDef{Volatility: "legacy", Subdomain: "generic"}},
		{def: ModuleDef{Volatility: "frozen", Subdomain: "supporting"}},
		{def: ModuleDef{Volatility: "hgih"}, volatility: true},
		{def: ModuleDef{Volatility: "critical", Subdomain: "cor"}, volatility: true, subdomain: true},
	} {
		if got := tc.def.UnknownVolatility(); got != tc.volatility {
			t.Errorf("%+v UnknownVolatility = %v, want %v", tc.def, got, tc.volatility)
		}
		if got := tc.def.UnknownSubdomain(); got != tc.subdomain {
			t.Errorf("%+v UnknownSubdomain = %v, want %v", tc.def, got, tc.subdomain)
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
		kernelModule: {Paths: []string{kernelGlob}},
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
			internal, glob := mm.MatchesInternal(tt.path, "")
			if internal != tt.wantInternal || glob != tt.wantGlob {
				t.Errorf("MatchesInternal(%q) = (%v, %q), want (%v, %q)", tt.path, internal, glob, tt.wantInternal, tt.wantGlob)
			}
		})
	}
}

func TestDeniedDependency(t *testing.T) {
	mm := BuildModuleMap(map[string]ModuleDef{
		modAllowBilling:  {VisibleTo: []string{modAllowShipping}},
		modAllowShipping: {DependsOn: []string{modAllowBilling}},
		kernelModule:     {DependsOn: []string{}},
		modAllowOpen:     {},
		modAllowSealed:   {VisibleTo: []string{}},
	})
	tests := []struct {
		from, to string
		want     []string
	}{
		{modAllowShipping, modAllowBilling, nil},
		{modAllowShipping, modAllowOpen, []string{allowlistDependsOn}},
		{modAllowOpen, modAllowBilling, []string{allowlistVisibleTo}},
		{kernelModule, modAllowBilling, []string{allowlistDependsOn, allowlistVisibleTo}},
		{kernelModule, modAllowOpen, []string{allowlistDependsOn}},
		{kernelModule, kernelModule, nil},
		{modAllowOpen, modAllowShipping, nil},
		{"", modAllowBilling, []string{allowlistVisibleTo}},
		{"", modAllowOpen, nil},
		{modAllowShipping, modAllowSealed, []string{allowlistDependsOn, allowlistVisibleTo}},
		{modAllowOpen, modAllowSealed, []string{allowlistVisibleTo}},
	}
	for _, tt := range tests {
		if got := mm.DeniedDependency(tt.from, tt.to); !slices.Equal(got, tt.want) {
			t.Errorf("DeniedDependency(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestDeclaresAllowlist(t *testing.T) {
	mm := BuildModuleMap(map[string]ModuleDef{
		modAllowAbsent: {},
		"empty-deps":   {DependsOn: []string{}},
		"empty-vis":    {VisibleTo: []string{}},
		"listed-deps":  {DependsOn: []string{modAllowAbsent}},
	})
	for name, want := range map[string]bool{modAllowAbsent: false, "empty-deps": true, "empty-vis": true, "listed-deps": true, "undeclared": false} {
		if got := mm.DeclaresAllowlist(name); got != want {
			t.Errorf("DeclaresAllowlist(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestSelectsModule(t *testing.T) {
	mm := BuildModuleMap(map[string]ModuleDef{
		modAllowBilling:  {Layer: layerDomainName, Role: RoleCore},
		modAllowShipping: {Layer: layerDomainName},
		modBillingAPI:    {Layer: "adapter", Role: RoleAdapter},
	})
	tests := []struct {
		selector string
		want     []string
	}{
		{modAllowBilling, []string{modAllowBilling}},
		{"billing*", []string{modAllowBilling, modBillingAPI}},
		{"layer:" + layerDomainName, []string{modAllowBilling, modAllowShipping}},
		{"role:adapter", []string{modBillingAPI}},
		{"role:core", []string{modAllowBilling}},
		{"layer:nowhere", nil},
		{"layer:", nil},
		{"role:", nil},
		{"warehouse", nil},
		{"**", []string{modAllowBilling, modBillingAPI, modAllowShipping}},
	}
	for _, tt := range tests {
		if got := mm.ModulesSelected(tt.selector); !slices.Equal(got, tt.want) {
			t.Errorf("ModulesSelected(%q) = %v, want %v", tt.selector, got, tt.want)
		}
	}
	if mm.SelectsModule("**", "undeclared") {
		t.Error("a selector selected an undeclared module")
	}
}

func TestValidModuleSelector(t *testing.T) {
	var mm ModuleMap
	for selector, want := range map[string]bool{
		modAllowBilling: true, "layer:" + layerDomainName: true, "role:core": true, "billing-*": true,
		"layer:": false, "role:": false, "": false, "billing[": false,
	} {
		if got := mm.ValidModuleSelector(selector); got != want {
			t.Errorf("ValidModuleSelector(%q) = %v, want %v", selector, got, want)
		}
	}
}

// TestDeniedDependencyReadsSelectorEntries pins that an allowlist entry is a
// module selector: a layer, a role, or a glob admits every module it selects.
func TestDeniedDependencyReadsSelectorEntries(t *testing.T) {
	mm := BuildModuleMap(map[string]ModuleDef{
		modAllowBilling:  {Layer: layerDomainName, VisibleTo: []string{"layer:" + policyTestApp, "role:composition_root"}},
		modAllowShipping: {Layer: policyTestApp, DependsOn: []string{"bill*"}},
		policyTestUnit:   {Role: RoleCompositionRoot},
		modAllowOpen:     {Layer: policyTestApp},
		modAllowSealed:   {Layer: "infra"},
	})
	tests := []struct {
		from, to string
		want     []string
	}{
		{modAllowShipping, modAllowBilling, nil},
		{policyTestUnit, modAllowBilling, nil},
		{modAllowOpen, modAllowBilling, nil},
		{modAllowSealed, modAllowBilling, []string{allowlistVisibleTo}},
		{modAllowShipping, modAllowOpen, []string{allowlistDependsOn}},
	}
	for _, tt := range tests {
		if got := mm.DeniedDependency(tt.from, tt.to); !slices.Equal(got, tt.want) {
			t.Errorf("DeniedDependency(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}
