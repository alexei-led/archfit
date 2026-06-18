# Archfit — Architecture Fitness Analysis

- **Target:** archfit (this repo)
- **Tool:** `archfit check --full --advisory --report` — `v0.2.0-1-g1b0d6d2-dirty`
- **Date:** 2026-06-17
- **Config:** `.archfit.yaml` (LLM-`update --apply` result; `gitnexus` + `risk_hub` enabled)
- **Verdict:** **PASS** — 0 gate findings, 0 warnings, 0 exceptions used.

## Scorecard

Bands: `strong` = healthy hard gate · `info` = report-only hotspot (never fails CI) ·
`n/a` = not computable in this mode.

| Metric                 | Result                                                                                                     | Band           |
| ---------------------- | ---------------------------------------------------------------------------------------------------------- | -------------- |
| `cycle`                | 0 import cycles                                                                                            | strong         |
| `coverage`             | 100% module coverage                                                                                       | strong         |
| `unbalanced_edge`      | 0 new high-risk unbalanced edges                                                                           | strong         |
| `encapsulation`        | n/a (no intrusive cross-module access detected)                                                            | n/a (low conf) |
| `risk_hub`             | 79 risk hubs (gitnexus-refined) — see below                                                                | info           |
| `blast_radius`         | 4 hubs: `model/graph` 75% (36 deps), `model/finding` 67%, `model/diagnostic` 58%, `config` 35%             | info           |
| `change_amplification` | 0 volatile change-impact hubs                                                                              | info           |
| `hidden_coupling`      | 64 pairs; top `internal/engine` (16), `internal/metrics` (9)                                               | info           |
| `structural_weight`    | 2 god-modules: `cmd/archfit` 1973 LOC (9×), `internal/initcfg` 1842 LOC (9×)                               | info           |
| `complexity`           | 14 functions CCN>15; worst `facts.Build` 26, `initcfg.writeModuleStanza` 26                                | info           |
| `change_locality`      | n/a in `--full`; **delta `--base HEAD~10`: 16 cross-module edges from 22 changed files, forward reach 24** | info           |

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
  `gitnexus` on PATH. Refresh locally: `node .gitnexus/run.cjs analyze`.

## Next actions (optional — all `info`, none gate)

1. `cmd/archfit` (1973 LOC) and `internal/initcfg` (1842 LOC) are god-modules at 9× median — split candidates.
2. `internal/config` is the top historical risk hub (gn×2.00) — changes there ripple widely; treat as a stable seam.
3. Top complexity offenders: `facts.Build` and `initcfg.writeModuleStanza` (CCN 26).
