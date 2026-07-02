package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/engine"
	"github.com/alexei-led/archfit/internal/llm"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/score"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	reviewProviderName  = "test/fixed"
	reviewDimBoundary   = "coupling_balance"
	reviewBandMixed     = "mixed"
	reviewModReal       = "real_module"
	reviewModLazy       = "lazy_mod"
	reviewNarrativeKeep = "keep"

	// matchedByStrength is the MatchedBy key for coupling strength.
	matchedByStrength = "strength"
	// edgeKindImports is the edge kind string for plain import edges.
	edgeKindImports = "imports"
)

// validReviewJSON is a well-formed LLM response that passes post-verification
// when the evidence contains modules "a" and "b" (from writeViolatingRepo).
const validReviewJSON = `{
  "overall_band": "mixed",
  "dimensions": [
    {
      "name": "coupling_balance",
      "band": "poor",
      "narrative": "The coupling between a and b is intrusive, raising maintenance effort and cascading change risk."
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
	text        string
	name        string
	lastRequest llm.Request
}

func (p *fixedProvider) Name() string { return p.name }
func (p *fixedProvider) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	p.lastRequest = req
	return llm.Response{Text: p.text}, nil
}

// appendLLMConfig appends a minimal ai block to an existing config file.
func appendLLMConfig(t *testing.T, cfgPath string) {
	t.Helper()
	raw, err := os.ReadFile(cfgPath) //nolint:gosec // test fixture
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, []byte("ai:\n  provider: ollama\n  model: test-model\n  base_url: http://127.0.0.1:0\n")...)
	if err := os.WriteFile(cfgPath, raw, 0o600); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
}

// runReviewCmd exercises runLLMReview end-to-end using a real runner (matching
// the appDeps that the top-level Run() function provides) and the given provider
// override. It loads config + runs the pipeline, then delegates to runLLMReview —
// mirrors the old ReviewCmd.Run flow without the now-deleted ReviewCmd struct.
func runReviewCmd(t *testing.T, cfgPath string, provider llm.Provider) (string, error) {
	t.Helper()
	ctx := context.Background()
	var buf bytes.Buffer
	deps := &appDeps{Runner: toolrun.New(), Stdout: &buf}

	cfg, err := loadConfig(ctx, cfgPath, false)
	if err != nil {
		return "", &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	configDir := filepath.Dir(cfgPath)
	base, _ := baseline.Load(ctx, filepath.Join(configDir, defaultBaselinePath))
	diag, sc, err := runPipeline(ctx, deps, cfg, cfgPath, "", false,
		engine.Mode{Full: true, Advisory: true, ReportOnly: true}, base)
	if err != nil {
		return "", &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	err = runLLMReview(ctx, deps, cfg, configDir, true, provider, diag, sc)
	return buf.String(), err
}

// TestRun_Analyze_LLM_DeterministicFirst verifies the analyze --llm integration
// through the provider seam: the deterministic decision report (ARCHFIT RESULT)
// renders BEFORE the off-gate LLM narrative section.
func TestRun_Analyze_LLM_DeterministicFirst(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)
	appendLLMConfig(t, cfgPath)

	var buf bytes.Buffer
	deps := &appDeps{Runner: toolrun.New(), Stdout: &buf}
	cmd := AnalyzeCmd{
		Config:           cfgPath,
		Full:             true,
		LLM:              true,
		Quiet:            true,
		Format:           []string{formatText},
		providerOverride: &fixedProvider{text: validReviewJSON, name: reviewProviderName},
	}
	_ = cmd.Run(deps)
	out := buf.String()

	det := strings.Index(out, "ARCHFIT RESULT")
	llmIdx := strings.Index(out, "Architecture Review")
	switch {
	case det < 0:
		t.Fatalf("deterministic decision report missing:\n%s", out)
	case llmIdx < 0:
		t.Fatalf("LLM narrative section missing:\n%s", out)
	case det > llmIdx:
		t.Errorf("deterministic report must precede the LLM section:\n%s", out)
	}
}

// failingProvider always errors at call time — simulates an LLM outage or
// backend reached but unavailable.
type failingProvider struct{ name string }

func (p *failingProvider) Name() string { return p.name }
func (p *failingProvider) Complete(context.Context, llm.Request) (llm.Response, error) {
	return llm.Response{}, errors.New("simulated LLM backend failure")
}

// TestRun_Analyze_GateLLM_FailureDoesNotMaskVerdict is the regression guard for
// the off-gate LLM contract: when `analyze --gate --llm` hits a gate violation
// AND the LLM narration fails, the exit code must reflect the gate verdict (1),
// never the LLM error (3). The LLM is advisory — its failure must not mask or
// change the gate result.
func TestRun_Analyze_GateLLM_FailureDoesNotMaskVerdict(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)
	appendLLMConfig(t, cfgPath)

	var buf bytes.Buffer
	deps := &appDeps{Runner: toolrun.New(), Stdout: &buf}
	cmd := AnalyzeCmd{
		Config:           cfgPath,
		Full:             true,
		Gate:             true,
		LLM:              true,
		Quiet:            true,
		Format:           []string{formatText},
		providerOverride: &failingProvider{name: reviewProviderName},
	}
	err := cmd.Run(deps)

	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 1 {
		t.Fatalf("want exitError{code:1} (gate verdict survives LLM failure), got %v\noutput:\n%s", err, buf.String())
	}
}

// TestReviewCmd_Run_SchemaValidation drives ReviewCmd end-to-end with a fake
// provider returning valid JSON and asserts all required output sections appear.
func TestReviewCmd_Run_SchemaValidation(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	cfgPath := writeViolatingRepo(t)
	appendLLMConfig(t, cfgPath)

	// "a" is a valid module from writeViolatingRepo; "nonexistent_module" must be dropped.
	jsonWithBadModule := `{
  "overall_band": "poor",
  "dimensions": [{"name": "coupling_balance", "band": "poor", "narrative": "Coupling is elevated."}],
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
	t.Parallel()
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

func TestParseReviewResponse_ToleratesFencesAndProse(t *testing.T) {
	t.Parallel()
	wrapped := "model preface\n```json\n" + validReviewJSON + "\n```\n"
	rev, err := parseReviewResponse(wrapped)
	if err != nil {
		t.Fatalf("parseReviewResponse returned error: %v", err)
	}
	if rev.OverallBand != reviewBandMixed || len(rev.Dimensions) != 1 {
		t.Fatalf("unexpected review response: %+v", rev)
	}
}

func TestParseReviewResponse_TruncatedJSONHint(t *testing.T) {
	t.Parallel()
	_, err := parseReviewResponse(`{"overall_band":"` + reviewBandMixed + `","dimensions":[`)
	if err == nil {
		t.Fatal("want error for truncated JSON")
	}
	if !strings.Contains(err.Error(), "appears truncated") {
		t.Fatalf("want truncation hint, got: %v", err)
	}
}

// TestParseReviewResponse_RecoversFromTrailingProse asserts the first-{ to
// last-} extraction recovers a valid payload when the model wraps a complete
// JSON object in prose and then gets cut off mid-sentence after the close brace.
func TestParseReviewResponse_RecoversFromTrailingProse(t *testing.T) {
	t.Parallel()
	wrapped := "Here is the architecture review you requested:\n\n" +
		validReviewJSON +
		"\n\nNote: this analysis was based on the supplied evidence and may be inc"
	rev, err := parseReviewResponse(wrapped)
	if err != nil {
		t.Fatalf("parseReviewResponse returned error: %v", err)
	}
	if rev.OverallBand != reviewBandMixed || len(rev.Dimensions) != 1 {
		t.Fatalf("unexpected recovered review response: %+v", rev)
	}
}

// TestBuildReviewPrompt_RespectsCaps feeds a deliberately oversized diagnostic
// and asserts every capped section stays within its budget — the regression
// guard for the ccgram token-overflow the review caps were added to prevent.
func TestBuildReviewPrompt_RespectsCaps(t *testing.T) {
	t.Parallel()
	const (
		findingsN = 200
		factsN    = 200
		metricsN  = 100
		dynamicN  = 100
	)
	diag := diagnostic.Diagnostic{}
	for i := 0; i < findingsN; i++ {
		diag.Findings = append(diag.Findings, finding.Finding{
			ID:       fmt.Sprintf("id%d", i),
			Kind:     finding.KindGate,
			RuleID:   fmt.Sprintf("rule%d", i),
			Severity: finding.SeverityHigh,
			Status:   finding.StatusNew,
			Edge: finding.EdgeEvidence{
				From: finding.Endpoint{Module: fmt.Sprintf("from%d", i)},
				To:   finding.Endpoint{Module: fmt.Sprintf("to%d", i)},
			},
		})
	}
	for i := 0; i < factsN; i++ {
		diag.FileFacts = append(diag.FileFacts, diagnostic.FileFact{
			Module:               fmt.Sprintf("mod%d", i),
			InboundModuleFanIn:   i,
			OutboundDestinations: i,
			LOC:                  i,
		})
	}
	for i := 0; i < metricsN; i++ {
		diag.Metrics = append(diag.Metrics, diagnostic.MetricResult{
			Name:    fmt.Sprintf("metric%d", i),
			Value:   float64(i),
			Band:    "poor",
			Display: fmt.Sprintf("%d/10", i),
		})
	}
	for i := 0; i < dynamicN; i++ {
		diag.DynamicImports = append(diag.DynamicImports, diagnostic.DynamicImport{
			Module: fmt.Sprintf("lazy%d", i),
			Count:  i + 1,
		})
	}

	prompt := buildReviewPrompt(diag, score.Scorecard{})

	// Each section uses a unique line marker, so a strings.Count is an exact
	// per-section line tally that must not exceed its cap.
	caps := []struct {
		label  string
		marker string
		limit  int
	}{
		{"finding examples", "- [gate] rule=", reviewMaxFindings},
		{"module facts", "inbound_fanin=", reviewMaxModuleFacts},
		{"metrics", " display=", reviewMaxMetrics},
		{"dynamic imports", "site(s)", reviewMaxDynamicFacts},
	}
	for _, c := range caps {
		if got := strings.Count(prompt, c.marker); got > c.limit {
			t.Errorf("%s rendered %d lines, exceeds cap %d", c.label, got, c.limit)
		}
	}

	// Truncation must actually have engaged for findings and dynamic imports
	// (both render an explicit "N more ... omitted" line when over the cap).
	if !strings.Contains(prompt, "more finding(s) omitted") {
		t.Errorf("oversized findings did not emit an omission marker:\n%s", prompt)
	}
	if !strings.Contains(prompt, "more module(s) omitted") {
		t.Errorf("oversized dynamic imports did not emit an omission marker:\n%s", prompt)
	}
}

// TestReviewCmd_PersistsRawResponse verifies the raw LLM response is dumped to
// the cache dir before parsing, so truncation/parse failures stay diagnosable.
func TestReviewCmd_PersistsRawResponse(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)
	appendLLMConfig(t, cfgPath)

	_, err := runReviewCmd(t, cfgPath, &fixedProvider{text: validReviewJSON, name: reviewProviderName})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	rawPath := filepath.Join(filepath.Dir(cfgPath), ".archfit-cache", "llm", rawReviewFile)
	got, err := os.ReadFile(rawPath) //nolint:gosec // test reads a known temp path
	if err != nil {
		t.Fatalf("raw review not persisted: %v", err)
	}
	if string(got) != validReviewJSON {
		t.Errorf("persisted raw response mismatch:\n%s", got)
	}
}

