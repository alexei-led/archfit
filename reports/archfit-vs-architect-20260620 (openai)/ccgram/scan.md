# archfit report

**Verdict:** fail (exit 1)
**Config hash:** `a3cd0f296df93dedd9f3cc32b8c2444e020763e7d0766f33df05bd82f27510bc`

## Summary

- gate findings: 3
- warnings: 58
- exceptions used: 0

## Metrics

- **encapsulation**: 7.8/10 — mixed (low confidence)
- **unbalanced_edge**: n/a — n/a (low confidence)
- **cohesion_lcom**: n/a — n/a (low confidence)
- **architecture_fitness**: 6.7/10 (2/3 signals); arch_tests: pkg/mod/golang.org/x/tools@v0.42.0/txtar/archive_test.go, tests/ccgram/test_handler_layering_invariants.py, tests/ccgram/test_query_layer_only_for_handlers.py; ci_linter: .github/workflows/ci.yml — info
- **functional_candidates**: 41 clone-duplicated cross-module pair(s) (18 also co-change) — info
- **change_locality**: n/a — n/a (low confidence)

## Structural facts (neutral evidence)

383 modules; top 5 per axis (full list in `--format json`):

- inbound module fan-in: ccgram.config (52), ccgram.thread_router (47), ccgram.providers.base (46), ccgram.handlers.messaging_pipeline.message_sender (44), ccgram.telegram_client (43)
- outbound destinations: ccgram.handlers.text.text_handler (23), ccgram.handlers.polling.window_tick.apply (22), ccgram.handlers.registry (22), ccgram.handlers.topics.directory_callbacks (22), ccgram.handlers.status.status_bar_actions (20)
- LOC: ccgram.hook (1234), ccgram.handlers.topics.directory_callbacks (1076), ccgram.handlers.polling.polling_state (1018), ccgram.tmux_manager (956), ccgram.providers.gemini (905)

## Dynamic / lazy imports (hidden-coupling risk)

Report-only. Dynamic/lazy imports are invisible to the static dependency
graph, so they can hide cycles and undercount coupling.

788 sites across 38 modules (full list in `--format json`):

- **tests/ccgram**: 161 (e.g. tests/ccgram/conftest.py:23[lazy_import], tests/ccgram/conftest.py:43[lazy_import], tests/ccgram/conftest.py:54[lazy_import])
- **tests/ccgram/handlers/shell**: 92 (e.g. tests/ccgram/handlers/shell/test_shell_capture.py:86[lazy_import], tests/ccgram/handlers/shell/test_shell_capture.py:98[lazy_import], tests/ccgram/handlers/shell/test_shell_capture.py:111[lazy_import])
- **tests/ccgram/handlers/polling**: 84 (e.g. tests/ccgram/handlers/polling/test_polling_strategies.py:311[lazy_import], tests/ccgram/handlers/polling/test_polling_strategies.py:448[lazy_import], tests/ccgram/handlers/polling/test_status_polling.py:492[lazy_import])
- **tests/ccgram/providers**: 62 (e.g. tests/ccgram/providers/conftest.py:14[lazy_import], tests/ccgram/providers/test_codex_format.py:140[lazy_import], tests/ccgram/providers/test_registry.py:64[lazy_import])
- **tests/integration**: 45 (e.g. tests/integration/conftest.py:24[lazy_import], tests/integration/test_autodetect_integration.py:63[lazy_import], tests/integration/test_message_dispatch.py:64[lazy_import])
- **src/ccgram**: 40 (e.g. src/ccgram/bootstrap.py:118[lazy_import], src/ccgram/bootstrap.py:226[lazy_import], src/ccgram/bootstrap.py:251[lazy_import])
- **tests/ccgram/handlers/interactive**: 33 (e.g. tests/ccgram/handlers/interactive/test_interactive_ui.py:145[lazy_import], tests/ccgram/handlers/interactive/test_interactive_ui.py:150[lazy_import], tests/ccgram/handlers/interactive/test_interactive_ui.py:159[lazy_import])
- **tests/ccgram/handlers/status**: 27 (e.g. tests/ccgram/handlers/status/test_status_bar_actions.py:146[lazy_import], tests/ccgram/handlers/status/test_status_bar_actions.py:165[lazy_import], tests/ccgram/handlers/status/test_status_bar_actions.py:172[lazy_import])
- **tests/ccgram/handlers**: 26 (e.g. tests/ccgram/handlers/conftest.py:10[lazy_import], tests/ccgram/handlers/conftest.py:27[lazy_import], tests/ccgram/handlers/conftest.py:28[lazy_import])
- **src/ccgram/handlers**: 24 (e.g. src/ccgram/handlers/agent_command.py:172[lazy_import], src/ccgram/handlers/agent_command.py:175[lazy_import], src/ccgram/handlers/agent_command.py:211[lazy_import])
- ... +28 more modules (use `--format json`)

