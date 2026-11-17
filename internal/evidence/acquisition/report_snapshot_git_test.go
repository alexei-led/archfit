package acquisition

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexei-led/archfit/v4/internal/toolrun"
)

const gitTool = "git"

func TestReportSnapshotSeesStagedUntrackedDeletedAndHeadChanges(t *testing.T) {
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
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q")
	write(".archfit.yaml", "policy")
	write("main.go", "package main\n")
	run("add", ".")
	run("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-qm", "initial")
	src := ReportSnapshotSource{Root: root, PolicyPath: filepath.Join(root, ".archfit.yaml"), Runner: runner}
	a := src.Capture(context.Background())
	if a.Digest == nil || a.HeadCommit == nil || a.Cleanliness != "clean" {
		t.Fatalf("initial capture: %+v", a)
	}
	write("main.go", "package main\n// staged\n")
	run("add", "main.go")
	b := src.Capture(context.Background())
	if b.Digest == nil || *a.Digest == *b.Digest || b.Cleanliness != "dirty" {
		t.Fatalf("staged change: %+v", b)
	}
	write("new.go", "package main\n")
	c := src.Capture(context.Background())
	if c.Digest == nil || *b.Digest == *c.Digest {
		t.Fatal("untracked file not captured")
	}
	if err := os.Remove(filepath.Join(root, "main.go")); err != nil {
		t.Fatal(err)
	}
	d := src.Capture(context.Background())
	if d.Digest == nil || *c.Digest == *d.Digest {
		t.Fatal("deleted file not captured")
	}
	run("add", "-A")
	run("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-qm", "next")
	e := src.Capture(context.Background())
	if e.HeadCommit == nil || *a.HeadCommit == *e.HeadCommit {
		t.Fatalf("HEAD change not captured: %+v", e)
	}
}
