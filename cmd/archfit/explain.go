package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/engine"
	"github.com/alexei-led/archfit/internal/llm"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
)

// ExplainCmd re-runs the engine and prints the details of a single finding.
type ExplainCmd struct {
	Config      string `short:"c" default:".archfit.yaml"`
	Root        string `short:"r" help:"Repository root to analyze (default: directory of --config). Use this when a CI policy config lives outside the checked-out repo." type:"path"`
	Fingerprint string `arg:"" help:"Finding fingerprint prefix."`
	AISummary   bool   `name:"ai-summary" help:"Append an AI narrative (off-gate; needs ai configured)."`
	Refresh     bool   `name:"refresh" help:"Re-run all extractors and refresh the cache. Use after installing or updating analyzer tools."`
}

func (c *ExplainCmd) Run(deps *appDeps) error {
	ctx := context.Background()

	cfg, err := loadConfig(ctx, c.Config)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	configDir := filepath.Dir(c.Config)
	existingBase, err := baseline.Load(ctx, filepath.Join(configDir, defaultBaselinePath))
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	// Same pipeline as check/scan: explain must resolve the finding from the
	// same evidence (providers, change history) that produced it.
	deps.refresh = c.Refresh
	diag, _, err := runPipeline(ctx, deps, cfg, newRunContext(c.Config, c.Root), engine.Mode{Full: true, Advisory: true}, existingBase)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	for _, f := range diag.Findings {
		if strings.HasPrefix(f.ID, c.Fingerprint) {
			_, _ = fmt.Fprintf(deps.Stdout, "id:         %s\n", f.ID)
			_, _ = fmt.Fprintf(deps.Stdout, "rule:       %s\n", f.RuleID)
			_, _ = fmt.Fprintf(deps.Stdout, "status:     %s\n", f.Status)
			_, _ = fmt.Fprintf(deps.Stdout, "severity:   %s\n", f.Severity)
			_, _ = fmt.Fprintf(deps.Stdout, "edge:       %s -> %s (%s)\n", f.Edge.From.Path, f.Edge.To.Path, f.Edge.Kind)
			if f.Edge.From.Module != "" || f.Edge.To.Module != "" {
				_, _ = fmt.Fprintf(deps.Stdout, "modules:    %s -> %s\n", f.Edge.From.Module, f.Edge.To.Module)
			}
			for _, loc := range f.Locations {
				_, _ = fmt.Fprintf(deps.Stdout, "location:   %s:%d\n", loc.File, loc.Line)
			}
			_, _ = fmt.Fprintf(deps.Stdout, "why:        %s\n", f.Why)
			_, _ = fmt.Fprintf(deps.Stdout, "constraint: %s\n", f.Constraint)
			for _, alt := range f.Alternatives {
				_, _ = fmt.Fprintf(deps.Stdout, "allowed:    %s\n", alt)
			}
			if c.AISummary {
				return explainNarrative(ctx, deps, cfg, c.Config, c.Refresh, f, diag)
			}
			return nil
		}
	}

	return &exitError{code: 3, msg: fmt.Sprintf("error: no finding with fingerprint prefix %q", c.Fingerprint)}
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
func explainNarrative(ctx context.Context, deps *appDeps, cfg config.Config, configPath string, refresh bool, f finding.Finding, diag diagnostic.Diagnostic) error {
	llmCfg, configured := cfg.LLM()
	if !configured {
		return &exitError{code: 3, msg: "error: --ai-summary needs ai configured (provider + model); see docs/guide/llm-enrich.md"}
	}
	provider, err := buildProvider(llmCfg)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v (see `archfit doctor`)", err)}
	}
	cache := llm.NewCache(provider, llmCacheDir(filepath.Dir(configPath)))
	cache.RefreshMode = refresh
	provider = cache

	resp, err := provider.Complete(ctx, llm.Request{System: explainSystemPrompt, User: buildExplainPrompt(f, diag)})
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	_, _ = fmt.Fprintf(deps.Stdout, "\nnarrative (%s, off-gate):\n%s\n", provider.Name(), strings.TrimSpace(resp.Text))
	return nil
}

// buildExplainPrompt serialises one finding and its module facts as the LLM
// user turn. Separated from explainNarrative so it can be unit-tested without
// a live provider.
//
// distance_basis is included so the LLM knows whether distance was derived from
// ownership boundaries or the code-structure fallback (single-owner repos).
// When the fallback was used, a (degenerate_owner_map) qualifier is appended
// to prevent cross-team framing on single-owner codebases.
func buildExplainPrompt(f finding.Finding, diag diagnostic.Diagnostic) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Finding:\n  rule: %s\n  severity: %s\n  status: %s\n  edge: %s -> %s (%s)\n  modules: %s -> %s\n  why: %s\n  constraint: %s\n",
		f.RuleID, f.Severity, f.Status, f.Edge.From.Path, f.Edge.To.Path, f.Edge.Kind,
		f.Edge.From.Module, f.Edge.To.Module, f.Why, f.Constraint)
	if strength, ok := f.MatchedBy["strength"]; ok {
		distanceBasis := f.MatchedBy["distance_basis"]
		distanceLabel := f.MatchedBy["distance"]
		if distanceBasis == string(coupling.DistanceBasisStructure) {
			distanceLabel += " (degenerate_owner_map)"
		}
		fmt.Fprintf(&b, "  strength: %s  distance: %s  distance_basis: %s\n", strength, distanceLabel, distanceBasis)
	}
	for _, mod := range []string{f.Edge.From.Module, f.Edge.To.Module} {
		fmt.Fprintf(&b, "%s", moduleFactLine(diag, mod))
	}
	return b.String()
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
