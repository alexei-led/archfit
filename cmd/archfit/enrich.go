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

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/engine"
	"github.com/alexei-led/archfit/internal/labels"
	"github.com/alexei-led/archfit/internal/llm"
	"github.com/alexei-led/archfit/internal/metrics"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"

	"github.com/goccy/go-yaml"
)

// enrichBatchSize bounds how many module pairs go into one LLM request.
const enrichBatchSize = 30

// EnrichCmd drafts model-vs-functional strength refinements for cross-module
// edges. Drafts land in .archfit-labels.yaml with status: draft; a human
// reviews, flips approved entries, and the deterministic gate consumes them.
type EnrichCmd struct {
	Config  string `short:"c" default:".archfit.yaml"`
	NoCache bool   `name:"no-cache" help:"Bypass the LLM response cache."`
}

// captureMetric records the MetricInput so enrich can reuse the exact
// pipeline evidence (graph, classifications) without re-implementing stages.
type captureMetric struct{ in *metrics.MetricInput }

func (m *captureMetric) Name() string    { return "enrich_capture" }
func (m *captureMetric) Version() string { return "enrich_capture.v0" }
func (m *captureMetric) Calculate(in metrics.MetricInput) diagnostic.MetricResult {
	*m.in = in
	return diagnostic.MetricResult{Name: m.Name(), Band: "info", Display: "internal capture"}
}

func (c *EnrichCmd) Run(deps *appDeps) error {
	ctx := context.Background()

	cfg, err := loadConfig(ctx, c.Config, false)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	llmCfg, configured := cfg.LLM()
	if !configured {
		return &exitError{code: 3, msg: "error: enrich needs tools.llm configured (provider + model); see docs/guide/llm-enrich.md"}
	}
	if !cfg.ScipEnabled() {
		return &exitError{code: 3, msg: "error: enrich needs tools.scip.enabled: on — the refinable strength hints come from the SCIP symbol index"}
	}

	configDir := filepath.Dir(c.Config)
	provider, err := buildProvider(llmCfg)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v (set the key and re-run; see `archfit doctor`)", err)}
	}
	if !c.NoCache {
		provider = llm.NewCache(provider, filepath.Join(configDir, ".archfit-cache", "llm"))
	}

	labelsPath := filepath.Join(configDir, defaultLabelsPath)
	existing, err := labels.Load(labelsPath)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	// Run the standard pipeline once, capturing the evidence the metrics saw.
	var captured metrics.MetricInput
	base, err := baseline.Load(ctx, filepath.Join(configDir, defaultBaselinePath))
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	if _, err := runPipeline(ctx, deps, cfg, c.Config, engine.Mode{Full: true}, base, &captureMetric{in: &captured}); err != nil {
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

// buildProvider constructs the configured LLM provider.
func buildProvider(c config.LLMConfig) (llm.Provider, error) {
	switch c.Provider {
	case "anthropic":
		return llm.NewAnthropic(c.Model)
	case "openai":
		return llm.NewOpenAI(c.Model)
	case "ollama":
		return llm.NewOllama(c.BaseURL, c.Model), nil
	default: // unreachable: config validation rejects unknown providers
		return nil, fmt.Errorf("unknown llm provider %q", c.Provider)
	}
}

// refinablePair is one candidate module pair with its evidence summary.
type refinablePair struct {
	From, To    string
	Strength    string // current heuristic strength (functional or model)
	EdgeCount   int
	SamplePaths []string // up to 5 "fromPath -> toPath" examples
}

// selectRefinablePairs picks cross-module pairs whose strength came from the
// blanket call-edge heuristic (functional/model via hint — not glob-decided,
// not already approved). Deterministic order (From, To).
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
		if cl.Strength != coupling.StrengthFunctional && cl.Strength != coupling.StrengthModel {
			continue
		}
		if cl.Distance == coupling.DistanceSameModule || cl.Distance == coupling.DistanceUnknown || cl.Distance == "" {
			continue
		}
		fromPath := strings.TrimPrefix(e.From, "file:")
		toPath := strings.TrimPrefix(e.To, "file:")
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
			From: e.From, To: e.To,
			Strength:  e.Strength,
			Rationale: e.Rationale,
			Status:    labels.StatusDraft,
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

// explainSystemPrompt frames the finding narrative.
const explainSystemPrompt = `You are an architecture reviewer applying Vlad Khononov's Balanced Coupling model (integration strength x distance x volatility).
Given one architecture finding with its evidence, explain in under 200 words:
1. why this coupling/violation matters (which dimension is imbalanced),
2. the concrete risk if left as-is,
3. a specific repair sketch consistent with the stated constraint.
Plain prose, no headings, no lists, no code fences.`

// explainNarrative appends an off-gate LLM narrative for one finding. The
// deterministic explain output above it is already printed; this only adds
// judgment on top — failures here never affect any verdict.
func explainNarrative(ctx context.Context, deps *appDeps, cfg config.Config, configPath string, noCache bool, f finding.Finding, diag diagnostic.Diagnostic) error {
	llmCfg, configured := cfg.LLM()
	if !configured {
		return &exitError{code: 3, msg: "error: --llm needs tools.llm configured (provider + model); see docs/guide/llm-enrich.md"}
	}
	provider, err := buildProvider(llmCfg)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v (see `archfit doctor`)", err)}
	}
	if !noCache {
		provider = llm.NewCache(provider, filepath.Join(filepath.Dir(configPath), ".archfit-cache", "llm"))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Finding:\n  rule: %s\n  severity: %s\n  status: %s\n  edge: %s -> %s (%s)\n  modules: %s -> %s\n  why: %s\n  constraint: %s\n",
		f.RuleID, f.Severity, f.Status, f.Edge.From.Path, f.Edge.To.Path, f.Edge.Kind,
		f.Edge.From.Module, f.Edge.To.Module, f.Why, f.Constraint)
	if strength, ok := f.MatchedBy["strength"]; ok {
		fmt.Fprintf(&b, "  strength: %s  distance: %s\n", strength, f.MatchedBy["distance"])
	}
	for _, mod := range []string{f.Edge.From.Module, f.Edge.To.Module} {
		fmt.Fprintf(&b, "%s", moduleFactLine(diag, mod))
	}

	resp, err := provider.Complete(ctx, llm.Request{System: explainSystemPrompt, User: b.String()})
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	_, _ = fmt.Fprintf(deps.Stdout, "\nnarrative (%s, off-gate):\n%s\n", provider.Name(), strings.TrimSpace(resp.Text))
	return nil
}

// moduleFactLine renders the structural facts of one module when present.
func moduleFactLine(diag diagnostic.Diagnostic, module string) string {
	if module == "" {
		return ""
	}
	for _, ff := range diag.FileFacts {
		if ff.Module == module {
			return fmt.Sprintf("Module %s facts: inbound_fanin=%d outbound=%d loc=%d\n",
				module, ff.InboundModuleFanIn, ff.OutboundDestinations, ff.LOC)
		}
	}
	return ""
}
