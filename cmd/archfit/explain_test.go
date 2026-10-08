package main

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
)

// TestBuildExplainPrompt_IncludesDistanceBasis verifies that buildExplainPrompt
// includes distance_basis in the prompt when the finding carries it in MatchedBy.
func TestBuildExplainPrompt_IncludesDistanceBasis(t *testing.T) {
	t.Parallel()
	f := finding.Finding{
		RuleID:   "bc/imbalanced_coupling",
		Severity: finding.SeverityHigh,
		Status:   finding.StatusNew,
		Edge: finding.EdgeEvidence{
			From: finding.Endpoint{Path: "internal/a", Module: "a"},
			To:   finding.Endpoint{Path: "internal/b", Module: "b"},
			Kind: edgeKindImports,
		},
		Why:        "high strength × high distance",
		Constraint: "lower strength or shorten distance",
		MatchedBy: map[string]string{
			matchedByStrength: enrichIntrusive,
			"distance":        "internal_remote",
			"distance_basis":  "ownership",
		},
	}
	prompt := buildExplainPrompt(f, result.Result{})
	if !strings.Contains(prompt, "distance_basis: ownership") {
		t.Errorf("prompt missing distance_basis:\n%s", prompt)
	}
}
