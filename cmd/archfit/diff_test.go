package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitCommitAll stages all files and creates a commit in dir.
// It sets a minimal identity so git does not reject the commit.
func gitCommitAll(t *testing.T, dir, msg string) {
	t.Helper()
	env := append(scrubGitFixtureEnv(os.Environ()),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	for _, args := range [][]string{
		{"add", "-A"},
		{"commit", "--allow-empty", "-m", msg},
	} {
		cmd := exec.Command("git", args...) //nolint:gosec // test fixture — fixed "git" binary, args are controlled test inputs
		cmd.Dir = dir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git %v failed: %v\n%s", args, err, out)
		}
	}
}

const (
	cmdDiff     = "diff"
	diffBaseRef = "HEAD~1"
)

// makeDiffFixtureRepo creates a minimal git repo with two commits and returns
// (repoDir, configPath). The repo contains a plain Go module so both score
// runs terminate quickly without any real analyzers.
func makeDiffFixtureRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()

	cfgContent := "version: 1\n"
	files := map[string]string{
		"go.mod":          "module example.com/difftest\n\ngo 1.21\n",
		"main.go":         goMainSrc,
		defaultConfigPath: cfgContent,
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	gitInitFixtureRepo(t, dir)
	gitCommitAll(t, dir, "initial commit")

	// Second commit: add another file so there is a real HEAD~1 → HEAD diff.
	secondFile := filepath.Join(dir, "util.go")
	if err := os.WriteFile(secondFile, []byte("package main\n\nfunc helper() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, dir, "add helper")

	return dir, filepath.Join(dir, ".archfit.yaml")
}

// TestDiffCmd_EmitsDeltaTable verifies that diff between HEAD~1 and HEAD
// produces a delta table and exits 0.
func TestDiffCmd_EmitsDeltaTable(t *testing.T) {
	t.Parallel()

	_, cfgPath := makeDiffFixtureRepo(t)

	var buf bytes.Buffer
	code := Run([]string{cmdDiff, diffBaseRef, "-c", cfgPath}, &buf)
	out := buf.String()

	if code != 0 {
		t.Fatalf("diff HEAD~1: exit=%d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "HEAD") {
		t.Errorf("diff output missing 'HEAD': %s", out)
	}
	if !strings.Contains(out, "Overall") {
		t.Errorf("diff output missing 'Overall': %s", out)
	}
}

// TestDiffCmd_JSONFormat verifies that --format json produces valid JSON with
// the expected top-level fields.
func TestDiffCmd_JSONFormat(t *testing.T) {
	t.Parallel()

	_, cfgPath := makeDiffFixtureRepo(t)

	var buf bytes.Buffer
	code := Run([]string{cmdDiff, diffBaseRef, "-c", cfgPath, "--format=json"}, &buf)
	if code != 0 {
		t.Fatalf("diff --format=json: exit=%d\noutput:\n%s", code, buf.String())
	}

	var res diffResult
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if res.BaseRef != diffBaseRef {
		t.Errorf("base_ref = %q, want HEAD~1", res.BaseRef)
	}
}

// TestDiffCmd_WorktreeCleanup verifies that the temporary worktree is removed
// after a successful diff run (no stale entry in `git worktree list`).
func TestDiffCmd_WorktreeCleanup(t *testing.T) {
	t.Parallel()

	repoDir, cfgPath := makeDiffFixtureRepo(t)

	var buf bytes.Buffer
	code := Run([]string{cmdDiff, diffBaseRef, "-c", cfgPath}, &buf)
	if code != 0 {
		t.Fatalf("diff HEAD~1: exit=%d\noutput:\n%s", code, buf.String())
	}

	// Check that only the main worktree remains (no stale archfit-diff-* entry).
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git worktree list: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "archfit-diff-") {
		t.Errorf("stale worktree entry found after diff:\n%s", out)
	}
}

// TestDiffCmd_NonGitRoot verifies that running diff in a plain directory
// (not a git repository) exits 3 with a clear error message.
func TestDiffCmd_NonGitRoot(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".archfit.yaml")
	if err := os.WriteFile(cfgPath, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	code := Run([]string{cmdDiff, diffBaseRef, "-c", cfgPath}, &buf)
	if code != 3 {
		t.Fatalf("non-git dir: exit=%d, want 3\noutput:\n%s", code, buf.String())
	}
	if !strings.Contains(buf.String(), "git repository") {
		t.Errorf("error message should mention 'git repository', got: %s", buf.String())
	}
}

// TestDiffCmd_BadRef verifies that an unknown git ref exits 3 with a clear
// error message.
func TestDiffCmd_BadRef(t *testing.T) {
	t.Parallel()

	_, cfgPath := makeDiffFixtureRepo(t)

	var buf bytes.Buffer
	code := Run([]string{cmdDiff, "refs/does-not-exist-xyz", "-c", cfgPath}, &buf)
	if code != 3 {
		t.Fatalf("bad ref: exit=%d, want 3\noutput:\n%s", code, buf.String())
	}
	if !strings.Contains(buf.String(), "worktree") {
		t.Errorf("error message should mention 'worktree', got: %s", buf.String())
	}
}

// TestDiffCmd_MarkdownFormat verifies T1: --format=markdown produces a
// markdown table with expected headings.
func TestDiffCmd_MarkdownFormat(t *testing.T) {
	t.Parallel()

	_, cfgPath := makeDiffFixtureRepo(t)

	var buf bytes.Buffer
	code := Run([]string{cmdDiff, diffBaseRef, "-c", cfgPath, "--format=markdown"}, &buf)
	out := buf.String()
	if code != 0 {
		t.Fatalf("diff --format=markdown: exit=%d\noutput:\n%s", code, out)
	}
	// A markdown table has pipe-delimited columns.
	if !strings.Contains(out, "|") {
		t.Errorf("markdown output missing pipe separator: %s", out)
	}
	if !strings.Contains(out, "Overall") {
		t.Errorf("markdown output missing 'Overall' row: %s", out)
	}
}

// TestDiffCmd_SubdirRoot verifies T2a: when --root points at a subdirectory
// inside the git root, diff exits 0 (the subdir exists in both HEAD~1 and HEAD).
func TestDiffCmd_SubdirRoot(t *testing.T) {
	t.Parallel()

	repoDir, cfgPath := makeDiffFixtureRepo(t)

	// Create a subdirectory that exists in both commits.
	subDir := filepath.Join(repoDir, "pkg")
	if err := os.MkdirAll(subDir, 0o750); err != nil {
		t.Fatal(err)
	}
	subFile := filepath.Join(subDir, "sub.go")
	if err := os.WriteFile(subFile, []byte("package pkg\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, repoDir, "add pkg subdir")

	var buf bytes.Buffer
	code := Run([]string{cmdDiff, "HEAD~1", "-c", cfgPath, flagRoot, subDir}, &buf)
	if code != 0 {
		t.Fatalf("diff --root subdir: exit=%d\noutput:\n%s", code, buf.String())
	}
}

// TestDiffCmd_SubtreeAboveGitRoot verifies T2b (Q1 guard): when --root resolves
// to a path outside the git root, diff must exit 3 with an error message.
// TestSubtreeInWorktree_ParentEscape verifies the D4 regression: a directory
// whose name starts with ".." (e.g. "..fixtures") must NOT be rejected by the
// parent-escape guard, while a true parent path ("../sibling") must be.
func TestSubtreeInWorktree_ParentEscape(t *testing.T) {
	t.Parallel()

	gitRoot := "/repo"
	wtDir := "/wt"

	// Valid subdirectory whose name starts with "..": must succeed.
	dotdotName := filepath.Join(gitRoot, "..fixtures")
	got, err := subtreeInWorktree(gitRoot, dotdotName, wtDir)
	if err != nil {
		t.Errorf("..fixtures subdir: unexpected error: %v", err)
	}
	if want := filepath.Join(wtDir, "..fixtures"); got != want {
		t.Errorf("..fixtures subdir: got %q, want %q", got, want)
	}

	// True parent escape: must be rejected.
	parent := filepath.Join(gitRoot, "..", "sibling")
	if _, err := subtreeInWorktree(gitRoot, parent, wtDir); err == nil {
		t.Error("parent escape: expected error, got nil")
	}
}

// TestDiffCmd_ConfigInSubdir verifies that when the config lives in a
// subdirectory and --root is omitted, diff analyses the whole repo (GitRoot),
// not just the config's subdirectory.  Pre-fix, headScanRoot was set to
// configDir (a subtree), so the HEAD scorecard diverged from check/score.
// Post-fix, passing c.Root="" to runScoreSide lets resolveScanRoot fall
// through to gitRoot — byte-identical to check/score's behaviour.
func TestDiffCmd_ConfigInSubdir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Config lives in a "configs" subdirectory; source files are at repo root.
	cfgDir := filepath.Join(dir, "configs")
	if err := os.MkdirAll(cfgDir, 0o750); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(cfgDir, ".archfit.yaml")

	files := map[string]string{
		filepath.Join(dir, "go.mod"):  "module example.com/subdirtest\n\ngo 1.21\n",
		filepath.Join(dir, "main.go"): goMainSrc,
		cfgPath:                       "version: 1\n",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	gitInitFixtureRepo(t, dir)
	gitCommitAll(t, dir, "initial commit")

	// Second commit so HEAD~1 exists.
	util := filepath.Join(dir, "util.go")
	if err := os.WriteFile(util, []byte("package main\n\nfunc helper() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, dir, "add helper")

	// --root omitted: diff must succeed (not silently narrow to configs/).
	// Pre-fix this would call runScoreSide with root=cfgDir for both sides,
	// causing a mismatch with check/score on the same repo.
	var buf bytes.Buffer
	code := Run([]string{cmdDiff, "HEAD~1", "-c", cfgPath}, &buf)
	if code != 0 {
		t.Fatalf("diff config-in-subdir (--root omitted): exit=%d\noutput:\n%s", code, buf.String())
	}

	// Verify the output looks like a real scorecard delta (not a crash or skip).
	out := buf.String()
	if !strings.Contains(out, "Overall") {
		t.Errorf("diff output missing 'Overall' row: %s", out)
	}
}

// With the C1 fix gitRoot is resolved from headRoot; if headRoot is outside any
// git repo the error fires in git.RepoRoot ("not a git repository") rather than
// in subtreeInWorktree ("not under git root") — both are correct rejections.
func TestDiffCmd_SubtreeAboveGitRoot(t *testing.T) {
	t.Parallel()

	_, cfgPath := makeDiffFixtureRepo(t)

	// Use a path that is definitely outside the fixture repo (parent of t.TempDir).
	outsideDir := filepath.Dir(filepath.Dir(cfgPath))

	var buf bytes.Buffer
	code := Run([]string{cmdDiff, diffBaseRef, "-c", cfgPath, flagRoot, outsideDir}, &buf)
	if code != 3 {
		t.Fatalf("--root above gitRoot: exit=%d, want 3\noutput:\n%s", code, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "not under git root") && !strings.Contains(out, "git repository") {
		t.Errorf("error message should mention git root or repository; got: %s", out)
	}
}
