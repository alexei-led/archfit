package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// archfitExecEnv makes the test binary run the archfit CLI: git runs the
// installed pre-commit hook as a process, and it calls archfit from PATH.
const archfitExecEnv = "ARCHFIT_TEST_EXEC_CLI"

func TestMain(m *testing.M) {
	if os.Getenv(archfitExecEnv) == "1" {
		os.Exit(runWithIO(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

const (
	// sessionSource is the SessionStart stdin key that says why a session started.
	sessionSource    = "source"
	traceSession     = "trace-session"
	traceRationale   = "impl is private to b"
	traceAlternative = "call the API b exports"
	// hookTraceCfg declares owners, subdomains and volatility, so config
	// quality has nothing to say, and a rule with a rationale and an
	// alternative, so a denial shows why and instead.
	hookTraceCfg = lintCleanModules + `rules:
  - id: no_b_impl
    type: forbidden_dependency
    gate: fail
    from: pkg/a/**
    to: pkg/b/impl
    rationale: ` + traceRationale + `
    alternatives: ["` + traceAlternative + `"]
`
)

// installedRuns installs the hooks of a harness into dir and returns the
// archfit arguments each event's command runs, keyed by the harness event.
// The trace calls exactly what the harness would.
func installedRuns(t *testing.T, dir, harness string) map[string][]string {
	t.Helper()
	if code, stdout, stderr := runHook(t, "", "install", "--harness", harness, "--dir", dir); code != 0 {
		t.Fatalf("hook install --harness %s: exit %d\n%s%s", harness, code, stdout, stderr)
	}
	raw, err := os.ReadFile(filepath.Join(dir, harnessConfigFile(harness))) //nolint:gosec // fixture path
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Hooks map[string][]hookGroup `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	runs := map[string][]string{}
	for event, groups := range file.Hooks {
		for _, group := range groups {
			for _, hook := range group.Hooks {
				if args, ok := strings.CutPrefix(hook.Command, "archfit hook "); ok {
					runs[event] = strings.Fields(args)
				}
			}
		}
	}
	return runs
}

// harnessEvent is the stdin a harness hands a hook.
func harnessEvent(t *testing.T, event, cwd string, extra map[string]any) string {
	t.Helper()
	payload := map[string]any{"session_id": traceSession, "cwd": cwd, "hook_event_name": event}
	for k, v := range extra {
		payload[k] = v
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// readSession decodes the session file the hooks keep for traceSession.
func readSession(t *testing.T, dir string) hookSession {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, cacheDirName, sessionDirName, sessionFileName(traceSession))) //nolint:gosec // fixture path
	if err != nil {
		t.Fatal(err)
	}
	var sess hookSession
	if err := json.Unmarshal(raw, &sess); err != nil {
		t.Fatal(err)
	}
	return sess
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...) //nolint:gosec // fixed git binary, controlled test args
	cmd.Dir = dir
	cmd.Env = scrubGitFixtureEnv(os.Environ())
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// TestHookTrace is the protocol trace of one agent session per harness: the
// agent edits through a shell, commits mid-session, and finishes. The
// violation is reported once after the edit and blocks the first stop only;
// the commit does not hide it, and the hooks never react to their own writes.
func TestHookTrace(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{harnessClaude, harnessCodex} {
		t.Run(harness, func(t *testing.T) {
			t.Parallel()
			dir := hookRepo(t, hookTraceCfg, hookCleanA, "")
			runs := installedRuns(t, dir, harness)
			run := func(event string, extra map[string]any) (int, string, string) {
				t.Helper()
				args, ok := runs[event]
				if !ok {
					t.Fatalf("no %s hook installed for %s: %v", event, harness, runs)
				}
				return runHook(t, harnessEvent(t, event, dir, extra), args...)
			}
			shellEdit := map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": "cat > " + filePkgAA}}

			// 1. The session starts at HEAD and the agent reads the bootstrap.
			code, stdout, stderr := run(harnessEventSessionStart, map[string]any{sessionSource: "startup"})
			if code != 0 || stdout != sessionBootstrap || stderr != "" {
				t.Fatalf("session start: exit %d\nstdout %q\nstderr %q", code, stdout, stderr)
			}
			if got, want := readSession(t, dir).BaseSHA, gitOutput(t, dir, "rev-parse", "HEAD"); got != want {
				t.Fatalf("session base = %q, want HEAD %q", got, want)
			}

			// 2. An edit through the shell adds the forbidden import.
			writeFixtureFile(t, dir, filePkgAA, hookViolatingA)
			code, stdout, stderr = run(harnessEventPostToolUse, shellEdit)
			if code != 2 || stdout != "" {
				t.Fatalf("post-tool-use after the edit: exit %d, stdout %q, want 2 and empty stdout\n%s", code, stdout, stderr)
			}
			if n := strings.Count(stderr, ruleNoBImpl); n != 1 {
				t.Errorf("stderr names %s %d times, want once:\n%s", ruleNoBImpl, n, stderr)
			}
			for _, want := range []string{filePkgAA, "example.com/test/pkg/b/impl", "why: " + traceRationale, "instead: " + traceAlternative} {
				if !strings.Contains(stderr, want) {
					t.Errorf("post-tool-use stderr is missing %q:\n%s", want, stderr)
				}
			}

			// 3. The next tool call with nothing new is silent.
			code, stdout, stderr = run(harnessEventPostToolUse, shellEdit)
			hookOutputCase{}.check(t, code, stdout, stderr)

			// 4. A commit mid-session does not hide the violation from the stop.
			gitCommitFixture(t, dir)
			code, stdout, stderr = run(harnessEventStop, map[string]any{stopHookActive: false})
			if code != 2 || stdout != "" || !strings.Contains(stderr, "change fail") || !strings.Contains(stderr, ruleNoBImpl) {
				t.Fatalf("first stop: exit %d, stdout %q, want 2 and the blocker on stderr:\n%s", code, stdout, stderr)
			}

			// 5. The harness's own repeat guard: a stop after a block lets the agent stop.
			code, stdout, stderr = run(harnessEventStop, map[string]any{stopHookActive: true})
			hookOutputCase{wantStdout: "still reports blockers"}.check(t, code, stdout, stderr)

			// 6. The same blockers on the same tree do not block again.
			code, stdout, stderr = run(harnessEventStop, map[string]any{stopHookActive: false})
			hookOutputCase{wantStdout: "already stopped on"}.check(t, code, stdout, stderr)

			// 7. The hooks' own writes are not edits.
			writeFixtureFile(t, dir, filepath.Join(cacheDirName, "scratch", "a.go"), hookViolatingA)
			code, stdout, stderr = run(harnessEventPostToolUse, shellEdit)
			hookOutputCase{}.check(t, code, stdout, stderr)
			for file := range readSession(t, dir).Files {
				if inArchfitCache(file) {
					t.Errorf("the session tracks archfit's own file %s", file)
				}
			}
		})
	}
}

// TestHookTraceGitOnly runs the same violation with only the git hook
// installed: git commit runs the hook as a process, and the commit fails with
// the rule named.
func TestHookTraceGitOnly(t *testing.T) {
	t.Parallel()
	dir := hookRepo(t, hookTraceCfg, hookCleanA, "")
	// A user's global core.hooksPath must not take the fixture's hook.
	gitFixture(t, dir, "config", "core.hooksPath", filepath.Join(".git", "hooks"))
	if code, stdout, stderr := runHook(t, "", "install", "--harness", harnessGit, "--dir", dir); code != 0 {
		t.Fatalf("hook install --harness git: exit %d\n%s%s", code, stdout, stderr)
	}
	writeFixtureFile(t, dir, filePkgAA, hookViolatingA)
	gitFixture(t, dir, "add", "-A")

	out, err := gitCommitWithHooks(t, dir)
	if err == nil {
		t.Fatalf("git commit succeeded with a violation staged:\n%s", out)
	}
	if !strings.Contains(out, ruleNoBImpl) {
		t.Errorf("git commit output does not name %s:\n%s", ruleNoBImpl, out)
	}
	if count := gitOutput(t, dir, "rev-list", "--count", "HEAD"); count != "1" {
		t.Errorf("HEAD has %s commits, want the fixture's 1", count)
	}
}

// gitCommitWithHooks runs git commit with its hooks, on a PATH whose archfit
// is this test binary.
func gitCommitWithHooks(t *testing.T, dir string) (string, error) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	shim := "#!/bin/sh\n" + archfitExecEnv + "=1 exec '" + self + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "archfit"), []byte(shim), 0o700); err != nil { //nolint:gosec // an executable test shim
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "violation")
	cmd.Dir = dir
	cmd.Env = append(scrubGitFixtureEnv(os.Environ()),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestHookRunPostToolUse pins the post-tool-use table: only a denial the
// change adds is reported, and an error never blocks.
func TestHookRunPostToolUse(t *testing.T) {
	t.Parallel()
	const extraFile = "pkg/a/extra.go"
	for _, tc := range []struct {
		name string
		repo func(*testing.T) string
		hookOutputCase
	}{
		{name: "a denied import in an edited file",
			repo:           func(t *testing.T) string { return hookRepo(t, hookTraceCfg, hookCleanA, hookViolatingA) },
			hookOutputCase: hookOutputCase{wantCode: 2, wantStderr: "which rule " + ruleNoBImpl + " denies"}},
		{name: "a denied import in an untracked file",
			repo: func(t *testing.T) string {
				dir := hookRepo(t, hookTraceCfg, hookCleanA, "")
				writeFixtureFile(t, dir, extraFile, hookViolatingA)
				return dir
			}, hookOutputCase: hookOutputCase{wantCode: 2, wantStderr: extraFile + " imports example.com/test/pkg/b/impl"}},
		{name: "an import that was there at the base is not new",
			repo: func(t *testing.T) string {
				return hookRepo(t, hookTraceCfg, hookViolatingA, hookViolatingA+"\n// edit\n")
			}},
		{name: "an import no rule denies is silent",
			repo: func(t *testing.T) string {
				return hookRepo(t, hookTraceCfg, hookCleanA, "package a\n\nimport \"strings\"\n\nvar _ = strings.ToUpper\n")
			}},
		{name: "an archfit error fails open",
			repo: func(t *testing.T) string {
				dir := hookRepo(t, hookTraceCfg, hookCleanA, hookViolatingA)
				if err := os.Remove(filepath.Join(dir, defaultConfigPath)); err != nil {
					t.Fatal(err)
				}
				return dir
			}, hookOutputCase: hookOutputCase{wantStdout: failedOpen}},
		{name: "outside a git repository fails open",
			repo: func(t *testing.T) string {
				dir := t.TempDir()
				writeFixtureFile(t, dir, filePkgAA, hookViolatingA)
				return dir
			}, hookOutputCase: hookOutputCase{wantStdout: failedOpen}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := tc.repo(t)
			code, stdout, stderr := runHook(t, harnessEvent(t, harnessEventPostToolUse, dir, nil), "run", hookEventPostToolUse)
			tc.check(t, code, stdout, stderr)
		})
	}
}

// TestHookRunSessionStartKeepsTheBase: Claude Code fires SessionStart again on
// resume and after compaction; the session's base must not move to a commit
// the agent made since.
func TestHookRunSessionStartKeepsTheBase(t *testing.T) {
	t.Parallel()
	dir := hookRepo(t, hookTraceCfg, hookCleanA, "")
	start := gitOutput(t, dir, "rev-parse", "HEAD")
	for _, source := range []string{"startup", "compact"} {
		if code, _, stderr := runHook(t, harnessEvent(t, harnessEventSessionStart, dir, map[string]any{sessionSource: source}), "run", hookEventSessionStart); code != 0 {
			t.Fatalf("session start (%s): exit %d\n%s", source, code, stderr)
		}
		commitFiles(t, dir, map[string]string{filePkgAA: hookCleanA + "\n// " + source + "\n"})
	}
	if got := readSession(t, dir).BaseSHA; got != start {
		t.Errorf("session base = %q after a second session start, want the first HEAD %q", got, start)
	}
}

// TestHookRunSessionStartBeforeTheFirstCommit records an empty base: the
// whole tree is the change.
func TestHookRunSessionStartBeforeTheFirstCommit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	gitInitFixtureRepo(t, dir)
	code, stdout, stderr := runHook(t, harnessEvent(t, harnessEventSessionStart, dir, nil), "run", hookEventSessionStart)
	if code != 0 || stdout != sessionBootstrap || stderr != "" {
		t.Fatalf("exit %d\nstdout %q\nstderr %q", code, stdout, stderr)
	}
	if sess := readSession(t, dir); sess.BaseSHA != "" || sess.StartedAt.IsZero() {
		t.Errorf("session = %+v, want an empty base and a start time", sess)
	}
}
