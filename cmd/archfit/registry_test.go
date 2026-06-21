package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// TestBuiltinConventionsCoverage keeps the registry honest: every language the
// composition root wires must have a NodeConvention in the model ring, or the
// core-ring metrics fall back to the slash/default heuristic for it silently.
func TestBuiltinConventionsCoverage(t *testing.T) {
	for _, lang := range languageRegistry {
		if _, ok := graph.BuiltinConventions[lang.ID]; !ok {
			t.Errorf("language %q in registry has no graph.BuiltinConventions entry", lang.ID)
		}
	}
}

// TestLangAliasesInInstallEnum guards that every value the install command's
// kong enum accepts resolves through the registry — install must never accept a
// language it cannot dispatch.
func TestLangAliasesInInstallEnum(t *testing.T) {
	enum := installEnumTag(t)
	if len(enum) == 0 {
		t.Fatal("InstallCmd.Lang has no enum tag values")
	}
	for _, v := range enum {
		if languageByAlias(v) == "" {
			t.Errorf("install enum value %q does not resolve via languageByAlias", v)
		}
	}
}

// installEnumTag reads the comma-separated kong `enum` struct tag off
// InstallCmd.Lang via reflection so the test tracks the tag without duplicating
// its literal.
func installEnumTag(t *testing.T) []string {
	t.Helper()
	f, ok := reflect.TypeOf(InstallCmd{}).FieldByName("Lang")
	if !ok {
		t.Fatal("InstallCmd has no Lang field")
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
	exs := buildExtractors(&toolrun.RunnerMock{}, config.Default())
	want := []string{config.LangGo, config.LangTypeScript, config.LangPython}
	if len(exs) != len(want) {
		t.Fatalf("buildExtractors returned %d extractors, want %d", len(exs), len(want))
	}
	for i, w := range want {
		if got := exs[i].Name(); got != w {
			t.Errorf("extractor[%d].Name() = %q, want %q", i, got, w)
		}
	}
}

// TestLanguageByAlias covers canonical IDs, short aliases, and unknown keys.
func TestLanguageByAlias(t *testing.T) {
	cases := map[string]string{
		"go":         config.LangGo,
		"typescript": config.LangTypeScript,
		"ts":         config.LangTypeScript,
		"python":     config.LangPython,
		"py":         config.LangPython,
		"":           "",
		"ruby":       "",
	}
	for key, want := range cases {
		if got := languageByAlias(key); got != want {
			t.Errorf("languageByAlias(%q) = %q, want %q", key, got, want)
		}
	}
}

// TestPrimaryExtractorTools pins the dependency-graph coverage names injected
// into the scorecard, in registry order.
func TestPrimaryExtractorTools(t *testing.T) {
	got := primaryExtractorTools()
	want := []string{toolGoPackages, toolDepCruiser, toolGrimp}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("primaryExtractorTools() = %v, want %v", got, want)
	}
}
