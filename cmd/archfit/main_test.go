package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
)

func gitInitFixtureRepo(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	cmd.Env = scrubGitFixtureEnv(os.Environ())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("git init failed (git unavailable?): %v\n%s", err, out)
	}
}

func scrubGitFixtureEnv(env []string) []string {
	blocked := map[string]bool{
		"GIT_DIR":        true,
		"GIT_WORK_TREE":  true,
		"GIT_COMMON_DIR": true,
		"GIT_PREFIX":     true,
	}

	filtered := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, found := strings.Cut(entry, "=")
		if found && blocked[name] {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

// writeViolatingRepo creates a minimal Go repo with one gate-failing
// dependency (pkg/a imports pkg/b/internal) and an archfit config that
// fails on it. Returns the config path.
func writeViolatingRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/test\n\ngo 1.21\n",
		filePkgAA: "package a\n\nimport \"example.com/test/pkg/b/internal/impl\"\n\n" +
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
	gitInitFixtureRepo(t, dir)
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
	code = Run([]string{cmdCheck, "-c", cfgPath, flagFull, flagReport}, &buf)
	if code != 0 {
		t.Fatalf("check with --report: exit = %d, want 0\noutput:\n%s", code, buf.String())
	}
	if !strings.Contains(strings.ToLower(buf.String()), "fail") {
		t.Errorf("--report must still render the fail verdict\noutput:\n%s", buf.String())
	}
}

const (
	flagFull   = "--full"
	cmdCheck   = "check"
	cmdExplain = "explain"
	fmtJSON    = "--format=json"
	flagReport = "--report"
	filePkgAA  = "pkg/a/a.go" // the gate-violating source file used across fixtures
)

// writeNonGoRepo creates a git repo with no analyzable source (README only) and
// the given archfit config body, returning the config path. The optional analyzers
// (dependency-cruiser, grimp, lizard, jscpd, gitnexus) are absent on such a tree,
// so every run yields a stable, non-empty CoverageGaps block — the input the
// opt-in hard tool-gate acts on.
func writeNonGoRepo(t *testing.T, cfgBody string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"README.md":     "# fixture\n",
		".archfit.yaml": cfgBody,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	gitInitFixtureRepo(t, dir)
	return filepath.Join(dir, ".archfit.yaml")
}

// TestRun_Check_RequireToolsHardGate verifies the opt-in hard tool-gate (Task 4):
// missing analyzers are warn-loud by default (exit 0 with a gaps block), but
// --require-tools and tools.<x>.gate: fail turn a missing tool into an exit-1
// policy failure — distinct from the exit-3 tool/config error.
func TestRun_Check_RequireToolsHardGate(t *testing.T) {
	type gapsDiag struct {
		Verdict      string `json:"verdict"`
		CoverageGaps []struct {
			Tool string `json:"tool"`
			Gate string `json:"gate"`
		} `json:"coverage_gaps"`
	}

	t.Run("default is warn-loud: exit 0 with a gaps block", func(t *testing.T) {
		cfgPath := writeNonGoRepo(t, "version: 1\n")
		var buf bytes.Buffer
		code := Run([]string{cmdCheck, "-c", cfgPath, flagFull, fmtJSON}, &buf)
		if code != 0 {
			t.Fatalf("default check: exit = %d, want 0\noutput:\n%s", code, buf.String())
		}
		var d gapsDiag
		if err := json.Unmarshal(buf.Bytes(), &d); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if len(d.CoverageGaps) == 0 {
			t.Fatalf("want a non-empty coverage_gaps block on a tool-less repo\noutput:\n%s", buf.String())
		}
		for _, g := range d.CoverageGaps {
			if g.Gate != gateWarn {
				t.Errorf("default gap %q gate = %q, want warn", g.Tool, g.Gate)
			}
		}
	})

	t.Run("--require-tools fails: every gap becomes a fail gate, exit 1", func(t *testing.T) {
		cfgPath := writeNonGoRepo(t, "version: 1\n")
		var buf bytes.Buffer
		code := Run([]string{cmdCheck, "-c", cfgPath, flagFull, "--require-tools", fmtJSON}, &buf)
		if code != 1 {
			t.Fatalf("check --require-tools: exit = %d, want 1\noutput:\n%s", code, buf.String())
		}
		var d gapsDiag
		if err := json.Unmarshal(buf.Bytes(), &d); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if d.Verdict != string(diagnostic.VerdictFail) {
			t.Errorf("verdict = %q, want fail", d.Verdict)
		}
		for _, g := range d.CoverageGaps {
			if g.Gate != gateFail {
				t.Errorf("gap %q gate = %q, want fail under --require-tools", g.Tool, g.Gate)
			}
		}
	})

	t.Run("per-tool gate: tools.go.gate fail on an absent analyzer exits 1", func(t *testing.T) {
		// go is disabled so go/packages reports absent (a gap) deterministically,
		// regardless of whether a Go toolchain happens to half-load a non-Go tree.
		cfg := "version: 1\ntools:\n  go:\n    enabled: off\n    gate: fail\n"
		cfgPath := writeNonGoRepo(t, cfg)
		var buf bytes.Buffer
		code := Run([]string{cmdCheck, "-c", cfgPath, flagFull, fmtJSON}, &buf)
		if code != 1 {
			t.Fatalf("tools.go.gate: fail → exit = %d, want 1\noutput:\n%s", code, buf.String())
		}
		var d gapsDiag
		if err := json.Unmarshal(buf.Bytes(), &d); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		var goGate string
		for _, g := range d.CoverageGaps {
			if g.Tool == toolGoPackages {
				goGate = g.Gate
			} else if g.Gate != gateWarn {
				t.Errorf("non-go gap %q gate = %q, want warn (per-tool gate is scoped)", g.Tool, g.Gate)
			}
		}
		if goGate != gateFail {
			t.Errorf("go/packages gate = %q, want fail", goGate)
		}
	})

	t.Run("--require-tools is not suppressed by --report", func(t *testing.T) {
		cfgPath := writeNonGoRepo(t, "version: 1\n")
		var buf bytes.Buffer
		code := Run([]string{cmdCheck, "-c", cfgPath, flagFull, flagReport, "--require-tools"}, &buf)
		if code != 1 {
			t.Fatalf("check --report --require-tools: exit = %d, want 1 (hard gate beats --report)\noutput:\n%s", code, buf.String())
		}
	})
}

// writeRepoWithExternalConfig creates a git Go repo with one gate-failing
// dependency (pkg/a → pkg/b/internal) and writes the archfit config that fails
// on it into a SEPARATE directory outside the repo. It returns (repoDir,
// cfgPath). This is the external-CI shape: the config lives nowhere near the
// analyzed tree, so only --root can point archfit at the repo.
func writeRepoWithExternalConfig(t *testing.T) (repoDir, cfgPath string) {
	t.Helper()
	repoDir = t.TempDir()
	srcFiles := map[string]string{
		"go.mod": "module example.com/test\n\ngo 1.21\n",
		filePkgAA: "package a\n\nimport \"example.com/test/pkg/b/internal/impl\"\n\n" +
			"func UseSecret() string { return impl.Secret() }\n",
		"pkg/b/internal/impl/impl.go": "package impl\n\nfunc Secret() string { return \"s\" }\n",
	}
	for name, content := range srcFiles {
		path := filepath.Join(repoDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	gitInitFixtureRepo(t, repoDir)

	// Config in its own directory, outside the repo. Module path globs are
	// repo-relative, so they resolve against the --root scan tree, not here.
	cfgDir := t.TempDir()
	cfgPath = filepath.Join(cfgDir, ".archfit.yaml")
	cfgBody := `version: 1
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
`
	if err := os.WriteFile(cfgPath, []byte(cfgBody), 0o600); err != nil {
		t.Fatal(err)
	}
	return repoDir, cfgPath
}

// TestRun_Check_RootDecoupledFromConfig verifies Task 6b: --root scans an
// arbitrary repo using a config that lives outside it, while omitting --root
// keeps the historical config-dir-as-root behaviour.
func TestRun_Check_RootDecoupledFromConfig(t *testing.T) {
	repoDir, cfgPath := writeRepoWithExternalConfig(t)

	t.Run("--root scans the repo via an external config", func(t *testing.T) {
		var buf bytes.Buffer
		code := Run([]string{cmdCheck, "--root", repoDir, "-c", cfgPath, flagFull, fmtJSON}, &buf)
		if code != 1 {
			t.Fatalf("check --root: exit = %d, want 1 (forbidden-dependency gate)\noutput:\n%s", code, buf.String())
		}
		var diag struct {
			Findings []struct {
				RuleID string `json:"rule_id"`
			} `json:"findings"`
		}
		if err := json.Unmarshal(buf.Bytes(), &diag); err != nil {
			t.Fatalf("invalid JSON: %v\noutput:\n%s", err, buf.String())
		}
		var found bool
		for _, f := range diag.Findings {
			if f.RuleID == "no_internal_access" {
				found = true
			}
		}
		if !found {
			t.Errorf("want the no_internal_access violation from the scanned repo; findings=%+v", diag.Findings)
		}
	})

	t.Run("omitting --root anchors at the config dir (unchanged default)", func(t *testing.T) {
		// Without --root the scan root is the config directory, which is not a git
		// repo here — scope resolution fails (exit 3), exactly as before this flag
		// existed. This proves --root is the only behavioural change.
		var buf bytes.Buffer
		code := Run([]string{cmdCheck, "-c", cfgPath, flagFull, fmtJSON}, &buf)
		if code != 3 {
			t.Fatalf("check without --root on an external config: exit = %d, want 3 (config dir is not a git repo)\noutput:\n%s", code, buf.String())
		}
	})
}

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

// TestRun_UnknownFlag_NotSilent verifies a bad flag exits 3 with a printed
// error, not a silent exit (manual parser.Parse does not print on its own).
func TestRun_UnknownFlag_NotSilent(t *testing.T) {
	var buf bytes.Buffer
	code := Run([]string{cmdCheck, "--definitely-not-a-flag"}, &buf)
	if code != 3 {
		t.Fatalf("unknown flag: exit = %d, want 3", code)
	}
	if strings.TrimSpace(buf.String()) == "" {
		t.Errorf("unknown flag produced no output; want a printed error")
	}
}

func TestRun_Doctor(t *testing.T) {
	var buf bytes.Buffer
	code := Run([]string{"doctor"}, &buf)
	// Exit code varies with which tools are installed, but doctor always renders
	// a report and never crashes (a panic would surface as a non-0/1 code via the
	// recover in Run, or a t-level failure).
	if code != 0 && code != 1 {
		t.Errorf("doctor exit = %d, want 0 (all present) or 1 (some missing)", code)
	}
	if strings.TrimSpace(buf.String()) == "" {
		t.Errorf("doctor produced no output; want a tool report")
	}
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
	code := Run([]string{cmdExplain, diag.Findings[0].ID[:8], "-c", cfgPath}, &buf)
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

// TestRun_Check_LabelsFileDeterministic verifies that check with a pinned
// labels file present produces byte-identical output across runs and that a
// malformed labels file fails loudly (exit 3) rather than silently altering
// the gate.
func TestRun_Check_LabelsFileDeterministic(t *testing.T) {
	cfgPath := writeViolatingRepo(t)
	dir := filepath.Dir(cfgPath)

	labelsYAML := `version: 1
labels:
  - from: a
    to: b
    strength: model
    rationale: "b types cross the boundary"
    status: approved
`
	if err := os.WriteFile(filepath.Join(dir, ".archfit-labels.yaml"), []byte(labelsYAML), 0o600); err != nil {
		t.Fatal(err)
	}

	var run1, run2 bytes.Buffer
	c1 := Run([]string{cmdCheck, "-c", cfgPath, flagFull, flagReport, fmtJSON}, &run1)
	c2 := Run([]string{cmdCheck, "-c", cfgPath, flagFull, flagReport, fmtJSON}, &run2)
	if c1 != 0 || c2 != 0 {
		t.Fatalf("exits = %d/%d, want 0/0", c1, c2)
	}
	if !bytes.Equal(run1.Bytes(), run2.Bytes()) {
		t.Error("check with labels file is not byte-identical across runs")
	}

	// Malformed labels file → loud failure.
	if err := os.WriteFile(filepath.Join(dir, ".archfit-labels.yaml"), []byte("labels:\n  - {from: a, to: b, strength: huge, status: approved}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if code := Run([]string{cmdCheck, "-c", cfgPath, flagFull, flagReport, fmtJSON}, &buf); code != 3 {
		t.Errorf("malformed labels file: exit = %d, want 3", code)
	}
}
