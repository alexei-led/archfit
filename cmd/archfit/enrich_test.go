package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/labels"
	"github.com/alexei-led/archfit/internal/llm"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/graph"
)

const (
	modA             = "a"
	modB             = "b"
	enrichModel      = "model"
	enrichFunctional = "functional"

	// fileNodeA and fileNodeB are the file-URI node identifiers used in
	// enrichFixture and the unknown-strength test.
	fileNodeA = "file:pkg/a/a.go"
	fileNodeB = "file:pkg/b/b.go"
)

func enrichFixture() (*graph.Graph, coupling.Index, config.ModuleMap) {
	cfg := config.Config{
		Version: 1,
		Modules: map[string]config.ModuleDef{
			modA: {Paths: []string{"pkg/a/**"}},
			modB: {Paths: []string{"pkg/b/**"}},
			"c":  {Paths: []string{"pkg/c/**"}},
		},
	}
	edges := []graph.Edge{
		{From: fileNodeA, To: fileNodeB, Kind: graph.EdgeKindImports, Language: "go"},
		{From: "file:pkg/a/a2.go", To: "file:pkg/b/b2.go", Kind: graph.EdgeKindImports, Language: "go"},
		{From: fileNodeA, To: "file:pkg/c/c.go", Kind: graph.EdgeKindImports, Language: "go"},
		{From: "file:pkg/c/c.go", To: fileNodeB, Kind: graph.EdgeKindImports, Language: "go"},
	}
	nodes := []graph.Node{
		{Kind: graph.NodeKindFile, Path: filePkgAA},
		{Kind: graph.NodeKindFile, Path: "pkg/a/a2.go"},
		{Kind: graph.NodeKindFile, Path: "pkg/b/b.go"},
		{Kind: graph.NodeKindFile, Path: "pkg/b/b2.go"},
		{Kind: graph.NodeKindFile, Path: "pkg/c/c.go"},
	}
	g := graph.Build([]graph.Facts{{Language: "go", Nodes: nodes, Edges: edges}})

	cross := coupling.Classification{Strength: coupling.StrengthFunctional, Distance: coupling.DistanceCrossModuleDiffOwner}
	idx := coupling.Index{
		fileNodeA + "\x00" + fileNodeB + "\x00imports":    cross,
		"file:pkg/a/a2.go\x00file:pkg/b/b2.go\x00imports": cross,
		// a→c is contract (glob-decided elsewhere) — must not be refinable.
		fileNodeA + "\x00file:pkg/c/c.go\x00imports": {Strength: coupling.StrengthContract, Distance: coupling.DistanceCrossModuleDiffOwner},
		// c→b functional but same-module distance? keep cross to test approved skip.
		"file:pkg/c/c.go\x00" + fileNodeB + "\x00imports": cross,
	}
	return g, idx, cfg.ForClassify().ModuleMap
}

func TestSelectRefinablePairs(t *testing.T) {
	t.Parallel()
	g, idx, mm := enrichFixture()

	// c→b is already approved — must be excluded.
	existing := []labels.Label{{From: "c", To: modB, Strength: enrichModel, Status: labels.StatusApproved}}

	pairs := selectRefinablePairs(g, idx, mm, existing)
	if len(pairs) != 1 {
		t.Fatalf("pairs = %+v, want exactly a->b", pairs)
	}
	p := pairs[0]
	if p.From != modA || p.To != modB {
		t.Errorf("pair = %s->%s, want a->b", p.From, p.To)
	}
	if p.EdgeCount != 2 {
		t.Errorf("edge count = %d, want 2", p.EdgeCount)
	}
	if p.Strength != enrichFunctional {
		t.Errorf("strength = %q", p.Strength)
	}
	if len(p.SamplePaths) != 2 || !strings.Contains(p.SamplePaths[0], "pkg/a/") {
		t.Errorf("samples = %v", p.SamplePaths)
	}
}

