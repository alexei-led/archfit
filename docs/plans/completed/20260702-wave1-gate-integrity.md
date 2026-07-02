# Wave 1: Gate Integrity — verdict path tells the truth

## Overview

Wave 1 of 7 from the 2026-07-02 eval (`reports/eval-2026-07-02-v1.1.2/00-FINDINGS.md`). No prior wave required.

archfit's exit code is currently untrustworthy in three verified ways (defects V1, V2, V3 in the findings report — all confirmed at root cause with repros):

- **V1 — metric-delta sign inversion.** `computeVerdict` treats `Delta < 0` as regression for every metric (`internal/engine/engine.go:452-467`). Correct for ratio metrics (encapsulation, coverage: higher is better), inverted for count metrics (cycle, unbalanced_edge: higher is worse). A newly introduced cycle yields Delta=+1 → silent PASS; fixing a cycle yields Delta=−1 → false WARN. No `TestComputeVerdict` exists anywhere.
- **V2 — coupling_balance can never gate.** The flagship metric is not one of the 5 registered metrics (`internal/metrics/metrics.go:52-58`); `metrics.coupling_balance.*` config is rejected (`internal/config/config.go:187`); the score is synthesized by `score.Synthesize` at `cmd/archfit/analyze.go:198` — after `diag.Verdict` is finalized at line 188 — and used only for rendering. BC advisory findings are explicitly excluded from `computeVerdict` (`internal/engine/engine.go:259-261`). No config path exists to make a coupling regression fail CI.
- **V3 — dead gate knobs.** `MetricEntry{Gate, MinDelta, MaxNew, MaxNewHigh}` (`internal/config/types.go:11-17`) are schema-validated (`config.go:191`, enum check only) and never consumed. The natural wiring point `Config.ForMetric()` (`internal/config/views.go:157-163`) has zero callers. Docs present them as working knobs (`docs/guide/configuration-reference.md:108-114,750-766`, `docs/spec/arch-fitness-spec-v0.4.md:440-456`).

After this wave: a metric regression in the correct direction gates, the documented per-metric knobs work, and coupling_balance can block CI when configured — which also makes `agent_tasks[]` populate for coupling drift with no changes to `agenttask.go` (findings that gate get `Kind: "gate"`, which is exactly the existing filter at `internal/agenttask/agenttask.go:48`).

## Context

- Verdict path: `internal/engine/engine.go` (`computeVerdict` ~452-467, advisory exclusion ~259-261), `cmd/archfit/analyze.go:180-260` (pipeline → verdict → score → exit code), `cmd/archfit/pipeline_run.go` (`agenttask.Build` runs at :321, INSIDE runPipeline — load-bearing for Task 4).
- Prior art: `internal/decision/decision.go` (`decideBand`) already combines Diagnostic + Scorecard into an n/a-aware narrative band, but feeds only text/markdown rendering, never the exit code — reuse its n/a semantics, do not duplicate them.
- Metric results: `internal/metrics/internal/result/result.go:138` (delta = `current - snap.Value`, direction-agnostic), metric families in `internal/metrics/boundary/` and `internal/metrics/modularity/`.
- Score synthesis: `internal/score/score.go` (`Synthesize`, `BandNA`), baseline in `.archfit-baseline.json`.
- Config: `internal/config/types.go`, `views.go` (`ForMetric`), `config.go`.
- Known gotcha: re-baselining `.archfit-baseline.json` previously produced a phantom negative-delta WARN (see project memory / git history) — that was V1 manifesting; the fix must make that scenario a test.

## Development Approach

- **Testing approach: tests-first for each defect** — reproduce the wrong verdict in a failing test, then fix.
- One behavioral change per commit; regenerate goldens (`go test ./internal/engine/ -run TestGolden`) deliberately per commit and inspect the diff — never batch two behavior changes into one golden regeneration.
- Gate after every task: `make test && make lint && make archfit`.
- `main` is protected: work on branch `fix/wave1-gate-integrity`, PR at the end.
- Every task includes new/updated tests; all tests pass before the next task.

