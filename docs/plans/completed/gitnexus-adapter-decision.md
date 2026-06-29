# gitnexus adapter — fix-or-drop decision (2026-06-11)

## Problem

The original adapter assumed `gitnexus impact --json <root>` returning
`[{"module", "impact"}]`. The real gitnexus CLI has no such interface — its
`impact` command takes a single _symbol_ target. The impact map was therefore
ALWAYS empty in real runs (coverage `partial`), a silently-dead integration.

## Pre-registered bar

Fix only if ONE stable command yields repo-wide per-module/per-file counts;
otherwise delete the provider end-to-end.

## Spike findings

- `gitnexus cypher -r <abs-root> "<query>"` executes raw Cypher against the
  knowledge graph; stdout is a clean JSON envelope `{markdown, row_count}`
  (logs go to stderr). `-r` accepts the absolute repo path.
- Graph schema: `File` nodes with `filePath`; a single `CodeRelation` edge
  label with a `type` property (`DEFINES`, `CALLS`, `ACCESSES`, `IMPORTS`,
  `EXTENDS`, `CONTAINS`, ...).
- One query returns per-file distinct dependant-file counts repo-wide,
  deterministically ordered. Verified on ccgram (144 files):
  `thread_router.py` 58, `message_sender.py` 44, `tmux_manager.py` 42 —
  consistent with SCIP inbound-fan-in rankings.
- Environment note: the gitnexus global install needed a one-time repair
  (`node .../@ladybugdb/core/install.js`) because bun skips lifecycle scripts;
  failure mode before repair is a nonzero exit → archfit degrades to `partial`.

## Decision: FIX

Bar met. The adapter now runs the dependants query via `cypher` and parses the
markdown table. Contract change: `Run` returns **file-path-keyed** counts
(was: fictional module-keyed). Consumers aggregate to module granularity
through the symbol graph's defining-file paths using MAX (a module's impact is
its most-depended-on file; SUM would double-count multi-file modules):

- `risk_hub`: `moduleImpactFromFiles` before the bounded [1.0, 2.0] factor.
- `facts.Build`: per-module max over `FileFact.Files`.

## Validation

ccgram with `tools.gitnexus.enabled: on`: coverage `ok` (144 files seen),
144 facts enriched, `polling_state` blast-radius 23 — the depth-1 dependant
signal the Tranche 1.5 design wanted from gitnexus. Config restored after.
