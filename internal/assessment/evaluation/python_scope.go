package evaluation

import (
	"strings"

	"github.com/alexei-led/archfit/v3/internal/policy"
)

func unscannedPythonReason(file string) string {
	return "Python source in rule scope was not analysed by the dependency producer: " + file
}

// A live match cannot hide omitted source in a mixed selector. Other languages
// retain their producer applicability rules; Python nodes identify scanned bodies.
func (inv selectorInventory) unscannedPythonSelector(rule policy.RuleDef) string {
	if !inv.pythonComplete {
		return ""
	}
	for _, file := range inv.files {
		if _, missing := inv.unanalysed[file]; !missing || !strings.HasSuffix(file, ".py") {
			continue
		}
		if rule.From == "" || inv.fileMatches(rule.Type, selectorFrom, rule.From, file) ||
			rule.To == "" || inv.fileMatches(rule.Type, selectorTo, rule.To, file) {
			return unscannedPythonReason(file)
		}
	}
	return ""
}
