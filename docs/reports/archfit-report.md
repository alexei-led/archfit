# archfit — architecture state

- **Verdict:** NEEDS ATTENTION
- **Blocking:** 0 active — hard gates: pass
- **Attention:** 2 dimension(s) flagged — 71 diagnostic(s)
- **Coverage:** 6 measured / 2 partial / 1 unmeasured (of 9)

## Next steps

1. Review why the gate reference does not compare (GATE REFERENCE) before you record a new one.
2. Supply a test coverage report for the current tree in the coverage: section.
3. Commit a deploy manifest for each deploy_unit.

## Dimensions

| Dimension | Status | Gate | Confidence | Denominator | Findings |
| --- | --- | --- | --- | --- | ---: |
| intent | measured | pass | high | declared rules evaluated 41/41 | 0 |
| structure | measured | warn | high | discovered dependencies resolved inside the declared module map 664/1632 | 2 |
| modularity | measured | pass | high | declared modules with a declared public surface 17/18 | 0 |
| coupling | measured | warn | high | cross-boundary edges scored 492/492 | 69 |
| change_locality | measured | pass | high | declared modules touched in the scanned history window 18/18 | 0 |
| complexity | measured | pass | high | declared modules with complete dependency-chain and degree values 18/18 | 0 |
| testability | partial | pass | medium | classified source files 608/614 | 0 |
| operations | partial | pass | medium | declared modules with a corroborated deploy unit and qualifying owner 3/18 | 0 |
| drift | unmeasured | not_applicable | unrated | _no denominator_ | 0 |

## Dimension metrics

### intent

- `declared_modules`: 18 count
- `declared_layers`: 6 count
- `declared_rules`: 41 count (41/41)
- `declared_waivers`: 0 count
- `waivers_used`: 0 count
- `expired_waivers`: 0 count

### structure

- `internal_edges`: 664 count
- `external_edges`: 968 count
- `same_module_edges`: 177 count
- `connected_modules`: 18 count
- `cycle`: 0 count

### modularity

- `declared_modules`: 18 count
- `public_surface_entries`: 58 count (17/18)
- `local_coupling_modules`: 8 count
- `blast_radius`: 10 count

### coupling

- `scored_edges`: 492 count (492/492)
- `abstained_edges`: 0 count
- `declared_external_edges`: 0 count
- `clone_only_seams`: 5 count
- `critical_band_edges`: 99 count
- `high_or_worse_edges`: 110 count
- `critical_high_distance_edges`: 0 count
- `seams`: 72 count
- `distributed_monolith_seams`: 0 count (72/72)
- `tight_seams`: 0 count
- `unrated_seams`: 0 count
- `unbalanced_edge`: 0 count

### change_locality

- `commits_scanned`: 500 count
- `modules_touched`: 18 count

### complexity

- `max_dependency_chain`: 9 count (18/18)
- `module_fan_in_p90`: 9 count (18/18)
- `module_fan_out_p90`: 9 count (18/18)
- `production_files`: 286 count
- `production_loc`: 64118 count
- `largest_production_file_loc`: 1938 count
- `function_loc_p50`: 20 count (2690/2690)
- `function_loc_p90`: 53 count (2690/2690)
- `function_loc_max`: 280 count (2690/2690)
- `functions_over_threshold`: 189 count (2690/2690)

### testability

- `test_files`: 322 count
- `production_files`: 286 count
- `test_to_production_files`: 1.1258741258741258 ratio (322/286)

### operations

- `modules_with_owner`: 18 count (18/18)
- `distinct_owners`: 1 count
- `owners_from_declared`: 18 count
- `owners_from_codeowners`: 0 count
- `owners_from_git_author_fallback`: 0 count
- `declared_deploy_units`: 16 count
- `corroborated_deploy_units`: 3 count
- `modules_with_corroborated_deploy_unit`: 3 count (3/18)
- `matching_declared_deploy_units`: 0 count
- `mismatched_declared_deploy_units`: 2 count
- `declared_external_systems`: 0 count
- `analyzers_reporting_coverage`: 7 count (7/7)
- `coverage_gaps`: 0 count
- `analyzers_not_applicable`: 5 count
- `coverage`: 1 ratio

## Evidence coverage

| Tool | Status | Reason |
| --- | --- | --- |
| scip | ok | — |
| scip-symbols | ok | — |
| go/packages | ok | — |
| dependency-cruiser | absent | — |
| grimp | absent | — |
| cargo | absent | — |
| loc | ok | — |
| deploy-unit | ok | — |
| jscpd | ok | — |
| ast-grep | disabled | — |
| ast-grep/syntax | ok | — |
| cargo-modules | absent | — |

## Coupling seams (72)

| Seam | Strength | Distance | Volatility | Scored | Critical | Median | Quadrant | Try |
| --- | --- | --- | --- | ---: | ---: | ---: | --- | --- |
| assessment-repair → relationship-analysis | symmetric | cross_module_same_owner | high | 21 | 15 | 2 | low_cohesion | reduce_strength |
| assessment-repair → architecture-policy | functional | cross_module_same_owner | high | 24 | 11 | 4 | low_cohesion | reduce_strength |
| analysis-application → assessment-repair | symmetric | cross_module_same_owner | high | 20 | 10 | 2 | low_cohesion | reduce_strength |
| evidence-acquisition → evidence-adapters | symmetric | cross_module_same_owner | high | 24 | 7 | 4 | low_cohesion | reduce_strength |
| relationship-analysis → architecture-policy | functional | cross_module_same_owner | high | 12 | 6 | 2 | low_cohesion | reduce_strength |
| policy-config-adapter → evidence-adapters | functional | cross_module_same_owner | high | 7 | 6 | 2 | low_cohesion | reduce_strength |
| analysis-application → relationship-analysis | functional | cross_module_same_owner | high | 9 | 5 | 2 | low_cohesion | reduce_strength |
| evidence-adapters → persistence-adapters | functional | cross_module_same_owner | high | 13 | 4 | 5 | low_cohesion | reduce_strength |
| analysis-application → architecture-policy | model | cross_module_same_owner | high | 4 | 4 | 2 | low_cohesion | reduce_strength |
| cli-composition → analysis-application | functional | cross_module_same_owner | high | 14 | 3 | 5 | low_cohesion | leave_alone |
| cli-composition → policy-config-adapter | functional | cross_module_same_owner | high | 16 | 3 | 5 | low_cohesion | leave_alone |
| evidence-adapters → relationship-analysis | model | cross_module_same_owner | high | 2 | 2 | 2 | low_cohesion | reduce_strength |
| cli-composition → config-lifecycle | symmetric | cross_module_same_owner | high | 6 | 2 | 6 | low_cohesion | leave_alone |
| evidence-acquisition → architecture-policy | functional | cross_module_same_owner | high | 5 | 2 | 5 | low_cohesion | reduce_strength |
| cli-composition → persistence-adapters | functional | cross_module_same_owner | high | 4 | 2 | 2 | low_cohesion | leave_alone |
| development-tools → relationship-analysis | functional | cross_module_same_owner | high | 4 | 2 | 2 | low_cohesion | leave_alone |
| evidence-acquisition → analysis-application | model | cross_module_same_owner | high | 3 | 2 | 2 | low_cohesion | reduce_strength |
| cli-composition → evidence-adapters | functional | cross_module_same_owner | high | 11 | 1 | 5 | low_cohesion | leave_alone |
| evidence-acquisition → assessment-repair | functional | cross_module_same_owner | high | 2 | 1 | 2 | low_cohesion | reduce_strength |
| architecture-tests → assessment-repair | model | cross_module_same_owner | high | 1 | 1 | 2 | low_cohesion | reduce_strength |

_… +52 more seams (see `--format json`)_

## Diagnostics (71)

