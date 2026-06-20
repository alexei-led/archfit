# ccgram architect-skill quick sweep

Mode: read-only architecture quick sweep. No final architecture scores assigned. Working model is reconstructed from docs, manifests, source samples, GitNexus, grimp, selected checks, and existing archfit artifacts.

## 1. Scope, refs, dirty-state risk

- Target repo: `/Users/alexei/Workspace/ccgram`.
- Output artifact: `/Users/alexei/Workspace/archfit/reports/archfit-vs-architect-20260620 (openai)/ccgram/architect-sweep.md`.
- Branch/ref: `main`, `HEAD=a6b9ba75e5440c95402310aba9ec2d43dad858fe`.
- Baseline used: `v3.5.1`, because the requested baseline expected it and existing archfit `delta.json` records `base: v3.5.1`. Note: nearest tag before `HEAD` is `v3.5.2`.
- Dirty state at review start and end:
  - Modified tracked docs: `CLAUDE.md`, `docs/presentaions/slides.md`.
  - Untracked archfit config/cache: `.archfit-cache/`, `.archfit-labels.yaml`, `.archfit.yaml`. Existing archfit artifacts reference a full config, but `.archfit.full.yaml` is not present in the current working tree after verification.
  - No tracked production-code dirty files. GitNexus `detect_changes(scope=all, repo=ccgram)` reported only the two tracked doc symbols and `risk_level: low`.
- Source was not modified. Tool cache writes were redirected when applicable (`RUFF_CACHE_DIR=$TMPDIR/ccgram-ruff-cache`) or stayed outside the repo (`uvx` cache).

## 2. Intent evidence with file refs

- Product intent: CCGram bridges Telegram Forum topics to tmux windows so users can monitor and control agent CLIs from Telegram; the terminal/tmux pane remains the source of truth (`README.md:11`, `README.md:19`, `README.md:25`, `README.md:67`).
- Core invariant: `1 Topic = 1 Window = 1 Session`, keyed by tmux window ID, not window name (`CLAUDE.md:33`, `.claude/rules/topic-architecture.md:1`).
- Topic-only constraint: no non-topic/General-topic backward compatibility (`CLAUDE.md:34`).
- Formatting/queue constraints: send-layer splitting only, entity-based Telegram formatting, per-user FIFO queue, hook-based session tracking (`CLAUDE.md:35-38`).
- Provider intent: per-window `AgentProvider` resolution under `src/ccgram/providers/`, with provider capability gates (`CLAUDE.md:92-95`, `.claude/rules/architecture.md:205`).
- Boundary intent: handlers depend on `TelegramClient` Protocol, not raw `telegram.Bot`; window/session reads go through query/port layers; polling has pure decision kernels (`.claude/rules/architecture.md:209-213`).
- Tooling intent: Python 3.14 project with ruff, pyright, deptry, pytest, uv (`pyproject.toml:7`, `pyproject.toml:21-57`, `pyproject.toml:76-132`; `Makefile:1-30`; `.github/workflows/ci.yml:1-26`).
- Archfit intent artifact: untracked `.archfit.yaml` defines `core` and `adapter` layers, modules such as `handlers`, `providers`, `telegram_adapter`, `tmux_adapter`, `app_bootstrap`, and currently has `no-import-cycles`/`no-forbidden-deps` as `gate: warn` (`.archfit.yaml:1`, `.archfit.yaml:19-34`, `.archfit.yaml:120-156`). Treat this as draft intent, not enforced source truth.

## 3. System map

- Language/package: Python 3.14, uv/hatch project (`pyproject.toml`).
- Package shape: `src/ccgram` has 173 Python files, 24 packages, about 34,592 non-comment LOC from the quick count.
- Deploy/runtime units:
  - CLI entrypoint: `ccgram = ccgram.main:main` (`pyproject.toml:42-43`).
  - Telegram bot process: PTB application built in `src/ccgram/bot.py` and lifecycle wiring in `src/ccgram/bootstrap.py`.
  - tmux runtime dependency: `src/ccgram/tmux_manager.py`, `src/ccgram/thread_router.py`.
  - Hook subprocess: `src/ccgram/hook.py` handles provider hook stdin and session-map/event files.
  - Optional Mini App: `src/ccgram/miniapp/`, disabled unless `CCGRAM_MINIAPP_BASE_URL` is set (`README.md:227-243`).
