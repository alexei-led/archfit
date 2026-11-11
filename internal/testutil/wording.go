// Package testutil holds checks that tests of several packages share.
package testutil

import (
	"regexp"
	"strings"
)

// notUsedWords are the words of the "Not used" column of design section 5.
// Archfit output names each thing with one word; these are the other words.
// "record" is not listed: section 5 forbids it only as the verb for an
// owner's answer, and section 8 names the output a record.
var notUsedWords = []string{
	"config", "architecture file", "classification", "component", "surface",
	"approved pair", "edge", "relation", "law", "intent", "decision", "guardrail",
	"waiver", "baseline entry", "debt", "temporary decision", "finding", "violation",
	"blocker", "problem", "task", "gate", "scan", "blocked", "denied",
	"needs attention", "healthy", "decision request", "CR", "accept", "decline",
	"architect", "approver", "rating", "score", "review", "analysis", "insight",
}

// gateNames are the two uses of "gate" that are names, not words: the check
// name and the policy key.
var gateNames = regexp.MustCompile(`(?i)archfit/gate|\bgate:`)

var notUsedPatterns = compileNotUsed(notUsedWords)

func compileNotUsed(words []string) []*regexp.Regexp {
	patterns := make([]*regexp.Regexp, len(words))
	for i, word := range words {
		phrase := strings.Join(strings.Fields(regexp.QuoteMeta(word)), `\s+`)
		patterns[i] = regexp.MustCompile(`(?i)(^|[^\pL\pN_])` + phrase + `($|[^\pL\pN_])`)
	}
	return patterns
}

// NotUsedIn returns the words of the Not used column that text uses, as
// whole words and in any case, in list order. A phrase matches across any
// whitespace.
func NotUsedIn(text string) []string {
	text = gateNames.ReplaceAllString(text, " ")
	var found []string
	for i, pattern := range notUsedPatterns {
		if pattern.MatchString(text) {
			found = append(found, notUsedWords[i])
		}
	}
	return found
}
