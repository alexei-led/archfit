package py_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/extract/py"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const (
	runtimeUVConfig      = "uv.toml"
	invalidTargetVenv    = "target venv"
	invalidTarget        = "target"
	invalidSymlink       = "symlink to target"
	provisionPythonFlag  = "--python"
	invalidRuntime       = "runtime failure"
	provisionNoConfig    = "--no-config"
	invalidRelative      = "relative"
	invalidNonExecutable = "non-executable"
)

func provisionGrimpPython(t *testing.T) string {
	t.Helper()
	requirePythonProducer(t)
	python := filepath.Join(t.TempDir(), "env", "bin", "python")
	for _, args := range [][]string{
		{"venv", provisionNoConfig, provisionPythonFlag, "3.14", filepath.Dir(filepath.Dir(python))},
		{"pip", "install", provisionNoConfig, provisionPythonFlag, python, "grimp==3.17"},
	} {
		out, err := toolrun.New().Run(t.Context(), toolrun.ToolCmd{Name: "uv", Args: args, WorkDir: t.TempDir()})
		if err != nil || out.ExitCode != 0 {
			t.Fatalf("provision real grimp: %v %s", err, out.Stderr)
		}
	}
	return python
}

func TestProvisionedPythonOfflineIgnoresTargetEnvironment(t *testing.T) {
	python := provisionGrimpPython(t)
	root := t.TempDir()
	marker := filepath.Join(root, "executed")
	poison := "import pathlib; pathlib.Path(" + strconv.Quote(marker) + ").write_text('executed')\n"
	writeImportFixture(t, root, map[string]string{
		inventoryProjectFile: "[build-system]\nrequires = []\nbuild-backend = 'evil'\nbackend-path = ['.']\n[project]\nname = 'app'\nversion = '0.0.0'\n",
		inventoryInit:        poison, inventoryA: inventoryImportB, inventoryB: "",
		"evil.py": poison, "setup.py": poison,
		runtimeUVConfig:   "this is deliberately invalid config\n",
		".python-version": "0.0.0\n",
		"grimp.py":        poison, "sitecustomize.py": poison,
		".venv/lib/python3.14/site-packages/target.pth": poison,
	})
	userBase := t.TempDir()
	writeImportFixture(t, userBase, map[string]string{"lib/python3.14/site-packages/usercustomize.py": poison})
	t.Setenv("ARCHFIT_TOOL_PYTHON", python)
	t.Setenv("PYTHONPATH", root)
	t.Setenv("PYTHONHOME", root)
	t.Setenv("VIRTUAL_ENV", filepath.Join(root, ".venv"))
	t.Setenv("PYTHONUSERBASE", userBase)
	t.Setenv("UV_CONFIG_FILE", filepath.Join(root, runtimeUVConfig))
	t.Setenv("UV_CACHE_DIR", t.TempDir())
	t.Setenv("UV_OFFLINE", "1")
	t.Setenv("UV_PYTHON_DOWNLOADS", "never")
	t.Setenv("PATH", t.TempDir()) // No uv or unrelated Python is available at runtime.
	realRunner := toolrun.New()
	runner := &toolrun.RunnerMock{
		DetectFunc: realRunner.Detect,
		RunFunc: func(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if cmd.Name != python || len(cmd.Args) == 0 || cmd.Args[0] != "-I" || cmd.WorkDir == root || cmd.WorkDir == "" {
				t.Fatalf("not an explicitly owned isolated interpreter: %+v", cmd)
			}
			return realRunner.Run(ctx, cmd)
		},
	}
	facts, cov, err := py.New(runner, evidenceports.ExtractConfig{Mode: evidenceports.ModeOn}).Extract(t.Context(), scope.Scope{Root: root})
	if err != nil || cov.Status != evidence.StatusOK || !strings.Contains(cov.Version, "grimp 3.17; python 3.14.") {
		t.Fatalf("offline provisioned environment lost real producer: cov=%+v err=%v", cov, err)
	}
	found := false
	for _, edge := range facts.Edges {
		found = found || edge.From == "module:app.a" && edge.To == "module:app.b"
	}
	if !found || cov.FilesSeen != 3 || cov.FilesApplicable != 3 {
		t.Fatalf("source imports/inventory missing: %+v %+v", facts, cov)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("target startup/config/build code executed: %v", err)
	}
}

