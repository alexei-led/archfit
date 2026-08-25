// enrich_abstained: the abstained-edge pass of the off-gate LLM label
// workflow. Where `config enrich` refines heuristic functional/model labels,
// this pass targets edges the deterministic pipeline ABSTAINED on (unknown
// strength — no type info, no SCIP, no config glob) and asks the LLM to judge
// them from code snippets. Proposals are always DRAFT labels (provenance: llm,
// self-reported confidence carried through); a human flips status: approved to
// pin — the gate never consumes drafts.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/llm"
)

const (
	// abstainedEdgeCap bounds how many abstained edges one run proposes labels
	// for. Edges beyond the cap are counted and disclosed, never silently dropped.
	abstainedEdgeCap = 100
	// abstainedBatchSize bounds module pairs per LLM request — smaller than
	// enrichBatchSize because each pair carries code snippets.
	abstainedBatchSize = 10
	// abstainedSampleLocs is the max sample usage locations included per pair.
	abstainedSampleLocs = 3
	// abstainedSnippetRadius is the number of source lines kept on each side of
	// a sample location.
	abstainedSnippetRadius = 3
	// abstainedSnippetLineCap truncates pathological source lines (minified
	// bundles) so one line cannot blow the prompt budget.
	abstainedSnippetLineCap = 200
)

// EnrichAbstainedCmd drafts coupling-strength labels for abstained
// (unknown-strength) cross-module edges into .archfit-labels.yaml.
type EnrichAbstainedCmd struct {
	enrichFlags
}

func (c *EnrichAbstainedCmd) Run(deps *appDeps) error {
	return c.runAbstainedEnrich(context.Background(), deps)
}

func (c *EnrichAbstainedCmd) runAbstainedEnrich(ctx context.Context, deps *appDeps) error {
	cfg, err := loadConfig(ctx, c.Config)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	llmCfg, configured := cfg.LLM()
	if !configured {
		return &exitError{code: 3, msg: "error: config enrich abstained needs ai configured (provider + model); see docs/guide/llm-enrich.md"}
	}

	configDir := filepath.Dir(c.Config)
	provider, err := buildCachedProvider(c.providerOverride, llmCfg, llmCacheDir(configDir), c.Refresh)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v (set the key and re-run; see `archfit doctor`)", err)}
	}

	labelsPath := filepath.Join(configDir, defaultLabelsPath)
	deps.refresh = c.Refresh
	snippetRoot := c.Root
	if snippetRoot == "" {
		snippetRoot = configDir
	}
	service := application.EnrichService{
		Stages: newAnalysisStages(c.Config, c.Root, cfg, deps), Labels: enrichmentLabelStore(),
		Judge:    abstainedJudgeAdapter{provider: provider, cfg: cfg, configPath: c.Config, root: scanRootForEvidence(configDir, c.Root)},
		Snippets: filesystemSnippetAdapter{},
	}
	out, err := service.Execute(ctx, application.EnrichmentRequest{ConfigPath: c.Config, Root: c.Root, SnippetRoot: snippetRoot, Refresh: c.Refresh, LabelsPath: labelsPath, Abstained: true, EdgeCap: abstainedEdgeCap, SampleCap: abstainedSampleLocs})
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	if out.NoCandidates {
		_, _ = fmt.Fprintln(deps.Stdout, "enrich abstained: no abstained cross-module edges — nothing to label")
		return nil
	}
	included := out.SelectedEdges
	if out.TotalEdges > included {
		_, _ = fmt.Fprintf(deps.Stdout, "enrich abstained: %d abstained edge(s) found; labeling the first %d (cap %d) — re-run after review to cover the rest\n", out.TotalEdges, included, abstainedEdgeCap)
	} else {
		_, _ = fmt.Fprintf(deps.Stdout, "enrich abstained: %d abstained edge(s) across %d module pair(s)\n", out.TotalEdges, out.Candidates)
	}
	_, _ = fmt.Fprintf(deps.Stdout, "enrich abstained: %d draft label(s) written to %s (%d approved entries kept)\n", out.Drafts, labelsPath, out.ApprovedKept)
	_, _ = fmt.Fprintln(deps.Stdout, "review each draft, set status: approved to pin it, delete to reject — the gate consumes approved labels only")
	return nil
}

// abstainedSample is one usage location of an abstained edge, with source context.
type abstainedSample struct {
	FromPath, ToPath string // edge endpoints (node paths)
	File             string // repo-relative location file; "" when the edge has no location
	Line             int
	Snippet          string // numbered source lines around Line; "" when unreadable
}

// abstainedPair aggregates the abstained edges between one ordered module pair.
type abstainedPair struct {
	From, To  string
	EdgeCount int // edges included this run (capped)
	Samples   []abstainedSample
}

