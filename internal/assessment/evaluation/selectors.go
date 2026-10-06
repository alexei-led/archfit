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
	// The module-selector sides of forbidden_dependency: they select declared
	// modules, not source.
	selectorFromModule = "from_module"
	selectorToModule   = "to_module"
)

// isModuleSide reports whether a selector side selects declared modules.
func isModuleSide(side string) bool { return side == selectorFromModule || side == selectorToModule }

// selectorMatchesNothingPrefix starts the unevaluated-rule reason for a
// selector that matches nothing. The App keys a policy-defect reason off it, so
// the text is a contract: "selector matches nothing: <from|to> <glob>".
const selectorMatchesNothingPrefix = "selector matches nothing: "

// selectorMatchesNothing is the reason for a rule whose selector is vacuous.
func selectorMatchesNothing(side, glob string) string {
	return selectorMatchesNothingPrefix + side + " " + glob
}

// selectorMatchesOnlyUnanalysedPrefix starts the reason for a dependency rule
// whose selector matches only source no dependency producer analyses (its
// language's extractor finds no project under the analysis root). It must not
// share the "selector matches nothing" prefix: the selector is not a typo, and
// the fix is to analyse that source, not to edit the config.
const selectorMatchesOnlyUnanalysedPrefix = "selector matches only source no dependency producer analyses: "

