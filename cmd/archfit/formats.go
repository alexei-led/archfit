package main

// Output format name constants shared across commands and tests.
const (
	formatJSON      = "json"
	formatText      = "text"
	formatMarkdown  = "markdown"
	formatMD        = "md" // alias accepted alongside formatMarkdown
	formatSarif     = "sarif"
	formatScorecard = "scorecard"
	// formatLegacyJSON is the pre-cutover JSON envelope, kept for one release
	// and never selected by default: --json is the architecture state.
	formatLegacyJSON = "legacy-json"
)