## Implementation Steps

### Task 1: TestComputeVerdict harness capturing today's bugs

- [x] add `internal/engine/verdict_test.go` with a table-driven `TestComputeVerdict` over constructed `Diagnostic`/metric-result inputs (computeVerdict is pure — no fixtures needed)
- [x] cases (initially expected-fail, marked with TODO(wave1) until Task 2): new cycle (count metric Delta=+1) must WARN/FAIL; fixed cycle (Delta=−1) must PASS; encapsulation drop (ratio Delta=−0.1) must WARN; encapsulation rise must PASS; unchanged metrics must PASS
- [x] cases for n/a: metric absent on either side → no delta contribution (no phantom WARN — this is the historical re-baseline gotcha)
- [x] run `make test-fast` — new tests fail for the inverted cases, pass for the rest; commit the harness with expected-current-behavior assertions inverted via TODO markers per ralphex partial-implementation rule

### Task 2: Per-metric delta direction (V1)

- [x] add an explicit direction to metric results: each metric declares `HigherIsBetter` (encapsulation, coverage) or `HigherIsWorse` (cycle, unbalanced_edge, blast_radius — check blast_radius mode in `internal/metrics/modularity/blast_radius.go` before assuming) in its result (`internal/metrics/internal/result/result.go`)
- [x] rewrite the delta clause in `computeVerdict` (`engine.go:452-467`) to use the direction; regression = worsening move only
- [x] flip the TODO(wave1) assertions in `TestComputeVerdict` to book-correct expectations; all cases green
- [x] regenerate goldens if output changed; inspect diff; commit (single behavior change) — `TestGolden` output unchanged; `cmd/archfit` byte-identical fixtures (`internal/extract/golang/testdata/{single-module,one-member-workspace}/baseline.json`) gained the 5 new `direction` fields (diff inspected: exactly encapsulation/coverage=`higher_is_better`, unbalanced_edge/cycle/blast_radius=`higher_is_worse`, nothing else)
- [x] run `make test && make lint && make archfit` — must pass before Task 3 — all green (`make archfit` exits 0, unchanged from before: self-config has no `coupling.gate` block yet)

### Task 3: Wire metrics.\*.gate / max_new / min_delta (V3)

- [x] consume `Config.ForMetric()` (`views.go:157-163`) in the verdict path: per-metric `gate: off` skips that metric's delta check; `warn` caps at WARN; `fail`/unset FAILs (rule-gate convention) — wired as `RunInput.MetricGates` built in `cmd/archfit/pipeline_run.go` via `cfg.ForMetric(m.Name())` per registered metric, consumed by `computeVerdict`
- [x] `max_new: N` — count metrics, breach when delta > N (default 0); `min_delta` — ratio metrics, breach when drop exceeds it (default 0); `max_new_high` — DELETED from schema (strict decode now rejects it loudly), docs, and spec: no natural consumer — `MetricResult` carries no severity partition, and high-severity finding counts already gate through rule gates. Bonus: `validate()` now rejects a knob on a metric of the wrong kind and any gate/threshold on informational `blast_radius` (validated-but-inert knobs were the V3 disease)
- [x] verified doc semantics; fixed `configuration-reference.md` (knob prose + max_new_high removal), `metrics.md` (stale verdict pseudo-code + dead `--report` flag), `concepts.md` (warn-only claim), `arch-fitness-spec-v0.4.md` (max_new_high + wrong metric names `cycles`/`unbalanced_edges`); regenerated `archfit.schema.json` (`make schema`)
- [x] table tests: each knob exercised (off/warn/fail × count/ratio × within/over threshold) in `TestComputeVerdict`; config validation table extended (negative/mismatched knobs, blast_radius, max_new_high decode rejection)
- [x] run `make test && make lint && make archfit` — all green; golden output unchanged (no regen needed); commit

