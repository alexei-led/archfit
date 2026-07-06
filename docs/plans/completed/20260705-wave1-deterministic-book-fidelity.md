# New Wave 1: deterministic book-fidelity closure

> **Executable ralphex plan.** Run with `ralphex docs/plans/completed/20260705-wave1-deterministic-book-fidelity.md` from the repo root. ralphex executes one `### Task N:` section at a time. Completed historical waves in `docs/plans/completed/` are context only and do not affect this new wave numbering.
>
> This plan is score-sensitive. Keep one behavior change per commit. If a task changes `coupling_balance` semantics, bump `ScoreVersion`, run attribution, and rebaseline deliberately.

## Overview

Close the remaining deterministic book-alignment gaps from the 2026-07-05 review without using the LLM in the gate. The focus is the book-methodology core: clone-only duplicated knowledge, first-class connascence evidence, confidence honesty, and explicit distance/compression disclosure.

## Source artifact

Primary source: `reports/book-alignment-review-2026-07-05/00-REVIEW.md`.

Source refs used by this plan:

- Report §1 verdict: deterministic Ch7-Ch10 core is strong, but connascence, runtime distance, clone-only duplicated knowledge, and semantic evidence remain incomplete.
- Report §2.1 Ch6 inventory: static/dynamic connascence is missing or only implicit.
- Report §2.2 Ch7 symmetric functional duplication: clone-only pairs are scored as advisories but stay outside `coupling_balance`.
- Report §2.2 Ch8 distance: middle rungs 3-7 are compressed; D=8 is absent unless future facts justify it.
- Report §2.3 Ch10/AppB: stability, cost-of-change, binary modularity/complexity, and global complexity are not first-class outputs.
- Report §3 metric table: `coupling_balance` currently excludes clone-only pairs and does not cap confidence for tiny fully scored samples.
- Report §6 gap list: P1 clone-only score policy, P2 connascence, P2 runtime/distance, P2 confidence, P3 TS diagnostics.
- Existing implementation context: `docs/design/bc-measurement-v4.md`, `docs/plans/completed/20260702-wave4-book-strength-distance.md`, `docs/plans/completed/20260702-wave5-book-quadrants-advisories.md`, `docs/plans/completed/20260702-wave7-llm-semantic-labels.md`.

## Success criteria

- Clone-only duplicated knowledge has an explicit score policy and visible JSON counters; the default v5 policy is intentionally documented and corpus-attributed.
- Static connascence evidence is first-class in JSON/Markdown for deterministic cases; unknown semantic/dynamic connascence is disclosed as unmeasured, not guessed.
- Tiny or under-measured graphs cannot report misleading high confidence; evidence lines explain the cap.
- Distance basis and compressed middle-rung behavior are disclosed. New rungs are added only when backed by stable deterministic facts.
- Whole-plan validation covers at least Go, Python, Rust, and TypeScript corpus repos.
- Docs explain which book concepts are scored, report-only, or semantic-only.

## Validation Commands

- `make build`
- `go test ./internal/classify/... ./internal/engine/... ./internal/model/coupling/... ./internal/model/diagnostic/... ./internal/score/...`
- `go test ./cmd/... ./internal/...`
- `make test`
- `make lint`
- `make archfit`
- `ATTRIB_REPOS_DIR=${ATTRIB_REPOS_DIR:-$HOME/Workspace} make corpus-attrib`
- `.bin/archfit analyze --gate --full --config .archfit.yaml --base origin/main`

## Implementation Steps

### Task 1: Characterize deterministic gap baseline

Justification: Report §6 lists score-affecting gaps; before changing formulas, the current behavior needs locked tests and corpus attribution.

Files:

- `internal/classify/clone_only_test.go` — characterize clone-only pair behavior and label suppression.
- `internal/engine/duplicated_knowledge_test.go` — characterize advisory-only current output.
- `internal/score/score_test.go` — characterize current confidence behavior on tiny/full-score graphs.
- `internal/engine/golden_test.go` — keep byte-identical output guard current.
- `docs/plans/completed/wave1-deterministic-baseline.md` — record pre-change corpus table and current artifacts.

