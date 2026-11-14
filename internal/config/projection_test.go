package config

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/policy"
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
