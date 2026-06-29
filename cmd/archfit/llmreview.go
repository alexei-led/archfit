// llmreview: off-gate LLM holistic narrative review helper. Lives in cmd by
// design — the arch ring rule forbids internal packages from importing the LLM
// layer, so the deterministic gate stays LLM-free while this helper adds judgment.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/llm"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/score"
)

const (
	reviewMaxTokens       = 8192
	reviewMaxFindings     = 80
	reviewMaxFindingTypes = 12
	reviewMaxModuleFacts  = 80
	reviewMaxMetrics      = 40
	reviewMaxDynamicFacts = 50

	// rawReviewFile is the debug dump of the last raw LLM review response,
	// written under the cache dir before parsing so truncation/parse failures
	// are diagnosable after the fact.
	rawReviewFile = "last-review.txt"
)

// persistRawReview writes the raw LLM response to <cacheDir>/last-review.txt
// before parsing. Best-effort: a write failure (or unwritable cache dir) must
// never fail the review, so errors are intentionally swallowed.
func persistRawReview(cacheDir, text string) {
	if err := os.MkdirAll(cacheDir, 0o750); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(cacheDir, rawReviewFile), []byte(text), 0o600)
}

// runLLMReview is the reusable LLM narrative review helper. It receives an
// already-loaded config, configDir, and pre-computed diagnostic + scorecard so
// it does NOT call loadConfig or runPipeline — those are the caller's job.
//
// providerOverride is a test seam: pass a non-nil fake to skip the real provider
// construction (mirrors the old ReviewCmd.providerOverride field).
// Pass nil in production to build the provider from cfg.LLM().
func runLLMReview(ctx context.Context, deps *appDeps, cfg config.Config, configDir string, noCache bool, providerOverride llm.Provider, diag diagnostic.Diagnostic, sc score.Scorecard) error {
	llmCfg, configured := cfg.LLM()
	if !configured {
		return &exitError{code: 3, msg: "error: --llm requires ai configured (provider + model); see docs/guide/llm-enrich.md"}
	}

	cacheDir := llmCacheDir(configDir)
	provider, err := buildCachedProvider(providerOverride, llmCfg, cacheDir, noCache)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v (set the key and re-run; see `archfit doctor`)", err)}
	}

	userPrompt := buildReviewPrompt(diag, sc)
	resp, err := provider.Complete(ctx, llm.Request{
		System:    reviewSystemPrompt,
		User:      userPrompt,
		MaxTokens: reviewMaxTokens,
	})
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	// Persist the raw response before parsing so truncation/parse failures are
	// diagnosable after the fact. Best-effort: never fail the review on a debug
	// write error — the parse below is what matters.
	persistRawReview(cacheDir, resp.Text)

	rev, err := parseReviewResponse(resp.Text)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: review: model response is not the required JSON: %v (raw response saved to %s)", err, filepath.Join(cacheDir, rawReviewFile))}
	}

	// Build the configured subdomain map for conflict detection.
	configSubdomains := make(map[string]string, len(cfg.Modules))
	for name, def := range cfg.Modules {
		if def.Subdomain != "" {
			configSubdomains[name] = def.Subdomain
		}
	}

	rev, dropped := postVerify(rev, diag, configSubdomains)
	if dropped > 0 {
		_, _ = fmt.Fprintf(deps.stderr(), "review: post-verification dropped %d unsupported claim(s)\n", dropped)
	}
	printReview(deps, provider.Name(), rev)
	return nil
}

func parseReviewResponse(text string) (reviewResponse, error) {
	payload := reviewJSONPayload(text)
	var rev reviewResponse
	if err := json.Unmarshal([]byte(payload), &rev); err != nil {
		if strings.Contains(err.Error(), "unexpected end") {
			return reviewResponse{}, fmt.Errorf("%w (response appears truncated; retry with a larger response budget or smaller evidence prompt)", err)
		}
		return reviewResponse{}, err
	}
	return rev, nil
}

