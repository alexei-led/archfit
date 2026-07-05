package main

import (
	"sort"
	"strings"
)

func firstEvidenceRefForTest(prompt string) string {
	refs := evidenceRefSet(strings.Split(prompt, "\n"))
	if len(refs) == 0 {
		return "diag:test"
	}
	keys := make([]string, 0, len(refs))
	for ref := range refs {
		keys = append(keys, ref)
	}
	sort.Strings(keys)
	return keys[0]
}
