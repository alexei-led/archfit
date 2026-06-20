# archfit report

**Verdict:** pass (exit 0)
**Config hash:** `742f2e72c04d9ba680d112cad95156c5d77e3f60f2716ed9fdc71cfef00a15c7`

## Summary

- gate findings: 0
- warnings: 0
- exceptions used: 0

## Metrics

- **encapsulation**: n/a — n/a (low confidence)
- **unbalanced_edge**: 0 new high-risk unbalanced edges — strong
- **cohesion_lcom**: n/a — n/a (low confidence)
- **architecture_fitness**: 3.3/10 (1/3 signals); arch_tests: __tests__/search-query-parser.test.ts — info
- **functional_candidates**: 86 clone-duplicated cross-module pair(s) (51 also co-change) — info
- **change_locality**: n/a — n/a (low confidence)

## Structural facts (neutral evidence)

124 modules; top 5 per axis (full list in `--format json`):

- inbound module fan-in: src/types.ts (55), src/resolution/types.ts (28), src/extraction/tree-sitter-helpers.ts (26), src/web-tree-sitter.d.ts (22), src/extraction/tree-sitter-types.ts (20)
- outbound destinations: src/resolution/frameworks/index.ts (21), src/extraction/languages/index.ts (20), src/index.ts (20), src/bin/codegraph.ts (18), src/extraction/tree-sitter.ts (14)
- LOC: src/extraction/tree-sitter.ts (4214), src/mcp/tools.ts (3375), src/resolution/import-resolver.ts (1825), src/db/queries.ts (1817), src/bin/codegraph.ts (1727)

## Dynamic / lazy imports (hidden-coupling risk)

Report-only. Dynamic/lazy imports are invisible to the static dependency
graph, so they can hide cycles and undercount coupling.

111 sites across 9 modules (full list in `--format json`):

- **__tests__**: 49 (e.g. __tests__/drupal.test.ts:91[require], __tests__/extraction.test.ts:1866[dynamic_import], __tests__/extraction.test.ts:4382[require])
- **bin**: 18 (e.g. src/bin/codegraph.ts:39[dynamic_import], src/bin/codegraph.ts:41[dynamic_import], src/bin/codegraph.ts:55[dynamic_import])
- **scripts**: 13 (e.g. scripts/npm-sdk.js:26[require], scripts/npm-sdk.js:27[require], scripts/npm-sdk.js:28[require])
- **extraction**: 9 (e.g. src/extraction/index.ts:675[dynamic_import], src/extraction/index.ts:678[dynamic_import], src/extraction/index.ts:690[dynamic_import])
- **installer**: 9 (e.g. src/installer/index.ts:46[dynamic_import], src/installer/index.ts:47[dynamic_import], src/installer/index.ts:386[require])
- **mcp**: 6 (e.g. src/mcp/engine.ts:24[dynamic_import], src/mcp/engine.ts:25[dynamic_import], src/mcp/engine.ts:25[require])
- **resolution**: 3 (e.g. src/resolution/types.ts:93[dynamic_import], src/resolution/types.ts:101[dynamic_import], src/resolution/types.ts:108[dynamic_import])
- **src**: 3 (e.g. src/index.ts:694[dynamic_import], src/utils.ts:145[require], src/utils.ts:409[dynamic_import])
- **db**: 1 (e.g. src/db/sqlite-adapter.ts:55[require])

## Supporting structural metrics (beyond Balanced Coupling)

Report-only. These metrics support Balanced Coupling reasoning but never gate.

- **cycle**: 2 import cycles — critical
- **coverage**: 100% coverage — strong
- **blast_radius**: 1 change-impact hub(s): .../tree-sitter-helpers/ts (35%, 41 deps) — info
- **change_amplification**: 0 volatile change-impact hubs — info
- **hidden_coupling**: 22 hidden-coupling pair(s); top: .../tree-sitter/ts (11), .../index/ts (3), .../java/ts (3), .../codegraph/ts (2) — info
- **structural_weight**: 11 god-module(s) (median 198 LOC): .../tree-sitter/ts (4214 LOC, 21x), .../tools/ts (3375 LOC, 17x), .../import-resolver/ts (1825 LOC, 9x), .../queries/ts (1817 LOC, 9x), .../codegraph/ts (1727 LOC, 8x)+6 more — info
- **complexity**: 55 complex function(s) (CCN>15): extractDrupalRoutes CCN 58 (.../frameworks/drupal.ts:97), rnEventEdges CCN 57 (.../resolution/callback-synthesizer.ts:1035), extractFromSpec CCN 55 (.../extraction/tree-sitter.ts:1907), resolveViaImport CCN 39 (.../resolution/import-resolver.ts:1077), collect CCN 37 (.../extraction/tree-sitter.ts:2074)+50 more — info
- **risk_hub**: 120 risk hub(s): .../types/ts [breadth 135, ×1.00, gn×1.00→135.00], .../types/ts [breadth 57, ×1.00, gn×1.00→57.00], .../tree-sitter-types/ts [breadth 56, ×1.00, gn×1.00→56.00], .../queries/ts [breadth 46, ×1.00, gn×1.00→46.00], .../index/ts [breadth 45, ×1.00, gn×1.00→45.00]+115 more — info
- **instability**: 20 unstable modules (I>0.7): .../codegraph/ts (I=1.00), .../uninstall/ts (I=1.00), .../index/ts (I=0.95), .../index/ts (I=0.91), .../index/ts (I=0.88)+15 more — info
- **propagation_cost**: PC=0.106 (N=118); 10 modules reach >50%: .../codegraph/ts (99%), .../index/ts (97%), .../index/ts (83%), .../daemon/ts (83%), .../engine/ts (83%)+5 more — info
- **change_coupling**: 23 change-coupled pair(s) (CC≥65%); top: .../c-cpp/ts↔.../tree-sitter/ts (CC=100%), .../csharp/ts↔.../tree-sitter/ts (CC=100%), .../java/ts↔.../tree-sitter/ts (CC=100%), .../php/ts↔.../tree-sitter/ts (CC=100%) — info

> Low-confidence proxies (footnote — full values in `--format json`).
> Derived without SCIP type kinds; do not read as authoritative.
> - abstractness: 0 modules with A>0.5 — info (low confidence)
> - martin_distance: 45 modules in zone of pain/uselessness (Dms>0.5): .../node-version-check/ts (Dms=1.00, A=0.00, I=0.00), .../markers/ts (Dms=1.00, A=0.00, I=0.00), .../migrations/ts (Dms=1.00, A=0.00, I=0.00), .../sqlite-adapter/ts (Dms=1.00, A=0.00, I=0.00), .../directory/ts (Dms=1.00, A=0.00, I=0.00)+40 more — info (low confidence)

## Distance confidence

- `code_structure`: always on (deterministic tree-distance baseline)
- `owner_source`: not reported (CODEOWNERS or git-author fallback)
- `deploy_unit_source`: not reported (auto-detect or config-authored)
- unresolved modules: 1 (edges to unknown modules use conservative distance)

## Coverage

- scip: ok (404 files)
- scip-symbols: ok (5832 files)
- go/packages: absent
- dependency-cruiser: partial (117 files)
- grimp: absent
- loc: ok (131 files)
- lizard: ok
- jscpd: ok (324 files)
- gitnexus: ok (28 files)
- ast-grep: ok
