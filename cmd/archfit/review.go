// review: off-gate LLM holistic narrative review. Lives in cmd by design —
// the arch ring rule forbids internal packages from importing the LLM layer,
// so the deterministic check gate stays LLM-free while this command adds judgment.
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
	"github.com/alexei-led/archfit/internal/engine"
	"github.com/alexei-led/archfit/internal/llm"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/score"
)

// ReviewCmd runs the full pipeline, synthesises a Scorecard, and feeds both to
// the LLM for a holistic narrative review. The LLM output is advisory only and
// never affects the check gate.
type ReviewCmd struct {
	Config  string `short:"c" default:".archfit.yaml"`
	NoCache bool   `name:"no-cache" help:"Bypass the LLM response cache."`

	// providerOverride is a test seam — set directly on the struct to inject a fake provider.
	providerOverride llm.Provider
}

func (c *ReviewCmd) Run(deps *appDeps) error {
	ctx := context.Background()

	cfg, err := loadConfig(ctx, c.Config, false)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	// Config-quality lint → stderr, same as check/score: review narrates over the
	// same evidence, so under-specified modules degrade what it can say.
	printConfigLint(os.Stderr, cfg.Lint())

	llmCfg, configured := cfg.LLM()
	if !configured {
		return &exitError{code: 3, msg: "error: archfit review needs tools.llm configured (provider + model); see docs/guide/llm-enrich.md"}
	}

	configDir := filepath.Dir(c.Config)
	cacheDir := filepath.Join(configDir, ".archfit-cache", "llm")
	provider, err := buildCachedProvider(c.providerOverride, llmCfg, cacheDir, c.NoCache)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v (set the key and re-run; see `archfit doctor`)", err)}
	}

	existingBase, err := baseline.Load(ctx, filepath.Join(configDir, defaultBaselinePath))
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	diag, err := runPipeline(ctx, deps, cfg, c.Config, false, engine.Mode{Full: true, Advisory: true}, existingBase)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	sc := score.Synthesize(diag)

	userPrompt := buildReviewPrompt(diag, sc)
	resp, err := provider.Complete(ctx, llm.Request{
		System:    reviewSystemPrompt,
		User:      userPrompt,
		MaxTokens: 2048,
	})
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	var rev reviewResponse
	text := strings.TrimSpace(resp.Text)
	// Tolerate accidental markdown fencing.
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &rev); err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: review: model response is not the required JSON: %v", err)}
	}

	rev = postVerify(rev, diag)
	printReview(deps, provider.Name(), rev)
	return nil
}

// reviewSystemPrompt grounds the LLM in Balanced Coupling vocabulary and
// constrains it to narrate existing findings only.
const reviewSystemPrompt = `You are an architecture reviewer applying Vlad Khononov's Balanced Coupling model.

## Vocabulary

Integration strength ladder (weakest → strongest): Contract < Model < Functional < Intrusive
Distance: local (same module) → internal-remote (same deployment, different module) → external (different deployment, ownership, or lifecycle)
Volatility: Core domain → high; Supporting subdomain → medium; Generic/infrastructure → low

Balance rule: maintenance effort ∝ strength × distance × volatility.
Worst case = high strength + high distance + high volatility → distributed-monolith risk (cascading changes across knowledge boundaries).
Cohesion = high strength + LOW distance = healthy; do NOT flag it.
Cheapest balancing move: change exactly ONE dimension (lower strength OR shorten distance OR reduce volatility).

Vocabulary to use: maintenance effort, cascading changes, knowledge boundaries, co-evolution, distributed monolith, big ball of mud.

## Constraints

You MUST:
- Only narrate, prioritize, and contextualise findings already present in the evidence supplied.
- Only classify volatility/subdomain for modules that appear in the evidence.
- Only propose dimension bands for dimensions named in the evidence.

You MUST NOT:
- Invent new gate violations or module names not present in the supplied evidence.
- Fabricate metrics or findings that are not in the input.
- Add prose beyond the JSON schema below.

## Required output

Respond with a single strict JSON object — no markdown fences, no prose outside the JSON:

{
  "overall_band": "<critical|poor|mixed|serviceable|strong>",
  "dimensions": [
    {
      "name": "<dimension_name>",
      "band": "<critical|poor|mixed|serviceable|strong>",
      "narrative": "<1-3 sentence plain prose using Vlad's vocabulary>"
    }
  ],
  "top_risks": [
    {
      "title": "<short risk title>",
      "modules": ["<module1>", "<module2>"],
      "narrative": "<2-3 sentence plain prose>",
      "balancing_move": "<cheapest single-dimension fix>"
    }
  ],
  "subdomain_suggestions": [
    {
      "module": "<module>",
      "suggested_subdomain": "<core|supporting|generic>",
      "rationale": "<one sentence>"
    }
  ]
}`

