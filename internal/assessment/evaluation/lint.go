package evaluation

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/policy"
)

// Config-lint severities. An error makes `archfit config lint` exit 1.
const (
	LintSeverityError   = "error"
	LintSeverityWarning = "warning"
	LintSeverityInfo    = "info"
)

// Config-lint diagnostic codes (archfit.config-lint.v1). New codes are
// additive; an existing code keeps its meaning.
const (
	LintDeadSelector         = "dead_selector"
	LintGuardRule            = "guard_rule"
	LintGuardMatchesSource   = "guard_matches_source"
	LintUnknownVolatility    = "unknown_volatility"
	LintUnknownSubdomain     = "unknown_subdomain"
	LintUndeclaredLayer      = "undeclared_layer"
	LintPublicOutsideModule  = "public_outside_module"
	LintPublicMatchesNothing = "public_matches_nothing"
	LintAmbiguousOwnership   = "ambiguous_ownership"
)

// PolicyDiagnostic is one configuration defect: a code, a severity, the config
// path it points at, and a one-line message.
type PolicyDiagnostic struct {
	Code     string
	Severity string
	Path     string
	Message  string
}

// IsError reports whether the diagnostic fails `archfit config lint`.
func (d PolicyDiagnostic) IsError() bool { return d.Severity == LintSeverityError }

// LintPolicy reports the configuration defects that loading accepts but that
// silently weaken the policy, judged against the rule-scope source inventory:
// dead rule selectors (the predicate rule scope uses; see selectorInventory for
// where lint and check inputs differ),
// guard rules, unknown module volatility or subdomain values, layers missing
// from `layers:`, public entries outside their module or matching no source,
// and source paths two modules claim at equal specificity. Diagnostics are
// sorted by path, then code, then message, so two ties that name the same first
// module keep one order.
func LintPolicy(p policy.PolicySnapshot, f Observations) []PolicyDiagnostic {
	inv := newSelectorInventory(p.Topology.ModuleMap, sourceInventoryFiles(f), f)
	out := ruleDiagnostics(p.Gates.Rules.Rules, inv)
	out = append(out, moduleValueDiagnostics(p.Topology)...)
	out = append(out, publicSurfaceDiagnostics(p.Topology, inv)...)
	out = append(out, ownershipDiagnostics(p.Topology.ModuleMap, inv)...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		if out[i].Code != out[j].Code {
			return out[i].Code < out[j].Code
		}
		return out[i].Message < out[j].Message
	})
	return out
}

// PolicyWarnings are the configuration defects analysis runs disclose as
// config warnings: an unknown module volatility or subdomain and an undeclared
// layer (schema v2 still loads them), and a warn-gated rule whose selector
// matches nothing. A fail-gated vacuous rule is not repeated: the decision's
// unevaluated required rules already name it.
func PolicyWarnings(p policy.PolicySnapshot, f Observations) []string {
	var out []string
	for _, d := range moduleValueDiagnostics(p.Topology) {
		out = append(out, d.Path+": "+d.Message)
	}
	inv := newSelectorInventory(p.Topology.ModuleMap, sourceInventoryFiles(f), f)
	for _, rule := range p.Gates.Rules.Rules {
		if rule.Gate != string(policy.GateWarn) || rule.Guard {
			continue
		}
		if side, glob, vacuous := inv.vacuousSelector(rule); vacuous {
			out = append(out, "rules["+rule.ID+"] is not evaluated: "+selectorMatchesNothing(side, glob)+
				" — fix the selector or set guard: true")
		}
	}
	return out
}

// ruleDiagnostics reports dead selectors and lists guard rules. A dead selector
// on a gate: off rule is a warning: the rule is compiled out, so nothing is
// vacuously passing, but it will be when the gate is turned back on.
func ruleDiagnostics(rules []policy.RuleDef, inv selectorInventory) []PolicyDiagnostic {
	var out []PolicyDiagnostic
	for _, rule := range rules {
		if _, reads := selectorRuleTypes[rule.Type]; !reads {
			continue
		}
		side, glob, vacuous := inv.vacuousSelector(rule)
		path := "rules[" + rule.ID + "]"
		switch {
		case rule.Guard && vacuous:
			out = append(out, PolicyDiagnostic{Code: LintGuardRule, Severity: LintSeverityInfo, Path: path,
				Message: "guard rule: " + side + " " + glob + " matches nothing by design"})
		case rule.Guard && inv.matches(rule.Type, selectorFrom, orMatchAll(rule.From)) &&
			inv.matches(rule.Type, selectorTo, orMatchAll(rule.To)):
			out = append(out, PolicyDiagnostic{Code: LintGuardMatchesSource, Severity: LintSeverityWarning, Path: path,
				Message: "guard rule matches scanned source: the guarded path exists — remove the code or drop guard: true"})
		case vacuous:
			severity := LintSeverityError
			if rule.Gate == string(policy.GateOff) {
				severity = LintSeverityWarning
			}
			out = append(out, PolicyDiagnostic{Code: LintDeadSelector, Severity: severity, Path: path + "." + side,
				Message: side + ": " + glob + " matches no scanned source" + inv.vacuityHint(glob) +
					"; fix the selector or set guard: true"})
		}
	}
	return out
}

// orMatchAll spells an empty rule selector, which means "match all", as the
// glob that matches everything.
func orMatchAll(selector string) string {
	if selector == "" {
		return "**"
	}
	return selector
}

// vacuityHint names why a dead selector can never match, when the spelling
// itself is the defect.
func (inv selectorInventory) vacuityHint(glob string) string {
	if unmatchableSelector(glob) {
		return " (selectors are relative paths or dotted/crate names: no leading !, ./, ../ or /, and no !( negation)"
	}
	if module, spelled := inv.goModulePathOf(glob); spelled {
		return " (selectors are scan-root-relative: drop the Go module path " + module + "/)"
	}
	return ""
}

