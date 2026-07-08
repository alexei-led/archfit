package py_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/alexei-led/archfit/internal/extract/py"
	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
	"github.com/alexei-led/archfit/internal/view"
)

// pyCacheRunner fakes uv: --version calls return a pinned version, grimp
// helper runs return the given JSON. helperCalls counts only the helper runs
// (Args[0] == "run" — the temp helper script path differs every run, which is
// exactly what the cache's EntryArgs override must absorb).
func pyCacheRunner(helperJSON string, helperCalls *int) *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: tool}, tool == "uv"
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if slices.Contains(cmd.Args, "--version") {
				return toolrun.Output{Stdout: []byte("uv 0.5.0\n")}, nil
			}
			*helperCalls++
			return toolrun.Output{Stdout: []byte(helperJSON)}, nil
		},
	}
}

func writePyFixture(t *testing.T) string {
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

// TestFactCache_HitAndPyFileInvalidation pins the Python cache contract: a
// warm Extract on an unchanged tree skips the grimp helper subprocess (even
// though the helper's temp path differs per run), and a .py edit invalidates.
func TestFactCache_HitAndPyFileInvalidation(t *testing.T) {
	t.Parallel()
	root := writePyFixture(t)
	calls := 0
	runner := pyCacheRunner(`{"edges":[{"importer":"pkg","imported":"pkg.a","line":1,"line_contents":"import pkg.a"}],"unresolved":0}`, &calls)
	ex := py.New(runner, view.ExtractConfig{Mode: view.ModeAuto})
	ex.Cache = factcache.NewStore(t.TempDir())
	ctx := context.Background()
	s := scope.Scope{Root: root}

	extract := func() {
		t.Helper()
		if _, _, err := ex.Extract(ctx, s); err != nil {
			t.Fatalf("Extract: %v", err)
		}
	}

	extract()
	if calls != 1 {
		t.Fatalf("cold run: want 1 helper call, got %d", calls)
	}
	extract()
	if calls != 1 {
		t.Fatalf("warm run: want cache hit (still 1 call), got %d", calls)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "a.py"), []byte("import sys\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	extract()
	if calls != 2 {
		t.Errorf("after .py edit: want re-run (2 calls), got %d", calls)
	}
}

func TestFactCache_VenvMetadataInvalidates(t *testing.T) {
	t.Parallel()
	root := writePyFixture(t)
	metadata := filepath.Join(root, ".venv", "lib", "python3.12", "site-packages", "dep-1.0.dist-info", "METADATA")
	if err := os.MkdirAll(filepath.Dir(metadata), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadata, []byte("Name: dep\nVersion: 1.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	runner := pyCacheRunner(`{"edges":[{"importer":"pkg","imported":"dep","line":1,"line_contents":"import dep"}],"unresolved":0}`, &calls)
	ex := py.New(runner, view.ExtractConfig{Mode: view.ModeAuto})
	ex.Cache = factcache.NewStore(t.TempDir())
	ctx := context.Background()
	s := scope.Scope{Root: root}

	for range 2 {
		if _, _, err := ex.Extract(ctx, s); err != nil {
			t.Fatalf("Extract: %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("warm run: want cache hit (1 call), got %d", calls)
	}

	if err := os.WriteFile(metadata, []byte("Name: dep\nVersion: 1.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ex.Extract(ctx, s); err != nil {
		t.Fatalf("Extract after venv metadata edit: %v", err)
	}
	if calls != 2 {
		t.Errorf("venv metadata edit must invalidate: want 2 helper calls, got %d", calls)
	}
}

// TestFactCache_ExcludedFileEditInvalidates pins key faithfulness: grimp
// analyses the package regardless of config `exclude:` globs, so editing a
// file those globs match must still invalidate the entry — the input-tree
// hash may over-approximate the tool's inputs, never under-approximate.
func TestFactCache_ExcludedFileEditInvalidates(t *testing.T) {
	t.Parallel()
	root := writePyFixture(t)
	calls := 0
	runner := pyCacheRunner(`{"edges":[],"unresolved":0}`, &calls)
	ex := py.New(runner, view.ExtractConfig{Mode: view.ModeAuto, Exclusions: []string{"pkg/a.py"}})
	ex.Cache = factcache.NewStore(t.TempDir())
	ctx := context.Background()
	s := scope.Scope{Root: root}

	for range 2 {
		if _, _, err := ex.Extract(ctx, s); err != nil {
			t.Fatalf("Extract: %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("warm run: want cache hit (1 call), got %d", calls)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "a.py"), []byte("import sys\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ex.Extract(ctx, s); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if calls != 2 {
		t.Errorf("excluded-file edit must invalidate: want 2 calls, got %d", calls)
	}
}

// TestFactCache_UnresolvedNotCached pins the partial-result veto: helper
// output with unresolved imports is never written to the cache.
func TestFactCache_UnresolvedNotCached(t *testing.T) {
	t.Parallel()
	root := writePyFixture(t)
	calls := 0
	runner := pyCacheRunner(`{"edges":[],"unresolved":3}`, &calls)
	ex := py.New(runner, view.ExtractConfig{Mode: view.ModeAuto})
	ex.Cache = factcache.NewStore(t.TempDir())
	ctx := context.Background()
	s := scope.Scope{Root: root}

	for range 2 {
		if _, _, err := ex.Extract(ctx, s); err != nil {
			t.Fatalf("Extract: %v", err)
		}
	}
	if calls != 2 {
		t.Errorf("unresolved output must not be cached: want 2 helper calls, got %d", calls)
	}
}
