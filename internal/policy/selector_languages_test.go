package policy_test

import (
	"slices"
	"sort"
	"testing"

	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/policy"
)

// TestSelectorLanguages pins which languages a rule selector can address.
//
// The vocabulary is the point: a selector is matched against graph node IDs,
// and those are spelled per language — slash paths for Go/TypeScript, dotted
// IDs for Python, `crate::mod` for Rust. A selector that cannot be spelled in a
// language's vocabulary can never match one of its nodes, whatever analyzers
// ran, so requiring that language's evidence for the rule is requiring evidence
// that could not change the outcome.
func TestSelectorLanguages(t *testing.T) {
	t.Parallel()
	mm := policy.BuildModuleMap(nil)
	tests := []struct {
		name    string
		pattern string
		want    []string
	}{
		{"slash path excludes dotted and :: vocabularies", "internal/llm/**", []string{graph.LangGo, graph.LangTypeScript}},
		{"slash path without a wildcard", "internal/config", []string{graph.LangGo, graph.LangTypeScript}},
		{"dotted python module id", "prefect.**", []string{graph.LangPython}},
		{"rust crate path", "mycrate::mod::**", []string{graph.LangRust}},
		{"no separator at all addresses every language", "**", []string{graph.LangGo, graph.LangPython, graph.LangRust, graph.LangTypeScript}},
		{"bare module name is ambiguous, so every language", "billing", []string{graph.LangGo, graph.LangPython, graph.LangRust, graph.LangTypeScript}},
		{"an explicit supported extension names one language", "src/**/*.go", []string{graph.LangGo}},
		{"a python extension beats the slash in the path", "src/**/*.py", []string{graph.LangPython}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			set := mm.SelectorLanguages(tc.pattern)
			got := make([]string, 0, len(set))
			for language := range set {
				got = append(got, language)
			}
			sort.Strings(got)
			if !slices.Equal(got, tc.want) {
				t.Errorf("SelectorLanguages(%q) = %v, want %v", tc.pattern, got, tc.want)
			}
		})
	}
}
