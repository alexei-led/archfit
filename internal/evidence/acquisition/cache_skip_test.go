package acquisition

import (
	"bytes"
	"testing"
)

// TestDiscloseCacheSkips pins the stderr line a run prints when the fact cache
// could not engage: the user cannot see a 2.5-minute re-extraction any other
// way, and a silent skip is what the defect was.
func TestDiscloseCacheSkips(t *testing.T) {
	tests := []struct {
		name  string
		label string
		skips []string
		want  string
	}{
		{
			name: "no skips write nothing",
			want: "",
		},
		{
			name:  "one skip is one warning line",
			skips: []string{"fact cache skipped: input tree has more than 20000 filesystem entries"},
			want:  "warning: fact cache skipped: input tree has more than 20000 filesystem entries\n",
		},
		{
			name:  "the run label prefixes each line",
			label: "config compare: ",
			skips: []string{"fact cache skipped: input tree has more than 20000 filesystem entries", "second"},
			want: "warning: config compare: fact cache skipped: input tree has more than 20000 filesystem entries\n" +
				"warning: config compare: second\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			discloseCacheSkips(&buf, tc.label, tc.skips)
			if got := buf.String(); got != tc.want {
				t.Errorf("stderr = %q, want %q", got, tc.want)
			}
		})
	}
}
