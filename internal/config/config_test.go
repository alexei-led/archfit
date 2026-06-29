package config_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
)

func TestLoad_Valid(t *testing.T) {
	cfg, err := config.Load(context.Background(), "testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Version != 1 {
		t.Errorf("Version = %d, want 1", cfg.Version)
	}
	if len(cfg.Modules) != 2 {
		t.Errorf("len(Modules) = %d, want 2", len(cfg.Modules))
	}
	if len(cfg.Layers) != 3 {
		t.Errorf("len(Layers) = %d, want 3", len(cfg.Layers))
	}
	if len(cfg.Rules) != 2 {
		t.Errorf("len(Rules) = %d, want 2", len(cfg.Rules))
	}
	if len(cfg.Exclude) != 2 {
		t.Errorf("len(Exclude) = %d, want 2", len(cfg.Exclude))
	}
	if len(cfg.Waivers) != 1 {
		t.Errorf("len(Waivers) = %d, want 1", len(cfg.Waivers))
	}

	// Verify language/analyzer modes (true→on, auto stays auto).
	if got := cfg.Languages.Go.Enabled; got != config.ModeOn {
		t.Errorf("languages.go.enabled = %q, want %q", got, config.ModeOn)
	}
	if got := cfg.Languages.TypeScript.Enabled; got != config.ModeAuto {
		t.Errorf("languages.typescript.enabled = %q, want %q", got, config.ModeAuto)
	}
	if got := cfg.Analyzers.Clones.Enabled; got != config.ModeAuto {
		t.Errorf("analyzers.clones.enabled = %q, want %q", got, config.ModeAuto)
	}

	// Verify module details.
	checkout, ok := cfg.Modules["checkout"]
	if !ok {
		t.Error("module checkout not found")
	} else {
		if checkout.Owner != "team-checkout" {
			t.Errorf("checkout.Owner = %q, want %q", checkout.Owner, "team-checkout")
		}
		if checkout.DeployUnit != "web-api" {
			t.Errorf("checkout.DeployUnit = %q, want %q", checkout.DeployUnit, "web-api")
		}
	}

	// Verify outputs.
	if !cfg.Outputs.JSON {
		t.Error("outputs.json = false, want true")
	}
	if cfg.Outputs.Markdown {
		t.Error("outputs.markdown = true, want false")
	}
}

func TestLoad_UnknownField(t *testing.T) {
	_, err := config.Load(context.Background(), "testdata/unknown_field.yaml")
	if err == nil {
		t.Fatal("Load: expected error for unknown field, got nil")
	}
	if !strings.Contains(err.Error(), "unknown") && !strings.Contains(err.Error(), "field") {
		t.Errorf("error %q does not mention unknown field", err.Error())
	}
}

