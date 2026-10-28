package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// existingClaudeSettings is a settings file a user already has: foreign keys,
// foreign hooks next to the events archfit installs into, an entry of the
// removed `archfit hook claude`, and an archfit hook run entry from an
// earlier install with another config.
const existingClaudeSettings = `{
  "model": "opus",
  "permissions": {"allow": ["Bash(go test:*)"], "deny": []},
  "hooks": {
    "PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "guard && echo <ok>"}]}],
    "Stop": [
      {"hooks": [{"type": "command", "command": "archfit hook claude"}]},
      {"hooks": [{"type": "command", "command": "notify-done", "timeout": 5}]},
      {"hooks": [{"type": "command", "command": "archfit hook run stop --harness claude -c old.yaml"}]}
    ],
    "SubagentStop": [{"hooks": [{"type": "command", "command": "archfit hook claude"}]}],
    "PostToolUse": [{"matcher": "Write", "hooks": [{"type": "command", "command": "fmt-file"}, {"type": "command", "command": "archfit hook run post-tool-use"}]}]
  }
}
`

// settingsFile is the decoded part of a hook file the install test reads.
type settingsFile struct {
	Hooks map[string][]struct {
		Matcher string `json:"matcher"`
		Hooks   []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	} `json:"hooks"`
}

func readSettings(t *testing.T, path string) (settingsFile, map[string]any) {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // fixture path
	if err != nil {
		t.Fatal(err)
	}
	var typed settingsFile
	var loose map[string]any
	if err := json.Unmarshal(raw, &typed); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &loose); err != nil {
		t.Fatal(err)
	}
	return typed, loose
}

// TestHookInstall pins the merge into a Claude Code or Codex hook file:
// archfit's events each get one archfit hook run entry, every foreign key and
// hook stays, the removed command goes, and a second install changes nothing.
func TestHookInstall(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		harness     string
		existing    string
		wantEvents  map[string]string // event → matcher
		wantForeign map[string][]string
		wantStdout  string
	}{
		{harness: harnessClaude, existing: existingClaudeSettings,
			wantEvents: map[string]string{
				harnessEventSessionStart: "", harnessEventPostToolUse: "Edit|Write|MultiEdit|NotebookEdit|Bash",
				harnessEventStop: "", harnessEventSubagentStop: "",
			},
			wantForeign: map[string][]string{
				"PreToolUse": {"guard && echo <ok>"}, harnessEventStop: {"notify-done"}, harnessEventPostToolUse: {"fmt-file"},
			}},
		{harness: harnessCodex,
			wantEvents: map[string]string{harnessEventSessionStart: "", harnessEventPostToolUse: "^apply_patch$|^Bash$", harnessEventStop: ""},
			wantStdout: "codex runs these hooks only after you trust them"},
	} {
		t.Run(tc.harness, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, harnessConfigFile(tc.harness))
			if tc.existing != "" {
				writeFixtureFile(t, dir, harnessConfigFile(tc.harness), tc.existing)
			}
			code, stdout, stderr := runHook(t, "", "install", flagHarness, tc.harness, flagDir, dir)
			if code != 0 || stderr != "" || !strings.Contains(stdout, "wrote "+path+"\n") || !strings.Contains(stdout, tc.wantStdout) {
				t.Fatalf("install: exit %d\nstdout %q\nstderr %q", code, stdout, stderr)
			}
			first, err := os.ReadFile(path) //nolint:gosec // fixture path
			if err != nil {
				t.Fatal(err)
			}
			if code, _, stderr := runHook(t, "", "install", flagHarness, tc.harness, flagDir, dir); code != 0 {
				t.Fatalf("second install: exit %d\n%s", code, stderr)
			}
			if second, _ := os.ReadFile(path); string(second) != string(first) { //nolint:gosec // fixture path
				t.Errorf("a second install changed the file:\nfirst:\n%s\nsecond:\n%s", first, second)
			}

			typed, loose := readSettings(t, path)
			for event, matcher := range tc.wantEvents {
				var ours []string
				for _, group := range typed.Hooks[event] {
					for _, hook := range group.Hooks {
						if strings.HasPrefix(hook.Command, hookRunCommand) {
							ours = append(ours, hook.Command)
							if group.Matcher != matcher || len(group.Hooks) != 1 {
								t.Errorf("%s: archfit's group has matcher %q and %d hooks, want %q and 1", event, group.Matcher, len(group.Hooks), matcher)
							}
						}
					}
				}
				if len(ours) != 1 || !strings.HasSuffix(ours[0], " --harness "+tc.harness) {
					t.Errorf("%s: archfit hook run entries = %q, want one for --harness %s", event, ours, tc.harness)
				}
			}
			events := map[string]bool{}
			for event := range tc.wantEvents {
				events[event] = true
			}
			for event := range tc.wantForeign {
				events[event] = true
			}
			if len(typed.Hooks) != len(events) {
				t.Errorf("events = %v, want archfit's and the foreign ones only: %v", reflect.ValueOf(typed.Hooks).MapKeys(), events)
			}
			for event, commands := range tc.wantForeign {
				for _, want := range commands {
					if !hasCommand(typed, event, want) {
						t.Errorf("%s: the foreign hook %q is gone", event, want)
					}
				}
			}
			if strings.Contains(string(first), legacyHookCommand) {
				t.Errorf("the removed %q is still installed:\n%s", legacyHookCommand, first)
			}
			if tc.existing != "" {
				var before map[string]any
				if err := json.Unmarshal([]byte(tc.existing), &before); err != nil {
					t.Fatal(err)
				}
				for _, key := range []string{"model", "permissions"} {
					if !reflect.DeepEqual(loose[key], before[key]) {
						t.Errorf("%s = %v, want it kept as %v", key, loose[key], before[key])
					}
				}
				if !strings.Contains(string(first), `"guard && echo <ok>"`) {
					t.Errorf("a foreign command was re-escaped:\n%s", first)
				}
			}
		})
	}
}

