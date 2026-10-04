package config

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/pattern"
	"github.com/alexei-led/archfit/internal/policy"
)

// globModuleA is the single-module path glob these projection fixtures share.
const globModuleA = "a/**"

func TestPolicySnapshotProjectsOwnershipAndDeployUnits(t *testing.T) {
	t.Parallel()
	cfg := Config{Version: 1, Modules: map[string]policy.ModuleDef{
		"cli": {Owner: "team-cli", DeployUnit: "archfit-cli", Paths: []string{"cmd/**"}},
	}}
	got := cfg.PolicySnapshot()
	if got.Ownership["cli"] != "team-cli" || got.DeployUnits["cli"] != "archfit-cli" {
		t.Fatalf("policy projection = %+v", got)
	}
}

func TestRunOptionsThreadsOneMergedExclusionSetThroughEveryStage(t *testing.T) {
	t.Parallel()
	options := Config{Exclude: []string{"generated/**", "!generated/keep.go"}}.RunOptions()
	goConfig := options.Extractors[LangGo]

	if !slices.Equal(goConfig.Exclusions, options.Exclusions) {
		t.Fatalf("Go extractor exclusions = %v, want shared merged exclusions %v", goConfig.Exclusions, options.Exclusions)
	}
	if !slices.Equal(options.Acquisition.Exclusions, options.Exclusions) {
		t.Fatalf("acquisition exclusions = %v, want shared merged exclusions %v", options.Acquisition.Exclusions, options.Exclusions)
	}
}

func TestValidateRulesRejectsUnknownRuleType(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".archfit.yaml")
	if err := os.WriteFile(path, []byte("version: 2\nrules:\n  - id: bad\n    type: bogus_type\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRules(cfg); err == nil || !strings.Contains(err.Error(), "unknown rule type") {
		t.Fatalf("want unknown-rule-type error, got %v", err)
	}
}

// TestWithIndependentModulesDoesNotAliasTheHeadModuleMap pins the base-tree
// isolation: Config is a value but its Modules map is shared, and the run's
// owner/deploy-unit backfill writes through it, so a base sub-run over an
// aliased map would inherit head-tree owners and never resolve its own.
func TestWithIndependentModulesDoesNotAliasTheHeadModuleMap(t *testing.T) {
	t.Parallel()
	head := Config{Modules: map[string]policy.ModuleDef{"a": {Paths: []string{globModuleA}}}}
	base := head.WithIndependentModules()
	base.Modules["a"] = policy.ModuleDef{Paths: []string{globModuleA}, Owner: "base-team"}
	if head.Modules["a"].Owner != "" {
		t.Fatalf("head module map mutated through the base copy: %+v", head.Modules["a"])
	}
}

// TestPreparerWarnsOnPatternsOutsideForbiddenPattern pins the v2.4 posture
// for rules[].patterns on any other rule type: the config still loads and the
// patterns still run, but analyze/check say the matches can never fire.
func TestPreparerWarnsOnPatternsOutsideForbiddenPattern(t *testing.T) {
	t.Parallel()
	patterns := []pattern.Def{{ID: "now", Lang: "go", Rule: "time.Now()"}}
	cfg := Config{Version: 2, Rules: []policy.RuleDef{
		{ID: "dead_patterns", Type: "forbidden_dependency", From: globModuleA, To: "b/**", Patterns: patterns},
		{ID: "live_patterns", Type: "forbidden_pattern", From: globModuleA,
			Patterns: []pattern.Def{{ID: "env", Lang: "go", Rule: "os.Environ()"}}},
	}}
	var buf strings.Builder
	if err := (Preparer{Config: cfg, Stderr: &buf, DiscloseLint: true}).Prepare(context.Background()); err != nil {
		t.Fatalf("patterns on another rule type must still load: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `warning: rule "dead_patterns"`) || !strings.Contains(out, "only evaluated by forbidden_pattern") {
		t.Errorf("stderr = %q, want a warning naming dead_patterns", out)
	}
	if strings.Contains(out, "live_patterns") {
		t.Errorf("stderr = %q, want no warning for the forbidden_pattern rule", out)
	}
	if got := len(cfg.ForPatterns()); got != 2 {
		t.Errorf("ForPatterns = %d patterns, want both rules' patterns still collected", got)
	}
}

// TestPreparerDisclosesLintOnlyWhenAsked pins the config-quality block to the
// analyze/check composition. Every other stage runs the same executor over a
// stderr stream it does not own — and `config compare` builds TWO preparers
// over one stream, so an ungated disclosure prints the block twice with nothing
// naming the side that produced it.
func TestPreparerDisclosesLintOnlyWhenAsked(t *testing.T) {
	t.Parallel()
	cfg := Config{Version: 1, Modules: map[string]policy.ModuleDef{
		"bare": {Paths: []string{globModuleA}}, // no owner, no volatility -> lint warns
	}}
	if len(cfg.Lint()) == 0 {
		t.Fatal("fixture must produce a lint warning")
	}
	for _, tc := range []struct {
		name     string
		disclose bool
		want     bool
	}{
		{"analyze/check", true, true},
		{"every other stage", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf strings.Builder
			if err := (Preparer{Config: cfg, Stderr: &buf, DiscloseLint: tc.disclose}).Prepare(context.Background()); err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			if got := strings.Contains(buf.String(), "config-quality"); got != tc.want {
				t.Errorf("disclosed = %t, want %t (stderr %q)", got, tc.want, buf.String())
			}
		})
	}
}
