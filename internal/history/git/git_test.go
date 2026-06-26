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

	cs, err := gitpkg.Changed(ctx, "", "main", "HEAD", "", mock)
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

	_, err := gitpkg.Changed(ctx, "", "bad", "ref", "", mock)
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

	_, err := gitpkg.Changed(ctx, "", "HEAD~1", "HEAD", "", runner)
	if err != nil {
		t.Skip("no parent commit or git error, skipping real Changed test")
	}
}

// --- Prefix / rebase tests ---

func TestHistory_EmptyPrefix_Noop(t *testing.T) {
	ctx := context.Background()
	var capturedArgs []string
	mock := &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			capturedArgs = cmd.Args
			return toolrun.Output{Stdout: []byte("@\nsubdir/a.go\nsubdir/b.go\n"), ExitCode: 0}, nil
		},
	}

	churn, _, _, err := gitpkg.History(ctx, "", "", mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, arg := range capturedArgs {
		if arg == "--" {
			t.Errorf("args must not contain '--' for empty prefix; got %v", capturedArgs)
			break
		}
	}
	if churn["subdir/a.go"] == 0 {
		t.Errorf("expected churn entry for subdir/a.go")
	}
	if churn["subdir/b.go"] == 0 {
		t.Errorf("expected churn entry for subdir/b.go")
	}
}

func TestHistory_Prefix_FilterAndRebase(t *testing.T) {
	ctx := context.Background()
	var capturedArgs []string
	mock := &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			capturedArgs = cmd.Args
			return toolrun.Output{
				Stdout:   []byte("@\nservices/api/handler.go\nservices/api/routes.go\nother/file.go\n"),
				ExitCode: 0,
			}, nil
		},
	}

	churn, _, _, err := gitpkg.History(ctx, "/repo", "services/api", mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hasDashDash := false
	hasPrefix := false
	for i, arg := range capturedArgs {
		if arg == "--" {
			hasDashDash = true
			if i+1 < len(capturedArgs) && capturedArgs[i+1] == "services/api" {
				hasPrefix = true
			}
		}
	}
	if !hasDashDash || !hasPrefix {
		t.Errorf("args must contain '-- services/api'; got %v", capturedArgs)
	}
	if churn["handler.go"] == 0 {
		t.Errorf("expected churn entry for handler.go (rebased)")
	}
	if churn["routes.go"] == 0 {
		t.Errorf("expected churn entry for routes.go (rebased)")
	}
	if churn["other/file.go"] != 0 {
		t.Errorf("other/file.go must be excluded (outside subtree)")
	}
}

func TestChanged_EmptyPrefix_Noop(t *testing.T) {
	ctx := context.Background()
	var capturedArgs []string
	mock := &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			capturedArgs = cmd.Args
			return toolrun.Output{Stdout: []byte("b.go\na.go\n"), ExitCode: 0}, nil
		},
	}

	cs, err := gitpkg.Changed(ctx, "", "main", "HEAD", "", mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"a.go", "b.go"}
	if len(cs.Files) != len(want) {
		t.Fatalf("files: got %v, want %v", cs.Files, want)
	}
	for i, f := range want {
		if cs.Files[i] != f {
			t.Errorf("files[%d]: got %q, want %q", i, cs.Files[i], f)
		}
	}
	for _, arg := range capturedArgs {
		if arg == "--" {
			t.Errorf("args must not contain '--' for empty prefix; got %v", capturedArgs)
			break
		}
	}
}

func TestChanged_Prefix_FilterAndRebase(t *testing.T) {
	ctx := context.Background()
	var capturedArgs []string
	mock := &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			capturedArgs = cmd.Args
			return toolrun.Output{
				Stdout:   []byte("services/api/routes.go\nservices/api/handler.go\nother/main.go\n"),
				ExitCode: 0,
			}, nil
		},
	}

	cs, err := gitpkg.Changed(ctx, "/repo", "main", "HEAD", "services/api", mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"handler.go", "routes.go"}
	if len(cs.Files) != len(want) {
		t.Fatalf("files: got %v, want %v", cs.Files, want)
	}
	for i, f := range want {
		if cs.Files[i] != f {
			t.Errorf("files[%d]: got %q, want %q", i, cs.Files[i], f)
		}
	}
	for _, f := range cs.Files {
		if f == "other/main.go" {
			t.Errorf("other/main.go must be excluded (outside subtree)")
		}
	}

	hasDashDash := false
	hasPrefix := false
	for i, arg := range capturedArgs {
		if arg == "--" {
			hasDashDash = true
			if i+1 < len(capturedArgs) && capturedArgs[i+1] == "services/api" {
				hasPrefix = true
			}
		}
	}
	if !hasDashDash || !hasPrefix {
		t.Errorf("args must contain '-- services/api'; got %v", capturedArgs)
	}
}

func TestChanged_OutsideSubtree_Excluded(t *testing.T) {
	ctx := context.Background()
	mock := &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{Stdout: []byte("completely/outside.go\n"), ExitCode: 0}, nil
		},
	}

	cs, err := gitpkg.Changed(ctx, "", "main", "HEAD", "myapp", mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cs.Files) != 0 {
		t.Errorf("expected empty files (all outside subtree), got %v", cs.Files)
	}
}