func reviewJSONPayload(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		if nl := strings.IndexByte(text, '\n'); nl >= 0 {
			text = text[nl+1:]
		} else {
			text = strings.TrimPrefix(text, "```")
		}
		text = strings.TrimSpace(text)
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}
	if strings.HasPrefix(text, "{") {
		return text
	}
	start := strings.IndexByte(text, '{')
	end := strings.LastIndexByte(text, '}')
	if start >= 0 && end > start {
		return strings.TrimSpace(text[start : end+1])
	}
	return text
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
- Return at most 7 dimensions, at most 3 top_risks, and at most 5 subdomain_suggestions.
- Keep every narrative under 450 characters.

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

// strengthWords is the set of coupling-strength words the LLM may assert in
// top_risk narratives and titles. Used by postVerify to cross-check claims
// against actual evidence so fabricated strength labels are never rendered.
var strengthWords = map[string]*regexp.Regexp{
	string(coupling.StrengthIntrusive):  regexp.MustCompile(`(?i)\bintrusive\b`),
	string(coupling.StrengthContract):   regexp.MustCompile(`(?i)\bcontract\b`),
	string(coupling.StrengthModel):      regexp.MustCompile(`(?i)\bmodel\b`),
	string(coupling.StrengthFunctional): regexp.MustCompile(`(?i)\bfunctional\b`),
	string(coupling.StrengthSymmetric):  regexp.MustCompile(`(?i)\bsymmetric\b`),
}

// postVerify drops LLM claims that cite entities not present in the evidence.
// Returns the filtered response and the number of dropped items (caller logs).
//
// configSubdomains maps module name → configured subdomain from .archfit.yaml.
// When a suggestion's SuggestedSubdomain conflicts with the configured value, the
// suggestion is kept but its Rationale is annotated with a conflict note so the
// reader knows the LLM disagrees with the explicit config.
func postVerify(rev reviewResponse, diag diagnostic.Diagnostic, configSubdomains map[string]string) (reviewResponse, int) {
	validModules := buildValidModules(diag)
	presentStrengths := buildPresentStrengths(diag)
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

	// Filter top_risks: drop unknown modules, drop entire risk if no valid
	// modules remain, and drop risks asserting a strength word absent from evidence.
	var n int
	rev.TopRisks, n = filterRisks(rev.TopRisks, validModules, presentStrengths)
	dropped += n

	// Filter subdomain_suggestions to known modules AND valid subdomains.
	// When the suggestion conflicts with the explicit config value, keep it but
	// annotate the rationale with a conflict note.
	filteredSug := rev.SubdomainSuggestions[:0]
	for _, s := range rev.SubdomainSuggestions {
		_, knownMod := validModules[s.Module]
		if !knownMod || !validSubdomains[s.SuggestedSubdomain] {
			dropped++
			continue
		}
		if configured, ok := configSubdomains[s.Module]; ok && configured != s.SuggestedSubdomain {
			s.Rationale = fmt.Sprintf("[conflicts with config subdomain=%s] %s", configured, s.Rationale)
		}
		filteredSug = append(filteredSug, s)
	}
	rev.SubdomainSuggestions = filteredSug

	return rev, dropped
}

// buildValidModules returns the set of module names attested by findings, file
// facts, or dynamic imports in diag.
func buildValidModules(diag diagnostic.Diagnostic) map[string]struct{} {
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
	// Dynamic/lazy-import modules are valid evidence the review may cite even
	// when they carry no static finding or file fact.
	for _, di := range diag.DynamicImports {
		if di.Module != "" {
			validModules[di.Module] = struct{}{}
		}
	}
	return validModules
}

