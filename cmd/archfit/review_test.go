package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/llm"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	reviewProviderName  = "test/fixed"
	reviewDimBoundary   = "boundary_integrity"
	reviewModReal       = "real_module"
	reviewNarrativeKeep = "keep"
)

// validReviewJSON is a well-formed LLM response that passes post-verification
// when the evidence contains modules "a" and "b" (from writeViolatingRepo).
const validReviewJSON = `{
  "overall_band": "mixed",
  "dimensions": [
    {
      "name": "boundary_integrity",
      "band": "poor",
      "narrative": "The boundary between a and b is breached by an intrusive dependency, raising maintenance effort and cascading change risk."
    }
  ],
  "top_risks": [
    {
      "title": "Intrusive cross-module access",
      "modules": ["a", "b"],
      "narrative": "Module a reaches into b internal packages directly. This high-strength cross-module coupling increases co-evolution pressure across knowledge boundaries.",
      "balancing_move": "Expose a stable contract interface from b and have a depend only on that."
    }
  ],
  "subdomain_suggestions": [
    {
      "module": "a",
      "suggested_subdomain": "supporting",
      "rationale": "Module a orchestrates other modules and maps to a supporting subdomain."
    }
  ]
}`

// fixedProvider returns one canned response regardless of request content.
type fixedProvider struct {
	text string
	name string
}

func (p *fixedProvider) Name() string { return p.name }
func (p *fixedProvider) Complete(_ context.Context, _ llm.Request) (llm.Response, error) {
	return llm.Response{Text: p.text}, nil
}

// appendLLMConfig appends a minimal tools.llm block to an existing config file.
func appendLLMConfig(t *testing.T, cfgPath string) {
	t.Helper()
	raw, err := os.ReadFile(cfgPath) //nolint:gosec // test fixture
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, []byte("tools:\n  llm:\n    provider: ollama\n    model: test-model\n    base_url: http://127.0.0.1:0\n")...)
	if err := os.WriteFile(cfgPath, raw, 0o600); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
}

// runReviewCmd runs ReviewCmd directly with a real runner (matching the
// appDeps that the top-level Run() function provides) and the given provider override.
func runReviewCmd(t *testing.T, cfgPath string, provider llm.Provider) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd := ReviewCmd{
		Config:           cfgPath,
		NoCache:          true,
		providerOverride: provider,
	}
	deps := &appDeps{Runner: toolrun.New(), Stdout: &buf}
	err := cmd.Run(deps)
	return buf.String(), err
}

// TestReviewCmd_Run_SchemaValidation drives ReviewCmd end-to-end with a fake
// provider returning valid JSON and asserts all required output sections appear.
func TestReviewCmd_Run_SchemaValidation(t *testing.T) {
	cfgPath := writeViolatingRepo(t)
	appendLLMConfig(t, cfgPath)

	out, err := runReviewCmd(t, cfgPath, &fixedProvider{text: validReviewJSON, name: reviewProviderName})
	if err != nil {
		t.Fatalf("Run returned error: %v\noutput:\n%s", err, out)
	}

	checks := []struct {
		label string
		want  string
	}{
		{"header", "## Architecture Review"},
		{"overall", "Overall:"},
		{"dimension name", reviewDimBoundary},
		{"top_risks title", "Intrusive cross-module access"},
		{"balancing_move", "Balancing move:"},
	}
	for _, tc := range checks {
		if !strings.Contains(out, tc.want) {
			t.Errorf("missing %s (%q)\nfull output:\n%s", tc.label, tc.want, out)
		}
	}
}

// TestReviewCmd_Run_EntityPostCheck verifies that invalid module names are
// dropped and valid ones are preserved in the output.
func TestReviewCmd_Run_EntityPostCheck(t *testing.T) {
	cfgPath := writeViolatingRepo(t)
	appendLLMConfig(t, cfgPath)

	// "a" is a valid module from writeViolatingRepo; "nonexistent_module" must be dropped.
	jsonWithBadModule := `{
  "overall_band": "poor",
  "dimensions": [{"name": "boundary_integrity", "band": "poor", "narrative": "Boundary breached."}],
  "top_risks": [
    {
      "title": "Mixed modules risk",
      "modules": ["a", "nonexistent_module"],
      "narrative": "Risk narrative.",
      "balancing_move": "Remove the bad reference."
    }
  ],
  "subdomain_suggestions": [
    {"module": "nonexistent_module", "suggested_subdomain": "core", "rationale": "Should be dropped."},
    {"module": "a", "suggested_subdomain": "supporting", "rationale": "Valid module."}
  ]
}`

	out, err := runReviewCmd(t, cfgPath, &fixedProvider{text: jsonWithBadModule, name: reviewProviderName})
	if err != nil {
		t.Fatalf("Run returned error: %v\noutput:\n%s", err, out)
	}

	if strings.Contains(out, "nonexistent_module") {
		t.Errorf("invalid module should have been dropped from output\n%s", out)
	}
	if !strings.Contains(out, "modules: a") {
		t.Errorf("valid module 'a' should remain in output\n%s", out)
	}
}

