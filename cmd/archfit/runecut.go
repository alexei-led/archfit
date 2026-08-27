package main

import "unicode/utf8"

// cutAtRuneBoundary returns s truncated to at most n bytes, backing the cut off
// to the nearest rune start. Slicing a UTF-8 string at an arbitrary byte index
// splits a multi-byte rune and emits invalid UTF-8. Both callers feed llm
// request fields, where encoding/json silently substitutes U+FFFD — so the model
// reads a corrupted identifier instead of the one the file contains.
func cutAtRuneBoundary(s string, n int) string {
	if n >= len(s) {
		return s
	}
	if n < 0 {
		return ""
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