// buildPresentStrengths returns the set of strength labels attested in diag:
// the union of MatchedBy["strength"] values from advisory findings AND the
// intrusive label implied by any uses_internal edge kind.
func buildPresentStrengths(diag diagnostic.Diagnostic) map[string]struct{} {
	present := make(map[string]struct{})
	for _, f := range diag.Findings {
		if s, ok := f.MatchedBy["strength"]; ok && s != "" {
			present[s] = struct{}{}
		}
		if f.Edge.Kind == string(graph.EdgeKindUsesInternal) {
			present[string(coupling.StrengthIntrusive)] = struct{}{}
		}
	}
	return present
}

// filterRisks filters top_risks entries, returning the kept slice and the
// number of dropped items (modules + whole-entry drops).
//
//   - Drops unknown module names within each risk (counts each as 1 dropped).
//   - Drops the whole entry when all listed modules were invalid.
//   - Drops the whole entry when its title or narrative asserts a strength word
//     absent from presentStrengths (prevents hallucinated "intrusive" claims).
func filterRisks(risks []reviewRisk, validModules, presentStrengths map[string]struct{}) ([]reviewRisk, int) {
	dropped := 0
	out := risks[:0]
	for _, r := range risks {
		var validMods []string
		for _, m := range r.Modules {
			if _, ok := validModules[m]; ok {
				validMods = append(validMods, m)
			} else {
				dropped++
			}
		}
		if len(r.Modules) > 0 && len(validMods) == 0 {
			// All modules were invalid — drop the whole risk entry.
			// Its modules were already counted above; don't double-count.
			continue
		}
		sort.Strings(validMods)
		r.Modules = validMods

		if riskAssertsMissingStrength(r, presentStrengths) {
			dropped++
			continue
		}
		out = append(out, r)
	}
	return out, dropped
}

// riskAssertsMissingStrength reports whether the risk's title or narrative
// contains a strength word absent from presentStrengths. Returns false when
// presentStrengths is empty (no coupling evidence at all → don't drop).
func riskAssertsMissingStrength(r reviewRisk, presentStrengths map[string]struct{}) bool {
	if len(presentStrengths) == 0 {
		return false
	}
	for word, re := range strengthWords {
		if _, present := presentStrengths[word]; present {
			continue
		}
		if re.MatchString(r.Title) || re.MatchString(r.Narrative) {
			return true
		}
	}
	return false
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
	_, _ = fmt.Fprintln(w, "and never affect the `analyze --gate` gate._")
}

func writeFindingGroups(b *strings.Builder, findings []finding.Finding) {
	if len(findings) == 0 {
		return
	}
	groups := make(map[string]int)
	for _, f := range findings {
		key := fmt.Sprintf("kind=%s rule=%s severity=%s status=%s", f.Kind, f.RuleID, f.Severity, f.Status)
		groups[key]++
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintf(b, "\n### Finding groups (top %d of %d)\n", minInt(reviewMaxFindingTypes, len(keys)), len(keys))
	for i, k := range keys {
		if i >= reviewMaxFindingTypes {
			fmt.Fprintf(b, "- ... %d more group(s) omitted\n", len(keys)-i)
			break
		}
		fmt.Fprintf(b, "- count=%d %s\n", groups[k], k)
	}
}

func writeFindingExamples(b *strings.Builder, findings []finding.Finding) {
	if len(findings) == 0 {
		return
	}
	ordered := append([]finding.Finding(nil), findings...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return reviewFindingSortKey(ordered[i]) < reviewFindingSortKey(ordered[j])
	})
	fmt.Fprintf(b, "\n### Finding examples (top %d of %d)\n", minInt(reviewMaxFindings, len(ordered)), len(ordered))
	for i, f := range ordered {
		if i >= reviewMaxFindings {
			fmt.Fprintf(b, "- ... %d more finding(s) omitted\n", len(ordered)-i)
			break
		}
		fmt.Fprintf(b, "- [%s] rule=%s severity=%s status=%s from=%s to=%s\n",
			f.Kind, f.RuleID, f.Severity, f.Status, f.Edge.From.Module, f.Edge.To.Module)
	}
}

