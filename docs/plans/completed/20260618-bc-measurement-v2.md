# Execution plan — Balanced Coupling measurement engine v2.0

Date: 2026-06-18. Status: READY FOR EXECUTION (after approval). Owner: engineer /
mutator agent / task runner. The architect produced this plan and stops; execution
is someone else's job.

## Overview

Implement the approved design `docs/design/bc-measurement-v2.md`: make archfit's
measurement engine a faithful, deterministic Balanced Coupling implementation across
Go/TS/Python, close the seven self-analysis gaps, add the standard coupling metrics,
and align the report with BC vocabulary — without putting an LLM or non-determinism on
the gate path.

Five phases, each a re-reviewable horizon (≤5 executable tasks). Phase 1 is detailed
in full; Phases 2–5 list concrete tasks and expand into full task detail at each
phase's re-review gate. Low-risk correctness first; the score-breaking `*.v2` bump and
report restructure land last.

### Resolved decisions (the three open questions, answered via Perplexity + advisor)

- **Calibration repos** (Task 9, Task 20) — resolved via Perplexity research. Go =
  archfit (self); TypeScript = `redwoodjs/redwood` (pinned tag — real `api` + `web`
  deploy units plus shared packages = genuine cross-module _and_ cross-deploy-unit
  coupling); Python = `saleor/saleor` (pinned tag — mid-sized production app,
  straightforward `scip-python` indexing). Backups: TS `nestjs/nest`; Py `django/django`.
  Stretch-only: `apache/airflow` (Perplexity flagged it too large/config-sensitive for
  _repeated_ indexing, so not the primary). All swappable without code change.
- **§6 standard-metric rollout** — resolved via advisor: ON by default, no opt-in flag.
  They are report-only and never gate, so they cannot break the verdict or baselines; the
  agent loop ignores rows it does not consume (zero noise); the only cost is a one-time
  `TestGolden` update captured in the Phase 3 commit.
- **Versioning** — resolved via advisor: one-shot `*.v2` metric-version bump + deliberate
  golden/baseline reset in Phase 5 (Task 21). No dual-emit, no scorer config flag — both
  add a dual-maintenance burden with a fixed expiry on a pre-1.0 tool. The `*.v2` string
  signals the break to anyone pinned to `*.v1`; old baselines encode meaningless v1 scores,
  so forcing `archfit baseline` re-run is the honest action. Ship it as **v0.3.0**, with a
  release note framing it as a breaking change in _scoring_, not in _verdict_ behavior.

## Source artifact

Approved design: `docs/design/bc-measurement-v2.md`. Tasks cite its sections and the
`file:line` evidence it records:

- Gap 1 (distance ignores code structure) — design §3#1, §4.2; `classify.go:198-222`,
  `ownership.go:60-69,225-266`.
- Gap 2 (no deploy-unit detector) — design §4.2; `config.go:153`.
- Gap 3 (churn on the gate) — design §3#3, §4.3; `classify.go:224-253`,
  `config/volatility.go`.
- Gap 4 (formula discards ordinality) — design §3#4, §5; `coupling.go:84-91`.
- Gap 5 (XOR mis-graded) — design §3#5, §5; `coupling.go:137-138`.
- Gap 6 (no connascence / runtime; generic-subdomain split) — design §4.1, §4.2, §4.3.
- Gap 7 (report not BC-aligned / not agent-actionable) — design §8.
- Standard metrics — design §6. LLM placement — design §7. Determinism — design §9.

## Success criteria

- The three BC dimensions each carry real signal on archfit itself: distance is no
  longer a constant; volatility on the gate is domain-sourced (zero churn input);
  strength uses all four levels in scoring.
- A single `Scorer` port produces a 0–10 integer score, band, machine-parseable reason,
  and deterministic cheapest-move per edge; both additive and multiplicative impls exist
  and one is locked by calibration.
- XOR (cohesive / loose-coupling) edges produce no finding; tight-coupling + volatile
  edges produce critical.
- Deploy units auto-detected for Go/TS/Python; owner resolution degrades honestly (no
  false "same owner everywhere").
