package py

import (
	"errors"
	"testing"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/model/graph"
)

func TestQueryEdgeSpellsTheExtractedEdge(t *testing.T) {
	t.Parallel()
	cfg := evidenceports.ExtractConfig{Internal: []string{"myapp.domain._*"}}
	for _, tc := range []struct {
		name, from, target, wantFrom, wantTo string
		wantKind                             graph.EdgeKind
	}{
		{name: "dotted target, src layout", from: "src/myapp/handlers.py", target: "myapp.domain", wantFrom: "module:myapp.handlers", wantTo: "module:myapp.domain", wantKind: graph.EdgeKindImports},
		{name: "package file target", from: "myapp/api/views.py", target: "myapp/domain/__init__.py", wantFrom: "module:myapp.api.views", wantTo: "module:myapp.domain", wantKind: graph.EdgeKindImports},
		{name: "internal target", from: "src/myapp/handlers.py", target: "myapp.domain._repo", wantFrom: "module:myapp.handlers", wantTo: "module:myapp.domain._repo", wantKind: graph.EdgeKindUsesInternal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			facts, err := QueryEdge("", cfg, tc.from, tc.target)
			if err != nil {
				t.Fatal(err)
			}
			e := facts.Edges[0]
			if e.From != tc.wantFrom || e.To != tc.wantTo || e.Kind != tc.wantKind || e.Language != langPython {
				t.Errorf("edge = %+v", e)
			}
			if len(e.Locations) != 1 || e.Locations[0].File != tc.from {
				t.Errorf("locations = %+v, want the importing file", e.Locations)
			}
		})
	}
	for _, external := range []string{"requests", "os.path"} {
		if _, err := QueryEdge("", cfg, "src/myapp/handlers.py", external); !errors.Is(err, evidenceports.ErrNotDecidable) {
			t.Errorf("an import of %s outside the first-party package must not be decidable: %v", external, err)
		}
	}
	if _, err := QueryEdge("", evidenceports.ExtractConfig{PyPackage: "src/core"}, "app/main.py", "core.models"); err != nil {
		t.Errorf("an import of the configured package is first-party: %v", err)
	}
	if _, err := QueryEdge("", cfg, "src/myapp/handlers.ts", "myapp.domain"); err == nil {
		t.Error("a non-Python importer must be an error")
	}
}
