# archfit — decision

- **Decision:** NEEDS ATTENTION
- **Gate:** PASS — 0 blocking
- **Warnings:** 68 advisory
- **Score:** 40 / 100 (poor)

Significant structural risks detected. Review required.

## Recommendations

### Must fix
- none

### Should fix
- **bc/imbalanced_coupling** — balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → high severity (contract coupling to a volatile target at low distance → cascading changes contained to one owner)

### Watch
- **bc/duplicated_knowledge** — duplicated knowledge: cross-module code clones between fact-adapters and pipeline-engine with no import edge — symmetric functional coupling; a change to the shared logic must be repeated in both modules. Extract the shared knowledge, or accept the pair with an approved label
- **bc/imbalanced_coupling** — balanced coupling: model integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **syntax_api_size_ceiling** — Module "evaluation-core" has 329 exported declarations, exceeding the limit of 320

## Why the score is low

- **coupling_balance** (40/100, poor): critical-band coupling at low distance — local high-strength/high-volatility coupling (cheap cascade), not a distributed monolith — 400 scored internal cross-boundary edges; mean book balance 4.6/10 → value 40; scored fraction: 100% (400 scored, 0 abstained, internal only); critical-band edges: 123 (0 distributed-monolith: critical at high distance)
  - _What moves it:_ Reduce high-fan-in functional edges across module boundaries or introduce stable contracts.
# archfit report

**Verdict:** warn (exit 2)
**Config hash:** `b4b679e4b675843cd7fd97f5bd3372e57c129d8ee9f7ae094943a76c3cd40d51`

## Summary

- gate findings: 0
- warnings: 68
- waivers used: 0

## Metrics

- **encapsulation**: n/a — n/a (low confidence)
- **unbalanced_edge**: 0 new high-risk unbalanced edges — strong

## Structural facts (neutral evidence)

102 modules; top 5 per axis (full list in `--format json`):

- inbound module fan-in: internal/model/diagnostic (52), internal/model/graph (44), internal/view (33), internal/model/finding (26), internal/toolrun (24)
- outbound destinations: cmd/archfit (44), internal/engine (22), internal/engine_test (20), internal/initcfg_test (16), internal/decision_test (9)
- LOC: cmd/archfit (8740), internal/initcfg (4924), internal/engine (2850), internal/classify (1657), internal/extract/golang (1523)

## Syntax surface (neutral evidence)

2213 declaration(s) extracted by ast-grep (full list in `--format json`):

- annotation: 1
- enum: 1
- function: 1506
- interface: 17
- method: 245
- struct: 252
- type_alias: 33
- type_leak: 158
- exported (public API): 2054

Per module:

- (unscoped): 61
- archfit-cli: 334
- architecture-tests: 5
- baseline-store: 21
- config-lifecycle: 233
- development-tools: 41
- evaluation-core: 342
- evaluation-model: 170
- evidence-model: 56
- fact-adapters: 452
- labels-policy: 13
- labels-store: 5
- llm-adapter: 30
- pipeline-contracts: 104
- pipeline-engine: 108
- policy-language: 119
- rendering: 95
- stage-views: 24

### Public API


`cmd/archfit/analyze.go` [archfit-cli]:
- `AnalyzeCmd` (struct)
- `Help` (method)
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
- `TestRun_Analyze_RejectsGateFlag` (function)
- ... +2034 more exported declarations (use `--format json`)

## Connascence evidence (deterministic)

Report-only. Static facts only; semantic and dynamic categories without deterministic evidence stay unmeasured.

- edges with evidence: 1058
- abstained edges: 4
- total evidence facts: 4105
- strength inferred from connascence: 73 edges
- by kind: algorithm=938, meaning=576, name=1503, type=1088
- by source: go/types=2817, scip=1288
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

35 signal(s) across 19 candidate(s):

