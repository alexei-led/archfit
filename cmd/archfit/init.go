package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alexei-led/archfit/internal/initcfg"
	"github.com/alexei-led/archfit/internal/llm"
)

// InitCmd discovers project structure and writes a starter archfit.yaml.
type InitCmd struct {
	Root   string `short:"r" help:"Project root directory." default:"."`
	Output string `short:"o" help:"Output file (use '-' for stdout)." default:".archfit.yaml"`
}

func (c *InitCmd) Run(deps *appDeps) error {
	root := c.Root
	if !filepath.IsAbs(root) {
		var err error
		root, err = filepath.Abs(root)
		if err != nil {
			return fmt.Errorf("resolving root: %w", err)
		}
	}
	ctx := context.Background()
	cfg, err := initcfg.Discover(ctx, root, deps.Runner)
	if err != nil {
		return fmt.Errorf("discovering project structure: %w", err)
	}
	yaml := initcfg.Render(cfg, nil, false)
	if c.Output == "-" {
		_, _ = fmt.Fprint(deps.Stdout, yaml)
		return nil
	}
	if err := os.WriteFile(c.Output, []byte(yaml), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", c.Output, err)
	}
	_, _ = fmt.Fprintf(deps.Stdout, "wrote %s\n", c.Output)
	return nil
}

// initClassifySystemPrompt instructs the LLM to act as a domain modeler and
// return a strict JSON array — no prose, no markdown fences.
//
// Enum contracts:
//   - subdomain: core | supporting | generic
//   - volatility: low | medium | high
//   - layer: must be chosen from the allowed set listed in the user prompt
const initClassifySystemPrompt = `You are a domain-driven design expert classifying software modules.

For each module, determine:
- subdomain: one of "core" (central business capability), "supporting" (enables core, not differentiating), or "generic" (commodity/utility, replaceable)
- volatility: one of "low" (stable interfaces, rarely changes), "medium" (changes occasionally), or "high" (frequently evolving)
- layer: choose from the allowed layer set provided in the user prompt; pick the closest semantic match
- name: a concise suggested module name (optional improvement; keep original if good)
- rationale: one sentence explaining the classification

Respond with a JSON ARRAY only — no prose, no markdown fences, no code blocks. Each entry must include a "module" field matching the provided module name exactly:
[{"module":"<name>","subdomain":"core|supporting|generic","volatility":"low|medium|high","layer":"<from allowed set>","name":"<suggested>","rationale":"<one sentence>"}]`

// classifyBatchSize bounds how many modules go into one LLM classify request.
const classifyBatchSize = 25

// classifyUserPrompt renders a batch of classify targets into the user turn.
func classifyUserPrompt(targets []initcfg.ClassifyTarget, layers []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Allowed layers: %s\n\nModules to classify:\n", strings.Join(layers, ", "))
	for _, t := range targets {
		fmt.Fprintf(&b, "\n- module: %s\n", t.Name)
		if len(t.Paths) > 0 {
			fmt.Fprintf(&b, "  paths: %s\n", strings.Join(t.Paths, ", "))
		}
		if len(t.Files) > 0 {
			fmt.Fprintf(&b, "  files: %s\n", strings.Join(t.Files, ", "))
		}
	}
	return b.String()
}

// classifyResponse mirrors one entry in the LLM's JSON array reply.
type classifyResponse struct {
	Module     string `json:"module"`
	Subdomain  string `json:"subdomain"`
	Volatility string `json:"volatility"`
	Layer      string `json:"layer"`
	Name       string `json:"name"`
	Rationale  string `json:"rationale"`
}

// validSubdomains and validVolatilities are the allowed enum values.
var (
	validSubdomains   = map[string]bool{"core": true, "supporting": true, "generic": true}
	validVolatilities = map[string]bool{"low": true, "medium": true, "high": true}
)

// classifyModules sends targets to the LLM in batches of classifyBatchSize and
// returns a map from module name to ModuleAnnotation.
//
// Parsing rules (mirroring enrich for consistency):
//   - Tolerate accidental ```json fences; reject everything else non-JSON.
//   - A malformed body (not a JSON array) is a hard error.
//   - Skip entries whose module name is not in the requested batch.
//   - Skip entries with an invalid subdomain or volatility enum value.
//   - Carry the raw layer string into ann.Layer even if it is not in layers —
//     the stanza helper / patcher decide validity downstream.
func classifyModules(ctx context.Context, p llm.Provider, targets []initcfg.ClassifyTarget, layers []string) (map[string]initcfg.ModuleAnnotation, error) {
	out := make(map[string]initcfg.ModuleAnnotation, len(targets))
	for start := 0; start < len(targets); start += classifyBatchSize {
		batch := targets[start:min(start+classifyBatchSize, len(targets))]
		resp, err := p.Complete(ctx, llm.Request{
			System: initClassifySystemPrompt,
			User:   classifyUserPrompt(batch, layers),
		})
		if err != nil {
			return nil, err
		}
		if err := parseClassifyResponse(resp.Text, batch, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// parseClassifyResponse parses one batch response and merges valid entries into dst.
func parseClassifyResponse(text string, batch []initcfg.ClassifyTarget, dst map[string]initcfg.ModuleAnnotation) error {
	// Tolerate accidental markdown fencing, nothing else.
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var entries []classifyResponse
	if err := json.Unmarshal([]byte(text), &entries); err != nil {
		return fmt.Errorf("classify: model response is not the required JSON array: %w", err)
	}

	// Index batch by name for O(1) lookup.
	requested := make(map[string]struct{}, len(batch))
	for _, t := range batch {
		requested[t.Name] = struct{}{}
	}

	for _, e := range entries {
		if _, ok := requested[e.Module]; !ok {
			continue // unknown module — skip
		}
		if !validSubdomains[e.Subdomain] {
			continue // invalid subdomain enum — skip
		}
		if !validVolatilities[e.Volatility] {
			continue // invalid volatility enum — skip
		}
		// Layer is carried raw even if out of the allowed set.
		dst[e.Module] = initcfg.ModuleAnnotation{
			Subdomain:     e.Subdomain,
			Volatility:    e.Volatility,
			Layer:         e.Layer,
			SuggestedName: e.Name,
		}
	}
	return nil
}
