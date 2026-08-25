package config

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/policy"
)

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
	if err := os.WriteFile(path, []byte("version: 1\nrules:\n  - id: bad\n    type: bogus_type\n"), 0o600); err != nil {
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
	head := Config{Modules: map[string]policy.ModuleDef{"a": {Paths: []string{"a/**"}}}}
	base := head.WithIndependentModules()
	base.Modules["a"] = policy.ModuleDef{Paths: []string{"a/**"}, Owner: "base-team"}
	if head.Modules["a"].Owner != "" {
		t.Fatalf("head module map mutated through the base copy: %+v", head.Modules["a"])
	}
}
