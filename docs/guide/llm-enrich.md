# LLM enrichment (off-gate): semantic labels, module roles, explain --llm

archfit's verdict is deterministic: `analyze` and `analyze --gate` never call a
model. LLM commands are opt-in, off-gate drafting tools. Their output affects
the gate only after a human reviews it and commits either `.archfit-labels.yaml`
or edited `.archfit.yaml` fields.

The semantic layer covers the two judgments static analysis cannot reliably
make:

- coupling strength for module pairs where static facts are weak or absent:
  `contract`, `model`, `functional`, or `intrusive`;
- domain role for modules and synthetic submodules: `core`, `supporting`, or
  `generic`, which drives reviewed volatility choices.

## Semantic labels workflow

The label workflow is draft -> review -> pin -> commit:

```text
archfit config enrich labels --config .archfit.yaml
archfit config enrich abstained --config .archfit.yaml
$EDITOR .archfit-labels.yaml
archfit analyze --gate --config .archfit.yaml --full
```

`config enrich labels` drafts labels for cross-module pairs whose current
strength is heuristic `functional` / `model` or still `unknown`. It batches
module-pair summaries.

`config enrich abstained` is stricter: it targets cross-module edges that the
scorer abstained on after config globs, compiler-grade facts, SCIP, and
heuristics had no strength answer. It includes endpoint names and small code
snippets, asks for `confidence: high|medium|low`, and fails rather than writing a
half-understood draft when the model response does not match the required JSON
schema. Unresolved external/library edges are missing facts, not ambiguous
facts, and are not label candidates.

Review `.archfit-labels.yaml` by deleting bad drafts and changing the keepers to
`status: approved`. That status change is the pin for labels. `--apply` is used
by owner, volatility, and subdomain draft files that write approved values into
`.archfit.yaml`; labels are already gate-readable once approved in the labels
file.

## Configuration

```yaml
analyzers:
  scip:
    enabled: auto # recommended: adds symbol-level static strength hints

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
    evidence_hash: 4f1c... # written by enrich; verified by analyze
    confidence: medium
    provenance: llm
    status: draft # flip to approved to pin
```

- Only `status: approved` entries affect classification; drafts are inert.
- Keep `provenance: llm` when the accepted judgment is still primarily the
  model's judgment. If a human re-reads the code and takes ownership of the
  classification, set `provenance: human` to restore full confidence.
- A label pins all edges of the ordered module pair (`from` → `to`).
- `evidence_hash` fingerprints the pair's import-graph edges at enrich time.
  On full runs, `analyze` recomputes it: a mismatch means the dependency surface
  changed since review — the label is ignored and a `labels/stale` advisory
  tells you to re-run enrich. Hand-authored labels may omit the hash.
- A malformed labels file fails `analyze` loudly (exit 3) — a half-read file
  must never silently alter the gate.

### Strength precedence

LLM labels fill gaps. They do not displace stronger static evidence.

| Rank | Source                     | Effect                                                                                                                                         |
| ---- | -------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| 1    | Config-authoritative facts | `internal:` / `public:` globs and declared architecture boundaries decide the floor and keep intrusive/public intent explicit.                 |
| 2    | Compiler-grade facts       | Go type information decides object-kind strength, including interfaces, DTOs, const/var reads, concrete types, and funcs.                      |
| 3    | SCIP facts                 | Symbol indexes upgrade TypeScript, Python, and Rust where available; SCIP-Go does not override Go type info.                                   |
| 4    | Heuristic                  | Language extractor fallback, usually `functional` or `model`.                                                                                  |
| 5    | Approved LLM label         | A `provenance: llm` label applies only when every static source left the strength unknown. It is attributed in `classified_edges.labeled_llm`. |
| 6    | Abstain                    | Unknown stays unscored. No fabricated ordinal enters `coupling_balance`.                                                                       |

Approved human or tool labels are explicit reviewer overrides and are treated as
reviewed config, not as LLM fill-ins. Use that provenance only when a reviewer or
tool owns the classification.

Approved `provenance: llm` labels with `confidence: medium` or `confidence: low`
lower `coupling_balance` confidence by one band. They can increase the scored
fraction, but they cannot make the confidence higher than the static baseline.

## Cost and token expectations

No CI gate run has LLM cost. After labels and config fields are committed,
`analyze` reads YAML only.

First-run enrich cost scales with candidate count and evidence size:

- `config enrich labels` sends up to 30 module pairs per request with module
  names, current heuristic strength, edge count, and sample dependency paths.
- `config enrich abstained` sends up to 10 module pairs per request, caps a run
  at 100 abstained edges, and includes up to 3 source snippets per pair. It is
  more token-heavy than the summary label pass.
- `config init --llm` and `config update --llm` scale mostly with module count
  and the README/docs/API evidence available for each module.

Responses are cached by provider, model, system prompt, and user prompt under
`.archfit-cache/llm/`. Re-running the same command with the same evidence should
reuse the cache; `--no-cache` bypasses reads and writes and may spend tokens
again. Provider prices change, so use the current provider price sheet for dollar
estimates rather than hard-coding costs in CI policy.

## Determinism

The gate stays reproducible: labels are plain YAML read deterministically, and
the arch ring test (`TestArchImports/llm_ring_unreachable_from_internal`)
proves at CI time that no internal package can even import the LLM layer.
Enrich itself is replayable through the content-addressed response cache at
`.archfit-cache/llm/` (ignored by git by default; commit it if you want
byte-identical enrich replay across machines). `--no-cache` forces fresh
calls.

## config enrich — owner, volatility, subdomain (draft -> review -> apply)

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

## config update --llm — subdomain and volatility proposals

`archfit config update --llm` adds a semantic review section to the normal config
drift report. It proposes per-module `subdomain: core|supporting|generic`, the
derived volatility it would imply, layer suggestions, and optional architectural
`role` values (`composition_root|adapter|core|shared_model|generated|test`),
with rationale tied to README, module names, docs headings, and public API shape.
The proposal is a diff for human review; it does not auto-apply LLM semantic
judgments into `.archfit.yaml`.

Synthetic modules are valid proposal targets, so large Rust crate trees and Go
workspace members can get differentiated role/volatility review instead of
inheriting one uniform value forever.

## explain --llm

`archfit explain <fingerprint> --llm` appends a Balanced Coupling narrative
(why the finding matters, the risk, a repair sketch) after the deterministic
explain output, using the same provider and cache. Without `--llm`, explain
is fully offline.

## Scope guard

Every LLM feature is off-gate and draft-first.

`archfit config init --llm` suggests `subdomain`, `volatility`, `layer`, and
`role` for discovered modules; `archfit config update --llm` emits review-only
role and volatility proposals; `config enrich` drafts coupling labels plus the
`owner`, `volatility`, and `subdomain` subcommands. None of them gate. Without
`--apply` the suggestions are commented, printed, or held in a draft file and
require human review before they become live fields. For `config init` and the
module-field `config enrich` commands, `--apply` can write reviewed values into
`.archfit.yaml`; for `config update --llm`, LLM role proposals remain review-only
even when structural `--apply` is used. Existing field values are never
overwritten. `analyze` is unaffected by these flags; it only reads the final
config and approved labels.
