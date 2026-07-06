# archfit — decision

- **Decision:** ACCEPTABLE WITH WATCH ITEMS
- **Gate:** PASS — 0 blocking
- **Warnings:** 82 advisory
- **Score:** 44 / 100 (mixed)

Acceptable with watch items. Monitor flagged areas.

## Recommendations

### Must fix
- none

### Should fix
- **bc/imbalanced_coupling** — balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (cheap to change; not a distributed monolith))

### Watch
- **bc/duplicated_knowledge** — duplicated knowledge: cross-module code clones between internal/engine and internal/extract/golang with no import edge — symmetric functional coupling; a change to the shared logic must be repeated in both modules. Extract the shared knowledge, or accept the pair with an approved label
- **bc/imbalanced_coupling** — balanced coupling: functional integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)

## Why the score is low

- **coupling_balance** (44/100, mixed): critical-band coupling at low distance — local high-strength/high-volatility coupling (cheap cascade), not a distributed monolith — 362 scored internal cross-boundary edges; mean book balance 4.9/10 → value 44; scored fraction: 100% (362 scored, 0 abstained, internal only); critical-band edges: 115 (0 distributed-monolith: critical at high distance)
  - _What moves it:_ Reduce high-fan-in functional edges across module boundaries or introduce stable contracts.
# archfit report

**Verdict:** pass (exit 0)
**Config hash:** `55e375aa889b0ca25d032cb18bb774adb915f985ecdd28b521a9d60f6338d1b5`

## Summary

- gate findings: 0
- warnings: 82
- waivers used: 0

## Metrics

- **encapsulation**: n/a — n/a (low confidence)
- **unbalanced_edge**: 0 new high-risk unbalanced edges — strong

## Structural facts (neutral evidence)

98 modules; top 5 per axis (full list in `--format json`):

- inbound module fan-in: internal/model/diagnostic (51), internal/model/graph (38), internal/config (35), internal/model/finding (26), internal/toolrun (24)
- outbound destinations: cmd/archfit (41), internal/engine (20), internal/engine_test (16), internal/initcfg_test (14), internal/extract/astgrep_test (8)
- LOC: cmd/archfit (6542), internal/initcfg (3766), internal/engine (1976), internal/config (1663), internal/classify (1535)

## Syntax surface (neutral evidence)

1981 declaration(s) extracted by ast-grep (full list in `--format json`):

- annotation: 1
- enum: 1
- function: 1351
- interface: 17
- method: 232
- struct: 215
- type_alias: 31
- type_leak: 133
- exported (public API): 1847

Per module:

- (unscoped): 61
- cmd/archfit: 284
- cmd/calibrate: 2
- internal: 3
- internal/agenttask: 17
- internal/baseline: 19
- internal/calibrate: 16
- internal/classify: 74
- internal/config: 136
- internal/configschema: 3
- internal/decision: 34
- internal/engine: 87
- internal/extract/astgrep: 72
- internal/extract/clones: 22
- internal/extract/deployunit: 20
- internal/extract/dynimports: 7
- internal/extract/golang: 49
- internal/extract/loc: 14
- internal/extract/manifest: 14
- internal/extract/py: 20
- internal/extract/runtime: 11
- internal/extract/rust: 26
- internal/extract/scip: 39
- internal/extract/ts: 24
- internal/factcache: 38
- internal/facts: 8
- internal/history: 14
- internal/initcfg: 191
- internal/labels: 13
- internal/labels/labelsio: 5
- internal/llm: 30
- internal/metrics: 83
- internal/model: 178
- internal/output: 81
- internal/ownership: 23
- internal/ports: 61
- internal/rules: 40
- internal/scope: 28
- internal/score: 31
- internal/staleness: 11
- internal/status: 21
- internal/syntax: 13
- internal/toolrun: 35
- scripts/eval/coverage: 23

### Public API


`cmd/archfit/analyze.go` [cmd/archfit]:
- `AnalyzeCmd` (struct)
- `Help` (method)
- `Run` (method)

`cmd/archfit/autopilot_test.go` [cmd/archfit]:
- `Name` (method)
- `Complete` (method)
- `TestInit_LLMDraft_OwnerCommentWritten` (function)

`cmd/archfit/baseline.go` [cmd/archfit]:
- `BaselineCmd` (struct)
- `Help` (method)
- `Run` (method)

