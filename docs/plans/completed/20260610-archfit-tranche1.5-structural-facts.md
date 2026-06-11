# archfit Tranche 1.5 — Per-file structural-facts block (deterministic)

## Overview

Make the intra-module hubs the validation spike's blind classifier missed
(`polling_state`, `directory_callbacks`) visible to the Tranche-2 LLM — by emitting
deterministic per-file **structural facts**, NOT risk rankings.

This supersedes the first Tranche-1.5 attempt
(`docs/plans/20260609-archfit-tranche1.5-intra-module-hubs.md`), whose two ranking
metrics (`cohesion_spread`, `shared_state_hub`) FAILED their 4-repo gate
(`docs/plans/notes/intra-module-hub-validation.md`). A follow-up signal probe on ccgram
established why and what works:

- **Deterministic code cannot RANK these hubs by risk.** Separating `config` (benign,
  read-only) from `polling_state` (risky, mutable), or `bootstrap` (wiring) from
  `directory_callbacks` (grab-bag), needs mutability/intent — the LLM's job. So the
  deterministic layer must emit FACTS and let the LLM rank.
- **`polling_state`** is invisible to SCIP symbol fan-in (peaks at 2; file max 8 = noise
  floor). It IS visible via SCIP **module-level inbound fan-in** (23 importing modules) and
  **GitNexus blast-radius** (13 direct / 41 transitive). No reader change recovers it.
- **`directory_callbacks`** cannot be measured by SCIP LCOM (Python SCIP omits
  `enclosing_range` → all symbols collapse to one cohesion component). It IS visible via
  **outbound distinct-destination count x LOC** (46 destinations) — the original
  `cohesion_spread` idea without its subsystem-collapse.

**Conclusion (probe-validated): NO `scip_reader.py` change is needed.** Every
discriminating fact already exists in collected data. We assemble a neutral per-file facts
block and let the Tranche-2 LLM judge.

Source of truth: `docs/design/hybrid-llm-strength-v0.1.md` §7 (Tranche 1.5).

**Hard constraint (unchanged):** `check` — the CI gate — stays pure deterministic and
LLM-free. The facts block is neutral evidence: no band, no score, never sets delta, never
gates.

## Context (from discovery)

- **Stack:** Go, kong CLI, hexagonal. Metrics are pure functions of `MetricInput`; engine-
  computed inputs (e.g. `fitness.Detect`, the ownership resolver) are produced in `cmd`/engine
  and attached to the diagnostic — the facts block follows THAT pattern, not the metric pattern.
- **Data already collected (no reader change):**
  - `symbol.Graph` (`internal/model/symbol`): `Module` (symbol -> dotted file/module path),
    `FanIn` (symbol -> distinct referencing docs), `Refs` (from_symbol -> set(to_symbol),
    cross-module). From `Refs` + `Module`: per-file outbound distinct-destination modules and
    per-file inbound module fan-in are both computable.
  - `MetricInput.FileLOC` (file -> LOC), `MetricInput.CoChange` (file-pair -> commits).
  - GitNexus optional provider (`internal/extract/gitnexus`, `tools.gitnexus.enabled`): symbol/
    file impact (blast-radius). Already integrated as an opt-in enrichment for `risk_hub`.
- **The failed first attempt left these to remove/repurpose:** `internal/metrics/cohesion_spread.go`
  (+test), `internal/metrics/shared_state_hub.go` (+test), their `metrics.New()` registrations,
  their `.archfit.yaml` entries, their config-test + renderer-test cases, and the
  `metrics_test.go`/`engine_test.go` metric-count assertions (currently 14).
- **Diagnostic + output:** `internal/model/diagnostic` holds the `Diagnostic` (metrics,
  findings, tool_coverage, ...). Renderers in `internal/output/{console,markdown,jsonout}`
  iterate it generically. The facts block is a new neutral field on the diagnostic.

## Development Approach

- **Testing approach: Regular** (code first, then table-driven stdlib tests). No new
  tool/subprocess boundary, so no new `moq`.
- Complete each task fully before the next; small focused changes; tests every task (success +
  n/a + the relevant edge), as separate checklist items; all tests pass before the next task.
