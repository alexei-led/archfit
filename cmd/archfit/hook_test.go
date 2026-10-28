package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/output/agentout"
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

// harnessEventStop and harnessEventSubagentStop are the event names Claude
// Code puts in a Stop hook's stdin.
const (
	harnessEventSessionStart = "SessionStart"
	harnessEventPostToolUse  = "PostToolUse"
	harnessEventStop         = "Stop"
	harnessEventSubagentStop = "SubagentStop"
)

// Hook test vocabulary: the stdin key of a repeated stop and the fail-open
// message.
const (
	stopHookActive = "stop_hook_active"
	failedOpen     = "failed open"
)

func stopEvent(event, cwd string, active bool) string {
	raw, _ := json.Marshal(map[string]any{"hook_event_name": event, "cwd": cwd, stopHookActive: active, "session_id": "s"})
	return string(raw)
}

// hookOutputCase is the exit code and the output channels one hook run must
// produce.
type hookOutputCase struct {
	wantCode   int
	wantStdout string // substring of the systemMessage; "" means empty stdout
	wantStderr string // substring of stderr; "" means empty stderr
}

func (tc hookOutputCase) check(t *testing.T, code int, stdout, stderr string) {
	t.Helper()
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

// TestHookRunStop pins one stdin fixture per row of the stop hook table: the
// exit code and the output channel. Every event cwd differs from the test
// process's own cwd, so the hook resolves --config against the event. With no
// session file the session base is HEAD.
func TestHookRunStop(t *testing.T) {
	t.Parallel()
	introduced := func(t *testing.T) string { return hookRepo(t, hookRuleCfg, hookCleanA, hookViolatingA) }
	for _, tc := range []struct {
		name     string
		repo     func(*testing.T) string
		event    string
		active   bool
		rawStdin string
		args     []string
		hookOutputCase
	}{
		{name: "repair on the first stop blocks", repo: introduced, event: harnessEventStop,
			hookOutputCase: hookOutputCase{wantCode: 2, wantStderr: "change fail, next_action repair"}},
		{name: "repair with stop_hook_active blocks only once", repo: introduced, event: harnessEventSubagentStop, active: true,
			hookOutputCase: hookOutputCase{wantStdout: "still reports blockers"}},
		{name: "codex reads the same protocol", repo: introduced, event: harnessEventStop, args: []string{"--harness", harnessCodex},
			hookOutputCase: hookOutputCase{wantCode: 2, wantStderr: "change fail, next_action repair"}},
		{name: "a blocker outside the change is repository attention, not a stop", event: harnessEventStop,
			repo: func(t *testing.T) string {
				return hookRepo(t, hookRuleCfg, hookViolatingA, hookViolatingA+"\n// unrelated edit\n")
			}},
		{name: "nothing to do is silent", event: harnessEventStop,
			repo: func(t *testing.T) string { return hookRepo(t, hookModules, hookCleanA, hookCleanA+"\n// edit\n") }},
		{name: "a clean tree skips the run", event: harnessEventStop,
			repo: func(t *testing.T) string { return hookRepo(t, hookRuleCfg, hookViolatingA, "") }},
		{name: "an archfit error fails open", event: harnessEventStop,
			repo: func(t *testing.T) string {
				dir := hookRepo(t, hookRuleCfg, hookCleanA, hookViolatingA)
				if err := os.Remove(filepath.Join(dir, defaultConfigPath)); err != nil {
					t.Fatal(err)
				}
				return dir
			}, hookOutputCase: hookOutputCase{wantStdout: failedOpen}},
		{name: "outside a git repository fails open", event: harnessEventStop,
			repo: func(t *testing.T) string {
				dir := t.TempDir()
				writeFixtureFile(t, dir, defaultConfigPath, hookRuleCfg)
				return dir
			}, hookOutputCase: hookOutputCase{wantStdout: failedOpen}},
		{name: "archfit's own cache is not a change", event: harnessEventStop,
			repo: func(t *testing.T) string {
				dir := hookRepo(t, hookRuleCfg, hookViolatingA, "")
				writeFixtureFile(t, dir, ".archfit-cache/facts/x.json", "{}")
				return dir
			}},
		{name: "a dead selector is repository attention, not a stop", event: harnessEventStop,
			repo: func(t *testing.T) string {
				return hookRepo(t, hookDeadSelectorCfg, hookCleanA, hookCleanA+"\n// edit\n")
			}},
		{name: "malformed stdin", repo: introduced, rawStdin: "{not json",
			hookOutputCase: hookOutputCase{wantCode: 1, wantStderr: "malformed hook event"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := tc.repo(t)
			stdin := tc.rawStdin
			if stdin == "" {
				stdin = stopEvent(tc.event, dir, tc.active)
			}
			code, stdout, stderr := runHook(t, stdin, append([]string{"run", hookEventStop}, tc.args...)...)
			tc.check(t, code, stdout, stderr)
		})
	}
}

// TestHookRunRejectsAnUnknownEvent: the event is part of the command line, so
// a typo is a usage error, never a silent pass.
func TestHookRunRejectsAnUnknownEvent(t *testing.T) {
	t.Parallel()
	code, stdout, stderr := runHook(t, "{}", "run", "pre-tool-use")
	if code != 3 || stdout != "" || !strings.Contains(stderr, "pre-tool-use") {
		t.Errorf("exit = %d, stdout = %q, stderr = %q; want 3 and the event named on stderr", code, stdout, stderr)
	}
}

// TestHookRunStopStderrCarriesTheRepair pins what the agent reads on a block:
// the finding ID, the file:line, the goal, and the validation command.
func TestHookRunStopStderrCarriesTheRepair(t *testing.T) {
	t.Parallel()
	dir := hookRepo(t, hookRuleCfg, hookCleanA, hookViolatingA)
	code, _, stderr := runHook(t, stopEvent(harnessEventStop, dir, false), "run", hookEventStop)
	if code != 2 {
		t.Fatalf("exit = %d, want 2\n%s", code, stderr)
	}
	for _, want := range []string{ruleNoBImpl, filePkgAA + ":3", "goal: ", "validate: archfit check", "--format agent"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr is missing %q:\n%s", want, stderr)
		}
	}
}

