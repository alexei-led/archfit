package git_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	gitpkg "github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestWorkingTreeCacheIgnoreSources(t *testing.T) {
	const trackedFile = "a.go"
	requireGit(t)
	for _, tc := range []struct {
		name                                 string
		repository, global, tracked, negated bool
	}{
		{name: "not ignored"},
		{name: "repository ignore", repository: true},
		{name: "global ignore", global: true},
		{name: "both ignore files", repository: true, global: true},
		{name: "already tracked", tracked: true},
		{name: "global ignore re-included by repository", repository: true, global: true, negated: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			global := filepath.Join(t.TempDir(), "config")
			t.Setenv("GIT_CONFIG_GLOBAL", global)
			t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			if tc.global {
				ignore := filepath.Join(t.TempDir(), "ignore")
				if err := os.WriteFile(ignore, []byte("/.archfit-cache/\n*.local\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(global, []byte("[core]\nexcludesFile = "+strconv.Quote(ignore)+"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			gitIn(t, dir, "init", "-q")
			writeIn(t, dir, trackedFile, "package a\n")
			if tc.repository {
				ignore := "/.archfit-cache/\n"
				if tc.negated {
					ignore = "!/.archfit-cache/\n!/.archfit-cache/**\n"
				}
				writeIn(t, dir, ".gitignore", ignore)
			}
			if tc.tracked {
				writeIn(t, dir, ".archfit-cache/state", "tracked cache\n")
			}
			gitIn(t, dir, "add", "-A")
			gitIn(t, dir, "commit", "-qm", "base")
			store, err := gitpkg.NewObjectStore(toolrun.New())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = store.Close() }()
			ctx := context.Background()
			clean, err := gitpkg.WorkingTreeID(ctx, dir, store)
			if err != nil {
				t.Fatal(err)
			}
			writeIn(t, dir, ".archfit-cache/state", "new cache\n")
			if tc.global {
				writeIn(t, dir, "secret.local", "globally ignored\n")
			}
			withCache, err := gitpkg.WorkingTreeID(ctx, dir, store)
			if err != nil || withCache != clean {
				t.Fatalf("with cache = %s, %v; want %s", withCache, err, clean)
			}
			writeIn(t, dir, "new.go", "package new\n")
			tree, err := gitpkg.WorkingTreeID(ctx, dir, store)
			if err != nil || tree == clean {
				t.Fatalf("untracked source = %s, %v; want a new tree", tree, err)
			}
			files, err := gitpkg.TreeFiles(ctx, dir, tree, store)
			want := []string{trackedFile, "new.go"}
			if tc.repository {
				want = append([]string{".gitignore"}, want...)
			}
			if err != nil || !slices.Equal(files, want) {
				t.Fatalf("files = %v, %v; want %v", files, err, want)
			}
			added, err := gitpkg.AddedLines(ctx, dir, gitOut(t, dir, "rev-parse", "HEAD"), store)
			if err != nil || len(added) != 1 || !slices.Equal(added["new.go"], []int{1}) {
				t.Fatalf("added = %v, %v; want only new.go line 1", added, err)
			}
		})
	}
}
