// Package ts implements the TypeScript import extractor using dependency-cruiser.
// It is an out-of-process adapter that invokes an owned dependency-cruiser executable directly,
// parses its JSON output, and normalises the result to graph.Facts.
// All subprocess execution goes through the toolrun.Runner boundary.
package ts
