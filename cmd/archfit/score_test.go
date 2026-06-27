package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

const cmdScore = "score"

// TestRun_Score_FullFlagParses verifies the Task 6 fix: `score --full` parses
// and runs (rc 0), where it previously failed kong parse (rc 3) because ScoreCmd
// had no Full field. Score is always report-only, so a violating repo still
// exits 0; the assertion is that the flag is accepted and a scorecard renders.
func TestRun_Score_FullFlagParses(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)

	cases := []struct {
		name string
		args []string
	}{
		{"with --full", []string{cmdScore, "-c", cfgPath, flagFull}},
		{"without --full (implied)", []string{cmdScore, "-c", cfgPath}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			code := Run(tc.args, &buf)
			if code != 0 {
				t.Fatalf("score exit = %d, want 0\noutput:\n%s", code, buf.String())
			}
			if !strings.Contains(buf.String(), "## Dimensions") {
				t.Errorf("score did not render a scorecard\noutput:\n%s", buf.String())
			}
		})
	}
}

// TestRun_Score_NoConfigFlag verifies the bug fix: `score --no-config` parses and
// runs (rc 0), where it previously failed kong parse (rc 3, "unknown flag
// --no-config") because ScoreCmd had no NoConfig field. --no-config ignores the
// on-disk .archfit.yaml and scores with built-in defaults; score is report-only,
// so a violating repo still exits 0 and renders a scorecard.
func TestRun_Score_NoConfigFlag(t *testing.T) {
	t.Parallel()
	dir := filepath.Dir(writeViolatingRepo(t))

	var buf bytes.Buffer
	code := Run([]string{cmdScore, "--no-config", "--root", dir}, &buf)
	if code != 0 {
		t.Fatalf("score --no-config exit = %d, want 0\noutput:\n%s", code, buf.String())
	}
	if !strings.Contains(buf.String(), "## Dimensions") {
		t.Errorf("score --no-config did not render a scorecard\noutput:\n%s", buf.String())
	}
}
