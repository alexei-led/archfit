# archfit — architecture state

- **Verdict:** BLOCKED
- **Blocking:** 1 active — hard gates: fail
- **Attention:** 2 dimension(s) flagged — 1 diagnostic(s)
- **Coverage:** 5 measured / 2 partial / 2 unmeasured (of 9)

## Dimensions

| Dimension | Status | Gate | Confidence | Denominator | Findings |
| --- | --- | --- | --- | --- | ---: |
| intent | measured | pass | high | declared rules evaluated 1/1 | 0 |
| structure | measured | fail | high | discovered dependencies resolved inside the declared module map 1/1 | 1 |
| modularity | measured | pass | low | declared modules with a declared public surface 0/2 | 0 |
| coupling | measured | warn | high | cross-boundary edges scored 1/1 | 1 |
| change_locality | unmeasured | not_applicable | unrated | _no denominator_ | 0 |
| complexity | measured | pass | high | declared modules with complete dependency-chain and degree values 2/2 | 0 |
| testability | partial | pass | medium | classified source files 2/2 | 0 |
| operations | partial | pass | medium | declared modules with a corroborated deploy unit and qualifying owner 0/2 | 0 |
| drift | unmeasured | not_applicable | unrated | _no denominator_ | 0 |

## Dimension metrics

### intent

- `declared_modules`: 2 count
- `declared_layers`: 0 count
- `declared_rules`: 1 count (1/1)
- `declared_waivers`: 0 count
- `waivers_used`: 0 count
- `expired_waivers`: 0 count

### structure

- `internal_edges`: 1 count
- `external_edges`: 0 count
- `same_module_edges`: 0 count
- `connected_modules`: 2 count
- `cycle`: 0 count

### modularity

- `declared_modules`: 2 count
- `public_surface_entries`: 0 count (0/2)
- `local_coupling_modules`: 0 count
- `blast_radius`: 1 count

### coupling

- `scored_edges`: 1 count (1/1)
- `abstained_edges`: 0 count
- `declared_external_edges`: 0 count
- `clone_only_seams`: 0 count
- `critical_band_edges`: 0 count
- `high_or_worse_edges`: 0 count
- `critical_high_distance_edges`: 0 count
- `seams`: 1 count
- `distributed_monolith_seams`: 0 count (1/1)
- `tight_seams`: 1 count
- `unrated_seams`: 0 count
- `unbalanced_edge`: 0 count

### complexity

- `max_dependency_chain`: 1 count (2/2)
- `module_fan_in_p90`: 1 count (2/2)
- `module_fan_out_p90`: 1 count (2/2)
- `production_files`: 2 count
- `production_loc`: 10 count
- `largest_production_file_loc`: 6 count

### testability

- `test_files`: 0 count
- `production_files`: 2 count
- `test_to_production_files`: 0 ratio (0/2)

### operations

- `modules_with_owner`: 2 count (2/2)
- `distinct_owners`: 2 count
- `owners_from_declared`: 2 count
- `owners_from_codeowners`: 0 count
- `owners_from_git_author_fallback`: 0 count
- `declared_deploy_units`: 0 count
- `corroborated_deploy_units`: 0 count
- `modules_with_corroborated_deploy_unit`: 0 count (0/2)
- `matching_declared_deploy_units`: 0 count
- `mismatched_declared_deploy_units`: 0 count
- `declared_external_systems`: 0 count
- `analyzers_reporting_coverage`: 4 count (4/4)
- `coverage_gaps`: 0 count
- `analyzers_not_applicable`: 7 count
- `coverage`: 1 ratio

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

## Not measured (12)

- **modularity — inferred public surface** (owner: assessment/metrics): no declared module states a public surface, so inferring one is outside this claim
- **change_locality — eligible commit sample** (owner: history/git): git history is unavailable or the history scan returned no eligible commit
- **change_locality — commit-to-module attribution** (owner: history/git): the history scan is incomplete, so not every eligible commit has a complete module attribution
- **complexity — function length distribution** (owner: syntax+evidence/acquisition): ast-grep supplied no complete function or method extent for part or all of the out-of-claim size distribution
- **complexity — cognitive complexity** (owner: syntax+evidence/acquisition): no cognitive-complexity analyzer is claimed; module-graph shape is the architecture-level measure
- **testability — executed test coverage** (owner: syntax/fileclass): v1 does not run a target repository's test suite; supplied coverage is not yet an input
- **testability — boundary test coverage** (owner: syntax/fileclass): which module boundaries a test actually exercises needs test-to-production import resolution, which v1 does not collect
- **operations — corroborated deploy unit** (owner: policy+evidence/acquisition): one or more declared modules have no independently corroborating deploy manifest
- **operations — observed runtime topology** (owner: policy+evidence/acquisition): committed manifests corroborate declared deploy units; they do not observe what is actually running
- **operations — supply-chain inventory** (owner: policy+evidence/acquisition): SBOM and vulnerability facts are a separate report family and have no collector in v1
- **drift — admissible persisted reference** (owner: assessment/decision): no comparable architecture-state reference is stored
- **drift — complete two-sided seam identity** (owner: assessment/decision): two-sided seam identity cannot be compared without an admissible persisted reference
# archfit report

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
- **blast_radius**: 1 of 2 modules are change-impact hubs: b (100%, 1 deps) — info (low confidence)

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

## Finding index (2)

| Finding | Status | Rule |
| --- | --- | --- |
| `ba3803eca947bf3c1b7efa8f37854d5e` | new | no_direct_b_dependency |
| `07af466df8bfd6e77ee485ac01c50a17` | new | bc/imbalanced_coupling |
