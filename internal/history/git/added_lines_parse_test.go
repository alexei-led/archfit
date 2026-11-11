package git

import "testing"

// TestParseAddedLinesRefusesWhatItCannotRead: a diff the parser does not
// understand is an error, never a shorter list of added lines.
func TestParseAddedLinesRefusesWhatItCannotRead(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, diff string }{
		{"context line inside a hunk", "+++ b/a.go\n@@ -1,2 +1,2 @@\n a\n-b\n+c\n"},
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