func TestReviewCmd_Run_UsesReviewTokenBudget(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)
	appendLLMConfig(t, cfgPath)

	provider := &fixedProvider{text: validReviewJSON, name: reviewProviderName}
	_, err := runReviewCmd(t, cfgPath, provider)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if provider.lastRequest.MaxTokens != reviewMaxTokens {
		t.Fatalf("MaxTokens = %d, want %d", provider.lastRequest.MaxTokens, reviewMaxTokens)
	}
}

// TestReviewCmd_Run_NoLLMConfig asserts exit code 3 with a ai hint
// when the config has no LLM provider configured.
func TestReviewCmd_Run_NoLLMConfig(t *testing.T) {
	t.Parallel()
	// writeViolatingRepo produces a config without ai.
	cfgPath := writeViolatingRepo(t)

	ctx := context.Background()
	var buf bytes.Buffer
	deps := &appDeps{Runner: toolrun.New(), Stdout: &buf}

	cfg, loadErr := loadConfig(ctx, cfgPath, false)
	if loadErr != nil {
		t.Fatalf("loadConfig: %v", loadErr)
	}
	configDir := filepath.Dir(cfgPath)
	// runLLMReview fires the "ai not configured" check before touching
	// the provider, so we can pass a nil diag+scorecard — they are never reached.
	err := runLLMReview(ctx, deps, cfg, configDir, true, nil,
		diagnostic.Diagnostic{}, score.Scorecard{})

	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 3 {
		t.Fatalf("want exitError{code:3}, got %v", err)
	}
	if !strings.Contains(ee.msg, "ai") {
		t.Errorf("want ai hint in message, got: %s", ee.msg)
	}
}

