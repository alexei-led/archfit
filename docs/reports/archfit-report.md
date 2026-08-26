# archfit — decision

- **Decision:** ACCEPTABLE WITH WATCH ITEMS
- **Gate:** PASS — 0 blocking
- **Warnings:** 105 advisory
- **Score:** 41 / 100 (mixed)

Acceptable with watch items. Monitor flagged areas.

## Recommendations

### Must fix
- none

### Should fix
- **bc/imbalanced_coupling** — balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (cheap to change; not a distributed monolith))

### Watch
- **bc/duplicated_knowledge** — duplicated knowledge: cross-module code clones between analysis-application and architecture-tests with no import edge — symmetric functional coupling; a change to the shared logic must be repeated in both modules. Extract the shared knowledge, or accept the pair with an approved label
- **bc/imbalanced_coupling** — balanced coupling: symmetric integration strength × cross_module_same_owner distance × high volatility → medium severity (unbalanced coupling → elevated maintenance effort)

## Why the score is low

- **coupling_balance** (41/100, mixed): critical-band coupling present; inspect the reported strength, distance, and volatility drivers — 364 scored internal cross-boundary edges; mean book balance 4.7/10 → value 41; scored fraction: 100% (364 scored, 0 abstained, internal only); critical-band edges: 69 (0 distributed-monolith: critical at high distance); strength distribution: contract=55, functional=114, model=160, symmetric=35; distance distribution: cross_module_same_owner=364; volatility distribution: high=180, low=29, medium=155; balance drivers: strength_distance=194, tie=29, volatility=141; critical drivers: strength_distance=69
  - _What moves it:_ Reduce high-fan-in functional edges across module boundaries or introduce stable contracts.
# archfit report

**Verdict:** pass (exit 0)
**Config hash:** `2bc857acb85bda728cc60354d4fc88fcfad7d596743d93836e268ef55bdb333a`

## Summary

- gate findings: 0
- warnings: 105
- waivers used: 0

## Metrics

- **encapsulation**: n/a — n/a (low confidence)
- **unbalanced_edge**: 0 new high-risk unbalanced edges — strong

## Structural facts (neutral evidence)

114 modules; top 5 per axis (full list in `--format json`):

- inbound module fan-in: internal/model/evidence (42), internal/relationship (30), internal/model/graph (29), internal/policy (29), internal/toolrun (28)
- outbound destinations: cmd/archfit (34), internal/evidence/acquisition (20), internal/assessment/evaluation (18), internal/extract/acquire (17), internal/initcfg_test (17)
- LOC: cmd/archfit (5617), internal/initcfg (4924), internal/application (2385), internal/relationship/classify (1731), internal/relationship/analysis (1619)

## Syntax surface (neutral evidence)

2794 declaration(s) extracted by ast-grep (full list in `--format json`):

- annotation: 1
- enum: 1
- function: 1639
- interface: 41
- method: 367
- struct: 430
- type_alias: 47
- type_leak: 268
- exported (public API): 2525

Per module:

- (unscoped): 61
- analysis-application: 210
- analysis-scope: 31
- architecture-policy: 39
- architecture-tests: 52
- assessment-repair: 438
- cli-composition: 323
- config-lifecycle: 233
- development-tools: 41
- evidence-acquisition: 78
- evidence-adapters: 460
- evidence-analysis: 13
- evidence-contracts: 88
- persistence-adapters: 130
- policy-config-adapter: 142
- provider-adapters: 30
- relationship-analysis: 244
- report-adapters: 100
- report-contract: 81

### Public API


`cmd/archfit/analysis_characterization_test.go` [cli-composition]:
- `TestAnalyzeCheckCharacterization` (function)

`cmd/archfit/analyze.go` [cli-composition]:
- `AnalyzeCmd` (struct)
- `Help` (method)
- `Run` (method)

`cmd/archfit/analyze_exit_test.go` [cli-composition]:
- `TestOutcomeExitCodeOwnsCLIOutcomeTranslation` (function)
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
- `Run` (method)
- ... +2505 more exported declarations (use `--format json`)

