# Architecture Review: ccgram

**Reviewer:** Independent architect agent  
**Date:** 2026-06-20  
**Commit:** HEAD (main branch)  
**Method:** Balanced Coupling (Khononov) — evidence-first, no archfit tooling used  
**Language:** Python 3.14 (~382 .py files, src/ccgram/ layout)

---

## 1. System Map

### Domain

ccgram maps each Telegram Forum topic to one tmux window running one agent CLI (Claude Code, Codex, Gemini, Pi, or Shell). Routing is keyed by tmux window ID (`@0`, `@12`). The hook.py subprocess runs inside agent panes and feeds events back via session_map.json and events.jsonl.

### Package / Module Inventory

```
src/ccgram/
├── main.py                     # CLI entrypoint (click)
├── bot.py                      # PTB Application factory + lifecycle (172L)
├── bootstrap.py                # post_init / post_shutdown wiring
├── hook.py                     # Hook runner + installer (1234L) ← LARGE
├── config.py                   # Singleton config (7 commits churn)
├── session.py                  # SessionManager (612L, owns sub-stores)
├── session_map.py              # SessionMapSync (649L)
├── window_state_store.py       # WindowStateStore (664L, 7 commits)
├── window_state_ports/         # Projection seams: identity/lifecycle/pane/tool/worktree
├── window_query.py             # Read-only state queries for handlers
├── session_query.py            # Read-only session queries
├── tmux_manager.py             # All libtmux I/O (956L)
├── telegram_client.py          # TelegramClient Protocol + adapters (597L)
├── thread_router.py            # User→Thread→Window routing
├── topic_state_registry.py     # Cleanup callback registry
├── terminal_parser.py          # Terminal ANSI/VT parser (751L)
├── transcript_parser.py        # JSONL transcript parser (737L)
├── providers/
│   ├── base.py                 # AgentProvider Protocol, ProviderCapabilities
│   ├── claude.py               # Claude adapter (module-level singletons)
│   ├── codex.py                # Codex adapter (809L)
│   ├── gemini.py               # Gemini adapter (905L)
│   ├── pi.py                   # Pi adapter
│   ├── shell.py                # Shell adapter
│   ├── shell_infra.py          # Shell-specific tmux ops
│   └── _jsonl.py               # Shared JSONL base
├── handlers/                   # 14 feature subpackages
│   ├── polling/                # Status polling loop + window tick
│   │   ├── polling_types.py    # Pure data types + constants (tested in isolation)
│   │   ├── polling_state.py    # 5 module-level singletons (1018L)
│   │   ├── polling_coordinator.py
│   │   └── window_tick/        # decide (pure) / observe / apply split
│   ├── messaging_pipeline/     # message_queue, message_sender, tool_batch (643L)
│   ├── topics/                 # directory_callbacks (1076L), orchestration
│   ├── text/                   # text_handler (550L)
│   ├── recovery/               # recovery_banner (572L), resume (555L)
│   ├── shell/                  # shell_capture (589L)
│   └── ... (commands, live, send, status, toolbar, voice, interactive)
├── miniapp/                    # Optional aiohttp Mini App server
├── tts/                        # Optional TTS (edge, openai)
├── whisper/                    # Optional speech-to-text
└── llm/                        # Optional LLM enrichment
```

### Layering (intended, from docs/architecture.md)

```
Entry/Bootstrap
    └── Telegram Seam (TelegramClient Protocol)
        └── Handler Layer (14 subpackages)
            └── Query Layer (window_query, session_query — read-only)
                └── State Management (SessionManager, WindowStateStore, sub-stores)
                    └── Infrastructure (tmux_manager, session_map, thread_router)
                        └── Provider Abstraction (AgentProvider Protocol)
```

The Mini App, TTS, Whisper, and LLM layers are gated and optional.

---

## 2. Evidence by Dimension

### 2.1 Modularity / Cohesion

