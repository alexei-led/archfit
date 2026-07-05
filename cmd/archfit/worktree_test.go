package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/toolrun"
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
	diffBaseRef = "HEAD~1"
	flagBase    = "--base"
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
	code := Run([]string{cmdAnalyze, flagBase, diffBaseRef, "-c", cfgPath}, &buf)
	out := buf.String()

	if code != 0 {
		t.Fatalf("diff HEAD~1: exit=%d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "ARCHFIT RESULT") {
		t.Errorf("--base text should render the decision report: %s", out)
	}
	if !strings.Contains(out, "CHANGE VS BASE") {
		t.Errorf("--base text missing the delta section: %s", out)
	}
}

// TestDiffCmd_JSONFormat verifies --base --json emits the SAME diagnostic schema
// as a normal --json run — a consistent machine contract, not a separate delta
// schema. (Regression guard for the old asymmetric diffResult output.)
func TestDiffCmd_JSONFormat(t *testing.T) {
	t.Parallel()

	_, cfgPath := makeDiffFixtureRepo(t)

	var buf bytes.Buffer
	code := Run([]string{cmdAnalyze, flagBase, diffBaseRef, "-c", cfgPath, "--format=json"}, &buf)
	if code != 0 {
		t.Fatalf("--base --json: exit=%d\noutput:\n%s", code, buf.String())
	}

	var diag struct {
		SchemaVersion string `json:"schema_version"`
		Verdict       string `json:"verdict"`
	}
	if err := json.Unmarshal(buf.Bytes(), &diag); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if diag.SchemaVersion == "" || diag.Verdict == "" {
		t.Errorf("--base --json must be the standard diagnostic schema (schema_version + verdict), got: %s", buf.String())
	}
}

// TestDiffCmd_WorktreeCleanup verifies that the temporary worktree is removed
// after a successful diff run (no stale entry in `git worktree list`).
func TestDiffCmd_WorktreeCleanup(t *testing.T) {
	t.Parallel()

	repoDir, cfgPath := makeDiffFixtureRepo(t)

	var buf bytes.Buffer
	code := Run([]string{cmdAnalyze, flagBase, diffBaseRef, "-c", cfgPath}, &buf)
	if code != 0 {
		t.Fatalf("diff HEAD~1: exit=%d\noutput:\n%s", code, buf.String())
	}

	// Check that only the main worktree remains — neither a random temp
	// checkout (archfit-base-*) nor the deterministic per-SHA one
	// (.archfit-cache/worktrees/<sha>) may stay registered.
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git worktree list: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "archfit-base-") || strings.Contains(string(out), "worktrees") {
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
	code := RunWithStderr([]string{cmdAnalyze, flagBase, diffBaseRef, "-c", cfgPath}, &buf, &buf)
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
	code := RunWithStderr([]string{cmdAnalyze, flagBase, "refs/does-not-exist-xyz", "-c", cfgPath}, &buf, &buf)
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
	code := Run([]string{cmdAnalyze, flagBase, diffBaseRef, "-c", cfgPath, "--format=markdown"}, &buf)
	out := buf.String()
	if code != 0 {
		t.Fatalf("diff --format=markdown: exit=%d\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "# archfit — decision") {
		t.Errorf("--base --markdown should lead with the decision summary: %s", out)
	}
	if !strings.Contains(out, "Change vs base") {
		t.Errorf("--base --markdown should include the delta section: %s", out)
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
	code := Run([]string{cmdAnalyze, flagBase, "HEAD~1", "-c", cfgPath, flagRoot, subDir}, &buf)
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
	code := Run([]string{cmdAnalyze, flagBase, "HEAD~1", "-c", cfgPath}, &buf)
	if code != 0 {
		t.Fatalf("diff config-in-subdir (--root omitted): exit=%d\noutput:\n%s", code, buf.String())
	}

	// Verify the output is the decision report with a delta section (not a crash/skip).
	out := buf.String()
	if !strings.Contains(out, "CHANGE VS BASE") {
		t.Errorf("--base output missing the delta section: %s", out)
	}
}

func TestBaseWorktreeParent_LocksDeterministicDir(t *testing.T) {
	sha := strings.Repeat("a", 40)
	runner := &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if cmd.Name == gitBinary && len(cmd.Args) >= 3 && cmd.Args[0] == "rev-parse" {
				return toolrun.Output{Stdout: []byte(sha + "\n")}, nil
			}
			return toolrun.Output{}, nil
		},
	}
	deps := &appDeps{Runner: runner}
	dir := t.TempDir()

	first, releaseFirst, err := baseWorktreeParent(context.Background(), deps, dir, diffBaseRef, dir)
	if err != nil {
		t.Fatal(err)
	}
	firstReleased := false
	t.Cleanup(func() {
		if !firstReleased {
			releaseFirst()
		}
	})
	if want := filepath.Join(dir, ".archfit-cache", "worktrees", sha); first != want {
		t.Fatalf("first worktree parent = %q, want %q", first, want)
	}

	done := make(chan struct{})
	var second string
	var releaseSecond func()
	var secondErr error
	go func() {
		second, releaseSecond, secondErr = baseWorktreeParent(context.Background(), deps, dir, diffBaseRef, dir)
		close(done)
	}()

	select {
	case <-done:
		t.Fatalf("second baseWorktreeParent returned before first lock was released: dir=%q err=%v", second, secondErr)
	case <-time.After(3 * baseWorktreeLockPoll):
	}

	releaseFirst()
	firstReleased = true
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("second baseWorktreeParent did not acquire lock after first release")
	}
	if secondErr != nil {
		t.Fatal(secondErr)
	}
	defer releaseSecond()
	if second != first {
		t.Fatalf("second worktree parent = %q, want locked deterministic dir %q", second, first)
	}
}

