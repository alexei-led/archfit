# Phase-2 validation — architect skill vs improved archfit (Task 19)

End-to-end comparison for the gap-closure plan Phase 2 (Tasks 7–18). Re-ran the
improved archfit (full + delta + `--format scorecard`) on the three baseline
repos, then ran an **independent, blind** architect-skill review on each
(read-only, no access to archfit output). Compares archfit's banded scorecard
against the expert bands and records which baseline blind zones are now closed
deterministically, surfaced report-only, or still out of scope.

- Date: 2026-06-19
- archfit: `v0.3.0-20-gb271736` (this branch, built 2026-06-19)
- Repos / HEADs: archfit (Go) `b271736`; ccgram (Python, src-layout) `a6b9ba7`;
  codegraph (TypeScript, no `node_modules`) `0e2789a`
- Delta bases: Go `HEAD~5` (`efe4320`); ccgram `HEAD~30` (`3ea54f3`, 193 `.py`);
  codegraph `HEAD~30` (`3a1ddf4`, 80 `.ts`)
- archfit commands per repo: `check --full --advisory --report --format json,markdown`,
  `check --base <ref> --advisory --report --format json`, and `score` (≡
  `check --advisory --report --format scorecard`); each run twice for the
  byte-identical double-run.
- Expert review: one `architecture:architect` agent per repo, blind to archfit
  output, scoring the same 7-dimension rubric (`scorecard.yaml` rubric_version 1,
  bands critical 0-20 / poor 21-40 / mixed 41-60 / serviceable 61-80 / strong
  81-100).

## Overall scorecard: archfit vs expert

| Repo           | archfit overall    | expert overall     | band agreement    | archfit verdict | expert verdict |
| -------------- | ------------------ | ------------------ | ----------------- | --------------- | -------------- |
| archfit (Go)   | 68/100 serviceable | 80/100 serviceable | **same band**     | pass            | pass           |
| ccgram (Py)    | 47/100 mixed       | 69/100 serviceable | adjacent (1 band) | fail            | pass-with-risk |
| codegraph (TS) | 54/100 mixed       | 43/100 mixed       | **same band**     | pass            | fail           |

Overall bands agree on 2 of 3 repos (Go, codegraph); ccgram is one band apart.
Verdicts differ on direction, not magnitude: archfit is **stricter** on ccgram
(it gates 3 import cycles → fail) and **more lenient** on codegraph (the flat
single-`core`-layer config collapses distance so no boundary rule fires → pass).
Both divergences trace to documented blind zones (below), not to nondeterminism
or metric bugs.

## Per-dimension: archfit band vs expert band

`✓` = same band; `≈` = adjacent band; `✗` = ≥2 bands apart.

### archfit (Go)

| Dimension               | archfit        | expert           | agree |
| ----------------------- | -------------- | ---------------- | ----- |
| boundary_integrity      | 60 mixed (low) | 88 strong (high) | ✗     |
| coupling_balance        | 90 strong      | 75 serviceable   | ≈     |
| dependency_graph_health | 63 serviceable | 82 strong        | ≈     |
| cohesion_modularity     | 34 poor        | 70 serviceable   | ✗     |
| change_locality         | 94 strong      | 78 serviceable   | ≈     |
| architecture_fitness    | 67 serviceable | 85 strong        | ≈     |

### ccgram (Python)

| Dimension               | archfit        | expert         | agree |
| ----------------------- | -------------- | -------------- | ----- |
| boundary_integrity      | 33 poor (low)  | 74 serviceable | ✗     |
| coupling_balance        | 50 mixed       | 62 mixed       | ✓     |
| dependency_graph_health | 18 critical    | 68 serviceable | ✗     |
| cohesion_modularity     | 36 poor        | 65 serviceable | ✗     |
| change_locality         | 80 serviceable | 72 serviceable | ✓     |
| architecture_fitness    | 67 serviceable | 71 serviceable | ✓     |

### codegraph (TypeScript)

