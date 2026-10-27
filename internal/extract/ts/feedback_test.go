package ts

import (
	"strings"
	"testing"
)

// A coverage reason is where a reader learns why evidence is short. It has to
// name the action, not the symptom, and stay on one line.

func TestEnvironmentIssueNamesTheFix(t *testing.T) {
	t.Parallel()
	var dc dcOutput
	dc.Summary.Environment.Issues = []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}{{
		Name:        "missing-typescript-transpiler",
		Description: "dependency-cruiser detected an environment that needs a TypeScript compiler.\nInstall one of the above for better results.",
	}}

	reason := environmentIssue(dc)
	for _, want := range []string{"missing-typescript-transpiler", "npm ci", "typescript", "image"} {
		if !strings.Contains(reason, want) {
			t.Errorf("reason %q does not mention %q", reason, want)
		}
	}
	if strings.Contains(reason, "\n") {
		t.Errorf("reason keeps a newline, which breaks one-line renderers: %q", reason)
	}
	if strings.Contains(reason, "Install one of the above") {
		t.Errorf("reason keeps dependency-cruiser's advice instead of one instruction: %q", reason)
	}
}

func TestNoEnvironmentIssueProducesNoReason(t *testing.T) {
	t.Parallel()
	if reason := environmentIssue(dcOutput{}); reason != "" {
		t.Fatalf("reason = %q, want empty", reason)
	}
}

func TestUnresolvedSpecifierReasonNamesTheCommand(t *testing.T) {
	t.Parallel()
	for _, want := range []string{"npm ci", "tsconfig", "%d of %d"} {
		if !strings.Contains(reasonUnresolvedSpecifiers, want) {
			t.Errorf("reason %q does not mention %q", reasonUnresolvedSpecifiers, want)
		}
	}
}
