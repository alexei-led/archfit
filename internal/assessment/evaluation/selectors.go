package evaluation

import (
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/policy"
)

// Rule selector sides, as a vacuity reason names them.
const (
	selectorFrom = "from"
	selectorTo   = "to"
)

// selectorMatchesNothingPrefix starts the unevaluated-rule reason for a
// selector that matches nothing. The App keys a policy-defect reason off it, so
// the text is a contract: "selector matches nothing: <from|to> <glob>".
const selectorMatchesNothingPrefix = "selector matches nothing: "

// selectorMatchesNothing is the reason for a rule whose selector is vacuous.
func selectorMatchesNothing(side, glob string) string {
	return selectorMatchesNothingPrefix + side + " " + glob
}

// selectorRuleTypes are the rule types that read their from:/to: selectors.
// Every other type ignores them, so their selectors cannot make it vacuous.
var selectorRuleTypes = map[string]struct{}{
	ruleTypeForbiddenDependency: {}, ruleTypePublicAPIOnly: {}, ruleTypeInternalAPIAccess: {},
	ruleTypeForbiddenPattern: {}, // reads from: only; to: is a config error on it
}

// fileSourceLanguages are the languages whose import edges start at the
// importing file's node: the Go and TypeScript extractors emit file: sources,
// while Python and Rust edges start at the module or crate node.
var fileSourceLanguages = map[string]struct{}{"go": {}, "typescript": {}}

// Selector vocabularies by separator: slash paths (file paths, Go package
// directories, TypeScript files), Python dotted IDs, and Rust crate::mod IDs.
const (
	sepPath   = "/"
	sepDotted = "."
	sepCrate  = "::"
)

// selectorInventory is the rule-scope source inventory as rule selectors see
// it: every in-scope source file, the graph-node selector each file projects
// to, the first-party Go module paths, and the Go standard-library packages.
// Rule evaluation and config lint decide selector vacuity with this one
// predicate. Their inputs differ in two places: check has Rust crate names
// from cargo metadata, and `check --lang` turns on a language the config
// switches off, so the in-scope files differ.
type selectorInventory struct {
	moduleMap policy.ModuleMap
	files     []string
	selectors map[string]string
	// roots are the leading segments of every file path and node selector,
	// keyed by the separator of the vocabulary they are spelled in (sepPath,
	// sepDotted, sepCrate). A target selector whose leading segment, cut in its
	// own vocabulary, is one of them names first-party source.
	roots map[string]map[string]struct{}
	// unresolved are the languages whose node identity the inventory could not
	// name for some file. Without cargo metadata Rust crate names are unknown,
	// so a selector Rust could spell cannot be proven to match nothing.
	unresolved map[string]struct{}
	goModules  []string
	goStdlib   []string
}

// newSelectorInventory builds the inventory over files, the rule-scope source
// inventory (sourceInventoryFiles) of f.
func newSelectorInventory(moduleMap policy.ModuleMap, files []string, f Observations) selectorInventory {
	inv := selectorInventory{
		moduleMap: moduleMap, files: files, selectors: f.SourceSelectors,
		roots:      map[string]map[string]struct{}{sepPath: {}, sepDotted: {}, sepCrate: {}},
		unresolved: map[string]struct{}{}, goModules: f.GoModulePaths, goStdlib: f.GoStdlibPackages,
	}
	for _, file := range inv.files {
		inv.addRoot(sepPath, file)
		language, selector, supported := ruleFileSelector(moduleMap, file, f.SourceSelectors)
		switch {
		case !supported:
		case selector == "" && strings.Contains(file, "/"):
			// A file below the root always has a node identity; an empty one was
			// withheld by the producer projection.
			inv.unresolved[language] = struct{}{}
		case selector != "" && selector != file:
			inv.addRoot(selectorSeparator(selector), selector)
		}
	}
	return inv
}

