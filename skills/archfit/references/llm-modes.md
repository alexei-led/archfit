# archfit AI/LLM modes (off-gate)

archfit's gate is deterministic — `archfit check` never calls a model. AI/LLM
features are opt-in and off-gate: `analyze --ai-summary`, `explain
--ai-summary`, `config init --ai-classify`, `config update --ai-classify`, and
`config enrich` labels / abstained / owner / subdomain / volatility. The gate
only reads the final config and approved labels.

## Scope

This file covers review-only AI authoring and narrative modes. It does not define
or weaken deterministic gate behavior.

## Configuration

```yaml
analyzers:
  scip:
    enabled: true # enrich benefits from symbol-level strength hints

ai:
  provider: anthropic # anthropic | openai | ollama
  model: claude-opus-4-8
  # base_url: http://localhost:11434/v1   # ollama only
```

API keys come from env only — `ANTHROPIC_API_KEY` / `OPENAI_API_KEY` — never
from config. archfit also best-effort loads a local `.env` from the current
working directory at startup, setting a key only when it is currently unset
(real env / CI secrets win). Keep `.env` out of git. Responses are cached at
`.archfit-cache/llm/`; `--refresh` forces fresh extractor/AI work and updates the
cache.

## analyze --ai-summary

If the AI config, provider, or key is missing, stop and report that setup is
incomplete. Do not guess or fall back to a different model.

`archfit analyze --ai-summary` is an off-gate narrative over deterministic
evidence. It appends a cited, holistic AI interpretation section after the normal
render. It can prioritize and explain existing findings, but it does not create
new gate facts and it does not change the gate verdict.

- Requires `ai:` configured or provider/model overrides where supported, plus the
  matching API key.
- Use it after a normal deterministic `analyze` / `check` run, not instead of it.
- If AI config or key setup is missing, it exits `3`; that is setup failure, not
  a gate failure.

## config init / config update classification

`config init --ai-classify` and `config update --ai-classify` suggest
`subdomain`, `volatility`, `layer`, `role`, and module-name improvements for
discovered modules. They never touch coupling strength — that is `config enrich`.
`role` declares architectural role (`composition_root`, `adapter`, `core`,
`shared_model`, `generated`, `test`) so wiring packages can be scored as
intentional cohesion rather than unbalanced fan-out.

Mode behavior:

- `config init` — structural scaffold only.
- `config init --ai-classify` — suggestions emitted as commented-inert YAML
  (`# subdomain: core  # ai-suggested — review and uncomment`); safe to inspect.
- `config init --ai-classify --apply` — writes AI classifications live into the
  generated config. Review before using that config as a gate.
- `config update` — structural drift report, writes nothing.
- `config update --apply` — writes structural drift live (added/path-drifted /
  removed modules) with backup.
- `config update --ai-classify` — structural drift report plus cited semantic
  proposals for unclassified modules.
- `config update --ai-classify --apply` — applies structural drift only; semantic
  AI proposals remain review-only.

## Apply guardrails

- Plan mode (no `--apply`) never writes the live config, except `config init`
  defaulting to `.archfit.yaml` for the generated scaffold.
- `config init --ai-classify --apply` is the direct-write path for AI
  classifications; use a draft output (`-o .archfit-draft.yaml`) when reviewing.
- `config update --apply` backs up the existing file and writes atomically.
- Existing `subdomain`/`volatility`/`layer` values are not overwritten by update
  proposals.
- A live `layer` should only be written when the value already appears in
  `layers:`; otherwise keep it as a comment/proposal.
- Module keys are never auto-renamed.
- `update --apply` aborts if the config changed since it was read, rather than
  clobbering concurrent edits.

Workflow: run plan/draft mode, review the drift report or commented suggestions,
then copy approved fields deliberately or re-run the appropriate apply command.
Treat applied classifications as a reviewed starting point, not as ground truth.

## What "structural drift" means (update)

- Added modules — discovered but absent from config → added as new stanzas.
- Path drift — discovered paths differ from config → module `paths:` replaced.
- Removed modules — config modules with no discovered paths → commented out with
  a marker; verify before deleting.

## enrich — labels and module metadata (draft → review → pin)

`config enrich labels` refines whether a cross-module edge is `functional`
(invokes behavior), `model` (types cross the boundary), `contract` (published
stable interface), or `intrusive` (reaches into internals). The deterministic
heuristic blanket-labels many edges as `functional` when deeper evidence is not
available.

`config enrich abstained` targets cross-module edges whose strength is unknown
and drafts labels from snippets. Use it when `coupling_balance` abstains because
strength could not be determined.

1. `archfit config enrich labels` drafts `.archfit-labels.yaml` entries
   (`status: draft` — inert).
2. `archfit config enrich abstained` drafts the same kind of label for
   unknown-strength pairs.
3. A human reviews each draft: flip keepers to `status: approved`, delete or
   leave rejected entries inert.
4. `archfit check` consumes approved labels only and stays AI-free.

Labels file notes:

- Only `status: approved` entries affect classification.
- Precedence: config `public`/`internal` globs > approved labels > SCIP hint.
- A label pins all edges of the ordered module pair (`from` → `to`).
- `evidence_hash` fingerprints the pair's edges at enrich time; on full runs the
  gate recomputes it. A mismatch raises a `labels/stale` advisory and the label
  is ignored until re-reviewed. Hand-authored labels may omit the hash.
- A malformed labels file fails loudly (exit 3); it never silently alters the
  gate.

## config enrich subdomain / owner / volatility

These modes draft module metadata the structural metrics depend on.

- `config enrich subdomain` drafts `core` / `supporting` / `generic` into
  `.archfit-subdomains.yaml`.
- `config enrich owner` reads `CODEOWNERS` plus module paths into
  `.archfit-owners.yaml`.
- `config enrich volatility` infers low / medium / high into
  `.archfit-volatility.yaml`.
- `--apply --reviewed-by <name>` writes only `status: approved` entries into
  `modules.<name>` and never overwrites a live field.

Filling these makes distance, encapsulation, and coupling metrics more honest;
without them, `encapsulation` and `coupling_balance` may stay partly `n/a`. Never
auto-pin without review.

## config init --ai-classify — full config draft workflow

For a one-shot full-config draft (structure + subdomain/volatility/layer/role +
owner), redirect output to a review file:

```sh
archfit config init --ai-classify --root . -o .archfit-draft.yaml
```

Without `--apply`, suggestions are written as commented-inert YAML — every field
is inert until you uncomment it. Review the draft, then either copy approved
fields into the live config manually or rerun with `--ai-classify --apply` to
write them live. Flags: `--root`/`-r`, `--output`/`-o` (`-` for stdout),
`--ai-provider`, `--ai-model`, `--refresh`.

## explain --ai-summary

`archfit explain <fingerprint> --ai-summary` appends a Balanced Coupling
narrative (why the finding matters, risk, repair sketch) after the deterministic
explain output, using the same provider/cache. Without `--ai-summary`, explain is
offline.

## Generated artifacts

AI modes can write repo-local drafts or cache:

- `.archfit-labels.yaml`
- `.archfit-subdomains.yaml`
- `.archfit-owners.yaml`
- `.archfit-volatility.yaml`
- `.archfit-cache/llm/`

Treat them as generated state. Review them before committing; prefer stdout or a
temp copy when the task is analysis-only.
