// Package py implements the Python import extractor using grimp.
// It is an out-of-process adapter that invokes an embedded helper script
// via uv run (preferred) or python3.14+ (fallback), parses the JSON output,
// and normalises the result to graph.Facts.
// All subprocess execution goes through the toolrun.Runner boundary.
package py
