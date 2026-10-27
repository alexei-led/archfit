package evaluation

import (
	"sort"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/policy"
)

// ruleTypeNewCrossModule reads module pairs, like the module-wide rules.
const ruleTypeNewCrossModule = "new_cross_module_dependency"

// scopedGapListLimit caps the gaps one reason names; the rest are counted.
const scopedGapListLimit = 5

// scopedEvidenceReason is the unevaluated reason of a dependency rule over a
// language whose producers report completeness per source file: the gaps of
// the incomplete files in the rule's dependency source scope, or "" when none
// lies there.
//
// Only the source side counts. An edge is found while compiling its source
// file, so a missing edge hides in an incomplete source file; a file that
// cannot bind a reference into an incomplete unit is reported incomplete
// itself.
func scopedEvidenceReason(rule policy.RuleDef, p policy.PolicySnapshot, f Observations, ev ScopedDependencyEvidence) string {
	if len(ev.Incomplete) == 0 {
		return ""
	}
	topology := p.Topology
	topology.ModuleMap = topology.ModuleMap.WithCrateOwners(f.CrateOwners)
	inv := newSelectorInventory(topology.ModuleMap, sourceInventoryFiles(f), f)
	reaches := inv.dependencySource(rule, topology)
	gaps := map[string]struct{}{}
	for file, gap := range ev.Incomplete {
		if _, out := f.OutOfScopeFiles[file]; !out && reaches(file) {
			gaps[gap] = struct{}{}
		}
	}
	if len(gaps) == 0 {
		return ""
	}
	return ev.Tool + " evidence is incomplete for the rule scope: " + gapSummary(ev, gaps)
}

// gapSummary names gaps, then what restores them. The gaps come first and
// short, so a report that bounds the text keeps which units are incomplete.
func gapSummary(ev ScopedDependencyEvidence, gaps map[string]struct{}) string {
	fixes := map[string]struct{}{}
	for gap := range gaps {
		if fix := ev.Fixes[gap]; fix != "" {
			fixes[fix] = struct{}{}
		}
	}
	summary := gapList(gaps, ", ")
	if len(fixes) > 0 {
		summary += "; fix: " + gapList(fixes, "; ")
	}
	return summary
}

// dependencySource reports whether a file is one whose outgoing edges a
// dependency rule reads.
func (inv selectorInventory) dependencySource(rule policy.RuleDef, topology policy.TopologyView) func(string) bool {
	ownedBy := func(modules func(string) bool) func(string) bool {
		return func(file string) bool {
			language, selector, _ := ruleFileSelector(topology.ModuleMap, file, inv.selectors)
			module, owned := fileOwner(topology.ModuleMap, file, language, selector)
			return owned && modules(module)
		}
	}
	declared := func(module string) bool { _, ok := topology.Modules[module]; return ok }
	switch rule.Type {
	case ruleTypeForbiddenDependency, ruleTypePublicAPIOnly, ruleTypeInternalAPIAccess:
		return func(file string) bool { return inv.sideFileMatches(rule, selectorFrom, file) }
	case ruleTypeLayerOrder, ruleTypeNewCrossModule, ruleTypeModuleCycle:
		return ownedBy(declared)
	case ruleTypeModuleDependencies:
		// depends_on limits its own module's edges; visible_to limits the edges
		// any declared module sends into its module.
		for _, def := range topology.Modules {
			if def.VisibleTo != nil {
				return ownedBy(declared)
			}
		}
		return ownedBy(func(module string) bool { return topology.Modules[module].DependsOn != nil })
	default:
		// cycle, and any rule type not listed, reads every edge.
		return func(string) bool { return true }
	}
}

// gapList joins items sorted with sep, naming at most scopedGapListLimit.
func gapList(items map[string]struct{}, sep string) string {
	list := make([]string, 0, len(items))
	for item := range items {
		list = append(list, item)
	}
	sort.Strings(list)
	if len(list) > scopedGapListLimit {
		list = append(list[:scopedGapListLimit], "and "+strconv.Itoa(len(list)-scopedGapListLimit)+" more")
	}
	return strings.Join(list, sep)
}

// scopedIncomplete returns the gaps of every in-scope incomplete file of
// language, nil when its evidence is complete or not scoped.
func scopedIncomplete(f Observations, language string) map[string]struct{} {
	ev, ok := f.ScopedDependencyEvidence[language]
	if !ok {
		return nil
	}
	var gaps map[string]struct{}
	for file, gap := range ev.Incomplete {
		if _, out := f.OutOfScopeFiles[file]; out {
			continue
		}
		if gaps == nil {
			gaps = map[string]struct{}{}
		}
		gaps[gap] = struct{}{}
	}
	return gaps
}