`cmd/archfit/byteidentical_test.go` [cmd/archfit]:
- `TestByteIdentical_SingleModule` (function)
- `TestByteIdentical_OneMemberWorkspace` (function)
- `TestByteIdentical_ColdWarmNoCache` (function)

`cmd/archfit/couplinggate_test.go` [cmd/archfit]:
- `TestCouplingGateView` (function)
- `TestRun_Analyze_CouplingGate_MinBandTrips` (function)
- `TestRun_Baseline_NoTripReasonOnStderr` (function)
- `TestRun_Analyze_CouplingGate_OffByDefault` (function)
- `TestRun_Analyze_CouplingGate_BandNANeverTrips` (function)
- `TestRun_Analyze_CouplingGate_MaxDrop` (function)
- `TestRun_Baseline_WritesScoreSnapshot` (function)
- `TestApplyCouplingGate_PromotionScope` (function)
- ... +1827 more exported declarations (use `--format json`)

## Connascence evidence (deterministic)

Report-only. Static facts only; semantic and dynamic categories without deterministic evidence stay unmeasured.

- edges with evidence: 949
- abstained edges: 4
- total evidence facts: 3604
- by kind: algorithm=849, meaning=487, name=1318, type=950
- by source: go/types=2525, scip=1079
- unmeasured: position, execution, timing, value, identity
- roadmap: name=deterministic_static, type=deterministic_static, meaning=deterministic_static, algorithm=deterministic_static, position=unmeasured_static, execution=unmeasured_dynamic (signals dynamic_imports/runtime_async_edges), timing=unmeasured_dynamic (signals dynamic_imports/runtime_async_edges), value=unmeasured_dynamic (signals dynamic_imports/runtime_async_edges), identity=unmeasured_dynamic (signals dynamic_imports/runtime_async_edges)

## Dynamic / lazy imports (hidden-coupling risk)

Report-only. Dynamic/lazy imports are invisible to the static dependency
graph, so they can hide cycles and undercount coupling.

9 sites across 3 modules (full list in `--format json`):

- **internal/extract/scip**: 6 (e.g. internal/extract/scip/scip_reader.py:63[lazy_import], internal/extract/scip/scip_reader_test.py:277[lazy_import], internal/extract/scip/scip_reader_test.py:278[lazy_import])
- **internal/extract/astgrep**: 2 (e.g. internal/extract/astgrep/testdata/integration/fixture.py:40[lazy_import], internal/extract/astgrep/testdata/integration/fixture.py:46[lazy_import])
- **internal/extract/py**: 1 (e.g. internal/extract/py/grimp_helper.py:60[lazy_import])

## Runtime async bridges (report-only)

Report-only. Runtime async evidence is grouped by module and by concrete
module→runtime-target relation; it never changes distance, score, or gate verdicts.

1 sites across 1 modules and 1 module→target relation(s). Full list in `--format json`.

- **internal/extract/runtime** → `github.com/IBM/sarama` [message_queue]: 1 (e.g. internal/extract/runtime/runtime_test.go:47[message_queue])

## Balanced Coupling advisories (74 rollups, 250 edges)

Same-shape edges between a module pair are grouped into one rollup.
Integration strength × distance × volatility lint messages.
Severity: `none` · `low` · `medium` · `high` · `critical`.

