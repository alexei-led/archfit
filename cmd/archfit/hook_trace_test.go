package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/initcfg/policy3"
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

// sessionSource is the SessionStart stdin key that says why a session started.
const (
	sessionSource = "source"
	traceSession  = "trace-session"
)

// installedRuns installs the hooks of a harness into dir and returns the
// archfit arguments each event's command runs, keyed by the harness event.
// The trace calls exactly what the harness would.
func installedRuns(t *testing.T, dir, harness string) map[string][]string {
	t.Helper()
	installFixtureHooks(t, dir, harness)
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

// outsideARepository names the hook-error case of the hook tables.
const outsideARepository = "outside a git repository the hook reports and stops nothing"

// Hook fixture files: an edit of pkg/tc/netem.go that only uses the standard
// library (allowed), and one that adds the graph error of design 8.1
// (tcImportsUtil, change_check_test.go).
const (
	tcUsesFmt = "package tc\n\nimport \"fmt\"\n\n// Netem shapes traffic.\nfunc Netem() string { return fmt.Sprint(\"netem\") }\n"
	chaosFile = "pkg/chaos/netem/netem.go"
	// graphRecord and denyRecord start the records of the two errors.
	graphRecord = "graph: pkg_tc -> pkg_util is not an allowed dependency"
	denyRecord  = "no-chaos-to-runtime: pkg_chaos must not depend on pkg_runtime"
	// canGraph starts the record archfit can prints for the graph error.
	canGraph = "pkg_tc -> pkg_util: graph: not an allowed dependency"
)

// coveredException lets pkg_tc depend on pkg_util until a far date: check
// passes the import, though archfit can answers no.
const coveredException = `exceptions:
  - rule: graph
    from: pkg_tc
    to: pkg_util
    expires: 2099-01-01
    why: "Request #12 is open."
`

// hookFixture is the change-check fixture: the base on main (origin/HEAD) and
// an empty change on the branch, with base the extra files of the base.
func hookFixture(t *testing.T, base map[string]string) string {
	t.Helper()
	return fixtureRepo(t, with(fixtureFiles(), base), nil)
}

// hookRun runs one hook event in dir, as a harness would.
func hookRun(t *testing.T, dir, harnessEventName, run string, extra map[string]any) (int, string, string) {
	t.Helper()
	return runHook(t, harnessEvent(t, harnessEventName, dir, extra), "run", run)
}

// TestHookTrace is the protocol trace of one agent session per harness,
// through the installed hooks: the agent edits through a shell, commits
// mid-session, and finishes. The error is reported once after the edit and
// stops the first stop only; the commit does not hide it.
func TestHookTrace(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{harnessClaude, harnessCodex} {
		t.Run(harness, func(t *testing.T) {
			t.Parallel()
			dir := hookFixture(t, nil)
			runs := installedRuns(t, dir, harness)
			run := func(event string, extra map[string]any) (int, string, string) {
				t.Helper()
				args, ok := runs[event]
				if !ok {
					t.Fatalf("no %s hook installed for %s: %v", event, harness, runs)
				}
				code, stdout, stderr := runHook(t, harnessEvent(t, event, dir, extra), args...)
				checkHookWords(t, stdout+stderr)
				return code, stdout, stderr
			}
			shellEdit := map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": "cat > " + tcNetem}}

			// 1. The session starts at HEAD and the agent reads the five lines.
			code, stdout, stderr := run(harnessEventSessionStart, map[string]any{sessionSource: "startup"})
			if code != 0 || stdout != policy3.Lines || stderr != "" {
				t.Fatalf("session start: exit %d\nstdout %q\nstderr %q", code, stdout, stderr)
			}
			if got, want := readSession(t, dir).BaseSHA, gitOutput(t, dir, "rev-parse", "HEAD"); got != want {
				t.Fatalf("session base = %q, want HEAD %q", got, want)
			}

			// 2. An edit through the shell adds the import of the graph error.
			writeFiles(t, dir, map[string]string{tcNetem: tcImportsUtil})
			code, stdout, stderr = run(harnessEventPostToolUse, shellEdit)
			if code != 2 || stdout != "" || !strings.Contains(stderr, tcNetem+":12: import pkg/util\n"+canGraph) {
				t.Fatalf("post-tool-use after the edit: exit %d, stdout %q, want 2 and the record\n%s", code, stdout, stderr)
			}

			// 3. The next tool call with nothing new is silent.
			code, stdout, stderr = run(harnessEventPostToolUse, shellEdit)
			hookOutputCase{}.check(t, code, stdout, stderr)

			// 4. A commit mid-session does not hide the error from the stop.
			gitRun(t, dir, "add", "-A")
			gitRun(t, dir, "commit", "-qm", "edit", "--no-gpg-sign")
			code, stdout, stderr = run(harnessEventStop, map[string]any{stopHookActive: false})
			if code != 2 || stdout != "" || !strings.Contains(stderr, graphRecord) || !strings.Contains(stderr, "archfit: fail, 1 error") {
				t.Fatalf("first stop: exit %d, stdout %q, want 2 and the record on stderr:\n%s", code, stdout, stderr)
			}

			// 5. The harness's own repeat guard: a stop after a stop lets the agent stop.
			code, stdout, stderr = run(harnessEventStop, map[string]any{stopHookActive: true})
			hookOutputCase{wantStdout: "still fails after one attempt"}.check(t, code, stdout, stderr)

			// 6. The same errors do not stop again.
			code, stdout, stderr = run(harnessEventStop, map[string]any{stopHookActive: false})
			hookOutputCase{wantStdout: "the same errors as at the last stop"}.check(t, code, stdout, stderr)
		})
	}
}

// TestHookTraceGitOnly: git commit runs the installed pre-commit hook as a
// process, and the commit fails with the record of the error.
func TestHookTraceGitOnly(t *testing.T) {
	t.Parallel()
	dir := hookFixture(t, nil)
	// A user's global core.hooksPath must not take the fixture's hook.
	gitRun(t, dir, "config", "core.hooksPath", filepath.Join(".git", "hooks"))
	installFixtureHooks(t, dir, harnessGit)
	writeFiles(t, dir, map[string]string{tcNetem: tcImportsUtil})
	gitRun(t, dir, "add", "-A")
	before := gitOutput(t, dir, "rev-parse", "HEAD")

	out, err := gitCommitWithHooks(t, dir)
	if err == nil {
		t.Fatalf("git commit succeeded with an error staged:\n%s", out)
	}
	if !strings.Contains(out, graphRecord) {
		t.Errorf("git commit output does not hold the record:\n%s", out)
	}
	if after := gitOutput(t, dir, "rev-parse", "HEAD"); after != before {
		t.Errorf("HEAD moved to %s, want the commit stopped", after)
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
	cmd := exec.Command("git", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "edit")
	cmd.Dir = dir
	cmd.Env = append(scrubGitFixtureEnv(os.Environ()),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestHookRunPostToolUse pins the post-tool-use table: only a no from archfit
// can stops the agent; a yes, an unknown, a covered import, a tree with no
// version 3 policy, and a hook error never do.
func TestHookRunPostToolUse(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		repo func(*testing.T) string
		hookOutputCase
	}{
		{name: "a forbidden import stops with the record",
			repo: func(t *testing.T) string {
				dir := hookFixture(t, nil)
				writeFiles(t, dir, map[string]string{tcNetem: tcImportsUtil})
				return dir
			}, hookOutputCase: hookOutputCase{wantCode: 2, wantStderr: tcNetem + ":12: import pkg/util\n" + canGraph}},
		{name: "a deny rule names why and instead",
			repo: func(t *testing.T) string {
				dir := hookFixture(t, nil)
				writeFiles(t, dir, map[string]string{chaosFile: chaosImportsRuntime})
				return dir
			}, hookOutputCase: hookOutputCase{wantCode: 2, wantStderr: "  instead: Add a method to container.Client."}},
		{name: "an allowed import is silent",
			repo: func(t *testing.T) string {
				dir := hookFixture(t, nil)
				writeFiles(t, dir, map[string]string{tcNetem: tcUsesFmt})
				return dir
			}},
		{name: "an import an exception covers is silent",
			repo: func(t *testing.T) string {
				dir := hookFixture(t, map[string]string{defaultConfigPath: fixturePolicy + coveredException})
				writeFiles(t, dir, map[string]string{tcNetem: tcImportsUtil})
				return dir
			}},
		{name: "an unknown answer is silent",
			repo: func(t *testing.T) string {
				dir := hookFixture(t, nil)
				writeFiles(t, dir, map[string]string{"tools/gen/gen.go": "package gen\n\nimport \"example.com/pumba/pkg/util\"\n\nvar _ = util.NotRunning\n"})
				return dir
			}},
		{name: "an import that was there at the session base is not new",
			repo: func(t *testing.T) string {
				return fixtureRepo(t, fixtureFiles(), map[string]string{tcNetem: tcImportsUtil})
			}},
		{name: "a version 2 policy is silent",
			repo: func(t *testing.T) string {
				dir := hookFixture(t, nil)
				writeFiles(t, dir, map[string]string{defaultConfigPath: hookModules, tcNetem: tcImportsUtil})
				return dir
			}},
		{name: outsideARepository,
			repo: func(t *testing.T) string {
				dir := t.TempDir()
				writeFiles(t, dir, map[string]string{tcNetem: tcImportsUtil})
				return dir
			}, hookOutputCase: hookOutputCase{wantStdout: couldNotRunMsg}},
		{name: "a broken session file: the hook reports and stops nothing",
			repo: func(t *testing.T) string {
				dir := hookFixture(t, nil)
				writeFiles(t, dir, map[string]string{
					tcNetem: tcImportsUtil,
					filepath.Join(cacheDirName, sessionDirName, sessionFileName(traceSession)): "{not json",
				})
				return dir
			}, hookOutputCase: hookOutputCase{wantStdout: couldNotRunMsg}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := tc.repo(t)
			code, stdout, stderr := hookRun(t, dir, harnessEventPostToolUse, hookEventPostToolUse, nil)
			tc.check(t, code, stdout, stderr)
		})
	}
}

// TestHookRunPostToolUseReportsOnce: a no is reported once while the file
// adds it, and again after it was removed and comes back.
func TestHookRunPostToolUseReportsOnce(t *testing.T) {
	t.Parallel()
	dir := hookFixture(t, nil)
	postToolUse := func(want hookOutputCase) {
		t.Helper()
		code, stdout, stderr := hookRun(t, dir, harnessEventPostToolUse, hookEventPostToolUse, nil)
		want.check(t, code, stdout, stderr)
	}
	reported := hookOutputCase{wantCode: 2, wantStderr: canGraph}
	writeFiles(t, dir, map[string]string{tcNetem: tcImportsUtil})
	postToolUse(reported)
	writeFiles(t, dir, map[string]string{runtimeGo: "package runtime\n\n// Do runs.\nfunc Do() {}\n\n// Stop stops.\nfunc Stop() {}\n"})
	postToolUse(hookOutputCase{})
	writeFiles(t, dir, map[string]string{tcNetem: tcUsesFmt})
	postToolUse(hookOutputCase{})
	writeFiles(t, dir, map[string]string{tcNetem: tcImportsUtil})
	postToolUse(reported)
}

// TestHookRunStop pins the stop table: only a fail stops the agent; an unknown
// and a hook error show a message and stop nothing.
func TestHookRunStop(t *testing.T) {
	t.Parallel()
	failing := func(t *testing.T) string {
		dir := hookFixture(t, nil)
		writeFiles(t, dir, map[string]string{tcNetem: tcImportsUtil})
		return dir
	}
	for _, tc := range []struct {
		name   string
		repo   func(*testing.T) string
		event  string
		active bool
		args   []string
		hookOutputCase
	}{
		{name: "a fail stops the agent with the record", repo: failing,
			hookOutputCase: hookOutputCase{wantCode: 2, wantStderr: graphRecord}},
		{name: "codex reads the same protocol", repo: failing, args: []string{flagHarness, harnessCodex},
			hookOutputCase: hookOutputCase{wantCode: 2, wantStderr: graphRecord}},
		{name: "a fail with stop_hook_active lets the agent stop", repo: failing, active: true,
			hookOutputCase: hookOutputCase{wantStdout: "still fails after one attempt"}},
		{name: "a pass is silent",
			repo: func(t *testing.T) string {
				dir := hookFixture(t, nil)
				writeFiles(t, dir, map[string]string{tcNetem: tcUsesFmt})
				return dir
			}},
		{name: "an exception covers the error",
			repo: func(t *testing.T) string {
				dir := hookFixture(t, map[string]string{defaultConfigPath: fixturePolicy + coveredException})
				writeFiles(t, dir, map[string]string{tcNetem: tcImportsUtil})
				return dir
			}},
		{name: "a changed policy is unknown: a message, no stop",
			repo: func(t *testing.T) string {
				dir := hookFixture(t, nil)
				writeFiles(t, dir, map[string]string{defaultConfigPath: fixturePolicy + coveredException, tcNetem: tcImportsUtil})
				return dir
			}, hookOutputCase: hookOutputCase{wantStdout: "policy: differs from origin/main"}},
		{name: "a clean tree skips the run", repo: func(t *testing.T) string { return hookFixture(t, nil) }},
		{name: "archfit's own cache is not a change",
			repo: func(t *testing.T) string {
				dir := hookFixture(t, nil)
				writeFiles(t, dir, map[string]string{filepath.Join(cacheDirName, "facts", "x.json"): "{}"})
				return dir
			}},
		{name: outsideARepository,
			repo: func(t *testing.T) string { return t.TempDir() }, hookOutputCase: hookOutputCase{wantStdout: couldNotRunMsg}},
		{name: "malformed stdin", repo: failing, event: "malformed",
			hookOutputCase: hookOutputCase{wantCode: 1, wantStderr: "malformed hook event"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := tc.repo(t)
			stdin := harnessEvent(t, harnessEventStop, dir, map[string]any{stopHookActive: tc.active})
			if tc.event == "malformed" {
				stdin = "{not json"
			}
			code, stdout, stderr := runHook(t, stdin, append([]string{"run", hookEventStop}, tc.args...)...)
			tc.check(t, code, stdout, stderr)
		})
	}
}

// TestHookRunStopOncePerErrorSet: the stop does not stop twice on the same set
// of errors, wherever their lines moved; a new error stops again, and so do
// the same errors after a pass.
func TestHookRunStopOncePerErrorSet(t *testing.T) {
	t.Parallel()
	dir := hookFixture(t, nil)
	stop := func(want hookOutputCase) {
		t.Helper()
		code, stdout, stderr := hookRun(t, dir, harnessEventStop, hookEventStop, map[string]any{stopHookActive: false})
		want.check(t, code, stdout, stderr)
	}
	same := hookOutputCase{wantStdout: "the same errors as at the last stop"}
	writeFiles(t, dir, map[string]string{tcNetem: tcImportsUtil})
	stop(hookOutputCase{wantCode: 2, wantStderr: graphRecord})
	stop(same)
	writeFiles(t, dir, map[string]string{tcNetem: "// Package tc.\n" + tcImportsUtil})
	stop(same)
	writeFiles(t, dir, map[string]string{chaosFile: chaosImportsRuntime})
	stop(hookOutputCase{wantCode: 2, wantStderr: denyRecord})
	stop(same)
	writeFiles(t, dir, map[string]string{tcNetem: tcUsesFmt, chaosFile: fixtureFiles()[chaosFile]})
	stop(hookOutputCase{})
	writeFiles(t, dir, map[string]string{tcNetem: tcImportsUtil})
	stop(hookOutputCase{wantCode: 2, wantStderr: graphRecord})
}

// TestHookRunSubagentStopDoesNotUseUpTheStop: a subagent shares the session;
// its stop is stopped, but must not keep the main agent's stop from stopping.
func TestHookRunSubagentStopDoesNotUseUpTheStop(t *testing.T) {
	t.Parallel()
	dir := hookFixture(t, nil)
	writeFiles(t, dir, map[string]string{tcNetem: tcImportsUtil})
	for _, event := range []string{harnessEventSubagentStop, harnessEventSubagentStop, harnessEventStop} {
		code, stdout, stderr := hookRun(t, dir, event, hookEventStop, map[string]any{stopHookActive: false})
		hookOutputCase{wantCode: 2, wantStderr: graphRecord}.check(t, code, stdout, stderr)
	}
}

// TestHookRunFromASubdirectory: the agent's shell changed directory, so the
// event cwd is below the repository root; the hooks still read the root.
func TestHookRunFromASubdirectory(t *testing.T) {
	t.Parallel()
	dir := hookFixture(t, nil)
	writeFiles(t, dir, map[string]string{tcNetem: tcImportsUtil})
	sub := filepath.Join(dir, "pkg", "tc")
	code, stdout, stderr := hookRun(t, sub, harnessEventPostToolUse, hookEventPostToolUse, nil)
	hookOutputCase{wantCode: 2, wantStderr: canGraph}.check(t, code, stdout, stderr)
	code, stdout, stderr = hookRun(t, sub, harnessEventStop, hookEventStop, nil)
	hookOutputCase{wantCode: 2, wantStderr: graphRecord}.check(t, code, stdout, stderr)
}

// TestHookRunSessionStart: the session starts at HEAD and keeps that base on
// resume and after compaction; before the first commit it has no base.
func TestHookRunSessionStart(t *testing.T) {
	t.Parallel()
	dir := hookFixture(t, nil)
	start := gitOutput(t, dir, "rev-parse", "HEAD")
	for _, source := range []string{"startup", "compact"} {
		code, stdout, stderr := hookRun(t, dir, harnessEventSessionStart, hookEventSessionStart, map[string]any{sessionSource: source})
		if code != 0 || stdout != policy3.Lines || stderr != "" {
			t.Fatalf("session start (%s): exit %d\nstdout %q\nstderr %q", source, code, stdout, stderr)
		}
		checkHookWords(t, stdout)
		writeFiles(t, dir, map[string]string{runtimeGo: "package runtime\n\n// Do runs " + source + ".\nfunc Do() {}\n"})
		gitRun(t, dir, "commit", "-qam", source, "--no-gpg-sign")
	}
	if got := readSession(t, dir).BaseSHA; got != start {
		t.Errorf("session base = %q after a second session start, want the first HEAD %q", got, start)
	}

	empty := t.TempDir()
	gitInitFixtureRepo(t, empty)
	code, stdout, stderr := hookRun(t, empty, harnessEventSessionStart, hookEventSessionStart, nil)
	if code != 0 || stdout != policy3.Lines || stderr != "" {
		t.Fatalf("before the first commit: exit %d\nstdout %q\nstderr %q", code, stdout, stderr)
	}
	if sess := readSession(t, empty); sess.BaseSHA != "" || sess.StartedAt.IsZero() {
		t.Errorf("session = %+v, want an empty base and a start time", sess)
	}
}

// TestHookRunPreCommit pins that the pre-commit hook judges the index, what
// the commit will contain, against HEAD, and never the files on disk. Only a
// fail stops the commit; an unknown and a hook error are shown.
func TestHookRunPreCommit(t *testing.T) {
	for _, tc := range []struct {
		name     string
		repo     func(*testing.T) string
		wantCode int
		wantErr  string // substring of stderr; "" means empty stderr
	}{
		{name: "a staged error stops the commit", wantCode: 1, wantErr: graphRecord,
			repo: func(t *testing.T) string {
				dir := hookFixture(t, nil)
				writeFiles(t, dir, map[string]string{tcNetem: tcImportsUtil})
				gitRun(t, dir, "add", "-A")
				return dir
			}},
		{name: "a staged clean file with an error in the working tree passes",
			repo: func(t *testing.T) string {
				dir := hookFixture(t, nil)
				writeFiles(t, dir, map[string]string{tcNetem: tcUsesFmt})
				gitRun(t, dir, "add", "-A")
				writeFiles(t, dir, map[string]string{tcNetem: tcImportsUtil, "pkg/tc/extra.go": tcImportsUtil})
				return dir
			}},
		{name: "a staged error with a clean working tree stops the commit", wantCode: 1, wantErr: graphRecord,
			repo: func(t *testing.T) string {
				dir := hookFixture(t, nil)
				writeFiles(t, dir, map[string]string{tcNetem: tcImportsUtil})
				gitRun(t, dir, "add", "-A")
				writeFiles(t, dir, map[string]string{tcNetem: tcUsesFmt})
				return dir
			}},
		{name: "an error HEAD already has is not this commit's",
			repo: func(t *testing.T) string {
				dir := fixtureRepo(t, fixtureFiles(), map[string]string{tcNetem: tcImportsUtil})
				writeFiles(t, dir, map[string]string{runtimeGo: "package runtime\n\n// Do runs.\nfunc Do() {}\n\n// Stop stops.\nfunc Stop() {}\n"})
				gitRun(t, dir, "add", "-A")
				return dir
			}},
		{name: "a staged policy change is unknown: shown, not stopped", wantErr: "policy: differs from ",
			repo: func(t *testing.T) string {
				dir := hookFixture(t, nil)
				writeFiles(t, dir, map[string]string{defaultConfigPath: fixturePolicy + coveredException, tcNetem: tcImportsUtil})
				gitRun(t, dir, "add", "-A")
				return dir
			}},
		{name: outsideARepository, wantErr: couldNotRunMsg,
			repo: func(t *testing.T) string { return t.TempDir() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := tc.repo(t)
			t.Chdir(dir)
			code, stdout, stderr := runHook(t, "", "run", hookEventPreCommit)
			checkHookWords(t, stdout+stderr)
			if code != tc.wantCode || stdout != "" {
				t.Fatalf("exit %d, want %d; stdout %q\nstderr:\n%s", code, tc.wantCode, stdout, stderr)
			}
			if tc.wantErr == "" && stderr != "" || !strings.Contains(stderr, tc.wantErr) {
				t.Errorf("stderr = %q, want %q", stderr, tc.wantErr)
			}
		})
	}
}

// TestHookRunPostToolUseCannotRecordTheSession: a session file the hook cannot
// write is the hook's own failure: a message, never a stop, also when the
// edit adds nothing to show.
func TestHookRunPostToolUseCannotRecordTheSession(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root writes into a read-only directory")
	}
	dir := hookFixture(t, nil)
	sessions := filepath.Join(dir, cacheDirName, sessionDirName)
	if err := os.MkdirAll(sessions, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sessions, 0o500); err != nil { //nolint:gosec // a read-only directory on purpose
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sessions, 0o750) }) //nolint:gosec // let t.TempDir remove it
	writeFiles(t, dir, map[string]string{tcNetem: tcUsesFmt})
	code, stdout, stderr := hookRun(t, dir, harnessEventPostToolUse, hookEventPostToolUse, nil)
	// No words check: the message ends with the error text of the system.
	if code != 0 || stderr != "" || !strings.Contains(systemMessageOf(t, stdout), "the session was not recorded") {
		t.Errorf("exit %d, stdout %q, stderr %q; want 0 and a systemMessage", code, stdout, stderr)
	}
}

// TestHookRunPostToolUseLeavesTheAnswerForRequest: the record of a no ends
// with archfit request; that command reads the answer the hook stored, as it
// reads the answer of archfit can.
func TestHookRunPostToolUseLeavesTheAnswerForRequest(t *testing.T) {
	t.Parallel()
	dir := hookFixture(t, nil)
	writeFiles(t, dir, map[string]string{tcNetem: tcImportsUtil})
	code, stdout, stderr := hookRun(t, dir, harnessEventPostToolUse, hookEventPostToolUse, nil)
	hookOutputCase{wantCode: 2, wantStderr: `next: archfit request -m "<why you need it>"`}.check(t, code, stdout, stderr)
	req := runCLI(t, fixtureDate, "request", "-m", "the netem shaper needs the message", "-C", dir)
	if out := req.stdout + req.stderr; strings.Contains(out, "run archfit check first") || !strings.Contains(out, "pkg_tc -> pkg_util") {
		t.Errorf("archfit request after the hook: exit %d\n%s", req.code, out)
	}
}
