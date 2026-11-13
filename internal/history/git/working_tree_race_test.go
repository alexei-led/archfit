package git_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	gitpkg "github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// TestWorkingTreeIDWhileTheCacheIsWritten proves archfit's own cache writes
// never fail the change detection. saveLast writes .archfit-cache/last.json
// through a temporary file and a rename, so a name that the git scan read can
// be gone before git stats it; before the retry, git add --all exited 128
// ("unable to stat .archfit-cache/last-*.json") and the check failed.
func TestWorkingTreeIDWhileTheCacheIsWritten(t *testing.T) {
	t.Parallel()
	requireGit(t)
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	writeIn(t, dir, "a.go", "package a\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "base")
	store, err := gitpkg.NewObjectStore(toolrun.New())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	cache := filepath.Join(dir, ".archfit-cache")
	if err := os.MkdirAll(cache, 0o750); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		last := filepath.Join(cache, "last.json")
		for {
			select {
			case <-stop:
				return
			default:
			}
			tmp, err := os.CreateTemp(cache, "last-*.json")
			if err != nil {
				continue
			}
			_, _ = tmp.Write([]byte("{}\n"))
			_ = tmp.Close()
			_ = os.Rename(tmp.Name(), last)
			_ = os.Remove(last)
		}
	}()

	ctx := context.Background()
	var first string
	for run := 0; run < 100; run++ {
		id, err := gitpkg.WorkingTreeID(ctx, dir, store)
		if err != nil {
			close(stop)
			<-done
			t.Fatalf("run %d: %v", run, err)
		}
		if first == "" {
			first = id
		}
		// The cache is archfit's own write: churn in it changes nothing, so
		// every run must give the same tree id.
		if id != first {
			close(stop)
			<-done
			t.Fatalf("run %d: tree = %s, want %s", run, id, first)
		}
	}
	close(stop)
	<-done
}
