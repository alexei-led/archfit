# archfit — decision

- **Decision:** ACCEPTABLE WITH WATCH ITEMS
- **Gate:** PASS — 0 blocking
- **Warnings:** 83 advisory
- **Score:** 46 / 100 (mixed)

Acceptable with watch items. Monitor flagged areas.

## Recommendations

### Must fix
- none

### Should fix
- **bc/imbalanced_coupling** — balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (cheap to change; not a distributed monolith))

### Watch
- **bc/duplicated_knowledge** — duplicated knowledge: cross-module code clones between fact-adapters and pipeline-engine with no import edge — symmetric functional coupling; a change to the shared logic must be repeated in both modules. Extract the shared knowledge, or accept the pair with an approved label
- **bc/imbalanced_coupling** — balanced coupling: model integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **syntax_api_size_ceiling** — Module "fact-adapters" has 431 exported declarations, exceeding the limit of 320

## Why the score is low

- **coupling_balance** (46/100, mixed): critical-band coupling present; inspect the reported strength, distance, and volatility drivers — 372 scored internal cross-boundary edges; mean book balance 5.1/10 → value 46; strength distribution: contract=29, functional=139, model=177, symmetric=27; distance distribution: cross_module_same_owner=372; volatility distribution: high=115, low=63, medium=194; balance drivers: strength_distance=136, tie=74, volatility=162; critical drivers: strength_distance=58; top module pairs: archfit-cli -> assessment-repair=30, fact-adapters -> evidence-model=30, archfit-cli -> fact-adapters=25, assessment-repair -> evidence-model=23, archfit-cli -> policy-language=22; scored fraction: 100% (372 scored, 0 abstained, internal only)
  - _What moves it:_ Reduce high-fan-in functional edges across module boundaries or introduce stable contracts.
# archfit report

**Verdict:** warn (exit 2)
**Config hash:** `1fcfb6e83da4fa6428532e82ad7541e43019dbbad1b89e3e2f3ddfe8a594584b`

## Summary

- gate findings: 0
- warnings: 83
- waivers used: 0

## Metrics

- **encapsulation**: n/a — n/a (low confidence)
- **unbalanced_edge**: 0 new high-risk unbalanced edges — strong

## Structural facts (neutral evidence)

110 modules; top 5 per axis (full list in `--format json`):

- inbound module fan-in: internal/model/graph (45), internal/model/evidence (43), internal/view (35), internal/assessment/finding (26), internal/toolrun (26)
- outbound destinations: cmd/archfit (39), internal/engine (25), internal/engine_test (22), internal/extract/acquire (18), internal/initcfg_test (16)
- LOC: cmd/archfit (8405), internal/initcfg (4924), internal/engine (2953), internal/relationship/classify (1653), internal/extract/golang (1523)

## Syntax surface (neutral evidence)

2297 declaration(s) extracted by ast-grep (full list in `--format json`):

- annotation: 1
- enum: 1
- function: 1535
- interface: 18
- method: 249
- struct: 275
- type_alias: 37
- type_leak: 181
- exported (public API): 2115

Per module:

- (unscoped): 64
- analysis-application: 13
- archfit-cli: 334
- architecture-model: 16
- architecture-tests: 9
- assessment-repair: 308
- baseline-store: 21
- config-lifecycle: 233
- development-tools: 41
- evidence-model: 81
- fact-adapters: 477
- labels-store: 5
- llm-adapter: 30
- pipeline-contracts: 104
- pipeline-engine: 111
- policy-language: 120
- relationship-analysis: 149
- rendering: 95
- report-contract: 38
- scan-contract: 25
- stage-views: 23

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
- ... +2095 more exported declarations (use `--format json`)

## Connascence evidence (deterministic)

Report-only. Static facts only; semantic and dynamic categories without deterministic evidence stay unmeasured.

- edges with evidence: 1093
- abstained edges: 4
- total evidence facts: 4290
- strength inferred from connascence: 66 edges
- by kind: algorithm=952, meaning=601, name=1566, type=1171
- by source: go/types=2918, scip=1372
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

35 signal(s) across 20 candidate(s):

