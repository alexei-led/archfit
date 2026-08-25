// enrich_values defines the owner and volatility command surfaces. Application
// owns target selection, draft merge, review, and apply sequencing; cmd retains
// flags and provider prompts.
package main

import (
	"context"

	"github.com/alexei-led/archfit/internal/application"
)

// valueBatchSize bounds how many modules go into one LLM draft request.
const valueBatchSize = 25

// valueSpec contains only command-adapter concerns for one scalar judgment.
type valueSpec struct {
	name           string
	draftPath      string
	systemPrompt   string
	withCodeowners bool
}

var ownerSpec = valueSpec{
	name:           "owner",
	draftPath:      defaultOwnersPath,
	withCodeowners: true,
	systemPrompt: `You are assigning a single responsible owner (a team handle or area name) to each software module.
Use the CODEOWNERS entries (when provided), repository evidence IDs, module paths, and file names to infer the owning team.
Prefer an existing CODEOWNERS owner whose path globs cover the module; otherwise infer a concise team/area name from the module's purpose. Cite evidence IDs in evidence_refs and rationale when repository evidence is relevant.
Respond with a STRICT JSON array only — no prose, no markdown fences. One object per module:
[{"module":"<name>","value":"<owner>","rationale":"<one sentence>","evidence_refs":["doc:README.md"],"basis":"semantic_judgment"}]
Use basis "deterministic_fact" only when the value comes directly from CODEOWNERS or another deterministic fact; otherwise use "semantic_judgment". Include every module exactly once.`,
}

var volatilitySpec = valueSpec{
	name:      "volatility",
	draftPath: defaultVolatilityPath,
	systemPrompt: `You are assessing how frequently each module's interface evolves (Balanced Coupling volatility).
For each module pick one of:
- "low": stable interfaces, rarely changes (foundational types, shared models).
- "medium": changes occasionally as features land.
- "high": frequently evolving (active feature work, churny adapters).
Use repository evidence IDs, module paths, and file names. Cite evidence IDs in evidence_refs and rationale when repository evidence is relevant. Respond with a STRICT JSON array only — no prose, no markdown fences. One object per module:
[{"module":"<name>","value":"low|medium|high","rationale":"<one sentence>","evidence_refs":["doc:README.md"],"basis":"semantic_judgment"}]
Use basis "deterministic_fact" only when the value directly restates deterministic evidence; otherwise use "semantic_judgment". Include every module exactly once.`,
}

func (c *enrichFlags) runValueDraft(ctx context.Context, deps *appDeps, spec valueSpec) error {
	return runConfigEnrichWorkflow(ctx, c, deps, application.ConfigEnrichKind(spec.name), spec, false, "")
}

func (c *enrichFlags) runValuePin(ctx context.Context, deps *appDeps, spec valueSpec, reviewedBy string) error {
	return runConfigEnrichWorkflow(ctx, c, deps, application.ConfigEnrichKind(spec.name), spec, true, reviewedBy)
}
