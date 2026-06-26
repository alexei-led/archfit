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
	typeTestInProduction        = "test_in_production"
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
	// struct_field_max test constants
	kindStructField = "struct_field"
	nameRepo        = "Repo"
	fileInfraRepo   = "infra/repo.go"
	pathDomainGlob  = "domain/**"
	pathInfraGlob   = "infra/**"
	// test_in_production test constants
	kindTestImport   = "test_import"
	frameworkTestify = "testify/mock"
	frameworkJest    = "jest"
	frameworkPytest  = "pytest"
	fileProdMock     = "pkg/container/mock_client.go"
	fileProdMockTS   = "pkg/service/mock_service.ts"
	fileProdMockPy   = "pkg/service/mock_service.py"
	fileTestGo       = "pkg/container/client_test.go"
	fileTestTS       = "pkg/service/service.test.ts"
	fileTestPy       = "pkg/service/test_service.py"
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
			layerDomain: {Paths: []string{pathDomainGlob}},
			moduleInfra: {Paths: []string{pathInfraGlob}},
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
			layerDomain: {Paths: []string{pathDomainGlob}},
			moduleInfra: {Paths: []string{pathInfraGlob}},
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

// ---------------------------------------------------------------------------
// TestInProduction
// ---------------------------------------------------------------------------

// makeTIPConfig builds a RuleConfig with two modules and a test_in_production rule.
func makeTIPConfig(gate string) config.RuleConfig {
	return config.Config{
		Version: 1,
		Modules: map[string]config.ModuleDef{
			"container": {Paths: []string{"pkg/container/**"}},
			"service":   {Paths: []string{"pkg/service/**"}},
		},
		Rules: []config.RuleDef{
			{ID: "tip", Type: typeTestInProduction, Gate: gate},
		},
	}.ForRules()
}

