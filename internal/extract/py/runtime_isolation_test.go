package py_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/extract/py"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestPythonRuntimeIgnoresTargetEnvironmentAndCache(t *testing.T) {
	requirePythonProducer(t)
	for _, name := range []string{"read-only source", "forged cache", "target virtualenv"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeImportFixture(t, root, map[string]string{
				inventoryProjectFile: inventoryProject, inventoryInit: "", inventoryA: inventoryImportB, inventoryB: "",
			})
			runner := toolrun.New()
			runUV := func(args ...string) {
				t.Helper()
				out, err := runner.Run(t.Context(), toolrun.ToolCmd{Name: "uv", Args: args, WorkDir: root})
				if err != nil || out.ExitCode != 0 {
					t.Fatalf("prepare target fixture: %v %s", err, out.Stderr)
				}
			}
			marker := filepath.Join(t.TempDir(), "executed")
			switch name {
			case "read-only source":
				info, err := os.Stat(root)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(root, info.Mode().Perm()&^0o222); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(root, info.Mode().Perm()) })
			case "forged cache":
				runUV("run", "--no-config", "--no-project", "--isolated", "--python", "3.14", "--with", "grimp==3.17", "python", "-I", "-c", "import sys,grimp; sys.path.insert(0,'.'); grimp.build_graph('app', exclude_type_checking_imports=True)")
				dataFiles, err := filepath.Glob(filepath.Join(root, ".grimp_cache", "*.data.json"))
				if err != nil || len(dataFiles) == 0 {
					t.Fatalf("seed real cache: files=%v err=%v", dataFiles, err)
				}
				for _, file := range dataFiles {
					if err := os.WriteFile(file, []byte(`{"imports_by_module":{"app":[],"app.a":[],"app.b":[]},"version":2}`), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				runUV("run", "--no-config", "--no-project", "--isolated", "--python", "3.14", "--with", "grimp==3.17", "python", "-I", "-c", "import sys,grimp; sys.path.insert(0,'.'); g=grimp.build_graph('app', exclude_type_checking_imports=True); assert not g.find_modules_directly_imported_by('app.a')")
			case "target virtualenv":
				venv := filepath.Join(root, ".venv")
				runUV("venv", "--no-config", "--python", "3.14", venv)
				writeImportFixture(t, root, map[string]string{
					".venv/lib/python3.14/site-packages/target.pth": "import pathlib; pathlib.Path(" + strconv.Quote(marker) + ").write_text('executed')\n",
				})
				t.Setenv("VIRTUAL_ENV", venv)
			}
			facts, coverage, err := py.New(runner, evidenceports.ExtractConfig{Mode: evidenceports.ModeOn, PyPackage: inventoryPackage}).Extract(t.Context(), scope.Scope{Root: root})
			if err != nil || coverage.Status != evidence.StatusOK {
				t.Fatalf("source evidence lost: coverage=%+v err=%v", coverage, err)
			}
			found := false
			for _, edge := range facts.Edges {
				found = found || edge.From == "module:app.a" && edge.To == "module:app.b"
			}
			if !found {
				t.Fatal("source import missing from evidence")
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("target environment executed: %v", err)
			}
		})
	}
}
