package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/toolrun"
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
	return writeCoupledRepo(t, coupledModulesCfg+`rules:
  - id: no_internal_access
    type: forbidden_dependency
    gate: fail
    from: pkg/a/**
    to: pkg/b/internal/**
`)
}

// TestRun_Analyze_GateVsReportOnly verifies the check-vs-analyze contract:
// the same violating repo exits 1 under 'archfit check' (gate) and 0 under
// 'archfit analyze' (report-only).
func TestRun_Analyze_GateVsReportOnly(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)

	var buf bytes.Buffer
	code := Run([]string{cmdCheck, "-c", cfgPath}, &buf)
	if code != 1 {
		t.Fatalf("check: exit = %d, want 1 (gate violation)\noutput:\n%s", code, buf.String())
	}

	buf.Reset()
	code = Run([]string{cmdAnalyze, "-c", cfgPath}, &buf)
	if code != 0 {
		t.Fatalf("analyze (report-only): exit = %d, want 0\noutput:\n%s", code, buf.String())
	}
	if !strings.Contains(strings.ToLower(buf.String()), "fail") {
		t.Errorf("report-only must still render the fail verdict\noutput:\n%s", buf.String())
	}
}

const (
	flagRefresh       = "--refresh"
	flagRoot          = "--root"
	flagJSON          = "--json"
	flagNoAdvisories  = "--no-advisories"
	goModStub         = "module example.com/test\n\ngo 1.21\n" // minimal go.mod shared by fixture repos
	cmdAnalyze        = "analyze"
	cmdCheck          = "check"
	cmdBaseline       = "baseline"
	cmdConfig         = "config" // config subcommand group (config init / config enrich …)
	cmdEnrich         = "enrich" // config enrich subcommand (config enrich owner / subdomain / …)
	cmdExplain        = "explain"
	fmtJSON           = "--format=json"
	flagVersion       = "--version"
	filePkgAA         = "pkg/a/a.go" // the gate-violating source file used across fixtures
	filePkgBImpl      = "pkg/b/internal/impl/impl.go"
	ruleNoInternalAcc = "no_internal_access" // rule ID in the violating-repo fixture
	explainConstraint = "constraint:"        // explain output field label
	explainRule       = "rule:"              // explain output field label
	explainEdge       = "edge:"              // explain output field label
	goMainSrc         = "package main\n\nfunc main() {}\n"
)

func implSource() string {
	return "package impl\n\nfunc " + "Secret() string { return \"s\" }\n"
}

// writeNonGoRepo creates a git repo with no analyzable source (README only) and
// the given archfit config body, returning the config path.
func writeNonGoRepo(t *testing.T, cfgBody string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"README.md":       "# fixture\n",
		defaultConfigPath: cfgBody,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	gitInitFixtureRepo(t, dir)
	return filepath.Join(dir, defaultConfigPath)
}

// writeGapRepo creates a git repo with no analyzable source and an archfit
// config that forces a go/packages gap via an explicit gate (overrides the
// project-marker suppression). Used to test the warn-loud / --require-tools path.
func writeGapRepo(t *testing.T, extraCfg string) string {
	t.Helper()
	// An explicit languages.go.gate bypasses the "no go.mod → suppress gap" logic
	// in coverage-gap derivation, making the go/packages absence a deterministic gap.
	cfg := "version: 1\nlanguages:\n  go:\n    gate: warn\n" + extraCfg
	return writeNonGoRepo(t, cfg)
}

