// Package py implements the Python import extractor using grimp.
// It is an out-of-process adapter that invokes an embedded PEP 723 helper script
// via uv run (preferred) or python3.12 (fallback), parses the JSON output,
// and normalises the result to graph.Facts.
// All subprocess execution goes through the toolrun.Runner boundary.
package py
