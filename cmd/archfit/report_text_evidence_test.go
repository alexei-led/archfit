package main

import (
	"strings"
	"testing"
)

func TestReportTextIncludesEvidenceAndLimits(t *testing.T) {
	t.Parallel()
	dir := fixtureRepo(t, fixtureFiles(), nil)
	got := runCLI(t, fixtureDate, cmdReport, "-C", dir)
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	for _, want := range []string{"evidence:\n", "snapshot: stable by pre/post check", "policy binding: match", "seam-eligible edges:", "all retained edges:", "volatility origins:", "interface inventory: unknown", "snapshot gaps:"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("report text missing %q:\n%s", want, got.stdout)
		}
	}
}
