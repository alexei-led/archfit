# Plan: Archfit Balanced Coupling Evidence Upgrades

## Overview

Improve archfit’s practical coverage of Vlad Khononov’s Balanced Coupling model without changing the Ch10 scoring formula, ordinals, or deterministic gate boundary.

This plan addresses five issues:

1. Distance signal is weak in one-owner repos.
2. Rust can be degenerate unless `cargo-modules` and SCIP are enabled.
3. TypeScript strength precision is coarse without SCIP overlay visibility.
4. Python strength precision is weak without SCIP overlay visibility.
5. Dynamic connascence is roadmap-only and needs deterministic report-only evidence.

Ralphex format note: this plan uses `### Task N:` task headers and task-local checkboxes only. Top-level validation commands are intended to run after each task.

## Source Artifacts

- Current implementation evidence:
  - `internal/model/coupling/scorer_book.go` — book formula and ordinals.
  - `internal/model/coupling/scorer.go` — `ScoreVersion`, `ScoreBand`.
  - `internal/score/score_boundary_coupling.go` — `coupling_balance` aggregation.
  - `internal/classify/classify.go` — strength, distance, volatility classification.
  - `internal/extract/rust/`, `internal/extract/scip/`, `internal/extract/ts/`, `internal/extract/py/`.
  - `internal/model/diagnostic/diagnostic.go` — report-only runtime and connascence blocks.
  - `cmd/archfit/pipeline_run.go` — SCIP opt-in and deterministic gate boundary.
- Advisor result:
  - `.pi-subagents/artifacts/ecb12cfc_cc-thingz.advisor_0_output.md`
- Ralphex docs:
  - plans are markdown files in `docs/plans/`;
  - task headers must be `### Task N:`;
  - validation commands run after each task;
  - command: `ralphex docs/plans/feature.md`.

## Guardrails

Do not change these unless a later design explicitly approves it:

- No scorer rewrite.
- No ordinal tuning.
- No one-owner distance inflation.
- No LLM path in the deterministic gate.
- No SCIP graph replacement for dependency-cruiser or grimp.
- No scoring of runtime/dynamic connascence yet.
- No fabricated Rust strength from `cargo metadata`.

## Selected Solutions

- **Rust:** make generated Rust configs non-degenerate by emitting explicit deep-analysis analyzers for Rust projects: `analyzers.cargo_modules.enabled: true` and `analyzers.scip.enabled: true`. Do not change no-config defaults.
- **TypeScript:** keep dependency-cruiser as graph/unresolved source; use SCIP only as semantic strength/connascence overlay and make overlay hit rate visible.
- **Python:** keep grimp as graph source; use SCIP only when symbol evidence proves strength; keep non-private grimp imports abstaining.
- **Distance:** improve distance-context reporting and deterministic config hints; do not alter Ch8/Ch10 distance ordinals.
- **Dynamic connascence:** add deterministic report-only signals using runtime async and dynamic import evidence; keep dynamic Ch6 categories unmeasured unless runtime trace evidence exists.

## Validation Commands

Run these after each task unless the task lists a narrower command first.

```sh
make build
make test
python3 internal/extract/scip/scip_reader_test.py
.bin/archfit doctor
.bin/archfit analyze --full --json --config .archfit.yaml > /tmp/archfit-bc-after.json
.bin/archfit analyze --gate --full --config .archfit.yaml
```

## Success Criteria

- `ScoreDefinition` still says `balance = max(|S−D|, 10−V) + 1`.
- `BookScorer` ordinals remain unchanged.
- Go compiler-grade strength is still not overwritten by SCIP.
- TS/Py/Rust SCIP overlay is observable by language and by applied/missed count.
- Rust generated config avoids degenerate single-crate analysis when tools are installed.
- One-owner distance output is clear: low socio-technical distance, not missing signal.
- Runtime/dynamic connascence appears only in report-only output.
- Existing archfit self-gate remains pass.

## Implementation Steps

### Task 1: Lock scorer and non-contamination guardrails

Justification:
The advisor rejected scorer rewrites. The book alignment depends on Ch10 remaining exact and report-only facts staying out of score/gate paths.

Options considered:

- Tune ordinals to improve self-score: rejected.
- Add report-only evidence with regression tests: selected.
- Bump `ScoreVersion` for reporting-only changes: rejected unless classification inputs change.

Files:

- `internal/model/coupling/scorer_book.go`
- `internal/model/coupling/scorer_book_test.go`
- `internal/model/coupling/scorer.go`
- `internal/score/score_boundary_coupling.go`
- `internal/score/score_test.go`
- `internal/engine/assemble_test.go`
- `cmd/archfit/pipeline_run.go`

Metrics to preserve:

- `coupling_balance.value`
- `coupling_balance.band`
- `classified_edges.scored`
- `classified_edges.abstained`
- `classified_edges.external`
- gate verdict

Impact commands:

- `gitnexus detect-changes --scope all || git diff --name-only`
- `gitnexus impact BookScorer --direction upstream || true`
- `gitnexus impact couplingBalance --direction upstream || true`

Verification commands:

```sh
go test ./internal/model/coupling ./internal/score ./internal/engine ./cmd/archfit
.bin/archfit analyze --gate --full --config .archfit.yaml
```

Manual checks:

- Confirm no LLM imports entered `internal/*` gate path.
- Confirm `ScoreVersion` is unchanged unless task output proves a scoring semantic changed.

- [x] Add or strengthen tests pinning `BookScorer` ordinals and formula.
- [x] Add or strengthen tests proving same-module/local/runtime/report-only facts do not change score or verdict.
- [x] Add a regression test proving SCIP does not override Go type-info strength.
- [x] Add a regression test proving disabled SCIP produces explicit disabled coverage, not a silent miss.
- [x] Run task verification commands.
- [x] Mark completed.

### Task 2: Make Rust generated configs non-degenerate

Justification:
Rust crate-level extraction can make single-crate repos degenerate. `cargo-modules` adds intra-crate module graph. SCIP adds semantic strength. The safest fix is generated explicit config for Rust projects, not global default behavior.

Options considered:

- Globally enable `cargo-modules` and SCIP by default: rejected; slow and toolchain-dependent.
- Infer functional strength from `cargo metadata`: rejected; manifest dependencies are not symbol facts.
- Emit explicit deep Rust analyzers in `config init/update` for Rust projects: selected.
- Only document the issue: rejected; too passive.

Files:

- `internal/config/tools.go`
- `internal/config/views.go`
- `internal/config/config_test.go`
- `cmd/archfit/init.go`
- `cmd/archfit/update.go`
- `cmd/archfit/update_test.go`
- `internal/initcfg/`
- `internal/extract/rust/rust.go`
- `internal/extract/rust/modules.go`
- `internal/extract/rust/modules_test.go`
- `internal/extract/scip/scip_strength.go`
- `docs/guide/languages.md`
- `docs/guide/configuration-reference.md`

Expected config behavior:
For projects with `Cargo.toml` and Rust enabled, generated or updated config should include:

```yaml
languages:
  rust:
    enabled: auto

analyzers:
  cargo_modules:
    enabled: true
  scip:
    enabled: true
```

Do not change `config.Default()` no-config behavior.

Metrics to consume:

- `tool_coverage` row for `cargo`
- `tool_coverage` row for `cargo-modules`
- `tool_coverage` row for `scip`
- `tool_coverage` row for `scip-symbols`
- `classified_edges.scored`
- `classified_edges.abstained`
- connected module count / `blast_radius` n/a status

Impact commands:

- `gitnexus impact Config.ForExtract --direction upstream || true`
- `gitnexus impact CargoModulesEnabled --direction upstream || true`
- `gitnexus impact ScipEnabled --direction upstream || true`
- `gitnexus detect-changes --scope all || git diff --name-only`

Verification commands:

```sh
go test ./internal/config ./cmd/archfit ./internal/initcfg ./internal/extract/rust ./internal/extract/scip
```

Manual checks:

- Confirm absent `cargo-modules` remains a coverage gap, not a hard crash.
- Confirm explicitly generated config is deterministic across machines.

- [x] Add Rust-project detection in config init/update if not already present.
- [x] Make Rust init/update emit explicit `cargo_modules` and `scip` analyzer blocks for Rust projects.
- [x] Keep no-config/default behavior unchanged.
- [x] Add tests for generated Rust config with `Cargo.toml`.
- [x] Add tests for missing `cargo-modules` and missing SCIP coverage behavior.
- [x] Update language/config docs with Rust deep-analysis recommendation and cost warning.
- [x] Run task verification commands.
- [x] Mark completed.

