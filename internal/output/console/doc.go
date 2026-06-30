// Package console renders an archfit decision.Report as terminal-native plain
// text: a decision-led summary (decision band, gate/advisory counts, score),
// categorized recommendations, per-dimension "why low / what moves it", an
// optional delta, and targets. No Markdown, no wide tables, no color — readable
// on a TTY and safe to pipe. Machine output goes through jsonout/sarif and the
// Markdown artifact through markdown; progress and timing go to stderr.
package console
