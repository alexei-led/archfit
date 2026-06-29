# archfit LLM modes (off-gate)

archfit's gate is deterministic — the deterministic gate (`analyze` without
`--llm`) never calls a model. LLM features are opt-in and off-gate: `analyze
--llm`, `init`/`update` classification, `enrich` labels and metadata,
`autopilot`, and `explain --llm`. The gate (`analyze --gate`) only reads the
final config and approved labels.

## Configuration

```yaml
analyzers:
  scip:
    enabled: true   # enrich needs symbol-level strength hints

ai:
  provider: anthropic   # anthropic | openai | ollama
  model: claude-opus-4-8
  # base_url: http://localhost:11434/v1   # ollama only
```

API keys come from env only — `ANTHROPIC_API_KEY` / `OPENAI_API_KEY` — never
from config. archfit also best-effort loads a local `.env` (cwd) at startup,
setting a key only when it is currently unset (real env / CI secrets win); keep
`.env` out of git. `archfit doctor` shows provider, key presence, and cache
status. Responses are cached at `.archfit-cache/llm/`; `--no-cache` forces fresh
calls.

## analyze --llm

`archfit analyze --llm` is an off-gate LLM narrative over deterministic evidence.
It appends a holistic LLM interpretation section after the deterministic output.
It can prioritize and explain existing findings, but it does not create new gate
facts and it does not change the gate verdict.

- Requires `ai:` configured and the matching API key.
- Use it for explanation and prioritization after a regular `analyze` run, not
  instead of it.
- If LLM config is missing, it exits `3`; that is a setup problem, not a gate
  failure.

## init / update classification (subdomain, volatility, layer)

`init --llm` and `update --llm` suggest `subdomain`, `volatility`, `layer`, and
`role` for discovered modules, plus a module-name improvement. They never touch
coupling strength — that is `enrich`. (`role` declares a module's architectural
role — `composition_root`, `adapter`, `core`, `shared_model`, `generated`, `test`
— so a wiring/`cmd` package's fan-out reads as cohesion, not unbalanced coupling.)

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

## enrich — labels and module metadata (draft → review → pin)

`enrich` refines whether a cross-module edge is `functional` (invokes behavior),
`model` (types cross the boundary), `contract` (published stable interface), or
`intrusive` (reaches into internals). The deterministic heuristic blanket-labels
most edges `functional`.

1. `archfit enrich` drafts into `.archfit-labels.yaml` (`status: draft` — inert).
2. A human reviews each draft: flip `status: approved` to pin, delete to reject.
   Never auto-approve drafts.
3. `analyze --gate` consumes approved labels only and stays LLM-free.

Labels file (`.archfit-labels.yaml`) notes:

- Only `status: approved` entries affect classification.
- Precedence: config `public`/`internal` globs > approved labels > SCIP hint.
- A label pins all edges of the ordered module pair (`from` → `to`).
- `evidence_hash` fingerprints the pair's edges at enrich time; on full runs
  `analyze --gate` recomputes it. A mismatch raises a `labels/stale` advisory and
  the label is ignored until re-reviewed. Hand-authored labels may omit the hash.
- A malformed labels file fails `analyze` loudly (exit 3) — it never silently
  alters the gate.

## enrich --subdomains / --owner / --volatility (draft → review → pin)

These modes draft module metadata the structural metrics depend on.

- `enrich --subdomains` drafts `core` / `supporting` / `generic` into
  `.archfit-subdomains.yaml`.
- `enrich --owner` reads `CODEOWNERS` plus module paths into `.archfit-owners.yaml`.
- `enrich --volatility` infers low / medium / high into `.archfit-volatility.yaml`.
- `--pin` writes only `status: approved` entries into `modules.<name>` and never
  overwrites a live field.

Filling these makes distance and encapsulation metrics more honest; without them,
`boundary_integrity` and `coupling_balance` may stay partly `n/a`. Never auto-pin
without review.

## autopilot — full config draft (review-only)

`archfit autopilot` drafts an entire `.archfit.yaml` in one shot (structure,
subdomain/volatility/layer/role, and a per-module owner), rendered in plan mode —
every field commented. It writes a review file (`.archfit-autopilot.yaml`) and
**refuses** to write `.archfit.yaml` directly (exit 3). Flags: `--root`/`-r`,
`--config`/`-c`, `--output`/`-o` (`-` for stdout), `--llm-provider`,
`--llm-model`, `--no-cache`. Review the draft, then move approved fields into the
live config — autopilot applies nothing.

## explain --llm

`archfit explain <fingerprint> --llm` appends a Balanced Coupling narrative (why
the finding matters, the risk, a repair sketch) after the deterministic explain
output, using the same provider and cache. Without `--llm`, explain is offline.

## Generated artifacts

LLM modes can write repo-local drafts or cache:

- `.archfit-labels.yaml`
- `.archfit-subdomains.yaml`
- `.archfit-owners.yaml`
- `.archfit-volatility.yaml`
- `.archfit-autopilot.yaml`
- `.archfit-cache/llm/`

Treat them as generated state. Review them before committing; prefer stdout or a
temp copy when the task is analysis-only.
