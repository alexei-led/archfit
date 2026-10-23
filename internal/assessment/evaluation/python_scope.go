package evaluation

import (
	"slices"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/policy"
)

func (inv selectorInventory) pythonTargetAddressable(rule policy.RuleDef, topology policy.TopologyView) bool {
	if rule.ToModule != "" {
		target := selectedModuleScope(topology, inv, inv.moduleMap.ModulesSelected(rule.ToModule))
		if target.status != ruleScopeApplicable {
			return true
		}
		_, python := target.languages[languagePython]
		return python
	}
	languages := inv.moduleMap.SelectorLanguages(rule.To)
	_, python := languages[languagePython]
	return python || len(languages) == 0
}

func (inv selectorInventory) pythonFileMatches(rule policy.RuleDef, side, file string) bool {
	pattern, moduleSelector := rule.From, rule.FromModule
	if side == selectorTo {
		pattern, moduleSelector = rule.To, rule.ToModule
	}
	if moduleSelector == "" {
		return pattern == "" || inv.fileMatches(rule.Type, side, pattern, file)
	}
	language, selector, _ := ruleFileSelector(inv.moduleMap, file, inv.selectors)
	owner, owned := fileOwner(inv.moduleMap, file, language, selector)
	return owned && slices.Contains(inv.moduleMap.ModulesSelected(moduleSelector), owner)
}

func unscannedPythonReason(file string) string {
	return "Python source in rule scope was not analysed by the dependency producer: " + file
}

// A live match cannot hide omitted source in a mixed selector. Other languages
// retain their producer applicability rules; Python nodes identify scanned bodies.
func (inv selectorInventory) unscannedPythonSelector(rule policy.RuleDef) string {
	if !inv.pythonComplete {
		return ""
	}
	if !slices.ContainsFunc(inv.files, func(file string) bool {
		return strings.HasSuffix(file, ".py") && inv.pythonFileMatches(rule, selectorFrom, file)
	}) {
		return ""
	}
	for _, file := range inv.files {
		if _, missing := inv.unanalysed[file]; !missing || !strings.HasSuffix(file, ".py") {
			continue
		}
		if inv.pythonFileMatches(rule, selectorFrom, file) || inv.pythonFileMatches(rule, selectorTo, file) {
			return unscannedPythonReason(file)
		}
	}
	return ""
}
