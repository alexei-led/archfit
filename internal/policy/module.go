// Module vocabulary: module definitions, path-to-module resolution, and
// architectural roles. These are policy declarations, not neutral model values —
// owner, layer, subdomain, volatility, and role are exactly the architecture
// decisions this package owns. Config decodes into them; every analysis stage
// consumes them.

package policy

import (
	gopath "path"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/model/graph"
)

// Role declares a module's architectural role. It refines Balanced-Coupling
// distance: a composition root (or generated/test module) fans out to the modules
// it wires by design, so that fan-out is cohesion — not high-distance coupling —
// and must not be scored as unbalanced. Optional; empty means "no role declared"
// (classified as today). See classify for the distance rule.
type Role string

// Role constants. cohesiveRole (in classify) treats composition_root,
// generated, and test as wiring/derived sources whose outbound fan-out is cohesion.
const (
	RoleCompositionRoot Role = "composition_root"
	RoleAdapter         Role = "adapter"
	RoleCore            Role = "core"
	RoleSharedModel     Role = "shared_model"
	RoleGenerated       Role = "generated"
	RoleTest            Role = "test"
)

// validRoles is the accepted set of ModuleDef.role values; empty is allowed.
var validRoles = map[Role]struct{}{
	RoleCompositionRoot: {}, RoleAdapter: {}, RoleCore: {},
	RoleSharedModel: {}, RoleGenerated: {}, RoleTest: {},
}

// ValidRole reports whether r is an accepted module role (empty is allowed).
func ValidRole(r Role) bool {
	if r == "" {
		return true
	}
	_, ok := validRoles[r]
	return ok
}

// ModuleDef defines a module's path ownership and metadata.
// Name kept as ModuleDef (not Def): it is the JSON-schema definition name
// in archfit.schema.json, an external contract.
type ModuleDef struct {
	Paths      []string  `yaml:"paths"`
	Public     []string  `yaml:"public"`
	Internal   []string  `yaml:"internal"`
	Layer      string    `yaml:"layer"`
	Subdomain  string    `yaml:"subdomain"`
	Volatility string    `yaml:"volatility"`
	Owner      string    `yaml:"owner"`
	DeployUnit string    `yaml:"deploy_unit"`
	Role       Role      `yaml:"role,omitempty"`
	ReviewedAt time.Time `yaml:"reviewed_at,omitempty"`
	ReviewedBy string    `yaml:"reviewed_by"`
	// DependsOn is the outbound allowlist: the only declared modules this
	// module may import. Each entry is a module name or a module selector
	// ("layer:<name>", "role:<role>", or a glob over module names). An absent
	// key leaves the module unconstrained; an empty list allows no first-party
	// module. The module_dependencies rule enforces it; external targets are
	// out of its scope.
	DependsOn []string `yaml:"depends_on,omitempty"`
	// VisibleTo is the inbound allowlist: the only declared modules that may
	// import this module, with the same entry forms as depends_on. An absent
	// key leaves the module visible to every module; an importer that no
	// module owns is always denied.
	VisibleTo []string `yaml:"visible_to,omitempty"`
}

// moduleVolatilities are the volatility values classification reads, matched
// case-insensitively; legacy is an alias for frozen.
var moduleVolatilities = map[string]struct{}{"high": {}, "medium": {}, "low": {}, "frozen": {}, "legacy": {}}

// moduleSubdomains are the subdomain values classification maps to a
// volatility (core high, supporting and generic low), matched case-insensitively.
var moduleSubdomains = map[string]struct{}{"core": {}, "supporting": {}, "generic": {}}

// UnknownVolatility reports whether d declares a volatility classification does
// not read. An unknown value is not a declaration: the module's volatility is
// then undeclared. Empty is undeclared, not unknown.
func (d ModuleDef) UnknownVolatility() bool {
	_, known := moduleVolatilities[strings.ToLower(d.Volatility)]
	return d.Volatility != "" && !known
}

