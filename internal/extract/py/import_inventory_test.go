package py_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/extract/py"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const (
	inventoryProjectFile   = "pyproject.toml"
	inventoryPackage       = "app"
	inventoryProject       = "[project]\nname = 'import-fixture'\nversion = '0.0.0'\n"
	inventoryInit          = "app/__init__.py"
	inventoryHandler       = "app/handler.py"
	inventoryA             = "app/a.py"
	inventoryB             = "app/b.py"
	inventoryImportB       = "import app.b\n"
	inventoryOtherInit     = "other/__init__.py"
	inventoryOtherB        = "other/b.py"
	inventoryInvalidSyntax = "invalid ("
)

func TestPythonImportInventoryDoesNotRequireExternalPackages(t *testing.T) {
	requirePythonProducer(t)
	runner := toolrun.New()
	root := t.TempDir()
	writeImportFixture(t, root, map[string]string{
		inventoryProjectFile: inventoryProject,
		inventoryInit:        "raise RuntimeError('target package must not execute')\n",
		inventoryHandler:     "import archfit_fixture_external.client\nfrom . import backend\n",
		"app/backend.py":     "value = 1\n",
	})
	ex := py.New(runner, evidenceports.ExtractConfig{Mode: evidenceports.ModeOn, PyPackage: inventoryPackage})
	facts, coverage, err := ex.Extract(t.Context(), scope.Scope{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Status != evidence.StatusOK || coverage.Unresolved != 0 {
		t.Fatalf("absent third-party package invalidated import evidence: %+v", coverage)
	}
	wantTargets := map[string]bool{"module:app.backend": false, "external:archfit_fixture_external.client": false}
	for _, edge := range facts.Edges {
		if edge.From == "module:app.handler" {
			if _, want := wantTargets[edge.To]; want {
				wantTargets[edge.To] = true
			}
		}
	}
	for target, found := range wantTargets {
		if !found {
			t.Errorf("missing source import %s: %+v", target, facts.Edges)
		}
	}

	externalRoot := t.TempDir()
	writeImportFixture(t, externalRoot, map[string]string{
		"archfit_fixture_external/__init__.py": "raise RuntimeError('external package must not execute')\n",
		"archfit_fixture_external/client.py":   "",
	})
	t.Setenv("PYTHONPATH", externalRoot)
	installedFacts, installedCoverage, err := ex.Extract(t.Context(), scope.Scope{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	sortImports := func(a, b graph.Edge) int {
		return strings.Compare(a.From+"\x00"+a.To, b.From+"\x00"+b.To)
	}
	slices.SortFunc(facts.Edges, sortImports)
	slices.SortFunc(installedFacts.Edges, sortImports)
	slices.SortFunc(facts.Nodes, func(a, b graph.Node) int { return strings.Compare(a.Path, b.Path) })
	slices.SortFunc(installedFacts.Nodes, func(a, b graph.Node) int { return strings.Compare(a.Path, b.Path) })
	if !reflect.DeepEqual(facts, installedFacts) || !reflect.DeepEqual(coverage, installedCoverage) {
		t.Fatalf("import evidence depends on installed packages:\nabsent: %+v %+v\npresent: %+v %+v", facts, coverage, installedFacts, installedCoverage)
	}
}

func TestPythonImportInventoryFailsClosedOnSourceGaps(t *testing.T) {
	requirePythonProducer(t)
	runner := toolrun.New()
	for _, tc := range []struct {
		name, source string
		complete     bool
	}{
		{"relative beyond package", "from ..backend import value\n", false},
		{"invalid syntax", "from app import (\n", false},
		{"invalid encoding", "\xff", false},
		{"UTF-8 BOM", "\xef\xbb\xbffrom . import value\n", true},
		{"declared source encoding", "# coding: iso-8859-1\n# caf\xe9\nfrom . import value\n", true},
		{"attribute import", "from app import value\n", true},
		{"relative attribute import", "from . import value\n", true},
		{"type checking import", "from typing import TYPE_CHECKING\nif TYPE_CHECKING:\n    import app.missing\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeImportFixture(t, root, map[string]string{
				inventoryProjectFile: inventoryProject,
				inventoryInit:        "value = 1\n",
				inventoryHandler:     tc.source,
			})
			ex := py.New(runner, evidenceports.ExtractConfig{Mode: evidenceports.ModeOn, PyPackage: inventoryPackage})
			_, coverage, err := ex.Extract(t.Context(), scope.Scope{Root: root})
			complete := err == nil && coverage.Status == evidence.StatusOK && coverage.Unresolved == 0
			if complete != tc.complete {
				t.Fatalf("complete=%v, want %v; coverage=%+v error=%v", complete, tc.complete, coverage, err)
			}
		})
	}
}

func TestPythonImportInventoryPreservesExplicitTargets(t *testing.T) {
	requirePythonProducer(t)
	for _, tc := range []struct {
		name, source string
		parent       bool
	}{
		{"absolute", "import app.missing\n", false},
		{"from", "from app.missing import value\n", false},
		{"relative", "from .missing import value\n", false},
		{"optional", "try:\n    import app.missing\nexcept ImportError:\n    pass\n", false},
		{"rebound exception", "ImportError = ValueError\ntry:\n    import app.missing\nexcept ImportError:\n    pass\n", false},
		{"function", "def load():\n    import app.missing\n", false},
		{"handler", "try:\n    pass\nexcept ImportError:\n    import app.missing\n", false},
		{"finally", "try:\n    pass\nfinally:\n    import app.missing\n", false},
		{"explicit parent on same line", "import app, app.missing\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeImportFixture(t, root, map[string]string{
				inventoryProjectFile: inventoryProject,
				inventoryInit:        "",
				inventoryHandler:     tc.source,
			})
			ex := py.New(toolrun.New(), evidenceports.ExtractConfig{Mode: evidenceports.ModeOn, PyPackage: inventoryPackage})
			facts, coverage, err := ex.Extract(t.Context(), scope.Scope{Root: root})
			if err != nil {
				t.Fatal(err)
			}
			if coverage.Status != evidence.StatusOK || coverage.FilesSeen != 2 || coverage.FilesApplicable != 2 {
				t.Fatalf("literal targets are not scanned source: %+v", coverage)
			}
			found, parent := false, false
			for _, edge := range facts.Edges {
				if edge.To == "module:app.missing" {
					found = edge.Confidence == confidenceLow && edge.StrengthHint == "" && len(edge.Locations) == 1
				}
				parent = parent || edge.To == "module:app"
			}
			if !found || parent != tc.parent {
				t.Fatalf("literal lost or parent dependency invented: %+v", facts.Edges)
			}
			if slices.ContainsFunc(facts.Nodes, func(n graph.Node) bool { return n.Path == "app.missing" }) {
				t.Fatal("an absent literal target was counted as an observed source module")
			}
		})
	}
}