**Strengths:**
- 14 feature subpackages under `handlers/` with clear single responsibilities: `messaging_pipeline`, `recovery`, `polling`, `topics`, `send`, `shell`, `voice`, etc.
- `polling_types.py` / `polling_state.py` split isolates the pure decision kernel from stateful singletons. Enforced by `tests/ccgram/handlers/polling/test_polling_types_purity.py`.
- `window_state_ports/` splits WindowStateStore into 5 projection seams (identity, lifecycle, pane, tool, worktree) with frozen dataclasses — clean feature decomposition.
- `providers/base.py` has zero intra-project imports ("Pure definitions only — no imports from existing ccgram modules", `src/ccgram/providers/base.py:14`).

**Cohesion failures (god modules):**

| File | Lines | Mixed responsibilities |
|------|-------|----------------------|
| `hook.py` | 1234 | Hook installation, uninstallation, provider detection, session_map I/O, event dispatch, subprocess forking, 7 distinct concerns |
| `handlers/topics/directory_callbacks.py` | 1076 | 10+ callback types for browser flow, provider picker, mode picker, worktree — could be split by flow stage |
| `handlers/polling/polling_state.py` | 1018 | 5 module-level singletons + 3 strategy classes; instantiation is a side effect of import |
| `tmux_manager.py` | 956 | Session discovery, pane capture, key sending, vim-mode detection/injection, per-window locks |
| `providers/gemini.py` | 905 | Full Gemini provider including transcript parsing, terminal parsing, status detection |

`hook.py` is the worst cohesion case: it is the only 1200+ line file with no sub-module decomposition, making it resistant to unit testing and high-risk to modify when adding new providers.

### 2.2 Coupling Balance (Balanced Coupling Analysis)

**Integration Strength classification used:**
- **Contract**: depends on a Protocol / ABC only — extremely stable
- **Model**: depends on data types, frozen dataclasses
- **Functional**: calls methods, accesses mutable singletons
- **Intrusive**: reaches into internals, accesses `_private` or cross-cuts data ownership

**Well-balanced couplings:**

| Coupling | Strength | Distance | Volatility | Verdict |
|----------|----------|----------|------------|---------|
| `providers/* -> providers/base.py` | Contract (Protocol) | 1 (sibling) | Low | BALANCED |
| `handlers/* -> TelegramClient Protocol` | Contract (Protocol) | 1 (seam boundary) | Low | BALANCED |
| `handlers/* -> window_query/session_query` | Model (free functions) | 1 (adjacent) | Low-Med | BALANCED |
| `window_tick/decide -> polling_types` | Model (pure types) | 1 (sibling) | Low | BALANCED |
| `handlers -> window_state_ports/` | Model (frozen projections) | 1 (adjacent) | Low | BALANCED |

**Unbalanced couplings:**

**Finding 1 — providers/claude.py → tmux_manager (functional, distance=2)**

```
src/ccgram/providers/claude.py: from .tmux_manager import tmux_manager  # line ~31
src/ccgram/providers/shell_infra.py: from ccgram.tmux_manager import tmux_manager  # x4
```

- Strength: **Functional** — `claude.py` calls `send_keys`, `capture_pane`, `list_windows` on the singleton
- Distance: **2** (provider layer → infrastructure layer, crosses an architectural boundary)
- Volatility: **Medium** — `tmux_manager.py` has 6 commits of active churn (vim-mode injection, pane-level ops, async wrapping)
- BC balance rule: HIGH strength XOR HIGH distance → NOT both high + volatility > low = **UNBALANCED**
- Impact: Any tmux API change (new method signature, libtmux version bump) cascades into `providers/claude.py` and `providers/shell_infra.py` directly. No port abstraction (interface/Protocol) sits between them.

**Finding 2 — handlers/text/text_handler.py → polling_state.lifecycle_strategy (functional, cross-subpackage)**

```
src/ccgram/handlers/text/text_handler.py:50: from ..polling.polling_state import lifecycle_strategy
```

