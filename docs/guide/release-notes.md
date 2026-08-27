# Release notes

## Unreleased — architecture-state reporting

This release retires the repository score. Read the breaking changes before
upgrading.

Breaking changes:

- **`--format json` now emits `archfit.architecture-state.v1` at the document
  root.** Root keys are `verdict`, `decision`, `comparison`, `measurement`,
  `dimensions` (nine envelopes), `coverage`, `findings`, `agent_tasks`, and
  `seams`. There is no repository scalar anywhere in it. Diagnostic-only blocks
  (`tool_coverage` detail, `owner_source`, `config_warnings`,
  `git_finding_delta`, `advisory_tasks`) are not part of the state contract.
- **`--format legacy-json` emits the pre-cutover diagnostic envelope**
  (`archfit.diagnostic.v2`) for exactly one release. It must be selected
  explicitly and never reaches the verdict or the exit code. Consumers that need
  a retired block should migrate now.
- **Config schema v2 is required.** `version: 1` and the retired
  `coupling.gate.min_band` / `coupling.gate.max_drop` keys are rejected with
  exit `3`. Migrate with `archfit config update --migration-only --apply`, which
  bumps the version, removes the retired keys with their documenting comments,
  and splices in a warn-mode replacement. It never infers `mode: fail`, running
  it twice is byte-identical, and the write is validated, backed up, and atomic.
- **The coupling gate is now
  `coupling.gate.distributed_monolith: {mode, max_new_seams}`.** It counts
  logical seams — one ordered module pair, however many imports express it — that
  are newly introduced against a *comparable* reference. `coupling_balance` no
  longer gates at all. Advisory promotion is gone: the seam gate names its own
  seams.
- **`archfit check`'s exit code IS the state verdict**: `healthy` -> 0,
  `needs_attention` -> 2, `blocked` -> 1, error -> 3. **Expect `2`, not `0`, on a
  healthy repo in v1** — complexity, testability, and operations report `partial`
  by contract, and any partial dimension is `needs_attention`. Gate on `1`; a
  recipe that fails on any non-zero exit will now fail every clean build.
- **Baseline schema v2** (`archfit.baseline.v2`) stores the four comparison
  fingerprints, hard-gate finding IDs, seam IDs, and the nine dimension
  snapshots — no repository scalar. A v1 file stays readable for its accepted
  fingerprints, is never rewritten on read, and can never support a
  state/dimension/seam comparison (reported as `legacy_score_snapshot_ignored`).
- `archfit baseline` capture is now a pure function of tree + config. Reading the
  file it was about to overwrite made the capture self-referential and it never
  settled; three captures on this repo produced 108, 164, then 148 entries.

New:

- Nine dimension envelopes (`intent`, `structure`, `modularity`, `coupling`,
  `change_locality`, `complexity`, `testability`, `operations`, `drift`), each
  with its own status, gate, confidence, denominator, metrics, and an explicit
  list of what it could not measure.
- A coupling **seam ledger**: one record per ordered module pair with a stable
  ID, a score distribution, raw owner/deploy/structural distance facts, the book
  Ch10 quadrant, and a balancing hypothesis.
- `archfit config update --migration-only [--json|--apply]`.
- Text, Markdown, SARIF, and scorecard all report the same verdict, dimension
  statuses, coverage split, and finding IDs. SARIF carries the state in
  `run.properties`; finding identity (ruleId, ruleIndex, `archfit/v1`
  fingerprint) is unchanged by the cutover.

Also, under `--format legacy-json` only:

- `classified_edges.by_balance_driver` / `by_critical_driver` — whether `|S-D|`
  or `10-V` drove each scored edge's book balance.
- `classified_edges.by_module_pair` — scored-edge concentration per module
  boundary. Like every other histogram the dimension reports, it counts scored
  CROSS-BOUNDARY edges only; same-module coupling is reported under
  `local_coupling`.
- Scorecard dimensions carry `raw_value` — the normalized mean before any
  confidence cap — and `cap_applied`, the name of the cap that moved it.
  `raw_value` is emitted whenever it is non-zero, so it appears alongside an
  equal `value` on an uncapped dimension; `cap_applied` appears only when a cap
  actually lowered the value.

The `coupling_balance` `summary` and `evidence` **values** changed (the keys did
not): the summaries now point at the driver histograms, and the scored-fraction
and critical-band lines were moved ahead of the histograms so the critical /
distributed-monolith count stays inside the truncated `why` text.

Internal: the analysis stage sequence moved behind one application-owned stage
executor; `internal/engine`, `internal/analysispipeline`, and `internal/view` are
gone.

Release validation:

- The final branch binary passed the strict 11-repository corpus across Go,
  Python, TypeScript/JavaScript, and Rust. Config migration, repeated JSON byte
  identity, five-format finding parity, and the `0/2/1/3` exit contract were
  checked.