- Major module clusters:
  - `handlers/`: Telegram command/callback/message orchestration; 85 Python files.
  - `providers/`: Claude/Codex/Gemini/Pi/Shell provider abstraction; 15 files.
  - `window_state_ports/`: feature ports over `WindowStateStore`; 6 files.
  - `llm/`, `whisper/`, `tts/`, `miniapp/`: optional capability adapters.
  - Core state: `session.py`, `session_map.py`, `session_monitor.py`, `window_state_store.py`, `thread_router.py`, `window_query.py`.
- Public interfaces and seams:
  - `AgentProvider` / `ProviderCapabilities` in `providers/base.py`.
  - `TelegramClient` Protocol in `telegram_client.py`.
  - `window_query` / `session_query` read projections and `window_state_ports/*` feature ports.
  - `TopicLifecycleStrategy`, polling `decide/observe/apply` split.

## 4. Tool coverage and commands run

### Tools used

- Git/GitNexus:
  - `git -C /Users/alexei/Workspace/ccgram status --short`
  - `git -C /Users/alexei/Workspace/ccgram rev-parse HEAD`
  - `git -C /Users/alexei/Workspace/ccgram describe --tags --abbrev=0`
  - `git -C /Users/alexei/Workspace/ccgram diff --stat v3.5.1..HEAD`
  - `git -C /Users/alexei/Workspace/ccgram diff --name-status v3.5.1..HEAD`
  - `git -C /Users/alexei/Workspace/ccgram log --oneline --decorate --name-status v3.5.1..HEAD --max-count=8`
  - `gitnexus_list_repos` showed `ccgram` index at `a6b9ba75e5440c95402310aba9ec2d43dad858fe`, not stale.
  - `gitnexus_detect_changes({scope:"all", repo:"ccgram"})`
  - `gitnexus_query` for Pi followup, stale dead autoclose, stale session-map refresh.
  - `gitnexus_context` for `_handle_pi_followup_command`, `_refresh_session_map_if_stale`, `_close_expired_topic`.
- Python/project checks:
  - `cd /Users/alexei/Workspace/ccgram && TMPDIR=${TMPDIR:-/tmp} RUFF_CACHE_DIR=$TMPDIR/ccgram-ruff-cache uv run --no-sync ruff check .` → passed.
  - `cd /Users/alexei/Workspace/ccgram && uv run --no-sync ruff format --check src/ tests/` → `380 files already formatted`.
  - `cd /Users/alexei/Workspace/ccgram && uv run --no-sync pyright` → `0 errors`.
  - `cd /Users/alexei/Workspace/ccgram && uv run --no-sync deptry src` → no dependency issues.
  - `cd /Users/alexei/Workspace/ccgram && uv run --no-sync python scripts/lint_lazy_imports.py` → no undocumented in-function imports.
  - `cd /Users/alexei/Workspace/ccgram && uv run --no-sync pytest tests/ccgram/test_query_layer_only_for_handlers.py tests/ccgram/test_window_state_access_audit.py tests/ccgram/test_window_store_import_boundary.py tests/integration/test_import_no_cycles.py tests/ccgram/handlers/polling/test_polling_types_purity.py -q` → `366 passed`.
  - `cd /Users/alexei/Workspace/ccgram && uv tree --depth 2` → 51 resolved packages, expected runtime/dev deps.
- Dependency graph:
  - `cd /Users/alexei/Workspace/ccgram && PYTHONPATH=src uvx --from grimp python - <<'PY' ... grimp.build_graph("ccgram") ... PY` → 173 modules; top hubs and cycles summarized below.
- Existing archfit artifacts read:
  - `reports/.../ccgram/scan.md`
  - `reports/.../ccgram/scorecard.md`
  - `reports/.../ccgram/delta-scorecard.md`
  - `reports/.../ccgram/full.json`
  - `reports/.../ccgram/delta.json`
  - `reports/.../ccgram/doctor.txt`

### Tools missing/failed/skipped

- `uvx --from import-linter lint-imports` failed with `Could not read any configuration`; no import-linter contracts are configured.
- `uvx pydeps ...` did not produce useful output in this quick pass; grimp and archfit artifacts covered the module graph instead.
- LSP/tree-sitter and full codegraph were not run; quick sweep used GitNexus, grimp, file reads, and selected tests.
- Full `make check` was not run. Closest gate coverage was ruff format/check, pyright, deptry, lazy-import lint, and selected architecture/import tests.

## 5. Full-current architecture observations

