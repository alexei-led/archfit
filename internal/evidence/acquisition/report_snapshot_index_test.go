package acquisition

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexei-led/archfit/v4/internal/toolrun"
)

func TestReportSnapshotDoesNotRefreshExistingIndex(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := toolrun.New()
	run := func(args ...string) {
		t.Helper()
		out, err := runner.Run(context.Background(), toolrun.ToolCmd{Name: gitTool, Args: args, WorkDir: root})
		if err != nil || out.ExitCode != 0 {
			t.Fatalf("git %v: %v %s", args, err, out.Stderr)
		}
	}
	run("init", "-q")
	policy := filepath.Join(root, ".archfit.yaml")
	source := filepath.Join(root, "main.go")
	for path, content := range map[string]string{policy: "policy", source: "package main\n"} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	run("add", ".")
	run("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-qm", "initial")
	future := time.Now().Add(5 * time.Second)
	if err := os.Chtimes(source, future, future); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(root, ".git", "index")
	before, err := os.ReadFile(index) //nolint:gosec // index belongs to the isolated test repository
	if err != nil {
		t.Fatal(err)
	}
	snap := (ReportSnapshotSource{Root: root, PolicyPath: policy, Runner: runner}).Capture(context.Background())
	after, err := os.ReadFile(index) //nolint:gosec // index belongs to the isolated test repository
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("snapshot refreshed existing Git index")
	}
	if snap.Digest == nil || snap.Cleanliness != "clean" {
		t.Fatalf("capture: %+v", snap)
	}
}
