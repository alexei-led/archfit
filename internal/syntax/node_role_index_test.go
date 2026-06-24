package syntax_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/syntax"
)

// Export constants from the syntax package for use in tests (black-box style).
// These shadow the package-level constants so test assertions are readable.
const (
	RoleHandler    = syntax.RoleHandler
	RoleService    = syntax.RoleService
	RoleRepository = syntax.RoleRepository
	RoleDomain     = syntax.RoleDomain
	ConfHigh       = syntax.ConfHigh
	ConfMedium     = syntax.ConfMedium
	ConfLow        = syntax.ConfLow
)

// file path constants reused across index test cases (goconst threshold = 3).
const (
	fileTSHandler  = "src/handlers/user.ts"
	fileTSService  = "src/services/auth.ts"
	fileGoHandler1 = "internal/handler/user.go"
	fileGoHandler2 = "internal/handler/health.go"
	fileSortTS     = "src/a.ts"
)

// buildGraph constructs a minimal Graph from a Facts slice (one language).
func buildGraph(facts graph.Facts) *graph.Graph {
	return graph.Build([]graph.Facts{facts})
}

// TestBuildNodeRoleIndex_TypeScript: TS file→file identity join.
func TestBuildNodeRoleIndex_TypeScript(t *testing.T) {
	g := buildGraph(graph.Facts{
		Language: graph.LangTypeScript,
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: fileTSHandler},
			{Kind: graph.NodeKindFile, Path: fileTSService},
		},
		Edges: []graph.Edge{
			{From: "file:" + fileTSHandler, To: "file:" + fileTSService, Kind: graph.EdgeKindImports, Language: graph.LangTypeScript},
		},
	})

	facts := []diagnostic.SyntaxFact{
		// role derivable at high confidence (framework route)
		{Language: graph.LangTypeScript, File: fileTSHandler, Kind: kindRoute, Name: "GET /users", Role: RoleHandler, RoleConf: ConfHigh, Evidence: kindRoute},
		// role at medium confidence (name suffix)
		{Language: graph.LangTypeScript, File: fileTSService, Kind: kindClass, Name: "AuthService", Role: RoleService, RoleConf: ConfMedium, Evidence: "name suffix AuthService"},
		// fact without a role — must not be indexed
		{Language: graph.LangTypeScript, File: fileTSHandler, Kind: kindFunction, Name: "helper"},
	}

	idx := syntax.BuildNodeRoleIndex(g, facts)

	t.Run("handler file resolves", func(t *testing.T) {
		hits := idx.RolesFor("file:" + fileTSHandler)
		if len(hits) != 1 {
			t.Fatalf("want 1 hit, got %d: %+v", len(hits), hits)
		}
		if hits[0].Role != RoleHandler || hits[0].Confidence != ConfHigh {
			t.Errorf("unexpected hit: %+v", hits[0])
		}
	})

	t.Run("service file resolves", func(t *testing.T) {
		hits := idx.RolesFor("file:" + fileTSService)
		if len(hits) != 1 {
			t.Fatalf("want 1 hit, got %d: %+v", len(hits), hits)
		}
		if hits[0].Role != RoleService {
			t.Errorf("unexpected role: %+v", hits[0])
		}
	})

	t.Run("no-role fact not indexed", func(t *testing.T) {
		hits := idx.RolesFor("file:" + fileTSHandler)
		for _, h := range hits {
			if h.Role == "" {
				t.Error("empty-role hit should not be indexed")
			}
		}
	})

	t.Run("unknown node returns nil", func(t *testing.T) {
		if hits := idx.RolesFor("file:nonexistent.ts"); hits != nil {
			t.Errorf("expected nil, got %v", hits)
		}
	})
}

