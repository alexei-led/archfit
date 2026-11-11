package evidence

// CheckFacts is what the check reads from one tree: the imports and literals
// of its files, and the items it could not read. The check reads the base and
// the head into one CheckFacts each and counts errors on both; it never scores.
// Paths are repository-relative and slash-separated.
type CheckFacts struct {
	Imports  []ImportFact        `json:"imports,omitempty"`
	Literals []LiteralOccurrence `json:"literals,omitempty"`
	Unknown  []UnknownItem       `json:"unknown,omitempty"`
}

// ImportFact is one import of a file. Target is the repository-relative path a
// first-party import resolves to ("." for the root package), or the library as
// imported when External is set.
type ImportFact struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Target   string `json:"target"`
	External bool   `json:"external,omitempty"`
}

// LiteralOccurrence is one string literal of a file, with its normalised value.
type LiteralOccurrence struct {
	File  string `json:"file"`
	Line  int    `json:"line"`
	Value string `json:"value"`
}

// UnknownItem is something in a file the check needs and cannot read: an
// import it cannot resolve, or a file it cannot parse. Line is zero when the
// item is the whole file. Missing evidence is not zero: an unknown item can
// stop a verdict.
type UnknownItem struct {
	File      string `json:"file"`
	Line      int    `json:"line,omitempty"`
	Specifier string `json:"specifier,omitempty"`
	Reason    string `json:"reason"`
}

// AddedLines holds, for each repository-relative file of the working tree, the
// ascending line numbers the change adds against its base. A file with no
// added line has no entry.
type AddedLines map[string][]int
