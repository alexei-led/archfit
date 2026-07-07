package engine

import (
	"context"
	"testing"

	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/ports"
)

func TestEnrichEdges_ScipDoesNotOverrideGoTypeInfoStrength(t *testing.T) {
	facts := graph.Facts{Edges: []graph.Edge{
		{
			From:         "file:pkg/a/a.go",
			To:           "file:pkg/b/b.go",
			Kind:         graph.EdgeKindImports,
			Language:     graph.LangGo,
			StrengthHint: string(coupling.StrengthModel),
		},
		{
			From:     "file:pkg/a/unknown.go",
			To:       "file:pkg/b/func.go",
			Kind:     graph.EdgeKindImports,
			Language: graph.LangGo,
		},
		{
			From:         "file:src/a.ts",
			To:           "file:src/b.ts",
			Kind:         graph.EdgeKindImports,
			Language:     graph.LangTypeScript,
			StrengthHint: string(coupling.StrengthModel),
		},
	}}
	strengths := map[string]string{
		"pkg/a/a.go\x00pkg/b/b.go":          string(coupling.StrengthFunctional),
		"pkg/a/unknown.go\x00pkg/b/func.go": string(coupling.StrengthFunctional),
		"src/a.ts\x00src/b.ts":              string(coupling.StrengthFunctional),
	}

	enrichEdges(context.Background(), ports.NopSymbolResolver{}, strengths, nil, facts)

	if got := facts.Edges[0].StrengthHint; got != string(coupling.StrengthModel) {
		t.Errorf("Go type-info StrengthHint = %q, want model; SCIP must not override compiler-grade Go strength", got)
	}
	if got := facts.Edges[1].StrengthHint; got != string(coupling.StrengthFunctional) {
		t.Errorf("Go edge without type-info StrengthHint = %q, want SCIP functional", got)
	}
	if got := facts.Edges[2].StrengthHint; got != string(coupling.StrengthFunctional) {
		t.Errorf("TypeScript StrengthHint = %q, want SCIP functional overlay", got)
	}
}