// TestBuildNodeRoleIndex_Go: Go files aggregate to package node.
func TestBuildNodeRoleIndex_Go(t *testing.T) {
	// Go package node path = stripped import path = directory in standard layout.
	g := buildGraph(graph.Facts{
		Language: graph.LangGo,
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: fileGoHandler1},
			{Kind: graph.NodeKindFile, Path: fileGoHandler2},
			{Kind: graph.NodeKindPackage, Path: "internal/handler"},
			{Kind: graph.NodeKindPackage, Path: "internal/repository"},
		},
		Edges: []graph.Edge{
			{From: "file:" + fileGoHandler1, To: "package:internal/repository", Kind: graph.EdgeKindImports, Language: graph.LangGo},
		},
	})

	facts := []diagnostic.SyntaxFact{
		// Two files in the same package → roles aggregate onto the package node.
		{Language: graph.LangGo, File: fileGoHandler1, Kind: kindFunction, Name: "UserHandler", Role: RoleHandler, RoleConf: ConfMedium, Evidence: "name suffix UserHandler"},
		{Language: graph.LangGo, File: fileGoHandler2, Kind: kindFunction, Name: "HealthHandler", Role: RoleHandler, RoleConf: ConfMedium, Evidence: "name suffix HealthHandler"},
		// Duplicate (same role+conf) → should be deduplicated.
		{Language: graph.LangGo, File: fileGoHandler1, Kind: kindFunction, Name: "AnotherHandler", Role: RoleHandler, RoleConf: ConfMedium, Evidence: "name suffix AnotherHandler"},
	}

	idx := syntax.BuildNodeRoleIndex(g, facts)

	t.Run("package node gets handler role", func(t *testing.T) {
		hits := idx.RolesFor("package:internal/handler")
		if len(hits) == 0 {
			t.Fatal("expected at least one hit for package node, got none")
		}
		if hits[0].Role != RoleHandler {
			t.Errorf("expected handler role, got %q", hits[0].Role)
		}
	})

	t.Run("duplicate role+conf deduplicated", func(t *testing.T) {
		hits := idx.RolesFor("package:internal/handler")
		for i, h := range hits {
			for j, h2 := range hits {
				if i != j && h.Role == h2.Role && h.Confidence == h2.Confidence {
					t.Errorf("duplicate (role=%q, conf=%q) at indices %d and %d", h.Role, h.Confidence, i, j)
				}
			}
		}
	})

	t.Run("file node itself has no hits", func(t *testing.T) {
		// Rules operate on package nodes in Go graphs; file nodes are structural.
		// NodeRoleIndex attaches to the package, not the file.
		hits := idx.RolesFor("file:" + fileGoHandler1)
		if len(hits) != 0 {
			t.Errorf("expected no hits on Go file node, got %v", hits)
		}
	})

	t.Run("non-matching package has no hits", func(t *testing.T) {
		if hits := idx.RolesFor("package:internal/repository"); hits != nil {
			t.Errorf("expected nil for unmatched package, got %v", hits)
		}
	})
}

// TestBuildNodeRoleIndex_Python: dotted module key join.
func TestBuildNodeRoleIndex_Python(t *testing.T) {
	// Python graph nodes are "module:<dotted>".
	g := buildGraph(graph.Facts{
		Language: graph.LangPython,
		Nodes: []graph.Node{
			// dotted module path as node Path
			{Kind: graph.NodeKindModule, Path: "billing.service"},
			{Kind: graph.NodeKindModule, Path: "billing.repository"},
		},
		Edges: []graph.Edge{
			{From: "module:billing.service", To: "module:billing.repository", Kind: graph.EdgeKindImports, Language: graph.LangPython},
		},
	})

	facts := []diagnostic.SyntaxFact{
		// File is slash path; must be normalized to dotted for node lookup.
		{Language: graph.LangPython, File: "billing/service.py", Kind: kindClass, Name: "BillingService", Role: RoleService, RoleConf: ConfMedium, Evidence: "name suffix BillingService"},
		{Language: graph.LangPython, File: "billing/repository.py", Kind: kindClass, Name: "BillingRepository", Role: RoleRepository, RoleConf: ConfMedium, Evidence: "name suffix BillingRepository"},
		// __init__.py → dotted parent module (billing.service).
		{Language: graph.LangPython, File: "billing/service/__init__.py", Kind: kindClass, Name: "ServiceInit", Role: RoleService, RoleConf: ConfLow, Evidence: "path contains service"},
	}

	idx := syntax.BuildNodeRoleIndex(g, facts)

	t.Run("dotted service module resolves", func(t *testing.T) {
		hits := idx.RolesFor("module:billing.service")
		if len(hits) == 0 {
			t.Fatal("expected hits for billing.service, got none")
		}
		if hits[0].Role != RoleService {
			t.Errorf("expected service role, got %q", hits[0].Role)
		}
	})

	t.Run("dotted repository module resolves", func(t *testing.T) {
		hits := idx.RolesFor("module:billing.repository")
		if len(hits) == 0 {
			t.Fatal("expected hits for billing.repository, got none")
		}
		if hits[0].Role != RoleRepository {
			t.Errorf("expected repository role, got %q", hits[0].Role)
		}
	})

	t.Run("init.py resolves to parent dotted module", func(t *testing.T) {
		// billing/service/__init__.py normalizes to "billing.service" which IS
		// in the graph — so it should resolve and merge with other service hits.
		hits := idx.RolesFor("module:billing.service")
		if len(hits) == 0 {
			t.Error("expected hits after __init__.py resolution")
		}
	})
}

