package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/initcfg"
	"github.com/alexei-led/archfit/internal/labels/labelsio"
	"github.com/alexei-led/archfit/internal/llm"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/module"
	"github.com/alexei-led/archfit/internal/relationship/coupling"
	"github.com/alexei-led/archfit/internal/relationship/labels"
)

// cmdAbstained is the `config enrich abstained` subcommand token.
const cmdAbstained = "abstained"

// abstainedFixture builds a graph + index with one abstained a→b pair (two
// unknown-strength edges, the first carrying a location), plus edges that must
// be excluded: known strength, same-module distance, external (unknown)
// distance, and an approved pair.
func abstainedFixture() (*graph.Graph, coupling.Index, module.Map) {
	cfg := config.Config{
		Version: 1,
		Modules: map[string]module.ModuleDef{
			modA: {Paths: []string{globPkgA}},
			modB: {Paths: []string{globPkgB}},
			"c":  {Paths: []string{"pkg/c/**"}},
		},
	}
	edges := []graph.Edge{
		{From: fileNodeA, To: fileNodeB, Kind: graph.EdgeKindImports, Language: "go",
			Locations: []graph.Location{{File: filePkgAA, Line: 3}}},
		{From: "file:pkg/a/a2.go", To: "file:pkg/b/b2.go", Kind: graph.EdgeKindImports, Language: "go"},
		{From: fileNodeA, To: fileNodeC, Kind: graph.EdgeKindImports, Language: "go"},
		{From: fileNodeC, To: fileNodeB, Kind: graph.EdgeKindImports, Language: "go"},
		{From: fileNodeC, To: "file:pkg/c/c2.go", Kind: graph.EdgeKindImports, Language: "go"},
		{From: "file:pkg/c/c2.go", To: "file:vendor/x/x.go", Kind: graph.EdgeKindImports, Language: "go"},
	}
	nodes := []graph.Node{
		{Kind: graph.NodeKindFile, Path: filePkgAA},
		{Kind: graph.NodeKindFile, Path: "pkg/a/a2.go"},
		{Kind: graph.NodeKindFile, Path: filePkgBB},
		{Kind: graph.NodeKindFile, Path: "pkg/b/b2.go"},
		{Kind: graph.NodeKindFile, Path: "pkg/c/c.go"},
		{Kind: graph.NodeKindFile, Path: "pkg/c/c2.go"},
		{Kind: graph.NodeKindFile, Path: "vendor/x/x.go"},
	}
	g := graph.Build([]graph.Facts{{Language: "go", Nodes: nodes, Edges: edges}})

	unknown := coupling.Classification{Strength: coupling.StrengthUnknown, Distance: coupling.DistanceCrossModuleDiffOwner}
	idx := coupling.Index{
		// a→b: abstained twice (the selectable pair).
		fileNodeA + "\x00" + fileNodeB + "\x00imports":    unknown,
		"file:pkg/a/a2.go\x00file:pkg/b/b2.go\x00imports": unknown,
		// a→c: known strength — NOT abstained, excluded.
		fileNodeA + "\x00file:pkg/c/c.go\x00imports": {Strength: coupling.StrengthFunctional, Distance: coupling.DistanceCrossModuleDiffOwner},
		// c→b: abstained but the pair is approved in the tests below.
		"file:pkg/c/c.go\x00" + fileNodeB + "\x00imports": unknown,
		// c→c2: same-module — excluded.
		"file:pkg/c/c.go\x00file:pkg/c/c2.go\x00imports": {Strength: coupling.StrengthUnknown, Distance: coupling.DistanceSameModule},
		// c2→vendor: external (unknown distance) — missing facts, excluded.
		"file:pkg/c/c2.go\x00file:vendor/x/x.go\x00imports": {Strength: coupling.StrengthUnknown, Distance: coupling.DistanceUnknown},
	}
	return g, idx, cfg.ForClassify().ModuleMap
}

