package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	hookCleanA     = "package a\n\nfunc UseSecret() string { return \"\" }\n"
	hookViolatingA = "package a\n\nimport \"example.com/test/pkg/b/impl\"\n\n" +
		"func UseSecret() string { return impl.Secret() }\n"
	hookImplFile = "pkg/b/impl/impl.go"
	hookModules  = `version: 2
modules:
  a:
    paths: ["pkg/a/**"]
  b:
    paths: ["pkg/b/**"]
`
	hookDeadSelectorCfg = hookModules + `rules:
  - id: no_ghost
    type: forbidden_dependency
    gate: fail
    from: pkg/ghost/**
    to: pkg/b/impl
`
	hookRuleCfg = hookModules + `rules:
  - id: no_b_impl
    type: forbidden_dependency
    gate: fail
    from: pkg/a/**
    to: pkg/b/impl
`
)

// hookRepo writes the coupled fixture with headA committed as pkg/a/a.go,
// then leaves worktreeA (when not empty) as an uncommitted change. Returns the
// repo root.
func hookRepo(t *testing.T, cfg, headA, worktreeA string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		markerGoMod:       goModStub,
		filePkgAA:         headA,
		hookImplFile:      implSource(),
		defaultConfigPath: cfg,
	}
	for name, content := range files {
		writeFixtureFile(t, dir, name, content)
	}
	gitInitFixtureRepo(t, dir)
	gitCommitFixture(t, dir)
	if worktreeA != "" {
		writeFixtureFile(t, dir, filePkgAA, worktreeA)
	}
	return dir
}

func writeFixtureFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// gitCommitFixture commits everything with a fixed identity and no signing,
// so the fixture does not depend on the user's git config.
func gitCommitFixture(t *testing.T, dir string) {
	t.Helper()
	env := append(scrubGitFixtureEnv(os.Environ()),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
	for _, args := range [][]string{
		{"add", "-A"},
		{"-c", "commit.gpgsign=false", "commit", "-q", "-m", "fixture"},
	} {
		cmd := exec.Command("git", args...) //nolint:gosec // fixed git binary, controlled test args
		cmd.Dir = dir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func runHook(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runWithIO(append([]string{"hook"}, args...), strings.NewReader(stdin), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func stopEvent(event, cwd string, active bool) string {
	raw, _ := json.Marshal(map[string]any{"hook_event_name": event, "cwd": cwd, "stop_hook_active": active, "session_id": "s"})
	return string(raw)
}

// TestHookClaude pins one stdin fixture per row of the Stop hook table: the
// exit code and the output channel. Every event cwd differs from the test
// process's own cwd, so the hook resolves --config against the event.
func TestHookClaude(t *testing.T) {
	t.Parallel()
	introduced := func(t *testing.T) string { return hookRepo(t, hookRuleCfg, hookCleanA, hookViolatingA) }
	for _, tc := range []struct {
		name       string
		repo       func(*testing.T) string
		event      string
		active     bool
		rawStdin   string
		wantCode   int
		wantStdout string // substring of the systemMessage; "" means empty stdout
		wantStderr string // substring of stderr; "" means empty stderr
	}{
		{name: "repair on the first stop blocks", repo: introduced, event: hookEventStop, wantCode: 2,
			wantStderr: "next_action repair"},
		{name: "repair with stop_hook_active blocks only once", repo: introduced, event: hookEventSubagentStop, active: true,
			wantStdout: "still reports blockers"},
		{name: "a blocker outside the change is reported", event: hookEventStop,
			repo: func(t *testing.T) string {
				return hookRepo(t, hookRuleCfg, hookViolatingA, hookViolatingA+"\n// unrelated edit\n")
			}, wantStdout: "next_action report_blocked"},
		{name: "nothing to do is silent", event: hookEventStop,
			repo: func(t *testing.T) string { return hookRepo(t, hookModules, hookCleanA, hookCleanA+"\n// edit\n") }},
		{name: "a clean tree skips the run", event: hookEventStop,
			repo: func(t *testing.T) string { return hookRepo(t, hookRuleCfg, hookViolatingA, "") }},
		{name: "an archfit error fails open", event: hookEventStop,
			repo: func(t *testing.T) string {
				dir := hookRepo(t, hookRuleCfg, hookCleanA, hookViolatingA)
				if err := os.Remove(filepath.Join(dir, defaultConfigPath)); err != nil {
					t.Fatal(err)
				}
				return dir
			}, wantStdout: "failed open"},
		{name: "archfit's own cache is not a change", event: hookEventStop,
			repo: func(t *testing.T) string {
				dir := hookRepo(t, hookRuleCfg, hookViolatingA, "")
				writeFixtureFile(t, dir, ".archfit-cache/facts/x.json", "{}")
				return dir
			}},
		{name: "a pre-existing dead selector is reported, not blocked", event: hookEventStop,
			repo: func(t *testing.T) string {
				return hookRepo(t, hookDeadSelectorCfg, hookCleanA, hookCleanA+"\n// edit\n")
			}, wantStdout: "next_action ask_owner"},
		{name: "another event is ignored", repo: introduced, event: "PreToolUse"},
		{name: "malformed stdin", repo: introduced, rawStdin: "{not json", wantCode: 1, wantStderr: "malformed hook event"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := tc.repo(t)
			stdin := tc.rawStdin
			if stdin == "" {
				stdin = stopEvent(tc.event, dir, tc.active)
			}
			code, stdout, stderr := runHook(t, stdin, "claude")
			if code != tc.wantCode {
				t.Fatalf("exit = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, tc.wantCode, stdout, stderr)
			}
			if tc.wantStdout == "" && stdout != "" {
				t.Errorf("stdout = %q, want empty", stdout)
			}
			if tc.wantStdout != "" {
				var msg struct {
					SystemMessage string `json:"systemMessage"`
				}
				if err := json.Unmarshal([]byte(stdout), &msg); err != nil || !strings.Contains(msg.SystemMessage, tc.wantStdout) {
					t.Errorf("stdout = %q (%v), want a systemMessage containing %q", stdout, err, tc.wantStdout)
				}
			}
			if tc.wantStderr == "" && stderr != "" {
				t.Errorf("stderr = %q, want empty", stderr)
			}
			if tc.wantStderr != "" && !strings.Contains(stderr, tc.wantStderr) {
				t.Errorf("stderr = %q, want %q", stderr, tc.wantStderr)
			}
		})
	}
}

// TestHookClaudeStderrCarriesTheRepair pins what the agent reads on a block:
// the finding ID, the file:line, the goal, and the validation command.
func TestHookClaudeStderrCarriesTheRepair(t *testing.T) {
	t.Parallel()
	dir := hookRepo(t, hookRuleCfg, hookCleanA, hookViolatingA)
	code, _, stderr := runHook(t, stopEvent(hookEventStop, dir, false), "claude")
	if code != 2 {
		t.Fatalf("exit = %d, want 2\n%s", code, stderr)
	}
	for _, want := range []string{"no_b_impl", filePkgAA + ":3", "goal: ", "validate: archfit check", "--format agent"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr is missing %q:\n%s", want, stderr)
		}
	}
}

// TestHookGit pins that the pre-commit hook judges the index — what the commit
// will contain — and never the files on disk: an unstaged or untracked edit
// neither blocks nor excuses a commit.
func TestHookGit(t *testing.T) {
	t.Parallel()
	const (
		untrackedFile = "pkg/a/extra.go"
		nestedConfig  = "policy/archfit.yaml"
	)
	for _, tc := range []struct {
		name     string
		repo     func(*testing.T) string
		config   string // repository-relative; empty is .archfit.yaml
		args     func(t *testing.T, dir string) []string
		wantCode int
		wantErr  string
	}{
		{name: "a staged repair blocks the commit", wantCode: 1,
			// The repair names the repository, never the removed snapshot.
			wantErr: "--base HEAD --format agent",
			repo: func(t *testing.T) string {
				dir := hookRepo(t, hookRuleCfg, hookCleanA, hookViolatingA)
				gitFixture(t, dir, "add", "-A")
				return dir
			}},
		{name: "a config below the root still scans the whole repository", wantCode: 1, config: nestedConfig,
			repo: func(t *testing.T) string {
				dir := hookRepo(t, hookRuleCfg, hookCleanA, hookViolatingA)
				writeFixtureFile(t, dir, nestedConfig, hookRuleCfg)
				gitFixture(t, dir, "add", "-A")
				return dir
			}},
		{name: "an unstaged repair does not", wantCode: 0,
			repo: func(t *testing.T) string { return hookRepo(t, hookRuleCfg, hookCleanA, hookViolatingA) }},
		{name: "an unstaged fix does not excuse a staged repair", wantCode: 1,
			repo: func(t *testing.T) string {
				dir := hookRepo(t, hookRuleCfg, hookCleanA, hookViolatingA)
				gitFixture(t, dir, "add", "-A")
				writeFixtureFile(t, dir, filePkgAA, hookCleanA)
				return dir
			}},
		{name: "an untracked file is not part of the commit", wantCode: 0,
			repo: func(t *testing.T) string {
				dir := hookRepo(t, hookRuleCfg, hookCleanA, "")
				writeFixtureFile(t, dir, untrackedFile, hookViolatingA)
				return dir
			}},
		{name: "git commit <path> passes a temporary index", wantCode: 1,
			repo: func(t *testing.T) string { return hookRepo(t, hookRuleCfg, hookCleanA, hookViolatingA) },
			args: func(t *testing.T, dir string) []string {
				index := filepath.Join(t.TempDir(), "next-index")
				gitFixtureEnv(t, dir, []string{"GIT_INDEX_FILE=" + index}, "read-tree", "HEAD")
				gitFixtureEnv(t, dir, []string{"GIT_INDEX_FILE=" + index}, "add", filePkgAA)
				return []string{"--index-file", index}
			}},
		{name: "a pre-existing blocker does not block", wantCode: 0,
			repo: func(t *testing.T) string {
				dir := hookRepo(t, hookRuleCfg, hookViolatingA, hookViolatingA+"\n// unrelated edit\n")
				gitFixture(t, dir, "add", "-A")
				return dir
			}},
		{name: "the first commit has no HEAD to scope against", wantCode: 1,
			repo: func(t *testing.T) string {
				dir := t.TempDir()
				for name, content := range map[string]string{
					markerGoMod: goModStub, filePkgAA: hookViolatingA,
					hookImplFile: implSource(), defaultConfigPath: hookRuleCfg,
				} {
					writeFixtureFile(t, dir, name, content)
				}
				gitInitFixtureRepo(t, dir)
				gitFixture(t, dir, "add", "-A")
				return dir
			}},
		{name: "a pre-existing dead selector does not block", wantCode: 0,
			repo: func(t *testing.T) string {
				dir := hookRepo(t, hookDeadSelectorCfg, hookCleanA, hookCleanA+"\n// edit\n")
				gitFixture(t, dir, "add", "-A")
				return dir
			}},
		{name: "a config the commit removes is an error", wantCode: 3, wantErr: "not in the index",
			repo: func(t *testing.T) string {
				dir := hookRepo(t, hookRuleCfg, hookCleanA, "")
				gitFixture(t, dir, "rm", "-q", "--cached", defaultConfigPath)
				return dir
			}},
		{name: "outside a git repository is an error", wantCode: 3,
			repo: func(t *testing.T) string {
				dir := t.TempDir()
				writeFixtureFile(t, dir, defaultConfigPath, hookRuleCfg)
				return dir
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := tc.repo(t)
			config := tc.config
			if config == "" {
				config = defaultConfigPath
			}
			args := []string{"git", "-c", filepath.Join(dir, config)}
			if tc.args != nil {
				args = append(args, tc.args(t, dir)...)
			}
			before := gitIndexState(t, dir)
			code, _, stderr := runHook(t, "", args...)
			if code != tc.wantCode {
				t.Errorf("exit = %d, want %d\n%s", code, tc.wantCode, stderr)
			}
			if !strings.Contains(stderr, tc.wantErr) {
				t.Errorf("stderr = %q, want %q", stderr, tc.wantErr)
			}
			if strings.Contains(stderr, cacheDirName+"/worktrees") {
				t.Errorf("stderr names the temporary snapshot checkout:\n%s", stderr)
			}
			if after := gitIndexState(t, dir); after != before {
				t.Errorf("the hook changed the index or the worktree:\nbefore %s\nafter  %s", before, after)
			}
		})
	}
}

// TestHookGitValidateNamesTheRepository pins the repair's validate command on
// a repository path that needs shell quoting: it names the config and root in
// the repository, quoted as one argument each, never the removed snapshot.
func TestHookGitValidateNamesTheRepository(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "team's repo")
	for name, content := range map[string]string{
		markerGoMod: goModStub, filePkgAA: hookCleanA, hookImplFile: implSource(), defaultConfigPath: hookRuleCfg,
	} {
		writeFixtureFile(t, dir, name, content)
	}
	gitInitFixtureRepo(t, dir)
	gitCommitFixture(t, dir)
	writeFixtureFile(t, dir, filePkgAA, hookViolatingA)
	gitFixture(t, dir, "add", "-A")
	repo, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runHook(t, "", "git", "-c", filepath.Join(dir, defaultConfigPath))
	if code != 1 {
		t.Fatalf("exit = %d, want 1\n%s", code, stderr)
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'" }
	want := "archfit check -c " + quote(filepath.Join(repo, defaultConfigPath)) + " --root " + quote(repo) + " --base HEAD --format agent"
	if !strings.Contains(stderr, want) {
		t.Errorf("stderr is missing the validate command %q:\n%s", want, stderr)
	}
}

// gitIndexState is the index and worktree state a hook must leave alone: the
// index bytes, the status, and every ref.
func gitIndexState(t *testing.T, dir string) string {
	t.Helper()
	index, err := os.ReadFile(filepath.Join(dir, ".git", "index")) //nolint:gosec // fixture path
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	status, _ := exec.Command("git", "-C", dir, "status", "--porcelain", "--untracked-files=all", "--", ".", ":(exclude,glob)**/"+cacheDirName+"/**").Output() //nolint:gosec // fixture repo
	refs, _ := exec.Command("git", "-C", dir, "for-each-ref").Output()                                                                                         //nolint:gosec // fixture repo
	return fmt.Sprintf("index=%x status=%q refs=%q", sha256.Sum256(index), status, refs)
}

func gitFixture(t *testing.T, dir string, args ...string) {
	t.Helper()
	gitFixtureEnv(t, dir, nil, args...)
}

func gitFixtureEnv(t *testing.T, dir string, env []string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...) //nolint:gosec // fixed git binary, controlled test args
	cmd.Dir = dir
	cmd.Env = append(scrubGitFixtureEnv(os.Environ()), env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
