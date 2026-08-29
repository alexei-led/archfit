# archfit — architecture state

- **Verdict:** NEEDS ATTENTION
- **Blocking:** 0 active — hard gates: pass
- **Attention:** 2 dimension(s) flagged — 109 diagnostic(s)
- **Coverage:** 5 measured / 3 partial / 1 unmeasured (of 9)

## Dimensions

| Dimension | Status | Gate | Confidence | Denominator | Findings |
| --- | --- | --- | --- | --- | ---: |
| intent | measured | pass | high | declared rules evaluated 60/60 | 0 |
| structure | measured | pass | high | discovered edges resolved to a declared module 546/1264 | 0 |
| modularity | measured | warn | high | declared modules with a declared public surface 17/18 | 1 |
| coupling | measured | warn | high | cross-boundary edges scored 385/385 | 108 |
| change_locality | measured | pass | high | declared modules touched in the scanned history window 18/18 | 0 |
| complexity | partial | pass | medium | production files in the source walk 234/234 | 0 |
| testability | partial | pass | medium | classified source files 442/448 | 0 |
| operations | partial | pass | medium | applicable analyzers reporting coverage 8/8 | 0 |
| drift | unmeasured | not_applicable | unrated | _no denominator_ | 0 |

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
| ast-grep | ok | — |
| ast-grep/syntax | ok | — |
| cargo-modules | absent | — |

## Coupling seams (67)

| Seam | Strength | Distance | Volatility | Scored | Critical | Median | Quadrant | Try |
| --- | --- | --- | --- | ---: | ---: | ---: | --- | --- |
| assessment-repair → relationship-analysis | symmetric | cross_module_same_owner | high | 17 | 11 | 2 | low_cohesion | reduce_strength |
| analysis-application → assessment-repair | symmetric | cross_module_same_owner | high | 20 | 11 | 2 | low_cohesion | reduce_strength |
| assessment-repair → architecture-policy | functional | cross_module_same_owner | high | 14 | 8 | 2 | low_cohesion | reduce_strength |
| analysis-application → relationship-analysis | functional | cross_module_same_owner | high | 8 | 5 | 2 | low_cohesion | reduce_strength |
| policy-config-adapter → evidence-adapters | functional | cross_module_same_owner | high | 6 | 5 | 2 | low_cohesion | reduce_strength |
| relationship-analysis → architecture-policy | functional | cross_module_same_owner | high | 11 | 4 | 5 | low_cohesion | reduce_strength |
| evidence-adapters → persistence-adapters | functional | cross_module_same_owner | high | 11 | 4 | 5 | low_cohesion | reduce_strength |
| cli-composition → analysis-application | functional | cross_module_same_owner | high | 12 | 3 | 5 | low_cohesion | leave_alone |
| cli-composition → policy-config-adapter | functional | cross_module_same_owner | high | 16 | 3 | 5 | low_cohesion | leave_alone |
| evidence-adapters → relationship-analysis | model | cross_module_same_owner | high | 2 | 2 | 2 | low_cohesion | reduce_strength |
| analysis-application → architecture-policy | model | cross_module_same_owner | high | 2 | 2 | 2 | low_cohesion | reduce_strength |
| cli-composition → persistence-adapters | functional | cross_module_same_owner | high | 4 | 2 | 2 | low_cohesion | leave_alone |
| evidence-acquisition → evidence-adapters | symmetric | cross_module_same_owner | high | 10 | 2 | 4 | low_cohesion | reduce_strength |
| development-tools → relationship-analysis | functional | cross_module_same_owner | high | 4 | 2 | 2 | low_cohesion | leave_alone |
| cli-composition → evidence-adapters | functional | cross_module_same_owner | high | 8 | 1 | 5 | low_cohesion | leave_alone |
| evidence-acquisition → assessment-repair | model | cross_module_same_owner | high | 1 | 1 | 2 | low_cohesion | reduce_strength |
| architecture-tests → assessment-repair | model | cross_module_same_owner | high | 1 | 1 | 2 | low_cohesion | reduce_strength |
| policy-config-adapter → relationship-analysis | model | cross_module_same_owner | high | 1 | 1 | 2 | low_cohesion | reduce_strength |
| evidence-adapters → architecture-policy | functional | cross_module_same_owner | high | 2 | 1 | 2 | low_cohesion | reduce_strength |
| policy-config-adapter → architecture-policy | functional | cross_module_same_owner | high | 5 | 1 | 5 | low_cohesion | reduce_strength |

_… +47 more seams (see `--format json`)_

## Top actionable findings

### Diagnostic (109)

- **bc/imbalanced_coupling** [critical] — balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (cheap to change; not a distributed monolith))
- **bc/imbalanced_coupling** [high] — balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → high severity (contract coupling to a volatile target at low distance → cascading changes contained to one owner)
- **bc/imbalanced_coupling** [critical] — balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (cheap to change; not a distributed monolith))
- **bc/imbalanced_coupling** [medium] — balanced coupling: symmetric integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **bc/imbalanced_coupling** [medium] — balanced coupling: contract integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **bc/imbalanced_coupling** [medium] — balanced coupling: model integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **bc/imbalanced_coupling** [high] — balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → high severity (contract coupling to a volatile target at low distance → cascading changes contained to one owner)
- **bc/imbalanced_coupling** [medium] — balanced coupling: functional integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **bc/imbalanced_coupling** [critical] — balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (cheap to change; not a distributed monolith))
- **bc/imbalanced_coupling** [medium] — balanced coupling: contract integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **bc/imbalanced_coupling** [medium] — balanced coupling: functional integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **bc/imbalanced_coupling** [medium] — balanced coupling: model integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **bc/imbalanced_coupling** [medium] — balanced coupling: contract integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **bc/imbalanced_coupling** [medium] — balanced coupling: functional integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **bc/imbalanced_coupling** [critical] — balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (cheap to change; not a distributed monolith))
- **bc/imbalanced_coupling** [medium] — balanced coupling: functional integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **bc/imbalanced_coupling** [critical] — balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (cheap to change; not a distributed monolith))
- **bc/imbalanced_coupling** [medium] — balanced coupling: model integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **bc/imbalanced_coupling** [high] — balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → high severity (contract coupling to a volatile target at low distance → cascading changes contained to one owner)
- **bc/imbalanced_coupling** [medium] — balanced coupling: functional integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)

_… +89 more (see `--format json`)_

## Comparison

- **Status:** not_requested
- **Reference:** none

## Not measured (10)