// reviewResponse is the schema-constrained JSON the LLM must return.
type reviewResponse struct {
	OverallBand          string                   `json:"overall_band"`
	Dimensions           []reviewDimension        `json:"dimensions"`
	TopRisks             []reviewRisk             `json:"top_risks"`
	SubdomainSuggestions []reviewSubdomainSuggest `json:"subdomain_suggestions"`
}

type reviewDimension struct {
	Name      string `json:"name"`
	Band      string `json:"band"`
	Narrative string `json:"narrative"`
}

type reviewRisk struct {
	Title         string   `json:"title"`
	Modules       []string `json:"modules"`
	Narrative     string   `json:"narrative"`
	BalancingMove string   `json:"balancing_move"`
}

type reviewSubdomainSuggest struct {
	Module             string `json:"module"`
	SuggestedSubdomain string `json:"suggested_subdomain"`
	Rationale          string `json:"rationale"`
}

// validDimNames is the set of known scorecard dimension names.
var validDimNames = map[string]struct{}{
	score.DimBoundaryIntegrity:     {},
	score.DimCouplingBalance:       {},
	score.DimDependencyGraphHealth: {},
	score.DimCohesionModularity:    {},
	score.DimChangeLocality:        {},
	score.DimArchitectureFitness:   {},
	score.DimAnalysisConfidence:    {},
}

// validBands is the scorecard band vocabulary the review must use (scorecard.yaml).
// validSubdomains (the DDD subdomain vocabulary) is shared with init.go.
var validBands = map[string]struct{}{
	string(score.BandCritical):    {},
	string(score.BandPoor):        {},
	string(score.BandMixed):       {},
	string(score.BandServiceable): {},
	string(score.BandStrong):      {},
}

// postVerify drops LLM claims that cite entities not present in the evidence.
// Dropped item counts are logged to stderr (silent when zero).
func postVerify(rev reviewResponse, diag diagnostic.Diagnostic) reviewResponse {
	// Build valid module set from FileFacts and finding endpoints.
	validModules := make(map[string]struct{})
	for _, ff := range diag.FileFacts {
		if ff.Module != "" {
			validModules[ff.Module] = struct{}{}
		}
	}
	for _, f := range diag.Findings {
		if f.Edge.From.Module != "" {
			validModules[f.Edge.From.Module] = struct{}{}
		}
		if f.Edge.To.Module != "" {
			validModules[f.Edge.To.Module] = struct{}{}
		}
	}
	// Dynamic/lazy-import modules are valid evidence the review may cite even when
	// they carry no static finding or file fact.
	for _, di := range diag.DynamicImports {
		if di.Module != "" {
			validModules[di.Module] = struct{}{}
		}
	}

	dropped := 0

	// Drop an overall band outside the rubric vocabulary so a fabricated label
	// ("excellent") is never presented as a real band.
	if _, ok := validBands[rev.OverallBand]; !ok {
		rev.OverallBand = ""
		dropped++
	}

	// Filter dimensions to known names AND valid bands.
	filteredDims := rev.Dimensions[:0]
	for _, d := range rev.Dimensions {
		_, knownName := validDimNames[d.Name]
		_, knownBand := validBands[d.Band]
		if knownName && knownBand {
			filteredDims = append(filteredDims, d)
		} else {
			dropped++
		}
	}
	rev.Dimensions = filteredDims

	// Filter top_risks: drop unknown modules; drop entire risk if no valid modules remain.
	filteredRisks := rev.TopRisks[:0]
	for _, r := range rev.TopRisks {
		var validMods []string
		for _, m := range r.Modules {
			if _, ok := validModules[m]; ok {
				validMods = append(validMods, m)
			} else {
				dropped++
			}
		}
		if len(r.Modules) > 0 && len(validMods) == 0 {
			// All modules were invalid — drop the whole risk entry. Its modules
			// were already counted as dropped above; don't double-count the risk.
			continue
		}
		sort.Strings(validMods)
		r.Modules = validMods
		filteredRisks = append(filteredRisks, r)
	}
	rev.TopRisks = filteredRisks

	// Filter subdomain_suggestions to known modules AND valid subdomains.
	filteredSug := rev.SubdomainSuggestions[:0]
	for _, s := range rev.SubdomainSuggestions {
		_, knownMod := validModules[s.Module]
		if knownMod && validSubdomains[s.SuggestedSubdomain] {
			filteredSug = append(filteredSug, s)
		} else {
			dropped++
		}
	}
	rev.SubdomainSuggestions = filteredSug

	if dropped > 0 {
		_, _ = fmt.Fprintf(os.Stderr, "review: post-verification dropped %d unsupported claim(s)\n", dropped)
	}
	return rev
}