// TestPostVerify_RejectsInvalidEnums asserts band/subdomain values outside the
// rubric vocabulary are dropped (overall blanked), while a dynamic-import module
// is accepted as valid evidence the review may cite.
func TestPostVerify_RejectsInvalidEnums(t *testing.T) {
	t.Parallel()
	diag := diagnostic.Diagnostic{
		FileFacts:      []diagnostic.FileFact{{Module: reviewModReal}},
		DynamicImports: []diagnostic.DynamicImport{{Module: reviewModLazy, Count: 3}},
	}
	rev := reviewResponse{
		OverallBand: "excellent", // outside rubric vocabulary → blanked
		Dimensions: []reviewDimension{
			{Name: reviewDimBoundary, Band: string(score.BandMixed), Narrative: "ok"},
			{Name: reviewDimBoundary, Band: "excellent", Narrative: "bad band → dropped"},
		},
		TopRisks: []reviewRisk{
			{Title: "lazy", Modules: []string{reviewModLazy}, Narrative: "lazy-import risk", BalancingMove: "x"},
		},
		SubdomainSuggestions: []reviewSubdomainSuggest{
			{Module: reviewModReal, SuggestedSubdomain: subdomainCore, Rationale: "ok"},
			{Module: reviewModReal, SuggestedSubdomain: "platform", Rationale: "bad subdomain → dropped"},
		},
	}

	result, _ := postVerify(rev, diag, nil)

	if result.OverallBand != "" {
		t.Errorf("overall_band = %q, want blanked", result.OverallBand)
	}
	if len(result.Dimensions) != 1 || result.Dimensions[0].Band != string(score.BandMixed) {
		t.Errorf("dimensions = %+v, want only the valid-band entry", result.Dimensions)
	}
	if len(result.TopRisks) != 1 || len(result.TopRisks[0].Modules) != 1 {
		t.Errorf("dynamic-import module should be accepted: %+v", result.TopRisks)
	}
	if len(result.SubdomainSuggestions) != 1 || result.SubdomainSuggestions[0].SuggestedSubdomain != subdomainCore {
		t.Errorf("subdomain suggestions = %+v, want only valid-subdomain entry", result.SubdomainSuggestions)
	}
}

