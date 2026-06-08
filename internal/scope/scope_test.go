package scope_test

import (
	"context"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

func newMockRunner(runFunc func(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error)) *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: "git", Path: "/usr/bin/git"}, true
		},
		RunFunc: runFunc,
	}
}

func TestResolve_Full(t *testing.T) {
	mock := newMockRunner(func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
		for _, a := range cmd.Args {
			if a == "--show-toplevel" {
				return toolrun.Output{Stdout: []byte("/fake/root\n")}, nil
			}
		}
		return toolrun.Output{}, nil
	})

	s, err := scope.Resolve(context.Background(), config.ScopeConfig{Full: true}, mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Mode != scope.ModeFull {
		t.Errorf("mode: got %q, want %q", s.Mode, scope.ModeFull)
	}
	if s.Root != "/fake/root" {
		t.Errorf("root: got %q, want %q", s.Root, "/fake/root")
	}
	if len(s.Changed) != 0 {
		t.Errorf("changed: expected empty, got %v", s.Changed)
	}
}

func TestResolve_Delta(t *testing.T) {
	mock := newMockRunner(func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
		for _, a := range cmd.Args {
			if a == "--show-toplevel" {
				return toolrun.Output{Stdout: []byte("/fake/root\n")}, nil
			}
		}
		// rev-parse HEAD: last arg is "HEAD"
		if len(cmd.Args) > 0 && cmd.Args[len(cmd.Args)-1] == "HEAD" {
			return toolrun.Output{Stdout: []byte("abc123\n")}, nil
		}
		// diff --name-only
		return toolrun.Output{Stdout: []byte("z.go\na.go\n")}, nil
	})

	s, err := scope.Resolve(context.Background(), config.ScopeConfig{Base: "main"}, mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Mode != scope.ModeDelta {
		t.Errorf("mode: got %q, want %q", s.Mode, scope.ModeDelta)
	}
	if s.Root != "/fake/root" {
		t.Errorf("root: got %q, want %q", s.Root, "/fake/root")
	}
	if s.Head != "abc123" {
		t.Errorf("head: got %q, want %q", s.Head, "abc123")
	}
	want := []string{"a.go", "z.go"}
	if len(s.Changed) != len(want) {
		t.Fatalf("changed len: got %d, want %d", len(s.Changed), len(want))
	}
	for i, f := range want {
		if s.Changed[i] != f {
			t.Errorf("changed[%d]: got %q, want %q", i, s.Changed[i], f)
		}
	}
}

func TestResolve_MissingGit(t *testing.T) {
	mock := newMockRunner(func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
		return toolrun.Output{ExitCode: 128, Stderr: []byte("not a git repo")}, nil
	})

	_, err := scope.Resolve(context.Background(), config.ScopeConfig{Full: true}, mock)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