// loadSnippet returns numbered source lines within abstainedSnippetRadius of
// line (1-based). Unreadable files and out-of-range lines return "".
func loadSnippet(root, file string, line int) string {
	clean := path.Clean(file)
	if strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") {
		return ""
	}
	osRel := filepath.FromSlash(clean)
	if !filepath.IsLocal(osRel) {
		return ""
	}
	full := filepath.Join(root, osRel)
	info, err := os.Lstat(full) //#nosec G304 -- repo-relative path from the analyzed tree
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return ""
	}
	rootEval, err := filepath.EvalSymlinks(root)
	if err != nil {
		return ""
	}
	fullEval, err := filepath.EvalSymlinks(full)
	if err != nil {
		return ""
	}
	rel, err := filepath.Rel(rootEval, fullEval)
	if err != nil || !filepath.IsLocal(rel) {
		return ""
	}
	data, err := os.ReadFile(full) //#nosec G304 -- repo-relative path from the analyzed tree
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	if line < 1 || line > len(lines) {
		return ""
	}
	start := max(line-abstainedSnippetRadius, 1)
	end := min(line+abstainedSnippetRadius, len(lines))
	var b strings.Builder
	for n := start; n <= end; n++ {
		text := lines[n-1]
		if len(text) > abstainedSnippetLineCap {
			text = text[:abstainedSnippetLineCap] + "…"
		}
		fmt.Fprintf(&b, "%4d: %s\n", n, text)
	}
	return b.String()
}

// abstainedSystemPrompt frames the abstained-edge judgment with the book's
// integration-strength level definitions quoted verbatim (Khononov, "Balancing
// Coupling in Software Design", Ch. 6) so the model anchors on the published
// model, not its own notion of coupling.
const abstainedSystemPrompt = `You are an architecture reviewer applying Vlad Khononov's Balanced Coupling model ("Balancing Coupling in Software Design").
Each module pair below has ABSTAINED edges: cross-module dependencies where no static signal (compiler type info, SCIP index, config glob) determined the integration strength. Classify each pair's integration strength using the book's four levels, defined verbatim:

- intrusive (strongest): "Occurs when private interfaces or implementation details are used for integration — internal databases, private objects, undocumented APIs. We must assume all knowledge about a component's implementation is shared."
- functional: "Occurs when multiple components share knowledge of their business requirements and must change together when those requirements evolve."
- model: "Occurs when components share knowledge of a business domain model. If the model changes — due to new domain insights — all coupled components must change accordingly."
- contract (weakest): "An integration contract encapsulates implementation details, functional requirements, and business models, making integration explicit and stable."

Judge from the repository evidence IDs, module names, edge endpoints, and code snippets. Report confidence honestly: "high" only when the snippets clearly show the coupling kind; "low" when the evidence is thin. Never invent certainty. Put cited repository evidence IDs in evidence_refs. Use an empty evidence_refs array when the judgment rests only on endpoint snippets.
Respond with a STRICT JSON array only — no prose, no markdown fences. One object per pair:
[{"from":"<module>","to":"<module>","strength":"contract|model|functional|intrusive","confidence":"high|medium|low","basis":"semantic_judgment","evidence_refs":["doc:<path>"],"rationale":"<one sentence>"}]
Include every pair exactly once.`

// abstainedUserPrompt renders one batch of pairs with their sample locations
// and code snippets as the user turn.
func abstainedUserPrompt(cfg config.Config, batch []abstainedPair, repoEvidence []string) string {
	var b strings.Builder
	if len(repoEvidence) > 0 {
		b.WriteString(repositoryEvidenceHeader + "\n")
		for _, ev := range repoEvidence {
			fmt.Fprintf(&b, "- %s\n", ev)
		}
		b.WriteString("\n")
	}
	b.WriteString("Module pairs with abstained edges:\n")
	for _, p := range batch {
		fmt.Fprintf(&b, "\n- from: %s%s\n  to: %s%s\n  abstained_edges: %d\n  samples:\n",
			p.From, moduleContext(cfg, p.From), p.To, moduleContext(cfg, p.To), p.EdgeCount)
		for _, s := range p.Samples {
			fmt.Fprintf(&b, "  - %s -> %s", s.FromPath, s.ToPath)
			if s.File != "" {
				fmt.Fprintf(&b, " (%s:%d)", s.File, s.Line)
			}
			b.WriteString("\n")
			if s.Snippet != "" {
				b.WriteString("    code:\n")
				for line := range strings.SplitSeq(strings.TrimRight(s.Snippet, "\n"), "\n") {
					b.WriteString("    " + line + "\n")
				}
			}
		}
	}
	return b.String()
}

// abstainedStrengths are the four book levels this pass may propose. Symmetric
// is deliberately absent — it comes from clone evidence only, never judgment.
var abstainedStrengths = map[string]struct{}{
	llmStrengthContract: {}, "model": {}, "functional": {}, "intrusive": {},
}

// abstainedConfidences are the self-reported confidence values carried into drafts.
var abstainedConfidences = map[string]struct{}{
	application.EnrichmentLabelConfidenceHigh: {}, application.EnrichmentLabelConfidenceMedium: {}, application.EnrichmentLabelConfidenceLow: {},
}

