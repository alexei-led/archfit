package git

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// TestSnapshotIndexLeavesTheIndexAloneAndSignsAsTheCommitter pins the
// snapshot's contract with the user's repository: write-tree reads a private
// copy of the index git handed the hook (write-tree writes its cache-tree back
// into the file it reads), the commit is unsigned and parented on HEAD, and it
// carries the identity git will record on the real commit at a fixed date.
func TestSnapshotIndexLeavesTheIndexAloneAndSignsAsTheCommitter(t *testing.T) {
	t.Parallel()
	const (
		tree   = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
		parent = "1111111111111111111111111111111111111111"
		commit = "2222222222222222222222222222222222222222"
	)
	index := filepath.Join(t.TempDir(), "next-index")
	if err := os.WriteFile(index, []byte("staged"), 0o600); err != nil {
		t.Fatal(err)
	}
	var writeTreeIndex string
	var commitCmd toolrun.ToolCmd
	runner := &toolrun.RunnerMock{RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
		switch cmd.Args[0] {
		case "write-tree":
			writeTreeIndex = envValue(cmd.Env, "GIT_INDEX_FILE")
			copied, err := os.ReadFile(writeTreeIndex) //nolint:gosec // test temp file
			if err != nil || string(copied) != "staged" {
				t.Errorf("write-tree read %q (%v), want a copy of the staged index", copied, err)
			}
			return toolrun.Output{Stdout: []byte(tree + "\n")}, nil
		case "rev-parse":
			return toolrun.Output{Stdout: []byte(parent + "\n")}, nil
		case "var":
			return toolrun.Output{Stdout: []byte("Jane Doe <jane@example.com> 1700000000 +0100\n")}, nil
		case "commit-tree":
			commitCmd = cmd
			return toolrun.Output{Stdout: []byte(commit + "\n")}, nil
		}
		t.Fatalf("unexpected git %v", cmd.Args)
		return toolrun.Output{}, nil
	}}

	got, err := SnapshotIndex(context.Background(), runner, t.TempDir(), index)
	if err != nil || got != commit {
		t.Fatalf("SnapshotIndex = %q, %v; want %q", got, err, commit)
	}
	if writeTreeIndex == "" || writeTreeIndex == index {
		t.Errorf("write-tree GIT_INDEX_FILE = %q, want a private copy of %q", writeTreeIndex, index)
	}
	if _, err := os.Stat(writeTreeIndex); !os.IsNotExist(err) {
		t.Errorf("the private index copy %q was not removed (%v)", writeTreeIndex, err)
	}
	if want := []string{"commit-tree", "--no-gpg-sign", tree, "-m", "archfit staged snapshot", "-p", parent}; !slices.Equal(commitCmd.Args, want) {
		t.Errorf("commit-tree args = %v, want %v", commitCmd.Args, want)
	}
	for key, want := range map[string]string{
		"GIT_AUTHOR_NAME": "Jane Doe", "GIT_AUTHOR_EMAIL": "jane@example.com", "GIT_AUTHOR_DATE": snapshotDate,
		"GIT_COMMITTER_NAME": "Jane Doe", "GIT_COMMITTER_EMAIL": "jane@example.com", "GIT_COMMITTER_DATE": snapshotDate,
	} {
		if got := envValue(commitCmd.Env, key); got != want {
			t.Errorf("commit-tree %s = %q, want %q", key, got, want)
		}
	}
}

// envValue returns the last value of key in env, as os/exec resolves duplicates.
func envValue(env []string, key string) string {
	value := ""
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			value = v
		}
	}
	return value
}
