# archfit report

**Verdict:** fail (exit 1)
**Config hash:** `38fd9d5005aab980d694fc9b99ee73c7f55d76eae11c5da3fc24961b629c6a5d`

## Summary

- gate findings: 3
- warnings: 0
- exceptions used: 0

## Metrics

- **encapsulation**: 7.8/10 — mixed (low confidence)
- **unbalanced_edge**: n/a — n/a (low confidence)
- **cohesion_lcom**: n/a — n/a (low confidence)
- **architecture_fitness**: 6.7/10 (2/3 signals); arch_tests: tests/ccgram/test_handler_layering_invariants.py, tests/ccgram/test_query_layer_only_for_handlers.py; ci_linter: .github/workflows/ci.yml — info
- **functional_candidates**: n/a — n/a (low confidence)
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

## Coverage gaps (4)

Analyzers that did not run. Their metrics are reported as n/a (never green) — install to measure them.

- **dependency-cruiser** [gate: warn] — affects coverage, coupling_balance, encapsulation, cycle, blast_radius
  - install: `npm install -g dependency-cruiser`
- **go/packages** [gate: warn] — affects coverage, coupling_balance, encapsulation, cycle, blast_radius
  - install: `https://go.dev/dl (bundled with the Go toolchain)`
- **jscpd** [gate: warn] — affects functional_candidates
  - install: `npm install -g jscpd`
- **lizard** [gate: warn] — affects complexity
  - install: `uv tool install lizard / pip install lizard`

## Config warnings (15)

Advisory — never gates. Under-specified modules degrade distance/volatility classification.

- module "app_bootstrap" omits owner, subdomain/volatility
- module "domain_config" omits owner, subdomain/volatility
- module "handlers" omits owner, subdomain/volatility
- module "hooks" omits owner, subdomain/volatility
- module "llm" omits owner, subdomain/volatility
- module "miniapp" omits owner, subdomain/volatility
- module "providers" omits owner, subdomain/volatility
- module "session_state" omits owner, subdomain/volatility
- module "telegram_adapter" omits owner, subdomain/volatility
- module "tmux_adapter" omits owner, subdomain/volatility
- module "transcript" omits owner, subdomain/volatility
- module "tts" omits owner, subdomain/volatility
- module "whisper" omits owner, subdomain/volatility
- module "window_state" omits owner, subdomain/volatility
- module "window_state_ports" omits owner, subdomain/volatility

## Gate findings (3)

- **no-import-cycles** [high] new — ccgram.bootstrap → ccgram.bot: Import cycle detected among 6 nodes: module:ccgram.bootstrap → module:ccgram.bot → module:ccgram.cli → module:ccgram.handlers.regis...
- **no-import-cycles** [medium] new — ccgram.handlers.agent_command → ccgram.handlers.callback_registry: Import cycle detected among 28 nodes: module:ccgram.handlers.agent_command → module:ccgram.handlers.callback_registry → module:ccgram...
- **no-import-cycles** [medium] new — ccgram.providers.process_detection → ccgram.providers.shell: Import cycle detected among 3 nodes: module:ccgram.providers.process_detection → module:ccgram.providers.shell → module:ccgram.provid...

## Agent tasks (3)

