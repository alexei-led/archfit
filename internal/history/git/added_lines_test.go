package git_test

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	gitpkg "github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// TestAddedLines pins what the change adds against its base, line by line:
// committed, staged, unstaged and untracked work counts; a pure rename, a
// deletion, an ignored file, an empty file and the target's own later commits
// do not; an added line that reads like a diff header is a line, not a header.
func TestAddedLines(t *testing.T) {
	requireGit(t)
	t.Parallel()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	writeIn(t, dir, ".gitignore", "*.log\n")
	writeIn(t, dir, "mod.go", "a\nb\nc\n")
	writeIn(t, dir, "old.go", "package x\n// one\n// two\n")
	writeIn(t, dir, "moved.go", "1\n2\n3\n4\n5\n")
	writeIn(t, dir, "gone.go", "x\n")
	writeIn(t, dir, "nonl.go", "a\nb")
	writeIn(t, dir, "sp ace.go", "x\n")
	writeIn(t, dir, "q\"t\tx.go", "x\n")
	writeIn(t, dir, "é.go", "x\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "base")
	gitIn(t, dir, "checkout", "-qb", "feature")

	// The target moves on after the fork; its commits are not the change's.
	gitIn(t, dir, "checkout", "-q", "main")
	writeIn(t, dir, "later.go", "x\n")
	writeIn(t, dir, "mod.go", "a\nb\nc\nlater\n")
	gitIn(t, dir, "commit", "-qam", "target")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "target adds later.go")
	gitIn(t, dir, "checkout", "-q", "feature")

	// Committed on the branch.
	writeIn(t, dir, "mod.go", "a\nB\nc\n")
	gitIn(t, dir, "mv", "old.go", "renamed.go")
	gitIn(t, dir, "mv", "moved.go", "moved2.go")
	writeIn(t, dir, "moved2.go", "1\n2\nthree\n4\n5\n")
	gitIn(t, dir, "rm", "-q", "gone.go")
	writeIn(t, dir, "nonl.go", "a\nc")
	writeIn(t, dir, "plus.go", "++ b/fake.go\n+++ b/fake.go\nz\n")
	writeIn(t, dir, "sp ace.go", "y\n")
	writeIn(t, dir, "q\"t\tx.go", "y\n")
	writeIn(t, dir, "é.go", "y\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "work")
	// Staged, unstaged and untracked.
	writeIn(t, dir, "staged.go", "s\n")
	gitIn(t, dir, "add", "staged.go")
	writeIn(t, dir, "mod.go", "a\nB\nc\nd\n")
	writeIn(t, dir, "new/u.go", "u1\nu2")
	writeIn(t, dir, "empty.go", "")
	writeIn(t, dir, "x.log", "ignored\n")
	if err := os.Symlink("mod.go", filepath.Join(dir, "ln.go")); err != nil {
		t.Fatal(err)
	}

	want := evidence.AddedLines{
		"mod.go":     {2, 4},
		"moved2.go":  {3},
		"nonl.go":    {2},
		"plus.go":    {1, 2, 3},
		"sp ace.go":  {1},
		"q\"t\tx.go": {1},
		"é.go":       {1},
		"staged.go":  {1},
		"new/u.go":   {1, 2},
		"ln.go":      {1},
	}
	runner := toolrun.New()
	for _, from := range []string{dir, filepath.Join(dir, "new")} {
		base, err := gitpkg.BaseCommit(context.Background(), from, "main", runner)
		if err != nil {
			t.Fatalf("BaseCommit from %s: %v", from, err)
		}
		got, err := gitpkg.AddedLines(context.Background(), from, base, runner)
		if err != nil {
			t.Fatalf("AddedLines from %s: %v", from, err)
		}
		if !maps.EqualFunc(got, want, slices.Equal) {
			t.Errorf("AddedLines from %s =\n%v\nwant\n%v", from, got, want)
		}
	}
}

func TestAddedLinesOfAnUnchangedTreeIsEmpty(t *testing.T) {
	requireGit(t)
	t.Parallel()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	writeIn(t, dir, "a.go", "package a\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "base")

	got, err := gitpkg.AddedLines(context.Background(), dir, "HEAD", toolrun.New())
	if err != nil || len(got) != 0 {
		t.Errorf("AddedLines = %v, %v; want none", got, err)
	}
}

func TestAddedLinesRefusesABadBase(t *testing.T) {
	requireGit(t)
	t.Parallel()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	writeIn(t, dir, "a.go", "package a\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "base")

	for _, base := range []string{"no-such-commit", "--cached"} {
		if got, err := gitpkg.AddedLines(context.Background(), dir, base, toolrun.New()); err == nil {
			t.Errorf("AddedLines(%q) = %v, want an error", base, got)
		}
	}
}
