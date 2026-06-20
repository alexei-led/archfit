// autopilot: one-shot LLM drafter for a full .archfit.yaml. It discovers project
// structure, classifies every module (subdomain/volatility/layer/role) and drafts
// an owner per module, then renders the whole config in plan mode — every
// suggestion is a review comment, nothing is applied. The draft lands in a
// separate review file; autopilot NEVER writes .archfit.yaml. Lives in cmd: the
// LLM layer must not cross into the core ring.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/initcfg"
	"github.com/alexei-led/archfit/internal/llm"
)

// AutopilotCmd drafts a full config for review. It reuses init's discover +
// classify plumbing and enrich's owner-draft pass, rendering everything as
// commented suggestions (plan mode).
type AutopilotCmd struct {
	Root        string `short:"r" help:"Project root directory." default:"."`
	Config      string `short:"c" help:"Existing config to read tools.llm from (optional)." default:".archfit.yaml"`
	Output      string `short:"o" help:"Draft output file (review-only; use '-' for stdout). Never .archfit.yaml." default:".archfit-autopilot.yaml"`
	LLMProvider string `name:"llm-provider" help:"LLM provider override (anthropic|openai|ollama)." default:"anthropic"`
	LLMModel    string `name:"llm-model"    help:"LLM model override."                              default:"claude-opus-4-8"`
	NoCache     bool   `name:"no-cache"     help:"Bypass the LLM response cache."`

	// providerOverride is a test seam — set directly on the struct to inject a fake provider.
	providerOverride llm.Provider
}

func (c *AutopilotCmd) Run(deps *appDeps) error {
	// Safety: autopilot drafts for review and must never overwrite the live config.
	if c.Output != "-" && filepath.Base(c.Output) == defaultConfigPath {
		return &exitError{code: 3, msg: "error: autopilot never writes .archfit.yaml directly — point --output at a review file and apply after review"}
	}

	root := c.Root
	if !filepath.IsAbs(root) {
		abs, err := filepath.Abs(root)
		if err != nil {
			return fmt.Errorf("resolving root: %w", err)
		}
		root = abs
	}

	ctx := context.Background()
	cfg, err := initcfg.Discover(ctx, root, deps.Runner)
	if err != nil {
		return fmt.Errorf("discovering project structure: %w", err)
	}

	// LLM config: existing tools.llm (best-effort) overridden by flags.
	var llmCfg config.LLMConfig
	if existingCfg, cfgErr := config.Load(ctx, c.Config); cfgErr == nil {
		if lc, ok := existingCfg.LLM(); ok {
			llmCfg = lc
		}
	}
	llmCfg.Provider = c.LLMProvider
	llmCfg.Model = c.LLMModel

	cacheDir := filepath.Join(root, ".archfit-cache", "llm")
	p, err := buildCachedProvider(c.providerOverride, llmCfg, cacheDir, c.NoCache)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v (set the key and re-run; see `archfit doctor`)", err)}
	}

	targets := initcfg.BuildClassifyTargets(root, cfg.Modules)
	ann, err := classifyModules(ctx, p, targets, cfg.Layers)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: classify failed: %v", err)}
	}
	warnPartialClassify(deps.Stdout, targets, ann)

	// Owner draft pass — merge suggested owners into the annotations.
	ownerDrafts, err := draftModuleValues(ctx, p, ownerSpec, targets, readCodeowners(root))
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: draft owners failed: %v", err)}
	}
	for _, d := range ownerDrafts {
		a := ann[d.Module]
		a.Owner = d.Value
		ann[d.Module] = a
	}

	// Plan mode: every suggestion is a commented hint — nothing applied.
	rendered := initcfg.Render(cfg, ann, false)
	if c.Output == "-" {
		_, _ = fmt.Fprint(deps.Stdout, rendered)
		return nil
	}
	if err := os.WriteFile(c.Output, []byte(rendered), 0o600); err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: writing draft: %v", err)}
	}
	_, _ = fmt.Fprintf(deps.Stdout,
		"autopilot: full config draft written to %s — review the commented suggestions, then copy approved fields into %s (nothing was applied)\n",
		c.Output, c.Config)
	return nil
}
