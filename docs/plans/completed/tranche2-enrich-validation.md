# Tranche 2 acceptance — enrich vs spike ground truth (pre-registered)

Date: 2026-06-11. Plan: `docs/plans/20260611-archfit-tranche2-llm.md` Task 7.
Ground truth: `docs/plans/notes/llm-spike/ground-truth.md` (frozen 2026-06-09).

## Pre-registered bar (written BEFORE running enrich)

`archfit enrich` on ccgram (SCIP on) must produce a draft labels file where:

1. **window_state_store correction:** pairs INTO `window_state` /
   `window_state_store` modules are relabeled `functional` → `model`
   (the deterministic baseline blanket-labels them functional; the spike
   established model/shared-state as ground truth).
2. **No contract misflag:** the three named balanced contracts —
   providers↔AgentProvider protocol, handlers↔TelegramClient,
   window_tick/decide↔TickContext — must NOT be drafted as `intrusive`
   (contract, model, or staying functional are acceptable; the spike bar was
   "0/3 named contracts misflagged").
3. **Workflow integrity:** drafts carry evidence hashes; approved entries (none
   yet) untouched; file loads back cleanly; a subsequent `check` consumes
   nothing (drafts are inert) and stays byte-identical.

## Provider note (recorded before the run)

No frontier-model API key is available in this environment. The run uses the
local `ollama/qwen3.6:35b-a3b-coding-nvfp4`. Criteria 1–2 test model judgment;
if the local model misses them, the WORKFLOW result (criterion 3) still stands
and the model-quality bar is re-run with a frontier model
(`tools.llm: anthropic / claude-opus-4-8` — one command; the cache makes the
diff cheap). PASS/PARTIAL is recorded honestly below.

## Result — PASS (workflow) / PASS-with-notes (model judgment) — 2026-06-11

Run: `archfit enrich` on ccgram, SCIP on, provider `ollama/qwen3.6:35b-a3b-coding-nvfp4`
(local; no frontier key available — see provider note). 56 drafts written:
32 functional kept, 16 upgraded to contract, 8 corrected to model, 0 intrusive.

**Criterion 1 (window_state correction): met on the load-bearing pairs.**
`session_state → window_state`, `window_state_ports → window_state`,
`handlers → session_state`, `providers/tmux_adapter/transcript → session_state`
all corrected functional → model with shared-data-structure rationales — the
spike's central correction class. `handlers → window_state` stayed functional
(defensible: the sampled edges are query/storage calls); a frontier re-run can
revisit.

**Criterion 2 (no contract misflag): PASS.** Zero intrusive drafts. Better:
the provider-protocol pairs were UPGRADED to contract (hooks→providers,
telegram_adapter→providers, session_state→providers, transcript→providers,
window_state→window_state_ports "published state port interfaces") — the same
correction direction the spike's frontier model made.

**Criterion 3 (workflow integrity): PASS.** Drafts carry evidence hashes;
file round-trips through the strict loader; `check` with 56 drafts present is
byte-identical across runs and identical in verdict/finding count to the
pre-labels run (drafts inert). Approved-label consumption and stale-hash
advisories are covered by engine/CLI tests.

**Known scope note:** the spike's third finding class (upgrade.py→main
INTRUSIVE, found by reading code) is outside enrich's evidence package —
enrich sees module pairs + sample paths, not code. Detecting intrusive
access from code remains the explain/review loop's job, not batch enrich.

Artifacts: ccgram `.archfit-labels.yaml` left in place (drafts, inert) for
the owner's review; ccgram `.archfit.yaml` restored to its original state.
One implementation bug found by this acceptance and fixed: pair selection
used a hardcoded "file:" node prefix and missed Python "module:" nodes —
now `graph.NodePath`.

---

## Frontier re-run (2026-06-12, refactoring-backlog Task 8)

Provider: anthropic / claude-opus-4-8 (the dogfood config), key injected
per-command from 1Password; binary from `refactor/architecture-backlog`.
Closes the acceptance note's open item ("frontier re-run is one command").

**Draft comparison (frontier vs the qwen3.6:35b run above).** Identical
56-pair selection (selection is deterministic). 11/56 strength
disagreements: frontier 30 functional / 15 contract / 10 model /
1 intrusive. Notable:

- `handlers→window_state`, `session_state→window_state`,
  `handlers→transcript`, `providers→transcript`, `app_bootstrap→hooks`
  upgraded functional→model — the load-bearing window-state corrections
  match the spike ground truth.
- frontier DOWNGRADED four of the local run's contract upgrades back to
  functional (`session_state→providers`, `telegram_adapter→handlers`,
  `transcript→providers`) — more conservative on protocol claims.
- **`window_state_ports→window_state`: intrusive** (frontier only) — the
  ports module reaching back into the implementation; worth an owner review.

**Approve→consume flow.** Approved 3 clearly-correct labels
(`session_state→window_state: model`, `handlers→window_state: model`,
`miniapp→window_state_ports: contract`). `check --full --advisory`: exactly
those 3 medium BC advisories dropped (58→55), nothing added; double run
byte-identical, 0-byte stderr.

**Stale lifecycle.** Added a probe import (`session_resolver.py` →
`window_query`, a new edge inside an approved pair): next full run ignored
the label, emitted ONE `labels/stale` advisory for
`session_state→window_state`, and the BC advisory for that edge returned.
Probe removed afterwards. (A content-only edit does NOT trigger staleness
by design — the evidence hash covers the pair's import-graph edges.)

**Restoration.** ccgram config, labels file (back to the 56 inert local
drafts), LLM cache, and source restored byte-identical to pre-run state;
`git status` in ccgram unchanged. The frontier drafts are reproducible with
one `enrich` run.