Preconditions: repo builds on current `main`; `.bin/archfit` can analyze self with `.archfit.yaml`.
Postconditions: current clone-only, connascence absence, confidence, and distance evidence are captured before behavior changes.

Fitness gate: Existing self gate is `make archfit`. No new gate yet; this task adds characterization only.

Impact commands:

- `gitnexus impact clone_only.go --direction upstream --depth 3`
- `gitnexus impact score_boundary_coupling.go --direction upstream --depth 3`
- `gitnexus detect-changes --scope all`

Verification commands:

- `go test ./internal/classify/... ./internal/engine/... ./internal/score/...`
- `make build`
- `ATTRIB_REPOS_DIR=${ATTRIB_REPOS_DIR:-$HOME/Workspace} make corpus-attrib`

Manual checks:

- Confirm the baseline note cites `reports/book-alignment-review-2026-07-05/00-REVIEW.md` and records skipped corpus repos, if any.

- [x] Add failing-or-characterization tests for the current advisory-only clone-only policy.
- [x] Add tests showing tiny fully scored graphs currently can appear high confidence.
- [x] Record current self/corpus scores in `docs/plans/completed/wave1-deterministic-baseline.md`.
- [x] Run the verification commands and paste the result summary into the note.

### Task 2: Make clone-only duplicated knowledge score policy explicit

Justification: Report §6 P1 says Ch7 symmetric functional duplication and Ch10 Example 3 remain outside the flagship score; report §2.2 calls clone-only pairs the core book case still excluded from `coupling_balance`.

Files:

- `internal/config/types.go` — add `coupling.duplicated_knowledge` policy enum, e.g. `score|advisory`.
- `internal/config/config_test.go` — validate enum/default behavior.
- `internal/classify/clone_only.go` — keep pair detection pure and deterministic.
- `internal/model/diagnostic/diagnostic.go` — add explicit counters for scored/advisory clone-only pairs.
- `internal/engine/assemble.go` — include clone-only pairs in `ClassifiedEdgeSummary` when policy is `score`.
- `internal/engine/advisories.go` — keep finding output and suppression behavior consistent.
- `internal/model/coupling/scorer.go` — bump `ScoreVersion` if default score semantics change.
- `internal/score/score_boundary_coupling.go` — include scored clone-only pairs in mean/caps/evidence.
- `docs/design/bc-measurement-v4.md` or new v5 design doc — document the policy and migration.
- `docs/guide/configuration-reference.md`, `docs/guide/metrics.md` — document output/config.

Preconditions: Task 1 baseline exists.
Postconditions: Clone-only duplicated knowledge has a deterministic, documented score policy; corpus score movement is attributed.

Fitness gate: New or changed `coupling.duplicated_knowledge` policy must be covered by self `.archfit.yaml` and `make archfit`. If default scoring changes, the gate should fail only for intentional band/drop changes and be rebaselined in a separate explicit step.

Impact commands:

- `gitnexus impact duplicatedKnowledgeAdvisories --direction upstream --depth 4`
- `gitnexus impact Synthesize --direction upstream --depth 4`
- `gitnexus detect-changes --scope all`

Verification commands:

- `go test ./internal/config/... ./internal/classify/... ./internal/engine/... ./internal/score/...`
- `make schema`
- `make archfit`
- `ATTRIB_REPOS_DIR=${ATTRIB_REPOS_DIR:-$HOME/Workspace} make corpus-attrib`

Manual checks:

- Decide whether the default policy is `score` for book fidelity or `advisory` for backward compatibility. If the default is not `score`, document why this deliberately leaves a flagship-score gap.

- [x] Add config enum/default validation and schema coverage.
- [x] Add summary counters for scored/advisory clone-only pairs.
- [x] Wire selected clone-only pairs into `coupling_balance` only through the explicit policy.
- [x] Bump score version and update design docs if score semantics change.
- [x] Run attribution and explain every score/band movement.

