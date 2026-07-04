package ts_test

import (
	"context"
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

// TestFactCache_HitAndTSConfigInvalidation pins the TS cache contract: a warm
// Extract on an unchanged tree skips the depcruise subprocess, and a
// tsconfig.json edit invalidates the entry.
func TestFactCache_HitAndTSConfigInvalidation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for name, content := range map[string]string{
		"package.json":  `{"name":"fixture"}`,
		"tsconfig.json": `{"compilerOptions":{}}`,
		"a.ts":          `export const a = 1;`,
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
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

// TestFactCache_UnresolvedNotCached pins the partial-result veto: output with
// unresolved import specifiers (usually missing node_modules — state the key
// cannot see) is never written, so the next run re-executes depcruise.
func TestFactCache_UnresolvedNotCached(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for name, content := range map[string]string{
		"package.json": `{"name":"fixture"}`,
		"a.ts":         `import x from "left-pad";`,
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
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