func TestAbstainedUserPrompt_IncludesRepositoryEvidenceIDs(t *testing.T) {
	t.Parallel()
	prompt := abstainedUserPrompt(
		config.Config{},
		[]abstainedPair{{From: modA, To: modB, EdgeCount: 1, Samples: []abstainedSample{{FromPath: fileNodeA, ToPath: fileNodeB}}}},
		[]string{"doc:docs/architecture/layers.md (doc) docs/architecture/layers.md: Layer intent"},
	)
	for _, want := range []string{repositoryEvidenceHeader, "doc:docs/architecture/layers.md", "Layer intent", "from: a"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestSelectAbstainedPairs(t *testing.T) {
	t.Parallel()
	g, idx, mm := abstainedFixture()
	existing := []labels.Label{{From: "c", To: modB, Strength: enrichModel, Status: labels.StatusApproved}}

	pairs, total := selectAbstainedPairs(g, idx, mm, existing, nil)
	if len(pairs) != 1 {
		t.Fatalf("pairs = %+v, want exactly a->b", pairs)
	}
	p := pairs[0]
	if p.From != modA || p.To != modB {
		t.Errorf("pair = %s->%s, want a->b", p.From, p.To)
	}
	if p.EdgeCount != 2 || total != 2 {
		t.Errorf("edge count = %d, total = %d, want 2, 2", p.EdgeCount, total)
	}
	if len(p.Samples) != 2 {
		t.Fatalf("samples = %+v, want 2", p.Samples)
	}
	// Context completeness: endpoints on every sample, location on the located edge.
	located := false
	for _, s := range p.Samples {
		if s.FromPath == "" || s.ToPath == "" {
			t.Errorf("sample missing endpoints: %+v", s)
		}
		if s.File == filePkgAA && s.Line == 3 {
			located = true
		}
	}
	if !located {
		t.Errorf("no sample carries the pkg/a/a.go:3 location: %+v", p.Samples)
	}
}

func TestSelectAbstainedPairs_UsesAugmentedSyntheticModules(t *testing.T) {
	t.Parallel()
	g, idx, originalMM, augmentedMM := syntheticRustPairFixture(coupling.StrengthUnknown)

	if pairs, total := selectAbstainedPairs(g, idx, originalMM, nil, nil); len(pairs) != 0 || total != 0 {
		t.Fatalf("unaugmented module map selected pairs = %+v total=%d, want none", pairs, total)
	}
	pairs, total := selectAbstainedPairs(g, idx, augmentedMM, nil, nil)
	if len(pairs) != 1 || total != 1 {
		t.Fatalf("augmented module map selected pairs = %+v total=%d, want one synthetic Rust pair", pairs, total)
	}
	if pairs[0].From != rustSyntheticFrom || pairs[0].To != rustSyntheticTo {
		t.Fatalf("pair = %s->%s, want %s->%s", pairs[0].From, pairs[0].To, rustSyntheticFrom, rustSyntheticTo)
	}
}

func TestSelectAbstainedPairs_StaleApprovedCanBeRedrafted(t *testing.T) {
	t.Parallel()
	g, idx, mm := abstainedFixture()
	key := labels.Key("c", modB)
	existing := []labels.Label{{
		From: "c", To: modB, Strength: enrichModel,
		EvidenceHash: staleEvidence, Status: labels.StatusApproved,
	}}
	evidence := map[string]string{key: currentEvidence}

	pairs, total := selectAbstainedPairs(g, idx, mm, existing, evidence)
	if total != 3 {
		t.Fatalf("total = %d, want 3 (stale approved pair should be eligible)", total)
	}
	found := false
	for _, p := range pairs {
		if labels.Key(p.From, p.To) == key {
			found = true
		}
	}
	if !found {
		t.Fatalf("stale approved abstained pair %q was not selected: %+v", key, pairs)
	}
}

func TestSelectAbstainedPairs_CapRespected(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		Version: 1,
		Modules: map[string]module.ModuleDef{
			modA: {Paths: []string{globPkgA}},
			modB: {Paths: []string{globPkgB}},
		},
	}
	const eligible = abstainedEdgeCap + 20
	edges := make([]graph.Edge, 0, eligible)
	nodes := make([]graph.Node, 0, 1+eligible)
	nodes = append(nodes, graph.Node{Kind: graph.NodeKindFile, Path: filePkgBB})
	idx := coupling.Index{}
	unknown := coupling.Classification{Strength: coupling.StrengthUnknown, Distance: coupling.DistanceCrossModuleDiffOwner}
	for i := range eligible {
		from := fmt.Sprintf("file:pkg/a/f%03d.go", i)
		edges = append(edges, graph.Edge{From: from, To: fileNodeB, Kind: graph.EdgeKindImports, Language: "go"})
		nodes = append(nodes, graph.Node{Kind: graph.NodeKindFile, Path: fmt.Sprintf("pkg/a/f%03d.go", i)})
		idx[from+"\x00"+fileNodeB+"\x00imports"] = unknown
	}
	g := graph.Build([]graph.Facts{{Language: "go", Nodes: nodes, Edges: edges}})

	pairs, total := selectAbstainedPairs(g, idx, cfg.ForClassify().ModuleMap, nil, nil)
	if total != eligible {
		t.Errorf("total = %d, want %d (edges beyond the cap must still be counted)", total, eligible)
	}
	included := 0
	for _, p := range pairs {
		included += p.EdgeCount
		if len(p.Samples) > abstainedSampleLocs {
			t.Errorf("pair %s->%s has %d samples, cap is %d", p.From, p.To, len(p.Samples), abstainedSampleLocs)
		}
	}
	if included != abstainedEdgeCap {
		t.Errorf("included edges = %d, want cap %d", included, abstainedEdgeCap)
	}
}

func TestLoadSnippet(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	dir := filepath.Join(base, "repo")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for i := 1; i <= 10; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	if err := os.WriteFile(filepath.Join(dir, "f.go"), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	got := loadSnippet(dir, "f.go", 5)
	if !strings.Contains(got, "5: line 5") || !strings.Contains(got, "2: line 2") || !strings.Contains(got, "8: line 8") {
		t.Errorf("snippet window wrong:\n%s", got)
	}
	if strings.Contains(got, "line 1\n") || strings.Contains(got, "line 9") {
		t.Errorf("snippet exceeds radius:\n%s", got)
	}

	// Clamp at file start.
	if got := loadSnippet(dir, "f.go", 1); !strings.Contains(got, "1: line 1") || strings.Contains(got, "line 5") {
		t.Errorf("start clamp wrong:\n%s", got)
	}
	// Missing file and out-of-range line degrade to "".
	if got := loadSnippet(dir, "missing.go", 3); got != "" {
		t.Errorf("missing file: %q, want empty", got)
	}
	if got := loadSnippet(dir, "f.go", 99); got != "" {
		t.Errorf("out-of-range line: %q, want empty", got)
	}
	if err := os.WriteFile(filepath.Join(base, "outside.go"), []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"../outside.go", "sub/../../outside.go", filepath.Join(base, "outside.go")} {
		if got := loadSnippet(dir, file, 1); got != "" {
			t.Errorf("escaping path %q: %q, want empty", file, got)
		}
	}
	if err := os.Symlink(base, filepath.Join(dir, "link-out")); err == nil {
		if got := loadSnippet(dir, "link-out/outside.go", 1); got != "" {
			t.Errorf("symlink escape: %q, want empty", got)
		}
	}

	// Pathological line is truncated.
	long := strings.Repeat("x", abstainedSnippetLineCap+50)
	if err := os.WriteFile(filepath.Join(dir, "long.go"), []byte(long+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := loadSnippet(dir, "long.go", 1); !strings.Contains(got, "…") || strings.Contains(got, long) {
		t.Errorf("long line not truncated:\n%s", got)
	}
}

func TestParseAbstainedResponse(t *testing.T) {
	t.Parallel()
	batch := []abstainedPair{{From: modA, To: modB}}

	t.Run("valid entry carries confidence", func(t *testing.T) {
		t.Parallel()
		text := "```json\n[" + abstainedDraftJSON(modA, modB, "contract", labels.ConfidenceHigh, "published interface") + "]\n```"
		got, err := parseAbstainedResponse(text, batch)
		if err != nil || len(got) != 1 {
			t.Fatalf("(%v, %v)", got, err)
		}
		d := got[0]
		if d.Status != labels.StatusDraft || d.Provenance != labels.ProvenanceLLM {
			t.Errorf("draft = %+v, want status draft, provenance llm", d)
		}
		if d.Confidence != labels.ConfidenceHigh {
			t.Errorf("confidence = %q, want self-reported high carried through", d.Confidence)
		}
		if d.Basis != initcfg.DraftBasisSemanticJudgment {
			t.Errorf("basis = %q, want %q", d.Basis, initcfg.DraftBasisSemanticJudgment)
		}
	})

	t.Run("unrequested pair skipped when requested pair is present", func(t *testing.T) {
		t.Parallel()
		text := `[` + abstainedDraftJSON(modA, modB, "contract", labels.ConfidenceHigh, "published interface") + `,
			{"from":"x","to":"y","strength":"model","confidence":"low","rationale":"hallucinated"}
		]`
		got, err := parseAbstainedResponse(text, batch)
		if err != nil || len(got) != 1 {
			t.Errorf("(%v, %v), want one requested draft and no error", got, err)
		}
	})

	// Schema violations are errors — they must trigger the retry path.
	for name, text := range map[string]string{
		"malformed body":     "the strength is model, trust me",
		"invalid strength":   `[{"from":"a","to":"b","strength":"mega","confidence":"low","rationale":"r"}]`,
		"symmetric rejected": `[{"from":"a","to":"b","strength":"symmetric","confidence":"low","rationale":"r"}]`,
		"invalid confidence": `[{"from":"a","to":"b","strength":"model","confidence":"certain","rationale":"r"}]`,
		"missing rationale":  `[{"from":"a","to":"b","strength":"model","confidence":"low","rationale":" "}]`,
		"missing requested":  `[{"from":"x","to":"y","strength":"model","confidence":"low","rationale":"hallucinated"}]`,
		"duplicate request":  `[` + abstainedDraftJSON(modA, modB, enrichModel, labels.ConfidenceLow, "r") + `,` + abstainedDraftJSON(modA, modB, enrichFunctional, labels.ConfidenceMedium, "r") + `]`,
		"missing basis":      `[{"from":"a","to":"b","strength":"model","confidence":"low","evidence_refs":[],"rationale":"r"}]`,
		"invalid basis":      `[{"from":"a","to":"b","strength":"model","confidence":"low","basis":"vibes","evidence_refs":[],"rationale":"r"}]`,
		"invalid ref":        fmt.Sprintf(`[{"from":"a","to":"b","strength":"model","confidence":"low","basis":%q,"evidence_refs":["bad ref"],"rationale":"r"}]`, initcfg.DraftBasisSemanticJudgment),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseAbstainedResponse(text, batch); err == nil {
				t.Error("want schema-validation error")
			}
		})
	}

	t.Run("unsupported ref", func(t *testing.T) {
		t.Parallel()
		text := fmt.Sprintf(`[{"from":"a","to":"b","strength":"model","confidence":"low","basis":%q,"evidence_refs":["api:missing"],"rationale":"r"}]`, initcfg.DraftBasisSemanticJudgment)
		if _, err := parseAbstainedResponse(text, batch, map[string]struct{}{enrichEvidenceAPIA: {}}); err == nil {
			t.Error("want unsupported-ref validation error")
		}
	})
}

// recordingProvider captures every request and replays canned responses.
type recordingProvider struct {
	responses []string
	requests  []llm.Request
}

func (p *recordingProvider) Name() string { return "test/recording" }
func (p *recordingProvider) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	p.requests = append(p.requests, req)
	if len(p.requests) > len(p.responses) {
		return llm.Response{}, fmt.Errorf("recordingProvider: no canned response for call %d", len(p.requests))
	}
	return llm.Response{Text: p.responses[len(p.requests)-1]}, nil
}

func TestRequestAbstainedBatch_RetriesOnceOnSchemaViolation(t *testing.T) {
	t.Parallel()
	batch := []abstainedPair{{From: modA, To: modB, EdgeCount: 1}}
	valid := `[` + abstainedDraftJSON(modA, modB, enrichFunctional, labels.ConfidenceMedium, "invokes behavior") + `]`

	t.Run("retry succeeds", func(t *testing.T) {
		t.Parallel()
		p := &recordingProvider{responses: []string{"not json at all", valid}}
		got, err := requestAbstainedBatch(context.Background(), p, "prompt", batch)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].Confidence != labels.ConfidenceMedium {
			t.Errorf("drafts = %+v", got)
		}
		if len(p.requests) != 2 {
			t.Fatalf("provider calls = %d, want 2 (one retry)", len(p.requests))
		}
		if !strings.Contains(p.requests[1].User, "rejected") {
			t.Errorf("retry turn must quote the violation back:\n%s", p.requests[1].User)
		}
	})

	t.Run("second failure fails the run", func(t *testing.T) {
		t.Parallel()
		p := &recordingProvider{responses: []string{"nope", "still nope"}}
		if _, err := requestAbstainedBatch(context.Background(), p, "prompt", batch); err == nil {
			t.Error("want error after failed retry")
		}
		if len(p.requests) != 2 {
			t.Errorf("provider calls = %d, want exactly 2 (no endless retries)", len(p.requests))
		}
	})
}