- **structure — structure of 718 edges leaving the module map** (owner: relationship/facts): the target is not a declared module, so its direction and layer cannot be judged
- **change_locality — essential vs accidental volatility** (owner: history/git): commit frequency corroborates a declared volatility; it cannot establish one
- **complexity — cognitive complexity** (owner: syntax+evidence/acquisition): v1 ships no cognitive-complexity analyzer; only the size tail is measured
- **testability — supplied coverage units** (owner: syntax/fileclass): coverage is disabled, so no supplied coverage units were observed
- **testability — coverage path resolution** (owner: syntax/fileclass): coverage is disabled, so no supplied coverage paths were available to resolve
- **testability — coverage module attribution** (owner: syntax/fileclass): coverage is disabled, so no supplied coverage was available to attribute to declared modules
- **testability — coverage freshness** (owner: syntax/fileclass): coverage is disabled, so no supplied coverage freshness could be established
- **operations — observed runtime topology** (owner: policy+evidence/acquisition): v1 reports declared owners and deploy units only; nothing observes what actually runs
- **operations — supply-chain inventory** (owner: policy+evidence/acquisition): SBOM and vulnerability facts have no collector in v1
- **drift — architecture drift** (owner: assessment/decision): legacy_score_snapshot_ignored: the stored baseline predates the architecture-state contract
# archfit report

**Config hash:** `c02a9e2a2f432139921abd6617c0ed010d0f0946085924d2b91c19d31338b2e3`

## Summary

- gate findings: 0
- warnings: 109
- waivers used: 0

## Metrics

- **encapsulation**: n/a — n/a (low confidence)
- **unbalanced_edge**: 0 new high-risk unbalanced edges — strong

## Structural facts (neutral evidence)

117 modules; top 5 per axis (full list in `--format json`):

- inbound module fan-in: internal/model/evidence (44), internal/relationship (31), internal/model/graph (29), internal/policy (29), internal/toolrun (28)
- outbound destinations: cmd/archfit (34), internal/evidence/acquisition (20), internal/assessment/evaluation (19), internal/extract/acquire (17), internal/initcfg_test (17)
- LOC: cmd/archfit (5835), internal/initcfg (5162), internal/application (2853), internal/assessment/evaluation (2069), internal/relationship/analysis (2011)

## Syntax surface (neutral evidence)

3015 declaration(s) extracted by ast-grep (full list in `--format json`):

- annotation: 1
- enum: 1
- function: 1787
- interface: 41
- method: 371
- struct: 480
- type_alias: 64
- type_leak: 270
- exported (public API): 2744

Per module:

- (unscoped): 61
- analysis-application: 236
- analysis-scope: 31
- architecture-policy: 41
- architecture-tests: 59
- assessment-repair: 501
- cli-composition: 342
- config-lifecycle: 249
- development-tools: 41
- evidence-acquisition: 78
- evidence-adapters: 456
- evidence-analysis: 13
- evidence-contracts: 88
- persistence-adapters: 134
- policy-config-adapter: 144
- provider-adapters: 30
- relationship-analysis: 278
- report-adapters: 120
- report-contract: 113

### Public API


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

`cmd/archfit/application_wiring.go` [cli-composition]:
- `Load` (method)
- `Save` (method)

`cmd/archfit/autopilot_test.go` [cli-composition]:
- `Name` (method)
- `Complete` (method)
- `TestInit_LLMDraft_OwnerCommentWritten` (function)

`cmd/archfit/baseline.go` [cli-composition]:
- `BaselineCmd` (struct)
- `Help` (method)
- `Run` (method)

`cmd/archfit/byteidentical_test.go` [cli-composition]:
- `TestByteIdentical_SingleModule` (function)
- `TestByteIdentical_OneMemberWorkspace` (function)
- `TestByteIdentical_ColdWarmNoCache` (function)

`cmd/archfit/check.go` [cli-composition]:
- `CheckCmd` (struct)
- `Help` (method)
- ... +2724 more exported declarations (use `--format json`)

## Connascence evidence (deterministic)

Report-only. Static facts only; semantic and dynamic categories without deterministic evidence stay unmeasured.

- edges with evidence: 1255
- abstained edges: 4
- total evidence facts: 4859
- strength inferred from connascence: 65 edges
- by kind: algorithm=1036, meaning=661, name=1796, type=1366
- by source: go/types=3313, scip=1546
- unmeasured: position, execution, timing, value, identity
- roadmap: name=deterministic_static, type=deterministic_static, meaning=deterministic_static, algorithm=deterministic_static, position=unmeasured_static, execution=unmeasured_dynamic (signals dynamic_imports/runtime_async_edges), timing=unmeasured_dynamic (signals dynamic_imports/runtime_async_edges), value=unmeasured_dynamic (signals dynamic_imports/runtime_async_edges), identity=unmeasured_dynamic (signals dynamic_imports/runtime_async_edges)

## Dynamic connascence signals (report-only)

Report-only. Static dynamic-import and runtime-async sites can guide Ch6 execution/timing review, but they are not runtime measurements and never change score or verdict.

- signals: 2 across 1 module/source group(s)
- still unmeasured: execution, timing, value, identity
- reason: static site evidence only; deterministic runtime ordering/value/identity trace evidence is absent
- **evidence-adapters** [dynamic_import; related: execution/timing; measured=false]: 2 (e.g. internal/extract/py/grimp_helper.py:177[lazy_import], internal/extract/scip/scip_reader.py:63[lazy_import])

## Dynamic / lazy imports (hidden-coupling risk)

Report-only. Dynamic/lazy imports are invisible to the static dependency
graph, so they can hide cycles and undercount coupling.

2 sites across 1 modules (full list in `--format json`):

- **evidence-adapters**: 2 (e.g. internal/extract/py/grimp_helper.py:177[lazy_import], internal/extract/scip/scip_reader.py:63[lazy_import])

## Distance config candidates (review-only)

Report-only. Static external, runtime, and dynamic evidence can suggest `external_systems` or `deploy_unit` review, but these candidates never change distance, score, or gate verdicts.

34 signal(s) across 20 candidate(s):

