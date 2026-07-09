# LLM enrichment (off-gate): semantic labels, draft config, analyze/explain --ai-summary

archfit's verdict is deterministic: `analyze` and `check` never call a
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
archfit check --config .archfit.yaml
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
    evidence_refs:
      - api:window_state
    basis: semantic_judgment
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
- `evidence_refs` lists repository evidence IDs the model cited. It may be
  empty when the judgment rests only on sample dependency paths or endpoint
  snippets, because those samples are prompt evidence but do not yet have stable
  evidence-pack IDs.
- `basis` is required on new LLM draft labels. Use `semantic_judgment` for
  coupling-strength judgments; `deterministic_fact` is reserved for entries that
  only restate tool/config facts.
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

## Architecture evidence pack

LLM draft commands share a bounded evidence pack so prompts cite the same source
IDs instead of ad-hoc raw dumps. The pack is off-gate input only; deterministic
analysis remains authoritative.

Evidence IDs use stable prefixes:

- `doc:<path>` — `README*`, `docs/design/**`, `docs/architecture/**`, ADR-like
  docs (`docs/adr/**`, `docs/adrs/**`, `docs/decisions/**`, or matching names).
- `comment:<path>` — package-level Go comments.
- `api:<module>` — configured `public:` globs and exported Go names found under
  module paths.
- `config:<path>` — bounded `.archfit.yaml` snippets with secret-like values
  redacted.
- `diag:<source>#<n>` — deterministic command summaries such as discovered
  language/module counts, update drift counts, or enrich candidate counts.

The builder sorts sources deterministically, caps each source type separately,
truncates each item, skips hidden/vendor/cache directories, and excludes
secret-like paths such as `.env`, credentials, tokens, keys, certificates, and
files whose names contain `secret`. Code-derived package comments and exported
names are Go-only today, plus configured `public:` globs for any language.
TypeScript, Python, and Rust LLM prompts still get docs, config snippets,
diagnostics, module names, dependency sample paths, and abstained-edge snippets,
but those samples do not yet have stable evidence-pack IDs. This is why label
drafts may have `evidence_refs: []` even when their rationale is based on sample
paths/snippets. This is a guardrail, not secret scanning: do
not run provider-backed LLM commands on a repo whose docs, comments, or public API
text contain secrets you would not send to that provider. Prompts require models
to cite these IDs in structured `evidence_refs` for every proposed module field,
label draft, owner, volatility, subdomain, `external_systems` entry, and rule
change. Label drafts may use `evidence_refs: []` when the prompt's sample paths
or snippets are the cited evidence. Each proposal also carries
`basis: deterministic_fact` when it only restates tool/config evidence, or
`basis: semantic_judgment` when the model is making an architectural judgment.
Draft files and update reports keep that metadata for review, while default plan mode still
leaves config unchanged. `analyze --ai-summary` uses the same pack alongside
deterministic finding IDs and metric IDs so the review can cite exactly what it is
interpreting.

## Cost and token expectations

No CI gate run has LLM cost. After labels and config fields are committed,
`analyze` reads YAML only.

First-run enrich cost scales with candidate count and evidence size:

- `config enrich labels` sends up to 30 module pairs per request with module
  names, current heuristic strength, edge count, sample dependency paths, and the
  shared evidence pack.
- `config enrich abstained` sends up to 10 module pairs per request, caps a run
  at 100 abstained edges, includes up to 3 source snippets per pair, and includes
  the shared evidence pack. It is more token-heavy than the summary label pass.
- `config init --ai-classify` and `config update --ai-classify` scale mostly with module count
  and bounded README/docs/comment/API/config/diagnostic evidence. They may also
  propose review-only `external_systems` entries when evidence names a vendor seam.

Responses are cached by provider, model, system prompt, and user prompt under
`.archfit-cache/llm/`. Re-running the same command with the same evidence should
reuse the cache; `--refresh` bypasses cache reads, makes fresh calls, and writes
fresh responses back to cache, so it may spend tokens again. The cache stores
provider responses, not prompts, but a response can still quote repository text.
Keep `.archfit-cache/` untracked unless those responses are safe to share.
Provider prices change, so use the current provider price sheet for dollar
estimates rather than hard-coding costs in CI policy.

## Determinism

The gate stays reproducible: labels are plain YAML read deterministically, and
the arch ring test (`TestArchImports/llm_ring_unreachable_from_internal`)
proves at CI time that no internal package can even import the LLM layer.
Enrich itself is replayable through the content-addressed response cache at
`.archfit-cache/llm/` (ignored by git by default; commit it only when the cached
responses contain no sensitive repository text and you want byte-identical enrich
replay across machines). `--refresh` forces fresh calls.