- The documented working model and implementation shape broadly align: Telegram handlers drive tmux, session monitor consumes transcripts/events, providers own CLI-specific behavior, and source docs explicitly name query/port seams (`.claude/rules/architecture.md:15-213`).
- The current project has executable fitness around some architectural seams:
  - CI runs format, lint, typecheck, deptry, unit tests, and integration tests (`.github/workflows/ci.yml:14-26`).
  - Handler state access is guarded by AST tests (`tests/ccgram/test_query_layer_only_for_handlers.py:1-16`, `tests/ccgram/test_window_state_access_audit.py:1-16`, `tests/ccgram/test_window_store_import_boundary.py:1-24`).
  - Polling purity is documented and tested (`.claude/rules/architecture.md:210-211`).
  - The selected architecture/import test subset passed: `366 passed in 36.05s`.
- Static dependency graph still has high-friction edges:
  - Grimp module count: 173.
  - Top inbound hubs: `ccgram.config` 52, `ccgram.thread_router` 47, `ccgram.handlers.messaging_pipeline.message_sender` 45, `ccgram.telegram_client` 43, `ccgram.tmux_manager` 41.
  - Top outbound modules: `ccgram.handlers.callback_registry` 23, `ccgram.handlers.polling.window_tick.apply` 21, `ccgram.handlers.registry` 21, `ccgram.handlers.text.text_handler` 21, `ccgram.handlers.topics.directory_callbacks` 21.
- Three static cycles match archfit's gate findings, but runtime import tests pass because several edges are intentionally lazy:
  - `ccgram.bootstrap -> ccgram.main -> ccgram.bot -> ccgram.bootstrap`; evidence: `bot.py` imports `.bootstrap` at `src/ccgram/bot.py:24`; `bootstrap.py` lazy-imports `.main` with cycle comments at `src/ccgram/bootstrap.py:223-226` and `250-251`.
  - `ccgram.providers.process_detection -> ccgram.providers.shell -> ccgram.providers.shell_infra -> ccgram.providers.process_detection`; evidence: `process_detection.py` imports `shell.KNOWN_SHELLS` at `src/ccgram/providers/process_detection.py:25`; `shell.py` re-exports `shell_infra` at `src/ccgram/providers/shell.py:21-29`.
  - Handler callback cycle starts at `agent_command -> callback_registry`, and `callback_registry.load_handlers()` imports callback-bearing handler modules for registration side effects (`src/ccgram/handlers/agent_command.py:35`, `src/ccgram/handlers/callback_registry.py:113-124`).
- This is not a runtime failure finding from the quick sweep. It is a confirmed static dependency-health and boundary-friction observation. The repo accepts lazy imports when annotated; archfit treats them as cycle gates.
- The largest architectural risk remains the core invariant's breadth: topic/window/session/provider state crosses Telegram, tmux, hook files, provider transcripts, and persistence. Existing docs and tests help, but the coupling is necessarily central.

## 6. Delta observations since baseline

Baseline: `v3.5.1`. Delta: 7 commits, 31 changed files, 1,150 insertions, 102 deletions.

- `627980a fix: refresh stale Pi transcript paths`
  - Changed `src/ccgram/hook.py` and `tests/ccgram/test_hook_provider_events.py`.
  - New/changed flow: `_resolve_transcript_path()` and `_refresh_session_map_if_stale()` refresh stale `session_map.json` entries for provider/session mismatches (`src/ccgram/hook.py:946-1042`).
  - GitNexus context: `_refresh_session_map_if_stale` is called by `_process_hook_stdin` and calls `_read_session_map_entry`, `_resolve_transcript_path`, `_update_session_map`.
  - Architecture effect: hook subprocess owns more session-map correction logic and provider-specific transcript path resolution. Good reliability fix; increases hook↔provider↔session-map coupling.
- `4692879 feat: add Pi followup command`
  - Changed docs, `src/ccgram/handlers/commands/forward.py`, `src/ccgram/providers/pi_discovery.py`, `src/ccgram/tmux_manager.py`, and tests.
  - Evidence: `/followup` is a synthetic Pi builtin (`src/ccgram/providers/pi_discovery.py:43-44`); `forward.py` special-cases Pi followup (`src/ccgram/handlers/commands/forward.py:113-137`, `228-231`); `tmux_manager.send_followup_to_window()` sends literal text, waits, then sends `M-Enter` (`src/ccgram/tmux_manager.py:935-955`).
  - Architecture effect: a provider-specific TUI gesture now crosses provider discovery, generic command forwarding, and tmux keystroke transport. Tests cover it (`tests/ccgram/handlers/commands/test_forward.py:171-196`, `tests/ccgram/test_tmux_followup.py:7-44`).
