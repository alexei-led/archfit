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

// forbiddenEdgeRule blocks the fixture's one cross-module edge. It replaces the
// retired scalar coupling gate as the way to make a check fail: schema v2's
// only coupling gate counts newly introduced seams against a comparable
// reference, which is not a thing a single CLI run can produce.
const forbiddenEdgeRule = `rules:
  - id: no-a-to-b
    type: forbidden_dependency
    from: "pkg/a/**"
    to: "pkg/b/**"
    gate: fail
`

func writeFailingCheckRepo(t *testing.T) string {
	t.Helper()
	return writeCoupledRepo(t, coupledModulesCfg+forbiddenEdgeRule)
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

// TestRun_Baseline_RejectsBaseFlag pins the removal of the ineffective
// `baseline --base` mode: the flag never changed what was written, so it is a
// parse error now rather than a silent no-op.
func TestRun_Baseline_RejectsBaseFlag(t *testing.T) {
	t.Parallel()
	cfgPath := writeFailingCheckRepo(t)

	code, stdout, stderr := runArchfit(t, cmdBaseline, "--base", "main", "-c", cfgPath)
	if code != 3 {
		t.Fatalf("baseline --base: exit = %d, want 3\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "--base") {
		t.Errorf("parse error should name the removed flag; stderr:\n%s", stderr)
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

// advisoryCheckDiag is a minimal JSON decoder for validating advisory suppression.
type advisoryCheckDiag struct {
	Findings []struct {
		Kind string `json:"kind"`
	} `json:"findings"`
	AdvisoryTasks []json.RawMessage `json:"advisory_tasks"`
}

func parseAdvisoryDiag(t *testing.T, jsonStr string) advisoryCheckDiag {
	t.Helper()
	var d advisoryCheckDiag
	if err := json.Unmarshal([]byte(jsonStr), &d); err != nil {
		t.Fatalf("parse advisory diag JSON: %v\nraw: %s", err, jsonStr)
	}
	return d
}

func TestRun_Check_NoAdvisoriesSuppressesAdvisoryFindings(t *testing.T) {
	t.Parallel()
	// A coupled repo without a coupling gate produces advisory BC findings.
	cfgPath := writeCoupledRepo(t, coupledModulesCfg)

	// Run with advisories enabled (default).
	_, withAdv, _ := runArchfit(t, cmdCheck, "--json", "-c", cfgPath)
	with := parseAdvisoryDiag(t, withAdv)

	// Run with advisories suppressed.
	_, withoutAdv, _ := runArchfit(t, cmdCheck, "--json", "--no-advisories", "-c", cfgPath)
	without := parseAdvisoryDiag(t, withoutAdv)

	// With advisories, there should be advisory-kind findings or advisory tasks.
	withCount := countAdvisoryFindings(with) + len(with.AdvisoryTasks)
	if withCount == 0 {
		t.Skip("fixture produced no advisory findings; test is not meaningful")
	}

	// Without advisories, neither advisory findings nor advisory tasks should appear.
	if n := countAdvisoryFindings(without); n > 0 {
		t.Errorf("--no-advisories: %d advisory finding(s) still in output", n)
	}
	if n := len(without.AdvisoryTasks); n > 0 {
		t.Errorf("--no-advisories: %d advisory task(s) still in output", n)
	}
}

func countAdvisoryFindings(d advisoryCheckDiag) int {
	n := 0
	for _, f := range d.Findings {
		if f.Kind == "advisory" {
			n++
		}
	}
	return n
}

func TestRun_Check_MinSeverityFiltersAdvisories(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, coupledModulesCfg)

	// Run with all advisories.
	_, allAdv, _ := runArchfit(t, cmdCheck, "--json", "-c", cfgPath)
	all := parseAdvisoryDiag(t, allAdv)

	// Run with only critical-severity advisories (most restrictive filter).
	_, critAdv, _ := runArchfit(t, cmdCheck, "--json", "--min-severity=critical", "-c", cfgPath)
	crit := parseAdvisoryDiag(t, critAdv)

	if n := countAdvisoryFindings(all); n == 0 {
		t.Skip("fixture produced no advisory findings; test is not meaningful")
	}

	// Critical-only run must never have MORE advisory findings than the unfiltered run.
	if critCount := countAdvisoryFindings(crit); critCount > countAdvisoryFindings(all) {
		t.Errorf("--min-severity=critical produced more advisory findings (%d) than unfiltered (%d)",
			critCount, countAdvisoryFindings(all))
	}
	if len(crit.AdvisoryTasks) > len(all.AdvisoryTasks) {
		t.Errorf("--min-severity=critical produced more advisory tasks (%d) than unfiltered (%d)",
			len(crit.AdvisoryTasks), len(all.AdvisoryTasks))
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