// addRoot records the leading segment of id, cut at sep, as a root of the sep
// vocabulary. A single-segment id (sep "") is a root in every vocabulary.
func (inv selectorInventory) addRoot(sep, id string) {
	if sep == "" {
		for _, roots := range inv.roots {
			roots[id] = struct{}{}
		}
		return
	}
	segment, _, _ := strings.Cut(id, sep)
	inv.roots[sep][segment] = struct{}{}
}

// vacuousSelector returns the first selector of rule that provably matches
// nothing the rule can see. Only the rule types that read selectors are
// checked, and an empty selector means "match all", so it is never vacuous.
func (inv selectorInventory) vacuousSelector(rule policy.RuleDef) (side, glob string, vacuous bool) {
	if _, reads := selectorRuleTypes[rule.Type]; !reads {
		return "", "", false
	}
	for _, s := range [...]struct{ side, glob string }{{selectorFrom, rule.From}, {selectorTo, rule.To}} {
		if s.glob != "" && inv.vacuous(rule.Type, s.side, s.glob) {
			return s.side, s.glob, true
		}
	}
	return "", "", false
}

// vacuous reports whether one selector of a ruleType rule matches nothing the
// rule can see.
//
// A source (from:) selector must match in-scope source: rule edges start there.
// A target (to:) selector is held to the inventory only when it is spelled as
// first-party source, because a ban on an external package (os, net/http,
// github.com/...) legitimately matches no source file. A target that names a Go
// standard-library package is external even when a first-party directory shares
// its first segment (database/sql beside a top-level database/). Spellings no node ID can
// take are vacuous on either side: an extglob negation or a leading "!",
// "./", "../" or "/", and a first-party Go package written with its go.mod
// module path (Go node IDs are scan-root-relative). A selector the inventory
// cannot judge, an unsupported file type or a language whose node identities
// are unknown, is not vacuous; rule scope reports it as undetermined instead.
func (inv selectorInventory) vacuous(ruleType, side, pattern string) bool {
	if _, spelled := inv.goModulePathOf(pattern); unmatchableSelector(pattern) || spelled {
		return true
	}
	if inv.matches(ruleType, side, pattern) || inv.undecidable(pattern) {
		return false
	}
	return side == selectorFrom || (inv.firstPartyShaped(pattern) && !inv.namesGoStdlib(pattern))
}

// matches reports whether pattern matches, for some in-scope file, what a
// ruleType rule compares its side selector with.
//
// forbidden_pattern matches its from: against the file path or the file's node
// selector. The dependency rules match graph edge endpoints, which each
// language spells its own way: a target is the module node the file projects
// to (a Go package directory, a TypeScript file, a dotted Python module, a Rust
// crate), and a source is the importing file in a language whose import edges
// start at file nodes (Go, TypeScript) and the module node otherwise. So a Go
// file glob never matches a target, a Go package directory never matches a
// source, and a slash path never matches a Python or Rust endpoint. A file of
// an unsupported type matches by path: no node vocabulary rules it out.
func (inv selectorInventory) matches(ruleType, side, pattern string) bool {
	for _, file := range inv.files {
		language, selector, supported := ruleFileSelector(inv.moduleMap, file, inv.selectors)
		var matched bool
		_, fileSource := fileSourceLanguages[language]
		switch {
		case !supported || ruleType == ruleTypeForbiddenPattern:
			matched = rulePatternMatches(pattern, file, selector)
		case side == selectorFrom && fileSource:
			matched, _ = doublestar.Match(pattern, file)
		case selector != "":
			matched, _ = doublestar.Match(pattern, selector)
		}
		if matched {
			return true
		}
	}
	return false
}

