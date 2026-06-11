# Intra-module Hub Metrics — 4-repo Validation

## Purpose

Gate check for Tranche 1.5 Task 5. Validates that `cohesion_spread` and `shared_state_hub`
surface the gold-standard hubs on ccgram without being dominated by architect-blessed
centralizations, and that both metrics are distinct from `risk_hub`/`blast_radius`/`structural_weight`
across all four repos.

## Final Threshold Values

- `cohesionSpreadLOCFloor` = 150 (unchanged from initial default)
- `sharedStateHubHotThreshold` = 8 (unchanged from initial default)
- `subsystem()` granularity = parent-package strip (last dotted segment removed, unchanged)

No tuning was applied. The task-5 tuning investigation showed:

- LOC floor is irrelevant for `directory_callbacks` (1076 LOC, far above any floor).
- A coarser 2-level subsystem strip collapses all handlers into 3 buckets (ccgram, ccgram.handlers,
  ccgram.providers), destroying the spread signal entirely.
- `HOT_THRESHOLD` feeds only `hotCount`, which is the secondary sort key for `shared_state_hub`;
  it cannot move `polling_state` above files with strictly higher max single-symbol fan-in.

## ccgram Results (Gold Standard)

Binary: `/Users/alexei/Workspace/archfit/.bin/archfit`
Command: `archfit check --full --advisory --report --severity low --format json`
SCIP: already enabled in `.archfit.yaml`.

### cohesion_spread top-5 (ccgram)

| Rank | File                                      | Spread | LOC |
| ---- | ----------------------------------------- | ------ | --- |
| 1    | ccgram.handlers.text.text_handler         | 11     | 550 |
| 2    | ccgram.handlers.polling.window_tick.apply | 11     | 541 |
| 3    | ccgram.handlers.registry                  | 9      | 168 |
| 4    | ccgram.handlers.commands.forward          | 8      | 284 |
| 5    | ccgram.bootstrap                          | 8      | 277 |

**Target `directory_callbacks`: rank 6, spread 7, LOC 1076** (not in top-5).

Subsystem targets for directory_callbacks: ccgram, ccgram.handlers, ccgram.handlers.messaging_pipeline,
ccgram.handlers.shell, ccgram.handlers.status, ccgram.handlers.topics, ccgram.providers (7 distinct).

Subsystem targets for bootstrap (rank 5, spread 8): ccgram, ccgram.handlers, ccgram.handlers.commands,
ccgram.handlers.messaging_pipeline, ccgram.handlers.polling, ccgram.handlers.shell, ccgram.handlers.topics,
ccgram.providers (8 distinct). Bootstrap has higher spread because it is wiring code that explicitly
imports from 8 distinct subsystems as its job.

Blessed hubs in top-5: only `ccgram.bootstrap` (rank 5). Not dominated (1 of 5).

### shared_state_hub top-5 (ccgram)

| Rank | File                             | Top symbol                  | Max fan-in | Hot count |
| ---- | -------------------------------- | --------------------------- | ---------- | --------- |
| 1    | ccgram.config                    | config/                     | 52         | 3         |
| 2    | ccgram.thread_router             | thread_router.thread_router | 47         | 8         |
| 3    | ccgram.tmux_manager              | tmux_manager                | 38         | 8         |
| 4    | ccgram.telegram_client           | TelegramClient#             | 29         | 2         |
| 5    | ccgram.handlers.callback_helpers | get_thread_id()             | 28         | 2         |

**Target `polling_state`: rank 16, max fan-in 8, top symbol `lifecycle_strategy`** (not in top-5).

Blessed hubs in top-5: `ccgram.config` (rank 1), `ccgram.thread_router` (rank 2),
`ccgram.tmux_manager` (rank 3). Three of five slots are architect-blessed centralization.

`polling_state`'s max single-symbol fan-in is 8, well below the top-5 floor of 28.
The primary sort key is raw max fan-in, which is not a tunable threshold. No legitimate
parameter change can surface `polling_state` into top-5 without redefining the metric.

**Root cause of the shared_state_hub failure:** The metric measures "which file owns the
highest-referenced single symbol." In Python codebases this captures module-level singletons
(the config object, the thread_router singleton, the tmux_manager instance) — all
infrastructure-layer objects designed to be widely shared. These correctly score high because
they ARE heavily shared. The intra-module mutable state problem (`polling_state.lifecycle_strategy`)
shows fan-in=8 within a polling subsystem, which is structurally lower than the application-wide
singletons. The metric cannot distinguish "read-only config singleton" from "mutable shared state"
— both appear as high fan-in. This is a signal-definition limit, not a threshold issue.

