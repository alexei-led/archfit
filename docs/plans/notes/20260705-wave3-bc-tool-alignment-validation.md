# Wave 3 Balanced Coupling tool-alignment validation

Date: 2026-07-06
Plan: `docs/plans/completed/20260705-wave3-bc-tool-alignment-improvements.md`
Source review: `docs/archived/reports/book-alignment-review-2026-07-05/00-REVIEW.md`

## Scope

Task 5 validates the completed P1-P3 wave:

- P1: runtime async relation facts stay report-only; non-Go strength refinement uses deterministic SCIP metadata where available; inferred-volatility propagation is transitive under `bc_score.v6`; docs terminology matches code.
- P2: distance reporting now exposes basis/compression, declared external seams stay explicit, clone-only duplicated knowledge remains policy-controlled and score-bearing by default, and tail-risk facts sit beside the mean.
- P3: connascence reporting separates deterministic static evidence from unmeasured static/dynamic categories, and docs separate book-exact behavior, sound adaptations, policy choices, report-only facts, and out-of-scope gaps.

## Validation commands

- `make all`: PASS.
  - Ran `gofmt`, `goimports`, `golangci-lint`, race/coverage tests, SCIP reader tests, build, and the self dogfood gate.
  - Self gate result inside `make all`: PASS, 0 blocking, 81 advisory, 43/100 mixed.
- `make archfit`: PASS.
  - Decision: ACCEPTABLE WITH WATCH ITEMS.
  - Gate: PASS, 0 blocking.
  - Warnings: 81 advisory.
  - Score: 43/100 mixed.
- `make archfit-report`: PASS.
  - Rewrote `docs/reports/archfit-report.md`; no tracked diff remained after generation.
- `.bin/archfit analyze --gate --full --config .archfit.yaml --base origin/main`: PASS.
  - Decision: ACCEPTABLE WITH WATCH ITEMS.
  - Gate: PASS, 0 blocking.
  - Warnings: 81 advisory.
  - Score: 43/100 mixed.
- retired corpus-attribution helper: PASS.
  - Corpus covered Go, Python, Rust, and TypeScript/JavaScript.
  - Skipped corpus repos: none.

## Final self JSON snapshot

`archfit analyze --full --format json --config .archfit.yaml` reported:

- `score_version`: `bc_score.v6`.
- `verdict`: `pass`.
- `coupling_balance`: 43/100, mixed, high confidence.
- Summary: 81 warnings, 0 gate findings.
- `classified_edges.total`: 964.
- `classified_edges.same_module`: 19.
- `classified_edges.scored`: 354.
- `classified_edges.abstained`: 0.
- `classified_edges.external`: 591.
- `classified_edges.clone_only_scored`: 11.
- `classified_edges.connected_modules`: 38.
- `classified_edges.by_distance_basis`: `code_structure=347`, `deploy_unit=7`.
- `classified_edges.distance_compression`: implemented rungs `2, 4, 7, 9, 10`; omitted rungs `1, 3, 5, 6, 8`; compressed middle rungs true.
- `classified_edges.tail_risk`: worst balance 2, lower decile balance 2, critical edges 113, high-or-worse edges 115, distributed-monolith edges 0, clone-only scored 11.
- `classified_edges.volatility_provenance`: `declared=40`, `cascade=1`, `inherited=0`, `undeclared=0`.
- `runtime_async`: 1 module rollup.
- `runtime_async_edges`: 1 source-module to runtime-target relation.
- `connascence.total_evidence`: 3603.
- `connascence.roadmap`: deterministic static for name/type/meaning/algorithm; position remains unmeasured static; execution/timing/value/identity remain unmeasured dynamic with `dynamic_imports` and `runtime_async_edges` listed only as related signals.

## Corpus attribution

Final snapshot source: retired local corpus-attribution helper over external checkouts.

| repo      | language      | final score | final band | scored | abstained | external |
| --------- | ------------- | ----------: | ---------- | -----: | --------: | -------: |
| archfit   | Go            |          43 | mixed      |    354 |         0 |      591 |
| ccgram    | Python        |          55 | mixed      |    497 |        18 |        0 |
| herdr     | Rust          |          29 | poor       |    686 |        18 |       24 |
| storybook | TypeScript/JS |          47 | mixed      |    330 |         0 |      181 |

Movement notes:

- No corpus band moved during the final v6 validation.
- The archfit self score stayed 43/mixed/high. Scored edge count is 354 and external count is 591 after the wave's own code growth and report-only fact additions.
- `ccgram` and `herdr` are unchanged from the prior deterministic attribution snapshots.
- `storybook` remains mixed. The v6 transitive volatility cascade lowers its score relative to the v5 clone-only attribution snapshot, which is the intentional score-versioned semantic change documented in `docs/design/20260705-bc-score-v6.md`.

## Docs and baseline state

Updated or verified docs:

- `docs/plans/notes/20260705-wave3-bc-tool-map.md`: records the final P1-P3 disposition and remaining review-only decisions.
- `docs/design/20260705-bc-score-v6.md`: records the score-version decision and corpus attribution for the transitive volatility cascade.
- `docs/design/bc-measurement-v4.md`: describes runtime async evidence, distance compression, tail-risk reporting, connascence roadmap, and the v6 score story.
- `docs/guide/concepts.md`: separates book-exact facts, sound adaptations, policy choices, report-only data, and out-of-scope gaps.
- `docs/guide/metrics.md`: documents `bc_score.v6`, distance basis/compression, tail risk, connascence, and runtime async fields.
- `docs/guide/commands.md` and `docs/guide/llm-enrich.md`: keep LLM use off-gate and reviewable.

Baseline/config decisions:

- `.archfit-baseline.json` was not changed in Task 5.
- `.archfit.yaml` was not changed in Task 5.
- The self dogfood gate passed without a baseline edit.
- `bc_score.v6` is a deliberate score-version change. Operators should re-run `archfit baseline` only after accepting v6 as the new anchor for their own configured gates.

Manual checks:

- No remaining guide/design `TODO`, `TBD`, `future work`, or `research note` markers were found.
- Runtime coupling, dynamic connascence, LLM draft labels, and unmeasured distance rungs are documented as report-only, review-only, or out of scope rather than silently scored.
- The guide docs read as a tool manual: command behavior, JSON/Markdown fields, policy defaults, and score-version caveats are stated directly.

## Scoped re-review handoff

Recommended follow-up review scope:

- `internal/model/coupling/*`
- `internal/classify/*`
- `internal/engine/*`
- `internal/score/*`
- `internal/model/diagnostic/*`
- `cmd/archfit/*` render paths
- `docs/guide/concepts.md`
- `docs/guide/metrics.md`
- `docs/guide/commands.md`
- `docs/guide/llm-enrich.md`
- `docs/design/bc-measurement-v4.md`
- `docs/design/20260705-bc-score-v6.md`

Review focus:

- Score-version rationale is sound and no score-changing semantics are undocumented.
- Runtime async and dynamic connascence remain report-only and cannot influence deterministic gates.
- LLM drafts remain inert until reviewed and pinned through config/labels.
- Corpus score movement is explained by documented v5/v6 semantics.
- Markdown and JSON output fields match the guide/design descriptions.

Status: recorded for handoff; not run as part of this deterministic validation task.