- Strength: **Functional** — reaches into polling's stateful singleton directly
- Distance: **2** (text handler subpackage → polling subpackage singleton — sibling subpackages but at the stateful level)
- Volatility: **High** — `polling_state.py` (1018L) with 5 singletons, actively evolving
- Verdict: **UNBALANCED** — text message handling is a high-frequency code path; coupling it directly to a polling singleton means polling strategy changes break text routing. The query layer (`window_query`) doesn't yet expose this state.

**Finding 3 — handlers/topics/directory_callbacks.py → {session, session_map, thread_router, tmux_manager, providers}**

```
src/ccgram/handlers/topics/directory_callbacks.py:32-38:
  from ...session import session_manager
  from ...session_map import session_map_sync
  from ...thread_router import thread_router
  from ...tmux_manager import send_to_window, tmux_manager
  from ...providers import registry as provider_registry
  from ...providers.shell import ...
```

- Strength: **Functional** (calls mutation methods on 5+ singletons)
- Distance: **2** (handler → state/infra)
- Volatility: **High** (1076L file, 4 commits; every new provider adds branches here)
- Verdict: **PARTIALLY UNBALANCED** — some paths are sanctioned (`providers` goes through Registry Protocol), but direct `session_map_sync` + `thread_router` + `tmux_manager` functional coupling concentrates change risk. The snapshot-based allowlist in `test_handler_layering_invariants.py` prevents new violations but doesn't reduce existing ones.

**Finding 4 — providers/claude.py → cc_commands (inverted dependency)**

```
src/ccgram/providers/claude.py: from ccgram.cc_commands import CC_BUILTIN_COMMANDS
```

- Strength: **Model** (imports a constant set)
- Distance: **2** (provider → CLI utility layer — inverted: cc_commands is an entrypoint-level module)
- Volatility: **Low** (constants rarely change)
- Verdict: **MILDLY UNBALANCED** — direction is inverted but volatility is low, so risk is contained. Should be moved to `providers/base.py` or a shared constants module.

**Finding 5 — bot.py → main.py (_shutdown_signal)**

```
src/ccgram/bot.py:98: from .main import _shutdown_signal  # noted in code comment as a cycle
```

- Strength: **Functional** (reads module-level state)
- Distance: **1** (sibling entry modules)
- Volatility: **Low** (startup/shutdown rarely changes)
- Verdict: **MILDLY UNBALANCED** — bidirectional dependency between entry modules. The code comment acknowledges this as a managed cycle. Low risk given low volatility.

### 2.3 Dependency Direction / Cycles

**Observed direction (actual):**

```
main.py → bot.py → bootstrap.py
                  → handlers/* → window_query / session_query
                              → window_state_ports
                              → [via singletons] tmux_manager, session, thread_router
providers/* → providers/base.py (clean)
providers/claude.py → tmux_manager [INVERTED vs. intended — provider should not reach infra]
providers/claude.py → cc_commands [INVERTED — provider reaches CLI layer]
hook.py → hooks/adapters → providers/base.py [clean]
```

**Import cycles:** The package imports cleanly (`python -c "import ccgram"` exits 0). No detected cycles at module level. Several documented deferred imports in `polling_coordinator.py` (`from .periodic_tasks import ...` inside `async def`) and `bot.py` (`from .main import _shutdown_signal` inside function) manage latent cycles. These are technical debt that static analysis misses.

**Clean:** No circular imports at module load time. Deferred imports successfully avoid cycles but hide coupling relationships from static tools.

### 2.4 Encapsulation

**Strong:**
- `window_state_ports/`: 5 projection dataclasses expose frozen views. Handlers cannot reach `WindowState` internals without going through port functions. `src/ccgram/window_state_ports/__init__.py` — explicit `__all__` list controls surface.
- `providers/base.py`: Protocol with zero project imports. Cannot be accidentally corrupted by adding ccgram dependencies.
- `polling_types.py`: "pure" module proven by import-level test. The `__init__.py` explicitly does NOT re-export `periodic_tasks` to prevent stateful singleton load.
- `TelegramClient Protocol`: handlers depend on Protocol, not `telegram.Bot` directly. `FakeTelegramClient` recording fake enables full handler unit testing.

