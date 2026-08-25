// enrich: the off-gate LLM workflow that drafts coupling-strength label
// refinements for human review. Lives in cmd by design — internal packages
// may never import the LLM layer (arch ring rule), so the gate stays
// deterministic while this command does the judgment-heavy work.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	apppipeline "github.com/alexei-led/archfit/internal/analysispipeline"
	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/llm"
)

// enrichBatchSize bounds how many module pairs go into one LLM request.
const enrichBatchSize = 30

type refinablePair = application.EnrichmentCandidatePair

// enrichFlags are the flags shared by every `config enrich` subcommand. Kong
// flattens the embedded struct, so each subcommand gets identical -c/--config,
// -r/--root, and --refresh flags.
type enrichFlags struct {
	Config  string `short:"c" name:"config" help:"Config file." default:".archfit.yaml"`
	Root    string `short:"r" name:"root" type:"path" help:"Repository root to analyze (default: directory of --config). Decouples the scanned repo from where the config lives."`
	Refresh bool   `name:"refresh" help:"Re-run all extractors and refresh the cache. Use after installing or updating analyzer tools."`

	// providerOverride is a test seam — set directly on the struct to inject a fake provider.
	providerOverride llm.Provider
}

// EnrichCmd groups the off-gate LLM annotation drafters. Each subcommand drafts
// one dimension into a review file; a human reviews and --apply writes approved
// entries into .archfit.yaml. Lives in cmd by design — internal packages may
// never import the LLM layer (arch ring rule), so the gate stays deterministic.
type EnrichCmd struct {
	Labels     EnrichLabelsCmd     `cmd:"" default:"withargs" help:"Draft coupling-strength labels (contract/functional/model/intrusive) for cross-module edges."`
	Abstained  EnrichAbstainedCmd  `cmd:"" help:"Draft coupling-strength labels for abstained (unknown-strength) cross-module edges, judged from code snippets."`
	Owner      EnrichOwnerCmd      `cmd:"" help:"Draft a module owner per module (uses CODEOWNERS context)."`
	Volatility EnrichVolatilityCmd `cmd:"" help:"Draft module volatility (low/medium/high)."`
	Subdomain  EnrichSubdomainCmd  `cmd:"" help:"Draft module subdomain (core/supporting/generic)."`
}

// EnrichLabelsCmd drafts coupling-strength label refinements into
// .archfit-labels.yaml (status: draft); the gate consumes approved entries.
type EnrichLabelsCmd struct {
	enrichFlags
}

func (c *EnrichLabelsCmd) Run(deps *appDeps) error {
	return c.runLabelEnrich(context.Background(), deps)
}

// EnrichOwnerCmd drafts a module owner per module; --apply writes approved entries.
type EnrichOwnerCmd struct {
	enrichFlags
	Apply      bool   `name:"apply" help:"Read approved entries from the draft file and write them into .archfit.yaml."`
	ReviewedBy string `name:"reviewed-by" help:"Human reviewer identity stamped on applied entries." default:""`
}

func (c *EnrichOwnerCmd) Run(deps *appDeps) error {
	if c.Apply {
		return c.runValuePin(context.Background(), deps, ownerSpec, c.ReviewedBy)
	}
	return c.runValueDraft(context.Background(), deps, ownerSpec)
}

// EnrichVolatilityCmd drafts module volatility; --apply writes approved entries.
type EnrichVolatilityCmd struct {
	enrichFlags
	Apply      bool   `name:"apply" help:"Read approved entries from the draft file and write them into .archfit.yaml."`
	ReviewedBy string `name:"reviewed-by" help:"Human reviewer identity stamped on applied entries." default:""`
}

func (c *EnrichVolatilityCmd) Run(deps *appDeps) error {
	if c.Apply {
		return c.runValuePin(context.Background(), deps, volatilitySpec, c.ReviewedBy)
	}
	return c.runValueDraft(context.Background(), deps, volatilitySpec)
}

// EnrichSubdomainCmd drafts module subdomain; --apply writes approved entries.
type EnrichSubdomainCmd struct {
	enrichFlags
	Apply      bool   `name:"apply" help:"Read approved entries from the draft file and write them into .archfit.yaml."`
	ReviewedBy string `name:"reviewed-by" help:"Human reviewer identity stamped on applied entries." default:""`
}

func (c *EnrichSubdomainCmd) Run(deps *appDeps) error {
	if c.Apply {
		return c.runSubdomainPin(context.Background(), deps, c.ReviewedBy)
	}
	return c.runSubdomainDraft(context.Background(), deps)
}