## Connascence evidence (deterministic)

Report-only. Static facts only; semantic and dynamic categories without deterministic evidence stay unmeasured.

- edges with evidence: 1193
- abstained edges: 4
- total evidence facts: 4594
- strength inferred from connascence: 65 edges
- by kind: algorithm=976, meaning=618, name=1700, type=1300
- by source: go/types=3145, scip=1449
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

- **cli-composition**: 169 commit(s) [declared volatility=medium]
- **evidence-adapters**: 97 commit(s) [declared volatility=medium]
- **config-lifecycle**: 76 commit(s) [declared volatility=medium]
- **policy-config-adapter**: 76 commit(s) [declared volatility=high]
- **report-adapters**: 49 commit(s) [declared volatility=medium]

## Advisory tasks (55)

Report-only rollups from grouped advisories; these do not affect verdict or gate status.
- **bc/imbalanced_coupling** [`088c6257`] Review 8 same-shape Balanced-Coupling advisory edges from analysis-application to assessment-repair and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 8
  - group members: 088c6257fe3eba57ac2a2fb4abbd6dda, 2e1590e3580d6096c9ef526ef72d1595, 5f32df6c7a80d4502aa9ef635567d328, 6b7786f551449325fafa7e87281e25f9, 90416a2e23a40d29eaca689b5da302c6, bac595ad96d976c901f09e50962ac0e7, c7b702dff75403dcc58a2a93a04b0e94, daa5c812154dc135b8bd5df2a5a2e631
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: internal/application/analysis.go, internal/application/base_compare.go, internal/application/baseline.go, internal/application/report.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`1f841c4e`] Review 4 same-shape Balanced-Coupling advisory edges from analysis-application to assessment-repair and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 4
  - group members: 1f841c4ef66c9d9a70430132618740e2, 2b2f68b0757a3eaeb5c9244588a8ff16, 50a698c4a94070e7e276fa6164316d8e, b23c3ff8a9301574ee41198330305536
  - score: 6/10
  - top files: internal/application/analysis.go, internal/application/base_compare.go, internal/application/compare.go, internal/application/report.go, internal/assessment/evaluation/relationship_projection.go
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
- **bc/imbalanced_coupling** [`3b3a0035`] Review 2 same-shape Balanced-Coupling advisory edges from analysis-application to evidence-contracts and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 2
  - group members: 3b3a0035c0f9a137bf5df57ffaf9161b, 8ebe12ee50b033fc6bbb0e2eb7884e20
  - score: 5/10
  - top files: internal/application/analysis.go, internal/application/report.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`86e21ab4`] Review 2 same-shape Balanced-Coupling advisory edges from analysis-application to relationship-analysis and reduce the coupling risk without changing gate policy.
  - severity: high; status: new; group_count: 2
  - group members: 86e21ab46f46cfc77479c37d61d6365a, ef3e28e610acf5df3aececcb8b96764c
  - score: 4/10
  - top files: internal/application/analysis.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=contract, distance=cross_module_same_owner, volatility=high
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
- **bc/imbalanced_coupling** [`40ee0174`] Review 2 same-shape Balanced-Coupling advisory edges from analysis-application to relationship-analysis and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 2
  - group members: 40ee017434202d1801b301f777e144ff, 9c396f88e33fd4632815029b08249e14
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: internal/application/capture.go, internal/application/enrichment_selection.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`67930282`] Review 4 same-shape Balanced-Coupling advisory edges from analysis-application to report-contract and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 4
  - group members: 6793028290cda533ad5cb1fe502698a9, 856b984c6f6570371ec806f50bdeb9eb, e945f398ba0acaaefad734f7ea16b263, ff0f7fb45b81ade146258afbdf355dd3
  - score: 5/10
  - top files: internal/application/analysis.go, internal/application/baseline.go, internal/application/explain.go, internal/application/report.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`07b0acba`] Review 4 same-shape Balanced-Coupling advisory edges from assessment-repair to architecture-policy and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 4
  - group members: 07b0acbafd10996d237ed83afbea5013, 744edeaae5b7fd628ebdfc37ebed4a63, b6c441e6a53d2965735ca16328b0848f, d9ee7131a6b7f46a1a5095370d7edd5f
  - score: 5/10
  - top files: internal/assessment/evaluation/advisories.go, internal/assessment/evaluation/assess.go, internal/assessment/rules/rules_api.go, internal/assessment/rules/rules_dependency.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=functional, distance=cross_module_same_owner, volatility=high
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`39280b30`] Review 8 same-shape Balanced-Coupling advisory edges from assessment-repair to architecture-policy and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 8
  - group members: 39280b30be11ccfb7099e2def1d47b8d, 4928bbc7cc6af3ad3d3e884be4d1e3f6, 5655b0a3b20b149603ec28ba8e5a89df, 5b4dedc0cf8243c58959b0dd935f1136, 71ec017e3902c4bfad1bfde3db7de350, 844f053bccc64638e1355c0ccde2c74f, ded385d022d1f711950a2fa8ed1d67af, f791b7772383ea8c5154650cf5de7c93
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: internal/assessment/evaluation/distance_context.go, internal/assessment/evaluation/evaluation.go, internal/assessment/evaluation/finalize.go, internal/assessment/evaluation/health_warnings.go, internal/assessment/metrics/metrics.go, internal/assessment/rules/rules.go, internal/assessment/staleness/staleness.go, internal/assessment/status/status.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`261cf14b`] Review 10 same-shape Balanced-Coupling advisory edges from assessment-repair to evidence-contracts and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 10
  - group members: 261cf14b927a372b977880be97fae663, 32aa0a8d9b525561b895230003bfcd8f, 52ab4e7c9c9797741ec665fe3ca63d65, 5529af17393932bd5258cce67b79f4ee, 7bd142c207029aed83ed13d951ca6d65, a54099ba4cad069f2b876d062dd363c6, a551977891de7500f28bd7a62a9c103e, bb4ea85fb98e74ca788dc7c656b1dd6f
  - score: 5/10
  - top files: internal/assessment/evaluation/assess.go, internal/assessment/evaluation/git_origin.go, internal/assessment/evaluation/projector.go, internal/assessment/evaluation/ruleset.go, internal/assessment/rules/rules.go, internal/assessment/signals/signal.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=contract, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`024a0546`] Review 13 same-shape Balanced-Coupling advisory edges from assessment-repair to evidence-contracts and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 13
  - group members: 024a054601f108bb2f65bdaa51b20789, 14a18a6187471cea76c40f7707552f67, 320ae359fba984604d56d4a447acfbfe, 50cd43dbb9c1221793f6fd4c86415426, 648d97e461b0a5af85490566837fcf26, 7a646ee19e64da05e1e8601767e44263, 8ea9a6926178f36d81560ff9aa942ed3, a77354659d123f26f9337af753d719ef
  - score: 5/10
  - top files: internal/assessment/agenttask/agenttask.go, internal/assessment/decision/config_compare.go, internal/assessment/decision/git_finding_delta.go, internal/assessment/evaluation/assess.go, internal/assessment/evaluation/distance_context.go, internal/assessment/evaluation/evaluation.go, internal/assessment/evaluation/health_warnings.go, internal/assessment/result/result.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=medium
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`14ac826f`] Review 10 same-shape Balanced-Coupling advisory edges from assessment-repair to relationship-analysis and reduce the coupling risk without changing gate policy.
  - severity: critical; status: new; group_count: 10
  - group members: 14ac826f0bb219f873ec6006917d978a, 27c5b67e5fb7a67865997ff2110a6b17, 4f50da9ecdce6a99125244d19c5a6501, 56ffebc60d204753e8d10c0b385735e7, 6c3d4affffa9159cb403577e4da2d96e, 8e06068b0c9d4bef16e983d52c4b732f, cd2a107f381402a0e43a65ce25ba1349, d81278395ebcad9954893dc63cd52ac2
  - cheapest move: reduce_strength
  - score: 2/10
  - top files: internal/assessment/evaluation/advisory_severity.go, internal/assessment/evaluation/assess.go, internal/assessment/evaluation/evaluation.go, internal/assessment/evaluation/projector.go, internal/assessment/evaluation/relationship_projection.go, internal/assessment/rules/rules.go, internal/assessment/rules/rules_api.go, internal/assessment/score/score_boundary_coupling.go
  - constraint: report-only advisory; do not promote to a gate unless coupling.gate policy changes
  - constraint: keep agent_tasks[] reserved for active gate findings
  - constraint: preserve or improve coupling shape: strength=model, distance=cross_module_same_owner, volatility=high
  - constraint: prefer cheapest_move: reduce_strength
  - validate: `archfit check -c .archfit.yaml`
