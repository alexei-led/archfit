package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetiredCommandsAreNotAccepted(t *testing.T) {
	t.Parallel()
	for _, command := range []string{
		"analyze", "baseline", "explain", "policy", "map",
		"config init", "config update", "config lint", "config compare", "config enrich",
		"agents-md", "skill install", "hook install",
	} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()
			args := append(strings.Fields(command), flagHelp)
			var stdout, stderr bytes.Buffer
			if code := RunWithStderr(args, &stdout, &stderr); code != exitUnknown {
				t.Fatalf("%s: exit %d, want 2; stdout %q; stderr %q", command, code, stdout.String(), stderr.String())
			}
			if stdout.Len() != 0 || stderr.Len() == 0 {
				t.Fatalf("%s: stdout %q; stderr %q; want a usage error only on stderr", command, stdout.String(), stderr.String())
			}
		})
	}
}

func TestDoctorErrorsUseExit2AndKeepTheirMessages(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		args       []string
		failOutput bool
		want       string
	}{
		{name: "fix with json", args: []string{cmdDoctor, flagFormat, formatJSON, "--fix"}, want: "error: doctor --format json only reports; run doctor --fix without it\n"},
		{name: "write report", args: []string{cmdDoctor, flagFormat, formatJSON}, failOutput: true, want: "error: write the doctor report: write refused\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			var code int
			if tc.failOutput {
				code = RunWithStderr(tc.args, refusedWriter{}, &stderr)
			} else {
				code = RunWithStderr(tc.args, &stdout, &stderr)
			}
			if code != exitUnknown || stderr.String() != tc.want {
				t.Errorf("exit %d, stderr %q; want 2, %q", code, stderr.String(), tc.want)
			}
		})
	}
}

type refusedWriter struct{}

func (refusedWriter) Write([]byte) (int, error) {
	return 0, errors.New("write refused")
}

func TestMissingPolicyLoaderUsesVersion4WordsAndExit2(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), defaultConfigPath)
	_, err := loadAnalysisConfig(t.Context(), path)
	var exit *exitError
	if !errors.As(err, &exit) || exit.ExitCode() != exitUnknown {
		t.Fatalf("error = %v; want exit 2", err)
	}
	if want := "archfit: policy not found: " + path + "\nnext: run archfit init"; err.Error() != want {
		t.Errorf("error = %q; want %q", err.Error(), want)
	}
}

func TestCheckRejectsRetiredFlagsAndVersion2Policies(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"--config", "-c", "--root", "--target", "--no-advisories", "--min-severity", "--refresh", "--require-tools", "--lang", "--json", "--markdown", "--sarif", "--progress", "-q"} {
		t.Run(flag, func(t *testing.T) {
			code, stdout, stderr := runArchfit(t, cmdCheck, flag)
			if code != exitUnknown || stdout != "" || !strings.Contains(stderr, "unknown") {
				t.Errorf("exit %d, stdout %q, stderr %q; want unknown flag and exit 2", code, stdout, stderr)
			}
		})
	}
	files := fixtureFiles()
	files[defaultConfigPath] = minimalValidYAML
	dir := fixtureRepo(t, files, nil)
	got := runCLI(t, fixtureDate, cmdCheck, "-C", dir)
	if got.code != exitUnknown || !strings.Contains(got.stdout+got.stderr, "archfit init") {
		t.Errorf("exit %d, stdout %q, stderr %q; want 2 and conversion guidance", got.code, got.stdout, got.stderr)
	}
}
