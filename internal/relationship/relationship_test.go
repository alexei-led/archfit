package relationship

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"
	"testing"
)

const (
	testBelongsTo  = "belongs_to"
	testImportKind = "imports"
	testModuleA    = "internal/a"
	testModuleB    = "internal/b"
	testNodeA      = "file:a.go"
	testNodeB      = "file:b.go"
)

func TestAnalysisResultOwnsOnlyRelationshipFacts(t *testing.T) {
	data, err := os.ReadFile("relationship.go")
	if err != nil {
		t.Fatal(err)
	}
	tree, err := parser.ParseFile(token.NewFileSet(), "relationship.go", data, 0)
	if err != nil {
		t.Fatal(err)
	}
	// The relationship contract names its own facts. It must not reach for the
	// policy vocabulary, the raw graph, or the classifier's internals — those are
	// the broad type owners that make this contract expensive to depend on.
	forbiddenImports := []string{"internal/policy", "internal/model/graph", "internal/relationship/coupling"}
	for _, imp := range tree.Imports {
		name := strings.Trim(imp.Path.Value, "\"")
		for _, forbidden := range forbiddenImports {
			if strings.Contains(name, forbidden) {
				t.Fatalf("relationship contract imports forbidden broad type owner %s", name)
			}
		}
	}
	for _, decl := range tree.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok.String() != "type" {
			continue
		}
		for _, spec := range gen.Specs {
			ts := spec.(*ast.TypeSpec)
			if ts.Name.Name != "AnalysisResult" {
				continue
			}
			st := ts.Type.(*ast.StructType)
			for _, field := range st.Fields.List {
				for _, name := range field.Names {
					switch name.Name {
					case "Config", "PreAugmentedModules", "CloneEvidence":
						t.Fatalf("AnalysisResult retains broad field %s", name.Name)
					}
				}
			}
		}
	}
}

func TestSetDependencyContract(t *testing.T) {
	dependency := Edge{
		FromID: "package:" + testModuleA, ToID: "package:" + testModuleB, FromPath: testModuleA, ToPath: testModuleB,
		Kind: testImportKind, Strength: StrengthContract, Distance: DistanceCrossModuleSameOwner, Volatility: VolatilityHigh,
	}
	set := Set{
		Nodes: []Node{{ID: dependency.FromID, Module: testModuleA, FirstParty: true}},
		Edges: []Edge{dependency, {FromID: testNodeA, ToID: testNodeB, Kind: "calls"}},
	}

	got := set.DependencyEdges()
	if len(got) != 1 || !reflect.DeepEqual(got[0], dependency) {
		t.Fatalf("DependencyEdges = %+v, want %+v", got, dependency)
	}
	got[0].Kind = "changed"
	if set.Edges[0].Kind != testImportKind {
		t.Fatal("DependencyEdges returned mutable Set storage")
	}
	if !dependency.BoundaryClassified() || !dependency.CrossBoundary() {
		t.Fatalf("classified cross-boundary dependency not recognized: %+v", dependency)
	}
	if !VolatilityResolved(dependency.Volatility) {
		t.Fatalf("volatility %q should be resolved", dependency.Volatility)
	}
}

func TestSetFindingLookupAndModuleKeys(t *testing.T) {
	edge := Edge{FromPath: testModuleA + "/a.go", ToPath: testModuleB + "/b.go", Kind: testImportKind}
	set := Set{Edges: []Edge{edge}}
	got, ok := set.FindByFindingEdge(edge.FromPath, edge.ToPath, edge.Kind)
	if !ok || !reflect.DeepEqual(got, edge) {
		t.Fatalf("FindByFindingEdge = (%+v, %t), want (%+v, true)", got, ok, edge)
	}
	if _, ok := set.FindByFindingEdge(edge.FromPath, edge.ToPath, "calls"); ok {
		t.Fatal("FindByFindingEdge matched a different edge kind")
	}

	for _, test := range []struct {
		id, want string
	}{
		{id: "package:" + testModuleA, want: testModuleA},
		{id: "file:" + testModuleA + "/a.go", want: testModuleA},
		{id: "external:github.com/acme/lib", want: "github.com/acme/lib"},
	} {
		if got := ModuleKey(test.id); got != test.want {
			t.Errorf("ModuleKey(%q) = %q, want %q", test.id, got, test.want)
		}
	}
}

