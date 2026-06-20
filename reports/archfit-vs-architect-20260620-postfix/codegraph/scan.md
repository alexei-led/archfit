# archfit report

**Verdict:** fail (exit 1)
**Config hash:** `a32c8a1ba873e6de25d9596a6b052c8ecc7b19b3237b42af245fc652156583d6`

## Summary

- gate findings: 2
- warnings: 7
- exceptions used: 0

## Metrics

- **encapsulation**: n/a — n/a (low confidence)
- **unbalanced_edge**: 0 new high-risk unbalanced edges — strong
- **cohesion_lcom**: n/a — n/a (low confidence)
- **architecture_fitness**: 3.3/10 (1/3 signals); arch_tests: __tests__/search-query-parser.test.ts — info
- **functional_candidates**: n/a — n/a (low confidence)
- **change_locality**: n/a — n/a (low confidence)

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

## Coverage gaps (4)

Analyzers that did not run. Their metrics are reported as n/a (never green) — install to measure them.

- **go/packages** [gate: warn] — affects coverage, coupling_balance, encapsulation, cycle, blast_radius
  - install: `https://go.dev/dl (bundled with the Go toolchain)`
- **grimp** [gate: warn] — affects coverage, coupling_balance, encapsulation, cycle, blast_radius
  - install: `uv tool install grimp / pip install grimp`
- **jscpd** [gate: warn] — affects functional_candidates
  - install: `npm install -g jscpd`
- **lizard** [gate: warn] — affects complexity
  - install: `uv tool install lizard / pip install lizard`

## Gate findings (2)

- **no-import-cycles** [high] new — src/extraction/razor-extractor.ts → src/extraction/svelte-extractor.ts: Import cycle detected among 4 nodes: file:src/extraction/razor-extractor.ts → file:src/extraction/svelte-extractor.ts → file:src/extr...
- **no-import-cycles** [high] new — src/index.ts → src/mcp/daemon.ts: Import cycle detected among 7 nodes: file:src/index.ts → file:src/mcp/daemon.ts → file:src/mcp/engine.ts → file:src/mcp/index.ts �...

## Agent tasks (2)

