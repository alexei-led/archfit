# New Wave 3: P1-P3 book-alignment tool improvements

> **Executable ralphex plan.** Run with `ralphex docs/plans/20260705-wave3-bc-tool-alignment-improvements.md` from the repo root. ralphex executes one `### Task N:` section at a time. This wave is off-gate by design until a task explicitly changes scoring semantics and the change is validated/rebaselined.
>
> Deterministic evidence first. Use syntax/search/graph tools first. Use `--llm` only when a gap is semantic and the deterministic tool path cannot close it, and only to draft reviewable labels/config/docs that are pinned before they influence deterministic runs.

## Overview

Make archfit a better Balanced Coupling tool, not just a better scorer.

The current review shows a strong book-formula core, but the tool still under-measures or coarsens several book concepts:

- runtime/lifecycle coupling is report-only,
- distance is compressed to a few rungs,
- non-Go strength semantics remain approximate,
- inferred volatility propagation is shallow,
- dynamic connascence is mostly unmeasured,
- some book/policy terminology still drifts in docs.

This wave closes the highest-value gaps first, then improves report fidelity, then tightens the book-alignment framing.

## Source artifact

Primary source: `reports/book-alignment-review-2026-07-05/00-REVIEW.md`.

Key review refs used by this plan:

- Book core is strong: formula, ordinals, abstain discipline, and the static strength/distance/volatility vocabulary are mostly aligned.
- P1 gap: runtime coupling is still report-only; non-Go strength semantics are tool-limited; volatility propagation is single-hop; docs/terminology drift remains.
- P2 gap: distance rungs are compressed; declared external seam handling is intentionally narrow; clone-only duplicated knowledge and tail-risk reporting need sharper presentation.
- P3 gap: dynamic connascence is still mostly report-only, and the report should separate book-exact behavior, sound adaptation, and policy choices.

Current implementation anchors:

- `internal/classify/classify.go`
- `internal/classify/distance_structure.go`
- `internal/classify/clone_only.go`
- `internal/model/coupling/scorer.go`
- `internal/model/coupling/scorer_book.go`
- `internal/score/score_boundary_coupling.go`
- `internal/engine/assemble.go`
- `internal/model/diagnostic/diagnostic.go`
- `internal/extract/*`
- `cmd/archfit/*`
- `docs/guide/concepts.md`
- `docs/guide/metrics.md`
- `docs/design/bc-measurement-v4.md`
- `docs/design/20260705-bc-score-v5.md`

## Current state snapshot

- Score version: `bc_score.v6`
- Current self score: `43 / mixed / high confidence`
- Distance basis currently implemented: `code_structure`, `ownership`, `deploy_unit`, `declared_external`
- Distance rungs currently disclosed: `2, 4, 7, 9, 10`
- `runtime_async` and `runtime_async_edges` are report-only
- non-Go strength refinement uses deterministic SCIP symbol-kind metadata where available; built-in TS/Python extractors stay conservative when object kinds are absent
- inferred volatility cascade is transitive/fixpoint when `coupling.volatility_cascade: true`
- `connascence` is partially measured and explicitly discloses unmeasured categories
- clone-only duplicated knowledge is score-bearing by default

## Success criteria

- P1 closes the biggest book-alignment gaps with deterministic evidence where possible, and uses LLM only for semantic gaps that syntax/graph tools cannot settle.
- P2 improves distance fidelity, seam handling, clone-only presentation, and tail-risk reporting without guessing new facts.
- P3 makes dynamic connascence and book-alignment framing honest: what is exact, what is adapted, what is policy, and what remains out of scope.
- Any score-changing semantics get tests, docs, and a deliberate validation/rebaseline step.
- Deterministic gate behavior stays reproducible.

## Tool policy

1. Start with local code search, graph, and tests.
2. Use `ctx_search` / `ctx_execute_file` for book excerpts and large outputs.
3. Use syntax and extractor evidence first (`internal/extract/*`, `ast-grep`, `SCIP`, graph facts, tests).
4. Use `--llm` only when a gap is semantic and deterministic tools cannot decide it.
5. LLM output must stay off-gate and reviewable: draft labels/config/docs/explanations only.
6. If a new value is still ambiguous after deterministic and LLM review, stop and ask the user before coding.