// UnknownSubdomain reports whether d declares a subdomain classification does
// not read. Empty is undeclared, not unknown.
func (d ModuleDef) UnknownSubdomain() bool {
	_, known := moduleSubdomains[strings.ToLower(d.Subdomain)]
	return d.Subdomain != "" && !known
}

// ModuleMap resolves a repo-relative path to the owning module name.
// It uses doublestar glob matching against module path patterns.
type ModuleMap struct {
	// sorted module names for deterministic iteration when globs overlap
	names   []string
	modules map[string]ModuleDef
	// crateModules maps each Rust crate spelling (package name and crate
	// identifier) to the declared module that owns the crate. Empty unless
	// WithCrateOwners attached them.
	crateModules map[string]string
}

// buildModuleMap constructs a ModuleMap from the Config's Modules.
// Module names are sorted alphabetically so iteration is deterministic.
func buildModuleMap(modules map[string]ModuleDef) ModuleMap {
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	sort.Strings(names)
	return ModuleMap{names: names, modules: modules}
}

// BuildModuleMap is the exported counterpart of buildModuleMap.
// It lets the engine rebuild a ModuleMap after module augmentation
// (AugmentModulesFromGraph, AugmentGoWorkspaceModules) without importing
// an unexported function.
func BuildModuleMap(modules map[string]ModuleDef) ModuleMap {
	return buildModuleMap(modules)
}

// Has reports whether a module with exactly this name (map key) is configured.
// Distinct from ModuleFor, which matches a repo-relative path against path globs.
func (mm ModuleMap) Has(name string) bool {
	_, ok := mm.modules[name]
	return ok
}

// ModuleFor returns the module name whose path globs match the given
// repo-relative path (forward-slash separated) MOST SPECIFICALLY. Among all
// matching modules, the one whose matching pattern has the longest literal
// prefix (segments before the first wildcard) wins, so a specific stanza
// (internal/model/**) always beats a catch-all (internal/**). Ties break by
// alphabetical module name (iteration is over sorted names with a strict-greater
// comparison, so the result is order-independent and deterministic). Returns
// ("", false) if no module matches.
//
// First-match resolution (the previous behaviour) let a broad catch-all glob
// shadow every specific module, collapsing real cross-module coupling to
// same-module and mis-classifying volatility/distance. Most-specific match makes
// resolution honour the documented "specific stanza wins" intent.
func (mm ModuleMap) ModuleFor(path string) (string, bool) {
	best := ""
	bestSpec := -1
	for _, name := range mm.names {
		for _, pattern := range mm.modules[name].Paths {
			if matched, _ := doublestar.Match(pattern, path); matched {
				if spec := globSpecificity(pattern); spec > bestSpec {
					bestSpec, best = spec, name
				}
			}
		}
	}
	if bestSpec < 0 {
		return "", false
	}
	return best, true
}

// CrateOwners maps each Rust crate spelling — the package name and the crate
// identifier of every crate root — to the declared module that owns the crate:
// the module whose path globs match the package name, the crate identifier, or
// a file in the crate's directory. That is the probe
// classify.AugmentCargoCrateNodes binds crate nodes with. A root crate (Dir "")
// has no directory boundary and is never claimed by one; a crate no module
// declares is left out. A package name wins over another crate's identifier of
// the same spelling, whatever the member order.
//
// Acquisition computes it where crate roots and policy meet; the rules receive
// the result through WithCrateOwners.
func (mm ModuleMap) CrateOwners(roots []graph.CrateRoot) map[string]string {
	owners := make(map[string]string, 2*len(roots))
	owner := make([]string, len(roots))
	for i, cr := range roots {
		owner[i] = mm.crateOwner(cr)
		if owner[i] != "" {
			owners[cr.Name] = owner[i]
		}
	}
	for i, cr := range roots {
		if _, taken := owners[cr.Crate]; !taken && owner[i] != "" && cr.Crate != "" {
			owners[cr.Crate] = owner[i]
		}
	}
	return owners
}

