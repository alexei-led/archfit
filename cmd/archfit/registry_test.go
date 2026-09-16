package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/extract/registry"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// TestBuiltinConventionsCoverage keeps the registry honest: every language the
// composition root wires must have a NodeConvention in the model ring, or the
// core-ring metrics fall back to the slash/default heuristic for it silently.
func TestBuiltinConventionsCoverage(t *testing.T) {
	t.Parallel()
	for _, lang := range registry.All() {
		if _, ok := graph.BuiltinConventions[lang.ID]; !ok {
			t.Errorf("language %q in registry has no graph.BuiltinConventions entry", lang.ID)
		}
	}
}

// TestLangAliasesInInstallEnum guards that every language value the doctor
// command's --lang flag accepts resolves through the registry.
func TestLangAliasesInInstallEnum(t *testing.T) {
	t.Parallel()
	enum := installEnumTag(t)
	if len(enum) == 0 {
		t.Fatal("DoctorCmd.Lang has no enum tag values")
	}
	for _, v := range enum {
		if registry.ByAlias(v) == "" {
			t.Errorf("doctor --lang enum value %q does not resolve via registry.ByAlias", v)
		}
	}
}

// installEnumTag reads the comma-separated kong `enum` struct tag off
// DoctorCmd.Lang via reflection so the test tracks the tag without duplicating
// its literal.
func installEnumTag(t *testing.T) []string {
	t.Helper()
	f, ok := reflect.TypeOf(DoctorCmd{}).FieldByName("Lang")
	if !ok {
		t.Fatal("DoctorCmd has no Lang field")
	}
	tag := f.Tag.Get("enum")
	if tag == "" {
		return nil
	}
	return strings.Split(tag, ",")
}

// TestBuildExtractorsOrder pins the go → ts → python build order (the
// graph-merge order the engine golden test depends on) and that each extractor
// reports the canonical language name.
func TestBuildExtractorsOrder(t *testing.T) {
	t.Parallel()
	exs := registry.Build(&toolrun.RunnerMock{}, config.Default().ExtractConfigs(), nil)
	want := []string{config.LangGo, config.LangTypeScript, config.LangPython, config.LangRust}
	if len(exs) != len(want) {
		t.Fatalf("registry.Build returned %d extractors, want %d", len(exs), len(want))
	}
	for i, w := range want {
		if got := exs[i].Name(); got != w {
			t.Errorf("extractor[%d].Name() = %q, want %q", i, got, w)
		}
	}
}

// TestExtractConfigsCoversEveryRegisteredLanguage is the guard that
// TestBuildExtractorsOrder cannot be: registry.Build indexes the Configs map by
// every registered language, and a missing key yields the ZERO ExtractConfig
// (empty mode, no exclusions) while still constructing an extractor — so a
// newly registered language would be silently unconfigured and the build-order
// test would still pass.
func TestExtractConfigsCoversEveryRegisteredLanguage(t *testing.T) {
	t.Parallel()
	configs := config.Default().ExtractConfigs()
	for _, lang := range registry.All() {
		got, ok := configs[lang.ID]
		if !ok {
			t.Errorf("ExtractConfigs() has no entry for registered language %q", lang.ID)
			continue
		}
		if want := config.Default().ForExtract(lang.ID); !reflect.DeepEqual(got, want) {
			t.Errorf("ExtractConfigs()[%q] = %+v, want the projected config %+v", lang.ID, got, want)
		}
	}
	if len(configs) != len(registry.All()) {
		t.Errorf("ExtractConfigs() has %d entries, want one per registered language (%d)",
			len(configs), len(registry.All()))
	}
}

// TestLanguageByAlias covers canonical IDs, short aliases, and unknown keys.
func TestLanguageByAlias(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"go":         config.LangGo,
		"typescript": config.LangTypeScript,
		"ts":         config.LangTypeScript,
		"python":     config.LangPython,
		"py":         config.LangPython,
		"rust":       config.LangRust,
		"rs":         config.LangRust,
		"":           "",
		"ruby":       "",
	}
	for key, want := range cases {
		if got := registry.ByAlias(key); got != want {
			t.Errorf("registry.ByAlias(%q) = %q, want %q", key, got, want)
		}
	}
}

// TestPrimaryExtractorTools pins the dependency-graph coverage names injected
// into the scorecard, in registry order.
func TestPrimaryExtractorTools(t *testing.T) {
	t.Parallel()
	got := registry.PrimaryTools()
	want := []string{toolGoPackages, toolDepCruiser, toolGrimp, toolCargo}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("registry.PrimaryTools() = %v, want %v", got, want)
	}
}
