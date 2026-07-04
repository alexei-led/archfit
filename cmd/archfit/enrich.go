// enrich: the off-gate LLM workflow that drafts coupling-strength label
// refinements for human review. Lives in cmd by design — internal packages
// may never import the LLM layer (arch ring rule), so the gate stays
// deterministic while this command does the judgment-heavy work.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/engine"
	"github.com/alexei-led/archfit/internal/initcfg"
	"github.com/alexei-led/archfit/internal/labels"
	"github.com/alexei-led/archfit/internal/labels/labelsio"
	"github.com/alexei-led/archfit/internal/llm"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/signal"

	"github.com/goccy/go-yaml"
)

// enrichBatchSize bounds how many module pairs go into one LLM request.
const enrichBatchSize = 30

// enrichFlags are the flags shared by every `config enrich` subcommand. Kong
// flattens the embedded struct, so each subcommand gets identical -c/--config,
// -r/--root, and --no-cache flags.
type enrichFlags struct {
	Config  string `short:"c" name:"config" help:"Config file." default:".archfit.yaml"`
	Root    string `short:"r" name:"root" type:"path" help:"Repository root to analyze (default: directory of --config). Decouples the scanned repo from where the config lives."`
	NoCache bool   `name:"no-cache" help:"Bypass archfit caches (extractor facts and LLM responses)."`

	// providerOverride is a test seam — set directly on the struct to inject a fake provider.
	providerOverride llm.Provider
}

// EnrichCmd groups the off-gate LLM annotation drafters. Each subcommand drafts
// one dimension into a review file; a human reviews and --apply writes approved
// entries into .archfit.yaml. Lives in cmd by design — internal packages may
// never import the LLM layer (arch ring rule), so the gate stays deterministic.
type EnrichCmd struct {
	Labels     EnrichLabelsCmd     `cmd:"" default:"withargs" help:"Draft coupling-strength labels (contract/functional/model/intrusive) for cross-module edges."`
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

// captureMetric records the common pipeline evidence (graph, classifications) so
// enrich can reuse it without re-implementing stages. It needs only CommonInput,
// so it implements the uniform Metric directly and projects to s.Common.
type captureMetric struct{ in *signal.CommonInput }

func (m *captureMetric) Name() string    { return "enrich_capture" }
func (m *captureMetric) Version() string { return "enrich_capture.v0" }
func (m *captureMetric) Calculate(in signal.CollectedSignals) diagnostic.MetricResult {
	*m.in = in.Common
	return diagnostic.MetricResult{Name: m.Name(), Band: "info", Display: "internal capture"}
}

// runLabelEnrich is the original coupling-strength label draft workflow.
func (c *enrichFlags) runLabelEnrich(ctx context.Context, deps *appDeps) error {
	cfg, err := loadConfig(ctx, c.Config, false)
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
	provider, err := buildCachedProvider(c.providerOverride, llmCfg, cacheDir, c.NoCache)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v (set the key and re-run; see `archfit doctor`)", err)}
	}

	labelsPath := filepath.Join(configDir, defaultLabelsPath)
	existing, err := labelsio.Load(labelsPath)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	// Run the standard pipeline once, capturing the evidence the metrics saw.
	var captured signal.CommonInput
	base, err := baseline.Load(ctx, filepath.Join(configDir, defaultBaselinePath))
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	deps.noCache = c.NoCache
	if _, _, err := runPipeline(ctx, deps, cfg, c.Config, c.Root, false, engine.Mode{Full: true}, base, &captureMetric{in: &captured}); err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	mm := cfg.ForClassify().ModuleMap
	pairs := selectRefinablePairs(captured.Graph, captured.Classifications, mm, existing)
	if len(pairs) == 0 {
		_, _ = fmt.Fprintln(deps.Stdout, "enrich: no refinable module pairs (no heuristic functional/model cross-module edges, or all pairs already approved)")
		return nil
	}

	drafts, err := draftLabels(ctx, provider, cfg, pairs)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	// Stamp drafts with the evidence hash the engine will verify later.
	wanted := make(map[string]struct{}, len(drafts))
	for _, d := range drafts {
		wanted[labels.Key(d.From, d.To)] = struct{}{}
	}
	evidence := engine.PairEvidence(captured.Graph, mm, wanted)
	for i := range drafts {
		drafts[i].EvidenceHash = evidence[labels.Key(drafts[i].From, drafts[i].To)]
	}

	merged := mergeDrafts(existing, drafts)
	if err := writeLabels(labelsPath, merged); err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	approvedKept := 0
	for _, l := range merged {
		if l.Status == labels.StatusApproved {
			approvedKept++
		}
	}
	_, _ = fmt.Fprintf(deps.Stdout, "enrich: %d draft label(s) written to %s (%d approved entries kept)\n", len(drafts), labelsPath, approvedKept)
	_, _ = fmt.Fprintln(deps.Stdout, "review each draft, set status: approved to pin it, delete to reject — the gate consumes approved labels only")
	return nil
}

