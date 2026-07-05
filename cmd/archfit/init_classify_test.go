package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/initcfg"
	"github.com/alexei-led/archfit/internal/llm"
)

const (
	testLayerDomain       = "domain"
	testMod0              = "mod0"
	testMod0Path          = "internal/mod0/**"
	testEvidenceRef       = "doc:README.md"
	testEvidenceRationale = "cite " + testEvidenceRef
	testRepoEvidence      = testEvidenceRef + " (doc) README.md: Architecture"
)

// fakeClassifyProvider returns canned responses per call, tracking call count.
type fakeClassifyProvider struct {
	responses []string
	calls     int
}

func (p *fakeClassifyProvider) Name() string { return "test/classify" }
func (p *fakeClassifyProvider) Complete(_ context.Context, _ llm.Request) (llm.Response, error) {
	if p.calls >= len(p.responses) {
		return llm.Response{}, fmt.Errorf("unexpected call %d", p.calls)
	}
	r := p.responses[p.calls]
	p.calls++
	return llm.Response{Text: r}, nil
}

// makeTargets builds n ClassifyTargets named "mod0".."modN-1".
func makeTargets(n int) []initcfg.ClassifyTarget {
	targets := make([]initcfg.ClassifyTarget, n)
	for i := range targets {
		targets[i] = initcfg.ClassifyTarget{
			Name:  fmt.Sprintf("mod%d", i),
			Paths: []string{fmt.Sprintf("internal/mod%d/**", i)},
		}
	}
	return targets
}

