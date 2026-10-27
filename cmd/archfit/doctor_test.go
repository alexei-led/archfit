package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// runDoctorCmd runs DoctorCmd.Run in dir (chdir'd so it picks up dir/.archfit.yaml
// via the hardcoded defaultConfigPath) and returns stdout.
func runDoctorCmd(t *testing.T, dir string) string {
	t.Helper()
	t.Chdir(dir)
	var buf bytes.Buffer
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, false
		},
	}
	deps := &appDeps{Runner: runner, Stdout: &buf}
	if err := (&DoctorCmd{}).Run(deps); err != nil {
		t.Fatalf("DoctorCmd.Run: %v", err)
	}
	return buf.String()
}

// A forbidden_dependency rule missing from:/to: globs is rejected at config
// load (rules.New), so doctor surfaces it as a load failure — the old
// advisory-only "dead by construction" note is gone with the vacuous rule.
func TestDoctorCmd_VacuousForbiddenDependencyFailsLoad(t *testing.T) {
	dir := t.TempDir()
	const yaml = `version: 2
rules:
  - id: no-forbidden-deps
    type: forbidden_dependency
    gate: warn
`
	if err := os.WriteFile(filepath.Join(dir, ".archfit.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	out := runDoctorCmd(t, dir)
	if !strings.Contains(out, ".archfit.yaml failed to load") {
		t.Errorf("expected doctor to surface the config load error, got:\n%s", out)
	}
	if !strings.Contains(out, "requires from or from_module") {
		t.Errorf("expected the load error to explain the empty globs, got:\n%s", out)
	}
}

func TestDoctorCmd_LiveForbiddenDependencyRuleLoadsClean(t *testing.T) {
	dir := t.TempDir()
	const yaml = `version: 2
rules:
  - id: no-cmd-to-internal
    type: forbidden_dependency
    gate: warn
    from: "cmd/**"
    to: "internal/**"
`
	if err := os.WriteFile(filepath.Join(dir, ".archfit.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	out := runDoctorCmd(t, dir)
	if strings.Contains(out, "failed to load") {
		t.Errorf("expected a rule with from:/to: set to load clean, got:\n%s", out)
	}
}

// TestDoctorCmd_NoConfigFile pins the fresh-repo path (before config init):
// the absent default config falls back to config.Default(), so doctor must
// succeed with no Config checks block rather than error out.
func TestDoctorCmd_NoConfigFile(t *testing.T) {
	out := runDoctorCmd(t, t.TempDir())
	if strings.Contains(out, "Config checks") {
		t.Errorf("expected no config-check output without a config file, got:\n%s", out)
	}
}

// TestDoctorCmd_BrokenConfigSurfacesError pins that a present-but-invalid
// .archfit.yaml is reported as a load failure, not silently rendered as an
// unconfigured LLM with the config checks skipped.
func TestDoctorCmd_BrokenConfigSurfacesError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".archfit.yaml"), []byte("version: [broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	out := runDoctorCmd(t, dir)
	if !strings.Contains(out, ".archfit.yaml failed to load") {
		t.Errorf("expected doctor to surface the config load error, got:\n%s", out)
	}
	if strings.Contains(out, "not configured") {
		t.Errorf("broken config must not read as merely unconfigured, got:\n%s", out)
	}
}

// TestDoctorCmd_RoslynHelperRow pins the env-probed helper row: unset is
// missing with the build hint, a valid directory is ok, and an unusable value
// fails doctor after the full report, as it fails analysis.
func TestDoctorCmd_RoslynHelperRow(t *testing.T) {
	const env = "ARCHFIT_DOTNET_ROSLYN"
	helper := t.TempDir()
	if err := os.WriteFile(filepath.Join(helper, "archfit-roslyn.dll"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, value string
		unset       bool
		wantRow     *regexp.Regexp
		wantErr     bool
	}{
		{name: "unset", unset: true, wantRow: doctorRow("archfit-roslyn", "missing", "build it with `make dotnet-roslyn`")},
		{name: "valid", value: helper, wantRow: doctorRow("archfit-roslyn", "ok", helper+" ("+helperIdentity+")")},
		{name: "relative", value: "dotnet-roslyn", wantRow: doctorRow("archfit-roslyn", "invalid", "ARCHFIT_DOTNET_ROSLYN must name the absolute helper directory"), wantErr: true},
		{name: "no helper there", value: t.TempDir(), wantRow: doctorRow("archfit-roslyn", "invalid", "ARCHFIT_DOTNET_ROSLYN must name a directory holding archfit-roslyn.dll"), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(env, tc.value)
			if tc.unset {
				if err := os.Unsetenv(env); err != nil {
					t.Fatal(err)
				}
			}
			t.Chdir(t.TempDir())
			var buf bytes.Buffer
			runner := &toolrun.RunnerMock{
				DetectFunc: func(context.Context, string) (toolrun.ToolInfo, bool) { return toolrun.ToolInfo{}, false },
				RunFunc: func(context.Context, toolrun.ToolCmd) (toolrun.Output, error) {
					return toolrun.Output{Stdout: []byte(helperIdentity + "\n")}, nil
				},
			}
			err := (&DoctorCmd{}).Run(&appDeps{Runner: runner, Stdout: &buf})
			if (err != nil) != tc.wantErr {
				t.Fatalf("DoctorCmd.Run error = %v, want error %v", err, tc.wantErr)
			}
			if !tc.wantRow.MatchString(buf.String()) {
				t.Errorf("doctor output lacks %s:\n%s", tc.wantRow, buf.String())
			}
			if row := doctorRow("dotnet", "missing", "https://dotnet.microsoft.com/download"); !row.MatchString(buf.String()) {
				t.Errorf("doctor output lacks the dotnet SDK row:\n%s", buf.String())
			}
		})
	}
}

const helperIdentity = "archfit-roslyn 1.0.0; Microsoft.CodeAnalysis 5.6.0; SDK 10.0.403"

// doctorRow matches one doctor table row: the name, then the status and the
// detail each starting a column.
func doctorRow(name, status, detail string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + ` +` + regexp.QuoteMeta(status) + ` +` + regexp.QuoteMeta(detail))
}

// TestDoctorCmd_ColumnsAlign pins that every row starts STATUS and the detail at
// the header's columns, the longest tool name included.
func TestDoctorCmd_ColumnsAlign(t *testing.T) {
	out := runDoctorCmd(t, t.TempDir())
	lines := strings.Split(out, "\n")
	header := lines[0]
	statusCol, detailCol := strings.Index(header, "STATUS"), strings.Index(header, "PATH / INSTALL")
	for _, line := range lines[2:] {
		if line == "" {
			break
		}
		if !strings.HasPrefix(line[statusCol:], "missing ") || line[detailCol-1] != ' ' || line[detailCol] == ' ' {
			t.Errorf("row is not aligned with the header:\n%s\n%s", header, line)
		}
	}
}

// TestDoctorCmd_DotNetSDKVersion pins the dotnet row: the SDK version the
// checked directory selects, or why its global.json cannot be satisfied.
func TestDoctorCmd_DotNetSDKVersion(t *testing.T) {
	const pinFailure = "A compatible .NET SDK was not found.\n\nRequested SDK version: 10.0.999\nglobal.json file: /repo/global.json\n\nInstalled SDKs:\n"
	tests := []struct {
		name    string
		out     toolrun.Output
		wantRow *regexp.Regexp
		wantErr bool
	}{
		{
			name:    "installed SDK",
			out:     toolrun.Output{Stdout: []byte("10.0.403\n")},
			wantRow: doctorRow("dotnet", "ok", "/usr/bin/dotnet (SDK 10.0.403)"),
		},
		{
			name:    "global.json pins a missing SDK",
			out:     toolrun.Output{Stderr: []byte(pinFailure), ExitCode: 155},
			wantRow: doctorRow("dotnet", "invalid", "dotnet --version fails in "),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			t.Setenv("ARCHFIT_DOTNET_ROSLYN", "")
			if err := os.Unsetenv("ARCHFIT_DOTNET_ROSLYN"); err != nil {
				t.Fatal(err)
			}
			var gotDir string
			runner := &toolrun.RunnerMock{
				DetectFunc: func(_ context.Context, name string) (toolrun.ToolInfo, bool) {
					return toolrun.ToolInfo{Path: "/usr/bin/dotnet"}, name == "dotnet"
				},
				RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
					gotDir = cmd.WorkDir
					return tt.out, nil
				},
			}
			var buf bytes.Buffer
			err := (&DoctorCmd{}).Run(&appDeps{Runner: runner, Stdout: &buf})
			if (err != nil) != tt.wantErr {
				t.Fatalf("DoctorCmd.Run error = %v, want error %v", err, tt.wantErr)
			}
			if !tt.wantRow.MatchString(buf.String()) {
				t.Errorf("doctor output lacks %s:\n%s", tt.wantRow, buf.String())
			}
			if tt.wantErr && !strings.Contains(buf.String(), "/repo/global.json requests SDK 10.0.999, which is not installed; install a matching SDK or relax the pin") {
				t.Errorf("doctor does not name the global.json pin:\n%s", buf.String())
			}
			if resolved, _ := filepath.EvalSymlinks(dir); gotDir != dir && gotDir != resolved {
				t.Errorf("dotnet --version ran in %q, want the checked directory %q", gotDir, dir)
			}
		})
	}
}