- `0682674 fix: align Pi slash command handling`
  - Broad cross-area change: README/docs, command catalog, registry, topics flow, providers, and integration tests.
  - Architecture effect: provider command-name translation and bot-native command collisions are an active volatility vector. This reinforces the need for provider command contracts instead of growing `forward.py` branches.
- `213fc2a fix: ignore stale dead autoclose timers`
  - Changed lifecycle/text handlers and tests.
  - Evidence: `_close_expired_topic()` now checks whether a dead-topic window is live and clears stale dead autoclose instead of deleting the topic (`src/ccgram/handlers/topics/topic_lifecycle.py:66-82`).
  - GitNexus context: `_close_expired_topic` is reached from `check_autoclose_timers`, participates in `run_lifecycle_tasks`, and calls Telegram client, tmux manager, thread router, lifecycle strategy, cleanup.
  - Architecture effect: lifecycle policy remains in topics handler package but coordinates across tmux, Telegram, router, and cleanup state.
- `a6b9ba7 docs: add remote development Slidev deck`
  - Docs-only tracked delta, plus current dirty edits in `docs/presentaions/slides.md` and `CLAUDE.md`.
- Change locality:
  - Two commits were clearly cross-area: Pi command alignment and Pi followup.
  - Top changed files since `v3.5.1`: `README.md` 3 commits; `docs/providers.md`, `tests/ccgram/handlers/commands/test_forward.py`, `src/ccgram/providers/pi_discovery.py`, `tests/ccgram/providers/test_pi.py`, `src/ccgram/handlers/commands/forward.py` 2 each.

## 7. Balanced Coupling relationship records

### BC-1: Telegram topic ↔ tmux window ↔ provider session identity

- Relationship: product core state model across Telegram topics, tmux windows, provider sessions, hook/session files.
- Strength: model/functional. The same `window_id`/topic/session concepts drive routing, monitoring, recovery, provider resolution, and state files (`CLAUDE.md:33-38`, `.claude/rules/topic-architecture.md:1`, `.claude/rules/architecture.md:195-205`).
- Distance: high. Crosses Telegram Bot API, tmux process state, local JSON files, provider transcript formats, and handler/session/provider modules.
- Volatility: high. This is the core product capability; recent changes touched session-map/provider identity and topic lifecycle.
- Severity: high candidate, mitigated by strong docs and several executable tests. Not a quick-sweep finding because the coupling is intended and partly guarded.
- Balancing move: keep high-strength state close behind explicit contracts: `ThreadRouter`/`SessionMapSync`/`WindowState` schemas, contract tests for state transitions, and archfit/import-linter checks for allowed directions.

### BC-2: Pi `/followup` command path: command forwarding ↔ provider discovery ↔ tmux keystrokes

- Relationship: `providers/pi_discovery.py` exposes `/followup`; `handlers/commands/forward.py` branches on provider; `tmux_manager.py` sends `M-Enter`.
- Strength: functional. The handler knows Pi command semantics and the tmux key gesture (`src/ccgram/handlers/commands/forward.py:113-137`, `src/ccgram/tmux_manager.py:935-955`).
- Distance: medium/high. Crosses handlers, providers, and tmux adapter in one deployable; runtime crosses Telegram-to-terminal.
- Volatility: medium/high. Pi command names and TUI keybindings are provider implementation details and changed in this delta.
- Severity: medium. Tests cover current behavior, but repeated provider-specific branches would make `forward.py` a command-policy hub.
- Balancing move: if more provider-native synthetic commands appear, move special-command handling behind a provider capability/contract; keep `forward.py` orchestration-only.

### BC-3: Hook session-map refresh ↔ provider transcript path resolution

- Relationship: hook subprocess refreshes `session_map.json` for stale provider/session/transcript entries.
- Strength: model/intrusive. Hook code reads/writes session-map persistence and applies provider-specific transcript-path fallback (`src/ccgram/hook.py:946-1042`).
- Distance: medium/high. Crosses hook subprocess, provider transcript conventions, session monitor state, and persistence file contract.
- Volatility: high. Provider hooks and transcript layouts vary by CLI; Pi was the recent trigger.
- Severity: medium/high candidate.
- Balancing move: move transcript resolution to provider adapters and session-map mutation to `SessionMapSync` or a narrow persistence contract; keep hook normalization separate from persistence repair.

