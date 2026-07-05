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

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/initcfg"
	"github.com/alexei-led/archfit/internal/llm"
)

// InitCmd discovers project structure and writes a starter archfit.yaml.
type InitCmd struct {
	Root        string `short:"r" help:"Project root directory." default:"."`
	Output      string `short:"o" help:"Output file (relative paths resolve against --root; use '-' for stdout)." default:".archfit.yaml"`
	Force       bool   `name:"force" help:"Overwrite an existing config (a timestamped backup is kept). Without it, init leaves an existing config untouched."`
	LLM         bool   `name:"llm"          help:"Run LLM classification pass (off-gate; requires ai or --llm-provider)."`
	Apply       bool   `name:"apply"        help:"Write LLM classifications live into .archfit.yaml (requires --llm; LLM judgment written directly — review before using as a gate)."`
	LLMProvider string `name:"llm-provider" help:"LLM provider override (anthropic|openai|ollama)."  default:"anthropic"`
	LLMModel    string `name:"llm-model"    help:"LLM model override."                                default:"claude-opus-4-8"`
	NoCache     bool   `name:"no-cache"     help:"Bypass the LLM response cache."`

	// providerOverride is a test seam — set directly on the struct to inject a fake provider.
	// It is never a CLI flag (no kong tag).
	providerOverride llm.Provider
}

func (c *InitCmd) Run(deps *appDeps) error {
	if c.Apply && !c.LLM {
		return &exitError{code: 3, msg: "error: --apply requires --llm"}
	}

	root := c.Root
	if !filepath.IsAbs(root) {
		var err error
		root, err = filepath.Abs(root)
		if err != nil {
			return fmt.Errorf("resolving root: %w", err)
		}
	}
	// Resolve the output path against --root so `init --root <dir>` writes into
	// <dir>, not the process CWD. Absolute paths and "-" (stdout) are kept as-is.
	out := c.Output
	if out != "-" && !filepath.IsAbs(out) {
		out = filepath.Join(root, out)
	}
	ctx := context.Background()
	cfg, err := initcfg.Discover(ctx, root, deps.Runner)
	if err != nil {
		return fmt.Errorf("discovering project structure: %w", err)
	}

	var ann map[string]initcfg.ModuleAnnotation
	if c.LLM {
		// Best-effort read of an existing config — tolerate failure.
		// Skip when writing to stdout: "-" is not a file path.
		var llmCfg config.LLMConfig
		if out != "-" {
			existingCfg, cfgErr := config.Load(ctx, out)
			if cfgErr == nil {
				if lc, ok := existingCfg.LLM(); ok {
					llmCfg = lc
				}
			}
		}
		// Flag values always override config values.
		llmCfg.Provider = c.LLMProvider
		llmCfg.Model = c.LLMModel

		cacheDir := llmCacheDir(root)
		p, buildErr := buildCachedProvider(c.providerOverride, llmCfg, cacheDir, c.NoCache)
		if buildErr != nil {
			return &exitError{code: 3, msg: fmt.Sprintf("error: %v (set the key and re-run; see `archfit doctor`)", buildErr)}
		}

		targets := initcfg.BuildClassifyTargets(root, cfg.Modules)
		configPath := out
		if configPath == "-" {
			configPath = ""
		}
		repoEvidence := architectureEvidenceLines(root, cfg.Modules, configPath, discoveredEvidenceDiagnostics(cfg))
		ann, err = classifyModulesWithEvidence(ctx, p, targets, cfg.Layers, repoEvidence)
		if err != nil {
			return &exitError{code: 3, msg: fmt.Sprintf("error: classify failed: %v", err)}
		}
		warnPartialClassify(deps.Stdout, targets, ann)

		// Owner-draft pass (folded from the former `autopilot` command): suggest an
		// owner per module so a full LLM draft (init --llm) carries owners too, not
		// just subdomain/volatility/layer. Off-gate, review-only unless --apply.
		ownerDrafts, derr := draftModuleValues(ctx, p, ownerSpec, targets, readCodeowners(root), repoEvidence)
		if derr != nil {
			return &exitError{code: 3, msg: fmt.Sprintf("error: draft owners failed: %v", derr)}
		}
		for _, d := range ownerDrafts {
			a := ann[d.Module]
			a.Owner = d.Value
			ann[d.Module] = a
		}
	}

	rendered := initcfg.Render(cfg, ann, c.Apply)
	if out == "-" {
		_, _ = fmt.Fprint(deps.Stdout, rendered)
		return nil
	}

	var original []byte
	if data, readErr := os.ReadFile(out); readErr == nil { //#nosec G304 — user-supplied output path
		original = data
	}
	// No-clobber guard: don't overwrite an existing config unless --force. An
	// existing config may carry architect-authored module mapping; init should not
	// silently replace it. This is a deliberate no-op (exit 0), not an error.
	if original != nil && !c.Force {
		// loadConfig (not bare config.Load) so a rule-type error makes this
		// report "failed to load", not "valid". out is absolute here, so the
		// defaultConfigPath missing-file fallback can never trigger.
		if _, loadErr := loadConfig(ctx, out, false); loadErr == nil {
			_, _ = fmt.Fprintf(deps.Stdout, "%s already exists and is valid — leaving it unchanged.\n"+
				"Re-run with --force to overwrite (a timestamped backup is kept).\n", out)
		} else {
			_, _ = fmt.Fprintf(deps.Stdout, "%s already exists but failed to load: %v\n"+
				"Re-run with --force to overwrite it (a timestamped backup is kept).\n", out, loadErr)
		}
		return nil
	}
	if err := safeWriteConfig(ctx, deps, out, []byte(rendered), original); err != nil {
		return err
	}
	printCacheGitignoreHint(deps.Stdout, root)
	return nil
}

