package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/score"
	"github.com/alexei-led/archfit/internal/llm"
	"github.com/alexei-led/archfit/internal/model/report"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	reviewProviderName  = "test/fixed"
	reviewDimBoundary   = "coupling_balance"
	reviewBandMixed     = "mixed"
	reviewModReal       = "real_module"
	reviewModLazy       = "lazy_mod"
	reviewNarrativeKeep = "keep"
	reviewNarrativeDrop = "drop"
	reviewBalancingMove = "fix"

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
      "claim_type": "semantic_interpretation",
      "metric_ids": ["coupling_balance"],
      "narrative": "The coupling between a and b is intrusive, raising maintenance effort and cascading change risk."
    }
  ],
  "top_risks": [
    {
      "title": "Intrusive cross-module access",
      "modules": ["a", "b"],
      "claim_type": "recommendation",
      "metric_ids": ["coupling_balance"],
      "narrative": "Module a reaches into b internal packages directly. This high-strength cross-module coupling increases co-evolution pressure across knowledge boundaries.",
      "balancing_move": "Expose a stable contract interface from b and have a depend only on that."
    }
  ],
  "subdomain_suggestions": [
    {
      "module": "a",
      "suggested_subdomain": "supporting",
      "claim_type": "recommendation",
      "metric_ids": ["coupling_balance"],
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

// runLLMReviewForTest exercises runLLMReview end-to-end using a real runner (matching
// the appDeps that the top-level Run() function provides) and the given provider
// override. It loads config + runs the pipeline, then delegates to runLLMReview —
// mirrors AnalyzeCmd with --ai-summary without going through the CLI parser.
func runLLMReviewForTest(t *testing.T, cfgPath string, provider llm.Provider) (string, error) {
	t.Helper()
	ctx := context.Background()
	var buf bytes.Buffer
	deps := &appDeps{Runner: toolrun.New(), Stdout: &buf}

	cfg, err := loadConfig(ctx, cfgPath)
	if err != nil {
		return "", &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	diag, sc, err := runPipeline(ctx, deps, cfg, cfgPath, "")
	if err != nil {
		return "", &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	doc := application.ProjectReport(diag, sc, nil, false)
	err = runLLMReview(ctx, deps, cfg, cfgPath, "", true, provider, doc)
	return buf.String(), err
}

// TestRun_Analyze_LLM_DeterministicFirst verifies the analyze --ai-summary integration
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
		AISummary:        true,
		Quiet:            true,
		Format:           []string{formatText},
		providerOverride: &fixedProvider{text: validReviewJSON, name: reviewProviderName},
	}
	_ = cmd.Run(deps)
	out := buf.String()

	det := strings.Index(out, "ARCHITECTURE STATE")
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
// the off-gate LLM contract: when a gate run with an AI summary hits a violation
// AND the LLM narration fails, the exit code must reflect the gate verdict (1),
// never the LLM error (3). The LLM is advisory — its failure must not mask or
// change the gate result.
func TestRun_Analyze_GateLLM_FailureDoesNotMaskVerdict(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)
	appendLLMConfig(t, cfgPath)

	var buf bytes.Buffer
	deps := &appDeps{Runner: toolrun.New(), Stdout: &buf}
	err := runScan(context.Background(), deps, scanRequest{
		configPath:       cfgPath,
		formats:          []string{formatText},
		quiet:            true,
		aiSummary:        true,
		providerOverride: &failingProvider{name: reviewProviderName},
		reportOnly:       false,
	})

	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 1 {
		t.Fatalf("want exitError{code:1} (gate verdict survives LLM failure), got %v\noutput:\n%s", err, buf.String())
	}
}

type analyzeJSONStableSections struct {
	Verdict  result.Verdict    `json:"verdict"`
	Findings []finding.Finding `json:"findings"`
	Score    score.Scorecard   `json:"score"`
}

func runAnalyzeJSON(t *testing.T, cfgPath string, llmEnabled bool, provider llm.Provider) (string, error) {
	t.Helper()
	out, _, err := runAnalyzeJSONWithStderr(t, cfgPath, llmEnabled, provider)
	return out, err
}

func runAnalyzeJSONWithStderr(t *testing.T, cfgPath string, llmEnabled bool, provider llm.Provider) (string, string, error) {
	t.Helper()
	var buf, stderr bytes.Buffer
	deps := &appDeps{Runner: toolrun.New(), Stdout: &buf, Stderr: &stderr}
	cmd := AnalyzeCmd{
		Config:           cfgPath,
		AISummary:        llmEnabled,
		Quiet:            true,
		Format:           []string{formatJSON},
		providerOverride: provider,
	}
	err := cmd.Run(deps)
	return buf.String(), stderr.String(), err
}

func decodeAnalyzeJSONStableSections(t *testing.T, out string) analyzeJSONStableSections {
	t.Helper()
	var sections analyzeJSONStableSections
	dec := json.NewDecoder(strings.NewReader(out))
	if err := dec.Decode(&sections); err != nil {
		t.Fatalf("decode analyze JSON: %v\noutput:\n%s", err, out)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("decode analyze JSON: trailing non-JSON content: %v\noutput:\n%s", err, out)
	}
	return sections
}

func TestRun_Analyze_JSONStableSectionsIgnoreAIConfigWithoutLLM(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)

	before, err := runAnalyzeJSON(t, cfgPath, false, nil)
	if err != nil {
		t.Fatalf("analyze before ai config: %v\n%s", err, before)
	}
	appendLLMConfig(t, cfgPath)
	after, err := runAnalyzeJSON(t, cfgPath, false, nil)
	if err != nil {
		t.Fatalf("analyze after ai config: %v\n%s", err, after)
	}
	if !reflect.DeepEqual(decodeAnalyzeJSONStableSections(t, before), decodeAnalyzeJSONStableSections(t, after)) {
		t.Fatalf("ai config changed verdict/findings/score without --ai-summary\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestRun_Analyze_LLMJSONDeterministicSectionsUnchanged(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)
	appendLLMConfig(t, cfgPath)

	deterministicOut, err := runAnalyzeJSON(t, cfgPath, false, nil)
	if err != nil {
		t.Fatalf("deterministic analyze: %v\n%s", err, deterministicOut)
	}
	llmOut, llmErr, err := runAnalyzeJSONWithStderr(t, cfgPath, true, &fixedProvider{text: validReviewJSON, name: reviewProviderName})
	if err != nil {
		t.Fatalf("analyze --ai-summary: %v\n%s", err, llmOut)
	}

	deterministic := decodeAnalyzeJSONStableSections(t, deterministicOut)
	withLLM := decodeAnalyzeJSONStableSections(t, llmOut)
	if !reflect.DeepEqual(deterministic, withLLM) {
		t.Fatalf("LLM review changed deterministic JSON sections\ndeterministic: %+v\nwith LLM: %+v", deterministic, withLLM)
	}
	if strings.Contains(llmOut, "Architecture Review") {
		t.Fatalf("analyze --ai-summary JSON stdout must stay valid JSON without appended review markdown:\n%s", llmOut)
	}
	if !strings.Contains(llmErr, "Architecture Review") {
		t.Fatalf("analyze --ai-summary JSON should emit the review section on stderr to keep stdout parseable:\n%s", llmErr)
	}
}

// TestLLMReview_Run_SchemaValidation drives analyze --ai-summary end-to-end with a fake
// provider returning valid JSON and asserts all required output sections appear.
func TestLLMReview_Run_SchemaValidation(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)
	appendLLMConfig(t, cfgPath)

	out, err := runLLMReviewForTest(t, cfgPath, &fixedProvider{text: validReviewJSON, name: reviewProviderName})
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

// TestLLMReview_Run_EntityPostCheck verifies that invalid module names are
// dropped and valid ones are preserved in the output.
func TestLLMReview_Run_EntityPostCheck(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)
	appendLLMConfig(t, cfgPath)

	// "a" is a valid module from writeViolatingRepo; "nonexistent_module" must be dropped.
	jsonWithBadModule := `{
  "overall_band": "poor",
  "dimensions": [{"name": "coupling_balance", "band": "poor", "claim_type": "semantic_interpretation", "metric_ids": ["coupling_balance"], "narrative": "Coupling is elevated."}],
  "top_risks": [
    {
      "title": "Mixed modules risk",
      "modules": ["a", "nonexistent_module"],
      "claim_type": "recommendation",
      "metric_ids": ["coupling_balance"],
      "narrative": "Risk narrative.",
      "balancing_move": "Remove the bad reference."
    }
  ],
  "subdomain_suggestions": [
    {"module": "nonexistent_module", "suggested_subdomain": "core", "claim_type": "recommendation", "metric_ids": ["coupling_balance"], "rationale": "Should be dropped."},
    {"module": "a", "suggested_subdomain": "supporting", "claim_type": "recommendation", "metric_ids": ["coupling_balance"], "rationale": "Valid module."}
  ]
}`

	out, err := runLLMReviewForTest(t, cfgPath, &fixedProvider{text: jsonWithBadModule, name: reviewProviderName})
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

// TestLLMReview_Run_InvalidJSON asserts exit code 3 with a descriptive message
// when the LLM returns non-JSON.
func TestLLMReview_Run_InvalidJSON(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)
	appendLLMConfig(t, cfgPath)

	_, err := runLLMReviewForTest(t, cfgPath, &fixedProvider{text: "not json", name: reviewProviderName})

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
	diag := report.Document{}
	for i := 0; i < findingsN; i++ {
		diag.Findings = append(diag.Findings, report.Finding{
			ID:       fmt.Sprintf("id%d", i),
			Kind:     report.FindingKindGate,
			RuleID:   fmt.Sprintf("rule%d", i),
			Severity: report.FindingSeverityHigh,
			Status:   report.FindingStatusNew,
			Edge: report.FindingEdge{
				From: report.FindingEndpoint{Module: fmt.Sprintf("from%d", i)},
				To:   report.FindingEndpoint{Module: fmt.Sprintf("to%d", i)},
			},
		})
	}
	for i := 0; i < factsN; i++ {
		diag.FileFacts = append(diag.FileFacts, report.FileFact{
			Module:               fmt.Sprintf("mod%d", i),
			InboundModuleFanIn:   i,
			OutboundDestinations: i,
			LOC:                  i,
		})
	}
	for i := 0; i < metricsN; i++ {
		diag.Metrics = append(diag.Metrics, report.MetricResult{
			Name:    fmt.Sprintf("metric%d", i),
			Value:   float64(i),
			Band:    string(report.ScoreBandPoor),
			Display: fmt.Sprintf("%d/10", i),
		})
	}
	for i := 0; i < dynamicN; i++ {
		diag.DynamicImports = append(diag.DynamicImports, report.DynamicImport{
			Module: fmt.Sprintf("lazy%d", i),
			Count:  i + 1,
		})
	}

	prompt := buildReviewPrompt(diag)

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

// TestLLMReview_PersistsRawResponse verifies the raw LLM response is dumped to
// the cache dir before parsing, so truncation/parse failures stay diagnosable.
func TestLLMReview_PersistsRawResponse(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)
	appendLLMConfig(t, cfgPath)

	_, err := runLLMReviewForTest(t, cfgPath, &fixedProvider{text: validReviewJSON, name: reviewProviderName})
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

func TestLLMReview_Run_UsesReviewTokenBudget(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)
	appendLLMConfig(t, cfgPath)

	provider := &fixedProvider{text: validReviewJSON, name: reviewProviderName}
	_, err := runLLMReviewForTest(t, cfgPath, provider)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if provider.lastRequest.MaxTokens != reviewMaxTokens {
		t.Fatalf("MaxTokens = %d, want %d", provider.lastRequest.MaxTokens, reviewMaxTokens)
	}
}

// TestLLMReview_Run_NoLLMConfig asserts exit code 3 with a ai hint
// when the config has no LLM provider configured.
func TestLLMReview_Run_NoLLMConfig(t *testing.T) {
	t.Parallel()
	// writeViolatingRepo produces a config without ai.
	cfgPath := writeViolatingRepo(t)

	ctx := context.Background()
	var buf bytes.Buffer
	deps := &appDeps{Runner: toolrun.New(), Stdout: &buf}

	cfg, loadErr := loadConfig(ctx, cfgPath)
	if loadErr != nil {
		t.Fatalf("loadConfig: %v", loadErr)
	}
	// runLLMReview fires the "ai not configured" check before touching
	// the provider, so we can pass a nil diag+scorecard — they are never reached.
	err := runLLMReview(ctx, deps, cfg, cfgPath, "", true, nil,
		report.Document{})

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
	diag := report.Document{
		FileFacts:      []report.FileFact{{Module: reviewModReal}},
		DynamicImports: []report.DynamicImport{{Module: reviewModLazy, Count: 3}},
	}
	rev := reviewResponse{
		OverallBand: "excellent", // outside rubric vocabulary → blanked
		Dimensions: []reviewDimension{
			{Name: reviewDimBoundary, Band: string(report.ScoreBandMixed), ClaimType: claimTypeSemanticInterpretation, MetricIDs: []string{reviewDimBoundary}, Narrative: "ok"},
			{Name: reviewDimBoundary, Band: "excellent", ClaimType: claimTypeSemanticInterpretation, MetricIDs: []string{reviewDimBoundary}, Narrative: "bad band → dropped"},
		},
		TopRisks: []reviewRisk{
			{Title: "lazy", Modules: []string{reviewModLazy}, ClaimType: claimTypeRecommendation, MetricIDs: []string{reviewDimBoundary}, Narrative: "lazy-import risk", BalancingMove: "x"},
		},
		SubdomainSuggestions: []reviewSubdomainSuggest{
			{Module: reviewModReal, SuggestedSubdomain: subdomainCore, ClaimType: claimTypeRecommendation, MetricIDs: []string{reviewDimBoundary}, Rationale: "ok"},
			{Module: reviewModReal, SuggestedSubdomain: "platform", ClaimType: claimTypeRecommendation, MetricIDs: []string{reviewDimBoundary}, Rationale: "bad subdomain → dropped"},
		},
	}

	result, _ := postVerify(rev, diag, nil)

	if result.OverallBand != "" {
		t.Errorf("overall_band = %q, want blanked", result.OverallBand)
	}
	if len(result.Dimensions) != 1 || result.Dimensions[0].Band != string(report.ScoreBandMixed) {
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
	diag := report.Document{
		DynamicImports: []report.DynamicImport{{Module: reviewModLazy, Count: 7}},
	}
	prompt := buildReviewPrompt(diag)
	if !strings.Contains(prompt, "Dynamic / lazy imports") || !strings.Contains(prompt, reviewModLazy) {
		t.Errorf("prompt missing dynamic-import section:\n%s", prompt)
	}
}

func TestBuildReviewPrompt_IncludesCitationInputs(t *testing.T) {
	t.Parallel()
	diag := report.Document{
		Findings: []report.Finding{{ID: "finding-1", Kind: report.FindingKindGate, RuleID: "r", Severity: report.FindingSeverityHigh}},
		Metrics:  []report.MetricResult{{Name: "module_fanout", Value: 1, Band: string(report.ScoreBandPoor), Display: "1"}},
		Score:    report.Scorecard{Dimensions: []report.Dimension{{Name: reviewDimBoundary}}},
	}
	prompt := buildReviewPrompt(diag, []string{"doc:architecture.md (doc) docs/architecture.md: Architecture intent"})
	for _, want := range []string{repositoryEvidenceHeader, "doc:architecture.md", "finding_id=finding-1", "metric_id=module_fanout", "metric_id=coupling_balance"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestPostVerify_DropsUncitedRecommendations(t *testing.T) {
	t.Parallel()
	diag := report.Document{FileFacts: []report.FileFact{{Module: reviewModReal}}}
	rev := reviewResponse{
		OverallBand: reviewBandMixed,
		TopRisks: []reviewRisk{
			{Title: "uncited", Modules: []string{reviewModReal}, ClaimType: claimTypeRecommendation, Narrative: reviewNarrativeDrop, BalancingMove: reviewBalancingMove},
			{Title: "misclassified", Modules: []string{reviewModReal}, ClaimType: claimTypeDeterministicFact, Narrative: reviewNarrativeDrop, BalancingMove: reviewBalancingMove},
			{Title: "cited", Modules: []string{reviewModReal}, ClaimType: claimTypeRecommendation, MetricIDs: []string{reviewDimBoundary}, Narrative: reviewNarrativeKeep, BalancingMove: reviewBalancingMove},
		},
		SubdomainSuggestions: []reviewSubdomainSuggest{
			{Module: reviewModReal, SuggestedSubdomain: subdomainCore, ClaimType: claimTypeRecommendation, Rationale: reviewNarrativeDrop},
			{Module: reviewModReal, SuggestedSubdomain: subdomainGeneric, ClaimType: claimTypeSemanticInterpretation, Rationale: reviewNarrativeDrop},
			{Module: reviewModReal, SuggestedSubdomain: subdomainSupporting, ClaimType: claimTypeRecommendation, MetricIDs: []string{reviewDimBoundary}, Rationale: reviewNarrativeKeep},
		},
	}

	result, dropped := postVerify(rev, diag, nil)
	if dropped < 4 {
		t.Fatalf("dropped = %d, want uncited and non-recommendation suggestion drops", dropped)
	}
	if len(result.TopRisks) != 1 || result.TopRisks[0].Title != "cited" {
		t.Fatalf("top risks = %+v, want only cited recommendation", result.TopRisks)
	}
	if len(result.SubdomainSuggestions) != 1 || result.SubdomainSuggestions[0].SuggestedSubdomain != subdomainSupporting {
		t.Fatalf("subdomain suggestions = %+v, want only cited recommendation", result.SubdomainSuggestions)
	}
}

func TestPostVerify_DropsUnknownCitationRefs(t *testing.T) {
	t.Parallel()
	diag := report.Document{FileFacts: []report.FileFact{{Module: reviewModReal}}}
	rev := reviewResponse{
		OverallBand: reviewBandMixed,
		TopRisks: []reviewRisk{
			{Title: "bad ref", Modules: []string{reviewModReal}, ClaimType: claimTypeRecommendation, EvidenceRefs: []string{"doc:missing.md"}, Narrative: reviewNarrativeDrop, BalancingMove: reviewBalancingMove},
			{Title: "good ref", Modules: []string{reviewModReal}, ClaimType: claimTypeRecommendation, EvidenceRefs: []string{"doc:architecture.md"}, Narrative: reviewNarrativeKeep, BalancingMove: reviewBalancingMove},
		},
	}
	citations := buildReviewCitationSet(diag, []string{"doc:architecture.md (doc) docs/architecture.md: Architecture intent"})

	result, _ := postVerify(rev, diag, nil, citations)
	if len(result.TopRisks) != 1 || result.TopRisks[0].Title != "good ref" {
		t.Fatalf("top risks = %+v, want only known evidence ref", result.TopRisks)
	}
}

func TestPostVerify_AcceptsConfiguredModulesWithoutRuntimeEvidence(t *testing.T) {
	t.Parallel()
	diag := report.Document{}
	citations := buildReviewCitationSet(diag, nil)
	citations.Modules[reviewModReal] = struct{}{}
	rev := reviewResponse{
		OverallBand: reviewBandMixed,
		SubdomainSuggestions: []reviewSubdomainSuggest{
			{Module: reviewModReal, SuggestedSubdomain: subdomainCore, ClaimType: claimTypeRecommendation, MetricIDs: []string{reviewDimBoundary}, Rationale: reviewNarrativeKeep},
		},
	}

	result, dropped := postVerify(rev, diag, nil, citations)
	if dropped != 0 {
		t.Fatalf("dropped = %d, want no configured-module suggestion drops", dropped)
	}
	if len(result.SubdomainSuggestions) != 1 || result.SubdomainSuggestions[0].Module != reviewModReal {
		t.Fatalf("subdomain suggestions = %+v, want configured module kept", result.SubdomainSuggestions)
	}
}

// TestPostVerify_DropsUnknownEntities unit-tests postVerify in isolation,
// covering every drop path without a full pipeline run.
func TestPostVerify_DropsUnknownEntities(t *testing.T) {
	t.Parallel()
	diag := report.Document{
		FileFacts: []report.FileFact{
			{Module: reviewModReal},
		},
		Findings: []report.Finding{
			{Edge: report.FindingEdge{
				From: report.FindingEndpoint{Module: "a"},
				To:   report.FindingEndpoint{Module: "b"},
			}},
		},
	}

	rev := reviewResponse{
		OverallBand: reviewBandMixed,
		Dimensions: []reviewDimension{
			{Name: reviewDimBoundary, Band: string(report.ScoreBandPoor), ClaimType: claimTypeSemanticInterpretation, MetricIDs: []string{reviewDimBoundary}, Narrative: "ok"},
			{Name: "fake_dimension", Band: reviewBandMixed, ClaimType: claimTypeSemanticInterpretation, MetricIDs: []string{reviewDimBoundary}, Narrative: "should be dropped"},
		},
		TopRisks: []reviewRisk{
			// All modules invalid → whole entry dropped.
			{Title: "All invalid", Modules: []string{"ghost_module"}, ClaimType: claimTypeRecommendation, MetricIDs: []string{reviewDimBoundary}, Narrative: "drop me", BalancingMove: "n/a"},
			// One valid, one invalid → invalid module stripped, entry kept.
			{Title: "Valid risk", Modules: []string{"a", "ghost_module"}, ClaimType: claimTypeRecommendation, MetricIDs: []string{reviewDimBoundary}, Narrative: reviewNarrativeKeep, BalancingMove: reviewBalancingMove},
			// No modules listed → kept as-is.
			{Title: "No modules", Modules: nil, ClaimType: claimTypeRecommendation, MetricIDs: []string{reviewDimBoundary}, Narrative: reviewNarrativeKeep, BalancingMove: reviewBalancingMove},
		},
		SubdomainSuggestions: []reviewSubdomainSuggest{
			{Module: reviewModReal, SuggestedSubdomain: subdomainCore, ClaimType: claimTypeRecommendation, MetricIDs: []string{reviewDimBoundary}, Rationale: reviewNarrativeKeep},
			{Module: "fake_module", SuggestedSubdomain: subdomainGeneric, ClaimType: claimTypeRecommendation, MetricIDs: []string{reviewDimBoundary}, Rationale: reviewNarrativeDrop},
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
	diag := report.Document{
		FileFacts: []report.FileFact{
			{Module: "a"},
			{Module: "b"},
		},
		Findings: []report.Finding{
			{
				Edge: report.FindingEdge{
					From: report.FindingEndpoint{Module: "a"},
					To:   report.FindingEndpoint{Module: "b"},
					Kind: edgeKindImports, // NOT uses_internal
				},
				MatchedBy: map[string]string{
					matchedByStrength: llmStrengthFunctional, // functional evidence only
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
				ClaimType:     claimTypeRecommendation,
				MetricIDs:     []string{reviewDimBoundary},
				Narrative:     "Module a uses intrusive access into b internals.",
				BalancingMove: "Expose a contract.",
			},
			{
				Title:         "Functional coupling",
				Modules:       []string{"a"},
				ClaimType:     claimTypeRecommendation,
				MetricIDs:     []string{reviewDimBoundary},
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
	diag := report.Document{
		FileFacts: []report.FileFact{
			{Module: modPayments},
		},
	}
	rev := reviewResponse{
		OverallBand: reviewBandMixed,
		SubdomainSuggestions: []reviewSubdomainSuggest{
			// LLM says "core"; config says "supporting" → conflict must be flagged.
			{Module: modPayments, SuggestedSubdomain: subdomainCore, ClaimType: claimTypeRecommendation, MetricIDs: []string{reviewDimBoundary}, Rationale: "central business logic"},
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
