// enrich_values: the --owner and --volatility enrich workflows. Both draft a
// single scalar per module via the off-gate LLM into a review file, then pin
// approved entries into .archfit.yaml. They share one generic implementation
// (valueSpec) since owner and volatility are structurally identical
// single-scalar module annotations. Lives in cmd — the LLM layer never crosses
// into the core ring.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/initcfg"
	"github.com/alexei-led/archfit/internal/llm"
)

// valueBatchSize bounds how many modules go into one LLM draft request.
const valueBatchSize = 25

// valueSpec describes one single-scalar enrich workflow. It names the user-facing
// field, the draft file, the config field the pin writes, how to read the current
// value, how to validate an LLM-proposed value, and the prompt that drives the draft.
type valueSpec struct {
	name           string                        // "owner" | "volatility"
	draftPath      string                        // defaultOwnersPath | defaultVolatilityPath
	field          initcfg.ModuleField           // FieldOwner | FieldVolatility
	current        func(config.ModuleDef) string // current live value for the field
	valid          func(string) bool             // nil = any non-empty value accepted
	systemPrompt   string
	withCodeowners bool // owner: include CODEOWNERS content in the prompt
}

var ownerSpec = valueSpec{
	name:           "owner",
	draftPath:      defaultOwnersPath,
	field:          initcfg.FieldOwner,
	current:        func(m config.ModuleDef) string { return m.Owner },
	valid:          nil,
	withCodeowners: true,
	systemPrompt: `You are assigning a single responsible owner (a team handle or area name) to each software module.
Use the CODEOWNERS entries (when provided), the module paths, and the file names to infer the owning team.
Prefer an existing CODEOWNERS owner whose path globs cover the module; otherwise infer a concise team/area name from the module's purpose.
Respond with a STRICT JSON array only — no prose, no markdown fences. One object per module:
[{"module":"<name>","value":"<owner>","rationale":"<one sentence>"}]
Include every module exactly once.`,
}

var volatilitySpec = valueSpec{
	name:      "volatility",
	draftPath: defaultVolatilityPath,
	field:     initcfg.FieldVolatility,
	current:   func(m config.ModuleDef) string { return m.Volatility },
	valid:     func(s string) bool { return validVolatilities[s] },
	systemPrompt: `You are assessing how frequently each module's interface evolves (Balanced Coupling volatility).
For each module pick one of:
- "low": stable interfaces, rarely changes (foundational types, shared models).
- "medium": changes occasionally as features land.
- "high": frequently evolving (active feature work, churny adapters).
Use the module paths and file names. Respond with a STRICT JSON array only — no prose, no markdown fences. One object per module:
[{"module":"<name>","value":"low|medium|high","rationale":"<one sentence>"}]
Include every module exactly once.`,
}

// runValueDraft drafts spec.field for every module that does not yet have it set,
// writing the suggestions to spec.draftPath for human review.
func (c *EnrichCmd) runValueDraft(ctx context.Context, deps *appDeps, spec valueSpec) error {
	cfg, err := loadConfig(ctx, c.Config, false)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	llmCfg, configured := cfg.LLM()
	if !configured {
		return &exitError{code: 3, msg: fmt.Sprintf("error: enrich --%s needs tools.llm configured (provider + model); see docs/guide/llm-enrich.md", spec.name)}
	}

	configDir := filepath.Dir(c.Config)
	cacheDir := filepath.Join(configDir, ".archfit-cache", "llm")
	provider, err := buildCachedProvider(c.providerOverride, llmCfg, cacheDir, c.NoCache)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v (set the key and re-run; see `archfit doctor`)", err)}
	}

	// Collect modules without the field set yet.
	var toFill []initcfg.ModuleDef
	for name, mod := range cfg.Modules {
		if spec.current(mod) != "" {
			continue
		}
		toFill = append(toFill, initcfg.ModuleDef{Name: name, Paths: mod.Paths})
	}
	sort.Slice(toFill, func(i, j int) bool { return toFill[i].Name < toFill[j].Name })
	if len(toFill) == 0 {
		_, _ = fmt.Fprintf(deps.Stdout, "enrich --%s: all modules already have %s set — nothing to draft\n", spec.name, spec.name)
		return nil
	}

	targets := initcfg.BuildClassifyTargets(configDir, toFill)
	codeowners := ""
	if spec.withCodeowners {
		codeowners = readCodeowners(configDir)
	}
	drafts, err := draftModuleValues(ctx, provider, spec, targets, codeowners)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: draft %s failed: %v", spec.name, err)}
	}
	warnPartialValues(deps.Stdout, spec, targets, drafts)

	draftPath := filepath.Join(configDir, spec.draftPath)
	existing, err := initcfg.LoadValueDrafts(draftPath, spec.name)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	merged := initcfg.MergeValueDrafts(existing, drafts)
	if err := initcfg.WriteValueDrafts(draftPath, merged); err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	_, _ = fmt.Fprintf(deps.Stdout,
		"enrich: %d draft %s(s) written to %s — review, set status: approved, then run enrich --%s --pin\n",
		len(drafts), spec.name, draftPath, spec.name)
	return nil
}

