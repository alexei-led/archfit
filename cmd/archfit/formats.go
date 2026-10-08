package main

import "github.com/alexei-led/archfit/v3/internal/output/agentout"

// Output format name constants shared across commands and tests.
const (
	formatJSON      = "json"
	formatText      = "text"
	formatMarkdown  = "markdown"
	formatMD        = "md" // short alias for formatMarkdown
	formatSarif     = "sarif"
	formatScorecard = "scorecard"
	formatAgent     = agentout.FormatName
)