// printCacheGitignoreHint reminds the user to gitignore .archfit-cache/ (the
// fact-cache + LLM-cache root that analyze creates beside the config) when the
// root .gitignore does not cover it yet. Substring check only — a commented or
// negated match is rare enough to tolerate a redundant hint.
func printCacheGitignoreHint(w io.Writer, root string) {
	data, err := os.ReadFile(filepath.Join(root, ".gitignore")) //#nosec G304 — user-supplied root
	if err == nil && strings.Contains(string(data), ".archfit-cache") {
		return
	}
	_, _ = fmt.Fprintln(w, "hint: add .archfit-cache/ to .gitignore — analyze caches extractor facts and LLM responses there (delete the directory to reset)")
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
- subdomain: one of "core" (competitive advantage/changing), "supporting" (boring, stable), or "generic" (solved problem, implementation may churn)
- volatility: one of "low", "medium", or "high", derived from the subdomain role and the repository evidence
- layer: choose from the allowed layer set provided in the user prompt; pick the closest semantic match
- role (optional): one of "composition_root" (wiring/main that fans out to everything), "adapter" (I/O boundary), "core" (domain logic), "shared_model" (cross-cutting types), "generated", or "test" — omit when none fits
- name: a concise suggested module name (optional improvement; keep original if good)
- rationale: one sentence referencing concrete repository evidence: cite evidence IDs when provided, plus module names, paths, public API globs, or listed files
- evidence_refs: a non-empty array of repository evidence IDs supporting the proposed fields
- basis: "deterministic_fact" when the proposal only restates deterministic evidence, otherwise "semantic_judgment"
- rule_suggestions (optional): review-only proposals for existing deterministic rules: forbidden_dependency, forbidden_role_dependency, public_api_max, public_api_change, or coupling.gate tuning. Every rule suggestion must include rationale, evidence_refs, and basis.
- external_system_suggestions (optional): review-only proposals for external_systems entries. Every suggestion must include name, targets, rationale, evidence_refs, and basis; volatility is optional and must be low|medium|high|frozen when set.

Respond with a JSON ARRAY only — no prose, no markdown fences, no code blocks. Each entry must include a "module" field matching the provided module name exactly:
[{"module":"<name>","subdomain":"core|supporting|generic","volatility":"low|medium|high","layer":"<from allowed set>","role":"<optional role or empty>","name":"<suggested>","rationale":"<one sentence>","evidence_refs":["doc:README.md"],"basis":"semantic_judgment","rule_suggestions":[{"id":"<stable-id>","type":"forbidden_dependency|forbidden_role_dependency|public_api_max|public_api_change|coupling.gate","from":"<selector>","to":"<selector>","max":20,"min_band":"serviceable","max_drop":5,"gate":"warn|fail","rationale":"<one sentence>","evidence_refs":["doc:README.md"],"basis":"semantic_judgment"}],"external_system_suggestions":[{"name":"payments-vendor","targets":["github.com/vendor/sdk/**"],"volatility":"low","rationale":"<one sentence>","evidence_refs":["doc:README.md"],"basis":"semantic_judgment"}]}]`

// classifyBatchSize bounds how many modules go into one LLM classify request.
const classifyBatchSize = 25

// classifyUserPrompt renders a batch of classify targets into the user turn.
func classifyUserPrompt(targets []initcfg.ClassifyTarget, layers []string, repoEvidence []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Allowed layers: %s\n\nModules to classify:\n", strings.Join(layers, ", "))
	for _, t := range targets {
		fmt.Fprintf(&b, "\n- module: %s\n", t.Name)
		if len(t.Paths) > 0 {
			fmt.Fprintf(&b, "  paths: %s\n", strings.Join(t.Paths, ", "))
		}
		if len(t.Public) > 0 {
			fmt.Fprintf(&b, "  public API globs: %s\n", strings.Join(t.Public, ", "))
		}
		if len(t.Files) > 0 {
			fmt.Fprintf(&b, "  files: %s\n", strings.Join(t.Files, ", "))
		}
	}
	if len(repoEvidence) > 0 {
		b.WriteString("\nRepository evidence:\n")
		for _, ev := range repoEvidence {
			fmt.Fprintf(&b, "- %s\n", ev)
		}
		b.WriteString("\nEvery proposed field, rule suggestion, and external_systems suggestion must cite repository evidence IDs in evidence_refs and set basis to deterministic_fact or semantic_judgment.\n")
	}
	return b.String()
}

// classifyResponse mirrors one entry in the LLM's JSON array reply.
type classifyResponse struct {
	Module                    string                             `json:"module"`
	Subdomain                 string                             `json:"subdomain"`
	Volatility                string                             `json:"volatility"`
	Layer                     string                             `json:"layer"`
	Role                      string                             `json:"role"`
	Name                      string                             `json:"name"`
	Rationale                 string                             `json:"rationale"`
	EvidenceRefs              []string                           `json:"evidence_refs"`
	Basis                     string                             `json:"basis"`
	RuleSuggestions           []ruleSuggestionResponse           `json:"rule_suggestions"`
	ExternalSystemSuggestions []externalSystemSuggestionResponse `json:"external_system_suggestions"`
}

// validSubdomains, validVolatilities, and validRoles are the allowed enum values.
var (
	validSubdomains   = map[string]bool{subdomainCore: true, subdomainSupporting: true, subdomainGeneric: true}
	validVolatilities = map[string]bool{volatilityLow: true, volatilityMedium: true, volatilityHigh: true}
	validRoles        = map[string]bool{
		string(config.RoleCompositionRoot): true, string(config.RoleAdapter): true, string(config.RoleCore): true,
		string(config.RoleSharedModel): true, string(config.RoleGenerated): true, string(config.RoleTest): true,
	}
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
	return classifyModulesWithEvidence(ctx, p, targets, layers, nil)
}

func classifyModulesWithEvidence(ctx context.Context, p llm.Provider, targets []initcfg.ClassifyTarget, layers []string, repoEvidence []string) (map[string]initcfg.ModuleAnnotation, error) {
	out := make(map[string]initcfg.ModuleAnnotation, len(targets))
	allowedRefs := evidenceRefSet(repoEvidence)
	for start := 0; start < len(targets); start += classifyBatchSize {
		batch := targets[start:min(start+classifyBatchSize, len(targets))]
		userPrompt := classifyUserPrompt(batch, layers, repoEvidence)
		var parseErr error
		var batchOut map[string]initcfg.ModuleAnnotation
		for attempt := 0; attempt < 2; attempt++ {
			resp, err := p.Complete(ctx, llm.Request{
				System: initClassifySystemPrompt,
				User:   userPrompt,
			})
			if err != nil {
				if parseErr != nil {
					return nil, parseErr
				}
				return nil, err
			}
			batchOut = make(map[string]initcfg.ModuleAnnotation, len(batch))
			parseErr = parseClassifyResponseWithEvidence(resp.Text, batch, batchOut, len(repoEvidence) > 0, allowedRefs)
			if parseErr == nil {
				break
			}
			userPrompt += "\n\nPrevious response was rejected: " + parseErr.Error() + "\nReturn only the required strict JSON array with rationale, evidence_refs, and basis for every proposal."
		}
		if parseErr != nil {
			return nil, parseErr
		}
		for name, annotation := range batchOut {
			out[name] = annotation
		}
	}
	return out, nil
}

// parseClassifyResponse parses one batch response and merges valid entries into dst.
func parseClassifyResponseWithEvidence(text string, batch []initcfg.ClassifyTarget, dst map[string]initcfg.ModuleAnnotation, requireEvidence bool, allowedRefs ...map[string]struct{}) error {
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
		if strings.TrimSpace(e.Rationale) == "" {
			return fmt.Errorf("classify: entry %q missing rationale", e.Module)
		}
		basis, refs, err := draftMetadata("classify entry", e.Module, e.Basis, e.EvidenceRefs, requireEvidence, firstAllowedEvidenceRefs(allowedRefs))
		if err != nil {
			return err
		}
		rules, err := parseRuleSuggestionResponses(e.Module, e.RuleSuggestions, requireEvidence, firstAllowedEvidenceRefs(allowedRefs))
		if err != nil {
			return err
		}
		externalSystems, err := parseExternalSystemSuggestionResponses(e.Module, e.ExternalSystemSuggestions, requireEvidence, firstAllowedEvidenceRefs(allowedRefs))
		if err != nil {
			return err
		}
		// Layer is carried raw even if out of the allowed set. Role is optional —
		// keep it only when it is a valid enum value, drop anything else.
		role := ""
		if validRoles[e.Role] {
			role = e.Role
		}
		dst[e.Module] = initcfg.ModuleAnnotation{
			Subdomain:                 e.Subdomain,
			Volatility:                e.Volatility,
			Layer:                     e.Layer,
			Role:                      role,
			SuggestedName:             e.Name,
			Rationale:                 strings.TrimSpace(e.Rationale),
			EvidenceRefs:              refs,
			Basis:                     basis,
			RuleSuggestions:           rules,
			ExternalSystemSuggestions: externalSystems,
		}
	}
	return nil
}

// warnPartialClassify prints a warning to w listing targets that were not
// returned with a complete annotation (missing from ann, or missing
// subdomain/volatility). Called after classifyModules in both InitCmd and
// UpdateCmd so the user knows which modules were left unclassified.
func warnPartialClassify(w io.Writer, targets []initcfg.ClassifyTarget, ann map[string]initcfg.ModuleAnnotation) {
	var missing []string
	for _, t := range targets {
		a, ok := ann[t.Name]
		if !ok || a.Subdomain == "" || a.Volatility == "" {
			missing = append(missing, t.Name)
		}
	}
	if len(missing) == 0 {
		return
	}
	sort.Strings(missing)
	_, _ = fmt.Fprintf(w, "warning: LLM did not classify %d module(s): %s — they were left unclassified\n",
		len(missing), strings.Join(missing, ", "))
}