- **module_cycles** [high] — Module evidence-adapters depends on persistence-adapters, and the two are in a dependency cycle among 2 declared modules
- **module_cycles** [high] — Module persistence-adapters depends on evidence-adapters, and the two are in a dependency cycle among 2 declared modules
- **bc/imbalanced_coupling** [critical] — balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (contained, not a distributed monolith))
- **bc/imbalanced_coupling** [critical] — balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (contained, not a distributed monolith))
- **bc/imbalanced_coupling** [medium] — balanced coupling: symmetric integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **bc/imbalanced_coupling** [medium] — balanced coupling: contract integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **bc/imbalanced_coupling** [medium] — balanced coupling: model integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **bc/imbalanced_coupling** [medium] — balanced coupling: functional integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **bc/imbalanced_coupling** [critical] — balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (contained, not a distributed monolith))
- **bc/imbalanced_coupling** [medium] — balanced coupling: model integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **bc/imbalanced_coupling** [medium] — balanced coupling: functional integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **bc/imbalanced_coupling** [critical] — balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (contained, not a distributed monolith))
- **bc/imbalanced_coupling** [medium] — balanced coupling: contract integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **bc/imbalanced_coupling** [medium] — balanced coupling: model integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **bc/imbalanced_coupling** [medium] — balanced coupling: symmetric integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **bc/imbalanced_coupling** [critical] — balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (contained, not a distributed monolith))
- **bc/imbalanced_coupling** [medium] — balanced coupling: symmetric integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **bc/imbalanced_coupling** [medium] — balanced coupling: functional integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **bc/imbalanced_coupling** [critical] — balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (contained, not a distributed monolith))
- **bc/imbalanced_coupling** [medium] — balanced coupling: functional integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)

_… +51 more (see `--format json`)_

## Comparison

- **Status:** not_requested
- **Reference:** none
- **Config hash:** `32ff53dc91afd98188c5a7b349eaa1a9bf1f22e207f452b173b75fbeb0ab2a0b`

## Gate reference

- **Status:** non_comparable
- **Reference:** `baseline`
- config_hash differs between the two runs (32ff53dc91af vs c02a9e2a2f43): a policy change is not a code change
- model_hash differs between the two runs (9ce12c865be1 vs 9afa91be2d55): a policy change is not a code change
- measurement_profile is missing from reference

## Not measured (12)

- **structure — external dependency structure** (owner: relationship/facts): the target of 968 dependencies is outside the declared module map, so its direction and layer are outside this claim (out of claim — no action)
- **change_locality — essential vs accidental volatility** (owner: history/git): commit frequency corroborates a declared volatility; it cannot establish one (out of claim — no action)
- **complexity — cognitive complexity** (owner: syntax+evidence/acquisition): no cognitive-complexity analyzer is claimed; module-graph shape is the architecture-level measure (out of claim — no action)
- **testability — supplied coverage units** (owner: syntax/fileclass): coverage is disabled, so no supplied coverage units were observed → supply a test coverage report for the current tree in the coverage: section
- **testability — coverage path resolution** (owner: syntax/fileclass): coverage is disabled, so no supplied coverage paths were available to resolve → supply a test coverage report for the current tree in the coverage: section
- **testability — coverage module attribution** (owner: syntax/fileclass): coverage is disabled, so no supplied coverage was available to attribute to declared modules → supply a test coverage report for the current tree in the coverage: section
- **testability — coverage freshness** (owner: syntax/fileclass): coverage is disabled, so no supplied coverage freshness could be established → supply a test coverage report for the current tree in the coverage: section
- **operations — corroborated deploy unit** (owner: policy+evidence/acquisition): one or more declared modules have no independently corroborating deploy manifest → commit a deploy manifest for each deploy_unit
- **operations — observed runtime topology** (owner: policy+evidence/acquisition): committed manifests corroborate declared deploy units; they do not observe what is actually running (out of claim — no action)
- **operations — supply-chain inventory** (owner: policy+evidence/acquisition): SBOM and vulnerability facts are a separate report family and have no collector in v1 (out of claim — no action)
- **drift — admissible persisted reference** (owner: assessment/decision): the stored baseline was written under different inputs → review why the gate reference does not compare (GATE REFERENCE) before you record a new one
- **drift — complete two-sided seam identity** (owner: assessment/decision): two-sided seam identity cannot be compared without an admissible persisted reference → review why the gate reference does not compare (GATE REFERENCE) before you record a new one

## Metrics

- **encapsulation**: n/a — n/a (low confidence)
- **unbalanced_edge**: 0 new high-risk unbalanced edges — strong

## Structural facts (neutral evidence)

126 modules; top 5 per axis (full list in `--format json`):

- inbound module fan-in: internal/model/evidence (51), internal/model/graph (32), internal/policy (32), internal/relationship (32), internal/toolrun (30)
- outbound destinations: cmd/archfit (42), internal/evidence/acquisition (27), internal/assessment/evaluation (20), internal_test (20), internal/application (19)
- LOC: cmd/archfit (7009), internal/initcfg (5514), internal/assessment/evaluation (5207), internal/application (3169), internal/relationship/analysis (2086)

## Syntax surface (neutral evidence)

3639 declaration(s) extracted by ast-grep (full list in `--format json`):

- annotation: 1
- enum: 1
- function: 2226
- interface: 45
- method: 464
- struct: 520
- type_alias: 67
- type_leak: 315
- exported (public API): 3323

Per module:

- (unscoped): 64
- analysis-application: 268
- analysis-scope: 32
- architecture-policy: 78
- architecture-tests: 61
- assessment-repair: 642
- cli-composition: 435
- config-lifecycle: 253
- development-tools: 41
- evidence-acquisition: 127
- evidence-adapters: 632
- evidence-analysis: 13
- evidence-contracts: 100
- persistence-adapters: 142
- policy-config-adapter: 160
- provider-adapters: 30
- relationship-analysis: 284
- report-adapters: 170
- report-contract: 107

### Public API


`cmd/archfit/agent_format_test.go` [cli-composition]:
- `TestFormatMatrix_AgentDigestCarriesTheState` (function)
- `TestFormatMatrix_AgentDigestNamesTheRatchet` (function)

`cmd/archfit/agentsmd.go` [cli-composition]:
- `AgentsMDCmd` (struct)
- `Help` (method)
- `Run` (method)

`cmd/archfit/agentsmd_test.go` [cli-composition]:
- `TestAgentsMDGoldens` (function)
- `TestAgentsMDWriteAndCheck` (function)
- `TestAgentsMDAppendKeepsEveryByte` (function)
- `TestAgentsMDNamesANonDefaultConfig` (function)
- `TestAgentsMDUsageErrors` (function)
- `TestAgentsMDRepositoryBlockIsCurrent` (function)

`cmd/archfit/allowlist_test.go` [cli-composition]:
- `TestRun_Check_ModuleAllowlistBlocksANewPairOnly` (function)
- `TestRun_ConfigLint_ReportsUnknownAllowlistModules` (function)

`cmd/archfit/analysis_characterization_test.go` [cli-composition]:
- `TestAnalyzeCheckCharacterization` (function)

`cmd/archfit/analyze.go` [cli-composition]:
- `AnalyzeCmd` (struct)
- `Help` (method)
- `Run` (method)

`cmd/archfit/analyze_exit_test.go` [cli-composition]:
- `TestOutcomeExitCodeOwnsCLIOutcomeTranslation` (function)
- `TestRunScanRejectsFormatConflictBeforeConfigLoad` (function)
- `TestRunScanWiresRefreshAndProgressBeforePreparation` (function)
- ... +3303 more exported declarations (use `--format json`)

## Connascence evidence (deterministic)

Report-only. Static facts only; semantic and dynamic categories without deterministic evidence stay unmeasured.

- edges with evidence: 1627
- abstained edges: 5
- total evidence facts: 6171
- strength inferred from connascence: 71 edges
- by kind: algorithm=1394, meaning=851, name=2291, type=1635
- by source: go/types=4266, scip=1905
- unmeasured: position, execution, timing, value, identity
- roadmap: name=deterministic_static, type=deterministic_static, meaning=deterministic_static, algorithm=deterministic_static, position=unmeasured_static, execution=unmeasured_dynamic (signals dynamic_imports/runtime_async_edges), timing=unmeasured_dynamic (signals dynamic_imports/runtime_async_edges), value=unmeasured_dynamic (signals dynamic_imports/runtime_async_edges), identity=unmeasured_dynamic (signals dynamic_imports/runtime_async_edges)