- **config-lifecycle** → `github.com/goccy/go-yaml/**` [imports from classified_external_edges; action=external_systems]: 4 (e.g. internal/initcfg/subdomains_draft.go:9[imports], internal/initcfg/value_draft.go:9[imports], internal/initcfg/yamledit_parse.go:11[imports])
- **evidence-adapters** → `github.com/bmatcuk/doublestar/**` [imports from classified_external_edges; action=external_systems]: 4 (e.g. internal/extract/golang/golang.go:12[imports], internal/extract/golang/members.go:11[imports], internal/extract/py/py.go:19[imports])
- **assessment-repair** → `github.com/bmatcuk/doublestar/**` [imports from classified_external_edges; action=external_systems]: 3 (e.g. internal/assessment/rules/rules_dependency.go:10[imports], internal/assessment/staleness/staleness.go:14[imports], internal/assessment/status/status.go:8[imports])
- **provider-adapters** → `github.com/openai/openai-go/**` [imports from classified_external_edges; action=external_systems]: 3 (e.g. internal/llm/openai.go:8[imports], internal/llm/openai.go:9[imports], internal/llm/openai.go:10[imports])
- **evidence-adapters** → `golang.org/x/mod/**` [imports from classified_external_edges; action=external_systems]: 2 (e.g. internal/extract/golang/cache.go:18[imports], internal/extract/golang/members.go:12[imports])
- **provider-adapters** → `github.com/anthropics/anthropic-sdk-go/**` [imports from classified_external_edges; action=external_systems]: 2 (e.g. internal/llm/anthropic.go:9[imports], internal/llm/anthropic.go:10[imports])
- **evidence-adapters** → `evidence-adapters` [dynamic_import from dynamic_connascence_signals; action=deploy_unit]: 2 (e.g. internal/extract/py/grimp_helper.py:177[lazy_import], internal/extract/scip/scip_reader.py:63[lazy_import])
- **evidence-adapters** → `evidence-adapters` [lazy_import from dynamic_imports; action=deploy_unit]: 2 (e.g. internal/extract/py/grimp_helper.py:177[lazy_import], internal/extract/scip/scip_reader.py:63[lazy_import])
- **architecture-policy** → `github.com/bmatcuk/doublestar/**` [imports from classified_external_edges; action=external_systems]: 1 (e.g. internal/policy/module.go:15[imports])
- **cli-composition** → `github.com/alecthomas/kong/**` [imports from classified_external_edges; action=external_systems]: 1 (e.g. cmd/archfit/main.go:12[imports])
- ... +10 more candidates (use `--format json`)

## Volatility corroboration (report-only)

Report-only. Source-control touch frequency is supporting evidence for Ch9 volatility judgments and never changes score or gate verdicts.

- source: git_history
- status: ok
- recent-history window: 500 commits
- commits scanned: 500
- modules touched: 18
- caveat: Supporting evidence only. Git history can reflect both essential and accidental volatility and never changes scoring or gate verdicts.

Top touched modules:

- **cli-composition**: 175 commit(s) [declared volatility=medium]
- **evidence-adapters**: 94 commit(s) [declared volatility=medium]
- **config-lifecycle**: 78 commit(s) [declared volatility=medium]
- **policy-config-adapter**: 72 commit(s) [declared volatility=high]
- **report-adapters**: 53 commit(s) [declared volatility=medium]

## Advisory tasks (56)