// parseAbstainedResponse validates the model's JSON against the required
// schema. Unlike the refinement pass, a schema violation (invalid strength or
// confidence, missing rationale) is an ERROR, not a silent skip — the caller
// retries the batch once with the violation quoted back. Entries for pairs
// that were never requested are hallucinations and are dropped without error,
// but every requested pair must appear exactly once.
func parseAbstainedResponse(text string, batch []abstainedPair, allowedRefs ...map[string]struct{}) ([]application.EnrichmentLabel, error) {
	text = trimJSONFences(text)

	var entries []application.EnrichmentLabelResponse
	if err := json.Unmarshal([]byte(text), &entries); err != nil {
		return nil, fmt.Errorf("response is not the required JSON array: %w", err)
	}

	requested := make(map[string]struct{}, len(batch))
	for _, p := range batch {
		requested[application.EnrichmentPairKey(p.From, p.To)] = struct{}{}
	}

	seen := make(map[string]struct{}, len(batch))
	out := make([]application.EnrichmentLabel, 0, len(entries))
	knownRefs := firstAllowedEvidenceRefs(allowedRefs)
	for _, e := range entries {
		key := application.EnrichmentPairKey(e.From, e.To)
		if _, ok := requested[key]; !ok {
			continue
		}
		if _, dup := seen[key]; dup {
			return nil, fmt.Errorf("entry %s->%s: duplicate response for requested pair", e.From, e.To)
		}
		seen[key] = struct{}{}
		if _, ok := abstainedStrengths[e.Strength]; !ok {
			return nil, fmt.Errorf("entry %s->%s: strength %q is not one of contract|model|functional|intrusive", e.From, e.To, e.Strength)
		}
		if _, ok := abstainedConfidences[e.Confidence]; !ok {
			return nil, fmt.Errorf("entry %s->%s: confidence %q is not one of high|medium|low", e.From, e.To, e.Confidence)
		}
		if strings.TrimSpace(e.Rationale) == "" {
			return nil, fmt.Errorf("entry %s->%s: rationale is required", e.From, e.To)
		}
		basis, refs, err := labelDraftMetadata("label draft", e.From+"->"+e.To, e.Basis, e.EvidenceRefs, knownRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, application.EnrichmentLabel{
			From:         e.From,
			To:           e.To,
			Strength:     e.Strength,
			Rationale:    e.Rationale,
			EvidenceRefs: refs,
			Basis:        basis,
			Status:       application.EnrichmentLabelStatusDraft,
			Provenance:   application.EnrichmentLabelProvenanceLLM,
			Confidence:   e.Confidence,
		})
	}
	for _, p := range batch {
		if _, ok := seen[application.EnrichmentPairKey(p.From, p.To)]; !ok {
			return nil, fmt.Errorf("missing response for requested pair %s->%s", p.From, p.To)
		}
	}
	return out, nil
}

// draftAbstainedLabels asks the provider to judge each batch of abstained
// pairs. A response that fails schema validation is retried once with the
// violation quoted back; a second failure fails the run — a half-understood
// draft file is never written.
func draftAbstainedLabels(ctx context.Context, p llm.Provider, cfg config.Config, pairs []abstainedPair, repoEvidence ...[]string) ([]application.EnrichmentLabel, error) {
	evidence := optionalEvidence(repoEvidence)
	var out []application.EnrichmentLabel
	for start := 0; start < len(pairs); start += abstainedBatchSize {
		batch := pairs[start:min(start+abstainedBatchSize, len(pairs))]
		drafts, err := requestAbstainedBatch(ctx, p, abstainedUserPrompt(cfg, batch, evidence), batch, evidenceRefSet(evidence))
		if err != nil {
			return nil, err
		}
		out = append(out, drafts...)
	}
	return out, nil
}

// requestAbstainedBatch executes one batch request with a single
// schema-validation retry.
func requestAbstainedBatch(ctx context.Context, p llm.Provider, user string, batch []abstainedPair, allowedRefs ...map[string]struct{}) ([]application.EnrichmentLabel, error) {
	resp, err := p.Complete(ctx, llm.Request{System: abstainedSystemPrompt, User: user})
	if err != nil {
		return nil, err
	}
	drafts, perr := parseAbstainedResponse(resp.Text, batch, allowedRefs...)
	if perr == nil {
		return drafts, nil
	}
	retryUser := user + "\n\nYour previous response was rejected: " + perr.Error() +
		"\nRespond again with ONLY the strict JSON array in the required schema."
	resp, err = p.Complete(ctx, llm.Request{System: abstainedSystemPrompt, User: retryUser})
	if err != nil {
		return nil, err
	}
	drafts, perr = parseAbstainedResponse(resp.Text, batch, allowedRefs...)
	if perr != nil {
		return nil, fmt.Errorf("enrich abstained: response failed schema validation after retry: %w", perr)
	}
	return drafts, nil
}

// trimJSONFences tolerates accidental markdown fencing around a JSON body,
// nothing else.
func trimJSONFences(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	return strings.TrimSpace(text)
}
