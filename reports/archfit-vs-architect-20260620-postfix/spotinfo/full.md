# archfit report

**Verdict:** pass (exit 0)
**Config hash:** `cd7d705a95a4b3e05f6f3864e4158042a3ba8616112fdefb15bd130242e1b4e0`

## Summary

- gate findings: 0
- warnings: 0
- exceptions used: 0

## Metrics

- **encapsulation**: n/a — n/a (low confidence)
- **unbalanced_edge**: 0 new high-risk unbalanced edges — strong
- **cohesion_lcom**: n/a — n/a (low confidence)
- **architecture_fitness**: 0.0/10 (0/3 signals) — info
- **functional_candidates**: n/a — n/a (low confidence)
- **change_locality**: n/a — n/a (low confidence)

## Coverage gaps (4)

Analyzers that did not run. Their metrics are reported as n/a (never green) — install to measure them.

- **dependency-cruiser** [gate: warn] — affects coverage, coupling_balance, encapsulation, cycle, blast_radius
  - install: `npm install -g dependency-cruiser`
- **grimp** [gate: warn] — affects coverage, coupling_balance, encapsulation, cycle, blast_radius
  - install: `uv tool install grimp / pip install grimp`
- **jscpd** [gate: warn] — affects functional_candidates
  - install: `npm install -g jscpd`
- **lizard** [gate: warn] — affects complexity
  - install: `uv tool install lizard / pip install lizard`

## Supporting structural metrics (beyond Balanced Coupling)

Report-only. These metrics support Balanced Coupling reasoning but never gate.

- **cycle**: 0 import cycles — strong
- **coverage**: 100% coverage — strong
- **blast_radius**: 2 change-impact hub(s): internal/spot (100%, 2 deps), internal/mcp (50%, 1 deps) — info (low confidence)
- **change_amplification**: 2 volatile hub(s): internal/spot (amp 0.70: 2 deps × 19 commits), internal/mcp (amp 0.26: 1 deps × 14 commits) — info (low confidence)
- **hidden_coupling**: 0 hidden-coupling pair(s) — info (low confidence)
- **structural_weight**: 0 god-modules (median 712 LOC) — info (low confidence)
- **complexity**: n/a — n/a (low confidence)
- **risk_hub**: n/a — n/a (low confidence)
- **instability**: 1 unstable modules (I>0.7): cmd/spotinfo (I=1.00) — info (low confidence)
- **propagation_cost**: PC=0.500 (N=3); 1 modules reach >50%: cmd/spotinfo (100%) — info (low confidence)
- **change_coupling**: 0 change-coupled pair(s) (CC≥65%) — info (low confidence)

> Low-confidence proxies (footnote — full values in `--format json`).
> Derived without SCIP type kinds; do not read as authoritative.
> - abstractness: 2 abstract modules (A>0.5): internal/mcp (A=1.00), internal/spot (A=1.00) — info (low confidence)
> - martin_distance: 0 modules in zone of pain/uselessness (Dms>0.5) — info (low confidence)

## Distance confidence

- `code_structure`: always on (deterministic tree-distance baseline)
- `owner_source`: not reported (CODEOWNERS or git-author fallback)
- `deploy_unit_source`: not reported (auto-detect or config-authored)

## Coverage

- scip: absent
- scip-symbols: absent
- go/packages: ok (8 files)
- dependency-cruiser: absent
- grimp: absent
- loc: ok (8 files)
- lizard: absent — complexity is opt-in — set `tools.complexity.enabled: true` in .archfit.yaml
- jscpd: absent — clone detection is opt-in — set `tools.clones.enabled: true` in .archfit.yaml
- gitnexus: ok (10 files) — gitnexus index auto-detected (.gitnexus/.codegraph present); refresh with `node .gitnexus/run.cjs analyze --index-only`
- ast-grep: ok
