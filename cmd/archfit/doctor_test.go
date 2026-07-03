package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/toolrun"
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

func TestDoctorCmd_FlagsDeadForbiddenDependencyRule(t *testing.T) {
	dir := t.TempDir()
	const yaml = `version: 1
rules:
  - id: no-forbidden-deps
    type: forbidden_dependency
    gate: warn
`
	if err := os.WriteFile(filepath.Join(dir, ".archfit.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	out := runDoctorCmd(t, dir)
	if !strings.Contains(out, `rule "no-forbidden-deps"`) {
		t.Errorf("expected doctor output to flag the dead rule, got:\n%s", out)
	}
	if !strings.Contains(out, "dead by construction") {
		t.Errorf("expected doctor output to explain why, got:\n%s", out)
	}
}

func TestDoctorCmd_NoWarningForLiveForbiddenDependencyRule(t *testing.T) {
	dir := t.TempDir()
	const yaml = `version: 1
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
	if strings.Contains(out, "Config checks") {
		t.Errorf("expected no dead-rule warning for a rule with from:/to: set, got:\n%s", out)
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

func TestDeadForbiddenDependencyRules(t *testing.T) {
	tests := []struct {
		name string
		def  config.RuleDef
		want bool
	}{
		{"both empty", config.RuleDef{ID: "r1", Type: ruleTypeForbiddenDependency}, true},
		{"from set", config.RuleDef{ID: "r2", Type: ruleTypeForbiddenDependency, From: "a/**"}, false},
		{"to set", config.RuleDef{ID: "r3", Type: ruleTypeForbiddenDependency, To: "b/**"}, false},
		{"both set", config.RuleDef{ID: "r4", Type: ruleTypeForbiddenDependency, From: "a/**", To: "b/**"}, false},
		{"different type", config.RuleDef{ID: "r5", Type: "forbidden_layer_direction"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Config{Rules: []config.RuleDef{tt.def}}
			got := deadForbiddenDependencyRules(cfg)
			if tt.want && len(got) != 1 {
				t.Errorf("deadForbiddenDependencyRules() = %v, want [%q]", got, tt.def.ID)
			}
			if !tt.want && len(got) != 0 {
				t.Errorf("deadForbiddenDependencyRules() = %v, want empty", got)
			}
		})
	}
}
