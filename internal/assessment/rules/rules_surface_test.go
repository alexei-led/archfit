package rules_test

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/rules"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship"
)

// surfaceModules declares the API surfaces the internal-access rules decide
// from. billing pairs a public root package with an internal catch-all, the
// shape that works on a root go.mod; svcbilling sits under a nested go.mod
// (svc/), where the Go extractor marks every `svc/internal/...` import
// uses_internal; shipping declares no surface at all.
const (
	langTypeScript = "typescript"
	langPython     = "python"
	langRust       = "rust"

	globBillingAll   = "internal/billing/**"
	globPyDomainAll  = "myapp.domain.**"
	pkgPyDomain      = "myapp.domain"
	nodeBillingDomPk = "package:internal/billing/domain"
)

func surfaceModules() map[string]policy.ModuleDef {
	return map[string]policy.ModuleDef{
		"billing": {
			Paths:    []string{globBillingAll},
			Public:   []string{"internal/billing/api"},
			Internal: []string{globBillingAll},
		},
		"shipping":   {Paths: []string{"internal/shipping/**"}},
		"server":     {Paths: []string{"svc/cmd/**"}},
		"svcbilling": {Paths: []string{"svc/internal/billing/**"}, Public: []string{"svc/internal/billing/api"}},
		"web":        {Paths: []string{"packages/web/**"}},
		"core": {
			Paths:    []string{"packages/core/**"},
			Internal: []string{"packages/core/src/internal/**"},
		},
		"pyapi": {Paths: []string{"myapp.api", "myapp.api.**"}},
		"pydomain": {
			Paths:    []string{pkgPyDomain, globPyDomainAll},
			Public:   []string{pkgPyDomain},
			Internal: []string{globPyDomainAll},
		},
		"rsship": {Paths: []string{"shop::shipping", "shop::shipping::**"}},
		"rsbill": {
			Paths:    []string{"shop::billing", "shop::billing::**"},
			Internal: []string{"shop::billing::ledger"},
		},
	}
}

