# archfit — decision

- **Decision:** ACCEPTABLE WITH WATCH ITEMS
- **Gate:** PASS — 0 blocking
- **Warnings:** 107 advisory
- **Score:** 47 / 100 (mixed)

Acceptable with watch items. Monitor flagged areas.

## Recommendations

### Must fix
- none

### Should fix
- **bc/imbalanced_coupling** — balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (cheap to change; not a distributed monolith))

### Watch
- **bc/duplicated_knowledge** — duplicated knowledge: cross-module code clones between fact-adapters and pipeline-engine with no import edge — symmetric functional coupling; a change to the shared logic must be repeated in both modules. Extract the shared knowledge, or accept the pair with an approved label
- **bc/imbalanced_coupling** — balanced coupling: contract integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **syntax_api_size_ceiling** — Module "fact-adapters" has 417 exported declarations, exceeding the limit of 320

## Why the score is low

- **coupling_balance** (47/100, mixed): critical-band coupling present; inspect the reported strength, distance, and volatility drivers — 421 scored internal cross-boundary edges; mean book balance 5.2/10 → value 47; strength distribution: contract=32, functional=157, model=205, symmetric=27; distance distribution: cross_module_same_owner=421; volatility distribution: high=99, low=68, medium=254; balance drivers: strength_distance=121, tie=93, volatility=207; critical drivers: strength_distance=47; top module pairs: archfit-cli -> fact-adapters=34, fact-adapters -> evidence-model=27, archfit-cli -> assessment-repair=22, archfit-cli -> policy-language=21, assessment-repair -> evidence-model=17; scored fraction: 100% (421 scored, 0 abstained, internal only)
  - _What moves it:_ Reduce high-fan-in functional edges across module boundaries or introduce stable contracts.
# archfit report

**Verdict:** warn (exit 2)
**Config hash:** `8d7e7453ab8f50774555659cee311d0ec77ba0b88dcfd0228eb653d9b6f4b7f5`

## Summary

- gate findings: 0
- warnings: 107
- waivers used: 0

## Metrics

- **encapsulation**: n/a — n/a (low confidence)
- **unbalanced_edge**: 0 new high-risk unbalanced edges — strong

## Structural facts (neutral evidence)

106 modules; top 5 per axis (full list in `--format json`):

- inbound module fan-in: internal/model/graph (44), internal/model/evidence (42), internal/view (33), internal/model/report (31), internal/model/finding (27)
- outbound destinations: cmd/archfit (46), internal/engine (24), internal/engine_test (21), internal/initcfg_test (16), internal/assessment/decision_test (10)
- LOC: cmd/archfit (8758), internal/initcfg (4924), internal/engine (2933), internal/relationship/classify (1657), internal/extract/golang (1523)

## Syntax surface (neutral evidence)

2256 declaration(s) extracted by ast-grep (full list in `--format json`):

- annotation: 1
- enum: 1
- function: 1520
- interface: 18
- method: 249
- struct: 258
- type_alias: 33
- type_leak: 176
- exported (public API): 2079

Per module:

- (unscoped): 74
- archfit-cli: 335
- architecture-model: 16
- architecture-tests: 7
- assessment-repair: 246
- baseline-store: 21
- config-lifecycle: 233
- coupling-model: 49
- development-tools: 41
- evaluation-core: 48
- evidence-model: 81
- fact-adapters: 452
- finding-model: 14
- labels-policy: 13
- labels-store: 5
- llm-adapter: 30
- pipeline-contracts: 104
- pipeline-engine: 109
- policy-language: 119
- relationship-analysis: 79
- rendering: 95
- report-contract: 36
- scan-contract: 25
- stage-views: 24

### Public API


`cmd/archfit/analyze.go` [archfit-cli]:
- `AnalyzeCmd` (struct)
- `Help` (method)
- `Run` (method)
- `Run` (method)

`cmd/archfit/autopilot_test.go` [archfit-cli]:
- `Name` (method)
- `Complete` (method)
- `TestInit_LLMDraft_OwnerCommentWritten` (function)

`cmd/archfit/baseline.go` [archfit-cli]:
- `BaselineCmd` (struct)
- `Help` (method)
- `Run` (method)

`cmd/archfit/byteidentical_test.go` [archfit-cli]:
- `TestByteIdentical_SingleModule` (function)
- `TestByteIdentical_OneMemberWorkspace` (function)
- `TestByteIdentical_ColdWarmNoCache` (function)

`cmd/archfit/check.go` [archfit-cli]:
- `CheckCmd` (struct)
- `Help` (method)
- `Run` (method)

