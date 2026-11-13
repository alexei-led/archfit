package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudePluginStatus(t *testing.T) {
	const enabledReason = "enabled and installed; managed settings and session load not verified"
	const installedRecord = `{"plugins":{"archfit@archfit":[{"scope":"user","installPath":"INSTALL"}]}}`
	const enabledSettings = `{"enabledPlugins":{"archfit@archfit":true}}`
	for _, tc := range []struct {
		name, settings, record, want string
		project, local, wantReason   string
		install                      bool
	}{
		{name: "absent", want: "absent"},
		{name: "off", settings: `{"enabledPlugins":{"archfit@archfit":false}}`, want: pluginDisabled},
		{name: "enabled", settings: enabledSettings, record: installedRecord, want: pluginUnknown, install: true, wantReason: enabledReason},
		{name: "project disables", settings: enabledSettings, record: installedRecord, install: true, project: `{"disableAllHooks":true}`, want: pluginDisabled},
		{name: "local overrides project", settings: enabledSettings, record: installedRecord, install: true, project: `{"disableAllHooks":true}`, local: `{"disableAllHooks":false}`, want: pluginUnknown, wantReason: enabledReason},
		{name: "malformed settings", settings: `{`, want: pluginUnknown},
		{name: "malformed record", settings: enabledSettings, record: `{`, want: pluginUnknown},
		{name: "missing record", settings: enabledSettings, want: pluginUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, repo := t.TempDir(), t.TempDir()
			t.Setenv("CLAUDE_CONFIG_DIR", home)
			if tc.settings != "" {
				if err := os.WriteFile(filepath.Join(home, "settings.json"), []byte(tc.settings), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.record != "" {
				install := filepath.Join(home, "installed")
				if tc.install {
					for _, rel := range []string{"hooks/hooks.json", ".claude-plugin/plugin.json"} {
						raw, err := os.ReadFile(filepath.Join("..", "..", "integrations", "claude-code", rel)) //nolint:gosec // repository fixture
						if err != nil {
							t.Fatal(err)
						}
						if err := os.MkdirAll(filepath.Dir(filepath.Join(install, rel)), 0700); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(filepath.Join(install, rel), raw, 0600); err != nil { //nolint:gosec // fixed relative fixture paths
							t.Fatal(err)
						}
					}
				}
				if err := os.MkdirAll(filepath.Join(home, "plugins"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(home, "plugins", "installed_plugins.json"), []byte(strings.ReplaceAll(tc.record, "INSTALL", install)), 0600); err != nil {
					t.Fatal(err)
				}
			}
			for name, contents := range map[string]string{"settings.json": tc.project, "settings.local.json": tc.local} {
				if contents == "" {
					continue
				}
				path := filepath.Join(repo, ".claude", name)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
					t.Fatal(err)
				}
			}
			got := claudePluginStatus(repo).Hooks[0]
			if got.Status != tc.want || (tc.wantReason != "" && got.Reason != tc.wantReason) {
				t.Fatalf("got %+v, want status %s reason %q", got, tc.want, tc.wantReason)
			}
		})
	}
}

func TestClaudePluginHooks(t *testing.T) {
	root := filepath.Join("..", "..")
	var manifest struct {
		Name string `json:"name"`
	}
	raw, err := os.ReadFile(filepath.Join(root, "integrations/claude-code/.claude-plugin/plugin.json")) //nolint:gosec // repository test fixture
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &manifest); err != nil || manifest.Name != "archfit" {
		t.Fatalf("manifest: %+v, %v", manifest, err)
	}
	var cfg struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	raw, err = os.ReadFile(filepath.Join(root, "integrations/claude-code/hooks/hooks.json")) //nolint:gosec // repository test fixture
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	for _, h := range harnessHooks(harnessClaude) {
		t.Run(h.event, func(t *testing.T) {
			groups := cfg.Hooks[h.event]
			if len(groups) != 1 || groups[0].Matcher != h.matcher || len(groups[0].Hooks) != 1 {
				t.Fatalf("invalid event: %+v", groups)
			}
			hook := groups[0].Hooks[0]
			if hook.Type != "command" {
				t.Fatal(hook.Type)
			}
			dir := t.TempDir()
			command := exec.Command("/bin/sh", "-c", hook.Command) //nolint:gosec // execute the checked-in hook under test
			command.Env = []string{"PATH=" + dir}
			command.Stdin = strings.NewReader(`{"test":true}`)
			out, err := command.CombinedOutput()
			if err != nil || !strings.Contains(string(out), "archfit not found") {
				t.Fatalf("missing: %s %v", out, err)
			}
			binary := filepath.Join(dir, "archfit")
			if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\"\n/bin/cat\nexit 2\n"), 0700); err != nil { //nolint:gosec // test executable
				t.Fatal(err)
			}
			command = exec.Command("/bin/sh", "-c", hook.Command) //nolint:gosec // execute the checked-in hook under test
			command.Env = []string{"PATH=" + dir}
			command.Stdin = strings.NewReader(`{"test":true}`)
			out, err = command.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 2 {
				t.Fatalf("exit: %s %v", out, err)
			}
			if !strings.Contains(string(out), hookRunLine(h.run, harnessClaude)[len("archfit "):]) || !strings.Contains(string(out), `{"test":true}`) {
				t.Fatalf("forward: %s", out)
			}
		})
	}
}

func TestDoctorPluginResolvesRepositoryRoot(t *testing.T) {
	repo := hookRepo(t, lintCleanModules+hookRuleSection, hookCleanA)
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	chdirIsolated(t, repo)
	writeFixtureFile(t, repo, ".claude/settings.json", `{"disableAllHooks":true}`)
	if err := os.MkdirAll(filepath.Join(repo, "subdir"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(repo, "subdir"))
	code, stdout, stderr := runArchfit(t, "doctor", "--format", formatJSON)
	if code != 0 {
		t.Fatalf("doctor: %d %s", code, stderr)
	}
	var doc struct {
		ClaudePlugin harnessStatus `json:"claude_plugin"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatal(err)
	}
	if got := doc.ClaudePlugin.Hooks[0]; got.Status != pluginDisabled {
		t.Fatalf("plugin: %+v", got)
	}
}
