package a

import "strings"

// Repeat calls a stdlib function — an EXTERNAL (non-first-party) target. The
// extractor must emit a real type-info strength hint for it: a declared
// `external_systems:` seam scores at DistanceExternal only when strength is
// known (abstain-not-fake). Expected strength hint: functional.
func Repeat(s string) string { return strings.Repeat(s, 2) }
