# archfit completion — deterministic core: agent feedback loop + gate fixes

## Overview

Close every remaining DETERMINISTIC gap between archfit today and its end state:
an architecture review and drift-prevention tool an AI coding agent can run on
every change and act on mechanically. Design:
`docs/design/agent-feedback-loop-v0.1.md`. The LLM layer is a separate plan
(`docs/plans/20260611-archfit-tranche2-llm.md`) and depends on nothing here
except Task 1 ordering hygiene.

Scope: populated `agent_tasks` repair blocks (spec §13), SARIF output (spec §12),
`change_locality` metric (spec §10.4), and four correctness/resilience fixes that
make the gate trustworthy in an agent loop (status-filter wiring, SCIP single
pass, explain-pipeline parity, gitnexus fix-or-drop).

Out of scope (deferred, recorded in design §6): MCP server, GitHub Action
wrapper, plugin protocol, `map_staleness` metric.

## Context (from discovery)

- `AgentTask` is an empty placeholder (`internal/model/diagnostic/diagnostic.go`);
  `agent_tasks` always serializes `[]`. Findings already carry `Why`,
  `Constraint`, `Alternatives` — the repair-block inputs exist.
- No SARIF renderer; `--format` enum is `json,text,markdown,md`; config parses a
  dead `sarif:` key.
- `new_cross_module_dependency` fires on ALL cross-module edges —
  `internal/rules/rules.go` TODO: engine never wires baseline evidence.
- `internal/engine/ports.go` TODO(perf): `Strengths` + `Symbols` each run the
  SCIP indexer — every scip-enabled run indexes the repo twice.
- `ExplainCmd` runs Nop providers + empty ChangeHistory — weaker evidence than
  the run that produced the finding. `runPipeline` (added 2026-06-11) is the
  shared path to reuse.
- `internal/extract/gitnexus` calls `gitnexus impact --json <root>` — an
  interface the real gitnexus CLI does not have (its `impact` takes a symbol
  target). The impact map is ALWAYS empty in real runs.
- 12 metrics exist; metric pattern: pure function of `MetricInput`, n/a (never
  false zero) when inputs absent; report-only bands are `info`.
- Renderers iterate the diagnostic generically; `engine.Renderer` is the port.

## Development Approach

- Testing approach: Regular (code first, then table-driven stdlib tests).
  `moq` only if a new tool/subprocess boundary appears (none expected except
  gitnexus, which already has RunnerMock fixtures).
- Complete each task fully before the next; all tests green + `make lint` 0
  issues before moving on.
- Everything here is deterministic and may run on the `check` gate — but only
  `agent_tasks` (gate findings) and `change_locality` (warn band per spec)
  affect gate-visible output; SARIF is a renderer; the fixes change no verdict
  semantics except the status filter (which REDUCES false errors).
- Hard constraint unchanged: zero LLM dependencies anywhere in this plan.

## Testing Strategy

- Unit tests per task (success + n/a/absent + edge case), table-driven.
- Renderer tests assert byte-deterministic output (SARIF double-render compare).
- The status-filter fix needs a regression pair: pre-existing edge (in baseline)
  does NOT fire; genuinely new edge fires.
- Acceptance (Task 9): full sweep on the 5 working repos (ccgram, pumba,
  spotinfo, codegraph, archfit) — no panics, byte-identical double runs,
  `agent_tasks` populated wherever gate findings exist, SARIF validates against
  the 2.1.0 schema.

## Progress Tracking

- Mark completed items `[x]` immediately when done.
- New tasks: `+` prefix. Blockers: `WARN` prefix.
- Keep this file in sync; update scope if implementation deviates.

## Solution Overview

Three feature additions (agent_tasks builder + SARIF renderer + change_locality
metric) following existing patterns (pure builder like `facts.Build`, renderer
port, metric interface), plus four fixes. No new architecture; every addition
slots into an existing seam.

## Technical Details

