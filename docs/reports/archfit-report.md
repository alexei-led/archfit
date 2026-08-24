# archfit — decision

- **Decision:** ACCEPTABLE WITH WATCH ITEMS
- **Gate:** PASS — 0 blocking
- **Warnings:** 84 advisory
- **Score:** 54 / 100 (mixed)

Acceptable with watch items. Monitor flagged areas.

## Recommendations

### Must fix
- none

### Should fix
- **bc/imbalanced_coupling** — balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → high severity (contract coupling to a volatile target at low distance → cascading changes contained to one owner)

### Watch
- **bc/duplicated_knowledge** — duplicated knowledge: cross-module code clones between fact-adapters and pipeline-engine with no import edge — symmetric functional coupling; a change to the shared logic must be repeated in both modules. Extract the shared knowledge, or accept the pair with an approved label
- **bc/imbalanced_coupling** — balanced coupling: contract integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
- **syntax_api_size_ceiling** — Module "evaluation-core" has 331 exported declarations, exceeding the limit of 320

## Why the score is low

- **coupling_balance** (54/100, mixed): critical-band coupling present; inspect the reported strength, distance, and volatility drivers — 429 scored internal cross-boundary edges; mean book balance 5.9/10 → value 54; strength distribution: contract=44, functional=156, model=204, symmetric=25; distance distribution: cross_module_same_owner=429; volatility distribution: high=59, low=129, medium=241; balance drivers: strength_distance=79, tie=92, volatility=258; critical drivers: strength_distance=14; top module pairs: archfit-cli -> fact-adapters=34, fact-adapters -> evidence-model=27, evaluation-core -> evidence-model=23, archfit-cli -> policy-language=21, archfit-cli -> scan-contract=20; scored fraction: 100% (429 scored, 0 abstained, internal only)
  - _What moves it:_ Reduce high-fan-in functional edges across module boundaries or introduce stable contracts.
# archfit report

**Verdict:** warn (exit 2)
**Config hash:** `d491f6d8ca41aa1cf40da2069f1eae94222dbb6263aef054df55e81f50cca0fe`

## Summary

- gate findings: 0
- warnings: 84
- waivers used: 0

## Metrics

- **encapsulation**: n/a — n/a (low confidence)
- **unbalanced_edge**: 0 new high-risk unbalanced edges — strong

## Structural facts (neutral evidence)

104 modules; top 5 per axis (full list in `--format json`):

- inbound module fan-in: internal/model/graph (44), internal/model/evidence (40), internal/view (33), internal/model/diagnostic (27), internal/model/report (27)
- outbound destinations: cmd/archfit (46), internal/engine (25), internal/engine_test (22), internal/initcfg_test (17), internal/decision_test (11)
- LOC: cmd/archfit (8745), internal/initcfg (4924), internal/engine (2915), internal/classify (1657), internal/extract/golang (1523)

## Syntax surface (neutral evidence)

2228 declaration(s) extracted by ast-grep (full list in `--format json`):

- annotation: 1
- enum: 1
- function: 1511
- interface: 17
- method: 245
- struct: 252
- type_alias: 33
- type_leak: 168
- exported (public API): 2059

Per module:

- (unscoped): 61
- archfit-cli: 334
- architecture-model: 16
- architecture-tests: 5
- baseline-store: 21
- config-lifecycle: 233
- coupling-model: 49
- development-tools: 41
- evaluation-core: 344
- evidence-model: 81
- fact-adapters: 452
- finding-model: 14
- labels-policy: 13
- labels-store: 5
- llm-adapter: 30
- pipeline-contracts: 104
- pipeline-engine: 109
- pipeline-state: 13
- policy-language: 119
- rendering: 95
- report-contract: 25
- scan-contract: 40
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
- ... +2039 more exported declarations (use `--format json`)

## Connascence evidence (deterministic)

Report-only. Static facts only; semantic and dynamic categories without deterministic evidence stay unmeasured.

- edges with evidence: 1087
- abstained edges: 4
- total evidence facts: 4235
- strength inferred from connascence: 78 edges
- by kind: algorithm=947, meaning=592, name=1558, type=1138
- by source: go/types=2883, scip=1352
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
- modules touched: 21
- caveat: Supporting evidence only. Git history can reflect both essential and accidental volatility and never changes scoring or gate verdicts.

Top touched modules:

- **archfit-cli**: 162 commit(s) [declared volatility=medium]
- **evaluation-core**: 150 commit(s) [declared volatility=high]
- **pipeline-engine**: 109 commit(s) [declared volatility=medium]
- **fact-adapters**: 101 commit(s) [declared volatility=medium]
- **policy-language**: 72 commit(s) [declared volatility=high]