| Dimension               | archfit        | expert   | agree |
| ----------------------- | -------------- | -------- | ----- |
| boundary_integrity      | 60 mixed (low) | 52 mixed | ✓     |
| coupling_balance        | 90 strong      | 48 mixed | ✗     |
| dependency_graph_health | 43 mixed       | 44 mixed | ✓     |
| cohesion_modularity     | 30 poor        | 42 mixed | ≈     |
| change_locality         | 70 serviceable | 53 mixed | ≈     |
| architecture_fitness    | 33 poor        | 22 poor  | ✓     |

Per-dimension agreement: **8 same-band, 7 adjacent, 6 two-or-more apart** across
the 18 cells (the meta dimension `analysis_confidence` is excluded — archfit 95/90/85
strong vs expert 82/82/72; archfit reads slightly high but same/adjacent band).
The systematic two-band gaps are not random — they cluster into three named,
explainable patterns described next.

## The three systematic tool-vs-expert divergences (residual gap, by design)

1. **cohesion_modularity is systematically lower in archfit** (Go 34 vs 70;
   ccgram 36 vs 65; codegraph 30 vs 42). archfit's cohesion proxy penalises
   god-modules by LOC skew + hidden-coupling pairs; the expert credits strong
   internal sub-package cohesion (ccgram `window_state_ports/`,
   `polling/window_tick/{decide,observe,apply}`; archfit's own `metrics/*`
   sub-packages). LOC-skew is a structural proxy and cannot see that a large file
   is internally well-factored — a known proxy ceiling, footnoted as
   low-confidence where it drives the headline (Task 16). This is exactly the
   "diagnosis layer" `review --llm` is built to narrate.

2. **boundary_integrity is gate-centric, not enforcement-aware.** archfit scores
   the _current violation count_: ccgram has 3 gated layer findings → 33 poor;
   Go/codegraph have no classified cross-boundary edges → 60 mixed at **low
   confidence**. The expert scores the _enforcement discipline_: ccgram's 4
   AST-based fitness tests + lazy-import linter with managed allow-lists → 74
   serviceable; archfit's 5 CI-gated ring rules → 88 strong. archfit measures
   "are boundaries violated right now"; the expert measures "are boundaries
   defended". Both are valid; archfit's low-confidence flag on the n/a cases is
   the honest hedge.

