package main

import (
	"bytes"
	"context"
	"encoding/json"
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
		{From: "file:pkg/a/a.go", To: "file:pkg/b/b.go", Kind: graph.EdgeKindImports, Language: "go"},
		{From: "file:pkg/a/a2.go", To: "file:pkg/b/b2.go", Kind: graph.EdgeKindImports, Language: "go"},
		{From: "file:pkg/a/a.go", To: "file:pkg/c/c.go", Kind: graph.EdgeKindImports, Language: "go"},
		{From: "file:pkg/c/c.go", To: "file:pkg/b/b.go", Kind: graph.EdgeKindImports, Language: "go"},
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
		"file:pkg/a/a.go\x00file:pkg/b/b.go\x00imports":   cross,
		"file:pkg/a/a2.go\x00file:pkg/b/b2.go\x00imports": cross,
		// a→c is contract (glob-decided elsewhere) — must not be refinable.
		"file:pkg/a/a.go\x00file:pkg/c/c.go\x00imports": {Strength: coupling.StrengthContract, Distance: coupling.DistanceCrossModuleDiffOwner},
		// c→b functional but same-module distance? keep cross to test approved skip.
		"file:pkg/c/c.go\x00file:pkg/b/b.go\x00imports": cross,
	}
	return g, idx, cfg.ForClassify().ModuleMap
}

func TestSelectRefinablePairs(t *testing.T) {
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
	batch := []refinablePair{{From: modA, To: modB}}

	t.Run("valid json with fences", func(t *testing.T) {
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
		if _, err := parseEnrichResponse("the strength is model, trust me", batch); err == nil {
			t.Error("prose response must be rejected")
		}
	})
}

func TestMergeDrafts(t *testing.T) {
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

// scriptedProvider returns canned responses per call.
type scriptedProvider struct {
	responses []string
	calls     int
}

func (p *scriptedProvider) Name() string { return "test/scripted" }
func (p *scriptedProvider) Complete(_ context.Context, _ llm.Request) (llm.Response, error) {
	r := p.responses[p.calls]
	p.calls++
	return llm.Response{Text: r}, nil
}

func TestDraftLabels_BatchesAndParses(t *testing.T) {
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