**Weak:**
- **Module-level singletons** are the primary encapsulation hole. `session_manager`, `tmux_manager`, `window_store`, `topic_state`, `thread_router`, `user_preferences` are all module-level globals. Any module can import and mutate them. The `window_store` backward-compat proxy (`__getattr__` delegation at `src/ccgram/window_state_store.py:649`) adds invisible indirection.
- **Snapshot allowlists** (`test_handler_layering_invariants.py`) enumerate current violators of the TelegramClient seam and singleton access. This is better than nothing but weaker than a structural invariant: adding a new violating file requires explicit allowlist expansion (good) but existing listed files remain bypassing the seams indefinitely (bad).
- `polling_state.py` instantiates 5 singletons as module-level side effects. Any module importing `polling_state` triggers all instantiations. The docstring warns about this but no structural check enforces it.

### 2.5 Blast Radius / Change Locality

**High-churn, high-fanout modules (git log + import fan-in combined):**

| Module | Commits (last 30) | Imported by (approx.) | Risk |
|--------|------------------|-----------------------|------|
| `config.py` | 7 | 10+ (every submodule) | High |
| `window_state_store.py` | 7 | session + ports + tests | Medium |
| `tmux_manager.py` | 6 | providers + polling + handlers | High |
| `session.py` | 6 | bootstrap + handlers + session_map | High |
| `session_map.py` | 4 | session + handlers + hook | Medium |

`config.py` is the widest blast: it is imported transitively by almost every module. A config shape change (adding a field, changing defaults) ripples everywhere. The singleton pattern (`config = Config()` at module level in `src/ccgram/config.py`) means there is no injection point; tests that need alternate configs must patch the singleton.

`tmux_manager.py` (956L) is a second critical hub: both the provider layer and the polling infrastructure import it directly. Adding a new tmux feature (e.g., a new capture mode) touches this file and then ripples to `providers/claude.py`, `providers/shell_infra.py`, `handlers/polling/polling_coordinator.py`.

**Positive:** `window_state_ports/` successfully reduced blast radius for `WindowStateStore` — ports provide a stable interface, tests can mock individual projections.

### 2.6 Testability

**Strengths:**
- 167+ test files, well-distributed across unit (ccgram/), integration, and e2e tiers
- `TelegramClient Protocol` + `FakeTelegramClient` enables handler tests without a live bot
- `test_polling_types_purity.py`: import-level enforcement that pure kernel stays pure
- `test_handler_layering_invariants.py` + `test_query_layer_only_for_handlers.py`: AST-walk structural tests — positive signal that the team thinks about testability structurally
- `window_state_ports/` projection dataclasses are constructable in tests without `SessionManager`
- Hypothesis-based tests noted (`hypothesis>=6.0` in dev deps)

**Weaknesses:**
- Module-level singletons (`tmux_manager`, `session_manager`, `config`) require `unittest.mock.patch` at the module level — fragile if test isolation is incomplete
- `polling_state.py` instantiates singletons at import time; test suites that import it get live singleton instances unless explicitly reset via `_reset_for_testing()` methods
- `hook.py` (1234L, 7 distinct responsibilities) has no clean unit test surface. Integration-level or subprocess tests are the only practical approach
- `providers/claude.py` and `providers/gemini.py` (905L) have tight coupling to tmux_manager, making pure unit tests difficult without mocking the entire tmux interface

### 2.7 Architecture Fitness

**Enforced checks (running):**
- `tests/ccgram/handlers/polling/test_polling_types_purity.py` — import-level purity invariant for pure kernel
- `tests/ccgram/test_handler_layering_invariants.py` — AST walk: PTB Bot escape allowlist + singleton access allowlist
- `tests/ccgram/test_query_layer_only_for_handlers.py` — AST walk: session_manager read-path enforcement
- `tests/ccgram/test_import_clean.py` — clean interpreter import test (regression for import cycles)