### BC-4: Dead-topic autoclose lifecycle ↔ tmux liveness ↔ Telegram topic deletion

- Relationship: autoclose timer decides whether to delete/close Telegram topic or clear stale dead timer after checking tmux window liveness.
- Strength: functional. The policy needs lifecycle timer state, `thread_router`, `tmux_manager.find_window_by_id`, Telegram client operations, and cleanup (`src/ccgram/handlers/topics/topic_lifecycle.py:42-116`).
- Distance: high runtime, medium code distance. Crosses Telegram/tmux; code is in one handler feature but calls several adapters.
- Volatility: medium. Lifecycle UX changes are likely but not as volatile as provider CLI semantics.
- Severity: medium.
- Balancing move: make lifecycle a small explicit state machine with contract tests around state transitions; keep side effects injected at the edge.

### BC-5: Static bootstrap/handler/provider cycles from lazy imports and compatibility re-exports

- Relationship: app bootstrap/Telegram bot lifecycle and provider shell infrastructure use lazy imports/re-exports to avoid cold-start cycles while preserving compatibility.
- Strength: functional, with some module-initialization coupling. Evidence: `bot.py` imports `.bootstrap` (`src/ccgram/bot.py:24`); `bootstrap.py` documents lazy cycle imports (`src/ccgram/bootstrap.py:223-226`, `250-251`); `shell.py` re-exports `shell_infra` (`src/ccgram/providers/shell.py:21-29`); `process_detection.py` imports through `shell` (`src/ccgram/providers/process_detection.py:25`).
- Distance: medium/high. Crosses app bootstrap, Telegram adapter, provider infrastructure, handler registry.
- Volatility: medium. These are wiring seams and test compatibility surfaces.
- Severity: medium. Runtime imports pass and lazy imports are documented, but static graph health remains poor and archfit gates flag them.
- Balancing move: replace compatibility re-export dependency edges with canonical imports, move test patch compatibility into dedicated shim modules, and encode accepted lazy-import exceptions in a cycle check rather than leaving broad graph cycles ambiguous.

## 8. Architect-skill blind spots vs archfit

- This quick sweep did not run the full archfit pipeline, SCIP extraction, jscpd clone detection, lizard complexity, or archfit's weighted scorecard. Archfit found quantitative signals I did not independently reproduce: 58 BC rollups / 421 edges, 41 cross-module duplicate pairs, 8 god modules, 115 blast-radius hubs, 73 unstable modules, propagation cost 0.26 (`scorecard.md`).
- I did not assign final scores. Archfit did: full overall `43/100`, delta `35/100`. Those are archfit artifact facts, not architect-skill scores.
- My grimp pass sampled cycles and hubs, but archfit has broader extraction coverage (`full.json` tool coverage: scip, grimp, loc, lizard, jscpd, gitnexus, ast-grep).
- I ran selected architecture/import tests, not full `make check`; archfit's CI-like coverage assumptions may include more test paths.
- LSP/tree-sitter were skipped, so no resolved symbol/reference proof beyond GitNexus and grimp.

## 9. Archfit blind spots vs architect skill

- Archfit flags import cycles as gates but does not distinguish runtime-failing import cycles from deliberately annotated lazy imports. The repo has a dedicated lazy-import lint and clean-interpreter import tests that passed; this changes the interpretation from “runtime breakage” to “static dependency-health/boundary friction.”
- Archfit reports `architecture_fitness` evidence including `pkg/mod/golang.org/x/tools@v0.42.0/txtar/archive_test.go`, which looks like external cache/tool noise for a Python repo. The human sweep would not count that as a ccgram architectural fitness check.
- Archfit's draft labels mark many relationships `volatility: undeclared`, causing many “declare_volatility” suggestions. The architect sweep can infer domain volatility from product intent: provider command handling and topic/window/session state are core/high-volatility; optional Mini App/tts/whisper are lower-volatility adapters.
- Archfit sees cross-module edges but not the design rationale for accepted seams: `TelegramClient` Protocol, query layer, window-state feature ports, and pure polling decision kernel are explicit architecture moves with tests.
- Archfit artifacts do not surface dirty-state risk as clearly: current working tree has modified docs and untracked archfit configs/cache. That matters for repeatability and artifact provenance.
- Archfit scorecards do not explain target design moves. The architect sweep turns findings into contract/fitness-check suggestions without editing source.

