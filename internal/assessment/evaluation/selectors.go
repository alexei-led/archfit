package evaluation

import (
	"strings"

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
	// roots are the leading segments of every file path and node selector. A
	// target selector that starts with one of them names first-party source.
	roots map[string]struct{}
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
		roots: map[string]struct{}{}, unresolved: map[string]struct{}{}, goModules: f.GoModulePaths,
		goStdlib: f.GoStdlibPackages,
	}
	for _, file := range inv.files {
		inv.roots[leadingSegment(file)] = struct{}{}
		language, selector, supported := ruleFileSelector(moduleMap, file, f.SourceSelectors)
		switch {
		case !supported:
		case selector == "" && strings.Contains(file, "/"):
			// A file below the root always has a node identity; an empty one was
			// withheld by the producer projection.
			inv.unresolved[language] = struct{}{}
		case selector != "" && selector != file:
			inv.roots[leadingSegment(selector)] = struct{}{}
		}
	}
	return inv
}

// vacuousSelector returns the first selector of rule that provably matches
// nothing the rule can see. Only the rule types that read selectors are
// checked, and an empty selector means "match all", so it is never vacuous.
func (inv selectorInventory) vacuousSelector(rule policy.RuleDef) (side, glob string, vacuous bool) {
	if _, reads := selectorRuleTypes[rule.Type]; !reads {
		return "", "", false
	}
	for _, s := range [...]struct{ side, glob string }{{selectorFrom, rule.From}, {selectorTo, rule.To}} {
		if s.glob != "" && inv.vacuous(s.side, s.glob) {
			return s.side, s.glob, true
		}
	}
	return "", "", false
}

// vacuous reports whether one selector matches nothing the rule can see.
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
func (inv selectorInventory) vacuous(side, pattern string) bool {
	if unmatchableSelector(pattern) || inv.spelledWithGoModulePath(pattern) {
		return true
	}
	if inv.matches(pattern) || inv.undecidable(pattern) {
		return false
	}
	return side == selectorFrom || (inv.firstPartyShaped(pattern) && !inv.namesGoStdlib(pattern))
}

// matches reports whether pattern matches an in-scope source file or the node
// selector it projects to: the same test rule scope applies to every file.
func (inv selectorInventory) matches(pattern string) bool {
	for _, file := range inv.files {
		_, selector, _ := ruleFileSelector(inv.moduleMap, file, inv.selectors)
		if rulePatternMatches(pattern, file, selector) {
			return true
		}
	}
	return false
}

// undecidable reports whether the inventory cannot prove pattern matches
// nothing: it names a file type outside the supported-source inventory, or a
// language whose node identities are unknown could spell it.
func (inv selectorInventory) undecidable(pattern string) bool {
	if strings.Contains(pattern, "/") && unsupportedOrAmbiguousSourcePattern(inv.moduleMap, pattern) {
		return true
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
// with a segment the inventory's paths or node selectors start with.
func (inv selectorInventory) firstPartyShaped(pattern string) bool {
	prefix := literalPrefix(pattern)
	if prefix == "" {
		return true
	}
	_, ok := inv.roots[leadingSegment(prefix)]
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

// spelledWithGoModulePath reports whether pattern names a first-party Go
// package by its import path. Go node IDs drop the module path, so such a
// selector never matches; only the full module path counts, because a shorter
// prefix (github.com/org/**) can still match external dependencies.
func (inv selectorInventory) spelledWithGoModulePath(pattern string) bool {
	for _, module := range inv.goModules {
		if module != "" && (pattern == module || strings.HasPrefix(pattern, module+"/")) {
			return true
		}
	}
	return false
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

// leadingSegment is s up to its first path, dotted-module, or crate separator,
// so slash paths, Python dotted IDs, and Rust crate::mod IDs share one root
// vocabulary.
func leadingSegment(s string) string {
	end := len(s)
	for _, sep := range [...]string{"/", ".", "::"} {
		if i := strings.Index(s, sep); i >= 0 && i < end {
			end = i
		}
	}
	return s[:end]
}