// TestBuildNodeRoleIndex_Rust: crate::mod file→node resolution.
func TestBuildNodeRoleIndex_Rust(t *testing.T) {
	// Rust graph nodes are "package:<crate::mod>" (from cargo-modules).
	crates := []graph.CrateRoot{
		{Dir: "crates/myapp", Name: "myapp"},
	}
	g := graph.Build([]graph.Facts{{
		Language: graph.LangRust,
		Nodes: []graph.Node{
			{Kind: graph.NodeKindPackage, Path: "myapp"},
			{Kind: graph.NodeKindPackage, Path: "myapp::api"},
			{Kind: graph.NodeKindPackage, Path: "myapp::db"},
		},
		Edges: []graph.Edge{
			{From: "package:myapp::api", To: "package:myapp::db", Kind: graph.EdgeKindDependsOn, Language: graph.LangRust},
		},
		CrateRoots: crates,
	}})

	facts := []diagnostic.SyntaxFact{
		// crates/myapp/src/api.rs → myapp::api (via RustFileToModuleKey with crateRoots)
		{Language: graph.LangRust, File: "crates/myapp/src/api.rs", Kind: kindFunction, Name: "handle_request", Role: RoleHandler, RoleConf: ConfMedium, Evidence: "name suffix handle_request"},
		// crates/myapp/src/db.rs → myapp::db
		{Language: graph.LangRust, File: "crates/myapp/src/db.rs", Kind: kindStruct, Name: "UserRepository", Role: RoleRepository, RoleConf: ConfMedium, Evidence: "name suffix UserRepository"},
	}

	idx := syntax.BuildNodeRoleIndex(g, facts)

	t.Run("api handler module resolves via crateRoots", func(t *testing.T) {
		hits := idx.RolesFor("package:myapp::api")
		if len(hits) == 0 {
			t.Fatal("expected hits for myapp::api, got none")
		}
		if hits[0].Role != RoleHandler {
			t.Errorf("expected handler, got %q", hits[0].Role)
		}
	})

	t.Run("db repository module resolves via crateRoots", func(t *testing.T) {
		hits := idx.RolesFor("package:myapp::db")
		if len(hits) == 0 {
			t.Fatal("expected hits for myapp::db, got none")
		}
		if hits[0].Role != RoleRepository {
			t.Errorf("expected repository, got %q", hits[0].Role)
		}
	})
}

// TestBuildNodeRoleIndex_ConfidenceSorting: hits sorted high→medium→low within role.
func TestBuildNodeRoleIndex_ConfidenceSorting(t *testing.T) {
	g := buildGraph(graph.Facts{
		Language: graph.LangTypeScript,
		Nodes:    []graph.Node{{Kind: graph.NodeKindFile, Path: fileSortTS}},
	})

	facts := []diagnostic.SyntaxFact{
		{Language: graph.LangTypeScript, File: fileSortTS, Kind: kindFunction, Name: "aHandler", Role: RoleHandler, RoleConf: ConfLow, Evidence: "path"},
		{Language: graph.LangTypeScript, File: fileSortTS, Kind: kindClass, Name: "AService", Role: RoleService, RoleConf: ConfHigh, Evidence: "decorator"},
		{Language: graph.LangTypeScript, File: fileSortTS, Kind: kindFunction, Name: "doService", Role: RoleService, RoleConf: ConfMedium, Evidence: "name"},
	}

	idx := syntax.BuildNodeRoleIndex(g, facts)
	hits := idx.RolesFor("file:" + fileSortTS)
	if len(hits) != 3 {
		t.Fatalf("want 3 hits (handler/low, service/high, service/medium), got %d: %+v", len(hits), hits)
	}
	// sorted: handler/low, service/high, service/medium
	// role order: "handler" < "service"; within service: high before medium
	if hits[0].Role != RoleHandler {
		t.Errorf("slot 0: want handler, got %q", hits[0].Role)
	}
	if hits[1].Role != RoleService || hits[1].Confidence != ConfHigh {
		t.Errorf("slot 1: want service/high, got %q/%q", hits[1].Role, hits[1].Confidence)
	}
	if hits[2].Role != RoleService || hits[2].Confidence != ConfMedium {
		t.Errorf("slot 2: want service/medium, got %q/%q", hits[2].Role, hits[2].Confidence)
	}
}

// TestBuildNodeRoleIndex_EmptyInputs: both nil and empty facts are safe.
func TestBuildNodeRoleIndex_EmptyInputs(t *testing.T) {
	g := buildGraph(graph.Facts{Language: graph.LangGo})
	idx := syntax.BuildNodeRoleIndex(g, nil)
	if hits := idx.RolesFor("package:anything"); hits != nil {
		t.Errorf("expected nil, got %v", hits)
	}
	idx2 := syntax.BuildNodeRoleIndex(g, []diagnostic.SyntaxFact{})
	if hits := idx2.RolesFor("package:anything"); hits != nil {
		t.Errorf("expected nil, got %v", hits)
	}
}
