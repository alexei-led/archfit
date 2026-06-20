package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
)

// TestBuildCoverageGaps verifies the coverage-gap table derivation:
// absent known tools produce a gap with the right gate; present or unknown
// tools produce no gap.
func TestBuildCoverageGaps(t *testing.T) {
	cfgFailGo := config.Config{Tools: config.ToolsConfig{
		config.LangGo: {Gate: config.GateFail},
	}}
	cfgWarn := config.Config{}

	cases := []struct {
		name      string
		cov       []diagnostic.Coverage
		cfg       config.Config
		wantTools []string // tool names in expected gap output (empty = no gaps)
		wantGate  string   // gate for first gap (when wantTools non-empty)
	}{
		{
			name:      "absent known tool produces gap",
			cov:       []diagnostic.Coverage{{Tool: toolGoPackages, Status: diagnostic.StatusAbsent}},
			cfg:       cfgWarn,
			wantTools: []string{toolGoPackages},
			wantGate:  gateWarn,
		},
		{
			name:      "absent tool with configured fail gate",
			cov:       []diagnostic.Coverage{{Tool: toolGoPackages, Status: diagnostic.StatusAbsent}},
			cfg:       cfgFailGo,
			wantTools: []string{toolGoPackages},
			wantGate:  gateFail,
		},
		{
			name:      "present tool produces no gap",
			cov:       []diagnostic.Coverage{{Tool: toolGoPackages, Status: diagnostic.StatusOK}},
			cfg:       cfgWarn,
			wantTools: nil,
		},
		{
			name:      "unknown tool produces no gap",
			cov:       []diagnostic.Coverage{{Tool: "unknown-tool", Status: diagnostic.StatusAbsent}},
			cfg:       cfgWarn,
			wantTools: nil,
		},
		{
			name: "multiple absent tools sorted by name",
			cov: []diagnostic.Coverage{
				{Tool: toolGrimp, Status: diagnostic.StatusAbsent},
				{Tool: toolGoPackages, Status: diagnostic.StatusAbsent},
				{Tool: toolJscpd, Status: diagnostic.StatusAbsent},
			},
			cfg:       cfgWarn,
			wantTools: []string{toolGoPackages, toolGrimp, toolJscpd},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gaps := buildCoverageGaps(tc.cov, tc.cfg)
			if len(gaps) != len(tc.wantTools) {
				t.Fatalf("gaps = %d, want %d: %+v", len(gaps), len(tc.wantTools), gaps)
			}
			for i, g := range gaps {
				if g.Tool != tc.wantTools[i] {
					t.Errorf("gap[%d].Tool = %q, want %q", i, g.Tool, tc.wantTools[i])
				}
			}
			if tc.wantGate != "" && len(gaps) > 0 && gaps[0].Gate != tc.wantGate {
				t.Errorf("gap[0].Gate = %q, want %q", gaps[0].Gate, tc.wantGate)
			}
		})
	}
}

// TestBuildConfigWarnings verifies the config-warnings block: lint warnings and
// tool errors are combined, nil is returned when both are empty.
func TestBuildConfigWarnings(t *testing.T) {
	t.Run("empty config and no tool errors returns nil", func(t *testing.T) {
		cfg := config.Config{Version: 1}
		if got := buildConfigWarnings(cfg, nil); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("tool errors appear after lint warnings", func(t *testing.T) {
		// A module with paths but no rules referencing it produces a lint warning.
		cfg := config.Config{
			Version: 1,
			Modules: map[string]config.ModuleDef{
				"orphan": {Paths: []string{"pkg/orphan/**"}},
			},
		}
		toolErrs := []string{"lizard: exit status 1"}
		got := buildConfigWarnings(cfg, toolErrs)
		if len(got) == 0 {
			t.Fatal("want at least one warning, got none")
		}
		// Tool error must appear somewhere in the output.
		found := false
		for _, w := range got {
			if w == "lizard: exit status 1" {
				found = true
			}
		}
		if !found {
			t.Errorf("tool error missing from warnings: %v", got)
		}
	})

	t.Run("tool errors only, no lint", func(t *testing.T) {
		cfg := config.Config{Version: 1}
		toolErrs := []string{"jscpd: not found"}
		got := buildConfigWarnings(cfg, toolErrs)
		if len(got) != 1 || got[0] != "jscpd: not found" {
			t.Errorf("got %v, want [jscpd: not found]", got)
		}
	})
}

// TestEffectiveConfigHash verifies that --no-config never hashes the on-disk
// config file: a run that ignored the file must report no hash, even when the
// file exists, so the hash never reflects (or changes with) an ignored file.
func TestEffectiveConfigHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".archfit.yaml")
	if err := os.WriteFile(path, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// noConfig=true: file present but ignored → empty hash.
	if got := effectiveConfigHash(path, true); got != "" {
		t.Errorf("effectiveConfigHash(existing, noConfig=true) = %q, want \"\"", got)
	}

	// noConfig=false: file present and read → non-empty hash.
	withCfg := effectiveConfigHash(path, false)
	if withCfg == "" {
		t.Error("effectiveConfigHash(existing, noConfig=false) = \"\", want a hash")
	}

	// noConfig=false but file absent → empty hash (never fails on missing config).
	if got := effectiveConfigHash(filepath.Join(dir, "absent.yaml"), false); got != "" {
		t.Errorf("effectiveConfigHash(absent, noConfig=false) = %q, want \"\"", got)
	}

	// Mutating the ignored file must NOT change the no-config result (stays empty).
	if err := os.WriteFile(path, []byte("version: 1\nmodules: {}\n"), 0o600); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}
	if got := effectiveConfigHash(path, true); got != "" {
		t.Errorf("effectiveConfigHash after mutating ignored file = %q, want \"\"", got)
	}
}

