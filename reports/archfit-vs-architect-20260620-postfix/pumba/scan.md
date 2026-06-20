# archfit report

**Verdict:** pass (exit 0)
**Config hash:** `5f5b284a102aaae6cf852e82420dcb7cc2b83a780612549b324c29907da46eaa`

## Summary

- gate findings: 0
- warnings: 3
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

## Balanced Coupling advisories (3 rollups, 11 edges)

Same-shape edges between a module pair are grouped into one rollup.
Integration strength × distance × volatility lint messages.
Severity: `none` · `low` · `medium` · `high` · `critical`.

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/commands.go -> pkg/chaos  [075afa2f]
  integration strength: contract      distance: cross_module_same_owner         volatility: high
  score: 8/10 (high) [multiplicative]
  why: balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: reduce_distance
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/logging.go -> pkg/chaos/cliflags  [03cd1bb5]
  integration strength: unknown       distance: cross_module_same_owner         volatility: high
  score: 8/10 (high) [multiplicative]
  why: balanced coupling: unknown integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: reduce_distance
  rollup: 7 same-shape edges (e.g. 03cd1bb5d17a03aa0d4a3e1537a7651d,335d55e575e36b47a906ba5e7396a727,47d7034d8bd0aa2c3fd6d7321e5baf7f,8377db42015d1664c7f7e5b2276fcde7,8eae8eb9df709e2e84471ddbb39f9a9c,b94ca1d9b02e368b07146e4e85360565,d8729f023289c590ed80b02abb66bf05)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/runtime.go -> pkg/runtime/docker  [57e8e1a4]
  integration strength: unknown       distance: cross_module_same_owner         volatility: high
  score: 8/10 (high) [multiplicative]
  why: balanced coupling: unknown integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: reduce_distance
  rollup: 3 same-shape edges (e.g. 57e8e1a409d3563c38f408a6d1a33d31,bb31aa361c2b39f9d59409a1e3295480,dd0c222b483eaec864d5a2431c3e9862)
```


## Supporting structural metrics (beyond Balanced Coupling)

Report-only. These metrics support Balanced Coupling reasoning but never gate.

- **cycle**: 0 import cycles — strong
- **coverage**: 100% coverage — strong
- **blast_radius**: 3 change-impact hub(s): pkg/container (82%, 14 deps), .../chaos/cliflags (65%, 11 deps), pkg/chaos (59%, 10 deps) — info
- **change_amplification**: 1 volatile hub(s): pkg/container (amp 0.19: 14 deps × 169 commits) — info
- **hidden_coupling**: 16 hidden-coupling pair(s); top: .../runtime/docker (5), mocks (4), .../chaos/netem (4), .../chaos/stress (4) — info
- **structural_weight**: 1 god-module(s) (median 366 LOC): .../runtime/docker (1503 LOC, 4x) — info
- **complexity**: n/a — n/a (low confidence)
- **risk_hub**: n/a — n/a (low confidence)
- **instability**: 7 unstable modules (I>0.7): cmd (I=1.00), .../iptables/cmd (I=0.83), .../lifecycle/cmd (I=0.83), .../netem/cmd (I=0.83), .../stress/cmd (I=0.83)+2 more — info
- **propagation_cost**: PC=0.199 (N=18); 1 modules reach >50%: cmd (94%) — info
- **change_coupling**: 0 change-coupled pair(s) (CC≥65%) — info

> Low-confidence proxies (footnote — full values in `--format json`).
> Derived without SCIP type kinds; do not read as authoritative.
> - abstractness: 3 abstract modules (A>0.5): pkg/chaos (A=1.00), pkg/container (A=1.00), pkg/util (A=1.00) — info (low confidence)
> - martin_distance: 4 modules in zone of pain/uselessness (Dms>0.5): mocks (Dms=1.00, A=0.00, I=0.00), .../chaos/cliflags (Dms=1.00, A=0.00, I=0.00), .../runtime/docker (Dms=0.67, A=0.00, I=0.33), .../chaos/cmd (Dms=0.57, A=0.00, I=0.43) — info (low confidence)

## Distance confidence

- `code_structure`: always on (deterministic tree-distance baseline)
- `owner_source`: not reported (CODEOWNERS or git-author fallback)
- `deploy_unit_source`: not reported (auto-detect or config-authored)

## Coverage

- scip: absent
- scip-symbols: absent
- go/packages: ok (95 files)
- dependency-cruiser: absent
- grimp: absent
- loc: ok (80 files)
- lizard: absent — complexity is opt-in — set `tools.complexity.enabled: true` in .archfit.yaml
- jscpd: absent — clone detection is opt-in — set `tools.clones.enabled: true` in .archfit.yaml
- gitnexus: ok (71 files) — gitnexus index auto-detected (.gitnexus/.codegraph present); refresh with `node .gitnexus/run.cjs analyze --index-only`
- ast-grep: ok
