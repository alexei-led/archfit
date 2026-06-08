package config_test

import (
	"context"
	"os"
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
	if len(cfg.Exclusions) != 2 {
		t.Errorf("len(Exclusions) = %d, want 2", len(cfg.Exclusions))
	}
	if len(cfg.Exceptions) != 1 {
		t.Errorf("len(Exceptions) = %d, want 1", len(cfg.Exceptions))
	}

	// Verify tool modes.
	git, ok := cfg.Tools["git"]
	if !ok {
		t.Error("tools.git not found")
	} else if git.Enabled != config.ModeOn {
		t.Errorf("tools.git.enabled = %q, want %q", git.Enabled, config.ModeOn)
	}
	dc, ok := cfg.Tools["dependency_cruiser"]
	if !ok {
		t.Error("tools.dependency_cruiser not found")
	} else if dc.Enabled != config.ModeAuto {
		t.Errorf("tools.dependency_cruiser.enabled = %q, want %q", dc.Enabled, config.ModeAuto)
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
	for i := 0; i < 10; i++ {
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

func TestForScope(t *testing.T) {
	cfg, err := config.Load(context.Background(), "testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	sc := cfg.ForScope()
	if len(sc.Exclusions) != len(cfg.Exclusions) {
		t.Errorf("ForScope().Exclusions len=%d, want %d", len(sc.Exclusions), len(cfg.Exclusions))
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

	t.Run("go_mode_default_auto", func(t *testing.T) {
		// go tool not in testdata/valid.yaml → defaults to auto
		ec := cfg.ForExtract("go")
		if ec.Mode != config.ModeAuto {
			t.Errorf("ForExtract(go).Mode = %q, want auto", ec.Mode)
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
		found := false
		for _, g := range ec.Internal {
			if g == "services/pricing/internal/**" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ForExtract internal globs missing pricing internal glob; got %v", ec.Internal)
		}
	})
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

func TestForStatus(t *testing.T) {
	cfg, err := config.Load(context.Background(), "testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	es := cfg.ForStatus()
	if len(es.Exceptions) != len(cfg.Exceptions) {
		t.Errorf("ForStatus().Exceptions len=%d, want %d", len(es.Exceptions), len(cfg.Exceptions))
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
		{"bool_true", "version: 1\ntools:\n  git:\n    enabled: true\n", false, config.ModeOn},
		{"bool_false", "version: 1\ntools:\n  git:\n    enabled: false\n", false, config.ModeOff},
		{"string_auto", "version: 1\ntools:\n  git:\n    enabled: auto\n", false, config.ModeAuto},
		{"string_on", "version: 1\ntools:\n  git:\n    enabled: on\n", false, config.ModeOn},
		{"string_off", "version: 1\ntools:\n  git:\n    enabled: off\n", false, config.ModeOff},
		{"invalid", "version: 1\ntools:\n  git:\n    enabled: maybe\n", true, ""},
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
			got := cfg.Tools["git"].Enabled
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
						{ID: "p2", Lang: "typescript", Rule: "bar($X)"},
						{ID: "p3", Lang: "typescript", Rule: "baz($X)"},
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

// writeFile is a test helper that writes content to path.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