// vacuityReason is the unevaluated reason for a vacuous selector of a ruleType
// rule.
func (inv selectorInventory) vacuityReason(ruleType, side, glob string) string {
	if inv.matchesOnlyUnanalysed(ruleType, side, glob) {
		return selectorMatchesOnlyUnanalysedPrefix + side + " " + glob
	}
	return selectorMatchesNothing(side, glob)
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

// languageRust is the Rust language id, whose crate::mod module-graph nodes
// have no file of their own.
const languageRust = "rust"

// Selector vocabularies by separator: slash paths (file paths, Go package
// directories, TypeScript files), Python dotted IDs, and Rust crate::mod IDs.
const (
	sepPath   = "/"
	sepDotted = "."
	sepCrate  = "::"
)

// selectorInventory is the rule-scope source inventory as rule selectors see
// it: every in-scope source file, the graph-node selector each file projects
// to, the Rust module-graph nodes, the first-party Go module paths, and the Go
// standard-library packages. Rule evaluation and config lint decide selector
// vacuity with this one predicate. Their inputs differ in two places: check has
// Rust crate names and module graph from cargo metadata and cargo-modules, and
// `check --lang` turns on a language the config switches off, so the in-scope
// files differ.
type selectorInventory struct {
	moduleMap policy.ModuleMap
	files     []string
	selectors map[string]string
	// unanalysed are the files no dependency producer analyses
	// (Observations.UnanalysedFiles). Only forbidden_pattern, which reads the
	// ast-grep pattern pass, matches its selectors against them.
	unanalysed map[string]struct{}
	// outOfScope are the walked files declared out of scope. They are not in
	// files; module rule scope reads them only to know which modules own them.
	outOfScope map[string]struct{}
	// rustModules are the crate::mod node IDs of the Rust module graph, and
	// rustModuleCrates the crates (library spelling) it covers: those with
	// nodes there and those cargo-modules graphed with no submodule.
	rustModules      []string
	rustModuleCrates map[string]struct{}
	// roots are the leading segments of every file path and node selector,
	// keyed by the separator of the vocabulary they are spelled in (sepPath,
	// sepDotted, sepCrate). A target selector whose leading segment, cut in its
	// own vocabulary, is one of them names first-party source.
	roots map[string]map[string]struct{}
	// unresolved are the languages whose node identity the inventory could not
	// name for some analysed file. Without cargo metadata Rust crate names are
	// unknown, so a selector Rust could spell cannot be proven to match nothing.
	unresolved map[string]struct{}
	goModules  []string
	goStdlib   []string
}

// newSelectorInventory builds the inventory over files, the rule-scope source
// inventory (sourceInventoryFiles) of f.
func newSelectorInventory(moduleMap policy.ModuleMap, files []string, f Observations) selectorInventory {
	inv := selectorInventory{
		moduleMap: moduleMap, files: files, selectors: f.SourceSelectors,
		unanalysed: f.UnanalysedFiles, outOfScope: f.OutOfScopeFiles,
		rustModules: f.RustModuleNodes, rustModuleCrates: map[string]struct{}{},
		roots:      map[string]map[string]struct{}{sepPath: {}, sepDotted: {}, sepCrate: {}},
		unresolved: map[string]struct{}{}, goModules: f.GoModulePaths, goStdlib: f.GoStdlibPackages,
	}
	for _, file := range inv.files {
		inv.addRoot(sepPath, file)
		language, selector, supported := ruleFileSelector(moduleMap, file, f.SourceSelectors)
		_, unanalysed := inv.unanalysed[file]
		switch {
		case !supported:
		case selector == "" && strings.Contains(file, "/"):
			// A file below the root always has a node identity; an empty one was
			// withheld by the producer projection. A file no producer analyses
			// (a Rust file outside every workspace member) has none to withhold.
			if !unanalysed {
				inv.unresolved[language] = struct{}{}
			}
		case selector != "" && selector != file:
			inv.addRoot(selectorSeparator(selector), selector)
			if language == languageRust {
				// Cargo names the package ("my-core"); crate::mod node IDs use the
				// library name (my_core).
				inv.roots[sepCrate][rustLibName(selector)] = struct{}{}
			}
		}
	}
	// A loaded crate is a crate:: root whatever its files project to: a
	// binary target's crate identifier (yazi) differs from its package (yazi-fm).
	for _, crate := range f.RustCrates {
		inv.roots[sepCrate][crate] = struct{}{}
	}
	for _, crate := range f.RustModuleGraphCrates {
		inv.rustModuleCrates[rustLibName(crate)] = struct{}{}
		inv.roots[sepCrate][rustLibName(crate)] = struct{}{}
	}
	for _, node := range inv.rustModules {
		crate, _, _ := strings.Cut(node, sepCrate)
		inv.rustModuleCrates[crate] = struct{}{}
		inv.roots[sepCrate][crate] = struct{}{}
	}
	return inv
}

// rustLibName is the library spelling of a Cargo package name.
func rustLibName(crate string) string {
	return strings.ReplaceAll(crate, "-", "_")
}

// analysedFiles are the inventory files a dependency producer analyses: the
// rule scope of every rule type but forbidden_pattern.
func (inv selectorInventory) analysedFiles() []string {
	if len(inv.unanalysed) == 0 {
		return inv.files
	}
	out := make([]string, 0, len(inv.files))
	for _, file := range inv.files {
		if _, unanalysed := inv.unanalysed[file]; !unanalysed {
			out = append(out, file)
		}
	}
	return out
}

// matchesRustModule reports whether pattern matches a Rust module-graph node.
func (inv selectorInventory) matchesRustModule(pattern string) bool {
	for _, node := range inv.rustModules {
		if matched, _ := doublestar.Match(pattern, node); matched {
			return true
		}
	}
	return false
}

// rustModulePath judges a crate::mod pattern against the module graph:
// decided when the crate's module graph is in hand (matched says whether a
// node matches), or when the crate is not a loaded crate at all (nothing can
// match). A wildcard crate, or a loaded crate the module graph does not cover,
// is undecided: an absent module graph is never an empty one.
func (inv selectorInventory) rustModulePath(pattern string) (matched, decided bool) {
	crate, _, belowCrate := strings.Cut(literalPrefix(pattern), sepCrate)
	switch {
	case !belowCrate:
		return false, false
	case inv.hasRustModuleGraph(crate):
		return inv.matchesRustModule(pattern), true
	default:
		_, known := inv.roots[sepCrate][crate]
		_, unresolved := inv.unresolved[languageRust]
		return false, !known && !unresolved
	}
}

// hasRustModuleGraph reports whether the module graph covers crate.
func (inv selectorInventory) hasRustModuleGraph(crate string) bool {
	_, ok := inv.rustModuleCrates[rustLibName(crate)]
	return ok
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
	for _, s := range [...]struct{ side, selector string }{{selectorFromModule, rule.FromModule}, {selectorToModule, rule.ToModule}} {
		if s.selector == "" {
			continue
		}
		if state := inv.moduleSideState(s.selector); state == moduleSideEmpty || state == moduleSideUnanalysed {
			return s.side, s.selector, true
		}
	}
	return "", "", false
}

// moduleSide states what the modules a module selector selects give a
// dependency rule to read.
type moduleSide uint8

const (
	// moduleSideLive: a selected module owns analysed source, or a module-graph
	// node of a crate::mod path.
	moduleSideLive moduleSide = iota
	// moduleSideUndecided: a selected module owns no inventoried file and its
	// paths name nothing the inventory can judge (a directory glob, a Rust
	// package name without crate roots, a crate::mod path without the module
	// graph). Rule scope abstains on it, as moduleRuleScope does.
	moduleSideUndecided
	// moduleSideUnanalysed: the selected modules own only source no dependency
	// producer analyses.
	moduleSideUnanalysed
	// moduleSideEmpty: no module is selected, or the selected modules provably
	// own nothing.
	moduleSideEmpty
)

// moduleSideState judges a module selector the way moduleRuleScope judges a
// module: by the in-scope files each selected module owns, and for a module
// with no owned file, by its paths — a crate::mod path against the module
// graph, an explicit source-file path as provably empty, anything else as
// undecidable. A module side is then held to the path-glob standard: empty
// matches nothing, unanalysed-only is the unanalysed case, and undecidable
// abstains instead of guessing either way.
func (inv selectorInventory) moduleSideState(selector string) moduleSide {
	selected := make(map[string]struct{})
	for _, module := range inv.moduleMap.ModulesSelected(selector) {
		selected[module] = struct{}{}
	}
	if len(selected) == 0 {
		return moduleSideEmpty
	}
	owning := make(map[string]struct{})
	unanalysedOnly := false
	for _, file := range inv.files {
		module, ok := inv.moduleMap.ModuleFor(file)
		if !ok {
			if _, sel, _ := ruleFileSelector(inv.moduleMap, file, inv.selectors); sel != "" {
				module, ok = inv.moduleMap.ModuleFor(sel)
			}
		}
		if _, in := selected[module]; !ok || !in {
			continue
		}
		if _, unanalysed := inv.unanalysed[file]; !unanalysed {
			return moduleSideLive
		}
		owning[module] = struct{}{}
		unanalysedOnly = true
	}
	undecided := false
	for module := range selected {
		if _, owns := owning[module]; owns {
			continue
		}
		switch inv.modulePathsState(module) {
		case moduleSideLive:
			return moduleSideLive
		case moduleSideUndecided:
			undecided = true
		}
	}
	switch {
	case undecided:
		return moduleSideUndecided
	case unanalysedOnly:
		return moduleSideUnanalysed
	default:
		return moduleSideEmpty
	}
}

// ownsRustModuleNode reports whether a module-graph node that pattern matches
// resolves to module: a crate-wide catch-all shadowed by more specific
// modules on every node owns nothing.
func (inv selectorInventory) ownsRustModuleNode(module, pattern string) bool {
	for _, node := range inv.rustModules {
		if matched, _ := doublestar.Match(pattern, node); !matched {
			continue
		}
		if owner, ok := inv.moduleMap.ModuleFor(node); ok && owner == module {
			return true
		}
	}
	return false
}

// modulePathsState judges a module that owns no inventoried file by its
// declared paths, with moduleRuleScope's rules.
func (inv selectorInventory) modulePathsState(module string) moduleSide {
	paths := inv.moduleMap.Paths(module)
	if len(paths) == 0 {
		return moduleSideUndecided
	}
	state := moduleSideEmpty
	for _, pattern := range paths {
		if strings.Contains(pattern, sepCrate) {
			if _, decided := inv.rustModulePath(pattern); !decided {
				state = moduleSideUndecided
			} else if inv.ownsRustModuleNode(module, pattern) {
				return moduleSideLive
			}
			continue
		}
		if !explicitlySupportedSourcePattern(inv.moduleMap, pattern) {
			state = moduleSideUndecided
		}
	}
	return state
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
//
// Every rule type but forbidden_pattern reads dependency edges, so only the
// files a dependency producer analyses can match, plus the Rust module-graph
// nodes, which have no file of their own.
func (inv selectorInventory) matches(ruleType, side, pattern string) bool {
	patternPass := ruleType == ruleTypeForbiddenPattern
	for _, file := range inv.files {
		if _, unanalysed := inv.unanalysed[file]; unanalysed && !patternPass {
			continue
		}
		if inv.fileMatches(ruleType, side, pattern, file) {
			return true
		}
	}
	return !patternPass && inv.matchesRustModule(pattern)
}

// matchesOnlyUnanalysed reports whether a dependency rule's selector, which
// matches nothing a dependency producer analyses, matches source no producer
// analyses: the rule is aimed at code archfit cannot read relationships from
// in this tree, which is not a selector typo.
func (inv selectorInventory) matchesOnlyUnanalysed(ruleType, side, pattern string) bool {
	if ruleType == ruleTypeForbiddenPattern {
		return false
	}
	if isModuleSide(side) {
		return inv.moduleSideState(pattern) == moduleSideUnanalysed
	}
	for file := range inv.unanalysed {
		if inv.fileMatches(ruleType, side, pattern, file) {
			return true
		}
	}
	return false
}

// fileMatches reports whether pattern matches what a ruleType rule compares
// its side selector with for one file.
func (inv selectorInventory) fileMatches(ruleType, side, pattern, file string) bool {
	language, selector, supported := ruleFileSelector(inv.moduleMap, file, inv.selectors)
	_, fileSource := fileSourceLanguages[language]
	var matched bool
	switch {
	case !supported || ruleType == ruleTypeForbiddenPattern:
		matched = rulePatternMatches(pattern, file, selector)
	case side == selectorFrom && fileSource:
		matched, _ = doublestar.Match(pattern, file)
	case selector != "":
		matched, _ = doublestar.Match(pattern, selector)
	}
	return matched
}

// undecidable reports whether the inventory cannot prove pattern matches
// nothing: it names a file type outside the supported-source inventory, a
// language whose node identities are unknown could spell it, or it names a
// Rust module the module graph does not cover. The inventory projects a Rust
// file only to its crate, and crate::mod nodes come from cargo-modules: a
// crate::mod selector is judged against them when they cover its crate, and
// is undecidable under a wildcard crate or a loaded crate they do not cover.
func (inv selectorInventory) undecidable(pattern string) bool {
	if strings.Contains(pattern, "/") && unsupportedOrAmbiguousSourcePattern(inv.moduleMap, pattern) {
		return true
	}
	if strings.Contains(pattern, sepCrate) {
		crate, _, belowCrate := strings.Cut(literalPrefix(pattern), sepCrate)
		if belowCrate && inv.hasRustModuleGraph(crate) {
			return false
		}
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