// runValuePin reads approved entries from spec.draftPath and writes them into
// .archfit.yaml, never removing or overwriting existing fields.
func (c *EnrichCmd) runValuePin(ctx context.Context, deps *appDeps, spec valueSpec) error {
	configDir := filepath.Dir(c.Config)
	draftPath := filepath.Join(configDir, spec.draftPath)

	draftFile, err := initcfg.LoadValueDrafts(draftPath, spec.name)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}

	var approved []initcfg.ValueDraft
	for _, d := range draftFile.Drafts {
		if d.Status == initcfg.DraftStatusApproved {
			approved = append(approved, d)
		}
	}
	if len(approved) == 0 {
		_, _ = fmt.Fprintf(deps.Stdout, "no approved %s drafts found — set status: approved in %s and re-run\n", spec.name, draftPath)
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

	current := make(map[string]string, len(cfg.Modules))
	for name, mod := range cfg.Modules {
		current[name] = spec.current(mod)
	}

	reviewedBy := c.ReviewedBy
	if reviewedBy == "" {
		reviewedBy = "enrich --" + spec.name
	}
	reviewedAt := time.Now().UTC()
	pins := make([]initcfg.ValuePin, 0, len(approved))
	for _, d := range approved {
		pins = append(pins, initcfg.ValuePin{
			Module:     d.Module,
			Value:      d.Value,
			ReviewedAt: reviewedAt,
			ReviewedBy: reviewedBy,
		})
	}

	edited, patched, err := initcfg.PinModuleValues(src, spec.field, current, pins)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: pin %s: %v", spec.name, err)}
	}
	if patched == 0 {
		_, _ = fmt.Fprintf(deps.Stdout, "no changes — all approved modules already have %s set\n", spec.name)
		return nil
	}

	if err := safeWriteConfig(ctx, deps, c.Config, edited, src); err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
	}
	_, _ = fmt.Fprintf(deps.Stdout, "pinned %d %s(s) into %s (reviewed_by: %s)\n", patched, spec.name, c.Config, reviewedBy)
	return nil
}

// valueResponse mirrors one element of the model's JSON answer.
type valueResponse struct {
	Module    string `json:"module"`
	Value     string `json:"value"`
	Rationale string `json:"rationale"`
}

// draftModuleValues sends targets to the LLM in batches and returns draft
// entries. codeowners (may be empty) is appended to every batch's user turn.
func draftModuleValues(ctx context.Context, p llm.Provider, spec valueSpec, targets []initcfg.ClassifyTarget, codeowners string) ([]initcfg.ValueDraft, error) {
	var out []initcfg.ValueDraft
	for start := 0; start < len(targets); start += valueBatchSize {
		batch := targets[start:min(start+valueBatchSize, len(targets))]
		resp, err := p.Complete(ctx, llm.Request{
			System: spec.systemPrompt,
			User:   valueUserPrompt(batch, codeowners),
		})
		if err != nil {
			return nil, err
		}
		drafts, err := parseValueResponse(resp.Text, spec, batch)
		if err != nil {
			return nil, err
		}
		out = append(out, drafts...)
	}
	return out, nil
}

// valueUserPrompt renders one batch of modules (and optional CODEOWNERS) as the user turn.
func valueUserPrompt(batch []initcfg.ClassifyTarget, codeowners string) string {
	var b strings.Builder
	if codeowners != "" {
		b.WriteString("CODEOWNERS:\n")
		b.WriteString(codeowners)
		b.WriteString("\n\n")
	}
	b.WriteString("Modules:\n")
	for _, t := range batch {
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

// parseValueResponse strictly parses the model's JSON and keeps only entries for
// requested modules with valid values. A malformed body is an error (never write
// a half-understood draft file); unknown modules / invalid values are skipped.
func parseValueResponse(text string, spec valueSpec, batch []initcfg.ClassifyTarget) ([]initcfg.ValueDraft, error) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")

	var entries []valueResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &entries); err != nil {
		return nil, fmt.Errorf("enrich --%s: model response is not the required JSON array: %w", spec.name, err)
	}

	requested := make(map[string]struct{}, len(batch))
	for _, t := range batch {
		requested[t.Name] = struct{}{}
	}

	out := make([]initcfg.ValueDraft, 0, len(entries))
	for _, e := range entries {
		if _, ok := requested[e.Module]; !ok {
			continue
		}
		v := strings.TrimSpace(e.Value)
		if v == "" {
			continue
		}
		if spec.valid != nil && !spec.valid(v) {
			continue
		}
		out = append(out, initcfg.ValueDraft{
			Module:    e.Module,
			Value:     v,
			Rationale: e.Rationale,
			Status:    initcfg.DraftStatusDraft,
		})
	}
	return out, nil
}

// warnPartialValues prints a warning listing modules the LLM did not return a
// usable value for, so the user knows which were left undrafted.
func warnPartialValues(w io.Writer, spec valueSpec, targets []initcfg.ClassifyTarget, drafts []initcfg.ValueDraft) {
	got := make(map[string]struct{}, len(drafts))
	for _, d := range drafts {
		got[d.Module] = struct{}{}
	}
	var missing []string
	for _, t := range targets {
		if _, ok := got[t.Name]; !ok {
			missing = append(missing, t.Name)
		}
	}
	if len(missing) == 0 {
		return
	}
	sort.Strings(missing)
	_, _ = fmt.Fprintf(w, "warning: LLM did not draft %s for %d module(s): %s\n",
		spec.name, len(missing), strings.Join(missing, ", "))
}

// readCodeowners returns the contents of a CODEOWNERS file found under root
// (root, .github/, docs/), or "" if none exists. Best-effort context for the
// owner draft prompt; capped to keep the prompt bounded.
func readCodeowners(root string) string {
	for _, rel := range []string{"CODEOWNERS", ".github/CODEOWNERS", "docs/CODEOWNERS"} {
		data, err := os.ReadFile(filepath.Join(root, rel)) //#nosec G304
		if err != nil {
			continue
		}
		const maxCodeowners = 8000 // bound the prompt; CODEOWNERS files are small
		if len(data) > maxCodeowners {
			data = data[:maxCodeowners]
		}
		return string(data)
	}
	return ""
}