// printReview writes the formatted narrative to deps.Stdout.
func printReview(deps *appDeps, providerName string, rev reviewResponse) {
	w := deps.Stdout
	_, _ = fmt.Fprintf(w, "## Architecture Review (off-gate LLM narrative, %s)\n\n", providerName)
	overall := rev.OverallBand
	if overall == "" {
		overall = "unrated"
	}
	_, _ = fmt.Fprintf(w, "**Overall: %s**\n", overall)

	if len(rev.Dimensions) > 0 {
		_, _ = fmt.Fprintf(w, "\n### Dimensions\n\n")
		for _, d := range rev.Dimensions {
			_, _ = fmt.Fprintf(w, "**%s**: %s\n", d.Name, d.Narrative)
		}
	}

	if len(rev.TopRisks) > 0 {
		_, _ = fmt.Fprintf(w, "\n### Top Risks\n\n")
		for _, r := range rev.TopRisks {
			mods := strings.Join(r.Modules, ", ")
			if mods != "" {
				_, _ = fmt.Fprintf(w, "**%s** (modules: %s)\n", r.Title, mods)
			} else {
				_, _ = fmt.Fprintf(w, "**%s**\n", r.Title)
			}
			_, _ = fmt.Fprintf(w, "%s\n", r.Narrative)
			_, _ = fmt.Fprintf(w, "Balancing move: %s\n\n", r.BalancingMove)
		}
	}

	if len(rev.SubdomainSuggestions) > 0 {
		_, _ = fmt.Fprintf(w, "### Subdomain Suggestions\n\n")
		for _, s := range rev.SubdomainSuggestions {
			_, _ = fmt.Fprintf(w, "- %s: %s — %s\n", s.Module, s.SuggestedSubdomain, s.Rationale)
		}
		_, _ = fmt.Fprintln(w)
	}

	_, _ = fmt.Fprintln(w, "---")
	_, _ = fmt.Fprintln(w, "_Review generated from deterministic archfit evidence. LLM narratives are advisory")
	_, _ = fmt.Fprintln(w, "and never affect the `check` gate._")
}

// buildReviewPrompt serialises the Diagnostic and Scorecard as the user turn.
func buildReviewPrompt(diag diagnostic.Diagnostic, sc score.Scorecard) string {
	var b strings.Builder

	// Scorecard summary.
	fmt.Fprintf(&b, "## Scorecard (overall %d, band %s)\n", sc.Overall, sc.OverallBand)
	for _, d := range sc.Dimensions {
		fmt.Fprintf(&b, "- %s: value=%d band=%s confidence=%s\n  summary: %s\n",
			d.Name, d.Value, d.Band, d.Confidence, d.Summary)
		for _, ev := range d.Evidence {
			fmt.Fprintf(&b, "  evidence: %s\n", ev)
		}
	}

	// Gate findings.
	var gateFindings, advisories int
	for _, f := range diag.Findings {
		if f.Kind == "gate" {
			gateFindings++
		} else {
			advisories++
		}
	}
	fmt.Fprintf(&b, "\n## Findings summary: %d gate violations, %d advisories\n", gateFindings, advisories)
	for _, f := range diag.Findings {
		fmt.Fprintf(&b, "- [%s] rule=%s severity=%s status=%s from=%s to=%s\n",
			f.Kind, f.RuleID, f.Severity, f.Status,
			f.Edge.From.Module, f.Edge.To.Module)
	}

	// Module facts.
	if len(diag.FileFacts) > 0 {
		fmt.Fprintf(&b, "\n## Module facts\n")
		for _, ff := range diag.FileFacts {
			fmt.Fprintf(&b, "- %s: inbound_fanin=%d outbound=%d loc=%d\n",
				ff.Module, ff.InboundModuleFanIn, ff.OutboundDestinations, ff.LOC)
		}
	}

	// Metrics.
	fmt.Fprintf(&b, "\n## Metrics\n")
	for _, m := range diag.Metrics {
		if m.Band == "info" || m.Band == "" {
			continue
		}
		fmt.Fprintf(&b, "- %s: value=%.2f band=%s display=%s\n", m.Name, m.Value, m.Band, m.Display)
	}

	// Dynamic / lazy imports (report-only): invisible to the static dependency
	// graph, so they hide cycles and undercount coupling. Surfaced here so the
	// review can narrate the lazy-import hidden-coupling risk the metrics miss.
	if len(diag.DynamicImports) > 0 {
		fmt.Fprintf(&b, "\n## Dynamic / lazy imports (hidden-coupling risk, report-only)\n")
		for _, di := range diag.DynamicImports {
			fmt.Fprintf(&b, "- %s: %d site(s)\n", di.Module, di.Count)
		}
	}

	return b.String()
}