func reviewFindingSortKey(f finding.Finding) string {
	kindRank := "1"
	if f.Kind == finding.KindGate {
		kindRank = "0"
	}
	severityRank := 9 - reviewSeverityRank(f.Severity)
	return fmt.Sprintf("%s:%d:%s:%s:%s:%s", kindRank, severityRank, f.RuleID, f.Edge.From.Module, f.Edge.To.Module, f.ID)
}

func reviewSeverityRank(s finding.Severity) int {
	switch s {
	case finding.SeverityCritical:
		return 4
	case finding.SeverityHigh:
		return 3
	case finding.SeverityMedium:
		return 2
	case finding.SeverityLow:
		return 1
	default:
		return 0
	}
}

func rankedReviewFileFacts(facts []diagnostic.FileFact, limit int) []diagnostic.FileFact {
	ordered := append([]diagnostic.FileFact(nil), facts...)
	sort.SliceStable(ordered, func(i, j int) bool {
		a := reviewFileFactWeight(ordered[i])
		b := reviewFileFactWeight(ordered[j])
		if a != b {
			return a > b
		}
		return ordered[i].Module < ordered[j].Module
	})
	if len(ordered) > limit {
		return ordered[:limit]
	}
	return ordered
}

func reviewFileFactWeight(ff diagnostic.FileFact) int {
	return ff.InboundModuleFanIn*10_000 + ff.OutboundDestinations*1_000 + ff.LOC
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
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

	// Gate findings. Summarise all findings and cap examples so large repos do not
	// push the model into long, truncated JSON responses.
	var gateFindings, advisories int
	for _, f := range diag.Findings {
		if f.Kind == finding.KindGate {
			gateFindings++
		} else {
			advisories++
		}
	}
	fmt.Fprintf(&b, "\n## Findings summary: %d gate violations, %d advisories\n", gateFindings, advisories)
	writeFindingGroups(&b, diag.Findings)
	writeFindingExamples(&b, diag.Findings)

	// Module facts.
	if len(diag.FileFacts) > 0 {
		facts := rankedReviewFileFacts(diag.FileFacts, reviewMaxModuleFacts)
		fmt.Fprintf(&b, "\n## Module facts (top %d of %d by fan-in/out/LOC)\n", len(facts), len(diag.FileFacts))
		for _, ff := range facts {
			fmt.Fprintf(&b, "- %s: inbound_fanin=%d outbound=%d loc=%d\n",
				ff.Module, ff.InboundModuleFanIn, ff.OutboundDestinations, ff.LOC)
		}
	}

	// Metrics.
	fmt.Fprintf(&b, "\n## Metrics (capped)\n")
	writtenMetrics := 0
	for _, m := range diag.Metrics {
		if m.Band == "info" || m.Band == "" {
			continue
		}
		if writtenMetrics >= reviewMaxMetrics {
			break
		}
		fmt.Fprintf(&b, "- %s: value=%.2f band=%s display=%s\n", m.Name, m.Value, m.Band, m.Display)
		writtenMetrics++
	}

	// Dynamic / lazy imports (report-only): invisible to the static dependency
	// graph, so they hide cycles and undercount coupling. Surfaced here so the
	// review can narrate the lazy-import hidden-coupling risk the metrics miss.
	if len(diag.DynamicImports) > 0 {
		fmt.Fprintf(&b, "\n## Dynamic / lazy imports (hidden-coupling risk, report-only; capped)\n")
		for i, di := range diag.DynamicImports {
			if i >= reviewMaxDynamicFacts {
				fmt.Fprintf(&b, "- ... %d more module(s) omitted\n", len(diag.DynamicImports)-i)
				break
			}
			fmt.Fprintf(&b, "- %s: %d site(s)\n", di.Module, di.Count)
		}
	}

	return b.String()
}