// undecidable reports whether the inventory cannot prove pattern matches
// nothing: it names a file type outside the supported-source inventory, a
// language whose node identities are unknown could spell it, or it names a
// Rust module below a crate. The inventory projects a Rust file only to its
// crate; crate::mod nodes come from cargo-modules or SCIP, so a crate::mod
// selector under a known crate (or with a wildcard crate) cannot be judged.
func (inv selectorInventory) undecidable(pattern string) bool {
	if strings.Contains(pattern, "/") && unsupportedOrAmbiguousSourcePattern(inv.moduleMap, pattern) {
		return true
	}
	if strings.Contains(pattern, sepCrate) {
		crate, _, belowCrate := strings.Cut(literalPrefix(pattern), sepCrate)
		if _, known := inv.roots[sepCrate][crate]; known || !belowCrate {
			return true
		}
	}
	if explicitlySupportedSourcePattern(inv.moduleMap, pattern) {
		return false
	}
	for language := range inv.moduleMap.SelectorLanguages(pattern) {
		if _, unknown := inv.unresolved[language]; unknown {
			return true
		}
	}
	return false
}

// firstPartyShaped reports whether a target selector is spelled as first-party
// source: its literal prefix is empty (it starts with a wildcard) or starts
// with a segment the inventory's paths or node selectors start with, both cut
// in the selector's own vocabulary (selectorSeparator). In the slash
// vocabulary a dot does not separate segments, and a leading segment holding
// one is a Go import-path domain (go.uber.org, k8s.io, github.com), never a
// first-party root: Go module paths are not node IDs.
func (inv selectorInventory) firstPartyShaped(pattern string) bool {
	prefix := literalPrefix(pattern)
	if prefix == "" {
		return true
	}
	sep := selectorSeparator(pattern)
	if sep == "" {
		for _, roots := range inv.roots {
			if _, ok := roots[prefix]; ok {
				return true
			}
		}
		return false
	}
	segment, _, _ := strings.Cut(prefix, sep)
	if sep == sepPath && strings.Contains(segment, ".") {
		return false
	}
	_, ok := inv.roots[sep][segment]
	return ok
}

// namesGoStdlib reports whether a target selector's literal prefix names a Go
// standard-library package or a path ancestor of one, on segment boundaries:
// database/sql and database/** name the standard library, database/shcema does
// not.
func (inv selectorInventory) namesGoStdlib(pattern string) bool {
	prefix := strings.TrimSuffix(literalPrefix(pattern), "/")
	if prefix == "" {
		return false
	}
	for _, pkg := range inv.goStdlib {
		if pkg == prefix || strings.HasPrefix(pkg, prefix+"/") {
			return true
		}
	}
	return false
}

// goModulePathOf returns the loaded Go module path pattern names a first-party
// package by. The Go extractor strips every import path under a loaded module
// path to its scan-root-relative directory, nested go.mod or not, so such a
// selector never matches a node; only the full module path counts, because a
// shorter prefix (github.com/org/**) can still match external dependencies.
func (inv selectorInventory) goModulePathOf(pattern string) (string, bool) {
	for _, module := range inv.goModules {
		if module != "" && (pattern == module || strings.HasPrefix(pattern, module+"/")) {
			return module, true
		}
	}
	return "", false
}

// unmatchableSelector reports a spelling no graph node ID can take. doublestar
// has no negation, so "!(...)" and a leading "!" are literal text; node IDs are
// clean relative paths or dotted/crate names, never "./x", "../x" or "/x".
func unmatchableSelector(pattern string) bool {
	return strings.HasPrefix(pattern, "!") || strings.Contains(pattern, "!(") ||
		strings.HasPrefix(pattern, "./") || strings.HasPrefix(pattern, "../") || strings.HasPrefix(pattern, "/")
}

// literalPrefix is pattern up to its first glob metacharacter.
func literalPrefix(pattern string) string {
	if i := strings.IndexAny(pattern, "*?[{"); i >= 0 {
		return pattern[:i]
	}
	return pattern
}

// selectorSeparator names the vocabulary a selector or node ID is spelled in by
// the separator it carries: a slash first, because Go import paths and file
// names carry dots that separate nothing, then a Rust "::", then a Python dot.
// One with none is "": a single segment any vocabulary can spell.
func selectorSeparator(pattern string) string {
	for _, sep := range [...]string{sepPath, sepCrate, sepDotted} {
		if strings.Contains(pattern, sep) {
			return sep
		}
	}
	return ""
}
