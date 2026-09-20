package py_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/extract/py"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

func TestExtractResolvesPhysicalLocations(t *testing.T) {
	const flatFile, srcFile = "app/logic.py", "src/app/logic.py"
	for _, tc := range []struct {
		name  string
		files []string
		want  string
	}{
		{"flat module", []string{flatFile}, flatFile},
		{"src module", []string{srcFile}, srcFile},
		{"package initializer", []string{"src/app/logic/__init__.py"}, "src/app/logic/__init__.py"},
		{"unsupported nested source root", []string{"services/api/src/app/logic.py"}, ""},
		{"ambiguous roots", []string{flatFile, srcFile}, ""},
		{"missing source", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, file := range append(tc.files, "pyproject.toml") {
				full := filepath.Join(root, file)
				if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			runner := &toolrun.RunnerMock{
				DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) { return toolrun.ToolInfo{Name: "uv"}, true },
				RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
					return toolrun.Output{Stdout: []byte(`{"edges":[{"importer":"app.logic","imported":"app.domain","line":7}],"unresolved":1,"unresolved_imports":[{"importer":"app.logic","imported":"external","line":9}]}`)}, nil
				},
			}
			facts, _, err := py.New(runner, evidenceports.ExtractConfig{Mode: evidenceports.ModeOn, PyPackage: "app"}).Extract(context.Background(), scope.Scope{Root: root})
			if err != nil {
				t.Fatal(err)
			}
			if len(facts.Edges) != 2 {
				t.Fatalf("edges = %+v", facts.Edges)
			}
			for _, edge := range facts.Edges {
				if tc.want == "" {
					if len(edge.Locations) != 0 {
						t.Errorf("invented location: %+v", edge.Locations)
					}
					continue
				}
				if len(edge.Locations) != 1 || edge.Locations[0].File != tc.want {
					t.Errorf("locations = %+v, want %s", edge.Locations, tc.want)
					continue
				}
				wantLine := 7
				if edge.To == "external:external" {
					wantLine = 9
				}
				if edge.Locations[0].Line != wantLine {
					t.Errorf("line = %d, want %d", edge.Locations[0].Line, wantLine)
				}
			}
		})
	}
}
