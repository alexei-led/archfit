package rules_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/module"
	"github.com/alexei-led/archfit/internal/rules"
	"github.com/alexei-led/archfit/internal/view"
)

// Node ID constants reused across multiple test cases.
const (
	nodeAFoo      = "file:services/a/foo.go"
	nodeBBar      = "file:services/b/bar.go"
	nodeCFoo      = "file:services/c/foo.go"
	nodeABaz      = "file:services/a/baz.go"
	nodeBQux      = "file:services/b/qux.go"
	nodeBIntBar   = "file:services/b/internal/bar.go"
	nodeBAPI      = "file:services/b/api.go"
	nodeDIntY     = "file:services/d/internal/y.go"
	nodeCFoo2     = "file:services/c/foo.go"
	nodeInfraRep  = "file:services/infra/repo.go"
	nodeDomMod    = "file:services/domain/model.go"
	nodeAppHnd    = "file:services/app/handler.go"
	nodeAppSvc    = "file:services/app/service.go"
	nodeDomA      = "file:services/domain/a.go"
	nodeDomB      = "file:services/domain/b.go"
	nodeExtFoo    = "file:external/foo.go"
	nodeDomainFoo = "file:domain/domain.go"
)

// Layer name constants.
const (
	layerDomain         = "domain"
	layerApplication    = "application"
	layerInfrastructure = "infrastructure"
)

