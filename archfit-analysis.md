# Archfit — Architecture Fitness Analysis

- **Target:** archfit (this repo)
- **Tool:** `archfit scan` (= `check --full --advisory --report`) — `v0.2.0-6-g4682765`
- **Date:** 2026-06-18
- **Config:** `.archfit.yaml` (committed; `gitnexus` + `risk_hub` enabled), gitnexus index refreshed (3,917 nodes / 11,787 edges)
- **Verdict:** **PASS** — 0 gate findings, 0 warnings, 0 exceptions used.

## Scorecard

Bands: `strong` = healthy hard gate · `info` = report-only hotspot (never fails CI) ·
`n/a` = not computable in this mode.

| Metric                 | Result                                                                                                             | Band           |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------ | -------------- |
| `cycle`                | 0 import cycles                                                                                                    | strong         |
| `coverage`             | 100% module coverage                                                                                               | strong         |
| `unbalanced_edge`      | 0 new high-risk unbalanced edges                                                                                   | strong         |
| `encapsulation`        | n/a (no intrusive cross-module access detected)                                                                    | n/a (low conf) |
| `risk_hub`             | 79 risk hubs (gitnexus-refined) — see below                                                                        | info           |
| `blast_radius`         | 4 hubs: `model/graph` 75% (36 deps), `model/finding` 67%, `model/diagnostic` 58%, `config` 35%                     | info           |
| `change_amplification` | 0 volatile change-impact hubs                                                                                      | info           |
| `hidden_coupling`      | 64 pairs; top `internal/engine` (16), `internal/metrics` (9)                                                       | info           |
| `structural_weight`    | 2 god-modules: `cmd/archfit` 1973 LOC (9×), `internal/initcfg` 1842 LOC (9×)                                       | info           |
| `complexity`           | 14 functions CCN>15; worst `facts.Build` 26, `initcfg.writeModuleStanza` 26                                        | info           |
| `change_locality`      | n/a in `--full`; **release delta `v0.1.0..v0.2.0`: 18 cross-module edges from 45 changed files, forward reach 26** | info           |

## risk_hub (gitnexus-refined)

`risk_hub` = breadth × config-volatility × **gitnexus factor `[1.0–2.0]`** (historical change impact).
It is the only metric that consumes the gitnexus signal.

| Module             | Breadth | gitnexus factor | Risk   |
| ------------------ | ------- | --------------- | ------ |
| `internal/config`  | 114     | ×2.00           | 228.00 |
| `internal/initcfg` | 149     | ×1.33           | 198.67 |
| `model/signal`     | 70      | ×1.71           | 119.58 |
| `model/graph`      | 55      | ×1.92           | 105.42 |
| `model/diagnostic` | 58      | ×1.71           | 99.08  |

`internal/config` tops the list despite lower raw breadth than `internal/initcfg`, because it
carries the max historical-impact multiplier (`×2.00`) — gitnexus re-ranking structural risk by
how often the most-depended-on code actually changes.

## Release delta (`v0.1.0 → v0.2.0`)

Computed in delta mode by checking out `v0.2.0` (so `HEAD` = the tag) and running
`archfit check --base v0.1.0`. `scan`/`--full` always reports `change_locality` as `n/a`
because there is no diff to localize; the value is only defined against a base ref.

| Metric            | Result                                                        | Band   |
| ----------------- | ------------------------------------------------------------- | ------ |
| `change_locality` | 18 cross-module edges from 45 changed files, forward reach 26 | info   |
| `unbalanced_edge` | 0 new high-risk unbalanced edges                              | strong |

The 12 changed non-test Go files cluster in `cmd/archfit/*` (new CLI: `enrich`, `init`,
`update`, `doctor`) and `internal/initcfg/*` (`update`/`classify`/`yamledit`) — the **same
cluster the full scan flags as heaviest** (god-modules + top risk hubs + worst complexity).
So the v0.2.0 work landed in the densest, highest-fan-out part of the tree, yet
`unbalanced_edge = 0` confirms it did so through existing sanctioned dependencies on
`config`/`model`/`classify`, without introducing risky new coupling. Breadth, not risk.

## Tool coverage

| Tool                          | Status                                 |
| ----------------------------- | -------------------------------------- |
| `go/packages`                 | ok (103 files)                         |
| `scip` / `scip-symbols`       | ok (709 / 2767)                        |
| `ast-grep`, `loc`, `lizard`   | ok                                     |
| `gitnexus`                    | ok (73 files)                          |
| `dependency-cruiser`, `grimp` | absent (TS/Py — not relevant; Go repo) |
| `jscpd` (clones)              | absent (clone metric disabled)         |

## Operational notes

- **`change_locality` needs `--base <ref>`, not gitnexus.** In `--full` there is no diff to localize,
  so it reports `n/a` (never a false zero). Use delta mode in PR/CI checks.
- **CI degradation:** without the `gitnexus` binary + a fresh index, `risk_hub` still computes with
  `gn×1.00` (no historical refinement). For the real factor in CI, run `gitnexus analyze` and keep
  `gitnexus` on PATH. Refresh locally: `node .gitnexus/run.cjs analyze --index-only`
  (`--index-only` keeps it from rewriting `CLAUDE.md`/`AGENTS.md` or installing skills).

## Next actions (optional — all `info`, none gate)

Priority follows cross-metric convergence: a target that shows up in several metrics at
once is higher-leverage than any single number.

1. **Split `internal/initcfg` first.** It is the only module that is simultaneously a
   god-module (1842 LOC, 9×), home to the worst function (`writeModuleStanza` CCN 26), the
   #2 risk hub (199), **and** the heaviest-changed area in the `v0.1.0 → v0.2.0` delta.
   Natural seams already exist as files: `yamledit.go`, `classify_targets.go`, `update.go`.
2. **Decompose the 3 worst functions:** `facts.Build` (CCN 26), `initcfg.writeModuleStanza`
   (26), `modularity.Calculate` (23). Each is one function over CCN 15 — extract branch arms
   or table-drive; this also cuts the test-path burden.
3. **Treat `model/*` + `config` as a stable seam, not a refactor target.** High fan-in is
   correct for shared data types. `internal/config` is the top risk hub (gn×2.00) and a 35%
   blast hub — review config-schema changes harder rather than restructuring them.
4. **`cmd/archfit` god-module is lowest priority** — 1973 LOC of CLI shell spread across
   per-subcommand files; wide but logic-shallow, counted as one module because it is one package.
5. **Spot-check `internal/engine` hidden coupling (16 pairs)** only if engine edits keep
   dragging `internal/metrics` edits; likely benign (shared golden tests / pipeline conventions).