`cmd/archfit/check_test.go` [archfit-cli]:
- `TestRun_Check_ExitCodeZeroOnCleanConfig` (function)
- `TestRun_Check_ExitCodeOneOnViolatedPolicy` (function)
- `TestRun_Check_ExitCodeThreeOnBadConfigPath` (function)
- `TestRun_Analyze_IsReportOnlyOnViolatedPolicy` (function)
- ... +2059 more exported declarations (use `--format json`)

## Connascence evidence (deterministic)

Report-only. Static facts only; semantic and dynamic categories without deterministic evidence stay unmeasured.

- edges with evidence: 1093
- abstained edges: 4
- total evidence facts: 4303
- strength inferred from connascence: 64 edges
- by kind: algorithm=956, meaning=614, name=1568, type=1165
- by source: go/types=2923, scip=1380
- unmeasured: position, execution, timing, value, identity
- roadmap: name=deterministic_static, type=deterministic_static, meaning=deterministic_static, algorithm=deterministic_static, position=unmeasured_static, execution=unmeasured_dynamic (signals dynamic_imports/runtime_async_edges), timing=unmeasured_dynamic (signals dynamic_imports/runtime_async_edges), value=unmeasured_dynamic (signals dynamic_imports/runtime_async_edges), identity=unmeasured_dynamic (signals dynamic_imports/runtime_async_edges)

## Dynamic connascence signals (report-only)

Report-only. Static dynamic-import and runtime-async sites can guide Ch6 execution/timing review, but they are not runtime measurements and never change score or verdict.

- signals: 2 across 1 module/source group(s)
- still unmeasured: execution, timing, value, identity
- reason: static site evidence only; deterministic runtime ordering/value/identity trace evidence is absent
- **fact-adapters** [dynamic_import; related: execution/timing; measured=false]: 2 (e.g. internal/extract/py/grimp_helper.py:177[lazy_import], internal/extract/scip/scip_reader.py:63[lazy_import])

## Dynamic / lazy imports (hidden-coupling risk)

Report-only. Dynamic/lazy imports are invisible to the static dependency
graph, so they can hide cycles and undercount coupling.

2 sites across 1 modules (full list in `--format json`):

- **fact-adapters**: 2 (e.g. internal/extract/py/grimp_helper.py:177[lazy_import], internal/extract/scip/scip_reader.py:63[lazy_import])

## Distance config candidates (review-only)

Report-only. Static external, runtime, and dynamic evidence can suggest `external_systems` or `deploy_unit` review, but these candidates never change distance, score, or gate verdicts.

35 signal(s) across 21 candidate(s):

- **fact-adapters** → `github.com/bmatcuk/doublestar/**` [imports from classified_external_edges; action=external_systems]: 5 (e.g. internal/extract/golang/golang.go:10[imports], internal/extract/golang/members.go:11[imports], internal/extract/py/py.go:17[imports])
- **config-lifecycle** → `github.com/goccy/go-yaml/**` [imports from classified_external_edges; action=external_systems]: 4 (e.g. internal/initcfg/subdomains_draft.go:9[imports], internal/initcfg/value_draft.go:9[imports], internal/initcfg/yamledit_parse.go:11[imports])
- **llm-adapter** → `github.com/openai/openai-go/**` [imports from classified_external_edges; action=external_systems]: 3 (e.g. internal/llm/openai.go:8[imports], internal/llm/openai.go:9[imports], internal/llm/openai.go:10[imports])
- **assessment-repair** → `github.com/bmatcuk/doublestar/**` [imports from classified_external_edges; action=external_systems]: 2 (e.g. internal/assessment/staleness/staleness.go:14[imports], internal/assessment/status/status.go:8[imports])
- **fact-adapters** → `golang.org/x/mod/**` [imports from classified_external_edges; action=external_systems]: 2 (e.g. internal/extract/golang/cache.go:16[imports], internal/extract/golang/members.go:12[imports])
- **llm-adapter** → `github.com/anthropics/anthropic-sdk-go/**` [imports from classified_external_edges; action=external_systems]: 2 (e.g. internal/llm/anthropic.go:9[imports], internal/llm/anthropic.go:10[imports])
- **fact-adapters** → `fact-adapters` [dynamic_import from dynamic_connascence_signals; action=deploy_unit]: 2 (e.g. internal/extract/py/grimp_helper.py:177[lazy_import], internal/extract/scip/scip_reader.py:63[lazy_import])
- **fact-adapters** → `fact-adapters` [lazy_import from dynamic_imports; action=deploy_unit]: 2 (e.g. internal/extract/py/grimp_helper.py:177[lazy_import], internal/extract/scip/scip_reader.py:63[lazy_import])
- **archfit-cli** → `github.com/alecthomas/kong/**` [imports from classified_external_edges; action=external_systems]: 1 (e.g. cmd/archfit/main.go:12[imports])
- **archfit-cli** → `github.com/bmatcuk/doublestar/**` [imports from classified_external_edges; action=external_systems]: 1 (e.g. cmd/archfit/draft_metadata.go:9[imports])
- ... +11 more candidates (use `--format json`)

