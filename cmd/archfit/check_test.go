package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeCheckFixtureRepo(t *testing.T, fixtureDir string) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	fixtureRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "testdata", fixtureDir)

	root := t.TempDir()
	if err := copyFixtureIntoDir(fixtureRoot, root); err != nil {
		t.Fatalf("copy fixture %q: %v", fixtureDir, err)
	}
	gitInitFixtureRepo(t, root)
	return filepath.Join(root, defaultConfigPath)
}

func runArchfit(t *testing.T, args ...string) (int, string, string) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	code := RunWithStderr(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestRun_Check_ExitCodeZeroOnCleanConfig(t *testing.T) {
	t.Parallel()
	cfgPath := writeCheckFixtureRepo(t, "golang")

	code, stdout, stderr := runArchfit(t, cmdCheck, "-c", cfgPath)
	if code != 0 {
		t.Fatalf("check clean config: exit = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}

func writeFailingCheckRepo(t *testing.T) string {
	t.Helper()
	return writeCoupledRepo(t, coupledModulesCfg+"coupling:\n  gate:\n    min_band: strong\n")
}

func TestRun_Check_ExitCodeOneOnViolatedPolicy(t *testing.T) {
	t.Parallel()
	cfgPath := writeFailingCheckRepo(t)

	code, stdout, stderr := runArchfit(t, cmdCheck, "-c", cfgPath)
	if code != 1 {
		t.Fatalf("check violated config: exit = %d, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}

func TestRun_Check_ExitCodeThreeOnBadConfigPath(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "missing.yaml")

	code, stdout, stderr := runArchfit(t, cmdCheck, "-c", missing)
	if code != 3 {
		t.Fatalf("check bad config path: exit = %d, want 3\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}

func TestRun_Analyze_IsReportOnlyOnViolatedPolicy(t *testing.T) {
	t.Parallel()
	cfgPath := writeFailingCheckRepo(t)

	code, stdout, stderr := runArchfit(t, cmdAnalyze, "-c", cfgPath)
	if code != 0 {
		t.Fatalf("analyze violated config: exit = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}

func TestRun_Analyze_RejectsGateFlag(t *testing.T) {
	t.Parallel()
	cfgPath := writeFailingCheckRepo(t)

	code, stdout, stderr := runArchfit(t, cmdAnalyze, "--gate", "-c", cfgPath)
	if code != 3 {
		t.Fatalf("analyze --gate: exit = %d, want 3\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if strings.TrimSpace(stderr) == "" {
		t.Fatalf("analyze --gate produced no parse error\nstdout:\n%s", stdout)
	}
}

func TestRun_Check_RejectsFullFlag(t *testing.T) {
	t.Parallel()
	cfgPath := writeFailingCheckRepo(t)

	code, stdout, stderr := runArchfit(t, cmdCheck, "--full", "-c", cfgPath)
	if code != 3 {
		t.Fatalf("check --full: exit = %d, want 3\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if strings.TrimSpace(stderr) == "" {
		t.Fatalf("check --full produced no parse error\nstdout:\n%s", stdout)
	}
}

func TestRun_Check_RejectsAISummaryFlag(t *testing.T) {
	t.Parallel()
	cfgPath := writeFailingCheckRepo(t)

	code, stdout, stderr := runArchfit(t, cmdCheck, "--ai-summary", "-c", cfgPath)
	if code != 3 {
		t.Fatalf("check --ai-summary: exit = %d, want 3\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if strings.TrimSpace(stderr) == "" {
		t.Fatalf("check --ai-summary produced no parse error\nstdout:\n%s", stdout)
	}
}

func TestRun_Check_AcceptsNoAdvisoriesFlag(t *testing.T) {
	t.Parallel()
	cfgPath := writeFailingCheckRepo(t)

	code, stdout, stderr := runArchfit(t, cmdCheck, "--no-advisories", "-c", cfgPath)
	if code == 3 {
		t.Fatalf("check --no-advisories: exit = %d, want 0 or 1\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}

func TestRun_Check_AcceptsMinSeverityFlag(t *testing.T) {
	t.Parallel()
	cfgPath := writeFailingCheckRepo(t)

	code, stdout, stderr := runArchfit(t, cmdCheck, "--min-severity=high", "-c", cfgPath)
	if code == 3 {
		t.Fatalf("check --min-severity: exit = %d, want 0 or 1\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}

func TestRun_Check_JSONOutputIsValid(t *testing.T) {
	t.Parallel()
	cfgPath := writeFailingCheckRepo(t)

	code, stdout, stderr := runArchfit(t, cmdCheck, "--json", "-c", cfgPath)
	if code != 0 && code != 1 {
		t.Fatalf("check --json: exit = %d, want 0 or 1\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	var got any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("check --json produced invalid JSON: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
}