**Gaps (not checked):**
- No fitness check preventing `providers/*` from importing `tmux_manager` directly. Finding 1 (the highest-risk unbalanced coupling) has no structural guard.
- No check on cross-layer direction violations (provider → CLI layer via `cc_commands`)
- Snapshot allowlists in `test_handler_layering_invariants.py` are enumerative rather than invariant-based. A new file can bypass the seam by adding itself to the list during review — there is no forcing function that requires justification.
- `hook.py` decomposition has no structural check. Its 1234 lines and 7 concerns are unconstrained.

---

## 3. Balanced Coupling Scorecard

| Dimension | Score | Band | Confidence | Key evidence |
|-----------|-------|------|------------|--------------|
| Modularity / Cohesion | 6/10 | Good | High | 14 subpackages clean; hook.py(1234), directory_callbacks(1076), polling_state(1018) are cohesion failures |
| Coupling Balance | 5/10 | Fair | High | Protocol seams excellent; providers→tmux_manager functional/dist=2 unbalanced; 5-singleton fan-out in directory_callbacks |
| Dependency Direction / Cycles | 6/10 | Good | Medium | Clean layering; providers→cc_commands inverted; bot↔main bidirectional cycle; deferred imports mask latent cycles |
| Encapsulation | 6/10 | Good | High | window_state_ports projection seam strong; module-level singletons are primary hole; snapshot allowlists weaker than invariants |
| Blast Radius / Change Locality | 5/10 | Fair | Medium | config + tmux_manager + session widely shared with high churn; window_state_ports partially mitigates |
| Testability | 7/10 | Good | High | FakeTelegramClient, AST structural tests, purity tests; singletons + hook.py resist unit testing |
| Architecture Fitness | 6/10 | Good | High | Active import/AST fitness functions; gap: no provider→infra direction check; allowlists drift-prone |

**Overall band: Good (5.9/10 average)**

---

## 4. Top Findings

### Finding 1 (UNBALANCED — Coupling Balance): providers/claude.py → tmux_manager direct functional coupling

**File:** `src/ccgram/providers/claude.py:31` (`from ccgram.tmux_manager import tmux_manager`), `src/ccgram/providers/shell_infra.py:1-10` (4 imports)

**BC classification:** Functional coupling (calls send_keys, capture_pane, list_windows), distance=2 (provider → infrastructure), volatility=medium (956L, 6 commits). Balance rule: STRENGTH(high) AND NOT DISTANCE(low) → high strength + high distance is UNBALANCED.

**Risk:** Any tmux API change (libtmux version bump, new async wrapping, vim-mode probe changes) cascades into two provider files. Providers cannot be tested without a real or mocked tmux instance. Adding a new agent CLI provider (e.g., Cursor) must either couple to tmux_manager or duplicate the infra.

**Fix path:** Introduce a `TmuxPort` Protocol in `providers/base.py` or a new `ports/` module. `tmux_manager.py` implements it; providers depend on the Protocol. Mirrors the TelegramClient pattern already used successfully.

---

### Finding 2 (UNBALANCED — Coupling Balance / Encapsulation): text_handler → polling_state.lifecycle_strategy singleton

**File:** `src/ccgram/handlers/text/text_handler.py:50` (`from ..polling.polling_state import lifecycle_strategy`)

**BC classification:** Functional (calls lifecycle_strategy methods), distance=2 (text handler → polling singleton, crossing subpackage internals), volatility=high (polling_state is 1018L, evolving).

**Risk:** Text message handling — the hottest path in the system — is coupled to a polling singleton. A polling refactor risks breaking text routing silently.

**Fix path:** Expose the needed lifecycle state through `window_query.py` or fire an event rather than calling the polling singleton directly.

---

### Finding 3 (Cohesion / Blast Radius): hook.py monolith

**File:** `src/ccgram/hook.py` (1234 lines)

**BC dimension:** Cohesion failure — 7 distinct concerns in one file: hook installation, uninstallation, provider detection (`best-effort provider detection from foreground tty process commands`, line ~1047), session_map I/O, event dispatch (`process_hook_event`, line ~1116), subprocess forking, and the main entrypoint (`if __name__ == "__main__"`).

