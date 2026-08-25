package main

import (
	"bytes"
	"testing"

	"github.com/alexei-led/archfit/internal/application"
)

func TestOutcomeExitCodeOwnsCLIOutcomeTranslation(t *testing.T) {
	for _, test := range []struct {
		name    string
		outcome application.Outcome
		want    int
	}{
		{name: "pass", outcome: application.OutcomePass, want: 0},
		{name: "warn", outcome: application.OutcomeWarn, want: 2},
		{name: "fail", outcome: application.OutcomeFail, want: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := outcomeExitCode(test.outcome); got != test.want {
				t.Fatalf("outcomeExitCode(%q) = %d, want %d", test.outcome, got, test.want)
			}
		})
	}
}

func TestRunScanWiresRefreshAndProgressBeforePreparation(t *testing.T) {
	var stderr bytes.Buffer
	deps := &appDeps{Stderr: &stderr}
	err := runScan(t.Context(), deps, scanRequest{
		configPath: t.TempDir() + "/missing.yaml",
		refresh:    true,
		progress:   progressNone,
	})
	if err == nil {
		t.Fatal("runScan error = nil, want missing config")
	}
	if !deps.refresh {
		t.Fatal("runScan did not propagate --refresh to pipeline dependencies")
	}
	if deps.progress == nil {
		t.Fatal("runScan did not wire pipeline progress callback")
	}
}