## Decisions and open questions

Decision: runtime coupling is part of **distance** in the Balanced Coupling model, but archfit must not score the current module-level `runtime_async` rollups. The selected approach is **hybrid**:

- keep runtime coupling report-only now,
- build deterministic edge-level or module-pair runtime facts first,
- score runtime distance later only for high-confidence relationships,
- never infer synchronous coupling from missing async evidence.

Open question:

- If a proposed semantic label cannot be pinned confidently, do we keep it out of scoring entirely or allow a documented temporary draft path?

## Validation Commands

- `make build`
- `make test`
- `make lint`
- `make archfit`
- `make archfit-report`
- `go test ./internal/... ./cmd/...`
- `.bin/archfit analyze --full --json --config .archfit.yaml > /tmp/archfit-wave3.json`
- `.bin/archfit analyze --full --json --llm --config .archfit.yaml > /tmp/archfit-wave3-llm.json`
- `ATTRIB_REPOS_DIR=${ATTRIB_REPOS_DIR:-$HOME/Workspace} make corpus-attrib` (only if a task changes score semantics)

## Implementation Steps

### Task 1: Analyze current state and close the tool/map gap

Justification: Before changing behavior, we need a current-state map that separates deterministic measurement gaps from semantic judgment gaps and identifies the right tool or LLM path for each missing value.

What this task does:

- Re-read the book sections that matter most for the current gaps (Ch6–Ch13 and numeric appendix anchors).
- Inspect the current code paths for strength, distance, volatility, connascence, scoring, and report-only blocks.
- Map each P1-P3 gap to:
  - deterministic source / tool,
  - semantic source / tool,
  - whether `--llm` is needed,
  - what exact value/metric needs to be calculated.
- Decide which gaps need an explicit user choice before coding.
- Produce a short plan note with any unresolved design questions.

Files / areas:

- `reports/book-alignment-review-2026-07-05/00-REVIEW.md`
- `.book/9780137353576.epub`
- `docs/guide/concepts.md`
- `docs/guide/metrics.md`
- `docs/design/bc-measurement-v4.md`
- `docs/design/20260705-bc-score-v5.md`
- `internal/classify/*`
- `internal/model/coupling/*`
- `internal/score/*`
- `internal/engine/*`
- `internal/extract/*`

Evidence strategy:

- Deterministic first: `rg`, `read`, `ctx_search`, `ctx_execute_file`, `gitnexus_context`, `gitnexus_impact`.
- Semantic only if necessary: draft an off-gate `--llm` prompt for the specific unresolved label/value.

Success criteria:

- Every P1-P3 gap is tagged as deterministic, semantic, or mixed.
- The likely tool path is explicit for each gap.
- The user-facing open question(s) are identified.

Impact commands:

- `gitnexus impact classify --direction downstream --depth 4`
- `gitnexus impact score --direction downstream --depth 4`
- `gitnexus detect-changes --scope all`

Verification commands:

- `ctx_search` over the book and existing review notes
- `go test ./internal/classify/... ./internal/model/coupling/... ./internal/score/...`

Manual checks:

- Confirm whether a missing value is truly missing or just not surfaced yet.
- Confirm whether each candidate belongs in scoring, report-only, or LLM-draft-only space.

- [x] Re-read the book anchors for Ch6, Ch7, Ch8, Ch9, Ch10, and Ch11.
- [x] Map each gap to deterministic evidence or semantic judgment.
- [x] Decide where `--llm` is actually needed and what prompt/model shape should be used.
- [x] Capture any user decisions needed before code changes.

Task 1 note: `docs/plans/notes/20260705-wave3-bc-tool-map.md`.

### Task 2: Close P1 — runtime coupling, strength semantics, volatility propagation, docs drift

Justification: P1 is the highest-value book-alignment gap set. Runtime coupling is still report-only, non-Go strength semantics are tool-limited, volatility propagation is shallow, and the docs still drift from code.

What this task does:

- Implement the selected hybrid runtime-coupling approach: keep current module-level runtime evidence report-only, then add edge-level/module-pair runtime facts that can safely feed scoring later.
- Do not score runtime coupling yet unless the task first proves deterministic, high-confidence relationship-level evidence and deliberately updates score semantics.
- Improve non-Go strength classification where deterministic evidence is available first.
- If syntax/graph tools cannot settle a semantic strength label, use `--llm` off-gate to draft a reviewed label/config change; pin it before it affects deterministic runs.
- Revisit volatility propagation and move beyond single-hop if the book-alignment review says transitive volatility matters more than the current local approximation.
- Fix docs/terminology drift so the code, review note, and user docs say the same thing.

Files / areas:

- `internal/classify/classify.go`
- `internal/classify/distance_structure.go`
- `internal/classify/volatility_provenance.go`
- `internal/extract/golang/*`
- `internal/extract/ts/*`
- `internal/extract/py/*`
- `internal/extract/scip/*`
- `internal/model/graph/*`
- `internal/model/coupling/*`
- `docs/guide/concepts.md`
- `docs/guide/metrics.md`
- `docs/design/bc-measurement-v4.md`
- `docs/design/20260705-bc-score-v5.md`

Evidence strategy:

- Deterministic first: compare extractor facts, graph facts, and current tests.
- Semantic fallback: use `--llm` only for unresolved model/functional/contract judgments or runtime/lifecycle interpretation.

Success criteria:

- P1 gaps are either implemented or explicitly documented as out of scope.
- No score change happens without tests and an explicit review of the impact.
- Docs no longer contradict code or the current review.

Impact commands:

- `gitnexus impact resolveDistanceVolatility --direction upstream --depth 4`
- `gitnexus impact computeEffectiveVolatility --direction upstream --depth 4`
- `gitnexus detect-changes --scope all`

Verification commands:

- `go test ./internal/classify/... ./internal/extract/... ./internal/model/coupling/... ./internal/score/...`
- `make archfit`
- `make archfit-report`

Manual checks:

- Verify the hybrid runtime-coupling treatment: book-correct conceptually, safe for the current product boundary, and not silently score-changing.
- Verify that any LLM-derived semantic label is reviewable and not silently admitted into the gate.

- [x] Keep runtime coupling report-only for current rollups; add edge-level/module-pair runtime facts as the prerequisite for later scoring.
- [x] Improve non-Go strength semantics with deterministic evidence first, then `--llm` only if needed.
- [x] Improve inferred-volatility propagation if the current single-hop model is too weak.
- [x] Fix terminology/docs drift and update tests.

### Task 3: Close P2 — distance fidelity, external seams, clone-only handling, tail-risk reporting

Justification: P2 improves how well the tool measures and communicates the book’s distance and hidden-coupling ideas without guessing facts.

What this task does:

- Expand distance fidelity only where deterministic facts already support a finer split.
- Keep compressed distance rungs compressed when no stable evidence exists.
- Re-check declared external seams and decide whether their handling should stay narrow or become more explicit in reporting.
- Audit clone-only duplicated knowledge for over/under inclusion and tighten evidence if needed.
- Add tail-risk reporting so a low mean cannot hide a concentrated hot spot.

Files / areas:

- `internal/classify/distance_structure.go`
- `internal/classify/external_systems.go`
- `internal/classify/clone_only.go`
- `internal/engine/assemble.go`
- `internal/engine/advisories.go`
- `internal/score/score_boundary_coupling.go`
- `internal/model/diagnostic/diagnostic.go`
- `docs/guide/metrics.md`
- `docs/design/bc-measurement-v4.md`

Evidence strategy:

- Deterministic only unless a seam classification truly needs semantic judgment.
- If a seam label or clone-only interpretation is not syntax-resolvable, use `--llm` off-gate to draft the decision, then pin it explicitly.

Success criteria:

- Distance stays honest about what is measured vs compressed.
- Tail-risk output makes severe edges visible beside the mean.
- Clone-only handling is either sharpened or explicitly defended.

Impact commands:

- `gitnexus impact classifyDistance --direction downstream --depth 4`
- `gitnexus impact CloneOnlyPairs --direction downstream --depth 4`
- `gitnexus detect-changes --scope all`

Verification commands:

- `go test ./internal/classify/... ./internal/engine/... ./internal/score/...`
- `make archfit`
- `make archfit-report`