func hasCommand(s settingsFile, event, command string) bool {
	for _, group := range s.Hooks[event] {
		for _, hook := range group.Hooks {
			if hook.Command == command {
				return true
			}
		}
	}
	return false
}

// TestHookInstallRefusesABrokenSettingsFile: a file archfit cannot parse is
// left as it is.
func TestHookInstallRefusesABrokenSettingsFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const broken = `{"hooks": [`
	writeFixtureFile(t, dir, harnessConfigFile(harnessClaude), broken)
	code, _, stderr := runHook(t, "", "install", flagHarness, harnessClaude, flagDir, dir)
	if code != 3 || !strings.Contains(stderr, "settings.json") {
		t.Errorf("exit %d, stderr %q; want 3 naming the file", code, stderr)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, harnessConfigFile(harnessClaude))); string(got) != broken { //nolint:gosec // fixture path
		t.Errorf("the broken file was rewritten: %q", got)
	}
}

// TestHookInstallGit pins the pre-commit hook: written where git runs hooks
// (core.hooksPath), executable, idempotent, and never over a hook archfit did
// not write.
func TestHookInstallGit(t *testing.T) {
	t.Parallel()
	const hooksPath = ".githooks"
	for _, tc := range []struct {
		name     string
		foreign  string
		config   string
		wantCode int
		wantRun  string
	}{
		{name: "a fresh hook", wantRun: "exec archfit hook run pre-commit\n"},
		{name: "a config path is passed on", config: "policy dir/archfit.yaml", wantRun: "exec archfit hook run pre-commit -c 'policy dir/archfit.yaml'\n"},
		{name: "a foreign hook is left alone", foreign: "#!/bin/sh\nmake lint\n", wantCode: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := hookRepo(t, hookModules, hookCleanA, "")
			gitFixture(t, dir, "config", "core.hooksPath", hooksPath)
			hook := filepath.Join(dir, hooksPath, hookEventPreCommit)
			if tc.foreign != "" {
				writeFixtureFile(t, dir, filepath.Join(hooksPath, hookEventPreCommit), tc.foreign)
			}
			args := []string{installSubcmd, flagHarness, harnessGit, flagDir, dir}
			if tc.config != "" {
				args = append(args, "-c", tc.config)
			}
			for range 2 {
				code, stdout, stderr := runHook(t, "", args...)
				if code != tc.wantCode {
					t.Fatalf("exit %d, want %d\nstdout %q\nstderr %q", code, tc.wantCode, stdout, stderr)
				}
				if tc.wantCode == 0 && stdout != "wrote "+hook+"\n" {
					t.Errorf("stdout = %q, want the hook path", stdout)
				}
				if tc.wantCode != 0 && !strings.Contains(stderr, hook) {
					t.Errorf("stderr = %q, want it to name %s", stderr, hook)
				}
			}
			content, err := os.ReadFile(hook) //nolint:gosec // fixture path
			if err != nil {
				t.Fatal(err)
			}
			if tc.foreign != "" {
				if string(content) != tc.foreign {
					t.Errorf("the foreign hook was changed:\n%s", content)
				}
				return
			}
			if !strings.HasSuffix(string(content), tc.wantRun) || !strings.Contains(string(content), gitHookMarker) {
				t.Errorf("hook =\n%s\nwant the marker and %q", content, tc.wantRun)
			}
			info, err := os.Stat(hook)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm()&0o100 == 0 {
				t.Errorf("the hook is not executable: %v", info.Mode())
			}
		})
	}
}