// moduleValueDiagnostics reports module values classification cannot read: an
// unknown volatility or subdomain leaves the module's volatility undeclared,
// and a layer missing from `layers:` drops the module from layer ranking.
func moduleValueDiagnostics(topology policy.TopologyView) []PolicyDiagnostic {
	var out []PolicyDiagnostic
	for _, name := range sortedModuleNames(topology.Modules) {
		def := topology.Modules[name]
		path := "modules." + name
		if def.UnknownVolatility() {
			out = append(out, PolicyDiagnostic{Code: LintUnknownVolatility, Severity: LintSeverityError, Path: path + ".volatility",
				Message: fmt.Sprintf("volatility %q is not one of high, medium, low, frozen, legacy; the module's volatility reads as undeclared", def.Volatility)})
		}
		if def.UnknownSubdomain() {
			out = append(out, PolicyDiagnostic{Code: LintUnknownSubdomain, Severity: LintSeverityError, Path: path + ".subdomain",
				Message: fmt.Sprintf("subdomain %q is not one of core, supporting, generic; it sets no volatility", def.Subdomain)})
		}
		if def.Layer != "" && !slices.Contains(topology.Layers, def.Layer) {
			out = append(out, PolicyDiagnostic{Code: LintUndeclaredLayer, Severity: LintSeverityError, Path: path + ".layer",
				Message: fmt.Sprintf("layer %q is not declared in layers: %v; forbidden_layer_direction skips the module", def.Layer, topology.Layers)})
		}
	}
	return out
}

// publicSurfaceDiagnostics reports public entries a module does not own or
// that name no scanned node. Public globs match edge targets, which are graph
// nodes (a Go package directory, a TypeScript file, a dotted Python module),
// so a stale entry silences a real intrusive edge.
//
// Ownership is judged on the nodes an entry matches, so glob syntax on either
// side ({a,b}, [ab]) compares by what it matches, not by its text. An entry
// that matches no scanned node is judged by its own text, read as a path.
func publicSurfaceDiagnostics(topology policy.TopologyView, inv selectorInventory) []PolicyDiagnostic {
	var out []PolicyDiagnostic
	for _, name := range sortedModuleNames(topology.Modules) {
		def := topology.Modules[name]
		owned := func(node string) bool {
			return slices.ContainsFunc(def.Paths, func(glob string) bool {
				matched, _ := doublestar.Match(glob, node)
				return matched
			})
		}
		for i, pub := range def.Public {
			path := fmt.Sprintf("modules.%s.public[%d]", name, i)
			nodes := inv.nodesMatching(pub)
			outside := slices.ContainsFunc(nodes, func(n string) bool { return !owned(n) })
			if len(nodes) == 0 {
				outside = !owned(pub)
			}
			if outside {
				out = append(out, PolicyDiagnostic{Code: LintPublicOutsideModule, Severity: LintSeverityError, Path: path,
					Message: fmt.Sprintf("public entry %q is outside the module's own paths %v", pub, def.Paths)})
			}
			if len(nodes) == 0 && !inv.undecidable(pub) {
				out = append(out, PolicyDiagnostic{Code: LintPublicMatchesNothing, Severity: LintSeverityError, Path: path,
					Message: fmt.Sprintf("public entry %q names no scanned package or module", pub)})
			}
		}
	}
	return out
}

// ownershipDiagnostics reports source paths two or more modules claim at equal
// specificity, one diagnostic per tied module set. Most-specific-wins resolves
// genuine nesting; an exact tie is resolved by module name, so every module
// but the first silently loses the paths.
func ownershipDiagnostics(mm policy.ModuleMap, inv selectorInventory) []PolicyDiagnostic {
	type tie struct {
		modules []string
		example string
		count   int
	}
	ties := map[string]*tie{}
	seen := map[string]struct{}{}
	for _, file := range inv.files {
		_, selector, _ := ruleFileSelector(inv.moduleMap, file, inv.selectors)
		for _, path := range []string{file, selector} {
			if _, done := seen[path]; path == "" || done {
				continue
			}
			seen[path] = struct{}{}
			modules := mm.OwnershipTie(path)
			if modules == nil {
				continue
			}
			key := strings.Join(modules, "\x00")
			if ties[key] == nil {
				ties[key] = &tie{modules: modules, example: path}
			}
			t := ties[key]
			t.count++
			if path < t.example {
				t.example = path
			}
		}
	}
	keys := make([]string, 0, len(ties))
	for key := range ties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]PolicyDiagnostic, 0, len(ties))
	for _, key := range keys {
		t := ties[key]
		out = append(out, PolicyDiagnostic{Code: LintAmbiguousOwnership, Severity: LintSeverityError,
			Path: "modules." + t.modules[0] + ".paths",
			Message: fmt.Sprintf("%d source path(s), e.g. %s, are claimed at equal specificity by modules %s; %s wins by name",
				t.count, t.example, strings.Join(t.modules, ", "), t.modules[0])})
	}
	return out
}

// nodesMatching returns the distinct graph-node selectors of in-scope files
// that pattern matches: what a public glob is matched against.
func (inv selectorInventory) nodesMatching(pattern string) []string {
	var nodes []string
	seen := map[string]struct{}{}
	for _, file := range inv.files {
		_, selector, _ := ruleFileSelector(inv.moduleMap, file, inv.selectors)
		if _, done := seen[selector]; selector == "" || done {
			continue
		}
		seen[selector] = struct{}{}
		if matched, _ := doublestar.Match(pattern, selector); matched {
			nodes = append(nodes, selector)
		}
	}
	return nodes
}

func sortedModuleNames(modules map[string]policy.ModuleDef) []string {
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
