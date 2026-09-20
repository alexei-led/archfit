package git_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	gitpkg "github.com/alexei-led/archfit/internal/history/git"
	"github.com/alexei-led/archfit/internal/toolrun"
)

func TestTouchCountsPreservesGitPathBytes(t *testing.T) {
	root := t.TempDir()
	runner := toolrun.New()
	ctx := context.Background()
	run := func(args ...string) {
		t.Helper()
		out, err := runner.Run(ctx, toolrun.ToolCmd{Name: "git", Args: args, WorkDir: root})
		if err != nil || out.ExitCode != 0 {
			t.Fatalf("git %v: %v %+v", args, err, out)
		}
	}
	run("init", "--initial-branch=main")
	files := []string{"café/core.py", " leading.py", "trailing.py ", "line\nbreak.py", "quote\".py", "back\\slash.py", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "commit"}
	for _, file := range files {
		full := filepath.Join(root, file)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("source\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	run("add", ".")
	run("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "add source")
	if err := os.Remove(filepath.Join(root, files[0])); err != nil {
		t.Fatal(err)
	}
	run("add", "-u")
	run("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "delete source")
	seen := make(map[string]int)
	moduleFor := func(file string) (string, bool) {
		seen[file]++
		for _, known := range files {
			if known == file {
				return "source", true
			}
		}
		return "", false
	}
	got := gitpkg.TouchCounts(ctx, root, "", moduleFor, runner)
	if got.Status != gitpkg.ModuleTouchStatusOK || got.CommitsScanned != 2 || got.TouchedByModule["source"] != 2 || got.FullHistory {
		t.Fatalf("history = %+v", got)
	}
	for _, file := range files {
		want := 1
		if file == files[0] {
			want = 2
		}
		if seen[file] != want {
			t.Errorf("path %q observed %d times, want %d; seen=%v", file, seen[file], want, seen)
		}
	}
}
