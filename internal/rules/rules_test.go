package rules_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/rules"
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
			{ID: "no-a-to-b", Type: "forbidden_dependency", From: "services/a/**", To: "services/b/**"},
		},
	}
	ruleSet := rules.New(cfg)
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
				if f.MatchedBy["from_glob"] != "services/a/**" {
					t.Errorf("matched_by.from_glob = %q, want %q", f.MatchedBy["from_glob"], "services/a/**")
				}
				if f.MatchedBy["to_glob"] != "services/b/**" {
					t.Errorf("matched_by.to_glob = %q, want %q", f.MatchedBy["to_glob"], "services/b/**")
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
	ruleSet := rules.New(cfg)
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
	ruleSet := rules.New(rc)
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

func TestNew_UnknownTypeSkipped(t *testing.T) {
	cfg := config.RuleConfig{
		Rules: []config.RuleDef{
			{ID: "known", Type: "public_api_only"},
			{ID: "unknown", Type: "cycle_check_future"},
		},
	}
	ruleSet := rules.New(cfg)
	if len(ruleSet) != 1 {
		t.Fatalf("New: got %d rules, want 1 (unknown type skipped)", len(ruleSet))
	}
	if ruleSet[0].ID() != "known" {
		t.Errorf("rule ID = %q, want %q", ruleSet[0].ID(), "known")
	}
}

func TestNew_EmptyRules(t *testing.T) {
	ruleSet := rules.New(config.RuleConfig{})
	if len(ruleSet) != 0 {
		t.Fatalf("New(empty): got %d rules, want 0", len(ruleSet))
	}
}
