# Phase-1 validation — correctness + determinism gate (Task 6)

Validation run for the gap-closure plan Phase 1 (Tasks 1–5). Re-ran archfit
(built from this branch) full + delta on the three baseline repos and asserted
the blind spots Phase 1 set out to close.

- Date: 2026-06-19
- archfit: `v0.3.0-7-g989c89e` (this branch, built 2026-06-19)
- Repos / HEADs: archfit (Go) `989c89e`; ccgram (Python, src-layout) `a6b9ba7`;
  codegraph (TypeScript, no `node_modules`) `0e2789a`
- Delta bases: Go `HEAD~5`; ccgram `HEAD~30` (221 changed files, 193 `.py`);
  codegraph `HEAD~30` (80 `.ts`)
- Commands per repo: `check --full --advisory --report --format json,markdown`
  and `check --base <ref> --advisory --report --format json`; each run twice for
  the byte-identical double-run.

## Two residual bugs found (and fixed in this task)

Validation on the real repos exposed two ceilings left by Tasks 1 and 3 that the
fixture tests did not reach. Both are now fixed with regression tests.

1. **change_locality false-0 on src-layout (Task 1 ceiling).** ccgram is a
   `src/`-layout project: git reports `src/ccgram/bootstrap.py`, but grimp's
   module node is `module:ccgram.bootstrap`, whose reconstructed candidate path
   (`ccgram/bootstrap.py`) drops the `src/` prefix and never matched the changed
   file → 0 cross-module edges at high confidence. Fix: `changedNodeIDs` now also
   matches each changed file with its single leading source-root segment stripped
   (`src/ccgram/bootstrap.py` → `ccgram/bootstrap.py`), kept only while
   multi-segment so it can never collapse to a bare basename and over-match an
   unrelated top-level module. Module-only; file nodes (Go/TS) are unchanged.
   (`internal/metrics/boundary/change_locality.go`)

2. **Node builtins leak into martin (Task 3 incomplete).** With no
   `node_modules`, dependency-cruiser lists node builtins (`fs`, `crypto`,
   `path`, `child_process`, `os`, `fs/promises`, …) and uninstalled npm packages
   (`commander`, `ignore`, `web-tree-sitter`, …) as their own top-level
   `module.source` entries. The TS extractor emitted every `module.source` as a
   first-party `file:` node, so 16 non-`src/` "modules" reached
   martin_distance as zone-of-pain (Dms=1.0). Task 3 only guarded the _dependency_
   side. Fix: guard the _source_ side too — skip `coreModule` source entries,
   mark `couldNotResolve` source entries external.
   (`internal/extract/ts/ts.go`)

## Assertions (all pass after the fixes)

| Assertion                                | Result                                                                                                                                                                              |
| ---------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ccgram delta `change_locality` ≠ false 0 | **340** cross-module edge(s) from 221 changed file(s); forward reach 76 (band info, high confidence)                                                                                |
| codegraph martin lists no node builtins  | **clean** — martin/instability/abstractness/blast_radius/propagation_cost; 35 zone-of-pain modules, all first-party `src/` (was 51 incl. `fs`/`crypto`/`commander`/`child_process`) |
| Go martin no builtins                    | clean                                                                                                                                                                               |
| BC advisories rendered as rollups        | ccgram: `## Balanced Coupling advisories (58 rollups, 421 edges)` — the 421-edge flood collapses to 58 grouped rollups                                                              |
| BC score value/band visible              | all 58 rollups carry `score_value`+`score_band`; markdown shows `score: 5/10 (medium) [multiplicative]` and a Vlad-vocabulary `why:` line                                           |

## Determinism

| Repo / mode            | double-run     | config_hash (full == delta) |
| ---------------------- | -------------- | --------------------------- |
| Go full / delta        | byte-identical | `041fe0550b831e15…`         |
| ccgram full / delta    | byte-identical | `98a3c9951b99530a…`         |
| codegraph full / delta | byte-identical | `ff4dff47dc2bb185…`         |

All six JSON double-runs are byte-identical; each repo's `config_hash` is stable
and identical across full and delta (same config).

## Verdicts

- Go full: pass (0 findings)
- ccgram full / delta: fail (61 findings — 3 gated import cycles + 58 BC advisory rollups)
- codegraph full / delta: pass (0 gated findings; 2 cross-internal cycles remain report-only per baseline)

## Baseline → after (the matrix this plan set out to beat)

| Signal                          | Baseline                                  | After Phase 1                                                               |
| ------------------------------- | ----------------------------------------- | --------------------------------------------------------------------------- |
| ccgram delta change_locality    | 0 "high confidence" (false)               | 340 cross-module edges (genuine, high confidence)                           |
| ccgram BC advisories            | 421 near-identical lines                  | 58 grouped rollups (421 edges folded), each with numeric score              |
| codegraph martin "zone of pain" | node builtins (`fs`/`crypto`/`commander`) | first-party `src/` modules only                                             |
| Go self BC score                | never emitted                             | n/a here (0 advisories on Go self); emission verified on ccgram (58 scores) |
| determinism                     | —                                         | byte-identical double-run + stable config_hash on all three repos           |

## Out of scope for Phase 1 (tracked for later phases)

- ccgram lazy/dynamic imports still hide cycles in the static graph (Task 9).
- codegraph semantic strength absent without `node_modules`; coverage is silent
  rather than explaining the enable step (Task 11).
- change_locality stripping handles a single-level source root only; a two-level
  root (`packages/x/src/…`) still won't match — acceptable for a report-only
  signal.
