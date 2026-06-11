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

## Result

To be filled after the run.