// cargoFakeRunner delegates every command to the real runner except cargo:
// --version returns a pinned version and `cargo metadata` returns a minimal
// synthesized workspace rooted at the command's WorkDir (cargo embeds absolute
// paths, which is exactly what the per-checkout fact-cache key must absorb).
// metadata WorkDirs are recorded so tests can count subprocess invocations.
type cargoFakeRunner struct {
	real toolrun.Runner

	mu            sync.Mutex
	metadataCalls []string
}

func (r *cargoFakeRunner) Detect(ctx context.Context, tool string) (toolrun.ToolInfo, bool) {
	if tool == "cargo" {
		return toolrun.ToolInfo{}, true
	}
	return r.real.Detect(ctx, tool)
}

func (r *cargoFakeRunner) Run(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
	if cmd.Name == "cargo" && len(cmd.Args) > 0 {
		switch cmd.Args[0] {
		case flagVersion:
			return toolrun.Output{Stdout: []byte("cargo 1.75.0\n")}, nil
		case "metadata":
			r.mu.Lock()
			r.metadataCalls = append(r.metadataCalls, cmd.WorkDir)
			r.mu.Unlock()
			return toolrun.Output{Stdout: fakeCargoMetadata(cmd.WorkDir)}, nil
		}
	}
	return r.real.Run(ctx, cmd)
}

func (r *cargoFakeRunner) Stream(ctx context.Context, cmd toolrun.ToolCmd, consume func(io.Reader) error) (toolrun.Output, error) {
	return r.real.Stream(ctx, cmd, consume)
}

func (r *cargoFakeRunner) metadataWorkDirs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.metadataCalls)
}

// fakeCargoMetadata synthesizes a one-crate `cargo metadata --no-deps` result
// rooted at workDir, mirroring cargo's absolute manifest_path/workspace_root.
func fakeCargoMetadata(workDir string) []byte {
	id := "path+file://" + workDir + "#demo@0.1.0"
	data, err := json.Marshal(map[string]any{
		"packages": []map[string]any{{
			"id":            id,
			"name":          "demo",
			"manifest_path": filepath.Join(workDir, "Cargo.toml"),
			"source":        nil,
			"dependencies":  []any{},
			"targets":       []map[string]any{{"name": "demo", "kind": []string{"bin"}}},
		}},
		"workspace_members": []string{id},
		"workspace_root":    workDir,
	})
	if err != nil {
		panic(err)
	}
	return data
}

