package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/initcfg"
	"github.com/alexei-led/archfit/internal/llm"
	"github.com/alexei-led/archfit/internal/relationship/labels"
)

const (
	modA             = "a"
	modB             = "b"
	enrichModel      = "model"
	enrichFunctional = "functional"
	enrichIntrusive  = "intrusive"
	staleEvidence    = "old"
	currentEvidence  = "current"

	// fileNodeA and fileNodeB are the file-URI node identifiers used in
	// enrichFixture and the unknown-strength test.
	fileNodeA = "file:pkg/a/a.go"
	fileNodeB = "file:pkg/b/b.go"
	fileNodeC = "file:pkg/c/c.go"
	filePkgBB = "pkg/b/b.go"

	// globPkgA and globPkgB are the module path globs shared by enrich fixtures.

	rustSyntheticFrom  = "crate::api"
	rustSyntheticTo    = "crate::domain"
	enrichEvidenceAPIA = "api:a"
)

func enrichDraftJSON(from, to, strength, rationale string) string {
	return fmt.Sprintf(`{"from":%q,"to":%q,"strength":%q,"basis":%q,"evidence_refs":[],"rationale":%q}`,
		from, to, strength, initcfg.DraftBasisSemanticJudgment, rationale)
}

func enrichDraftJSONWithRefs(from, to, strength, rationale string, refs ...string) string {
	data, err := json.Marshal(struct {
		From         string   `json:"from"`
		To           string   `json:"to"`
		Strength     string   `json:"strength"`
		Basis        string   `json:"basis"`
		EvidenceRefs []string `json:"evidence_refs"`
		Rationale    string   `json:"rationale"`
	}{
		From:         from,
		To:           to,
		Strength:     strength,
		Basis:        initcfg.DraftBasisSemanticJudgment,
		EvidenceRefs: refs,
		Rationale:    rationale,
	})
	if err != nil {
		panic(err)
	}
	return string(data)
}

func abstainedDraftJSON(from, to, strength, confidence, rationale string) string {
	return fmt.Sprintf(`{"from":%q,"to":%q,"strength":%q,"confidence":%q,"basis":%q,"evidence_refs":[],"rationale":%q}`,
		from, to, strength, confidence, initcfg.DraftBasisSemanticJudgment, rationale)
}

