package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	gitpkg "github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// gitIn runs git in dir with a fixed identity and no user config.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main"}, args...)...) //nolint:gosec // fixed binary, test args
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeIn(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestAdded_Real pins what counts as added: committed, untracked, renamed-to,
// and oddly named files are listed with repository-relative names from any
// directory; a modified file is not; a file the base deleted after the fork is
// not the branch's.
func TestAdded_Real(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	writeIn(t, dir, "svc/kept.go", "package svc\n")
	writeIn(t, dir, "svc/old.go", "package svc\n// old\n")
	writeIn(t, dir, "legacy/x.go", "package legacy\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "base")
	gitIn(t, dir, "checkout", "-qb", "feature")

	// The base branch deletes legacy/x.go after the fork.
	gitIn(t, dir, "checkout", "-q", "main")
	gitIn(t, dir, "rm", "-q", "legacy/x.go")
	gitIn(t, dir, "commit", "-qm", "drop legacy")
	gitIn(t, dir, "checkout", "-q", "feature")

	writeIn(t, dir, "svc/new.go", "package svc\n")
	gitIn(t, dir, "mv", "svc/old.go", "svc/renamed.go")
	writeIn(t, dir, "svc/kept.go", "package svc\n// modified\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "work")
	writeIn(t, dir, "svc/untracked.go", "package svc\n")
	writeIn(t, dir, "svc/é quoted.go", "package svc\n")

	want := []string{"svc/new.go", "svc/renamed.go", "svc/untracked.go", "svc/é quoted.go"}
	for _, from := range []string{dir, filepath.Join(dir, "svc")} {
		got, err := gitpkg.Added(context.Background(), from, "main", toolrun.New())
		if err != nil {
			t.Fatalf("Added from %s: %v", from, err)
		}
		if !slices.Equal(got, want) {
			t.Errorf("Added from %s = %q, want %q", from, got, want)
		}
	}
}

// TestAdded_UnbornHEAD: before the first commit every staged and untracked
// file is added.
func TestAdded_UnbornHEAD(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	writeIn(t, dir, "staged.go", "package x\n")
	gitIn(t, dir, "add", "staged.go")
	writeIn(t, dir, ".tools/x.go", "package tools\n")

	got, err := gitpkg.Added(context.Background(), dir, "HEAD", toolrun.New())
	if err != nil {
		t.Fatalf("Added: %v", err)
	}
	if want := []string{".tools/x.go", "staged.go"}; !slices.Equal(got, want) {
		t.Errorf("Added = %q, want %q", got, want)
	}
}
