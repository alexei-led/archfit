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

	overlay := enrichEdges(context.Background(), ports.NopSymbolResolver{}, strengths, nil, facts)

	if got := facts.Edges[0].StrengthHint; got != string(coupling.StrengthModel) {
		t.Errorf("Go type-info StrengthHint = %q, want model; SCIP must not override compiler-grade Go strength", got)
	}
	if got := facts.Edges[1].StrengthHint; got != string(coupling.StrengthFunctional) {
		t.Errorf("Go edge without type-info StrengthHint = %q, want SCIP functional", got)
	}
	if got := facts.Edges[2].StrengthHint; got != string(coupling.StrengthFunctional) {
		t.Errorf("TypeScript StrengthHint = %q, want SCIP functional overlay", got)
	}
	report := overlay.report()
	if report == nil {
		t.Fatal("overlay report = nil, want TypeScript counters")
	}
	gotTS := report.ByLanguage[graph.LangTypeScript]
	if gotTS.CandidateEdges != 1 || gotTS.Applied != 1 || gotTS.Missed != 0 {
		t.Errorf("TypeScript overlay = %+v, want 1 candidate, 1 applied, 0 missed", gotTS)
	}
	if _, ok := report.ByLanguage[graph.LangGo]; ok {
		t.Errorf("Go overlay counters should be omitted; got %+v", report.ByLanguage[graph.LangGo])
	}
}

func TestEnrichEdges_SemanticOverlayCountsHitsAndMissesByLanguage(t *testing.T) {
	facts := graph.Facts{Edges: []graph.Edge{
		{From: "file:src/a.ts", To: "file:src/b.ts", Kind: graph.EdgeKindImports, Language: graph.LangTypeScript},
		{From: "package:myapp.api", To: "package:myapp.services", Kind: graph.EdgeKindImports, Language: graph.LangPython},
		{From: "module:demo::api", To: "module:demo::core", Kind: graph.EdgeKindImports, Language: graph.LangRust},
		{From: "file:src/missed.ts", To: "file:src/unknown.ts", Kind: graph.EdgeKindImports, Language: graph.LangTypeScript},
	}}
	strengths := map[string]string{
		"src/a.ts\x00src/b.ts":        string(coupling.StrengthModel),
		"myapp.api\x00myapp.services": string(coupling.StrengthIntrusive),
		"demo::api\x00demo::core":     string(coupling.StrengthFunctional),
	}

	report := enrichEdges(context.Background(), ports.NopSymbolResolver{}, strengths, nil, facts).report()
	if report == nil {
		t.Fatal("overlay report = nil")
	}

	tests := []struct {
		language                               string
		wantCandidate, wantApplied, wantMissed int
		wantAfter                              string
	}{
		{graph.LangTypeScript, 2, 1, 1, string(coupling.StrengthModel)},
		{graph.LangPython, 1, 1, 0, string(coupling.StrengthIntrusive)},
		{graph.LangRust, 1, 1, 0, string(coupling.StrengthFunctional)},
	}
	for _, tt := range tests {
		got := report.ByLanguage[tt.language]
		if got.CandidateEdges != tt.wantCandidate || got.Applied != tt.wantApplied || got.Missed != tt.wantMissed {
			t.Errorf("%s overlay = %+v, want candidate/applied/missed %d/%d/%d", tt.language, got, tt.wantCandidate, tt.wantApplied, tt.wantMissed)
		}
		if got.After[tt.wantAfter] == 0 {
			t.Errorf("%s overlay after[%s] = 0, want counted", tt.language, tt.wantAfter)
		}
	}
	if got := facts.Edges[3].StrengthHint; got != "" {
		t.Errorf("missed TypeScript StrengthHint = %q, want empty", got)
	}
	if got := report.ByLanguage[graph.LangTypeScript].After[unknownStrength]; got != 1 {
		t.Errorf("TypeScript after[unknown] = %d, want 1 missed key counted", got)
	}
}
