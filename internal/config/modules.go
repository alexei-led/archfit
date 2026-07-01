package config

import (
	gopath "path"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/model/graph"
)

// ModuleRole declares a module's architectural role. It refines Balanced-Coupling
// distance: a composition root (or generated/test module) fans out to the modules
// it wires by design, so that fan-out is cohesion — not high-distance coupling —
// and must not be scored as unbalanced. Optional; empty means "no role declared"
// (classified as today). See classify for the distance rule.
type ModuleRole string

// ModuleRole constants. cohesiveRole (in classify) treats composition_root,
// generated, and test as wiring/derived sources whose outbound fan-out is cohesion.
const (
	RoleCompositionRoot ModuleRole = "composition_root"
	RoleAdapter         ModuleRole = "adapter"
	RoleCore            ModuleRole = "core"
	RoleSharedModel     ModuleRole = "shared_model"
	RoleGenerated       ModuleRole = "generated"
	RoleTest            ModuleRole = "test"
)

// moduleRoles is the accepted set of ModuleDef.role values; empty is allowed.
var moduleRoles = map[ModuleRole]struct{}{
	RoleCompositionRoot: {}, RoleAdapter: {}, RoleCore: {},
	RoleSharedModel: {}, RoleGenerated: {}, RoleTest: {},
}

// ModuleDef defines a module's path ownership and metadata.
type ModuleDef struct {
	Paths      []string   `yaml:"paths"`
	Public     []string   `yaml:"public"`
	Internal   []string   `yaml:"internal"`
	Layer      string     `yaml:"layer"`
	Subdomain  string     `yaml:"subdomain"`
	Volatility string     `yaml:"volatility"`
	Owner      string     `yaml:"owner"`
	DeployUnit string     `yaml:"deploy_unit"`
	Role       ModuleRole `yaml:"role,omitempty"`
	ReviewedAt time.Time  `yaml:"reviewed_at,omitempty"`
	ReviewedBy string     `yaml:"reviewed_by"`
}

// RuleDef declares a single architecture rule.
type RuleDef struct {
	ID        string       `yaml:"id"`
	Type      string       `yaml:"type"`
	Gate      string       `yaml:"gate"`
	From      string       `yaml:"from"`
	To        string       `yaml:"to"`
	FromLayer string       `yaml:"from_layer"`
	ToLayer   string       `yaml:"to_layer"`
	Max       *int         `yaml:"max,omitempty"`       // public_api_max: exported-declaration ceiling per module
	Threshold *int         `yaml:"threshold,omitempty"` // reserved: per-rule integer threshold
	Patterns  []PatternDef `yaml:"patterns,omitempty"`
}

// WaiverDef grants an approved, time-boxed deviation from a rule (`waivers:`).
// A finding matching a waiver is suppressed until `expires` passes, after which
// it gates again. reason/approved_by record the governance trail.
type WaiverDef struct {
	Rule       string `yaml:"rule"`
	From       string `yaml:"from"`
	To         string `yaml:"to"`
	Reason     string `yaml:"reason"`
	ApprovedBy string `yaml:"approved_by"`
	Expires    string `yaml:"expires"`
}

// ModuleMap resolves a repo-relative path to the owning module name.
// It uses doublestar glob matching against module path patterns.
type ModuleMap struct {
	// sorted module names for deterministic iteration when globs overlap
	names   []string
	modules map[string]ModuleDef
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
func (mm ModuleMap) ModuleForFile(file string) (string, bool) {
	if mod, ok := mm.ModuleFor(file); ok {
		return mod, true
	}
	lang, ok := extToLang[gopath.Ext(file)]
	if !ok {
		return "", false
	}
	key := graph.BuiltinConventions.Lookup(lang).FileToModuleKey(file)
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

// LayerFor returns the layer name for the module that owns the given repo-relative
// path. Returns ("", false) if no module matches or the module has no layer set.
func (mm ModuleMap) LayerFor(path string) (string, bool) {
	name, ok := mm.ModuleFor(path)
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
