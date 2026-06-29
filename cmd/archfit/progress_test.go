package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// TestResolveProgress covers the visibility/animation decision. A bytes.Buffer is
// never a TTY, so "auto" resolves off unless an env/mode forces plain output.
func TestResolveProgress(t *testing.T) {
	var buf bytes.Buffer
	cases := []struct {
		name        string
		mode        string
		quiet       bool
		ci, term    string
		wantEnabled bool
		wantLive    bool
	}{
		{"quiet wins", progressAuto, true, "", "", false, false},
		{"none off", progressNone, false, "", "", false, false},
		{"auto non-tty off", progressAuto, false, "", "", false, false},
		{"plain non-tty on, not live", progressPlain, false, "", "", true, false},
		{"ci suppresses auto", progressAuto, false, "1", "", false, false},
		{"ci allows forced plain", progressPlain, false, "1", "", true, false},
		{"dumb term suppresses auto", progressAuto, false, "", "dumb", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CI", tc.ci)
			t.Setenv("TERM", tc.term)
			enabled, live := resolveProgress(&buf, tc.mode, tc.quiet)
			if enabled != tc.wantEnabled || live != tc.wantLive {
				t.Errorf("resolveProgress = (enabled=%v, live=%v), want (%v, %v)",
					enabled, live, tc.wantEnabled, tc.wantLive)
			}
		})
	}
}

// TestProgressReporter_PlainOutput verifies the plain (non-TTY) line-per-phase
// output: a banner, one numbered line per completed phase, and a total.
func TestProgressReporter_PlainOutput(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("TERM", "")
	var buf bytes.Buffer
	r := newProgressReporter(&buf, 2, progressPlain, false, time.Now())
	r.banner("Archfit analyzing demo")
	r.advance("Loading config")
	r.advance("Scoring architecture")
	r.finish()

	out := buf.String()
	for _, want := range []string{
		"Archfit analyzing demo",
		"[1/2] Loading config",
		"[2/2] Scoring architecture",
		"Done in ",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plain progress output missing %q\n---\n%s", want, out)
		}
	}
	// Plain mode must never emit carriage returns (log-safe).
	if strings.Contains(out, "\r") {
		t.Errorf("plain mode must not use carriage returns:\n%q", out)
	}
}

// TestProgressReporter_DisabledIsSilent verifies a disabled reporter writes nothing.
func TestProgressReporter_DisabledIsSilent(t *testing.T) {
	var buf bytes.Buffer
	r := newProgressReporter(&buf, 3, progressNone, false, time.Now())
	r.banner("hi")
	r.advance("x")
	r.finish()
	if buf.Len() != 0 {
		t.Errorf("disabled reporter must be silent, got: %q", buf.String())
	}
}
