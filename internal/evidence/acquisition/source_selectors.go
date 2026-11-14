package acquisition

import (
	"github.com/bmatcuk/doublestar/v4"

	evidencecontract "github.com/alexei-led/archfit/v3/internal/evidence"
	"github.com/alexei-led/archfit/v3/internal/extract/registry"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

// declaredOutOfScope names the walked source files the configuration declared
// outside the analysis scope: a file an effective `exclude:` glob matches, or a
// file of a language switched off by `languages.<id>.enabled: false`.
//
// Rule scope skips these files. Without that, one stray tooling script (a
// `.cjs` config wrapper, a Python helper run through uv) put its language in
// scope for every rule over its directory, and the rule stayed unevaluated
// waiting for a producer the configuration had deliberately switched off. This
// is declared scope, never analyzer availability: an absent producer for a
// language that is still in scope keeps the rule unevaluated.
//
// exclusions must be the set RunOptions merged once: scope.MergeExclusions is
// not idempotent, so it is never applied again here. A switched-off language is
// read through primaryDisabledByConfig, the predicate behind the disabled
// coverage row, so a language switched off with an explicit gate stays in scope
// and its rules stay honest about the producer that did not run.
func declaredOutOfScope(f evidencecontract.Facts, exclusions []string, cov CoverageOptions) map[string]struct{} {
	disabled := disabledLanguages(cov)
	var out map[string]struct{}
	consider := func(file string) {
		if !outOfDeclaredScope(file, exclusions, disabled) {
			return
		}
		if out == nil {
			out = make(map[string]struct{})
		}
		out[file] = struct{}{}
	}
	for file := range f.FileLOC {
		consider(file)
	}
	for file := range f.FileClassIndex {
		consider(file)
	}
	return out
}

// disabledLanguages are the languages switched off with no explicit gate, the
// predicate behind the disabled coverage row (primaryDisabledByConfig).
func disabledLanguages(cov CoverageOptions) map[string]struct{} {
	disabled := make(map[string]struct{})
	for _, lang := range registry.All() {
		if primaryDisabledByConfig(cov, lang.PrimaryTool) {
			disabled[lang.ID] = struct{}{}
		}
	}
	return disabled
}

// outOfDeclaredScope reports whether one repo-relative source file is outside
// the declared analysis scope. Exclusion globs match the slash path the same
// way the Go extractor's own exclusion check does.
func outOfDeclaredScope(file string, exclusions []string, disabled map[string]struct{}) bool {
	if language, _, ok := (policy.ModuleMap{}).RuleSelectorForFile(file); ok {
		if _, off := disabled[language]; off {
			return true
		}
	}
	for _, glob := range exclusions {
		if matched, _ := doublestar.Match(glob, file); matched {
			return true
		}
	}
	return false
}