- **fact-adapters** → `github.com/bmatcuk/doublestar/**` [imports from classified_external_edges; action=external_systems]: 5 (e.g. internal/extract/golang/golang.go:10[imports], internal/extract/golang/members.go:11[imports], internal/extract/py/py.go:17[imports])
- **config-lifecycle** → `github.com/goccy/go-yaml/**` [imports from classified_external_edges; action=external_systems]: 4 (e.g. internal/initcfg/subdomains_draft.go:9[imports], internal/initcfg/value_draft.go:9[imports], internal/initcfg/yamledit_parse.go:11[imports])
- **assessment-repair** → `github.com/bmatcuk/doublestar/**` [imports from classified_external_edges; action=external_systems]: 3 (e.g. internal/assessment/rules/rules_dependency.go:10[imports], internal/assessment/staleness/staleness.go:14[imports], internal/assessment/status/status.go:8[imports])
- **llm-adapter** → `github.com/openai/openai-go/**` [imports from classified_external_edges; action=external_systems]: 3 (e.g. internal/llm/openai.go:8[imports], internal/llm/openai.go:9[imports], internal/llm/openai.go:10[imports])
- **fact-adapters** → `golang.org/x/mod/**` [imports from classified_external_edges; action=external_systems]: 2 (e.g. internal/extract/golang/cache.go:16[imports], internal/extract/golang/members.go:12[imports])
- **llm-adapter** → `github.com/anthropics/anthropic-sdk-go/**` [imports from classified_external_edges; action=external_systems]: 2 (e.g. internal/llm/anthropic.go:9[imports], internal/llm/anthropic.go:10[imports])
- **fact-adapters** → `fact-adapters` [dynamic_import from dynamic_connascence_signals; action=deploy_unit]: 2 (e.g. internal/extract/py/grimp_helper.py:177[lazy_import], internal/extract/scip/scip_reader.py:63[lazy_import])
- **fact-adapters** → `fact-adapters` [lazy_import from dynamic_imports; action=deploy_unit]: 2 (e.g. internal/extract/py/grimp_helper.py:177[lazy_import], internal/extract/scip/scip_reader.py:63[lazy_import])
- **archfit-cli** → `github.com/alecthomas/kong/**` [imports from classified_external_edges; action=external_systems]: 1 (e.g. cmd/archfit/main.go:12[imports])
- **archfit-cli** → `github.com/bmatcuk/doublestar/**` [imports from classified_external_edges; action=external_systems]: 1 (e.g. cmd/archfit/draft_metadata.go:9[imports])
- ... +10 more candidates (use `--format json`)

## Volatility corroboration (report-only)

Report-only. Source-control touch frequency is supporting evidence for Ch9 volatility judgments and never changes score or gate verdicts.

- source: git_history
- status: ok
- recent-history window: 500 commits
- commits scanned: 500
- modules touched: 19
- caveat: Supporting evidence only. Git history can reflect both essential and accidental volatility and never changes scoring or gate verdicts.

Top touched modules:

- **archfit-cli**: 163 commit(s) [declared volatility=medium]
- **pipeline-engine**: 109 commit(s) [declared volatility=medium]
- **fact-adapters**: 102 commit(s) [declared volatility=medium]
- **policy-language**: 73 commit(s) [declared volatility=high]
- **config-lifecycle**: 72 commit(s) [declared volatility=medium]

## Advisory tasks (49)

