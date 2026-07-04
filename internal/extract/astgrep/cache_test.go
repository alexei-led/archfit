package astgrep_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/extract/astgrep"
	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// TestFactCache_FindHitAndTimeoutNotCached pins the ast-grep cache contract:
// a warm Find on an unchanged tree skips the sg subprocess, a source edit
// invalidates, and a timed-out (exec-error) run is never cached.
func TestFactCache_FindHitAndTimeoutNotCached(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	scans := 0
	fail := false
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: tool}, tool == "sg"
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if slices.Contains(cmd.Args, "--version") {
				return toolrun.Output{Stdout: []byte("sg 0.30.0\n")}, nil
			}
			if fail {
				return toolrun.Output{}, context.DeadlineExceeded
			}
			scans++
			return toolrun.Output{Stdout: []byte(`[{"text":"x","file":"a.go","range":{"start":{"line":0,"column":0}},"rule":{"id":"r"}}]`)}, nil
		},
	}
	a := astgrep.New(runner)
	a.Cache = factcache.NewStore(t.TempDir())
	ctx := context.Background()
	s := scope.Scope{Root: root}
	patterns := config.PatternConfig{{ID: "p1", Lang: "go", Rule: "func $F"}}

	find := func() error {
		_, _, err := a.Find(ctx, s, patterns)
		return err
	}

	if err := find(); err != nil {
		t.Fatalf("cold Find: %v", err)
	}
	if err := find(); err != nil {
		t.Fatalf("warm Find: %v", err)
	}
	if scans != 1 {
		t.Fatalf("want warm Find served from cache (1 sg run), got %d", scans)
	}

	// Source edit invalidates; make the run fail like a timeout — the failed
	// run must not be cached, so the following healthy run scans again.
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n\nvar X = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fail = true
	if err := find(); err == nil {
		t.Fatal("timed-out Find: want error, got nil")
	}
	fail = false
	if err := find(); err != nil {
		t.Fatalf("healthy Find after timeout: %v", err)
	}
	if scans != 2 {
		t.Errorf("want healthy run to scan (timeout not cached), got %d scans", scans)
	}
}