- The facts block is **report-only neutral evidence** — never gates, never sets delta.
- Absent SCIP/symbol graph -> facts block is empty/absent (info), never a false zero.
- **No `scip_reader.py` change.** If implementation seems to need one, STOP and reassess — the
  probe says the facts come from already-collected data.

## Testing Strategy

- **Unit tests:** required every task. Table-driven over hand-built `symbol.Graph` + `FileLOC`
  - `CoChange` fixtures (no real SCIP). Assert the facts (inbound fan-in, outbound destinations,
    LOC, co-change) compute correctly and deterministically (sorted, total-order, tie-broken).
- **Acceptance (Task 6) = the spike RE-RUN, not a metric rank.** A blind classifier, fed the
  enriched ccgram evidence (facts block) under a by-document firewall, must now surface
  `polling_state` and `directory_callbacks` as risky intra-module hubs. Neutral facts only
  (numbers, not a "here are the hubs" list) or the test is circular.
- **Determinism (Task 7):** two `check` runs on the same commit/config produce byte-identical
  output; grep the `check` path — zero LLM dependencies; confirm the facts block does not enter
  the verdict.
- No e2e/UI tests (CLI project).

## Progress Tracking

- Mark completed items `[x]` immediately when done.
- New tasks: `+` prefix. Blockers: `WARN` prefix.
- Keep this file in sync; update scope if implementation deviates.

## Solution Overview

A new `internal/facts` package assembles, per source file, a neutral `FileFact` from
already-collected data:

- `inbound_module_fanin` — count of distinct OTHER modules whose symbols reference this file's
  symbols (from `symbol.Graph.Refs` reversed + `Module`).
- `outbound_destinations` — count of distinct destination modules this file's symbols reference
  (from `Refs` + `Module`), at raw module granularity (NOT subsystem-collapsed).
- `loc` — from `FileLOC`.
- `cochange_partners` — top co-change partner files (from `CoChange`).
- `blast_radius` — GitNexus impact (direct + transitive) when `tools.gitnexus.enabled: on` and
  present; absent otherwise.

`facts.Build(...)` is a pure function; `cmd`/engine calls it (like `fitness.Detect`) and
attaches `[]FileFact` to the diagnostic. Renderers emit it as neutral evidence: full list in
JSON (for the LLM), a compact top-by-each-axis table in markdown. Console stays minimal. The
block carries no risk verdict — ranking/judgment is the Tranche-2 LLM's job.

## Technical Details

- **`FileFact`** (in `internal/facts` or `internal/model/...`): `{ File string; Module string;
InboundModuleFanIn int; OutboundDestinations int; LOC int; CoChangePartners []string;
BlastRadius *BlastInfo }` (BlastRadius nil when gitnexus absent). All counts deterministic.
- **Inbound module fan-in:** for each file F, distinct `Module[from]` over all `from -> to` in
  `Refs` where `Module[to]`'s file == F and `Module[from] != Module[to]`. (config will score
  high here too — that is EXPECTED; the LLM separates read-only config from mutable state.)
- **Outbound destinations:** for each file F, distinct `Module[to]` over all `from -> to` where
  `from`'s file == F and modules differ. directory_callbacks ~ 46.
- **Neutrality guard (load-bearing for the Task-6 gate):** the facts block is plain per-file
  numbers. It MUST NOT label, rank-by-risk, star, or annotate any file as a "hub"/"risk" — or
  the spike re-run becomes circular (the LLM would just read our conclusion).
- **No new gate/config tool keys.** Reuses `tools.gitnexus`. The block is always emitted when a
  symbol graph exists; empty otherwise.

## What Goes Where

- **Implementation Steps** (`[ ]`): all code, tests, docs in this repo.
- **Post-Completion** (no checkboxes): the Tranche-2 build remains gated on this Tranche's
  spike re-run passing.

## Implementation Steps

### Task 1: Remove the failed ranking metrics

**Files:**

