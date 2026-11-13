package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const flagHarness = "--harness"

func installFixtureHooks(t *testing.T, dir, harness string) {
	t.Helper()
	var err error
	if harness == harnessGit {
		_, err = installGitHook(t.Context(), toolrun.New(), dir)
	} else {
		_, err = installHarnessHooks(dir, harness)
	}
	if err != nil {
		t.Fatalf("install %s fixture hooks: %v", harness, err)
	}
}

func TestInstallHarnessHooksPreservesForeignSettingsAndIsIdempotent(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{harnessClaude, harnessCodex} {
		t.Run(harness, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			file := harnessConfigFile(harness)
			writeFixtureFile(t, dir, file, `{"model":"opus","hooks":{"Stop":[{"hooks":[{"type":"command","command":"notify-done"}]},{"hooks":[{"type":"command","command":"archfit hook claude"}]}]}}`)
			path, err := installHarnessHooks(dir, harness)
			if err != nil {
				t.Fatal(err)
			}
			first := readFile(t, path)
			if strings.Contains(first, legacyHookCommand) || !strings.Contains(first, "notify-done") {
				t.Errorf("hooks: %s", first)
			}
			var settings struct {
				Model string                 `json:"model"`
				Hooks map[string][]hookGroup `json:"hooks"`
			}
			if err := json.Unmarshal([]byte(first), &settings); err != nil {
				t.Fatal(err)
			}
			if settings.Model != "opus" {
				t.Errorf("model = %q, want opus", settings.Model)
			}
			for _, want := range harnessHooks(harness) {
				var count int
				for _, group := range settings.Hooks[want.event] {
					for _, h := range group.Hooks {
						if h.Command == hookRunLine(want.run, harness) {
							count++
							if group.Matcher != want.matcher {
								t.Errorf("%s matcher = %q, want %q", want.event, group.Matcher, want.matcher)
							}
						}
					}
				}
				if count != 1 {
					t.Errorf("%s has %d archfit hooks, want 1", want.event, count)
				}
			}
			if _, err := installHarnessHooks(dir, harness); err != nil {
				t.Fatal(err)
			}
			if got := readFile(t, path); got != first {
				t.Errorf("second install changed settings: %s", got)
			}
		})
	}
}

func TestInstallHarnessHooksRefusesBrokenSettings(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const broken = `{"hooks":[`
	path := filepath.Join(dir, harnessConfigFile(harnessClaude))
	writeFixtureFile(t, dir, harnessConfigFile(harnessClaude), broken)
	if _, err := installHarnessHooks(dir, harnessClaude); err == nil || !strings.Contains(err.Error(), "settings.json") {
		t.Errorf("error = %v, want the broken file", err)
	}
	if got := readFile(t, path); got != broken {
		t.Errorf("settings changed: %s", got)
	}
}

func TestInstallHarnessHooksWritesThroughSymlinks(t *testing.T) {
	t.Parallel()
	dir, dotfiles := t.TempDir(), t.TempDir()
	target := filepath.Join(dotfiles, "settings.json")
	writeFixtureFile(t, dotfiles, "settings.json", `{"model":"opus"}`)
	link := filepath.Join(dir, harnessConfigFile(harnessClaude))
	if err := os.MkdirAll(filepath.Dir(link), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := installHarnessHooks(dir, harnessClaude); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink replaced: %v", err)
	}
	if got := readFile(t, target); !strings.Contains(got, hookRunLine(hookEventStop, harnessClaude)) || !strings.Contains(got, "opus") {
		t.Errorf("target: %s", got)
	}
}

func TestInstallGitHookKeepsForeignHooks(t *testing.T) {
	t.Parallel()
	for _, foreign := range []string{"", "#!/bin/sh\nmake lint\n"} {
		t.Run(foreign, func(t *testing.T) {
			t.Parallel()
			dir := hookRepo(t, hookModules, hookCleanA)
			gitFixture(t, dir, "config", "core.hooksPath", ".githooks")
			path := filepath.Join(dir, ".githooks", hookEventPreCommit)
			if foreign != "" {
				writeFixtureFile(t, dir, ".githooks/"+hookEventPreCommit, foreign)
			}
			for range 2 {
				_, err := installGitHook(t.Context(), toolrun.New(), dir)
				if foreign != "" {
					if err == nil || !strings.Contains(err.Error(), path) {
						t.Errorf("error = %v, want foreign hook path", err)
					}
					if got := readFile(t, path); got != foreign {
						t.Errorf("foreign hook changed: %s", got)
					}
				} else {
					if err != nil {
						t.Fatal(err)
					}
					if got := readFile(t, path); !strings.Contains(got, gitHookMarker) || !strings.HasSuffix(got, "exec archfit hook run pre-commit\n") {
						t.Errorf("hook: %s", got)
					}
					info, err := os.Stat(path)
					if err != nil || info.Mode().Perm()&0o100 == 0 {
						t.Errorf("hook not executable: %v, %v", info, err)
					}
				}
			}
		})
	}
}
