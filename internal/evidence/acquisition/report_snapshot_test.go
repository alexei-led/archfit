package acquisition

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v4/internal/toolrun"
)

func TestReportSnapshotRawBytesAndPolicyExclusion(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, path), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(".archfit.yaml", "policy A\n")
	write("main.go", "package main\r\n")
	src := ReportSnapshotSource{Root: root, PolicyPath: filepath.Join(root, ".archfit.yaml"), Exclude: []string{".archfit.yaml"}}
	a := src.Capture(context.Background())
	if a.Digest == nil || a.PolicyDigest == nil || a.Files["main.go"] == "" {
		t.Fatalf("missing captured inputs: %+v", a)
	}
	write("main.go", "package main\n")
	b := src.Capture(context.Background())
	if b.Digest == nil || *a.Digest == *b.Digest || *a.PolicyDigest != *b.PolicyDigest {
		t.Fatalf("raw edit hidden: a=%+v b=%+v", a, b)
	}
	write("added.go", "package main\n")
	c := src.Capture(context.Background())
	if c.Digest == nil || *c.Digest == *b.Digest {
		t.Fatal("addition not detected")
	}
	if err := os.Remove(filepath.Join(root, "main.go")); err != nil {
		t.Fatal(err)
	}
	d := src.Capture(context.Background())
	if d.Digest == nil || *d.Digest == *c.Digest {
		t.Fatal("deletion not detected")
	}
	write(".archfit.yaml", "policy B\n")
	e := src.Capture(context.Background())
	if e.PolicyDigest == nil || *e.PolicyDigest == *d.PolicyDigest {
		t.Fatal("excluded policy change not detected")
	}
}

func TestReportSnapshotEscapingSymlinkAndCapStayUnknown(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		setup func(*testing.T, string)
	}{
		{"escaping symlink", func(t *testing.T, root string) {
			t.Helper()
			outside := t.TempDir()
			if err := os.Symlink(outside, filepath.Join(root, "outside")); err != nil {
				t.Fatal(err)
			}
		}},
		{"byte cap", func(t *testing.T, root string) {
			t.Helper()
			f, err := os.Create(filepath.Join(root, "large.go")) //nolint:gosec // isolated test fixture
			if err != nil {
				t.Fatal(err)
			}
			if err := f.Truncate(reportSnapshotBytes + 1); err != nil {
				t.Fatal(err)
			}
			_ = f.Close()
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			policy := filepath.Join(root, ".archfit.yaml")
			if err := os.WriteFile(policy, []byte("policy"), 0600); err != nil {
				t.Fatal(err)
			}
			tc.setup(t, root)
			snap := (ReportSnapshotSource{Root: root, PolicyPath: policy}).Capture(context.Background())
			if snap.Digest != nil || snap.PolicyDigest == nil || len(snap.Unknowns) == 0 {
				t.Fatalf("partial snapshot claimed complete: %+v", snap)
			}
		})
	}
}

func TestReportSnapshotDoesNotWriteGitObjectsOrIndex(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := toolrun.New()
	run := func(args ...string) {
		t.Helper()
		r, e := runner.Run(context.Background(), toolrun.ToolCmd{Name: gitTool, Args: args, WorkDir: root})
		if e != nil || r.ExitCode != 0 {
			t.Fatalf("git %v: %v %s", args, e, r.Stderr)
		}
	}
	run("init", "-q")
	policy := filepath.Join(root, ".archfit.yaml")
	if err := os.WriteFile(policy, []byte("policy"), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir(filepath.Join(root, ".git", "objects"))
	if err != nil {
		t.Fatal(err)
	}
	snap := (ReportSnapshotSource{Root: root, PolicyPath: policy, Runner: runner}).Capture(context.Background())
	after, err := os.ReadDir(filepath.Join(root, ".git", "objects"))
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatal("object directory changed")
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "index")); !os.IsNotExist(err) {
		t.Fatalf("index created or inaccessible: %v", err)
	}
	if snap.HeadCommit != nil || snap.Cleanliness != "dirty" {
		t.Fatalf("unborn git evidence: %+v", snap)
	}
}

func TestReportSnapshotMissingPolicyNeverClaimsComplete(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	snap := (ReportSnapshotSource{Root: root, PolicyPath: filepath.Join(root, ".archfit.yaml")}).Capture(context.Background())
	if snap.Digest != nil || snap.PolicyDigest != nil {
		t.Fatalf("missing policy accepted: %+v", snap)
	}
	if len(snap.Unknowns) == 0 || !strings.Contains(snap.Unknowns[len(snap.Unknowns)-1].Fact, "git") {
		t.Fatalf("missing git disclosure: %+v", snap.Unknowns)
	}
}