// WithCrateOwners returns mm with the crate owners CrateOwners resolved, so
// ModuleForNode can place a Rust node no path glob claims in the declared
// module that owns its crate. ModuleFor itself is unchanged.
func (mm ModuleMap) WithCrateOwners(owners map[string]string) ModuleMap {
	mm.crateModules = owners
	return mm
}

// crateOwner returns the declared module that owns one crate, or "".
func (mm ModuleMap) crateOwner(cr graph.CrateRoot) string {
	for _, probe := range []string{cr.Name, cr.Crate} {
		if probe == "" {
			continue
		}
		if mod, ok := mm.ModuleFor(probe); ok {
			return mod
		}
	}
	if cr.Dir != "" {
		if mod, ok := mm.ModuleFor(cr.Dir + "/Cargo.toml"); ok {
			return mod
		}
	}
	return ""
}

// ModuleForNode resolves a graph-node path of the given language to its
// declared module. A path glob decides first, exactly as in ModuleFor. A Rust
// node no glob claims — a crate-level package node, or a cargo-modules
// "<crate>::<mod>" node — then belongs to the declared module that owns its
// crate (WithCrateOwners). Without that, a crate declared by package name
// ("yazi-shared") never owns its own "yazi_shared::url::buf" nodes, and the
// rules read two modules of one crate as two unrelated, unowned paths.
//
// The fallback is Rust-only: a Python package or Go directory can share a
// crate's name in a mixed repository, and must never take its module.
func (mm ModuleMap) ModuleForNode(path, language string) (string, bool) {
	if mod, ok := mm.ModuleFor(path); ok {
		return mod, true
	}
	if language != graph.LangRust || len(mm.crateModules) == 0 {
		return "", false
	}
	crate, _, _ := strings.Cut(path, "::")
	mod, ok := mm.crateModules[crate]
	return mod, ok
}

// Allowlist keys, as module_dependencies findings name them in
// matched_by.violates.
const (
	allowlistDependsOn = "depends_on"
	allowlistVisibleTo = "visible_to"
)

// DeniedDependency reports which allowlist keys deny a dependency from module
// from on module to, in the order depends_on, visible_to; nil allows it. from
// is "" for an importer no declared module owns: no depends_on applies to it,
// and a visible_to list always denies it. to must be a declared module, and a
// dependency inside one module is never denied. A list entry is a module
// selector (SelectsModule). This is the one predicate the module_dependencies
// rule decides with.
func (mm ModuleMap) DeniedDependency(from, to string) []string {
	if from == to {
		return nil
	}
	var violates []string
	if allow := mm.modules[from].DependsOn; from != "" && allow != nil && !mm.anySelects(allow, to) {
		violates = append(violates, allowlistDependsOn)
	}
	if allow := mm.modules[to].VisibleTo; allow != nil && (from == "" || !mm.anySelects(allow, from)) {
		violates = append(violates, allowlistVisibleTo)
	}
	return violates
}

// anySelects reports whether one of selectors selects module.
func (mm ModuleMap) anySelects(selectors []string, module string) bool {
	return slices.ContainsFunc(selectors, func(selector string) bool { return mm.SelectsModule(selector, module) })
}

// Module selector prefixes. Any other selector is a glob over module names.
const (
	selectorLayerPrefix = "layer:"
	selectorRolePrefix  = "role:"
)

// SelectsModule reports whether a module selector selects the declared module
// named module: "layer:<name>" selects the modules with that layer,
// "role:<role>" the modules with that role, and any other selector is a
// doublestar glob over module names, so an exact name selects that module.
// Only declared modules are ever selected.
func (mm ModuleMap) SelectsModule(selector, module string) bool {
	def, declared := mm.modules[module]
	if !declared {
		return false
	}
	if layer, ok := strings.CutPrefix(selector, selectorLayerPrefix); ok {
		return layer != "" && def.Layer == layer
	}
	if role, ok := strings.CutPrefix(selector, selectorRolePrefix); ok {
		return role != "" && string(def.Role) == role
	}
	matched, _ := doublestar.Match(selector, module)
	return matched
}

