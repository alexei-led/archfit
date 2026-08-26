# archfit — architecture state

- **Verdict:** BLOCKED
- **Blocking:** 1 active — hard gates: fail
- **Attention:** 2 dimension(s) flagged — 1 diagnostic(s)
- **Coverage:** 4 measured / 3 partial / 2 unmeasured (of 9)

## Dimensions

| Dimension | Status | Gate | Confidence | Denominator | Findings |
| --- | --- | --- | --- | --- | ---: |
| intent | measured | pass | high | declared rules evaluated 1/1 | 0 |
| structure | measured | fail | high | discovered edges resolved to a declared module 1/1 | 1 |
| modularity | measured | pass | low | declared modules with a declared public surface 0/2 | 0 |
| coupling | measured | warn | high | cross-boundary edges scored 1/1 | 1 |
| change_locality | unmeasured | not_applicable | unrated | _no denominator_ | 0 |
| complexity | partial | pass | medium | production files in the source walk 2/2 | 0 |
| testability | partial | pass | medium | classified source files 2/2 | 0 |
| operations | partial | pass | medium | analyzers reporting coverage 4/11 | 0 |
| drift | unmeasured | not_applicable | unrated | _no denominator_ | 0 |

## Evidence coverage

| Tool | Status | Reason |
| --- | --- | --- |
| go/packages | ok | — |
| dependency-cruiser | absent | — |
| grimp | absent | — |
| cargo | absent | — |
| loc | ok | — |
| deploy-unit | ok | — |
| jscpd | disabled | clone detection disabled by config — set `analyzers.clones.enabled: true` in .archfit.yaml to enable |
| scip | disabled | opt-in: analyzers.scip.enabled |
| ast-grep/syntax | disabled | opt-in: analyzers.syntax.enabled |
| ast-grep | ok | — |
| cargo-modules | absent | — |

## Coupling seams (1)

| Seam | Strength | Distance | Volatility | Scored | Critical | Median | Quadrant | Try |
| --- | --- | --- | --- | ---: | ---: | ---: | --- | --- |
| a → b | functional | cross_module_different_owner | low | 1 | 0 | 8 | tight | leave_alone |

## Top actionable findings

### Blocking (1)

- **no_direct_b_dependency** [medium] — Import from pkg/a/** to pkg/b/** is explicitly forbidden

### Diagnostic (1)

- **bc/imbalanced_coupling** [low] — balanced coupling: functional integration strength × cross_module_different_owner distance × low volatility → low severity (unbalanced coupling → elevated maintenance effort)

## Comparison

- **Status:** not_requested
- **Reference:** none

## Not measured (8)

- **modularity — public surface** (owner: assessment/metrics): no declared module states a public surface, so every symbol reads as equally public
- **change_locality — change locality** (owner: history/git): git history is unavailable: the tree is not a repository, or no module was declared to attribute commits to
- **complexity — cognitive complexity** (owner: syntax+evidence/acquisition): v1 ships no cognitive-complexity analyzer; only the size tail is measured
- **testability — executed test coverage** (owner: syntax/fileclass): v1 does not run a target repository's test suite; supplied coverage is not yet an input
- **testability — boundary test coverage** (owner: syntax/fileclass): which module boundaries a test actually exercises needs test-to-production import resolution, which v1 does not collect
- **operations — observed runtime topology** (owner: policy+evidence/acquisition): v1 reports declared owners and deploy units only; nothing observes what actually runs
- **operations — supply-chain inventory** (owner: policy+evidence/acquisition): SBOM and vulnerability facts have no collector in v1
- **drift — architecture drift** (owner: assessment/decision): no comparable architecture-state reference: no comparable architecture-state reference is stored
# archfit report

**Verdict:** fail (exit 1)
**Config hash:** `0f7b1dd7cce2ed8eed516d5f81983186aeba1f240f01b913c1d30fbb0814d6e6`

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