- **no-import-cycles** [`20ebf187`] Break the import cycle: Import cycle detected among 4 nodes: file:src/extraction/razor-extractor.ts → file:src/extraction/svelte-extractor.ts → file:src/extraction/tree-sitter.ts → file:src/extraction/vue-extractor.ts
  - files: src/extraction/razor-extractor.ts, src/extraction/svelte-extractor.ts
  - constraint: Break the cycle by introducing an abstraction or reorganizing packages
  - constraint: public surface of module "extraction": [src/extraction/**]
  - validate: `archfit check -c /Users/alexei/Workspace/codegraph/.archfit.yaml --full`
- **no-import-cycles** [`61223c3a`] Break the import cycle: Import cycle detected among 7 nodes: file:src/index.ts → file:src/mcp/daemon.ts → file:src/mcp/engine.ts → file:src/mcp/index.ts → file:src/mcp/proxy.ts → file:src/mcp/session.ts → file:src/mcp/tools.ts
  - files: src/index.ts, src/mcp/daemon.ts
  - constraint: Break the cycle by introducing an abstraction or reorganizing packages
  - constraint: public surface of module "mcp": [src/mcp/**]
  - validate: `archfit check -c /Users/alexei/Workspace/codegraph/.archfit.yaml --full`

## Balanced Coupling advisories (7 rollups, 14 edges)

Same-shape edges between a module pair are grouped into one rollup.
Integration strength × distance × volatility lint messages.
Severity: `none` · `low` · `medium` · `high` · `critical`.

```
ARCHFIT[BC-UNBALANCED MEDIUM] src/bin/codegraph.ts -> src/extraction/extraction-version.ts  [5073ad80]
  integration strength: contract      distance: cross_module_same_owner         volatility: high
  score: 8/10 (high) [multiplicative]
  why: balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: reduce_distance
  rollup: 3 same-shape edges (e.g. 5073ad803a43019ec16b22915c4d4885,7a49c44627193bf4cc1ef6c95260af8e,fc16b13d115a3c9e1e9c41df2affc0b4)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] src/context/formatter.ts -> src/extraction/generated-detection.ts  [fde0b024]
  integration strength: contract      distance: cross_module_same_owner         volatility: high
  score: 8/10 (high) [multiplicative]
  why: balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: reduce_distance
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] src/db/queries.ts -> src/extraction/generated-detection.ts  [9251b373]
  integration strength: contract      distance: cross_module_same_owner         volatility: high
  score: 8/10 (high) [multiplicative]
  why: balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: reduce_distance
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] src/extraction/index.ts -> src/resolution/frameworks/index.ts  [649914e7]
  integration strength: contract      distance: cross_module_same_owner         volatility: high
  score: 8/10 (high) [multiplicative]
  why: balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: reduce_distance
  rollup: 2 same-shape edges (e.g. 649914e7c8c6ed7e205dfe79e8238334,75eb3df5873ebb1fd4068831a0f90aaa)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] src/mcp/index.ts -> src/extraction/wasm-runtime-flags.ts  [3c0c8e1f]
  integration strength: contract      distance: cross_module_same_owner         volatility: high
  score: 8/10 (high) [multiplicative]
  why: balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: reduce_distance
  rollup: 3 same-shape edges (e.g. 3c0c8e1f379045dbbfeeb61f88c5ee19,4e758742d6c222870818c1bbf7c7a2d2,594e73e55c1c3ee676e92d7dad0ad131)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] src/resolution/frameworks/drupal.ts -> src/extraction/tree-sitter-helpers.ts  [233b335a]
  integration strength: contract      distance: cross_module_same_owner         volatility: high
  score: 8/10 (high) [multiplicative]
  why: balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: reduce_distance
  rollup: 3 same-shape edges (e.g. 233b335a4ad9a1aadb578b09d3da279b,42e687642abbbfc2326a00f359bccbf2,53075202ab7dd7beea642f59675c87e1)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] src/sync/watcher.ts -> src/extraction/index.ts  [a67b8478]
  integration strength: contract      distance: cross_module_same_owner         volatility: high
  score: 8/10 (high) [multiplicative]
  why: balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: reduce_distance
```


## Supporting structural metrics (beyond Balanced Coupling)

Report-only. These metrics support Balanced Coupling reasoning but never gate.

- **cycle**: 2 import cycles — critical
- **coverage**: 100% coverage — strong
- **blast_radius**: 1 change-impact hub(s): .../tree-sitter-helpers/ts (35%, 41 deps) — info
- **change_amplification**: 0 volatile change-impact hubs — info
- **hidden_coupling**: 22 hidden-coupling pair(s); top: .../tree-sitter/ts (11), .../index/ts (3), .../java/ts (3), .../codegraph/ts (2) — info
- **structural_weight**: 11 god-module(s) (median 198 LOC): .../tree-sitter/ts (4214 LOC, 21x), .../tools/ts (3375 LOC, 17x), .../import-resolver/ts (1825 LOC, 9x), .../queries/ts (1817 LOC, 9x), .../codegraph/ts (1727 LOC, 8x)+6 more — info
- **complexity**: n/a — n/a (low confidence)
- **risk_hub**: n/a — n/a (low confidence)
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

- scip: absent
- scip-symbols: absent
- go/packages: absent
- dependency-cruiser: partial (117 files)
- grimp: absent
- loc: ok (131 files)
- lizard: absent — complexity is opt-in — set `tools.complexity.enabled: true` in .archfit.yaml
- jscpd: absent — clone detection is opt-in — set `tools.clones.enabled: true` in .archfit.yaml
- gitnexus: ok (28 files) — gitnexus index auto-detected (.gitnexus/.codegraph present); refresh with `node .gitnexus/run.cjs analyze --index-only`
- ast-grep: ok