- The Rust follow-up used rustup toolchain `1.98.0`, `rust-analyzer 1.98.0`, and
  `cargo-modules 0.26.0`. The owned corpus harness pins
  `RUSTUP_TOOLCHAIN=1.98.0` by default without modifying third-party projects.
- `partial` and `unmeasured` remain honest v1 outcomes: runtime coverage,
  cognitive complexity, runtime topology, SBOM, and vulnerability state are not
  collected by this release unless a future analyzer explicitly supplies them.

Upgrade checklist:

1. Run `archfit config update --migration-only --json -c .archfit.yaml` and
   review the candidate.
2. Apply it with `archfit config update --migration-only --apply -c
   .archfit.yaml`.
3. Update CI to accept `check` exit `2` as `needs_attention` and block on `1` or
   `3` according to local policy.
4. Migrate JSON consumers to `archfit.architecture-state.v1`; use
   `legacy-json` only during this one-release compatibility window.
5. Regenerate a v2 baseline only after reviewing the new state and findings.

## v1.7.0 configuration confidence

Breaking changes:

- `archfit baseline --base <ref>` removed. It never changed the saved baseline —
  a baseline always records the tree as checked out. To compare against a ref,
  use `archfit check --base <ref>` or `archfit analyze --base <ref>`. The removed
  flag now returns `archfit: unknown flag --base` with exit `3`.

New:

- `archfit config compare <candidate>` measures one source tree under two
  configurations and reports the difference. Report-only: exit `0` on success,
  exit `3` on an input or runtime error, and findings never move the exit code.
  Both sides use an empty accepted baseline, so the comparison is raw
  measurement. A higher candidate score is never reported as better.
- `archfit config update --json` emits one machine-readable review document
  (`archfit.config-review.v1`) with status `action_required`, `review_available`,
  or `no_known_issues`. `--json` with `--apply`, `--ai-classify`, or `--refresh`
  is a usage error (exit `3`) rejected before any discovery or write.
- `config update --apply` no longer deletes, comments out, or re-keys a
  configured module stanza. A configured module and a discovered module that own
  the same paths under different names are reported as `name_drift`, and
  configured modules discovery did not emit stay under `removed_modules`. Both
  are review-only and neither raises the status to `action_required`.
- `--base <ref> --json` adds a report-only `git_finding_delta` block that sorts
  the current `agent_tasks[]` into `introduced`, `pre_existing`, and
  `unknown_origin`. Missing analyzer evidence produces `unknown`, never a false
  `introduced`.

Baseline file:

- `.archfit-baseline.json` records `rubric_version` alongside the scorer version
  in its score snapshot, so `coupling.gate.max_drop` refuses to anchor against an
  incompatible snapshot instead of comparing across rubrics. Existing baselines
  without the field are read as rubric version `1`; the file changes on the next
  `archfit baseline` run.

## v1.6.0 CLI redesign

Breaking changes:

- `archfit check` replaces `archfit analyze --gate` and `archfit --gate` for CI
  and agent validation.
- Removed flags:
  - `--gate` → use `archfit check`
  - `--full` → full scan is now the default
  - `--advisory` → advisories are on by default; use `--no-advisories` to hide them
  - `--no-config` → initialize config first with `archfit config init --root .`
- Renamed flags:
  - `--severity` → `--min-severity`
  - `analyze --llm` → `analyze --ai-summary`
  - `explain --llm` → `explain --ai-summary`
  - `config init --llm` → `config init --ai-classify`
  - `config update --llm` → `config update --ai-classify`
  - `--llm-provider` → `--ai-provider`
  - `--llm-model` → `--ai-model`
  - `--no-cache` → `--refresh`

`--refresh` bypasses cache reads but still writes fresh results back to cache.

Release notes for all versions are maintained on GitHub Releases:

<https://github.com/alexei-led/archfit/releases>

The canonical changelog, migration notes, and per-version details are there.

## Next release note draft

Use this entry in the next annotated tag message:

```text
LLM semantic labels and module-role review are now available as an off-gate
workflow.

- `archfit config enrich labels` drafts `.archfit-labels.yaml` entries for
  weak static strength classifications; `archfit config enrich abstained`
  targets cross-module edges whose strength was unknown after all static
  sources and includes source snippets plus model-reported confidence.
- Approved labels are deterministic committed YAML. Drafts are inert, stale
  evidence hashes are ignored with a `labels/stale` advisory, and
  `provenance: llm` labels with medium/low confidence lower
  `coupling_balance` confidence by one band.
- Strength precedence is explicit:
  config-authoritative > compiler-grade > SCIP/heuristic static facts >
  approved LLM label > abstain. LLM labels fill unknown cells; they do not
  override static classifications.
- `archfit config update --ai-classify` proposes review-only module subdomains
  (`core|supporting|generic`), derived volatility, layer suggestions, and
  optional architectural roles, including synthetic module keys, so repos with
  many generated module declarations can review differentiated domain
  volatility without putting model calls on the gate.
```
