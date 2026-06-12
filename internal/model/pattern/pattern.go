// Package pattern defines the shared structural match result type produced by
// pattern-search ports (PatternProvider) and consumed by rules as evidence.
package pattern

// Match is a single ast-grep structural match result, shared by ports
// (producer) and rules (consumer evidence).
type Match struct {
	File    string // repo-relative file path
	Pattern string // pattern ID from PatternDef.ID
	Text    string // matched source text
	Node    string // syntax node kind (e.g. "call_expression")
	Line    int    // 1-based line number
	Column  int    // 0-based column offset
}
