# archfit report

**Verdict:** pass (exit 0)
**Config hash:** `88062c8802fa14d4d58949ce8cb56e7f884474db5f561a8ee67c25c395b7b95a`

## Summary

- gate findings: 0
- warnings: 2
- exceptions used: 0

## Metrics

- **encapsulation**: n/a — n/a (low confidence)
- **unbalanced_edge**: 0 new high-risk unbalanced edges — strong
- **cohesion_lcom**: 0 fragmented module(s) (19 measurable) — info
- **architecture_fitness**: 0.0/10 (0/3 signals) — info
- **functional_candidates**: 13 clone-duplicated cross-module pair(s) (9 also co-change) — info
- **change_locality**: n/a — n/a (low confidence)

## Structural facts (neutral evidence)

38 modules; top 5 per axis (full list in `--format json`):

- inbound module fan-in: pkg/container (16), pkg/chaos (13), pkg/chaos/cliflags (11), pkg/chaos/cmd (5), mocks (3)
- outbound destinations: cmd (10), pkg/chaos/iptables/cmd (5), pkg/chaos/lifecycle/cmd (5), pkg/chaos/netem/cmd (5), pkg/chaos/stress/cmd (5)
- LOC: pkg/runtime/docker (1503), pkg/runtime/containerd (1291), pkg/runtime/podman (938), pkg/chaos/netem (818), pkg/chaos/lifecycle (520)

## Balanced Coupling advisories (2 rollups, 10 edges)

Same-shape edges between a module pair are grouped into one rollup.
Integration strength × distance × volatility lint messages.
Severity: `none` · `low` · `medium` · `high` · `critical`.

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/logging.go -> pkg/chaos/cliflags  [03cd1bb5]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
  rollup: 7 same-shape edges (e.g. 03cd1bb5d17a03aa0d4a3e1537a7651d,335d55e575e36b47a906ba5e7396a727,47d7034d8bd0aa2c3fd6d7321e5baf7f,8377db42015d1664c7f7e5b2276fcde7,8eae8eb9df709e2e84471ddbb39f9a9c,b94ca1d9b02e368b07146e4e85360565,d8729f023289c590ed80b02abb66bf05)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/runtime.go -> pkg/runtime/docker  [57e8e1a4]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
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
- **complexity**: 1 complex function(s) (CCN>15): resolveOnPath CCN 16 (.gitnexus/run.cjs:64) — info
- **risk_hub**: 20 risk hub(s): pkg/container [breadth 139, ×1.00, gn×1.66→230.54], .../runtime/docker [breadth 67, ×1.00, gn×1.17→78.44], .../runtime/containerd [breadth 68, ×1.00, gn×1.05→71.32], .../runtime/podman [breadth 61, ×1.00, gn×1.07→65.46], .../chaos/netem [breadth 42, ×1.00, gn×1.20→50.20]+15 more — info
- **instability**: 7 unstable modules (I>0.7): cmd (I=1.00), .../iptables/cmd (I=0.83), .../lifecycle/cmd (I=0.83), .../netem/cmd (I=0.83), .../stress/cmd (I=0.83)+2 more — info
- **propagation_cost**: PC=0.199 (N=18); 1 modules reach >50%: cmd (94%) — info
- **change_coupling**: 0 change-coupled pair(s) (CC≥65%) — info

> Low-confidence proxies (footnote — full values in `--format json`).
> Derived without SCIP type kinds; do not read as authoritative.
> - abstractness: 0 modules with A>0.5 — info (low confidence)
> - martin_distance: 7 modules in zone of pain/uselessness (Dms>0.5): mocks (Dms=1.00, A=0.00, I=0.00), .../chaos/cliflags (Dms=1.00, A=0.00, I=0.00), pkg/container (Dms=1.00, A=0.00, I=0.00), pkg/util (Dms=1.00, A=0.00, I=0.00), pkg/chaos (Dms=0.83, A=0.00, I=0.17)+2 more — info (low confidence)

## Distance confidence

- `code_structure`: always on (deterministic tree-distance baseline)
- `owner_source`: not reported (CODEOWNERS or git-author fallback)
- `deploy_unit_source`: not reported (auto-detect or config-authored)

## Coverage

- scip: ok (424 files)
- scip-symbols: ok (3012 files)
- go/packages: ok (95 files)
- dependency-cruiser: absent
- grimp: absent
- loc: ok (80 files)
- lizard: ok
- jscpd: ok (256 files)
- gitnexus: ok (71 files)
- ast-grep: ok