Report-only rollups from grouped advisories; these do not affect verdict or gate status.
- **bc/imbalanced_coupling** [`69d3b879`] Review 2 same-shape Balanced-Coupling advisory edges from analysis-application to architecture-policy and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 2
  - group members: 69d3b879c3ce3a737a2305ff389cb1e7, e02e63908f68ae279bf02f1541b270df
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: internal/application/analysis.go, internal/application/relationship_report.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`088c6257`] Review 11 same-shape Balanced-Coupling advisory edges from analysis-application to assessment-repair and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 11
  - group members: 088c6257fe3eba57ac2a2fb4abbd6dda, 2e1590e3580d6096c9ef526ef72d1595, 499d6f0c19e84b00db0bf9e6d9fc0a0d, 5f32df6c7a80d4502aa9ef635567d328, 6b7786f551449325fafa7e87281e25f9, 781faa92ba09bc38dab9f7474e345930, 90416a2e23a40d29eaca689b5da302c6, bac595ad96d976c901f09e50962ac0e7
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: internal/application/analysis.go, internal/application/base_compare.go, internal/application/baseline.go, internal/application/relationship_report.go, internal/application/report.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`1f841c4e`] Review 8 same-shape Balanced-Coupling advisory edges from analysis-application to assessment-repair and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 8
  - group members: 1f841c4ef66c9d9a70430132618740e2, 2b2f68b0757a3eaeb5c9244588a8ff16, 46d1b53cf0d5c82479706ca7e3f74e52, 50a698c4a94070e7e276fa6164316d8e, 9a8e5d61d663876efe2854b608cfb29f, b23c3ff8a9301574ee41198330305536, bf9c5ee4242875ff9d121eea63a29737, c63bae9ac43a663e4677d59c87aa9d83
  - score: 6/10
  - top files: internal/application/analysis.go, internal/application/base_compare.go, internal/application/baseline.go, internal/application/compare.go, internal/application/report.go, internal/assessment/evaluation/relationship_projection.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=symmetric, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`4615d491`] Review 2 same-shape Balanced-Coupling advisory edges from analysis-application to evidence-contracts and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 2
  - group members: 4615d49132092a7113ecc8067314fc1e, 81716c06d3e6351857b461e684667a87
  - score: 5/10
  - top files: internal/application/analysis.go, internal/application/base_compare.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=contract, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`3b3a0035`] Review 3 same-shape Balanced-Coupling advisory edges from analysis-application to evidence-contracts and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 3
  - group members: 3b3a0035c0f9a137bf5df57ffaf9161b, 52bfd50b6fa2f79a86c95672af211c64, 8ebe12ee50b033fc6bbb0e2eb7884e20
  - score: 5/10
  - top files: internal/application/analysis.go, internal/application/relationship_report.go, internal/application/report.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`bd9da20f`] Review 2 same-shape Balanced-Coupling advisory edges from analysis-application to relationship-analysis and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 2
  - group members: bd9da20f2cdaa72e29841b99517bb28b, ec535315f78fd6a77d308ed7d9ccfc00
  - score: 5/10
  - top files: internal/application/analysis.go, internal/application/enrichment_selection.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`40ee0174`] Review 5 same-shape Balanced-Coupling advisory edges from analysis-application to relationship-analysis and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 5
  - group members: 40ee017434202d1801b301f777e144ff, 55d937f911ce6b7a354a73aef82c01cc, 9c396f88e33fd4632815029b08249e14, af440bc871e12427303d9930385db551, ef3e28e610acf5df3aececcb8b96764c
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: internal/application/analysis.go, internal/application/capture.go, internal/application/enrich.go, internal/application/enrichment_selection.go, internal/application/relationship_report.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`67930282`] Review 4 same-shape Balanced-Coupling advisory edges from analysis-application to report-contract and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 4
  - group members: 6793028290cda533ad5cb1fe502698a9, 856b984c6f6570371ec806f50bdeb9eb, e945f398ba0acaaefad734f7ea16b263, f4652ed80ac1f67fcd4fd6719e1e437b
  - score: 5/10
  - top files: internal/application/analysis.go, internal/application/base_compare.go, internal/application/baseline.go, internal/application/explain.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`07b0acba`] Review 5 same-shape Balanced-Coupling advisory edges from assessment-repair to architecture-policy and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 5
  - group members: 07b0acbafd10996d237ed83afbea5013, 71ec017e3902c4bfad1bfde3db7de350, 744edeaae5b7fd628ebdfc37ebed4a63, b6c441e6a53d2965735ca16328b0848f, d9ee7131a6b7f46a1a5095370d7edd5f
  - score: 5/10
  - top files: internal/assessment/evaluation/advisories.go, internal/assessment/evaluation/assess.go, internal/assessment/evaluation/health_warnings.go, internal/assessment/rules/rules_api.go, internal/assessment/rules/rules_dependency.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`39280b30`] Review 8 same-shape Balanced-Coupling advisory edges from assessment-repair to architecture-policy and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 8
  - group members: 39280b30be11ccfb7099e2def1d47b8d, 43daa0c91bacd482d5ac9ba560dd211e, 4928bbc7cc6af3ad3d3e884be4d1e3f6, 4c89874a384dea9db132e2e31d24d0af, 5655b0a3b20b149603ec28ba8e5a89df, 844f053bccc64638e1355c0ccde2c74f, ded385d022d1f711950a2fa8ed1d67af, f791b7772383ea8c5154650cf5de7c93
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: internal/assessment/evaluation/dimensions.go, internal/assessment/evaluation/evaluation.go, internal/assessment/evaluation/finalize.go, internal/assessment/evaluation/state.go, internal/assessment/metrics/metrics.go, internal/assessment/rules/rules.go, internal/assessment/staleness/staleness.go, internal/assessment/status/status.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`024a0546`] Review 12 same-shape Balanced-Coupling advisory edges from assessment-repair to evidence-contracts and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 12
  - group members: 024a054601f108bb2f65bdaa51b20789, 261cf14b927a372b977880be97fae663, 32aa0a8d9b525561b895230003bfcd8f, 52ab4e7c9c9797741ec665fe3ca63d65, 5529af17393932bd5258cce67b79f4ee, 7bd142c207029aed83ed13d951ca6d65, 94ef434560797a80798b50a195ff1c9d, a54099ba4cad069f2b876d062dd363c6
  - score: 5/10
  - top files: internal/assessment/evaluation/assess.go, internal/assessment/evaluation/evaluation.go, internal/assessment/evaluation/git_origin.go, internal/assessment/evaluation/projector.go, internal/assessment/evaluation/ruleset.go, internal/assessment/result/result.go, internal/assessment/rules/rules.go, internal/assessment/signals/signal.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=contract, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`14a18a61`] Review 13 same-shape Balanced-Coupling advisory edges from assessment-repair to evidence-contracts and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 13
  - group members: 14a18a6187471cea76c40f7707552f67, 320ae359fba984604d56d4a447acfbfe, 50cd43dbb9c1221793f6fd4c86415426, 648d97e461b0a5af85490566837fcf26, 7a646ee19e64da05e1e8601767e44263, 8ea9a6926178f36d81560ff9aa942ed3, a77354659d123f26f9337af753d719ef, aaf1d01e7cf774dd01a437340d8cb9e5
  - score: 5/10
  - top files: internal/assessment/agenttask/agenttask.go, internal/assessment/decision/config_compare.go, internal/assessment/decision/git_finding_delta.go, internal/assessment/evaluation/assess.go, internal/assessment/evaluation/dimensions.go, internal/assessment/evaluation/evaluation.go, internal/assessment/evaluation/health_warnings.go, internal/assessment/metrics/boundary/coverage.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`14ac826f`] Review 11 same-shape Balanced-Coupling advisory edges from assessment-repair to relationship-analysis and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 11
  - group members: 14ac826f0bb219f873ec6006917d978a, 27c5b67e5fb7a67865997ff2110a6b17, 3d8129ecba9ecee3a27d37e249e2c0db, 4f50da9ecdce6a99125244d19c5a6501, 56ffebc60d204753e8d10c0b385735e7, 6c3d4affffa9159cb403577e4da2d96e, 8e06068b0c9d4bef16e983d52c4b732f, cd2a107f381402a0e43a65ce25ba1349
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: internal/assessment/evaluation/advisories.go, internal/assessment/evaluation/advisory_severity.go, internal/assessment/evaluation/assess.go, internal/assessment/evaluation/dimensions.go, internal/assessment/evaluation/evaluation.go, internal/assessment/evaluation/relationship_projection.go, internal/assessment/rules/rules.go, internal/assessment/rules/rules_api.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`08ad9f3c`] Review 6 same-shape Balanced-Coupling advisory edges from assessment-repair to relationship-analysis and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 6
  - group members: 08ad9f3c6f1298ff86bb271fc0443532, 28513b27f490f50de7fc955712d8423b, 6b7bba3f58a30123ec30a10181b79d71, 76c8702dc476268037aca1b726f06a57, 813705841e9588cd040cac52a82a4c07, b37e406b9bf118840a5c56225daff7f2
  - score: 6/10
  - top files: internal/assessment/evaluation/advisories.go, internal/assessment/finding/finding.go, internal/assessment/metrics/boundary/cycle.go, internal/assessment/metrics/boundary/encapsulation.go, internal/assessment/metrics/boundary/unbalanced_edge.go, internal/assessment/metrics/internal/modgraph/modgraph.go, internal/assessment/rules/rules_dependency.go, internal/relationship/analysis/analysis.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=symmetric, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`15d7aca5`] Review 9 same-shape Balanced-Coupling advisory edges from cli-composition to analysis-application and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 9
  - group members: 15d7aca5826ff9e51126d30d5d399056, 18e7dfc05a495bff4ffa44a0cf712f5d, 1b2c300670d5634c581336201844d0d9, 29c1b0a7f479b468d7cd39d93f39b1ef, 6ac1d11778423abca8a0bce2def3243f, 8e165ca94f6eaea79c2913e475dad8a9, b549bcb092615763dd52d93db2c1574a, caaf0379ba18807746a28dec549f84d2
  - score: 5/10
  - top files: cmd/archfit/analyze.go, cmd/archfit/application_wiring.go, cmd/archfit/baseline.go, cmd/archfit/config_compare.go, cmd/archfit/config_enrich_adapters.go, cmd/archfit/enrich.go, cmd/archfit/enrich_abstained.go, cmd/archfit/explain.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`12585c74`] Review 3 same-shape Balanced-Coupling advisory edges from cli-composition to analysis-application and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 3
  - group members: 12585c746dab4b7aab1efe23619be40b, 6b88563fa58136a54d164ffde0b0676f, dd9b00ab182816ef88ec5dbaf7a3bff8
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: cmd/archfit/config_update_adapters.go, cmd/archfit/enrich_values.go, cmd/archfit/enrichment_judges.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`09ea5d95`] Review 2 same-shape Balanced-Coupling advisory edges from cli-composition to config-lifecycle and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 2
  - group members: 09ea5d95ec52268d4b39d9ffc93ab11c, 5cfeae578d37e771c6910a5c61a18319
  - score: 5/10
  - top files: cmd/archfit/draft_metadata.go, cmd/archfit/llmreview.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`0be20e8b`] Review 5 same-shape Balanced-Coupling advisory edges from cli-composition to config-lifecycle and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 5
  - group members: 0be20e8b1f3def37d037daf6b331b75a, 18ebbe3ec25eef143658721dfd510987, 93024e1e9b2eb9a93c211f7d8723f9ac, c8838259ed1db91f80347f0fb74b80b8, d0de32cf75db6cf7d8d67e7a9705556b
  - score: 6/10
  - top files: cmd/archfit/config_enrich_adapters.go, cmd/archfit/config_migrate.go, cmd/archfit/config_update_adapters.go, cmd/archfit/draft_metadata.go, cmd/archfit/evidence_pack.go, cmd/archfit/init.go, cmd/archfit/runecut.go, internal/initcfg/evidence_pack.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=symmetric, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`176de03c`] Review 7 same-shape Balanced-Coupling advisory edges from cli-composition to evidence-adapters and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 7
  - group members: 176de03c572cc06654a543910586709c, 2ad62b3aa083c71f1b451c00f8e72cfa, 51cd4c6f13b8779fe5be0150f9797bc9, 52a9743259f68337a35f3e18cb3cd4b4, 5b49cc4aa21e26b775fbe8a434a242b2, b171d8e322cc6a251ff9cb939ac5ec62, ef84cbb62eaeeba96fd23246ce878465
  - score: 5/10
  - top files: cmd/archfit/config_update_adapters.go, cmd/archfit/doctor.go, cmd/archfit/main.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`3913f266`] Review 2 same-shape Balanced-Coupling advisory edges from cli-composition to persistence-adapters and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 2
  - group members: 3913f26696ce7693d537a7188c765780, 554d5ba62be4e0b8d65deabf45165cdb
  - score: 5/10
  - top files: cmd/archfit/application_wiring.go, cmd/archfit/config_update_adapters.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`07498823`] Review 2 same-shape Balanced-Coupling advisory edges from cli-composition to persistence-adapters and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 2
  - group members: 074988238668fed37150966bd40f1e8f, 57802f70abea74c0988e77e48720c9b2
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: cmd/archfit/application_wiring.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`08b3051f`] Review 13 same-shape Balanced-Coupling advisory edges from cli-composition to policy-config-adapter and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 13
  - group members: 08b3051f24559cabe4895bd0768bafe5, 2ab6dd6c6c98eb5ad56e21a522d3d938, 4fb003f14c3faea28b169e0c85e9a259, 58bc8aba5d9ecb08efc4e074f63ee8c5, 6804cad616031e6bad6a4a9a8ddb1762, a993f8ba00e7797f62983e493ae1c1db, abbfe5778c068b4859b4fa62894e8190, b6278e00ffce641cb5c9df46276d9dac
  - score: 5/10
  - top files: cmd/archfit/analysis_config.go, cmd/archfit/analyze.go, cmd/archfit/application_wiring.go, cmd/archfit/config_compare.go, cmd/archfit/config_enrich_adapters.go, cmd/archfit/config_update_adapters.go, cmd/archfit/doctor.go, cmd/archfit/enrich.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`029b9c05`] Review 3 same-shape Balanced-Coupling advisory edges from cli-composition to policy-config-adapter and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 3
  - group members: 029b9c055ed2b0724c019b28844d9a66, 7300bc30e7a2eb6066153e29571b8221, e549a50c51ae95c56561a6604ccbaa2d
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: cmd/archfit/enrichment_judges.go, cmd/archfit/evidence_pack.go, cmd/archfit/rust_config_update.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`01ce599a`] Review 4 same-shape Balanced-Coupling advisory edges from cli-composition to provider-adapters and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 4
  - group members: 01ce599a2e595405a07cb76681c5ab01, 13fdf9f16fbc4f8c4bbfdfb7f2c8364b, 1a6bb1ce644dbee9eb45434c759dbadc, dc7bbf57dbec19c38717b0bbf75de0f9
  - score: 5/10
  - top files: cmd/archfit/analyze.go, cmd/archfit/config_update_adapters.go, cmd/archfit/enrichment_judges.go, cmd/archfit/update.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=contract, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`4dee1477`] Review 6 same-shape Balanced-Coupling advisory edges from cli-composition to provider-adapters and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 6
  - group members: 4dee147706e26c8ecd7f40d2b284c29b, 7a80f7a3cb06def6fd4e8921baafd18c, 87730e067efe316da849db5512a9350f, 8a83e18dca7af9c5fb368854cfef31b7, 9e723b910eda14bf00f742e79e6e1033, af2e5b717e51a4cfdd2d805330373cf5
  - score: 5/10
  - top files: cmd/archfit/config_enrich_adapters.go, cmd/archfit/enrich.go, cmd/archfit/enrich_abstained.go, cmd/archfit/explain.go, cmd/archfit/init.go, cmd/archfit/llmreview.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`

_…and 31 more advisory tasks (see --json for the full list)._

## Balanced Coupling advisories (103 rollups, 351 edges)

Same-shape edges between a module pair are grouped into one rollup.
Integration strength × distance × volatility lint messages.
Severity: `none` · `low` · `medium` · `high` · `critical`.

```
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/archfit/application_wiring.go -> internal/history/git  [07498823]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 2 same-shape edges (e.g. 074988238668fed37150966bd40f1e8f,57802f70abea74c0988e77e48720c9b2)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/archfit/config_update_adapters.go -> internal/application  [12585c74]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 3 same-shape edges (e.g. 12585c746dab4b7aab1efe23619be40b,6b88563fa58136a54d164ffde0b0676f,dd9b00ab182816ef88ec5dbaf7a3bff8)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/archfit/consts.go -> internal/extract/registry  [e19785bd]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/archfit/enrichment_judges.go -> internal/config  [029b9c05]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 3 same-shape edges (e.g. 029b9c055ed2b0724c019b28844d9a66,7300bc30e7a2eb6066153e29571b8221,e549a50c51ae95c56561a6604ccbaa2d)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/calibrate/main.go -> internal/relationship/scoring  [0ad7fdb6]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 2 same-shape edges (e.g. 0ad7fdb637603421a7b4aad0c527c584,a1d4f8ff196211ce44973ff39823f9c6)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/application/analysis.go -> internal/assessment/result  [088c6257]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 11 same-shape edges (e.g. 088c6257fe3eba57ac2a2fb4abbd6dda,2e1590e3580d6096c9ef526ef72d1595,499d6f0c19e84b00db0bf9e6d9fc0a0d,5f32df6c7a80d4502aa9ef635567d328,6b7786f551449325fafa7e87281e25f9,781faa92ba09bc38dab9f7474e345930,90416a2e23a40d29eaca689b5da302c6,bac595ad96d976c901f09e50962ac0e7)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/application/analysis.go -> internal/policy  [69d3b879]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 2 same-shape edges (e.g. 69d3b879c3ce3a737a2305ff389cb1e7,e02e63908f68ae279bf02f1541b270df)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/application/analysis.go -> internal/relationship  [40ee0174]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 5 same-shape edges (e.g. 40ee017434202d1801b301f777e144ff,55d937f911ce6b7a354a73aef82c01cc,9c396f88e33fd4632815029b08249e14,af440bc871e12427303d9930385db551,ef3e28e610acf5df3aececcb8b96764c)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/assessment/evaluation/advisories.go -> internal/relationship  [14ac826f]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 11 same-shape edges (e.g. 14ac826f0bb219f873ec6006917d978a,27c5b67e5fb7a67865997ff2110a6b17,3d8129ecba9ecee3a27d37e249e2c0db,4f50da9ecdce6a99125244d19c5a6501,56ffebc60d204753e8d10c0b385735e7,6c3d4affffa9159cb403577e4da2d96e,8e06068b0c9d4bef16e983d52c4b732f,cd2a107f381402a0e43a65ce25ba1349)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/assessment/evaluation/dimensions.go -> internal/policy  [39280b30]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 8 same-shape edges (e.g. 39280b30be11ccfb7099e2def1d47b8d,43daa0c91bacd482d5ac9ba560dd211e,4928bbc7cc6af3ad3d3e884be4d1e3f6,4c89874a384dea9db132e2e31d24d0af,5655b0a3b20b149603ec28ba8e5a89df,844f053bccc64638e1355c0ccde2c74f,ded385d022d1f711950a2fa8ed1d67af,f791b7772383ea8c5154650cf5de7c93)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/baseline/baseline.go -> internal/assessment/status  [e0052281]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/config/config.go -> internal/evidence/ports  [08244a90]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 5 same-shape edges (e.g. 08244a901e2803d2593cf96eddbefc72,31184fe9723516948fe8083f33b097d0,3ef969a59cc23213595a6775a85a9974,83dbd6a4fbb9068f0723cef599e1a334,bb9c06f306bc8d340ebd9d892847d800)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/config/projection.go -> internal/application  [35f79297]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/config/tools.go -> internal/policy  [881a242a]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/config/views.go -> internal/relationship/classify  [dbf15249]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/configschema/schema.go -> internal/config  [981183bc]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/evidence/acquisition/options.go -> internal/extract/registry  [427ea639]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 2 same-shape edges (e.g. 427ea639cccd4c6eaf80dce72d6cdf37,8a8deeff5ed584a95c2eacdbb4ff91d6)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/evidence/acquisition/service.go -> internal/application  [e83b01e8]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/evidence/acquisition/service.go -> internal/assessment/evaluation  [2472e9c5]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/evidence/acquisition/warnings.go -> internal/ownership  [ed4840b0]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/evidence/acquisition/warnings.go -> internal/policy  [016e681e]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/evidence/acquisition/warnings.go -> internal/relationship/labels  [222d43e9]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/extract/acquire/acquire.go -> internal/factcache  [1188317f]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 4 same-shape edges (e.g. 1188317f62fd61b5f831721759ab12d1,8149c3562d42e6aa10dd2b0fd2f209f8,e1ce9e049201a80ed5e4c65efc43734c,ea5ed353e6e486607ef7a6740fab8c2e)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/extract/acquire/acquire.go -> internal/policy  [a04e297f]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/extract/py/py.go -> internal/relationship/coupling  [3ccff7c7]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 2 same-shape edges (e.g. 3ccff7c727bf3881a6cf53d64570ef16,a35b693b468e718b11f725e00d91b286)
```

- ... +78 more rollups (use `--format json`)

## Advisories (6)

- **bc/duplicated_knowledge** [medium] new — internal/application/report.go → internal/testutil/report/convert.go: duplicated knowledge: cross-module code clones between analysis-application and architecture-tests with no import edge — symmetric func...
- **bc/duplicated_knowledge** [medium] new — internal/assessment/result/result.go → internal/model/report/document.go: duplicated knowledge: cross-module code clones between assessment-repair and report-contract with no import edge — symmetric functional...
- **bc/duplicated_knowledge** [medium] new — internal/initcfg/evidence_pack.go → internal/output/console/report.go: duplicated knowledge: cross-module code clones between config-lifecycle and report-adapters with no import edge — symmetric functional ...
- **bc/duplicated_knowledge** [medium] new — internal/labels/labelsio/labelsio.go → internal/llm/cache.go: duplicated knowledge: cross-module code clones between persistence-adapters and provider-adapters with no import edge — symmetric funct...
- **bc/duplicated_knowledge** [medium] new — internal/model/evidence/evidence.go → internal/model/report/evidence.go: duplicated knowledge: cross-module code clones between evidence-contracts and report-contract with no import edge — symmetric functiona...
- **syntax_api_size_ceiling** [medium] new — assessment-repair → assessment-repair: Module "assessment-repair" has 450 exported declarations, exceeding the limit of 430

## Supporting structural metrics (beyond Balanced Coupling)

Report-only. These metrics support Balanced Coupling reasoning but never gate.

- **cycle**: 0 import cycles — strong
- **coverage**: 100% coverage — strong
- **blast_radius**: 6 of 74 modules are change-impact hubs: .../model/evidence (64%, 47 deps), internal/relationship (47%, 34 deps), .../model/graph (44%, 32 deps), .../model/pattern (38%, 28 deps), .../model/symbol (32%, 23 deps)+1 more — info

## Distance confidence

- `code_structure`: always on (deterministic tree-distance baseline)
- `owner_source`: config
- `deploy_unit_source`: ok
- `owner_model`: single_owner_degenerate
- deploy-unit detector mapped modules: 3
- distance basis: code_structure=376, deploy_unit=9
- interpretation: same-owner is the lowest cross-module distance; this is a low socio-technical distance signal, not missing ownership; deploy_unit and declared external_systems evidence can still raise distance when configured/detected
- connected modules in coupling sample: 18
- distance rungs implemented: D=2, D=4, D=7, D=9, D=10; omitted/compressed: D=1, D=3, D=5, D=6, D=8
- code-structure boundary crossings: 2→376
- code-structure shared-ancestor depth: 0→376
- distance compression: D=3/D=5/D=6/D=8 remain compressed: current graph/config facts distinguish same module, same owner, different owner, deploy unit, and declared vendor seam, but not finer package/library distance without guessing.
- D=1 compressed: object/member-level distance is not available from module dependency edges
- D=3 compressed: current facts distinguish same module vs cross-module, but not object/package micro-distance
- D=5 compressed: package/library middle distance is not split without explicit stable package-boundary metadata
- D=6 compressed: intermediate ownership/library distance has no deterministic signal beyond owner and tree structure
- D=8 compressed: library-like seams remain compressed: undeclared libraries stay excluded, while declared external_systems score at D=10
- undeclared external/library edges excluded: 718
- clone-only duplicated knowledge: 5 scored, 0 advisory-only
- tail risk: worst balance 2/10; lower-decile balance 2/10; high-or-worse edges 89/385 (23%); critical 79; distributed-monolith 0
- clone-only tail: worst balance 6/10; high-or-worse 0/5 scored clone-only pairs

## Coverage

- scip: ok (1755 files)
- scip-symbols: ok (10074 files)
- go/packages: ok (236 files)
- dependency-cruiser: absent
- grimp: absent
- cargo: absent
- loc: ok (234 files)
- deploy-unit: ok (6 files)
- jscpd: ok (337 files)
- ast-grep: ok
- ast-grep/syntax: ok (412 files)
- cargo-modules: absent

## Finding index (109)

| Finding | Status | Rule |
| --- | --- | --- |
| `69d3b879c3ce3a737a2305ff389cb1e7` | new | bc/imbalanced_coupling |
| `8444168e3db5da6a0127fb1d1ddb19f6` | new | bc/imbalanced_coupling |
| `088c6257fe3eba57ac2a2fb4abbd6dda` | new | bc/imbalanced_coupling |
| `1f841c4ef66c9d9a70430132618740e2` | new | bc/imbalanced_coupling |
| `4615d49132092a7113ecc8067314fc1e` | new | bc/imbalanced_coupling |
| `3b3a0035c0f9a137bf5df57ffaf9161b` | new | bc/imbalanced_coupling |
| `86e21ab46f46cfc77479c37d61d6365a` | new | bc/imbalanced_coupling |
| `bd9da20f2cdaa72e29841b99517bb28b` | new | bc/imbalanced_coupling |
| `40ee017434202d1801b301f777e144ff` | new | bc/imbalanced_coupling |
| `367ede3d7f17ccc5bc5251a927be6db7` | new | bc/imbalanced_coupling |
| `ff0f7fb45b81ade146258afbdf355dd3` | new | bc/imbalanced_coupling |
| `6793028290cda533ad5cb1fe502698a9` | new | bc/imbalanced_coupling |
| `688cebe5ec01b73b7c76c2e48d8fb028` | new | bc/imbalanced_coupling |
| `8046d9e0621750119e12fbad2c787ee3` | new | bc/imbalanced_coupling |
| `6ff80d129819b87f7e62f0edbd688f8c` | new | bc/imbalanced_coupling |
| `4ae0dcde706ef7333cb7428bc0eb4c80` | new | bc/imbalanced_coupling |
| `c19d5d66ac01d90bfe0d78e5198685c6` | new | bc/imbalanced_coupling |
| `1739a440ff7c65f82d106bc58e3c93b1` | new | bc/imbalanced_coupling |
| `72aae8ce0418334e888e19276f0a33bf` | new | bc/imbalanced_coupling |
| `07b0acbafd10996d237ed83afbea5013` | new | bc/imbalanced_coupling |
| `39280b30be11ccfb7099e2def1d47b8d` | new | bc/imbalanced_coupling |
| `024a054601f108bb2f65bdaa51b20789` | new | bc/imbalanced_coupling |
| `14a18a6187471cea76c40f7707552f67` | new | bc/imbalanced_coupling |
| `4f3d760538a570506a5d02bdab58bee0` | new | bc/imbalanced_coupling |
| `14ac826f0bb219f873ec6006917d978a` | new | bc/imbalanced_coupling |
| `08ad9f3c6f1298ff86bb271fc0443532` | new | bc/imbalanced_coupling |
| `15d7aca5826ff9e51126d30d5d399056` | new | bc/imbalanced_coupling |
| `12585c746dab4b7aab1efe23619be40b` | new | bc/imbalanced_coupling |
| `09ea5d95ec52268d4b39d9ffc93ab11c` | new | bc/imbalanced_coupling |
| `0be20e8b1f3def37d037daf6b331b75a` | new | bc/imbalanced_coupling |
| `fcb2c3af2a2bccd71b2521c715d08d66` | new | bc/imbalanced_coupling |
| `176de03c572cc06654a543910586709c` | new | bc/imbalanced_coupling |
| `e19785bd2a4e8ce6d810dc43b364814b` | new | bc/imbalanced_coupling |
| `eddc084a761aefcf44f5e1c6b1021f32` | new | bc/imbalanced_coupling |
| `feb6b9e7d13d752f9c75182ce614754b` | new | bc/imbalanced_coupling |
| `3913f26696ce7693d537a7188c765780` | new | bc/imbalanced_coupling |
| `074988238668fed37150966bd40f1e8f` | new | bc/imbalanced_coupling |
| `08b3051f24559cabe4895bd0768bafe5` | new | bc/imbalanced_coupling |
| `029b9c055ed2b0724c019b28844d9a66` | new | bc/imbalanced_coupling |
| `01ce599a2e595405a07cb76681c5ab01` | new | bc/imbalanced_coupling |
| `4dee147706e26c8ecd7f40d2b284c29b` | new | bc/imbalanced_coupling |
| `bac855784f6375d70fafc86e9174c111` | new | bc/imbalanced_coupling |
| `242a8cc66492e9618b2e5cabc00a265d` | new | bc/imbalanced_coupling |
| `525840fc8330703c0692415a21cf4c8f` | new | bc/imbalanced_coupling |
| `86666b35572dd027448b28bffd56ca62` | new | bc/imbalanced_coupling |
| `d8c64b9b7736b12551e0501203f53592` | new | bc/imbalanced_coupling |
| `994263a03a96dbfaedda63e4b8197fc9` | new | bc/imbalanced_coupling |
| `51ad2e4f376a391f948ef1a33b7a6e11` | new | bc/imbalanced_coupling |
| `981183bceb2c3fc19c21f4f06915a332` | new | bc/imbalanced_coupling |
| `497f5eb92a9017ff823b7d3a95dcf57f` | new | bc/imbalanced_coupling |
| `75435625022bf51b091a639cc9f58efa` | new | bc/imbalanced_coupling |
| `26a6d8f21eaea67b22e168985df10833` | new | bc/imbalanced_coupling |
| `a45c29b221afaa4ccceb32bc5d9c57d5` | new | bc/imbalanced_coupling |
| `0c5fc2a0a2116350ac630c4cf1137bc2` | new | bc/imbalanced_coupling |
| `0ad7fdb637603421a7b4aad0c527c584` | new | bc/imbalanced_coupling |
| `e83b01e8879297fd84f3c20f80f35df8` | new | bc/imbalanced_coupling |
| `1b1b651ef6278011aa2b28e88cfe43fd` | new | bc/imbalanced_coupling |
| `86bdce74707ac8e7c163eb38d27591aa` | new | bc/imbalanced_coupling |
| `016e681e350a7b9f64711847a3c55036` | new | bc/imbalanced_coupling |
| `2472e9c5dd85957da047770defe380a6` | new | bc/imbalanced_coupling |
| `76eb9b0185310484f38d648e5106b9fe` | new | bc/imbalanced_coupling |
| `427ea639cccd4c6eaf80dce72d6cdf37` | new | bc/imbalanced_coupling |
| `454bddf40b558e38b6a06a299336ec93` | new | bc/imbalanced_coupling |
| `acc940d72bd31d86f74c4420cb5c9d34` | new | bc/imbalanced_coupling |
| `3756a47bfcee33461b6cd6cdf2023fdb` | new | bc/imbalanced_coupling |
| `ae45f78ce8eba5d5a6892722cf3a0d54` | new | bc/imbalanced_coupling |
| `39b64c61f98496c49c51073a3780d91f` | new | bc/imbalanced_coupling |
| `ed4840b01ccbbe25f5cc444a2eb0896a` | new | bc/imbalanced_coupling |
| `3b838968ddfb60245a3ac977a786868d` | new | bc/imbalanced_coupling |
| `222d43e9a81495d242f36845e4fdb63f` | new | bc/imbalanced_coupling |
| `295ca67a5ccbc7aa7c1b640c2ad5f6ad` | new | bc/imbalanced_coupling |
| `a04e297f017511e99bee49f1bac93470` | new | bc/imbalanced_coupling |
| `08c681ce55f6727b8b77dd3545a215ac` | new | bc/imbalanced_coupling |
| `0b205e78a5a510d8c817df578b2d1f2d` | new | bc/imbalanced_coupling |
| `03161988e1f74fbb8798cb93adc81646` | new | bc/imbalanced_coupling |
| `2c3a0447dd65cbfe29bf677b8b2420f8` | new | bc/imbalanced_coupling |
| `1188317f62fd61b5f831721759ab12d1` | new | bc/imbalanced_coupling |
| `3ccff7c727bf3881a6cf53d64570ef16` | new | bc/imbalanced_coupling |
| `5dc177b7d429b6e473c3975f03dca5d7` | new | bc/imbalanced_coupling |
| `7721c807260391fb7b3f825e2bb25de3` | new | bc/imbalanced_coupling |
| `b80edf266266d5d2ef69336e49fcdca0` | new | bc/imbalanced_coupling |
| `e0052281947927e38d7ccd7ec819e3e5` | new | bc/imbalanced_coupling |
| `1f17bf6a7985f31d428c95c3f3bf969b` | new | bc/imbalanced_coupling |
| `382f8fbfa55a240c9563e9709a8eb2fb` | new | bc/imbalanced_coupling |
| `6efd59da1be96f4d3f3fd3eb57fc3603` | new | bc/imbalanced_coupling |
| `35f792970b6a8fb6109547bdd688ec7f` | new | bc/imbalanced_coupling |
| `614ee95e159796ccb96d322c042c5694` | new | bc/imbalanced_coupling |
| `881a242a665360836d10f66f5f005e3c` | new | bc/imbalanced_coupling |
| `ae66ef0d5fd05668821c865237e4d80a` | new | bc/imbalanced_coupling |
| `384a4c05d69a6a07c7af2be0bb485c71` | new | bc/imbalanced_coupling |
| `d7699f3bc50924cb5213951bb648fd14` | new | bc/imbalanced_coupling |
| `08244a901e2803d2593cf96eddbefc72` | new | bc/imbalanced_coupling |
| `16356f10bac0fda2dfccf2b120865d2b` | new | bc/imbalanced_coupling |
| `dbf1524996cd43a906d2b8b1b8aa5daa` | new | bc/imbalanced_coupling |
| `76f0904b89e3183658a8b2b006d245ba` | new | bc/imbalanced_coupling |
| `01b59d297d70a52756ee213f7f006469` | new | bc/imbalanced_coupling |
| `17b1c08443525528864360ee6ac18886` | new | bc/imbalanced_coupling |
| `093e032bba6ac6f06c5749a635af7e99` | new | bc/imbalanced_coupling |
| `108a8f5162c596eae12329198aaf67aa` | new | bc/imbalanced_coupling |
| `120b88a914757627b6c3fc85f7f8e1e9` | new | bc/imbalanced_coupling |
| `20a6a2d090e4ec3c01b14f32a541edef` | new | bc/imbalanced_coupling |
| `18ed8f9145dd0599b278815b55a45bcc` | new | bc/imbalanced_coupling |
| `2e7195a3fcefee3161e601e0b6292b49` | new | bc/imbalanced_coupling |
| `7d1653760b760d38e90f6b188fa527e9` | new | bc/duplicated_knowledge |
| `d8aa67ce1be41a3fa6804d4939180b42` | new | bc/duplicated_knowledge |
| `82c798195ae7edc20ad3b3066a0053e8` | new | bc/duplicated_knowledge |
| `b088715ad4f2e2debcb026bd942f6c67` | new | bc/duplicated_knowledge |
| `806e208194bc4f65a519c7f24cb45f8e` | new | bc/duplicated_knowledge |
| `90578a16f4ad116c1009426c4ae12ffb` | new | syntax_api_size_ceiling |
