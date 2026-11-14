package acquisition_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/evidence/acquisition"
	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/model/symbol"
	"github.com/alexei-led/archfit/v3/internal/scope"
)

func TestCollectPreservesNativeHintsWithoutSymbolResolver(t *testing.T) {
	const native = "native"
	calls := 0
	resolver := &evidenceports.SymbolResolverMock{
		StrengthsFunc: func(context.Context, scope.Scope) (map[string]string, evidence.Coverage, error) {
			calls++
			return map[string]string{"a.go\x00b.go": "intrusive", "a.cs\x00b.cs": "intrusive"}, evidence.Coverage{Tool: "scip"}, nil
		},
		SymbolsFunc: func(context.Context, scope.Scope) (symbol.Graph, evidence.Coverage, error) {
			calls++
			return symbol.Graph{Module: map[string]string{"sym": "a"}}, evidence.Coverage{Tool: "scip-symbols"}, nil
		},
		ResolveFunc: func(_ context.Context, _, to string) (string, string) {
			calls++
			return to, "high"
		},
	}
	hints := []graph.ConnascenceHint{{Kind: graph.ConnascenceType, Source: "compiler"}}
	edges := []graph.Edge{
		{From: "file:a.go", To: "file:b.go", Kind: graph.EdgeKindImports, Language: graph.LangGo, StrengthHint: "model", DataStrengthHint: "contract", ConnascenceHints: hints},
		{From: "file:a.cs", To: "file:b.cs", Kind: graph.EdgeKindImports, Language: graph.LangDotNet, StrengthHint: "functional", DataStrengthHint: "model", ConnascenceHints: hints},
	}
	got, err := acquisition.Collect(context.Background(), acquisition.Input{
		Resolver:   resolver,
		Extractors: []evidenceports.Extractor{stubExtractor{name: native, coverage: native, facts: graph.Facts{Edges: append([]graph.Edge(nil), edges...)}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Errorf("symbol resolver called %d times, want none", calls)
	}
	if !got.SCIPSymbols.Empty() {
		t.Errorf("symbol graph = %+v, want empty", got.SCIPSymbols)
	}
	if len(got.Coverages) != 1 || got.Coverages[0].Tool != native {
		t.Errorf("coverage = %+v, want only native extractor", got.Coverages)
	}
	want := []graph.Edge{edges[1], edges[0]}
	if !reflect.DeepEqual(got.Graph.Edges(), want) {
		t.Errorf("native edges = %+v, want %+v", got.Graph.Edges(), want)
	}
}
