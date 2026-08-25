package analysis_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/module"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship/analysis"
)

func TestAnalyzeReturnsRelationshipContract(t *testing.T) {
	g := graph.Build([]graph.Facts{{
		Nodes: []graph.Node{{Kind: graph.NodeKindFile, Path: "a/a.go", Language: graph.LangGo}, {Kind: graph.NodeKindFile, Path: "b/b.go", Language: graph.LangGo}},
		Edges: []graph.Edge{{From: "file:a/a.go", To: "file:b/b.go", Kind: graph.EdgeKindImports, Language: graph.LangGo}},
	}})
	modules := map[string]module.ModuleDef{
		"a": {Paths: []string{"a/**"}, Owner: "team-a", DeployUnit: "a", Subdomain: "core"},
		"b": {Paths: []string{"b/**"}, Owner: "team-b", DeployUnit: "b", Subdomain: "supporting"},
	}
	got := analysis.Analyze(analysis.Input{Graph: g, Policy: policy.RelationshipPolicy{Topology: policy.TopologyView{Modules: modules, ModuleMap: module.BuildMap(modules)}}})
	if len(got.Relationships.Edges) != 1 {
		t.Fatalf("relationship edges = %d, want 1", len(got.Relationships.Edges))
	}
	if got.Relationships.Edges[0].FromModule != "a" || got.Relationships.Edges[0].ToModule != "b" {
		t.Fatalf("edge modules = %+v", got.Relationships.Edges[0])
	}
}