- Standard metrics (Instability, Abstractness, Dms, propagation_cost, change_coupling)
  reported, clearly labeled "beyond Balanced Coupling", never gating.
- Runtime async + connascence CoT/CoA reported with confidence, never gating in v1.
- `check` makes zero LLM/network/clock/random calls; `internal/arch_test.go` proves it.
- Report reads in BC vocabulary as agent-actionable lint messages; same repo+config →
  byte-identical JSON.

## Validation Commands

Whole-plan, run green before declaring any phase complete:

- `make all` — fmt → lint → test → build.
- `go test -race ./...` — full suite incl. race.
- `go test ./internal/ -run TestArchImports` — import-ring invariant (core ring purity).
- `go test ./internal/engine/ -run TestGolden` — golden output (regenerate deliberately).
- `go test ./internal/model/coupling/ -run TestScorer` — Scorer matrix (added Task 4).
- `make calibrate` — calibration harness (added Task 9); informational, not a CI gate.
- `.bin/archfit check --full --advisory --report --format json` on archfit + each
  calibration repo — determinism spot-check (run twice, diff must be empty).

GitNexus impact (index present at `.gitnexus/`): for a changed package P, reverse-deps
via `node .gitnexus/run.cjs query "<cypher: nodes importing P>"`; fallback when GitNexus
is unavailable: `git diff --name-only <base>` plus `go list -e -deps ./... | rg P` for Go
reverse dependencies. Refresh index first with `node .gitnexus/run.cjs analyze --index-only`.

## Implementation Steps

## Phase 1 — Correctness, no new data (immediate horizon)

### Task 1: Fix XOR severity grading