// TestConfigToolGate verifies the coverage-tool → config-key gate resolution:
// an unmapped tool and an empty gate both default to warn; a configured gate on
// the mapped key (e.g. tools.go.gate for go/packages) is surfaced verbatim.
func TestConfigToolGate(t *testing.T) {
	cfg := config.Config{Tools: config.ToolsConfig{
		config.LangGo:         {Gate: config.GateFail},
		config.LangTypeScript: {Gate: config.GateOff},
		config.LangPython:     {}, // unset → warn
	}}
	cases := []struct {
		tool string
		want string
	}{
		{toolGoPackages, gateFail},               // tools.go.gate: fail
		{toolDepCruiser, string(config.GateOff)}, // tools.typescript.gate: off
		{toolGrimp, gateWarn},                    // tools.python unset → default
		{"loc", gateWarn},                        // unmapped tool → default
		{toolGitnexus, gateWarn},                 // mapped key present but unset
	}
	for _, tc := range cases {
		if got := configToolGate(cfg, tc.tool); got != tc.want {
			t.Errorf("configToolGate(%q) = %q, want %q", tc.tool, got, tc.want)
		}
	}
}

// TestApplyToolGate verifies the hard-gate decision: --require-tools raises every
// gap to fail and stamps the verdict; an explicit per-tool fail gate trips without
// the flag; an all-warn run with no flag does not trip and leaves the verdict alone.
func TestApplyToolGate(t *testing.T) {
	t.Run("require-tools raises all gaps to fail", func(t *testing.T) {
		diag := diagnostic.Diagnostic{
			Verdict: diagnostic.VerdictPass,
			CoverageGaps: []diagnostic.CoverageGap{
				{Tool: toolGrimp, Gate: gateWarn},
				{Tool: toolLizard, Gate: gateWarn},
			},
		}
		if !applyToolGate(&diag, true) {
			t.Fatal("applyToolGate(require=true) = false, want true (hard gate)")
		}
		if diag.Verdict != diagnostic.VerdictFail {
			t.Errorf("verdict = %q, want fail", diag.Verdict)
		}
		for _, g := range diag.CoverageGaps {
			if g.Gate != gateFail {
				t.Errorf("gap %q gate = %q, want fail", g.Tool, g.Gate)
			}
		}
	})

	t.Run("explicit fail gate trips without the flag", func(t *testing.T) {
		diag := diagnostic.Diagnostic{
			Verdict: diagnostic.VerdictPass,
			CoverageGaps: []diagnostic.CoverageGap{
				{Tool: toolGrimp, Gate: gateWarn},
				{Tool: toolGoPackages, Gate: gateFail},
			},
		}
		if !applyToolGate(&diag, false) {
			t.Fatal("applyToolGate with a fail gap = false, want true")
		}
		if diag.Verdict != diagnostic.VerdictFail {
			t.Errorf("verdict = %q, want fail", diag.Verdict)
		}
		if diag.CoverageGaps[0].Gate != gateWarn {
			t.Errorf("warn gap was mutated to %q without --require-tools", diag.CoverageGaps[0].Gate)
		}
	})

	t.Run("all warn, no flag, does not trip", func(t *testing.T) {
		diag := diagnostic.Diagnostic{
			Verdict: diagnostic.VerdictPass,
			CoverageGaps: []diagnostic.CoverageGap{
				{Tool: toolGrimp, Gate: gateWarn},
			},
		}
		if applyToolGate(&diag, false) {
			t.Fatal("applyToolGate(all warn, require=false) = true, want false")
		}
		if diag.Verdict != diagnostic.VerdictPass {
			t.Errorf("verdict = %q, want pass (unchanged)", diag.Verdict)
		}
	})
}

// TestOutputInsideRootWarning verifies the path hygiene check: a config/output
// directory strictly inside the analyzed root warns; the root itself or any path
// outside it does not.
func TestOutputInsideRootWarning(t *testing.T) {
	root := filepath.FromSlash("/repo")
	cases := []struct {
		name    string
		dir     string
		wantMsg bool
	}{
		{"root itself is fine", filepath.FromSlash("/repo"), false},
		{"subdir inside root warns", filepath.FromSlash("/repo/reports"), true},
		{"nested subdir warns", filepath.FromSlash("/repo/a/b"), true},
		{"sibling outside root is fine", filepath.FromSlash("/other"), false},
		{"parent of root is fine", filepath.FromSlash("/"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := outputInsideRootWarning(root, tc.dir)
			if (got != "") != tc.wantMsg {
				t.Errorf("outputInsideRootWarning(%q, %q) = %q, wantMsg=%v", root, tc.dir, got, tc.wantMsg)
			}
		})
	}
}
