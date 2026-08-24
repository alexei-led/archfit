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
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/module"
	"github.com/alexei-led/archfit/internal/relationship/coupling"
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
	globPkgA = "pkg/a/**"
	globPkgB = "pkg/b/**"

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

func enrichFixture() (*graph.Graph, coupling.Index, module.Map) {
	cfg := config.Config{
		Version: 1,
		Modules: map[string]module.ModuleDef{
			modA: {Paths: []string{globPkgA}},
			modB: {Paths: []string{globPkgB}},
			"c":  {Paths: []string{"pkg/c/**"}},
		},
	}
	edges := []graph.Edge{
		{From: fileNodeA, To: fileNodeB, Kind: graph.EdgeKindImports, Language: "go"},
		{From: "file:pkg/a/a2.go", To: "file:pkg/b/b2.go", Kind: graph.EdgeKindImports, Language: "go"},
		{From: fileNodeA, To: fileNodeC, Kind: graph.EdgeKindImports, Language: "go"},
		{From: fileNodeC, To: fileNodeB, Kind: graph.EdgeKindImports, Language: "go"},
	}
	nodes := []graph.Node{
		{Kind: graph.NodeKindFile, Path: filePkgAA},
		{Kind: graph.NodeKindFile, Path: "pkg/a/a2.go"},
		{Kind: graph.NodeKindFile, Path: filePkgBB},
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

	pairs := selectRefinablePairs(g, idx, mm, existing, nil)
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

func syntheticRustPairFixture(strength coupling.Strength) (*graph.Graph, coupling.Index, module.Map, module.Map) {
	cfg := config.Config{Version: 1, Modules: map[string]module.ModuleDef{}}
	from := graph.Node{Kind: graph.NodeKindModule, Path: rustSyntheticFrom, Language: graph.LangRust}
	to := graph.Node{Kind: graph.NodeKindModule, Path: rustSyntheticTo, Language: graph.LangRust}
	edge := graph.Edge{From: from.ID(), To: to.ID(), Kind: graph.EdgeKindImports, Language: graph.LangRust}
	g := graph.Build([]graph.Facts{{
		Language: graph.LangRust,
		Nodes:    []graph.Node{from, to},
		Edges:    []graph.Edge{edge},
	}})
	idx := coupling.Index{
		edge.From + "\x00" + edge.To + "\x00" + string(edge.Kind): {
			Strength: strength,
			Distance: coupling.DistanceCrossModuleDiffOwner,
		},
	}
	return g, idx, cfg.ForClassify().ModuleMap, enrichModuleMap(cfg, g)
}

func TestSelectRefinablePairs_UsesAugmentedSyntheticModules(t *testing.T) {
	t.Parallel()
	g, idx, originalMM, augmentedMM := syntheticRustPairFixture(coupling.StrengthFunctional)

	if got := selectRefinablePairs(g, idx, originalMM, nil, nil); len(got) != 0 {
		t.Fatalf("unaugmented module map selected pairs = %+v, want none", got)
	}
	pairs := selectRefinablePairs(g, idx, augmentedMM, nil, nil)
	if len(pairs) != 1 {
		t.Fatalf("augmented module map selected pairs = %+v, want synthetic Rust pair", pairs)
	}
	if pairs[0].From != rustSyntheticFrom || pairs[0].To != rustSyntheticTo {
		t.Fatalf("pair = %s->%s, want %s->%s", pairs[0].From, pairs[0].To, rustSyntheticFrom, rustSyntheticTo)
	}
}

func TestSelectRefinablePairs_StaleApprovedCanBeRedrafted(t *testing.T) {
	t.Parallel()
	g, idx, mm := enrichFixture()
	key := labels.Key("c", modB)
	existing := []labels.Label{{
		From: "c", To: modB, Strength: enrichModel,
		EvidenceHash: staleEvidence, Status: labels.StatusApproved,
	}}
	evidence := map[string]string{key: currentEvidence}

	pairs := selectRefinablePairs(g, idx, mm, existing, evidence)
	found := false
	for _, p := range pairs {
		if labels.Key(p.From, p.To) == key {
			found = true
		}
	}
	if !found {
		t.Fatalf("stale approved pair %q was not selected for a replacement draft: %+v", key, pairs)
	}
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

func TestMergeDrafts(t *testing.T) {
	t.Parallel()
	existing := []labels.Label{
		{From: modA, To: modB, Strength: enrichModel, Status: labels.StatusApproved},
		{From: modB, To: modA, Strength: enrichFunctional, Status: labels.StatusDraft},
	}
	drafts := []labels.Label{
		{From: modA, To: modB, Strength: enrichIntrusive, Status: labels.StatusDraft}, // must NOT clobber approved
		{From: modB, To: modA, Strength: enrichModel, Status: labels.StatusDraft},     // replaces old draft
		{From: "c", To: modA, Strength: "contract", Status: labels.StatusDraft},       // new
	}

	merged := mergeDrafts(existing, drafts, nil)
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

func TestMergeDrafts_ReplacesStaleApproved(t *testing.T) {
	t.Parallel()
	key := labels.Key(modA, modB)
	existing := []labels.Label{{
		From: modA, To: modB, Strength: enrichModel,
		EvidenceHash: staleEvidence, Status: labels.StatusApproved,
	}}
	drafts := []labels.Label{{
		From: modA, To: modB, Strength: enrichIntrusive,
		EvidenceHash: currentEvidence, Status: labels.StatusDraft,
	}}

	merged := mergeDrafts(existing, drafts, map[string]string{key: currentEvidence})
	if len(merged) != 1 {
		t.Fatalf("merged = %+v, want one replacement", merged)
	}
	if merged[0].Status != labels.StatusDraft || merged[0].Strength != enrichIntrusive {
		t.Fatalf("stale approved label was not replaced: %+v", merged[0])
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
		Modules: map[string]module.ModuleDef{
			modA: {Paths: []string{globPkgA}},
			modB: {Paths: []string{globPkgB}},
		},
	}
	edges := []graph.Edge{
		{From: fileNodeA, To: fileNodeB, Kind: graph.EdgeKindImports, Language: "go"},
	}
	nodes := []graph.Node{
		{Kind: graph.NodeKindFile, Path: filePkgAA},
		{Kind: graph.NodeKindFile, Path: filePkgBB},
	}
	g := graph.Build([]graph.Facts{{Language: "go", Nodes: nodes, Edges: edges}})
	idx := coupling.Index{
		fileNodeA + "\x00" + fileNodeB + "\x00imports": {
			Strength: coupling.StrengthUnknown,
			Distance: coupling.DistanceCrossModuleDiffOwner,
		},
	}
	mm := cfg.ForClassify().ModuleMap

	pairs := selectRefinablePairs(g, idx, mm, nil, nil)
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
