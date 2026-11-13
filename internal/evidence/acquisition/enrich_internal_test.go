package acquisition

import (
	"context"
	"testing"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/relationship/coupling"
)

const (
	testFilePkgA = "file:pkg/a/a.go"
	testFilePkgB = "file:pkg/b/b.go"
	testFileSrcA = "file:src/a.ts"
	testFileSrcB = "file:src/b.ts"
)

const (
	keyGoAB = "pkg/a/a.go\x00pkg/b/b.go"
	keyTSAB = "src/a.ts\x00src/b.ts"
)

func TestEnrichEdges_ScipDoesNotOverrideGoTypeInfoStrength(t *testing.T) {
	facts := graph.Facts{Edges: []graph.Edge{
		{
			From:         testFilePkgA,
			To:           testFilePkgB,
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
			From:         testFileSrcA,
			To:           testFileSrcB,
			Kind:         graph.EdgeKindImports,
			Language:     graph.LangTypeScript,
			StrengthHint: string(coupling.StrengthModel),
		},
	}}
	strengths := map[string]string{
		keyGoAB:                             string(coupling.StrengthFunctional),
		"pkg/a/unknown.go\x00pkg/b/func.go": string(coupling.StrengthFunctional),
		keyTSAB:                             string(coupling.StrengthFunctional),
	}

	enrichEdges(context.Background(), evidenceports.NopSymbolResolver{}, strengths, nil, nil, facts)

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

func TestEnrichEdges_DataStrengthFromScipSkipsGoTypeInfoEdges(t *testing.T) {
	facts := graph.Facts{Edges: []graph.Edge{
		{From: testFilePkgA, To: testFilePkgB, Kind: graph.EdgeKindImports, Language: graph.LangGo, StrengthHint: string(coupling.StrengthFunctional)},
		{From: testFileSrcA, To: testFileSrcB, Kind: graph.EdgeKindImports, Language: graph.LangTypeScript},
		{From: "file:src/c.ts", To: "file:src/d.ts", Kind: graph.EdgeKindImports, Language: graph.LangTypeScript},
	}}
	strengths := map[string]string{
		keyGoAB:                string(coupling.StrengthFunctional),
		keyTSAB:                string(coupling.StrengthFunctional),
		"src/c.ts\x00src/d.ts": string(coupling.StrengthFunctional),
	}
	data := map[string]string{
		keyGoAB: string(coupling.StrengthModel),
		keyTSAB: string(coupling.StrengthModel),
	}

	enrichEdges(context.Background(), evidenceports.NopSymbolResolver{}, strengths, data, nil, facts)

	want := []string{"", string(coupling.StrengthModel), ""}
	for i, w := range want {
		if got := facts.Edges[i].DataStrengthHint; got != w {
			t.Errorf("edge %d DataStrengthHint = %q, want %q", i, got, w)
		}
	}
}
