package main

import (
	"bytes"
	"strings"
	"testing"
)

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
		{"with --full", []string{"score", "-c", cfgPath, flagFull}},
		{"without --full (implied)", []string{"score", "-c", cfgPath}},
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