// TestHookRunPreCommit pins that the pre-commit hook judges the index — what
// the commit will contain — and never the files on disk: an unstaged or
// untracked edit neither blocks nor excuses a commit.
func TestHookRunPreCommit(t *testing.T) {
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
			args := []string{"run", hookEventPreCommit, "-c", filepath.Join(dir, config)}
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

// TestHookRunPreCommitValidateNamesTheRepository pins the repair's validate
// command on a repository path that needs shell quoting: it names the config
// and root in the repository, quoted as one argument each, never the removed
// snapshot.
func TestHookRunPreCommitValidateNamesTheRepository(t *testing.T) {
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
	code, _, stderr := runHook(t, "", "run", hookEventPreCommit, "-c", filepath.Join(dir, defaultConfigPath))
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

// TestHookOutcomeFollowsTheChangeStatus feeds the agent JSON of the five
// result-contract cases to the hook decision. The same files are the CLI's
// output, so the CLI and the hook cannot read one result two ways.
func TestHookOutcomeFollowsTheChangeStatus(t *testing.T) {
	t.Parallel()
	cases := map[string]hookOutcome{
		"new_violation.json":                         hookBlock,
		"existing_violation.json":                    hookAllow,
		"unmeasured_ratchet_from_profile_drift.json": hookAllow,
		"dead_selector.json":                         hookAllow,
		"missing_evidence.json":                      hookReport,
	}
	dir := filepath.Join("..", "..", "internal", "output", "agentout", "testdata", "cases")
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			raw, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // fixture path is a constant
			if err != nil {
				t.Fatal(err)
			}
			var result agentout.Result
			if err := json.Unmarshal(raw, &result); err != nil {
				t.Fatalf("decode %s: %v", name, err)
			}
			if got := outcomeOf(result); got != want {
				t.Errorf("outcome = %d for change %q, want %d", got, result.Change.Status, want)
			}
		})
	}
}

// TestIdentityGitFactsFollowTheAnalysisRoot: with an external policy directory
// the tree digest names the analyzed checkout, never the repository that holds
// the config.
func TestIdentityGitFactsFollowTheAnalysisRoot(t *testing.T) {
	t.Parallel()
	root := hookRepo(t, hookModules, hookCleanA, "")
	policyDir := t.TempDir()
	writeFixtureFile(t, policyDir, defaultConfigPath, hookModules)
	gitInitFixtureRepo(t, policyDir)
	gitCommitFixture(t, policyDir)

	_, out, stderr := runArchfit(t, cmdCheck, "-c", filepath.Join(policyDir, defaultConfigPath), "--root", root, "--progress=none", "--format="+formatJSON)
	var state struct {
		Identity struct {
			HeadTreeDigest string `json:"head_tree_digest"`
			PolicyDigest   string `json:"policy_digest"`
		} `json:"identity"`
	}
	if err := json.Unmarshal([]byte(out), &state); err != nil {
		t.Fatalf("decode: %v\nstderr: %s", err, stderr)
	}
	tree := func(dir string) string {
		raw, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD^{tree}").Output() //nolint:gosec // fixture directory from t.TempDir
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(raw))
	}
	if want := tree(root); state.Identity.HeadTreeDigest != want {
		t.Errorf("head_tree_digest = %q, want the analysis root's tree %q (config repo tree is %q)", state.Identity.HeadTreeDigest, want, tree(policyDir))
	}
	if sum := sha256.Sum256([]byte(hookModules)); state.Identity.PolicyDigest != hex.EncodeToString(sum[:]) {
		t.Errorf("policy_digest = %q, want the digest of the external config", state.Identity.PolicyDigest)
	}
}