// ModulesSelected returns the declared modules a module selector selects, in
// name order; nil when it selects none.
func (mm ModuleMap) ModulesSelected(selector string) []string {
	var out []string
	for _, name := range mm.names {
		if mm.SelectsModule(selector, name) {
			out = append(out, name)
		}
	}
	return out
}

// DeclaresCrateModulePath reports whether the named module declares a Rust
// crate::mod path, which names module-graph nodes rather than files.
func (mm ModuleMap) DeclaresCrateModulePath(name string) bool {
	return slices.ContainsFunc(mm.modules[name].Paths, func(p string) bool { return strings.Contains(p, "::") })
}

// ValidModuleSelector reports whether selector is well formed: a "layer:" or
// "role:" selector names a value, and a glob is a valid doublestar pattern.
// A well-formed selector may still select no module.
func (mm ModuleMap) ValidModuleSelector(selector string) bool {
	for _, prefix := range []string{selectorLayerPrefix, selectorRolePrefix} {
		if value, ok := strings.CutPrefix(selector, prefix); ok {
			return value != ""
		}
	}
	return selector != "" && doublestar.ValidatePattern(selector)
}

// DeclaresAllowlist reports whether the named module declares depends_on or
// visible_to, empty lists included.
func (mm ModuleMap) DeclaresAllowlist(name string) bool {
	def := mm.modules[name]
	return def.DependsOn != nil || def.VisibleTo != nil
}

// MatchesInternal reports whether the declared module surfaces make a graph-node
// path internal, and which glob decided. An empty glob means no declaration
// speaks about path, so the caller keeps its own language-level signal.
//
// Precedence, the same public-before-internal order the coupling classifier
// applies:
//  1. a public: glob of the module that owns path decides "not internal";
//  2. otherwise any module's internal: glob decides "internal";
//  3. otherwise nothing is decided.
//
// The public side is narrowed to the owning module's own declaration: one
// module's public: glob cannot open another module's surface. The owner is
// resolved with ModuleForNode, so a Rust crate::mod node is owned by the
// module that declares its crate.
//
// path is in the graph-node vocabulary ModuleFor resolves: slash paths for Go
// and TypeScript, dotted IDs for Python, crate::mod for Rust. language is the
// node's language.
func (mm ModuleMap) MatchesInternal(path, language string) (internal bool, glob string) {
	if owner, ok := mm.ModuleForNode(path, language); ok {
		if g, matched := firstMatch(mm.modules[owner].Public, path); matched {
			return false, g
		}
	}
	for _, name := range mm.names {
		if g, matched := firstMatch(mm.modules[name].Internal, path); matched {
			return true, g
		}
	}
	return false, ""
}

// firstMatch returns the first glob in globs that matches path.
func firstMatch(globs []string, path string) (string, bool) {
	for _, g := range globs {
		if matched, _ := doublestar.Match(g, path); matched {
			return g, true
		}
	}
	return "", false
}

// OwnershipTie returns the modules that claim path at the highest specificity
// when more than one does, in name order. ModuleFor then resolves the tie by
// name, so every module after the first silently loses the path. Nil when at
// most one module claims path at the highest specificity.
func (mm ModuleMap) OwnershipTie(path string) []string {
	best := -1
	winners := make([]string, 0, 2)
	for _, name := range mm.names {
		for _, pattern := range mm.modules[name].Paths {
			if matched, _ := doublestar.Match(pattern, path); !matched {
				continue
			}
			switch spec := globSpecificity(pattern); {
			case spec > best:
				best, winners = spec, append(winners[:0], name)
			case spec == best && !slices.Contains(winners, name):
				winners = append(winners, name)
			}
		}
	}
	if len(winners) < 2 {
		return nil
	}
	return winners
}

// globSpecificity ranks a glob pattern by how specific it is: the byte length of
// its literal prefix, i.e. everything before the first wildcard metacharacter
// (* ? [ {). A pattern with no wildcard (an exact path) is maximally specific
// (full length). "internal/model/**" (15) beats "internal/**" (9).
func globSpecificity(pattern string) int {
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*', '?', '[', '{':
			return i
		}
	}
	return len(pattern)
}