- Delete: `internal/metrics/shared_state_hub.go`, `internal/metrics/shared_state_hub_test.go`
- Delete: `internal/metrics/cohesion_spread.go`, `internal/metrics/cohesion_spread_test.go`
- Modify: `internal/metrics/metrics.go` (remove both registrations from `New()`)
- Modify: `internal/metrics/metrics_test.go`, `internal/engine/engine_test.go` (metric count 14 -> 12, drop the two names)
- Modify: `.archfit.yaml` (remove the two `metrics.*` entries), `internal/config/config_test.go`, `internal/config/testdata/new_tools_metrics.yaml` (drop the added entries/cases)
- Modify: `internal/output/{console,markdown,jsonout}/*_test.go` (remove the two metrics' render test cases)

- [x] delete the two metric files + their tests; remove registrations from `metrics.New()`
- [x] fix metric-count/name assertions (`metrics_test.go`, `engine_test.go`) to 12
- [x] remove the two `.archfit.yaml` entries and the config/renderer test cases that referenced them
- [x] run `go build ./...` and `go test ./...` — must be fully green before Task 2
- [x] run `make lint` — 0 issues

### Task 2: facts.Build assembler (pure, from collected data)

**Files:**

- Create: `internal/facts/facts.go`
- Create: `internal/facts/facts_test.go`

- [x] define `FileFact` and `Build(g symbol.Graph, fileLOC map[string]int, coChange map[[2]string]int) []FileFact` — compute inbound module fan-in, outbound distinct-destination count, LOC, top co-change partners per file; deterministic total-order sort (by file name) for stable output
- [x] empty/absent symbol graph -> empty slice (no panic, no false zero)
- [x] keep it neutral: facts only, no risk label/rank field
- [x] write tests: a fixture where one file has high inbound fan-in and another high outbound destinations; co-change partners resolved; empty-graph -> empty; determinism (stable order)
- [x] run tests — must pass before Task 3

**Deviation (Task 3 discovery):** the first Build used a heuristic module-key→file-path
prefix join. That join silently misses for Python (dotted module keys vs slash file
paths — ccgram, the Task-6 gate repo, would get LOC=0 and no co-change partners) and
attributed nested-module LOC nondeterministically (first match in map order). Fixed by
carrying the reader's already-emitted per-symbol `path` field through `symbol.Graph.Path`
(Go-side parse only — NO `scip_reader.py` change) and joining exactly on defining-file
paths. `FileFact` moved to `model/diagnostic` (it is part of the output contract; model
stays import-clean) with JSON tags; the speculative `File`==`Module` dual field became
`Module` + `Files []string` (real defining-file list, useful to the LLM).

### Task 3: Attach facts to the diagnostic and wire through engine/cmd

**Files:**

- Modify: `internal/model/diagnostic/*.go` (add a `FileFacts []FileFact` field, or a small `ModuleInternals` holder)
- Modify: `internal/engine/engine.go`, `internal/engine/ports.go` as needed
- Modify: `cmd/archfit/main.go` (call `facts.Build` with the collected SymbolGraph/FileLOC/CoChange and attach)
- Modify: `internal/engine/engine_test.go`

- [x] add the facts field to the diagnostic with a doc comment (neutral evidence, never gates)
- [x] wire `cmd`/engine to build and attach the facts (mirror how `FitnessSignals`/ownership are produced and threaded); empty when SCIP off — built inside `engine.Run` (NOT cmd): the symbol graph only exists inside the engine (`sr.Symbols`), so the engine is the only correct call site
- [x] confirm the facts field is NOT consumed by `computeVerdict`/gate logic
- [x] write tests: engine attaches populated facts; empty when no symbol graph
- [x] run tests — must pass before Task 4

### Task 4: Render the facts block (JSON + markdown)

**Files:**

- Modify: `internal/output/jsonout/jsonout.go` (+test)
- Modify: `internal/output/markdown/markdown.go` (+test)
- Modify: `internal/output/console/console.go` only if needed (keep console minimal)

- [x] JSON: emit the full `FileFacts` list (for the LLM) — confirm it serializes; add/verify a test
- [x] markdown: a compact "Per-file structural facts" section — top-N files by each axis (inbound, outbound, LOC), neutral table, deterministic; n/a/empty rendered cleanly (rendered as "Structural facts (neutral evidence)" — `-` lists per package convention, no box tables)
- [x] keep it NEUTRAL — no "hub"/"risk" labeling (Task-6 gate depends on this) — enforced by a renderer test that greps the output for hub/risk
- [x] write/update renderer tests (present + empty)
- [x] run tests — must pass before Task 5

### Task 5: GitNexus blast-radius enrichment (optional)

**Files:**

- Modify: `internal/facts/facts.go` (+test) — accept an optional impact map
- Modify: `cmd/archfit/main.go` (pass gitnexus impact when `tools.gitnexus.enabled: on`)
- Reuse: `internal/extract/gitnexus`

- [x] when gitnexus is enabled + present, enrich each `FileFact` with blast-radius (direct + transitive); nil/absent otherwise (no fabrication) — implemented as `GitnexusImpact *int` (the collected `GitnexusImpact map[string]int` carries a single impact count per module, not a direct/transitive split; the plan's `BlastInfo` assumed data we do not collect)
- [x] record source/coverage so the LLM knows whether blast-radius was available — the gitnexus tool-coverage entry (ok/absent/partial) is already in the diagnostic; `gitnexus_impact` is omitted from JSON when nil
- [x] write tests with a canned impact map (present enriches; absent -> nil)
- [x] run tests — must pass before Task 6

**WARN (pre-existing, discovered during this task):** `internal/extract/gitnexus` invokes
`gitnexus impact --json <root>` — an interface the real gitnexus CLI does not have (real
`impact` takes a _symbol_ target and has no repo-wide JSON dump). In real runs the impact
map is always empty (coverage `partial`), so this enrichment never fires. Tracked as a
follow-up outside this Tranche; the Task-6 gate must therefore pass on SCIP facts alone.

### Task 6: Acceptance — spike RE-RUN (the gate)

**Files:**

- Create: `docs/plans/notes/structural-facts-spike-rerun.md` (pre-registered bar + result)

- [x] build `.bin/archfit`; run a full scan on ccgram (scip on, gitnexus on if available); capture the facts block (JSON) — gitnexus absent (adapter contract mismatch, see Task 5 WARN); 383 modules captured
- [x] PRE-REGISTER the bar BEFORE classifying: a blind classifier, fed ONLY the facts block + permission to read ccgram `src/` (firewalled from all `docs/**`, CLAUDE.md, AGENTS.md, llm.txt, the spike notes), must flag `polling_state` (mutable shared-state hub) AND `directory_callbacks` (low-cohesion grab-bag) among its top intra-module risks, judging mutability/cohesion from the code + the neutral facts
- [x] run the blind classifier; diff vs the pre-registered bar — polling_state ranked #2 (mutable shared-state), directory_callbacks #3 (low-cohesion grab-bag); benign high-scorers (config, providers.base, thread_router) correctly cleared
- [x] WARN if the targets do NOT surface: STOP — not triggered; both targets surfaced without gitnexus
- [x] write the result log; mark gate PASS/FAIL — **PASS** (`docs/plans/notes/structural-facts-spike-rerun.md`)

### Task 7: Verify acceptance + determinism + docs

**Files:**

- Modify: `README.md`, `docs/design/hybrid-llm-strength-v0.1.md` (mark Tranche 1.5 implemented)

- [x] full suite `go test -count=1 ./...` green; `make lint` 0 issues
- [x] determinism: two `check` runs byte-identical; grep `check` path — zero LLM deps; confirm verdict unaffected by the facts block — verified byte-identical both with SCIP off and on; no LLM imports anywhere in `internal`/`cmd`; engine test asserts FileFacts attach without changing the verdict, and `computeVerdict` does not read them
- [x] `archfit scan` on this repo — no panic, facts block renders — 87 modules with exact Go LOC joins (self-config now enables `tools.scip`)
- [x] document the facts block (JSON shape + markdown section) in README; mark design §7 Tranche 1.5 implemented
- [x] move both this plan and the superseded `20260609-...intra-module-hubs.md` to `docs/plans/completed/`

## Post-Completion

_No checkboxes — external action or later tranche._

**Tranche-2 coupling refinement remains gated on Task 6 passing** — the whole point of this
Tranche is that the LLM can now see the intra-module hubs. If Task 6 fails, Tranche 1.5 is
reassessed (add the missing fact / require gitnexus) before Tranche 2 proceeds.

**GitNexus is opt-in:** when absent, the facts block omits blast-radius and the polling_state
axis is weaker (SCIP module-inbound-fan-in only). Document this coverage caveat; the spike
re-run should record whether it passed with or without gitnexus.