// Install flags the hook and skill tests pass.
const (
	flagDir     = "--dir"
	flagHarness = "--harness"
)

// TestHookInstallWarnsWhenTheHooksCannotFindTheConfig: hook run resolves -c
// against the repository root, so an install from a subproject that leaves -c
// at its default would load nothing. The install says so and names the path.
func TestHookInstallWarnsWhenTheHooksCannotFindTheConfig(t *testing.T) {
	t.Parallel()
	const service = "service"
	subConfig := service + "/" + defaultConfigPath
	for _, tc := range []struct {
		name, dir, config, wantStderr string
		noRootConfig                  bool
	}{
		{name: "the root config"},
		{name: "a subproject config named from the root", dir: service, config: subConfig},
		{name: "a subproject config left at the default beside a root config", dir: service,
			wantStderr: ", not the config in "},
		{name: "a subproject config left at the default", dir: service, noRootConfig: true,
			wantStderr: "the hooks resolve -c against the repository root; pass -c " + subConfig + "\n"},
		{name: "a config that does not exist", config: "nope.yaml",
			wantStderr: "the hooks resolve -c against the repository root, so they cannot load it\n"},
	} {
		for _, harness := range []string{harnessClaude, harnessGit} {
			t.Run(tc.name+"/"+harness, func(t *testing.T) {
				t.Parallel()
				repo := hookRepo(t, hookModules, hookCleanA, "")
				writeFixtureFile(t, repo, subConfig, hookModules)
				if tc.noRootConfig {
					if err := os.Remove(filepath.Join(repo, defaultConfigPath)); err != nil {
						t.Fatal(err)
					}
				}
				args := []string{installSubcmd, flagHarness, harness, flagDir, filepath.Join(repo, tc.dir)}
				if tc.config != "" {
					args = append(args, "-c", tc.config)
				}
				code, _, stderr := runHook(t, "", args...)
				if code != 0 {
					t.Fatalf("exit %d, stderr %q", code, stderr)
				}
				if tc.wantStderr == "" && stderr != "" || !strings.Contains(stderr, tc.wantStderr) {
					t.Errorf("stderr = %q, want %q in it", stderr, tc.wantStderr)
				}
			})
		}
	}
}

// TestHookInstallGitOutsideARepository is an error, not a hook in a guessed place.
func TestHookInstallGitOutsideARepository(t *testing.T) {
	t.Parallel()
	code, _, stderr := runHook(t, "", "install", flagHarness, harnessGit, flagDir, t.TempDir())
	if code != 3 || !strings.Contains(stderr, "not in a git repository") {
		t.Errorf("exit %d, stderr %q; want 3", code, stderr)
	}
}

// TestHookInstallWritesThroughASymlink: a settings file kept as a symlink
// stays a symlink, and its target gets the hooks.
func TestHookInstallWritesThroughASymlink(t *testing.T) {
	t.Parallel()
	dir, dotfiles := t.TempDir(), t.TempDir()
	target := filepath.Join(dotfiles, "settings.json")
	writeFixtureFile(t, dotfiles, "settings.json", `{"model": "opus"}`)
	link := filepath.Join(dir, harnessConfigFile(harnessClaude))
	if err := os.MkdirAll(filepath.Dir(link), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runHook(t, "", "install", flagHarness, harnessClaude, flagDir, dir); code != 0 {
		t.Fatalf("install: exit %d\n%s", code, stderr)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the settings symlink was replaced: %v", err)
	}
	typed, loose := readSettings(t, target)
	if loose["model"] != "opus" || !hasCommand(typed, harnessEventStop, "archfit hook run stop --harness claude") {
		t.Errorf("the link target = %v, want the hooks merged in", loose)
	}
}