## Dynamic connascence signals (report-only)

Report-only. Static dynamic-import and runtime-async sites can guide Ch6 execution/timing review, but they are not runtime measurements and never change score or verdict.

- signals: 8 across 2 module/source group(s)
- still unmeasured: execution, timing, value, identity
- reason: static site evidence only; deterministic runtime ordering/value/identity trace evidence is absent
- **evidence-adapters** [dynamic_import; related: execution/timing; measured=false]: 6 (e.g. internal/extract/py/grimp_helper.py:179[lazy_import], internal/extract/scip/scip_reader.py:63[lazy_import], internal/extract/ts/config_snapshot.cjs:1[require])
- **development-tools** [dynamic_import; related: execution/timing; measured=false]: 2 (e.g. scripts/eval/corpus_sweep.py:1156[lazy_import], scripts/eval/language_contracts.py:232[lazy_import])

## Dynamic / lazy imports (hidden-coupling risk)

Report-only. Dynamic/lazy imports are invisible to the static dependency
graph, so they can hide cycles and undercount coupling.

8 sites across 2 modules (full list in `--format json`):

- **evidence-adapters**: 6 (e.g. internal/extract/py/grimp_helper.py:179[lazy_import], internal/extract/scip/scip_reader.py:63[lazy_import], internal/extract/ts/config_snapshot.cjs:1[require])
- **development-tools**: 2 (e.g. scripts/eval/corpus_sweep.py:1156[lazy_import], scripts/eval/language_contracts.py:232[lazy_import])

## Distance config candidates (review-only)

Report-only. Static external, runtime, and dynamic evidence can suggest `external_systems` or `deploy_unit` review, but these candidates never change distance, score, or gate verdicts.

60 signal(s) across 25 candidate(s):

- **assessment-repair** → `github.com/bmatcuk/doublestar/**` [imports from classified_external_edges; action=external_systems]: 8 (e.g. internal/assessment/evaluation/dimensions.go:9[imports], internal/assessment/evaluation/lint.go:9[imports], internal/assessment/evaluation/selectors.go:6[imports])
- **evidence-adapters** → `evidence-adapters` [dynamic_import from dynamic_connascence_signals; action=deploy_unit]: 6 (e.g. internal/extract/py/grimp_helper.py:179[lazy_import], internal/extract/scip/scip_reader.py:63[lazy_import], internal/extract/ts/config_snapshot.cjs:1[require])
- **evidence-adapters** → `evidence-adapters` [require from dynamic_imports; action=deploy_unit]: 6 (e.g. internal/extract/py/grimp_helper.py:179[lazy_import], internal/extract/scip/scip_reader.py:63[lazy_import], internal/extract/ts/config_snapshot.cjs:1[require])
- **evidence-adapters** → `github.com/bmatcuk/doublestar/**` [imports from classified_external_edges; action=external_systems]: 5 (e.g. internal/extract/golang/members.go:11[imports], internal/extract/golang/query.go:14[imports], internal/extract/py/locations.go:9[imports])
- **config-lifecycle** → `github.com/goccy/go-yaml/**` [imports from classified_external_edges; action=external_systems]: 4 (e.g. internal/initcfg/subdomains_draft.go:9[imports], internal/initcfg/value_draft.go:9[imports], internal/initcfg/yamledit_parse.go:11[imports])
- **evidence-adapters** → `golang.org/x/mod/**` [imports from classified_external_edges; action=external_systems]: 4 (e.g. internal/extract/coverage/normalize.go:13[imports], internal/extract/golang/cache.go:20[imports], internal/extract/golang/members.go:12[imports])
- **provider-adapters** → `github.com/openai/openai-go/**` [imports from classified_external_edges; action=external_systems]: 3 (e.g. internal/llm/openai.go:8[imports], internal/llm/openai.go:9[imports], internal/llm/openai.go:10[imports])
- **cli-composition** → `github.com/bmatcuk/doublestar/**` [imports from classified_external_edges; action=external_systems]: 2 (e.g. cmd/archfit/draft_metadata.go:9[imports], cmd/archfit/policy.go:18[imports])
- **policy-config-adapter** → `github.com/goccy/go-yaml/**` [imports from classified_external_edges; action=external_systems]: 2 (e.g. internal/config/config.go:18[imports], internal/config/types.go:10[imports])
- **provider-adapters** → `github.com/anthropics/anthropic-sdk-go/**` [imports from classified_external_edges; action=external_systems]: 2 (e.g. internal/llm/anthropic.go:9[imports], internal/llm/anthropic.go:10[imports])
- ... +15 more candidates (use `--format json`)

## Volatility corroboration (report-only)

Report-only. Source-control touch frequency is supporting evidence for Ch9 volatility judgments and never changes score or gate verdicts.

- source: git_history
- status: ok
- recent-history window: 500 commits
- commits scanned: 500
- modules touched: 18
- caveat: Supporting evidence only. Git history can reflect both essential and accidental volatility and never changes scoring or gate verdicts.

Top touched modules:

- **cli-composition**: 182 commit(s) [declared volatility=medium]
- **evidence-adapters**: 114 commit(s) [declared volatility=medium]
- **config-lifecycle**: 87 commit(s) [declared volatility=medium]
- **policy-config-adapter**: 75 commit(s) [declared volatility=high]
- **assessment-repair**: 60 commit(s) [declared volatility=high]

## Advisory tasks (51)