### Task 3: Add first-class deterministic connascence evidence

Justification: Report §2.1 says Ch6 connascence is missing or only implicit; report §6 P2 asks for edge metadata/reporting for static cases first.

Files:

- `internal/model/coupling/coupling.go` — add deterministic connascence categories and provenance types.
- `internal/model/diagnostic/diagnostic.go` — add `connascence` report block or edge evidence field.
- `internal/extract/golang/golang.go` — map compiler-grade facts to static connascence where deterministic.
- `internal/extract/ts/ts.go` — map type-only/runtime import evidence to static connascence where deterministic.
- `internal/extract/py/py.go` — expose private/dotted/import evidence without inventing object-kind meaning.
- `internal/extract/scip/scip_strength.go` and `internal/extract/scip/scip_reader.py` — reuse SCIP symbol kinds where precise.
- `internal/classify/classify.go` — attach connascence evidence to classified edges without overriding strength/distance.
- `internal/output` renderers or current JSON/Markdown render paths — show deterministic connascence summary.
- `docs/guide/metrics.md`, `docs/guide/concepts.md` — document scored vs report-only Ch6 evidence.

Preconditions: Task 2 is merged or consciously deferred; output shape changes are acceptable.
Postconditions: Deterministic connascence categories appear in reports; semantic/dynamic categories remain unmeasured unless pinned later by a separate semantic workflow.

Fitness gate: Report-only by default. No gate verdict change allowed in this task; `make archfit` should remain pass unless schema/golden output expectations change.

Impact commands:

- `gitnexus impact ClassifiedEdge --direction upstream --depth 4`
- `gitnexus impact runPipeline --direction upstream --depth 4`
- `gitnexus detect-changes --scope all`

Verification commands:

- `go test ./internal/extract/golang/... ./internal/extract/ts/... ./internal/extract/py/... ./internal/extract/scip/...`
- `go test ./internal/classify/... ./internal/engine/... ./cmd/...`
- `make archfit-report`

Manual checks:

- Confirm no test or prompt claims timing/execution/value/identity connascence is deterministic unless backed by a concrete source.

- [x] Define the minimal deterministic connascence model: name/type/meaning/algorithm/position only where facts support it.
- [x] Add per-language tests that prove both positive detection and honest abstention.
- [x] Render a compact connascence summary in JSON and Markdown.
- [x] Update docs with the Ch6 mapping and unmeasured dynamic categories.

### Task 4: Improve confidence and distance transparency

Justification: Report §6 P2 says confidence does not cap on tiny fully scored graphs; report §2.2 says Ch8 middle distance rungs are compressed and D=8 is missing unless stable facts exist.

Files:

- `internal/score/score_boundary_coupling.go` — add minimum-sample/module-count confidence caps.
- `internal/score/score.go` — disclose cap reasons consistently.
- `internal/classify/distance_structure.go` — expose distance-basis histogram and any deterministic split decisions.
- `internal/model/diagnostic/diagnostic.go` — carry distance-basis/compression evidence.
- `internal/engine/assemble.go` — populate new evidence from classified edges.
- `internal/score/score_test.go`, `internal/classify/distance_structure_test.go`, `internal/engine/assemble_test.go` — focused coverage.
- `docs/design/bc-measurement-v4.md` or new v5 design doc — explain whether mid-rungs changed or remain compressed.
- `docs/guide/metrics.md` — document confidence caps and distance basis.

Preconditions: Task 3 output shape is stable.
Postconditions: Small samples and compressed distance rungs are visible and cannot be mistaken for high-confidence book precision.

Fitness gate: Confidence cap can affect score confidence but should not by itself fail `coupling.gate`; if band/value semantics change, treat it as score-version work.

Impact commands:

- `gitnexus impact coupling_balance --direction upstream --depth 4`
- `gitnexus impact DistanceBasis --direction upstream --depth 4`
- `gitnexus detect-changes --scope all`

