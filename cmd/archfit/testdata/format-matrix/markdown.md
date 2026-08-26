# archfit — decision

- **Decision:** FAIL
- **Gate:** FAIL — 1 blocking
- **Warnings:** 1 advisory
- **Score:** 78 / 100 (serviceable)

Gate violations. Fix before merge.

## Recommendations

### Must fix
- **no_direct_b_dependency** — Import from pkg/a/** to pkg/b/** is explicitly forbidden

### Should fix
- none

### Watch
- **bc/imbalanced_coupling** — balanced coupling: functional integration strength × cross_module_different_owner distance × low volatility → low severity (unbalanced coupling → elevated maintenance effort)
# archfit report

**Verdict:** fail (exit 1)
**Config hash:** `e686c0a4ca1ab9671d3ddc6d48da64e60ff9d2fd49d1cf9573c7e4c15850d826`

## Summary

- gate findings: 1
- warnings: 1
- waivers used: 0

## Metrics

- **encapsulation**: n/a — n/a (low confidence)
- **unbalanced_edge**: 0 new high-risk unbalanced edges — strong

## Connascence evidence (deterministic)

Report-only. Static facts only; semantic and dynamic categories without deterministic evidence stay unmeasured.

- edges with evidence: 1
- abstained edges: 0
- total evidence facts: 2
- by kind: algorithm=1, name=1
- by source: go/types=2
- unmeasured: position, execution, timing, value, identity
- roadmap: name=deterministic_static, type=deterministic_static, meaning=deterministic_static, algorithm=deterministic_static, position=unmeasured_static, execution=unmeasured_dynamic (signals dynamic_imports/runtime_async_edges), timing=unmeasured_dynamic (signals dynamic_imports/runtime_async_edges), value=unmeasured_dynamic (signals dynamic_imports/runtime_async_edges), identity=unmeasured_dynamic (signals dynamic_imports/runtime_async_edges)

## Gate findings (1)

- **no_direct_b_dependency** [medium] new — pkg/a/a.go → pkg/b: Import from pkg/a/** to pkg/b/** is explicitly forbidden

## Agent tasks (1)

- **no_direct_b_dependency** [`ba3803ec`] Remove the forbidden dependency from pkg/a/a.go on pkg/b; depend on b's public API or move the shared code to an allowed location.
  - files: pkg/a/a.go, pkg/b
  - constraint: Remove the dependency or move the code
  - validate: `archfit check -c <ROOT>/.archfit.yaml`

## Balanced Coupling advisories (1 rollups, 1 edges)

Same-shape edges between a module pair are grouped into one rollup.
Integration strength × distance × volatility lint messages.
Severity: `none` · `low` · `medium` · `high` · `critical`.

```
ARCHFIT[BC-UNBALANCED LOW] pkg/a/a.go -> pkg/b  [07af466d]
  integration strength: functional    distance: cross_module_different_owner    volatility: low
  score: 8/10 (low) [book]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × low volatility → low severity (unbalanced coupling → elevated maintenance effort)
```


## Supporting structural metrics (beyond Balanced Coupling)

Report-only. These metrics support Balanced Coupling reasoning but never gate.

- **cycle**: 0 import cycles — strong
- **coverage**: 100% coverage — strong
- **blast_radius**: 1 of 2 modules are change-impact hubs: pkg/b (100%, 1 deps) — info (low confidence)

## Distance confidence

- `code_structure`: always on (deterministic tree-distance baseline)
- `owner_source`: config
- `deploy_unit_source`: ok
- `owner_model`: multi_owner
- distance basis: ownership=1
- interpretation: ownership has multiple distinct owners, so owner distance can distinguish same-owner and different-owner module edges
- connected modules in coupling sample: 2
- distance rungs implemented: D=2, D=4, D=7, D=9, D=10; omitted/compressed: D=1, D=3, D=5, D=6, D=8
- distance compression: D=3/D=5/D=6/D=8 remain compressed: current graph/config facts distinguish same module, same owner, different owner, deploy unit, and declared vendor seam, but not finer package/library distance without guessing.
- D=1 compressed: object/member-level distance is not available from module dependency edges
- D=3 compressed: current facts distinguish same module vs cross-module, but not object/package micro-distance
- D=5 compressed: package/library middle distance is not split without explicit stable package-boundary metadata
- D=6 compressed: intermediate ownership/library distance has no deterministic signal beyond owner and tree structure
- D=8 compressed: library-like seams remain compressed: undeclared libraries stay excluded, while declared external_systems score at D=10
- tail risk: worst balance 8/10; lower-decile balance 8/10; high-or-worse edges 0/1 (0%); critical 0; distributed-monolith 0

## Coverage

- go/packages: ok (2 files)
- dependency-cruiser: absent
- grimp: absent
- cargo: absent
- loc: ok (2 files)
- deploy-unit: ok
- jscpd: disabled — clone detection disabled by config — set `analyzers.clones.enabled: true` in .archfit.yaml to enable
- scip: disabled — opt-in: analyzers.scip.enabled
- ast-grep/syntax: disabled — opt-in: analyzers.syntax.enabled
- ast-grep: ok
- cargo-modules: absent
