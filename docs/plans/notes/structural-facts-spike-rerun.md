# Structural-facts spike RE-RUN — Tranche 1.5 acceptance gate

Date: 2026-06-11. Plan: `docs/plans/20260610-archfit-tranche1.5-structural-facts.md` Task 6.

## Pre-registered bar (written BEFORE running the classifier)

A blind classifier (fresh-context subagent) is given:

- the `file_facts` block from an archfit full run on ccgram (SCIP on), verbatim JSON;
- permission to read ccgram `src/**` source files only.

It is firewalled from: ccgram `docs/**`, `CLAUDE.md`, `AGENTS.md`, `llm.txt`,
archfit's repo (including this note and all spike notes), and any prior analysis
of ccgram. The facts block is neutral: per-module numbers only — no hub/risk
labels (enforced by renderer test + the gate prompt quotes raw JSON).

**Bar (must meet BOTH to PASS):**

1. The classifier's top intra-module risk list (top 5) includes the
   **polling/state module** (`ccgram.tui.polling_state` or however it names it),
   identified as a _mutable shared-state_ concern — judged from the code +
   high inbound module fan-in.
2. The same list includes the **directory_callbacks module**
   (`ccgram.tui.directory_callbacks`), identified as a _low-cohesion grab-bag_
   concern — judged from the code + high outbound distinct-destination count.

False-positive tolerance: other modules may rank alongside them (config-style
modules scoring high on inbound are expected and acceptable); the gate only
requires both targets to surface in the top 5 with the right _kind_ of concern.

Coverage caveat to record with the result: gitnexus enrichment unavailable
(adapter calls a CLI interface the real gitnexus does not provide), so
`gitnexus_impact` is absent — the run tests the SCIP+git axes only.

## Run setup

- archfit commit: 08c925e (+ working tree as of this note)
- target: ~/Workspace/ccgram, `.archfit.yaml` with `tools.scip.enabled: on`
- command: `archfit check --full --advisory --report --format json`

## Result — PASS (2026-06-11)

Run: archfit @ 08c925e on ccgram, SCIP on, gitnexus **absent** (adapter contract
mismatch — see plan Task 5 WARN). 383 modules in `file_facts`; LOC and co-change
joins populated via the new `symbol.Graph.Path` exact join.

Blind classifier (fresh subagent, firewalled per the bar) returned this top 5:

1. `ccgram.hook` — god-file, five unrelated responsibilities (LOC 1234)
2. **`ccgram.handlers.polling.polling_state`** — "five module-level singletons
   instantiated at import time, each owning distinct mutable state ... entangled
   through shared TerminalPollState references" — the _mutable shared-state_
   concern. ✓ bar #1
3. **`ccgram.handlers.topics.directory_callbacks`** — "1,076-LOC callback
   dispatcher ... 22 imports from 11 modules ... `context.user_data` as a
   stringly-typed session ledger, no typed state object" — the _low-cohesion
   grab-bag_ concern. ✓ bar #2
4. `ccgram.handlers.polling.window_tick.apply` — thickest coupling node (22 outbound)
5. `ccgram.session` — state hub serializing four sub-stores

Benign high-scorers correctly separated: `config` (fan-in 52, read-only env
reader), `providers.base` (fan-in 46, contract/ABC), `thread_router` (fan-in 47,
single responsibility), `telegram_client`, `message_sender` — the exact
discrimination (mutable-state hub vs benign config; grab-bag vs wiring) that the
failed Tranche-1.5 ranking metrics could not make deterministically.

**Both pre-registered targets surfaced in the top 5 with the right concern
kind, without gitnexus. Gate: PASS. Tranche 2 (LLM coupling refinement) is
unblocked.**

Facts that carried the signal: polling_state — inbound 13 (rank 17/383) +
LOC 1018 single file (shortlisted), judgment from code; directory_callbacks —
outbound 22 (rank 4/383) + LOC 1076 (shortlisted), judgment from code.
