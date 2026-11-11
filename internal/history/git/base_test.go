package git_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	gitpkg "github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// noSuchRef names no commit in any test repository.
const noSuchRef = "no-such-commit"

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

// gitOut runs git in dir like gitIn and returns its trimmed stdout.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...) //nolint:gosec // fixed binary, test args
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func TestBaseCommitIsTheForkPoint(t *testing.T) {
	requireGit(t)
	t.Parallel()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	writeIn(t, dir, "svc/a.go", "package svc\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "fork")
	fork := gitOut(t, dir, "rev-parse", "HEAD")
	gitIn(t, dir, "checkout", "-qb", "feature")
	writeIn(t, dir, "svc/b.go", "package svc\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "work")
	gitIn(t, dir, "checkout", "-q", "main")
	writeIn(t, dir, "svc/c.go", "package svc\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "target moves on")
	gitIn(t, dir, "checkout", "-q", "feature")

	for _, from := range []string{dir, filepath.Join(dir, "svc")} {
		got, err := gitpkg.BaseCommit(context.Background(), from, "main", toolrun.New())
		if err != nil || got != fork {
			t.Errorf("BaseCommit from %s = %q, %v; want the fork %q", from, got, err, fork)
		}
	}
}

func TestBaseCommitRefusesWhatItCannotCompare(t *testing.T) {
	requireGit(t)
	t.Parallel()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	writeIn(t, dir, "a.go", "package a\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "main")
	gitIn(t, dir, "checkout", "-q", "--orphan", "orphan")
	gitIn(t, dir, "commit", "-qm", "no shared history")

	// A depth-1 clone: git still finds a merge base, but it may be the cut.
	src := t.TempDir()
	gitIn(t, src, "init", "-q")
	for _, msg := range []string{"one", "two", "three"} {
		gitIn(t, src, "commit", "-q", "--allow-empty", "-m", msg)
	}
	shallow := filepath.Join(t.TempDir(), "shallow")
	gitIn(t, filepath.Dir(shallow), "clone", "-q", "--depth", "1", "file://"+src, shallow)

	for _, tc := range []struct {
		name, dir, target string
		want              error // nil: any error that is neither sentinel
	}{
		{name: "no shared commit", dir: dir, target: "main", want: gitpkg.ErrNoMergeBase},
		{name: "shallow clone", dir: shallow, target: "HEAD", want: gitpkg.ErrShallowClone},
		{name: "unknown target", dir: dir, target: "no-such-branch"},
		{name: "target read as an option", dir: dir, target: "--all"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := gitpkg.BaseCommit(context.Background(), tc.dir, tc.target, toolrun.New())
			if err == nil {
				t.Fatalf("BaseCommit = %q, want an error", got)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Errorf("BaseCommit error = %v, want %v", err, tc.want)
			}
			if tc.want == nil && (errors.Is(err, gitpkg.ErrNoMergeBase) || errors.Is(err, gitpkg.ErrShallowClone)) {
				t.Errorf("BaseCommit error = %v, want a plain error", err)
			}
		})
	}
}

func TestTreeOfNamesTheContent(t *testing.T) {
	requireGit(t)
	t.Parallel()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	writeIn(t, dir, "a.go", "package a\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "one")
	first := gitOut(t, dir, "rev-parse", "HEAD")
	gitIn(t, dir, "commit", "-q", "--allow-empty", "-m", "same tree")

	runner := toolrun.New()
	a, err := gitpkg.TreeOf(context.Background(), dir, first, runner)
	if err != nil {
		t.Fatal(err)
	}
	b, err := gitpkg.TreeOf(context.Background(), dir, "HEAD", runner)
	if err != nil || a != b || a == first {
		t.Errorf("TreeOf = %q and %q (%v); want one tree id for two commits with the same files", a, b, err)
	}
	// rev-parse echoes an argument it reads as an option and exits 0.
	for _, commit := range []string{noSuchRef, "--all"} {
		if got, err := gitpkg.TreeOf(context.Background(), dir, commit, runner); err == nil {
			t.Errorf("TreeOf(%q) = %q, want an error", commit, got)
		}
	}
}

// TestSnapshotTreeEqualsGitArchive: on a tree without export attributes the
// snapshot holds what `git archive` holds, and the repository the user works
// in does not change: not the index, not the working tree.
func TestSnapshotTreeEqualsGitArchive(t *testing.T) {
	requireGit(t)
	if _, err := exec.LookPath("tar"); err != nil {
		t.Skip("tar not installed")
	}
	t.Parallel()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	writeIn(t, dir, "pkg/a/a.go", "package a\n")
	writeIn(t, dir, "dir with space/b.go", "package b\n")
	writeIn(t, dir, "run.sh", "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(dir, "run.sh"), 0o755); err != nil { //nolint:gosec // an executable fixture
		t.Fatal(err)
	}
	if err := os.Symlink("pkg/a/a.go", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "base")
	commit := gitOut(t, dir, "rev-parse", "HEAD")

	// Staged, unstaged, deleted and untracked work the snapshot must not see.
	writeIn(t, dir, "pkg/a/a.go", "package a\n// staged\n")
	gitIn(t, dir, "add", "pkg/a/a.go")
	writeIn(t, dir, "pkg/a/a.go", "package a\n// staged\n// unstaged\n")
	writeIn(t, dir, "new.go", "package main\n")
	if err := os.Remove(filepath.Join(dir, "run.sh")); err != nil {
		t.Fatal(err)
	}
	status := gitOut(t, dir, "status", "--porcelain")
	index := readFile(t, filepath.Join(dir, ".git", "index"))

	snap := filepath.Join(t.TempDir(), "snap")
	if err := gitpkg.SnapshotTree(context.Background(), filepath.Join(dir, "pkg"), commit, snap, toolrun.New()); err != nil {
		t.Fatalf("SnapshotTree: %v", err)
	}

	got := describeTree(t, snap)
	if want := describeTree(t, extractArchive(t, dir, commit)); !maps.Equal(got, want) {
		t.Errorf("snapshot differs from git archive:\nsnapshot: %v\narchive:  %v", got, want)
	}
	for file, want := range map[string]string{
		"pkg/a/a.go":          `file exec=false "package a\n"`,
		"dir with space/b.go": `file exec=false "package b\n"`,
		"run.sh":              `file exec=true "#!/bin/sh\n"`,
		"link":                "symlink pkg/a/a.go",
	} {
		if got[file] != want {
			t.Errorf("snapshot %s = %q, want %q", file, got[file], want)
		}
	}
	if len(got) != 4 {
		t.Errorf("snapshot holds %d files, want the 4 committed: %v", len(got), got)
	}
	if got := gitOut(t, dir, "status", "--porcelain"); got != status {
		t.Errorf("git status after the snapshot = %q, want %q", got, status)
	}
	if !bytes.Equal(readFile(t, filepath.Join(dir, ".git", "index")), index) {
		t.Error("the snapshot changed the repository's index")
	}
	if got := string(readFile(t, filepath.Join(dir, "pkg/a/a.go"))); got != "package a\n// staged\n// unstaged\n" {
		t.Errorf("the snapshot changed the working tree: pkg/a/a.go = %q", got)
	}
}

// TestSnapshotTreeKeepsExportIgnoredFiles: export attributes shape an archive,
// not the code. A file `git archive` leaves out is still in the base, or its
// unchanged code would look added on the head.
func TestSnapshotTreeKeepsExportIgnoredFiles(t *testing.T) {
	requireGit(t)
	if _, err := exec.LookPath("tar"); err != nil {
		t.Skip("tar not installed")
	}
	t.Parallel()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	writeIn(t, dir, ".gitattributes", "internal.go export-ignore\nversion.go export-subst\n")
	writeIn(t, dir, "internal.go", "package a\n")
	writeIn(t, dir, "version.go", "package a\n// $Format:%H$\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "base")

	snap := filepath.Join(t.TempDir(), "snap")
	if err := gitpkg.SnapshotTree(context.Background(), dir, "HEAD", snap, toolrun.New()); err != nil {
		t.Fatalf("SnapshotTree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(extractArchive(t, dir, "HEAD"), "internal.go")); !os.IsNotExist(err) {
		t.Fatalf("git archive kept internal.go (%v); the fixture no longer shows the difference", err)
	}
	if got := string(readFile(t, filepath.Join(snap, "internal.go"))); got != "package a\n" {
		t.Errorf("snapshot internal.go = %q, want the committed file", got)
	}
	if got := string(readFile(t, filepath.Join(snap, "version.go"))); got != "package a\n// $Format:%H$\n" {
		t.Errorf("snapshot version.go = %q, want the committed bytes", got)
	}
}

// TestSnapshotTreeRefusesWhatIsNotACommit: an unknown commit is an error,
// and so is an option, which read-tree would take (--empty gives an empty
// base, and every head line would look added).
func TestSnapshotTreeRefusesWhatIsNotACommit(t *testing.T) {
	requireGit(t)
	t.Parallel()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	gitIn(t, dir, "commit", "-q", "--allow-empty", "-m", "base")
	for _, commit := range []string{noSuchRef, "--empty"} {
		if err := gitpkg.SnapshotTree(context.Background(), dir, commit, t.TempDir(), toolrun.New()); err == nil {
			t.Errorf("SnapshotTree(%q) = nil error", commit)
		}
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // test temp file
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// extractArchive writes `git archive <rev>` into a new directory with tar.
func extractArchive(t *testing.T, repo, rev string) string {
	t.Helper()
	tarball := filepath.Join(t.TempDir(), "tree.tar")
	gitIn(t, repo, "archive", "--format=tar", "-o", tarball, rev)
	out := t.TempDir()
	if msg, err := exec.Command("tar", "-xf", tarball, "-C", out).CombinedOutput(); err != nil { //nolint:gosec // fixed binary, test paths
		t.Fatalf("tar: %v\n%s", err, msg)
	}
	return out
}

// describeTree maps each file under root to its kind, executable bit and
// content, or to its symlink target.
func describeTree(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			files[rel] = "symlink " + target
			return err
		}
		data, err := os.ReadFile(path) //nolint:gosec // test temp tree
		files[rel] = fmt.Sprintf("file exec=%t %q", info.Mode()&0o111 != 0, data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