### Task 3: Make SCIP semantic-strength overlay reliable and visible for TS/Py/Rust

Justification:
dependency-cruiser and grimp are the right graph sources, but they cannot always prove model vs functional strength. SCIP can refine strength, but key mismatches can silently under-apply overlays.

Options considered:

- Replace dependency-cruiser/grimp graphs with SCIP graphs: rejected; graph coverage and unresolved honesty would degrade.
- Trust SCIP silently: rejected; users need overlay coverage/hit rate.
- Keep graph extractors and add visible SCIP overlay stats: selected.

Files:

- `internal/engine/engine.go`
- `internal/engine/assemble.go`
- `internal/engine/assemble_test.go`
- `internal/model/diagnostic/diagnostic.go`
- `internal/extract/scip/scip_strength.go`
- `internal/extract/scip/scip_reader.py`
- `internal/extract/scip/scip_strength_test.go`
- `internal/extract/ts/ts.go`
- `internal/extract/ts/ts_test.go`
- `internal/extract/py/py.go`
- `internal/extract/py/py_test.go`
- `internal/output/jsonout/`
- `internal/output/markdown/`

Expected output change:
Add a report-only semantic overlay summary, for example:

```json
"semantic_strength_overlay": {
  "by_language": {
    "typescript": {"candidate_edges": 120, "applied": 88, "missed": 32},
    "python": {"candidate_edges": 40, "applied": 12, "missed": 28},
    "rust": {"candidate_edges": 30, "applied": 30, "missed": 0}
  }
}
```

Exact shape may differ, but it must be:

- deterministic;
- report-only;
- visible in JSON and markdown;
- not used by gates directly.

Metrics to consume:

- `dependency-cruiser.unresolved / specifiers_seen`
- `grimp.unresolved`
- SCIP coverage status
- SCIP overlay applied count
- SCIP overlay miss count
- strength distribution before/after overlay
- `classified_edges.abstained`

Impact commands:

- `gitnexus impact enrichEdges --direction upstream || true`
- `gitnexus impact parseReaderEdges --direction upstream || true`
- `gitnexus impact parseReaderConnascence --direction upstream || true`
- `gitnexus detect-changes --scope all || git diff --name-only`

Verification commands:

```sh
go test ./internal/engine ./internal/extract/scip ./internal/extract/ts ./internal/extract/py ./internal/output/jsonout ./internal/output/markdown
python3 internal/extract/scip/scip_reader_test.py
```

Manual checks:

- Confirm Go strength stays compiler-grade.
- Confirm unresolved TS imports still cap confidence and remain disclosed.
- Confirm Python non-private imports still abstain unless SCIP proves a strength.

- [x] Add per-language SCIP overlay counters in the engine or diagnostic assembly.
- [x] Add tests for successful TS overlay key matching.
- [x] Add tests for successful Python dotted-module overlay key matching.
- [x] Add tests for successful Rust module/crate overlay key matching.
- [x] Add tests for missed overlay keys being counted, not silently ignored.
- [x] Add output rendering for overlay stats in JSON and markdown.
- [x] Run task verification commands.
- [x] Mark completed.

### Task 4: Improve distance meaning without distance inflation

Justification:
One-owner repos legitimately have low socio-technical distance. The fix is clearer distance context and deterministic hints, not changing distance ordinals.

Options considered:

- Reweight `cross_module_same_owner`: rejected; violates book ordinals.
- Treat same-owner repos as different-owner to create stronger signal: rejected; false evidence.
- Expose owner degeneracy and distance basis clearly: selected.
- Add deploy/external hints only when deterministic evidence exists: selected.

Files:

- `internal/classify/classify.go`
- `internal/classify/distance_structure.go`
- `internal/classify/distance_structure_test.go`
- `internal/classify/external_systems.go`
- `internal/extract/deployunit/deployunit.go`
- `internal/engine/assemble.go`
- `internal/engine/assemble_test.go`
- `internal/model/diagnostic/diagnostic.go`
- `internal/output/markdown/`
- `internal/output/jsonout/`
- `cmd/archfit/update.go`
- `cmd/archfit/init.go`
- `.archfit.yaml`
- `docs/guide/metrics.md`
- `docs/guide/configuration-reference.md`