## Gate findings (3)

- **no-import-cycles** [high] new — ccgram.bootstrap → ccgram.bot: Import cycle detected among 6 nodes: module:ccgram.bootstrap → module:ccgram.bot → module:ccgram.cli → module:ccgram.handlers.regis...
- **no-import-cycles** [medium] new — ccgram.handlers.agent_command → ccgram.handlers.callback_registry: Import cycle detected among 28 nodes: module:ccgram.handlers.agent_command → module:ccgram.handlers.callback_registry → module:ccgram...
- **no-import-cycles** [medium] new — ccgram.providers.process_detection → ccgram.providers.shell: Import cycle detected among 3 nodes: module:ccgram.providers.process_detection → module:ccgram.providers.shell → module:ccgram.provid...

## Agent tasks (3)

- **no-import-cycles** [`ae4ef4b3`] Break the import cycle: Import cycle detected among 28 nodes: module:ccgram.handlers.agent_command → module:ccgram.handlers.callback_registry → module:ccgram.handlers.cleanup → module:ccgram.handlers.hook_events → module:ccgram.handlers.interactive → module:ccgram.handlers.interactive.interactive_callbacks → module:ccgram.handlers.live.live_view → module:ccgram.handlers.live.pane_callbacks → module:ccgram.handlers.live.screenshot_callbacks → module:ccgram.handlers.messaging_pipeline.message_queue → module:ccgram.handlers.messaging_pipeline.tool_batch → module:ccgram.handlers.recovery.history_callbacks → module:ccgram.handlers.recovery.recovery_banner → module:ccgram.handlers.recovery.recovery_callbacks → module:ccgram.handlers.recovery.resume_command → module:ccgram.handlers.recovery.resume_picker → module:ccgram.handlers.send.send_callbacks → module:ccgram.handlers.sessions_dashboard → module:ccgram.handlers.shell.shell_commands → module:ccgram.handlers.shell.shell_prompt_orchestrator → module:ccgram.handlers.status.status_bar_actions → module:ccgram.handlers.status.status_bubble → module:ccgram.handlers.sync_command → module:ccgram.handlers.toolbar.toolbar_callbacks → module:ccgram.handlers.topics.directory_callbacks → module:ccgram.handlers.topics.topic_orchestration → module:ccgram.handlers.topics.window_callbacks → module:ccgram.handlers.voice.voice_callbacks
  - files: ccgram.handlers.agent_command, ccgram.handlers.callback_registry
  - constraint: Break the cycle by introducing an abstraction or reorganizing packages
  - validate: `archfit check -c .archfit.alltools.llm.yaml --full`
- **no-import-cycles** [`b64908dd`] Break the import cycle: Import cycle detected among 3 nodes: module:ccgram.providers.process_detection → module:ccgram.providers.shell → module:ccgram.providers.shell_infra
  - files: ccgram.providers.process_detection, ccgram.providers.shell
  - constraint: Break the cycle by introducing an abstraction or reorganizing packages
  - validate: `archfit check -c .archfit.alltools.llm.yaml --full`
- **no-import-cycles** [`cac3cea9`] Break the import cycle: Import cycle detected among 6 nodes: module:ccgram.bootstrap → module:ccgram.bot → module:ccgram.cli → module:ccgram.handlers.registry → module:ccgram.handlers.upgrade → module:ccgram.main
  - files: ccgram.bootstrap, ccgram.bot
  - constraint: Break the cycle by introducing an abstraction or reorganizing packages
  - validate: `archfit check -c .archfit.alltools.llm.yaml --full`

## Balanced Coupling advisories (58 rollups, 421 edges)

