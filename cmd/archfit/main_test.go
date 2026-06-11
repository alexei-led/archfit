package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeViolatingRepo creates a minimal Go repo with one gate-failing
// dependency (pkg/a imports pkg/b/internal) and an archfit config that
// fails on it. Returns the config path.
func writeViolatingRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/test\n\ngo 1.21\n",
		"pkg/a/a.go": "package a\n\nimport \"example.com/test/pkg/b/internal/impl\"\n\n" +
			"func UseSecret() string { return impl.Secret() }\n",
		"pkg/b/internal/impl/impl.go": "package impl\n\nfunc Secret() string { return \"s\" }\n",
		".archfit.yaml": `version: 1
modules:
  a:
    paths: ["pkg/a/**"]
  b:
    paths: ["pkg/b/**"]
    internal: ["pkg/b/internal/**"]
rules:
  - id: no_internal_access
    type: forbidden_dependency
    gate: fail
    from: pkg/a/**
    to: pkg/b/internal/**
`,
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Scope resolution requires a git repo root.
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("git init failed (git unavailable?): %v\n%s", err, out)
	}
	return filepath.Join(dir, ".archfit.yaml")
}

// TestRun_Check_ReportSuppressesFailureExit verifies the --report contract:
// the same violating repo exits 1 without --report and 0 with it.
func TestRun_Check_ReportSuppressesFailureExit(t *testing.T) {
	cfgPath := writeViolatingRepo(t)

	var buf bytes.Buffer
	code := Run([]string{cmdCheck, "-c", cfgPath, flagFull}, &buf)
	if code != 1 {
		t.Fatalf("check without --report: exit = %d, want 1 (gate violation)\noutput:\n%s", code, buf.String())
	}

	buf.Reset()
	code = Run([]string{cmdCheck, "-c", cfgPath, flagFull, "--report"}, &buf)
	if code != 0 {
		t.Fatalf("check with --report: exit = %d, want 0\noutput:\n%s", code, buf.String())
	}
	if !strings.Contains(strings.ToLower(buf.String()), "fail") {
		t.Errorf("--report must still render the fail verdict\noutput:\n%s", buf.String())
	}
}

const (
	flagFull = "--full"
	cmdCheck = "check"
	fmtJSON  = "--format=json"
)

func TestRun_Version(t *testing.T) {
	var buf bytes.Buffer
	code := Run([]string{"--version"}, &buf)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	out := buf.String()
	if !strings.Contains(out, "archfit version") {
		t.Errorf("expected version output, got: %q", out)
	}
}

func TestRun_NoArgs(t *testing.T) {
	var buf bytes.Buffer
	code := Run([]string{"--help"}, &buf)
	// kong exits 0 on --help
	if code != 0 {
		t.Fatalf("expected exit 0 for --help, got %d (output: %q)", code, buf.String())
	}
}

func TestRun_Doctor(t *testing.T) {
	var buf bytes.Buffer
	code := Run([]string{"doctor"}, &buf)
	// Just verify it doesn't panic/crash (exit code may vary based on available tools)
	t.Logf("doctor exit %d output: %q", code, buf.String())
	_ = code
}

func TestRun_Help_ShowsScan(t *testing.T) {
	var buf bytes.Buffer
	code := Run([]string{"--help"}, &buf)
	if code != 0 {
		t.Fatalf("expected exit 0 for --help, got %d", code)
	}
	out := buf.String()
	if !strings.Contains(out, "scan") {
		t.Errorf("--help output does not mention 'scan' subcommand; got:\n%s", out)
	}
}

// TestRun_Explain_ResolvesViaFullPipeline verifies explain finds the gate
// finding through the same pipeline as check, with module labels resolved.
func TestRun_Explain_ResolvesViaFullPipeline(t *testing.T) {
	cfgPath := writeViolatingRepo(t)

	// Get the finding fingerprint from a check run.
	var buf bytes.Buffer
	Run([]string{cmdCheck, "-c", cfgPath, flagFull, fmtJSON}, &buf)
	var diag struct {
		Findings []struct {
			ID string `json:"id"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(buf.Bytes(), &diag); err != nil || len(diag.Findings) == 0 {
		t.Fatalf("no findings from check: err=%v output=%s", err, buf.String())
	}

	buf.Reset()
	code := Run([]string{"explain", diag.Findings[0].ID[:8], "-c", cfgPath}, &buf)
	if code != 0 {
		t.Fatalf("explain exit = %d, want 0\noutput:\n%s", code, buf.String())
	}
	out := buf.String()
	for _, want := range []string{"rule:", "edge:", "modules:    a -> b", "constraint:"} {
		if !strings.Contains(out, want) {
			t.Errorf("explain output missing %q\noutput:\n%s", want, out)
		}
	}
}

// TestRun_Check_AgentTasksPopulated verifies the spec §13 repair block: an
// active gate finding yields one agent task with goal, files, and a
// validation command matching the invocation.
func TestRun_Check_AgentTasksPopulated(t *testing.T) {
	cfgPath := writeViolatingRepo(t)

	var buf bytes.Buffer
	Run([]string{cmdCheck, "-c", cfgPath, flagFull, fmtJSON}, &buf)

	var diag struct {
		AgentTasks []struct {
			FindingID  string   `json:"finding_id"`
			RuleID     string   `json:"rule_id"`
			Goal       string   `json:"goal"`
			Files      []string `json:"files"`
			Validation []string `json:"validation"`
		} `json:"agent_tasks"`
	}
	if err := json.Unmarshal(buf.Bytes(), &diag); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(diag.AgentTasks) != 1 {
		t.Fatalf("agent_tasks = %d, want 1\noutput:\n%s", len(diag.AgentTasks), buf.String())
	}
	task := diag.AgentTasks[0]
	if task.RuleID != "no_internal_access" {
		t.Errorf("rule_id = %q", task.RuleID)
	}
	if !strings.Contains(task.Goal, "pkg/a/a.go") {
		t.Errorf("goal = %q, want from-path in goal", task.Goal)
	}
	if len(task.Files) == 0 {
		t.Error("files is empty")
	}
	if len(task.Validation) != 1 || !strings.Contains(task.Validation[0], "archfit check -c "+cfgPath) {
		t.Errorf("validation = %v, want exact re-check command", task.Validation)
	}
}