```
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/archfit/analyze.go -> internal/model/coupling  [023add20]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 25 same-shape edges (e.g. 023add20bf21c6007cb54b20b4de973a,07b9a1eb1b66a64fb3b072f6ede30bbe,0b2dbc0f272690e8c7a570abeccba563,1101eb26f63e6ca5402405104c4bac48,13a9c9667d34ede7c883fe65e7544000,1aa41e85b46a513fc461a16873e16743,23ba33f54622e001343e75d13c1fe970,37722647f1df242ab64b7ec66569f66c)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] cmd/calibrate/main.go -> internal/model/coupling  [75435625]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 2 same-shape edges (e.g. 75435625022bf51b091a639cc9f58efa,a967c418163b644b40b995788d7bd4cc)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/agenttask/agenttask.go -> internal/model/diagnostic  [bdfc0c52]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 3 same-shape edges (e.g. bdfc0c52b053c27c5bcc38b5eb3b9df3,c1c592a79be7180dae44010e42425c35,d9a01d9468996d0059ce390cadbf1b70)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/baseline/baseline.go -> internal/model/coupling  [454623ac]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 2 same-shape edges (e.g. 454623ac5dcf7a709861bba1c2f1f8ce,f971a7602d142896a56ddb2fc5e18675)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/classify/classify.go -> internal/model/coupling  [165e5b76]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 10 same-shape edges (e.g. 165e5b763556240968c09f3d592f9642,227c3973b3d3eda81d946095bc86adf4,2a96bee4c488a3cc4772a3cb20fd57c0,42a0c26c5e5ddd301021b15be867eff8,9897a63693a09fdd53ba5760e29f9a92,a406159fba3461fe434aee9f8e0af0f2,b4b2137f413cf48430fa3c88b02fe730,beaa841afba5f8e73fe10a61a3ac6178)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/config/modules.go -> internal/model/graph  [34946a9c]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 3 same-shape edges (e.g. 34946a9c3cf44598d022039b6a2b0b97,7bbb6e2ce5f3469efc15556e14421ac8,efcbaf55c8b76c71529bd7d2e62b5c37)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/decision/decision.go -> internal/model/diagnostic  [18300913]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 2 same-shape edges (e.g. 183009137b0830a764b2164146ec9e48,437bdfb6abb49e5b99bc2031c4438c78)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/engine/advisories.go -> internal/model/finding  [007ef6e3]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 7 same-shape edges (e.g. 007ef6e39304725637aa5419ed7af1a3,0ba9d83cc4f75d559f3d512133773924,25c6ddf4fc3f297909f7b07735e5c6fa,73b783ffd431541432e890093f2d0754,968e8781e3ea5cf4468a3f19454101c9,d53d28418c611d167b0bc1c9fe46bcff,ed07a896dbd1a200721defd705bb98e7)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/facts/facts.go -> internal/model/diagnostic  [a316b614]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 2 same-shape edges (e.g. a316b614d1ce2d53b345b0d31f1b9ce3,ba7e5997569250c23d98c117a78bd62a)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/metrics/boundary/coverage.go -> internal/model/diagnostic  [00ffec20]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 21 same-shape edges (e.g. 00ffec204ce4883b72c1b6796c9399a7,11953ac65d298e2934698fc7d79ca3e3,23d1a22d068f12740e77f7949fc01aa1,299aeff8824518fd8bc7df9bc0e3d06a,454128e18ac2e4438c59c39645e522fa,479ed060095f20c6c79ac77fc0689365,54c2e9bede083a1a40157655ac2c9a67,5766309fd4d1da09e5620e75af864c6e)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/output/jsonout/jsonout.go -> internal/model/coupling  [31ba1874]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 11 same-shape edges (e.g. 31ba1874b60a662b9b355f8508f3e843,3ff79c222feb620984378863a20f4ebc,499ee1de26c19ab5192d2613b165e3ae,63639586ab27ad4284dcc1bc1a9f8d6e,687b11cc0b926ed7b5b99eccfa661ecb,6988a01e7690866eb52ce9e4115f2e3a,9e63ea4afb7689a20834eae682e6d883,9f337c0ca3e09b7e6548152a8d937276)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/ports/extractor_moq.go -> internal/model/diagnostic  [0124a055]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 11 same-shape edges (e.g. 0124a055f95424a7dbf75316ad5a50d1,40b3c25955d11b5b2927306b226e942b,5b29d0650094b5a797c6ae3b5a87ed8b,72fcea69c6af0a9db863a1c9529d4fb8,749cb1951c277ee634e09f4201179779,7d4fa4746e82c06c599f2ae55b040fc3,7fe1f124aa4ac5646c14807484c28794,882d600c05087ccdcc3e0a17d130a953)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/rules/rules.go -> internal/model/diagnostic  [0fb2a210]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 8 same-shape edges (e.g. 0fb2a2103c0e0af8ba4bf1626580267f,1a90396f91f8c74db334cb80b84fac6f,217cc9b8807c0a9d91c1c12ccba6ebe1,34f2164d2065782490f679a000b4cbc6,a99c2fd244bd65dfc26c832b0f1eb6e5,ac2b8625b4f106032c382037ddce3f3f,aca2d93721cf996d8796e05b37a74304,cd35d21a79ad42ebb4acd83c11227a30)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/score/score.go -> internal/model/diagnostic  [4cadb25d]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 4 same-shape edges (e.g. 4cadb25d652cd697832922d5705f9a14,76c4086c44d878c2dfeba01fced41fe9,ada66c10ab9a5c04e6d8e6bb1db51444,e876813b021fd19143b00856301ab582)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/staleness/staleness.go -> internal/model/finding  [5b8a6c4e]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
  rollup: 2 same-shape edges (e.g. 5b8a6c4e116fa0cf7241063ff74dfd7e,d07c9d8f5aa4476c0a7a87db6fa9ea8d)
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/status/status.go -> internal/model/finding  [56c980e5]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED CRITICAL] internal/syntax/fileclass.go -> internal/model/fileclass  [976956ad]
  integration strength: model         distance: cross_module_same_owner         volatility: high
  score: 2/10 (critical) [book]
  why: balanced coupling: model integration strength × cross_module_same_owner distance × high volatility → critical severity (model coupling to a volatile target at low distance → local cascade (ch...
  cheapest move: reduce_strength
```

