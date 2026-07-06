# Gate integrity — verdict-path decisions (wave 1)

Date: 2026-07-02. Status: SHIPPED (PR #25, branch `fix/wave1-gate-integrity`).
Fixes defects V1/V2/V3 from `docs/archived/reports/eval-2026-07-02-v1.1.2/00-FINDINGS.md`.
Related plan: `docs/plans/completed/20260702-wave1-gate-integrity.md`.

---

## 1. Direction-aware metric deltas (V1)

Every metric result declares its polarity — `direction: higher_is_better`
(encapsulation, coverage) or `higher_is_worse` (cycle, unbalanced_edge,
blast_radius) — and `computeVerdict` treats only a worsening move as a
regression.

Why: the old clause treated `Delta < 0` as a regression for every metric.
Correct for ratios, inverted for counts — a newly introduced cycle
(Delta=+1) passed silently, and fixing a cycle (Delta=−1) produced a false
WARN (the historical "re-baseline phantom WARN" gotcha was this bug
manifesting). Polarity is a property of the metric definition, not a user
choice, so it lives in the metric result
(`internal/metrics/internal/result`), never in config.

## 2. Per-metric gate knobs wired, not deleted (V3)

`metrics.*.gate` / `max_new` / `min_delta` are consumed by the verdict:
`gate` follows the rule-gate convention (`off` skips, `warn` caps at WARN,
`fail`/unset blocks); `max_new` bounds count-metric increases; `min_delta`
bounds ratio-metric drops. Gates are projected per metric via
`Config.ForMetric()` into `RunInput.MetricGates` in `cmd/archfit`.

`max_new_high` was **deleted** from schema, docs, and spec instead of wired:
`MetricResult` carries no severity partition, and high-severity finding
counts already gate through rule gates. `validate()` now rejects a knob on a
metric of the wrong kind and any gate/threshold on informational
`blast_radius` — a validated-but-inert knob is worse than an absent one
(that inertness was the V3 disease).

## 3. coupling_balance gates through `coupling.gate`, inside the pipeline (V2)

The synthesised score gates through a dedicated
`coupling.gate: {min_band, max_drop}` block — not through `metrics:`, because
`coupling_balance` is not a registered metric and has no `MetricResult`.
`score.Synthesize` + `applyCouplingGate` run INSIDE `runPipeline`, before
verdict finalization and `agenttask.Build`; synthesizing after the pipeline
(the pre-wave-1 shape) made the score unreachable by any gate by
construction.

- `min_band` is a band floor; `critical` is rejected as inert and an empty
  block is rejected outright.
- `max_drop` compares against `baseline.ScoreSnapshot`, written by
  `archfit baseline` only when the score is measured; no anchor → the drop
  check is skipped, never guessed.
- Layering: config (support) must not import score (core), so the
  coupling-gate projection (`couplingGateView`) lives in `cmd/archfit`, not
  on `Config` — the dogfood `forbidden_layer_direction` gate caught the
  inverted version during development.
- On trip: verdict FAIL (exit 1), reasons to stderr, and ACTIVE
  `bc/imbalanced_coupling` advisories are promoted to `Kind: "gate"` so they
  pass the unchanged `agenttask` filter into `agent_tasks[]`. Baselined and
  waived advisories stay triaged.
- The score is synthesised from `ClassifiedEdges` (before advisory
  filtering), so a trip can find no promotable advisory — advisory output
  off, or `coupling.min_severity` above every active edge. That case emits
  one synthetic `bc/coupling_gate` gate finding carrying the trip reasons, so
  `summary.gate_findings` and `agent_tasks[]` never contradict a FAIL
  verdict. `archfit baseline` skips it (per-run trip state, not a triageable
  edge).
- The stderr echo happens in `analyze` only (re-evaluating the pure gate
  decision), not inside the shared `runPipeline` — baseline/enrich/explain and
  `--base` scoring must not print enforcement-sounding lines they don't act on.
- `archfit baseline` persists BC findings with their native advisory kind: the
  promotion is per-run output state, and a stored `"gate"` kind would orphan
  the baseline entry (`status.Assign` matches stored kind against the pass
  kind, yielding a phantom "fixed" gate finding and no advisory-side
  resolution).

## 4. BandNA never gates

An unmeasured `coupling_balance` (band `n/a`) never trips the gate,
regardless of knobs.

Why: `n/a` is an abstention — empty module map, non-matching globs, empty
SCIP index, degenerate (<2 connected modules) graph, partial coverage — not
evidence of bad architecture. Gating on it would flip CI red exactly on the
runs where archfit knows least (e.g. storybook-style partial-coverage TS
runs) and would push users to delete the gate rather than fix coverage.
Abstain ≠ fail is the same discipline as the v1.1.1
n/a-not-fabricated-60 fix. Verified end-to-end: a no-source repo with the
strictest knobs exits 0.