// TestRun_Check_RequireToolsHardGate verifies the opt-in hard tool-gate (Task 4):
// missing analyzers are warn-loud by default (exit 0 with a gaps block), but
// --require-tools and tools.<x>.gate: fail turn a missing tool into an exit-1
// policy failure — distinct from the exit-3 tool/config error.
func TestRun_Check_RequireToolsHardGate(t *testing.T) {
	t.Parallel()
	type gapsDiag struct {
		Verdict      string `json:"verdict"`
		CoverageGaps []struct {
			Tool string `json:"tool"`
			Gate string `json:"gate"`
		} `json:"coverage_gaps"`
	}

	t.Run("default is warn-loud: exit 0 with a gaps block", func(t *testing.T) {
		t.Parallel()
		cfgPath := writeGapRepo(t, "")
		var buf bytes.Buffer
		code := Run([]string{cmdAnalyze, "-c", cfgPath, flagRefresh, fmtJSON}, &buf)
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
		t.Parallel()
		cfgPath := writeGapRepo(t, "")
		var buf bytes.Buffer
		code := Run([]string{cmdCheck, "-c", cfgPath, flagRefresh, "--require-tools", fmtJSON}, &buf)
		if code != 1 {
			t.Fatalf("check --require-tools: exit = %d, want 1\noutput:\n%s", code, buf.String())
		}
		var d gapsDiag
		if err := json.Unmarshal(buf.Bytes(), &d); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if d.Verdict != string(result.VerdictFail) {
			t.Errorf("verdict = %q, want fail", d.Verdict)
		}
		for _, g := range d.CoverageGaps {
			if g.Gate != gateFail {
				t.Errorf("gap %q gate = %q, want fail under --require-tools", g.Tool, g.Gate)
			}
		}
	})

	t.Run("per-tool gate: tools.go.gate fail on an absent analyzer exits 1", func(t *testing.T) {
		t.Parallel()
		// go is disabled so go/packages reports absent (a gap) deterministically,
		// regardless of whether a Go toolchain happens to half-load a non-Go tree.
		cfg := "version: 1\nlanguages:\n  go:\n    enabled: false\n    gate: fail\n"
		cfgPath := writeNonGoRepo(t, cfg)
		var buf bytes.Buffer
		code := Run([]string{cmdCheck, "-c", cfgPath, flagRefresh, fmtJSON}, &buf)
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

	t.Run("--require-tools stays report-only under analyze", func(t *testing.T) {
		t.Parallel()
		cfgPath := writeGapRepo(t, "")
		var buf bytes.Buffer
		code := Run([]string{cmdAnalyze, "-c", cfgPath, flagRefresh, "--require-tools"}, &buf)
		if code != 0 {
			t.Fatalf("analyze --require-tools: exit = %d, want 0 (report-only)\noutput:\n%s", code, buf.String())
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
	base := t.TempDir()
	repoDir = filepath.Join(base, "repo with spaces")
	if err := os.MkdirAll(repoDir, 0o750); err != nil {
		t.Fatal(err)
	}
	srcFiles := map[string]string{
		markerGoMod: goModStub,
		filePkgAA: "package a\n\nimport \"example.com/test/pkg/b/internal/impl\"\n\n" +
			"func UseSecret() string { return impl.Secret() }\n",
		filePkgBImpl: implSource(),
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
	cfgDir := filepath.Join(base, "config with spaces")
	if err := os.MkdirAll(cfgDir, 0o750); err != nil {
		t.Fatal(err)
	}
	cfgPath = filepath.Join(cfgDir, defaultConfigPath)
	cfgBody := `version: 1
modules:
  a:
    paths: ["pkg/a/**"]
    owner: team-a
  b:
    paths: ["pkg/b/**"]
    internal: ["pkg/b/internal/**"]
    owner: team-b
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
	t.Parallel()
	repoDir, cfgPath := writeRepoWithExternalConfig(t)

	t.Run("--root scans the repo via an external config", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		code := Run([]string{cmdCheck, flagRoot, repoDir, "-c", cfgPath, flagRefresh, fmtJSON}, &buf)
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
			if f.RuleID == ruleNoInternalAcc {
				found = true
			}
		}
		if !found {
			t.Errorf("want the no_internal_access violation from the scanned repo; findings=%+v", diag.Findings)
		}
	})

	t.Run("omitting --root anchors at the config dir (no violations on empty dir)", func(t *testing.T) {
		t.Parallel()
		// Without --root the scan root is the config directory. Since Task 2 made
		// non-git + full mode non-fatal, the run succeeds with exit 0 and finds no
		// violations (the config dir has no source code). This confirms --root is
		// required to scan the actual repo — without it, nothing is analysed.
		var buf bytes.Buffer
		code := Run([]string{cmdAnalyze, "-c", cfgPath, flagRefresh, fmtJSON}, &buf)
		if code != 0 {
			t.Fatalf("check without --root on an external config: exit = %d, want 0 (empty config dir → pass)\noutput:\n%s", code, buf.String())
		}
		var diag struct {
			Findings []struct {
				RuleID string `json:"rule_id"`
			} `json:"findings"`
		}
		if err := json.Unmarshal(buf.Bytes(), &diag); err != nil {
			t.Fatalf("invalid JSON: %v\noutput:\n%s", err, buf.String())
		}
		for _, f := range diag.Findings {
			if f.RuleID == ruleNoInternalAcc {
				t.Errorf("found no_internal_access violation on empty config dir — scan root must be the repo, not the config dir")
			}
		}
	})
}

func TestRun_Version(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	code := Run([]string{flagVersion}, &buf)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	out := buf.String()
	if !strings.Contains(out, "archfit version") {
		t.Errorf("expected version output, got: %q", out)
	}
}

func TestRun_NoArgs(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	// Bare invocation routes to analyze (default command). With no .archfit.yaml
	// in the test working directory (cmd/archfit/), config is now required and
	// the command exits 3 with a helpful next-command hint.
	code := RunWithStderr(nil, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("bare invocation without config: exit = %d, want 3; stdout:\n%s", code, stdout.String())
	}
	if !strings.Contains(stderr.String(), "config init") {
		t.Errorf("bare invocation error should hint at config init; stderr:\n%s", stderr.String())
	}
}

// TestRun_UnknownFlag_NotSilent verifies a bad flag exits 3 with a printed
// error, not a silent exit (manual parser.Parse does not print on its own).
func TestRun_UnknownFlag_NotSilent(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	code := RunWithStderr([]string{cmdAnalyze, "--definitely-not-a-flag"}, &buf, &buf)
	if code != 3 {
		t.Fatalf("unknown flag: exit = %d, want 3", code)
	}
	if strings.TrimSpace(buf.String()) == "" {
		t.Errorf("unknown flag produced no output; want a printed error")
	}
}

func TestRunWithStderr_ErrorsGoToStderrNotStdout(t *testing.T) {
	t.Parallel()
	var out, errOut bytes.Buffer
	code := RunWithStderr([]string{cmdAnalyze, "--definitely-not-a-flag"}, &out, &errOut)
	if code != 3 {
		t.Fatalf("exit = %d, want 3", code)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Errorf("errors must not pollute stdout; got stdout: %q", out.String())
	}
	if strings.TrimSpace(errOut.String()) == "" {
		t.Error("error message should be written to stderr; got empty stderr")
	}
}

func TestRun_Doctor(t *testing.T) {
	t.Parallel()
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

func TestRun_Help_ShowsCommands(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	code := Run([]string{flagHelp}, &buf)
	if code != 0 {
		t.Fatalf("expected exit 0 for --help, got %d", code)
	}
	out := buf.String()
	for _, want := range []string{"Analysis", "Setup & config", "analyze", docsURL, ciDocsURL} {
		if !strings.Contains(out, want) {
			t.Errorf("--help output missing %q; got:\n%s", want, out)
		}
	}
}

func TestRun_CheckHelp_ShowsAgentLoop(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	code := Run([]string{cmdCheck, flagHelp}, &buf)
	if code != 0 {
		t.Fatalf("expected exit 0 for check --help, got %d", code)
	}
	out := buf.String()
	for _, want := range []string{"Run the architecture gate", "archfit check", "--format sarif"} {
		if !strings.Contains(out, want) {
			t.Errorf("check --help output missing %q; got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Languages to analyze") {
		t.Errorf("check --help should not lead with a hard-coded language list; got:\n%s", out)
	}
}

// TestRun_Explain_ResolvesViaFullPipeline verifies explain finds the gate
// finding through the same pipeline as check, with module labels resolved.
func TestRun_Explain_ResolvesViaFullPipeline(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)

	// Get the finding fingerprint from a check run.
	var buf bytes.Buffer
	Run([]string{cmdAnalyze, "-c", cfgPath, flagRefresh, fmtJSON}, &buf)
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
	for _, want := range []string{explainRule, explainEdge, "modules:    a -> b", explainConstraint} {
		if !strings.Contains(out, want) {
			t.Errorf("explain output missing %q\noutput:\n%s", want, out)
		}
	}
}

// TestRun_Explain_HonorsRoot verifies that explain --root scopes the pipeline
// to the given repo when the config lives outside the repo (external-CI shape).
// Without --root the pipeline analyses the empty config dir and finds nothing,
// so explain exits 3 ("no finding with fingerprint prefix").
// This is the regression guard for the c.Root wiring (Task 1 fix).
func TestRun_Explain_HonorsRoot(t *testing.T) {
	t.Parallel()
	repoDir, cfgPath := writeRepoWithExternalConfig(t)

	// Capture the finding ID from check --root so we have a valid fingerprint.
	var checkBuf bytes.Buffer
	code := Run([]string{cmdCheck, flagRoot, repoDir, "-c", cfgPath, flagRefresh, fmtJSON}, &checkBuf)
	if code != 1 {
		t.Fatalf("check --root: exit = %d, want 1 (gate violation)\noutput:\n%s", code, checkBuf.String())
	}
	var checkDiag struct {
		Findings []struct {
			ID string `json:"id"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(checkBuf.Bytes(), &checkDiag); err != nil || len(checkDiag.Findings) == 0 {
		t.Fatalf("no findings from check --root: err=%v output=%s", err, checkBuf.String())
	}
	fp := checkDiag.Findings[0].ID[:8]

	t.Run("with --root resolves the finding", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		code := Run([]string{cmdExplain, fp, flagRoot, repoDir, "-c", cfgPath}, &buf)
		if code != 0 {
			t.Fatalf("explain --root: exit = %d, want 0\noutput:\n%s", code, buf.String())
		}
		out := buf.String()
		for _, want := range []string{explainRule, explainEdge, explainConstraint} {
			if !strings.Contains(out, want) {
				t.Errorf("explain --root output missing %q\noutput:\n%s", want, out)
			}
		}
		if !strings.Contains(out, fp) {
			t.Errorf("explain --root output does not contain fingerprint %q\noutput:\n%s", fp, out)
		}
	})

	t.Run("without --root does not find the fingerprint (pre-patch regression proof)", func(t *testing.T) {
		t.Parallel()
		// Without --root the scan root is the config directory (an empty temp dir).
		// The pipeline finds no findings → explain exits 3 (no matching fingerprint).
		var buf bytes.Buffer
		code := Run([]string{cmdExplain, fp, "-c", cfgPath}, &buf)
		if code != 3 {
			t.Fatalf("explain without --root: exit = %d, want 3 (no finding in empty config dir)\noutput:\n%s", code, buf.String())
		}
	})
}

// TestRun_Explain_BackCompatNoRoot verifies that explain without --root is
// unchanged when the config is co-located with the repo source (the common case).
// This is the back-compat guard: --root is additive and must not break callers
// that do not pass it.
func TestRun_Explain_BackCompatNoRoot(t *testing.T) {
	t.Parallel()
	// writeViolatingRepo puts config inside the repo dir, so omitting --root
	// still analyses the right tree.
	cfgPath := writeViolatingRepo(t)

	var checkBuf bytes.Buffer
	Run([]string{cmdAnalyze, "-c", cfgPath, flagRefresh, fmtJSON}, &checkBuf)
	var checkDiag struct {
		Findings []struct {
			ID string `json:"id"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(checkBuf.Bytes(), &checkDiag); err != nil || len(checkDiag.Findings) == 0 {
		t.Fatalf("no findings from check: err=%v output=%s", err, checkBuf.String())
	}

	var buf bytes.Buffer
	code := Run([]string{cmdExplain, checkDiag.Findings[0].ID[:8], "-c", cfgPath}, &buf)
	if code != 0 {
		t.Fatalf("explain without --root: exit = %d, want 0\noutput:\n%s", code, buf.String())
	}
	out := buf.String()
	for _, want := range []string{explainRule, explainEdge, "modules:    a -> b", explainConstraint} {
		if !strings.Contains(out, want) {
			t.Errorf("explain without --root output missing %q\noutput:\n%s", want, out)
		}
	}
}

// TestRun_Explain_SeverityMatchesCheck verifies that explain reports the same
// severity for a finding as check does. This is the regression guard for the
// P3 fix: before Task 4 check derived Severity from BalanceResult while explain
// rendered cl.Score.Band, so they could diverge on symmetric-strength edges.
func TestRun_Explain_SeverityMatchesCheck(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)

	// Get the finding ID and severity from check.
	var checkBuf bytes.Buffer
	if code := Run([]string{cmdAnalyze, "-c", cfgPath, flagRefresh, fmtJSON}, &checkBuf); code == 3 {
		t.Fatalf("check exited 3 (config error)\noutput:\n%s", checkBuf.String())
	}
	var checkDiag struct {
		Findings []struct {
			ID       string `json:"id"`
			Severity string `json:"severity"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(checkBuf.Bytes(), &checkDiag); err != nil {
		t.Fatalf("unmarshal check output: %v\noutput:\n%s", err, checkBuf.String())
	}
	if len(checkDiag.Findings) == 0 {
		t.Fatalf("writeViolatingRepo produced no findings — forbidden_dependency rule must fire; output:\n%s", checkBuf.String())
	}

	f := checkDiag.Findings[0]
	var explainBuf bytes.Buffer
	if code := Run([]string{cmdExplain, f.ID[:8], "-c", cfgPath}, &explainBuf); code != 0 {
		t.Fatalf("explain exit = %d, want 0\noutput:\n%s", code, explainBuf.String())
	}
	out := explainBuf.String()
	wantLine := "severity:   " + f.Severity
	if !strings.Contains(out, wantLine) {
		t.Errorf("explain severity mismatch:\ncheck reported %q\nexplain output:\n%s", f.Severity, out)
	}
}

// TestRun_Check_AgentTasksPopulated verifies the spec §13 repair block: an
// active gate finding yields one agent task with goal, files, and a
// validation command matching the invocation.
func TestRun_Check_AgentTasksPopulated(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)

	var buf bytes.Buffer
	Run([]string{cmdAnalyze, "-c", cfgPath, flagRefresh, fmtJSON}, &buf)

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
	if task.RuleID != ruleNoInternalAcc {
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

func TestRun_Check_AgentTaskValidationReplaysRootAndQuotesPaths(t *testing.T) {
	t.Parallel()
	repoDir, cfgPath := writeRepoWithExternalConfig(t)

	var buf bytes.Buffer
	Run([]string{cmdAnalyze, flagRoot, repoDir, "-c", cfgPath, flagRefresh, fmtJSON}, &buf)

	var diag struct {
		AgentTasks []struct {
			Validation []string `json:"validation"`
		} `json:"agent_tasks"`
	}
	if err := json.Unmarshal(buf.Bytes(), &diag); err != nil {
		t.Fatalf("invalid JSON: %v\noutput:\n%s", err, buf.String())
	}
	if len(diag.AgentTasks) != 1 || len(diag.AgentTasks[0].Validation) != 1 {
		t.Fatalf("agent_tasks validation = %+v\noutput:\n%s", diag.AgentTasks, buf.String())
	}
	got := diag.AgentTasks[0].Validation[0]
	if !strings.Contains(got, "-c '"+cfgPath+"'") {
		t.Errorf("validation = %q, want quoted external config path", got)
	}
	if !strings.Contains(got, "--root '"+repoDir+"'") {
		t.Errorf("validation = %q, want quoted --root path", got)
	}
}

// TestRun_Check_MissingBaselineFile verifies that when no .archfit-baseline.json
// exists, archfit check exits on the real verdict (exit 1 for violations), not
// a hard error (exit 3). The baseline file is optional; its absence is not an error.
func TestRun_Check_MissingBaselineFile(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)

	var stdout, stderr bytes.Buffer
	cmd := CheckCmd{
		Config: cfgPath,
		Format: []string{formatJSON},
	}
	deps := &appDeps{Runner: toolrun.New(), Stdout: &stdout, Stderr: &stderr}
	err := cmd.Run(deps)

	// Exit code must be the real verdict (1 = gate violation), not 3.
	exitCode := 0
	var ee *exitError
	if errors.As(err, &ee) {
		exitCode = ee.code
	}
	if exitCode != 1 {
		t.Fatalf("exit = %d, want 1 (gate violation drives verdict, missing baseline file is not a hard error)\nstdout:\n%s\nstderr:\n%s",
			exitCode, stdout.String(), stderr.String())
	}
}

// TestRun_Check_LabelsFileDeterministic verifies that check with a pinned
// labels file present produces byte-identical output across runs and that a
// malformed labels file fails loudly (exit 3) rather than silently altering
// the gate.
func TestRun_Check_LabelsFileDeterministic(t *testing.T) {
	t.Parallel()
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
	c1 := Run([]string{cmdAnalyze, "-c", cfgPath, flagRefresh, fmtJSON}, &run1)
	c2 := Run([]string{cmdAnalyze, "-c", cfgPath, flagRefresh, fmtJSON}, &run2)
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
	if code := Run([]string{cmdAnalyze, "-c", cfgPath, flagRefresh, fmtJSON}, &buf); code != 3 {
		t.Errorf("malformed labels file: exit = %d, want 3", code)
	}
}

// TestRun_Check_NoBaselineWarningAbsent verifies (T6 regression guard) that
// analyze must never emit "no baseline found" on stderr.
func TestRun_Check_NoBaselineWarningAbsent(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)

	var stdout, stderr bytes.Buffer
	cmd := CheckCmd{
		Config: cfgPath,
		Format: []string{formatJSON},
		// Base intentionally omitted.
	}
	deps := &appDeps{Runner: toolrun.New(), Stdout: &stdout, Stderr: &stderr}
	var ee *exitError
	if err := cmd.Run(deps); errors.As(err, &ee) && ee.code == 3 {
		t.Fatalf("analyze exited 3 (config/pipeline error), pipeline never ran; stderr: %q", stderr.String())
	}

	if strings.Contains(stderr.String(), "no baseline found") {
		t.Errorf("check without --base must not emit 'no baseline found'; stderr: %q", stderr.String())
	}
}

// TestRun_Check_ScipDisabledCoverageRow verifies T7: when analyzers.scip.enabled is
// absent (defaults off), the pipeline injects a StatusDisabled coverage row for
// "scip" so that tool_coverage in JSON output reports "disabled" rather than
// leaving the entry absent.
func TestRun_Analyze_ScipDisabledCoverageRow(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)

	var buf bytes.Buffer
	// Report-only mode (exit 0) so we can parse the JSON regardless of gate verdict.
	if code := Run([]string{cmdAnalyze, "-c", cfgPath, flagRefresh, fmtJSON}, &buf); code == 3 {
		t.Fatalf("analyze exited 3 (config/pipeline error)\noutput:\n%s", buf.String())
	}

	var out struct {
		ToolCoverage []struct {
			Tool   string `json:"tool"`
			Status string `json:"status"`
		} `json:"tool_coverage"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v\noutput:\n%s", err, buf.String())
	}

	found := false
	for _, c := range out.ToolCoverage {
		if c.Tool == toolScip {
			found = true
			if c.Status != string(result.StatusDisabled) {
				t.Errorf("scip coverage status = %q, want %q", c.Status, result.StatusDisabled)
			}
			break
		}
	}
	if !found {
		t.Errorf("no 'scip' entry in tool_coverage; got %+v", out.ToolCoverage)
	}
}

func TestRun_Analyze_DeployUnitCoverageRowIsDiagnosticOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	files := map[string]string{
		markerGoMod:       "module example.com/deploycov\n\ngo 1.21\n",
		"cmd/api/main.go": goMainSrc,
		defaultConfigPath: `version: 1
modules:
  cmd/api:
    paths: ["cmd/api/**"]
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
	gitInitFixtureRepo(t, dir)

	var buf bytes.Buffer
	cfgPath := filepath.Join(dir, defaultConfigPath)
	if code := Run([]string{cmdAnalyze, "-c", cfgPath, flagRefresh, fmtJSON}, &buf); code == 3 {
		t.Fatalf("analyze exited 3 (config/pipeline error)\noutput:\n%s", buf.String())
	}

	var out struct {
		Metrics []struct {
			Name  string  `json:"name"`
			Value float64 `json:"value"`
		} `json:"metrics"`
		ToolCoverage []struct {
			Tool            string `json:"tool"`
			FilesSeen       int    `json:"files_seen"`
			FilesApplicable int    `json:"files_applicable"`
			Status          string `json:"status"`
		} `json:"tool_coverage"`
		DistanceContext struct {
			DeployUnitDetectedModules int `json:"deploy_unit_detected_modules"`
		} `json:"distance_context"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v\noutput:\n%s", err, buf.String())
	}

	coverageFound := false
	for _, m := range out.Metrics {
		if m.Name == "coverage" {
			coverageFound = true
			if m.Value > 1.0 {
				t.Fatalf("coverage metric = %v, want <= 1.0", m.Value)
			}
		}
	}
	if !coverageFound {
		t.Fatalf("coverage metric not found in output")
	}

	deployCoverageFound := false
	for _, c := range out.ToolCoverage {
		if c.Tool == toolDeployUnit {
			deployCoverageFound = true
			if c.Status != string(result.StatusOK) {
				t.Errorf("deploy-unit coverage status = %q, want %q", c.Status, result.StatusOK)
			}
			if c.FilesSeen == 0 {
				t.Errorf("deploy-unit files_seen = 0, want detected evidence")
			}
			if c.FilesApplicable != 0 {
				t.Errorf("deploy-unit files_applicable = %d, want 0 so coverage ignores it", c.FilesApplicable)
			}
		}
	}
	if !deployCoverageFound {
		t.Fatalf("no deploy-unit entry in tool_coverage: %+v", out.ToolCoverage)
	}
	if out.DistanceContext.DeployUnitDetectedModules == 0 {
		t.Fatalf("distance_context.deploy_unit_detected_modules = 0, want detected evidence")
	}
}

// TestRun_Check_FileClassConfigWiredToPipeline verifies M1: a user-supplied
// file_class: generated_globs pattern in .archfit.yaml reaches the FileClassIndex
// through the pipeline (cfg.ForFileClass() → loc.RunWithConfig). The test
// creates a repo with a custom-named generated file that would NOT be detected
// by built-in heuristics, configures it via file_class: generated_globs, and
// verifies the loc tool_coverage reflects a successful walk (status ok).
// The file_class config will exclude the custom generated file from production
// metrics, so the overall pipeline must not error or miscategorise it.
func TestRun_Check_FileClassConfigWiredToPipeline(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	files := map[string]string{
		markerGoMod: "module example.com/fctest\n\ngo 1.21\n",
		"main.go":   goMainSrc,
		// Custom generated file — NOT matched by built-in filename heuristics.
		// Only config-supplied generated_globs should catch it.
		"codegen/mycodegen_output.go": "package codegen\n\nfunc Generated() {}\n",
		defaultConfigPath: `version: 1
file_class:
  generated_globs:
    - "codegen/**"
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
	gitInitFixtureRepo(t, dir)

	cfgPath := filepath.Join(dir, defaultConfigPath)
	var buf bytes.Buffer
	if code := Run([]string{cmdAnalyze, "-c", cfgPath, flagRefresh, fmtJSON}, &buf); code == 3 {
		t.Fatalf("check exited 3 (pipeline error)\noutput:\n%s", buf.String())
	}

	// Verify loc ran successfully — proves RunWithConfig was called without error.
	var out struct {
		ToolCoverage []struct {
			Tool   string `json:"tool"`
			Status string `json:"status"`
		} `json:"tool_coverage"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v\noutput:\n%s", err, buf.String())
	}
	found := false
	for _, c := range out.ToolCoverage {
		if c.Tool == toolLoc {
			found = true
			if c.Status != string(result.StatusOK) {
				t.Errorf("loc coverage status = %q, want ok", c.Status)
			}
			break
		}
	}
	if !found {
		t.Errorf("no 'loc' entry in tool_coverage; got %+v", out.ToolCoverage)
	}
}