- **fact-adapters** → `github.com/bmatcuk/doublestar/**` [imports from classified_external_edges; action=external_systems]: 5 (e.g. internal/extract/golang/golang.go:10[imports], internal/extract/golang/members.go:11[imports], internal/extract/py/py.go:17[imports])
- **config-lifecycle** → `github.com/goccy/go-yaml/**` [imports from classified_external_edges; action=external_systems]: 4 (e.g. internal/initcfg/subdomains_draft.go:9[imports], internal/initcfg/value_draft.go:9[imports], internal/initcfg/yamledit_parse.go:11[imports])
- **evaluation-core** → `github.com/bmatcuk/doublestar/**` [imports from classified_external_edges; action=external_systems]: 4 (e.g. internal/classify/classify.go:10[imports], internal/rules/rules_dependency.go:10[imports], internal/staleness/staleness.go:14[imports])
- **llm-adapter** → `github.com/openai/openai-go/**` [imports from classified_external_edges; action=external_systems]: 3 (e.g. internal/llm/openai.go:8[imports], internal/llm/openai.go:9[imports], internal/llm/openai.go:10[imports])
- **fact-adapters** → `golang.org/x/mod/**` [imports from classified_external_edges; action=external_systems]: 2 (e.g. internal/extract/golang/cache.go:16[imports], internal/extract/golang/members.go:12[imports])
- **llm-adapter** → `github.com/anthropics/anthropic-sdk-go/**` [imports from classified_external_edges; action=external_systems]: 2 (e.g. internal/llm/anthropic.go:9[imports], internal/llm/anthropic.go:10[imports])
- **fact-adapters** → `fact-adapters` [dynamic_import from dynamic_connascence_signals; action=deploy_unit]: 2 (e.g. internal/extract/py/grimp_helper.py:177[lazy_import], internal/extract/scip/scip_reader.py:63[lazy_import])
- **fact-adapters** → `fact-adapters` [lazy_import from dynamic_imports; action=deploy_unit]: 2 (e.g. internal/extract/py/grimp_helper.py:177[lazy_import], internal/extract/scip/scip_reader.py:63[lazy_import])
- **archfit-cli** → `github.com/alecthomas/kong/**` [imports from classified_external_edges; action=external_systems]: 1 (e.g. cmd/archfit/main.go:12[imports])
- **archfit-cli** → `github.com/bmatcuk/doublestar/**` [imports from classified_external_edges; action=external_systems]: 1 (e.g. cmd/archfit/draft_metadata.go:9[imports])
- ... +9 more candidates (use `--format json`)

## Volatility corroboration (report-only)

Report-only. Source-control touch frequency is supporting evidence for Ch9 volatility judgments and never changes score or gate verdicts.

- source: git_history
- status: ok
- recent-history window: 500 commits
- commits scanned: 500
- modules touched: 16
- caveat: Supporting evidence only. Git history can reflect both essential and accidental volatility and never changes scoring or gate verdicts.

Top touched modules:

- **archfit-cli**: 162 commit(s) [declared volatility=medium]
- **evaluation-core**: 149 commit(s) [declared volatility=high]
- **pipeline-engine**: 108 commit(s) [declared volatility=medium]
- **fact-adapters**: 101 commit(s) [declared volatility=medium]
- **evaluation-model**: 82 commit(s) [declared volatility=high]

## Advisory tasks (44)

