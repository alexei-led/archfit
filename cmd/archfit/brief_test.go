package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/output/brief"
)

const nextActionRepair = "repair"

// TestFormatMatrix_BriefActsOnTheState pins the text and Markdown brief on the
// violating fixture: the blocker carries its file:line, goal, and check
// command; the first next step agrees with the agent digest's next_action
// (repair: fix the blocker); and every NOT MEASURED fact names the step that
// closes it or is marked out of claim.
func TestFormatMatrix_BriefActsOnTheState(t *testing.T) {
	t.Parallel()
	requireHealthyExtraction(t)
	cfgPath := writeViolatingRepo(t)

	_, raw, _ := runArchfit(t, cmdCheck, "-c", cfgPath, "--progress=none", "--format="+formatAgent)
	if got := decodeAgentResult(t, raw).NextAction; got != nextActionRepair {
		t.Fatalf("next_action = %q, want repair: the fixture is the repair case", got)
	}

	text := string(runFormatOutput(t, cfgPath, formatText))
	for _, want := range []string{"BLOCKERS (1)", filePkgAA + ":", "    goal:  ", "    check: archfit check -c ", "NEXT STEPS\n\n  1. Fix blocker "} {
		if !strings.Contains(text, want) {
			t.Errorf("text brief missing %q:\n%s", want, text)
		}
	}
	unknowns := strings.SplitN(strings.SplitN(text, "NOT MEASURED (", 2)[1], "\n\n", 3)[1]
	lines := strings.Split(strings.TrimRight(unknowns, "\n"), "\n")
	if len(lines)%3 != 0 || len(lines) == 0 {
		t.Fatalf("NOT MEASURED entries are not fact/reason/step triples:\n%s", unknowns)
	}
	for i := 2; i < len(lines); i += 3 {
		step := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(step, "→ ") && step != brief.OutOfClaim {
			t.Errorf("NOT MEASURED %q has no step: %q", strings.TrimSpace(lines[i-2]), step)
		}
	}

	md := string(runFormatOutput(t, cfgPath, formatMarkdown))
	section := strings.SplitN(strings.SplitN(md, "## Not measured (", 2)[1], "\n\n", 3)[1]
	for _, line := range strings.Split(strings.SplitN(section, "\n\n", 2)[0], "\n") {
		if !strings.Contains(line, " → ") && !strings.HasSuffix(line, brief.OutOfClaim) {
			t.Errorf("Markdown NOT MEASURED line has no step: %q", line)
		}
	}
	if !strings.Contains(md, "## Next steps\n\n1. Fix blocker ") || strings.Count(md, "\n# ") != 0 || !strings.HasPrefix(md, "# ") {
		t.Errorf("Markdown brief must open with the one H1 and lead its next steps with the blocker:\n%s", md)
	}
}

// TestRun_Check_BriefBaselineStep pins the gate-reference step end to end: a
// clean tree with no stored reference is offered `archfit baseline`; once a
// baseline exists and the config changes, the reference no longer compares and
// the brief asks for a review instead of a blanket re-baseline.
func TestRun_Check_BriefBaselineStep(t *testing.T) {
	t.Parallel()
	const (
		record = "Record a gate reference once the findings are reviewed: archfit baseline"
		review = "Review why the gate reference does not compare"
	)
	dir := hookRepo(t, hookModules, hookCleanA, "")
	cfg := filepath.Join(dir, defaultConfigPath)
	check := func() string {
		var buf bytes.Buffer
		if code := Run([]string{cmdCheck, "-c", cfg, flagRefresh}, &buf); code == 1 || code == 3 {
			t.Fatalf("check exit = %d\n%s", code, buf.String())
		}
		return buf.String()
	}
	if out := check(); !strings.Contains(out, record) || strings.Contains(out, review) {
		t.Fatalf("no stored reference: want the baseline step\n%s", out)
	}
	var buf bytes.Buffer
	if code := Run([]string{cmdBaseline, "-c", cfg, flagRefresh}, &buf); code != 0 {
		t.Fatalf("baseline exit = %d\n%s", code, buf.String())
	}
	writeFixtureFile(t, dir, defaultConfigPath, hookModules+"coupling:\n  volatility_cascade: true\n")
	if out := check(); !strings.Contains(out, review) || strings.Contains(out, record) {
		t.Fatalf("drifted reference: want the review step, never a blanket baseline\n%s", out)
	}
}
