package ts

import (
	"testing"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/model/graph"
)

func TestQueryEdgeSpellsTheExtractedEdge(t *testing.T) {
	t.Parallel()
	cfg := evidenceports.ExtractConfig{Internal: []string{"web/db/internal/**"}}
	for _, tc := range []struct {
		name, from, target string
		wantKind           graph.EdgeKind
	}{
		{name: "public target", from: "web/app.ts", target: "web/db/client.ts", wantKind: graph.EdgeKindImports},
		{name: "internal target", from: "web/app.tsx", target: "web/db/internal/pool.ts", wantKind: graph.EdgeKindUsesInternal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			facts, err := QueryEdge("", cfg, tc.from, tc.target)
			if err != nil {
				t.Fatal(err)
			}
			e := facts.Edges[0]
			if e.From != "file:"+tc.from || e.To != "file:"+tc.target || e.Kind != tc.wantKind || e.Language != langTS || len(e.Locations) != 0 {
				t.Errorf("edge = %+v", e)
			}
		})
	}
	if _, err := QueryEdge("", cfg, "web/app.ts", "web/db"); err == nil {
		t.Error("a target that is not a source file must be an error")
	}
}
