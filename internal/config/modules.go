package config

import (
	"sort"
	"time"

	"github.com/bmatcuk/doublestar/v4"
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
	Patterns  []PatternDef `yaml:"patterns,omitempty"`
}

// ExceptionDef grants a temporary exception to a rule.
type ExceptionDef struct {
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

// Has reports whether a module with exactly this name (map key) is configured.
// Distinct from ModuleFor, which matches a repo-relative path against path globs.
func (mm ModuleMap) Has(name string) bool {
	_, ok := mm.modules[name]
	return ok
}

// ModuleFor returns the first module name whose path globs match the given
// repo-relative path (forward-slash separated). When multiple modules match,
// the alphabetically-first module name wins (deterministic tiebreak).
// Returns ("", false) if no module matches.
func (mm ModuleMap) ModuleFor(path string) (string, bool) {
	for _, name := range mm.names {
		def := mm.modules[name]
		for _, pattern := range def.Paths {
			matched, _ := doublestar.Match(pattern, path)
			if matched {
				return name, true
			}
		}
	}
	return "", false
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
