package evaluation

import (
	"sort"
	"strconv"
	"strings"
)

// scopedGapListLimit caps the gaps one reason names; the rest are counted.
const scopedGapListLimit = 5

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