// validResponse builds a valid JSON array response for the given targets,
// using testLayerDomain as the layer value.
func validResponse(targets []initcfg.ClassifyTarget) string {
	parts := make([]string, len(targets))
	for i, t := range targets {
		parts[i] = fmt.Sprintf(
			`{"module":%q,"subdomain":"core","volatility":"low","layer":%q,"name":"","rationale":"test"}`,
			t.Name, testLayerDomain,
		)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func TestClassifyModules_ValidResponse(t *testing.T) {
	t.Parallel()
	targets := makeTargets(2)
	layers := []string{testLayerDomain, "application", "infrastructure"}
	p := &fakeClassifyProvider{responses: []string{validResponse(targets)}}

	got, err := classifyModules(context.Background(), p, targets, layers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d annotations, want 2", len(got))
	}
	for _, t2 := range targets {
		ann, ok := got[t2.Name]
		if !ok {
			t.Errorf("missing annotation for %q", t2.Name)
			continue
		}
		if ann.Subdomain != subdomainCore {
			t.Errorf("%s: subdomain = %q, want core", t2.Name, ann.Subdomain)
		}
		if ann.Volatility != volatilityLow {
			t.Errorf("%s: volatility = %q, want low", t2.Name, ann.Volatility)
		}
		if ann.Layer != testLayerDomain {
			t.Errorf("%s: layer = %q, want %s", t2.Name, ann.Layer, testLayerDomain)
		}
		if ann.Rationale == "" {
			t.Errorf("%s: rationale is empty", t2.Name)
		}
	}
	if p.calls != 1 {
		t.Errorf("provider calls = %d, want 1", p.calls)
	}
}

func TestClassifyModules_UnknownModuleSkipped(t *testing.T) {
	t.Parallel()
	targets := []initcfg.ClassifyTarget{{Name: "mymod", Paths: []string{"internal/mymod/**"}}}
	layers := []string{testLayerDomain}
	// Response includes "mymod" (valid) and "ghost" (unknown — should be skipped).
	resp := fmt.Sprintf(`[
		{"module":"ghost","subdomain":"core","volatility":"low","layer":%q,"name":"","rationale":"hallucinated"},
		{"module":"mymod","subdomain":"supporting","volatility":"medium","layer":%q,"name":"","rationale":"ok"}
	]`, testLayerDomain, testLayerDomain)
	p := &fakeClassifyProvider{responses: []string{resp}}

	got, err := classifyModules(context.Background(), p, targets, layers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, exists := got["ghost"]; exists {
		t.Error("ghost module should have been skipped")
	}
	ann, ok := got["mymod"]
	if !ok {
		t.Fatal("mymod annotation missing")
	}
	if ann.Subdomain != "supporting" || ann.Volatility != "medium" {
		t.Errorf("mymod annotation = %+v", ann)
	}
}

func TestClassifyModules_InvalidEnumSkipped(t *testing.T) {
	t.Parallel()
	targets := []initcfg.ClassifyTarget{
		{Name: "a", Paths: []string{"internal/a/**"}},
		{Name: "b", Paths: []string{"internal/b/**"}},
		{Name: "c", Paths: []string{"internal/c/**"}},
	}
	layers := []string{testLayerDomain}
	// "a" has invalid subdomain, "b" has invalid volatility, "c" is valid.
	resp := fmt.Sprintf(`[
		{"module":"a","subdomain":"INVALID","volatility":"low","layer":%q,"name":"","rationale":"bad subdomain"},
		{"module":"b","subdomain":"core","volatility":"INVALID","layer":%q,"name":"","rationale":"bad volatility"},
		{"module":"c","subdomain":"generic","volatility":"high","layer":%q,"name":"","rationale":"valid"}
	]`, testLayerDomain, testLayerDomain, testLayerDomain)
	p := &fakeClassifyProvider{responses: []string{resp}}

	got, err := classifyModules(context.Background(), p, targets, layers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, exists := got["a"]; exists {
		t.Error("a with invalid subdomain should have been skipped")
	}
	if _, exists := got["b"]; exists {
		t.Error("b with invalid volatility should have been skipped")
	}
	ann, ok := got["c"]
	if !ok {
		t.Fatal("c annotation missing")
	}
	if ann.Subdomain != "generic" || ann.Volatility != "high" {
		t.Errorf("c annotation = %+v", ann)
	}
}

func TestClassifyModules_OutOfSetLayerCarried(t *testing.T) {
	t.Parallel()
	targets := []initcfg.ClassifyTarget{{Name: "svc", Paths: []string{"internal/svc/**"}}}
	layers := []string{testLayerDomain, "application"} // "infrastructure" is NOT in allowed set
	resp := `[{"module":"svc","subdomain":"core","volatility":"low","layer":"infrastructure","name":"","rationale":"out-of-set layer"}]`
	p := &fakeClassifyProvider{responses: []string{resp}}

	got, err := classifyModules(context.Background(), p, targets, layers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ann, ok := got["svc"]
	if !ok {
		t.Fatal("svc annotation missing")
	}
	// Raw layer value must be carried even though it is not in layers.
	if ann.Layer != "infrastructure" {
		t.Errorf("layer = %q, want infrastructure (raw value carried)", ann.Layer)
	}
}

func TestClassifyModules_MalformedBodyError(t *testing.T) {
	t.Parallel()
	targets := []initcfg.ClassifyTarget{{Name: testMod0, Paths: []string{testMod0Path}}}
	layers := []string{testLayerDomain}
	p := &fakeClassifyProvider{responses: []string{"not a JSON array at all"}}

	_, err := classifyModules(context.Background(), p, targets, layers)
	if err == nil {
		t.Error("malformed body must return an error")
	}
}

func TestClassifyModules_MissingRationaleError(t *testing.T) {
	t.Parallel()
	targets := []initcfg.ClassifyTarget{{Name: testMod0, Paths: []string{testMod0Path}}}
	layers := []string{testLayerDomain}
	resp := `[{"module":"mod0","subdomain":"core","volatility":"low","layer":"domain","name":"","rationale":" "}]`
	p := &fakeClassifyProvider{responses: []string{resp}}

	_, err := classifyModules(context.Background(), p, targets, layers)
	if err == nil {
		t.Fatal("missing rationale must return an error")
	}
	if !strings.Contains(err.Error(), "missing rationale") {
		t.Fatalf("error = %v, want missing rationale", err)
	}
}

func TestClassifyModulesWithEvidence_MissingEvidenceRefsError(t *testing.T) {
	t.Parallel()
	targets := []initcfg.ClassifyTarget{{Name: testMod0, Paths: []string{testMod0Path}}}
	layers := []string{testLayerDomain}
	resp := `[{"module":"mod0","subdomain":"core","volatility":"low","layer":"domain","name":"","rationale":"test","basis":"semantic_judgment"}]`
	p := &fakeClassifyProvider{responses: []string{resp}}

	_, err := classifyModulesWithEvidence(context.Background(), p, targets, layers, []string{testRepoEvidence})
	if err == nil {
		t.Fatal("missing evidence_refs must return an error")
	}
	if !strings.Contains(err.Error(), "missing evidence_refs") {
		t.Fatalf("error = %v, want missing evidence_refs", err)
	}
}

func TestClassifyModulesWithEvidence_InvalidEvidenceRefsError(t *testing.T) {
	t.Parallel()
	targets := []initcfg.ClassifyTarget{{Name: testMod0, Paths: []string{testMod0Path}}}
	layers := []string{testLayerDomain}
	resp := `[{"module":"mod0","subdomain":"core","volatility":"low","layer":"domain","name":"","rationale":"test","evidence_refs":["finding:123"],"basis":"semantic_judgment"}]`
	p := &fakeClassifyProvider{responses: []string{resp}}

	_, err := classifyModulesWithEvidence(context.Background(), p, targets, layers, []string{testRepoEvidence})
	if err == nil {
		t.Fatal("invalid evidence_refs must return an error")
	}
	if !strings.Contains(err.Error(), "invalid evidence_refs") {
		t.Fatalf("error = %v, want invalid evidence_refs", err)
	}
}

func TestClassifyModulesWithEvidence_UnsupportedEvidenceRefsError(t *testing.T) {
	t.Parallel()
	targets := []initcfg.ClassifyTarget{{Name: testMod0, Paths: []string{testMod0Path}}}
	layers := []string{testLayerDomain}
	resp := `[{"module":"mod0","subdomain":"core","volatility":"low","layer":"domain","name":"","rationale":"test","evidence_refs":["doc:missing.md"],"basis":"semantic_judgment"}]`
	p := &fakeClassifyProvider{responses: []string{resp}}

	_, err := classifyModulesWithEvidence(context.Background(), p, targets, layers, []string{testRepoEvidence})
	if err == nil {
		t.Fatal("unsupported evidence_refs must return an error")
	}
	if !strings.Contains(err.Error(), "unsupported evidence_refs") {
		t.Fatalf("error = %v, want unsupported evidence_refs", err)
	}
}

func TestClassifyModulesWithEvidence_ParsesRuleSuggestions(t *testing.T) {
	t.Parallel()
	targets := []initcfg.ClassifyTarget{{Name: testMod0, Paths: []string{testMod0Path}}}
	layers := []string{testLayerDomain}
	resp := `[{"module":"mod0","subdomain":"core","volatility":"low","layer":"domain","name":"","rationale":"test cites doc:README.md","evidence_refs":["doc:README.md"],"basis":"semantic_judgment","rule_suggestions":[{"id":"no-mod0-to-adapter","type":"forbidden_dependency","from":"internal/mod0/**","to":"internal/adapter/**","gate":"warn","rationale":"test cites doc:README.md","evidence_refs":["doc:README.md"],"basis":"semantic_judgment"}]}]`
	p := &fakeClassifyProvider{responses: []string{resp}}

	got, err := classifyModulesWithEvidence(context.Background(), p, targets, layers, []string{testRepoEvidence})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ann := got[testMod0]
	if len(ann.EvidenceRefs) != 1 || ann.Basis != "semantic_judgment" {
		t.Fatalf("annotation missing metadata: %+v", ann)
	}
	if len(ann.RuleSuggestions) != 1 {
		t.Fatalf("rule suggestions = %+v, want one", ann.RuleSuggestions)
	}
	rule := ann.RuleSuggestions[0]
	if rule.Type != ruleTypeForbiddenDependency || len(rule.EvidenceRefs) != 1 || rule.Basis != initcfg.DraftBasisSemanticJudgment {
		t.Fatalf("bad rule suggestion: %+v", rule)
	}
}

func TestClassifyModulesWithEvidence_ParsesExternalSystemSuggestions(t *testing.T) {
	t.Parallel()
	targets := []initcfg.ClassifyTarget{{Name: testMod0, Paths: []string{testMod0Path}}}
	layers := []string{testLayerDomain}
	resp := `[{"module":"mod0","subdomain":"core","volatility":"low","layer":"domain","name":"","rationale":"test cites doc:README.md","evidence_refs":["doc:README.md"],"basis":"semantic_judgment","external_system_suggestions":[{"name":"payments-vendor","targets":["github.com/vendor/sdk/**"],"volatility":"low","rationale":"vendor seam cites doc:README.md","evidence_refs":["doc:README.md"],"basis":"semantic_judgment"}]}]`
	p := &fakeClassifyProvider{responses: []string{resp}}

	got, err := classifyModulesWithEvidence(context.Background(), p, targets, layers, []string{testRepoEvidence})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ann := got[testMod0]
	if len(ann.ExternalSystemSuggestions) != 1 {
		t.Fatalf("external system suggestions = %+v, want one", ann.ExternalSystemSuggestions)
	}
	s := ann.ExternalSystemSuggestions[0]
	if s.Name != "payments-vendor" || len(s.Targets) != 1 || s.Targets[0] != "github.com/vendor/sdk/**" || s.Basis != initcfg.DraftBasisSemanticJudgment {
		t.Fatalf("bad external system suggestion: %+v", s)
	}
}

func TestRuleSuggestionShape_MatchesRuleSchema(t *testing.T) {
	t.Parallel()
	maxAPI := 20
	allowed := map[string]struct{}{testEvidenceRef: {}}
	entries := []ruleSuggestionResponse{
		{ID: "track-api", Type: ruleTypePublicAPIChange, Gate: "warn", Rationale: testEvidenceRationale, EvidenceRefs: []string{testEvidenceRef}, Basis: initcfg.DraftBasisSemanticJudgment},
		{ID: "api-max", Type: ruleTypePublicAPIMax, Gate: "warn", Max: &maxAPI, Rationale: testEvidenceRationale, EvidenceRefs: []string{testEvidenceRef}, Basis: initcfg.DraftBasisSemanticJudgment},
		{ID: "coupling-floor", Type: ruleTypeCouplingGate, Gate: "fail", MinBand: "serviceable", Rationale: testEvidenceRationale, EvidenceRefs: []string{testEvidenceRef}, Basis: initcfg.DraftBasisSemanticJudgment},
	}

	got, err := parseRuleSuggestionResponses(testMod0, entries, true, allowed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(entries) {
		t.Fatalf("rule suggestions = %+v, want %d", got, len(entries))
	}
	if got[0].Type != ruleTypePublicAPIChange || got[0].From != "" || got[0].To != "" {
		t.Fatalf("public_api_change should not require from/to: %+v", got[0])
	}
	if got[1].Type != ruleTypePublicAPIMax || got[1].Max == nil || *got[1].Max != maxAPI || got[1].From != "" {
		t.Fatalf("public_api_max should only require max: %+v", got[1])
	}
}

func TestClassifyModulesWithEvidence_UnsupportedRuleSuggestionEvidenceRefsError(t *testing.T) {
	t.Parallel()
	targets := []initcfg.ClassifyTarget{{Name: testMod0, Paths: []string{testMod0Path}}}
	layers := []string{testLayerDomain}
	resp := `[{"module":"mod0","subdomain":"core","volatility":"low","layer":"domain","name":"","rationale":"test cites doc:README.md","evidence_refs":["doc:README.md"],"basis":"semantic_judgment","rule_suggestions":[{"id":"bad-ref","type":"forbidden_dependency","from":"internal/mod0/**","to":"internal/adapter/**","rationale":"bad","evidence_refs":["doc:missing.md"],"basis":"semantic_judgment"}]}]`
	p := &fakeClassifyProvider{responses: []string{resp}}

	_, err := classifyModulesWithEvidence(context.Background(), p, targets, layers, []string{testRepoEvidence})
	if err == nil {
		t.Fatal("unsupported rule suggestion evidence_refs must return an error")
	}
	if !strings.Contains(err.Error(), "unsupported evidence_refs") {
		t.Fatalf("error = %v, want unsupported evidence_refs", err)
	}
}

func TestClassifyUserPrompt_IncludesRepoEvidenceAndPublicAPI(t *testing.T) {
	t.Parallel()
	targets := []initcfg.ClassifyTarget{{
		Name:   "billing",
		Paths:  []string{"internal/billing/**"},
		Public: []string{"internal/billing/api.go"},
		Files:  []string{"api.go"},
	}}
	got := classifyUserPrompt(targets, []string{testLayerDomain}, []string{"README.md: Billing"})
	for _, want := range []string{"public API globs: internal/billing/api.go", "Repository evidence:", "README.md: Billing"} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

func TestClassifyModules_BatchBoundary(t *testing.T) {
	t.Parallel()
	// 26 targets → 2 batches (25 + 1) with classifyBatchSize=25.
	targets := makeTargets(26)
	layers := []string{testLayerDomain}

	batch1 := targets[:25]
	batch2 := targets[25:]
	p := &fakeClassifyProvider{responses: []string{
		validResponse(batch1),
		validResponse(batch2),
	}}

	got, err := classifyModules(context.Background(), p, targets, layers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.calls != 2 {
		t.Errorf("provider calls = %d, want exactly 2 batches", p.calls)
	}
	if len(got) != 26 {
		t.Errorf("annotations = %d, want 26", len(got))
	}
}

func TestClassifyModules_JSONFenceTolerated(t *testing.T) {
	t.Parallel()
	targets := []initcfg.ClassifyTarget{{Name: testMod0, Paths: []string{testMod0Path}}}
	layers := []string{testLayerDomain}
	// Wrap valid JSON in markdown fences — should still parse.
	resp := "```json\n" + validResponse(targets) + "\n```"
	p := &fakeClassifyProvider{responses: []string{resp}}

	got, err := classifyModules(context.Background(), p, targets, layers)
	if err != nil {
		t.Fatalf("unexpected error with fenced JSON: %v", err)
	}
	if _, ok := got["mod0"]; !ok {
		t.Error("mod0 annotation missing after fence strip")
	}
}