// TestDiffCmd_BaseSideFactCacheReuse pins Wave 6 Task 4: the base worktree
// path is a deterministic function of the resolved base commit SHA
// (.archfit-cache/worktrees/<sha>), so a second `--base <same-ref>` run
// serves both sides' cargo metadata from the fact cache — zero extractor
// subprocess calls — with byte-identical output. Moving the ref (new base
// SHA) re-runs the base side: immutability is keyed to the commit, never
// assumed across refs.
func TestDiffCmd_BaseSideFactCacheReuse(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	files := map[string]string{
		"Cargo.toml":      "[package]\nname = \"demo\"\nversion = \"0.1.0\"\nedition = \"2021\"\n",
		"src/main.rs":     "fn main() {}\n",
		defaultConfigPath: minimalValidYAML,
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
	mainRS := filepath.Join(dir, "src", "main.rs")
	if err := os.WriteFile(mainRS, []byte("fn main() { let _x = 1; }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, dir, "edit main.rs")

	cfgPath := filepath.Join(dir, defaultConfigPath)
	runBase := func() (string, []string) {
		t.Helper()
		fake := &cargoFakeRunner{real: toolrun.New()}
		var stdout, stderr bytes.Buffer
		cmd := AnalyzeCmd{Config: cfgPath, Base: diffBaseRef, Format: []string{formatJSON}}
		err := cmd.Run(&appDeps{Runner: fake, Stdout: &stdout, Stderr: &stderr})
		var ee *exitError
		if err != nil && (!errors.As(err, &ee) || ee.code > 1) {
			t.Fatalf("analyze --base: %v\nstderr:\n%s", err, stderr.String())
		}
		return stdout.String(), fake.metadataWorkDirs()
	}

	out1, calls1 := runBase()
	if len(calls1) != 2 {
		t.Fatalf("cold --base run: cargo metadata calls = %d (%v), want 2 (head + base)", len(calls1), calls1)
	}

	out2, calls2 := runBase()
	if len(calls2) != 0 {
		t.Errorf("warm --base run: cargo metadata calls = %v, want none (both sides cached)", calls2)
	}
	if out1 != out2 {
		t.Errorf("warm --base output differs from cold run:\n%s", firstDiffLine(out1, out2))
	}

	// Move the ref: HEAD~1 now names a different commit, so the base side must
	// re-run (fresh SHA ⇒ fresh worktree path ⇒ cache miss). The head side's
	// cargo metadata key is manifests-only, so the .rs edit leaves it cached.
	if err := os.WriteFile(mainRS, []byte("fn main() { let _x = 2; }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, dir, "edit main.rs again")
	_, calls3 := runBase()
	if len(calls3) != 1 || !strings.Contains(calls3[0], "worktrees") {
		t.Errorf("--base after ref moved: cargo metadata calls = %v, want exactly one base-side call under .archfit-cache/worktrees", calls3)
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

	// Errors go to stderr; capture both streams in one buffer (this is an exit-3
	// path with no stdout output to corrupt).
	var buf bytes.Buffer
	code := RunWithStderr([]string{cmdAnalyze, flagBase, diffBaseRef, "-c", cfgPath, flagRoot, outsideDir}, &buf, &buf)
	if code != 3 {
		t.Fatalf("--root above gitRoot: exit=%d, want 3\noutput:\n%s", code, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "not under git root") && !strings.Contains(out, "git repository") {
		t.Errorf("error message should mention git root or repository; got: %s", out)
	}
}

// A leading-dash --base ref must be rejected before reaching git, where it
// would parse as a flag: `git worktree add --detach <dir> --force` silently
// checks out HEAD and the delta compares HEAD to itself.
func TestDiffCmd_DashRef(t *testing.T) {
	t.Parallel()

	_, cfgPath := makeDiffFixtureRepo(t)

	var buf bytes.Buffer
	code := RunWithStderr([]string{cmdAnalyze, "--base=--force", "-c", cfgPath}, &buf, &buf)
	if code != 3 {
		t.Fatalf("dash ref: exit=%d, want 3\noutput:\n%s", code, buf.String())
	}
	if !strings.Contains(buf.String(), "invalid --base ref") {
		t.Errorf("error should name the invalid ref; got: %s", buf.String())
	}
}