- Justification: Gap 5 (design §3#5, §5); `coupling.go:137-138` returns `SeverityLow`
  for the two BC-modular quadrants, which should be `SeverityNone`.
- Files:
  - `internal/model/coupling/coupling.go` — in `BalanceResult`, return `SeverityNone`
    for asymmetric (high-strength+low-distance, low-strength+high-distance) cases; keep
    the over-decoupled-volatile-seam (low+low+high-volatility) at `SeverityMedium`.
  - `internal/model/coupling/coupling_test.go` — assert each of the four quadrants.
- Preconditions: clean tree on a feature branch.
- Postconditions: cohesive and loose-coupling edges yield no finding; existing
  intrusive-escalation paths unchanged.
- Impact: `git diff --name-only` (coupling.go is high fan-in — `model/diagnostic`/`graph`
  consumers); GitNexus: `node .gitnexus/run.cjs query "<importers of model/coupling>"`.
- Verification: `go test ./internal/model/coupling/ -run TestBalance`;
  `go test ./internal/ -run TestArchImports`.
- Manual checks:
  - Confirm no rule currently relies on the old `SeverityLow` for XOR edges (grep
    `SeverityLow` consumers in `internal/rules`).
- Work items:
  - [ ] Change the asymmetric return to `SeverityNone`.
  - [ ] Add four-quadrant table test (none/none/medium-if-volatile/critical-if-volatile).
  - [ ] Run coupling + arch-imports tests green.

### Task 2: Remove git churn from gate volatility

- Justification: Gap 3 (design §3#3, §4.3); `classify.go:224-253` consumes churn-derived
  volatility via `effectiveModules()`. BC forbids commit-history volatility on the gate.
- Files:
  - `internal/classify/classify.go` — `classifyVolatility` reads explicit `volatility`
    → `subdomain` → (Task 3 heuristic) → `VolatilityUnknown`. Never reads churn.
  - `internal/config/volatility.go` — keep `DeriveVolatility`/`ApplyVolatility` but stop
    feeding `derivedVolatility` into the classify view; expose churn only to report-only
    metric inputs (`risk_hub`, `change_amplification`).
  - `internal/config/config.go` — split the view: `ClassifyView` (no churn) vs a
    `ChurnView`/metrics input that carries implementation volatility.
- Preconditions: Task 1 merged.
- Postconditions: gate volatility is domain-sourced or `unknown`; churn still powers the
  two report-only metrics; `c.Modules` stays hand-authored-only (unchanged invariant).
- Impact: `go list -e -deps ./... | rg 'classify|config'`; GitNexus reverse-deps of
  `internal/config`.
- Verification: `go test ./internal/classify/... ./internal/config/...`;
  `go test ./internal/metrics/... -run 'RiskHub|ChangeAmplification'` (churn still flows
  to these).
- Manual checks:
  - Confirm `risk_hub` and `change_amplification` outputs are unchanged on archfit (churn
    path intact for report-only).
- Work items:
  - [ ] Introduce the two config views; remove churn from the classify view.
  - [ ] Repoint `classifyVolatility` fallback to `VolatilityUnknown`.
  - [ ] Keep churn wired to report-only metric inputs.
  - [ ] Tests green incl. the metrics that still consume churn.

### Task 3: Deterministic domain-volatility heuristic (priority-3 source)

- Justification: Gap 3/6 (design §4.3); replace the removed churn fallback with a
  deterministic, domain-oriented default so unconfigured repos are not all `unknown`.
- Files:
  - `internal/classify/volatility_heuristic.go` (new) — path-pattern table:
    generic (`vendor/`,`lib/`,`util/`,`pkg/common/`) → low; supporting
    (`infra/`,`platform/`,`storage/`,`db/`) → medium; else → unknown (do NOT guess
    `core`/high — conservative). Applied only after explicit config, before `unknown`.
  - `internal/classify/volatility_heuristic_test.go` (new).
- Preconditions: Task 2 merged.
- Postconditions: modules with no config get a conservative domain band or `unknown`;
  never churn-derived.
- Impact: local to `classify`; `git diff --name-only`.
- Verification: `go test ./internal/classify/ -run TestVolatilityHeuristic`.
- Manual checks:
  - Eyeball the pattern table against archfit's module paths for obvious misclassification.
- Work items:
  - [ ] Implement the pattern table + matcher (reuse doublestar).
  - [ ] Wire as source priority 3 in `classifyVolatility`.
  - [ ] Table-driven tests incl. the `unknown` default.

### Task 4: Scorer port + additive and multiplicative impls

- Justification: Gap 4 (design §3#4, §5); collapse-to-2-bits is the core scoring defect.
  Introduce a swappable, testable continuous scorer; default keeps current behavior until
  calibration (Task 20) locks the winner.
- Files:
  - `internal/model/coupling/scorer.go` (new) — `Scorer` interface; `EdgeScore`
    (`Value int`, `Band Severity`, `Reason string`, `Breakdown`, `CheapestMove string`);
    frozen ordinal→number tables as named consts with BC-rationale comments.
  - `internal/model/coupling/scorer_additive.go`,
    `internal/model/coupling/scorer_multiplicative.go` (new) — the two impls; integer
    math; multiplicative carries the intrusive floor + structural penalties hooks.
  - `internal/model/coupling/scorer_test.go` (new) — table-driven over the 4×4×4 matrix;
    assert XOR=none, intrusive floor, clamp [0,10], deterministic cheapest-move.
  - `internal/classify/classify.go` — populate `Classification` and attach the score via
    the configured scorer (default = a shim reproducing today's bands until Task 20).
- Preconditions: Tasks 1–3 merged.
- Postconditions: every classified edge carries a 0–10 score, band, reason, and
  cheapest-move; no float on the gate.
- Impact: `model/coupling` is highest fan-in — `node .gitnexus/run.cjs query
"<importers of model/coupling>"`; expect `classify`, `rules`, `metrics`, `engine`.
- Verification: `go test ./internal/model/coupling/ -run TestScorer`;
  `go test ./internal/ -run TestArchImports`; `go test ./internal/engine/ -run TestGolden`
  (regenerate deliberately if the shim shifts output — inspect diff).
- Manual checks:
  - Confirm the default shim leaves archfit's current bands unchanged (no premature score
    churn before calibration).
- Work items:
  - [ ] Define `Scorer`/`EdgeScore`/`Breakdown` + frozen const tables.
  - [ ] Implement additive and multiplicative scorers.
  - [ ] Matrix tests (XOR, floor, clamp, cheapest-move determinism).
  - [ ] Wire default shim into classify; keep golden stable.

### Task 5: Phase 1 verification and documentation

- Justification: phase gate; lock correctness before adding data sources.
- Files: `docs/design/bc-measurement-v2.md` (status note); `CHANGELOG`/release notes if
  present.
- Preconditions: Tasks 1–4 merged.
- Postconditions: Phase 1 green; design doc annotated with Phase-1-done.
- Impact: `git diff --name-only` for the phase branch.
- Verification: full `make all`; `go test -race ./...`; determinism spot-check (run
  `archfit check` twice, diff empty).
- Manual checks:
  - Re-review gate: confirm gate volatility shows no churn dependency; XOR fix verified on
    archfit (expect still 0 gate findings, but advisories now correctly graded).
- Work items:
  - [ ] Run whole-plan validation green.
  - [ ] Annotate design doc Phase-1 status.
  - [ ] Open Phase-2 re-review.

**Re-review gate → Phase 2.** Run a scoped `architecture-review` on `classify`/`coupling`
before starting Phase 2.

## Phase 2 — Distance signal (horizon 2; expand to full task detail at the gate)

### Task 6: code_structure_distance baseline

- Justification: Gap 1 (design §4.2) — the always-available distance signal archfit lacks.
- Files: `internal/classify/distance_structure.go` (new) — deterministic tree-distance
  from two module paths; `classify.go` — fold into the `max(...)` distance composite.
- Verification: `go test ./internal/classify/ -run TestCodeStructureDistance` (siblings
  near, distant trees far); `TestArchImports`.
- Manual checks: spot-check `metrics/boundary` ↔ `metrics/modularity` reads closer than
  `cmd/archfit` ↔ `extract/py`.
- Work items: [ ] implement; [ ] integrate composite; [ ] tests.

### Task 7: Degenerate-owner suppression

- Justification: Gap 1 (design §4.2) — single git-author owner must not read as
  "same owner everywhere = low distance".
- Files: `internal/ownership/ownership.go` and/or its consumer in `cmd/archfit/pipeline.go`
  — when the resolver yields one owner for the whole repo, suppress ownership's distance
  contribution (let code-structure dominate); CODEOWNERS multi-owner unaffected.
- Verification: `go test ./internal/ownership/ -run TestDegenerateOwner`; determinism
  spot-check.
- Manual checks: confirm a real multi-team CODEOWNERS fixture still yields owner distance.
- Work items: [ ] detect single-owner case; [ ] suppress contribution; [ ] tests both ways.

### Task 8: Deploy-unit detector

- Justification: Gap 2 (design §4.2) — `cross_deploy_unit` unreachable today.
- Files: `internal/extract/deployunit/` (new adapter, via `toolrun.Runner`) — Go `main`
  pkgs (`go list`), TS `package.json` workspaces, Python `pyproject.toml`, `Dockerfile`,
  k8s `Deployment`/`StatefulSet`; `internal/config/config.go` — `FillMissingDeployUnits`
  mirroring `FillMissingOwners`; `cmd/archfit/pipeline.go` — wire it.
- Verification: `go test ./internal/extract/deployunit/...` with per-language fixtures;
  `archfit doctor` still green; determinism spot-check.
- Manual checks: confirm `deploy_unit_confidence: inferred|config` recorded; config wins.
- Work items: [ ] per-language detectors; [ ] FillMissingDeployUnits; [ ] pipeline wire;
  [ ] fixtures.

### Task 9: Calibration harness + golden refresh

- Justification: design §5 calibration protocol; needed to choose the Scorer (Task 20).
- Files: `Makefile` (`calibrate` target); `scripts/calibrate.*` (new) — run both scorers
  on archfit (Go) + pinned `redwoodjs/redwood` (TS) + pinned `saleor/saleor` (Py), emit
  per-edge band-agreement
  report; `internal/engine` golden regenerated for distance changes (inspect diff).
- Verification: `make calibrate` produces a report; `go test ./internal/engine/ -run
TestGolden` after deliberate regeneration.
- Manual checks: review the agreement report for face validity (not a CI gate).
- Work items: [ ] harness; [ ] pin calibration repos; [ ] regenerate golden with diff review.

### Task 10: Phase 2 verification

- Whole-plan validation; design doc Phase-2 note; **re-review gate → Phase 3** (scoped
  `architecture-review` on distance + deployunit).

## Phase 3 — Standard metrics, report-only (horizon 3)

### Task 11: Instability / Abstractness / Distance-from-main-sequence

- Justification: design §6. Files: `internal/metrics/modularity/martin.go` (new) — `I`,
  `A`, `Dms=abs(A+I-1)` from the existing dep graph + SCIP exports; unstable-dependency
  edge penalty hook for the multiplicative scorer. Verification: `go test
./internal/metrics/modularity/ -run TestMartin`. Manual: confirm shared-DTO trap noted
  (high-Ca low-I not auto-"good"). Work items: [ ] compute; [ ] band; [ ] tests.

### Task 12: Propagation cost

- Justification: design §6. Files: `internal/metrics/modularity/propagation.go` (new) —
  `PC = reachable_pairs/(N^2-N)` via transitive closure; system + per-module.
  Verification: `go test -run TestPropagationCost`. Work items: [ ] closure; [ ] PC; [ ] tests.

### Task 13: Change-coupling formula

- Justification: design §6 — sharpen `hidden_coupling` with CodeScene's
  `CC(A,B)=C_AB/min(C_A,C_B)`, flag ≥65%; source git/gitnexus.
- Files: `internal/metrics/modularity/change_coupling.go` (or extend hidden_coupling).
  Verification: `go test -run TestChangeCoupling`. Work items: [ ] formula; [ ] threshold;
  [ ] tests.

### Task 14: Phase 3 verification — whole-plan; **re-review gate → Phase 4**.

## Phase 4 — Speculative signals, report-only (horizon 4)

### Task 15: Runtime async detection

- Justification: design §4.2 runtime_adjust. Files: `internal/extract/runtime/` (new,
  ast-grep patterns per language) + off-gate `library→integration-kind` YAML table; edge
  `AsyncBridge` annotation on `graph.Edge`; scorer adds +1 distance level when set
  (report-only). Verification: `go test ./internal/extract/runtime/...` with fixtures.
  Manual: confirm "absence of signal ≠ synchronous" recorded as `confidence: low`.
  Work items: [ ] patterns; [ ] table; [ ] annotation; [ ] fixtures.

### Task 16: Connascence CoT/CoA tags

- Justification: design §4.1. Files: `internal/classify` — tag edges `connascence: type`
  (SCIP struct/interface) and `connascence: algorithm` (clone pair crossing a module
  boundary); report-only, not scored. Verification: `go test -run TestConnascenceTags`.
  Work items: [ ] CoT from SCIP; [ ] CoA from clones; [ ] tests.

### Task 17: `enrich --subdomains` (LLM draft → review → pin)

- Justification: design §7. Files: `cmd/archfit/enrich.go` (extend), `internal/initcfg`
  (subdomain draft+pin writing `reviewed_at`/`reviewed_by`). LLM off-gate only; gate reads
  the pin. Verification: `go test ./internal/initcfg/...`; confirm `internal/arch_test.go`
  still proves `check` has no LLM import. Manual: dry-run a draft on archfit, human-review
  one module. Work items: [ ] draft prompt; [ ] pin writer; [ ] staleness wired;
  [ ] gate-purity test.

### Task 18: Phase 4 verification — whole-plan; **re-review gate → Phase 5**.

## Phase 5 — Report restructure + calibration decision (horizon 5)

### Task 19: BC-aligned report

- Justification: Gap 7 (design §8). Files: `internal/engine` markdown/JSON writers —
  lint-message format (`ARCHFIT[id sev] from→to`, strength/distance/volatility line, score
  breakdown, why, cheapest-move), `config_hash`, "beyond Balanced Coupling" section,
  distance-confidence section. Verification: `go test ./internal/engine/ -run TestGolden`
  (regenerate, inspect). Work items: [ ] lint format; [ ] sections; [ ] config_hash; [ ] golden.

### Task 20: Run calibration, lock the Scorer

- Justification: design §5. Files: set the default `Scorer` to the calibration winner;
  freeze its const tables; remove the legacy shim. Verification: `make calibrate` final
  report; `go test ./internal/model/coupling/...`. Manual: record the decision + agreement
  numbers in the design doc. Work items: [ ] choose; [ ] freeze consts; [ ] drop shim.

### Task 21: Final verification, `*.v2` bump, documentation, re-review

- Justification: phase gate + design §11 risk 1; versioning decision (resolved via
  advisor). Files: bump affected `metric_version` strings to `*.v2`; regenerate golden +
  reset baselines deliberately; update `docs/guide`, `CLAUDE.md` (commands/metrics), and
  the self-analysis report (`archfit-analysis.md`); write a **v0.3.0** release note framing
  this as a breaking change in _scoring_ (continuous Scorer replaces `BalanceResult`; old
  `*.v1` baselines are invalid, re-run `archfit baseline`) and explicitly NOT a change in
  _verdict/gate_ behavior. Verification: `make all`; `go test -race ./...`;
  `TestArchImports`; `TestGolden`; determinism spot-check (twice, empty diff);
  `archfit doctor`. Manual: re-run `archfit scan` on archfit and confirm the new report
  reads in BC vocabulary with correct gradings. Work items: [ ] `*.v2` bump; [ ] baseline
  reset; [ ] `docs/guide` + `CLAUDE.md` + `archfit-analysis.md` updated; [ ] v0.3.0 release
  note (breaking scoring, not verdict); [ ] whole-plan green; [ ] record re-review.

## Acceptance criteria

- All five Validation Commands green; `archfit check` byte-identical across two runs.
- Distance varies across archfit's edges (not constant); gate volatility has zero churn
  input; scoring uses all four strength + four distance levels.
- One `Scorer` locked by calibration; XOR edges = no finding; tight+volatile = critical;
  each edge carries reason + cheapest-move.
- Deploy units detected for Go/TS/Py fixtures; owner resolution degrades honestly.
- Standard metrics (I/A/Dms, PC, change_coupling) present, labeled beyond-BC, non-gating.
- Runtime async + connascence reported with confidence, non-gating.
- `internal/arch_test.go` proves `check` imports no LLM/os/exec in the core ring.
- Report is BC-vocabulary lint messages; `docs/design/bc-measurement-v2.md` and
  `archfit-analysis.md` updated.

## Safety notes

- **Blast radius:** `internal/model/coupling` and `internal/config` are the two highest
  fan-in packages (self-analysis: config breadth 114, model/graph 75% reverse-deps).
  Tasks 1, 2, 4 touch them — run `TestArchImports` + `TestGolden` after each, and the
  GitNexus reverse-deps query before merging.
- **Irreversible-ish:** Task 21's `*.v2` bump + baseline reset invalidates pinned
  baselines downstream. It is deliberate and confined to the final task; communicate in
  release notes. Reversible by reverting the version bump, but consumers must re-baseline.
- **Determinism is the cardinal risk.** New detectors (deployunit, runtime, ownership
  suppression) must read only static repo state via `toolrun.Runner`; the gate stays
  integer-only. Every phase ends with a twice-run empty-diff determinism check.
- **No big-bang.** Each task is independently committable and verifiable; the default
  Scorer shim (Task 4) keeps output stable until calibration (Task 20) so scores don't
  churn mid-stream.
- **Execution handoff:** an engineer, mutator agent, or task runner executes this plan
  after approval, one phase per horizon, re-reviewing at each gate. If run by a task
  runner, use this exact path: `docs/plans/20260618-bc-measurement-v2.md`.

## Re-review

After Phase 5, run a scoped `architecture-review` over `classify`, `model/coupling`,
`config`, `extract/deployunit`, `extract/runtime`, and `engine` to verify the
implementation matches `docs/design/bc-measurement-v2.md` and that determinism + core-ring
purity invariants hold. Re-run `archfit scan` on archfit and confirm the report and
gradings reflect the new model.