- **bc/imbalanced_coupling** [`08ad9f3c`] Review 7 same-shape Balanced-Coupling advisory edges from assessment-repair to relationship-analysis and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 7
  - group members: 08ad9f3c6f1298ff86bb271fc0443532, 28513b27f490f50de7fc955712d8423b, 3d8129ecba9ecee3a27d37e249e2c0db, 6b7bba3f58a30123ec30a10181b79d71, 76c8702dc476268037aca1b726f06a57, 813705841e9588cd040cac52a82a4c07, b37e406b9bf118840a5c56225daff7f2
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
- **bc/imbalanced_coupling** [`0be20e8b`] Review 4 same-shape Balanced-Coupling advisory edges from cli-composition to config-lifecycle and reduce the coupling risk without changing gate policy.
  - severity: medium; status: new; group_count: 4
  - group members: 0be20e8b1f3def37d037daf6b331b75a, 18ebbe3ec25eef143658721dfd510987, c8838259ed1db91f80347f0fb74b80b8, d0de32cf75db6cf7d8d67e7a9705556b
  - score: 6/10
  - top files: cmd/archfit/config_enrich_adapters.go, cmd/archfit/config_update_adapters.go, cmd/archfit/draft_metadata.go, cmd/archfit/evidence_pack.go, cmd/archfit/init.go, internal/initcfg/evidence_pack.go, internal/initcfg/initcfg.go
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