func TestSetCycles(t *testing.T) {
	t.Run("two_node_cycle", func(t *testing.T) {
		set := Set{Edges: []Edge{
			{FromID: testNodeA, ToID: testNodeB, Kind: testImportKind},
			{FromID: testNodeB, ToID: testNodeA, Kind: testImportKind},
		}}
		got := set.Cycles()
		if len(got) != 1 {
			t.Fatalf("Cycles() = %v, want one SCC", got)
		}
		if want := []string{testNodeA, testNodeB}; !reflect.DeepEqual(got[0], want) {
			t.Fatalf("Cycles()[0] = %v, want %v", got[0], want)
		}
	})

	t.Run("acyclic", func(t *testing.T) {
		set := Set{Edges: []Edge{
			{FromID: testNodeA, ToID: testNodeB, Kind: testImportKind},
		}}
		if got := set.Cycles(); len(got) != 0 {
			t.Fatalf("Cycles() = %v, want none", got)
		}
	})

	t.Run("non_dependency_edges_excluded", func(t *testing.T) {
		set := Set{Edges: []Edge{
			{FromID: testNodeA, ToID: testNodeB, Kind: testBelongsTo},
			{FromID: testNodeB, ToID: testNodeA, Kind: testBelongsTo},
		}}
		if got := set.Cycles(); len(got) != 0 {
			t.Fatalf("Cycles() = %v, want none for belongs_to edges", got)
		}
	})

	t.Run("self_loop_not_a_cycle", func(t *testing.T) {
		set := Set{Edges: []Edge{
			{FromID: testNodeA, ToID: testNodeA, Kind: testImportKind},
		}}
		if got := set.Cycles(); len(got) != 0 {
			t.Fatalf("Cycles() = %v, want none for a self-loop", got)
		}
	})

	t.Run("go_file_to_package_edges_never_cycle", func(t *testing.T) {
		set := Set{Edges: []Edge{
			{FromID: "file:billing/app/notify.go", ToID: "package:shipping/api", Kind: testImportKind},
			{FromID: "file:shipping/app/bill.go", ToID: "package:billing/app", Kind: testImportKind},
		}}
		if got := set.Cycles(); len(got) != 0 {
			t.Fatalf("Cycles() = %v, want none: file -> package edges cannot close a node cycle", got)
		}
	})
}

// moduleEdge is a dependency edge between two resolved modules.
func moduleEdge(from, to, fromModule, toModule string) Edge {
	return Edge{FromID: from, ToID: to, FromModule: fromModule, ToModule: toModule, Kind: testImportKind}
}

func TestSetModuleCycles(t *testing.T) {
	const (
		billing  = "billing"
		shipping = "shipping"
		catalog  = "catalog"
	)
	tests := []struct {
		name  string
		edges []Edge
		keep  func(string) bool
		want  [][]string
	}{
		{
			name: "two module cycle through different files and packages",
			edges: []Edge{
				moduleEdge("file:billing/app/notify.go", "package:shipping/api", billing, shipping),
				moduleEdge("file:shipping/app/bill.go", "package:billing/app", shipping, billing),
			},
			want: [][]string{{billing, shipping}},
		},
		{
			name: "three module cycle",
			edges: []Edge{
				moduleEdge("file:a.go", "package:b", billing, shipping),
				moduleEdge("file:b.go", "package:c", shipping, catalog),
				moduleEdge("file:c.go", "package:a", catalog, billing),
			},
			want: [][]string{{billing, catalog, shipping}},
		},
		{
			name: "acyclic module graph",
			edges: []Edge{
				moduleEdge("file:a.go", "package:b", billing, shipping),
				moduleEdge("file:b.go", "package:c", shipping, catalog),
			},
		},
		{
			name: "edges inside one module are not a module cycle",
			edges: []Edge{
				moduleEdge("file:billing/a.go", "package:billing/b", billing, billing),
				moduleEdge("file:billing/b.go", "package:billing/a", billing, billing),
			},
		},
		{
			name: "unowned endpoints never join a cycle",
			edges: []Edge{
				moduleEdge("file:billing/a.go", "package:tools/gen", billing, ""),
				moduleEdge("file:tools/gen/g.go", "package:billing", "", billing),
			},
		},
		{
			name: "non dependency edges are ignored",
			edges: []Edge{
				{FromID: "file:a.go", ToID: "package:b", FromModule: billing, ToModule: shipping, Kind: testBelongsTo},
				{FromID: "file:b.go", ToID: "package:a", FromModule: shipping, ToModule: billing, Kind: testBelongsTo},
			},
		},
		{
			name: "keep drops modules outside the declared set",
			edges: []Edge{
				moduleEdge("package:shop::billing", "package:shop::auto", billing, "shop::auto"),
				moduleEdge("package:shop::auto", "package:shop::billing", "shop::auto", billing),
			},
			keep: func(m string) bool { return m == billing },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Set{Edges: tc.edges}.ModuleCycles(tc.keep)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ModuleCycles() = %v, want %v", got, tc.want)
			}
		})
	}
}
