// Package pattern defines the shared structural match result type produced by
// pattern-search ports (PatternProvider) and consumed by rules as evidence.
package pattern

// Match is a single ast-grep structural match result, shared by ports
// (producer) and rules (consumer evidence).
type Match struct {
	File    string // repo-relative file path
	Pattern string // pattern ID from Def.ID
	Text    string // matched source text
	Node    string // syntax node kind (e.g. "call_expression")
	Line    int    // 1-based line number
	Column  int    // 0-based column offset
}

// Def declares a single structural pattern rule for the pattern-search port.
// It is the neutral request vocabulary shared by the policy that authors the
// pattern and the adapter that executes it, so neither has to import the other.
type Def struct {
	ID   string `yaml:"id"`
	Lang string `yaml:"lang"`
	Rule string `yaml:"rule"`
}

// Config is the list of pattern definitions passed to a PatternProvider.
type Config []Def