// runSubdomainDraft calls the LLM to classify unclassified modules and writes
// draft entries into .archfit-subdomains.yaml for human review.
func (c *enrichFlags) runSubdomainDraft(ctx context.Context, deps *appDeps) error {
	cfg, err := loadConfig(ctx, c.Config, false)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	llmCfg, configured := cfg.LLM()
	if !configured {
		return &exitError{code: 3, msg: "error: config enrich subdomain needs ai configured (provider + model); see docs/guide/llm-enrich.md"}
	}

	configDir := filepath.Dir(c.Config)
	cacheDir := llmCacheDir(configDir)
	provider, err := buildCachedProvider(c.providerOverride, llmCfg, cacheDir, c.NoCache)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v (set the key and re-run; see `archfit doctor`)", err)}
	}

	// Collect modules without a subdomain set yet.
	var toClassify []initcfg.ModuleDef
	for name, mod := range cfg.Modules {
		if mod.Subdomain != "" {
			continue
		}
		toClassify = append(toClassify, initcfg.ModuleDef{
			Name:  name,
			Paths: mod.Paths,
		})
	}
	// Sort for determinism.
	sort.Slice(toClassify, func(i, j int) bool { return toClassify[i].Name < toClassify[j].Name })

	if len(toClassify) == 0 {
		_, _ = fmt.Fprintln(deps.Stdout, "config enrich subdomain: all modules already have subdomain set — nothing to draft")
		return nil
	}

	root := c.Root
	if root == "" {
		root = configDir
	}
	targets := initcfg.BuildClassifyTargets(root, toClassify)
	ann, err := classifyModules(ctx, provider, targets, cfg.Layers)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: classify failed: %v", err)}
	}
	warnPartialClassify(deps.Stdout, targets, ann)

	// Convert annotations to draft entries.
	var newDrafts []initcfg.SubdomainDraft
	for _, t := range targets {
		a, ok := ann[t.Name]
		if !ok || a.Subdomain == "" {
			continue
		}
		newDrafts = append(newDrafts, initcfg.SubdomainDraft{
			Module:     t.Name,
			Subdomain:  a.Subdomain,
			Volatility: a.Volatility,
			Rationale:  "", // classifyModules doesn't surface rationale separately
			Status:     initcfg.SubdomainStatusDraft,
		})
	}

	subdomainsPath := filepath.Join(configDir, defaultSubdomainsPath)
	existing, err := initcfg.LoadSubdomainDrafts(subdomainsPath)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	merged := initcfg.MergeSubdomainDrafts(existing, newDrafts)
	if err := initcfg.WriteSubdomainDrafts(subdomainsPath, merged); err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	_, _ = fmt.Fprintf(deps.Stdout,
		"enrich: %d draft subdomain(s) written to %s — review, set status: approved, then run config enrich subdomain --apply\n",
		len(newDrafts), subdomainsPath)
	return nil
}

// runSubdomainPin reads approved entries from .archfit-subdomains.yaml and
// applies them into .archfit.yaml.
func (c *enrichFlags) runSubdomainPin(ctx context.Context, deps *appDeps, reviewedBy string) error {
	configDir := filepath.Dir(c.Config)
	subdomainsPath := filepath.Join(configDir, defaultSubdomainsPath)

	draftFile, err := initcfg.LoadSubdomainDrafts(subdomainsPath)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	var approved []initcfg.SubdomainDraft
	for _, d := range draftFile.Drafts {
		if d.Status == initcfg.SubdomainStatusApproved {
			approved = append(approved, d)
		}
	}
	if len(approved) == 0 {
		_, _ = fmt.Fprintln(deps.Stdout, "no approved subdomain drafts found — set status: approved in "+subdomainsPath+" and re-run")
		return nil
	}

	src, err := os.ReadFile(c.Config) //#nosec G304
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: reading config: %v", err)}
	}

	cfg, err := loadConfig(ctx, c.Config, false)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	// Build current subdomain map.
	currentSubdomains := make(map[string]string, len(cfg.Modules))
	for name, mod := range cfg.Modules {
		currentSubdomains[name] = mod.Subdomain
	}

	// Build pins from approved drafts.
	if reviewedBy == "" {
		reviewedBy = "config enrich subdomain"
	}
	reviewedAt := time.Now().UTC()
	pins := make([]initcfg.SubdomainPin, 0, len(approved))
	for _, d := range approved {
		pins = append(pins, initcfg.SubdomainPin{
			Module:     d.Module,
			Subdomain:  d.Subdomain,
			Volatility: d.Volatility,
			ReviewedAt: reviewedAt,
			ReviewedBy: reviewedBy,
		})
	}

	edited, patched, err := initcfg.PinSubdomains(src, currentSubdomains, pins)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: pin subdomains: %v", err)}
	}
	if patched == 0 {
		_, _ = fmt.Fprintln(deps.Stdout, "no changes — all approved modules already have subdomain set")
		return nil
	}

	if err := safeWriteConfig(ctx, deps, c.Config, edited, src); err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	_, _ = fmt.Fprintf(deps.Stdout, "pinned %d subdomain(s) into %s (reviewed_by: %s)\n", patched, c.Config, reviewedBy)
	return nil
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

