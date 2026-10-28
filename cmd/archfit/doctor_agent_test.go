package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/report"
)

// doctorJSON runs doctor --format json in the working directory and decodes it.
func doctorJSON(t *testing.T) doctorReport {
	t.Helper()
	code, stdout, stderr := runArchfit(t, "doctor", "--format", formatJSON)
	if code != 0 {
		t.Fatalf("doctor --format json: exit %d\n%s", code, stderr)
	}
	var doc doctorReport
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout)
	}
	if doc.SchemaVersion != doctorSchemaVersion || len(doc.Tools) == 0 {
		t.Fatalf("doctor report = %+v, want %s with the tool rows", doc, doctorSchemaVersion)
	}
	return doc
}

// hookStates flattens the agent loop to "harness event" → "status (reason)".
func hookStates(loop agentLoop) map[string]string {
	out := map[string]string{}
	for _, h := range loop.Harnesses {
		for _, hook := range h.Hooks {
			state := hook.Status
			if hook.Reason != "" {
				state += " (" + hook.Reason + ")"
			}
			out[h.Name+" "+hook.Event] = state
		}
	}
	return out
}

// TestDoctorAgentLoop pins the agent loop section: a hook that is not
// installed, or a Codex hook Codex does not trust yet, is inactive; an
// installed one is active, a trusted Codex hook with its hash not verified.
func TestDoctorAgentLoop(t *testing.T) {
	dir := hookRepo(t, lintCleanModules+hookRuleSection, hookCleanA, "")
	gitFixture(t, dir, "config", "core.hooksPath", filepath.Join(".git", "hooks"))
	codexHomeDir := t.TempDir()
	t.Setenv("CODEX_HOME", codexHomeDir)
	t.Chdir(dir)

	notInstalled := hookInactive + " (" + reasonNotInstalled + ")"
	for key, state := range hookStates(doctorJSON(t).AgentLoop) {
		if state != notInstalled {
			t.Errorf("before install: %s is %q, want %q", key, state, notInstalled)
		}
	}

	for _, harness := range []string{harnessClaude, harnessCodex, harnessGit} {
		if code, _, stderr := runHook(t, "", "install", "--harness", harness, "--dir", dir); code != 0 {
			t.Fatalf("install %s: exit %d\n%s", harness, code, stderr)
		}
	}
	untrusted := hookInactive + " (" + reasonNotTrusted + ")"
	trusted := hookActive + " (" + reasonHashNotVerified + ")"
	installed := map[string]string{
		"claude SessionStart": hookActive, "claude PostToolUse": hookActive, "claude Stop": hookActive, "claude SubagentStop": hookActive,
		"codex SessionStart": untrusted, "codex PostToolUse": untrusted, "codex Stop": untrusted,
		"git pre-commit": hookActive,
	}
	assertStates(t, "installed, codex untrusted", hookStates(doctorJSON(t).AgentLoop), installed)

	repo, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	hooksJSON := filepath.Join(repo, harnessConfigFile(harnessCodex))
	var toml strings.Builder
	for _, event := range []string{"session_start", "post_tool_use", "stop"} {
		toml.WriteString("[hooks.state.\"" + hooksJSON + ":" + event + ":0:0\"]\ntrusted_hash = \"sha256:0\"\n\n")
	}
	writeFixtureFile(t, codexHomeDir, "config.toml", toml.String())
	for _, event := range []string{"codex SessionStart", "codex PostToolUse", "codex Stop"} {
		installed[event] = trusted
	}
	doc := doctorJSON(t)
	assertStates(t, "codex trusted", hookStates(doc.AgentLoop), installed)
	if claude := doc.AgentLoop.Harnesses[0]; !claude.Detected || claude.DetectedBy != ".claude/ in the repository" {
		t.Errorf("claude = %+v, want detected by its .claude directory", claude)
	}

	scope := doc.AgentLoop.Scope
	if scope.Target == "" || scope.Unresolved != "" || scope.Error != "" || len(scope.PolicyChanged)+len(scope.PolicyStale) != 0 {
		t.Errorf("scope = %+v, want a resolved target and no policy difference", scope)
	}
}

func assertStates(t *testing.T, when string, got, want map[string]string) {
	t.Helper()
	for key, state := range want {
		if got[key] != state {
			t.Errorf("%s: %s is %q, want %q", when, key, got[key], state)
		}
	}
	if len(got) != len(want) {
		t.Errorf("%s: hooks = %v, want %v", when, got, want)
	}
}

// TestDoctorAgentLoopPolicyGuard: doctor shows the policy guard check would
// apply, here a config the branch changed, and keeps the text report above
// the new section as it was.
func TestDoctorAgentLoopPolicyGuard(t *testing.T) {
	dir := scopeRepo(t, lintCleanModules+hookRuleSection, hookCleanA)
	commitFiles(t, dir, map[string]string{defaultConfigPath: lintCleanModules})
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Chdir(dir)

	scope := doctorJSON(t).AgentLoop.Scope
	if scope.Mode != report.ScopeModeMergeBase || len(scope.PolicyChanged) != 1 || scope.PolicyChanged[0] != defaultConfigPath {
		t.Errorf("scope = %+v, want merge_base with %s changed on the branch", scope, defaultConfigPath)
	}

	code, stdout, stderr := runArchfit(t, "doctor")
	if code != 0 {
		t.Fatalf("doctor: exit %d\n%s", code, stderr)
	}
	order := []string{"TOOL", "\nLLM (off-gate", "\nAgent loop", "policy guard: changed on this branch: " + defaultConfigPath}
	last := -1
	for _, want := range order {
		at := strings.Index(stdout, want)
		if at <= last {
			t.Fatalf("doctor text is missing %q after the previous section:\n%s", want, stdout)
		}
		last = at
	}
}

// TestDoctorJSONRejectsFix: --format json only reports.
func TestDoctorJSONRejectsFix(t *testing.T) {
	t.Parallel()
	code, stdout, stderr := runArchfit(t, "doctor", "--format", formatJSON, "--fix")
	if code != 3 || stdout != "" || !strings.Contains(stderr, "only reports") {
		t.Errorf("exit %d, stdout %q, stderr %q; want 3", code, stdout, stderr)
	}
}