```
ARCHFIT[BC-UNBALANCED HIGH] internal/engine/assemble.go -> internal/model/diagnostic  [686a4227]
  integration strength: contract      distance: cross_module_same_owner         volatility: high
  score: 4/10 (high) [book]
  why: balanced coupling: contract integration strength × cross_module_same_owner distance × high volatility → high severity (contract coupling to a volatile target at low distance → cascading chang...
  rollup: 2 same-shape edges (e.g. 686a4227aa56e3f3536faa5f23ba0642,bede9acc5cc65babf90634011d7c9c6a)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/analyze.go -> internal/config  [0253289b]
  integration strength: functional    distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: functional integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 15 same-shape edges (e.g. 0253289bc1623d1c847fbb4b14db01ca,029b9c055ed2b0724c019b28844d9a66,4fb003f14c3faea28b169e0c85e9a259,58bc8aba5d9ecb08efc4e074f63ee8c5,90b6c329a2a1e1ef28f8e49d40037cb4,a993f8ba00e7797f62983e493ae1c1db,abbfe5778c068b4859b4fa62894e8190,bdbdd79ac64e2262fef55dab3fcd4956)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/analyze.go -> internal/decision  [a6f062f5]
  integration strength: functional    distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: functional integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/analyze.go -> internal/engine  [2e85c357]
  integration strength: functional    distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: functional integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 7 same-shape edges (e.g. 2e85c357a78837ee6437847d6597e135,6c5f08524e9135bb334dc9ae2ee37c4e,9452042ab3d1dd826d9fe09eb9d78b89,94aed280063bd37945a3a57011320bac,96635ffe4f92e42e9b433fb079f202fb,cb27127473df45cef48198ddc0f8d9bb,fa94b51f67a28f758634f247b99040f4)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/analyze.go -> internal/output/console  [242a8cc6]
  integration strength: functional    distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: functional integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 5 same-shape edges (e.g. 242a8cc66492e9618b2e5cabc00a265d,3d274fb8a8479e90ef68543938150453,83f06ac754fc79418d0f127b74b143cb,978820d1593945c318aa9d9220a8b2ca,b317bcad5f5ca1ef233ec9324fd61299)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/analyze.go -> internal/score  [11d815cc]
  integration strength: functional    distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: functional integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 4 same-shape edges (e.g. 11d815cc391fbb3e4c6a6898eee51cd7,8d3baeecbad359e0aec66e7cafd203fe,cd1a78c7b466486889d13993285aa57e,f85736366f2ee73e90e97832b555b6d5)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/draft_metadata.go -> internal/initcfg  [09ea5d95]
  integration strength: functional    distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: functional integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 7 same-shape edges (e.g. 09ea5d95ec52268d4b39d9ffc93ab11c,18ebbe3ec25eef143658721dfd510987,5cfeae578d37e771c6910a5c61a18319,713435dc7222cb1d64468c472eb20600,b3a81c151f702a38907f50d25d5d3be2,d0de32cf75db6cf7d8d67e7a9705556b,ed7a32d97c89c445b9f64a12432273bc)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] cmd/archfit/pipeline_config.go -> internal/rules  [5785eb4b]
  integration strength: functional    distance: cross_module_same_owner         volatility: medium
  score: 5/10 (medium) [book]
  why: balanced coupling: functional integration strength × cross_module_same_owner distance × medium volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  rollup: 2 same-shape edges (e.g. 5785eb4b39bf46ade814ce4f614006cb,a44e7ad048371eb1a19d72f56767e3b4)
```

