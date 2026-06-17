# archfit LLM modes (off-gate)

archfit's gate is deterministic — `check` never calls a model. LLM features are
opt-in and off-gate: `init`/`update` classification, `enrich` coupling labels,
and `explain --llm`. `check` only reads the final config and approved labels.

## Configuration

```yaml
tools:
  scip:
    enabled: "on" # enrich needs symbol-level strength hints
  llm:
    provider: anthropic # anthropic | openai | ollama
    model: claude-opus-4-8
    # base_url: http://localhost:11434/v1   # ollama only
```

API keys come from env only — `ANTHROPIC_API_KEY` / `OPENAI_API_KEY` — never
from config. `archfit doctor` shows provider, key presence, and cache status.
Responses are cached at `.archfit-cache/llm/`; `--no-cache` forces fresh calls.

LLM flags (`init` and `update`): `--llm`, `--llm-provider`, `--llm-model`,
`--no-cache`.

## init / update classification (subdomain, volatility, layer)

`init --llm` and `update --llm` suggest `subdomain`, `volatility`, and `layer`
for discovered modules, plus a module-name improvement. They never touch coupling
strength — that is `enrich`.

Mode behavior:

- `init` — structural scaffold only.
- `init --llm` — suggestions emitted as commented-inert YAML
  (`# subdomain: core  # llm-suggested — review and uncomment`); safe to gate on.
- `init --llm --apply` — `subdomain`/`volatility`/`layer` written live; a name
  suggestion stays a comment.
- `update` — drift report, writes nothing.
- `update --apply` — structural drift written live (added / path-drifted /
  removed modules).
- `update --llm` — drift report plus classification of unclassified modules.
- `update --llm --apply` — structural drift + classification written live.

`init --apply` requires `--llm`; `--apply` alone is valid only for `update`.

## Apply guardrails

- Plan mode (no `--apply`) never writes the config.
- `--apply` completes the full LLM pass first, validates the result with
  `config.Load`, backs up the existing file, and writes atomically (temp+rename).
- Existing `subdomain`/`volatility`/`layer` values are never overwritten — only
  absent fields are filled.
- A live `layer` is written only when the value already appears in `layers:`;
  otherwise it stays a comment.
- Module keys are never auto-renamed.
- `update --apply` aborts if the config changed since it was read, rather than
  clobber concurrent edits.

Workflow: run plan mode, review the drift report or commented suggestions, then
re-run with `--apply`. Treat applied classifications as a reviewed starting point.

## What "structural drift" means (update)

- Added modules — discovered but absent from config → added as new stanzas.
- Path drift — discovered paths differ from config → module `paths:` replaced.
- Removed modules — config modules with no discovered paths → commented out with
  a marker; verify before deleting.

## enrich — coupling-strength labels (draft → review → pin)

`enrich` refines whether a cross-module edge is `functional` (invokes behavior),
`model` (types cross the boundary), `contract` (published stable interface), or
`intrusive` (reaches into internals). The deterministic heuristic blanket-labels
most edges `functional`.

1. `archfit enrich` drafts into `.archfit-labels.yaml` (`status: draft` — inert).
2. A human reviews each draft: flip `status: approved` to pin, delete to reject.
   Never auto-approve drafts.
3. `check` consumes approved labels only and stays LLM-free.

Labels file (`.archfit-labels.yaml`) notes:

- Only `status: approved` entries affect classification.
- Precedence: config `public`/`internal` globs > approved labels > SCIP hint.
- A label pins all edges of the ordered module pair (`from` → `to`).
- `evidence_hash` fingerprints the pair's edges at enrich time; on full runs
  `check` recomputes it. A mismatch raises a `labels/stale` advisory and the
  label is ignored until re-reviewed. Hand-authored labels may omit the hash.
- A malformed labels file fails `check` loudly (exit 3) — it never silently
  alters the gate.

## explain --llm

`archfit explain <fingerprint> --llm` appends a Balanced Coupling narrative (why
the finding matters, the risk, a repair sketch) after the deterministic explain
output, using the same provider and cache. Without `--llm`, explain is offline.