### Task 4: coupling_balance gate path (V2)

- [x] move the `score.Synthesize` call INTO the pipeline: today it runs only from `cmd/archfit/analyze.go:198`, AFTER `runPipeline` (analyze.go:188) has already finalized the verdict AND frozen `diag.AgentTasks` (`agenttask.Build` at `cmd/archfit/pipeline_run.go:321`). Merely swapping analyze.go:188/:198 is NOT enough — Synthesize (or at least the gate-trip decision) must execute inside the pipeline before both verdict computation and `agenttask.Build`. `pipeline_run.go` lives in `cmd/`, so calling `internal/score` from there keeps the import rings untouched — done: `runPipeline` now returns `(Diagnostic, score.Scorecard, error)`; `Synthesize` + `applyCouplingGate` run inside it, before `agenttask.Build`; the cargo-modules coverage append moved ABOVE the synthesis (it feeds the partial-module-graph confidence cap). All five callers adapted; worktree/analyze no longer re-synthesize
- [x] add config `coupling.gate:` block — `min_band: poor|mixed|serviceable|strong` (band floor; `critical` rejected as inert, empty block rejected) and `max_drop: N` (`*int`, 0 = any drop trips) — baseline schema did NOT store the score: extended with `baseline.ScoreSnapshot` (`score.coupling_balance` + `band`, omitempty), written by `archfit baseline` only when measured; schema regenerated (`make schema`). Gotcha found by the dogfood gate itself: the projection cannot be a `Config` method (config=support layer must not import score=core layer — `make archfit` FAILED on `forbidden_layer_direction`); `couplingGateView` lives in `cmd/archfit/pipeline_run.go`
- [x] hard rule: `BandNA` never gates (abstain ≠ fail; storybook/partial-coverage runs must not flip CI red) — explicit test (`TestEvaluateCouplingGate` BandNA case + e2e `TestRun_Analyze_CouplingGate_BandNANeverTrips`: no-source repo, strictest knobs, exit 0)
- [x] when the coupling gate trips, emit the triggering BC findings with `Kind: "gate"` so they flow into `agent_tasks[]` through the existing filter (`agenttask.go:48` unchanged); verify agent_tasks now carry file:line for coupling drift — `applyCouplingGate` promotes ACTIVE `bc/imbalanced_coupling` advisories (baselined/waived stay triaged), adjusts Summary counters, prints trip reasons to stderr; e2e asserts the bc agent task carries Files + Goal
- [x] tests: band floor trip, max_drop trip, BandNA no-trip, gate off by default (no config → today's behavior, backward compatible) — unit table in `internal/score/gate_test.go` (11 cases incl. boundary drop==max_drop, band==floor) + config validation table + 5 cmd e2e tests; verified live on archfit itself: `min_band: mixed` → exit 1 with "band poor below floor" stderr reason, unmodified config → exit 0
- [x] regen goldens deliberately; update `CLAUDE.md` invariants (severity source note gains the coupling.gate path); commit — `TestGolden` unchanged (engine untouched), no fixture diffs; CLAUDE.md gained the coupling-gate invariant + `gate:` bullet cross-ref; `make test && make lint && make archfit` all green

### Task 5: Four-language corpus verification

- [x] build: `make build`; run the gate per language with the saved eval configs (`reports/eval-2026-06-30-corpus/configs/` — the v1.1.2 report dir holds only findings, the configs live in the 06-30 corpus dir — and each repo's own `.archfit.yaml`)
- [x] Go — archfit self: temporary `coupling.gate.min_band: mixed` → exit 1, stderr `coupling gate: coupling_balance band "poor" (score 39) is below the configured floor "mixed"`; config restored → `make archfit` exit 0
- [x] Python — `~/workspace/ccgram`: two consecutive `--gate` runs both exit 2 (consistent; the WARN is the repo's intentional advisory-severity gate, same as the eval). Cycle direction in a throwaway APFS clone: fresh baseline (cycle=2) → injected `tts↔whisper` mutual imports → cycle=3, JSON shows `delta: 1, direction: higher_is_worse` → Decision FAIL exit 1 (pre-V1-fix this was a silent PASS); copy deleted. Observation for the backlog: the TEXT renderer prints `Decision FAIL` yet `Gate PASS · 0 blocking` with no metric-delta line — the tripping delta is only visible in JSON
- [x] Rust — `~/workspace/herdr`: `min_band: mixed` → exit 1, `band "poor" (score 25) below floor "mixed"`; `agent_tasks` non-empty (630 bc tasks, each with goal + validation command). Caveat: task `files` are cargo-modules module IDs (`herdr::detect`), not disk paths — they map to real `src/` modules but an agent must resolve them; known crate/module-granularity limitation (per-file resolver was removed as dead code), not a gate defect. Config restored from backup
- [x] TypeScript — `~/workspace/storybook` (partial coverage, no node_modules): eval config + `min_band: mixed` → measures 48/mixed (matches eval), gate does not trip, exit 0. Explicit BandNA probe (module globs matching nothing + strictest `min_band: strong`) → `coupling_balance` band `n/a`, gate does not trip, exit 0 — abstain ≠ fail verified on a real TS repo
- [x] restore all corpus repos to clean `git status` — restored everything this task touched (herdr config from backup, archfit config via git checkout, throwaway copy deleted); ccgram's pre-existing uncommitted `.archfit.yaml` schema migration and herdr's pre-existing untracked eval artifacts predate this task and were deliberately left as found; `make all` green; PR opened

### Task 6: [Final] Documentation

- [x] update `docs/guide/configuration-reference.md` (coupling.gate, per-metric knobs — real semantics only) — added `### coupling.gate` subsection (min_band/max_drop semantics, empty-block/critical rejection, max_drop needs the baseline ScoreSnapshot anchor, BandNA never gates, trip → FAIL + advisory promotion into agent_tasks), updated the top-level key index, and cross-referenced from `## metrics` (`metrics.coupling_balance` is a config error). Per-metric knob docs were already real-semantics from Task 3 — verified, no change needed
- [x] update `docs/design/` decision note: why direction-aware deltas, why BandNA never gates — new `docs/design/20260702-gate-integrity.md` recording all four wave-1 decisions: polarity lives in the metric result not config; knobs wired + `max_new_high` deleted (validated-but-inert is the disease); coupling.gate evaluated inside runPipeline with the projection in cmd (layer ring); abstain ≠ fail rationale
- [x] `reports/eval-2026-07-02-v1.1.2/00-FINDINGS.md` backlog items V1/V2/V3: mark fixed with commit refs — defects table rows V1/V2/V3 and backlog items 1/2/5 marked ✅ FIXED/DONE with PR #25 + branch commits (`cd1f1e7`+`f12bd22`, `f4f00f6`, `8e6c45e`). Note: `reports/` is git-ignored, so this update is local-only by design

## Technical Details

- Direction belongs to the metric definition, not config — a metric's polarity is not a user choice.
- Exit-code contract unchanged: 0 pass / 2 warn / 1 fail / 3 error (verify against `cmd/archfit/analyze.go:230-237` before assuming — the hardGate tool-coverage path (`applyToolGate`, `cmd/archfit/pipeline_coverage.go:200-213`) stays independent).
- Backward compatibility: absent `coupling.gate` ⇒ byte-identical verdict behavior to v1.1.2 except the V1 direction fix (which changes verdicts only where they were wrong).

## Post-Completion

- Re-run the corpus verification (the 12-repo experiment from `reports/eval-2026-07-02-v1.1.2/corpus-experiments.md`) against the merged binary before starting Wave 2.
- Consider a patch release (v1.1.3 or v1.2.0 — coupling.gate is a feature): tag-triggered flow only, never manual release.