func TestEnrichUserPrompt_IncludesRepositoryEvidenceIDs(t *testing.T) {
	t.Parallel()
	prompt := enrichUserPrompt(
		config.Config{},
		[]refinablePair{{From: modA, To: modB, Strength: enrichFunctional, EdgeCount: 1, SamplePaths: []string{"pkg/a/a.go -> pkg/b/b.go"}}},
		[]string{enrichEvidenceAPIA + " (api) a: exported names: Service"},
	)
	for _, want := range []string{repositoryEvidenceHeader, enrichEvidenceAPIA, "exported names: Service", "from: a"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestParseEnrichResponse(t *testing.T) {
	t.Parallel()
	batch := []refinablePair{{From: modA, To: modB}}

	t.Run("valid json with fences", func(t *testing.T) {
		t.Parallel()
		text := "```json\n[" + enrichDraftJSONWithRefs(modA, modB, enrichModel, "types cross", enrichEvidenceAPIA) + "]\n```"
		got, err := parseEnrichResponse(text, batch, map[string]struct{}{enrichEvidenceAPIA: {}})
		if err != nil || len(got) != 1 {
			t.Fatalf("(%v, %v)", got, err)
		}
		if got[0].Status != labels.StatusDraft || got[0].Strength != enrichModel {
			t.Errorf("draft = %+v", got[0])
		}
		if got[0].Basis != initcfg.DraftBasisSemanticJudgment || len(got[0].EvidenceRefs) != 1 || got[0].EvidenceRefs[0] != enrichEvidenceAPIA {
			t.Errorf("metadata = basis %q refs %v", got[0].Basis, got[0].EvidenceRefs)
		}
	})

	t.Run("unrequested pair and invalid strength skipped", func(t *testing.T) {
		t.Parallel()
		text := `[` + enrichDraftJSON("x", "y", enrichModel, "hallucinated pair") + `,` + enrichDraftJSON(modA, modB, "mega", "invalid strength") + `]`
		got, err := parseEnrichResponse(text, batch)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Errorf("got = %+v, want none", got)
		}
	})

	t.Run("malformed body is an error", func(t *testing.T) {
		t.Parallel()
		if _, err := parseEnrichResponse("the strength is model, trust me", batch); err == nil {
			t.Error("prose response must be rejected")
		}
	})

	for name, text := range map[string]string{
		"missing basis":   `[{"from":"a","to":"b","strength":"model","evidence_refs":[],"rationale":"types cross"}]`,
		"invalid basis":   `[{"from":"a","to":"b","strength":"model","basis":"vibes","evidence_refs":[],"rationale":"types cross"}]`,
		"invalid ref":     `[{"from":"a","to":"b","strength":"model","basis":"semantic_judgment","evidence_refs":["bad ref"],"rationale":"types cross"}]`,
		"unsupported ref": `[{"from":"a","to":"b","strength":"model","basis":"semantic_judgment","evidence_refs":["api:missing"],"rationale":"types cross"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseEnrichResponse(text, batch, map[string]struct{}{enrichEvidenceAPIA: {}}); err == nil {
				t.Error("want schema-validation error")
			}
		})
	}
}

// scriptedProvider returns canned responses per call.
type scriptedProvider struct {
	responses []string
	calls     int
}

func (p *scriptedProvider) Name() string { return "test/scripted" }
func (p *scriptedProvider) Complete(_ context.Context, _ llm.Request) (llm.Response, error) {
	if p.calls >= len(p.responses) {
		return llm.Response{}, fmt.Errorf("scriptedProvider: no canned response for call %d (have %d)", p.calls, len(p.responses))
	}
	r := p.responses[p.calls]
	p.calls++
	return llm.Response{Text: r}, nil
}

// TestDraftLabels_ProvenanceAndConfidence checks that drafts carry provenance:llm
// and confidence:medium on all parsed entries.
func TestDraftLabels_ProvenanceAndConfidence(t *testing.T) {
	t.Parallel()
	pairs := []refinablePair{
		{From: modA, To: modB, Strength: enrichFunctional},
	}
	resp := `[` + enrichDraftJSON(modA, modB, enrichModel, "types cross") + `]`
	p := &scriptedProvider{responses: []string{resp}}

	got, err := draftLabels(context.Background(), p, config.Config{}, pairs)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 draft, got %d", len(got))
	}
	d := got[0]
	if d.Provenance != labels.ProvenanceLLM {
		t.Errorf("provenance = %q, want %q", d.Provenance, labels.ProvenanceLLM)
	}
	if d.Confidence != labels.ConfidenceMedium {
		t.Errorf("confidence = %q, want %q", d.Confidence, labels.ConfidenceMedium)
	}
	if d.Status != labels.StatusDraft {
		t.Errorf("status = %q, want %q", d.Status, labels.StatusDraft)
	}
	if d.Basis != initcfg.DraftBasisSemanticJudgment {
		t.Errorf("basis = %q, want %q", d.Basis, initcfg.DraftBasisSemanticJudgment)
	}
}

func TestDraftLabels_BatchesAndParses(t *testing.T) {
	t.Parallel()
	// 31 pairs → two batches with enrichBatchSize 30.
	pairs := make([]refinablePair, 31)
	for i := range pairs {
		from := string(rune('a' + i%26))
		pairs[i] = refinablePair{From: from, To: "z", Strength: enrichFunctional}
	}
	// Build per-batch valid responses.
	build := func(ps []refinablePair) string {
		parts := make([]string, 0, len(ps))
		for _, p := range ps {
			parts = append(parts, enrichDraftJSON(p.From, "z", enrichModel, "r"))
		}
		return "[" + strings.Join(parts, ",") + "]"
	}
	responses := []string{build(pairs[:30]), build(pairs[30:])}

	p := &scriptedProvider{responses: responses}
	got, err := draftLabels(context.Background(), p, config.Config{}, pairs)
	if err != nil {
		t.Fatal(err)
	}
	if p.calls != 2 {
		t.Errorf("provider calls = %d, want 2 batches", p.calls)
	}
	// Duplicate from-keys collapse is mergeDrafts' job; here we expect raw drafts.
	if len(got) != 31 {
		t.Errorf("drafts = %d, want 31", len(got))
	}
}

// TestRun_Explain_LLMNarrative drives explain --ai-summary end-to-end against a mock
// OpenAI-compatible server (the ollama provider path).
func TestRun_Explain_LLMNarrative(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c1","object":"chat.completion","model":"m",
			"choices":[{"index":0,"message":{"role":"assistant","content":"This intrusive edge bypasses the module contract."},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(srv.Close)

	cfgPath := writeViolatingRepo(t)
	// Append the llm tool config to the fixture.
	cfgRaw, _ := os.ReadFile(cfgPath) //nolint:gosec // test fixture path from t.TempDir
	cfgRaw = append(cfgRaw, []byte("ai:\n  provider: ollama\n  model: test-model\n  base_url: "+srv.URL+"\n")...)
	if err := os.WriteFile(cfgPath, cfgRaw, 0o600); err != nil { //nolint:gosec // test fixture path from t.TempDir
		t.Fatal(err)
	}

	// Find the finding fingerprint.
	var buf bytes.Buffer
	Run([]string{cmdAnalyze, "-c", cfgPath, flagRefresh, fmtJSON}, &buf)
	var diag struct {
		Findings []struct {
			ID string `json:"id"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(buf.Bytes(), &diag); err != nil || len(diag.Findings) == 0 {
		t.Fatalf("no findings: %v\n%s", err, buf.String())
	}

	buf.Reset()
	code := Run([]string{cmdExplain, diag.Findings[0].ID[:8], "-c", cfgPath, "--ai-summary", flagRefresh}, &buf)
	if code != 0 {
		t.Fatalf("explain --ai-summary exit = %d\n%s", code, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "narrative (ollama/test-model, off-gate):") {
		t.Errorf("missing narrative header\n%s", out)
	}
	if !strings.Contains(out, "This intrusive edge bypasses the module contract.") {
		t.Errorf("missing narrative body\n%s", out)
	}
	// The deterministic dump must still precede it.
	if !strings.Contains(out, "rule:") || !strings.Contains(out, "constraint:") {
		t.Errorf("deterministic explain output missing\n%s", out)
	}
}

// TestRun_Explain_LLMUnconfigured verifies the setup hint when ai is absent.
func TestRun_Explain_LLMUnconfigured(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)
	var buf bytes.Buffer
	Run([]string{cmdAnalyze, "-c", cfgPath, flagRefresh, fmtJSON}, &buf)
	var diag struct {
		Findings []struct {
			ID string `json:"id"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(buf.Bytes(), &diag); err != nil || len(diag.Findings) == 0 {
		t.Fatal("no findings")
	}
	buf.Reset()
	if code := Run([]string{cmdExplain, diag.Findings[0].ID[:8], "-c", cfgPath, "--ai-summary"}, &buf); code != 3 {
		t.Errorf("exit = %d, want 3 with setup hint", code)
	}
}
