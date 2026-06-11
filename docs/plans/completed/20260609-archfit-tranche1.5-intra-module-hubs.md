# archfit Tranche 1.5 — Intra-module hub signals (deterministic)

> **SUPERSEDED (2026-06-10).** This attempt's two ranking metrics (`cohesion_spread`,
> `shared_state_hub`) FAILED the 4-repo gate (`docs/plans/notes/intra-module-hub-validation.md`).
> A follow-up signal probe showed deterministic code cannot RANK these hubs and that no reader
> change recovers `polling_state` from SCIP symbol fan-in. Replaced by the facts-block design in
> `docs/plans/20260610-archfit-tranche1.5-structural-facts.md`. Kept for the record.

## Overview

The validation spike (`docs/plans/notes/llm-spike/result.md`) showed the planned
Tranche-2 LLM coupling layer is **blind to a real class of hubs**: two capable blind
classifiers both missed ccgram's `polling_state` (a mutable singleton shared among
sibling polling files) and `directory_callbacks` (one 1086-line file bundling 7 unrelated
subflows). Both are **intra-module** problems that today's metrics do not surface:

- `risk_hub` measures only **inbound** cross-module surface-breadth (how wide a module's
  externally-referenced surface is), so it misses a file whose problem is **outbound**
  spread (`directory_callbacks`) or **depth** of fan-in on a single shared symbol
  (`polling_state` has a narrow surface but ~22 dependents on one strategy class).
- `blast_radius`/`structural_weight`/`change_amplification` operate at config-module
  granularity, hiding intra-`handlers` structure.

This Tranche adds **two deterministic, report-only metrics** over the SCIP symbol graph
archfit already collects, so these hubs appear in the scan evidence the Tranche-2 LLM
consumes. It is a hard prerequisite for the LLM coupling layer: the model cannot refine
coupling it cannot see.

**These are candidate surfacers, not problem detectors** (same contract as
`functional_candidates`). They cannot see mutability or intent, so they will also surface
benign centralizations (a read-only `config` singleton, a wiring `bootstrap`). That is
acceptable: the downstream LLM/human judges mutability and intent. The honest bar (Task 5)
is that the genuinely-risky hubs surface **without the list being drowned by blessed
centralization** — not that every top entry is a real problem.

Source of truth: `docs/design/hybrid-llm-strength-v0.1.md` §7 (Tranche 1.5).

**Hard constraint (unchanged):** `check` — the CI gate — stays pure deterministic and
LLM-free. Both new metrics are `band: info` (report-only), never gate, never set delta.

## Context (from discovery)

- **Stack:** Go, kong CLI, hexagonal. Metrics are pure functions of `MetricInput`; all I/O
  (tools, FS, git) happens in `cmd`/engine and is handed in.
- **Metric contract** (`internal/metrics/metrics.go:47`): `Metric{ Name(); Version();
Calculate(MetricInput) diagnostic.MetricResult }`, registered as a slice in
  `metrics.New()` (`metrics.go:666`). 11 metrics today.
- **The needed data already exists in `symbol.Graph`** (`internal/model/symbol`,
  populated by `internal/extract/scip`): **no `scip_reader.py` change is required.**
  - `Graph.Module` — symbol → its dotted file/python-module path (file-level granularity,
    e.g. `ccgram.handlers.polling.polling_state`).
  - `Graph.FanIn` — symbol → count of **distinct referencing documents**, computed over
    **all** internal references (not cross-module-filtered, excludes the defining doc).
    So a singleton referenced by 22 sibling files has `FanIn = 22`.
  - `Graph.Refs` — `from_symbol → set(to_symbol)` **cross-module** reference edges
    (`from_mod != to_mod`). Attribution is document-scoped (SCIP indexers do not populate
    `enclosing_range`), which is accurate at **file** granularity (the whole file does
    reference those modules) even though per-call-site is unavailable.
- **`MetricInput`** (`metrics.go:55`) already carries `SymbolGraph`, `FileLOC`
  (file → LOC, tests excluded), `FileChurn`, `CoChange`. No new inputs, no new ports.
