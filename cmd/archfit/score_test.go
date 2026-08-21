package main

import (
	"bytes"
	"strings"
	"testing"
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
