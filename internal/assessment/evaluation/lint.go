package evaluation

import (
	"fmt"
	"slices"
	"sort"

	"github.com/alexei-led/archfit/v3/internal/policy"
)

// Severity of a module-value diagnostic reported by PolicyWarnings.
const (
	LintSeverityError = "error"
)

// Module-value diagnostic codes used by PolicyWarnings.
const (
	LintUnknownVolatility = "unknown_volatility"
	LintUnknownSubdomain  = "unknown_subdomain"
	LintUndeclaredLayer   = "undeclared_layer"
	LintUnknownModule     = "unknown_module"
)

// PolicyDiagnostic is one configuration defect: a code, a severity, the config
// path it points at, and a one-line message.
type PolicyDiagnostic struct {
	Code     string
	Severity string
	Path     string
	Message  string
}

// PolicyWarnings are the configuration defects analysis runs disclose as
// config warnings: an unknown module volatility or subdomain, an undeclared
// layer, and an allowlist entry naming no module (schema v2 still loads them),
// and a warn-gated rule whose selector matches nothing or that has no allowlist
// to enforce. A fail-gated vacuous rule is not repeated: the decision's
// unevaluated required rules already name it.
func PolicyWarnings(p policy.PolicySnapshot, f Observations) []string {
	var out []string
	for _, d := range moduleValueDiagnostics(p.Topology) {
		out = append(out, d.Path+": "+d.Message)
	}
	inv := newSelectorInventory(p.Topology.ModuleMap, sourceInventoryFiles(f), f)
	for _, rule := range p.Gates.Rules.Rules {
		if rule.Gate != string(policy.GateWarn) {
			continue
		}
		if rule.Type == ruleTypeModuleDependencies && !declaresAnyAllowlist(p.Topology) {
			out = append(out, "rules["+rule.ID+"] is not evaluated: "+allowlistsDeclareNothing)
			continue
		}
		if rule.Guard {
			continue
		}
		side, glob, vacuous := inv.vacuousSelector(rule)
		switch {
		case !vacuous:
		case inv.matchesOnlyUnanalysed(rule.Type, side, glob):
			out = append(out, "rules["+rule.ID+"] is not evaluated: "+inv.vacuityReason(rule.Type, side, glob)+
				" — its language's extractor finds no project under the analysis root")
		default:
			out = append(out, "rules["+rule.ID+"] is not evaluated: "+selectorMatchesNothing(side, glob)+
				" — fix the selector or set guard: true")
		}
	}
	return out
}

// declaresAnyAllowlist reports whether any module declares depends_on or
// visible_to, an empty list included.
func declaresAnyAllowlist(topology policy.TopologyView) bool {
	for name := range topology.Modules {
		if topology.ModuleMap.DeclaresAllowlist(name) {
			return true
		}
	}
	return false
}

// moduleValueDiagnostics reports module values the engine cannot read: an
// unknown volatility or subdomain leaves the module's volatility undeclared, a
// layer missing from `layers:` drops the module from layer ranking, and a
// depends_on or visible_to entry that selects no declared module allows
// nothing.
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
		for _, list := range []struct {
			key     string
			entries []string
		}{{"depends_on", def.DependsOn}, {"visible_to", def.VisibleTo}} {
			key := list.key
			for i, entry := range list.entries {
				if len(topology.ModuleMap.ModulesSelected(entry)) == 0 {
					out = append(out, PolicyDiagnostic{Code: LintUnknownModule, Severity: LintSeverityError, Path: fmt.Sprintf("%s.%s[%d]", path, key, i),
						Message: fmt.Sprintf("%s entry %q names no declared module, layer, or role; it allows nothing", key, entry)})
				}
			}
		}
	}
	return out
}

func sortedModuleNames(modules map[string]policy.ModuleDef) []string {
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