func TestTestInProduction(t *testing.T) {
	emptyGraph := makeGraph(nil)
	ev := func(facts []diagnostic.SyntaxFact) rules.Evidence {
		return rules.Evidence{SyntaxFacts: facts}
	}

	t.Run("prod_go_file_imports_testify_fires", func(t *testing.T) {
		facts := []diagnostic.SyntaxFact{
			{Language: "go", File: fileProdMock, Kind: kindTestImport, Name: frameworkTestify, Framework: frameworkTestify},
		}
		rs, err := rules.New(makeTIPConfig("fail"))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		findings := rs[0].Check(emptyGraph, ev(facts))
		if len(findings) != 1 {
			t.Fatalf("want 1 finding, got %d", len(findings))
		}
		f := findings[0]
		if f.Kind != kindGate {
			t.Errorf("Kind=%q, want %q", f.Kind, kindGate)
		}
		if f.MatchedBy["file"] != fileProdMock {
			t.Errorf("matched_by.file=%q, want %q", f.MatchedBy["file"], fileProdMock)
		}
		if f.MatchedBy["framework"] != frameworkTestify {
			t.Errorf("matched_by.framework=%q, want %q", f.MatchedBy["framework"], frameworkTestify)
		}
		if f.Why == "" {
			t.Error("Why is empty")
		}
		if f.Constraint == "" {
			t.Error("Constraint is empty")
		}
		// endpoint resolves to module path
		if f.Edge.From.Path != "container" {
			t.Errorf("Edge.From.Path=%q, want %q", f.Edge.From.Path, "container")
		}
	})

	t.Run("go_test_file_suppressed", func(t *testing.T) {
		facts := []diagnostic.SyntaxFact{
			{Language: "go", File: fileTestGo, Kind: kindTestImport, Name: frameworkTestify, Framework: frameworkTestify},
		}
		rs, _ := rules.New(makeTIPConfig("fail"))
		if findings := rs[0].Check(emptyGraph, ev(facts)); len(findings) != 0 {
			t.Fatalf("test file: want 0 findings, got %d", len(findings))
		}
	})

	t.Run("ts_prod_file_fires", func(t *testing.T) {
		facts := []diagnostic.SyntaxFact{
			{Language: "typescript", File: fileProdMockTS, Kind: kindTestImport, Name: frameworkJest, Framework: frameworkJest},
		}
		rs, _ := rules.New(makeTIPConfig("fail"))
		findings := rs[0].Check(emptyGraph, ev(facts))
		if len(findings) != 1 {
			t.Fatalf("ts prod: want 1 finding, got %d", len(findings))
		}
	})

	t.Run("ts_test_file_suppressed", func(t *testing.T) {
		facts := []diagnostic.SyntaxFact{
			{Language: "typescript", File: fileTestTS, Kind: kindTestImport, Name: "jest", Framework: "jest"},
		}
		rs, _ := rules.New(makeTIPConfig("fail"))
		if findings := rs[0].Check(emptyGraph, ev(facts)); len(findings) != 0 {
			t.Fatalf("ts test file: want 0 findings, got %d", len(findings))
		}
	})

	t.Run("py_prod_file_fires", func(t *testing.T) {
		facts := []diagnostic.SyntaxFact{
			{Language: "python", File: fileProdMockPy, Kind: kindTestImport, Name: frameworkPytest, Framework: frameworkPytest},
		}
		rs, _ := rules.New(makeTIPConfig("fail"))
		findings := rs[0].Check(emptyGraph, ev(facts))
		if len(findings) != 1 {
			t.Fatalf("py prod: want 1 finding, got %d", len(findings))
		}
	})

	t.Run("py_test_file_suppressed", func(t *testing.T) {
		facts := []diagnostic.SyntaxFact{
			{Language: "python", File: fileTestPy, Kind: kindTestImport, Name: "pytest", Framework: "pytest"},
		}
		rs, _ := rules.New(makeTIPConfig("fail"))
		if findings := rs[0].Check(emptyGraph, ev(facts)); len(findings) != 0 {
			t.Fatalf("py test file: want 0 findings, got %d", len(findings))
		}
	})

	t.Run("non_test_import_kind_ignored", func(t *testing.T) {
		facts := []diagnostic.SyntaxFact{
			{Language: "go", File: fileProdMock, Kind: "function", Name: "Mock", Framework: ""},
		}
		rs, _ := rules.New(makeTIPConfig("fail"))
		if findings := rs[0].Check(emptyGraph, ev(facts)); len(findings) != 0 {
			t.Fatalf("non-test_import kind: want 0 findings, got %d", len(findings))
		}
	})

	t.Run("empty_syntax_facts_returns_nil", func(t *testing.T) {
		rs, _ := rules.New(makeTIPConfig("fail"))
		if findings := rs[0].Check(emptyGraph, ev(nil)); len(findings) != 0 {
			t.Fatalf("empty facts: want 0, got %d", len(findings))
		}
	})

	t.Run("file_outside_module_uses_file_path", func(t *testing.T) {
		outsideFile := "cmd/main.go"
		facts := []diagnostic.SyntaxFact{
			{Language: "go", File: outsideFile, Kind: kindTestImport, Name: frameworkTestify, Framework: frameworkTestify},
		}
		rs, _ := rules.New(makeTIPConfig("fail"))
		findings := rs[0].Check(emptyGraph, ev(facts))
		if len(findings) != 1 {
			t.Fatalf("outside module: want 1 finding, got %d", len(findings))
		}
		if findings[0].Edge.From.Path != outsideFile {
			t.Errorf("Edge.From.Path=%q, want %q", findings[0].Edge.From.Path, outsideFile)
		}
	})

	t.Run("deduplication_same_file_framework", func(t *testing.T) {
		facts := []diagnostic.SyntaxFact{
			{Language: "go", File: fileProdMock, Kind: kindTestImport, Name: frameworkTestify, Framework: frameworkTestify},
			{Language: "go", File: fileProdMock, Kind: kindTestImport, Name: frameworkTestify, Framework: frameworkTestify},
		}
		rs, _ := rules.New(makeTIPConfig("fail"))
		findings := rs[0].Check(emptyGraph, ev(facts))
		if len(findings) != 1 {
			t.Fatalf("dedup: want 1 finding, got %d", len(findings))
		}
	})

	t.Run("gate_semantics", func(t *testing.T) {
		facts := []diagnostic.SyntaxFact{
			{Language: "go", File: fileProdMock, Kind: kindTestImport, Name: frameworkTestify, Framework: frameworkTestify},
		}

		t.Run("off_skips_finding", func(t *testing.T) {
			rs, _ := rules.New(makeTIPConfig("off"))
			if findings := rs[0].Check(emptyGraph, ev(facts)); len(findings) != 0 {
				t.Fatalf("gate:off: want 0, got %d", len(findings))
			}
		})
		t.Run("warn_produces_advisory", func(t *testing.T) {
			rs, _ := rules.New(makeTIPConfig("warn"))
			findings := rs[0].Check(emptyGraph, ev(facts))
			if len(findings) != 1 {
				t.Fatalf("gate:warn: want 1, got %d", len(findings))
			}
			if findings[0].Kind != kindAdvisory {
				t.Errorf("gate:warn Kind=%q, want advisory", findings[0].Kind)
			}
		})
		t.Run("fail_produces_gate_finding", func(t *testing.T) {
			rs, _ := rules.New(makeTIPConfig("fail"))
			findings := rs[0].Check(emptyGraph, ev(facts))
			if len(findings) != 1 {
				t.Fatalf("gate:fail: want 1, got %d", len(findings))
			}
			if findings[0].Kind != kindGate {
				t.Errorf("gate:fail Kind=%q, want gate", findings[0].Kind)
			}
		})
		t.Run("unset_gate_produces_advisory", func(t *testing.T) {
			// test_in_production defaults to warn (advisory) when gate is unset.
			rs, _ := rules.New(makeTIPConfig(""))
			findings := rs[0].Check(emptyGraph, ev(facts))
			if len(findings) != 1 {
				t.Fatalf("gate unset: want 1, got %d", len(findings))
			}
			if findings[0].Kind != kindAdvisory {
				t.Errorf("gate unset Kind=%q, want advisory (warn-by-default)", findings[0].Kind)
			}
		})
	})
}

// ---------------------------------------------------------------------------
// StructFieldMax tests
// ---------------------------------------------------------------------------

