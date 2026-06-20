# archfit report

**Verdict:** pass (exit 0)
**Config hash:** `25f594782358e5c2786fd8a0d32104e7e7c421dec76cab1bb3ae5d51a9ec2264`

## Summary

- gate findings: 0
- warnings: 0
- exceptions used: 0

## Metrics

- **encapsulation**: n/a — n/a (low confidence)
- **unbalanced_edge**: 0 new high-risk unbalanced edges — strong
- **cohesion_lcom**: 0 fragmented module(s) (3 measurable) — info (low confidence)
- **architecture_fitness**: 0.0/10 (0/3 signals) — info
- **functional_candidates**: 1 clone-duplicated cross-module pair(s) (1 also co-change) — info
- **change_locality**: n/a — n/a (low confidence)

## Structural facts (neutral evidence)

6 modules; top 5 per axis (full list in `--format json`):

- inbound module fan-in: internal/spot (3), internal/mcp (2), cmd/spotinfo (1)
- outbound destinations: cmd/spotinfo (2), cmd/spotinfo.test (1), internal/mcp (1), internal/mcp.test (1), internal/spot.test (1)
- LOC: internal/spot (1433), cmd/spotinfo (712), internal/mcp (518)

## Supporting structural metrics (beyond Balanced Coupling)

Report-only. These metrics support Balanced Coupling reasoning but never gate.

- **cycle**: 0 import cycles — strong
- **coverage**: 100% coverage — strong
- **blast_radius**: 2 change-impact hub(s): internal/spot (100%, 2 deps), internal/mcp (50%, 1 deps) — info (low confidence)
- **change_amplification**: 2 volatile hub(s): internal/spot (amp 0.70: 2 deps × 19 commits), internal/mcp (amp 0.26: 1 deps × 14 commits) — info (low confidence)
- **hidden_coupling**: 0 hidden-coupling pair(s) — info (low confidence)
- **structural_weight**: 0 god-modules (median 712 LOC) — info (low confidence)
- **complexity**: 3 complex function(s) (CCN>15): GetSpotSavings CCN 25 (.../spot/client.go:183), execMainCmd CCN 23 (.../spotinfo/main.go:166), resolveOnPath CCN 16 (.gitnexus/run.cjs:64) — info
- **risk_hub**: 3 risk hub(s): internal/spot [breadth 72, ×1.00, gn×2.00→144.00], internal/mcp [breadth 62, ×1.00, gn×1.45→90.18], cmd/spotinfo [breadth 21, ×1.00, gn×1.09→22.91] — info
- **instability**: 1 unstable modules (I>0.7): cmd/spotinfo (I=1.00) — info (low confidence)
- **propagation_cost**: PC=0.500 (N=3); 1 modules reach >50%: cmd/spotinfo (100%) — info (low confidence)
- **change_coupling**: 0 change-coupled pair(s) (CC≥65%) — info (low confidence)

> Low-confidence proxies (footnote — full values in `--format json`).
> Derived without SCIP type kinds; do not read as authoritative.
> - abstractness: 0 modules with A>0.5 — info (low confidence)
> - martin_distance: 1 modules in zone of pain/uselessness (Dms>0.5): internal/spot (Dms=1.00, A=0.00, I=0.00) — info (low confidence)

## Distance confidence

- `code_structure`: always on (deterministic tree-distance baseline)
- `owner_source`: not reported (CODEOWNERS or git-author fallback)
- `deploy_unit_source`: not reported (auto-detect or config-authored)

## Coverage

- scip: ok (41 files)
- scip-symbols: ok (633 files)
- go/packages: ok (8 files)
- dependency-cruiser: absent
- grimp: absent
- loc: ok (8 files)
- lizard: ok
- jscpd: ok (61 files)
- gitnexus: ok (10 files)
- ast-grep: ok