// buildCachedProvider constructs a provider and, unless noCache is true, wraps
// it in a disk-backed response cache rooted at cacheDir. The override seam
// (used in tests) bypasses both provider construction and caching.
func buildCachedProvider(override llm.Provider, cfg config.LLMConfig, cacheDir string, noCache bool) (llm.Provider, error) {
	if override != nil {
		return override, nil
	}
	p, err := buildProvider(cfg)
	if err != nil {
		return nil, err
	}
	if !noCache {
		p = llm.NewCache(p, cacheDir)
	}
	return p, nil
}

// llmCacheDir returns the on-disk LLM response cache directory under baseDir.
// One definition of the ".archfit-cache/llm" layout shared by every LLM command
// (enrich, review, explain, autopilot, init, update) and reported by doctor.
func llmCacheDir(baseDir string) string {
	return filepath.Join(baseDir, ".archfit-cache", "llm")
}

// factsCacheDir returns the extractor fact-cache directory under baseDir —
// facts/ beside llm/ in the same .archfit-cache root (fact-cache.md D1), so
// "delete .archfit-cache to reset" stays the single troubleshooting answer.
func factsCacheDir(baseDir string) string {
	return filepath.Join(baseDir, ".archfit-cache", "facts")
}

// baseWorktreesDir returns the --base worktree parent directory under baseDir —
// worktrees/ beside facts/ and llm/ in the same .archfit-cache root. Each
// `analyze --base <ref>` checks the base ref out under worktrees/<sha>, a
// deterministic path, so the per-checkout fact-cache keys (which fold the scan
// root in because cached subprocess output embeds absolute paths) hit on a
// repeat run against the same ref. The checkout is removed after each run;
// only the fact blobs persist.
func baseWorktreesDir(baseDir string) string {
	return filepath.Join(baseDir, ".archfit-cache", "worktrees")
}

// refinablePair is one candidate module pair with its evidence summary.
type refinablePair struct {
	From, To    string
	Strength    string // current heuristic strength (functional or model)
	EdgeCount   int
	SamplePaths []string // up to 5 "fromPath -> toPath" examples
}