// runLabelEnrich is the original coupling-strength label draft workflow.
func (c *enrichFlags) runLabelEnrich(ctx context.Context, deps *appDeps) error {
	cfg, err := loadConfig(ctx, c.Config)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	llmCfg, configured := cfg.LLM()
	if !configured {
		return &exitError{code: 3, msg: "error: enrich needs ai configured (provider + model); see docs/guide/llm-enrich.md"}
	}
	// SCIP is no longer required. Without it, functional/model edges are selected
	// as before; additionally, unknown-strength cross-module edges are always
	// eligible — those are exactly the pairs where LLM judgment adds the most value.

	configDir := filepath.Dir(c.Config)
	cacheDir := llmCacheDir(configDir)
	provider, err := buildCachedProvider(c.providerOverride, llmCfg, cacheDir, c.Refresh)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v (set the key and re-run; see `archfit doctor`)", err)}
	}

	labelsPath := filepath.Join(configDir, defaultLabelsPath)

	deps.refresh = c.Refresh
	analyzer := newUseCaseAnalyzer(c.Config, c.Root, cfg, deps)
	root := scanRootForEvidence(configDir, c.Root)
	service := application.EnrichService{
		Preparer: analyzer, Analyzer: analyzer, Labels: enrichmentLabelStore(),
		Policy: apppipeline.EnrichmentPolicy{}, Judge: labelJudgeAdapter{provider: provider, cfg: cfg, configPath: c.Config, root: root},
	}
	out, err := service.Execute(ctx, application.EnrichmentRequest{ConfigPath: c.Config, Root: c.Root, Refresh: c.Refresh, LabelsPath: labelsPath})
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	if out.NoCandidates {
		_, _ = fmt.Fprintln(deps.Stdout, "enrich: no refinable module pairs (no heuristic functional/model cross-module edges, or all pairs already approved)")
		return nil
	}
	_, _ = fmt.Fprintf(deps.Stdout, "enrich: %d draft label(s) written to %s (%d approved entries kept)\n", out.Drafts, labelsPath, out.ApprovedKept)
	_, _ = fmt.Fprintln(deps.Stdout, "review each draft, set status: approved to pin it, delete to reject — the gate consumes approved labels only")
	return nil
}

func (c *enrichFlags) runSubdomainDraft(ctx context.Context, deps *appDeps) error {
	return runConfigEnrichWorkflow(ctx, c, deps, application.ConfigEnrichSubdomain, valueSpec{}, false, "")
}

func (c *enrichFlags) runSubdomainPin(ctx context.Context, deps *appDeps, reviewedBy string) error {
	return runConfigEnrichWorkflow(ctx, c, deps, application.ConfigEnrichSubdomain, valueSpec{}, true, reviewedBy)
}

// buildProvider constructs the configured LLM provider.
func buildProvider(c config.LLMConfig) (llm.Provider, error) {
	switch c.Provider {
	case providerAnthropic:
		return llm.NewAnthropic(c.Model)
	case providerOpenAI:
		return llm.NewOpenAI(c.Model)
	case providerOllama:
		return llm.NewOllama(c.BaseURL, c.Model), nil
	default: // unreachable: config validation rejects unknown providers
		return nil, fmt.Errorf("unknown llm provider %q", c.Provider)
	}
}

// buildCachedProvider constructs a provider and wraps it in a disk-backed
// response cache rooted at cacheDir. refresh bypasses cache reads but still
// stores the fresh response. The override seam (used in tests) bypasses both
// provider construction and caching.
func buildCachedProvider(override llm.Provider, cfg config.LLMConfig, cacheDir string, refresh bool) (llm.Provider, error) {
	if override != nil {
		return override, nil
	}
	p, err := buildProvider(cfg)
	if err != nil {
		return nil, err
	}
	cache := llm.NewCache(p, cacheDir)
	cache.RefreshMode = refresh
	return cache, nil
}

// llmCacheDir returns the on-disk LLM response cache directory under baseDir.
// One definition of the ".archfit-cache/llm" layout shared by every LLM command
// (config init/update/enrich, analyze --ai-summary, explain --ai-summary) and reported by doctor.
func llmCacheDir(baseDir string) string {
	return filepath.Join(baseDir, ".archfit-cache", "llm")
}

// factsCacheDir returns the extractor fact-cache directory under baseDir —
// facts/ beside llm/ in the same .archfit-cache root (fact-cache.md D1), so
// "delete .archfit-cache to reset" stays the single troubleshooting answer.
func factsCacheDir(baseDir string) string {
	return filepath.Join(baseDir, ".archfit-cache", "facts")
}

