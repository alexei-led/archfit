package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/testutil"
)

// rulesGoldens holds the text of each form of `archfit rules`. Regenerate
// with ARCHFIT_UPDATE_RULES=1 and read the diff.
const rulesGoldens = "testdata/rules"

// rulesPolicy adds layers, a public part, a note and an exception to the
// fixture policy, so each form has something to show.
const rulesPolicy = `version: 3
owners: [alexei-led]
layers: [core, adapters]
modules:
  pkg_chaos:
    paths: [pkg/chaos/**]
    public: [pkg/chaos]
    volatility: high
    layer: core
    depends_on: [pkg_runtime]
  pkg_runtime:
    paths: [pkg/runtime/**]
    volatility: low
    layer: adapters
  pkg_tc:
    paths: [pkg/tc/**]
    volatility: high
    layer: core
  pkg_util:
    paths: [pkg/util/**]
    subdomain: generic
rules:
  - id: no-chaos-to-runtime
    deny: { from: pkg_chaos, to: pkg_runtime }
    why: A direct import ties a chaos action to one runtime.
    instead: Add a method to container.Client.
  - id: fan-out-through-runner
    note: Chaos actions fan out through RunOnContainers.
    why: One place checks the target limit before injection.
` + expiredException

func TestRulesForms(t *testing.T) {
	t.Parallel()
	dir := fixtureRepo(t, with(fixtureFiles(), map[string]string{defaultConfigPath: rulesPolicy}),
		map[string]string{tcNetem: tcImportsUtil})
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "paths", args: []string{tcNetem, pkgUtil, "tools/gen.go"}},
		{name: "custom_rule", args: []string{"no-chaos-to-runtime"}},
		{name: "builtin_rule", args: []string{"duplication"}},
		{name: "changed_files", args: nil},
		{name: "graph", args: []string{"--graph"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := runCLI(t, fixtureDate, append([]string{cmdRules, "-C", dir}, tc.args...)...)
			if got.code != 0 || got.stderr != "" {
				t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
			}
			assertRulesGolden(t, tc.name, got.stdout)
		})
	}
}

func assertRulesGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join(rulesGoldens, name+".txt")
	if os.Getenv("ARCHFIT_UPDATE_RULES") != "" {
		if err := os.MkdirAll(rulesGoldens, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path) //nolint:gosec // test goldens
	if err != nil {
		t.Fatalf("%v (regenerate with ARCHFIT_UPDATE_RULES=1)", err)
	}
	if got != string(want) {
		t.Errorf("%s:\n--- got\n%s--- want\n%s", path, got, want)
	}
}

// TestNewCommandsUseNoNotUsedWord holds the CLI goldens and the help of
// check, can and rules to the words of design section 5.
func TestNewCommandsUseNoNotUsedWord(t *testing.T) {
	t.Parallel()
	texts := map[string]string{}
	for _, cmd := range []string{cmdCheck, cmdCan, cmdRules} {
		got := runCLI(t, fixtureDate, cmd, "--help")
		if got.code != 0 || got.stdout == "" {
			t.Fatalf("%s --help: exit %d, stdout %q", cmd, got.code, got.stdout)
		}
		texts[cmd+" --help"] = got.stdout
	}
	for _, dir := range []string{rulesGoldens, designExamples} {
		goldens, err := filepath.Glob(filepath.Join(dir, "*.txt"))
		if err != nil || len(goldens) == 0 {
			t.Fatalf("no golden in %s: %v", dir, err)
		}
		for _, path := range goldens {
			data, err := os.ReadFile(path) //nolint:gosec // test goldens
			if err != nil {
				t.Fatal(err)
			}
			texts[path] = string(data)
		}
	}
	for name, text := range texts {
		if words := testutil.NotUsedIn(text); len(words) > 0 {
			t.Errorf("%s uses %s:\n%s", name, strings.Join(words, ", "), text)
		}
	}
}
