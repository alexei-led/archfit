package main

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/report"
)

// ruleScopeDecision is the slice of the published state these tests read.
type ruleScopeDecision struct {
	Decision struct {
		HardGates   string `json:"hard_gates"`
		Unevaluated []struct {
			RuleID string `json:"rule_id"`
			Reason string `json:"reason"`
		} `json:"unevaluated_required_rules"`
	} `json:"decision"`
}

// checkDecision runs `archfit check --json` and decodes the decision block. A
// blocked run or a usage error fails the test: these fixtures carry no
// violation, so exit 1 or 3 would be a different defect.
func checkDecision(t *testing.T, cfgPath string) ruleScopeDecision {
	t.Helper()
	code, stdout, stderr := runArchfit(t, cmdCheck, "-c", cfgPath, flagRefresh, "--json")
	if code == 1 || code == 3 {
		t.Fatalf("check: exit = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	var got ruleScopeDecision
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode state: %v\n%s", err, stdout)
	}
	return got
}

// ruleScopeModulesCfg declares two modules over a Go repo that type-checks, so
// go/packages completes and only rule scope can hold a rule unevaluated.
const ruleScopeModulesCfg = `version: 2
modules:
  a:
    paths: ["pkg/a/**"]
    owner: team
  b:
    paths: ["pkg/b/**"]
    owner: team
`

// writeRuleScopeRepo creates a compiling two-package Go repo (pkg/a imports
// pkg/b) with the given config body and returns the config path.
func writeRuleScopeRepo(t *testing.T, cfgBody string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{
		markerGoMod:       "module example.com/scope\n\ngo 1.21\n",
		"pkg/a/a.go":      "package a\n\nimport \"example.com/scope/pkg/b\"\n\nfunc A() string { return b.B() }\n",
		"pkg/b/b.go":      "package b\n\nfunc B() string { return \"b\" }\n",
		defaultConfigPath: cfgBody,
	} {
		writeFileAt(t, dir, name, content)
	}
	gitInitFixtureRepo(t, dir)
	return filepath.Join(dir, defaultConfigPath)
}

// TestRun_Check_VacuousSelectorIsNotEvaluated runs the real check with a rule
// whose target was renamed away: the rule cannot fire, so it is listed as not
// evaluated with the selector named, unless it is a declared guard.
func TestRun_Check_VacuousSelectorIsNotEvaluated(t *testing.T) {
	t.Parallel()
	const rule = `rules:
  - id: a-not-to-catalog
    type: forbidden_dependency
    from: "pkg/a/**"
    to: "pkg/catalog/**"
    gate: fail
`
	for _, tc := range []struct {
		name   string
		guard  string
		want   report.HardGateState
		reason string
	}{
		{name: "renamed target", want: report.HardGateUnmeasured, reason: "selector matches nothing: to pkg/catalog/**"},
		{name: "declared guard", guard: "    guard: true\n", want: report.HardGatePass},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := checkDecision(t, writeRuleScopeRepo(t, ruleScopeModulesCfg+rule+tc.guard))
			if got.Decision.HardGates != string(tc.want) {
				t.Fatalf("hard_gates = %q, want %q: %+v", got.Decision.HardGates, tc.want, got.Decision)
			}
			if tc.reason == "" {
				if len(got.Decision.Unevaluated) != 0 {
					t.Fatalf("unevaluated_required_rules = %+v, want none", got.Decision.Unevaluated)
				}
				return
			}
			if len(got.Decision.Unevaluated) != 1 || got.Decision.Unevaluated[0].Reason != tc.reason {
				t.Fatalf("unevaluated_required_rules = %+v, want one with reason %q", got.Decision.Unevaluated, tc.reason)
			}
		})
	}
}

// TestRun_Check_DisclosesUnreadableConfigValues pins that values schema v2
// still loads but classification cannot read, and a warn-gated rule that can
// never fire, reach the user as config warnings instead of passing silently.
func TestRun_Check_DisclosesUnreadableConfigValues(t *testing.T) {
	t.Parallel()
	cfg := `version: 2
layers: [domain]
modules:
  a:
    paths: ["pkg/a/**"]
    owner: team
    volatility: hgih
    layer: domian
  b:
    paths: ["pkg/b/**"]
    owner: team
rules:
  - id: a-not-to-catalog
    type: forbidden_dependency
    from: "pkg/a/**"
    to: "pkg/catalog/**"
    gate: warn
`
	code, stdout, stderr := runArchfit(t, cmdCheck, "-c", writeRuleScopeRepo(t, cfg), flagRefresh, "--json")
	if code != 2 {
		t.Fatalf("check: exit = %d, want 2 (warnings never fail the load or the gate)\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	for _, want := range []string{
		`warning: modules.a.volatility: volatility "hgih" is not one of`,
		`warning: modules.a.layer: layer "domian" is not declared`,
		"warning: rules[a-not-to-catalog] is not evaluated: selector matches nothing: to pkg/catalog/**",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, stderr)
		}
	}
}

// TestRun_Check_RuleScopeHonorsDeclaredScope runs the real check over a Go repo
// with one stray tooling script inside a rule's source scope. The repo has no
// package.json, so the TypeScript extractor's own applicability probe finds no
// project and the script is out of dependency rule scope: the rule does not
// wait for dependency-cruiser. Only an explicit gate on the language, which
// demands to be told the producer did not run, keeps the script in scope.
func TestRun_Check_RuleScopeHonorsDeclaredScope(t *testing.T) {
	t.Parallel()
	const rule = `rules:
  - id: b-not-to-a
    type: forbidden_dependency
    from: "pkg/b/**"
    to: "pkg/a/**"
    gate: fail
`
	for _, tc := range []struct {
		name        string
		declaration string
		want        report.HardGateState
		unevaluated []string
	}{
		{name: "stray script without a typescript project", want: report.HardGatePass},
		{name: "explicit gate on the absent language", declaration: "languages:\n  typescript:\n    gate: warn\n",
			want: report.HardGateUnmeasured, unevaluated: []string{"b-not-to-a"}},
		{name: "excluded script", declaration: "exclude: [\"pkg/b/tools/**\"]\n", want: report.HardGatePass},
		{name: "switched-off language", declaration: "languages:\n  typescript:\n    enabled: false\n", want: report.HardGatePass},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfgPath := writeRuleScopeRepo(t, ruleScopeModulesCfg+tc.declaration+rule)
			writeFileAt(t, filepath.Dir(cfgPath), "pkg/b/tools/gen.cjs", "module.exports = {};\n")

			got := checkDecision(t, cfgPath)
			if got.Decision.HardGates != string(tc.want) {
				t.Fatalf("hard_gates = %q, want %q: %+v", got.Decision.HardGates, tc.want, got.Decision)
			}
			var ids []string
			for _, rule := range got.Decision.Unevaluated {
				ids = append(ids, rule.RuleID)
			}
			if !slices.Equal(ids, tc.unevaluated) {
				t.Fatalf("unevaluated_required_rules = %v, want %v", ids, tc.unevaluated)
			}
		})
	}
}