3. **coupling_balance inherits config-as-truth.** On codegraph archfit reads 90
   strong ("no unbalanced-coupling advisories over classified edges") because the
   repo's `.archfit.yaml` puts all 12 modules in one `core` layer with no owner —
   so distance collapses and no BC edge can be classed high-distance. The expert,
   reading intent from the code, scores 48 mixed (intra-extraction god-file cycle,
   sideways `db → extraction` / `extraction → resolution` edges, lazy `require()`
   hiding the `index` cycle). This is the **config-as-truth blind spot** (plan
   blind-spot #6): a flat config produces a falsely-strong coupling score. Task 14's
   config-quality lint now _warns_ about exactly this (Go self-run flagged 24
   under-specified modules on stderr); declared-vs-observed drift detection remains
   out of scope.

Where the config is real and edges are classified, archfit and expert **agree**:
ccgram coupling_balance 50 vs 62 (both mixed), architecture_fitness 67 vs 71
(both serviceable), change_locality 80 vs 72 (both serviceable); codegraph
dependency_graph_health 43 vs 44, architecture_fitness 33 vs 22 (both poor).

## Baseline blind-spot matrix → after Phase 2

The plan's six blind-spot categories (Context section), re-checked on the real repos:

| #   | Blind spot (baseline)                                          | After Phase 2                                                                                                                                                                                         | State                  |
| --- | -------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------- |
| 1   | Coverage-driven (silent n/a from missing/opt-in tools)         | Coverage states the reason + enable step: codegraph `scip: absent` → "install node_modules"; `lizard: absent` → opt-in step; gitnexus **auto-detected** (ccgram `.gitnexus`, 144 files) + refresh cmd | **closed** (T11/12/13) |
| 2   | Node-ID inconsistency (Python false-0; TS leaks file:<name>)   | ccgram delta change_locality **340 cross-module edges** (was false-0); codegraph martin lists **no node builtins** (`fs`/`crypto`/`commander` gone)                                                   | **closed** (T1/T3)     |
| 3   | Distance/calibration (421 advisory flood; flat names collapse) | ccgram 421 BC edges → **58 grouped rollups**, each with numeric `score_value`/`score_band`; dotted-name distance structural; volatility undeclared vs unknown separated                               | **closed** (T4/T5/T10) |
| 4   | Semantic/dynamic (lazy imports hide cycles)                    | Dynamic-import signal (report-only): ccgram **788 sites / 38 modules**, codegraph **111 / 9** — points exactly at the cycles the static graph misses; full cycle resolution stays semantic            | **surfaced** (T9)      |
| 5   | No synthesis (flat metrics, every band info)                   | Banded 7-dimension `score` / `--format scorecard` emitted on all three repos — this comparison runs on it                                                                                             | **closed** (T15/T16)   |
| 6   | Config-as-truth (minimal config → noise / false-strong)        | Config-quality lint warns under-specified modules (Go self: 24 flagged); residual symptom = codegraph false-strong coupling_balance; drift detection out of scope                                     | **partial** (T14)      |

**Blind-zone reduction is real:** the two hard false signals from the baseline
matrix (ccgram change_locality false-0; codegraph martin flagging node builtins)
are gone; the 421-advisory flood is collapsed 7× to 58 scored rollups; and the
lazy-import class that hid 40+ cycles at baseline is now surfaced report-only
(ccgram 788 sites) — the architect's ccgram review independently found 6 cycles,
4 of them broken by lazy imports, which is precisely what the 788-site signal
points at. archfit's static graph sees only the 3 cycles **not** hidden by lazy
imports; the dynamic-import signal is the deterministic pointer to the rest.

### What `review --llm` would address (skipped — no key)

`archfit review` exits cleanly with an actionable message when `ANTHROPIC_API_KEY`
is unset (verified: `error: llm provider anthropic not configured … set the key
and re-run`). The off-gate narrative layer is exactly the class behind the three
systematic divergences above — crediting lazy-import discipline (ccgram graph
health), distinguishing intentional library-facade fan-out from a
distributed-monolith edge (codegraph), and contextualising LOC-skew god-modules
that are internally cohesive. It narrates and prioritises **existing** evidence;
it never invents gate violations and never affects `check` (LLM-off-gate
invariant, `internal/arch_test.go`). Live validation deferred to Task 21 with a
provisioned key.

### Still out of scope by design

- Full semantic cycle resolution through lazy/dynamic imports (needs runtime or
  complete SCIP) — archfit surfaces the risk report-only, does not resolve it.
- Declared-vs-observed config drift (flag a flat/under-specified config as
  _wrong_, not just _under-specified_).
- The "architect POV" diagnosis layer (intent, ownership, contract, placement
  judgments) — deterministically surfaced as locations; narrated only by
  `review --llm`.

## Determinism

| Repo / mode | full      | delta     | scorecard | config_hash (full == delta) |
| ----------- | --------- | --------- | --------- | --------------------------- |
| Go          | identical | identical | identical | `ecfa8486739e9264…` (same)  |
| ccgram      | identical | identical | identical | `98a3c9951b99530a…` (same)  |
| codegraph   | identical | identical | identical | `ff4dff47dc2bb185…` (same)  |

All nine JSON/scorecard double-runs are byte-identical; each repo's `config_hash`
is stable and identical across full and delta (same config). `make test` (race,
full suite) and `make lint` (0 issues) green; core-ring import (`TestArchImports`)

- golden (`TestGolden`) + LLM-off-gate invariants hold.

## Verdicts (archfit)

- Go full: pass (0 findings) — scorecard 68/100 serviceable
- ccgram full / delta: fail (61 findings — 3 gated import cycles + 58 BC advisory rollups) — scorecard 47/100 mixed
- codegraph full / delta: pass (0 gated findings; 2 cycles report-only) — scorecard 54/100 mixed
