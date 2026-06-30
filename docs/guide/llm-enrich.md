# LLM enrichment (off-gate): config enrich, labels, explain --llm

archfit's verdict is deterministic — `analyze` never calls a model. The LLM
layer refines the one thing deterministic analysis cannot judge: whether a
cross-module dependency's integration strength is really `functional`
(invokes behavior), `model` (types cross the boundary), `contract`
(published stable interface), or `intrusive` (reaches into internals). The
deterministic heuristic blanket-labels most call edges `functional`; the
spike that validated this design measured ~91% of edges landing there.

The workflow is **draft → review → apply**:

```text
archfit config enrich labels   # LLM drafts → .archfit-labels.yaml (status: draft)
$EDITOR .archfit-labels.yaml   # review: approve or delete each draft
archfit analyze                # consumes APPROVED labels only — still LLM-free
```

## Configuration

```yaml
analyzers:
  scip:
    enabled: true # enrich needs the symbol-level strength hints

ai:
  provider: anthropic # anthropic | openai | ollama
  model: claude-opus-4-8
  # base_url: http://localhost:11434/v1   # ollama only
```

API keys come from the standard env vars only — `ANTHROPIC_API_KEY` /
`OPENAI_API_KEY` — never from config. archfit also best-effort loads a local
`.env` (cwd) at startup, setting a key only when it is currently unset — real env
vars and CI secrets always win. Keep `.env` out of git (gitignored by default).
`archfit doctor` shows provider, key presence, and cache status.

## The labels file (.archfit-labels.yaml)

```yaml
version: 1
labels:
  - from: handlers
    to: window_state
    strength: model
    rationale: "WindowState dataclasses cross the boundary"
    evidence_hash: 4f1c… # written by enrich; verified by analyze
    status: draft # ← flip to approved to pin
```

- Only `status: approved` entries affect classification; drafts are inert.
- Precedence: config `public`/`internal` globs > approved labels > SCIP hint.
- A label pins all edges of the ordered module pair (`from` → `to`).
- `evidence_hash` fingerprints the pair's import-graph edges at enrich time.
  On full runs, `analyze` recomputes it: a mismatch means the dependency surface
  changed since review — the label is ignored and a `labels/stale` advisory
  tells you to re-run enrich. Hand-authored labels may omit the hash.
- A malformed labels file fails `analyze` loudly (exit 3) — a half-read file
  must never silently alter the gate.

## Determinism

The gate stays reproducible: labels are plain YAML read deterministically, and
the arch ring test (`TestArchImports/llm_ring_unreachable_from_internal`)
proves at CI time that no internal package can even import the LLM layer.
Enrich itself is replayable through the content-addressed response cache at
`.archfit-cache/llm/` (ignored by git by default; commit it if you want
byte-identical enrich replay across machines). `--no-cache` forces fresh
calls.

## config enrich — owner, volatility, subdomain (draft → review → apply)

Beyond coupling labels, `config enrich` drafts module metadata fields that the
structural metrics depend on. Filling them is the through-line that makes distance
classification work: ownership contributes to distance for modules in repos with
genuine multi-team ownership, and `coupling_balance` stops being `n/a`.

> **Note on encapsulation:** `encapsulation` scores the ratio of contract/intrusive
> edges to total cross-module edges. It becomes measurable when modules have
> explicit `public:` / `internal:` globs that let archfit classify edge kinds —
> not from owner pinning alone. Pinning `owner` improves distance classification
> but does not by itself make encapsulation measurable.

```sh
archfit config enrich owner        # drafts owner per module → .archfit-owners.yaml
archfit config enrich volatility   # drafts volatility (low/medium/high) → .archfit-volatility.yaml
$EDITOR .archfit-owners.yaml       # review: set status: approved on the keepers
archfit config enrich owner --apply  # writes approved entries into modules.<name>
```

- `owner` reads `CODEOWNERS` (when present) plus the module's paths/files to
  suggest a responsible owner; the draft lands in `.archfit-owners.yaml`.
- `volatility` infers `low` / `medium` / `high` from module structure into
  `.archfit-volatility.yaml`.
- `subdomain` suggests `core` / `supporting` / `generic` per module into
  `.archfit-subdomains.yaml`.
- `--apply` writes only `status: approved` entries into `modules.<name>` and
  **never overwrites a live field** — drafts for already-set fields are skipped.
- These never touch coupling strength (that is the `labels` subcommand) and never
  affect `analyze`.

## config init --llm — full config draft (review-only)

`archfit config init --llm` drafts an entire `.archfit.yaml` in one shot: it discovers
structure, classifies every module (subdomain, volatility, layer, and `role`),
drafts an owner per module, and renders the whole config in plan mode — every
suggested field commented.

```sh
archfit config init --llm --root . -o .archfit-autopilot.yaml
archfit config init --llm --root . -o -   # stream to stdout
```

Direct it to a side file with `-o` to keep it review-only: review the draft,
then move approved fields into the live config deliberately, or re-run with
`--apply` to write approved values live. Same provider, cache, and key handling
as the other LLM commands.

## explain --llm

`archfit explain <fingerprint> --llm` appends a Balanced Coupling narrative
(why the finding matters, the risk, a repair sketch) after the deterministic
explain output, using the same provider and cache. Without `--llm`, explain
is fully offline.

## Scope guard

Every LLM feature is off-gate and draft-first.

`archfit config init --llm`, `archfit config update --llm`, and
`archfit config init --llm -o <draft>` suggest `subdomain`, `volatility`, `layer`,
and `role` for discovered modules; `config enrich` drafts coupling labels plus the
`owner`, `volatility`, and `subdomain` subcommands. None of them gate. Without
`--apply` the suggestions are commented or held in a draft file and require human
review before they become live fields. With `--apply`, approved values are written
directly — treat them as a starting point and review before running gates. Existing
field values are never overwritten. `analyze` is unaffected by these flags; it only
reads the final config.