func TestLoad_MissingVersion(t *testing.T) {
	_, err := config.Load(context.Background(), "testdata/missing_version.yaml")
	if err == nil {
		t.Fatal("Load: expected error for missing version, got nil")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Errorf("error %q does not mention version", err.Error())
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := config.Load(context.Background(), "testdata/nonexistent.yaml")
	if err == nil {
		t.Fatal("Load: expected error for missing file, got nil")
	}
}

func TestModuleFor(t *testing.T) {
	cfg, err := config.Load(context.Background(), "testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	mm := cfg.ModuleMapView()

	tests := []struct {
		path   string
		want   string
		wantOK bool
	}{
		{"services/checkout/handler.go", "checkout", true},
		{"services/checkout/deep/nested/file.go", "checkout", true},
		{"services/pricing/contracts/api.go", "pricing", true},
		{"services/pricing/internal/impl.go", "pricing", true},
		{"cmd/main.go", "", false},
		{"", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got, ok := mm.ModuleFor(tc.path)
			if ok != tc.wantOK {
				t.Errorf("ModuleFor(%q): ok=%v, want %v", tc.path, ok, tc.wantOK)
			}
			if got != tc.want {
				t.Errorf("ModuleFor(%q): module=%q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestModuleFor_Deterministic(t *testing.T) {
	// Two modules whose globs could overlap — same path must always return the
	// same (alphabetically-first) module name.
	cfg := config.Config{
		Version: 1,
		Modules: map[string]config.ModuleDef{
			"beta":  {Paths: []string{"shared/**"}},
			"alpha": {Paths: []string{"shared/**"}},
		},
	}
	mm := cfg.ModuleMapView()

	const path = "shared/util.go"
	first, _ := mm.ModuleFor(path)
	for range 10 {
		got, _ := mm.ModuleFor(path)
		if got != first {
			t.Errorf("ModuleFor(%q) is non-deterministic: got %q then %q", path, first, got)
		}
	}
	// Alpha sorts before beta — alpha wins.
	if first != "alpha" {
		t.Errorf("ModuleFor(%q) = %q, want alpha (alphabetically first)", path, first)
	}
}

// TestModuleFor_MostSpecific guards the catch-all shadowing regression: a broad
// fallback glob (internal/**) must never shadow a specific module
// (internal/model/**), even though the catch-all sorts first alphabetically.
// Resolution is most-specific-match, so the specific stanza wins where it
// applies and the catch-all only absorbs paths nothing more specific matches.
func TestModuleFor_MostSpecific(t *testing.T) {
	const catchAll = "internal" // broad fallback stanza; repeated below
	cfg := config.Config{
		Version: 1,
		Modules: map[string]config.ModuleDef{
			catchAll:          {Paths: []string{"internal/**"}},
			"internal/model":  {Paths: []string{"internal/model/**"}},
			"internal/engine": {Paths: []string{"internal/engine/**"}},
		},
	}
	mm := cfg.ModuleMapView()

	tests := []struct {
		path string
		want string
	}{
		{"internal/model/diagnostic/x.go", "internal/model"}, // specific beats catch-all
		{"internal/engine/run.go", "internal/engine"},        // specific beats catch-all
		{"internal/arch_test.go", catchAll},                  // only the catch-all matches
		{"internal/scope/scope.go", catchAll},                // no specific stanza → catch-all
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got, ok := mm.ModuleFor(tc.path)
			if !ok || got != tc.want {
				t.Errorf("ModuleFor(%q) = %q (ok=%v), want %q", tc.path, got, ok, tc.want)
			}
		})
	}
}

func TestForScope(t *testing.T) {
	cfg, err := config.Load(context.Background(), "testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	sc := cfg.ForScope()
	if len(sc.Exclusions) != len(cfg.Exclude) {
		t.Errorf("ForScope().Exclusions len=%d, want %d", len(sc.Exclusions), len(cfg.Exclude))
	}
}

func TestForExtract(t *testing.T) {
	cfg, err := config.Load(context.Background(), "testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	t.Run("typescript_mode_auto", func(t *testing.T) {
		ec := cfg.ForExtract("typescript")
		if ec.Mode != config.ModeAuto {
			t.Errorf("ForExtract(typescript).Mode = %q, want auto", ec.Mode)
		}
	})

	t.Run("go_mode_on", func(t *testing.T) {
		// testdata/valid.yaml sets languages.go.enabled: true → ModeOn
		ec := cfg.ForExtract("go")
		if ec.Mode != config.ModeOn {
			t.Errorf("ForExtract(go).Mode = %q, want on", ec.Mode)
		}
	})

	t.Run("paths_populated", func(t *testing.T) {
		ec := cfg.ForExtract("go")
		if len(ec.Paths) == 0 {
			t.Error("ForExtract(go).Paths is empty")
		}
	})

	t.Run("internal_globs_collected", func(t *testing.T) {
		ec := cfg.ForExtract("go")
		// pricing module has internal: ["services/pricing/internal/**"]
		if !slices.Contains(ec.Internal, "services/pricing/internal/**") {
			t.Errorf("ForExtract internal globs missing pricing internal glob; got %v", ec.Internal)
		}
	})

	t.Run("rust_mode_default_auto", func(t *testing.T) {
		// rust tool not in testdata/valid.yaml → defaults to auto
		ec := cfg.ForExtract("rust")
		if ec.Mode != config.ModeAuto {
			t.Errorf("ForExtract(rust).Mode = %q, want auto", ec.Mode)
		}
	})

	t.Run("rust_fields_populated", func(t *testing.T) {
		rc := config.Config{
			Version: 1,
			Languages: config.LanguagesConfig{Rust: config.RustLanguage{
				Manifest:       rustManifestPath,
				Features:       rustFeatures,
				IncludeDevDeps: true,
			}},
		}
		ec := rc.ForExtract("rust")
		if ec.CargoManifest != rustManifestPath {
			t.Errorf("CargoManifest = %q, want %q", ec.CargoManifest, rustManifestPath)
		}
		if !slices.Equal(ec.CargoFeatures, rustFeatures) {
			t.Errorf("CargoFeatures = %v, want %v", ec.CargoFeatures, rustFeatures)
		}
		if !ec.IncludeDevDeps {
			t.Error("IncludeDevDeps = false, want true")
		}
	})

	t.Run("rust_fields_absent_for_non_rust", func(t *testing.T) {
		// A non-rust language must not pick up the Cargo fields.
		rc := config.Config{Version: 1, Languages: config.LanguagesConfig{Rust: config.RustLanguage{Manifest: "Cargo.toml", IncludeDevDeps: true}}}
		ec := rc.ForExtract("go")
		if ec.CargoManifest != "" || ec.CargoFeatures != nil || ec.IncludeDevDeps {
			t.Errorf("non-rust lang leaked rust fields: %+v", ec)
		}
	})

	t.Run("go_module_filter_propagated", func(t *testing.T) {
		// tools.go.modules.include/exclude must surface in ForExtract("go").
		gc := config.Config{
			Version: 1,
			Languages: config.LanguagesConfig{Go: config.GoLanguage{
				Enabled: config.ModeAuto,
				Modules: config.GoModuleFilter{
					Include: []string{globSvcAll},
					Exclude: []string{"svc/legacy"},
				},
			}},
		}
		ec := gc.ForExtract("go")
		if !slices.Equal(ec.GoModuleInclude, []string{globSvcAll}) {
			t.Errorf("GoModuleInclude = %v, want [svc/**]", ec.GoModuleInclude)
		}
		if !slices.Equal(ec.GoModuleExclude, []string{"svc/legacy"}) {
			t.Errorf("GoModuleExclude = %v, want [svc/legacy]", ec.GoModuleExclude)
		}
	})

	t.Run("go_module_filter_absent_for_non_go", func(t *testing.T) {
		// tools.go.modules must not leak into other language extractors.
		gc := config.Config{
			Version: 1,
			Languages: config.LanguagesConfig{Go: config.GoLanguage{
				Modules: config.GoModuleFilter{Include: []string{globSvcAll}},
			}},
		}
		ec := gc.ForExtract("typescript")
		if ec.GoModuleInclude != nil || ec.GoModuleExclude != nil {
			t.Errorf("go module filter leaked into typescript extractor: %+v", ec)
		}
	})
}

func TestDefaultIncludesRust(t *testing.T) {
	cfg := config.Default()
	if got := cfg.Languages.Rust.Enabled; got != config.ModeAuto {
		t.Errorf("Default rust mode = %q, want auto", got)
	}
}

// Test fixtures factored out to keep goconst quiet about repeated literals.
const (
	rustManifestPath = "crates/core/Cargo.toml"
	globSvcAll       = "svc/**" // tools.go.modules include glob used across subtests
)

var rustFeatures = []string{"serde", "tokio"}

func TestLoadRustFields(t *testing.T) {
	yaml := "version: 1\n" +
		"languages:\n" +
		"  rust:\n" +
		"    manifest: " + rustManifestPath + "\n" +
		"    features: [serde, tokio]\n" +
		"    include_dev_deps: true\n"
	path := filepath.Join(t.TempDir(), "rust.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(context.Background(), path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	ec := cfg.ForExtract("rust")
	if ec.CargoManifest != rustManifestPath {
		t.Errorf("CargoManifest = %q, want %q", ec.CargoManifest, rustManifestPath)
	}
	if !slices.Equal(ec.CargoFeatures, rustFeatures) {
		t.Errorf("CargoFeatures = %v, want %v", ec.CargoFeatures, rustFeatures)
	}
	if !ec.IncludeDevDeps {
		t.Error("IncludeDevDeps = false, want true")
	}
}

func TestForClassify(t *testing.T) {
	cfg, err := config.Load(context.Background(), "testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cc := cfg.ForClassify()
	if len(cc.Modules) != len(cfg.Modules) {
		t.Errorf("ForClassify().Modules len=%d, want %d", len(cc.Modules), len(cfg.Modules))
	}
	if len(cc.Layers) != len(cfg.Layers) {
		t.Errorf("ForClassify().Layers len=%d, want %d", len(cc.Layers), len(cfg.Layers))
	}
}

// TestWithExplicitOwners verifies the test seam: a Config literal (bypassing Load)
// carries no explicit owners until marked, and WithExplicitOwners threads through
// ForClassify so classify can treat marked ownership as authoritative.
func TestWithExplicitOwners(t *testing.T) {
	cfg := config.Config{
		Version: 1,
		Modules: map[string]config.ModuleDef{
			"a": {Owner: "x"},
			"b": {Owner: "x"},
		},
	}
	if eo := cfg.ForClassify().ExplicitOwners; len(eo) != 0 {
		t.Errorf("unmarked literal ExplicitOwners = %v, want empty", eo)
	}
	got := cfg.WithExplicitOwners("a").ForClassify().ExplicitOwners
	if !got["a"] {
		t.Error(`ExplicitOwners["a"] = false, want true`)
	}
	if got["b"] {
		t.Error(`ExplicitOwners["b"] = true, want false (not marked)`)
	}
}

// TestLoad_PopulatesExplicitOwners verifies the production path: Load records a
// module as having an explicit owner iff its YAML `owner:` is non-empty, BEFORE
// any resolver fill.
func TestLoad_PopulatesExplicitOwners(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".archfit.yaml")
	yaml := "version: 1\n" +
		"modules:\n" +
		"  owned:\n    paths: [\"owned/**\"]\n    owner: team-x\n" +
		"  bare:\n    paths: [\"bare/**\"]\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.Load(context.Background(), path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	eo := cfg.ForClassify().ExplicitOwners
	if !eo["owned"] {
		t.Error(`ExplicitOwners["owned"] = false, want true (has YAML owner)`)
	}
	if eo["bare"] {
		t.Error(`ExplicitOwners["bare"] = true, want false (no owner)`)
	}
}

func TestForRules(t *testing.T) {
	cfg, err := config.Load(context.Background(), "testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	rc := cfg.ForRules()
	if len(rc.Rules) != len(cfg.Rules) {
		t.Errorf("ForRules().Rules len=%d, want %d", len(rc.Rules), len(cfg.Rules))
	}
}

func TestForMetric(t *testing.T) {
	cfg, err := config.Load(context.Background(), "testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	t.Run("known_metric", func(t *testing.T) {
		mc := cfg.ForMetric("encapsulation")
		if !mc.Enabled {
			t.Error("ForMetric(encapsulation).Enabled = false, want true")
		}
		if mc.Gate != "warn" {
			t.Errorf("ForMetric(encapsulation).Gate = %q, want warn", mc.Gate)
		}
	})

	t.Run("unknown_metric_zero_value", func(t *testing.T) {
		mc := cfg.ForMetric("nonexistent")
		if mc.Enabled {
			t.Error("ForMetric(nonexistent).Enabled = true, want false")
		}
	})
}

func TestForStaleness_GateOffDisables(t *testing.T) {
	const gateOff, gateWarn, gateFail = "off", "warn", "fail"
	tests := []struct {
		name        string
		mapReview   config.ModuleReviewConfig
		wantEnabled bool
	}{
		{"gate off disables", config.ModuleReviewConfig{Gate: gateOff}, false},
		{"gate off overrides stale_after", config.ModuleReviewConfig{Gate: gateOff, StaleAfter: "720h"}, false},
		{"gate warn enables", config.ModuleReviewConfig{Gate: gateWarn}, true},
		{"gate fail enables", config.ModuleReviewConfig{Gate: gateFail}, true},
		{"stale_after enables", config.ModuleReviewConfig{StaleAfter: "720h"}, true},
		{"nothing set stays disabled", config.ModuleReviewConfig{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Config{ModuleReview: tc.mapReview}
			if got := cfg.ForStaleness().Enabled; got != tc.wantEnabled {
				t.Errorf("ForStaleness().Enabled = %v, want %v", got, tc.wantEnabled)
			}
		})
	}
}

func TestForStatus(t *testing.T) {
	cfg, err := config.Load(context.Background(), "testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	es := cfg.ForWaivers()
	if len(es.Waivers) != len(cfg.Waivers) {
		t.Errorf("ForWaivers().Waivers len=%d, want %d", len(es.Waivers), len(cfg.Waivers))
	}
}

func TestForOutput(t *testing.T) {
	cfg, err := config.Load(context.Background(), "testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	oc := cfg.ForOutput()
	if !oc.JSON {
		t.Error("ForOutput().JSON = false, want true")
	}
	if oc.Markdown {
		t.Error("ForOutput().Markdown = true, want false")
	}
}

func TestToolMode_UnmarshalYAML(t *testing.T) {
	// Inline config struct to test ToolMode decoding directly via Load.
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		want    config.ToolMode
	}{
		{"bool_true", "version: 1\nanalyzers:\n  clones:\n    enabled: true\n", false, config.ModeOn},
		{"bool_false", "version: 1\nanalyzers:\n  clones:\n    enabled: false\n", false, config.ModeOff},
		{"string_auto", "version: 1\nanalyzers:\n  clones:\n    enabled: auto\n", false, config.ModeAuto},
		// on/off are no longer accepted — canonical vocabulary is true|false|auto.
		{"bare_on_rejected", "version: 1\nanalyzers:\n  clones:\n    enabled: on\n", true, ""},
		{"quoted_on_rejected", "version: 1\nanalyzers:\n  clones:\n    enabled: \"on\"\n", true, ""},
		{"quoted_off_rejected", "version: 1\nanalyzers:\n  clones:\n    enabled: \"off\"\n", true, ""},
		{"invalid", "version: 1\nanalyzers:\n  clones:\n    enabled: maybe\n", true, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Write to a temp file to reuse config.Load.
			tmp := t.TempDir() + "/cfg.yaml"
			if err := writeFile(tmp, tc.yaml); err != nil {
				t.Fatalf("write temp: %v", err)
			}
			cfg, err := config.Load(context.Background(), tmp)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			got := cfg.Analyzers.Clones.Enabled
			if got != tc.want {
				t.Errorf("Enabled = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLoad_Patterns(t *testing.T) {
	cfg, err := config.Load(context.Background(), "testdata/patterns.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Verify the rule with patterns parsed correctly.
	if len(cfg.Rules) != 2 {
		t.Fatalf("len(Rules) = %d, want 2", len(cfg.Rules))
	}
	rule := cfg.Rules[0]
	if rule.ID != "no_unsafe_ptr" {
		t.Errorf("Rules[0].ID = %q, want no_unsafe_ptr", rule.ID)
	}
	if len(rule.Patterns) != 2 {
		t.Fatalf("Rules[0].Patterns len=%d, want 2", len(rule.Patterns))
	}

	p0 := rule.Patterns[0]
	if p0.ID != "unsafe_cast" {
		t.Errorf("Patterns[0].ID = %q, want unsafe_cast", p0.ID)
	}
	if p0.Lang != "go" {
		t.Errorf("Patterns[0].Lang = %q, want go", p0.Lang)
	}
	if p0.Rule != "unsafe.Pointer($X)" {
		t.Errorf("Patterns[0].Rule = %q, want unsafe.Pointer($X)", p0.Rule)
	}

	p1 := rule.Patterns[1]
	if p1.ID != "reflect_unexported" {
		t.Errorf("Patterns[1].ID = %q, want reflect_unexported", p1.ID)
	}

	// Rule without patterns should have nil/empty Patterns slice.
	if len(cfg.Rules[1].Patterns) != 0 {
		t.Errorf("Rules[1].Patterns len=%d, want 0", len(cfg.Rules[1].Patterns))
	}
}

func TestForPatterns(t *testing.T) {
	tests := []struct {
		name    string
		rules   []config.RuleDef
		wantLen int
		wantIDs []string
	}{
		{
			name:    "no_rules",
			rules:   nil,
			wantLen: 0,
		},
		{
			name: "rules_without_patterns",
			rules: []config.RuleDef{
				{ID: "r1", Type: "forbidden_dependency"},
				{ID: "r2", Type: "public_api_only"},
			},
			wantLen: 0,
		},
		{
			name: "one_rule_with_patterns",
			rules: []config.RuleDef{
				{
					ID:   "r1",
					Type: "forbidden_dependency",
					Patterns: []config.PatternDef{
						{ID: "p1", Lang: "go", Rule: "unsafe.Pointer($X)"},
						{ID: "p2", Lang: "go", Rule: "reflect.ValueOf($X)"},
					},
				},
			},
			wantLen: 2,
			wantIDs: []string{"p1", "p2"},
		},
		{
			name: "multiple_rules_with_patterns",
			rules: []config.RuleDef{
				{
					ID: "r1",
					Patterns: []config.PatternDef{
						{ID: "p1", Lang: "go", Rule: "foo($X)"},
					},
				},
				{ID: "r2"}, // no patterns
				{
					ID: "r3",
					Patterns: []config.PatternDef{
						{ID: "p2", Lang: langTypeScript, Rule: "bar($X)"},
						{ID: "p3", Lang: langTypeScript, Rule: "baz($X)"},
					},
				},
			},
			wantLen: 3,
			wantIDs: []string{"p1", "p2", "p3"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Config{
				Version: 1,
				Rules:   tc.rules,
			}
			got := cfg.ForPatterns()
			if len(got) != tc.wantLen {
				t.Errorf("ForPatterns() len=%d, want %d", len(got), tc.wantLen)
			}
			for i, wantID := range tc.wantIDs {
				if i >= len(got) {
					t.Errorf("ForPatterns()[%d] missing, want ID=%q", i, wantID)
					continue
				}
				if got[i].ID != wantID {
					t.Errorf("ForPatterns()[%d].ID = %q, want %q", i, got[i].ID, wantID)
				}
			}
		})
	}
}

func TestLoad_ExistingConfigUnchanged(t *testing.T) {
	// Existing configs without patterns: must still load cleanly.
	cfg, err := config.Load(context.Background(), "testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load valid.yaml: %v", err)
	}
	for _, r := range cfg.Rules {
		if len(r.Patterns) != 0 {
			t.Errorf("rule %q: Patterns len=%d, want 0 (existing config has no patterns)", r.ID, len(r.Patterns))
		}
	}
	// ForPatterns on a config with no patterns returns empty, not nil.
	pc := cfg.ForPatterns()
	if len(pc) != 0 {
		t.Errorf("ForPatterns() len=%d on no-patterns config, want 0", len(pc))
	}
}

// TestLoad_NewToolsAndMetrics verifies that new tool entries and the three
// Tranche-1 metric entries parse correctly (checkbox: load with new keys).
func TestLoad_NewToolsAndMetrics(t *testing.T) {
	cfg, err := config.Load(context.Background(), "testdata/new_tools_metrics.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// analyzers.clones.enabled: true
	if got := cfg.Analyzers.Clones.Enabled; got != config.ModeOn {
		t.Errorf("analyzers.clones.enabled = %q, want on", got)
	}
	if !cfg.ClonesEnabled() {
		t.Error("ClonesEnabled() = false, want true when enabled")
	}

	// metrics.risk_hub: enabled false
	rh := cfg.ForMetric("risk_hub")
	if rh.Enabled {
		t.Error("ForMetric(risk_hub).Enabled = true, want false")
	}

	// metrics.architecture_fitness: enabled true, gate warn
	af := cfg.ForMetric("architecture_fitness")
	if !af.Enabled {
		t.Error("ForMetric(architecture_fitness).Enabled = false, want true")
	}
	if af.Gate != "warn" {
		t.Errorf("ForMetric(architecture_fitness).Gate = %q, want warn", af.Gate)
	}

	// metrics.functional_candidates: enabled false
	fc := cfg.ForMetric("functional_candidates")
	if fc.Enabled {
		t.Error("ForMetric(functional_candidates).Enabled = true, want false")
	}
}

// TestNewToolsDefaultOff verifies that absent clones entries default to
// disabled (zero ToolMode is not ModeOn), matching the opt-in contract.
func TestNewToolsDefaultOff(t *testing.T) {
	cfg := config.Config{Version: 1}
	if cfg.ClonesEnabled() {
		t.Error("ClonesEnabled() = true when absent, want false")
	}
}

// TestNewMetricsDefaultZero verifies that absent Tranche-1 metric entries return
// a zero MetricEntry (Enabled=false, Gate=""), consistent with ForMetric contract.
func TestNewMetricsDefaultZero(t *testing.T) {
	cfg := config.Config{Version: 1}
	for _, name := range []string{"risk_hub", "architecture_fitness", "functional_candidates"} {
		mc := cfg.ForMetric(name)
		if mc.Enabled {
			t.Errorf("ForMetric(%q).Enabled = true on empty config, want false", name)
		}
		if mc.Gate != "" {
			t.Errorf("ForMetric(%q).Gate = %q on empty config, want empty", name, mc.Gate)
		}
	}
}

// TestNewToolInvalidMode verifies that an invalid mode value for an analyzer key is rejected.
func TestNewToolInvalidMode(t *testing.T) {
	yaml := "version: 1\nanalyzers:\n  clones:\n    enabled: maybe\n"
	tmp := t.TempDir() + "/cfg.yaml"
	if err := writeFile(tmp, yaml); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	_, err := config.Load(context.Background(), tmp)
	if err == nil {
		t.Error("Load: expected error for invalid clones mode, got nil")
	}
}

// TestLoad_SelfConfig verifies that the project's own .archfit.yaml — the realistic
// config we run locally and in CI — loads cleanly and is well-formed. It deliberately
// does NOT pin opt-in toggles (risk_hub, …): those are operational choices,
// not invariants, and pinning them broke this test on every legitimate config change.
// Toggle-accessor behavior is covered against synthetic configs by TestForMetric,
// TestNewToolsDefaultOff, and the testdata fixture tests above.
func TestLoad_SelfConfig(t *testing.T) {
	// Go tests run with cwd = package dir (internal/config); repo root is two levels up.
	cfg, err := config.Load(context.Background(), "../../.archfit.yaml")
	if err != nil {
		t.Fatalf("Load self-config: %v", err)
	}
	if len(cfg.Modules) == 0 {
		t.Error("self-config: no modules parsed")
	}
	if len(cfg.Layers) == 0 {
		t.Error("self-config: no layers parsed")
	}
}

// TestSelfConfig_ExtractModuleMap verifies that all known internal/extract
// sub-packages are declared as explicit modules in the self-config, each with
// layer: adapter and role: adapter. This prevents accidental coverage by the
// broad internal/extract fallback stanza and makes adapter boundaries visible.
func TestSelfConfig_ExtractModuleMap(t *testing.T) {
	// Go tests run with cwd = package dir (internal/config); repo root is two levels up.
	cfg, err := config.Load(context.Background(), "../../.archfit.yaml")
	if err != nil {
		t.Fatalf("Load self-config: %v", err)
	}

	// These are the actual directories that exist under internal/extract.
	wantExtractModules := []string{
		"internal/extract/golang",
		"internal/extract/ts",
		"internal/extract/py",
		"internal/extract/rust",
		"internal/extract/scip",
		"internal/extract/astgrep",
		"internal/extract/deployunit",
		"internal/extract/runtime",
		"internal/extract/dynimports",
		"internal/extract/clones",
		"internal/extract/complexity",
		"internal/extract/loc",
	}

	for _, modName := range wantExtractModules {
		t.Run(modName, func(t *testing.T) {
			def, ok := cfg.Modules[modName]
			if !ok {
				t.Fatalf("module %q not found in self-config", modName)
			}
			if def.Layer != layerAdapter {
				t.Errorf("module %q: layer = %q, want adapter", modName, def.Layer)
			}
			if def.Role != config.RoleAdapter {
				t.Errorf("module %q: role = %q, want adapter", modName, def.Role)
			}
		})
	}
}

// TestSelfConfig_HistoryIsAdapter verifies that internal/history is declared as
// layer: adapter (git I/O adapter, not support).
func TestSelfConfig_HistoryIsAdapter(t *testing.T) {
	cfg, err := config.Load(context.Background(), "../../.archfit.yaml")
	if err != nil {
		t.Fatalf("Load self-config: %v", err)
	}
	def, ok := cfg.Modules["internal/history"]
	if !ok {
		t.Fatal("module internal/history not found in self-config")
	}
	if def.Layer != layerAdapter {
		t.Errorf("internal/history: layer = %q, want adapter", def.Layer)
	}
	if def.Role != config.RoleAdapter {
		t.Errorf("internal/history: role = %q, want adapter", def.Role)
	}
}

// TestSelfConfig_ScoreIsCore verifies that internal/score has an explicit module
// stanza in the self-config and is classified as core.
func TestSelfConfig_ScoreIsCore(t *testing.T) {
	cfg, err := config.Load(context.Background(), "../../.archfit.yaml")
	if err != nil {
		t.Fatalf("Load self-config: %v", err)
	}
	def, ok := cfg.Modules["internal/score"]
	if !ok {
		t.Fatal("module internal/score not found in self-config: add an explicit stanza so it is not hidden by the internal/** fallback")
	}
	if def.Layer != layerCore {
		t.Errorf("internal/score: layer = %q, want core", def.Layer)
	}
}

// TestSelfConfig_CmdIsCompositionRoot verifies that cmd/archfit carries
// role: composition_root so the distance model treats its fan-out as cohesion,
// not high-distance coupling.
func TestSelfConfig_CmdIsCompositionRoot(t *testing.T) {
	cfg, err := config.Load(context.Background(), "../../.archfit.yaml")
	if err != nil {
		t.Fatalf("Load self-config: %v", err)
	}
	def, ok := cfg.Modules["cmd/archfit"]
	if !ok {
		t.Fatal("module cmd/archfit not found in self-config")
	}
	if def.Role != config.RoleCompositionRoot {
		t.Errorf("cmd/archfit: role = %q, want composition_root", def.Role)
	}
}

// TestSelfConfig_RoleLayerConformance checks that every module with role: adapter
// is in layer: adapter, and role: composition_root is in layer: cmd.
// This catches mismatches between role declarations and layer assignments.
func TestSelfConfig_RoleLayerConformance(t *testing.T) {
	cfg, err := config.Load(context.Background(), "../../.archfit.yaml")
	if err != nil {
		t.Fatalf("Load self-config: %v", err)
	}

	// Collect module names for deterministic ordering.
	names := make([]string, 0, len(cfg.Modules))
	for name := range cfg.Modules {
		names = append(names, name)
	}
	slices.Sort(names)

	for _, name := range names {
		def := cfg.Modules[name]
		switch def.Role {
		case config.RoleAdapter:
			if def.Layer != layerAdapter {
				t.Errorf("module %q: role=adapter but layer=%q (want adapter)", name, def.Layer)
			}
		case config.RoleCompositionRoot:
			if def.Layer != "cmd" {
				t.Errorf("module %q: role=composition_root but layer=%q (want cmd)", name, def.Layer)
			}
		case config.RoleCore:
			if def.Layer != layerCore && def.Layer != "engine" {
				t.Errorf("module %q: role=core but layer=%q (want core or engine)", name, def.Layer)
			}
		default:
			if def.Role != "" {
				t.Errorf("module %q: unknown role %q", name, def.Role)
			}
		}
	}
}

// TestFillMissingOwners verifies that FillMissingOwners merges resolved ownership
// correctly: config owner wins, resolver fills gaps, absent entries are unchanged.
func TestFillMissingOwners(t *testing.T) {
	const (
		resolvedOwnerA = "@team-alpha"
		resolvedOwnerB = "@team-beta"
		configOwnerX   = "@team-x"
		pathPkgA       = "pkg/a/**"
		pathPkgB       = "pkg/b/**"
	)

	tests := []struct {
		name     string
		modules  map[string]config.ModuleDef
		resolved map[string]string
		want     map[string]string // module name → expected Owner after call
	}{
		{
			name: "fills modules with no owner",
			modules: map[string]config.ModuleDef{
				"a": {Paths: []string{pathPkgA}},
				"b": {Paths: []string{pathPkgB}},
			},
			resolved: map[string]string{
				"a": resolvedOwnerA,
				"b": resolvedOwnerB,
			},
			want: map[string]string{
				"a": resolvedOwnerA,
				"b": resolvedOwnerB,
			},
		},
		{
			name: "config owner wins over resolver",
			modules: map[string]config.ModuleDef{
				"a": {Paths: []string{pathPkgA}, Owner: configOwnerX},
				"b": {Paths: []string{pathPkgB}},
			},
			resolved: map[string]string{
				"a": resolvedOwnerA, // must not overwrite configOwnerX
				"b": resolvedOwnerB,
			},
			want: map[string]string{
				"a": configOwnerX,   // config wins
				"b": resolvedOwnerB, // resolver fills gap
			},
		},
		{
			name: "module absent from resolved stays unchanged",
			modules: map[string]config.ModuleDef{
				"a": {Paths: []string{pathPkgA}},
				"b": {Paths: []string{pathPkgB}},
			},
			resolved: map[string]string{
				"a": resolvedOwnerA,
				// "b" absent from resolved
			},
			want: map[string]string{
				"a": resolvedOwnerA,
				"b": "", // unchanged — no owner from either source
			},
		},
		{
			name: "empty resolved map — no change",
			modules: map[string]config.ModuleDef{
				"a": {Paths: []string{pathPkgA}},
			},
			resolved: map[string]string{},
			want: map[string]string{
				"a": "",
			},
		},
		{
			name: "empty resolved owner string — no change",
			modules: map[string]config.ModuleDef{
				"a": {Paths: []string{pathPkgA}},
			},
			resolved: map[string]string{
				"a": "", // empty string — must not be written
			},
			want: map[string]string{
				"a": "",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Config{
				Version: 1,
				Modules: tc.modules,
			}
			cfg.FillMissingOwners(tc.resolved)
			for mod, wantOwner := range tc.want {
				def, ok := cfg.Modules[mod]
				if !ok {
					t.Errorf("module %q not found after FillMissingOwners", mod)
					continue
				}
				if def.Owner != wantOwner {
					t.Errorf("module %q: Owner = %q, want %q", mod, def.Owner, wantOwner)
				}
			}
		})
	}
}

// TestLoad_ValidateEnums checks that bad coupling.min_severity and gate values
// are rejected at load with a descriptive error, not silently accepted (which would
// disable the check the field was meant to configure), while valid values load clean.
func TestLoad_ValidateEnums(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string // substring the error must mention; "" = expect success
	}{
		{
			name:    "valid bc severity",
			yaml:    "version: 1\ncoupling:\n  min_severity: critical\n",
			wantErr: "",
		},
		{
			name:    "empty bc severity is allowed",
			yaml:    yamlV1,
			wantErr: "",
		},
		{
			name:    "invalid bc severity",
			yaml:    "version: 1\ncoupling:\n  min_severity: severe\n",
			wantErr: "coupling.min_severity",
		},
		{
			name:    "valid rule gates",
			yaml:    "version: 1\nrules:\n  - id: r1\n    type: cycle\n    gate: fail\n  - id: r2\n    type: cycle\n    gate: warn\n",
			wantErr: "",
		},
		{
			name:    "empty rule gate is allowed",
			yaml:    "version: 1\nrules:\n  - id: r1\n    type: cycle\n",
			wantErr: "",
		},
		{
			name:    "invalid rule gate names the rule id",
			yaml:    "version: 1\nrules:\n  - id: nocycle\n    type: cycle\n    gate: block\n",
			wantErr: "rules[nocycle]",
		},
		{
			name:    "invalid rule gate without id falls back to index",
			yaml:    "version: 1\nrules:\n  - type: cycle\n    gate: block\n",
			wantErr: "rules[#0]",
		},
		{
			name:    "invalid metric gate names the metric",
			yaml:    "version: 1\nmetrics:\n  cycle:\n    enabled: true\n    gate: nope\n",
			wantErr: "metrics.cycle",
		},
		{
			name:    "invalid module_review gate",
			yaml:    "version: 1\nmodule_review:\n  gate: maybe\n",
			wantErr: "module_review",
		},
		{
			name:    "valid language gate",
			yaml:    "version: 1\nlanguages:\n  go:\n    enabled: auto\n    gate: fail\n",
			wantErr: "",
		},
		{
			name:    "empty language gate is allowed",
			yaml:    "version: 1\nlanguages:\n  go:\n    enabled: auto\n",
			wantErr: "",
		},
		{
			name:    "invalid language gate names the language",
			yaml:    "version: 1\nlanguages:\n  go:\n    enabled: auto\n    gate: block\n",
			wantErr: "languages.go",
		},
		{
			name:    "off is a valid gate",
			yaml:    "version: 1\nmodule_review:\n  gate: off\n",
			wantErr: "",
		},
		{
			name:    "invalid stale_after duration",
			yaml:    "version: 1\nmodule_review:\n  stale_after: \"180 days\"\n",
			wantErr: "module_review.stale_after",
		},
		{
			name:    "valid stale_after duration",
			yaml:    "version: 1\nmodule_review:\n  stale_after: 720h\n",
			wantErr: "",
		},
		{
			name:    "invalid analyzer timeout rejected",
			yaml:    "version: 1\nanalyzers:\n  scip:\n    timeout: \"5min\"\n",
			wantErr: "analyzers.scip.timeout",
		},
		{
			name:    "valid analyzer timeout accepted",
			yaml:    "version: 1\nanalyzers:\n  scip:\n    timeout: \"5m\"\n",
			wantErr: "",
		},
		{
			name:    "valid module role",
			yaml:    "version: 1\nmodules:\n  cmd:\n    paths: [\"cmd/**\"]\n    role: composition_root\n",
			wantErr: "",
		},
		{
			name:    "empty module role is allowed",
			yaml:    "version: 1\nmodules:\n  cmd:\n    paths: [\"cmd/**\"]\n",
			wantErr: "",
		},
		{
			name:    "invalid module role names the module",
			yaml:    "version: 1\nmodules:\n  cmd:\n    paths: [\"cmd/**\"]\n    role: wiring\n",
			wantErr: "modules.cmd.role",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmp := filepath.Join(t.TempDir(), "cfg.yaml")
			if err := writeFile(tmp, tc.yaml); err != nil {
				t.Fatalf("write temp: %v", err)
			}
			_, err := config.Load(context.Background(), tmp)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Load: unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Load: expected error mentioning %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not mention %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// TestLoad_ToolGate verifies the languages.<x>.gate field parses into the typed
// GateMode and that an omitted gate is the empty value (callers default it to warn).
func TestLoad_ToolGate(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "cfg.yaml")
	yaml := "version: 1\n" +
		"languages:\n" +
		"  go:\n" +
		"    enabled: auto\n" +
		"    gate: fail\n" +
		"  typescript:\n" +
		"    enabled: auto\n"
	if err := writeFile(tmp, yaml); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	cfg, err := config.Load(context.Background(), tmp)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.ToolGate("go"); got != config.GateFail {
		t.Errorf("languages.go.gate = %q, want %q", got, config.GateFail)
	}
	if got := cfg.ToolGate("typescript"); got != "" {
		t.Errorf("languages.typescript.gate = %q, want \"\" (unset)", got)
	}
}

// Config-quality lint test constants (kept out of literals for goconst).
const (
	lintPath  = "a/**"
	lintOwner = "owner"
	lintVol   = "subdomain/volatility"
	lintTeam  = "team-a"

	// Layer name constants used in self-config conformance tests.
	layerAdapter = "adapter"
	layerCore    = "core"

	// langTypeScript and yamlV1 appear in many tests; kept as constants to
	// satisfy the goconst linter.
	langTypeScript = "typescript"
	yamlV1         = "version: 1\n"
)

func TestLint(t *testing.T) {
	tests := []struct {
		name string
		mod  config.ModuleDef
		want []string // expected Missing tokens; nil = no warning for this module
	}{
		{
			name: "fully specified",
			mod:  config.ModuleDef{Paths: []string{lintPath}, Owner: lintTeam, Subdomain: layerCore, Volatility: "high"},
			want: nil,
		},
		{
			name: "missing owner only",
			mod:  config.ModuleDef{Paths: []string{lintPath}, Subdomain: layerCore},
			want: []string{lintOwner},
		},
		{
			name: "missing subdomain and volatility only",
			mod:  config.ModuleDef{Paths: []string{lintPath}, Owner: lintTeam},
			want: []string{lintVol},
		},
		{
			name: "missing all three",
			mod:  config.ModuleDef{Paths: []string{lintPath}},
			want: []string{lintOwner, lintVol},
		},
		{
			name: "subdomain alone resolves volatility",
			mod:  config.ModuleDef{Paths: []string{lintPath}, Owner: lintTeam, Subdomain: "generic"},
			want: nil,
		},
		{
			name: "volatility alone resolves volatility",
			mod:  config.ModuleDef{Paths: []string{lintPath}, Owner: lintTeam, Volatility: "low"},
			want: nil,
		},
		{
			name: "pathless module is not linted",
			mod:  config.ModuleDef{Owner: ""}, // no paths → classifies nothing
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Config{Modules: map[string]config.ModuleDef{"m": tc.mod}}
			got := cfg.Lint()
			if tc.want == nil {
				if len(got) != 0 {
					t.Fatalf("Lint() = %v, want no warnings", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("Lint() = %v, want exactly one warning", got)
			}
			if got[0].Module != "m" {
				t.Errorf("module = %q, want %q", got[0].Module, "m")
			}
			if !slices.Equal(got[0].Missing, tc.want) {
				t.Errorf("Missing = %v, want %v (order is fixed)", got[0].Missing, tc.want)
			}
		})
	}
}

func TestLint_DeterministicModuleOrder(t *testing.T) {
	// Map iteration is random; Lint must return modules in sorted name order.
	bare := config.ModuleDef{Paths: []string{"x/**"}}
	cfg := config.Config{Modules: map[string]config.ModuleDef{
		"mod-z": bare,
		"mod-a": bare,
		"mod-m": bare,
		"ok":    {Paths: []string{"o/**"}, Owner: "t", Volatility: "low"}, // no warning
	}}

	first := cfg.Lint()
	gotModules := make([]string, len(first))
	for i, w := range first {
		gotModules[i] = w.Module
	}
	wantModules := []string{"mod-a", "mod-m", "mod-z"}
	if !slices.Equal(gotModules, wantModules) {
		t.Fatalf("module order = %v, want %v", gotModules, wantModules)
	}

	// Stable across repeated calls (no map-order leakage).
	for range 20 {
		again := cfg.Lint()
		for i := range again {
			if again[i].Module != first[i].Module || !slices.Equal(again[i].Missing, first[i].Missing) {
				t.Fatalf("Lint() not deterministic: %v vs %v", again, first)
			}
		}
	}
}

func TestLintWarning_String(t *testing.T) {
	w := config.LintWarning{Module: "billing", Missing: []string{lintOwner, lintVol}}
	got := w.String()
	want := `module "billing" omits owner, subdomain/volatility`
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// analyzers.syntax — SyntaxEnabled + ForSyntax
// ---------------------------------------------------------------------------

func TestSyntaxEnabled(t *testing.T) {
	cases := []struct {
		name string
		mode string
		want bool
	}{
		{"true enables", "true", true},
		{"false disables", "false", false},
		{"auto disables (opt-in only)", "auto", false},
		{"absent disables", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var yamlBody string
			if tc.mode == "" {
				yamlBody = "version: 1\n"
			} else {
				yamlBody = "version: 1\nanalyzers:\n  syntax:\n    enabled: " + tc.mode + "\n"
			}
			dir := t.TempDir()
			path := filepath.Join(dir, ".archfit.yaml")
			if err := writeFile(path, yamlBody); err != nil {
				t.Fatalf("writeFile: %v", err)
			}
			cfg, err := config.Load(context.Background(), path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got := cfg.SyntaxEnabled(); got != tc.want {
				t.Errorf("SyntaxEnabled() = %v, want %v (mode=%q)", got, tc.want, tc.mode)
			}
		})
	}
}

func TestForSyntax_Mode(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		enabled bool
	}{
		{
			name:    "enabled when true",
			yaml:    "version: 1\nanalyzers:\n  syntax:\n    enabled: true\n",
			enabled: true,
		},
		{
			name:    "disabled when auto",
			yaml:    "version: 1\nanalyzers:\n  syntax:\n    enabled: auto\n",
			enabled: false,
		},
		{
			name:    "disabled when false",
			yaml:    "version: 1\nanalyzers:\n  syntax:\n    enabled: false\n",
			enabled: false,
		},
		{
			name:    "disabled when absent",
			yaml:    "version: 1\n",
			enabled: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ".archfit.yaml")
			if err := writeFile(path, tc.yaml); err != nil {
				t.Fatalf("writeFile: %v", err)
			}
			cfg, err := config.Load(context.Background(), path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			sc := cfg.ForSyntax()
			if sc.Enabled != tc.enabled {
				t.Errorf("ForSyntax().Enabled = %v, want %v", sc.Enabled, tc.enabled)
			}
		})
	}
}

func TestForSyntax_Languages(t *testing.T) {
	allFour := []string{"go", "typescript", "python", "rust"}

	t.Run("all four when no tools configured", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".archfit.yaml")
		if err := writeFile(path, "version: 1\n"); err != nil {
			t.Fatalf("writeFile: %v", err)
		}
		cfg, _ := config.Load(context.Background(), path)
		sc := cfg.ForSyntax()
		if !slices.Equal(sc.Languages, allFour) {
			t.Errorf("Languages = %v, want %v", sc.Languages, allFour)
		}
	})

	t.Run("excludes language set to off", func(t *testing.T) {
		yaml := "version: 1\nlanguages:\n  rust:\n    enabled: false\n"
		dir := t.TempDir()
		path := filepath.Join(dir, ".archfit.yaml")
		if err := writeFile(path, yaml); err != nil {
			t.Fatalf("writeFile: %v", err)
		}
		cfg, _ := config.Load(context.Background(), path)
		sc := cfg.ForSyntax()
		want := []string{"go", "typescript", "python"}
		if !slices.Equal(sc.Languages, want) {
			t.Errorf("Languages = %v, want %v", sc.Languages, want)
		}
	})

	t.Run("includes language set to auto", func(t *testing.T) {
		yaml := "version: 1\nlanguages:\n  python:\n    enabled: auto\n"
		dir := t.TempDir()
		path := filepath.Join(dir, ".archfit.yaml")
		if err := writeFile(path, yaml); err != nil {
			t.Fatalf("writeFile: %v", err)
		}
		cfg, _ := config.Load(context.Background(), path)
		sc := cfg.ForSyntax()
		if !slices.Equal(sc.Languages, allFour) {
			t.Errorf("Languages = %v, want %v (auto should be included)", sc.Languages, allFour)
		}
	})

	t.Run("all off yields empty languages", func(t *testing.T) {
		yaml := "version: 1\nlanguages:\n  go:\n    enabled: false\n  typescript:\n    enabled: false\n  python:\n    enabled: false\n  rust:\n    enabled: false\n"
		dir := t.TempDir()
		path := filepath.Join(dir, ".archfit.yaml")
		if err := writeFile(path, yaml); err != nil {
			t.Fatalf("writeFile: %v", err)
		}
		cfg, _ := config.Load(context.Background(), path)
		sc := cfg.ForSyntax()
		if len(sc.Languages) != 0 {
			t.Errorf("Languages = %v, want empty", sc.Languages)
		}
	})
}

// writeFile is a test helper that writes content to path.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
