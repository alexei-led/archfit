package ts_test

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/extract/ts"
	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// cacheFixtureRunner fakes bunx: --version calls return a pinned version,
// depcruise scans return the given JSON. depcruiseCalls counts only the scans.
func cacheFixtureRunner(depcruiseJSON string, depcruiseCalls *int) *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: tool}, tool == launcherBunx
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if slices.Contains(cmd.Args, "--version") {
				return toolrun.Output{Stdout: []byte("16.0.0\n")}, nil
			}
			*depcruiseCalls++
			return toolrun.Output{Stdout: []byte(depcruiseJSON)}, nil
		},
	}
}

// writeTSFixture materialises a minimal TS project: package.json, a.ts with
// the given content, plus any extra files (nested paths allowed).
func writeTSFixture(t *testing.T, mainTS string, extra map[string]string) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{"package.json": `{"name":"fixture"}`, "a.ts": mainTS}
	maps.Copy(files, extra)
	for name, content := range files {
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

// TestFactCache_HitAndTSConfigInvalidation pins the TS cache contract: a warm
// Extract on an unchanged tree skips the depcruise subprocess, and a
// tsconfig.json edit invalidates the entry.
func TestFactCache_HitAndTSConfigInvalidation(t *testing.T) {
	t.Parallel()
	root := writeTSFixture(t, `export const a = 1;`, map[string]string{
		"tsconfig.json": `{"compilerOptions":{}}`,
	})
	calls := 0
	runner := cacheFixtureRunner(`{"modules":[{"source":"a.ts","dependencies":[]}]}`, &calls)
	ex := ts.New(runner, config.ExtractConfig{Mode: config.ModeAuto, Src: "."})
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
		t.Fatalf("cold run: want 1 depcruise call, got %d", calls)
	}
	extract()
	if calls != 1 {
		t.Fatalf("warm run: want cache hit (still 1 call), got %d", calls)
	}

	// tsconfig change invalidates (manifest is key material).
	if err := os.WriteFile(filepath.Join(root, "tsconfig.json"), []byte(`{"compilerOptions":{"paths":{}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	extract()
	if calls != 2 {
		t.Errorf("after tsconfig edit: want re-run (2 calls), got %d", calls)
	}
}

// TestFactCache_ExcludedFileEditInvalidates pins key faithfulness: depcruise
// skips only node_modules, not config `exclude:` globs, so editing a file
// those globs match must still invalidate the entry — the input-tree hash may
// over-approximate the tool's inputs, never under-approximate.
func TestFactCache_ExcludedFileEditInvalidates(t *testing.T) {
	t.Parallel()
	root := writeTSFixture(t, `export const a = 1;`, map[string]string{
		"legacy/old.ts": `export const old = 1;`,
	})
	calls := 0
	runner := cacheFixtureRunner(`{"modules":[{"source":"a.ts","dependencies":[]}]}`, &calls)
	ex := ts.New(runner, config.ExtractConfig{Mode: config.ModeAuto, Src: ".", Exclusions: []string{"legacy/**"}})
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
	if err := os.WriteFile(filepath.Join(root, "legacy", "old.ts"), []byte(`export const old = 2;`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ex.Extract(ctx, s); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if calls != 2 {
		t.Errorf("excluded-file edit must invalidate: want 2 calls, got %d", calls)
	}
}

// TestFactCache_UnresolvedNotCached pins the partial-result veto: output with
// unresolved import specifiers (usually missing node_modules — state the key
// cannot see) is never written, so the next run re-executes depcruise.
func TestFactCache_UnresolvedNotCached(t *testing.T) {
	t.Parallel()
	root := writeTSFixture(t, `import x from "left-pad";`, nil)
	calls := 0
	unresolvedJSON := `{"modules":[{"source":"a.ts","dependencies":[{"module":"left-pad","couldNotResolve":true}]}]}`
	runner := cacheFixtureRunner(unresolvedJSON, &calls)
	ex := ts.New(runner, config.ExtractConfig{Mode: config.ModeAuto, Src: "."})
	ex.Cache = factcache.NewStore(t.TempDir())
	ctx := context.Background()
	s := scope.Scope{Root: root}

	for range 2 {
		if _, _, err := ex.Extract(ctx, s); err != nil {
			t.Fatalf("Extract: %v", err)
		}
	}
	if calls != 2 {
		t.Errorf("unresolved output must not be cached: want 2 depcruise calls, got %d", calls)
	}
}

func TestFactCache_SubtreeRootManifestInvalidates(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	sub := filepath.Join(repo, "packages", "pkg")
	files := map[string]string{
		"package.json":                 `{"name":"workspace"}`,
		".dependency-cruiser.json":     `{}`,
		"packages/pkg/package.json":    `{"name":"pkg"}`,
		"packages/pkg/a.ts":            `export const a = 1;`,
		"packages/pkg/tsconfig.json":   `{"compilerOptions":{}}`,
		"packages/other/package.json":  `{"name":"other"}`,
		"packages/other/irrelevant.ts": `export const x = 1;`,
	}
	for name, content := range files {
		full := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	runner := cacheFixtureRunner(`{"modules":[{"source":"packages/pkg/a.ts","dependencies":[]}]}`, &calls)
	ex := ts.New(runner, config.ExtractConfig{Mode: config.ModeAuto, Src: "."})
	ex.Cache = factcache.NewStore(t.TempDir())
	ctx := context.Background()
	s := scope.Scope{Root: sub, GitRoot: repo, SubtreePrefix: "packages/pkg"}

	for range 2 {
		if _, _, err := ex.Extract(ctx, s); err != nil {
			t.Fatalf("Extract: %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("warm subtree run: want cache hit (1 call), got %d", calls)
	}

	if err := os.WriteFile(filepath.Join(repo, ".dependency-cruiser.json"), []byte(`{"options":{"doNotFollow":{}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ex.Extract(ctx, s); err != nil {
		t.Fatalf("Extract after root depcruise config edit: %v", err)
	}
	if calls != 2 {
		t.Errorf("root depcruise config edit must invalidate subtree cache: want 2 calls, got %d", calls)
	}
}
