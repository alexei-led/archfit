package git_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	gitpkg "github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// TestWorkingTreeID pins what the id of the working tree sees: a committed
// tree gives HEAD's tree; an unstaged edit, a staged edit, an untracked file
// and a deletion each give a new id; an ignored file, archfit's cache
// directory and the directory the call starts in do not; going back to the
// same content gives the same id. The repository's index and status do not
// change.
func TestWorkingTreeID(t *testing.T) {
	requireGit(t)
	t.Parallel()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	writeIn(t, dir, ".gitignore", "*.log\n")
	writeIn(t, dir, "a.go", "a\n")
	writeIn(t, dir, "sub/b.go", "b\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "base")
	runner := toolrun.New()
	id := func() string {
		t.Helper()
		got, err := gitpkg.WorkingTreeID(context.Background(), dir, runner)
		if err != nil {
			t.Fatalf("WorkingTreeID: %v", err)
		}
		return got
	}

	clean := id()
	if head := gitOut(t, dir, "rev-parse", "HEAD^{tree}"); clean != head {
		t.Fatalf("clean id = %s, want HEAD's tree %s", clean, head)
	}
	fromSub, err := gitpkg.WorkingTreeID(context.Background(), filepath.Join(dir, "sub"), runner)
	if err != nil || fromSub != clean {
		t.Errorf("id from sub = %s (%v), want %s", fromSub, err, clean)
	}
	writeIn(t, dir, "x.log", "ignored\n")
	if got := id(); got != clean {
		t.Errorf("an ignored file changed the id: %s, want %s", got, clean)
	}
	writeIn(t, dir, ".archfit-cache/last.json", "{}\n")
	if got := id(); got != clean {
		t.Errorf("archfit's own cache, not ignored, changed the id: %s, want %s", got, clean)
	}

	writeIn(t, dir, "a.go", "a\nedited\n")
	edited := id()
	if edited == clean {
		t.Error("an unstaged edit kept the id")
	}
	status := gitOut(t, dir, "status", "--porcelain")
	gitIn(t, dir, "add", "a.go")
	if got := id(); got != edited {
		t.Errorf("staging the edit changed the id: %s, want %s", got, edited)
	}
	if got := gitOut(t, dir, "status", "--porcelain"); got == status {
		t.Fatal("git add did not stage the edit; the next check is void")
	}
	status = gitOut(t, dir, "status", "--porcelain")
	_ = id()
	if got := gitOut(t, dir, "status", "--porcelain"); got != status {
		t.Errorf("git status after WorkingTreeID = %q, want %q", got, status)
	}

	writeIn(t, dir, "new.go", "new\n")
	untracked := id()
	if untracked == edited {
		t.Error("an untracked file kept the id")
	}
	if err := os.Remove(filepath.Join(dir, "sub", "b.go")); err != nil {
		t.Fatal(err)
	}
	if got := id(); got == untracked {
		t.Error("a deletion kept the id")
	}

	gitIn(t, dir, "reset", "-q", "--hard")
	if err := os.Remove(filepath.Join(dir, "new.go")); err != nil {
		t.Fatal(err)
	}
	if got := id(); got != clean {
		t.Errorf("the same content gave %s, want %s", got, clean)
	}
}

func TestWorkingTreeIDOutsideARepository(t *testing.T) {
	requireGit(t)
	t.Parallel()
	if _, err := gitpkg.WorkingTreeID(context.Background(), t.TempDir(), toolrun.New()); err == nil {
		t.Error("WorkingTreeID outside a repository: want an error")
	}
}
