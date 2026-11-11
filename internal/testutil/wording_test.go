package testutil_test

import (
	"slices"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/testutil"
)

func TestNotUsedIn(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want []string
	}{
		{name: "clean record", text: "pkg/tc/netem.go:12: graph: pkg_tc -> pkg_util is not an allowed dependency\narchfit: fail, 1 error\n"},
		{name: "a whole word", text: "1 finding", want: []string{"finding"}},
		{name: "any case", text: "Waiver added", want: []string{"waiver"}},
		{name: "a short upper-case word", text: "open a cr for it", want: []string{"CR"}},
		{name: "a phrase across whitespace", text: "needs\n  attention", want: []string{"needs attention"}},
		{name: "a phrase and its word", text: "a decision request", want: []string{"decision", "decision request"}},
		{name: "words at the edges", text: "edge", want: []string{"edge"}},
		{name: "words next to punctuation", text: "(score: 9)", want: []string{"score"}},
		{name: "a longer word is not the word", text: "tasks accepted relationship scores"},
		{name: "a word inside an identifier is not the word", text: "config_file edge_count"},
		{name: "the check name", text: "the check archfit/gate failed"},
		{name: "the policy key", text: "gate: required"},
		{name: "gate as a word", text: "the gate is closed", want: []string{"gate"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := testutil.NotUsedIn(tc.text); !slices.Equal(got, tc.want) {
				t.Errorf("NotUsedIn(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}