// selectRefinablePairs picks cross-module pairs that benefit from LLM judgment:
//   - functional or model strength (heuristic blanket-labelled call edges), or
//   - unknown strength (no heuristic available — LLM judgment needed most here).
//
// Contract and intrusive strengths are already decided (glob or SCIP); they are
// excluded. Already-approved pairs are skipped. Deterministic order (From, To).
func selectRefinablePairs(g *graph.Graph, idx coupling.Index, mm config.ModuleMap, existing []labels.Label) []refinablePair {
	if g == nil {
		return nil
	}
	approved := make(map[string]struct{})
	for _, l := range existing {
		if l.Status == labels.StatusApproved {
			approved[labels.Key(l.From, l.To)] = struct{}{}
		}
	}

	type agg struct {
		strength string
		count    int
		samples  []string
	}
	pairs := map[string]*agg{}
	for _, e := range g.Edges() {
		cl, ok := idx[e.From+"\x00"+e.To+"\x00"+string(e.Kind)]
		if !ok {
			continue
		}
		// Include functional/model (heuristic) and unknown (no heuristic — needs judgment).
		// Contract and intrusive are already decided; skip them.
		if cl.Strength != coupling.StrengthFunctional &&
			cl.Strength != coupling.StrengthModel &&
			cl.Strength != coupling.StrengthUnknown {
			continue
		}
		if cl.Distance == coupling.DistanceSameModule || cl.Distance == coupling.DistanceUnknown || cl.Distance == "" {
			continue
		}
		fromPath := graph.NodePath(e.From)
		toPath := graph.NodePath(e.To)
		fromMod, okF := mm.ModuleFor(fromPath)
		toMod, okT := mm.ModuleFor(toPath)
		if !okF || !okT || fromMod == toMod {
			continue
		}
		key := labels.Key(fromMod, toMod)
		if _, isApproved := approved[key]; isApproved {
			continue
		}
		a := pairs[key]
		if a == nil {
			a = &agg{strength: string(cl.Strength)}
			pairs[key] = a
		}
		a.count++
		if len(a.samples) < 5 {
			a.samples = append(a.samples, fromPath+" -> "+toPath)
		}
	}

	out := make([]refinablePair, 0, len(pairs))
	for key, a := range pairs {
		from, to, _ := strings.Cut(key, "\x00")
		sort.Strings(a.samples)
		out = append(out, refinablePair{From: from, To: to, Strength: a.strength, EdgeCount: a.count, SamplePaths: a.samples})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out
}

// enrichSystemPrompt frames the Balanced Coupling refinement task.
const enrichSystemPrompt = `You are an architecture reviewer applying Vlad Khononov's Balanced Coupling model.
For each module pair below, the tool's deterministic heuristic labeled the dependency's integration strength from call edges — usually a blanket "functional". Refine it:
- "model": the consumer depends on the producer's data structures / domain model (types cross the boundary).
- "functional": the consumer invokes the producer's behavior (calls functions, no deep type dependence).
- "contract": the consumer depends only on a deliberately published, stable interface.
- "intrusive": the consumer reaches into internals, private state, or implementation details.
Use the module names, subdomain/volatility context, and sample dependency paths. Respect intended centralization: shared infrastructure or config hubs are not automatically intrusive.
Respond with a STRICT JSON array only — no prose, no markdown fences. One object per pair:
[{"from":"<module>","to":"<module>","strength":"model|functional|contract|intrusive","rationale":"<one sentence>"}]
Include every pair exactly once.`

// draftLabels asks the provider to refine each batch of pairs and parses the
// strict-JSON responses into draft labels.
func draftLabels(ctx context.Context, p llm.Provider, cfg config.Config, pairs []refinablePair) ([]labels.Label, error) {
	var out []labels.Label
	for start := 0; start < len(pairs); start += enrichBatchSize {
		batch := pairs[start:min(start+enrichBatchSize, len(pairs))]
		resp, err := p.Complete(ctx, llm.Request{
			System: enrichSystemPrompt,
			User:   enrichUserPrompt(cfg, batch),
		})
		if err != nil {
			return nil, err
		}
		drafts, err := parseEnrichResponse(resp.Text, batch)
		if err != nil {
			return nil, err
		}
		out = append(out, drafts...)
	}
	return out, nil
}

// enrichUserPrompt renders one batch of pairs as the user turn.
func enrichUserPrompt(cfg config.Config, batch []refinablePair) string {
	var b strings.Builder
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

// enrichResponse mirrors one element of the model's JSON answer.
type enrichResponse struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Strength  string `json:"strength"`
	Rationale string `json:"rationale"`
}

// parseEnrichResponse strictly parses the model's JSON and keeps only entries
// matching requested pairs with valid strengths. A malformed body is an error
// (never write a half-understood draft file); unknown pairs/strengths in an
// otherwise-valid body are skipped.
func parseEnrichResponse(text string, batch []refinablePair) ([]labels.Label, error) {
	// Tolerate accidental markdown fencing, nothing else.
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")

	var entries []enrichResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &entries); err != nil {
		return nil, fmt.Errorf("enrich: model response is not the required JSON array: %w", err)
	}

	requested := make(map[string]struct{}, len(batch))
	for _, p := range batch {
		requested[labels.Key(p.From, p.To)] = struct{}{}
	}

	out := make([]labels.Label, 0, len(entries))
	for _, e := range entries {
		if _, ok := requested[labels.Key(e.From, e.To)]; !ok {
			continue
		}
		if !labels.ValidStrength(e.Strength) {
			continue
		}
		out = append(out, labels.Label{
			From:       e.From,
			To:         e.To,
			Strength:   e.Strength,
			Rationale:  e.Rationale,
			Status:     labels.StatusDraft,
			Provenance: labels.ProvenanceLLM,
			Confidence: labels.ConfidenceMedium,
		})
	}
	return out, nil
}

// mergeDrafts merges new drafts into the existing labels: approved entries are
// untouchable; an existing draft for the same pair is replaced; output is
// sorted (From, To) for a deterministic file.
func mergeDrafts(existing, drafts []labels.Label) []labels.Label {
	byKey := map[string]labels.Label{}
	for _, l := range existing {
		byKey[labels.Key(l.From, l.To)] = l
	}
	for _, d := range drafts {
		key := labels.Key(d.From, d.To)
		if cur, ok := byKey[key]; ok && cur.Status == labels.StatusApproved {
			continue
		}
		byKey[key] = d
	}

	out := make([]labels.Label, 0, len(byKey))
	for _, l := range byKey {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out
}

// writeLabels writes the labels file atomically (temp + rename).
func writeLabels(path string, lbls []labels.Label) error {
	data, err := yaml.Marshal(labels.File{Version: 1, Labels: lbls})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".labels-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // best-effort cleanup on error paths
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
