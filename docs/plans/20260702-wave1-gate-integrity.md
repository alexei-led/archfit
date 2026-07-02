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

- [ ] add `internal/engine/verdict_test.go` with a table-driven `TestComputeVerdict` over constructed `Diagnostic`/metric-result inputs (computeVerdict is pure — no fixtures needed)
- [ ] cases (initially expected-fail, marked with TODO(wave1) until Task 2): new cycle (count metric Delta=+1) must WARN/FAIL; fixed cycle (Delta=−1) must PASS; encapsulation drop (ratio Delta=−0.1) must WARN; encapsulation rise must PASS; unchanged metrics must PASS
- [ ] cases for n/a: metric absent on either side → no delta contribution (no phantom WARN — this is the historical re-baseline gotcha)
- [ ] run `make test-fast` — new tests fail for the inverted cases, pass for the rest; commit the harness with expected-current-behavior assertions inverted via TODO markers per ralphex partial-implementation rule

### Task 2: Per-metric delta direction (V1)

- [ ] add an explicit direction to metric results: each metric declares `HigherIsBetter` (encapsulation, coverage) or `HigherIsWorse` (cycle, unbalanced_edge, blast_radius — check blast_radius mode in `internal/metrics/modularity/blast_radius.go` before assuming) in its result (`internal/metrics/internal/result/result.go`)
- [ ] rewrite the delta clause in `computeVerdict` (`engine.go:452-467`) to use the direction; regression = worsening move only
- [ ] flip the TODO(wave1) assertions in `TestComputeVerdict` to book-correct expectations; all cases green
- [ ] regenerate goldens if output changed; inspect diff; commit (single behavior change)
- [ ] run `make test && make lint && make archfit` — must pass before Task 3

### Task 3: Wire metrics.\*.gate / max_new / min_delta (V3)

- [ ] consume `Config.ForMetric()` (`views.go:157-163`) in the verdict path: per-metric `gate: off` skips that metric's delta check; `warn` caps at WARN; `fail`/unset may FAIL per existing severity conventions
- [ ] `max_new: N` — for count metrics, allowed increase before the gate trips (default 0); `min_delta` — for ratio metrics, tolerated drop; `max_new_high` — decide: wire against high-severity finding counts if a consumer is natural, otherwise DELETE the field from schema and docs (dead knobs are worse than absent knobs — do not leave it validated-but-inert)
- [ ] verify semantics match what `docs/guide/configuration-reference.md:108-114,750-766` already promises; fix docs where behavior legitimately differs
- [ ] table tests: each knob exercised (off/warn/fail × count/ratio × within/over threshold)
- [ ] run `make test && make lint && make archfit`; regen goldens only if output changed; commit

### Task 4: coupling_balance gate path (V2)

- [ ] move the `score.Synthesize` call INTO the pipeline: today it runs only from `cmd/archfit/analyze.go:198`, AFTER `runPipeline` (analyze.go:188) has already finalized the verdict AND frozen `diag.AgentTasks` (`agenttask.Build` at `cmd/archfit/pipeline_run.go:321`). Merely swapping analyze.go:188/:198 is NOT enough — Synthesize (or at least the gate-trip decision) must execute inside the pipeline before both verdict computation and `agenttask.Build`. `pipeline_run.go` lives in `cmd/`, so calling `internal/score` from there keeps the import rings untouched
- [ ] add config `coupling.gate:` block — `min_band: poor|mixed|strong` (band floor; verdict trips when current band is below floor) and `max_drop: N` (points vs `.archfit-baseline.json` stored score; check baseline schema stores the score — extend if not)
- [ ] hard rule: `BandNA` never gates (abstain ≠ fail; storybook/partial-coverage runs must not flip CI red) — explicit test
- [ ] when the coupling gate trips, emit the triggering BC findings with `Kind: "gate"` so they flow into `agent_tasks[]` through the existing filter (`agenttask.go:48` unchanged); verify agent_tasks now carry file:line for coupling drift
- [ ] tests: band floor trip, max_drop trip, BandNA no-trip, gate off by default (no config → today's behavior, backward compatible)
- [ ] regen goldens deliberately; update `CLAUDE.md` invariants (severity source note gains the coupling.gate path); commit

### Task 5: Four-language corpus verification

- [ ] build: `make build`; run the gate per language with the saved eval configs (`reports/eval-2026-07-02-v1.1.2/` and each repo's own `.archfit.yaml`):
- [ ] Go — archfit self (`make archfit`): with a temporary `coupling.gate.min_band: mixed` the run must FAIL (self scores 39/poor); without the block, PASS as today
- [ ] Python — `~/workspace/ccgram`: analyze exits consistently; cycle-delta direction sane (introduce a scratch cross-module cycle in a throwaway copy, verify WARN/FAIL, not PASS)
- [ ] Rust — `~/workspace/herdr` (25/poor, 97% scored): `min_band: mixed` must FAIL; verify agent_tasks non-empty and files exist on disk
- [ ] TypeScript — `~/workspace/storybook` (partial coverage): coupling gate must NOT trip on low-confidence/BandNA; exit 0
- [ ] restore all corpus repos to clean `git status`; run `make all`; open PR

### Task 6: [Final] Documentation

- [ ] update `docs/guide/configuration-reference.md` (coupling.gate, per-metric knobs — real semantics only)
- [ ] update `docs/design/` decision note: why direction-aware deltas, why BandNA never gates
- [ ] `reports/eval-2026-07-02-v1.1.2/00-FINDINGS.md` backlog items V1/V2/V3: mark fixed with commit refs

## Technical Details

- Direction belongs to the metric definition, not config — a metric's polarity is not a user choice.
- Exit-code contract unchanged: 0 pass / 2 warn / 1 fail / 3 error (verify against `cmd/archfit/analyze.go:230-237` before assuming — the hardGate tool-coverage path (`applyToolGate`, `cmd/archfit/pipeline_coverage.go:200-213`) stays independent).
- Backward compatibility: absent `coupling.gate` ⇒ byte-identical verdict behavior to v1.1.2 except the V1 direction fix (which changes verdicts only where they were wrong).

## Post-Completion

- Re-run the corpus verification (the 12-repo experiment from `reports/eval-2026-07-02-v1.1.2/corpus-experiments.md`) against the merged binary before starting Wave 2.
- Consider a patch release (v1.1.3 or v1.2.0 — coupling.gate is a feature): tag-triggered flow only, never manual release.