## 10. Reliability/repeatability notes

- GitNexus freshness: `ccgram` index matched `HEAD=a6b9ba75e5440c95402310aba9ec2d43dad858fe`; no stale-index warning.
- Dirty state is documented. Re-running after staging/cleaning docs and either committing/removing `.archfit*` files may change archfit and GitNexus dirty-change observations.
- Existing archfit artifacts are under `/Users/alexei/Workspace/archfit/reports/archfit-vs-architect-20260620 (openai)/ccgram/`; `llm-review.md` and `llm-review-nocache.md` contain JSON parse errors and were not used as evidence.
- Tool interpretation:
  - Nonzero `import-linter` exit was configuration absence, not a boundary violation.
  - Static cycles from grimp/archfit include lazy imports; runtime import tests passed.
  - `uvx` tool cache is outside the source repo; target `git status --short` was unchanged after checks.
- Quality self-check:
  - Structure: yes — follows the 12 requested sections.
  - Clarity: yes — separates facts, observations, candidates, and archfit artifact facts.
  - Usefulness: yes — names specific seams and balancing moves.
  - Repeatability: yes — includes refs, commands, and tool failures.
  - Helpfulness limit: this is a quick sweep, not a full architecture review; no final scores.

## 11. Target architecture/design suggestions and executable fitness checks

1. Promote a reviewed module map from `.archfit.yaml` into committed architecture intent.
   - Current `.archfit.yaml` is generated and says to review/promote gates (`.archfit.yaml:1`).
   - Keep `core`/`adapter`, but classify volatility: provider command handling and topic/window/session mapping are core/high; optional Mini App/tts/whisper are adapter/supporting.
   - Fitness check: add a reviewed full archfit config to CI (for example `archfit check -c .archfit.full.yaml --full`) only after restoring/regenerating that config and triaging lazy-cycle exceptions plus generated/cache path noise.

2. Make provider-specific command behavior a provider contract.
   - Current special case: Pi `/followup` lives in generic command forwarding (`src/ccgram/handlers/commands/forward.py:113-137`, `228-231`).
   - Target: `AgentProvider` exposes optional `handle_synthetic_command` or declarative command capability. `forward.py` stays provider-agnostic.
   - Fitness check: an AST/import test that `handlers/commands/forward.py` does not branch on provider names except through the provider contract.

3. Narrow hook persistence repair.
   - Current: `hook.py` resolves Pi transcript paths and refreshes `session_map.json` (`src/ccgram/hook.py:946-1042`).
   - Target: hook adapters normalize event payloads; provider adapters resolve transcript paths; `SessionMapSync` owns persistence update rules.
   - Fitness check: unit/contract tests for stale provider/session-map refresh cases already exist (`tests/ccgram/test_hook_provider_events.py:54-191`); add a boundary test forbidding direct session-map persistence repair outside the sync layer except for approved hook bootstrap paths.

4. Resolve or explicitly allow static cycles.
   - Current: lazy imports are documented and linted, but grimp/archfit still sees cycles.
   - Target: eliminate easy cycles (`providers.process_detection` should import `shell_infra` directly, not through `shell` compatibility re-export) and encode remaining bootstrap cycles as intentional lazy exceptions.
   - Fitness check: grimp cycle check in CI with an allowlist file requiring rationale per remaining cycle.

5. Keep existing architectural tests and make them more visible.
   - Already useful: query layer, window state access audit, window store import boundary, polling purity, lazy-import lint.
   - Fitness check: group these under a named `make test-architecture` target so architecture fitness is cheap and repeatable without running every unit/integration test.

## 12. Next checks

1. Clean or commit dirty docs and decide whether `.archfit*.yaml` should be committed. Then rerun `git status --short` and GitNexus `detect_changes`.
2. Run full project gate: `cd /Users/alexei/Workspace/ccgram && make check`.
3. Restore/regenerate the full archfit config if needed, then run full archfit after label/config review: `archfit check -c .archfit.full.yaml --full`.
4. Add/run a grimp cycle script that separates module-level imports from annotated lazy imports.
5. Review `.archfit-labels.yaml` volatility classifications; replace `undeclared` with core/supporting/generic rationale before treating BC rollups as final findings.
6. If changing source, run GitNexus impact first on the target symbol and then rerun `gitnexus_detect_changes(repo:"ccgram")` after edits.