- **Reusable patterns** (`internal/metrics/risk_hub.go`): `moduleSurfaceBreadth` (group by
  `g.Module[sym]`), `naResult` when the graph is empty, `band: info`, top-N display naming
  the specific files/symbols, churn-independent. The two new metrics mirror this shape.
- **Metrics run by default**; explicit `metrics.<name>` config entries are optional
  overrides (band/gate) — ccgram's `.archfit.yaml` has no `metrics:` block yet `risk_hub`
  ran. Self-config entries are added for visibility/tuning, not to enable the metric.

## Development Approach

- **Testing approach: Regular** (code first, then table-driven stdlib tests; matches every
  existing metric test). No new tool/subprocess boundary is introduced, so no new `moq`.
- Complete each task fully before the next; small focused changes.
- **Every task includes new/updated tests** (success + n/a + a distinctness case), as
  separate checklist items. **All tests pass before the next task.**
- New metrics are **report-only (`band: info`)** — they never gate, preserving the current
  verdict semantics.
- Absent SCIP/symbol graph → `naResult` (info), never a false zero.
- **Pure-Go only.** If implementation reveals the symbol graph lacks a needed field, STOP
  and reassess scope before touching `scip_reader.py` (the spike's premise is that it does
  not).

## Testing Strategy

- **Unit tests:** required every task. Table-driven over hand-built `symbol.Graph` +
  `FileLOC` fixtures (no real SCIP/subprocess). Each metric test includes a **distinctness
  case**: a fixture where the new metric ranks differently from `risk_hub` (proving it adds
  signal, not a rename).
- **Acceptance (Task 5):** real-repo validation across 4 repos (ccgram, codegraph, pumba,
  spotinfo) — a gate distinct from unit tests. The gold standard is ccgram: `polling_state`
  must surface in `shared_state_hub`, `directory_callbacks` in `cohesion_spread`.
- **Determinism check (Task 6):** two `check` runs on the same commit/config produce
  byte-identical diagnostics (sort all ranked output by a total order incl. tie-breakers);
  grep the `check` path — zero LLM dependencies.
- No e2e/UI tests (CLI project).

## Progress Tracking

- Mark completed items `[x]` immediately when done.
- New tasks: `➕` prefix. Blockers: `⚠️` prefix.
- Keep this file in sync; update scope if implementation deviates.

## Solution Overview

Two pure-Go metrics over the existing `symbol.Graph`, plus `FileLOC`:

- **`shared_state_hub`** — for each file (a value of `g.Module`), the **maximum single
  symbol fan-in** among its symbols (and that symbol's name), plus a count of its symbols
  whose fan-in exceeds a threshold. Ranks files that own a heavily-shared symbol. Catches
  `polling_state` (narrow surface, deep fan-in) that `risk_hub` breadth buries.
- **`cohesion_spread`** — for each file, the count of **distinct target subsystems** its
  symbols reference outbound (group `Refs` by source file, count distinct
  parent-package of each target), gated by a small LOC floor and displayed with LOC.
  Ranks large files that reach into many unrelated subsystems. Catches `directory_callbacks`.

Both: `band: info`, churn-independent, `naResult` when the graph is empty, top-N display
naming the offending files/symbols (so the Tranche-2 LLM gets concrete handles). Volatility
is **not** applied (these are structural counts, no double-count with churn metrics).

## Technical Details

- **`shared_state_hub` scoring:** `score(file) = max over file's symbols of FanIn[sym]`.
  Secondary: `hotCount(file) = |{sym in file : FanIn[sym] >= HOT_THRESHOLD}|` (default
  threshold tuned in Task 5; start at 8). Display: `file [top_symbol fan-in=N, M hot]`.
  Rank by `score` desc, then `hotCount`, then file name (deterministic).
- **`cohesion_spread` scoring:** for each source file `F`, `targets(F) = { subsystem(to) :
exists from in F with from→to in Refs }`, where `subsystem(x)` = `x` with its last dotted
  segment stripped (e.g. `ccgram.providers.base` → `ccgram.providers`); `spread(F) =
|targets(F)|`. Filter to `FileLOC[F] >= LOC_FLOOR` (default 150, tuned in Task 5) to drop
  trivial dispatchers. Display: `file [spread S subsystems, LOC L]`. Rank by `spread` desc,
  then LOC, then file name.
  - **Subsystem granularity is the main tuning knob** (Task 5): parent-package strip is the
    default; if it over/under-collapses on a repo, the fallback is mapping the target file
    to its config-module via the module map. Decide empirically in validation; keep the
    chosen rule documented in the metric definition string.
- **Distinctness from existing metrics (must hold, asserted in tests + Task 5):**
  `cohesion_spread` is outbound (vs `risk_hub` inbound, `structural_weight` pure-LOC);
  `shared_state_hub` is single-symbol depth (vs `risk_hub` surface width). A fixture where
  one file wins each new metric but not `risk_hub` is part of each unit test.
- **No new config tool keys.** Optional `metrics.{cohesion_spread,shared_state_hub}` entries
  reuse the existing `MetricEntry` shape.

## What Goes Where

- **Implementation Steps** (`[ ]`): all code, tests, self-config, docs in this repo.
- **Post-Completion** (no checkboxes): the 4-repo manual validation notes are recorded in
  the repo (Task 5), but the per-repo runs require those repos to be present locally.

## Implementation Steps

### Task 1: Register the two metrics in self-config

**Files:**

- Modify: `.archfit.yaml` (self-config: add `metrics.cohesion_spread`,
  `metrics.shared_state_hub`, default `band: info`)
- Modify: `internal/config/config.go` only if a new key is rejected by the loader
- Modify: `internal/config/config_test.go`

- [x] add `metrics.cohesion_spread` and `metrics.shared_state_hub` entries (reuse
      `MetricEntry`; band info), consistent with the existing `risk_hub` entry
- [x] confirm the loader accepts the new metric names (unknown-key tolerance already exists;
      add only if a test shows rejection)
- [x] write/extend test: config with the two new metric entries loads; defaults applied when
      absent
- [x] run tests — must pass before Task 2

### Task 2: cohesion_spread metric (outbound fan-out / low cohesion)

**Files:**

- Create: `internal/metrics/cohesion_spread.go`
- Create: `internal/metrics/cohesion_spread_test.go`
- Modify: `internal/metrics/metrics.go` (register `CohesionSpreadMetric{}` in `New()`)

- [x] implement `fileSpread(g symbol.Graph) map[string]int` — group `Refs` by
      `g.Module[from]`, count distinct `subsystem(g.Module[to])` per source file
- [x] implement `CohesionSpreadMetric.Calculate` — rank files by spread (LOC floor filter,
      LOC as secondary sort), `band: info`, top-N display naming file + spread + LOC
- [x] `naResult` when `SymbolGraph` is empty (no false zero); register in `metrics.New()`
- [x] write tests: a wide-spread large file ranks top; a single-subsystem file ranks low;
      LOC floor drops a tiny multi-import dispatcher; n/a when no graph
- [x] write distinctness test: a file that wins `cohesion_spread` but not `risk_hub`
- [x] run tests — must pass before Task 3

### Task 3: shared_state_hub metric (per-symbol fan-in concentration)

**Files:**

- Create: `internal/metrics/shared_state_hub.go`
- Create: `internal/metrics/shared_state_hub_test.go`
- Modify: `internal/metrics/metrics.go` (register `SharedStateHubMetric{}` in `New()`)

- [x] implement `fileMaxFanIn(g symbol.Graph) (max map[string]int, topSym map[string]string,
hot map[string]int)` — per file, max single-symbol `FanIn`, the winning symbol, and
      count of symbols with `FanIn >= HOT_THRESHOLD`
- [x] implement `SharedStateHubMetric.Calculate` — rank files by max fan-in (then hot count),
      `band: info`, top-N display naming file + hottest symbol + fan-in + hot count
- [x] `naResult` when `SymbolGraph` is empty; register in `metrics.New()`
- [x] write tests: a narrow-surface/deep-fan-in file outranks a wide-surface/shallow file;
      cyclic/self refs excluded (defining doc already excluded by the reader); n/a when no graph
- [x] write distinctness test: a file that wins `shared_state_hub` but not `risk_hub`
- [x] run tests — must pass before Task 4

### Task 4: Render new metrics in all outputs

**Files:**

- Modify: `internal/output/console/console.go`
- Modify: `internal/output/markdown/markdown.go`
- Modify: `internal/output/jsonout/jsonout.go` (usually automatic via the metric struct)
- Modify: corresponding `_test.go` files

- [x] confirm `cohesion_spread` and `shared_state_hub` render with band/confidence/display in
      console + markdown + json
- [x] ensure top-N truncation and `n/a` rendering match existing info metrics
- [x] write/update renderer tests for the two metrics (present + n/a)
- [x] run tests — must pass before Task 5

### Task 5: Validate across 4 repos (acceptance gate)

**Files:**

- Create: `docs/plans/notes/intra-module-hub-validation.md` (results log)

- [x] run `archfit scan` (scip on) on ccgram → confirm `directory_callbacks` is a top
      `cohesion_spread` hub AND `polling_state` is a top `shared_state_hub` hub
- [x] run on codegraph, pumba, spotinfo → record top-5 per metric per repo
- [x] confirm both metrics are **distinct** from `risk_hub`/`blast_radius`/`structural_weight`
      on each repo (different top files, not a reordered duplicate) — note overlaps
- [x] **signal-to-noise check (the real bar):** cross-reference each metric's ccgram top-N
      against the architect-blessed centralizations (`config`, `utils`, `tmux_manager`,
      `bootstrap`/`app_bootstrap` — intended glue / low-risk generic per review line 572).
      A target surfacing while the list is dominated by blessed hubs is a FAIL (this is the
      trap that killed transitive-`risk_hub`: it surfaced things but was mostly noise)
- [x] tune `HOT_THRESHOLD`, `LOC_FLOOR`, and the `subsystem()` granularity if a target hub
      does not surface or the list is dominated by blessed/artifact hubs; re-run; record
      final values
- [x] ⚠️ BLOCKER: `directory_callbacks` ranks 6th on cohesion_spread (spread=7, top-5 floor=8);
      `polling_state` ranks 16th on shared_state_hub (fan-in=8, top-5 floor=28); shared_state_hub
      top-5 dominated 3/5 by blessed hubs (config/thread_router/tmux_manager); no legitimate
      threshold change can surface either target — signal definitions need reassessment before
      proceeding (see docs/plans/notes/intra-module-hub-validation.md)
- [x] write the results log (docs/plans/notes/intra-module-hub-validation.md); ⚠️ GATE: FAIL —
      directory_callbacks rank 6 (not top-5); polling_state rank 16 (not top-5); shared_state_hub
      top-5 dominated 3/5 by blessed hubs; signal definitions need reassessment

### Task 6: Verify acceptance criteria

- [ ] both metrics implemented, registered in `metrics.New()`, and `band: info`
- [ ] **determinism gate:** two `check` runs on the same commit/config produce byte-identical
      diagnostics (verify total-order sorting incl. tie-breakers); grep `check` path — zero
      LLM dependencies
- [ ] run full suite: `make test` (uncached, `go test -count=1 ./...`) — all packages pass
- [ ] run lint: `make lint` (golangci-lint) — 0 issues
- [ ] confirm baseline/delta unchanged: info-band metrics set no delta, never appear in gate
      findings; verdict unaffected by their presence
- [ ] run `archfit scan` on this repo — no panic, sensible output

### Task 7: Documentation

**Files:**

- Modify: `README.md`, `docs/design/hybrid-llm-strength-v0.1.md`

- [ ] document the two new metrics and their definitions (outbound spread vs inbound breadth;
      single-symbol depth vs surface width)
- [ ] update design §7: mark Tranche 1.5 implemented; note it unblocks the Tranche-2 coupling
      layer
- [ ] move this plan to `docs/plans/completed/` when Tranche 1.5 is done

## Post-Completion

_No checkboxes — external action or later work._

**Re-validate the Tranche-2 coupling layer:** once these metrics ship, re-run the spike's
coupling half (a blind classifier over the enriched evidence package) and confirm
`polling_state` and `directory_callbacks` now surface to the LLM. That re-validation is the
real payoff of this Tranche and the entry condition for building Tranche-2 coupling refinement.

**Threshold tuning is data, not code:** prefer adjusting `HOT_THRESHOLD`/`LOC_FLOOR`/subsystem
granularity in the validation log and (if promoted) config, over hard-coding repo-specific
values.
