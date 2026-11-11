package evidence

// CheckFacts is what the check reads from one tree: the imports and literals
// of its files, and the items it could not read. The check reads the base and
// the head into one CheckFacts each and counts errors on both; it never scores.
// Paths are repository-relative and slash-separated.
type CheckFacts struct {
	Imports  []ImportFact        `json:"imports,omitempty"`
	Literals []LiteralOccurrence `json:"literals,omitempty"`
	Unknown  []UnknownItem       `json:"unknown,omitempty"`
	// Sources are the source files of the tree the extractors read, test and
	// generated files included: the unowned rule finds production code in no
	// module among them, and an import target that is one of them is a file.
	Sources []string `json:"sources,omitempty"`
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
	// Shape is what the literal looks like, for the copy noise limit. Empty
	// when the extractor did not say.
	Shape LiteralShape `json:"shape,omitempty"`
}

// LiteralShape is what a literal looks like. The duplication rule never
// counts a single word or a struct tag as a copy.
type LiteralShape string

// The shapes of a literal.
const (
	// ShapeOther is any literal that is not a single word or a struct tag.
	ShapeOther LiteralShape = "other"
	// ShapeWord is a single word, such as an identifier or a key.
	ShapeWord LiteralShape = "word"
	// ShapeTag is a Go struct tag, such as `json:"name"`.
	ShapeTag LiteralShape = "tag"
)

// UnknownItem is something the check needs and cannot read: an import it
// cannot resolve, a file it cannot parse, a tool that is not installed, or a
// base it cannot read. Line is zero when the item is the whole file; File is
// empty when the item is about the whole tree, such as a missing tool.
// Missing evidence is not zero: an unknown item can stop a verdict.
type UnknownItem struct {
	File      string `json:"file"`
	Line      int    `json:"line,omitempty"`
	Specifier string `json:"specifier,omitempty"`
	Reason    string `json:"reason"`
	// Cause says what is missing.
	Cause UnknownCause `json:"cause"`
	// Fix is the one step that restores the evidence, as the record prints it
	// after "next:".
	Fix string `json:"fix"`
}

// UnknownCause says what kind of evidence an unknown item misses. The values
// are those of the result's unknown causes.
type UnknownCause string

// The causes of an unknown item.
const (
	// CauseImport: an import cannot be resolved, or a file cannot be parsed.
	CauseImport UnknownCause = "import"
	// CauseTool: a tool the check needs is not installed or fails.
	CauseTool UnknownCause = "tool"
	// CauseBase: the base tree cannot be read.
	CauseBase UnknownCause = "base"
)

// AddedLines holds, for each repository-relative file of the working tree, the
// ascending line numbers the change adds against its base. A file with no
// added line has no entry.
type AddedLines map[string][]int
