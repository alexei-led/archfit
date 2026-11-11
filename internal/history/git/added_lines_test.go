package git_test

import (
	"bytes"
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	gitpkg "github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// TestAddedLines pins what the change adds against its base, line by line:
// committed, staged, unstaged and untracked work counts, and a file git shows
// as binary is read as text; a rename (git mv or a plain mv), a file only no
// longer tracked, a deletion, an ignored file, an empty file and the target's
// own later commits add nothing; an added line that reads like a diff header
// is a line, not a header. The repository's index and status do not change.
func TestAddedLines(t *testing.T) {
	requireGit(t)
	t.Parallel()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	writeIn(t, dir, ".gitignore", "*.log\n")
	writeIn(t, dir, ".gitattributes", "gen.pb.go -diff\n")
	writeIn(t, dir, "gen.pb.go", "package gen\n")
	writeIn(t, dir, "nul.go", "a\x00\nb\n")
	writeIn(t, dir, "mv.go", "package x\nimport \"a\"\nimport \"b\"\n")
	writeIn(t, dir, "tracked.go", "t\n")
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
	writeIn(t, dir, "gen.pb.go", "package gen\nimport \"z\"\n")
	writeIn(t, dir, "nul.go", "a\x00\nB\n")
	if err := os.Rename(filepath.Join(dir, "mv.go"), filepath.Join(dir, "mvd.go")); err != nil {
		t.Fatal(err)
	}
	writeIn(t, dir, "mvd.go", "package x\nimport \"a\"\nimport \"b\"\nimport \"c\"\n")
	gitIn(t, dir, "rm", "-q", "--cached", "tracked.go")
	status := gitOut(t, dir, "status", "--porcelain")
	index := readFile(t, filepath.Join(dir, ".git", "index"))

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
		"gen.pb.go":  {2},
		"nul.go":     {2},
		"mvd.go":     {4},
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
	if got := gitOut(t, dir, "status", "--porcelain"); got != status {
		t.Errorf("git status after AddedLines = %q, want %q", got, status)
	}
	if !bytes.Equal(readFile(t, filepath.Join(dir, ".git", "index")), index) {
		t.Error("AddedLines changed the repository's index")
	}
}

// TestAddedLinesIgnoresDiffConfig: git config that changes the shape of a
// diff (context between hunks, prefixes, renames, the algorithm, colour, an
// external diff, quoting) changes nothing in the added lines.
func TestAddedLinesIgnoresDiffConfig(t *testing.T) {
	requireGit(t)
	t.Parallel()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	writeIn(t, dir, "hunks.go", "a\n\nc\nd\ne\n")
	writeIn(t, dir, "old.go", "1\n2\n3\n4\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "base")
	base := gitOut(t, dir, "rev-parse", "HEAD")
	writeIn(t, dir, "hunks.go", "A\n\nc\nD\ne\n")
	gitIn(t, dir, "mv", "old.go", "new.go")
	writeIn(t, dir, "é.go", "x\n")

	want := evidence.AddedLines{"hunks.go": {1, 4}, "é.go": {1}}
	runner := toolrun.New()
	plain, err := gitpkg.AddedLines(context.Background(), dir, base, runner)
	if err != nil || !maps.EqualFunc(plain, want, slices.Equal) {
		t.Fatalf("AddedLines without config = %v, %v; want %v", plain, err, want)
	}
	for _, setting := range []string{
		"diff.interHunkContext=5", "diff.suppressBlankEmpty=true", "diff.noprefix=true",
		"diff.mnemonicPrefix=true", "diff.renames=false", "diff.algorithm=histogram",
		"diff.external=false", "color.ui=always", "core.quotePath=false",
	} {
		key, value, _ := strings.Cut(setting, "=")
		gitIn(t, dir, "config", key, value)
	}
	got, err := gitpkg.AddedLines(context.Background(), dir, base, runner)
	if err != nil || !maps.EqualFunc(got, want, slices.Equal) {
		t.Errorf("AddedLines with diff config = %v, %v; want %v", got, err, want)
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

	for _, base := range []string{noSuchRef, "--cached", "-R"} {
		if got, err := gitpkg.AddedLines(context.Background(), dir, base, toolrun.New()); err == nil {
			t.Errorf("AddedLines(%q) = %v, want an error", base, got)
		}
	}
}
