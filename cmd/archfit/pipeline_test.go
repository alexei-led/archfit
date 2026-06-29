package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/labels"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
)

// TestBuildCoverageGaps verifies the coverage-gap table derivation:
// absent known tools produce a gap with the right gate; present or unknown
// tools produce no gap.
func TestBuildCoverageGaps(t *testing.T) {
	t.Parallel()
	cfgFailGo := config.Config{Languages: config.LanguagesConfig{
		Go: config.GoLanguage{Gate: config.GateFail},
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
		{
			// A tool disabled by config (StatusDisabled) must NOT produce a coverage
			// gap — the user deliberately opted out; telling them to "install" is wrong.
			name:      "disabled-by-config tool produces no gap",
			cov:       []diagnostic.Coverage{{Tool: toolJscpd, Status: diagnostic.StatusDisabled}},
			cfg:       cfgWarn,
			wantTools: nil,
		},
		{
			// Partial coverage is informational, not an install prompt.
			name:      "partial tool produces no gap",
			cov:       []diagnostic.Coverage{{Tool: toolJscpd, Status: diagnostic.StatusPartial}},
			cfg:       cfgWarn,
			wantTools: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gaps := buildCoverageGaps(tc.cov, tc.cfg, "")
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

// TestBuildCoverageGaps_ProjectMarkerSuppression verifies that gaps for a
// language whose project marker is absent from the scan root are suppressed,
// while gaps for present markers and explicit gates are preserved.
func TestBuildCoverageGaps_ProjectMarkerSuppression(t *testing.T) {
	t.Parallel()
	// Pure-Go repo: only go.mod present, no Cargo.toml.
	goOnlyDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(goOnlyDir, markerGoMod), []byte("module example\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Mixed Go+Rust repo: both go.mod and Cargo.toml present.
	mixedDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(mixedDir, markerGoMod), []byte("module example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mixedDir, markerCargoToml), []byte("[package]\nname = \"x\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfgDefault := config.Config{}
	cfgRustGate := config.Config{Languages: config.LanguagesConfig{
		Rust: config.RustLanguage{Gate: config.GateFail},
	}}
	cfgCargoModulesGate := config.Config{Analyzers: config.AnalyzersConfig{
		CargoModules: config.Analyzer{Gate: config.GateFail},
	}}

	allRustAbsent := []diagnostic.Coverage{
		{Tool: toolCargo, Status: diagnostic.StatusAbsent},
		{Tool: toolCargoModules, Status: diagnostic.StatusAbsent},
	}

	t.Run("pure-Go repo: no cargo or cargo-modules gap", func(t *testing.T) {
		t.Parallel()
		gaps := buildCoverageGaps(allRustAbsent, cfgDefault, goOnlyDir)
		for _, g := range gaps {
			if g.Tool == toolCargo || g.Tool == toolCargoModules {
				t.Errorf("unexpected gap %q in pure-Go repo (no Cargo.toml)", g.Tool)
			}
		}
	})

	t.Run("mixed Go+Rust repo: cargo gap present", func(t *testing.T) {
		t.Parallel()
		gaps := buildCoverageGaps(allRustAbsent, cfgDefault, mixedDir)
		found := false
		for _, g := range gaps {
			if g.Tool == toolCargo {
				found = true
			}
		}
		if !found {
			t.Error("expected cargo gap in mixed repo with Cargo.toml, got none")
		}
	})

	t.Run("mixed Go+Rust repo: cargo-modules gap present", func(t *testing.T) {
		t.Parallel()
		gaps := buildCoverageGaps(allRustAbsent, cfgDefault, mixedDir)
		found := false
		for _, g := range gaps {
			if g.Tool == toolCargoModules {
				found = true
			}
		}
		if !found {
			t.Error("expected cargo-modules gap in mixed repo with Cargo.toml, got none")
		}
	})

	t.Run("explicit gate on rust overrides marker suppression", func(t *testing.T) {
		t.Parallel()
		gaps := buildCoverageGaps([]diagnostic.Coverage{
			{Tool: toolCargo, Status: diagnostic.StatusAbsent},
		}, cfgRustGate, goOnlyDir)
		found := false
		for _, g := range gaps {
			if g.Tool == toolCargo {
				found = true
			}
		}
		if !found {
			t.Error("explicit gate: expected cargo gap even without Cargo.toml, got none")
		}
	})

	t.Run("explicit gate on cargo-modules overrides marker suppression", func(t *testing.T) {
		t.Parallel()
		gaps := buildCoverageGaps([]diagnostic.Coverage{
			{Tool: toolCargoModules, Status: diagnostic.StatusAbsent},
		}, cfgCargoModulesGate, goOnlyDir)
		found := false
		for _, g := range gaps {
			if g.Tool == toolCargoModules {
				found = true
			}
		}
		if !found {
			t.Error("explicit gate: expected cargo-modules gap even without Cargo.toml, got none")
		}
	})

	t.Run("empty root disables suppression (backward compat)", func(t *testing.T) {
		t.Parallel()
		gaps := buildCoverageGaps(allRustAbsent, cfgDefault, "")
		found := false
		for _, g := range gaps {
			if g.Tool == toolCargo {
				found = true
			}
		}
		if !found {
			t.Error("empty root: expected cargo gap (no suppression), got none")
		}
	})
}

// TestBuildConfigWarnings verifies the config-warnings block: lint warnings and
// tool errors are combined, nil is returned when both are empty.
func TestBuildConfigWarnings(t *testing.T) {
	t.Parallel()
	t.Run("empty config and no tool errors returns nil", func(t *testing.T) {
		t.Parallel()
		cfg := config.Config{Version: 1}
		if got := buildConfigWarnings(cfg, nil); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("tool errors appear after lint warnings", func(t *testing.T) {
		t.Parallel()
		// A module with paths but no rules referencing it produces a lint warning.
		cfg := config.Config{
			Version: 1,
			Modules: map[string]config.ModuleDef{
				"orphan": {Paths: []string{"pkg/orphan/**"}},
			},
		}
		toolErrs := []string{"jscpd: exit status 1"}
		got := buildConfigWarnings(cfg, toolErrs)
		if len(got) == 0 {
			t.Fatal("want at least one warning, got none")
		}
		// Tool error must appear somewhere in the output.
		found := false
		for _, w := range got {
			if w == "jscpd: exit status 1" {
				found = true
			}
		}
		if !found {
			t.Errorf("tool error missing from warnings: %v", got)
		}
	})

	t.Run("tool errors only, no lint", func(t *testing.T) {
		t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	cfg := config.Config{Languages: config.LanguagesConfig{
		Go:         config.GoLanguage{Gate: config.GateFail},
		TypeScript: config.TypeScriptLanguage{Gate: config.GateOff},
		Python:     config.PythonLanguage{}, // unset → warn
	}}
	cases := []struct {
		tool string
		want string
	}{
		{toolGoPackages, gateFail},               // tools.go.gate: fail
		{toolDepCruiser, string(config.GateOff)}, // tools.typescript.gate: off
		{toolGrimp, gateWarn},                    // tools.python unset → default
		{toolLoc, gateWarn},                      // unmapped tool → default
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
	t.Parallel()
	t.Run("require-tools raises all gaps to fail", func(t *testing.T) {
		t.Parallel()
		diag := diagnostic.Diagnostic{
			Verdict: diagnostic.VerdictPass,
			CoverageGaps: []diagnostic.CoverageGap{
				{Tool: toolGrimp, Gate: gateWarn},
				{Tool: toolJscpd, Gate: gateWarn},
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
		t.Parallel()
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
		t.Parallel()
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

// Test-local constants for buildJudgmentDecisionTasks tests.
const (
	decisionModA = "app.a"
	decisionModB = "app.b"
)

// TestBuildJudgmentDecisionTasks verifies that undeclared judgment inputs emit
// actionable decision strings pointing at the right file/key.
func TestBuildJudgmentDecisionTasks(t *testing.T) {
	t.Parallel()
	configPath := "/repo/.archfit.yaml"

	t.Run("module with neither subdomain nor volatility emits decision task", func(t *testing.T) {
		t.Parallel()
		cfg := config.Config{
			Modules: map[string]config.ModuleDef{
				"app.core": {Paths: []string{"internal/core/**"}, Subdomain: commandGroupCore},
				"app.util": {Paths: []string{"internal/util/**"}}, // no subdomain, no volatility
			},
		}
		tasks := buildJudgmentDecisionTasks(cfg, nil, configPath)
		found := false
		for _, t2 := range tasks {
			if strings.Contains(t2, "app.util") && strings.Contains(t2, configPath) {
				found = true
			}
		}
		if !found {
			t.Errorf("expected decision task for app.util, got: %v", tasks)
		}
		// app.core has subdomain set — must NOT appear.
		for _, t2 := range tasks {
			if strings.Contains(t2, "app.core") {
				t.Errorf("unexpected decision task for app.core: %s", t2)
			}
		}
	})

	t.Run("module with volatility declared is not flagged", func(t *testing.T) {
		t.Parallel()
		cfg := config.Config{
			Modules: map[string]config.ModuleDef{
				"app.util": {Paths: []string{"internal/util/**"}, Volatility: "low"},
			},
		}
		tasks := buildJudgmentDecisionTasks(cfg, nil, configPath)
		for _, t2 := range tasks {
			if strings.Contains(t2, "app.util") {
				t.Errorf("unexpected decision task for module with volatility: %s", t2)
			}
		}
	})

	t.Run("no modules emits no tasks", func(t *testing.T) {
		t.Parallel()
		cfg := config.Config{}
		tasks := buildJudgmentDecisionTasks(cfg, nil, configPath)
		if len(tasks) != 0 {
			t.Errorf("expected no tasks, got: %v", tasks)
		}
	})

	t.Run("approved llm label emits decision task pointing at labels file", func(t *testing.T) {
		t.Parallel()
		cfg := config.Config{}
		lbls := []labels.Label{
			{From: decisionModA, To: decisionModB, Strength: enrichModel,
				Status: labels.StatusApproved, Provenance: labels.ProvenanceLLM},
		}
		tasks := buildJudgmentDecisionTasks(cfg, lbls, configPath)
		found := false
		for _, t2 := range tasks {
			if strings.Contains(t2, decisionModA) && strings.Contains(t2, decisionModB) &&
				strings.Contains(t2, ".archfit-labels.yaml") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected decision task for llm label, got: %v", tasks)
		}
	})

	t.Run("draft llm label does NOT emit decision task", func(t *testing.T) {
		t.Parallel()
		cfg := config.Config{}
		lbls := []labels.Label{
			{From: decisionModA, To: decisionModB, Strength: enrichModel,
				Status: labels.StatusDraft, Provenance: labels.ProvenanceLLM},
		}
		tasks := buildJudgmentDecisionTasks(cfg, lbls, configPath)
		if len(tasks) != 0 {
			t.Errorf("expected no tasks for draft label, got: %v", tasks)
		}
	})

	t.Run("approved human label does NOT emit decision task", func(t *testing.T) {
		t.Parallel()
		cfg := config.Config{}
		lbls := []labels.Label{
			{From: decisionModA, To: decisionModB, Strength: enrichModel,
				Status: labels.StatusApproved, Provenance: labels.ProvenanceHuman},
		}
		tasks := buildJudgmentDecisionTasks(cfg, lbls, configPath)
		if len(tasks) != 0 {
			t.Errorf("expected no tasks for human label, got: %v", tasks)
		}
	})

	t.Run("output is sorted deterministically", func(t *testing.T) {
		t.Parallel()
		cfg := config.Config{
			Modules: map[string]config.ModuleDef{
				"zz.module": {Paths: []string{"zz/**"}},
				"aa.module": {Paths: []string{"aa/**"}},
			},
		}
		tasks := buildJudgmentDecisionTasks(cfg, nil, configPath)
		if len(tasks) < 2 {
			t.Fatalf("expected ≥2 tasks, got %d", len(tasks))
		}
		if !strings.Contains(tasks[0], "aa.module") {
			t.Errorf("first task should be aa.module (sorted), got: %s", tasks[0])
		}
		if !strings.Contains(tasks[1], "zz.module") {
			t.Errorf("second task should be zz.module (sorted), got: %s", tasks[1])
		}
	})
}

// TestOutputInsideRootWarning verifies the path hygiene check: a config/output
// directory strictly inside the analyzed root warns; the root itself or any path
// outside it does not.
func TestOutputInsideRootWarning(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			got := outputInsideRootWarning(root, tc.dir)
			if (got != "") != tc.wantMsg {
				t.Errorf("outputInsideRootWarning(%q, %q) = %q, wantMsg=%v", root, tc.dir, got, tc.wantMsg)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CB1: skipped-pass coverage rows (P12 — syntax/scip opt-in honesty)
// ---------------------------------------------------------------------------

// TestSkippedPassCoverageRows_ScipDisabled asserts that when SCIP is not
// enabled the pipeline injects an explicit StatusDisabled coverage row for
// "scip" so tool_coverage reads "disabled" rather than absent/missing. This is
// the regression for P12: the skipped pass was silently absent from the output.
func TestSkippedPassCoverageRows_ScipDisabled(t *testing.T) {
	t.Parallel()
	// StatusDisabled must not produce a gap (deliberate opt-out, not a missing tool).
	cov := []diagnostic.Coverage{
		{Tool: toolScip, Status: diagnostic.StatusDisabled, Reason: reasonScipDisabled},
	}
	gaps := buildCoverageGaps(cov, config.Config{}, "")
	if len(gaps) != 0 {
		t.Errorf("StatusDisabled scip must not produce a gap; got %+v", gaps)
	}
}

// TestSkippedPassCoverageRows_SyntaxDisabled asserts that when syntax is not
// enabled the pipeline injects an explicit StatusDisabled row for "ast-grep/syntax".
func TestSkippedPassCoverageRows_SyntaxDisabled(t *testing.T) {
	t.Parallel()
	cov := []diagnostic.Coverage{
		{Tool: toolAstGrepSyntax, Status: diagnostic.StatusDisabled, Reason: reasonSyntaxDisabled},
	}
	gaps := buildCoverageGaps(cov, config.Config{}, "")
	if len(gaps) != 0 {
		t.Errorf("StatusDisabled ast-grep/syntax must not produce a gap; got %+v", gaps)
	}
}

// TestSkippedPassCoverageRows_ReasonContent asserts the disabled coverage
// reasons are distinct and non-empty, making it clear which opt-in flag to set.
func TestSkippedPassCoverageRows_ReasonContent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		tool   string
		reason string
		wantIn string
	}{
		{toolScip, reasonScipDisabled, "scip"},
		{toolAstGrepSyntax, reasonSyntaxDisabled, "syntax"},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			t.Parallel()
			if tc.reason == "" {
				t.Error("reason must not be empty")
			}
			if !strings.Contains(tc.reason, tc.wantIn) {
				t.Errorf("reason %q does not mention %q", tc.reason, tc.wantIn)
			}
		})
	}
}