- **agent_tasks:** `internal/agenttask` package, `Build(findings, rules config,
module map) []diagnostic.AgentTask` — pure, deterministic, sorted by
  FindingID. Goal templates keyed by rule type live with the builder. Only gate
  findings with status new/expired_exception produce tasks. Engine attaches
  during assembly (same spot as FileFacts).
- **SARIF:** `internal/output/sarif`, SARIF 2.1.0, single run. Stable ordering,
  no timestamps. Level map: gate new/expired → error; advisory → warning;
  fixed/excepted → note. Metrics into `run.properties.metrics`.
- **change_locality:** needs the changed-file set in `MetricInput` (new field
  `ChangedFiles []string`, engine passes scope's diff list; empty in --full).
  Value = new cross-module edges originating in changed files (fingerprint not
  in baseline). n/a when no base ref or no baseline.
- **Status filter:** engine passes baseline-accepted fingerprints into
  `rules.Evidence`; `new_cross_module_dependency` computes each candidate
  edge's fingerprint and skips accepted ones. Exact mechanism may shift after
  reading the fingerprint helpers — keep the semantic: "fires only on edges
  absent from baseline".
- **SCIP single pass:** memoize the indexer+reader raw output per (root, lang)
  inside the scip Adapter for the lifetime of one run; `Strengths` and
  `Symbols` parse the same bytes.
- **gitnexus:** spike first — query the installed CLI (`cypher --help`, graph
  schema) for a repo-wide per-module dependant count. If a stable query exists:
  rewrite `Run` to use it (still opt-in, absent when no `.gitnexus` index). If
  not: DELETE the provider (adapter, config key, risk_hub factor, FileFact
  field stays but always nil → remove field too) and record the decision.
  No silently-dead integration may remain.

## What Goes Where

- **Implementation Steps** (`[ ]`): all code, tests, docs in this repo.
- **Post-Completion** (no checkboxes): docs/skills rewrite happens at the end of
  the Tranche-2 plan (single docs pass over both plans' features).

## Implementation Steps

### Task 1: new_cross_module_dependency fires only on new edges

**Files:**

- Modify: `internal/rules/rules.go` (+test), `internal/engine/engine_test.go`

**Deviation (discovery):** the baseline/status machinery ALREADY provides the
"new" semantics — `status.Assign` marks baselined fingerprints `StatusBaseline`
and the verdict counts only new/expired. Filtering inside the rule (the stale
TODO's suggestion) would BREAK fixed-finding detection (suppressed fingerprints
would be falsely reported fixed). The gap was a misleading comment + zero test
coverage, not behavior.

- [x] wire baseline-accepted fingerprints into rule evidence at the engine call site — NOT NEEDED; replaced the stale TODO with documentation of the actual mechanism
- [x] removed the dead `Evidence.Findings` field (never populated, never read)
- [x] tests: engine-level regression — no baseline → new finding + fail; baselined fingerprint → StatusBaseline + pass + finding still emitted (fixed-detection invariant)
- [x] run tests — green before Task 2

### Task 2: SCIP single-pass per run

**Files:**

- Modify: `internal/extract/scip/scip.go` (+`scip_test.go`/`symbols_test.go`)

- [x] memoize the index+reader pipeline result per (root, tool-lang) in the Adapter; `Strengths`+`Symbols` share it (coverage Tool rewrapped per caller; Status/Version preserved)
- [x] test: RunnerMock counts ONE indexer invocation when both methods run
- [x] verify `archfit scan` on this repo: scip coverage unchanged (ok/ok), wall ~4.5s vs ~8s double-indexed
- [x] run tests — green before Task 3

### Task 3: explain runs the real pipeline

**Files:**

- Modify: `cmd/archfit/main.go` (+`main_test.go`)

- [x] `ExplainCmd` resolves evidence via `runPipeline` (same providers/history as check); output adds resolved modules, locations, allowed alternatives
- [x] test: explain on the violating fixture prints the finding with module labels resolved
- [x] run tests — green before Task 4

### Task 4: gitnexus fix-or-drop (decision gate)

**Files:**

- Spike note: `docs/plans/notes/gitnexus-adapter-decision.md`
- Modify or delete: `internal/extract/gitnexus/*`, config key, `risk_hub` factor, `FileFact.GitnexusImpact`

- [x] spike: enumerate what the installed gitnexus CLI can actually return repo-wide; write the decision note (pre-register: fix only if ONE stable command yields per-module counts) — `gitnexus cypher` qualifies; decision: FIX
- [x] implement the decision — adapter rewritten to the cypher dependants query; contract is now FILE-keyed counts; risk_hub + facts aggregate to modules via symbol-graph paths (MAX over files)
- [x] tests updated to the real contract; no test asserts the fictional `impact --json <root>`
- [x] run tests — green before Task 5 — live ccgram validation: coverage ok, 144 facts enriched, polling_state blast-radius 23

### Task 5: agent_tasks repair blocks

**Files:**

- Modify: `internal/model/diagnostic/diagnostic.go`
- Create: `internal/agenttask/agenttask.go` (+test)
- Modify: `internal/engine/engine.go` (+test), `internal/output/markdown/markdown.go` (+test), `internal/output/jsonout/jsonout_test.go`
- Modify: `internal/arch_test.go` (agenttask joins the core ring)

- [ ] replace the placeholder `AgentTask` with the spec §13 shape (finding_id, rule_id, goal, constraints, files, validation) — JSON tags per design doc
- [ ] `agenttask.Build`: goal template per rule type; constraints from rule def + target module public globs; validation = reproducible check command; sorted, deterministic
- [ ] engine attaches tasks for active gate findings only (advisories never)
- [ ] markdown: compact "Agent tasks" section; JSON: full block; tests for both + empty case
- [ ] run tests — green before Task 6

### Task 6: SARIF 2.1.0 renderer

**Files:**

- Create: `internal/output/sarif/sarif.go` (+test)
- Modify: `cmd/archfit/main.go` (format enum + case)

- [ ] renderer per design §3 (rules, results, levels, locations, metrics in properties, no timestamps)
- [ ] `--format sarif` in the enum; render wired in check/scan path
- [ ] tests: golden shape assertions + double-render byte-identical + empty diagnostic
- [ ] validate one real output against the SARIF 2.1.0 schema (jv or sarif-tools; record how)
- [ ] run tests — green before Task 7

### Task 7: change_locality metric

**Files:**

- Modify: `internal/metrics/metrics.go` (MetricInput.ChangedFiles), `internal/engine/engine.go`, `cmd/archfit/main.go` (pass diff scope)
- Create: `internal/metrics/change_locality.go` (+test)
- Modify: `internal/metrics/metrics_test.go`, `internal/engine/engine_test.go` (metric count 12 → 13), `.archfit.yaml`

- [ ] thread the resolved diff file set into MetricInput (empty in --full)
- [ ] metric per design §4: new cross-module edges from changed files; warn band on baseline regression; n/a without base/baseline
- [ ] tests: new edge counted; unchanged file's edge not; --full → n/a; determinism
- [ ] run tests — green before Task 8

### Task 8: stale comment + doc hygiene

**Files:**

- Modify: `internal/metrics/doc.go`, `internal/metrics/metrics.go`, `internal/rules/doc.go`, `internal/model/coupling/coupling.go`, `docs/guide/README.md`

- [ ] drop stale "Phase 1" framing from package docs; fix the dead `docs/output.md` link
- [ ] run `make lint` + full suite — green before Task 9

### Task 9: acceptance sweep (the gate for this plan)

**Files:**

- Create: `docs/plans/notes/completion-deterministic-validation.md`

- [ ] build `.bin/archfit`; run full scan + check on ccgram, pumba, spotinfo, codegraph, archfit — no panics; record per-repo verdicts
- [ ] double-run byte-identical on at least archfit (scip on) and ccgram
- [ ] `agent_tasks` populated on a repo with gate findings; SARIF emitted and schema-valid
- [ ] `go test -count=1 ./...` green; `make lint` 0 issues
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion

Docs + skills rewrite happens once, at the end of the Tranche-2 plan (single
pass covering both plans' features). Deferred items stay recorded in design §6.