## Volatility corroboration (report-only)

Report-only. Source-control touch frequency is supporting evidence for Ch9 volatility judgments and never changes score or gate verdicts.

- source: git_history
- status: ok
- recent-history window: 500 commits
- commits scanned: 500
- modules touched: 20
- caveat: Supporting evidence only. Git history can reflect both essential and accidental volatility and never changes scoring or gate verdicts.

Top touched modules:

- **archfit-cli**: 163 commit(s) [declared volatility=medium]
- **pipeline-engine**: 109 commit(s) [declared volatility=medium]
- **fact-adapters**: 102 commit(s) [declared volatility=medium]
- **policy-language**: 73 commit(s) [declared volatility=high]
- **config-lifecycle**: 71 commit(s) [declared volatility=medium]

## Advisory tasks (63)

Report-only rollups from grouped advisories; these do not affect verdict or gate status.
- **bc/imbalanced_coupling** [`01b0db59`] Review 5 same-shape Balanced-Coupling advisory edges from archfit-cli to architecture-model and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 5
  - group members: 01b0db59b3d599ee59003a372499ce46, 4b2864a9c1285d45af622ac77680ffef, 6598b0dcac0376215ec99a950cd84c32, 6ba848bfe4df1afe3143a49cc5d0f3c2, ffd9ffd66a94686756c9b123f4756158
  - score: 5/10
  - top files: cmd/archfit/enrich.go, cmd/archfit/enrich_abstained.go, cmd/archfit/pipeline_run.go, cmd/archfit/update.go, cmd/archfit/volatility_corroboration.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`5aadacc6`] Review 2 same-shape Balanced-Coupling advisory edges from archfit-cli to architecture-model and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 2
  - group members: 5aadacc61f3c99b11144b76d63dada09, 69c05033655351da6ead64fbac3f8485
  - score: 5/10
  - top files: cmd/archfit/enrich_values.go, cmd/archfit/init.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`0780c16c`] Review 7 same-shape Balanced-Coupling advisory edges from archfit-cli to assessment-repair and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 7
  - group members: 0780c16cc0d72072e026e84a116779d5, 45af9af8b0b826a4f5ee16c83ee390aa, 8d0711a56383e0409bd8b3a9df5fdd02, 9ff044de9d3757b9b697fb0d3e182ff7, b28dc8588f6f6c167b1d2bc99c8549a1, d59ae2d1ff3b3fdfd824c782d0827ab6, edebca9952b56020f60288160ce29322
  - score: 5/10
  - top files: cmd/archfit/analyze.go, cmd/archfit/config_compare.go, cmd/archfit/git_finding_delta.go, cmd/archfit/pipeline_run.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`00e06a21`] Review 15 same-shape Balanced-Coupling advisory edges from archfit-cli to assessment-repair and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 15
  - group members: 00e06a2185efc95931f6a281ffd4f667, 21c16936229b739455c9ebdf57e9fe70, 2cf86ec76f5fb9af79ce9f72fdfa4d9e, 2fc4b0a0d9aa93e9e860c407bc7942da, 33c4a1a29f030c88964656ae87ccd545, 43478e904b0af9f35f2d5ac695427271, 460fe751331eabafc1f1a72d055f53da, 5c55b79a21dc42c25d59494216c1e7c7
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: cmd/archfit/analyze.go, cmd/archfit/baseline.go, cmd/archfit/config_compare.go, cmd/archfit/distance_context.go, cmd/archfit/enrich.go, cmd/archfit/enrich_abstained.go, cmd/archfit/explain.go, cmd/archfit/git_finding_delta.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`09ea5d95`] Review 2 same-shape Balanced-Coupling advisory edges from archfit-cli to config-lifecycle and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 2
  - group members: 09ea5d95ec52268d4b39d9ffc93ab11c, 5cfeae578d37e771c6910a5c61a18319
  - score: 5/10
  - top files: cmd/archfit/draft_metadata.go, cmd/archfit/llmreview.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`18ebbe3e`] Review 5 same-shape Balanced-Coupling advisory edges from archfit-cli to config-lifecycle and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 5
  - group members: 18ebbe3ec25eef143658721dfd510987, 713435dc7222cb1d64468c472eb20600, b3a81c151f702a38907f50d25d5d3be2, d0de32cf75db6cf7d8d67e7a9705556b, ed7a32d97c89c445b9f64a12432273bc
  - score: 6/10
  - top files: cmd/archfit/draft_metadata.go, cmd/archfit/enrich.go, cmd/archfit/enrich_values.go, cmd/archfit/evidence_pack.go, cmd/archfit/init.go, cmd/archfit/update.go, internal/initcfg/evidence_pack.go, internal/initcfg/initcfg.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=symmetric, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`0b2dbc0f`] Review 5 same-shape Balanced-Coupling advisory edges from archfit-cli to coupling-model and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 5
  - group members: 0b2dbc0f272690e8c7a570abeccba563, 1aa41e85b46a513fc461a16873e16743, 23ba33f54622e001343e75d13c1fe970, 37722647f1df242ab64b7ec66569f66c, ca0d3a81e3286df526c19838b5bbcf89
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: cmd/archfit/baseline.go, cmd/archfit/enrich.go, cmd/archfit/enrich_abstained.go, cmd/archfit/explain.go, cmd/archfit/llmreview.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`5785eb4b`] Review 2 same-shape Balanced-Coupling advisory edges from archfit-cli to evaluation-core and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 2
  - group members: 5785eb4b39bf46ade814ce4f614006cb, a44e7ad048371eb1a19d72f56767e3b4
  - score: 5/10
  - top files: cmd/archfit/pipeline_config.go, cmd/archfit/pipeline_run.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`3ba9d033`] Review 3 same-shape Balanced-Coupling advisory edges from archfit-cli to evidence-model and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 3
  - group members: 3ba9d033b58d56d4b89b1f09d311ecc5, 4bd04cc81d5e64503ec8e462ef1005fc, 4d4bcf55e22e3424ca3f048d4932a6e9
  - score: 5/10
  - top files: cmd/archfit/enrich.go, cmd/archfit/enrich_abstained.go, cmd/archfit/update.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`384e5f46`] Review 10 same-shape Balanced-Coupling advisory edges from archfit-cli to evidence-model and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 10
  - group members: 384e5f46af347c23de423b7fd61c7310, 413473b096e767fc422ba9a2afd75db0, 438ec1eb886a7e9ca09937f4f463765f, 4eca2b7d711e5fd7adab7c39338bb317, 58607b45e00ebaffe292ff85039d0588, 813f6882972abacc9fc3623cc1fd5195, a387e4b0ecbb56c4545d04cd8cd1e79b, a6731bd114fbe1c761f03f07ab0f40aa
  - score: 5/10
  - top files: cmd/archfit/distance_context.go, cmd/archfit/git_finding_delta.go, cmd/archfit/llmreview.go, cmd/archfit/pipeline_config.go, cmd/archfit/pipeline_coverage.go, cmd/archfit/pipeline_run.go, cmd/archfit/pipeline_warnings.go, cmd/archfit/update.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`3f597ecb`] Review 3 same-shape Balanced-Coupling advisory edges from archfit-cli to fact-adapters and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 3
  - group members: 3f597ecb1a08b4e1b2277101ef03ce68, 4392dba45e4f134ae5350609d5ff2b60, 49c86332aa2eb6e1adabd18aec1fd613
  - score: 5/10
  - top files: cmd/archfit/pipeline_run.go, cmd/archfit/registry.go, cmd/archfit/volatility_corroboration.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=contract, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`07bb0946`] Review 29 same-shape Balanced-Coupling advisory edges from archfit-cli to fact-adapters and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 29
  - group members: 07bb094642d4e9307f6dd14debdd31c1, 0ab4b821189ca416e79ebd4b206efec8, 0f119e240a390a5c3e7f78d343de8897, 1ae64ab6f037bfec759dc62a0b9ed3d4, 1d75b7ae0957f19ac294d1fbd26e8a9d, 204f43b88478926dda7e96c50706eef6, 297e7ac553b7790c16502f128922cd3c, 2ade6dbafdaabdcdb26854a677e20a33
  - score: 5/10
  - top files: cmd/archfit/doctor.go, cmd/archfit/main.go, cmd/archfit/pipeline_coverage.go, cmd/archfit/pipeline_run.go, cmd/archfit/registry.go, cmd/archfit/update.go, cmd/archfit/volatility_corroboration.go, cmd/archfit/worktree.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`460f5c07`] Review 2 same-shape Balanced-Coupling advisory edges from archfit-cli to fact-adapters and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 2
  - group members: 460f5c073149fa15d3d66549c22e0b37, 8ae6c5b610a92a7136eeb8692fffdea6
  - score: 5/10
  - top files: cmd/archfit/pipeline_config.go, cmd/archfit/registry.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`023add20`] Review 5 same-shape Balanced-Coupling advisory edges from archfit-cli to finding-model and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 5
  - group members: 023add20bf21c6007cb54b20b4de973a, 1101eb26f63e6ca5402405104c4bac48, 8afbbad051fd4b9e5d7916b078495f68, 8bf233b1773be9a6613ef77094c131e1, 9e3eebe06f46134755564db05a15cbd4
  - score: 5/10
  - top files: cmd/archfit/baseline.go, cmd/archfit/explain.go, cmd/archfit/git_finding_delta.go, cmd/archfit/llmreview.go, cmd/archfit/pipeline_run.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`7d9740d6`] Review 2 same-shape Balanced-Coupling advisory edges from archfit-cli to labels-policy and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 2
  - group members: 7d9740d6c49cfef92c4c088a38146c54, 85c3b03c48087edcc548ecca5771626d
  - score: 5/10
  - top files: cmd/archfit/enrich.go, cmd/archfit/enrich_abstained.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`01ce599a`] Review 2 same-shape Balanced-Coupling advisory edges from archfit-cli to llm-adapter and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 2
  - group members: 01ce599a2e595405a07cb76681c5ab01, 13fdf9f16fbc4f8c4bbfdfb7f2c8364b
  - score: 5/10
  - top files: cmd/archfit/analyze.go, cmd/archfit/update.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=contract, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`4dee1477`] Review 6 same-shape Balanced-Coupling advisory edges from archfit-cli to llm-adapter and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 6
  - group members: 4dee147706e26c8ecd7f40d2b284c29b, 7a80f7a3cb06def6fd4e8921baafd18c, 87730e067efe316da849db5512a9350f, 8a83e18dca7af9c5fb368854cfef31b7, af2e5b717e51a4cfdd2d805330373cf5, ee1d062958cb74b32d1073cf500c10a6
  - score: 6/10
  - top files: cmd/archfit/enrich.go, cmd/archfit/enrich_abstained.go, cmd/archfit/enrich_values.go, cmd/archfit/explain.go, cmd/archfit/init.go, cmd/archfit/llmreview.go, internal/llm/cache.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=symmetric, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`2e85c357`] Review 5 same-shape Balanced-Coupling advisory edges from archfit-cli to pipeline-engine and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 5
  - group members: 2e85c357a78837ee6437847d6597e135, 9452042ab3d1dd826d9fe09eb9d78b89, 94aed280063bd37945a3a57011320bac, a114da5336cf0e0842e4113fb5beefb3, cb27127473df45cef48198ddc0f8d9bb
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: cmd/archfit/analyze.go, cmd/archfit/baseline.go, cmd/archfit/config_compare.go, cmd/archfit/explain.go, cmd/archfit/worktree.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`6c5f0852`] Review 4 same-shape Balanced-Coupling advisory edges from archfit-cli to pipeline-engine and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 4
  - group members: 6c5f08524e9135bb334dc9ae2ee37c4e, 7f564ac56da87c57a9bb568b6e7af750, 96635ffe4f92e42e9b433fb079f202fb, fa94b51f67a28f758634f247b99040f4
  - score: 6/10
  - top files: cmd/archfit/enrich.go, cmd/archfit/enrich_abstained.go, cmd/archfit/pipeline_run.go, cmd/archfit/update.go, internal/engine/assemble.go, internal/engine/labels.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=symmetric, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`0253289b`] Review 16 same-shape Balanced-Coupling advisory edges from archfit-cli to policy-language and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 16
  - group members: 0253289bc1623d1c847fbb4b14db01ca, 1cc457d122eda82495745b71a8623a10, 2ab6dd6c6c98eb5ad56e21a522d3d938, 4fb003f14c3faea28b169e0c85e9a259, 58bc8aba5d9ecb08efc4e074f63ee8c5, 60348cc93b9075d49384cef144d1e6b2, 90b6c329a2a1e1ef28f8e49d40037cb4, a993f8ba00e7797f62983e493ae1c1db
  - score: 5/10
  - top files: cmd/archfit/analyze.go, cmd/archfit/config_compare.go, cmd/archfit/doctor.go, cmd/archfit/enrich.go, cmd/archfit/enrich_abstained.go, cmd/archfit/explain.go, cmd/archfit/git_finding_delta.go, cmd/archfit/init.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`029b9c05`] Review 5 same-shape Balanced-Coupling advisory edges from archfit-cli to policy-language and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 5
  - group members: 029b9c055ed2b0724c019b28844d9a66, 05fe7e65f0032d88ec6f205d6ba85bed, 07d394a78707ad0aa666083f94fef636, 7300bc30e7a2eb6066153e29571b8221, bfcaec1b44a0e71292d68b55e69c71a2
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: cmd/archfit/distance_context.go, cmd/archfit/evidence_pack.go, cmd/archfit/pipeline_warnings.go, cmd/archfit/rust_config_update.go, cmd/archfit/worktree.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`242a8cc6`] Review 5 same-shape Balanced-Coupling advisory edges from archfit-cli to rendering and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 5
  - group members: 242a8cc66492e9618b2e5cabc00a265d, 3d274fb8a8479e90ef68543938150453, 83f06ac754fc79418d0f127b74b143cb, 978820d1593945c318aa9d9220a8b2ca, b317bcad5f5ca1ef233ec9324fd61299
  - score: 5/10
  - top files: cmd/archfit/analyze.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`3c44e399`] Review 2 same-shape Balanced-Coupling advisory edges from archfit-cli to report-contract and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 2
  - group members: 3c44e3991724dedb2252f9b03a2d70fa, 86666b35572dd027448b28bffd56ca62
  - score: 5/10
  - top files: cmd/archfit/baseline.go, cmd/archfit/config_compare.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`19b306c7`] Review 6 same-shape Balanced-Coupling advisory edges from archfit-cli to report-contract and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 6
  - group members: 19b306c7c0e0eeb92975c35f07a3e7e8, 26f689d26724c64e72fe21b61c574ddf, 44c0119c29efa86c0170591acdedf715, 50dd35946c71b6f3e4ecc75475e1e153, a033f814bea82eef9941d07505eeef07, c19dff487bdc6ec0d3520669d9bc96a5
  - score: 5/10
  - top files: cmd/archfit/enrich.go, cmd/archfit/git_finding_delta.go, cmd/archfit/pipeline_config.go, cmd/archfit/pipeline_coverage.go, cmd/archfit/pipeline_run.go, cmd/archfit/pipeline_warnings.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`0e30cc4b`] Review 3 same-shape Balanced-Coupling advisory edges from assessment-repair to coupling-model and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 3
  - group members: 0e30cc4b45d34630716a06481517f77e, c655781a7251e421de4e3a406562689e, dde67742b8446b212a15fb105f7de2ca
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: internal/assessment/metrics/boundary/encapsulation.go, internal/assessment/score/score_boundary_coupling.go, internal/assessment/signals/signal.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`

_…and 38 more advisory tasks (see --json for the full list)._

## Balanced Coupling advisories (102 rollups, 352 edges)

Same-shape edges between a module pair are grouped into one rollup.
Integration strength × distance × volatility lint messages.
Severity: `none` · `low` · `medium` · `high` · `critical`.

```
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/archfit/analyze.go -> internal/assessment/result  [00e06a21]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 15 same-shape edges (e.g. 00e06a2185efc95931f6a281ffd4f667,21c16936229b739455c9ebdf57e9fe70,2cf86ec76f5fb9af79ce9f72fdfa4d9e,2fc4b0a0d9aa93e9e860c407bc7942da,33c4a1a29f030c88964656ae87ccd545,43478e904b0af9f35f2d5ac695427271,460fe751331eabafc1f1a72d055f53da,5c55b79a21dc42c25d59494216c1e7c7)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/archfit/analyze.go -> internal/engine  [2e85c357]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 5 same-shape edges (e.g. 2e85c357a78837ee6437847d6597e135,9452042ab3d1dd826d9fe09eb9d78b89,94aed280063bd37945a3a57011320bac,a114da5336cf0e0842e4113fb5beefb3,cb27127473df45cef48198ddc0f8d9bb)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/archfit/baseline.go -> internal/model/coupling  [0b2dbc0f]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 5 same-shape edges (e.g. 0b2dbc0f272690e8c7a570abeccba563,1aa41e85b46a513fc461a16873e16743,23ba33f54622e001343e75d13c1fe970,37722647f1df242ab64b7ec66569f66c,ca0d3a81e3286df526c19838b5bbcf89)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/archfit/distance_context.go -> internal/config  [029b9c05]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 5 same-shape edges (e.g. 029b9c055ed2b0724c019b28844d9a66,05fe7e65f0032d88ec6f205d6ba85bed,07d394a78707ad0aa666083f94fef636,7300bc30e7a2eb6066153e29571b8221,bfcaec1b44a0e71292d68b55e69c71a2)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/calibrate/main.go -> internal/model/coupling  [a967c418]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/assessment/metrics/boundary/encapsulation.go -> internal/model/coupling  [0e30cc4b]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 3 same-shape edges (e.g. 0e30cc4b45d34630716a06481517f77e,c655781a7251e421de4e3a406562689e,dde67742b8446b212a15fb105f7de2ca)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/baseline/baseline.go -> internal/assessment/status  [e0052281]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/baseline/baseline.go -> internal/model/coupling  [454623ac]
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
ARCHFIT[BC-UNBALANCED CRITICAL] internal/engine/advisories.go -> internal/assessment/result  [0c46da3b]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 2 same-shape edges (e.g. 0c46da3b624e5eabd602b1358b2349de,5fadb2c3bffc3e7dd43fb2f2013775f1)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/engine/engine.go -> internal/model/coupling  [d53d2841]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/extract/py/py.go -> internal/model/coupling  [9c7cdcce]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 2 same-shape edges (e.g. 9c7cdcced43d9e244f06f4f4d084e523,fe2bbcf6b45a06e997e90bfe1f22135a)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/output/jsonout/jsonout.go -> internal/model/coupling  [63639586]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/relationship/classify/distance_structure.go -> internal/model/coupling  [1866ace4]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 4 same-shape edges (e.g. 1866ace40750c6083a78c80e1d05f6a8,35cfd021c22373b1ea8133a6f5cd7464,8e798ec36b4aa9a597910cf9fc023e08,a417647671f05574b8a04a462956621c)
```

```
ARCHFIT[BC-UNBALANCED HIGH] internal/view/view.go -> internal/model/coupling  [2c6f1457]
  integration strength: contract      distance: cross_module_same_owner         volatility: high
  score: 4/10 (high) [book]
  why: balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → high severity (contract coupling to a volatile target at low distance → cascading chang...
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/analyze.go -> internal/assessment/decision  [0780c16c]
  integration strength: functional    distance: cross_module_same_owner         volatility: high
  score: 5/10 (medium) [book]
  why: balanced coupling: functional integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 7 same-shape edges (e.g. 0780c16cc0d72072e026e84a116779d5,45af9af8b0b826a4f5ee16c83ee390aa,8d0711a56383e0409bd8b3a9df5fdd02,9ff044de9d3757b9b697fb0d3e182ff7,b28dc8588f6f6c167b1d2bc99c8549a1,d59ae2d1ff3b3fdfd824c782d0827ab6,edebca9952b56020f60288160ce29322)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/analyze.go -> internal/config  [0253289b]
  integration strength: functional    distance: cross_module_same_owner         volatility: high
  score: 5/10 (medium) [book]
  why: balanced coupling: functional integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 16 same-shape edges (e.g. 0253289bc1623d1c847fbb4b14db01ca,1cc457d122eda82495745b71a8623a10,2ab6dd6c6c98eb5ad56e21a522d3d938,4fb003f14c3faea28b169e0c85e9a259,58bc8aba5d9ecb08efc4e074f63ee8c5,60348cc93b9075d49384cef144d1e6b2,90b6c329a2a1e1ef28f8e49d40037cb4,a993f8ba00e7797f62983e493ae1c1db)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/analyze.go -> internal/llm  [01ce599a]
  integration strength: contract      distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: contract integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 2 same-shape edges (e.g. 01ce599a2e595405a07cb76681c5ab01,13fdf9f16fbc4f8c4bbfdfb7f2c8364b)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/analyze.go -> internal/output/console  [242a8cc6]
  integration strength: functional    distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: functional integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 5 same-shape edges (e.g. 242a8cc66492e9618b2e5cabc00a265d,3d274fb8a8479e90ef68543938150453,83f06ac754fc79418d0f127b74b143cb,978820d1593945c318aa9d9220a8b2ca,b317bcad5f5ca1ef233ec9324fd61299)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/baseline.go -> internal/model/finding  [023add20]
  integration strength: model         distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 5 same-shape edges (e.g. 023add20bf21c6007cb54b20b4de973a,1101eb26f63e6ca5402405104c4bac48,8afbbad051fd4b9e5d7916b078495f68,8bf233b1773be9a6613ef77094c131e1,9e3eebe06f46134755564db05a15cbd4)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/baseline.go -> internal/model/report  [3c44e399]
  integration strength: functional    distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: functional integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 2 same-shape edges (e.g. 3c44e3991724dedb2252f9b03a2d70fa,86666b35572dd027448b28bffd56ca62)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/distance_context.go -> internal/model/evidence  [384e5f46]
  integration strength: model         distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 10 same-shape edges (e.g. 384e5f46af347c23de423b7fd61c7310,413473b096e767fc422ba9a2afd75db0,438ec1eb886a7e9ca09937f4f463765f,4eca2b7d711e5fd7adab7c39338bb317,58607b45e00ebaffe292ff85039d0588,813f6882972abacc9fc3623cc1fd5195,a387e4b0ecbb56c4545d04cd8cd1e79b,a6731bd114fbe1c761f03f07ab0f40aa)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/doctor.go -> internal/toolrun  [07bb0946]
  integration strength: functional    distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: functional integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 29 same-shape edges (e.g. 07bb094642d4e9307f6dd14debdd31c1,0ab4b821189ca416e79ebd4b206efec8,0f119e240a390a5c3e7f78d343de8897,1ae64ab6f037bfec759dc62a0b9ed3d4,1d75b7ae0957f19ac294d1fbd26e8a9d,204f43b88478926dda7e96c50706eef6,297e7ac553b7790c16502f128922cd3c,2ade6dbafdaabdcdb26854a677e20a33)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/draft_metadata.go -> internal/initcfg  [09ea5d95]
  integration strength: model         distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 2 same-shape edges (e.g. 09ea5d95ec52268d4b39d9ffc93ab11c,5cfeae578d37e771c6910a5c61a18319)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/enrich.go -> internal/engine  [6c5f0852]
  integration strength: symmetric     distance: cross_module_same_owner         volatility: high
  score: 6/10 (medium) [book]
  why: balanced coupling: symmetric integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 4 same-shape edges (e.g. 6c5f08524e9135bb334dc9ae2ee37c4e,7f564ac56da87c57a9bb568b6e7af750,96635ffe4f92e42e9b433fb079f202fb,fa94b51f67a28f758634f247b99040f4)
```

- ... +77 more rollups (use `--format json`)

## Advisories (5)

- **bc/duplicated_knowledge** [medium] new — internal/extract/golang/cache.go → internal/engine/assemble.go: duplicated knowledge: cross-module code clones between fact-adapters and pipeline-engine with no import edge — symmetric functional cou...
- **syntax_api_size_ceiling** [medium] new — fact-adapters → fact-adapters: Module "fact-adapters" has 417 exported declarations, exceeding the limit of 320
- **map/uncovered_path** [] new: node "internal/application/report.go" is not covered by any module paths glob
- **map/uncovered_path** [] new: node "internal/application/analysis.go" is not covered by any module paths glob
- **map/uncovered_path** [] new: node "internal/application" is not covered by any module paths glob

## Supporting structural metrics (beyond Balanced Coupling)

Report-only. These metrics support Balanced Coupling reasoning but never gate.

- **cycle**: 0 import cycles — strong
- **coverage**: 100% coverage — strong
- **blast_radius**: 6 of 67 modules are change-impact hubs: .../model/graph (71%, 47 deps), .../model/evidence (56%, 37 deps), .../model/finding (44%, 29 deps), .../model/coupling (41%, 27 deps), .../model/report (38%, 25 deps)+1 more — info

## Distance confidence

- `code_structure`: always on (deterministic tree-distance baseline)
- `owner_source`: config
- `deploy_unit_source`: ok
- `owner_model`: single_owner_degenerate
- deploy-unit detector mapped modules: 3
- distance basis: code_structure=414, deploy_unit=7
- interpretation: same-owner is the lowest cross-module distance; this is a low socio-technical distance signal, not missing ownership; deploy_unit and declared external_systems evidence can still raise distance when configured/detected
- connected modules in coupling sample: 22
- distance rungs implemented: D=2, D=4, D=7, D=9, D=10; omitted/compressed: D=1, D=3, D=5, D=6, D=8
- code-structure boundary crossings: 2→414
- code-structure shared-ancestor depth: 0→414
- distance compression: D=3/D=5/D=6/D=8 remain compressed: current graph/config facts distinguish same module, same owner, different owner, deploy unit, and declared vendor seam, but not finer package/library distance without guessing.
- D=1 compressed: object/member-level distance is not available from module dependency edges
- D=3 compressed: current facts distinguish same module vs cross-module, but not object/package micro-distance
- D=5 compressed: package/library middle distance is not split without explicit stable package-boundary metadata
- D=6 compressed: intermediate ownership/library distance has no deterministic signal beyond owner and tree structure
- D=8 compressed: library-like seams remain compressed: undeclared libraries stay excluded, while declared external_systems score at D=10
- undeclared external/library edges excluded: 625
- clone-only duplicated knowledge: 1 scored, 0 advisory-only
- tail risk: worst balance 2/10; lower-decile balance 2/10; high-or-worse edges 48/421 (11%); critical 47; distributed-monolith 0
- clone-only tail: worst balance 6/10; high-or-worse 0/1 scored clone-only pairs

## Coverage

- scip: ok (1516 files)
- scip-symbols: ok (7321 files)
- go/packages: ok (185 files)
- dependency-cruiser: absent
- grimp: absent
- cargo: absent
- loc: ok (183 files)
- deploy-unit: ok (6 files)
- jscpd: ok (273 files)
- ast-grep: ok
- ast-grep/syntax: ok (322 files)
- cargo-modules: absent
