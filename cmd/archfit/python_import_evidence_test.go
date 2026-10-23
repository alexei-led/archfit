package main

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const pythonProjectFile = "pyproject.toml"

func TestCheckPythonUnscannedSourceNeverCountsAsConformance(t *testing.T) {
	skipShortWithoutRequiredTools(t)
	requireTool(t, "uv")
	for _, tc := range []struct {
		from    string
		unknown bool
	}{
		{"scripts.**", true}, {"**", true}, {"app.handler", false},
	} {
		t.Run(tc.from, func(t *testing.T) {
			root := t.TempDir()
			config := `version: 2
languages:
  python:
    package: app
modules:
  app:
    paths: [app.**]
rules:
  - id: no-backend
    type: forbidden_dependency
    from: "` + tc.from + `"
    to: app.backend
    gate: fail
`
			for name, content := range map[string]string{
				pythonProjectFile: "[project]\nname='fixture'\nversion='0.0.0'\n",
				"app/__init__.py": "", "app/backend.py": "", "app/handler.py": "import absent_fixture_external\n",
				"scripts/run.py": "import app.backend\n", defaultConfigPath: config,
			} {
				writeFileAt(t, root, name, content)
			}
			gitInitFixtureRepo(t, root)
			code, stdout, stderr := runArchfit(t, cmdCheck, "-c", filepath.Join(root, defaultConfigPath), "--json")
			var got ruleScopeDecision
			if err := json.Unmarshal([]byte(stdout), &got); err != nil || code == 3 {
				t.Fatalf("check exit=%d err=%v\n%s\n%s", code, err, stdout, stderr)
			}
			if (len(got.Decision.Unevaluated) != 0) != tc.unknown || (got.Decision.HardGates == "pass") == tc.unknown {
				t.Fatalf("decision=%+v want unknown=%v", got.Decision, tc.unknown)
			}
		})
	}
}

func TestCheckPythonRulesWithMissingExternalPackages(t *testing.T) {
	skipShortWithoutRequiredTools(t)
	requireTool(t, "uv")
	const hardGatePassState = "pass"
	type observedFinding struct {
		RuleID string `json:"rule_id"`
	}
	const config = `version: 2
languages:
  go:
    enabled: false
  typescript:
    enabled: false
  rust:
    enabled: false
  python:
    package: app
modules:
  handlers:
    paths: [app.handler]
  backend:
    paths: [app.backend]
rules:
  - id: no-handler-backend
    type: forbidden_dependency
    from: app.handler
    to: app.backend
    gate: fail
`
	for _, tc := range []struct {
		name, source, hardGates string
		unevaluated             int
		blocked                 bool
		rules                   string
	}{
		{"external package absent", "import archfit_fixture_external\n", hardGatePassState, 0, false, ""},
		{"forbidden internal import", "import archfit_fixture_external\nfrom . import backend\n", hardGateFail, 0, true, ""},
		{"guarded forbidden import", "try:\n    from . import backend\nexcept ImportError:\n    pass\n", hardGateFail, 0, true, ""},
		{"from import and missing target share line", "from app import backend; import app.backend.missing\n", hardGateFail, 0, true, ""},
		{"missing internal literal", "import app.missing\n", hardGatePassState, 0, false, ""},
		{"missing internal ban", "try:\n    import app.missing\nexcept ImportError:\n    pass\n", hardGateFail, 0, true, "  - id: no-handler-backend\n    type: forbidden_dependency\n    from: app.handler\n    to: app.missing\n    gate: fail\n    guard: true\n"},
		{"external ban", "import archfit_fixture_external\n", hardGateFail, 0, true, "  - id: no-handler-backend\n    type: forbidden_dependency\n    from: app.handler\n    to: archfit_fixture_external\n    gate: fail\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			cfg := config
			if tc.rules != "" {
				header, _, found := strings.Cut(config, "rules:\n")
				if !found {
					t.Fatal("fixture has no rules section")
				}
				cfg = header + "rules:\n" + tc.rules
			}
			for name, content := range map[string]string{
				pythonProjectFile: "[project]\nname = 'import-fixture'\nversion = '0.0.0'\n",
				"app/__init__.py": "raise RuntimeError('target must not execute')\n",
				"app/handler.py":  tc.source,
				"app/backend.py":  "value = 1\n",
				defaultConfigPath: cfg,
			} {
				writeFileAt(t, root, name, content)
			}
			gitInitFixtureRepo(t, root)
			code, stdout, stderr := runArchfit(t, cmdCheck, "-c", filepath.Join(root, defaultConfigPath), "--json")
			if code == 3 || (code == 1) != tc.blocked {
				t.Fatalf("check exit=%d blocked=%v\n%s\n%s", code, tc.blocked, stdout, stderr)
			}
			var got struct {
				ruleScopeDecision
				Findings []observedFinding `json:"findings"`
			}
			if err := json.Unmarshal([]byte(stdout), &got); err != nil {
				t.Fatal(err)
			}
			if got.Decision.HardGates != tc.hardGates || len(got.Decision.Unevaluated) != tc.unevaluated {
				t.Fatalf("decision=%+v, want hard_gates=%s unevaluated=%d", got.Decision, tc.hardGates, tc.unevaluated)
			}
			if tc.blocked && !slices.ContainsFunc(got.Findings, func(f observedFinding) bool {
				return f.RuleID == "no-handler-backend"
			}) {
				t.Fatalf("forbidden internal import has no finding:\n%s", stdout)
			}
		})
	}
}
