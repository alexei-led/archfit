package py

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
)

const (
	handlersFile   = "src/myapp/handlers.py"
	handlersModule = "module:myapp.handlers"
	domainModule   = "myapp.domain"
)

// srcLayout writes a src-layout tree: grimp builds myapp and lib, and the
// stray top-level tests package is not built (discoverPackages prefers src/).
func srcLayout(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"src/myapp/__init__.py", "src/lib/__init__.py", "tests/__init__.py"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestQueryEdgeSpellsTheExtractedEdge(t *testing.T) {
	t.Parallel()
	root := srcLayout(t)
	cfg := evidenceports.ExtractConfig{Internal: []string{"myapp.domain._*"}}
	for _, tc := range []struct {
		name, from, target, wantFrom, wantTo string
		wantKind                             graph.EdgeKind
	}{
		{name: "dotted target, src layout", from: handlersFile, target: domainModule, wantFrom: handlersModule, wantTo: "module:myapp.domain", wantKind: graph.EdgeKindImports},
		{name: "package file target", from: "src/myapp/api/views.py", target: "src/myapp/domain/__init__.py", wantFrom: "module:myapp.api.views", wantTo: "module:myapp.domain", wantKind: graph.EdgeKindImports},
		{name: "internal target", from: handlersFile, target: "myapp.domain._repo", wantFrom: handlersModule, wantTo: "module:myapp.domain._repo", wantKind: graph.EdgeKindUsesInternal},
		{name: "another built package", from: handlersFile, target: "lib.util", wantFrom: handlersModule, wantTo: "module:lib.util", wantKind: graph.EdgeKindImports},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			facts, err := QueryEdge(root, cfg, tc.from, tc.target)
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
	if _, err := QueryEdge(root, cfg, "src/myapp/handlers.ts", domainModule); err == nil {
		t.Error("a non-Python importer must be an error")
	}
}

func TestQueryEdgeFollowsWhatGrimpBuilds(t *testing.T) {
	t.Parallel()
	root := srcLayout(t)
	for _, tc := range []struct {
		name, from, target string
		cfg                evidenceports.ExtractConfig
		want               error
	}{
		{name: "stdlib target", from: handlersFile, target: "os.path", want: evidenceports.ErrNotDecidable},
		{name: "third-party target", from: handlersFile, target: "requests", want: evidenceports.ErrNotDecidable},
		{name: "importer in a stray package", from: "tests/test_a.py", target: "tests.helpers", want: evidenceports.ErrNotExtracted},
		{name: "importer outside the configured package", from: "src/lib/x.py", target: domainModule, cfg: evidenceports.ExtractConfig{PyPackage: "myapp"}, want: evidenceports.ErrNotExtracted},
		{name: "configured package", from: handlersFile, target: domainModule, cfg: evidenceports.ExtractConfig{PyPackage: "myapp"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := QueryEdge(root, tc.cfg, tc.from, tc.target)
			if tc.want == nil && err != nil || tc.want != nil && !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// TestQueryEdgeLocationFollowsExclusions pins that an importer an exclusion
// glob matches carries no location, as pythonSourceLocations leaves it: the
// rule pass then counts the edge as production, as check does.
func TestQueryEdgeLocationFollowsExclusions(t *testing.T) {
	t.Parallel()
	facts, err := QueryEdge(srcLayout(t), evidenceports.ExtractConfig{Exclusions: []string{"src/myapp/legacy/**"}}, "src/myapp/legacy/old.py", domainModule)
	if err != nil {
		t.Fatal(err)
	}
	if locations := facts.Edges[0].Locations; len(locations) != 0 {
		t.Errorf("locations = %+v, want none for an excluded importer", locations)
	}
}
