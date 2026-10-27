package acquire

import (
	"strings"
	"testing"
)

// A disabled analyzer row explains itself: why it is off, and the exact key
// that turns it on. "opt-in: <key>" alone made readers look the answer up.
func TestDisabledReasonsExplainWhyAndHow(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"scip":   ReasonSCIPDisabled,
		"syntax": ReasonSyntaxDisabled,
	}
	for name, reason := range cases {
		for _, want := range []string{"slow", "analyzers." + name + ".enabled"} {
			if !strings.Contains(reason, want) {
				t.Errorf("%s reason %q does not mention %q", name, reason, want)
			}
		}
	}
}