Same-shape edges between a module pair are grouped into one rollup.
Integration strength × distance × volatility lint messages.
Severity: `none` · `low` · `medium` · `high` · `critical`.

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.bootstrap -> ccgram.cc_commands  [9f786f45]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
  rollup: 3 same-shape edges (e.g. 9f786f45a67370d0185b21b6d91eddbc,bd286153958c7c3686062ac50fab96d2,f4d916da56bd4b725ea5ead61292a5ae)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.bootstrap -> ccgram.handlers.messaging_pipeline.message_routing  [482ff4aa]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
  rollup: 5 same-shape edges (e.g. 482ff4aadd8fb86c1656e23f6c3a88c5,5dd111882e64418bf46154025e58da8c,661cdfe21ff638b83d62f0bb591945e4,e3076822e8913d8eb3317b1513746001,e81d34eca711260b4c2fe274bdaa3de2)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.bootstrap -> ccgram.handlers.shell.shell_capture  [1c29162e]
  integration strength: intrusive     distance: cross_module_different_owner    volatility: undeclared
  score: 3/10 (low) [multiplicative]
  why: balanced coupling: intrusive integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.bootstrap -> ccgram.session_monitor  [1918371e]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
  rollup: 2 same-shape edges (e.g. 1918371e177d8481be942751790d1a06,5c4fa4f3be15b37fcc63f7554feef829)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.bootstrap -> ccgram.utils  [145d0375]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
  rollup: 6 same-shape edges (e.g. 145d0375f7a4b4b94ec2b060d6fea6c4,2ac4efa00248d7c424f278bdf99ff2c1,324c6a7a0859035e623843ba6e48e524,533c171a747c1c67566907055d0a89ce,b693778bd53467d36e4c5d6c9767a787,ecde5101110114cde5d6e764160994c5)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.bot -> ccgram.bootstrap  [dbab0d49]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.bot -> ccgram.handlers.topics.new_command  [10b1ff2b]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
  rollup: 8 same-shape edges (e.g. 10b1ff2b808c6d0d4ec5a4d89780f2d5,167fb97197ad5e049b45ed88c2c32a2e,22d936a9f3a6ff6c9e6dd0e90aa0d59b,36e8a06b2363a797abda01cf9f230909,4c36d30723bc1a30e8d3884df984731a,94b917a67680708dace81a63c0fb59e1,a30617c5e3b73b43e1f4bcdb61cec1ef,ce15cef11f178f56b07dfd7ddc3eefc6)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.bot -> ccgram.session  [5be6241a]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.bot -> ccgram.thread_router  [d7064382]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.cc_commands -> ccgram.config  [04a4822c]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
  rollup: 5 same-shape edges (e.g. 04a4822c6b9310794e405bbf56934f9a,0ef06b82fabb0f281088e1de6636238c,47165a1620a6a9bf0f4c117f3dd8d3d7,bd8a91839fba45453c507ef014c74d37,fd360e8242b5eaaa9b1d9074bb2c719c)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.cc_commands -> ccgram.providers.base  [ff10a55d]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.command_catalog -> ccgram.providers.base  [7a4179fa]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.handlers.file_handler -> ccgram.config  [03bb613a]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
  rollup: 63 same-shape edges (e.g. 03bb613aff6d9c2ab68396bf0bd85925,03df9960a43d3aaee6058ff85d413d75,0576f23848976a00a15b44ebff3ca46f,07ce5480170028b3bf9384a328a87adf,0902c25d54b28f839f5caa39ef753d8b,0a0855c435e4113c5df04513da1c5ded,101d25b21a4a2f4e1afa4fa08cb5cf03,10b950816fff9d9254caeaafd00a13b1)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.handlers.hook_events -> ccgram.window_query  [046e74ac]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
  rollup: 38 same-shape edges (e.g. 046e74ac5a921d00d1f11f6a392899b6,08b85ff6c9ba4ddab82e2ea9879a1114,19f3188b107d00bf3e54a1867c13a8a9,23c00283dffc5e63cd885f4d473ec68e,2fed543611e0f974b4c82b52bb0b707d,33ef1562e713644f3037ffec9f695fe6,4129f439828f7bc856c7721f733c4aa4,4310fae97e8c4d2f4d1e9f662cc120db)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.handlers.interactive.interactive_ui -> ccgram.providers  [0cf299aa]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
  rollup: 31 same-shape edges (e.g. 0cf299aa17938540bf24bf01e0cef56f,14e16423827da32542e109a56ee1ee98,15d5510bedd6b705c4852acaa0ae2a37,1ca2930b777f01c327b1c1e448317ad8,21ebcec230e540c49a63294cc1515fa2,2839bb14404c642252e5b5112b88af39,33c541b9b2b16a9cc0da0e90438af426,343eb6465e07042c319f9c0e67560a03)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.handlers.interactive.interactive_ui -> ccgram.window_state_ports.pane_state  [2253f123]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
  rollup: 10 same-shape edges (e.g. 2253f123b7f3d120692d59e6d725c684,2eb9766d1ac410439f86f75945b947dc,51759f62c26e07e245ea194826d62826,53ac35594ed981f0bf879d15cbd94c47,85716c519187a6e650a7c18da134cc29,8bb76a031067a1ee06ecbc1af8b163dc,9d640c8a6bf779dde350752c07bc1f79,b8b4dab8f3b545703deea025397a2bd8)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.handlers.live.screenshot_callbacks -> ccgram.telegram_client  [036aaf68]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
  rollup: 27 same-shape edges (e.g. 036aaf68a4535c05fa8dfcf9c60bb809,09bb71948332997707d33cb0defe4a8b,0da9a8c50862608d0b93f1268b492875,23764bf85b750de973a2ca736a55c34b,277ca1062c145e81b18566b0ba45b048,2a8f7b22be9a3b779288d78de2bf350f,4064d0103a36880cacf4649c65aa494e,492ed37aafa0cb70a4d3ad77dba065c8)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.handlers.messaging_pipeline.message_queue -> ccgram.tts  [1d4fc82f]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.handlers.messaging_pipeline.tool_batch -> ccgram.claude_task_state  [1e20af2b]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
  rollup: 38 same-shape edges (e.g. 1e20af2b9dcbccf4d21114effdc68c4c,25b94aa1a12ab6312e8bf5473e16e8af,26581d2256411d4236956e0df391fc4c,2e1a7fd34afae89bd4e022c3b8673ff6,321bc4abf08202c5eeec59fad0a9afe1,3700eb5633c74ef476f1d3b5e4df48c8,37592a4f99b3dc230a9084715bef9a30,3f6e1917149a4ca70236fd71da471422)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.handlers.polling.polling_state -> ccgram.terminal_parser  [3cda2ebb]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
  rollup: 4 same-shape edges (e.g. 3cda2ebbf56d5b701a891b23fb1b1556,4cd4a318e9ccb64f38b1acae21102a5c,d5bd7ad873015bad826a5462bfcb0457,d5f89367485aa0c44d15af17cf1e4e88)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.handlers.shell.shell_commands -> ccgram.llm  [82a569a5]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
  rollup: 3 same-shape edges (e.g. 82a569a5e046f7587c4e6224ece91fd9,9e61a18b7461581956025c97519fe599,d9d5b8889618e09a049acb177b7fb14c)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.handlers.status.status_bar_actions -> ccgram.miniapp.auth  [952e2008]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.handlers.toolbar.toolbar_callbacks -> ccgram.tmux_manager  [006621c8]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
  rollup: 76 same-shape edges (e.g. 006621c8ce78f2098f3a134ec2e203aa,06956b54c890d00f6cc7d15e7514547f,0aefe20678d9cdce8e7ac697aa321313,0b68054c0b1accc0dccf81a27a0cac6b,0e441ea90600b4b45a95d2798d307c55,1685579b1294977222b54c9944f35188,16f1d3f46ca8cfb9d2ce4dc0285c74ca,1b406f4e634b7609d404bd140a2187ee)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.handlers.upgrade -> ccgram.main  [f5290040]
  integration strength: intrusive     distance: cross_module_different_owner    volatility: undeclared
  score: 3/10 (low) [multiplicative]
  why: balanced coupling: intrusive integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