func TestProvisionedPythonInvalidOverrideDoesNotFallback(t *testing.T) {
	for _, name := range []string{"empty", invalidRelative, "missing", "directory", invalidNonExecutable, invalidTarget, invalidSymlink, invalidTargetVenv, invalidRuntime} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeImportFixture(t, root, map[string]string{inventoryProjectFile: inventoryProject, inventoryInit: ""})
			path := ""
			switch name {
			case invalidRelative:
				path = "python"
			case "missing":
				path = filepath.Join(t.TempDir(), "missing")
			case "directory":
				path = t.TempDir()
			case invalidNonExecutable, invalidTarget, invalidSymlink, invalidTargetVenv, invalidRuntime:
				dir := t.TempDir()
				if name == invalidTarget || name == invalidSymlink {
					dir = root
				}
				path = filepath.Join(dir, "python")
				mode := os.FileMode(0o700)
				if name == invalidNonExecutable {
					mode = 0o600
				}
				if err := os.WriteFile(path, []byte("not a Python interpreter"), mode); err != nil {
					t.Fatal(err)
				}
				if name == invalidSymlink {
					link := filepath.Join(t.TempDir(), "python")
					if err := os.Symlink(path, link); err != nil {
						t.Fatal(err)
					}
					path = link
				}
			}
			if name == invalidTargetVenv {
				base := path
				path = filepath.Join(root, "python")
				if err := os.Symlink(base, path); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("ARCHFIT_TOOL_PYTHON", path)
			runner := &toolrun.RunnerMock{
				DetectFunc: func(context.Context, string) (toolrun.ToolInfo, bool) {
					t.Fatal("invalid explicit interpreter must not trigger tool discovery")
					return toolrun.ToolInfo{}, false
				},
				RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
					if name != invalidRuntime || cmd.Name != path {
						t.Fatalf("invalid interpreter executed: %+v", cmd)
					}
					return toolrun.Output{ExitCode: 1, Stderr: []byte("missing required tool dependency")}, nil
				},
			}
			for _, mode := range []evidenceports.ToolMode{evidenceports.ModeAuto, evidenceports.ModeOn} {
				_, _, err := py.New(runner, evidenceports.ExtractConfig{Mode: mode}).Extract(t.Context(), scope.Scope{Root: root})
				if err == nil || !strings.Contains(err.Error(), "ARCHFIT_TOOL_PYTHON") {
					t.Fatalf("invalid explicit runtime was hidden (mode=%s): %v", mode, err)
				}
			}
		})
	}
}

func TestPythonUVFallbackIgnoresExplicitTargetConfigAndEnvFile(t *testing.T) {
	withoutToolPythonOverride(t)
	requirePythonProducer(t)
	root := t.TempDir()
	marker := filepath.Join(root, "executed")
	poison := "import pathlib; pathlib.Path(" + strconv.Quote(marker) + ").write_text('executed')\n"
	writeImportFixture(t, root, map[string]string{
		inventoryProjectFile: "[build-system]\nrequires = []\nbuild-backend = 'evil'\nbackend-path = ['.']\n[project]\nname = 'app'\nversion = '0.0.0'\n",
		inventoryInit:        poison, inventoryA: inventoryImportB, inventoryB: "",
		"evil.py": poison, "setup.py": poison, "sitecustomize.py": poison, "grimp.py": poison,
		runtimeUVConfig: "not valid config\n",
	})
	t.Setenv("UV_CONFIG_FILE", filepath.Join(root, runtimeUVConfig))
	t.Setenv("UV_ENV_FILE", filepath.Join(root, "must-not-read"))
	t.Setenv("UV_WORKING_DIR", root)
	t.Setenv("UV_PROJECT", root)
	t.Setenv("PYTHONPATH", root)
	t.Setenv("PYTHONHOME", root)
	t.Setenv("VIRTUAL_ENV", filepath.Join(root, ".venv"))
	t.Setenv("PYTHONUSERBASE", root)
	facts, cov, err := py.New(toolrun.New(), evidenceports.ExtractConfig{Mode: evidenceports.ModeOn}).Extract(t.Context(), scope.Scope{Root: root})
	if err != nil || cov.Status != evidence.StatusOK || len(facts.Edges) == 0 {
		t.Fatalf("isolated uv fallback read target configuration: %+v %v", cov, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("target startup/config/build code executed: %v", err)
	}
}
