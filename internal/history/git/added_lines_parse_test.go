package git

import (
	"maps"
	"slices"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/evidence"
)

// TestParseAddedLinesNumbersAddedLinesOnly: a context line moves the line
// number on but is not added.
func TestParseAddedLinesNumbersAddedLinesOnly(t *testing.T) {
	t.Parallel()
	diff := "+++ b/a.go\n@@ -1,4 +1,4 @@\n-a\n+A\n b\n c\n-d\n+D\n"
	got, err := parseAddedLines([]byte(diff))
	if want := (evidence.AddedLines{"a.go": {1, 4}}); err != nil || !maps.EqualFunc(got, want, slices.Equal) {
		t.Errorf("parseAddedLines = %v, %v; want %v", got, err, want)
	}
}

// TestParseAddedLinesRefusesWhatItCannotRead: a diff the parser does not
// understand is an error, never a shorter list of added lines.
func TestParseAddedLinesRefusesWhatItCannotRead(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, diff string }{
		{"blank line inside a hunk", "+++ b/a.go\n@@ -1,2 +1,2 @@\n\n-b\n+c\n"},
		{"hunk cut short", "+++ b/a.go\n@@ -0,0 +1,2 @@\n+a\n"},
		{"hunk header without a new range", "+++ b/a.go\n@@ -1 @@\n"},
		{"hunk header with a bad number", "+++ b/a.go\n@@ -1 +x,2 @@\n"},
		{"badly quoted name", "+++ \"b/a\\q.go\"\n"},
		{"name without the b/ prefix", "+++ a.go\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := parseAddedLines([]byte(tc.diff)); err == nil {
				t.Errorf("parseAddedLines = %v, want an error", got)
			}
		})
	}
}