func TestParseEnrichResponse(t *testing.T) {
	t.Parallel()
	batch := []refinablePair{{From: modA, To: modB}}

	t.Run("valid json with fences", func(t *testing.T) {
		t.Parallel()
		text := "```json\n[{\"from\":\"a\",\"to\":\"b\",\"strength\":\"model\",\"rationale\":\"types cross\"}]\n```"
		got, err := parseEnrichResponse(text, batch)
		if err != nil || len(got) != 1 {
			t.Fatalf("(%v, %v)", got, err)
		}
		if got[0].Status != labels.StatusDraft || got[0].Strength != enrichModel {
			t.Errorf("draft = %+v", got[0])
		}
	})

	t.Run("unrequested pair and invalid strength skipped", func(t *testing.T) {
		t.Parallel()
		text := `[
			{"from":"x","to":"y","strength":"model","rationale":"hallucinated pair"},
			{"from":"a","to":"b","strength":"mega","rationale":"invalid strength"}
		]`
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
}

func TestMergeDrafts(t *testing.T) {
	t.Parallel()
	existing := []labels.Label{
		{From: modA, To: modB, Strength: enrichModel, Status: labels.StatusApproved},
		{From: modB, To: modA, Strength: enrichFunctional, Status: labels.StatusDraft},
	}
	drafts := []labels.Label{
		{From: modA, To: modB, Strength: "intrusive", Status: labels.StatusDraft}, // must NOT clobber approved
		{From: modB, To: modA, Strength: enrichModel, Status: labels.StatusDraft}, // replaces old draft
		{From: "c", To: modA, Strength: "contract", Status: labels.StatusDraft},   // new
	}

	merged := mergeDrafts(existing, drafts)
	if len(merged) != 3 {
		t.Fatalf("merged = %+v, want 3", merged)
	}
	byKey := map[string]labels.Label{}
	for _, l := range merged {
		byKey[labels.Key(l.From, l.To)] = l
	}
	if got := byKey[labels.Key(modA, modB)]; got.Status != labels.StatusApproved || got.Strength != enrichModel {
		t.Errorf("approved entry clobbered: %+v", got)
	}
	if got := byKey[labels.Key(modB, modA)]; got.Strength != enrichModel {
		t.Errorf("draft not replaced: %+v", got)
	}
	// Deterministic order.
	if merged[0].From > merged[1].From || merged[1].From > merged[2].From {
		t.Errorf("not sorted: %+v", merged)
	}
}

// TestEnrich_DraftModesMutuallyExclusive verifies that combining draft modes is
// rejected before any work runs, instead of silently running just one.
func TestEnrich_DraftModesMutuallyExclusive(t *testing.T) {
	t.Parallel()
	combos := []EnrichCmd{
		{Owner: true, Volatility: true},
		{Subdomains: true, Owner: true},
		{Subdomains: true, Volatility: true},
		{Subdomains: true, Owner: true, Volatility: true},
	}
	for _, c := range combos {
		var buf bytes.Buffer
		err := c.Run(&appDeps{Stdout: &buf})
		var ee *exitError
		if !errors.As(err, &ee) || ee.code != 3 {
			t.Errorf("Owner=%v Subdomains=%v Volatility=%v: want exitError code 3, got %v",
				c.Owner, c.Subdomains, c.Volatility, err)
		}
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

// TestSelectRefinablePairs_UnknownStrength verifies that edges with
// StrengthUnknown (no heuristic available) are selected without requiring SCIP.
func TestSelectRefinablePairs_UnknownStrength(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		Version: 1,
		Modules: map[string]config.ModuleDef{
			modA: {Paths: []string{"pkg/a/**"}},
			modB: {Paths: []string{"pkg/b/**"}},
		},
	}
	edges := []graph.Edge{
		{From: fileNodeA, To: fileNodeB, Kind: graph.EdgeKindImports, Language: "go"},
	}
	nodes := []graph.Node{
		{Kind: graph.NodeKindFile, Path: filePkgAA},
		{Kind: graph.NodeKindFile, Path: "pkg/b/b.go"},
	}
	g := graph.Build([]graph.Facts{{Language: "go", Nodes: nodes, Edges: edges}})
	idx := coupling.Index{
		fileNodeA + "\x00" + fileNodeB + "\x00imports": {
			Strength: coupling.StrengthUnknown,
			Distance: coupling.DistanceCrossModuleDiffOwner,
		},
	}
	mm := cfg.ForClassify().ModuleMap

	pairs := selectRefinablePairs(g, idx, mm, nil)
	if len(pairs) != 1 {
		t.Fatalf("pairs = %+v, want exactly a->b for unknown strength", pairs)
	}
	if pairs[0].Strength != string(coupling.StrengthUnknown) {
		t.Errorf("strength = %q, want %q", pairs[0].Strength, coupling.StrengthUnknown)
	}
}

// TestDraftLabels_ProvenanceAndConfidence checks that drafts carry provenance:llm
// and confidence:medium on all parsed entries.
func TestDraftLabels_ProvenanceAndConfidence(t *testing.T) {
	t.Parallel()
	pairs := []refinablePair{
		{From: modA, To: modB, Strength: enrichFunctional},
	}
	resp := `[{"from":"a","to":"b","strength":"model","rationale":"types cross"}]`
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
			parts = append(parts, `{"from":"`+p.From+`","to":"z","strength":"model","rationale":"r"}`)
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

// TestRun_Explain_LLMNarrative drives explain --llm end-to-end against a mock
// OpenAI-compatible server (the ollama provider path).
func TestRun_Explain_LLMNarrative(t *testing.T) {
	t.Parallel()
	skipSlowPipelineTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c1","object":"chat.completion","model":"m",
			"choices":[{"index":0,"message":{"role":"assistant","content":"This intrusive edge bypasses the module contract."},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(srv.Close)

	cfgPath := writeViolatingRepo(t)
	// Append the llm tool config to the fixture.
	cfgRaw, _ := os.ReadFile(cfgPath) //nolint:gosec // test fixture path from t.TempDir
	cfgRaw = append(cfgRaw, []byte("tools:\n  llm:\n    provider: ollama\n    model: test-model\n    base_url: "+srv.URL+"\n")...)
	if err := os.WriteFile(cfgPath, cfgRaw, 0o600); err != nil { //nolint:gosec // test fixture path from t.TempDir
		t.Fatal(err)
	}

	// Find the finding fingerprint.
	var buf bytes.Buffer
	Run([]string{cmdCheck, "-c", cfgPath, flagFull, flagReport, fmtJSON}, &buf)
	var diag struct {
		Findings []struct {
			ID string `json:"id"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(buf.Bytes(), &diag); err != nil || len(diag.Findings) == 0 {
		t.Fatalf("no findings: %v\n%s", err, buf.String())
	}

	buf.Reset()
	code := Run([]string{cmdExplain, diag.Findings[0].ID[:8], "-c", cfgPath, "--llm", "--no-cache"}, &buf)
	if code != 0 {
		t.Fatalf("explain --llm exit = %d\n%s", code, buf.String())
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

// TestRun_Explain_LLMUnconfigured verifies the setup hint when tools.llm is absent.
func TestRun_Explain_LLMUnconfigured(t *testing.T) {
	t.Parallel()
	skipSlowPipelineTest(t)
	cfgPath := writeViolatingRepo(t)
	var buf bytes.Buffer
	Run([]string{cmdCheck, "-c", cfgPath, flagFull, flagReport, fmtJSON}, &buf)
	var diag struct {
		Findings []struct {
			ID string `json:"id"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(buf.Bytes(), &diag); err != nil || len(diag.Findings) == 0 {
		t.Fatal("no findings")
	}
	buf.Reset()
	if code := Run([]string{cmdExplain, diag.Findings[0].ID[:8], "-c", cfgPath, "--llm"}, &buf); code != 3 {
		t.Errorf("exit = %d, want 3 with setup hint", code)
	}
}
