# archfit report

**Verdict:** pass (exit 0)
**Config hash:** `c011e7b3502f123890c61c1832405ff60ae357fab73846193ddc94940285df42`

## Summary

- gate findings: 0
- warnings: 0
- exceptions used: 0

## Metrics

- **encapsulation**: n/a — n/a (low confidence)
- **unbalanced_edge**: 0 new high-risk unbalanced edges — strong
- **architecture_fitness**: 6.7/10 (2/3 signals); arch_tests: cmd/archfit/autopilot_test.go, cmd/archfit/enrich_subdomains_test.go, cmd/archfit/enrich_test.go (+79 more); ci_linter: .github/workflows/ci.yaml, .github/workflows/release.yaml — info
- **functional_candidates**: n/a — n/a (low confidence)
- **change_locality**: 71 cross-module edge(s) from 65 changed file(s); forward reach 69 file(s) — info

## Structural facts (neutral evidence)

140 modules; top 5 per axis (full list in `--format json`):

- inbound module fan-in: internal/model/diagnostic (49), internal/config (32), internal/model/graph (32), internal/model/finding (24), internal/toolrun (22)
- outbound destinations: cmd/archfit (40), internal/engine (17), internal/engine_test (16), internal/metrics/modularity (8), internal/extract/astgrep_test (7)
- LOC: cmd/archfit (3669), internal/initcfg (2205), internal/metrics/modularity (1515), internal/config (832), internal/engine (822)

## Dynamic / lazy imports (hidden-coupling risk)

Report-only. Dynamic/lazy imports are invisible to the static dependency
graph, so they can hide cycles and undercount coupling.

2 sites across 1 modules (full list in `--format json`):

- **internal**: 2 (e.g. internal/extract/py/grimp_helper.py:52[lazy_import], internal/extract/scip/scip_reader.py:39[lazy_import])

## Coverage gaps (3)

Analyzers that did not run. Their metrics are reported as n/a (never green) — install to measure them.

- **dependency-cruiser** [gate: warn] — affects coverage, coupling_balance, encapsulation, cycle, blast_radius
  - install: `npm install -g dependency-cruiser`
- **grimp** [gate: warn] — affects coverage, coupling_balance, encapsulation, cycle, blast_radius
  - install: `uv tool install grimp / pip install grimp`
- **jscpd** [gate: warn] — affects functional_candidates
  - install: `npm install -g jscpd`

## Supporting structural metrics (beyond Balanced Coupling)

Report-only. These metrics support Balanced Coupling reasoning but never gate.

- **cycle**: 0 import cycles — strong
- **coverage**: 100% coverage — strong
- **blast_radius**: 5 change-impact hub(s): .../model/graph (71%, 39 deps), .../model/finding (64%, 35 deps), .../model/diagnostic (56%, 31 deps), .../model/coupling (51%, 28 deps), internal/config (33%, 18 deps) — info
- **change_amplification**: 0 volatile change-impact hubs — info
- **hidden_coupling**: 76 hidden-coupling pair(s); top: internal/engine (16), .../output/markdown (10), internal/metrics (9), .../extract/scip (8) — info
- **structural_weight**: 3 god-module(s) (median 221 LOC): cmd/archfit (3669 LOC, 16x), internal/initcfg (2205 LOC, 9x), .../metrics/modularity (1515 LOC, 6x) — info
- **complexity**: 20 complex function(s) (CCN>15): writeModuleStanza CCN 30 (.../initcfg/initcfg.go:370), Build CCN 26 (.../facts/facts.go:32), Calculate CCN 23 (.../modularity/hidden_coupling.go:27), Extract CCN 22 (.../golang/golang.go:43), Calculate CCN 22 (.../modularity/change_coupling.go:50)+15 more — info
- **risk_hub**: 91 risk hub(s): internal/config [breadth 142, ×0.66, gn×1.87→174.94], internal/initcfg [breadth 208, ×0.66, gn×1.27→173.89], .../model/diagnostic [breadth 84, ×1.00, gn×1.73→145.60], .../model/signal [breadth 74, ×1.00, gn×1.70→125.80], .../model/coupling [breadth 69, ×1.00, gn×1.73→119.60]+86 more — info
- **instability**: 17 unstable modules (I>0.7): cmd/archfit (I=1.00), .../extract/runtime (I=1.00), .../metrics/metricstest (I=1.00), internal/engine (I=0.94), .../metrics/modularity (I=0.89)+12 more — info
- **propagation_cost**: PC=0.092 (N=56); 1 modules reach >50%: cmd/archfit (95%) — info
- **change_coupling**: 1 change-coupled pair(s) (CC≥65%); top: internal/engine↔.../model/signal (CC=100%) — info

> Low-confidence proxies (footnote — full values in `--format json`).
> Derived without SCIP type kinds; do not read as authoritative.
> - abstractness: 0 modules with A>0.5 — info (low confidence)
> - martin_distance: 15 modules in zone of pain/uselessness (Dms>0.5): internal (Dms=1.00, A=0.00, I=0.00), internal/labels (Dms=1.00, A=0.00, I=0.00), internal/llm (Dms=1.00, A=0.00, I=0.00), .../model/clone (Dms=1.00, A=0.00, I=0.00), .../model/coupling (Dms=1.00, A=0.00, I=0.00)+10 more — info (low confidence)

## Distance confidence

- `code_structure`: always on (deterministic tree-distance baseline)
- `owner_source`: not reported (CODEOWNERS or git-author fallback)
- `deploy_unit_source`: not reported (auto-detect or config-authored)

## Coverage

- scip: ok (903 files)
- scip-symbols: ok (3825 files)
- go/packages: ok (128 files)
- dependency-cruiser: absent
- grimp: absent
- loc: ok (130 files)
- lizard: ok
- jscpd: absent — clone detection is opt-in — set `tools.clones.enabled: true` in .archfit.yaml
- gitnexus: ok (94 files)
- ast-grep: ok