func TestDraftAbstainedLabels_Batches(t *testing.T) {
	t.Parallel()
	// abstainedBatchSize+1 pairs → two provider calls.
	pairs := make([]abstainedPair, abstainedBatchSize+1)
	for i := range pairs {
		pairs[i] = abstainedPair{From: fmt.Sprintf("m%02d", i), To: "z", EdgeCount: 1}
	}
	build := func(ps []abstainedPair) string {
		parts := make([]string, 0, len(ps))
		for _, p := range ps {
			parts = append(parts, abstainedDraftJSON(p.From, "z", enrichModel, labels.ConfidenceLow, "r"))
		}
		return "[" + strings.Join(parts, ",") + "]"
	}
	p := &recordingProvider{responses: []string{build(pairs[:abstainedBatchSize]), build(pairs[abstainedBatchSize:])}}

	got, err := draftAbstainedLabels(context.Background(), p, config.Config{}, pairs)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.requests) != 2 {
		t.Errorf("provider calls = %d, want 2 batches", len(p.requests))
	}
	if len(got) != len(pairs) {
		t.Errorf("drafts = %d, want %d", len(got), len(pairs))
	}
	if !strings.Contains(p.requests[0].System, "intrusive (strongest)") {
		t.Errorf("system prompt must carry the book level definitions:\n%s", p.requests[0].System)
	}
}