func TestPythonImportInventoryUsesRepositorySources(t *testing.T) {
	requirePythonProducer(t)
	root, shadow := t.TempDir(), t.TempDir()
	writeImportFixture(t, root, map[string]string{
		inventoryProjectFile:  inventoryProject,
		"src/app/__init__.py": "raise RuntimeError('target package must not execute')\n",
		"src/app/handler.py":  "from . import backend\n",
		"src/app/backend.py":  "",
	})
	writeImportFixture(t, shadow, map[string]string{
		inventoryInit:    "",
		inventoryHandler: "",
		"app/backend.py": "",
	})
	t.Setenv("PYTHONPATH", shadow)
	ex := py.New(toolrun.New(), evidenceports.ExtractConfig{Mode: evidenceports.ModeOn, PyPackage: inventoryPackage})
	facts, coverage, err := ex.Extract(t.Context(), scope.Scope{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Status != evidence.StatusOK {
		t.Fatalf("coverage=%+v", coverage)
	}
	if !slices.ContainsFunc(facts.Edges, func(edge graph.Edge) bool {
		return edge.From == "module:app.handler" && edge.To == "module:app.backend"
	}) {
		t.Fatalf("shadowed package hid repository imports: %+v", facts.Edges)
	}
}

func TestPythonImportProjectLayouts(t *testing.T) {
	requirePythonProducer(t)
	for _, tc := range []struct {
		name, prefix, configured string
		files                    map[string]string
		complete                 bool
	}{
		{"flat", "", inventoryPackage, map[string]string{inventoryInit: "", inventoryA: inventoryImportB, inventoryB: ""}, true},
		{"src", "src", inventoryPackage, map[string]string{inventoryInit: "", inventoryA: inventoryImportB, inventoryB: ""}, true},
		{"multiple packages", "", "", map[string]string{inventoryInit: "", inventoryA: "import other.b\n", inventoryOtherInit: "", inventoryOtherB: ""}, true},
		{"omitted unreferenced package", "", inventoryPackage, map[string]string{inventoryInit: "", inventoryA: "", inventoryOtherInit: "", inventoryOtherB: "import other.c\n", "other/c.py": ""}, false},
		{"omitted first-party package", "", inventoryPackage, map[string]string{inventoryInit: "", inventoryA: "import other.b\n", inventoryOtherInit: "", inventoryOtherB: ""}, false},
		{"missing configured root", "", "absent", map[string]string{inventoryInit: ""}, false},
		{"namespace root unsupported", "", inventoryPackage, map[string]string{inventoryA: ""}, false},
		{"separate service root unsupported", "service", inventoryPackage, map[string]string{inventoryInit: ""}, false},
		{"ignored nonmodules", "", inventoryPackage, map[string]string{inventoryInit: "", inventoryA: inventoryImportB, inventoryB: "", "app/.ipynb_checkpoints/x.py": inventoryInvalidSyntax, "app/sample-project/x.py": inventoryInvalidSyntax, "app/extra.dot.py": inventoryInvalidSyntax, "app/.hidden.py": inventoryInvalidSyntax}, true},
		{"unscanned namespace portion", "", inventoryPackage, map[string]string{inventoryInit: "", inventoryB: "", "app/sub/x.py": inventoryImportB}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeImportFixture(t, root, map[string]string{inventoryProjectFile: inventoryProject})
			writeImportFixture(t, filepath.Join(root, tc.prefix), tc.files)
			ex := py.New(toolrun.New(), evidenceports.ExtractConfig{Mode: evidenceports.ModeOn, PyPackage: tc.configured})
			facts, coverage, err := ex.Extract(t.Context(), scope.Scope{Root: root})
			if complete := err == nil && coverage.Status == evidence.StatusOK; complete != tc.complete {
				t.Fatalf("complete=%v, want %v; coverage=%+v error=%v", complete, tc.complete, coverage, err)
			}
			if tc.complete && len(facts.Edges) == 0 {
				t.Fatal("project imports were lost")
			}
		})
	}
}

func TestPythonImportInventoryRejectsAnalyzerNameCollision(t *testing.T) {
	requirePythonProducer(t)
	root := t.TempDir()
	marker := filepath.Join(root, "executed")
	t.Setenv("PYTHONPATH", root)
	writeImportFixture(t, root, map[string]string{
		inventoryProjectFile: inventoryProject,
		"grimp/__init__.py":  "open(" + strconv.Quote(marker) + ", 'w').write('executed')\n",
	})
	ex := py.New(toolrun.New(), evidenceports.ExtractConfig{Mode: evidenceports.ModeOn, PyPackage: "grimp"})
	_, coverage, err := ex.Extract(t.Context(), scope.Scope{Root: root})
	if err == nil || !strings.Contains(err.Error(), "cannot bind package") || coverage.Status == evidence.StatusOK {
		t.Fatalf("analyzer-name collision was not rejected: coverage=%+v error=%v", coverage, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("target package executed: %v", err)
	}
}

func TestPythonImportInventoryReportsPerFileParseGaps(t *testing.T) {
	requirePythonProducer(t)
	const unscannedFile = "app/sub/bad.py"
	for _, tc := range []struct{ name, source, reason string }{
		{"invalid syntax", inventoryInvalidSyntax, "SyntaxError under Python 3.14"},
		{"invalid encoding", "# coding: ascii\nvalue = '\xff'\n", "UnicodeDecodeError under Python 3.14"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeImportFixture(t, root, map[string]string{
				inventoryProjectFile: inventoryProject, inventoryInit: "", unscannedFile: tc.source,
			})
			_, coverage, err := py.New(toolrun.New(), evidenceports.ExtractConfig{Mode: evidenceports.ModeOn, PyPackage: inventoryPackage}).Extract(t.Context(), scope.Scope{Root: root})
			if err != nil || coverage.Status != evidence.StatusPartial || coverage.UnresolvedInputsMissing != 1 ||
				!strings.Contains(coverage.Reason, unscannedFile) || !strings.Contains(coverage.Reason, tc.reason) {
				t.Fatalf("per-file parse gap: coverage=%+v err=%v", coverage, err)
			}
			if coverage.FilesSeen != 1 || coverage.FilesApplicable != 2 {
				t.Fatalf("unparsed file counted as scanned: %+v", coverage)
			}
		})
	}
}

func writeImportFixture(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func requirePythonProducer(t *testing.T) {
	t.Helper()
	required := os.Getenv("ARCHFIT_REQUIRE_TOOLS") == "1"
	if testing.Short() && !required {
		t.Skip("real Python producer skipped under -short")
	}
	if _, ok := toolrun.New().Detect(t.Context(), "uv"); !ok {
		if required {
			t.Fatal("uv is missing and ARCHFIT_REQUIRE_TOOLS=1")
		}
		t.Skip("uv is not installed")
	}
}

func TestPythonImportProducerDoesNotRunProjectSetup(t *testing.T) {
	requirePythonProducer(t)
	root := t.TempDir()
	marker := filepath.Join(root, "build-hook-executed")
	writeImportFixture(t, root, map[string]string{
		inventoryProjectFile: inventoryProject + "\n[build-system]\nrequires = []\nbuild-backend = 'build_hook'\nbackend-path = ['.']\n",
		"build_hook.py":      "open(" + strconv.Quote(marker) + ", 'w').write('executed')\nraise RuntimeError('build hook must not execute')\n",
		"uv.toml":            "index-url = 'not-a-valid-package-index'\n",
		".python-version":    filepath.Join(root, "untrusted-python") + "\n",
		"untrusted-python":   "#!/bin/sh\nprintf executed > " + strconv.Quote(marker) + "\nexit 1\n",
		inventoryInit:        "",
		inventoryA:           "import archfit_fixture_external\n",
	})

	_, coverage, err := py.New(toolrun.New(), evidenceports.ExtractConfig{Mode: evidenceports.ModeOn, PyPackage: inventoryPackage}).Extract(t.Context(), scope.Scope{Root: root})
	if err != nil || coverage.Status != evidence.StatusOK {
		t.Fatalf("project setup affected source extraction: coverage=%+v error=%v", coverage, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("project build hook executed: %v", err)
	}
}

func TestPythonImportProducerRejectsDirectoryLinksBeforeGrimp(t *testing.T) {
	requirePythonProducer(t)
	for _, cycle := range []bool{false, true} {
		t.Run(strconv.FormatBool(cycle), func(t *testing.T) {
			root, target := t.TempDir(), t.TempDir()
			writeImportFixture(t, root, map[string]string{inventoryProjectFile: inventoryProject, inventoryInit: ""})
			if cycle {
				target = filepath.Join(root, inventoryPackage)
			} else {
				writeImportFixture(t, target, map[string]string{"__init__.py": "", "a.py": "import forbidden_external\n"})
			}
			if err := os.Symlink(target, filepath.Join(root, inventoryPackage, "shared")); err != nil {
				t.Fatal(err)
			}
			_, _, err := py.New(toolrun.New(), evidenceports.ExtractConfig{Mode: evidenceports.ModeOn, PyPackage: inventoryPackage}).Extract(t.Context(), scope.Scope{Root: root})
			if err == nil || !strings.Contains(err.Error(), "directory symlink in Python source") {
				t.Fatalf("directory link reached grimp: %v", err)
			}
		})
	}
}

func TestPythonDirectFallbackKeepsTargetImportPathsIsolated(t *testing.T) {
	withoutToolPythonOverride(t)
	requirePythonProducer(t)
	realRunner := toolrun.New()
	analyzerEnv := filepath.Join(t.TempDir(), "analyzer")
	pythonPath := filepath.Join(analyzerEnv, "bin", "python")
	for _, args := range [][]string{
		{"venv", "--no-config", "--python", "3.14", analyzerEnv},
		{"pip", "install", "--no-config", "--python", pythonPath, "grimp==3.17"},
	} {
		out, err := realRunner.Run(t.Context(), toolrun.ToolCmd{Name: "uv", Args: args})
		if err != nil || out.ExitCode != 0 {
			t.Fatalf("prepare analyzer interpreter: %v %s", err, out.Stderr)
		}
	}
	root := t.TempDir()
	marker := filepath.Join(root, "executed")
	writeImportFixture(t, root, map[string]string{
		inventoryProjectFile: inventoryProject, inventoryInit: "raise RuntimeError('target must not execute')\n", inventoryA: inventoryImportB, inventoryB: "",
	})
	shadow := t.TempDir()
	writeImportFixture(t, shadow, map[string]string{
		"grimp/__init__.py": "open(" + strconv.Quote(marker) + ", 'w').write('executed')\n",
	})
	t.Setenv("PYTHONPATH", shadow)
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, name string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: pythonPath}, name == "python3.14"
		},
		RunFunc: realRunner.Run,
	}
	facts, coverage, err := py.New(runner, evidenceports.ExtractConfig{Mode: evidenceports.ModeOn, PyPackage: inventoryPackage}).Extract(t.Context(), scope.Scope{Root: root})
	if err != nil || coverage.Status != evidence.StatusOK || len(facts.Edges) == 0 {
		t.Fatalf("direct fallback lost source evidence: coverage=%+v error=%v", coverage, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("target shadow package executed: %v", err)
	}
}
