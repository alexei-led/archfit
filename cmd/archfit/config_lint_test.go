package main

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

func TestConfigLint_ExitCodesAndOutput(t *testing.T) {
	t.Parallel()
	const live = `rules:
  - id: b-not-to-a
    type: forbidden_dependency
    from: "pkg/b/**"
    to: "pkg/a/**"
    gate: fail
    rationale: "b is the leaf; a reaching it back would close a cycle"
`
	const unexplained = `rules:
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
    rationale: "the catalog is private to its team"
`
	for _, tc := range []struct {
		name     string
		cfg      string
		args     []string
		wantCode int
		wantText string
	}{
		{name: "clean config", cfg: ruleScopeModulesCfg + live, wantCode: 0},
		{
			name: "blocking rule without a rationale warns", cfg: ruleScopeModulesCfg + unexplained, wantCode: 0,
			wantText: "warning missing_rationale rules[b-not-to-a].rationale: blocking rule has no rationale: say which consequence it prevents, so the agent fixes the cause\n",
		},
		{
			name: "strict makes it an error", cfg: ruleScopeModulesCfg + unexplained, args: []string{"--strict"}, wantCode: 1,
			wantText: "error missing_rationale rules[b-not-to-a].rationale: blocking rule has no rationale: say which consequence it prevents, so the agent fixes the cause\n",
		},
		{name: "strict passes a rationale", cfg: ruleScopeModulesCfg + live, args: []string{"--strict"}, wantCode: 0},
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
			code, stdout, stderr := runArchfit(t, append([]string{"config", "lint", "-c", cfgPath}, tc.args...)...)
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
    rationale: "the catalog is private to its team"
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
    rationale: "SQL belongs in the database module"
  - id: schema_typo
    type: forbidden_dependency
    from: "app/**"
    to: "database/shcema/**"
    gate: fail
    rationale: "the schema is private to the database module"
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

// TestSelfConfigBlockingRulesHaveRationales: archfit's own policy says why
// each blocking rule exists, so an agent that hits one fixes the cause.
func TestSelfConfigBlockingRulesHaveRationales(t *testing.T) {
	t.Parallel()
	cfg := filepath.Join("..", "..", defaultConfigPath)
	code, stdout, stderr := runArchfit(t, "config", "lint", "-c", cfg, "--strict")
	if code != 0 || strings.Contains(stdout, "missing_rationale") {
		t.Fatalf("config lint --strict on the self config: exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}

// TestSelfConfigAlternativesAreImportable: an alternative that names a package
// is advice an agent follows with an import. Judge each one by the policy that
// judges the import: the package must not be denied from where the rule's own
// importers live.
func TestSelfConfigAlternativesAreImportable(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	raw, err := os.ReadFile(filepath.Join(root, defaultConfigPath)) //nolint:gosec // the repository policy file, a constant path
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Rules []struct {
			ID           string   `yaml:"id"`
			From         string   `yaml:"from"`
			Alternatives []string `yaml:"alternatives"`
		} `yaml:"rules"`
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	// Only internal packages are import destinations. cmd/archfit appears in
	// advice as the place to wire an adapter; a file reference is where to add
	// code, not what to import.
	pkgRef := regexp.MustCompile(`internal/[a-z0-9_/]+(\.go)?`)
	checked := 0
	for _, rule := range cfg.Rules {
		importer := firstGoFileUnder(t, root, rule.From)
		for _, alt := range rule.Alternatives {
			for _, target := range pkgRef.FindAllString(alt, -1) {
				if strings.HasSuffix(target, ".go") || importer == "" {
					continue
				}
				target = strings.TrimSuffix(target, "/")
				checked++
				code, stdout, stderr := runArchfit(t, "policy", "can-import", importer, target, "-c", filepath.Join(root, defaultConfigPath))
				if code == 1 || strings.Contains(stdout, `"answer":"denied"`) {
					t.Errorf("rule %s: alternative %q sends %s to %s, which the policy denies (exit %d)\n%s%s", rule.ID, alt, importer, target, code, stdout, stderr)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no alternative named a package: the check is vacuous")
	}
}

// firstGoFileUnder returns a production Go file under the literal directory a
// rule selector starts with, relative to root, or "" when none is found.
func firstGoFileUnder(t *testing.T, root, selector string) string {
	t.Helper()
	dir := selector
	if open := strings.IndexAny(dir, "{*"); open >= 0 {
		dir = dir[:open]
	}
	if strings.HasSuffix(selector, ".go") && !strings.ContainsAny(selector, "{*") {
		return selector
	}
	dir = strings.TrimSuffix(dir, "/")
	if dir == "" {
		return ""
	}
	var found string
	_ = filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			rel, _ := filepath.Rel(root, path)
			found = filepath.ToSlash(rel)
		}
		return nil
	})
	return found
}