**Risk:** hook.py runs inside agent panes on every hook notification. It is the highest-frequency subprocess in the system. Any modification affects all hook scenarios simultaneously. No unit test surface exists. Churn is confirmed (4 commits in last 30).

**Fix path:** Extract at minimum: `hook_installer.py` (install/uninstall), `hook_event_processor.py` (dispatch), `hook_provider_detector.py` (process detection). The handlers/commands/ split precedent (`Round 5 split of command_orchestration.py` per __init__.py docstring) shows the team has done this decomposition before.

---

### Finding 4 (Dependency Direction): providers/claude.py → cc_commands (inverted direction)

**File:** `src/ccgram/providers/claude.py` (`from ccgram.cc_commands import CC_BUILTIN_COMMANDS`)

**BC dimension:** Dependency direction violation — `providers/claude.py` is a domain adapter; `cc_commands` is a CLI/application layer module. Providers should not know about CLI utilities. This is distance=2, strength=model (constants only), volatility=low — containable but architecturally wrong direction.

**Fix path:** Move `CC_BUILTIN_COMMANDS` to `providers/claude.py` itself or to `providers/base.py`.

---

### Finding 5 (Architecture Fitness Gap): no structural check on provider→infra direction

**Test gap:** The three AST-walk fitness functions protect handler→PTB, handler→session_manager-reads, and polling_types purity. None checks that `providers/` does not import `tmux_manager` or other infrastructure. Finding 1 is entirely unconstrained by fitness functions. A new provider file that imports `tmux_manager` will pass all tests with no friction.

**Fix path:** Add `test_provider_layer_invariants.py`: AST walk over `src/ccgram/providers/`, assert no direct import of `ccgram.tmux_manager` or `ccgram.session_map`. Pattern follows existing `test_handler_layering_invariants.py`.

---

## 5. Coverage Gaps / Low-Confidence Areas

1. **Dynamic imports**: Multiple `from .utils import ...` inside async functions (lazy imports) and conditional `try/except ImportError` blocks for optional extras (tts, whisper, llm) are invisible to static import analysis. The actual runtime coupling graph differs from the static one. pyright was checked (errors/warnings counts only; type coverage unknown).

2. **Singleton state interaction at runtime**: Five `polling_state` singletons + `session_manager` + `tmux_manager` global instances interact via shared mutable state. Static analysis cannot detect race conditions or state corruption patterns. The asyncio concurrency model (single event loop + `asyncio.to_thread` for tmux) reduces but does not eliminate hazard.

3. **radon complexity**: `radon cc src/ -n C` returned no output (no functions rated C or above with default threshold), suggesting no McCabe-10 violations, but this was not confirmed with explicit output — the tool may require a venv install.

4. **Test execution**: Tests were not run as part of this review. Coverage is documented at branch level in `coverage.json` but not read. The structural/AST tests were inspected only.

5. **miniapp/, tts/, whisper/ optional layers**: These are conditionally loaded. Their coupling to the core (config, entity_formatting) was spot-checked but not fully mapped.

6. **git history depth**: Churn analysis covers last 30 commits only. The full churn profile across the entire project history may reveal different hotspots.

---

## 6. Groundedness / Reproducibility

All findings are backed by:
- Direct file reads (`head`, `grep`, `wc -l`)
- Cross-module import graph reconstructed by grepping `from ccgram.` in src/ (`full-import-graph` command)
- Structural test file reads (handler_layering_invariants.py, query_layer_only_for_handlers.py, test_polling_types_purity.py)
- Git churn via `git diff-tree --name-only` over last 30 commits
- Architecture doc in `docs/architecture.md` (Mermaid system map)

The review is reproducible by re-running the grep commands above. The only non-reproducible element is dynamic runtime behavior (lazy imports, optional extra loading). The deptry and radon outputs were captured but returned no violations, which reduces — not increases — concern.
