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
  provider: anthropic
  model: claude-opus-5-5
```

Providers are `anthropic`, `openai`, `deepseek`, `ollama`, and `openai-compatible`.
Use the provider's API model ID.
`base_url` can override an endpoint. Generic `openai-compatible` requires it.
For generic endpoints, `api_key_env` names the credential variable. Omit it for keyless access.
Never put a credential value in the configuration.
Optional `effort`, `max_tokens`, and `timeout` control generation.
The default timeout is `5m`. Init/update flags override configured provider and model values.

Native providers use `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, or `DEEPSEEK_API_KEY`.
Ollama does not need a key. Generic endpoints use the configured `api_key_env`, or no key when omitted.
Archfit loads `.env` from the current directory and explicit root/config directories.
Existing environment values take precedence. Keep `.env` out of git.

Responses are cached at `.archfit-cache/llm/`.
`--refresh` bypasses cached AI responses.
Analysis and enrichment commands also refresh extractor facts when they accept that flag.

## analyze --ai-summary

If required configuration or a configured credential is missing, stop and report the incomplete setup.
Keyless providers do not require a credential.
Do not guess or fall back to a different model.

`archfit analyze --ai-summary` is an off-gate narrative over deterministic
evidence. It appends a cited, holistic AI interpretation section after the normal
render. It can prioritize and explain existing findings, but it does not create
new gate facts and it does not change the gate verdict.

- Requires `ai:` configured or provider/model overrides where supported, plus any required credential.
- Use it after a normal deterministic `analyze` / `check` run, not instead of it.
- If required configuration or a configured credential is missing, the command exits `3`.
  This is a setup failure, not a gate failure.

## config init / config update classification

`config init --ai-classify` and `config update --ai-classify` suggest
`subdomain`, `volatility`, `layer`, `role`, and module-name improvements for
discovered modules. They never touch coupling strength — that is `config enrich`.
`role` declares architectural role (`composition_root`, `adapter`, `core`,
`shared_model`, `generated`, `test`) so wiring packages can be scored as
intentional cohesion rather than unbalanced fan-out.

Mode behavior:

- `config init` — structural scaffold only.
- `config init --ai-classify` — suggestions appear as commented YAML
  (`# subdomain: core  # ai-suggested — review and uncomment`). They are safe to
  inspect.
- `config init --ai-classify --apply` — writes AI classifications live into the
  generated config. Review before using that config as a gate.
- `config update` — structural drift report, writes nothing.
- `config update --apply` — writes added modules and path changes with a backup.
  Removed modules and name drift remain review-only.
- `config update --ai-classify` — structural drift report plus cited semantic
  proposals for unclassified modules.
- `config update --ai-classify --apply` — applies structural drift only.
  Semantic AI proposals remain review-only.

## Apply guardrails

- Plan mode (no `--apply`) never writes the live config, except `config init`
  defaulting to `.archfit.yaml` for the generated scaffold.
- `config init --ai-classify --apply` writes AI classifications directly.
  Use a draft output (`-o .archfit-draft.yaml`) for review.
- `config update --apply` backs up the existing file and writes atomically.
- Existing `subdomain`/`volatility`/`layer` values are not overwritten by update
  proposals.
- A live `layer` must use a value from `layers:`. Keep other values as comments
  or proposals.
- Module keys are never auto-renamed.
- `update --apply` aborts if the config changed since it was read, rather than
  clobbering concurrent edits.

Workflow: run plan/draft mode. Review the drift report or commented suggestions.
Then copy approved fields or run the appropriate apply command. Treat applied
classifications as a reviewed starting point, not ground truth.

## What "structural drift" means (update)

- Added modules — discovered but absent from config → added as new stanzas.
- Path drift — discovered paths differ from config → module `paths:` replaced.
- Removed modules — config modules with no discovered paths appear in the review
  report. Archfit does not remove or comment out their stanzas. Review the
  findings and remove a stanza manually when the owner approves.

## enrich — labels and module metadata (draft → review → pin)

`config enrich labels` refines whether a cross-module edge is `functional`
(invokes behavior), `model` (types cross the boundary), `contract` (published
stable interface), or `intrusive` (reaches into internals). The deterministic
heuristic blanket-labels many edges as `functional` when deeper evidence is not
available.

`config enrich abstained` targets cross-module edges with unknown strength. It
drafts labels from snippets. Use it when `coupling_balance` abstains because
archfit cannot determine strength.

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
- `evidence_hash` fingerprints the pair's edges at enrich time. On full runs,
  the gate recomputes the hash. A mismatch raises a `labels/stale` advisory.
  Archfit ignores the label until review. Hand-authored labels can omit the hash.
- A malformed labels file returns exit 3. It does not silently change the gate.

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

These fields give distance, encapsulation, and coupling metrics more evidence.
Without them, `encapsulation` and `coupling_balance` can remain partly `n/a`.
Review metadata before pinning it.

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

Treat them as generated state. Review them before committing. Use stdout or a
temporary copy for analysis-only tasks.