Report-only rollups from grouped advisories; these do not affect verdict or gate status.
- **bc/imbalanced_coupling** [`2ff049a7`] Review 3 same-shape Balanced-Coupling advisory edges from analysis-application to architecture-policy and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 3
  - group members: 2ff049a792718c864d7222e5a4ceaf2c, 8b59222a42fcc083b2ef32492cb0bc73, e02e63908f68ae279bf02f1541b270df
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: internal/application/analysis.go, internal/application/config_lint.go, internal/application/policy_query.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`2e1590e3`] Review 9 same-shape Balanced-Coupling advisory edges from analysis-application to assessment-repair and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 9
  - group members: 2e1590e3580d6096c9ef526ef72d1595, 499d6f0c19e84b00db0bf9e6d9fc0a0d, 5f32df6c7a80d4502aa9ef635567d328, 6b7786f551449325fafa7e87281e25f9, 781faa92ba09bc38dab9f7474e345930, bac595ad96d976c901f09e50962ac0e7, c7b702dff75403dcc58a2a93a04b0e94, cf8373bf8903ca9840144950b2324e80
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: internal/application/analysis.go, internal/application/base_compare.go, internal/application/baseline.go, internal/application/relationship_report.go, internal/application/report.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`25583a96`] Review 8 same-shape Balanced-Coupling advisory edges from analysis-application to assessment-repair and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 8
  - group members: 25583a9622c5bc1e0fee18f579fa6a12, 2af8fb58ddd7b4ac6e437c8a8b301005, 2b2f68b0757a3eaeb5c9244588a8ff16, 46d1b53cf0d5c82479706ca7e3f74e52, 50a698c4a94070e7e276fa6164316d8e, 9a8e5d61d663876efe2854b608cfb29f, bf9c5ee4242875ff9d121eea63a29737, c63bae9ac43a663e4677d59c87aa9d83
  - score: 6/10
  - top files: internal/application/analysis.go, internal/application/base_compare.go, internal/application/baseline.go, internal/application/compare.go, internal/application/config_lint.go, internal/application/policy_query.go, internal/application/report.go, internal/assessment/evaluation/relationship_projection.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=symmetric, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`2cc4ff1f`] Review 2 same-shape Balanced-Coupling advisory edges from analysis-application to evidence-contracts and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 2
  - group members: 2cc4ff1fdf50fcbe4a39b2f9afcb7493, 81716c06d3e6351857b461e684667a87
  - score: 5/10
  - top files: internal/application/analysis.go, internal/application/baseline.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=contract, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`52bfd50b`] Review 4 same-shape Balanced-Coupling advisory edges from analysis-application to evidence-contracts and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 4
  - group members: 52bfd50b6fa2f79a86c95672af211c64, 6b79ec12f5ed02cb8aa3cf8b25c46a0d, 80b17afe77a58a5122ae47a27213b396, 8ebe12ee50b033fc6bbb0e2eb7884e20
  - score: 5/10
  - top files: internal/application/analysis.go, internal/application/policy_query.go, internal/application/relationship_report.go, internal/application/report_text.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`3ef83540`] Review 2 same-shape Balanced-Coupling advisory edges from analysis-application to relationship-analysis and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 2
  - group members: 3ef835400a5770fb0e5874bf35280310, ec535315f78fd6a77d308ed7d9ccfc00
  - score: 5/10
  - top files: internal/application/analysis.go, internal/application/policy_query.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`55d937f9`] Review 4 same-shape Balanced-Coupling advisory edges from analysis-application to relationship-analysis and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 4
  - group members: 55d937f911ce6b7a354a73aef82c01cc, 9c396f88e33fd4632815029b08249e14, af440bc871e12427303d9930385db551, ef3e28e610acf5df3aececcb8b96764c
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: internal/application/analysis.go, internal/application/capture.go, internal/application/enrich.go, internal/application/relationship_report.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`0d17f93d`] Review 4 same-shape Balanced-Coupling advisory edges from analysis-application to report-contract and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 4
  - group members: 0d17f93d13ffef3ee5e3a6f2b69f48b8, 856b984c6f6570371ec806f50bdeb9eb, e945f398ba0acaaefad734f7ea16b263, f4652ed80ac1f67fcd4fd6719e1e437b
  - score: 5/10
  - top files: internal/application/analysis.go, internal/application/base_compare.go, internal/application/baseline.go, internal/application/report_text.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`43daa0c9`] Review 11 same-shape Balanced-Coupling advisory edges from assessment-repair to architecture-policy and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 11
  - group members: 43daa0c91bacd482d5ac9ba560dd211e, 455acb12fb6bc8dc6bf8a8416d1a7661, 4c89874a384dea9db132e2e31d24d0af, 5a5ae9f159fb8fd7d2bfd93d10bc9c2a, 71ec017e3902c4bfad1bfde3db7de350, 744edeaae5b7fd628ebdfc37ebed4a63, 7da7c9997f570a6d4cfd4fdf1fca0393, b6c441e6a53d2965735ca16328b0848f
  - score: 5/10
  - top files: internal/assessment/evaluation/advisories.go, internal/assessment/evaluation/assess.go, internal/assessment/evaluation/dimensions.go, internal/assessment/evaluation/health_warnings.go, internal/assessment/evaluation/judge.go, internal/assessment/evaluation/lint.go, internal/assessment/evaluation/selectors.go, internal/assessment/evaluation/state.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`4928bbc7`] Review 10 same-shape Balanced-Coupling advisory edges from assessment-repair to architecture-policy and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 10
  - group members: 4928bbc7cc6af3ad3d3e884be4d1e3f6, 5655b0a3b20b149603ec28ba8e5a89df, 662cb3ccc21d240995e0102d889efe96, 844f053bccc64638e1355c0ccde2c74f, b9750e7ae76edb08e68b50dd7e5418fe, ca9d038dca3f61e305a1b884a9135624, d660aa240ae2f1d6a8fa1f10dba37172, ded385d022d1f711950a2fa8ed1d67af
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: internal/assessment/evaluation/advisories.go, internal/assessment/evaluation/complexity.go, internal/assessment/evaluation/finalize.go, internal/assessment/evaluation/inventory.go, internal/assessment/finding/finding.go, internal/assessment/metrics/metrics.go, internal/assessment/rules/rules.go, internal/assessment/staleness/staleness.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`261cf14b`] Review 12 same-shape Balanced-Coupling advisory edges from assessment-repair to evidence-contracts and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 12
  - group members: 261cf14b927a372b977880be97fae663, 32aa0a8d9b525561b895230003bfcd8f, 52ab4e7c9c9797741ec665fe3ca63d65, 5529af17393932bd5258cce67b79f4ee, 84fd83626c7ccd40cb31cf5a11186635, 94ef434560797a80798b50a195ff1c9d, a54099ba4cad069f2b876d062dd363c6, a551977891de7500f28bd7a62a9c103e
  - score: 5/10
  - top files: internal/assessment/decision/state_comparison.go, internal/assessment/evaluation/assess.go, internal/assessment/evaluation/evaluation.go, internal/assessment/evaluation/projector.go, internal/assessment/evaluation/ruleset.go, internal/assessment/evaluation/task_origin.go, internal/assessment/rules/rules.go, internal/assessment/signals/signal.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=contract, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`320ae359`] Review 17 same-shape Balanced-Coupling advisory edges from assessment-repair to evidence-contracts and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 17
  - group members: 320ae359fba984604d56d4a447acfbfe, 53f990a03a781a06df644737e16f4ad8, 648d97e461b0a5af85490566837fcf26, 701989929e62f5b5b2295de4ce2c86b8, 7a646ee19e64da05e1e8601767e44263, 7c7671290ef3fb5d01e3ca6f3721edee, 846cc62a7b022f3d5545ff16d1b7b532, 8ea9a6926178f36d81560ff9aa942ed3
  - score: 5/10
  - top files: internal/assessment/agenttask/agenttask.go, internal/assessment/decision/task_origin.go, internal/assessment/evaluation/assess.go, internal/assessment/evaluation/complexity.go, internal/assessment/evaluation/dimensions.go, internal/assessment/evaluation/health_warnings.go, internal/assessment/evaluation/inventory.go, internal/assessment/evaluation/projector.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`506dbac7`] Review 4 same-shape Balanced-Coupling advisory edges from assessment-repair to evidence-contracts and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 4
  - group members: 506dbac7f4eb8369b27d1152ffbf1ff1, 844c0acaefc8d10cde786d094f08cca7, ede7695939e3dba08c8a0a5b67897441, f141928daa6831cf607661079e5e3e00
  - score: 6/10
  - top files: internal/assessment/agenttask/agenttask.go, internal/assessment/decision/config_compare.go, internal/assessment/decision/measurement.go, internal/assessment/rules/rules_dependency.go, internal/assessment/rules/rules_pattern.go, internal/model/graph/convention.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=symmetric, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`1c75d7df`] Review 14 same-shape Balanced-Coupling advisory edges from assessment-repair to relationship-analysis and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 14
  - group members: 1c75d7dfef7a1ac9304d545fe3930bdf, 1efc51733064480ebd1524237566a40b, 27c5b67e5fb7a67865997ff2110a6b17, 28513b27f490f50de7fc955712d8423b, 3d8129ecba9ecee3a27d37e249e2c0db, 4f50da9ecdce6a99125244d19c5a6501, 55d8777f8fffee3f32734e32c9cbc495, 56ffebc60d204753e8d10c0b385735e7
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: internal/assessment/evaluation/advisories.go, internal/assessment/evaluation/assess.go, internal/assessment/evaluation/dimensions.go, internal/assessment/evaluation/evaluation.go, internal/assessment/evaluation/judge.go, internal/assessment/evaluation/relationship_projection.go, internal/assessment/evaluation/uncovered.go, internal/assessment/metrics/internal/modgraph/modgraph.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`20ae16d0`] Review 5 same-shape Balanced-Coupling advisory edges from assessment-repair to relationship-analysis and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 5
  - group members: 20ae16d0505f044a52761ddc937b04dc, 6b7bba3f58a30123ec30a10181b79d71, 76c8702dc476268037aca1b726f06a57, 813705841e9588cd040cac52a82a4c07, b37e406b9bf118840a5c56225daff7f2
  - score: 6/10
  - top files: internal/assessment/evaluation/advisories.go, internal/assessment/evaluation/complexity.go, internal/assessment/finding/finding.go, internal/assessment/metrics/boundary/cycle.go, internal/assessment/metrics/boundary/encapsulation.go, internal/assessment/metrics/boundary/unbalanced_edge.go, internal/relationship/analysis/analysis.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=symmetric, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`18e7dfc0`] Review 10 same-shape Balanced-Coupling advisory edges from cli-composition to analysis-application and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 10
  - group members: 18e7dfc05a495bff4ffa44a0cf712f5d, 1b2c300670d5634c581336201844d0d9, 29c1b0a7f479b468d7cd39d93f39b1ef, 6ac1d11778423abca8a0bce2def3243f, 8e165ca94f6eaea79c2913e475dad8a9, afddbea84ff014f35f0adcfe6077d348, b549bcb092615763dd52d93db2c1574a, c1bfcf1cca66a5ddb9400293a3ba2144
  - score: 5/10
  - top files: cmd/archfit/analyze.go, cmd/archfit/application_wiring.go, cmd/archfit/config_compare.go, cmd/archfit/config_enrich_adapters.go, cmd/archfit/config_lint.go, cmd/archfit/enrich.go, cmd/archfit/enrich_abstained.go, cmd/archfit/explain.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`6b88563f`] Review 2 same-shape Balanced-Coupling advisory edges from cli-composition to analysis-application and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 2
  - group members: 6b88563fa58136a54d164ffde0b0676f, dd9b00ab182816ef88ec5dbaf7a3bff8
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: cmd/archfit/config_update_adapters.go, cmd/archfit/enrichment_judges.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`6e46b555`] Review 2 same-shape Balanced-Coupling advisory edges from cli-composition to architecture-policy and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 2
  - group members: 6e46b555e604bee94146d5c59e9bf5f5, ed27ebfdbae6cda0e5ccc2e7bcb611d8
  - score: 5/10
  - top files: cmd/archfit/agentsmd.go, cmd/archfit/policy.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`18ebbe3e`] Review 3 same-shape Balanced-Coupling advisory edges from cli-composition to config-lifecycle and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 3
  - group members: 18ebbe3ec25eef143658721dfd510987, c8838259ed1db91f80347f0fb74b80b8, d0de32cf75db6cf7d8d67e7a9705556b
  - score: 6/10
  - top files: cmd/archfit/config_enrich_adapters.go, cmd/archfit/config_update_adapters.go, cmd/archfit/draft_metadata.go, cmd/archfit/evidence_pack.go, cmd/archfit/init.go, cmd/archfit/runecut.go, internal/initcfg/evidence_pack.go, internal/initcfg/initcfg.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=symmetric, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`2966e080`] Review 8 same-shape Balanced-Coupling advisory edges from cli-composition to evidence-adapters and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 8
  - group members: 2966e0804c1e5f7f6831422c2f90ea41, 2ad62b3aa083c71f1b451c00f8e72cfa, 51cd4c6f13b8779fe5be0150f9797bc9, 52a9743259f68337a35f3e18cb3cd4b4, 5b49cc4aa21e26b775fbe8a434a242b2, b171d8e322cc6a251ff9cb939ac5ec62, bc267beb799f4340ececc35eb1d3dfe9, ef84cbb62eaeeba96fd23246ce878465
  - score: 5/10
  - top files: cmd/archfit/config_update_adapters.go, cmd/archfit/doctor.go, cmd/archfit/hook.go, cmd/archfit/init.go, cmd/archfit/main.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`2ab6dd6c`] Review 12 same-shape Balanced-Coupling advisory edges from cli-composition to policy-config-adapter and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 12
  - group members: 2ab6dd6c6c98eb5ad56e21a522d3d938, 4fb003f14c3faea28b169e0c85e9a259, 58bc8aba5d9ecb08efc4e074f63ee8c5, 6804cad616031e6bad6a4a9a8ddb1762, a993f8ba00e7797f62983e493ae1c1db, abbfe5778c068b4859b4fa62894e8190, b6278e00ffce641cb5c9df46276d9dac, c21247591935909647198b333e200b51
  - score: 5/10
  - top files: cmd/archfit/analyze.go, cmd/archfit/application_wiring.go, cmd/archfit/config_compare.go, cmd/archfit/config_enrich_adapters.go, cmd/archfit/config_update_adapters.go, cmd/archfit/doctor.go, cmd/archfit/enrich.go, cmd/archfit/enrich_abstained.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`7300bc30`] Review 2 same-shape Balanced-Coupling advisory edges from cli-composition to policy-config-adapter and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 2
  - group members: 7300bc30e7a2eb6066153e29571b8221, e549a50c51ae95c56561a6604ccbaa2d
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: cmd/archfit/enrichment_judges.go, cmd/archfit/rust_config_update.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`13fdf9f1`] Review 3 same-shape Balanced-Coupling advisory edges from cli-composition to provider-adapters and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 3
  - group members: 13fdf9f16fbc4f8c4bbfdfb7f2c8364b, 1a6bb1ce644dbee9eb45434c759dbadc, dc7bbf57dbec19c38717b0bbf75de0f9
  - score: 5/10
  - top files: cmd/archfit/config_update_adapters.go, cmd/archfit/enrichment_judges.go, cmd/archfit/update.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=contract, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`7a80f7a3`] Review 5 same-shape Balanced-Coupling advisory edges from cli-composition to provider-adapters and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 5
  - group members: 7a80f7a3cb06def6fd4e8921baafd18c, 87730e067efe316da849db5512a9350f, 8a83e18dca7af9c5fb368854cfef31b7, 9e723b910eda14bf00f742e79e6e1033, af2e5b717e51a4cfdd2d805330373cf5
  - score: 5/10
  - top files: cmd/archfit/config_enrich_adapters.go, cmd/archfit/enrich.go, cmd/archfit/enrich_abstained.go, cmd/archfit/explain.go, cmd/archfit/init.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`3d274fb8`] Review 6 same-shape Balanced-Coupling advisory edges from cli-composition to report-adapters and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 6
  - group members: 3d274fb8a8479e90ef68543938150453, 4117b9928678a5f01ea3ef4ec4db40ef, 83f06ac754fc79418d0f127b74b143cb, 978820d1593945c318aa9d9220a8b2ca, a836f879beae5a41c3b8780126cc3015, b317bcad5f5ca1ef233ec9324fd61299
  - score: 5/10
  - top files: cmd/archfit/analyze.go, cmd/archfit/hook.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`

_…and 26 more advisory tasks (see --json for the full list)._

## Balanced Coupling advisories (171 rollups, 452 edges)

Same-shape edges between a module pair are grouped into one rollup.
Integration strength × distance × volatility lint messages.
Severity: `none` · `low` · `medium` · `high` · `critical`.

```
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/archfit/application_wiring.go -> internal/labels/labelsio  [07498823]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/archfit/consts.go -> internal/extract/registry  [e19785bd]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/archfit/draft_metadata.go -> internal/initcfg  [09ea5d95]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/archfit/enrich_values.go -> internal/application  [12585c74]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/archfit/evidence_pack.go -> internal/config  [029b9c05]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/application/analysis.go -> internal/assessment/score  [088c6257]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/application/enrichment_selection.go -> internal/relationship  [40ee0174]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/application/relationship_report.go -> internal/policy  [69d3b879]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/assessment/evaluation/advisory_severity.go -> internal/relationship  [14ac826f]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/assessment/evaluation/evaluation.go -> internal/policy  [39280b30]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/baseline/baseline.go -> internal/assessment/status  [e0052281]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/calibrate/calibrate.go -> internal/relationship/coupling  [0ad7fdb6]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/config/projection.go -> internal/application  [35f79297]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/config/tools.go -> internal/policy  [881a242a]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/config/views.go -> internal/evidence/ports  [08244a90]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/config/views.go -> internal/relationship/classify  [dbf15249]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/configschema/schema.go -> internal/config  [981183bc]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/evidence/acquisition/options.go -> internal/extract/registry  [427ea639]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/evidence/acquisition/options.go -> internal/policy  [016e681e]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
  rollup: 2 same-shape edges (e.g. 016e681e350a7b9f64711847a3c55036,1b1b651ef6278011aa2b28e88cfe43fd)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/evidence/acquisition/service.go -> internal/application  [e83b01e8]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/evidence/acquisition/warnings.go -> internal/ownership  [ed4840b0]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/evidence/acquisition/warnings.go -> internal/relationship/labels  [222d43e9]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/extract/acquire/acquire.go -> internal/factcache  [1188317f]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/extract/acquire/acquire.go -> internal/policy  [a04e297f]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/extract/py/py.go -> internal/relationship/coupling  [3ccff7c7]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (co...
  cheapest move: reduce_strength
```

- ... +146 more rollups (use `--format json`)

## Advisories (9)

- **module_cycles** [high] new: Module persistence-adapters depends on evidence-adapters, and the two are in a dependency cycle among 2 declared modules
- **module_cycles** [high] new: Module evidence-adapters depends on persistence-adapters, and the two are in a dependency cycle among 2 declared modules
- **bc/duplicated_knowledge** [medium] baseline — internal/application/report.go → internal/testutil/report/convert.go: duplicated knowledge: cross-module code clones between analysis-application and architecture-tests with no import edge — symmetric func...
- **bc/duplicated_knowledge** [medium] baseline — internal/assessment/result/result.go → internal/model/report/document.go: duplicated knowledge: cross-module code clones between assessment-repair and report-contract with no import edge — symmetric functional...
- **bc/duplicated_knowledge** [medium] baseline — internal/initcfg/evidence_pack.go → internal/output/console/report.go: duplicated knowledge: cross-module code clones between config-lifecycle and report-adapters with no import edge — symmetric functional ...
- **bc/duplicated_knowledge** [medium] baseline — internal/labels/labelsio/labelsio.go → internal/llm/cache.go: duplicated knowledge: cross-module code clones between persistence-adapters and provider-adapters with no import edge — symmetric funct...
- **bc/duplicated_knowledge** [medium] baseline — internal/model/evidence/evidence.go → internal/model/report/evidence.go: duplicated knowledge: cross-module code clones between evidence-contracts and report-contract with no import edge — symmetric functiona...
- **syntax_api_size_ceiling** [medium] baseline — assessment-repair → assessment-repair: Module "assessment-repair" has 586 exported declarations, exceeding the limit of 430
- **syntax_api_size_ceiling** [medium] baseline — evidence-adapters → evidence-adapters: Module "evidence-adapters" has 555 exported declarations, exceeding the limit of 430

## Supporting structural metrics (beyond Balanced Coupling)

Report-only. These metrics support Balanced Coupling reasoning but never gate.

- **cycle**: 0 import cycles — strong
- **coverage**: 100% coverage — strong
- **blast_radius**: 10 of 18 modules are change-impact hubs: evidence-contracts (76%, 13 deps), architecture-policy (65%, 11 deps), evidence-analysis (65%, 11 deps), analysis-scope (59%, 10 deps), relationship-analysis (59%, 10 deps)+5 more — info

## Distance confidence

- `code_structure`: always on (deterministic tree-distance baseline)
- `owner_source`: config
- `deploy_unit_source`: ok
- `owner_model`: single_owner_degenerate
- deploy-unit detector mapped modules: 3
- distance basis: code_structure=483, deploy_unit=9
- interpretation: same-owner is the lowest cross-module distance; this is a low socio-technical distance signal, not missing ownership; deploy_unit and declared external_systems evidence can still raise distance when configured/detected
- connected modules in coupling sample: 18
- distance rungs implemented: D=2, D=4, D=7, D=9, D=10; omitted/compressed: D=1, D=3, D=5, D=6, D=8
- code-structure boundary crossings: 2→483
- code-structure shared-ancestor depth: 0→483
- distance compression: D=3/D=5/D=6/D=8 remain compressed: current graph/config facts distinguish same module, same owner, different owner, deploy unit, and declared vendor seam, but not finer package/library distance without guessing.
- D=1 compressed: object/member-level distance is not available from module dependency edges
- D=3 compressed: current facts distinguish same module vs cross-module, but not object/package micro-distance
- D=5 compressed: package/library middle distance is not split without explicit stable package-boundary metadata
- D=6 compressed: intermediate ownership/library distance has no deterministic signal beyond owner and tree structure
- D=8 compressed: library-like seams remain compressed: undeclared libraries stay excluded, while declared external_systems score at D=10
- undeclared external/library edges excluded: 968
- clone-only duplicated knowledge: 5 scored, 0 advisory-only
- tail risk: worst balance 2/10; lower-decile balance 2/10; high-or-worse edges 110/492 (22%); critical 99; distributed-monolith 0
- clone-only tail: worst balance 6/10; high-or-worse 0/5 scored clone-only pairs

## Coverage

- scip: ok (2409 files)
- scip-symbols: ok (12403 files)
- go/packages: ok (286 files)
- dependency-cruiser: absent
- grimp: absent
- cargo: absent
- loc: ok (286 files)
- deploy-unit: ok (8 files)
- jscpd: ok (402 files)
- ast-grep: disabled
- ast-grep/syntax: ok (562 files)
- cargo-modules: absent

## Finding index (180)

| Finding | Status | Rule |
| --- | --- | --- |
| `69d3b879c3ce3a737a2305ff389cb1e7` | baseline | bc/imbalanced_coupling |
| `2ff049a792718c864d7222e5a4ceaf2c` | new | bc/imbalanced_coupling |
| `8444168e3db5da6a0127fb1d1ddb19f6` | baseline | bc/imbalanced_coupling |
| `088c6257fe3eba57ac2a2fb4abbd6dda` | baseline | bc/imbalanced_coupling |
| `2e1590e3580d6096c9ef526ef72d1595` | new | bc/imbalanced_coupling |
| `1f841c4ef66c9d9a70430132618740e2` | baseline | bc/imbalanced_coupling |
| `25583a9622c5bc1e0fee18f579fa6a12` | new | bc/imbalanced_coupling |
| `4615d49132092a7113ecc8067314fc1e` | baseline | bc/imbalanced_coupling |
| `2cc4ff1fdf50fcbe4a39b2f9afcb7493` | new | bc/imbalanced_coupling |
| `3b3a0035c0f9a137bf5df57ffaf9161b` | baseline | bc/imbalanced_coupling |
| `52bfd50b6fa2f79a86c95672af211c64` | new | bc/imbalanced_coupling |
| `86e21ab46f46cfc77479c37d61d6365a` | baseline | bc/imbalanced_coupling |
| `bd9da20f2cdaa72e29841b99517bb28b` | baseline | bc/imbalanced_coupling |
| `3ef835400a5770fb0e5874bf35280310` | new | bc/imbalanced_coupling |
| `40ee017434202d1801b301f777e144ff` | baseline | bc/imbalanced_coupling |
| `55d937f911ce6b7a354a73aef82c01cc` | new | bc/imbalanced_coupling |
| `367ede3d7f17ccc5bc5251a927be6db7` | baseline | bc/imbalanced_coupling |
| `ff0f7fb45b81ade146258afbdf355dd3` | baseline | bc/imbalanced_coupling |
| `6793028290cda533ad5cb1fe502698a9` | baseline | bc/imbalanced_coupling |
| `0d17f93d13ffef3ee5e3a6f2b69f48b8` | new | bc/imbalanced_coupling |
| `688cebe5ec01b73b7c76c2e48d8fb028` | baseline | bc/imbalanced_coupling |
| `8046d9e0621750119e12fbad2c787ee3` | baseline | bc/imbalanced_coupling |
| `6ff80d129819b87f7e62f0edbd688f8c` | baseline | bc/imbalanced_coupling |
| `4ae0dcde706ef7333cb7428bc0eb4c80` | baseline | bc/imbalanced_coupling |
| `c19d5d66ac01d90bfe0d78e5198685c6` | baseline | bc/imbalanced_coupling |
| `1739a440ff7c65f82d106bc58e3c93b1` | baseline | bc/imbalanced_coupling |
| `72aae8ce0418334e888e19276f0a33bf` | baseline | bc/imbalanced_coupling |
| `07b0acbafd10996d237ed83afbea5013` | baseline | bc/imbalanced_coupling |
| `43daa0c91bacd482d5ac9ba560dd211e` | new | bc/imbalanced_coupling |
| `39280b30be11ccfb7099e2def1d47b8d` | baseline | bc/imbalanced_coupling |
| `4928bbc7cc6af3ad3d3e884be4d1e3f6` | new | bc/imbalanced_coupling |
| `024a054601f108bb2f65bdaa51b20789` | baseline | bc/imbalanced_coupling |
| `261cf14b927a372b977880be97fae663` | new | bc/imbalanced_coupling |
| `14a18a6187471cea76c40f7707552f67` | baseline | bc/imbalanced_coupling |
| `320ae359fba984604d56d4a447acfbfe` | new | bc/imbalanced_coupling |
| `4f3d760538a570506a5d02bdab58bee0` | baseline | bc/imbalanced_coupling |
| `506dbac7f4eb8369b27d1152ffbf1ff1` | new | bc/imbalanced_coupling |
| `14ac826f0bb219f873ec6006917d978a` | baseline | bc/imbalanced_coupling |
| `1c75d7dfef7a1ac9304d545fe3930bdf` | new | bc/imbalanced_coupling |
| `08ad9f3c6f1298ff86bb271fc0443532` | baseline | bc/imbalanced_coupling |
| `20ae16d0505f044a52761ddc937b04dc` | new | bc/imbalanced_coupling |
| `15d7aca5826ff9e51126d30d5d399056` | baseline | bc/imbalanced_coupling |
| `18e7dfc05a495bff4ffa44a0cf712f5d` | new | bc/imbalanced_coupling |
| `12585c746dab4b7aab1efe23619be40b` | baseline | bc/imbalanced_coupling |
| `6b88563fa58136a54d164ffde0b0676f` | new | bc/imbalanced_coupling |
| `6e46b555e604bee94146d5c59e9bf5f5` | new | bc/imbalanced_coupling |
| `db7cf7eb90b3de70d97c35125f9ba784` | new | bc/imbalanced_coupling |
| `09ea5d95ec52268d4b39d9ffc93ab11c` | baseline | bc/imbalanced_coupling |
| `5cfeae578d37e771c6910a5c61a18319` | new | bc/imbalanced_coupling |
| `0be20e8b1f3def37d037daf6b331b75a` | baseline | bc/imbalanced_coupling |
| `18ebbe3ec25eef143658721dfd510987` | new | bc/imbalanced_coupling |
| `fcb2c3af2a2bccd71b2521c715d08d66` | baseline | bc/imbalanced_coupling |
| `1e403aa4c68755db46cb3092b68ec621` | new | bc/imbalanced_coupling |
| `1d87155bdccc134b62dd72d5ed379094` | new | bc/imbalanced_coupling |
| `176de03c572cc06654a543910586709c` | baseline | bc/imbalanced_coupling |
| `2966e0804c1e5f7f6831422c2f90ea41` | new | bc/imbalanced_coupling |
| `e19785bd2a4e8ce6d810dc43b364814b` | baseline | bc/imbalanced_coupling |
| `eddc084a761aefcf44f5e1c6b1021f32` | baseline | bc/imbalanced_coupling |
| `019bca04cf4a5e47b4064236f86888a5` | new | bc/imbalanced_coupling |
| `feb6b9e7d13d752f9c75182ce614754b` | baseline | bc/imbalanced_coupling |
| `3913f26696ce7693d537a7188c765780` | baseline | bc/imbalanced_coupling |
| `554d5ba62be4e0b8d65deabf45165cdb` | new | bc/imbalanced_coupling |
| `074988238668fed37150966bd40f1e8f` | baseline | bc/imbalanced_coupling |
| `57802f70abea74c0988e77e48720c9b2` | new | bc/imbalanced_coupling |
| `08b3051f24559cabe4895bd0768bafe5` | baseline | bc/imbalanced_coupling |
| `2ab6dd6c6c98eb5ad56e21a522d3d938` | new | bc/imbalanced_coupling |
| `029b9c055ed2b0724c019b28844d9a66` | baseline | bc/imbalanced_coupling |
| `7300bc30e7a2eb6066153e29571b8221` | new | bc/imbalanced_coupling |
| `01ce599a2e595405a07cb76681c5ab01` | baseline | bc/imbalanced_coupling |
| `13fdf9f16fbc4f8c4bbfdfb7f2c8364b` | new | bc/imbalanced_coupling |
| `4dee147706e26c8ecd7f40d2b284c29b` | baseline | bc/imbalanced_coupling |
| `7a80f7a3cb06def6fd4e8921baafd18c` | new | bc/imbalanced_coupling |
| `bac855784f6375d70fafc86e9174c111` | baseline | bc/imbalanced_coupling |
| `242a8cc66492e9618b2e5cabc00a265d` | baseline | bc/imbalanced_coupling |
| `3d274fb8a8479e90ef68543938150453` | new | bc/imbalanced_coupling |
| `351fec2c113000956959ea63e15d2cf2` | new | bc/imbalanced_coupling |
| `525840fc8330703c0692415a21cf4c8f` | baseline | bc/imbalanced_coupling |
| `966de504d3218b404ba33f1f51a1db81` | new | bc/imbalanced_coupling |
| `86666b35572dd027448b28bffd56ca62` | baseline | bc/imbalanced_coupling |
| `ffb36399c480fbc5582e3500161d98ce` | new | bc/imbalanced_coupling |
| `7f461a14aae1bbdb961e731caea12447` | baseline | bc/imbalanced_coupling |
| `038d51209ab206374d23b5ea752cafae` | new | bc/imbalanced_coupling |
| `994263a03a96dbfaedda63e4b8197fc9` | baseline | bc/imbalanced_coupling |
| `51ad2e4f376a391f948ef1a33b7a6e11` | baseline | bc/imbalanced_coupling |
| `93d12df05a4547c1977a6dc0844c7bb6` | new | bc/imbalanced_coupling |
| `981183bceb2c3fc19c21f4f06915a332` | baseline | bc/imbalanced_coupling |
| `497f5eb92a9017ff823b7d3a95dcf57f` | baseline | bc/imbalanced_coupling |
| `75435625022bf51b091a639cc9f58efa` | baseline | bc/imbalanced_coupling |
| `26a6d8f21eaea67b22e168985df10833` | baseline | bc/imbalanced_coupling |
| `a45c29b221afaa4ccceb32bc5d9c57d5` | baseline | bc/imbalanced_coupling |
| `0c5fc2a0a2116350ac630c4cf1137bc2` | baseline | bc/imbalanced_coupling |
| `1163394a44b1ef00322bc385feb09a5b` | new | bc/imbalanced_coupling |
| `0ad7fdb637603421a7b4aad0c527c584` | baseline | bc/imbalanced_coupling |
| `a1d4f8ff196211ce44973ff39823f9c6` | new | bc/imbalanced_coupling |
| `66133c943ebed3523a1e187c43d890f4` | new | bc/imbalanced_coupling |
| `e83b01e8879297fd84f3c20f80f35df8` | baseline | bc/imbalanced_coupling |
| `7dce75be9471d8f1a76efe0167e87cb7` | new | bc/imbalanced_coupling |
| `86bdce74707ac8e7c163eb38d27591aa` | baseline | bc/imbalanced_coupling |
| `4b6f8230c3a43a5397c2d03c9e48dce2` | new | bc/imbalanced_coupling |
| `016e681e350a7b9f64711847a3c55036` | baseline | bc/imbalanced_coupling |
| `2472e9c5dd85957da047770defe380a6` | baseline | bc/imbalanced_coupling |
| `5aad718927acd40520671fc3f53f9afc` | new | bc/imbalanced_coupling |
| `76eb9b0185310484f38d648e5106b9fe` | baseline | bc/imbalanced_coupling |
| `cf27cf1ab975e9b9db64ba0cc0b9003e` | new | bc/imbalanced_coupling |
| `427ea639cccd4c6eaf80dce72d6cdf37` | baseline | bc/imbalanced_coupling |
| `26aa816ebf4a89bf5dd754884849e04b` | new | bc/imbalanced_coupling |
| `10ebd4f5c079cb2baff9d5de735d356f` | baseline | bc/imbalanced_coupling |
| `38266706c45e30154392afcf3f1f0bf0` | new | bc/imbalanced_coupling |
| `acc940d72bd31d86f74c4420cb5c9d34` | baseline | bc/imbalanced_coupling |
| `3756a47bfcee33461b6cd6cdf2023fdb` | baseline | bc/imbalanced_coupling |
| `4e5c0daa91c5e2df6d20229cc81cd02e` | new | bc/imbalanced_coupling |
| `ae45f78ce8eba5d5a6892722cf3a0d54` | baseline | bc/imbalanced_coupling |
| `385b6d539ec286d7ac0e34018ba1b047` | new | bc/imbalanced_coupling |
| `39b64c61f98496c49c51073a3780d91f` | baseline | bc/imbalanced_coupling |
| `5a38a073a03e252a02247685db390d9b` | new | bc/imbalanced_coupling |
| `ed4840b01ccbbe25f5cc444a2eb0896a` | baseline | bc/imbalanced_coupling |
| `3b838968ddfb60245a3ac977a786868d` | baseline | bc/imbalanced_coupling |
| `fda8f2ed763e5593a359696a16249ee3` | new | bc/imbalanced_coupling |
| `222d43e9a81495d242f36845e4fdb63f` | baseline | bc/imbalanced_coupling |
| `295ca67a5ccbc7aa7c1b640c2ad5f6ad` | baseline | bc/imbalanced_coupling |
| `a04e297f017511e99bee49f1bac93470` | baseline | bc/imbalanced_coupling |
| `08c681ce55f6727b8b77dd3545a215ac` | baseline | bc/imbalanced_coupling |
| `10966b817f2f4a360fee69676ce8bd25` | new | bc/imbalanced_coupling |
| `0b205e78a5a510d8c817df578b2d1f2d` | baseline | bc/imbalanced_coupling |
| `02aa6a70cc4ae6290c83bf31a719ee91` | new | bc/imbalanced_coupling |
| `03161988e1f74fbb8798cb93adc81646` | baseline | bc/imbalanced_coupling |
| `040a6f8d9a5a454ffd14fad84f2f1405` | new | bc/imbalanced_coupling |
| `2c3a0447dd65cbfe29bf677b8b2420f8` | baseline | bc/imbalanced_coupling |
| `487a2e1a2ff047836bfe89f80dedb435` | new | bc/imbalanced_coupling |
| `1188317f62fd61b5f831721759ab12d1` | baseline | bc/imbalanced_coupling |
| `8149c3562d42e6aa10dd2b0fd2f209f8` | new | bc/imbalanced_coupling |
| `3ccff7c727bf3881a6cf53d64570ef16` | baseline | bc/imbalanced_coupling |
| `a35b693b468e718b11f725e00d91b286` | new | bc/imbalanced_coupling |
| `5dc177b7d429b6e473c3975f03dca5d7` | baseline | bc/imbalanced_coupling |
| `976956ad97154a9321518b026b377d06` | new | bc/imbalanced_coupling |
| `7721c807260391fb7b3f825e2bb25de3` | baseline | bc/imbalanced_coupling |
| `b80edf266266d5d2ef69336e49fcdca0` | baseline | bc/imbalanced_coupling |
| `e0052281947927e38d7ccd7ec819e3e5` | baseline | bc/imbalanced_coupling |
| `1f17bf6a7985f31d428c95c3f3bf969b` | baseline | bc/imbalanced_coupling |
| `3d243be1372abdd8d4d1a012fff3dd06` | new | bc/imbalanced_coupling |
| `d746616284b16b13198692270b38cd9c` | new | bc/imbalanced_coupling |
| `947e399899d041834dadfc86ea2c6ef2` | baseline | bc/imbalanced_coupling |
| `382f8fbfa55a240c9563e9709a8eb2fb` | baseline | bc/imbalanced_coupling |
| `6efd59da1be96f4d3f3fd3eb57fc3603` | baseline | bc/imbalanced_coupling |
| `35f792970b6a8fb6109547bdd688ec7f` | baseline | bc/imbalanced_coupling |
| `614ee95e159796ccb96d322c042c5694` | baseline | bc/imbalanced_coupling |
| `5460d77ed1c75d296fbdad8b213b69fe` | new | bc/imbalanced_coupling |
| `881a242a665360836d10f66f5f005e3c` | baseline | bc/imbalanced_coupling |
| `ae66ef0d5fd05668821c865237e4d80a` | baseline | bc/imbalanced_coupling |
| `384a4c05d69a6a07c7af2be0bb485c71` | baseline | bc/imbalanced_coupling |
| `d7699f3bc50924cb5213951bb648fd14` | baseline | bc/imbalanced_coupling |
| `08244a901e2803d2593cf96eddbefc72` | baseline | bc/imbalanced_coupling |
| `31184fe9723516948fe8083f33b097d0` | new | bc/imbalanced_coupling |
| `16356f10bac0fda2dfccf2b120865d2b` | baseline | bc/imbalanced_coupling |
| `dbf1524996cd43a906d2b8b1b8aa5daa` | baseline | bc/imbalanced_coupling |
| `01b59d297d70a52756ee213f7f006469` | baseline | bc/imbalanced_coupling |
| `35758cbb8060d219264429a5279c06e0` | new | bc/imbalanced_coupling |
| `17b1c08443525528864360ee6ac18886` | baseline | bc/imbalanced_coupling |
| `4dc39ac70b6e69c8af9c2db3a80093c4` | new | bc/imbalanced_coupling |
| `093e032bba6ac6f06c5749a635af7e99` | baseline | bc/imbalanced_coupling |
| `f35cc32ff2d80a08c76fc78636b8b7ed` | new | bc/imbalanced_coupling |
| `108a8f5162c596eae12329198aaf67aa` | baseline | bc/imbalanced_coupling |
| `3999ad010ae199a51f53bc3a350f7479` | new | bc/imbalanced_coupling |
| `120b88a914757627b6c3fc85f7f8e1e9` | baseline | bc/imbalanced_coupling |
| `76aa4d1c8e5bc4e6d7e82f041b65443c` | new | bc/imbalanced_coupling |
| `20a6a2d090e4ec3c01b14f32a541edef` | baseline | bc/imbalanced_coupling |
| `9614d09732f3fed2a3597b0e93e3b094` | new | bc/imbalanced_coupling |
| `18ed8f9145dd0599b278815b55a45bcc` | baseline | bc/imbalanced_coupling |
| `5aae2dc9b648a11ee00d99ffedbf7f4c` | new | bc/imbalanced_coupling |
| `2e7195a3fcefee3161e601e0b6292b49` | baseline | bc/imbalanced_coupling |
| `15f11ec8efff625c4974b26fb0998df5` | new | bc/imbalanced_coupling |
| `7d1653760b760d38e90f6b188fa527e9` | baseline | bc/duplicated_knowledge |
| `d8aa67ce1be41a3fa6804d4939180b42` | baseline | bc/duplicated_knowledge |
| `82c798195ae7edc20ad3b3066a0053e8` | baseline | bc/duplicated_knowledge |
| `b088715ad4f2e2debcb026bd942f6c67` | baseline | bc/duplicated_knowledge |
| `806e208194bc4f65a519c7f24cb45f8e` | baseline | bc/duplicated_knowledge |
| `de1baaf9965964cd3bd9a5d3d3246f0e` | new | module_cycles |
| `5e6d12522dd3bf1ddae5d288da7c29f7` | new | module_cycles |
| `90578a16f4ad116c1009426c4ae12ffb` | baseline | syntax_api_size_ceiling |
| `b98c265d65853beceff848d802bb31a0` | baseline | syntax_api_size_ceiling |
