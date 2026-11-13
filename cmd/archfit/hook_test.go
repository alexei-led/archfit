package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/testutil"
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

// harnessEventStop and harnessEventSubagentStop are the event names Claude
// Code puts in a Stop hook's stdin.
const (
	harnessEventSessionStart = "SessionStart"
	harnessEventPostToolUse  = "PostToolUse"
	harnessEventStop         = "Stop"
	harnessEventSubagentStop = "SubagentStop"
)

// Hook test vocabulary: the stdin key of a repeated stop and the message of a
// hook that fails for its own reasons.
const (
	stopHookActive = "stop_hook_active"
	couldNotRunMsg = "archfit hook could not run"
)

// hookOutputCase is the exit code and the output channels one hook run must
// produce.
type hookOutputCase struct {
	wantCode   int
	wantStdout string // substring of the systemMessage; "" means empty stdout
	wantStderr string // substring of stderr; "" means empty stderr
}

func (tc hookOutputCase) check(t *testing.T, code int, stdout, stderr string) {
	t.Helper()
	checkHookWords(t, stdout+stderr)
	if code != tc.wantCode {
		t.Fatalf("exit = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, tc.wantCode, stdout, stderr)
	}
	if tc.wantStdout == "" && stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if tc.wantStdout != "" {
		if msg := systemMessageOf(t, stdout); !strings.Contains(msg, tc.wantStdout) {
			t.Errorf("stdout = %q, want a systemMessage containing %q", stdout, tc.wantStdout)
		}
	}
	if tc.wantStderr == "" && stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
	if tc.wantStderr != "" && !strings.Contains(stderr, tc.wantStderr) {
		t.Errorf("stderr = %q, want %q", stderr, tc.wantStderr)
	}
}

// checkHookWords fails the test when hook output uses a word of the "Not
// used" column of design section 5.
func checkHookWords(t *testing.T, output string) {
	t.Helper()
	if words := testutil.NotUsedIn(output); len(words) > 0 {
		t.Errorf("hook output uses %q (design 5, the Not used column):\n%s", words, output)
	}
}

// systemMessageOf decodes the hook's {"systemMessage": ...} stdout.
func systemMessageOf(t *testing.T, stdout string) string {
	t.Helper()
	var msg struct {
		SystemMessage string `json:"systemMessage"`
	}
	if err := json.Unmarshal([]byte(stdout), &msg); err != nil {
		t.Fatalf("stdout = %q is not a systemMessage: %v", stdout, err)
	}
	return msg.SystemMessage
}

// TestHookRunRejectsAnUnknownEvent: the event is part of the command line, so
// a typo is a usage error, never a silent pass.
func TestHookRunRejectsAnUnknownEvent(t *testing.T) {
	t.Parallel()
	code, stdout, stderr := runHook(t, "{}", "run", "pre-tool-use")
	if code != exitUnknown || stdout != "" || !strings.Contains(stderr, "pre-tool-use") {
		t.Errorf("exit = %d, stdout = %q, stderr = %q; want 2 and the event named on stderr", code, stdout, stderr)
	}
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

// TestSessionFileName keeps a harness session id to a safe file name.
func TestSessionFileName(t *testing.T) {
	t.Parallel()
	for id, want := range map[string]string{
		"0b6f1c4e-9d2a-4c1b-8f3e-7a5d2c9e1b40": "0b6f1c4e-9d2a-4c1b-8f3e-7a5d2c9e1b40.json",
		"../../etc/passwd":                     "______etc_passwd.json",
		"":                                     "default.json",
	} {
		if got := sessionFileName(id); got != want {
			t.Errorf("sessionFileName(%q) = %q, want %q", id, got, want)
		}
	}
}