Report-only rollups from grouped advisories; these do not affect verdict or gate status.
- **bc/imbalanced_coupling** [`2e1590e3`] Review 2 same-shape Balanced-Coupling advisory edges from analysis-application to assessment-repair and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 2
  - group members: 2e1590e3580d6096c9ef526ef72d1595, 6b7786f551449325fafa7e87281e25f9
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: internal/application/report.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
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
- **bc/imbalanced_coupling** [`00e06a21`] Review 23 same-shape Balanced-Coupling advisory edges from archfit-cli to assessment-repair and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 23
  - group members: 00e06a2185efc95931f6a281ffd4f667, 21c16936229b739455c9ebdf57e9fe70, 21f36446fc5dfd15c3fd3922ee9de762, 2cf86ec76f5fb9af79ce9f72fdfa4d9e, 2fc4b0a0d9aa93e9e860c407bc7942da, 33c4a1a29f030c88964656ae87ccd545, 43478e904b0af9f35f2d5ac695427271, 460fe751331eabafc1f1a72d055f53da
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
- **bc/imbalanced_coupling** [`3ba9d033`] Review 3 same-shape Balanced-Coupling advisory edges from archfit-cli to evidence-model and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 3
  - group members: 3ba9d033b58d56d4b89b1f09d311ecc5, 4bd04cc81d5e64503ec8e462ef1005fc, 4d4bcf55e22e3424ca3f048d4932a6e9
  - score: 5/10
  - top files: cmd/archfit/enrich.go, cmd/archfit/enrich_abstained.go, cmd/archfit/update.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`413473b0`] Review 9 same-shape Balanced-Coupling advisory edges from archfit-cli to evidence-model and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 9
  - group members: 413473b096e767fc422ba9a2afd75db0, 438ec1eb886a7e9ca09937f4f463765f, 4eca2b7d711e5fd7adab7c39338bb317, 58607b45e00ebaffe292ff85039d0588, 813f6882972abacc9fc3623cc1fd5195, a387e4b0ecbb56c4545d04cd8cd1e79b, a6731bd114fbe1c761f03f07ab0f40aa, ae93d5d28a7da9387c7e51686bd59ee5
  - score: 5/10
  - top files: cmd/archfit/distance_context.go, cmd/archfit/git_finding_delta.go, cmd/archfit/llmreview.go, cmd/archfit/pipeline_config.go, cmd/archfit/pipeline_coverage.go, cmd/archfit/pipeline_warnings.go, cmd/archfit/update.go, cmd/archfit/volatility_corroboration.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`3f597ecb`] Review 2 same-shape Balanced-Coupling advisory edges from archfit-cli to fact-adapters and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 2
  - group members: 3f597ecb1a08b4e1b2277101ef03ce68, 4392dba45e4f134ae5350609d5ff2b60
  - score: 5/10
  - top files: cmd/archfit/pipeline_run.go, cmd/archfit/volatility_corroboration.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=contract, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`1ae64ab6`] Review 19 same-shape Balanced-Coupling advisory edges from archfit-cli to fact-adapters and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 19
  - group members: 1ae64ab6f037bfec759dc62a0b9ed3d4, 4945284da72839693fb0f7e0b2a6a43c, 5b49cc4aa21e26b775fbe8a434a242b2, 66da4a692d600efd7972aa7cb76817e2, 6ca58a14888e3f360d2946f60c6c7c25, 73763dbeaf885dbf35c5d319163910eb, 78cc3e1cc48b10e49f42d330b795bf40, 8c749806d46ce69939627a666ea5d2bd
  - score: 5/10
  - top files: cmd/archfit/doctor.go, cmd/archfit/git_finding_delta.go, cmd/archfit/main.go, cmd/archfit/pipeline_config.go, cmd/archfit/pipeline_coverage.go, cmd/archfit/pipeline_run.go, cmd/archfit/update.go, cmd/archfit/volatility_corroboration.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`2fcaec75`] Review 4 same-shape Balanced-Coupling advisory edges from archfit-cli to fact-adapters and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 4
  - group members: 2fcaec75674031bb07d3aba9a629703c, 4b745847706c5003fd20fbc92b4fdd0d, 8ae6c5b610a92a7136eeb8692fffdea6, e19785bd2a4e8ce6d810dc43b364814b
  - score: 5/10
  - top files: cmd/archfit/adapter_config.go, cmd/archfit/consts.go, cmd/archfit/pipeline_config.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`17bd7f32`] Review 3 same-shape Balanced-Coupling advisory edges from archfit-cli to labels-store and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 3
  - group members: 17bd7f323d0f2d4d1ba083270bbd2351, 199dd723d3d23ffb37a5ab3e4c12e184, ce9b29f04f06eca7c944737106be16b2
  - score: 5/10
  - top files: cmd/archfit/enrich.go, cmd/archfit/enrich_abstained.go, cmd/archfit/pipeline_run.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=high
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
- **bc/imbalanced_coupling** [`6c5f0852`] Review 5 same-shape Balanced-Coupling advisory edges from archfit-cli to pipeline-engine and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 5
  - group members: 6c5f08524e9135bb334dc9ae2ee37c4e, 6f210f6bf9656ce3733207c48295f8be, 7f564ac56da87c57a9bb568b6e7af750, 96635ffe4f92e42e9b433fb079f202fb, fa94b51f67a28f758634f247b99040f4
  - score: 6/10
  - top files: cmd/archfit/enrich.go, cmd/archfit/enrich_abstained.go, cmd/archfit/pipeline_config.go, cmd/archfit/pipeline_run.go, cmd/archfit/update.go, internal/engine/assemble.go, internal/engine/labels.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=symmetric, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`0253289b`] Review 17 same-shape Balanced-Coupling advisory edges from archfit-cli to policy-language and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 17
  - group members: 0253289bc1623d1c847fbb4b14db01ca, 1cc457d122eda82495745b71a8623a10, 2ab6dd6c6c98eb5ad56e21a522d3d938, 4fb003f14c3faea28b169e0c85e9a259, 58bc8aba5d9ecb08efc4e074f63ee8c5, 60348cc93b9075d49384cef144d1e6b2, 90b6c329a2a1e1ef28f8e49d40037cb4, a993f8ba00e7797f62983e493ae1c1db
  - score: 5/10
  - top files: cmd/archfit/adapter_config.go, cmd/archfit/analyze.go, cmd/archfit/config_compare.go, cmd/archfit/doctor.go, cmd/archfit/enrich.go, cmd/archfit/enrich_abstained.go, cmd/archfit/enrich_values.go, cmd/archfit/explain.go
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
- **bc/imbalanced_coupling** [`00f1235c`] Review 3 same-shape Balanced-Coupling advisory edges from archfit-cli to relationship-analysis and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 3
  - group members: 00f1235ca2deb4c560c18d373e2dd3eb, 04fabc7332a34430a85dc1f177384fc1, 8c02a10341f298de380896cb8bbd3c5a
  - score: 5/10
  - top files: cmd/archfit/enrich.go, cmd/archfit/enrich_abstained.go, cmd/archfit/update.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`0034c41e`] Review 5 same-shape Balanced-Coupling advisory edges from archfit-cli to relationship-analysis and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 5
  - group members: 0034c41e84d2e32a73b74863de478116, 501bc702d2a38451a36a508d84adb2fc, 95a0f5a3ed8def39e781a0a922546b31, d3889ace1750ac76d95ce1dc31ce7507, f3439e89d85718803e89a0587530bf31
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: cmd/archfit/enrich.go, cmd/archfit/enrich_abstained.go, cmd/archfit/explain.go, cmd/archfit/llmreview.go, cmd/archfit/pipeline_config.go
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
- **bc/imbalanced_coupling** [`0ebd278b`] Review 2 same-shape Balanced-Coupling advisory edges from assessment-repair to architecture-model and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 2
  - group members: 0ebd278b2ffbc1a090834cb0f2591ccc, c9e913582d246d7116e88433d77b11be
  - score: 5/10
  - top files: internal/assessment/rules/rules_api.go, internal/assessment/rules/rules_dependency.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`32aa0a8d`] Review 4 same-shape Balanced-Coupling advisory edges from assessment-repair to evidence-model and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 4
  - group members: 32aa0a8d9b525561b895230003bfcd8f, 52ab4e7c9c9797741ec665fe3ca63d65, a54099ba4cad069f2b876d062dd363c6, f160d87c267c84ad4ced73b776523aee
  - score: 5/10
  - top files: internal/assessment/rules/rules.go, internal/assessment/signals/signal.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=contract, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`0f3c8989`] Review 10 same-shape Balanced-Coupling advisory edges from assessment-repair to evidence-model and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 10
  - group members: 0f3c898957c6317f89338e039e67944b, 3628c074038f697596fa7ef965b4c8c0, 7c7f892f9c592f5bc79d2941482f1a74, 7d3c522c97424cbd7ea6fc2fe9ca1ff3, 8587d6bf21bd4cb53b15541e6dda441e, adebe2861d93b7ae7d04488fdf87b5c1, ae8256ae408af85dda74385c6a0adfd2, bafe71a1bdfcc8525876995611dda019
  - score: 5/10
  - top files: internal/assessment/agenttask/agenttask.go, internal/assessment/finding/finding.go, internal/assessment/metrics/boundary/cycle.go, internal/assessment/metrics/boundary/encapsulation.go, internal/assessment/metrics/boundary/unbalanced_edge.go, internal/assessment/metrics/internal/modgraph/modgraph.go, internal/assessment/metrics/metricstest/helpers.go, internal/assessment/rules/rules_api.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`024a0546`] Review 9 same-shape Balanced-Coupling advisory edges from assessment-repair to evidence-model and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 9
  - group members: 024a054601f108bb2f65bdaa51b20789, 320ae359fba984604d56d4a447acfbfe, 34295f21744e30f16cbc7367f4ccc643, 648d97e461b0a5af85490566837fcf26, a77354659d123f26f9337af753d719ef, aaf1d01e7cf774dd01a437340d8cb9e5, bc3ae64943354c067ac241337aaa748b, e4b74383ba424ee15c1522b0a92740b0
  - score: 5/10
  - top files: internal/assessment/agenttask/agenttask.go, internal/assessment/decision/config_compare.go, internal/assessment/metrics/boundary/coverage.go, internal/assessment/result/result.go, internal/assessment/rules/rules.go, internal/assessment/score/score.go, internal/assessment/signals/signal.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`2cd874e9`] Review 3 same-shape Balanced-Coupling advisory edges from assessment-repair to relationship-analysis and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 3
  - group members: 2cd874e9ab70323f8cf5c2a35543d25a, 98baa3d9333104c71c3508968c9ed4db, c2bc8bdee020064ad875f9bbe89df00b
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: internal/assessment/metrics/boundary/encapsulation.go, internal/assessment/score/score_boundary_coupling.go, internal/assessment/signals/signal.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`

_…and 24 more advisory tasks (see --json for the full list)._

## Balanced Coupling advisories (79 rollups, 308 edges)

Same-shape edges between a module pair are grouped into one rollup.
Integration strength × distance × volatility lint messages.
Severity: `none` · `low` · `medium` · `high` · `critical`.

```
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/archfit/analyze.go -> internal/assessment/result  [00e06a21]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 23 same-shape edges (e.g. 00e06a2185efc95931f6a281ffd4f667,21c16936229b739455c9ebdf57e9fe70,21f36446fc5dfd15c3fd3922ee9de762,2cf86ec76f5fb9af79ce9f72fdfa4d9e,2fc4b0a0d9aa93e9e860c407bc7942da,33c4a1a29f030c88964656ae87ccd545,43478e904b0af9f35f2d5ac695427271,460fe751331eabafc1f1a72d055f53da)
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
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/archfit/distance_context.go -> internal/config  [029b9c05]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 5 same-shape edges (e.g. 029b9c055ed2b0724c019b28844d9a66,05fe7e65f0032d88ec6f205d6ba85bed,07d394a78707ad0aa666083f94fef636,7300bc30e7a2eb6066153e29571b8221,bfcaec1b44a0e71292d68b55e69c71a2)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/archfit/enrich.go -> internal/relationship/coupling  [0034c41e]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 5 same-shape edges (e.g. 0034c41e84d2e32a73b74863de478116,501bc702d2a38451a36a508d84adb2fc,95a0f5a3ed8def39e781a0a922546b31,d3889ace1750ac76d95ce1dc31ce7507,f3439e89d85718803e89a0587530bf31)
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
ARCHFIT[BC-UNBALANCED CRITICAL] internal/application/report.go -> internal/assessment/finding  [2e1590e3]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 2 same-shape edges (e.g. 2e1590e3580d6096c9ef526ef72d1595,6b7786f551449325fafa7e87281e25f9)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/assessment/metrics/boundary/encapsulation.go -> internal/relationship/coupling  [2cd874e9]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 3 same-shape edges (e.g. 2cd874e9ab70323f8cf5c2a35543d25a,98baa3d9333104c71c3508968c9ed4db,c2bc8bdee020064ad875f9bbe89df00b)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/baseline/baseline.go -> internal/assessment/status  [e0052281]
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
ARCHFIT[BC-UNBALANCED CRITICAL] internal/engine/advisories.go -> internal/assessment/finding  [0c46da3b]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 7 same-shape edges (e.g. 0c46da3b624e5eabd602b1358b2349de,308e4f071d061940e1d1b1b5e4d1d331,368f70e14c48e70552607396341001d1,40d3a5a9e8a7e847cf9493b7ccaa0698,5fadb2c3bffc3e7dd43fb2f2013775f1,bc8d85d392423a641795f03754f155c0,d2e58d4a349877489336e86efb1ef07d)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/engine/engine.go -> internal/relationship/coupling  [1ad1d365]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 2 same-shape edges (e.g. 1ad1d365db9f5a0cdc85d2ec1341d510,231d34ca9e52302f4ff3d6004035ce35)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/extract/py/py.go -> internal/relationship/coupling  [3ccff7c7]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 2 same-shape edges (e.g. 3ccff7c727bf3881a6cf53d64570ef16,a35b693b468e718b11f725e00d91b286)
```

```
ARCHFIT[BC-UNBALANCED HIGH] internal/engine/engine.go -> internal/relationship/labels  [bf0be5d2]
  integration strength: contract      distance: cross_module_same_owner         volatility: high
  score: 4/10 (high) [book]
  why: balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → high severity (contract coupling to a volatile target at low distance → cascading chang...
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/adapter_config.go -> internal/config  [0253289b]
  integration strength: functional    distance: cross_module_same_owner         volatility: high
  score: 5/10 (medium) [book]
  why: balanced coupling: functional integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 17 same-shape edges (e.g. 0253289bc1623d1c847fbb4b14db01ca,1cc457d122eda82495745b71a8623a10,2ab6dd6c6c98eb5ad56e21a522d3d938,4fb003f14c3faea28b169e0c85e9a259,58bc8aba5d9ecb08efc4e074f63ee8c5,60348cc93b9075d49384cef144d1e6b2,90b6c329a2a1e1ef28f8e49d40037cb4,a993f8ba00e7797f62983e493ae1c1db)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/adapter_config.go -> internal/extract/acquire  [2fcaec75]
  integration strength: model         distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 4 same-shape edges (e.g. 2fcaec75674031bb07d3aba9a629703c,4b745847706c5003fd20fbc92b4fdd0d,8ae6c5b610a92a7136eeb8692fffdea6,e19785bd2a4e8ce6d810dc43b364814b)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/analyze.go -> internal/application  [caaf0379]
  integration strength: functional    distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: functional integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/analyze.go -> internal/assessment/decision  [0780c16c]
  integration strength: functional    distance: cross_module_same_owner         volatility: high
  score: 5/10 (medium) [book]
  why: balanced coupling: functional integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 7 same-shape edges (e.g. 0780c16cc0d72072e026e84a116779d5,45af9af8b0b826a4f5ee16c83ee390aa,8d0711a56383e0409bd8b3a9df5fdd02,9ff044de9d3757b9b697fb0d3e182ff7,b28dc8588f6f6c167b1d2bc99c8549a1,d59ae2d1ff3b3fdfd824c782d0827ab6,edebca9952b56020f60288160ce29322)
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
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/baseline.go -> internal/model/report  [3c44e399]
  integration strength: functional    distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: functional integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/distance_context.go -> internal/model/evidence  [413473b0]
  integration strength: model         distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 9 same-shape edges (e.g. 413473b096e767fc422ba9a2afd75db0,438ec1eb886a7e9ca09937f4f463765f,4eca2b7d711e5fd7adab7c39338bb317,58607b45e00ebaffe292ff85039d0588,813f6882972abacc9fc3623cc1fd5195,a387e4b0ecbb56c4545d04cd8cd1e79b,a6731bd114fbe1c761f03f07ab0f40aa,ae93d5d28a7da9387c7e51686bd59ee5)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/doctor.go -> internal/extract/registry  [1ae64ab6]
  integration strength: functional    distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: functional integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 19 same-shape edges (e.g. 1ae64ab6f037bfec759dc62a0b9ed3d4,4945284da72839693fb0f7e0b2a6a43c,5b49cc4aa21e26b775fbe8a434a242b2,66da4a692d600efd7972aa7cb76817e2,6ca58a14888e3f360d2946f60c6c7c25,73763dbeaf885dbf35c5d319163910eb,78cc3e1cc48b10e49f42d330b795bf40,8c749806d46ce69939627a666ea5d2bd)
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
  rollup: 5 same-shape edges (e.g. 6c5f08524e9135bb334dc9ae2ee37c4e,6f210f6bf9656ce3733207c48295f8be,7f564ac56da87c57a9bb568b6e7af750,96635ffe4f92e42e9b433fb079f202fb,fa94b51f67a28f758634f247b99040f4)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/enrich.go -> internal/labels/labelsio  [17bd7f32]
  integration strength: functional    distance: cross_module_same_owner         volatility: high
  score: 5/10 (medium) [book]
  why: balanced coupling: functional integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 3 same-shape edges (e.g. 17bd7f323d0f2d4d1ba083270bbd2351,199dd723d3d23ffb37a5ab3e4c12e184,ce9b29f04f06eca7c944737106be16b2)
```

- ... +54 more rollups (use `--format json`)

## Advisories (4)

- **bc/duplicated_knowledge** [medium] new — internal/extract/golang/cache.go → internal/engine/assemble.go: duplicated knowledge: cross-module code clones between fact-adapters and pipeline-engine with no import edge — symmetric functional cou...
- **syntax_api_size_ceiling** [medium] new — fact-adapters → fact-adapters: Module "fact-adapters" has 431 exported declarations, exceeding the limit of 320
- **map/uncovered_path** [] new: node "internal/testutil/report/convert.go" is not covered by any module paths glob
- **map/uncovered_path** [] new: node "internal/testutil/report" is not covered by any module paths glob

## Supporting structural metrics (beyond Balanced Coupling)

Report-only. These metrics support Balanced Coupling reasoning but never gate.

- **cycle**: 0 import cycles — strong
- **coverage**: 100% coverage — strong
- **blast_radius**: 5 of 71 modules are change-impact hubs: .../model/graph (60%, 42 deps), .../model/evidence (57%, 40 deps), .../model/module (34%, 24 deps), .../model/report (30%, 21 deps), internal/view (30%, 21 deps) — info

## Distance confidence

- `code_structure`: always on (deterministic tree-distance baseline)
- `owner_source`: config
- `deploy_unit_source`: ok
- `owner_model`: single_owner_degenerate
- deploy-unit detector mapped modules: 3
- distance basis: code_structure=365, deploy_unit=7
- interpretation: same-owner is the lowest cross-module distance; this is a low socio-technical distance signal, not missing ownership; deploy_unit and declared external_systems evidence can still raise distance when configured/detected
- connected modules in coupling sample: 19
- distance rungs implemented: D=2, D=4, D=7, D=9, D=10; omitted/compressed: D=1, D=3, D=5, D=6, D=8
- code-structure boundary crossings: 2→365
- code-structure shared-ancestor depth: 0→365
- distance compression: D=3/D=5/D=6/D=8 remain compressed: current graph/config facts distinguish same module, same owner, different owner, deploy unit, and declared vendor seam, but not finer package/library distance without guessing.
- D=1 compressed: object/member-level distance is not available from module dependency edges
- D=3 compressed: current facts distinguish same module vs cross-module, but not object/package micro-distance
- D=5 compressed: package/library middle distance is not split without explicit stable package-boundary metadata
- D=6 compressed: intermediate ownership/library distance has no deterministic signal beyond owner and tree structure
- D=8 compressed: library-like seams remain compressed: undeclared libraries stay excluded, while declared external_systems score at D=10
- undeclared external/library edges excluded: 627
- clone-only duplicated knowledge: 1 scored, 0 advisory-only
- tail risk: worst balance 2/10; lower-decile balance 2/10; high-or-worse edges 59/372 (15%); critical 58; distributed-monolith 0
- clone-only tail: worst balance 6/10; high-or-worse 0/1 scored clone-only pairs

## Coverage

- scip: ok (1531 files)
- scip-symbols: ok (7536 files)
- go/packages: ok (190 files)
- dependency-cruiser: absent
- grimp: absent
- cargo: absent
- loc: ok (188 files)
- deploy-unit: ok (6 files)
- jscpd: ok (278 files)
- ast-grep: ok
- ast-grep/syntax: ok (325 files)
- cargo-modules: absent
