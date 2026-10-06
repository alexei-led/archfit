package golang

import (
	"os"
	"path/filepath"
	"testing"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/model/graph"
)

const queryTargetDir = "internal/b"

func TestQueryEdgeSpellsTheExtractedEdge(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for name, body := range map[string]string{
		"go.mod":           "module example.com/q\n\ngo 1.21\n",
		"tools/go.mod":     "module example.com/q/tools\n\ngo 1.21\n",
		"internal/a/a.go":  "package a\n",
		"tools/gen/gen.go": "package gen\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name, target, wantTo string
		wantKind             graph.EdgeKind
	}{
		{name: "repo-relative dir", target: queryTargetDir, wantTo: queryTargetDir, wantKind: graph.EdgeKindImports},
		{name: "import path", target: "example.com/q/internal/b", wantTo: queryTargetDir, wantKind: graph.EdgeKindImports},
		{name: "nested member import path", target: "example.com/q/tools/gen", wantTo: "tools/gen", wantKind: graph.EdgeKindImports},
		{name: "root package", target: "example.com/q", wantTo: "", wantKind: graph.EdgeKindImports},
		{name: "go internal segment", target: "pkg/b/internal/x", wantTo: "pkg/b/internal/x", wantKind: graph.EdgeKindUsesInternal},
		{name: "external package", target: "github.com/x/y", wantTo: "github.com/x/y", wantKind: graph.EdgeKindImports},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			facts, err := QueryEdge(root, evidenceports.ExtractConfig{}, "internal/a/a.go", tc.target)
			if err != nil {
				t.Fatal(err)
			}
			e := facts.Edges[0]
			if e.From != "file:internal/a/a.go" || e.To != "package:"+tc.wantTo || e.Kind != tc.wantKind || e.Language != graph.LangGo {
				t.Errorf("edge = %+v", e)
			}
		})
	}
	if _, err := QueryEdge(root, evidenceports.ExtractConfig{}, "internal/a/a.ts", "internal/b"); err == nil {
		t.Error("a non-Go importer must be an error")
	}
}
