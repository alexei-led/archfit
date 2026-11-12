package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/application/request"
)

// requestGoldens holds the text of `archfit request` without the App (D5).
const requestGoldens = "testdata/request"

const (
	cmdRequest      = "request"
	whyGraph        = "netem needs the shared retry helper"
	ruleGraph       = "graph"
	ruleDuplication = "duplication"
	flagExpires     = "--expires"
)

// TestRequestFromTheCLI: check or can, then request, on the examples of
// design 8.1. Without the App the request is printed and the exit is 2.
func TestRequestFromTheCLI(t *testing.T) {
	t.Parallel()
	base := fixtureFiles()
	unprotected := fixtureFiles()
	delete(unprotected, ".github/workflows/archfit.yml")
	for _, tc := range []struct {
		name       string
		base, head map[string]string
		first      []string
		request    []string
	}{
		{name: ruleGraph, base: base, head: map[string]string{tcNetem: tcImportsUtil},
			first: []string{cmdCheck}, request: []string{"-m", whyGraph}},
		{name: ruleDuplication, base: base, head: map[string]string{tcInterface: tcCopiesLiteral},
			first:   []string{cmdCheck},
			request: []string{"-m", "tc parses the name before pkg_util loads", flagExpires, "2026-12-10"}},
		{name: "can", base: base, first: []string{cmdCan, pkgTC, pkgUtil}, request: []string{"-m", whyGraph}},
		{name: "not_protected", base: unprotected, head: map[string]string{tcNetem: tcImportsUtil},
			first: []string{cmdCheck}, request: []string{"-m", whyGraph}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := fixtureRepo(t, tc.base, tc.head)
			runCLI(t, fixtureDate, append(tc.first, "-C", dir)...)
			got := runCLI(t, fixtureDate, append([]string{cmdRequest, "-C", dir}, tc.request...)...)
			if got.code != exitUnknown || got.stderr != "" {
				t.Errorf("exit = %d, stderr %q; want 2 and nothing on stderr", got.code, got.stderr)
			}
			if !strings.HasSuffix(got.stdout, "\n") || !strings.HasPrefix(lastLine(got.stdout), askOwner) {
				t.Errorf("the request does not end with %q:\n%s", askOwner, got.stdout)
			}
			assertCLIGolden(t, requestGoldens, tc.name, got.stdout)
		})
	}
}

// TestRequestRefusesFromTheCLI: every refusal exits 2 with its reason on
// stderr and prints no request.
func TestRequestRefusesFromTheCLI(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		head    map[string]string
		first   []string
		edit    bool
		request []string
		want    string
	}{
		{name: "no check output", head: map[string]string{tcNetem: tcImportsUtil},
			request: []string{"-m", whyGraph}, want: "run archfit check first"},
		{name: "an edit after the check", head: map[string]string{tcNetem: tcImportsUtil}, first: []string{cmdCheck}, edit: true,
			request: []string{"-m", whyGraph}, want: "run archfit check first"},
		{name: "a pass", head: map[string]string{runtimeGo: fixtureFiles()[runtimeGo] + "\n// Done.\n"}, first: []string{cmdCheck},
			request: []string{"-m", whyGraph}, want: "nothing to ask: archfit: pass"},
		{name: "no reason off a terminal", head: map[string]string{tcNetem: tcImportsUtil}, first: []string{cmdCheck},
			want: `a reason is required: archfit request -m "<why>"`},
		{name: "a date in the past", head: map[string]string{tcNetem: tcImportsUtil}, first: []string{cmdCheck},
			request: []string{"-m", whyGraph, flagExpires, "2026-11-01"}, want: "--expires is not a later date"},
		{name: "a date too far ahead", head: map[string]string{tcNetem: tcImportsUtil}, first: []string{cmdCheck},
			request: []string{"-m", whyGraph, flagExpires, "2027-12-01"}, want: "--expires is too far ahead: want 2027-11-10 or earlier"},
		{name: "the flag at the end of the reason", head: map[string]string{tcNetem: tcImportsUtil}, first: []string{cmdCheck},
			request: []string{"-m", whyGraph + " --expires soon"}, want: "the reason ends like the --expires flag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := fixtureRepo(t, fixtureFiles(), tc.head)
			if tc.first != nil {
				runCLI(t, fixtureDate, append(tc.first, "-C", dir)...)
			}
			if tc.edit {
				writeFiles(t, dir, map[string]string{tcNetem: tcImportsUtil + "\n// Edited.\n"})
			}
			got := runCLI(t, fixtureDate, append([]string{cmdRequest, "-C", dir}, tc.request...)...)
			if got.code != exitUnknown || got.stdout != "" {
				t.Errorf("exit = %d, stdout %q; want 2 and no request", got.code, got.stdout)
			}
			if !strings.HasPrefix(got.stderr, "archfit: ") || !strings.Contains(got.stderr, tc.want) {
				t.Errorf("stderr = %q, want archfit: ... %q", got.stderr, tc.want)
			}
		})
	}

	got := runCLI(t, fixtureDate, cmdRequest, "-m", whyGraph, "-C", t.TempDir())
	if got.code != exitUnknown || !strings.Contains(got.stderr, notARepo) {
		t.Errorf("outside a repository: exit %d, stderr %q", got.code, got.stderr)
	}
}

// TestLineReason reads the reason a terminal gives, after the question.
func TestLineReason(t *testing.T) {
	t.Parallel()
	var prompt strings.Builder
	got, err := lineReason{in: strings.NewReader(whyGraph + "\nmore\n"), prompt: &prompt}.Reason(context.Background())
	if err != nil || got != whyGraph+"\n" || prompt.String() != "reason: " {
		t.Errorf("Reason = %q, %v after %q; want the first line after %q", got, err, prompt.String(), "reason: ")
	}
	got, err = lineReason{in: strings.NewReader("no newline"), prompt: &prompt}.Reason(context.Background())
	if err != nil || got != "no newline" {
		t.Errorf("Reason at end of input = %q, %v", got, err)
	}
	boom := errors.New("boom")
	if _, err := (lineReason{in: failingReader{boom}, prompt: &prompt}).Reason(context.Background()); !errors.Is(err, boom) {
		t.Errorf("Reason on a read error = %v, want %v", err, boom)
	}
}

// TestNoTerminalReasonInTests: a command run with its own stdin never asks.
func TestNoTerminalReasonInTests(t *testing.T) {
	t.Parallel()
	if r := terminalReason(&appDeps{Stdin: strings.NewReader("")}); r != nil {
		t.Errorf("terminalReason with a given stdin = %v, want nil", r)
	}
	var _ request.ReasonReader = lineReason{}
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

func lastLine(text string) string {
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	return lines[len(lines)-1]
}
