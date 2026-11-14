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

func TestPythonReportUnresolvedIsNeverClean(t *testing.T) {
	t.Parallel()
	requireAstGrep(t)
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"pyproject.toml": "", fileFlatAppInit: "",
		"app/core.py": "import app.generated_missing\n",
	})
	ex := registry.New("python", toolrun.New(), evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto}, nil)
	facts, cov, err := ex.Extract(context.Background(), scope.Scope{Root: root})
	if err != nil || cov.Status != evidence.StatusPartial || cov.Unresolved != 1 || cov.UnresolvedInputsMissing != 1 {
		t.Fatalf("coverage=%+v err=%v, want unresolved import, never a clean scan", cov, err)
	}
	if len(facts.Edges) != 1 || facts.Edges[0].To != "module:app.generated_missing" || !strings.Contains(cov.Reason, "app/core.py:1: app.generated_missing") {
		t.Fatalf("facts=%+v coverage=%+v, want the unresolved import and its location retained", facts, cov)
	}
}