## Other Repos

### codegraph (TypeScript)

SCIP enabled temporarily; restored after run.

**cohesion_spread top-5:**

1. .../index/ts [spread 20, LOC 1103]
2. .../codegraph/ts [spread 18, LOC 1727]
3. .../tree-sitter/ts [spread 14, LOC 4214]
4. .../index/ts [spread 12, LOC 1078]
5. .../index/ts [spread 11, LOC 457]

**shared_state_hub top-5:**

1. .../types/ts [top: ts`/ fan-in=48, 39 hot]
2. .../types/ts [top: ts`/ fan-in=26, 25 hot]
3. .../tree-sitter-helpers/ts [top: ts`/ fan-in=26, 4 hot]
4. .../tree-sitter-types/ts [top: LanguageExtractor# fan-in=20, 22 hot]
5. .../d/ts [top: Node#`type`() fan-in=20, 12 hot]

Signal quality: cohesion_spread surfaces genuine wide-spread index/codegraph files (large
orchestrators reaching many modules). shared_state_hub surfaces type-definition files with
very high fan-in, which are structural (types are inherently shared) — similar pattern to
the ccgram blessed-hub dominance problem.

### pumba (Go)

SCIP enabled temporarily; restored after run.

**cohesion_spread top-5:**

1. cmd [spread 10, LOC 503]
2. .../netem/cmd [spread 5, LOC 516]
3. .../lifecycle/cmd [spread 5, LOC 369]
4. .../chaos/netem [spread 4, LOC 818]
5. .../chaos/iptables [spread 4, LOC 299]

**shared_state_hub top-5:**

1. pkg/container [top: container`/ fan-in=95, 57 hot]
2. pkg/chaos [top: chaos`/ fan-in=60, 11 hot]
3. .../chaos/cliflags [top: cliflags`/ fan-in=33, 9 hot]
4. .../chaos/cmd [top: Spec#Build/ fan-in=17, 10 hot]
5. .../runtime/docker [top: dockerClient# fan-in=17, 3 hot]

Signal quality: cohesion_spread correctly surfaces the cmd entry-point (legitimately
reaches all chaos subcommands) and orchestrators. shared_state_hub shows pkg/container
and pkg/chaos as top entries — these are the core package types, not intra-module mutable
state. cmd at rank 4 is the legitimate hub. Blast_radius confirms container and chaos are
already known hubs.

### spotinfo (Go, small repo)

SCIP enabled temporarily; restored after run.

**cohesion_spread top-5:**

1. cmd/spotinfo [spread 2, LOC 712]
2. internal/mcp [spread 1, LOC 518]

Only 2 files above LOC floor with any spread — too small to show hub patterns.

**shared_state_hub top-5:**

1. internal/spot [top: Advice# fan-in=17, 18 hot]
2. internal/mcp [top: Config#Logger/ fan-in=6, 0 hot]
3. cmd/spotinfo [top: mcpModeEnv/ fan-in=2, 0 hot]
4. .../spotinfo/test [top: benchmarks/ fan-in=1, 0 hot]
5. .../mcp/test [top: tests/ fan-in=1, 0 hot]

Only 6 files total (tiny repo). Both metrics return sensible output — no panic, no false
zeros, n/a-not-triggered (SCIP did index).

## Distinctness Check

All three existing metrics for comparison:

**ccgram risk_hub top-5:** ccgram.window_state_store, .../handlers/callback_data, .../providers/base,
ccgram.telegram_client, ccgram.config — all inbound surface-breadth leaders.

**ccgram blast_radius top-5:** ccgram.utils, ccgram.config, .../providers/base,
ccgram.expandable_quote, ccgram.thread_router — transitive dependency count.

**ccgram structural_weight top-5:** ccgram.hook (1234 LOC), directory_callbacks (1076 LOC),
polling_state (1018 LOC), ccgram.tmux_manager (956 LOC), .../providers/gemini (905 LOC) — pure LOC.

**cohesion_spread top-5:** text_handler, window_tick/apply, registry, commands/forward, bootstrap.
None of these appear in risk_hub or blast_radius top-5. cohesion_spread is measuring outbound
fan-out breadth, not inbound surface or dependency count. **Distinctness confirmed.**

**shared_state_hub top-5:** ccgram.config, ccgram.thread_router, ccgram.tmux_manager,
ccgram.telegram_client, ccgram.handlers.callback_helpers.
config and thread_router overlap with blast_radius top-5; telegram_client overlaps with
risk_hub top-4. The metric is partially capturing the same infrastructure singletons
already flagged by other metrics. **Partial overlap with existing metrics on ccgram.**

Across pumba and codegraph, cohesion_spread and shared_state_hub show different top files
from risk_hub and structural_weight in every case — distinctness holds on Go and TypeScript repos.

## Signal-to-Noise Check (Real Gate)

Architect-blessed centralizations for ccgram (from `.archfit.yaml` and review): `ccgram.config`,
`ccgram.utils`, `ccgram.tmux_manager`, `ccgram.bootstrap`, `ccgram.bootstrap`/`ccgram.app_bootstrap`.

**cohesion_spread:** only `ccgram.bootstrap` appears in top-5 (rank 5). NOT dominated. The four
non-blessed entries (`text_handler`, `window_tick/apply`, `registry`, `commands/forward`) are
legitimate large handlers with wide outbound spread. Signal-to-noise is acceptable for this metric
on its own. However the gold-standard target `directory_callbacks` is at rank 6.

**shared_state_hub:** THREE of five top entries are architect-blessed centralization (`config`,
`thread_router`, `tmux_manager` at ranks 1-3). The metric IS dominated by blessed hubs on ccgram.
This is the same failure mode that killed transitive-`risk_hub`.

## Tuning Investigation Summary

The only tunable knobs are `HOT_THRESHOLD`, `LOC_FLOOR`, and `subsystem()` granularity.

- `LOC_FLOOR`: raising it would remove small files but `directory_callbacks` (1076 LOC) is already
  far above any plausible floor; not relevant. Lowering it adds noise.
- `HOT_THRESHOLD`: controls only the secondary sort key `hotCount` for `shared_state_hub`. Cannot
  move `polling_state` above files with strictly higher `maxFanIn`. Irrelevant to the failure.
- `subsystem()` granularity: tested 2-level strip. Result collapses all handler subsystems into
  3 buckets — destroys signal. The current 1-level parent-package strip is the correct granularity.

No threshold combination can surface both targets into top-5 without redefining the primary
scoring function. The failure is structural (signal definition), not a parameter choice.

## VERDICT: FAIL

**Reason:** Two blockers, one per metric.

**cohesion_spread:** `directory_callbacks` ranks 6th (spread=7, LOC=1076). The gold standard
requires it in top-5. The list is NOT dominated by blessed hubs (1/5 is bootstrap). The metric
itself has good signal — `directory_callbacks` is genuinely the 6th highest-spread file — but
the metric definition prioritizes outbound fan-out breadth, and `bootstrap` (a blessed wiring
hub that legitimately imports from 8 subsystems) outscores it at spread=8. No legitimate
subsystem granularity change can close this gap without damaging signal on other repos.

**shared_state_hub:** `polling_state` ranks 16th (max fan-in=8). The gold standard requires it
in top-5. The top-5 is dominated 3/5 by architect-blessed centralizations (config, thread_router,
tmux_manager) with fan-in scores 38-52. The metric measures max single-symbol fan-in, which
is dominated by application-wide infrastructure singletons. Intra-module mutable state with
fan-in=8 within a polling subsystem cannot compete. This is a signal-definition limit: the
metric cannot distinguish "intra-module mutable state hub" from "application-wide read-only
config singleton." Both appear as high fan-in files.

**Reassessment needed before proceeding** (per plan ⚠️ branch): the `shared_state_hub` signal
definition needs to be reconsidered. Options include: (a) normalizing fan-in by module size to
penalize app-wide singletons; (b) restricting to intra-module fan-in only (same config-module
files referencing each other); (c) using a different primary signal (e.g., files referenced only
by siblings within the same module, not cross-module). `cohesion_spread` is close (rank 6) but
needs to account for the fact that wiring/bootstrap files legitimately score high on outbound
spread — possibly weighting against known bootstrap-pattern files, or using module-scoped spread.
