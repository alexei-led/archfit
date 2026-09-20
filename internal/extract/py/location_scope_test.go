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

func TestExtractLocationsRespectSourceScope(t *testing.T) {
	const source = "app/logic.py"
	for _, tc := range []struct {
		name       string
		copy       string
		exclusions []string
		unreadable bool
		want       string
	}{
		{"build copy", "build/lib/app/logic.py", nil, false, source},
		{"fixture copy", "testdata/app/logic.py", nil, false, source},
		{"excluded copy", "mirror/app/logic.py", []string{"mirror/**"}, false, source},
		{"different nested module", "app/plugins/app/logic.py", nil, false, source},
		{"excluded source", "", []string{"app/**"}, false, ""},
		{"unreadable unrelated directory", "", nil, true, source},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, file := range []string{source, tc.copy, "pyproject.toml"} {
				if file == "" {
					continue
				}
				full := filepath.Join(root, file)
				if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.unreadable {
				private := filepath.Join(root, "private")
				if err := os.Mkdir(private, 0o000); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := os.Remove(private); err != nil {
						t.Error(err)
					}
				})
				if _, err := os.ReadDir(private); err == nil {
					t.Skip("runtime bypasses directory permissions")
				}
			}
			runner := &toolrun.RunnerMock{
				DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) { return toolrun.ToolInfo{Name: "uv"}, true },
				RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
					return toolrun.Output{Stdout: []byte(`{"edges":[{"importer":"app.logic","imported":"app.domain","line":7}]}`)}, nil
				},
			}
			facts, _, err := py.New(runner, evidenceports.ExtractConfig{Mode: evidenceports.ModeOn, PyPackage: "app", Exclusions: tc.exclusions}).Extract(context.Background(), scope.Scope{Root: root})
			if err != nil {
				t.Fatal(err)
			}
			if len(facts.Edges) != 1 {
				t.Fatalf("edges = %+v", facts.Edges)
			}
			locations := facts.Edges[0].Locations
			if tc.want == "" {
				if len(locations) != 0 {
					t.Fatalf("excluded source location = %+v", locations)
				}
			} else if len(locations) != 1 || locations[0].File != tc.want {
				t.Fatalf("locations = %+v, want %s", locations, tc.want)
			}
		})
	}
}

func TestExtractLocationsPreserveEffectiveReinclusions(t *testing.T) {
	const source = "reports/api.py"
	for _, tc := range []struct {
		name         string
		configured   []string
		wantLocation bool
	}{
		{"default exclusion", nil, false},
		{"explicit re-inclusion", []string{"!reports"}, true},
		{"explicit exclusion restored", []string{"!reports", "reports/**"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "reports"), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, source), nil, 0o600); err != nil {
				t.Fatal(err)
			}
			runner := &toolrun.RunnerMock{
				DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) { return toolrun.ToolInfo{Name: "uv"}, true },
				RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
					return toolrun.Output{Stdout: []byte(`{"edges":[{"importer":"reports.api","imported":"reports.domain","line":9}]}`)}, nil
				},
			}
			cfg := evidenceports.ExtractConfig{Mode: evidenceports.ModeOn, PyPackage: "reports", Exclusions: scope.MergeExclusions(tc.configured)}
			facts, _, err := py.New(runner, cfg).Extract(context.Background(), scope.Scope{Root: root})
			if err != nil {
				t.Fatal(err)
			}
			if len(facts.Edges) != 1 {
				t.Fatalf("edges = %+v", facts.Edges)
			}
			locations := facts.Edges[0].Locations
			if !tc.wantLocation {
				if len(locations) != 0 {
					t.Fatalf("excluded location = %+v", locations)
				}
			} else if len(locations) != 1 || locations[0].File != source || locations[0].Line != 9 {
				t.Fatalf("re-included location = %+v, want %s:9", locations, source)
			}
		})
	}
}