// Rule type and ID constants for gate semantics tests.
const (
	typeForbiddenDependency = "forbidden_dependency"
	typePublicAPIMax        = "public_api_max"
	typePublicAPIChange     = "public_api_change"
	kindGate                = "gate"
	kindAdvisory            = "advisory"
	gateWarn                = "warn"
	ruleIDNoDep             = "no-dep"
	ruleIDNoInternalAccess  = "no-internal-access"
	typePublicAPIOnly       = "public_api_only"
	typeInternalAPIAccess   = "internal_api_access"
	globServicesA           = "services/a/**"
	globServicesB           = "services/b/**"
	// publicAPIMax / publicAPIChange test constants
	fileDomainA     = "domain/a.go"
	fileDomainB     = "domain/b.go"
	moduleInfra     = "infra"
	kindFunction    = "function"
	kindStruct      = "struct"
	nameFuncA       = "FuncA"
	nameInternal    = "internal"
	nameMain        = "Main"
	fileCmd         = "cmd/main.go"
	kindStructField = "struct_field"
	nameRepo        = "Repo"
	fileInfraRepo   = "infra/repo.go"
	pathDomainGlob  = "domain/**"
	pathInfraGlob   = "infra/**"
	// cycle / new_cross_module_dependency test constants
	typeCycle                    = "cycle"
	typeNewCrossModuleDependency = "new_cross_module_dependency"
	ruleIDCycle                  = "no-cycles"
	ruleIDCrossModule            = "cross-module"
	moduleA                      = "moduleA"
	moduleB                      = "moduleB"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func makeGraph(edges []graph.Edge) *graph.Graph {
	var nodes []graph.Node
	seen := make(map[string]bool)
	for _, e := range edges {
		for _, id := range []string{e.From, e.To} {
			if !seen[id] {
				seen[id] = true
				kind, path, _ := strings.Cut(id, ":")
				nodes = append(nodes, graph.Node{Kind: graph.NodeKind(kind), Path: path})
			}
		}
	}
	return graph.Build([]graph.Facts{{Nodes: nodes, Edges: edges, Language: "go"}})
}

// ---------------------------------------------------------------------------
// ForbiddenDependency
// ---------------------------------------------------------------------------

func TestForbiddenDependency(t *testing.T) {
	cfg := view.RuleConfig{
		Rules: []view.RuleDef{
			{ID: "no-a-to-b", Type: typeForbiddenDependency, From: globServicesA, To: globServicesB},
		},
	}
	ruleSet, err := rules.New(cfg)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if len(ruleSet) != 1 {
		t.Fatalf("New: got %d rules, want 1", len(ruleSet))
	}
	r := ruleSet[0]

	tests := []struct {
		name      string
		edges     []graph.Edge
		wantCount int
	}{
		{
			name: "matching_edge_produces_finding",
			edges: []graph.Edge{
				{From: nodeAFoo, To: nodeBBar, Kind: graph.EdgeKindImports},
			},
			wantCount: 1,
		},
		{
			name: "non_matching_from_no_finding",
			edges: []graph.Edge{
				{From: nodeCFoo, To: nodeBBar, Kind: graph.EdgeKindImports},
			},
			wantCount: 0,
		},
		{
			name: "non_matching_to_no_finding",
			edges: []graph.Edge{
				{From: nodeAFoo, To: nodeCFoo2, Kind: graph.EdgeKindImports},
			},
			wantCount: 0,
		},
		{
			name: "multiple_matching_edges",
			edges: []graph.Edge{
				{From: nodeAFoo, To: nodeBBar, Kind: graph.EdgeKindImports},
				{From: nodeABaz, To: nodeBQux, Kind: graph.EdgeKindImports},
			},
			wantCount: 2,
		},
	}

	ev := rules.Evidence{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := makeGraph(tc.edges)
			findings := r.Check(g, ev)
			if len(findings) != tc.wantCount {
				t.Fatalf("Check: got %d findings, want %d", len(findings), tc.wantCount)
			}
			for _, f := range findings {
				if f.RuleID != "no-a-to-b" {
					t.Errorf("finding.RuleID = %q, want %q", f.RuleID, "no-a-to-b")
				}
				if f.MatchedBy["from_glob"] != globServicesA {
					t.Errorf("matched_by.from_glob = %q, want %q", f.MatchedBy["from_glob"], globServicesA)
				}
				if f.MatchedBy["to_glob"] != globServicesB {
					t.Errorf("matched_by.to_glob = %q, want %q", f.MatchedBy["to_glob"], globServicesB)
				}
				if f.Why == "" {
					t.Error("finding.Why is empty")
				}
				if f.Constraint == "" {
					t.Error("finding.Constraint is empty")
				}
				if f.Edge.From.Path == "" {
					t.Error("finding.Edge.From.Path is empty")
				}
				if f.Edge.To.Path == "" {
					t.Error("finding.Edge.To.Path is empty")
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// PublicAPIOnly
// ---------------------------------------------------------------------------

func TestPublicAPIOnly(t *testing.T) {
	cfg := view.RuleConfig{
		Rules: []view.RuleDef{
			{ID: ruleIDNoInternalAccess, Type: typePublicAPIOnly},
		},
	}
	ruleSet, err := rules.New(cfg)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if len(ruleSet) != 1 {
		t.Fatalf("New: got %d rules, want 1", len(ruleSet))
	}
	r := ruleSet[0]

	tests := []struct {
		name      string
		edges     []graph.Edge
		wantCount int
	}{
		{
			name: "uses_internal_edge_produces_finding",
			edges: []graph.Edge{
				{From: nodeAFoo, To: nodeBIntBar, Kind: graph.EdgeKindUsesInternal},
			},
			wantCount: 1,
		},
		{
			name: "imports_edge_no_finding",
			edges: []graph.Edge{
				{From: nodeAFoo, To: nodeBAPI, Kind: graph.EdgeKindImports},
			},
			wantCount: 0,
		},
		{
			name: "depends_on_edge_no_finding",
			edges: []graph.Edge{
				{From: nodeAFoo, To: nodeBAPI, Kind: graph.EdgeKindDependsOn},
			},
			wantCount: 0,
		},
		{
			name: "multiple_uses_internal",
			edges: []graph.Edge{
				{From: nodeAFoo, To: nodeBIntBar, Kind: graph.EdgeKindUsesInternal},
				{From: nodeCFoo, To: nodeDIntY, Kind: graph.EdgeKindUsesInternal},
			},
			wantCount: 2,
		},
	}

	ev := rules.Evidence{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := makeGraph(tc.edges)
			findings := r.Check(g, ev)
			if len(findings) != tc.wantCount {
				t.Fatalf("Check: got %d findings, want %d", len(findings), tc.wantCount)
			}
			for _, f := range findings {
				if f.RuleID != ruleIDNoInternalAccess {
					t.Errorf("finding.RuleID = %q, want %q", f.RuleID, ruleIDNoInternalAccess)
				}
				if f.MatchedBy["edge_kind"] != "uses_internal" {
					t.Errorf("matched_by.edge_kind = %q, want %q", f.MatchedBy["edge_kind"], "uses_internal")
				}
				if f.MatchedBy["to_path"] == "" {
					t.Error("matched_by.to_path is empty")
				}
				if f.Why == "" {
					t.Error("finding.Why is empty")
				}
				if f.Constraint == "" {
					t.Error("finding.Constraint is empty")
				}
				if f.Edge.To.Path == "" {
					t.Error("finding.Edge.To.Path is empty")
				}
			}
		})
	}
}

// TestPublicAPIOnly_ModuleMap covers V5: publicAPIOnly must not fire when the
// module map says both endpoints of a uses_internal edge belong to the same
// module (idiomatic self-access, e.g. domain importing domain/internal), but
// must still fire on genuine cross-module internal access.
func TestPublicAPIOnly_ModuleMap(t *testing.T) {
	const moduleDomain = "domain"
	cfg := config.Config{
		Version: 1,
		Modules: map[string]module.ModuleDef{
			moduleDomain: {Paths: []string{"domain/**"}},
			moduleA:      {Paths: []string{"services/a/**"}},
			moduleB:      {Paths: []string{"services/b/**"}},
		},
		Rules: []view.RuleDef{
			{ID: ruleIDNoInternalAccess, Type: typePublicAPIOnly},
		},
	}
	rc := cfg.ForRules()
	ruleSet, err := rules.New(rc)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	r := ruleSet[0]
	ev := rules.Evidence{}

	t.Run("same_module_self_access_no_finding", func(t *testing.T) {
		g := makeGraph([]graph.Edge{
			{From: nodeDomainFoo, To: "file:domain/internal/helper.go", Kind: graph.EdgeKindUsesInternal},
		})
		findings := r.Check(g, ev)
		if len(findings) != 0 {
			t.Fatalf("same-module self-access: got %d findings, want 0: %+v", len(findings), findings)
		}
	})

	t.Run("cross_module_internal_access_still_fires", func(t *testing.T) {
		g := makeGraph([]graph.Edge{
			{From: nodeAFoo, To: nodeBIntBar, Kind: graph.EdgeKindUsesInternal},
		})
		findings := r.Check(g, ev)
		if len(findings) != 1 {
			t.Fatalf("cross-module access: got %d findings, want 1", len(findings))
		}
		f := findings[0]
		if !strings.Contains(f.Why, "Cross-module") {
			t.Errorf("Why = %q, want it to mention Cross-module for a genuine cross-module edge", f.Why)
		}
		if !strings.Contains(f.Why, moduleA) || !strings.Contains(f.Why, moduleB) {
			t.Errorf("Why = %q, want it to name both modules %q and %q", f.Why, moduleA, moduleB)
		}
	})

	t.Run("unresolved_endpoint_why_does_not_claim_cross_module", func(t *testing.T) {
		// Neither endpoint is covered by the module map — the rule can't tell
		// same-module from cross-module, so it fires (module-blind fallback) but
		// must not falsely claim "Cross-module" it cannot substantiate.
		g := makeGraph([]graph.Edge{
			{From: nodeCFoo, To: nodeDIntY, Kind: graph.EdgeKindUsesInternal},
		})
		findings := r.Check(g, ev)
		if len(findings) != 1 {
			t.Fatalf("unresolved endpoints: got %d findings, want 1", len(findings))
		}
		if strings.Contains(findings[0].Why, "Cross-module") {
			t.Errorf("Why = %q, must not claim Cross-module when the module map can't confirm it", findings[0].Why)
		}
	})

	t.Run("mixed_resolution_one_endpoint_unresolved_still_fires", func(t *testing.T) {
		// fromPath resolves to "domain" but toPath is outside any configured
		// module — sameModule requires BOTH endpoints resolved, so this must
		// not be treated as same-module and must still fire.
		g := makeGraph([]graph.Edge{
			{From: nodeDomainFoo, To: "file:services/x/internal/other.go", Kind: graph.EdgeKindUsesInternal},
		})
		findings := r.Check(g, ev)
		if len(findings) != 1 {
			t.Fatalf("mixed resolution: got %d findings, want 1: %+v", len(findings), findings)
		}
	})
}

// TestInternalAPIAccess_ModuleMap mirrors TestPublicAPIOnly_ModuleMap for the
// internal_api_access rule: same-module self-access must not fire (the V5
// module-map skip applies to both uses_internal rules), cross-module and
// module-blind edges still fire.
func TestInternalAPIAccess_ModuleMap(t *testing.T) {
	cfg := config.Config{
		Version: 1,
		Modules: map[string]module.ModuleDef{
			"domain": {Paths: []string{pathDomainGlob}},
			moduleA:  {Paths: []string{globServicesA}},
			moduleB:  {Paths: []string{globServicesB}},
		},
		Rules: []view.RuleDef{
			{ID: ruleIDNoInternalAccess, Type: typeInternalAPIAccess},
		},
	}
	ruleSet, err := rules.New(cfg.ForRules())
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	r := ruleSet[0]
	ev := rules.Evidence{}

	tests := []struct {
		name string
		edge graph.Edge
		want int
	}{
		{
			name: "same_module_self_access_no_finding",
			edge: graph.Edge{From: nodeDomainFoo, To: "file:domain/internal/helper.go", Kind: graph.EdgeKindUsesInternal},
			want: 0,
		},
		{
			name: "cross_module_internal_access_fires",
			edge: graph.Edge{From: nodeAFoo, To: nodeBIntBar, Kind: graph.EdgeKindUsesInternal},
			want: 1,
		},
		{
			name: "unresolved_endpoints_module_blind_fallback_fires",
			edge: graph.Edge{From: nodeCFoo, To: nodeDIntY, Kind: graph.EdgeKindUsesInternal},
			want: 1,
		},
		{
			// fromPath resolves to "domain" but toPath is outside any
			// configured module — sameModule requires BOTH endpoints
			// resolved, so this must still fire.
			name: "mixed_resolution_one_endpoint_unresolved_fires",
			edge: graph.Edge{From: nodeDomainFoo, To: "file:services/x/internal/other.go", Kind: graph.EdgeKindUsesInternal},
			want: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			findings := r.Check(makeGraph([]graph.Edge{tc.edge}), ev)
			if len(findings) != tc.want {
				t.Fatalf("got %d findings, want %d: %+v", len(findings), tc.want, findings)
			}
		})
	}
}

// TestPublicAPIOnly_PerLanguage locks publicAPIOnly.Check's EdgeKind gating
// only: Check never reads Edge.Language, so all three subtests below assert
// the exact same thing — a plain EdgeKindImports edge produces no finding —
// regardless of the language label attached to the edge. Per-language
// differences in WHEN graph.EdgeKindUsesInternal actually gets assigned (Go
// extractor: lexically, on any "/internal/" import path; TS/Python
// extractors: only when a module declares an `internal:` glob
// (matchesInternal); Rust extractor: never) live in the extractors, not in
// this rule or in this test.
func TestPublicAPIOnly_PerLanguage(t *testing.T) {
	cfg := view.RuleConfig{
		Rules: []view.RuleDef{
			{ID: ruleIDNoInternalAccess, Type: typePublicAPIOnly},
		},
	}
	ruleSet, err := rules.New(cfg)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	r := ruleSet[0]
	ev := rules.Evidence{}

	tests := []struct {
		name     string
		language string
	}{
		{name: "typescript", language: "typescript"},
		{name: "python", language: "python"},
		{name: "rust", language: "rust"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Plain import edge, as extractors emit when no internal: glob
			// matches (TS/Python) or unconditionally (Rust never sets uses_internal).
			g := makeGraph([]graph.Edge{
				{From: nodeAFoo, To: nodeBIntBar, Kind: graph.EdgeKindImports, Language: tc.language},
			})
			findings := r.Check(g, ev)
			if len(findings) != 0 {
				t.Fatalf("%s import edge: got %d findings, want 0 (rule is inert without uses_internal): %+v", tc.language, len(findings), findings)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CycleRule
// ---------------------------------------------------------------------------

// newCycleRule builds a single cycleRule (wrapped in the gate default) for
// the given gate string ("" = unset/default fail).
func newCycleRule(t *testing.T, gate string) rules.Rule {
	t.Helper()
	cfg := view.RuleConfig{
		Rules: []view.RuleDef{
			{ID: ruleIDCycle, Type: typeCycle, Gate: gate},
		},
	}
	ruleSet, err := rules.New(cfg)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	return ruleSet[0]
}

func TestCycleRule(t *testing.T) {
	ev := rules.Evidence{}
	// A -> B -> A: a 2-node strongly-connected component.
	cyclicEdges := []graph.Edge{
		{From: nodeAFoo, To: nodeBBar, Kind: graph.EdgeKindImports},
		{From: nodeBBar, To: nodeAFoo, Kind: graph.EdgeKindImports},
	}

	t.Run("cycle_fires_one_finding", func(t *testing.T) {
		r := newCycleRule(t, "")
		findings := r.Check(makeGraph(cyclicEdges), ev)
		if len(findings) != 1 {
			t.Fatalf("got %d findings, want 1: %+v", len(findings), findings)
		}
		f := findings[0]
		if f.RuleID != ruleIDCycle {
			t.Errorf("RuleID = %q, want %q", f.RuleID, ruleIDCycle)
		}
		if f.Why == "" {
			t.Error("finding.Why is empty")
		}
		if f.Constraint == "" {
			t.Error("finding.Constraint is empty")
		}
	})

	t.Run("no_cycle_no_finding", func(t *testing.T) {
		r := newCycleRule(t, "")
		g := makeGraph([]graph.Edge{{From: nodeAFoo, To: nodeBBar, Kind: graph.EdgeKindImports}})
		findings := r.Check(g, ev)
		if len(findings) != 0 {
			t.Fatalf("acyclic graph: got %d findings, want 0: %+v", len(findings), findings)
		}
	})

	t.Run("fingerprint_stable_across_checks", func(t *testing.T) {
		r := newCycleRule(t, "")
		g := makeGraph(cyclicEdges)
		first := r.Check(g, ev)
		second := r.Check(g, ev)
		if len(first) != 1 || len(second) != 1 {
			t.Fatalf("got %d and %d findings, want 1 and 1", len(first), len(second))
		}
		if first[0].ID != second[0].ID {
			t.Errorf("finding ID not stable across Check calls: %q vs %q", first[0].ID, second[0].ID)
		}
	})

	t.Run("gate_semantics", func(t *testing.T) {
		t.Run("off_skips_finding", func(t *testing.T) {
			r := newCycleRule(t, "off")
			findings := r.Check(makeGraph(cyclicEdges), ev)
			if len(findings) != 0 {
				t.Fatalf("gate:off: got %d findings, want 0", len(findings))
			}
		})
		t.Run("warn_produces_advisory", func(t *testing.T) {
			r := newCycleRule(t, gateWarn)
			findings := r.Check(makeGraph(cyclicEdges), ev)
			if len(findings) != 1 {
				t.Fatalf("gate:warn: got %d findings, want 1", len(findings))
			}
			if findings[0].Kind != kindAdvisory {
				t.Errorf("gate:warn finding.Kind = %q, want advisory", findings[0].Kind)
			}
		})
		t.Run("fail_produces_gate_finding", func(t *testing.T) {
			r := newCycleRule(t, "fail")
			findings := r.Check(makeGraph(cyclicEdges), ev)
			if len(findings) != 1 {
				t.Fatalf("gate:fail: got %d findings, want 1", len(findings))
			}
			if findings[0].Kind != kindGate {
				t.Errorf("gate:fail finding.Kind = %q, want gate", findings[0].Kind)
			}
		})
		t.Run("unset_gate_produces_gate_finding", func(t *testing.T) {
			r := newCycleRule(t, "")
			findings := r.Check(makeGraph(cyclicEdges), ev)
			if len(findings) != 1 {
				t.Fatalf("gate unset: got %d findings, want 1", len(findings))
			}
			if findings[0].Kind != kindGate {
				t.Errorf("gate unset finding.Kind = %q, want gate", findings[0].Kind)
			}
		})
	})
}

// ---------------------------------------------------------------------------
// NewCrossModuleDependency
// ---------------------------------------------------------------------------

func TestCrossModuleDependency(t *testing.T) {
	cfg := config.Config{
		Version: 1,
		Modules: map[string]module.ModuleDef{
			moduleA: {Paths: []string{globServicesA}},
			moduleB: {Paths: []string{globServicesB}},
		},
		Rules: []view.RuleDef{
			{ID: ruleIDCrossModule, Type: typeNewCrossModuleDependency},
		},
	}
	ruleSet, err := rules.New(cfg.ForRules())
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	r := ruleSet[0]
	ev := rules.Evidence{}

	t.Run("cross_module_edge_fires", func(t *testing.T) {
		g := makeGraph([]graph.Edge{{From: nodeAFoo, To: nodeBBar, Kind: graph.EdgeKindImports}})
		findings := r.Check(g, ev)
		if len(findings) != 1 {
			t.Fatalf("got %d findings, want 1: %+v", len(findings), findings)
		}
		f := findings[0]
		if f.MatchedBy["from_module"] != moduleA || f.MatchedBy["to_module"] != moduleB {
			t.Errorf("matched_by = %+v, want from_module=%s to_module=%s", f.MatchedBy, moduleA, moduleB)
		}
		if f.Why == "" {
			t.Error("finding.Why is empty")
		}
	})

	t.Run("same_module_edge_does_not_fire", func(t *testing.T) {
		g := makeGraph([]graph.Edge{{From: nodeAFoo, To: nodeABaz, Kind: graph.EdgeKindImports}})
		findings := r.Check(g, ev)
		if len(findings) != 0 {
			t.Fatalf("same-module edge: got %d findings, want 0: %+v", len(findings), findings)
		}
	})

	t.Run("unresolved_endpoint_is_skipped", func(t *testing.T) {
		// Neither endpoint is covered by the module map. Unlike sameModule's
		// module-blind fallback (which fires when it can't confirm same-module),
		// newCrossModuleDependency requires BOTH endpoints resolved and skips
		// the edge entirely when either is unowned.
		g := makeGraph([]graph.Edge{{From: nodeCFoo, To: nodeDIntY, Kind: graph.EdgeKindImports}})
		findings := r.Check(g, ev)
		if len(findings) != 0 {
			t.Fatalf("unresolved endpoint: got %d findings, want 0 (skipped, not module-blind fallback): %+v", len(findings), findings)
		}
	})
}

// ---------------------------------------------------------------------------
// ForbiddenLayerDirection
// ---------------------------------------------------------------------------

func TestForbiddenLayerDirection(t *testing.T) {
	// layers: [domain(0), application(1), infrastructure(2)]
	// violation: fromRank > toRank (e.g. infrastructure→domain, infrastructure→application)
	cfg := config.Config{
		Version: 1,
		Layers:  []string{layerDomain, layerApplication, layerInfrastructure},
		Modules: map[string]module.ModuleDef{
			"domain": {Paths: []string{"services/domain/**"}, Layer: layerDomain},
			"app":    {Paths: []string{"services/app/**"}, Layer: layerApplication},
			"infra":  {Paths: []string{"services/infra/**"}, Layer: layerInfrastructure},
		},
		Rules: []view.RuleDef{
			{ID: "no-upward-deps", Type: "forbidden_layer_direction"},
		},
	}
	rc := cfg.ForRules()
	ruleSet, err := rules.New(rc)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if len(ruleSet) != 1 {
		t.Fatalf("New: got %d rules, want 1", len(ruleSet))
	}
	r := ruleSet[0]

	tests := []struct {
		name      string
		edges     []graph.Edge
		wantCount int
	}{
		{
			// infra(rank=2) → domain(rank=0): fromRank > toRank → allowed
			name: "infra_to_domain_allowed",
			edges: []graph.Edge{
				{From: nodeInfraRep, To: nodeDomMod, Kind: graph.EdgeKindImports},
			},
			wantCount: 0,
		},
		{
			// domain(rank=0) → infra(rank=2): fromRank < toRank → violation
			name: "domain_to_infra_is_violation",
			edges: []graph.Edge{
				{From: nodeDomMod, To: nodeInfraRep, Kind: graph.EdgeKindImports},
			},
			wantCount: 1,
		},
		{
			// app(rank=1) → domain(rank=0): fromRank > toRank → allowed
			name: "app_to_domain_allowed",
			edges: []graph.Edge{
				{From: nodeAppHnd, To: nodeDomMod, Kind: graph.EdgeKindImports},
			},
			wantCount: 0,
		},
		{
			// domain(rank=0) → app(rank=1): fromRank < toRank → violation
			name: "domain_to_app_is_violation",
			edges: []graph.Edge{
				{From: nodeDomMod, To: nodeAppSvc, Kind: graph.EdgeKindImports},
			},
			wantCount: 1,
		},
		{
			name: "same_layer_no_violation",
			edges: []graph.Edge{
				{From: nodeDomA, To: nodeDomB, Kind: graph.EdgeKindImports},
			},
			wantCount: 0,
		},
		{
			name: "unknown_module_skipped",
			edges: []graph.Edge{
				{From: nodeExtFoo, To: nodeDomMod, Kind: graph.EdgeKindImports},
			},
			wantCount: 0,
		},
	}

	ev := rules.Evidence{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := makeGraph(tc.edges)
			findings := r.Check(g, ev)
			if len(findings) != tc.wantCount {
				t.Fatalf("Check: got %d findings, want %d", len(findings), tc.wantCount)
			}
			for _, f := range findings {
				if f.RuleID != "no-upward-deps" {
					t.Errorf("finding.RuleID = %q, want %q", f.RuleID, "no-upward-deps")
				}
				if f.MatchedBy["from_layer"] == "" {
					t.Error("matched_by.from_layer is empty")
				}
				if f.MatchedBy["to_layer"] == "" {
					t.Error("matched_by.to_layer is empty")
				}
				if f.Why == "" {
					t.Error("finding.Why is empty")
				}
				if f.Constraint == "" {
					t.Error("finding.Constraint is empty")
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// rules.New — mapping and unknown type skipping
// ---------------------------------------------------------------------------

func TestNew_UnknownTypeError(t *testing.T) {
	cfg := view.RuleConfig{
		Rules: []view.RuleDef{
			{ID: "unknown", Type: "cycle_check_future"},
		},
	}
	_, err := rules.New(cfg)
	if err == nil {
		t.Fatal("New: got nil error, want error for unknown rule type")
	}
}

func TestNew_ForbiddenDependencyRequiresFromTo(t *testing.T) {
	cases := []struct {
		name    string
		from    string
		to      string
		wantErr bool
	}{
		{"both set", pathDomainGlob, "infra/**", false},
		{"empty from", "", "infra/**", true},
		{"empty to", pathDomainGlob, "", true},
		{"both empty", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := view.RuleConfig{
				Rules: []view.RuleDef{
					{ID: "fd", Type: "forbidden_dependency", From: tc.from, To: tc.to},
				},
			}
			_, err := rules.New(cfg)
			if tc.wantErr && err == nil {
				t.Fatal("New: got nil error, want error for empty from/to glob")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("New: unexpected error: %v", err)
			}
		})
	}
}

// TestNew_MalformedGlobError pins glob validation at load time: a malformed
// from/to pattern makes doublestar.Match error (discarded in Check), so
// without this the rule would load clean and silently fire zero findings.
func TestNew_MalformedGlobError(t *testing.T) {
	const badGlob = "internal/[" // unclosed character class: doublestar.ErrBadPattern
	cases := []struct {
		name string
		def  view.RuleDef
	}{
		{"forbidden_dependency bad from", view.RuleDef{ID: "fd", Type: typeForbiddenDependency, From: badGlob, To: pathInfraGlob}},
		{"forbidden_dependency bad to", view.RuleDef{ID: "fd", Type: typeForbiddenDependency, From: pathDomainGlob, To: badGlob}},
		{"public_api_only bad from", view.RuleDef{ID: "pao", Type: "public_api_only", From: badGlob}},
		{"internal_api_access bad to", view.RuleDef{ID: "iaa", Type: "internal_api_access", To: badGlob}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := rules.New(view.RuleConfig{Rules: []view.RuleDef{tc.def}})
			if err == nil {
				t.Fatal("New: got nil error, want error for malformed glob")
			}
		})
	}
}

func TestNew_EmptyRules(t *testing.T) {
	ruleSet, err := rules.New(view.RuleConfig{})
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if len(ruleSet) != 0 {
		t.Fatalf("New(empty): got %d rules, want 0", len(ruleSet))
	}
}

// ---------------------------------------------------------------------------
// Gate semantics
// ---------------------------------------------------------------------------

func TestGateSemantics(t *testing.T) {
	edge := graph.Edge{From: nodeAFoo, To: nodeBBar, Kind: graph.EdgeKindImports}
	ev := rules.Evidence{}

	t.Run("off_skips_finding", func(t *testing.T) {
		cfg := view.RuleConfig{
			Rules: []view.RuleDef{
				{ID: ruleIDNoDep, Type: typeForbiddenDependency, From: globServicesA, To: globServicesB, Gate: "off"},
			},
		}
		rs, err := rules.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		findings := rs[0].Check(makeGraph([]graph.Edge{edge}), ev)
		if len(findings) != 0 {
			t.Fatalf("gate:off: got %d findings, want 0", len(findings))
		}
	})

	t.Run("warn_produces_advisory", func(t *testing.T) {
		cfg := view.RuleConfig{
			Rules: []view.RuleDef{
				{ID: ruleIDNoDep, Type: typeForbiddenDependency, From: globServicesA, To: globServicesB, Gate: gateWarn},
			},
		}
		rs, err := rules.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		findings := rs[0].Check(makeGraph([]graph.Edge{edge}), ev)
		if len(findings) != 1 {
			t.Fatalf("gate:warn: got %d findings, want 1", len(findings))
		}
		if findings[0].Kind != "advisory" {
			t.Errorf("gate:warn finding.Kind=%q, want advisory", findings[0].Kind)
		}
	})

	t.Run("fail_produces_gate_finding", func(t *testing.T) {
		cfg := view.RuleConfig{
			Rules: []view.RuleDef{
				{ID: ruleIDNoDep, Type: typeForbiddenDependency, From: globServicesA, To: globServicesB, Gate: "fail"},
			},
		}
		rs, err := rules.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		findings := rs[0].Check(makeGraph([]graph.Edge{edge}), ev)
		if len(findings) != 1 {
			t.Fatalf("gate:fail: got %d findings, want 1", len(findings))
		}
		if findings[0].Kind != kindGate {
			t.Errorf("gate:fail finding.Kind=%q, want gate", findings[0].Kind)
		}
	})

	t.Run("unset_gate_produces_gate_finding", func(t *testing.T) {
		cfg := view.RuleConfig{
			Rules: []view.RuleDef{
				{ID: ruleIDNoDep, Type: typeForbiddenDependency, From: globServicesA, To: globServicesB},
			},
		}
		rs, err := rules.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		findings := rs[0].Check(makeGraph([]graph.Edge{edge}), ev)
		if len(findings) != 1 {
			t.Fatalf("gate unset: got %d findings, want 1", len(findings))
		}
		if findings[0].Kind != kindGate {
			t.Errorf("gate unset finding.Kind=%q, want gate", findings[0].Kind)
		}
	})

	t.Run("unknown_type_returns_error", func(t *testing.T) {
		cfg := view.RuleConfig{
			Rules: []view.RuleDef{
				{ID: "x", Type: "nonexistent_type"},
			},
		}
		_, err := rules.New(cfg)
		if err == nil {
			t.Fatal("want error for unknown type, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// PublicAPIMax
// ---------------------------------------------------------------------------

// maxPtr returns a pointer to n, for use in RuleDef.Max.
func maxPtr(n int) *int { return &n }

// makePublicAPIMaxConfig constructs a Config with two modules and a
// public_api_max rule, then returns the RuleConfig view for rules.New.
func makePublicAPIMaxConfig(ceiling int, gate string) view.RuleConfig {
	return config.Config{
		Version: 1,
		Modules: map[string]module.ModuleDef{
			layerDomain: {Paths: []string{pathDomainGlob}},
			moduleInfra: {Paths: []string{pathInfraGlob}},
		},
		Rules: []view.RuleDef{
			{ID: "api-max", Type: typePublicAPIMax, Gate: gate, Max: maxPtr(ceiling)},
		},
	}.ForRules()
}

// assertLocationFiles checks that locs carries exactly the given files, in
// order — agent_tasks files[] trusts finding.Locations, so a rule that only
// ever sets Edge.From/To.Path to the bare module name must populate real
// Locations instead.
func assertLocationFiles(t *testing.T, locs []graph.Location, wantFiles ...string) {
	t.Helper()
	if len(locs) != len(wantFiles) {
		t.Fatalf("locations = %+v, want files %v", locs, wantFiles)
	}
	for i, want := range wantFiles {
		if locs[i].File != want {
			t.Errorf("locations[%d].File = %q, want %q", i, locs[i].File, want)
		}
	}
}

func TestPublicAPIMax(t *testing.T) {
	// SyntaxFacts used across subtests: domain has 3 exported, infra has 1 exported.
	allFacts := make([]diagnostic.SyntaxFact, 0, 5)
	allFacts = append(allFacts,
		diagnostic.SyntaxFact{Language: graph.LangGo, File: fileDomainA, Kind: kindFunction, Name: nameFuncA, Exported: true},
		diagnostic.SyntaxFact{Language: graph.LangGo, File: fileDomainA, Kind: kindFunction, Name: "FuncB", Exported: true},
		diagnostic.SyntaxFact{Language: graph.LangGo, File: fileDomainB, Kind: kindStruct, Name: "Model", Exported: true},
		diagnostic.SyntaxFact{Language: graph.LangGo, File: fileDomainB, Kind: kindFunction, Name: nameInternal, Exported: false},
		diagnostic.SyntaxFact{Language: graph.LangGo, File: fileInfraRepo, Kind: kindStruct, Name: nameRepo, Exported: true},
	)

	emptyGraph := makeGraph(nil)
	ev := func(facts []diagnostic.SyntaxFact) rules.Evidence {
		return rules.Evidence{SyntaxFacts: facts}
	}

	t.Run("under_limit_no_finding", func(t *testing.T) {
		// ceiling=5: both modules under limit → no findings.
		rc := makePublicAPIMaxConfig(5, "")
		rs, err := rules.New(rc)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if findings := rs[0].Check(emptyGraph, ev(allFacts)); len(findings) != 0 {
			t.Fatalf("want 0 findings, got %d", len(findings))
		}
	})

	t.Run("over_limit_emits_finding_with_correct_count", func(t *testing.T) {
		// ceiling=2: domain has 3 exported → 1 finding; infra has 1 → no finding.
		rc := makePublicAPIMaxConfig(2, "")
		rs, err := rules.New(rc)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		findings := rs[0].Check(emptyGraph, ev(allFacts))
		if len(findings) != 1 {
			t.Fatalf("want 1 finding, got %d", len(findings))
		}
		f := findings[0]
		if f.MatchedBy["module"] != layerDomain {
			t.Errorf("matched_by.module=%q, want %q", f.MatchedBy["module"], layerDomain)
		}
		if f.MatchedBy["count"] != "3" {
			t.Errorf("matched_by.count=%q, want %q", f.MatchedBy["count"], "3")
		}
		if f.MatchedBy["max"] != "2" {
			t.Errorf("matched_by.max=%q, want %q", f.MatchedBy["max"], "2")
		}
		if f.Why == "" {
			t.Error("Why is empty")
		}
		if f.Constraint == "" {
			t.Error("Constraint is empty")
		}
		// Locations must carry the real declaring files, never the bare
		// module name — agent_tasks files[] trusts this list blindly.
		assertLocationFiles(t, f.Locations, fileDomainA, fileDomainB)
	})

	t.Run("per_module_scoping_both_over", func(t *testing.T) {
		// ceiling=0: domain(3)>0 fires, infra(1)>0 fires → 2 findings.
		rc := makePublicAPIMaxConfig(0, "")
		rs, err := rules.New(rc)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		findings := rs[0].Check(emptyGraph, ev(allFacts))
		if len(findings) != 2 {
			t.Fatalf("ceiling=0: want 2 findings, got %d", len(findings))
		}
		// Findings are emitted in sorted module-name order: domain < infra.
		if findings[0].MatchedBy["module"] != layerDomain {
			t.Errorf("findings[0].module=%q, want %q", findings[0].MatchedBy["module"], layerDomain)
		}
		if findings[1].MatchedBy["module"] != moduleInfra {
			t.Errorf("findings[1].module=%q, want %q", findings[1].MatchedBy["module"], moduleInfra)
		}
	})

	t.Run("only_domain_over_limit", func(t *testing.T) {
		// ceiling=1: domain(3)>1 fires, infra(1)=1 does not.
		rc := makePublicAPIMaxConfig(1, "")
		rs, err := rules.New(rc)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		findings := rs[0].Check(emptyGraph, ev(allFacts))
		if len(findings) != 1 {
			t.Fatalf("want 1 finding, got %d", len(findings))
		}
		if findings[0].MatchedBy["module"] != layerDomain {
			t.Errorf("module=%q, want %q", findings[0].MatchedBy["module"], layerDomain)
		}
	})

	t.Run("empty_syntax_facts_no_findings", func(t *testing.T) {
		rc := makePublicAPIMaxConfig(0, "")
		rs, err := rules.New(rc)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if findings := rs[0].Check(emptyGraph, ev(nil)); len(findings) != 0 {
			t.Fatalf("empty facts: want 0 findings, got %d", len(findings))
		}
	})

	t.Run("unexported_decls_not_counted", func(t *testing.T) {
		// Only unexported facts → no exported count → no findings.
		onlyUnexported := []diagnostic.SyntaxFact{
			{Language: graph.LangGo, File: fileDomainA, Kind: kindFunction, Name: nameInternal, Exported: false},
			{Language: graph.LangGo, File: fileDomainB, Kind: kindFunction, Name: "helper", Exported: false},
		}
		rc := makePublicAPIMaxConfig(0, "")
		rs, err := rules.New(rc)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if findings := rs[0].Check(emptyGraph, ev(onlyUnexported)); len(findings) != 0 {
			t.Fatalf("unexported only: want 0, got %d", len(findings))
		}
	})

	t.Run("file_not_in_any_module_skipped", func(t *testing.T) {
		// File outside declared modules → not counted.
		outsideFacts := []diagnostic.SyntaxFact{
			{Language: graph.LangGo, File: fileCmd, Kind: kindFunction, Name: nameMain, Exported: true},
		}
		rc := makePublicAPIMaxConfig(0, "")
		rs, err := rules.New(rc)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if findings := rs[0].Check(emptyGraph, ev(outsideFacts)); len(findings) != 0 {
			t.Fatalf("outside module: want 0, got %d", len(findings))
		}
	})

	t.Run("gate_semantics", func(t *testing.T) {
		// domain(3) > ceiling(2) fires. Check that gate modes work.
		overFacts := allFacts // domain=3, infra=1, ceiling=2 → 1 finding

		t.Run("off_skips_finding", func(t *testing.T) {
			rs, _ := rules.New(makePublicAPIMaxConfig(2, "off"))
			if findings := rs[0].Check(emptyGraph, ev(overFacts)); len(findings) != 0 {
				t.Fatalf("gate:off: want 0 findings, got %d", len(findings))
			}
		})
		t.Run("warn_produces_advisory", func(t *testing.T) {
			rs, _ := rules.New(makePublicAPIMaxConfig(2, gateWarn))
			findings := rs[0].Check(emptyGraph, ev(overFacts))
			if len(findings) != 1 {
				t.Fatalf("gate:warn: want 1 finding, got %d", len(findings))
			}
			if findings[0].Kind != kindAdvisory {
				t.Errorf("gate:warn Kind=%q, want %q", findings[0].Kind, kindAdvisory)
			}
		})
		t.Run("fail_produces_gate_finding", func(t *testing.T) {
			rs, _ := rules.New(makePublicAPIMaxConfig(2, "fail"))
			findings := rs[0].Check(emptyGraph, ev(overFacts))
			if len(findings) != 1 {
				t.Fatalf("gate:fail: want 1 finding, got %d", len(findings))
			}
			if findings[0].Kind != kindGate {
				t.Errorf("gate:fail Kind=%q, want %q", findings[0].Kind, kindGate)
			}
		})
		t.Run("unset_gate_produces_gate_finding", func(t *testing.T) {
			rs, _ := rules.New(makePublicAPIMaxConfig(2, ""))
			findings := rs[0].Check(emptyGraph, ev(overFacts))
			if len(findings) != 1 {
				t.Fatalf("gate unset: want 1 finding, got %d", len(findings))
			}
			if findings[0].Kind != kindGate {
				t.Errorf("gate unset Kind=%q, want %q", findings[0].Kind, kindGate)
			}
		})
	})
}

func TestPublicAPIMax_InvalidConfig(t *testing.T) {
	t.Run("nil_max_returns_error", func(t *testing.T) {
		_, err := rules.New(view.RuleConfig{Rules: []view.RuleDef{
			{ID: "x", Type: typePublicAPIMax},
		}})
		if err == nil {
			t.Fatal("want error for nil Max, got nil")
		}
	})

	t.Run("negative_max_returns_error", func(t *testing.T) {
		_, err := rules.New(view.RuleConfig{Rules: []view.RuleDef{
			{ID: "x", Type: typePublicAPIMax, Max: maxPtr(-1)},
		}})
		if err == nil {
			t.Fatal("want error for negative Max, got nil")
		}
	})

	t.Run("zero_max_is_valid", func(t *testing.T) {
		_, err := rules.New(view.RuleConfig{Rules: []view.RuleDef{
			{ID: "x", Type: typePublicAPIMax, Max: maxPtr(0)},
		}})
		if err != nil {
			t.Fatalf("unexpected error for max=0: %v", err)
		}
	})

	t.Run("valid_config_returns_no_error", func(t *testing.T) {
		_, err := rules.New(view.RuleConfig{Rules: []view.RuleDef{
			{ID: "x", Type: typePublicAPIMax, Max: maxPtr(10)},
		}})
		if err != nil {
			t.Fatalf("unexpected error for valid config: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// PublicAPIChange
// ---------------------------------------------------------------------------

// makePublicAPIChangeConfig constructs a RuleConfig with two modules and a
// public_api_change rule with the given gate.
func makePublicAPIChangeConfig(gate string) view.RuleConfig {
	return config.Config{
		Version: 1,
		Modules: map[string]module.ModuleDef{
			layerDomain: {Paths: []string{pathDomainGlob}},
			moduleInfra: {Paths: []string{pathInfraGlob}},
		},
		Rules: []view.RuleDef{
			{ID: "api-change", Type: typePublicAPIChange, Gate: gate},
		},
	}.ForRules()
}

func TestPublicAPIChange(t *testing.T) {
	// Facts: domain has 2 exported + 1 unexported; infra has 1 exported; one file outside modules.
	exportedFacts := []diagnostic.SyntaxFact{
		{Language: graph.LangGo, File: fileDomainA, Kind: kindFunction, Name: nameFuncA, Exported: true},
		{Language: graph.LangGo, File: fileDomainB, Kind: kindStruct, Name: "Model", Exported: true},
		{Language: graph.LangGo, File: fileDomainB, Kind: kindFunction, Name: nameInternal, Exported: false},
		{Language: graph.LangGo, File: fileInfraRepo, Kind: kindStruct, Name: nameRepo, Exported: true},
		{Language: graph.LangGo, File: fileCmd, Kind: kindFunction, Name: nameMain, Exported: true}, // outside any module
	}

	emptyGraph := makeGraph(nil)
	ev := func(facts []diagnostic.SyntaxFact) rules.Evidence {
		return rules.Evidence{SyntaxFacts: facts}
	}

	t.Run("emits_one_finding_per_exported_decl_per_module", func(t *testing.T) {
		rs, err := rules.New(makePublicAPIChangeConfig("fail"))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		findings := rs[0].Check(emptyGraph, ev(exportedFacts))
		// domain: FuncA + Model = 2; infra: Repo = 1; cmd/main.go outside modules = 0
		if len(findings) != 3 {
			t.Fatalf("want 3 findings (2 domain + 1 infra), got %d", len(findings))
		}
	})

	t.Run("empty_syntax_facts_returns_nil", func(t *testing.T) {
		rs, err := rules.New(makePublicAPIChangeConfig("fail"))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if findings := rs[0].Check(emptyGraph, ev(nil)); len(findings) != 0 {
			t.Fatalf("empty facts: want 0 findings, got %d", len(findings))
		}
	})

	t.Run("unexported_decls_not_emitted", func(t *testing.T) {
		onlyUnexported := []diagnostic.SyntaxFact{
			{Language: graph.LangGo, File: fileDomainA, Kind: kindFunction, Name: "helper", Exported: false},
		}
		rs, err := rules.New(makePublicAPIChangeConfig("fail"))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if findings := rs[0].Check(emptyGraph, ev(onlyUnexported)); len(findings) != 0 {
			t.Fatalf("unexported only: want 0, got %d", len(findings))
		}
	})

	t.Run("file_outside_modules_skipped", func(t *testing.T) {
		outsideFacts := []diagnostic.SyntaxFact{
			{Language: graph.LangGo, File: fileCmd, Kind: kindFunction, Name: nameMain, Exported: true},
		}
		rs, err := rules.New(makePublicAPIChangeConfig("fail"))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if findings := rs[0].Check(emptyGraph, ev(outsideFacts)); len(findings) != 0 {
			t.Fatalf("outside module: want 0, got %d", len(findings))
		}
	})

	t.Run("duplicate_name_in_module_deduped", func(t *testing.T) {
		// Two facts with same (module, name) → only one finding.
		dupFacts := []diagnostic.SyntaxFact{
			{Language: graph.LangGo, File: fileDomainA, Kind: kindFunction, Name: "Start", Exported: true},
			{Language: graph.LangGo, File: fileDomainB, Kind: "method", Name: "Start", Exported: true},
		}
		rs, err := rules.New(makePublicAPIChangeConfig("fail"))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		findings := rs[0].Check(emptyGraph, ev(dupFacts))
		if len(findings) != 1 {
			t.Fatalf("duplicate name: want 1 finding (deduped), got %d", len(findings))
		}
	})

	t.Run("finding_matchedby_has_module_name_kind_file", func(t *testing.T) {
		oneFact := []diagnostic.SyntaxFact{
			{Language: graph.LangGo, File: fileDomainA, Kind: kindFunction, Name: nameFuncA, Exported: true},
		}
		rs, err := rules.New(makePublicAPIChangeConfig("fail"))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		findings := rs[0].Check(emptyGraph, ev(oneFact))
		if len(findings) != 1 {
			t.Fatalf("want 1 finding, got %d", len(findings))
		}
		f := findings[0]
		if f.MatchedBy["module"] != layerDomain {
			t.Errorf("MatchedBy[module]=%q, want %q", f.MatchedBy["module"], layerDomain)
		}
		if f.MatchedBy["name"] != "FuncA" {
			t.Errorf("MatchedBy[name]=%q, want FuncA", f.MatchedBy["name"])
		}
		if f.MatchedBy["kind"] != kindFunction {
			t.Errorf("MatchedBy[kind]=%q, want %q", f.MatchedBy["kind"], kindFunction)
		}
		if f.MatchedBy["file"] != fileDomainA {
			t.Errorf("MatchedBy[file]=%q, want %q", f.MatchedBy["file"], fileDomainA)
		}
		assertLocationFiles(t, f.Locations, fileDomainA)
	})

	t.Run("stable_fingerprint_across_runs", func(t *testing.T) {
		// Same fact → same ID on repeated calls.
		oneFact := []diagnostic.SyntaxFact{
			{Language: graph.LangGo, File: fileDomainA, Kind: kindFunction, Name: nameFuncA, Exported: true},
		}
		rs, err := rules.New(makePublicAPIChangeConfig("fail"))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		id1 := rs[0].Check(emptyGraph, ev(oneFact))[0].ID
		id2 := rs[0].Check(emptyGraph, ev(oneFact))[0].ID
		if id1 != id2 {
			t.Errorf("fingerprint unstable: %q != %q", id1, id2)
		}
	})

	t.Run("gate_semantics", func(t *testing.T) {
		oneFact := []diagnostic.SyntaxFact{
			{Language: graph.LangGo, File: fileDomainA, Kind: kindFunction, Name: nameFuncA, Exported: true},
		}

		t.Run("off_skips_finding", func(t *testing.T) {
			rs, _ := rules.New(makePublicAPIChangeConfig("off"))
			if findings := rs[0].Check(emptyGraph, ev(oneFact)); len(findings) != 0 {
				t.Fatalf("gate:off: want 0 findings, got %d", len(findings))
			}
		})
		t.Run("warn_produces_advisory", func(t *testing.T) {
			rs, _ := rules.New(makePublicAPIChangeConfig(gateWarn))
			findings := rs[0].Check(emptyGraph, ev(oneFact))
			if len(findings) != 1 {
				t.Fatalf("gate:warn: want 1 finding, got %d", len(findings))
			}
			if findings[0].Kind != kindAdvisory {
				t.Errorf("gate:warn Kind=%q, want %q", findings[0].Kind, kindAdvisory)
			}
		})
		t.Run("fail_produces_gate_finding", func(t *testing.T) {
			rs, _ := rules.New(makePublicAPIChangeConfig("fail"))
			findings := rs[0].Check(emptyGraph, ev(oneFact))
			if len(findings) != 1 {
				t.Fatalf("gate:fail: want 1 finding, got %d", len(findings))
			}
			if findings[0].Kind != kindGate {
				t.Errorf("gate:fail Kind=%q, want %q", findings[0].Kind, kindGate)
			}
		})
		t.Run("unset_gate_produces_advisory", func(t *testing.T) {
			// public_api_change defaults to warn (advisory) when gate is unset.
			rs, _ := rules.New(makePublicAPIChangeConfig(""))
			findings := rs[0].Check(emptyGraph, ev(oneFact))
			if len(findings) != 1 {
				t.Fatalf("gate unset: want 1 finding, got %d", len(findings))
			}
			if findings[0].Kind != kindAdvisory {
				t.Errorf("gate unset Kind=%q, want %q (warn-by-default)", findings[0].Kind, kindAdvisory)
			}
		})
	})
}

// ---------------------------------------------------------------------------
// PublicAPITypeLeak
// ---------------------------------------------------------------------------

const (
	typePublicAPITypeLeak = "public_api_type_leak"
	typeCliContext        = "cli.Context"
)

// makeTypeLeakGraph builds a graph with the given package nodes (no edges needed).
func makeTypeLeakGraph(pkgPaths []string) *graph.Graph {
	nodes := make([]graph.Node, len(pkgPaths))
	for i, p := range pkgPaths {
		nodes[i] = graph.Node{Kind: graph.NodeKindPackage, Path: p}
	}
	return graph.Build([]graph.Facts{{Nodes: nodes, Language: "go"}})
}

func TestPublicAPITypeLeak(t *testing.T) {
	const (
		ruleID         = "no-type-leak"
		fileDomain     = "domain/service.go"
		moduleNameDom  = "domain"
		typeLeakKind   = "type_leak"
		externalCliPkg = "github.com/urfave/cli/v2"
	)

	rc := config.Config{
		Version: 1,
		Modules: map[string]module.ModuleDef{
			moduleNameDom: {Paths: []string{"domain/**"}},
		},
		Rules: []view.RuleDef{
			{ID: ruleID, Type: typePublicAPITypeLeak},
		},
	}.ForRules()
	rs, err := rules.New(rc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	r := rs[0]

	t.Run("external_type_leak_fires", func(t *testing.T) {
		g := makeTypeLeakGraph([]string{externalCliPkg})
		ev := rules.Evidence{
			SyntaxFacts: []diagnostic.SyntaxFact{
				{Kind: typeLeakKind, Name: typeCliContext, File: fileDomain, Language: "go"},
			},
		}
		findings := r.Check(g, ev)
		if len(findings) != 1 {
			t.Fatalf("want 1 finding, got %d: %+v", len(findings), findings)
		}
		f := findings[0]
		if f.RuleID != ruleID {
			t.Errorf("RuleID=%q, want %q", f.RuleID, ruleID)
		}
		if f.MatchedBy["module"] != moduleNameDom {
			t.Errorf("matched_by.module=%q, want %q", f.MatchedBy["module"], moduleNameDom)
		}
		if f.MatchedBy["type"] != typeCliContext {
			t.Errorf("matched_by.type=%q, want %q", f.MatchedBy["type"], typeCliContext)
		}
		if f.Why == "" {
			t.Error("Why is empty")
		}
		if f.Severity != finding.SeverityMedium {
			t.Errorf("Severity=%v, want Medium", f.Severity)
		}
		assertLocationFiles(t, f.Locations, fileDomain)
	})

	t.Run("first_party_type_no_finding", func(t *testing.T) {
		// "internal" has no dot — not a fully-qualified external path.
		g := makeTypeLeakGraph([]string{"internal/service"})
		ev := rules.Evidence{
			SyntaxFacts: []diagnostic.SyntaxFact{
				{Kind: typeLeakKind, Name: "internal.Service", File: fileDomain, Language: "go"},
			},
		}
		findings := r.Check(g, ev)
		if len(findings) != 0 {
			t.Fatalf("want 0 findings for first-party type, got %d", len(findings))
		}
	})

	t.Run("no_syntax_facts_returns_nil", func(t *testing.T) {
		g := makeTypeLeakGraph([]string{externalCliPkg})
		ev := rules.Evidence{}
		findings := r.Check(g, ev)
		if len(findings) != 0 {
			t.Fatalf("want 0 findings with empty SyntaxFacts, got %d", len(findings))
		}
	})

	t.Run("dedup_same_module_and_type", func(t *testing.T) {
		g := makeTypeLeakGraph([]string{externalCliPkg})
		ev := rules.Evidence{
			SyntaxFacts: []diagnostic.SyntaxFact{
				{Kind: typeLeakKind, Name: typeCliContext, File: fileDomain, Language: "go"},
				{Kind: typeLeakKind, Name: typeCliContext, File: fileDomain, Language: "go"},
			},
		}
		findings := r.Check(g, ev)
		if len(findings) != 1 {
			t.Fatalf("want 1 finding after dedup, got %d", len(findings))
		}
	})

	t.Run("default_gate_is_warn", func(t *testing.T) {
		// gatedRule wraps with gate="warn" → Kind=advisory.
		g := makeTypeLeakGraph([]string{externalCliPkg})
		ev := rules.Evidence{
			SyntaxFacts: []diagnostic.SyntaxFact{
				{Kind: typeLeakKind, Name: typeCliContext, File: fileDomain, Language: "go"},
			},
		}
		findings := r.Check(g, ev)
		if len(findings) != 1 {
			t.Fatalf("want 1 finding, got %d", len(findings))
		}
		if findings[0].Kind != kindAdvisory {
			t.Errorf("Kind=%q, want %q (gate: warn default)", findings[0].Kind, kindAdvisory)
		}
	})

	t.Run("no_ext_pkgs_returns_nil", func(t *testing.T) {
		// Ceiling: no dotted external nodes → extPkgs empty → returns nil regardless
		// of type_leak facts. Documents the Go-only graph limitation.
		g := makeTypeLeakGraph(nil)
		ev := rules.Evidence{
			SyntaxFacts: []diagnostic.SyntaxFact{
				{Kind: typeLeakKind, Name: typeCliContext, File: fileDomain, Language: "go"},
			},
		}
		findings := r.Check(g, ev)
		if len(findings) != 0 {
			t.Fatalf("want 0 findings when extPkgs empty, got %d: %+v", len(findings), findings)
		}
	})

	t.Run("first_party_basename_collision_fires", func(t *testing.T) {
		// A graph with both an external "github.com/urfave/cli/v2" node AND a
		// first-party undotted "cli" package share a basename. The collision guard was
		// removed because it caused false negatives: real external leaks were suppressed
		// whenever any first-party package shared a basename with an external one. This
		// is the regression test: the external leak MUST fire even when a same-basename
		// first-party package exists. A false positive (flagging a first-party reference)
		// is acceptable for this report-only/default-warn candidate surfacer.
		g := makeTypeLeakGraph([]string{externalCliPkg, "cli"})
		ev := rules.Evidence{
			SyntaxFacts: []diagnostic.SyntaxFact{
				{Kind: typeLeakKind, Name: typeCliContext, File: fileDomain, Language: "go"},
			},
		}
		findings := r.Check(g, ev)
		if len(findings) != 1 {
			t.Fatalf("want 1 finding: external leak fires even when basename collides with first-party package, got %d: %+v", len(findings), findings)
		}
	})

	t.Run("external_type_leak_fires_edge_target_graph", func(t *testing.T) {
		// Build a graph shaped like the Go extractor: external package as edge target only,
		// no NodeKindPackage node for it. This is how Go graphs actually look.
		fileNode := graph.Node{Kind: graph.NodeKindFile, Path: fileDomain}
		extPkgID := graph.Node{Kind: graph.NodeKindPackage, Path: externalCliPkg}.ID()
		edge := graph.Edge{
			From:       fileNode.ID(),
			To:         extPkgID,
			Kind:       graph.EdgeKindImports,
			Language:   "go",
			Confidence: "high",
		}
		g := graph.Build([]graph.Facts{{Nodes: []graph.Node{fileNode}, Edges: []graph.Edge{edge}, Language: "go"}})
		ev := rules.Evidence{
			SyntaxFacts: []diagnostic.SyntaxFact{
				{Kind: typeLeakKind, Name: typeCliContext, File: fileDomain, Language: "go"},
			},
		}
		findings := r.Check(g, ev)
		if len(findings) != 1 {
			t.Fatalf("want 1 finding on edge-target graph, got %d: %+v", len(findings), findings)
		}
	})

	t.Run("edge_target_first_party_collision_fires", func(t *testing.T) {
		// Edge-target external path whose basename collides with a first-party undotted
		// package node. The collision guard was removed — see first_party_basename_collision_fires
		// for rationale. External leak MUST fire; the first-party node does not suppress it.
		fileNode := graph.Node{Kind: graph.NodeKindFile, Path: fileDomain}
		firstPartyNode := graph.Node{Kind: graph.NodeKindPackage, Path: "cli"} // undotted first-party
		extPkgID := graph.Node{Kind: graph.NodeKindPackage, Path: externalCliPkg}.ID()
		edge := graph.Edge{
			From:       fileNode.ID(),
			To:         extPkgID,
			Kind:       graph.EdgeKindImports,
			Language:   "go",
			Confidence: "high",
		}
		g := graph.Build([]graph.Facts{{
			Nodes:    []graph.Node{fileNode, firstPartyNode},
			Edges:    []graph.Edge{edge},
			Language: "go",
		}})
		ev := rules.Evidence{
			SyntaxFacts: []diagnostic.SyntaxFact{
				{Kind: typeLeakKind, Name: typeCliContext, File: fileDomain, Language: "go"},
			},
		}
		findings := r.Check(g, ev)
		if len(findings) != 1 {
			t.Fatalf("want 1 finding: external leak fires even when edge-target basename collides with first-party package, got %d: %+v", len(findings), findings)
		}
	})
}