- ... +49 more rollups (use `--format json`)

## Advisories (8)

- **bc/duplicated_knowledge** [medium] new — internal/engine/engine.go → internal/extract/golang/cache.go: duplicated knowledge: cross-module code clones between internal/engine and internal/extract/golang with no import edge — symmetric func...
- **bc/duplicated_knowledge** [medium] new — internal/extract/astgrep/astgrep.go → internal/extract/clones/clones.go: duplicated knowledge: cross-module code clones between internal/extract/astgrep and internal/extract/clones with no import edge — symme...
- **bc/duplicated_knowledge** [medium] new — internal/extract/astgrep/astgrep.go → internal/extract/rust/rust.go: duplicated knowledge: cross-module code clones between internal/extract/astgrep and internal/extract/rust with no import edge — symmetr...
- **bc/duplicated_knowledge** [medium] new — internal/extract/astgrep/astgrep.go → internal/extract/scip/scip_strength.go: duplicated knowledge: cross-module code clones between internal/extract/astgrep and internal/extract/scip with no import edge — symmetr...
- **bc/duplicated_knowledge** [medium] new — internal/extract/golang/golang.go → internal/extract/ts/ts.go: duplicated knowledge: cross-module code clones between internal/extract/golang and internal/extract/ts with no import edge — symmetric ...
- **bc/duplicated_knowledge** [medium] new — internal/extract/py/py.go → internal/extract/rust/rust.go: duplicated knowledge: cross-module code clones between internal/extract/py and internal/extract/rust with no import edge — symmetric fu...
- **bc/duplicated_knowledge** [medium] new — internal/extract/py/py.go → internal/extract/ts/ts.go: duplicated knowledge: cross-module code clones between internal/extract/py and internal/extract/ts with no import edge — symmetric func...
- **bc/duplicated_knowledge** [medium] new — internal/extract/scip/scip_strength.go → internal/initcfg/discover_py.go: duplicated knowledge: cross-module code clones between internal/extract/scip and internal/initcfg with no import edge — symmetric funct...

## Supporting structural metrics (beyond Balanced Coupling)

Report-only. These metrics support Balanced Coupling reasoning but never gate.

- **cycle**: 0 import cycles — strong
- **coverage**: 100% coverage — strong
- **blast_radius**: 7 of 60 modules are change-impact hubs: .../model/graph (75%, 44 deps), .../model/finding (58%, 34 deps), .../model/coupling (53%, 31 deps), .../model/diagnostic (53%, 31 deps), .../model/fileclass (44%, 26 deps)+2 more — info

## Distance confidence

- `code_structure`: always on (deterministic tree-distance baseline)
- `owner_source`: config
- `deploy_unit_source`: not reported (auto-detect or config-authored)
- connected modules in coupling sample: 41
- distance basis: code_structure=355, deploy_unit=7
- distance rungs implemented: D=2, D=4, D=7, D=9, D=10; omitted/compressed: D=1, D=3, D=5, D=6, D=8
- distance compression: D=3/D=5/D=6/D=8 remain compressed: current graph/config facts distinguish same module, same owner, different owner, deploy unit, and declared vendor seam, but not finer package/library distance without guessing.
- D=1 compressed: object/member-level distance is not available from module dependency edges
- D=3 compressed: current facts distinguish same module vs cross-module, but not object/package micro-distance
- D=5 compressed: package/library middle distance is not split without explicit stable package-boundary metadata
- D=6 compressed: intermediate ownership/library distance has no deterministic signal beyond owner and tree structure
- D=8 compressed: library-like seams remain compressed: undeclared libraries stay excluded, while declared external_systems score at D=10
- undeclared external/library edges excluded: 584
- clone-only duplicated knowledge: 11 scored, 0 advisory-only
- tail risk: worst balance 2/10; lower-decile balance 2/10; high-or-worse edges 117/362 (32%); critical 115; distributed-monolith 0
- clone-only tail: worst balance 5/10; high-or-worse 0/11 scored clone-only pairs

## Coverage

- scip: ok (1213 files)
- scip-symbols: ok (5942 files)
- go/packages: ok (165 files)
- dependency-cruiser: absent
- grimp: absent
- cargo: absent
- loc: ok (162 files)
- jscpd: ok (225 files)
- ast-grep: ok
- ast-grep: ok (293 files)
- cargo-modules: absent