- **no-import-cycles** [`ae4ef4b3`] Break the import cycle: Import cycle detected among 28 nodes: module:ccgram.handlers.agent_command → module:ccgram.handlers.callback_registry → module:ccgram.handlers.cleanup → module:ccgram.handlers.hook_events → module:ccgram.handlers.interactive → module:ccgram.handlers.interactive.interactive_callbacks → module:ccgram.handlers.live.live_view → module:ccgram.handlers.live.pane_callbacks → module:ccgram.handlers.live.screenshot_callbacks → module:ccgram.handlers.messaging_pipeline.message_queue → module:ccgram.handlers.messaging_pipeline.tool_batch → module:ccgram.handlers.recovery.history_callbacks → module:ccgram.handlers.recovery.recovery_banner → module:ccgram.handlers.recovery.recovery_callbacks → module:ccgram.handlers.recovery.resume_command → module:ccgram.handlers.recovery.resume_picker → module:ccgram.handlers.send.send_callbacks → module:ccgram.handlers.sessions_dashboard → module:ccgram.handlers.shell.shell_commands → module:ccgram.handlers.shell.shell_prompt_orchestrator → module:ccgram.handlers.status.status_bar_actions → module:ccgram.handlers.status.status_bubble → module:ccgram.handlers.sync_command → module:ccgram.handlers.toolbar.toolbar_callbacks → module:ccgram.handlers.topics.directory_callbacks → module:ccgram.handlers.topics.topic_orchestration → module:ccgram.handlers.topics.window_callbacks → module:ccgram.handlers.voice.voice_callbacks
  - files: ccgram.handlers.agent_command, ccgram.handlers.callback_registry
  - constraint: Break the cycle by introducing an abstraction or reorganizing packages
  - validate: `archfit check -c /Users/alexei/Workspace/ccgram/.archfit.yaml --full`
- **no-import-cycles** [`b64908dd`] Break the import cycle: Import cycle detected among 3 nodes: module:ccgram.providers.process_detection → module:ccgram.providers.shell → module:ccgram.providers.shell_infra
  - files: ccgram.providers.process_detection, ccgram.providers.shell
  - constraint: Break the cycle by introducing an abstraction or reorganizing packages
  - validate: `archfit check -c /Users/alexei/Workspace/ccgram/.archfit.yaml --full`
- **no-import-cycles** [`cac3cea9`] Break the import cycle: Import cycle detected among 6 nodes: module:ccgram.bootstrap → module:ccgram.bot → module:ccgram.cli → module:ccgram.handlers.registry → module:ccgram.handlers.upgrade → module:ccgram.main
  - files: ccgram.bootstrap, ccgram.bot
  - constraint: Break the cycle by introducing an abstraction or reorganizing packages
  - validate: `archfit check -c /Users/alexei/Workspace/ccgram/.archfit.yaml --full`

## Supporting structural metrics (beyond Balanced Coupling)

Report-only. These metrics support Balanced Coupling reasoning but never gate.

- **cycle**: 3 import cycles — critical
- **coverage**: 100% coverage — strong
- **blast_radius**: 115 change-impact hub(s): ccgram.utils (68%, 116 deps), ccgram.config (63%, 108 deps), .../providers/base (59%, 101 deps), ccgram.expandable_quote (56%, 96 deps), ccgram.thread_router (55%, 94 deps)+110 more — info
- **change_amplification**: 2 volatile hub(s): ccgram.config (amp 0.23: 108 deps × 25 commits), ccgram.tmux_manager (amp 0.17: 87 deps × 24 commits) — info
- **hidden_coupling**: 3 hidden-coupling pair(s); top: .../providers/codex (2), ccgram.cli (1), ccgram.config (1), .../providers/gemini (1) — info
- **structural_weight**: 8 god-module(s) (median 174 LOC): ccgram.hook (1234 LOC, 7x), .../topics/directory_callbacks (1076 LOC, 6x), .../polling/polling_state (1018 LOC, 5x), ccgram.tmux_manager (956 LOC, 5x), .../providers/gemini (905 LOC, 5x)+3 more — info
- **complexity**: n/a — n/a (low confidence)
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
- lizard: absent — complexity is opt-in — set `tools.complexity.enabled: true` in .archfit.yaml
- jscpd: absent — clone detection is opt-in — set `tools.clones.enabled: true` in .archfit.yaml
- gitnexus: ok (144 files) — gitnexus index auto-detected (.gitnexus/.codegraph present); refresh with `node .gitnexus/run.cjs analyze --index-only`
- ast-grep: ok