// extToLang maps a source-file extension (including the leading dot, e.g.
// ".py") to the canonical language id that claims it, built once from
// graph.BuiltinConventions[*].FileExtensions. Each extension is claimed by
// exactly one language, so map-iteration order during construction is
// immaterial.
var extToLang = buildExtToLang()

func buildExtToLang() map[string]string {
	idx := make(map[string]string)
	for lang, conv := range graph.BuiltinConventions {
		for _, ext := range conv.FileExtensions {
			idx[ext] = lang
		}
	}
	return idx
}

// RuleSelectorForFile returns the supported language and graph-node path that
// policy rule selectors evaluate for a real source file. False means no
// registered source convention owns the extension. Supplied crate roots replace
// directory conventions with Cargo's crate identities; unmatched Rust files
// retain their language but have an unknown selector.
func (mm ModuleMap) RuleSelectorForFile(file string, roots ...graph.CrateRoot) (language, selector string, ok bool) {
	language, ok = extToLang[gopath.Ext(file)]
	if !ok {
		return "", "", false
	}
	if language == graph.LangRust && len(roots) > 0 {
		best := -1
		for _, root := range roots {
			dir := strings.Trim(gopath.Clean(root.Dir), "/")
			if dir == "." {
				dir = ""
			}
			if (dir == "" || strings.HasPrefix(file, dir+"/")) && len(dir) > best {
				selector, best = root.Name, len(dir)
			}
		}
		return language, selector, true
	}
	return language, graph.BuiltinConventions.Lookup(language).FileToModuleKey(file), true
}

// SelectorLanguages returns the languages whose module-node vocabulary the rule
// selector pattern is able to address.
//
// A rule selector is matched against graph node IDs, and those IDs are spelled
// differently per language: Go and TypeScript use slash paths, Python uses
// dotted module IDs, Rust uses `crate::mod`. A selector therefore excludes
// languages by its own shape, independently of which analyzers ran — a slash
// glob can never match a dotted Python node ID, which is why the configuration
// reference insists Python globs are written `prefect.**` and not
// `src/prefect/**`.
//
// The separators come from the shipped conventions (NodeConvention.
// ModuleSegmentSep), so a new language declares its vocabulary once rather than
// teaching this function about itself.
//
// An explicit supported extension names exactly one language and wins. A
// pattern carrying no separator at all ("**") addresses every language, which
// is the conservative answer.
func (mm ModuleMap) SelectorLanguages(pattern string) map[string]struct{} {
	if ext := gopath.Ext(pattern); ext != "" && !strings.ContainsAny(ext, "*?[{") {
		if language, ok := extToLang[ext]; ok {
			return map[string]struct{}{language: {}}
		}
	}
	anySeparator := false
	for _, conv := range graph.BuiltinConventions {
		if conv.ModuleSegmentSep != "" && strings.Contains(pattern, conv.ModuleSegmentSep) {
			anySeparator = true
			break
		}
	}
	out := make(map[string]struct{}, len(graph.BuiltinConventions))
	for language, conv := range graph.BuiltinConventions {
		sep := conv.ModuleSegmentSep
		if sep == "" || !anySeparator || strings.Contains(pattern, sep) {
			out[language] = struct{}{}
		}
	}
	return out
}