_…and 30 more advisory tasks (see --json for the full list)._

## Balanced Coupling advisories (101 rollups, 331 edges)

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
  rollup: 8 same-shape edges (e.g. 088c6257fe3eba57ac2a2fb4abbd6dda,2e1590e3580d6096c9ef526ef72d1595,5f32df6c7a80d4502aa9ef635567d328,6b7786f551449325fafa7e87281e25f9,90416a2e23a40d29eaca689b5da302c6,bac595ad96d976c901f09e50962ac0e7,c7b702dff75403dcc58a2a93a04b0e94,daa5c812154dc135b8bd5df2a5a2e631)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/application/analysis.go -> internal/policy  [e02e6390]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/application/capture.go -> internal/relationship  [40ee0174]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 2 same-shape edges (e.g. 40ee017434202d1801b301f777e144ff,9c396f88e33fd4632815029b08249e14)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/assessment/evaluation/advisory_severity.go -> internal/relationship  [14ac826f]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 10 same-shape edges (e.g. 14ac826f0bb219f873ec6006917d978a,27c5b67e5fb7a67865997ff2110a6b17,4f50da9ecdce6a99125244d19c5a6501,56ffebc60d204753e8d10c0b385735e7,6c3d4affffa9159cb403577e4da2d96e,8e06068b0c9d4bef16e983d52c4b732f,cd2a107f381402a0e43a65ce25ba1349,d81278395ebcad9954893dc63cd52ac2)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/assessment/evaluation/distance_context.go -> internal/policy  [39280b30]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 8 same-shape edges (e.g. 39280b30be11ccfb7099e2def1d47b8d,4928bbc7cc6af3ad3d3e884be4d1e3f6,5655b0a3b20b149603ec28ba8e5a89df,5b4dedc0cf8243c58959b0dd935f1136,71ec017e3902c4bfad1bfde3db7de350,844f053bccc64638e1355c0ccde2c74f,ded385d022d1f711950a2fa8ed1d67af,f791b7772383ea8c5154650cf5de7c93)
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

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/labels/labelsio/labelsio.go -> internal/application  [7721c807]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
```

- ... +76 more rollups (use `--format json`)

## Advisories (4)

- **bc/duplicated_knowledge** [medium] new — internal/application/report.go → internal/testutil/report/convert.go: duplicated knowledge: cross-module code clones between analysis-application and architecture-tests with no import edge — symmetric func...
- **bc/duplicated_knowledge** [medium] new — internal/assessment/result/result.go → internal/model/report/document.go: duplicated knowledge: cross-module code clones between assessment-repair and report-contract with no import edge — symmetric functional...
- **bc/duplicated_knowledge** [medium] new — internal/labels/labelsio/labelsio.go → internal/llm/cache.go: duplicated knowledge: cross-module code clones between persistence-adapters and provider-adapters with no import edge — symmetric funct...
- **bc/duplicated_knowledge** [medium] new — internal/model/evidence/evidence.go → internal/model/report/evidence.go: duplicated knowledge: cross-module code clones between evidence-contracts and report-contract with no import edge — symmetric functiona...

## Supporting structural metrics (beyond Balanced Coupling)

Report-only. These metrics support Balanced Coupling reasoning but never gate.

- **cycle**: 0 import cycles — strong
- **coverage**: 100% coverage — strong
- **blast_radius**: 6 of 73 modules are change-impact hubs: .../model/evidence (65%, 47 deps), internal/relationship (47%, 34 deps), .../model/graph (44%, 32 deps), .../model/pattern (39%, 28 deps), .../model/symbol (32%, 23 deps)+1 more — info

## Distance confidence

- `code_structure`: always on (deterministic tree-distance baseline)
- `owner_source`: config
- `deploy_unit_source`: ok
- `owner_model`: single_owner_degenerate
- deploy-unit detector mapped modules: 3
- distance basis: code_structure=355, deploy_unit=9
- interpretation: same-owner is the lowest cross-module distance; this is a low socio-technical distance signal, not missing ownership; deploy_unit and declared external_systems evidence can still raise distance when configured/detected
- connected modules in coupling sample: 18
- distance rungs implemented: D=2, D=4, D=7, D=9, D=10; omitted/compressed: D=1, D=3, D=5, D=6, D=8
- code-structure boundary crossings: 2→355
- code-structure shared-ancestor depth: 0→355
- distance compression: D=3/D=5/D=6/D=8 remain compressed: current graph/config facts distinguish same module, same owner, different owner, deploy unit, and declared vendor seam, but not finer package/library distance without guessing.
- D=1 compressed: object/member-level distance is not available from module dependency edges
- D=3 compressed: current facts distinguish same module vs cross-module, but not object/package micro-distance
- D=5 compressed: package/library middle distance is not split without explicit stable package-boundary metadata
- D=6 compressed: intermediate ownership/library distance has no deterministic signal beyond owner and tree structure
- D=8 compressed: library-like seams remain compressed: undeclared libraries stay excluded, while declared external_systems score at D=10
- undeclared external/library edges excluded: 690
- clone-only duplicated knowledge: 4 scored, 0 advisory-only
- tail risk: worst balance 2/10; lower-decile balance 2/10; high-or-worse edges 81/364 (22%); critical 69; distributed-monolith 0
- clone-only tail: worst balance 6/10; high-or-worse 0/4 scored clone-only pairs

## Coverage

- scip: ok (1650 files)
- scip-symbols: ok (9135 files)
- go/packages: ok (225 files)
- dependency-cruiser: absent
- grimp: absent
- cargo: absent
- loc: ok (223 files)
- deploy-unit: ok (6 files)
- jscpd: ok (319 files)
- ast-grep: ok
- ast-grep/syntax: ok (390 files)
- cargo-modules: absent
