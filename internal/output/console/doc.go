// Package console renders the canonical architecture state as terminal-native
// plain text: verdict inputs, dimension evidence, seams, actionable findings,
// task origins, and comparison status. It emits no repository score and uses no
// Markdown, wide tables, or color, so output stays readable on a TTY and safe to
// pipe. Machine output goes through jsonout/sarif and the Markdown artifact
// through markdown; progress and timing go to stderr.
package console
