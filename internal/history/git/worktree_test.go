package git

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/toolrun"
)

func TestSubtreeInWorktreeRejectsParentEscape(t *testing.T) {
	t.Parallel()
	got, err := SubtreeInWorktree("/repo", filepath.Join("/repo", "..fixtures"), "/wt")
	if err != nil || got != filepath.Join("/wt", "..fixtures") {
		t.Fatalf("valid ..fixtures subtree = %q, %v", got, err)
	}
	if _, err := SubtreeInWorktree("/repo", filepath.Join("/repo", "..", "sibling"), "/wt"); err == nil {
		t.Fatal("parent escape accepted")
	}
}

func TestWorktreeParentLocksDeterministicDir(t *testing.T) {
	sha := strings.Repeat("a", 40)
	runner := &toolrun.RunnerMock{RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
		if cmd.Name == gitBinary && len(cmd.Args) >= 3 && cmd.Args[0] == "rev-parse" {
			return toolrun.Output{Stdout: []byte(sha + "\n")}, nil
		}
		return toolrun.Output{}, nil
	}}
	dir := t.TempDir()
	first, release, err := WorktreeParent(context.Background(), runner, dir, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	if want := filepath.Join(dir, ".archfit-cache", "worktrees", sha); first != want {
		t.Fatalf("base worktree parent = %q, want %q", first, want)
	}
}
