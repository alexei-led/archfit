package main

import (
	"strings"
	"testing"
)

func TestHelpShowsOnlyTheSevenCommands(t *testing.T) {
	t.Parallel()
	code, stdout, stderr := runArchfit(t, flagHelp)
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	for _, command := range []string{cmdCheck, cmdCan, cmdRules, cmdRequest, cmdInit, cmdReport, cmdDoctor} {
		if !strings.Contains(stdout, "  "+command) {
			t.Errorf("help missing %s:\n%s", command, stdout)
		}
	}
	for _, retired := range []string{"analyze", "baseline", "explain", "policy can-import", "config init", "agents-md", "skill install", "hook install", "Analysis\n", "Setup & config"} {
		if strings.Contains(stdout, retired) {
			t.Errorf("help includes %q:\n%s", retired, stdout)
		}
	}
}

func TestUsageErrorsAreOnStderrAndExit2(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {"check", "--not-a-flag"}, {"unknown-command"}} {
		code, stdout, stderr := runArchfit(t, args...)
		if code != exitUnknown || stdout != "" || strings.TrimSpace(stderr) == "" {
			t.Errorf("%v: exit %d, stdout %q, stderr %q; want 2 and stderr only", args, code, stdout, stderr)
		}
	}
}