const typeStructFieldMax = "struct_field_max"

// makeStructFieldMaxConfig constructs a Config with two modules and a
// struct_field_max rule, then returns the RuleConfig view for rules.New.
func makeStructFieldMaxConfig(ceiling int) config.RuleConfig {
	return config.Config{
		Version: 1,
		Modules: map[string]config.ModuleDef{
			layerDomain: {Paths: []string{pathDomainGlob}},
			moduleInfra: {Paths: []string{pathInfraGlob}},
		},
		Rules: []config.RuleDef{
			{ID: "sf-max", Type: typeStructFieldMax, Max: maxPtr(ceiling)},
		},
	}.ForRules()
}

func TestStructFieldMax(t *testing.T) {
	// struct_field SyntaxFacts: domain has AppState(10 fields), Small(2 fields);
	// infra has Repo(3 fields).
	allFacts := []diagnostic.SyntaxFact{
		{Language: graph.LangGo, File: fileDomainA, Kind: kindStructField, Name: "AppState", Count: 10},
		{Language: graph.LangGo, File: fileDomainB, Kind: kindStructField, Name: "Small", Count: 2},
		{Language: graph.LangGo, File: fileInfraRepo, Kind: kindStructField, Name: nameRepo, Count: 3},
		// non struct_field facts should be ignored
		{Language: graph.LangGo, File: fileDomainA, Kind: kindFunction, Name: nameFuncA, Exported: true},
	}
	emptyGraph := makeGraph(nil)
	ev := func(facts []diagnostic.SyntaxFact) rules.Evidence {
		return rules.Evidence{SyntaxFacts: facts}
	}

	t.Run("under_limit_no_finding", func(t *testing.T) {
		rc := makeStructFieldMaxConfig(10)
		rs, err := rules.New(rc)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if findings := rs[0].Check(emptyGraph, ev(allFacts)); len(findings) != 0 {
			t.Fatalf("want 0 findings at ceiling=10 (AppState=10), got %d", len(findings))
		}
	})

	t.Run("over_limit_emits_finding", func(t *testing.T) {
		rc := makeStructFieldMaxConfig(5)
		rs, err := rules.New(rc)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		findings := rs[0].Check(emptyGraph, ev(allFacts))
		if len(findings) != 1 {
			t.Fatalf("want 1 finding (AppState>5), got %d", len(findings))
		}
		f := findings[0]
		if f.MatchedBy["struct"] != "AppState" {
			t.Errorf("MatchedBy[struct]=%q, want AppState", f.MatchedBy["struct"])
		}
		if f.MatchedBy["count"] != "10" {
			t.Errorf("MatchedBy[count]=%q, want 10", f.MatchedBy["count"])
		}
	})

	t.Run("zero_count_ignored", func(t *testing.T) {
		// Count=0 (tuple/unit struct) must never fire a finding.
		facts := []diagnostic.SyntaxFact{
			{Language: graph.LangGo, File: fileDomainA, Kind: kindStructField, Name: "UnitStruct", Count: 0},
		}
		rc := makeStructFieldMaxConfig(0)
		rs, err := rules.New(rc)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if findings := rs[0].Check(emptyGraph, ev(facts)); len(findings) != 0 {
			t.Fatalf("Count=0 must be ignored, got %d findings", len(findings))
		}
	})

	t.Run("empty_facts_returns_nil", func(t *testing.T) {
		rc := makeStructFieldMaxConfig(1)
		rs, err := rules.New(rc)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if findings := rs[0].Check(emptyGraph, ev(nil)); len(findings) != 0 {
			t.Fatalf("want 0 findings for empty facts, got %d", len(findings))
		}
	})

	t.Run("default_gate_is_warn", func(t *testing.T) {
		// struct_field_max has defaultGateForType="warn".
		rc := makeStructFieldMaxConfig(5) // gate unset → warn
		rs, err := rules.New(rc)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		findings := rs[0].Check(emptyGraph, ev(allFacts))
		if len(findings) != 1 {
			t.Fatalf("want 1 finding, got %d", len(findings))
		}
		if findings[0].Kind != kindAdvisory {
			t.Errorf("gate unset Kind=%q, want advisory (warn-by-default)", findings[0].Kind)
		}
	})
}

func TestStructFieldMax_InvalidConfig(t *testing.T) {
	t.Run("nil_max_returns_error", func(t *testing.T) {
		_, err := rules.New(config.RuleConfig{Rules: []config.RuleDef{
			{ID: "x", Type: typeStructFieldMax},
		}})
		if err == nil {
			t.Fatal("want error for nil Max, got nil")
		}
	})

	t.Run("negative_max_returns_error", func(t *testing.T) {
		_, err := rules.New(config.RuleConfig{Rules: []config.RuleDef{
			{ID: "x", Type: typeStructFieldMax, Max: maxPtr(-1)},
		}})
		if err == nil {
			t.Fatal("want error for negative Max, got nil")
		}
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
		Modules: map[string]config.ModuleDef{
			moduleNameDom: {Paths: []string{"domain/**"}},
		},
		Rules: []config.RuleDef{
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