## Advisory tasks (49)

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
  - severity: medium; status: new; group_count: 5
  - group members: 0b2dbc0f272690e8c7a570abeccba563, 1aa41e85b46a513fc461a16873e16743, 23ba33f54622e001343e75d13c1fe970, 37722647f1df242ab64b7ec66569f66c, ca0d3a81e3286df526c19838b5bbcf89
  - score: 5/10
  - top files: cmd/archfit/baseline.go, cmd/archfit/enrich.go, cmd/archfit/enrich_abstained.go, cmd/archfit/explain.go, cmd/archfit/llmreview.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=medium
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
- **bc/imbalanced_coupling** [`07b9a1eb`] Review 3 same-shape Balanced-Coupling advisory edges from archfit-cli to pipeline-state and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 3
  - group members: 07b9a1eb1b66a64fb3b072f6ede30bbe, 68d1ef67cc2004e6a5f5bfe33ad076e7, d359e673ec3d9540fc5be2d996ff18e9
  - score: 5/10
  - top files: cmd/archfit/enrich.go, cmd/archfit/enrich_abstained.go, cmd/archfit/pipeline_run.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=medium
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
- **bc/imbalanced_coupling** [`51ad2e4f`] Review 2 same-shape Balanced-Coupling advisory edges from config-lifecycle to fact-adapters and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 2
  - group members: 51ad2e4f376a391f948ef1a33b7a6e11, 93d12df05a4547c1977a6dc0844c7bb6
  - score: 6/10
  - top files: internal/extract/scip/scip_strength.go, internal/initcfg/discover_go.go, internal/initcfg/discover_py.go, internal/initcfg/discover_rust.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=symmetric, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`064b7049`] Review 3 same-shape Balanced-Coupling advisory edges from evaluation-core to architecture-model and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 3
  - group members: 064b704932adc56ad81cb12cde95b5ac, 2927e1a22f4246cc1eeed1d7049a58ac, d18ec674f1db3e1a99beeaffdabb3686
  - score: 5/10
  - top files: internal/classify/classify.go, internal/rules/rules_api.go, internal/rules/rules_dependency.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`15c7e813`] Review 2 same-shape Balanced-Coupling advisory edges from evaluation-core to architecture-model and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 2
  - group members: 15c7e813baf3daf01100626ddfe6fadf, cc773e88a6a026d2440cafdfde859f9c
  - score: 5/10
  - top files: internal/classify/distance_structure.go, internal/staleness/staleness.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`

_…and 24 more advisory tasks (see --json for the full list)._

## Balanced Coupling advisories (81 rollups, 299 edges)

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
ARCHFIT[BC-UNBALANCED HIGH] cmd/archfit/config_compare.go -> internal/score  [cd1a78c7]
  integration strength: contract      distance: cross_module_same_owner         volatility: high
  score: 4/10 (high) [book]
  why: balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → high severity (contract coupling to a volatile target at low distance → cascading chang...
  rollup: 2 same-shape edges (e.g. cd1a78c7b466486889d13993285aa57e,f8ff62aa48ab68d9e361493a3617ca37)
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
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/baseline.go -> internal/model/coupling  [0b2dbc0f]
  integration strength: model         distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 5 same-shape edges (e.g. 0b2dbc0f272690e8c7a570abeccba563,1aa41e85b46a513fc461a16873e16743,23ba33f54622e001343e75d13c1fe970,37722647f1df242ab64b7ec66569f66c,ca0d3a81e3286df526c19838b5bbcf89)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/baseline.go -> internal/model/finding  [023add20]
  integration strength: model         distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 5 same-shape edges (e.g. 023add20bf21c6007cb54b20b4de973a,1101eb26f63e6ca5402405104c4bac48,8afbbad051fd4b9e5d7916b078495f68,8bf233b1773be9a6613ef77094c131e1,9e3eebe06f46134755564db05a15cbd4)
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

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/enrich.go -> internal/labels  [7d9740d6]
  integration strength: functional    distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: functional integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 2 same-shape edges (e.g. 7d9740d6c49cfef92c4c088a38146c54,85c3b03c48087edcc548ecca5771626d)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/enrich.go -> internal/llm  [4dee1477]
  integration strength: symmetric     distance: cross_module_same_owner         volatility: medium
  score: 6/10 (medium) [book]
  why: balanced coupling: symmetric integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 6 same-shape edges (e.g. 4dee147706e26c8ecd7f40d2b284c29b,7a80f7a3cb06def6fd4e8921baafd18c,87730e067efe316da849db5512a9350f,8a83e18dca7af9c5fb368854cfef31b7,af2e5b717e51a4cfdd2d805330373cf5,ee1d062958cb74b32d1073cf500c10a6)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/enrich.go -> internal/model/graph  [3ba9d033]
  integration strength: functional    distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: functional integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 3 same-shape edges (e.g. 3ba9d033b58d56d4b89b1f09d311ecc5,4bd04cc81d5e64503ec8e462ef1005fc,4d4bcf55e22e3424ca3f048d4932a6e9)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/enrich.go -> internal/model/module  [01b0db59]
  integration strength: functional    distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: functional integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 5 same-shape edges (e.g. 01b0db59b3d599ee59003a372499ce46,4b2864a9c1285d45af622ac77680ffef,6598b0dcac0376215ec99a950cd84c32,6ba848bfe4df1afe3143a49cc5d0f3c2,ffd9ffd66a94686756c9b123f4756158)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/enrich.go -> internal/model/signal  [07b9a1eb]
  integration strength: model         distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 3 same-shape edges (e.g. 07b9a1eb1b66a64fb3b072f6ede30bbe,68d1ef67cc2004e6a5f5bfe33ad076e7,d359e673ec3d9540fc5be2d996ff18e9)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/enrich_values.go -> internal/model/module  [5aadacc6]
  integration strength: model         distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 2 same-shape edges (e.g. 5aadacc61f3c99b11144b76d63dada09,69c05033655351da6ead64fbac3f8485)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/init.go -> internal/initcfg  [18ebbe3e]
  integration strength: symmetric     distance: cross_module_same_owner         volatility: medium
  score: 6/10 (medium) [book]
  why: balanced coupling: symmetric integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 5 same-shape edges (e.g. 18ebbe3ec25eef143658721dfd510987,713435dc7222cb1d64468c472eb20600,b3a81c151f702a38907f50d25d5d3be2,d0de32cf75db6cf7d8d67e7a9705556b,ed7a32d97c89c445b9f64a12432273bc)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/llmreview.go -> internal/model/graph  [413473b0]
  integration strength: model         distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/pipeline_config.go -> internal/labels  [3ed3e523]
  integration strength: model         distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/pipeline_config.go -> internal/ownership  [460f5c07]
  integration strength: model         distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 2 same-shape edges (e.g. 460f5c073149fa15d3d66549c22e0b37,8ae6c5b610a92a7136eeb8692fffdea6)
```