Expected output change:
Add or strengthen a distance-context report, for example:

```json
"distance_context": {
  "owner_model": "single_owner_degenerate",
  "distance_basis": {
    "code_structure": 300,
    "ownership": 0,
    "deploy_unit": 0,
    "declared_external": 0
  },
  "interpretation": "same-owner is the lowest cross-module distance; this is a low socio-technical distance signal, not missing ownership"
}
```

Metrics to consume:

- `classified_edges.by_distance`
- `classified_edges.by_distance_basis`
- `distance_compression` / omitted rung reasons if present
- owner source/degradation warnings
- deploy-unit detected module count
- declared external count

Impact commands:

- `gitnexus impact classifyDistance --direction upstream || true`
- `gitnexus impact moduleDistance --direction upstream || true`
- `gitnexus impact Detect --direction upstream --repo archfit || true`
- `gitnexus detect-changes --scope all || git diff --name-only`

Verification commands:

```sh
go test ./internal/classify ./internal/extract/deployunit ./internal/engine ./internal/output/jsonout ./internal/output/markdown ./cmd/archfit
.bin/archfit analyze --full --json --config .archfit.yaml > /tmp/archfit-distance-after.json
```

Manual checks:

- Confirm `.archfit.yaml` still says single-owner same-owner distance is intentional.
- Confirm no ordinal or formula code changed.
- Confirm external systems remain review/config based, not inferred active config.

- [ ] Add or strengthen owner-degeneracy/distance-basis reporting.
- [ ] Improve text/markdown explanation for single-owner repositories.
- [ ] Add deterministic config-update hints for `deploy_unit` when deployunit detector has evidence.
- [ ] Keep `external_systems` suggestions review-only unless explicitly applied by a human.
- [ ] Add tests for one-owner repo distance interpretation.
- [ ] Add tests proving deploy-unit evidence can raise distance only when deterministic.
- [ ] Run task verification commands.
- [ ] Mark completed.

### Task 5: Add dynamic connascence signals as report-only evidence

Justification:
Ch6 dynamic connascence is not deterministically measured today. Static async/import evidence can help humans, but scoring it now would overclaim. The safe fix is a report-only signal block with explicit unmeasured categories.

Options considered:

- Score dynamic connascence now: rejected; insufficient deterministic runtime proof.
- Add runtime tracing requirement now: rejected; too much scope and environment-specific.
- Reuse runtime async and dynamic import evidence as report-only signals: selected.

Files:

- `internal/model/coupling/coupling.go`
- `internal/model/diagnostic/diagnostic.go`
- `internal/extract/runtime/runtime.go`
- `internal/extract/runtime/runtime_test.go`
- `internal/extract/dynimports/dynimports.go`
- `internal/extract/dynimports/dynimports_test.go`
- `internal/engine/assemble.go`
- `internal/engine/engine_test.go`
- `internal/output/jsonout/`
- `internal/output/markdown/`
- `docs/guide/metrics.md`

Expected output change:
Add a report-only dynamic connascence signal block, for example:

```json
"dynamic_connascence_signals": {
  "signals": [
    {
      "kind": "runtime_async",
      "related_connascence": ["execution", "timing"],
      "measured": false,
      "module": "internal/example",
      "count": 3
    }
  ],
  "unmeasured": ["execution", "timing", "value", "identity"]
}
```

Exact shape may differ, but it must state:

- signal source;
- related Ch6 category;
- whether it is measured;
- why it is report-only;
- sites or modules involved.

Metrics to consume:

- `runtime_async_edges`
- `runtime_async_modules`
- `runtime_async_sites`
- dynamic import sites
- `connascence.unmeasured`
- score/verdict before/after report-only block

Impact commands:

- `gitnexus impact ConnascenceReport --direction upstream || true`
- `gitnexus impact RuntimeAsyncEdge --direction upstream || true`
- `gitnexus impact unmeasuredConnascenceKinds --direction upstream || true`
- `gitnexus detect-changes --scope all || git diff --name-only`

Verification commands:

```sh
go test ./internal/extract/runtime ./internal/extract/dynimports ./internal/engine ./internal/output/jsonout ./internal/output/markdown
.bin/archfit analyze --full --json --config .archfit.yaml > /tmp/archfit-dynamic-connascence-after.json
```

Manual checks:

- Confirm output does not imply dynamic connascence is fully measured.
- Confirm dynamic signals do not change `coupling_balance`, verdict, or gate findings.

- [ ] Define a diagnostic model for dynamic connascence signals.
- [ ] Assemble runtime async and dynamic import facts into the new report-only block.
- [ ] Preserve `ConnascenceReport.Unmeasured` for execution/timing/value/identity unless true runtime trace evidence exists.
- [ ] Render the report in JSON.
- [ ] Render a concise markdown section.
- [ ] Add non-contamination tests proving score and verdict are unchanged by these report-only facts.
- [ ] Run task verification commands.
- [ ] Mark completed.

### Task 6: Final verification, documentation, and re-review

Justification:
The preceding tasks change architecture evidence surfaces. The final task proves the gate is deterministic, docs match behavior, and the plan is ready for architecture re-review.

Files:

- `README.md`
- `CLAUDE.md`
- `docs/guide/metrics.md`
- `docs/guide/languages.md`
- `docs/guide/configuration-reference.md`
- `docs/guide/llm-enrich.md`
- all files changed by Tasks 1–5

Metrics to record:

- `coupling_balance.value`
- `coupling_balance.band`
- `classified_edges.scored`
- `classified_edges.abstained`
- `classified_edges.external`
- Rust `cargo-modules` and SCIP coverage
- TS/Py SCIP overlay hit/miss counts
- distance basis distribution
- dynamic connascence signal counts

Impact commands:

- `gitnexus detect-changes --scope all || git diff --name-only`
- `git diff --stat`
- `git diff --name-only`

Verification commands:

```sh
make build
make test
python3 internal/extract/scip/scip_reader_test.py
.bin/archfit doctor
.bin/archfit analyze --full --json --config .archfit.yaml > /tmp/archfit-final.json
.bin/archfit analyze --gate --full --config .archfit.yaml
```

Manual checks:

- Compare `/tmp/archfit-final.json` with the pre-plan baseline.
- Confirm score changes are explainable by better evidence, not formula changes.
- Confirm docs warn about Rust deep-analysis runtime cost.
- Confirm no report-only block feeds the gate.

- [ ] Run full validation commands.
- [ ] Update docs for Rust deep analysis, SCIP overlay semantics, distance context, and dynamic connascence signal limits.
- [ ] Record before/after metric deltas in the final commit message or PR body.
- [ ] Confirm `git diff --name-only` matches expected files.
- [ ] Run `ralphex --review docs/plans/20260707-archfit-bc-evidence-upgrades.md` after task completion if full ralphex review was skipped.
- [ ] Mark completed.

## Acceptance Criteria

- All validation commands pass.
- `BookScorer` formula and ordinals are unchanged.
- `ScoreVersion` is unchanged unless a scoring semantic, not report-only output, changed and was explicitly justified.
- Generated Rust config enables deep Rust analysis for Rust projects without changing no-config defaults.
- SCIP overlay stats are visible for TS/Py/Rust and tested.
- dependency-cruiser and grimp remain graph/unresolved authorities.
- One-owner distance reports are clearer and do not inflate distance.
- Dynamic connascence signals are report-only and explicitly marked unmeasured where appropriate.
- `.bin/archfit analyze --gate --full --config .archfit.yaml` passes.

## Safety Notes

- `cargo-modules` and SCIP can be slow. Keep them explicit in generated config and documented as deep analysis.
- Do not make PATH-dependent analyzer auto-enablement affect deterministic gate semantics.
- Do not infer external systems as active config from LLM or weak static evidence.
- Do not convert runtime async or dynamic imports into score inputs.
- If a task changes the score, explain which new deterministic evidence entered `classified_edges`; do not hide it as a rendering-only change.

## Re-review Recommendation

After implementation, run a scoped architecture review focused on:

- scorer non-contamination;
- analyzer coverage honesty;
- TS/Py/Rust semantic-strength overlay correctness;
- distance reporting clarity;
- dynamic connascence report-only boundary.