// ModuleForFile returns the module name owning file, a repo-relative REAL
// source file path (as opposed to ModuleFor's graph-node-ID space, which is
// dotted for Python and crate-relative for Rust). It tries the raw file path
// against ModuleFor first — that is exactly today's ModuleFor behavior, and
// for Go/TS/Rust configs (directory- or file-path-style globs) it already
// matches, most-specific-glob semantics and all. Only when the raw path
// matches nothing does it derive file's language from its extension and
// retry with the path normalized into that language's node-key form via
// graph.BuiltinConventions[lang].FileToModuleKey (e.g. a Python file becomes
// its dotted module path) — a genuine second chance for configs declared in
// node-key form (the mandated convention, e.g. Python dotted globs), which
// the raw slash path can never match.
//
// Raw-first, not normalize-first: normalizing unconditionally would collapse
// a real file path to a coarser key (e.g. a Rust file to its bare crate name)
// even when the raw path itself would have matched a more specific or
// differently-shaped glob, silently downgrading or losing the match. Trying
// the raw path first preserves ModuleFor's existing, tested resolution for
// every language whose configs are already glob-compatible with real file
// paths, and only reaches for language-specific normalization as a fallback.
// Supplied crate roots use Cargo's identities for the Rust fallback.
func (mm ModuleMap) ModuleForFile(file string, roots ...graph.CrateRoot) (string, bool) {
	if mod, ok := mm.ModuleFor(file); ok {
		return mod, true
	}
	_, key, ok := mm.RuleSelectorForFile(file, roots...)
	if !ok {
		return "", false
	}
	if key == "" || key == file {
		return "", false
	}
	return mm.ModuleFor(key)
}

// IsModuleRoot reports whether dir is the literal root directory of the module
// that owns it (the wildcard-free prefix of one of its declared Paths globs),
// as opposed to an arbitrary subdirectory nested deeper within a larger
// module's tree. Returns false if dir does not resolve to any module.
//
// Used to distinguish a genuine deploy-unit boundary (a main.go at a module's
// own root) from an incidental one (a dev-tool/migration-helper main.go
// buried a few directories inside a much larger module) — the book's distance
// ladder (Methods → Objects → Namespaces/Packages → (Micro)Services → Systems)
// treats a package entry point and a genuinely separate deployable as
// different tiers; tagging every main.go as its own deploy unit regardless of
// depth conflates them.
func (mm ModuleMap) IsModuleRoot(dir string) bool {
	name, ok := mm.ModuleForFile(dir)
	if !ok {
		return false
	}
	for _, pattern := range mm.modules[name].Paths {
		if globRoot(pattern) == dir {
			return true
		}
	}
	return false
}

// globRoot returns the literal (wildcard-free) directory prefix of a glob
// pattern — the part before the first "*"/"?"/"[" meta-character, with any
// trailing path separator trimmed. A pattern with no meta-character is
// returned unchanged (it is itself a literal path).
func globRoot(pattern string) string {
	idx := strings.IndexAny(pattern, "*?[")
	if idx == -1 {
		return pattern
	}
	return strings.TrimSuffix(pattern[:idx], "/")
}

// ModuleRootDirs returns, for every module with at least one Paths glob, the
// literal (wildcard-free) root of its first Paths pattern. Used as the
// agent_tasks files[] last-resort fallback when no finding location resolves
// to a real file. For slash globs the root is a directory prefix; for Python's
// dotted globs (e.g. "myapp.domain.**") it is the dotted module-ID prefix with
// the trailing separator dot trimmed ("myapp.domain") — the resolver turns it
// into a real path via the Python file-candidate probe, never emitting the
// dotted form itself.
func ModuleRootDirs(modules map[string]ModuleDef) map[string]string {
	out := make(map[string]string, len(modules))
	for name, def := range modules {
		if len(def.Paths) == 0 {
			continue
		}
		out[name] = strings.TrimRight(globRoot(def.Paths[0]), ".")
	}
	return out
}

// LayerFor returns the layer name for the module that owns the given graph-node
// path of the given language (ModuleForNode). Returns ("", false) if no module
// matches or the module has no layer set.
func (mm ModuleMap) LayerFor(path, language string) (string, bool) {
	name, ok := mm.ModuleForNode(path, language)
	if !ok {
		return "", false
	}
	def := mm.modules[name]
	if def.Layer == "" {
		return "", false
	}
	return def.Layer, true
}

// LayerForName returns the layer name for a module looked up by its exact name
// (map key). Returns ("", false) if the name is not found or the module has no
// layer set. Use LayerFor when you have a file path; use LayerForName when you
// already hold a module name from ModuleFor.
func (mm ModuleMap) LayerForName(name string) (string, bool) {
	def, ok := mm.modules[name]
	if !ok || def.Layer == "" {
		return "", false
	}
	return def.Layer, true
}
