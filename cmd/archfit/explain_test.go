package main

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/model/finding"
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
	prompt := buildExplainPrompt(f, result.Diagnostic{})
	if !strings.Contains(prompt, "distance_basis: ownership") {
		t.Errorf("prompt missing distance_basis:\n%s", prompt)
	}
	// ownership basis must not add the degenerate qualifier
	if strings.Contains(prompt, "degenerate_owner_map") {
		t.Errorf("ownership basis should not add degenerate_owner_map qualifier:\n%s", prompt)
	}
}

// TestBuildExplainPrompt_DegenerateOwnerQualifier verifies that a finding whose
// distance_basis is "code_structure" (single-owner fallback) gets the
// "(degenerate_owner_map)" qualifier appended to the distance label so the LLM
// does not frame it as a cross-team boundary.
func TestBuildExplainPrompt_DegenerateOwnerQualifier(t *testing.T) {
	t.Parallel()
	f := finding.Finding{
		RuleID:   "bc/imbalanced_coupling",
		Severity: finding.SeverityMedium,
		Status:   finding.StatusNew,
		Edge: finding.EdgeEvidence{
			From: finding.Endpoint{Path: "pkg/foo", Module: "foo"},
			To:   finding.Endpoint{Path: "pkg/bar", Module: "bar"},
			Kind: edgeKindImports,
		},
		Why:        "structural distance mismatch",
		Constraint: "lower strength",
		MatchedBy: map[string]string{
			matchedByStrength: "functional",
			"distance":        "internal_remote",
			"distance_basis":  "code_structure",
		},
	}
	prompt := buildExplainPrompt(f, result.Diagnostic{})
	if !strings.Contains(prompt, "distance_basis: code_structure") {
		t.Errorf("prompt missing distance_basis:\n%s", prompt)
	}
	if !strings.Contains(prompt, "degenerate_owner_map") {
		t.Errorf("code_structure basis must add (degenerate_owner_map) qualifier:\n%s", prompt)
	}
}