// TestReviewCmd_Run_InvalidJSON asserts exit code 3 with a descriptive message
// when the LLM returns non-JSON.
func TestReviewCmd_Run_InvalidJSON(t *testing.T) {
	cfgPath := writeViolatingRepo(t)
	appendLLMConfig(t, cfgPath)

	_, err := runReviewCmd(t, cfgPath, &fixedProvider{text: "not json", name: reviewProviderName})

	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 3 {
		t.Fatalf("want exitError{code:3}, got %v", err)
	}
	if !strings.Contains(ee.msg, "model response is not the required JSON") {
		t.Errorf("want JSON error message, got: %s", ee.msg)
	}
}

// TestReviewCmd_Run_NoLLMConfig asserts exit code 3 with a tools.llm hint
// when the config has no LLM provider configured.
func TestReviewCmd_Run_NoLLMConfig(t *testing.T) {
	// writeViolatingRepo produces a config without tools.llm.
	cfgPath := writeViolatingRepo(t)

	var buf bytes.Buffer
	cmd := ReviewCmd{
		Config:  cfgPath,
		NoCache: true,
		// providerOverride absent — config check fires first.
	}
	deps := &appDeps{Runner: toolrun.New(), Stdout: &buf}
	err := cmd.Run(deps)

	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 3 {
		t.Fatalf("want exitError{code:3}, got %v", err)
	}
	if !strings.Contains(ee.msg, "tools.llm") {
		t.Errorf("want tools.llm hint in message, got: %s", ee.msg)
	}
}

// TestPostVerify_DropsUnknownEntities unit-tests postVerify in isolation,
// covering every drop path without a full pipeline run.
func TestPostVerify_DropsUnknownEntities(t *testing.T) {
	diag := diagnostic.Diagnostic{
		FileFacts: []diagnostic.FileFact{
			{Module: reviewModReal},
		},
		Findings: []finding.Finding{
			{Edge: finding.EdgeEvidence{
				From: finding.Endpoint{Module: "a"},
				To:   finding.Endpoint{Module: "b"},
			}},
		},
	}

	rev := reviewResponse{
		OverallBand: "mixed",
		Dimensions: []reviewDimension{
			{Name: reviewDimBoundary, Band: "poor", Narrative: "ok"},
			{Name: "fake_dimension", Band: "mixed", Narrative: "should be dropped"},
		},
		TopRisks: []reviewRisk{
			// All modules invalid → whole entry dropped.
			{Title: "All invalid", Modules: []string{"ghost_module"}, Narrative: "drop me", BalancingMove: "n/a"},
			// One valid, one invalid → invalid module stripped, entry kept.
			{Title: "Valid risk", Modules: []string{"a", "ghost_module"}, Narrative: reviewNarrativeKeep, BalancingMove: "fix"},
			// No modules listed → kept as-is.
			{Title: "No modules", Modules: nil, Narrative: reviewNarrativeKeep, BalancingMove: "fix"},
		},
		SubdomainSuggestions: []reviewSubdomainSuggest{
			{Module: reviewModReal, SuggestedSubdomain: subdomainCore, Rationale: reviewNarrativeKeep},
			{Module: "fake_module", SuggestedSubdomain: subdomainGeneric, Rationale: "drop"},
		},
	}

	result := postVerify(rev, diag)

	// Only known dimension kept.
	if len(result.Dimensions) != 1 || result.Dimensions[0].Name != reviewDimBoundary {
		t.Errorf("dimensions = %+v, want [%s]", result.Dimensions, reviewDimBoundary)
	}

	// "All invalid" dropped; "Valid risk" kept with only "a"; "No modules" kept.
	if len(result.TopRisks) != 2 {
		t.Fatalf("top_risks count = %d, want 2: %+v", len(result.TopRisks), result.TopRisks)
	}
	if result.TopRisks[0].Title != "Valid risk" {
		t.Errorf("expected Valid risk first, got %q", result.TopRisks[0].Title)
	}
	if len(result.TopRisks[0].Modules) != 1 || result.TopRisks[0].Modules[0] != "a" {
		t.Errorf("modules = %v, want [a]", result.TopRisks[0].Modules)
	}
	if result.TopRisks[1].Title != "No modules" {
		t.Errorf("expected No modules second, got %q", result.TopRisks[1].Title)
	}

	// Only real_module suggestion kept.
	if len(result.SubdomainSuggestions) != 1 || result.SubdomainSuggestions[0].Module != reviewModReal {
		t.Errorf("subdomain suggestions = %+v, want [%s]", result.SubdomainSuggestions, reviewModReal)
	}
}