// enrichSystemPrompt frames the Balanced Coupling refinement task.
const enrichSystemPrompt = `You are an architecture reviewer applying Vlad Khononov's Balanced Coupling model.
For each module pair below, the tool's deterministic heuristic labeled the dependency's integration strength from call edges — usually a blanket "functional". Refine it:
- "model": the consumer depends on the producer's data structures / domain model (types cross the boundary).
- "functional": the consumer invokes the producer's behavior (calls functions, no deep type dependence).
- "contract": the consumer depends only on a deliberately published, stable interface.
- "intrusive": the consumer reaches into internals, private state, or implementation details.
Use the repository evidence IDs, module names, subdomain/volatility context, and sample dependency paths. Respect intended centralization: shared infrastructure or config hubs are not automatically intrusive. Put cited repository evidence IDs in evidence_refs. Use an empty evidence_refs array when the judgment rests only on sample dependency paths.
Respond with a STRICT JSON array only — no prose, no markdown fences. One object per pair:
[{"from":"<module>","to":"<module>","strength":"model|functional|contract|intrusive","basis":"semantic_judgment","evidence_refs":["doc:<path>"],"rationale":"<one sentence>"}]
Include every pair exactly once.`

// draftLabels asks the provider to refine each batch of pairs and parses the
// strict-JSON responses into draft labels.
func draftLabels(ctx context.Context, p llm.Provider, cfg config.Config, pairs []refinablePair, repoEvidence ...[]string) ([]application.EnrichmentLabel, error) {
	evidence := optionalEvidence(repoEvidence)
	var out []application.EnrichmentLabel
	for start := 0; start < len(pairs); start += enrichBatchSize {
		batch := pairs[start:min(start+enrichBatchSize, len(pairs))]
		resp, err := p.Complete(ctx, llm.Request{
			System: enrichSystemPrompt,
			User:   enrichUserPrompt(cfg, batch, evidence),
		})
		if err != nil {
			return nil, err
		}
		drafts, err := parseEnrichResponse(resp.Text, batch, evidenceRefSet(evidence))
		if err != nil {
			return nil, err
		}
		out = append(out, drafts...)
	}
	return out, nil
}

// enrichUserPrompt renders one batch of pairs as the user turn.
func enrichUserPrompt(cfg config.Config, batch []refinablePair, repoEvidence []string) string {
	var b strings.Builder
	if len(repoEvidence) > 0 {
		b.WriteString(repositoryEvidenceHeader + "\n")
		for _, ev := range repoEvidence {
			fmt.Fprintf(&b, "- %s\n", ev)
		}
		b.WriteString("\n")
	}
	b.WriteString("Module pairs to refine:\n")
	for _, p := range batch {
		fmt.Fprintf(&b, "\n- from: %s%s\n  to: %s%s\n  heuristic_strength: %s\n  edge_count: %d\n  sample_dependencies:\n",
			p.From, moduleContext(cfg, p.From), p.To, moduleContext(cfg, p.To), p.Strength, p.EdgeCount)
		for _, s := range p.SamplePaths {
			fmt.Fprintf(&b, "    - %s\n", s)
		}
	}
	return b.String()
}

// moduleContext renders the human-authored subdomain/volatility annotations.
func moduleContext(cfg config.Config, module string) string {
	def, ok := cfg.Modules[module]
	if !ok {
		return ""
	}
	parts := []string{}
	if def.Subdomain != "" {
		parts = append(parts, "subdomain="+def.Subdomain)
	}
	if def.Volatility != "" {
		parts = append(parts, "volatility="+def.Volatility)
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

// parseEnrichResponse strictly parses the model's JSON and keeps only entries
// matching requested pairs with valid strengths. A malformed body is an error
// (never write a half-understood draft file); unknown pairs/strengths in an
// otherwise-valid body are skipped.
func parseEnrichResponse(text string, batch []refinablePair, allowedRefs ...map[string]struct{}) ([]application.EnrichmentLabel, error) {
	text = trimJSONFences(text)

	var entries []application.EnrichmentLabelResponse
	if err := json.Unmarshal([]byte(text), &entries); err != nil {
		return nil, fmt.Errorf("enrich: model response is not the required JSON array: %w", err)
	}

	requested := make(map[string]struct{}, len(batch))
	for _, p := range batch {
		requested[application.EnrichmentPairKey(p.From, p.To)] = struct{}{}
	}

	out := make([]application.EnrichmentLabel, 0, len(entries))
	knownRefs := firstAllowedEvidenceRefs(allowedRefs)
	for _, e := range entries {
		key := application.EnrichmentPairKey(e.From, e.To)
		if _, ok := requested[key]; !ok {
			continue
		}
		if !application.ValidEnrichmentStrength(e.Strength) {
			continue
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
			Confidence:   application.EnrichmentLabelConfidenceMedium,
		})
	}
	return out, nil
}

func labelDraftMetadata(scope, name, basis string, refs []string, allowedRefs map[string]struct{}) (string, []string, error) {
	basis, refs, err := draftMetadata(scope, name, basis, refs, false, allowedRefs)
	if err != nil {
		return "", nil, err
	}
	if basis == "" {
		return "", nil, fmt.Errorf("%s %q missing basis", scope, name)
	}
	return basis, refs, nil
}
