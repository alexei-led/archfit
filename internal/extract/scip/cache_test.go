package scip

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// scipCacheRunner fakes a Go project's SCIP toolchain: scip-go + uv are
// detected, --version probes answer, the indexer writes its --output file,
// and the uv reader run returns readerJSON. indexRuns/readerRuns count the
// expensive phases (probes excluded).
func scipCacheRunner(readerJSON string, indexRuns, readerRuns *int) *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: tool}, tool == indexerGo || tool == "uv"
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			switch {
			case slices.Contains(cmd.Args, "--version"):
				return toolrun.Output{Stdout: []byte(cmd.Name + " 1.0\n")}, nil
			case cmd.Name == indexerGo:
				*indexRuns++
				for i, arg := range cmd.Args {
					if arg == flagOutput && i+1 < len(cmd.Args) {
						_ = os.WriteFile(cmd.Args[i+1], []byte("scip"), 0o600)
					}
				}
				return toolrun.Output{ExitCode: 0}, nil
			case cmd.Name == "uv":
				*readerRuns++
				return toolrun.Output{Stdout: []byte(readerJSON)}, nil
			default:
				return toolrun.Output{}, nil
			}
		},
	}
}

// TestFactCache_PipelineHitAndSourceInvalidation pins the SCIP cache
// contract: a second adapter (a new run) on an unchanged tree serves the
// reader output from the cache — zero index/read subprocesses — and a source
// edit invalidates. The per-run pipeCache memoization is bypassed by using a
// fresh Adapter per run, exactly like separate archfit invocations.
func TestFactCache_PipelineHitAndSourceInvalidation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for name, content := range map[string]string{
		"go.mod": "module example.com/demo\n\ngo 1.24\n",
		"a.go":   "package demo\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	store := factcache.NewStore(t.TempDir())
	readerJSON := `{"edges":[{"from":"a","to":"b","strength":"contract"}],"symbols":[{"symbol":"s","path":"a.go","module":"a","fan_in":1}]}`
	indexRuns, readerRuns := 0, 0
	ctx := context.Background()
	s := scope.Scope{Root: root}

	strengths := func() map[string]string {
		t.Helper()
		a := New(scipCacheRunner(readerJSON, &indexRuns, &readerRuns), 0)
		a.Cache = store
		m, cov, err := a.Strengths(ctx, s)
		if err != nil {
			t.Fatalf("Strengths: %v", err)
		}
		if cov.Status != "ok" {
			t.Fatalf("Strengths status = %q, want ok", cov.Status)
		}
		return m
	}

	cold := strengths()
	if indexRuns != 1 || readerRuns != 1 {
		t.Fatalf("cold run: want 1 index + 1 read, got %d/%d", indexRuns, readerRuns)
	}
	warm := strengths()
	if indexRuns != 1 || readerRuns != 1 {
		t.Fatalf("warm run: want full cache hit, got %d/%d index/read runs", indexRuns, readerRuns)
	}
	if len(warm) != len(cold) || warm["a\x00b"] != cold["a\x00b"] {
		t.Errorf("warm strengths differ from cold: %v vs %v", warm, cold)
	}

	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package demo\n\nvar X = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	strengths()
	if indexRuns != 2 || readerRuns != 2 {
		t.Errorf("after source edit: want re-index (2/2), got %d/%d", indexRuns, readerRuns)
	}
}

// TestCacheKey_LockfileInvalidates pins the over-hash contract for the
// resolution environment: the indexers resolve symbols against installed
// dependencies, so a lockfile-only bump (no source edit) must change the key.
func TestCacheKey_LockfileInvalidates(t *testing.T) {
	t.Parallel()
	cases := []struct {
		lang     string
		source   string
		manifest string
		lockfile string
	}{
		{langTS, "a.ts", manifestPkgJSON, "yarn.lock"},
		{langTS, "a.ts", manifestPkgJSON, "package-lock.json"},
		{langPy, "a.py", manifestPyproject, "uv.lock"},
		{langPy, "a.py", manifestPyproject, "requirements.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.lang+"/"+tc.lockfile, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			for name, content := range map[string]string{
				tc.source:   "x\n",
				tc.manifest: "{}\n",
			} {
				if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			indexRuns, readerRuns := 0, 0
			a := New(scipCacheRunner("{}", &indexRuns, &readerRuns), 0)
			a.Cache = factcache.NewStore(t.TempDir())
			ctx := context.Background()

			before := a.cacheKey(ctx, root, "indexer", "pkg", tc.lang)
			if before == "" {
				t.Fatal("cacheKey returned empty key")
			}
			if err := os.WriteFile(filepath.Join(root, tc.lockfile), []byte("dep@2\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			after := a.cacheKey(ctx, root, "indexer", "pkg", tc.lang)
			if after == before {
				t.Errorf("key unchanged after %s appeared — lockfile is outside the input hash", tc.lockfile)
			}
		})
	}
}
