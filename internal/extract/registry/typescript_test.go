package registry_test

import (
	"context"
	"strings"
	"testing"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/extract/registry"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

type typescriptReportRunner struct {
	toolrun.Runner
	t *testing.T
}

func (r typescriptReportRunner) Detect(ctx context.Context, name string) (toolrun.ToolInfo, bool) {
	r.t.Helper()
	if name != "ast-grep" {
		r.t.Fatalf("unexpected TypeScript runtime probe %s", name)
	}
	return r.Runner.Detect(ctx, name)
}

func (r typescriptReportRunner) Run(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
	r.t.Helper()
	if !strings.HasSuffix(cmd.Name, "ast-grep") {
		r.t.Fatalf("unexpected TypeScript runtime %s", cmd.Name)
	}
	return r.Runner.Run(ctx, cmd)
}

func TestTypeScriptReportUsesParserAndDisclosesUnknowns(t *testing.T) {
	t.Parallel()
	requireAstGrep(t)
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"tsconfig.json": `{"extends":"missing-package/config"}`,
		"src/a.ts":      "import './b';\nimport './missing';\nimport 'not-declared';\n",
		"src/b.ts":      "export const b = 1;\n",
	})
	ex := registry.New("typescript", typescriptReportRunner{Runner: toolrun.New(), t: t}, evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto}, nil)
	facts, cov, err := ex.Extract(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if cov.Tool != "tsimports" || cov.Status != evidence.StatusPartial || cov.Unresolved != 3 || cov.UnresolvedInputsMissing != 3 {
		t.Fatalf("coverage = %+v; want 2 unknown imports and 1 unread config", cov)
	}
	for _, item := range []string{"src/a.ts", "./missing", "not-declared", "tsconfig.json", "missing-package/config"} {
		if !strings.Contains(cov.Reason, item) {
			t.Errorf("reason %q omits %q", cov.Reason, item)
		}
	}
	found := false
	for _, edge := range facts.Edges {
		if edge.From == "file:src/a.ts" && edge.To == "file:src/b.ts" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing real first-party edge: %+v", facts.Edges)
	}
	for _, edge := range facts.Edges {
		foundTarget := false
		for _, node := range facts.Nodes {
			if node.ID() == edge.To {
				foundTarget = true
			}
		}
		if !foundTarget {
			t.Errorf("edge target has no node: %+v", edge)
		}
	}
}

func TestTypeScriptReportMissingParserIsNotComplete(t *testing.T) {
	t.Parallel()
	for _, mode := range []evidenceports.ToolMode{evidenceports.ModeAuto, evidenceports.ModeOn} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFiles(t, root, map[string]string{"a.ts": "import './missing';\n"})
			runner := &toolrun.RunnerMock{DetectFunc: func(context.Context, string) (toolrun.ToolInfo, bool) { return toolrun.ToolInfo{}, false }}
			ex := registry.New("typescript", runner, evidenceports.ExtractConfig{Mode: mode}, nil)
			_, cov, err := ex.Extract(context.Background(), scope.Scope{Root: root})
			if (err != nil) != (mode == evidenceports.ModeOn) || cov.Tool != "tsimports" || cov.Status != evidence.StatusPartial || cov.FilesApplicable != 1 || cov.FilesSeen != 0 || !strings.Contains(cov.Reason, "ast-grep") {
				t.Fatalf("coverage=%+v err=%v, want missing parser disclosed", cov, err)
			}
		})
	}
}