Verification commands:

- `go test ./internal/score/... ./internal/classify/... ./internal/engine/...`
- `make archfit`
- `ATTRIB_REPOS_DIR=${ATTRIB_REPOS_DIR:-$HOME/Workspace} make corpus-attrib`

Manual checks:

- Review whether any proposed D=3-D=7 split uses facts already in the graph/config. Reject any split based only on naming taste.

- [x] Add confidence caps for tiny scored-edge counts and tiny connected-module counts.
- [x] Add evidence strings for each confidence cap reason.
- [x] Add a distance-basis/compression summary to JSON/Markdown.
- [x] Either implement deterministic mid-rung split with tests, or document why the current compression remains deliberate.

### Task 5: Corpus validation, docs, and re-review handoff

Justification: Report §7 did not rerun one representative external corpus repo per language; this plan changes book-facing outputs and possibly score semantics.

Files:

- `docs/plans/completed/wave1-deterministic-validation.md` — final validation log.
- `docs/guide/metrics.md` — final output reference.
- `docs/guide/concepts.md` — book-methodology mapping.
- `docs/guide/configuration-reference.md` — config changes.
- `docs/design/bc-measurement-v4.md` — score-version design update.
- `.archfit-baseline.json` — update only if intentional and documented.
- `.archfit.yaml` — update only for deliberate self-dogfood gate coverage.

Preconditions: Tasks 1-4 verification commands pass.
Postconditions: Whole-plan checks pass; corpus movement is explained; docs and baseline match behavior.

Fitness gate: Final self gate must pass with `make archfit`; if `ScoreVersion` changed, baseline migration is explicit and documented.

Impact commands:

- `gitnexus detect-changes --scope all`

Verification commands:

- `make all`
- `.bin/archfit analyze --gate --full --config .archfit.yaml --base origin/main`
- `ATTRIB_REPOS_DIR=${ATTRIB_REPOS_DIR:-$HOME/Workspace} make corpus-attrib`
- `.bin/archfit analyze --full --markdown --config .archfit.yaml > /tmp/archfit-wave1-report.md`

Manual checks:

- Check corpus table covers Go, Python, Rust, and TypeScript. If a repo is skipped, record the missing checkout/tool reason.
- Check docs do not imply LLM output affects gate decisions.

- [x] Run final whole-plan validation commands.
- [x] Write `docs/plans/completed/wave1-deterministic-validation.md` with per-repo score/band/scored/abstained/external movement.
- [x] Update user docs and design docs for every output/config change.
- [x] Rebaseline self only if score-version or gate behavior changed intentionally.
- [x] Record the scoped `architecture-review` follow-up.

## Acceptance criteria

- New deterministic outputs are covered by unit tests and at least one JSON/Markdown rendering test.
- Score movement, if any, has a `ScoreVersion` decision and a corpus attribution table.
- `make all` passes.
- `make archfit` passes.
- The final validation note explains Go/Python/Rust/TypeScript corpus behavior.
- Docs clearly separate scored, report-only, semantic-only, and unmeasured book concepts.

## Safety notes

This plan can change score semantics and CI gate behavior. Treat `ScoreVersion`, `.archfit-baseline.json`, and self `.archfit.yaml` changes as high blast radius. Do not bundle multiple score-moving changes in one commit. No destructive steps or data migrations are expected.

The architect does not apply this plan. An engineer, mutator agent, or ralphex executes it after approval.

## Re-review

After implementation, run a scoped `architecture-review` on deterministic scoring and reporting:

- `internal/model/coupling/*`
- `internal/classify/*`
- `internal/engine/*`
- `internal/score/*`
- `internal/model/diagnostic/*`
- `cmd/archfit/*` render paths

Acceptance signals for re-review: score-version rationale is sound, LLM-free gate boundary still holds, corpus score movement is explained, and Ch6/Ch10 docs match output.