// writeAbstainedRepo builds a Go repo whose single cross-module edge is a
// blank (side-effect) import: go/packages resolves no used objects, so the
// edge has no type-info strength hint → classify abstains. aiBody appends the
// ai stanza (provider config).
func writeAbstainedRepo(t *testing.T, aiBody string) (cfgPath, dir string) {
	t.Helper()
	dir = t.TempDir()
	files := map[string]string{
		markerGoMod:       goModStub,
		filePkgAA:         "package a\n\nimport _ \"example.com/test/pkg/b\"\n\nfunc A() string { return \"a\" }\n",
		filePkgBB:         "package b\n\nfunc init() {}\n",
		defaultConfigPath: coupledModulesCfg + aiBody,
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	gitInitFixtureRepo(t, dir)
	return filepath.Join(dir, defaultConfigPath), dir
}

// TestRun_EnrichAbstained_E2E drives config enrich abstained end-to-end: real
// pipeline on a blank-import repo, LLM mocked at the system boundary (an
// OpenAI-compatible httptest server behind the ollama provider). Verifies the
// draft-not-pinned guarantee: the proposal lands as a draft with provenance
// llm and self-reported confidence, and a pre-existing approved label is
// untouched.
func TestRun_EnrichAbstained_E2E(t *testing.T) {
	t.Parallel()
	content := `[` + abstainedDraftJSON(modA, modB, enrichFunctional, labels.ConfidenceLow, "side-effect import registers b's init") + `]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "c1", "object": "chat.completion", "model": "m",
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": content},
				"finish_reason": "stop",
			}},
		})
	}))
	t.Cleanup(srv.Close)

	cfgPath, dir := writeAbstainedRepo(t, "ai:\n  provider: ollama\n  model: test-model\n  base_url: "+srv.URL+"\n")

	// Pre-seed an approved label for an unrelated pair — it must survive untouched.
	labelsPath := filepath.Join(dir, defaultLabelsPath)
	preSeeded := []labels.Label{{From: modB, To: modA, Strength: enrichModel, Status: labels.StatusApproved, Provenance: labels.ProvenanceHuman}}
	if err := writeLabels(labelsPath, preSeeded); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	code := Run([]string{cmdConfig, cmdEnrich, cmdAbstained, "-c", cfgPath, flagRefresh}, &buf)
	if code != 0 {
		t.Fatalf("exit = %d, want 0\noutput:\n%s", code, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "1 draft label(s) written") {
		t.Errorf("missing draft summary:\n%s", out)
	}

	got, err := labelsio.Load(labelsPath)
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]labels.Label{}
	for _, l := range got {
		byKey[labels.Key(l.From, l.To)] = l
	}
	draft, ok := byKey[labels.Key(modA, modB)]
	if !ok {
		t.Fatalf("no a->b draft written; labels = %+v", got)
	}
	if draft.Status != labels.StatusDraft {
		t.Errorf("status = %q — proposals must NEVER be auto-pinned", draft.Status)
	}
	if draft.Provenance != labels.ProvenanceLLM || draft.Confidence != labels.ConfidenceLow {
		t.Errorf("draft = %+v, want provenance llm with self-reported confidence low", draft)
	}
	if draft.Strength != enrichFunctional || draft.EvidenceHash == "" {
		t.Errorf("draft = %+v, want functional with a non-empty evidence hash", draft)
	}
	approvedKept, ok := byKey[labels.Key(modB, modA)]
	if !ok || approvedKept.Status != labels.StatusApproved || approvedKept.Strength != enrichModel {
		t.Errorf("pre-seeded approved label clobbered: %+v", approvedKept)
	}
}

// TestRun_EnrichAbstained_NothingToLabel: on a repo where every cross-module
// edge has a static strength signal (writeCoupledRepo's edge is intrusive via
// the internal glob), the pass reports nothing to label instead of inventing work.
func TestRun_EnrichAbstained_NothingToLabel(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, coupledModulesCfg+"ai:\n  provider: ollama\n  model: test-model\n  base_url: \"http://unused\"\n")

	var buf bytes.Buffer
	code := Run([]string{cmdConfig, cmdEnrich, cmdAbstained, "-c", cfgPath, flagRefresh}, &buf)
	if code != 0 {
		t.Fatalf("exit = %d, want 0\noutput:\n%s", code, buf.String())
	}
	if !strings.Contains(buf.String(), "nothing to label") {
		t.Errorf("want 'nothing to label', got:\n%s", buf.String())
	}
}

// TestRun_EnrichAbstained_LLMUnconfigured: without an ai stanza the command
// exits 3 with a setup hint before running the pipeline.
func TestRun_EnrichAbstained_LLMUnconfigured(t *testing.T) {
	t.Parallel()
	cfgPath, _ := writeAbstainedRepo(t, "")

	var buf bytes.Buffer
	if code := Run([]string{cmdConfig, cmdEnrich, cmdAbstained, "-c", cfgPath}, &buf); code != 3 {
		t.Errorf("exit = %d, want 3 (llm not configured)\noutput: %s", code, buf.String())
	}
}