- ... +56 more rollups (use `--format json`)

## Advisories (3)

- **bc/duplicated_knowledge** [medium] new — internal/extract/golang/cache.go → internal/engine/assemble.go: duplicated knowledge: cross-module code clones between fact-adapters and pipeline-engine with no import edge — symmetric functional cou...
- **syntax_api_size_ceiling** [medium] new — evaluation-core → evaluation-core: Module "evaluation-core" has 331 exported declarations, exceeding the limit of 320
- **syntax_api_size_ceiling** [medium] new — fact-adapters → fact-adapters: Module "fact-adapters" has 417 exported declarations, exceeding the limit of 320

## Supporting structural metrics (beyond Balanced Coupling)

Report-only. These metrics support Balanced Coupling reasoning but never gate.

- **cycle**: 0 import cycles — strong
- **coverage**: 100% coverage — strong
- **blast_radius**: 6 of 65 modules are change-impact hubs: .../model/graph (67%, 43 deps), .../model/evidence (47%, 30 deps), .../model/coupling (42%, 27 deps), .../model/report (38%, 24 deps), .../model/finding (36%, 23 deps)+1 more — info

## Distance confidence

- `code_structure`: always on (deterministic tree-distance baseline)
- `owner_source`: config
- `deploy_unit_source`: ok
- `owner_model`: single_owner_degenerate
- deploy-unit detector mapped modules: 3
- distance basis: code_structure=422, deploy_unit=7
- interpretation: same-owner is the lowest cross-module distance; this is a low socio-technical distance signal, not missing ownership; deploy_unit and declared external_systems evidence can still raise distance when configured/detected
- connected modules in coupling sample: 21
- distance rungs implemented: D=2, D=4, D=7, D=9, D=10; omitted/compressed: D=1, D=3, D=5, D=6, D=8
- code-structure boundary crossings: 2→422
- code-structure shared-ancestor depth: 0→422
- distance compression: D=3/D=5/D=6/D=8 remain compressed: current graph/config facts distinguish same module, same owner, different owner, deploy unit, and declared vendor seam, but not finer package/library distance without guessing.
- D=1 compressed: object/member-level distance is not available from module dependency edges
- D=3 compressed: current facts distinguish same module vs cross-module, but not object/package micro-distance
- D=5 compressed: package/library middle distance is not split without explicit stable package-boundary metadata
- D=6 compressed: intermediate ownership/library distance has no deterministic signal beyond owner and tree structure
- D=8 compressed: library-like seams remain compressed: undeclared libraries stay excluded, while declared external_systems score at D=10
- undeclared external/library edges excluded: 620
- clone-only duplicated knowledge: 1 scored, 0 advisory-only
- tail risk: worst balance 2/10; lower-decile balance 5/10; high-or-worse edges 16/429 (3%); critical 14; distributed-monolith 0
- clone-only tail: worst balance 6/10; high-or-worse 0/1 scored clone-only pairs

## Coverage

- scip: ok (1540 files)
- scip-symbols: ok (7148 files)
- go/packages: ok (181 files)
- dependency-cruiser: absent
- grimp: absent
- cargo: absent
- loc: ok (179 files)
- deploy-unit: ok (6 files)
- jscpd: ok (268 files)
- ast-grep: ok
- ast-grep/syntax: ok (317 files)
- cargo-modules: absent