```

```
ARCHFIT[BC-UNBALANCED MEDIUM] ccgram.handlers.voice.voice_handler -> ccgram.whisper.base  [7d23630f]
  integration strength: functional    distance: cross_module_different_owner    volatility: undeclared
  score: 5/10 (medium) [multiplicative]
  why: balanced coupling: functional integration strength × cross_module_different_owner distance × undeclared volatility → medium severity (unbalanced coupling → elevated maintenance effort)
  cheapest move: declare_volatility
  rollup: 2 same-shape edges (e.g. 7d23630fd5f2f85430675d8032c221c9,8ceb5109cca2b02b7f620587fae8fd1d)
```

- ... +33 more rollups (use `--format json`)

## Supporting structural metrics (beyond Balanced Coupling)

Report-only. These metrics support Balanced Coupling reasoning but never gate.

- **cycle**: 3 import cycles — critical
- **coverage**: 100% coverage — strong
- **blast_radius**: 115 change-impact hub(s): ccgram.utils (68%, 116 deps), ccgram.config (63%, 108 deps), .../providers/base (59%, 101 deps), ccgram.expandable_quote (56%, 96 deps), ccgram.thread_router (55%, 94 deps)+110 more — info
- **change_amplification**: 2 volatile hub(s): ccgram.config (amp 0.23: 108 deps × 25 commits), ccgram.tmux_manager (amp 0.17: 87 deps × 24 commits) — info
- **hidden_coupling**: 3 hidden-coupling pair(s); top: .../providers/codex (2), ccgram.cli (1), ccgram.config (1), .../providers/gemini (1) — info
- **structural_weight**: 8 god-module(s) (median 174 LOC): ccgram.hook (1234 LOC, 7x), .../topics/directory_callbacks (1076 LOC, 6x), .../polling/polling_state (1018 LOC, 5x), ccgram.tmux_manager (956 LOC, 5x), .../providers/gemini (905 LOC, 5x)+3 more — info
- **complexity**: 27 complex function(s) (CCN>15): parse_entries CCN 72 (.../ccgram/transcript_parser.py:401), format_tool_use_summary CCN 28 (.../ccgram/transcript_parser.py:165), audit_state CCN 28 (.../ccgram/session.py:337), _apply_ansi_codes CCN 27 (.../ccgram/screenshot.py:182), _process_hook_stdin CCN 22 (.../ccgram/hook.py:1115)+22 more — info
- **risk_hub**: 158 risk hub(s): ccgram.window_state_store [breadth 73, ×1.00, gn×1.43→104.47], ccgram.telegram_client [breadth 69, ×1.00, gn×1.45→99.93], .../providers/base [breadth 71, ×1.00, gn×1.22→86.91], ccgram.tmux_manager [breadth 47, ×1.00, gn×1.72→81.03], .../handlers/callback_data [breadth 72, ×1.00, gn×1.00→72.00]+153 more — info
- **instability**: 73 unstable modules (I>0.7): .../handlers/polling (I=1.00), .../handlers/recovery (I=1.00), .../handlers/status (I=1.00), .../handlers/text (I=1.00), .../handlers/topics (I=1.00)+68 more — info
- **propagation_cost**: PC=0.263 (N=172); 54 modules reach >50%: ccgram.bootstrap (96%), ccgram.bot (96%), ccgram.cli (96%), .../handlers/registry (96%), .../handlers/upgrade (96%)+49 more — info
- **change_coupling**: 2 change-coupled pair(s) (CC≥65%); top: .../providers/codex↔.../providers/gemini (CC=67%), .../providers/codex↔.../providers/pi (CC=67%) — info

> Low-confidence proxies (footnote — full values in `--format json`).
> Derived without SCIP type kinds; do not read as authoritative.
> - abstractness: 0 modules with A>0.5 — info (low confidence)
> - martin_distance: 62 modules in zone of pain/uselessness (Dms>0.5): ccgram._version (Dms=1.00, A=0.00, I=0.00), ccgram.expandable_quote (Dms=1.00, A=0.00, I=0.00), .../handlers/callback_data (Dms=1.00, A=0.00, I=0.00), .../messaging_pipeline/message_task (Dms=1.00, A=0.00, I=0.00), .../topics/worktree (Dms=1.00, A=0.00, I=0.00)+57 more — info (low confidence)

## Distance confidence

- `code_structure`: always on (deterministic tree-distance baseline)
- `owner_source`: not reported (CODEOWNERS or git-author fallback)
- `deploy_unit_source`: not reported (auto-detect or config-authored)

## Coverage

- scip: ok (2658 files)
- scip-symbols: ok (19817 files)
- go/packages: absent
- dependency-cruiser: absent
- grimp: ok (172 files)
- loc: ok (819 files)
- lizard: ok
- jscpd: ok (439 files)
- gitnexus: ok (144 files)
- ast-grep: ok
