package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/config"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// fixRepo is a git repository with no commit and the pre-commit hooks in its
// own .git/hooks, whatever the user's core.hooksPath. files are written into it.
func fixRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	gitInitFixtureRepo(t, dir)
	gitFixture(t, dir, "config", "core.hooksPath", filepath.Join(".git", "hooks"))
	for name, content := range files {
		writeFixtureFile(t, dir, name, content)
	}
	return dir
}

// runDoctorFix runs doctor --fix in dir, isolated from the user's home, with
// no tool on PATH: only the files of the repository and CODEX_HOME detect a
// harness. withCodex makes CODEX_HOME exist.
func runDoctorFix(t *testing.T, dir string, cmd DoctorCmd, withCodex bool) string {
	t.Helper()
	codexHome := chdirIsolated(t, dir)
	if withCodex {
		if err := os.MkdirAll(codexHome, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	runner := &toolrun.RunnerMock{
		DetectFunc: func(context.Context, string) (toolrun.ToolInfo, bool) { return toolrun.ToolInfo{}, false },
		RunFunc:    toolrun.New().Run,
	}
	var out bytes.Buffer
	cmd.Fix = true
	if err := cmd.Run(&appDeps{Runner: runner, Stdout: &out}); err != nil {
		t.Fatalf("doctor --fix: %v\n%s", err, out.String())
	}
	return out.String()
}

// fixSection is the output of doctor --fix after the report.
func fixSection(out string) string {
	_, fix, _ := strings.Cut(out, "\ninstalling hooks:")
	return fix
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // fixture path
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// userClaudeSettings has a setting and a Stop hook that are not archfit's.
const userClaudeSettings = `{"model": "opus", "hooks": {"Stop": [{"hooks": [{"type": "command", "command": "echo bye"}]}]}}`

// TestDoctorFixInstallsTheHooksOfTheDetectedHarnesses: Claude Code is
// detected by .claude/, Codex by CODEX_HOME, git by the repository. A second
// run writes the same bytes, and the user's own settings and hooks stay.
func TestDoctorFixInstallsTheHooksOfTheDetectedHarnesses(t *testing.T) {
	dir := fixRepo(t, map[string]string{harnessConfigFile(harnessClaude): userClaudeSettings, defaultConfigPath: "version: 3\n"})
	out := fixSection(runDoctorFix(t, dir, DoctorCmd{}, true))

	repo, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	claude := filepath.Join(repo, harnessConfigFile(harnessClaude))
	codex := filepath.Join(repo, harnessConfigFile(harnessCodex))
	preCommit := filepath.Join(repo, ".git", "hooks", hookEventPreCommit)
	for _, want := range []string{"wrote " + claude, "wrote " + codex, "wrote " + preCommit, codexTrustNotice} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor --fix output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "warning:") {
		t.Errorf("doctor --fix warns with no ignored settings and no foreign hook:\n%s", out)
	}

	var settings struct {
		Model string                 `json:"model"`
		Hooks map[string][]hookGroup `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(readFile(t, claude)), &settings); err != nil {
		t.Fatal(err)
	}
	if settings.Model != "opus" {
		t.Errorf("the model setting is %q, want opus kept", settings.Model)
	}
	var stopCommands []string
	for _, g := range settings.Hooks["Stop"] {
		for _, h := range g.Hooks {
			stopCommands = append(stopCommands, h.Command)
		}
	}
	if !slices.Contains(stopCommands, "echo bye") || !slices.Contains(stopCommands, hookRunLine(hookEventStop, harnessClaude, "")) {
		t.Errorf("Stop hooks = %v, want the user's and archfit's", stopCommands)
	}

	before := map[string]string{claude: readFile(t, claude), codex: readFile(t, codex), preCommit: readFile(t, preCommit)}
	runDoctorFix(t, dir, DoctorCmd{}, true)
	for path, content := range before {
		if got := readFile(t, path); got != content {
			t.Errorf("a second doctor --fix changed %s:\n%s\nwant\n%s", path, got, content)
		}
	}
}

// TestDoctorFixLeavesAForeignPreCommitHook: a pre-commit hook archfit did not
// write keeps its bytes; doctor says what to add to it and does not fail.
func TestDoctorFixLeavesAForeignPreCommitHook(t *testing.T) {
	const foreign = "#!/bin/sh\nexec pre-commit run\n"
	dir := fixRepo(t, map[string]string{filepath.Join(".git", "hooks", hookEventPreCommit): foreign})
	out := fixSection(runDoctorFix(t, dir, DoctorCmd{}, false))
	if got := readFile(t, filepath.Join(dir, ".git", "hooks", hookEventPreCommit)); got != foreign {
		t.Errorf("the pre-commit hook is now:\n%s", got)
	}
	if !strings.Contains(out, "warning: ") || !strings.Contains(out, hookRunLine(hookEventPreCommit, "", "")) {
		t.Errorf("doctor --fix does not say what to add to the hook:\n%s", out)
	}
	if !strings.Contains(out, defaultConfigPath+" does not exist") {
		t.Errorf("doctor --fix does not say the policy is missing:\n%s", out)
	}
}

// TestDoctorFixWithoutAHarnessWritesNothing: outside a git repository there is
// no harness; doctor says so and writes no file, in the directory or the home.
func TestDoctorFixWithoutAHarnessWritesNothing(t *testing.T) {
	dir := t.TempDir()
	out := fixSection(runDoctorFix(t, dir, DoctorCmd{}, false))
	if !strings.Contains(out, noHarness) {
		t.Errorf("doctor --fix does not say it found no harness:\n%s", out)
	}
	for _, root := range []string{dir, os.Getenv("HOME")} {
		if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
			t.Errorf("%s holds %v after doctor --fix (err %v), want nothing", root, entries, err)
		}
	}
}

// TestDoctorWarnsWhenGitIgnoresTheSettings: a .gitignore that ignores .claude/
// keeps the hooks out of every other checkout; doctor and doctor --fix say so.
// A harness that is not here gets no warning, and a settings file the
// repository tracks despite the rule reaches every clone, so it gets none.
func TestDoctorWarnsWhenGitIgnoresTheSettings(t *testing.T) {
	const warning = "warning: git ignores "
	for _, tc := range []struct {
		name    string
		tracked bool
		want    bool
	}{
		{name: "an ignored settings file", want: true},
		{name: "a tracked settings file", tracked: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := fixRepo(t, map[string]string{".gitignore": ".claude/\n.codex/\n", harnessConfigFile(harnessClaude): "{}"})
			if tc.tracked {
				gitFixture(t, dir, "add", "-f", harnessConfigFile(harnessClaude))
			}
			out := runDoctorFix(t, dir, DoctorCmd{}, false)
			repo, err := filepath.EvalSymlinks(dir)
			if err != nil {
				t.Fatal(err)
			}
			report, fix, _ := strings.Cut(out, "\ninstalling hooks:")
			claude := warning + filepath.Join(repo, harnessConfigFile(harnessClaude))
			for name, text := range map[string]string{"report": report, "fix": fix} {
				if got := strings.Contains(text, claude); got != tc.want {
					t.Errorf("the %s part warns about the Claude Code settings: %v, want %v:\n%s", name, got, tc.want, text)
				}
			}
			if codex := warning + filepath.Join(repo, harnessConfigFile(harnessCodex)); strings.Contains(out, codex) {
				t.Errorf("doctor warns about the settings of Codex, which is not here:\n%s", out)
			}
		})
	}
}

// TestDoctorFixKeepsOutOfASharedHooksDirectory: a core.hooksPath outside the
// repository (a global hooks directory) runs its hooks in every repository,
// so doctor --fix writes no pre-commit hook there and says what to add.
// A relative core.hooksPath under a symlink that leaves the repository is
// outside it too, also before the hooks directory exists.
func TestDoctorFixKeepsOutOfASharedHooksDirectory(t *testing.T) {
	for _, tc := range []struct {
		name      string
		hooksPath func(t *testing.T, repo, shared string) string
	}{
		{name: "an absolute hooks path", hooksPath: func(_ *testing.T, _, shared string) string { return shared }},
		{name: "a relative hooks path under a symlink", hooksPath: func(t *testing.T, repo, shared string) string {
			if err := os.Symlink(shared, filepath.Join(repo, ".shared")); err != nil {
				t.Fatal(err)
			}
			return filepath.Join(".shared", "hooks")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := fixRepo(t, nil)
			shared := t.TempDir()
			gitFixture(t, dir, "config", "core.hooksPath", tc.hooksPath(t, dir, shared))
			out := fixSection(runDoctorFix(t, dir, DoctorCmd{}, false))
			if entries, err := os.ReadDir(shared); err != nil || len(entries) != 0 {
				t.Errorf("the shared hooks directory holds %v (err %v), want nothing", entries, err)
			}
			if !strings.Contains(out, "outside it") || !strings.Contains(out, hookRunLine(hookEventPreCommit, "", "")) {
				t.Errorf("doctor --fix does not say why it wrote no pre-commit hook:\n%s", out)
			}
		})
	}
}

// TestDoctorFixInstallsHooksWhenAToolchainInstallFails: the hooks do not wait
// for a toolchain install, which can fail for reasons of its own.
func TestDoctorFixInstallsHooksWhenAToolchainInstallFails(t *testing.T) {
	dir := fixRepo(t, map[string]string{harnessConfigFile(harnessClaude): "{}"})
	chdirIsolated(t, dir)
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, name string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, name == "brew"
		},
		RunFunc: func(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if cmd.Name == "brew" {
				return toolrun.Output{ExitCode: 1, Stderr: []byte("brew is offline")}, nil
			}
			return toolrun.New().Run(ctx, cmd)
		},
	}
	var out bytes.Buffer
	err := (&DoctorCmd{Fix: true}).Run(&appDeps{Runner: runner, Stdout: &out})
	if err == nil || !strings.Contains(err.Error(), "brew is offline") {
		t.Errorf("doctor --fix error = %v, want the failed toolchain install", err)
	}
	if !strings.Contains(readFile(t, filepath.Join(dir, harnessConfigFile(harnessClaude))), hookRunCommand) {
		t.Errorf("doctor --fix wrote no Claude Code hook after the failed install:\n%s", out.String())
	}
}

// TestDoctorFixReadsANullSettingsFile: a settings file that holds JSON null
// is an empty one.
func TestDoctorFixReadsANullSettingsFile(t *testing.T) {
	dir := fixRepo(t, map[string]string{harnessConfigFile(harnessClaude): "null"})
	runDoctorFix(t, dir, DoctorCmd{}, false)
	if !strings.Contains(readFile(t, filepath.Join(dir, harnessConfigFile(harnessClaude))), hookRunCommand) {
		t.Error("doctor --fix wrote no hook into a null settings file")
	}
}

// TestDoctorFixDryRunWritesNothing: --dry-run names the files and leaves the
// repository as it was.
func TestDoctorFixDryRunWritesNothing(t *testing.T) {
	dir := fixRepo(t, map[string]string{harnessConfigFile(harnessClaude): userClaudeSettings})
	out := fixSection(runDoctorFix(t, dir, DoctorCmd{DryRun: true}, false))
	if !strings.Contains(out, "would write ") || strings.Contains(out, "wrote ") {
		t.Errorf("dry run output:\n%s", out)
	}
	if got := readFile(t, filepath.Join(dir, harnessConfigFile(harnessClaude))); got != userClaudeSettings {
		t.Errorf("the dry run changed the Claude Code settings:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "hooks", hookEventPreCommit)); err == nil {
		t.Error("the dry run wrote the pre-commit hook")
	}
}

// TestDoctorJSONToolGroups: each tool row names its group, and the check
// group lists git for every language and ast-grep for the languages it reads.
func TestDoctorJSONToolGroups(t *testing.T) {
	chdirIsolated(t, t.TempDir())
	rows := map[string]toolRow{}
	for _, r := range doctorJSON(t).Tools {
		rows[r.Name] = r
	}
	for name, want := range map[string]toolRow{
		"git":      {Group: toolGroupCheck},
		"ast-grep": {Group: toolGroupCheck, Languages: []string{config.LangTypeScript, config.LangPython, config.LangRust, config.LangDotNet}},
		"go":       {Group: toolGroupReport, Languages: []string{config.LangGo}},
		"jscpd":    {Group: toolGroupReport},
	} {
		got := rows[name]
		if got.Group != want.Group || !slices.Equal(got.Languages, want.Languages) {
			t.Errorf("%s = group %q languages %v, want %q %v", name, got.Group, got.Languages, want.Group, want.Languages)
		}
	}
}