Manual checks:

- Ensure no new distance rung is invented from naming taste.
- Ensure clone-only scoring remains book-credible, not clone-noise-driven.

- [x] Add finer distance rungs only when a deterministic split exists.
- [x] Reconfirm declared external seam handling and reporting.
- [x] Tighten clone-only duplicated-knowledge evidence and tail-risk output.
- [x] Update tests and docs for the new reporting shape.

### Task 4: Close P3 — dynamic connascence roadmap and book-alignment framing

Justification: P3 is mostly about honest reporting and clearer separation between book-exact measurements, useful adaptations, and semantic-only material.

What this task does:

- Define which connascence categories are deterministic today and which remain unmeasured.
- Keep dynamic connascence categories report-only unless a deterministic source genuinely appears.
- Use `--llm` only for semantic explanation or draft proposals, never to silently change a gate result.
- Update the book-alignment framing so the tool’s docs clearly separate:
  - book-exact,
  - sound adaptation,
  - policy choice,
  - out of scope.

Files / areas:

- `internal/model/coupling/*`
- `internal/model/diagnostic/*`
- `internal/engine/assemble.go`
- `cmd/archfit/*`
- `docs/guide/concepts.md`
- `docs/guide/metrics.md`
- `docs/guide/commands.md`
- `docs/guide/llm-enrich.md`
- `docs/design/*`

Evidence strategy:

- Deterministic reporting first.
- Semantic framing and narratives can use `--llm`, but only as off-gate drafts/reviews.

Success criteria:

- The docs make it impossible to confuse a report-only concept with a scored one.
- The remaining book gaps are named honestly.
- The user can tell which choices are archfit policy and which are book facts.

Impact commands:

- `gitnexus impact buildConnascenceReport --direction upstream --depth 4`
- `gitnexus impact analyze --direction downstream --depth 3`
- `gitnexus detect-changes --scope all`

Verification commands:

- `go test ./internal/... ./cmd/...`
- `make archfit-report`

Manual checks:

- Read the docs as a new user: it should be obvious what is measured, what is adapted, and what is not measured.

- [ ] Define the dynamic connascence roadmap.
- [ ] Update the book-alignment framing docs.
- [ ] Add or update tests for any new report-only fields.
- [ ] Keep LLM usage clearly off-gate and reviewable.

### Task 5: Validate corpus, refresh notes, and hand off

Justification: This wave changes the tool’s measurement story; final validation needs to prove the new behavior is intentional and reproducible.

What this task does:

- Run the full validation loop.
- Re-run corpus checks when a task changes score semantics.
- Refresh the wave note and any affected docs.
- Confirm the final state is ready for review or commit.

Files / areas:

- `docs/plans/notes/wave1-deterministic-validation.md` or a new wave3 validation note
- `docs/guide/*`
- `docs/design/*`
- `.archfit-baseline.json` only if an intentional score semantic change requires rebaseline
- `.archfit.yaml` only if the self-dogfood gate needs an explicit configuration update

Success criteria:

- `make all` passes.
- `make archfit` passes.
- Any intentional score/version changes are explicitly documented and corpus-attributed.

Impact commands:

- `gitnexus detect-changes --scope all`

Verification commands:

- `make all`
- `make archfit`
- `make archfit-report`
- `ATTRIB_REPOS_DIR=${ATTRIB_REPOS_DIR:-$HOME/Workspace} make corpus-attrib` (if score semantics changed)
- `.bin/archfit analyze --gate --full --config .archfit.yaml --base origin/main`

Manual checks:

- Confirm there are no undocumented semantic changes left.
- Confirm the docs read as a tool manual, not as a research note.

- [ ] Run the final validation suite.
- [ ] Update the validation note and any affected docs.
- [ ] Rebaseline only if a score/version change is intentional and documented.
- [ ] Record the scoped re-review handoff.

## Acceptance criteria

- All P1-P3 gaps are either implemented, explicitly out of scope, or documented as report-only.
- Any semantic change that affects config or scoring has a reviewable `--llm` draft path and tests.
- Deterministic output remains reproducible.
- `make all` and `make archfit` pass at the end of the wave.
