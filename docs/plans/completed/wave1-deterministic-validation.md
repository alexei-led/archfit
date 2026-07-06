# Wave 1 deterministic validation

Source plan: `docs/plans/completed/20260705-wave1-deterministic-book-fidelity.md`.
Source review: `docs/archived/reports/book-alignment-review-2026-07-05/00-REVIEW.md`.

## Validation commands

- `gitnexus detect-changes --scope all`: PASS.
  - Before Task 5 edits: no changes detected.
  - After Task 5 edits: low risk, 7 changed files, no affected processes reported.
- `make all`: PASS on final rerun after docs/plan edits.
  - First run failed because the byte-identical Go fixture baselines did not yet include the Task 4 `classified_edges.by_distance_basis`, `classified_edges.connected_modules`, `classified_edges.distance_compression`, and confidence-cap evidence fields.
  - Regenerated and reviewed `internal/extract/golang/testdata/single-module/baseline.json` and `internal/extract/golang/testdata/one-member-workspace/baseline.json`, then reran successfully.
- `.bin/archfit analyze --gate --full --config .archfit.yaml --base origin/main`: PASS.
  - Decision: ACCEPTABLE WITH WATCH ITEMS.
  - Gate: PASS, 0 blocking.
  - Warnings: 81 advisory.
  - `coupling_balance`: 43/100, mixed.
- retired corpus-attribution helper: PASS.
  - Corpus covered Go, Python, Rust, and TypeScript/JavaScript.
  - Skipped corpus repos: none.
- `.bin/archfit analyze --full --markdown --config .archfit.yaml > /tmp/archfit-wave1-report.md`: PASS.
  - Generated Markdown report at `/tmp/archfit-wave1-report.md`.

## Final self JSON snapshot

`archfit analyze --full --format json --config .archfit.yaml` reported:

- `score_version`: `bc_score.v5`.
- `coupling_balance`: 43/100, mixed, high confidence.
- `classified_edges.scored`: 350.
- `classified_edges.abstained`: 0.
- `classified_edges.external`: 572.
- `classified_edges.clone_only_scored`: 11.
- `classified_edges.clone_only_advisory`: 0.
- `classified_edges.connected_modules`: 38.
- `classified_edges.by_distance_basis`: `code_structure=346`, `deploy_unit=4`.
- `classified_edges.distance_compression`: compressed middle rungs true; implemented rungs 2, 4, 7, 9, 10; omitted rungs 1, 3, 5, 6, 8.

## Corpus attribution

Baseline is Task 1 from `docs/plans/completed/wave1-deterministic-baseline.md`.
Final is the Task 5 retired corpus-attribution helper run.

| repo      | language      | baseline score/band/scored/abstained/external | final score/band/scored/abstained/external | movement                                                                                                                                                                                                  |
| --------- | ------------- | --------------------------------------------- | ------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| archfit   | Go            | 42 / mixed / 337 / 0 / 572                    | 43 / mixed / 350 / 0 / 572                 | +1 score, +13 scored, band unchanged, abstained unchanged, external unchanged. Final self has 11 clone-only scored pairs; the remaining scored-count movement is normal self-code growth during the wave. |
| ccgram    | Python        | 55 / mixed / 497 / 18 / 0                     | 55 / mixed / 497 / 18 / 0                  | No score, band, scored, abstained, or external movement.                                                                                                                                                  |
| herdr     | Rust          | 29 / poor / 686 / 18 / 24                     | 29 / poor / 686 / 18 / 24                  | No score, band, scored, abstained, or external movement.                                                                                                                                                  |
| storybook | TypeScript/JS | 48 / mixed / 310 / 0 / 181                    | 49 / mixed / 330 / 0 / 181                 | +1 score, +20 scored, band unchanged, abstained unchanged, external unchanged. Final corpus JSON has 20 clone-only scored pairs.                                                                          |

No corpus band moved. The only score movements are the deliberate `bc_score.v5` clone-only duplicated-knowledge policy effects in archfit and storybook. The confidence-cap and distance-compression work is visible in JSON/Markdown evidence and does not change score values or gate verdicts by itself.

## Docs and baseline state

Updated or verified docs:

- `docs/guide/configuration-reference.md`: documents `coupling.duplicated_knowledge: score|advisory`, the default score policy, and clone-only JSON counters.
- `docs/guide/metrics.md`: documents `bc_score.v5`, clone-only denominator behavior, confidence caps, distance basis/compression fields, and report-only connascence.
- `docs/guide/concepts.md`: maps scored, report-only, semantic-only, and unmeasured book concepts.
- `docs/design/bc-measurement-v4.md`: design v5.0 now records clone-only scoring, deterministic distance compression, sample-size confidence caps, and the final self scorecard.
- `docs/archived/design/20260705-bc-score-v5.md`: records the score-version policy decision and points to this final validation note for the post-wave attribution snapshot.

Baseline state:

- `.archfit-baseline.json` was not changed in Task 5.
- `.archfit.yaml` was not changed in Task 5.
- The self gate passed without a baseline edit. `ScoreVersion` moved to `bc_score.v5` in Task 2; operators should re-run `archfit baseline` only when their configured gates need a new accepted anchor.

## Scoped architecture-review follow-up

Record this follow-up after the deterministic wave lands:

- Scope:
  - `internal/model/coupling/*`
  - `internal/classify/*`
  - `internal/engine/*`
  - `internal/score/*`
  - `internal/model/diagnostic/*`
  - `cmd/archfit/*` render paths
- Review focus:
  - score-version rationale is sound;
  - LLM-free gate boundary still holds;
  - corpus score movement is explained;
  - Ch6/Ch10 docs match JSON and Markdown output.
- Status: recorded for handoff; not run as part of this deterministic validation task.
