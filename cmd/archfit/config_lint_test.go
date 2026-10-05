package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestConfigLint_ExitCodesAndOutput(t *testing.T) {
	t.Parallel()
	const live = `rules:
  - id: b-not-to-a
    type: forbidden_dependency
    from: "pkg/b/**"
    to: "pkg/a/**"
    gate: fail
`
	const dead = `rules:
  - id: a-not-to-catalog
    type: forbidden_dependency
    from: "pkg/a/**"
    to: "pkg/catalog/**"
    gate: fail
`
	for _, tc := range []struct {
		name     string
		cfg      string
		wantCode int
		wantText string
	}{
		{name: "clean config", cfg: ruleScopeModulesCfg + live, wantCode: 0},
		{
			name: "dead selector", cfg: ruleScopeModulesCfg + dead, wantCode: 1,
			wantText: "error dead_selector rules[a-not-to-catalog].to: to: pkg/catalog/** matches no scanned source; fix the selector or set guard: true\n",
		},
		{
			name: "guard", cfg: ruleScopeModulesCfg + dead + "    guard: true\n", wantCode: 0,
			wantText: "info guard_rule rules[a-not-to-catalog]: guard rule: to pkg/catalog/** matches nothing by design\n",
		},
		{
			name: "unknown volatility", cfg: ruleScopeModulesCfg + "    volatility: hgih\n" + live, wantCode: 1,
			wantText: `error unknown_volatility modules.b.volatility: volatility "hgih" is not one of high, medium, low, frozen, legacy; the module's volatility reads as undeclared` + "\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfgPath := writeRuleScopeRepo(t, tc.cfg)
			code, stdout, stderr := runArchfit(t, "config", "lint", "-c", cfgPath)
			if code != tc.wantCode {
				t.Fatalf("config lint: exit = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, tc.wantCode, stdout, stderr)
			}
			if stdout != tc.wantText {
				t.Fatalf("config lint text:\n got  %q\n want %q", stdout, tc.wantText)
			}
		})
	}
}

func TestConfigLint_JSONDocument(t *testing.T) {
	t.Parallel()
	cfgPath := writeRuleScopeRepo(t, ruleScopeModulesCfg+`rules:
  - id: a-not-to-catalog
    type: forbidden_dependency
    from: "pkg/a/**"
    to: "pkg/catalog/**"
    gate: fail
`)
	code, stdout, stderr := runArchfit(t, "config", "lint", "-c", cfgPath, "--json")
	if code != 1 {
		t.Fatalf("config lint --json: exit = %d, want 1\nstderr:\n%s", code, stderr)
	}
	const want = `{
  "schema_version": "archfit.config-lint.v1",
  "diagnostics": [
    {
      "code": "dead_selector",
      "severity": "error",
      "path": "rules[a-not-to-catalog].to",
      "message": "to: pkg/catalog/** matches no scanned source; fix the selector or set guard: true"
    }
  ]
}
`
	if stdout != want {
		t.Fatalf("config lint --json:\n got  %s\n want %s", stdout, want)
	}

	cleanPath := writeRuleScopeRepo(t, ruleScopeModulesCfg)
	if code, stdout, _ := runArchfit(t, "config", "lint", "-c", cleanPath, "--json"); code != 0 ||
		!strings.Contains(stdout, `"diagnostics": []`) {
		t.Fatalf("clean config lint --json: exit = %d, want 0 with an empty diagnostics array:\n%s", code, stdout)
	}
}

func TestConfigLint_UnreadableConfigExitsThree(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"missing":     "",
		"unknown key": "version: 2\nmodulez: {}\n",
		"old schema":  "version: 1\n",
	} {
		path := filepath.Join(dir, strings.ReplaceAll(name, " ", "-")+".yaml")
		if body != "" {
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if code, _, stderr := runArchfit(t, "config", "lint", "-c", path); code != 3 || stderr == "" {
			t.Errorf("%s: config lint exit = %d (stderr %q), want 3 with an error", name, code, stderr)
		}
	}
}

// TestConfigLintAndCheck_StdlibTargetBesideFirstPartyDirectory runs the real
// toolchain probe: a ban on database/sql in a repo with a top-level database/
// directory is an external ban in both lint and check, while a typo under that
// directory is still a dead selector.
func TestConfigLintAndCheck_StdlibTargetBesideFirstPartyDirectory(t *testing.T) {
	t.Parallel()
	cfgPath := writeRuleFixtureRepo(t, map[string]string{
		markerGoMod:                 fixtureShopGoMod,
		"app/app.go":                "package app\n\nfunc Answer() int { return 42 }\n",
		"database/schema/schema.go": "package schema\n\nconst Version = 1\n",
		defaultConfigPath: `version: 2
modules:
  app:
    paths: ["app/**"]
    owner: team-a
  database:
    paths: ["database/**"]
    owner: team-d
rules:
  - id: no_sql
    type: forbidden_dependency
    from: "app/**"
    to: "database/sql"
    gate: fail
  - id: schema_typo
    type: forbidden_dependency
    from: "app/**"
    to: "database/shcema/**"
    gate: fail
`,
	})
	code, stdout, stderr := runArchfit(t, "config", "lint", "-c", cfgPath)
	want := "error dead_selector rules[schema_typo].to: to: database/shcema/** matches no scanned source; fix the selector or set guard: true\n"
	if code != 1 || stdout != want {
		t.Fatalf("config lint: exit %d, stdout %q, want exit 1 and only the typo\nstderr:\n%s", code, stdout, stderr)
	}
	_, state := checkStateOf(t, cfgPath)
	unevaluated := make([]string, 0, len(state.Decision.UnevaluatedRequiredRules))
	for _, rule := range state.Decision.UnevaluatedRequiredRules {
		unevaluated = append(unevaluated, rule.RuleID+": "+rule.Reason)
	}
	if wantRules := []string{"schema_typo: selector matches nothing: to database/shcema/**"}; !slices.Equal(unevaluated, wantRules) {
		t.Fatalf("unevaluated required rules = %q, want %q", unevaluated, wantRules)
	}
}