Report-only rollups from grouped advisories; these do not affect verdict or gate status.
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
- **bc/imbalanced_coupling** [`cd1a78c7`] Review 2 same-shape Balanced-Coupling advisory edges from archfit-cli to evaluation-core and reduce the coupling risk without changing gate policy.
  - severity: high; status: new; group_count: 2
  - group members: cd1a78c7b466486889d13993285aa57e, f8ff62aa48ab68d9e361493a3617ca37
  - score: 4/10
  - top files: cmd/archfit/config_compare.go, cmd/archfit/worktree.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=contract, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`11d815cc`] Review 10 same-shape Balanced-Coupling advisory edges from archfit-cli to evaluation-core and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 10
  - group members: 11d815cc391fbb3e4c6a6898eee51cd7, 5785eb4b39bf46ade814ce4f614006cb, 6497d4b5d60e5d066ce042ff08519fcb, 693accf04efccec0be544b0e55ac5637, 8d3baeecbad359e0aec66e7cafd203fe, a44e7ad048371eb1a19d72f56767e3b4, a6f062f5dc3caa479ad03202d557b299, c5aedc56bf76d77834a70caa5c178e48
  - score: 5/10
  - top files: cmd/archfit/analyze.go, cmd/archfit/config_compare.go, cmd/archfit/git_finding_delta.go, cmd/archfit/pipeline_config.go, cmd/archfit/pipeline_run.go, cmd/archfit/update.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`ca479745`] Review 2 same-shape Balanced-Coupling advisory edges from archfit-cli to evaluation-core and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 2
  - group members: ca479745d83831953a1a94a9701e3606, f85736366f2ee73e90e97832b555b6d5
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: cmd/archfit/baseline.go, cmd/archfit/llmreview.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`01b0db59`] Review 5 same-shape Balanced-Coupling advisory edges from archfit-cli to evaluation-model and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 5
  - group members: 01b0db59b3d599ee59003a372499ce46, 4b2864a9c1285d45af622ac77680ffef, 6598b0dcac0376215ec99a950cd84c32, 6ba848bfe4df1afe3143a49cc5d0f3c2, ffd9ffd66a94686756c9b123f4756158
  - score: 5/10
  - top files: cmd/archfit/enrich.go, cmd/archfit/enrich_abstained.go, cmd/archfit/pipeline_run.go, cmd/archfit/update.go, cmd/archfit/volatility_corroboration.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`023add20`] Review 30 same-shape Balanced-Coupling advisory edges from archfit-cli to evaluation-model and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 30
  - group members: 023add20bf21c6007cb54b20b4de973a, 07b9a1eb1b66a64fb3b072f6ede30bbe, 0b2dbc0f272690e8c7a570abeccba563, 0f6827872c9c6855fdfe736670d7371d, 1101eb26f63e6ca5402405104c4bac48, 13a9c9667d34ede7c883fe65e7544000, 1aa41e85b46a513fc461a16873e16743, 23ba33f54622e001343e75d13c1fe970
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: cmd/archfit/analyze.go, cmd/archfit/baseline.go, cmd/archfit/config_compare.go, cmd/archfit/distance_context.go, cmd/archfit/enrich.go, cmd/archfit/enrich_abstained.go, cmd/archfit/enrich_values.go, cmd/archfit/explain.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
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
- **bc/imbalanced_coupling** [`3f597ecb`] Review 3 same-shape Balanced-Coupling advisory edges from archfit-cli to fact-adapters and reduce the coupling risk without changing gate policy.
  - severity: high; status: new; group_count: 3
  - group members: 3f597ecb1a08b4e1b2277101ef03ce68, 4392dba45e4f134ae5350609d5ff2b60, 49c86332aa2eb6e1adabd18aec1fd613
  - score: 4/10
  - top files: cmd/archfit/pipeline_run.go, cmd/archfit/registry.go, cmd/archfit/volatility_corroboration.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=contract, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`07bb0946`] Review 29 same-shape Balanced-Coupling advisory edges from archfit-cli to fact-adapters and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 29
  - group members: 07bb094642d4e9307f6dd14debdd31c1, 0ab4b821189ca416e79ebd4b206efec8, 0f119e240a390a5c3e7f78d343de8897, 1ae64ab6f037bfec759dc62a0b9ed3d4, 1d75b7ae0957f19ac294d1fbd26e8a9d, 204f43b88478926dda7e96c50706eef6, 297e7ac553b7790c16502f128922cd3c, 2ade6dbafdaabdcdb26854a677e20a33
  - score: 5/10
  - top files: cmd/archfit/doctor.go, cmd/archfit/main.go, cmd/archfit/pipeline_coverage.go, cmd/archfit/pipeline_run.go, cmd/archfit/registry.go, cmd/archfit/update.go, cmd/archfit/volatility_corroboration.go, cmd/archfit/worktree.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`460f5c07`] Review 2 same-shape Balanced-Coupling advisory edges from archfit-cli to fact-adapters and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 2
  - group members: 460f5c073149fa15d3d66549c22e0b37, 8ae6c5b610a92a7136eeb8692fffdea6
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: cmd/archfit/pipeline_config.go, cmd/archfit/registry.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
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
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`454623ac`] Review 3 same-shape Balanced-Coupling advisory edges from baseline-store to evaluation-model and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 3
  - group members: 454623ac5dcf7a709861bba1c2f1f8ce, 6efd59da1be96f4d3f3fd3eb57fc3603, f971a7602d142896a56ddb2fc5e18675
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: internal/baseline/baseline.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`51ad2e4f`] Review 2 same-shape Balanced-Coupling advisory edges from config-lifecycle to fact-adapters and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 2
  - group members: 51ad2e4f376a391f948ef1a33b7a6e11, 93d12df05a4547c1977a6dc0844c7bb6
  - score: 6/10
  - top files: internal/extract/scip/scip_strength.go, internal/initcfg/discover_go.go, internal/initcfg/discover_py.go, internal/initcfg/discover_rust.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=symmetric, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`a967c418`] Review 2 same-shape Balanced-Coupling advisory edges from development-tools to evaluation-model and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 2
  - group members: a967c418163b644b40b995788d7bd4cc, b8bc44dd75d2b31d4311672712d0520f
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: cmd/calibrate/main.go, scripts/eval/coverage/main.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`1a90396f`] Review 3 same-shape Balanced-Coupling advisory edges from evaluation-core to evaluation-model and reduce the coupling risk without changing gate policy.
  - severity: high; status: new; group_count: 3
  - group members: 1a90396f91f8c74db334cb80b84fac6f, 2b57e12ee0f68eee1fb68cbb24723219, 704b1c3fc2f631e0e3b4f0c2c0ace876
  - score: 4/10
  - top files: internal/classify/volatility_provenance.go, internal/metrics/metrics.go, internal/rules/rules.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=contract, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`00ffec20`] Review 9 same-shape Balanced-Coupling advisory edges from evaluation-core to evaluation-model and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 9
  - group members: 00ffec204ce4883b72c1b6796c9399a7, 064b704932adc56ad81cb12cde95b5ac, 0fb2a2103c0e0af8ba4bf1626580267f, 21672d6ed5c0741a945994d3cf53c18f, 2927e1a22f4246cc1eeed1d7049a58ac, 9897a63693a09fdd53ba5760e29f9a92, ba01893d86d84e37d5462f5487616ec6, beaa841afba5f8e73fe10a61a3ac6178
  - score: 5/10
  - top files: internal/classify/classify.go, internal/classify/clone_only.go, internal/decision/decision.go, internal/metrics/boundary/unbalanced_edge.go, internal/metrics/metrics.go, internal/rules/rules_api.go, internal/rules/rules_dependency.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`11953ac6`] Review 36 same-shape Balanced-Coupling advisory edges from evaluation-core to evaluation-model and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 36
  - group members: 11953ac65d298e2934698fc7d79ca3e3, 15c7e813baf3daf01100626ddfe6fadf, 183009137b0830a764b2164146ec9e48, 23d1a22d068f12740e77f7949fc01aa1, 2a96bee4c488a3cc4772a3cb20fd57c0, 42a0c26c5e5ddd301021b15be867eff8, 437bdfb6abb49e5b99bc2031c4438c78, 454128e18ac2e4438c59c39645e522fa
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: internal/agenttask/agenttask.go, internal/classify/distance_structure.go, internal/classify/external_systems.go, internal/classify/location.go, internal/classify/volatility_provenance.go, internal/decision/config_compare.go, internal/decision/decision.go, internal/facts/facts.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`

_…and 19 more advisory tasks (see --json for the full list)._

## Balanced Coupling advisories (65 rollups, 331 edges)

Same-shape edges between a module pair are grouped into one rollup.
Integration strength × distance × volatility lint messages.
Severity: `none` · `low` · `medium` · `high` · `critical`.

```
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/archfit/analyze.go -> internal/engine  [2e85c357]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 5 same-shape edges (e.g. 2e85c357a78837ee6437847d6597e135,9452042ab3d1dd826d9fe09eb9d78b89,94aed280063bd37945a3a57011320bac,a114da5336cf0e0842e4113fb5beefb3,cb27127473df45cef48198ddc0f8d9bb)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/archfit/analyze.go -> internal/model/diagnostic  [023add20]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 30 same-shape edges (e.g. 023add20bf21c6007cb54b20b4de973a,07b9a1eb1b66a64fb3b072f6ede30bbe,0b2dbc0f272690e8c7a570abeccba563,0f6827872c9c6855fdfe736670d7371d,1101eb26f63e6ca5402405104c4bac48,13a9c9667d34ede7c883fe65e7544000,1aa41e85b46a513fc461a16873e16743,23ba33f54622e001343e75d13c1fe970)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/archfit/baseline.go -> internal/score  [ca479745]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 2 same-shape edges (e.g. ca479745d83831953a1a94a9701e3606,f85736366f2ee73e90e97832b555b6d5)
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
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/archfit/pipeline_config.go -> internal/ownership  [460f5c07]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 2 same-shape edges (e.g. 460f5c073149fa15d3d66549c22e0b37,8ae6c5b610a92a7136eeb8692fffdea6)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/calibrate/main.go -> internal/model/coupling  [a967c418]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 2 same-shape edges (e.g. a967c418163b644b40b995788d7bd4cc,b8bc44dd75d2b31d4311672712d0520f)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/agenttask/agenttask.go -> internal/model/diagnostic  [11953ac6]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 36 same-shape edges (e.g. 11953ac65d298e2934698fc7d79ca3e3,15c7e813baf3daf01100626ddfe6fadf,183009137b0830a764b2164146ec9e48,23d1a22d068f12740e77f7949fc01aa1,2a96bee4c488a3cc4772a3cb20fd57c0,42a0c26c5e5ddd301021b15be867eff8,437bdfb6abb49e5b99bc2031c4438c78,454128e18ac2e4438c59c39645e522fa)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/baseline/baseline.go -> internal/model/coupling  [454623ac]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 3 same-shape edges (e.g. 454623ac5dcf7a709861bba1c2f1f8ce,6efd59da1be96f4d3f3fd3eb57fc3603,f971a7602d142896a56ddb2fc5e18675)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/baseline/baseline.go -> internal/status  [e1e107d3]
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
ARCHFIT[BC-UNBALANCED CRITICAL] internal/engine/advisories.go -> internal/model/diagnostic  [007ef6e3]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 10 same-shape edges (e.g. 007ef6e39304725637aa5419ed7af1a3,0ba9d83cc4f75d559f3d512133773924,0d0e66664dc1b391b2876217256b1f7c,25c6ddf4fc3f297909f7b07735e5c6fa,686a4227aa56e3f3536faa5f23ba0642,968e8781e3ea5cf4468a3f19454101c9,b75cbcf8a255e3780df9775bee3a3b07,bede9acc5cc65babf90634011d7c9c6a)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/extract/astgrep/astgrep.go -> internal/model/diagnostic  [25784e70]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 14 same-shape edges (e.g. 25784e708e94d1207ffce4bb1b409125,25ecef079168a23145131f764702ded5,3f0095eb439b7e574e5ac7d166bf6c10,67ef729249dd71fc8dad5f4f81eb8a4b,83de912c5efa7ca493dced008222a193,86aae874ee17dc5bece82c0dabd6ce16,8f168613359f4271141665db23781f7b,9297d07355c21dcf208e562c7e92d159)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/output/jsonout/jsonout.go -> internal/model/coupling  [31ba1874]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 10 same-shape edges (e.g. 31ba1874b60a662b9b355f8508f3e843,3ff79c222feb620984378863a20f4ebc,499ee1de26c19ab5192d2613b165e3ae,63639586ab27ad4284dcc1bc1a9f8d6e,687b11cc0b926ed7b5b99eccfa661ecb,9e63ea4afb7689a20834eae682e6d883,9f337c0ca3e09b7e6548152a8d937276,a5c02118d289d2d70f5d9d1ad29bc42a)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/ports/ports.go -> internal/model/diagnostic  [7d4fa474]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/view/view.go -> internal/model/module  [814f70ea]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED HIGH] cmd/archfit/config_compare.go -> internal/score  [cd1a78c7]
  integration strength: contract      distance: cross_module_same_owner         volatility: high
  score: 4/10 (high) [book]
  why: balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → high severity (contract coupling to a volatile target at low distance → cascading chang...
  rollup: 2 same-shape edges (e.g. cd1a78c7b466486889d13993285aa57e,f8ff62aa48ab68d9e361493a3617ca37)
```

```
ARCHFIT[BC-UNBALANCED HIGH] cmd/archfit/pipeline_run.go -> internal/toolrun  [3f597ecb]
  integration strength: contract      distance: cross_module_same_owner         volatility: high
  score: 4/10 (high) [book]
  why: balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → high severity (contract coupling to a volatile target at low distance → cascading chang...
  rollup: 3 same-shape edges (e.g. 3f597ecb1a08b4e1b2277101ef03ce68,4392dba45e4f134ae5350609d5ff2b60,49c86332aa2eb6e1adabd18aec1fd613)
```

```
ARCHFIT[BC-UNBALANCED HIGH] cmd/archfit/worktree.go -> internal/model/module  [af2bc574]
  integration strength: contract      distance: cross_module_same_owner         volatility: high
  score: 4/10 (high) [book]
  why: balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → high severity (contract coupling to a volatile target at low distance → cascading chang...
```

```
ARCHFIT[BC-UNBALANCED HIGH] internal/classify/volatility_provenance.go -> internal/model/module  [1a90396f]
  integration strength: contract      distance: cross_module_same_owner         volatility: high
  score: 4/10 (high) [book]
  why: balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → high severity (contract coupling to a volatile target at low distance → cascading chang...
  rollup: 3 same-shape edges (e.g. 1a90396f91f8c74db334cb80b84fac6f,2b57e12ee0f68eee1fb68cbb24723219,704b1c3fc2f631e0e3b4f0c2c0ace876)
```

```
ARCHFIT[BC-UNBALANCED HIGH] internal/initcfg/initcfg.go -> internal/toolrun  [994263a0]
  integration strength: contract      distance: cross_module_same_owner         volatility: high
  score: 4/10 (high) [book]
  why: balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → high severity (contract coupling to a volatile target at low distance → cascading chang...
```

```
ARCHFIT[BC-UNBALANCED HIGH] internal/output/jsonout/jsonout.go -> internal/model/diagnostic  [6988a01e]
  integration strength: contract      distance: cross_module_same_owner         volatility: high
  score: 4/10 (high) [book]
  why: balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → high severity (contract coupling to a volatile target at low distance → cascading chang...
```

```
ARCHFIT[BC-UNBALANCED HIGH] internal/ports/extractor_moq.go -> internal/model/diagnostic  [0124a055]
  integration strength: contract      distance: cross_module_same_owner         volatility: high
  score: 4/10 (high) [book]
  why: balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → high severity (contract coupling to a volatile target at low distance → cascading chang...
  rollup: 4 same-shape edges (e.g. 0124a055f95424a7dbf75316ad5a50d1,749cb1951c277ee634e09f4201179779,c2650806cd39f761b01ecd31a2396b65,c48d766e0355d534987c46f78d7e13a7)
```

```
ARCHFIT[BC-UNBALANCED HIGH] internal/view/view.go -> internal/model/coupling  [2c6f1457]
  integration strength: contract      distance: cross_module_same_owner         volatility: high
  score: 4/10 (high) [book]
  why: balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → high severity (contract coupling to a volatile target at low distance → cascading chang...
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/analyze.go -> internal/config  [0253289b]
  integration strength: functional    distance: cross_module_same_owner         volatility: high
  score: 5/10 (medium) [book]
  why: balanced coupling: functional integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 16 same-shape edges (e.g. 0253289bc1623d1c847fbb4b14db01ca,1cc457d122eda82495745b71a8623a10,2ab6dd6c6c98eb5ad56e21a522d3d938,4fb003f14c3faea28b169e0c85e9a259,58bc8aba5d9ecb08efc4e074f63ee8c5,60348cc93b9075d49384cef144d1e6b2,90b6c329a2a1e1ef28f8e49d40037cb4,a993f8ba00e7797f62983e493ae1c1db)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/analyze.go -> internal/decision  [11d815cc]
  integration strength: functional    distance: cross_module_same_owner         volatility: high
  score: 5/10 (medium) [book]
  why: balanced coupling: functional integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 10 same-shape edges (e.g. 11d815cc391fbb3e4c6a6898eee51cd7,5785eb4b39bf46ade814ce4f614006cb,6497d4b5d60e5d066ce042ff08519fcb,693accf04efccec0be544b0e55ac5637,8d3baeecbad359e0aec66e7cafd203fe,a44e7ad048371eb1a19d72f56767e3b4,a6f062f5dc3caa479ad03202d557b299,c5aedc56bf76d77834a70caa5c178e48)
```

- ... +40 more rollups (use `--format json`)

## Advisories (3)

- **bc/duplicated_knowledge** [medium] new — internal/extract/golang/cache.go → internal/engine/assemble.go: duplicated knowledge: cross-module code clones between fact-adapters and pipeline-engine with no import edge — symmetric functional cou...
- **syntax_api_size_ceiling** [medium] new — evaluation-core → evaluation-core: Module "evaluation-core" has 329 exported declarations, exceeding the limit of 320
- **syntax_api_size_ceiling** [medium] new — fact-adapters → fact-adapters: Module "fact-adapters" has 417 exported declarations, exceeding the limit of 320

## Supporting structural metrics (beyond Balanced Coupling)

Report-only. These metrics support Balanced Coupling reasoning but never gate.

- **cycle**: 0 import cycles — strong
- **coverage**: 100% coverage — strong
- **blast_radius**: 6 of 63 modules are change-impact hubs: .../model/graph (74%, 46 deps), .../model/finding (55%, 34 deps), .../model/diagnostic (50%, 31 deps), .../model/coupling (44%, 27 deps), .../model/module (35%, 22 deps)+1 more — info

## Distance confidence

- `code_structure`: always on (deterministic tree-distance baseline)
- `owner_source`: config
- `deploy_unit_source`: ok
- `owner_model`: single_owner_degenerate
- deploy-unit detector mapped modules: 3
- distance basis: code_structure=393, deploy_unit=7
- interpretation: same-owner is the lowest cross-module distance; this is a low socio-technical distance signal, not missing ownership; deploy_unit and declared external_systems evidence can still raise distance when configured/detected
- connected modules in coupling sample: 16
- distance rungs implemented: D=2, D=4, D=7, D=9, D=10; omitted/compressed: D=1, D=3, D=5, D=6, D=8
- code-structure boundary crossings: 2→393
- code-structure shared-ancestor depth: 0→393
- distance compression: D=3/D=5/D=6/D=8 remain compressed: current graph/config facts distinguish same module, same owner, different owner, deploy unit, and declared vendor seam, but not finer package/library distance without guessing.
- D=1 compressed: object/member-level distance is not available from module dependency edges
- D=3 compressed: current facts distinguish same module vs cross-module, but not object/package micro-distance
- D=5 compressed: package/library middle distance is not split without explicit stable package-boundary metadata
- D=6 compressed: intermediate ownership/library distance has no deterministic signal beyond owner and tree structure
- D=8 compressed: library-like seams remain compressed: undeclared libraries stay excluded, while declared external_systems score at D=10
- undeclared external/library edges excluded: 617
- clone-only duplicated knowledge: 1 scored, 0 advisory-only
- tail risk: worst balance 2/10; lower-decile balance 2/10; high-or-worse edges 139/400 (34%); critical 123; distributed-monolith 0
- clone-only tail: worst balance 6/10; high-or-worse 0/1 scored clone-only pairs

## Coverage

- scip: ok (1433 files)
- scip-symbols: ok (7069 files)
- go/packages: ok (179 files)
- dependency-cruiser: absent
- grimp: absent
- cargo: absent
- loc: ok (177 files)
- deploy-unit: ok (6 files)
- jscpd: ok (264 files)
- ast-grep: ok
- ast-grep/syntax: ok (315 files)
- cargo-modules: absent