## analyze --ai-summary — cited architect review

`archfit analyze --ai-summary` keeps the deterministic `analyze` output first, then emits
an advisory architect review. Text and Markdown runs append the review to stdout;
`json`, `sarif`, and `scorecard` runs write it to stderr so stdout remains
parseable. The review schema requires each dimension, top risk, and subdomain
suggestion to state a `claim_type`:

- `deterministic_fact` — a direct restatement of tool/config evidence.
- `semantic_interpretation` — architectural judgment over cited evidence.
- `recommendation` — suggested action or classification; must cite at least one
  `finding_id`, `metric_id`, or repository `evidence_ref`.
- `unknown` — evidence is too weak to classify.

Post-verification drops fabricated module names, unsupported bands/subdomains,
unknown claim types, unsupported citations, and uncited recommendations. This can
change only the appended review text; it never mutates verdicts, findings,
metrics, scores, config, labels, or the gate.

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
- Every owner, volatility, and subdomain draft includes `rationale`, `evidence_refs`,
  and `basis` so reviewers can see what facts were cited and whether the entry is
  deterministic evidence or semantic judgment.
- `--apply` writes only `status: approved` entries into `modules.<name>` and
  **never overwrites a live field** — drafts for already-set fields are skipped.
- These never touch coupling strength (that is the `labels` subcommand) and never
  affect `analyze`.

## config init --ai-classify — full config draft or direct apply

`archfit config init --ai-classify` drafts an entire `.archfit.yaml` in one shot: it discovers
structure, classifies every module (subdomain, volatility, layer, and `role`),
drafts an owner per module, and renders the whole config in plan mode — every
suggested field commented.

```sh
archfit config init --ai-classify --root . -o .archfit-init-llm.yaml
archfit config init --ai-classify --root . -o -   # stream to stdout
```

Direct it to a side file with `-o` to keep it review-only: review the draft,
then move approved fields into the live config deliberately. `--apply` skips that
review handoff and writes the LLM classifications live into the generated config,
so inspect and edit the file before using it as a gate. Same provider, cache, and
key handling as the other LLM commands.

## config update --ai-classify — cited module and rule proposals

`archfit config update --ai-classify` adds a semantic review section to the normal config
drift report. It proposes per-module `subdomain: core|supporting|generic`, the
derived volatility it would imply, layer suggestions, and optional architectural
`role` values (`composition_root|adapter|core|shared_model|generated|test`),
with rationale tied to README, module names, docs headings, and public API shape.
Each module proposal includes `basis` and `evidence_refs`.

The same response may include review-only rule suggestions for deterministic
config mechanisms only: `forbidden_dependency`, `forbidden_role_dependency`,
`public_api_max`, `public_api_change`, and `coupling.gate` tuning. Unsupported
rule types and suggestions missing evidence refs are rejected instead of being
written as half-understood config.

The proposal is a diff for human review; it does not auto-apply LLM semantic
judgments or rule suggestions into `.archfit.yaml`.

Synthetic modules are valid proposal targets, so large Rust crate trees and Go
workspace members can get differentiated role/volatility review instead of
inheriting one uniform value forever.

## explain --ai-summary

`archfit explain <fingerprint> --ai-summary` appends a Balanced Coupling narrative
(why the finding matters, the risk, a repair sketch) after the deterministic
explain output, using the same provider and cache. Without `--ai-summary`, explain
is fully offline.

## Scope guard

Every LLM feature is off-gate and draft-first. LLM output may explain
`connascence.roadmap`, `dynamic_imports`, or `runtime_async_edges`, but those
blocks remain report-only unless a deterministic extractor later supplies the
missing fact. A narrative cannot create a connascence category, score input,
baseline delta, or gate finding.

`archfit config init --ai-classify` suggests `subdomain`, `volatility`, `owner`, `layer`,
and `role` for discovered modules; `archfit config update --ai-classify` emits
review-only module and rule proposals; `config enrich` drafts coupling labels plus
the `owner`, `volatility`, and `subdomain` subcommands. None of them gate. Without
`--apply` the suggestions are commented, printed, or held in a draft file and
require human review before they become live fields. `config init --ai-classify --apply`
writes model suggestions live directly into the generated config; treat that file
as unreviewed until a human checks it. Module-field `config enrich --apply` is
different: it reads only `status: approved` entries from the draft files and pins
those reviewed values into `.archfit.yaml`. For `config update --ai-classify`, LLM
semantic and rule proposals remain review-only even when structural `--apply` is
used. Existing field values are never overwritten. `analyze` is unaffected by
these flags; it only reads the final config and approved labels.
