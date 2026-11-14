package registry_test

import (
	"context"
	"strings"
	"testing"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/extract/registry"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const (
	fileAppInit     = "src/app/__init__.py"
	fileFlatAppInit = "app/__init__.py"
)

type pythonReportRunner struct {
	toolrun.Runner
	t *testing.T
}

func (r pythonReportRunner) Detect(ctx context.Context, name string) (toolrun.ToolInfo, bool) {
	r.t.Helper()
	if name != registry.ToolAstGrep {
		r.t.Fatalf("Python report probed a runtime: %s", name)
	}
	return r.Runner.Detect(ctx, name)
}

func (r pythonReportRunner) Run(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
	r.t.Helper()
	if !strings.HasSuffix(cmd.Name, "ast-grep") {
		r.t.Fatalf("Python report ran a runtime: %s", cmd.Name)
	}
	return r.Runner.Run(ctx, cmd)
}

func TestPythonReportUsesParser(t *testing.T) {
	t.Parallel()
	requireAstGrep(t)
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"pyproject.toml":             "",
		fileAppInit:                  "",
		"src/app/core.py":            "from app.plugins import adapter\nimport os\nimport app.missing\n",
		"src/app/plugins/adapter.py": "",
	})
	runner := pythonReportRunner{Runner: toolrun.New(), t: t}
	ex := registry.New("python", runner, evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto}, nil)
	facts, cov, err := ex.Extract(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if cov.Tool != "pyimports" || cov.Status != evidence.StatusPartial || cov.Unresolved != 1 || cov.UnresolvedInputsMissing != 1 {
		t.Fatalf("coverage = %+v, want one unresolved Python parser import", cov)
	}
	if !strings.Contains(cov.Reason, "src/app/core.py:3") || !strings.Contains(cov.Reason, "app.missing") {
		t.Errorf("unresolved import has no source location: %+v", cov)
	}
	found := map[string]graph.Edge{}
	for _, edge := range facts.Edges {
		found[edge.To] = edge
	}
	for _, tc := range []struct {
		target     string
		line       int
		confidence string
	}{
		{"module:app.plugins.adapter", 1, "high"},
		{"external:os", 2, "low"},
		{"module:app.missing", 3, "low"},
	} {
		edge, ok := found[tc.target]
		if !ok || edge.From != "module:app.core" || edge.Confidence != tc.confidence || len(edge.Locations) != 1 || edge.Locations[0].File != "src/app/core.py" || edge.Locations[0].Line != tc.line {
			t.Errorf("edge to %s = %+v, want located Python import", tc.target, edge)
		}
	}
}

func TestPythonReportFailureAndApplicability(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		cfg   evidenceports.ExtractConfig
		files map[string]string
		want  string
	}{
		{"missing parser", evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto}, map[string]string{fileFlatAppInit: ""}, evidence.StatusAbsent},
		{"disabled", evidenceports.ExtractConfig{Mode: evidenceports.ModeOff}, map[string]string{fileFlatAppInit: ""}, evidence.StatusAbsent},
		{"loose scripts", evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto}, map[string]string{"script.py": "import os\n"}, evidence.StatusAbsent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFiles(t, root, tc.files)
			runner := &toolrun.RunnerMock{DetectFunc: func(context.Context, string) (toolrun.ToolInfo, bool) { return toolrun.ToolInfo{}, false }}
			ex := registry.New("python", runner, tc.cfg, nil)
			_, cov, err := ex.Extract(context.Background(), scope.Scope{Root: root})
			if err != nil || cov.Status != tc.want || cov.Tool != "pyimports" {
				t.Fatalf("coverage=%+v err=%v, want %s", cov, err, tc.want)
			}
		})
	}
}
