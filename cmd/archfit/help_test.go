package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRun_Help_TopLevel asserts the top-level --help surface: commands that must
// be present and commands that were removed and must not appear.
func TestRun_Help_TopLevel(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	code := Run([]string{flagHelp}, &buf)
	if code != 0 {
		t.Fatalf("--help exit = %d, want 0", code)
	}
	out := buf.String()

	for _, want := range []string{cmdAnalyze, cmdBaseline, cmdExplain, "doctor", cmdConfig} {
		if !strings.Contains(out, want) {
			t.Errorf("--help missing %q:\n%s", want, out)
		}
	}
	// Commands folded or removed in the CLI restructure must not appear as top-level commands.
	for _, gone := range []string{"autopilot", "calibrate"} {
		if strings.Contains(out, gone) {
			t.Errorf("--help still shows removed command %q:\n%s", gone, out)
		}
	}
}

// TestRun_Help_ConfigSubtree asserts that the config subcommand tree is correct.
func TestRun_Help_ConfigSubtree(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	code := Run([]string{cmdConfig, flagHelp}, &buf)
	if code != 0 {
		t.Fatalf("config --help exit = %d, want 0", code)
	}
	out := buf.String()
	for _, want := range []string{"init", "update", cmdEnrich} {
		if !strings.Contains(out, want) {
			t.Errorf("config --help missing %q:\n%s", want, out)
		}
	}
}
