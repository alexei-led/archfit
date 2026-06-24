package rules_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/rules"
	"github.com/alexei-led/archfit/internal/syntax"
)

// Node ID constants reused across multiple test cases.
const (
	nodeAFoo     = "file:services/a/foo.go"
	nodeBBar     = "file:services/b/bar.go"
	nodeCFoo     = "file:services/c/foo.go"
	nodeABaz     = "file:services/a/baz.go"
	nodeBQux     = "file:services/b/qux.go"
	nodeBIntBar  = "file:services/b/internal/bar.go"
	nodeBAPI     = "file:services/b/api.go"
	nodeDIntY    = "file:services/d/internal/y.go"
	nodeCFoo2    = "file:services/c/foo.go"
	nodeInfraRep = "file:services/infra/repo.go"
	nodeDomMod   = "file:services/domain/model.go"
	nodeAppHnd   = "file:services/app/handler.go"
	nodeAppSvc   = "file:services/app/service.go"
	nodeDomA     = "file:services/domain/a.go"
	nodeDomB     = "file:services/domain/b.go"
	nodeExtFoo   = "file:external/foo.go"
)

// Layer name constants.
const (
	layerDomain         = "domain"
	layerApplication    = "application"
	layerInfrastructure = "infrastructure"
)

// Rule type and ID constants for gate semantics tests.
const (
	typeForbiddenDependency     = "forbidden_dependency"
	typeForbiddenRoleDependency = "forbidden_role_dependency"
	typePublicAPIMax            = "public_api_max"
	typePublicAPIChange         = "public_api_change"
	kindGate                    = "gate"
	kindAdvisory                = "advisory"
	ruleIDNoDep                 = "no-dep"
	ruleIDRoleDep               = "no-handler-to-repo"
	globServicesA               = "services/a/**"
	globServicesB               = "services/b/**"
	// publicAPIMax / publicAPIChange test constants
	fileDomainA  = "domain/a.go"
	fileDomainB  = "domain/b.go"
	moduleInfra  = "infra"
	kindFunction = "function"
	kindStruct   = "struct"
	nameFuncA    = "FuncA"
	nameInternal = "internal"
	nameMain     = "Main"
	fileCmd      = "cmd/main.go"
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
	cfg := config.RuleConfig{
		Rules: []config.RuleDef{
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
	cfg := config.RuleConfig{
		Rules: []config.RuleDef{
			{ID: "no-internal-access", Type: "public_api_only"},
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
				if f.RuleID != "no-internal-access" {
					t.Errorf("finding.RuleID = %q, want %q", f.RuleID, "no-internal-access")
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

// ---------------------------------------------------------------------------
// ForbiddenLayerDirection
// ---------------------------------------------------------------------------

func TestForbiddenLayerDirection(t *testing.T) {
	// layers: [domain(0), application(1), infrastructure(2)]
	// violation: fromRank > toRank (e.g. infrastructure→domain, infrastructure→application)
	cfg := config.Config{
		Version: 1,
		Layers:  []string{layerDomain, layerApplication, layerInfrastructure},
		Modules: map[string]config.ModuleDef{
			"domain": {Paths: []string{"services/domain/**"}, Layer: layerDomain},
			"app":    {Paths: []string{"services/app/**"}, Layer: layerApplication},
			"infra":  {Paths: []string{"services/infra/**"}, Layer: layerInfrastructure},
		},
		Rules: []config.RuleDef{
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
	cfg := config.RuleConfig{
		Rules: []config.RuleDef{
			{ID: "unknown", Type: "cycle_check_future"},
		},
	}
	_, err := rules.New(cfg)
	if err == nil {
		t.Fatal("New: got nil error, want error for unknown rule type")
	}
}

func TestNew_EmptyRules(t *testing.T) {
	ruleSet, err := rules.New(config.RuleConfig{})
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
		cfg := config.RuleConfig{
			Rules: []config.RuleDef{
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
		cfg := config.RuleConfig{
			Rules: []config.RuleDef{
				{ID: ruleIDNoDep, Type: typeForbiddenDependency, From: globServicesA, To: globServicesB, Gate: "warn"},
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
		cfg := config.RuleConfig{
			Rules: []config.RuleDef{
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
		cfg := config.RuleConfig{
			Rules: []config.RuleDef{
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
		cfg := config.RuleConfig{
			Rules: []config.RuleDef{
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
// ForbiddenRoleDependency
// ---------------------------------------------------------------------------

// makeRoleGraph builds a graph and NodeRoleIndex for forbidden_role_dependency tests.
// facts must already have Role/RoleConf set (pre-derived).
func makeRoleGraph(facts []graph.Facts, syntaxFacts []diagnostic.SyntaxFact) (*graph.Graph, *syntax.NodeRoleIndex) {
	g := graph.Build(facts)
	idx := syntax.BuildNodeRoleIndex(g, syntaxFacts)
	return g, idx
}

// roleDef is a shorthand for building a forbidden_role_dependency RuleDef.
// Always uses ruleIDRoleDep, RoleHandler→RoleRepository. minConf and gate are variable.
func roleDef(minConf, gate string) config.RuleDef {
	return config.RuleDef{
		ID:            ruleIDRoleDep,
		Type:          typeForbiddenRoleDependency,
		FromRole:      syntax.RoleHandler,
		ToRole:        syntax.RoleRepository,
		MinConfidence: minConf,
		Gate:          gate,
	}
}

// assertRoleFinding checks that a finding has the expected from_role, to_role, and non-empty Why.
func assertRoleFinding(t *testing.T, f finding.Finding, wantFrom, wantTo string) {
	t.Helper()
	if f.MatchedBy["from_role"] != wantFrom {
		t.Errorf("matched_by.from_role=%q, want %q", f.MatchedBy["from_role"], wantFrom)
	}
	if f.MatchedBy["to_role"] != wantTo {
		t.Errorf("matched_by.to_role=%q, want %q", f.MatchedBy["to_role"], wantTo)
	}
	if f.Why == "" {
		t.Error("Why is empty")
	}
}

func TestForbiddenRoleDependency(t *testing.T) {
	const (
		ruleID    = "no-handler-to-repo"
		fromRole  = syntax.RoleHandler
		toRole    = syntax.RoleRepository
		nodeHFile = "src/handler/user.ts"
		nodeRFile = "src/repository/user.ts"
	)

	// Reusable TypeScript graph: two file nodes + one import edge.
	tsFacts := []graph.Facts{{
		Language: graph.LangTypeScript,
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: nodeHFile},
			{Kind: graph.NodeKindFile, Path: nodeRFile},
		},
		Edges: []graph.Edge{
			{From: "file:" + nodeHFile, To: "file:" + nodeRFile, Kind: graph.EdgeKindImports, Language: graph.LangTypeScript},
		},
	}}

	t.Run("violation fires at high confidence", func(t *testing.T) {
		syntaxFacts := []diagnostic.SyntaxFact{
			{Language: graph.LangTypeScript, File: nodeHFile, Role: syntax.RoleHandler, RoleConf: syntax.ConfHigh},
			{Language: graph.LangTypeScript, File: nodeRFile, Role: syntax.RoleRepository, RoleConf: syntax.ConfHigh},
		}
		g, idx := makeRoleGraph(tsFacts, syntaxFacts)
		ev := rules.Evidence{Roles: idx}

		rs, err := rules.New(config.RuleConfig{Rules: []config.RuleDef{roleDef("", "")}})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		findings := rs[0].Check(g, ev)
		if len(findings) != 1 {
			t.Fatalf("want 1 finding, got %d", len(findings))
		}
		f := findings[0]
		if f.RuleID != ruleID {
			t.Errorf("RuleID=%q, want %q", f.RuleID, ruleID)
		}
		if f.MatchedBy["from_role"] != fromRole {
			t.Errorf("matched_by.from_role=%q, want %q", f.MatchedBy["from_role"], fromRole)
		}
		if f.MatchedBy["to_role"] != toRole {
			t.Errorf("matched_by.to_role=%q, want %q", f.MatchedBy["to_role"], toRole)
		}
		if f.Why == "" {
			t.Error("Why is empty")
		}
		if f.Constraint == "" {
			t.Error("Constraint is empty")
		}
	})

	t.Run("suppressed when to_role below default threshold", func(t *testing.T) {
		// handler=high, repository=medium — default min is high → no fire.
		syntaxFacts := []diagnostic.SyntaxFact{
			{Language: graph.LangTypeScript, File: nodeHFile, Role: syntax.RoleHandler, RoleConf: syntax.ConfHigh},
			{Language: graph.LangTypeScript, File: nodeRFile, Role: syntax.RoleRepository, RoleConf: syntax.ConfMedium},
		}
		g, idx := makeRoleGraph(tsFacts, syntaxFacts)
		ev := rules.Evidence{Roles: idx}

		rs, err := rules.New(config.RuleConfig{Rules: []config.RuleDef{roleDef("", "")}})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		findings := rs[0].Check(g, ev)
		if len(findings) != 0 {
			t.Fatalf("want 0 findings (below threshold), got %d", len(findings))
		}
	})

	t.Run("MinConfidence relaxed fires at medium", func(t *testing.T) {
		// Same handler=high, repository=medium but MinConfidence="medium" → fires.
		syntaxFacts := []diagnostic.SyntaxFact{
			{Language: graph.LangTypeScript, File: nodeHFile, Role: syntax.RoleHandler, RoleConf: syntax.ConfHigh},
			{Language: graph.LangTypeScript, File: nodeRFile, Role: syntax.RoleRepository, RoleConf: syntax.ConfMedium},
		}
		g, idx := makeRoleGraph(tsFacts, syntaxFacts)
		ev := rules.Evidence{Roles: idx}

		rs, err := rules.New(config.RuleConfig{Rules: []config.RuleDef{roleDef(syntax.ConfMedium, "")}})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		findings := rs[0].Check(g, ev)
		if len(findings) != 1 {
			t.Fatalf("want 1 finding with relaxed threshold, got %d", len(findings))
		}
	})

	t.Run("nil Roles no panic", func(t *testing.T) {
		g := graph.Build(tsFacts)
		ev := rules.Evidence{Roles: nil}

		rs, err := rules.New(config.RuleConfig{Rules: []config.RuleDef{roleDef("", "")}})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		findings := rs[0].Check(g, ev)
		if len(findings) != 0 {
			t.Fatalf("nil Roles: want 0 findings, got %d", len(findings))
		}
	})

	t.Run("gate semantics", func(t *testing.T) {
		syntaxFacts := []diagnostic.SyntaxFact{
			{Language: graph.LangTypeScript, File: nodeHFile, Role: syntax.RoleHandler, RoleConf: syntax.ConfHigh},
			{Language: graph.LangTypeScript, File: nodeRFile, Role: syntax.RoleRepository, RoleConf: syntax.ConfHigh},
		}
		g, idx := makeRoleGraph(tsFacts, syntaxFacts)

		t.Run("off_skips_finding", func(t *testing.T) {
			rs, _ := rules.New(config.RuleConfig{Rules: []config.RuleDef{roleDef("", "off")}})
			if findings := rs[0].Check(g, rules.Evidence{Roles: idx}); len(findings) != 0 {
				t.Fatalf("gate:off: got %d findings, want 0", len(findings))
			}
		})
		t.Run("warn_produces_advisory", func(t *testing.T) {
			rs, _ := rules.New(config.RuleConfig{Rules: []config.RuleDef{roleDef("", "warn")}})
			findings := rs[0].Check(g, rules.Evidence{Roles: idx})
			if len(findings) != 1 {
				t.Fatalf("gate:warn: got %d findings, want 1", len(findings))
			}
			if findings[0].Kind != "advisory" {
				t.Errorf("gate:warn Kind=%q, want advisory", findings[0].Kind)
			}
		})
		t.Run("fail_produces_gate_finding", func(t *testing.T) {
			rs, _ := rules.New(config.RuleConfig{Rules: []config.RuleDef{roleDef("", "fail")}})
			findings := rs[0].Check(g, rules.Evidence{Roles: idx})
			if len(findings) != 1 {
				t.Fatalf("gate:fail: got %d findings, want 1", len(findings))
			}
			if findings[0].Kind != "gate" {
				t.Errorf("gate:fail Kind=%q, want gate", findings[0].Kind)
			}
		})
		t.Run("unset_gate_produces_gate_finding", func(t *testing.T) {
			rs, _ := rules.New(config.RuleConfig{Rules: []config.RuleDef{roleDef("", "")}})
			findings := rs[0].Check(g, rules.Evidence{Roles: idx})
			if len(findings) != 1 {
				t.Fatalf("gate unset: got %d findings, want 1", len(findings))
			}
			if findings[0].Kind != "gate" {
				t.Errorf("gate unset Kind=%q, want gate", findings[0].Kind)
			}
		})
	})

	t.Run("per-language endpoint resolution", func(t *testing.T) {
		t.Run("Go package nodes", func(t *testing.T) {
			// Go rules fire on package→package edges; roles attach to package nodes.
			goFacts := []graph.Facts{{
				Language: graph.LangGo,
				Nodes: []graph.Node{
					{Kind: graph.NodeKindPackage, Path: "internal/handler"},
					{Kind: graph.NodeKindPackage, Path: "internal/repository"},
				},
				Edges: []graph.Edge{
					{From: "package:internal/handler", To: "package:internal/repository", Kind: graph.EdgeKindImports, Language: graph.LangGo},
				},
			}}
			syntaxFacts := []diagnostic.SyntaxFact{
				{Language: graph.LangGo, File: "internal/handler/user.go", Role: syntax.RoleHandler, RoleConf: syntax.ConfHigh},
				{Language: graph.LangGo, File: "internal/repository/user.go", Role: syntax.RoleRepository, RoleConf: syntax.ConfHigh},
			}
			g, idx := makeRoleGraph(goFacts, syntaxFacts)
			rs, err := rules.New(config.RuleConfig{Rules: []config.RuleDef{roleDef("", "")}})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			findings := rs[0].Check(g, rules.Evidence{Roles: idx})
			if len(findings) != 1 {
				t.Fatalf("Go: want 1 finding, got %d", len(findings))
			}
			assertRoleFinding(t, findings[0], syntax.RoleHandler, syntax.RoleRepository)
		})

		t.Run("Python module nodes", func(t *testing.T) {
			pyFacts := []graph.Facts{{
				Language: graph.LangPython,
				Nodes: []graph.Node{
					{Kind: graph.NodeKindModule, Path: "billing.handler"},
					{Kind: graph.NodeKindModule, Path: "billing.repository"},
				},
				Edges: []graph.Edge{
					{From: "module:billing.handler", To: "module:billing.repository", Kind: graph.EdgeKindImports, Language: graph.LangPython},
				},
			}}
			syntaxFacts := []diagnostic.SyntaxFact{
				{Language: graph.LangPython, File: "billing/handler.py", Role: syntax.RoleHandler, RoleConf: syntax.ConfHigh},
				{Language: graph.LangPython, File: "billing/repository.py", Role: syntax.RoleRepository, RoleConf: syntax.ConfHigh},
			}
			g, idx := makeRoleGraph(pyFacts, syntaxFacts)
			rs, err := rules.New(config.RuleConfig{Rules: []config.RuleDef{roleDef("", "")}})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			findings := rs[0].Check(g, rules.Evidence{Roles: idx})
			if len(findings) != 1 {
				t.Fatalf("Python: want 1 finding, got %d", len(findings))
			}
		})

		t.Run("TypeScript file nodes", func(t *testing.T) {
			// Already covered by the top-level TS graph; verify identity map explicitly.
			syntaxFacts := []diagnostic.SyntaxFact{
				{Language: graph.LangTypeScript, File: nodeHFile, Role: syntax.RoleHandler, RoleConf: syntax.ConfHigh},
				{Language: graph.LangTypeScript, File: nodeRFile, Role: syntax.RoleRepository, RoleConf: syntax.ConfHigh},
			}
			g, idx := makeRoleGraph(tsFacts, syntaxFacts)
			rs, err := rules.New(config.RuleConfig{Rules: []config.RuleDef{roleDef("", "")}})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			findings := rs[0].Check(g, rules.Evidence{Roles: idx})
			if len(findings) != 1 {
				t.Fatalf("TypeScript: want 1 finding, got %d", len(findings))
			}
		})

		t.Run("Rust package nodes", func(t *testing.T) {
			// Dir is the crate root (where Cargo.toml lives); files live under Dir/src/.
			crates := []graph.CrateRoot{{Dir: "myapp", Name: "myapp"}}
			rustFacts := []graph.Facts{{
				Language: graph.LangRust,
				Nodes: []graph.Node{
					{Kind: graph.NodeKindPackage, Path: "myapp::handler"},
					{Kind: graph.NodeKindPackage, Path: "myapp::repository"},
				},
				Edges: []graph.Edge{
					{From: "package:myapp::handler", To: "package:myapp::repository", Kind: graph.EdgeKindDependsOn, Language: graph.LangRust},
				},
				CrateRoots: crates,
			}}
			syntaxFacts := []diagnostic.SyntaxFact{
				// myapp/src/handler/mod.rs → myapp::handler via RustFileToModuleKey
				{Language: graph.LangRust, File: "myapp/src/handler/mod.rs", Role: syntax.RoleHandler, RoleConf: syntax.ConfHigh},
				{Language: graph.LangRust, File: "myapp/src/repository/mod.rs", Role: syntax.RoleRepository, RoleConf: syntax.ConfHigh},
			}
			g, idx := makeRoleGraph(rustFacts, syntaxFacts)
			rs, err := rules.New(config.RuleConfig{Rules: []config.RuleDef{roleDef("", "")}})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			findings := rs[0].Check(g, rules.Evidence{Roles: idx})
			if len(findings) != 1 {
				t.Fatalf("Rust: want 1 finding, got %d", len(findings))
			}
		})
	})
}

func TestForbiddenRoleDependency_InvalidConfig(t *testing.T) {
	t.Run("empty FromRole returns error", func(t *testing.T) {
		_, err := rules.New(config.RuleConfig{Rules: []config.RuleDef{
			{ID: "x", Type: typeForbiddenRoleDependency, ToRole: syntax.RoleRepository},
		}})
		if err == nil {
			t.Fatal("want error for empty FromRole, got nil")
		}
	})

	t.Run("empty ToRole returns error", func(t *testing.T) {
		_, err := rules.New(config.RuleConfig{Rules: []config.RuleDef{
			{ID: "x", Type: typeForbiddenRoleDependency, FromRole: syntax.RoleHandler},
		}})
		if err == nil {
			t.Fatal("want error for empty ToRole, got nil")
		}
	})

	t.Run("unknown FromRole returns error", func(t *testing.T) {
		_, err := rules.New(config.RuleConfig{Rules: []config.RuleDef{
			{ID: "x", Type: typeForbiddenRoleDependency, FromRole: "gateway", ToRole: syntax.RoleRepository},
		}})
		if err == nil {
			t.Fatal("want error for unknown from_role, got nil")
		}
	})

	t.Run("unknown ToRole returns error", func(t *testing.T) {
		_, err := rules.New(config.RuleConfig{Rules: []config.RuleDef{
			{ID: "x", Type: typeForbiddenRoleDependency, FromRole: syntax.RoleHandler, ToRole: "store"},
		}})
		if err == nil {
			t.Fatal("want error for unknown to_role, got nil")
		}
	})

	t.Run("bad MinConfidence returns error", func(t *testing.T) {
		_, err := rules.New(config.RuleConfig{Rules: []config.RuleDef{
			{ID: "x", Type: typeForbiddenRoleDependency, FromRole: syntax.RoleHandler, ToRole: syntax.RoleRepository, MinConfidence: "hi"},
		}})
		if err == nil {
			t.Fatal("want error for unknown min_confidence, got nil")
		}
	})

	t.Run("valid config returns no error", func(t *testing.T) {
		_, err := rules.New(config.RuleConfig{Rules: []config.RuleDef{
			{ID: "x", Type: typeForbiddenRoleDependency, FromRole: syntax.RoleHandler, ToRole: syntax.RoleRepository, MinConfidence: syntax.ConfMedium},
		}})
		if err != nil {
			t.Fatalf("unexpected error for valid config: %v", err)
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
func makePublicAPIMaxConfig(ceiling int, gate string) config.RuleConfig {
	return config.Config{
		Version: 1,
		Modules: map[string]config.ModuleDef{
			layerDomain: {Paths: []string{"domain/**"}},
			moduleInfra: {Paths: []string{"infra/**"}},
		},
		Rules: []config.RuleDef{
			{ID: "api-max", Type: typePublicAPIMax, Gate: gate, Max: maxPtr(ceiling)},
		},
	}.ForRules()
}

func TestPublicAPIMax(t *testing.T) {
	// SyntaxFacts used across subtests: domain has 3 exported, infra has 1 exported.
	allFacts := make([]diagnostic.SyntaxFact, 0, 5)
	allFacts = append(allFacts,
		diagnostic.SyntaxFact{Language: graph.LangGo, File: fileDomainA, Kind: kindFunction, Name: nameFuncA, Exported: true},
		diagnostic.SyntaxFact{Language: graph.LangGo, File: fileDomainA, Kind: kindFunction, Name: "FuncB", Exported: true},
		diagnostic.SyntaxFact{Language: graph.LangGo, File: fileDomainB, Kind: kindStruct, Name: "Model", Exported: true},
		diagnostic.SyntaxFact{Language: graph.LangGo, File: fileDomainB, Kind: kindFunction, Name: nameInternal, Exported: false},
		diagnostic.SyntaxFact{Language: graph.LangGo, File: "infra/repo.go", Kind: kindStruct, Name: "Repo", Exported: true},
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
			rs, _ := rules.New(makePublicAPIMaxConfig(2, "warn"))
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
		_, err := rules.New(config.RuleConfig{Rules: []config.RuleDef{
			{ID: "x", Type: typePublicAPIMax},
		}})
		if err == nil {
			t.Fatal("want error for nil Max, got nil")
		}
	})

	t.Run("negative_max_returns_error", func(t *testing.T) {
		_, err := rules.New(config.RuleConfig{Rules: []config.RuleDef{
			{ID: "x", Type: typePublicAPIMax, Max: maxPtr(-1)},
		}})
		if err == nil {
			t.Fatal("want error for negative Max, got nil")
		}
	})

	t.Run("zero_max_is_valid", func(t *testing.T) {
		_, err := rules.New(config.RuleConfig{Rules: []config.RuleDef{
			{ID: "x", Type: typePublicAPIMax, Max: maxPtr(0)},
		}})
		if err != nil {
			t.Fatalf("unexpected error for max=0: %v", err)
		}
	})

	t.Run("valid_config_returns_no_error", func(t *testing.T) {
		_, err := rules.New(config.RuleConfig{Rules: []config.RuleDef{
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
func makePublicAPIChangeConfig(gate string) config.RuleConfig {
	return config.Config{
		Version: 1,
		Modules: map[string]config.ModuleDef{
			layerDomain: {Paths: []string{"domain/**"}},
			moduleInfra: {Paths: []string{"infra/**"}},
		},
		Rules: []config.RuleDef{
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
		{Language: graph.LangGo, File: "infra/repo.go", Kind: kindStruct, Name: "Repo", Exported: true},
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
			rs, _ := rules.New(makePublicAPIChangeConfig("warn"))
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
