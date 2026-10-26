package scip

import (
	"strings"
	"testing"
)

// Reasons on the semantic-strength rows must tell the reader what to change:
// install the target's dependencies, or turn the pass off deliberately.

func TestTypeScriptReasonNamesDependencyInstallAndOptOut(t *testing.T) {
	t.Parallel()
	for _, want := range []string{"npm ci", "scip-typescript", "analyzers.scip.enabled"} {
		if !strings.Contains(reasonTSNoNodeModules, want) {
			t.Errorf("reason %q does not mention %q", reasonTSNoNodeModules, want)
		}
	}
}

func TestRustAttributionReasonNamesTheKnobs(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"rust_attribution":{"status":"partial","unresolved_symbols":3,"diagnostics":[{"reason":"ambiguous_target","count":3}]}}`)
	cov, err := rustReaderCoverage(raw)
	if err != nil {
		t.Fatalf("rustReaderCoverage: %v", err)
	}
	for _, want := range []string{"3 unresolved symbols", "languages.rust.manifest", "analyzers.scip.enabled"} {
		if !strings.Contains(cov.Reason, want) {
			t.Errorf("reason %q does not mention %q", cov.Reason, want)
		}
	}
}
