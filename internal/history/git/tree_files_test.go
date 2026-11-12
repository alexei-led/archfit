package git_test

import (
	"context"
	"slices"
	"testing"

	gitpkg "github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// TestTreeFiles lists the files of the working tree id: nested, untracked and
// odd names included, a deleted file and an ignored one out.
func TestTreeFiles(t *testing.T) {
	requireGit(t)
	t.Parallel()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	writeIn(t, dir, ".gitignore", "*.log\n")
	writeIn(t, dir, "a.go", "a\n")
	writeIn(t, dir, "gone.go", "g\n")
	writeIn(t, dir, "sub/deep/b.go", "b\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "base")
	writeIn(t, dir, "new dir/with space.go", "n\n")
	writeIn(t, dir, "x.log", "ignored\n")
	gitIn(t, dir, "rm", "-q", "gone.go")

	runner := toolrun.New()
	tree, err := gitpkg.WorkingTreeID(context.Background(), dir, runner)
	if err != nil {
		t.Fatalf("WorkingTreeID: %v", err)
	}
	got, err := gitpkg.TreeFiles(context.Background(), dir, tree, runner)
	if err != nil {
		t.Fatalf("TreeFiles: %v", err)
	}
	want := []string{".gitignore", "a.go", "new dir/with space.go", "sub/deep/b.go"}
	if !slices.Equal(got, want) {
		t.Errorf("TreeFiles = %q, want %q", got, want)
	}
}

func TestTreeFilesErrors(t *testing.T) {
	requireGit(t)
	t.Parallel()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	for _, tree := range []string{"--help", "0123456789abcdef0123456789abcdef01234567"} {
		if _, err := gitpkg.TreeFiles(context.Background(), dir, tree, toolrun.New()); err == nil {
			t.Errorf("TreeFiles(%q): no error", tree)
		}
	}
}
