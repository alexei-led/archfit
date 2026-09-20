package py_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/extract/registry"
	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

func TestReviewPythonCacheProducerDrift(t *testing.T) {
	root := writeProfileCacheFixture(t)
	helperJSON := `{"edges":[{"importer":"pkg","imported":"pkg.a","line":1}],"unresolved":0,"producer_version":"grimp 3.16; python 3.14.2"}`
	calls := 0
	runner := &toolrun.RunnerMock{
		DetectFunc: func(context.Context, string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: "uv"}, true
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if len(cmd.Args) == 1 && cmd.Args[0] == "--version" {
				return toolrun.Output{Stdout: []byte("uv 0.10.0")}, nil
			}
			calls++
			return toolrun.Output{Stdout: []byte(helperJSON)}, nil
		},
	}
	var ex evidenceports.Extractor
	for _, descriptor := range registry.All() {
		if descriptor.ID == "python" {
			ex = descriptor.NewExtractor(runner, evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto}, factcache.NewStore(t.TempDir()))
			break
		}
	}
	if ex == nil {
		t.Fatal("Python extractor is not registered")
	}
	ctx, sc := context.Background(), scope.Scope{Root: root}
	_, first, err := ex.Extract(ctx, sc)
	if err != nil {
		t.Fatal(err)
	}

	producerVersion := "grimp 3.17; python 3.14.3"
	helperJSON = `{"edges":[{"importer":"pkg","imported":"pkg.b","line":1}],"unresolved":0,"producer_version":"grimp 3.17; python 3.14.3"}`
	warmFacts, warm, err := ex.Extract(ctx, sc)
	if err != nil {
		t.Fatal(err)
	}
	freshFacts, fresh, err := ex.Extract(ctx, sc)
	if err != nil {
		t.Fatal(err)
	}

	if first.Version == warm.Version {
		t.Fatalf("producer update was not observed: first=%q warm=%q", first.Version, warm.Version)
	}
	if warm.Version != producerVersion || fresh.Version != producerVersion {
		t.Fatalf("warm/fresh profile identity = %q/%q, want %q", warm.Version, fresh.Version, producerVersion)
	}
	if len(warmFacts.Edges) != 1 || warmFacts.Edges[0].To != "module:pkg.b" {
		t.Fatalf("warm facts did not report updated producer output: %+v", warmFacts.Edges)
	}
	if len(freshFacts.Edges) != 1 || freshFacts.Edges[0].To != "module:pkg.b" {
		t.Fatalf("fresh facts did not report updated producer output: %+v", freshFacts.Edges)
	}
	if calls != 3 {
		t.Fatalf("Python fact cache must be bypassed: helper calls=%d, want 3", calls)
	}
}

func writeProfileCacheFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range map[string]string{
		"pyproject.toml":  "[project]\nname = \"fixture\"\n",
		"pkg/__init__.py": "",
		"pkg/a.py":        "import os\n",
	} {
		full := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
