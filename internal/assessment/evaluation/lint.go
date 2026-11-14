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

// PolicyWarnings discloses module values the legacy decoder accepts but the
// report cannot interpret. It never evaluates a rule.
func PolicyWarnings(p policy.PolicySnapshot, _ Observations) []string {
	out := make([]string, 0, len(p.Topology.Modules))
	for _, d := range moduleValueDiagnostics(p.Topology) {
		out = append(out, d.Path+": "+d.Message)
	}
	return out
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
