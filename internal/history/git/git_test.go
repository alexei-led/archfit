package git_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	gitpkg "github.com/alexei-led/archfit/internal/history/git"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// --- Mock-based tests ---

func TestChanged_Mock(t *testing.T) {
	ctx := context.Background()
	mock := &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{Stdout: []byte("b.go\na.go\n"), ExitCode: 0}, nil
		},
	}

	cs, err := gitpkg.Changed(ctx, "", "main", "HEAD", mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cs.Base != "main" {
		t.Errorf("Base: got %q, want %q", cs.Base, "main")
	}
	if cs.Head != "HEAD" {
		t.Errorf("Head: got %q, want %q", cs.Head, "HEAD")
	}
	want := []string{"a.go", "b.go"}
	if len(cs.Files) != len(want) {
		t.Fatalf("Files len: got %d, want %d", len(cs.Files), len(want))
	}
	for i, f := range cs.Files {
		if f != want[i] {
			t.Errorf("Files[%d]: got %q, want %q", i, f, want[i])
		}
	}
}

func TestChanged_NonZeroExit(t *testing.T) {
	ctx := context.Background()
	mock := &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{Stderr: []byte("fatal: bad revision"), ExitCode: 128}, nil
		},
	}

	_, err := gitpkg.Changed(ctx, "", "bad", "ref", mock)
	if err == nil {
		t.Fatal("expected error for non-zero exit, got nil")
	}
}

func TestHeadRef_Mock(t *testing.T) {
	const sha = "abc1234567890abcdef1234567890abcdef123456"
	ctx := context.Background()
	mock := &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{Stdout: []byte(sha + "\n"), ExitCode: 0}, nil
		},
	}

	got, err := gitpkg.HeadRef(ctx, "", mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != sha {
		t.Errorf("HeadRef: got %q, want %q", got, sha)
	}
}

// --- Real-git tests (archfit repo is itself a git repo) ---

func TestRepoRoot_Real(t *testing.T) {
	ctx := context.Background()
	runner := toolrun.New()

	root, err := gitpkg.RepoRoot(ctx, "", runner)
	if err != nil {
		t.Fatalf("RepoRoot: %v", err)
	}
	if root == "" {
		t.Fatal("RepoRoot returned empty string")
	}
	if !filepath.IsAbs(root) {
		t.Errorf("RepoRoot returned non-absolute path: %q", root)
	}
}

func TestHeadRef_Real(t *testing.T) {
	ctx := context.Background()
	runner := toolrun.New()

	sha, err := gitpkg.HeadRef(ctx, "", runner)
	if err != nil {
		t.Fatalf("HeadRef: %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("HeadRef: expected 40-char SHA, got %d chars: %q", len(sha), sha)
	}
	for _, c := range sha {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Errorf("HeadRef: non-hex char %q in SHA %q", c, sha)
			break
		}
	}
}

func TestChanged_Real(t *testing.T) {
	ctx := context.Background()
	runner := toolrun.New()

	_, err := gitpkg.Changed(ctx, "", "HEAD~1", "HEAD", runner)
	if err != nil {
		t.Skip("no parent commit or git error, skipping real Changed test")
	}
}
