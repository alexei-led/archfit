# archfit — architecture state

- **Verdict:** BLOCKED
- **Blocking:** 1 active — hard gates: fail
- **Attention:** 2 dimension(s) flagged — 1 diagnostic(s)
- **Coverage:** 5 measured / 2 partial / 2 unmeasured (of 9)

## Blockers (1)

- **`ba3803ec` no_direct_b_dependency** — pkg/a/a.go -> pkg/b
  - location: `pkg/a/a.go:3`
  - why: Import from pkg/a/** to pkg/b/** is explicitly forbidden
  - goal: Remove the forbidden dependency from pkg/a/a.go on pkg/b; move shared behavior to a location permitted by the existing dependency rules.
  - check: `archfit check -c <ROOT>/.archfit.yaml`

## Next steps

1. Fix blocker ba3803ec (no_direct_b_dependency).
2. Supply a test coverage report for the current tree in the coverage: section.
3. Commit a deploy manifest for each deploy_unit.

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
- `library_edges`: 0 count
- `unmapped_first_party_edges`: 0 count
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
- `analyzers_reporting_coverage`: 3 count (3/3)
- `coverage_gaps`: 0 count
- `analyzers_not_applicable`: 8 count
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
| ast-grep | disabled | — |
| cargo-modules | absent | — |

## Coupling seams (1)

| Seam | Strength | Distance | Volatility | Scored | Critical | Median | Quadrant | Try |
| --- | --- | --- | --- | ---: | ---: | ---: | --- | --- |
| a → b | functional | cross_module_different_owner | low | 1 | 0 | 8 | tight | leave_alone |

## Diagnostics (1)

- **bc/imbalanced_coupling** [low] — balanced coupling: functional integration strength × cross_module_different_owner distance × low volatility → low severity (unbalanced coupling → elevated maintenance effort)

## Comparison

- **Status:** not_requested
- **Reference:** none
- **Config hash:** `0f7b1dd7cce2ed8eed516d5f81983186aeba1f240f01b913c1d30fbb0814d6e6`

## Gate reference

- **Status:** non_comparable
- **Reference:** `baseline`
- no baseline file was loaded

## Not measured (14)

- **modularity — inferred public surface** (owner: assessment/metrics): no declared module states a public surface, so inferring one is outside this claim (out of claim — no action)
- **change_locality — eligible commit sample** (owner: history/git): the history scan returned no eligible commit (ok), so co-change cannot be distinguished from a stable tree → run on a full checkout with commit history (in CI, fetch-depth: 0)
- **change_locality — commit-to-module attribution** (owner: history/git): the history scan is incomplete, so not every eligible commit has a complete module attribution → run on a full checkout with commit history (in CI, fetch-depth: 0)
- **complexity — function length distribution** (owner: syntax+evidence/acquisition): ast-grep supplied no complete function or method extent for part or all of the out-of-claim size distribution (out of claim — no action)
- **complexity — cognitive complexity** (owner: syntax+evidence/acquisition): no cognitive-complexity analyzer is claimed; module-graph shape is the architecture-level measure (out of claim — no action)
- **testability — supplied coverage units** (owner: syntax/fileclass): coverage is disabled, so no supplied coverage units were observed → supply a test coverage report for the current tree in the coverage: section
- **testability — coverage path resolution** (owner: syntax/fileclass): coverage is disabled, so no supplied coverage paths were available to resolve → supply a test coverage report for the current tree in the coverage: section
- **testability — coverage module attribution** (owner: syntax/fileclass): coverage is disabled, so no supplied coverage was available to attribute to declared modules → supply a test coverage report for the current tree in the coverage: section
- **testability — coverage freshness** (owner: syntax/fileclass): coverage is disabled, so no supplied coverage freshness could be established → supply a test coverage report for the current tree in the coverage: section
- **operations — corroborated deploy unit** (owner: policy+evidence/acquisition): one or more declared modules have no independently corroborating deploy manifest → commit a deploy manifest for each deploy_unit
- **operations — observed runtime topology** (owner: policy+evidence/acquisition): committed manifests corroborate declared deploy units; they do not observe what is actually running (out of claim — no action)
- **operations — supply-chain inventory** (owner: policy+evidence/acquisition): SBOM and vulnerability facts are a separate report family and have no collector in v1 (out of claim — no action)
- **drift — admissible persisted reference** (owner: assessment/decision): no comparable architecture-state reference is stored → fix the blockers, then record a gate reference: archfit baseline, with the same -c and --root as this run
- **drift — complete two-sided seam identity** (owner: assessment/decision): two-sided seam identity cannot be compared without an admissible persisted reference → fix the blockers, then record a gate reference: archfit baseline, with the same -c and --root as this run

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

## Volatility corroboration (report-only)

Report-only. Source-control touch frequency is supporting evidence for Ch9 volatility judgments and never changes score or gate verdicts.

- source: git_history
- status: ok
- caveat: Supporting evidence only. Git history can reflect both essential and accidental volatility and never changes scoring or gate verdicts.

## Agent tasks (1)

- **no_direct_b_dependency** [`ba3803ec`] Remove the forbidden dependency from pkg/a/a.go on pkg/b; move shared behavior to a location permitted by the existing dependency rules.
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

- `module_boundary`: always on (any module boundary is D=9, level-relative)
- `owner_source`: config
- `deploy_unit_source`: ok
- `owner_model`: multi_owner
- distance basis: ownership=1
- interpretation: every module boundary is D=9 (level-relative); ownership has multiple distinct owners, so a differing owner makes the token cross_module_different_owner, and severity does not change
- connected modules in coupling sample: 2
- distance rungs implemented: D=2, D=9, D=10; omitted/compressed: D=1, D=3, D=4, D=5, D=6, D=7, D=8
- containment boundary crossings: 2→1
- containment shared-ancestor depth: 0→1
- distance compression: Distance is level-relative (Ch10, Ch12, Ch13): inside one codebase the module boundary is the far end. Owner and deploy unit name the boundary and never move the rung.
- D=1 compressed: object/member-level distance is not available from module dependency edges
- D=3 compressed: current facts distinguish same module vs cross-module, but not object/package micro-distance
- D=4 compressed: level-relative distance: a module boundary is the far end of the in-house ladder, so no cross-module edge scores below 9
- D=5 compressed: package/library middle distance is not split without explicit stable package-boundary metadata
- D=6 compressed: owner and deploy unit name the boundary; they do not change the rung
- D=7 compressed: level-relative distance: owner changes relabel a seam, they never lower or raise D
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
- ast-grep: disabled
- cargo-modules: absent

## Finding index (2)

| Finding | Status | Rule |
| --- | --- | --- |
| `ba3803eca947bf3c1b7efa8f37854d5e` | new | no_direct_b_dependency |
| `07af466df8bfd6e77ee485ac01c50a17` | new | bc/imbalanced_coupling |
