package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/report"
)

const fmtScorecard = "--format=scorecard"

// TestRun_Analyze_ScorecardFormatParses verifies that `analyze --format scorecard`
// parses and runs (rc 0) both on a cold cache (--refresh) and on the cached path.
// Scorecard is always report-only, so a violating repo still exits 0; the
// assertion is that the format is accepted and a scorecard renders.
func TestRun_Analyze_ScorecardFormatParses(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)

	cases := []struct {
		name string
		args []string
	}{
		{"with --refresh", []string{cmdAnalyze, fmtScorecard, "-c", cfgPath, flagRefresh}},
		{"without --refresh (cached facts)", []string{cmdAnalyze, fmtScorecard, "-c", cfgPath}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			code := Run(tc.args, &buf)
			if code != 0 {
				t.Fatalf("analyze --format scorecard exit = %d, want 0\noutput:\n%s", code, buf.String())
			}
			if !strings.Contains(buf.String(), "## Dimensions") {
				t.Errorf("analyze --format scorecard did not render a scorecard\noutput:\n%s", buf.String())
			}
		})
	}
}

// TestRun_Analyze_NoAdvisoriesWithScorecardAndJSON pins one meaning for
// --no-advisories across output combinations: requesting a scorecard alongside
// JSON no longer forces advisories back on, and suppressing them does not move
// the coupling measurement — the dimension is computed from the classified edge
// set, before advisory filtering.
func TestRun_Analyze_NoAdvisoriesWithScorecardAndJSON(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, coupledModulesCfg)

	run := func(t *testing.T, extra ...string) (advisoryCheckDiag, string) {
		t.Helper()
		args := append([]string{cmdAnalyze, fmtJSON, fmtScorecard, "-c", cfgPath}, extra...)
		var buf bytes.Buffer
		if code := Run(args, &buf); code != 0 {
			t.Fatalf("analyze %v: exit = %d\noutput:\n%s", extra, code, buf.String())
		}
		// JSON renders first; decode just that document and leave the scorecard
		// text that follows it in the buffer.
		var d advisoryCheckDiag
		if err := json.NewDecoder(bytes.NewReader(buf.Bytes())).Decode(&d); err != nil {
			t.Fatalf("decode leading JSON document: %v\noutput:\n%s", err, buf.String())
		}
		return d, scorecardCouplingLine(t, buf.String())
	}

	withDiag, withCoupling := run(t)
	// The fixture must actually produce advisories, or every assertion below
	// holds over an empty set and the test silently stops testing.
	if countAdvisoryFindings(withDiag)+len(withDiag.AdvisoryTasks) == 0 {
		t.Fatalf("fixture regression: the coupled repo produced no advisory findings or tasks: %+v", withDiag)
	}

	withoutDiag, withoutCoupling := run(t, flagNoAdvisories)
	if n := countAdvisoryFindings(withoutDiag); n > 0 {
		t.Errorf("--no-advisories alongside --format scorecard: %d advisory finding(s) still in output", n)
	}
	if n := len(withoutDiag.AdvisoryTasks); n > 0 {
		t.Errorf("--no-advisories alongside --format scorecard: %d advisory task(s) still in output", n)
	}
	if withCoupling != withoutCoupling {
		t.Errorf("--no-advisories moved the coupling measurement: %q, want %q", withoutCoupling, withCoupling)
	}
}

// scorecardCouplingLine returns the coupling dimension's denominator line — the
// measurement that advisory filtering must not move.
func scorecardCouplingLine(t *testing.T, out string) string {
	t.Helper()
	inCoupling := false
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "### ") {
			inCoupling = strings.HasPrefix(line, "### "+report.DimensionCoupling+" ")
			continue
		}
		if inCoupling && strings.HasPrefix(line, "denominator: ") {
			return line
		}
	}
	t.Fatalf("no coupling denominator line in the scorecard output:\n%s", out)
	return ""
}

// TestRun_Analyze_NoConfigFlagRejected verifies that --no-config (removed in v2)
// produces a parse error. Config is now required for analyze and check.
func TestRun_Analyze_NoConfigFlagRejected(t *testing.T) {
	t.Parallel()
	var out, errBuf bytes.Buffer
	code := RunWithStderr([]string{cmdAnalyze, fmtScorecard, "--no-config"}, &out, &errBuf)
	if code != 3 {
		t.Fatalf("analyze --no-config: exit = %d, want 3 (parse error); stderr:\n%s", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "--no-config") {
		t.Errorf("parse error should name the removed flag; stderr:\n%s", errBuf.String())
	}
}