func surfaceRule(t *testing.T, ruleType string) rules.Rule {
	t.Helper()
	modules := surfaceModules()
	ruleSet, err := rules.New(policy.RuleConfig{
		Rules:     []policy.RuleDef{{ID: ruleIDNoInternalAccess, Type: ruleType}},
		ModuleMap: policy.BuildModuleMap(modules),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return ruleSet[0]
}

// TestInternalAccessRules_DecideFromDeclaredSurfaces pins the declared-surface
// precedence for both internal-access rules across the four node vocabularies:
// a public: glob of the target's module exempts, any internal: glob fires, and
// only an undeclared target falls back to the extractor's uses_internal kind.
func TestInternalAccessRules_DecideFromDeclaredSurfaces(t *testing.T) {
	importLine := []relationship.Location{{File: "internal/shipping/domain/shipment.go", Line: 3}}
	tests := []struct {
		name     string
		edge     testEdge
		want     int
		wantGlob string
	}{
		{
			name: "go root go.mod bypass of a declared internal package fires",
			edge: testEdge{From: "file:internal/shipping/domain/shipment.go", To: nodeBillingDomPk,
				Kind: edgeKindImports, Locations: importLine},
			want: 1, wantGlob: globBillingAll,
		},
		{
			name: "go import through the declared public package does not fire",
			edge: testEdge{From: "file:internal/shipping/api/api.go", To: "package:internal/billing/api", Kind: edgeKindImports},
			want: 0,
		},
		{
			name: "go same-module access to its own internal package does not fire",
			edge: testEdge{From: "file:internal/billing/api/api.go", To: nodeBillingDomPk, Kind: edgeKindImports},
			want: 0,
		},
		{
			name: "go undeclared target with a plain import does not fire",
			edge: testEdge{From: "file:internal/billing/api/api.go", To: "package:internal/shipping/domain", Kind: edgeKindImports},
			want: 0,
		},
		{
			name: "go nested go.mod import through declared public wins over the internal segment",
			edge: testEdge{From: "file:svc/cmd/server/main.go", To: "package:svc/internal/billing/api", Kind: edgeKindUsesInternal},
			want: 0,
		},
		{
			name: "go undeclared target keeps the extractor's uses_internal kind",
			edge: testEdge{From: "file:svc/cmd/server/main.go", To: "package:svc/internal/shipping/api", Kind: edgeKindUsesInternal},
			want: 1,
		},
		{
			name: "go unowned importer into a declared internal package fires",
			edge: testEdge{From: "file:tools/gen/main.go", To: nodeBillingDomPk, Kind: edgeKindImports},
			want: 1, wantGlob: globBillingAll,
		},
		{
			name: "non-dependency edge into a declared internal package does not fire",
			edge: testEdge{From: "file:internal/shipping/domain/shipment.go", To: nodeBillingDomPk, Kind: "exposes"},
			want: 0,
		},
		{
			name: "typescript file edge into a declared internal file fires",
			edge: testEdge{From: "file:packages/web/src/app.ts", To: "file:packages/core/src/internal/cache.ts",
				Kind: edgeKindImports, Language: langTypeScript},
			want: 1, wantGlob: "packages/core/src/internal/**",
		},
		{
			name: "python dotted edge into a declared internal module fires",
			edge: testEdge{From: "module:myapp.api.views", To: "module:myapp.domain.store", Kind: edgeKindImports, Language: langPython},
			want: 1, wantGlob: globPyDomainAll,
		},
		{
			name: "python dotted edge into the declared public package does not fire",
			edge: testEdge{From: "module:myapp.api.views", To: "module:myapp.domain", Kind: edgeKindImports, Language: langPython},
			want: 0,
		},
		{
			name: "rust crate::mod edge into a declared internal module fires",
			edge: testEdge{From: "package:shop::shipping", To: "package:shop::billing::ledger", Kind: edgeKindImports, Language: langRust},
			want: 1, wantGlob: "shop::billing::ledger",
		},
	}
	for _, ruleType := range []string{typePublicAPIOnly, typeInternalAPIAccess} {
		r := surfaceRule(t, ruleType)
		for _, tc := range tests {
			t.Run(ruleType+"/"+tc.name, func(t *testing.T) {
				findings := r.Check(makeGraph([]testEdge{tc.edge}), rules.Evidence{})
				if len(findings) != tc.want {
					t.Fatalf("got %d findings, want %d: %+v", len(findings), tc.want, findings)
				}
				if tc.want == 0 {
					return
				}
				f := findings[0]
				if got := f.MatchedBy["internal_glob"]; got != tc.wantGlob {
					t.Errorf("matched_by.internal_glob = %q, want %q", got, tc.wantGlob)
				}
				if f.Edge.To.Path != relationship.NodePath(tc.edge.To) {
					t.Errorf("edge.to.path = %q, want %q", f.Edge.To.Path, relationship.NodePath(tc.edge.To))
				}
				if len(tc.edge.Locations) > 0 && (len(f.Locations) != 1 || f.Locations[0] != tc.edge.Locations[0]) {
					t.Errorf("locations = %+v, want %+v", f.Locations, tc.edge.Locations)
				}
			})
		}
	}
}

// TestInternalAccessRules_KeepExistingFindingIDs pins the fingerprints a
// pre-existing uses_internal finding carried before declared surfaces decided:
// the IDs were captured from the unmodified engine on the coupled fixture
// (pkg/a/a.go importing pkg/b/internal/impl, b declaring internal:
// pkg/b/internal/**). Deciding from declarations must never re-key them.
func TestInternalAccessRules_KeepExistingFindingIDs(t *testing.T) {
	modules := map[string]policy.ModuleDef{
		"a": {Paths: []string{"pkg/a/**"}},
		"b": {Paths: []string{"pkg/b/**"}, Internal: []string{"pkg/b/internal/**"}},
	}
	edge := testEdge{From: "file:pkg/a/a.go", To: "package:pkg/b/internal/impl", Kind: edgeKindUsesInternal,
		Locations: []relationship.Location{{File: "pkg/a/a.go", Line: 3}}}
	tests := []struct {
		id, ruleType, wantID string
	}{
		{id: "api_only", ruleType: typePublicAPIOnly, wantID: "f810d2b178af5bf73b38ef270b476e1a"},
		{id: "internal_access", ruleType: typeInternalAPIAccess, wantID: "740c5c4d955ce6e174ae4bf7d673a8a8"},
	}
	for _, tc := range tests {
		t.Run(tc.ruleType, func(t *testing.T) {
			ruleSet, err := rules.New(policy.RuleConfig{
				Rules:     []policy.RuleDef{{ID: tc.id, Type: tc.ruleType}},
				ModuleMap: policy.BuildModuleMap(modules),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			findings := ruleSet[0].Check(makeGraph([]testEdge{edge}), rules.Evidence{})
			if len(findings) != 1 {
				t.Fatalf("got %d findings, want 1: %+v", len(findings), findings)
			}
			if findings[0].ID != tc.wantID {
				t.Errorf("finding ID = %q, want the pre-change %q", findings[0].ID, tc.wantID)
			}
		})
	}
}