// TestBuildReviewPrompt_IncludesDynamicImports asserts the report-only
// dynamic/lazy-import block is fed to the LLM so it can narrate the hidden
// coupling the static metrics miss.
func TestBuildReviewPrompt_IncludesDynamicImports(t *testing.T) {
	t.Parallel()
	diag := diagnostic.Diagnostic{
		DynamicImports: []diagnostic.DynamicImport{{Module: reviewModLazy, Count: 7}},
	}
	prompt := buildReviewPrompt(diag, score.Scorecard{})
	if !strings.Contains(prompt, "Dynamic / lazy imports") || !strings.Contains(prompt, reviewModLazy) {
		t.Errorf("prompt missing dynamic-import section:\n%s", prompt)
	}
}

// TestPostVerify_DropsUnknownEntities unit-tests postVerify in isolation,
// covering every drop path without a full pipeline run.
func TestPostVerify_DropsUnknownEntities(t *testing.T) {
	t.Parallel()
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
		OverallBand: reviewBandMixed,
		Dimensions: []reviewDimension{
			{Name: reviewDimBoundary, Band: "poor", Narrative: "ok"},
			{Name: "fake_dimension", Band: reviewBandMixed, Narrative: "should be dropped"},
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

	result, _ := postVerify(rev, diag, nil)

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

// TestPostVerify_DropsUnsupportedStrengthClaim verifies that a top_risk
// asserting "intrusive" when no uses_internal edge or MatchedBy["strength"]
// evidence is present is dropped and counted (herdr hallucination regression).
func TestPostVerify_DropsUnsupportedStrengthClaim(t *testing.T) {
	t.Parallel()
	// Diagnostic has one finding with no strength evidence and no uses_internal edge.
	diag := diagnostic.Diagnostic{
		FileFacts: []diagnostic.FileFact{
			{Module: "a"},
			{Module: "b"},
		},
		Findings: []finding.Finding{
			{
				Edge: finding.EdgeEvidence{
					From: finding.Endpoint{Module: "a"},
					To:   finding.Endpoint{Module: "b"},
					Kind: edgeKindImports, // NOT uses_internal
				},
				MatchedBy: map[string]string{
					matchedByStrength: "functional", // functional evidence only
				},
			},
		},
	}

	// LLM hallucinated an "intrusive" risk with no supporting evidence.
	rev := reviewResponse{
		OverallBand: reviewBandMixed,
		TopRisks: []reviewRisk{
			{
				Title:         "Intrusive cross-module access",
				Modules:       []string{"a", "b"},
				Narrative:     "Module a uses intrusive access into b internals.",
				BalancingMove: "Expose a contract.",
			},
			{
				Title:         "Functional coupling",
				Modules:       []string{"a"},
				Narrative:     "High functional coupling between a and b.",
				BalancingMove: "Reduce coupling.",
			},
		},
	}

	result, dropped := postVerify(rev, diag, nil)

	// The "intrusive" risk must be dropped; the "functional" risk must be kept.
	if dropped < 1 {
		t.Errorf("dropped = %d, want >= 1 (intrusive claim must be counted)", dropped)
	}
	if len(result.TopRisks) != 1 {
		t.Fatalf("top_risks count = %d, want 1: %+v", len(result.TopRisks), result.TopRisks)
	}
	if result.TopRisks[0].Title != "Functional coupling" {
		t.Errorf("expected Functional coupling kept, got %q", result.TopRisks[0].Title)
	}
}

// TestPostVerify_FlagsConfigSubdomainConflict verifies that a subdomain
// suggestion conflicting with the config value is kept but annotated.
func TestPostVerify_FlagsConfigSubdomainConflict(t *testing.T) {
	t.Parallel()
	const modPayments = "payments"
	diag := diagnostic.Diagnostic{
		FileFacts: []diagnostic.FileFact{
			{Module: modPayments},
		},
	}
	rev := reviewResponse{
		OverallBand: reviewBandMixed,
		SubdomainSuggestions: []reviewSubdomainSuggest{
			// LLM says "core"; config says "supporting" → conflict must be flagged.
			{Module: modPayments, SuggestedSubdomain: subdomainCore, Rationale: "central business logic"},
		},
	}

	configSubdomains := map[string]string{modPayments: subdomainSupporting}
	result, _ := postVerify(rev, diag, configSubdomains)

	if len(result.SubdomainSuggestions) != 1 {
		t.Fatalf("suggestion must be kept (not dropped): %+v", result.SubdomainSuggestions)
	}
	rationale := result.SubdomainSuggestions[0].Rationale
	if !strings.Contains(rationale, "conflicts with config") {
		t.Errorf("rationale missing conflict annotation: %q", rationale)
	}
	if !strings.Contains(rationale, subdomainSupporting) {
		t.Errorf("rationale missing configured subdomain %q: %q", subdomainSupporting, rationale)
	}
}
