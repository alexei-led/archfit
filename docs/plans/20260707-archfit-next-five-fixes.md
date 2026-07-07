# Plan: Archfit Next Five Fixes

## Overview

This plan addresses the next five issues after the prior balanced-coupling evidence work.
It keeps the deterministic gate/score boundary intact and uses Ralphex's required
`### Task N:` task headers with task-local checkboxes only.

## Why these five, in this order

Advisor-selected priority order:

1. Coverage ratio semantics must stop exceeding 100%.
2. SCIP semantic-strength overlay must stay visible on successful zero-hit runs.
3. Advisory findings need their own repair-task channel.
4. The `internal/model` hub needs measurement splitting and then a focused refactor.
5. Runtime/dynamic evidence needs report-only distance-config candidates, not scoring.

## Current evidence snapshot

- Current self-run: `coverage = 1.0003789314134142` (>100%).
- Current self-run: `semantic_strength_overlay = null`.
- Current self-run: `dynamic_imports = 3`, `runtime_async_edges = 1`, `dynamic_connascence_signals = 4`.
- Current self-run: `internal/model` receives 32 advisories, including 19 critical and 1 high.
- Current self-run: `blast_radius = 7` hubs, including `internal/model/diagnostic`, `internal/model/graph`, `internal/model/coupling`.
- Current self-run: `warnings = 88`, `gate_findings = 0`.

Advisor verdict: proceed, keep fixes deterministic, do not retune scoring, do not
auto-score runtime/dynamic evidence, and do not put advisory work into gate-only
`agent_tasks[]`.

## Guardrails

- Do not change `bc_score.v6`, ordinals, or `coupling_balance` formula.
- Do not auto-score runtime/dynamic evidence.
- Keep `agent_tasks[]` gate-only.
- Add new advisory/task channels only as report-only diagnostics.
- Use `/tmp` for scratch JSON and compare outputs.
- Do not create or update report directories.

## Selected solutions

- **Issue 1:** treat `deploy-unit` as auxiliary distance evidence, not file coverage.
  Do not clamp coverage to 100%.
- **Issue 2:** track SCIP overlay candidates by SCIP run status, not by map length.
  Empty-but-successful runs should show `candidate_edges` with `applied=0` / `missed>0`.
- **Issue 3:** add a deterministic `advisory_tasks[]` block for grouped BC advisories.
  Keep it separate from `agent_tasks[]`.
- **Issue 4:** strangle the `internal/model` hub in two passes: first split measurement
  in `.archfit.yaml`, then refactor one hot seam.
- **Issue 5:** add report-only `distance_config_candidates[]` from runtime/dynamic
  evidence and surface review hints in config update.

## Execution order

- Task 1 and Task 2 are correctness fixes and should land first.
- Task 3 reduces advisory noise and should land before the large hub refactor.
- Task 4 is the highest blast-radius change and should come after the smaller fixes.
- Task 5 is report-only and can land last.

## Validation baseline

Run these after each task unless the task narrows the checks further:

```sh
make test
.bin/archfit analyze --full --json -c .archfit.yaml > /tmp/archfit-next-five.json
.bin/archfit analyze --gate --full -c .archfit.yaml
```

---

### Task 1: Normalize coverage accounting so deploy-unit never pushes coverage above 100%

Why this needs a fix:

- Coverage is mathematically impossible today: the current JSON run reports >100%.
- `deploy-unit` is evidence for distance and topology, not a file-scope coverage
  extractor.
- Letting an auxiliary row contribute to coverage distorts the only ratio metric
  that the repo presents as a percentage.

Advisor-selected solution:

- Keep `deploy-unit` in diagnostics and distance confidence.
- Remove it from the coverage denominator, e.g. by making the row non-contributing
  (`FilesApplicable: 0`) or by adding an explicit contribution flag that coverage
  ignores.
- Let `distance_context.deploy_unit_detected_modules` carry the useful signal.

Rejected alternatives:

- Clamp the final coverage value to 1.0. That hides a semantic bug.
- Create a new "auxiliary coverage" metric. That adds a new surface to maintain
  without fixing the current one.

Likely files:

- `cmd/archfit/pipeline_run.go`
- `internal/metrics/boundary/coverage.go`
- `internal/metrics/boundary/coverage_test.go` or equivalent test file
- `cmd/archfit/pipeline_test.go`
- `docs/guide/metrics.md`

Metrics / evidence to consume:

- `metrics.coverage.value` must stay `<= 1.0`
- `tool_coverage[deploy-unit]` must still exist
- `distance_context.deploy_unit_detected_modules` must still report detected modules

Task checklist:

- [x] Change the deploy-unit row so coverage no longer counts it.
- [x] Add or adjust a regression test that proves coverage cannot exceed 100% when deploy-unit is present.
- [x] Keep deploy-unit distance evidence visible in the diagnostic output.
- [x] Update docs if they currently imply auxiliary evidence contributes to coverage.
- [x] Re-run the targeted coverage tests and the full JSON analyze check.

Validation:

```sh
go test ./internal/metrics/boundary ./cmd/archfit -run 'Coverage|DeployUnit'
.bin/archfit analyze --full --json --no-cache -c .archfit.yaml | jq '.metrics[] | select(.name=="coverage")'
```

---

### Task 2: Keep SCIP semantic-strength overlay visible on successful zero-hit runs

Why this needs a fix:

- A successful SCIP run with zero strength entries currently looks the same as no
  overlay at all.
- That hides under-application and makes TS/Python/Rust strength refinement look
  absent when SCIP actually ran.
- The current gate is `len(scipStrength) > 0`, which is the wrong signal.

Advisor-selected solution:

- Pass a boolean or enum derived from SCIP coverage status into overlay tracking.
- Treat `scip` `status: ok` / `partial` as "SCIP ran", even when the strength map is
  empty.
- In that case, allow the overlay to report `candidate_edges` with `applied=0` and
  `missed=candidate_edges`.
- Keep Go excluded from overlay tracking.

Rejected alternatives:

- Docs-only fix. That leaves the implementation blind.
- A config-derived enable flag. That duplicates the coverage record.
- Always emit a blank overlay block. That hides whether SCIP ran or not.

Likely files:

- `internal/engine/engine.go`
- `internal/engine/semantic_overlay.go`
- `internal/engine/enrich_internal_test.go`
- `internal/engine/engine_test.go`
- `internal/output/jsonout/jsonout_test.go`
- `internal/output/markdown/markdown_metrics.go`
- `internal/output/markdown/markdown_test.go`
- `docs/guide/metrics.md`

Metrics / evidence to consume:

- `semantic_strength_overlay.by_language.*.candidate_edges`
- `semantic_strength_overlay.by_language.*.applied`
- `semantic_strength_overlay.by_language.*.missed`
- `tool_coverage[scip].status`
- `tool_coverage[scip].reason`

Task checklist:

- [x] Thread SCIP run status into overlay tracking.
- [x] Make zero-hit successful SCIP runs produce visible overlay counters.
- [x] Add regression coverage for TS, Python, and Rust zero-hit cases.
- [x] Keep empty/absent SCIP runs distinct in the rendered output.
- [x] Update the metrics docs to match the new visibility rule.

Validation:

```sh
go test ./internal/engine ./internal/extract/scip ./internal/output/jsonout ./internal/output/markdown
```

---

### Task 3: Add advisory-task rollups for grouped BC advisories

Why this needs a fix:

- The current advisory flood is too noisy for humans to work through one edge at a time.
- `agent_tasks[]` is gate-only by contract; advisory findings do not belong there.
- Grouped Balanced-Coupling advisories already carry useful rollup metadata.

Advisor-selected solution:

- Add a separate deterministic `advisory_tasks[]` diagnostic block.
- Build it from grouped BC advisories (especially `bc/imbalanced_coupling`, and
  optionally `bc/duplicated_knowledge` if the grouping shape supports it).
- Carry `finding_id`/group IDs, rule ID, severity, `group_count`, representative
  `group_members`, `cheapest_move`, top files, constraints, and validation commands.
- Keep `agent_tasks[]` untouched and gate-only.

Rejected alternatives:

- Promote advisories to gates. That changes semantics and would block on review noise.
- Generate one task per edge. That recreates the flood.
- Use the LLM to invent repair tasks. That breaks determinism and auditability.

Likely files:

- `internal/model/diagnostic/diagnostic.go`
- `internal/engine/advisories.go`
- `cmd/archfit/analyze.go` or the diagnostic assembly path
- `internal/output/jsonout/jsonout.go` and tests
- `internal/output/markdown/markdown_metrics.go` and tests
- `docs/guide/agent-feedback.md`
- `docs/guide/commands.md`

Metrics / evidence to consume:

- `findings[].matched_by.group_count`
- `findings[].matched_by.group_members`
- `findings[].matched_by.cheapest_move`
- `findings[].severity`
- `findings[].locations`
- `findings[].matched_by.score_value`

Task checklist:

- [ ] Add a new report-only advisory-task model/type.
- [ ] Populate it from grouped BC advisories.
- [ ] Render it in JSON and Markdown.
- [ ] Keep `agent_tasks[]` gate-only.
- [ ] Update the agent-feedback docs so the channel split is explicit.
- [ ] Verify advisories still do not change verdict or gate status.

Validation:

```sh
go test ./internal/engine ./internal/output/jsonout ./internal/output/markdown ./cmd/archfit ./internal/agenttask
```

---

### Task 4: Strangle the internal/model hub in two passes

Why this needs a fix:

- `internal/model` is still the dominant structural hub in the current self-run.
- It attracts the largest advisory and blast-radius concentration.
- Tuning thresholds would hide the problem instead of reducing actual coupling.

Advisor-selected solution:

- Pass 1: split measurement in `.archfit.yaml` so the broad `internal/model` module is
  replaced by reviewed submodules (`graph`, `finding`, `diagnostic`, `coupling`,
  `fileclass`, `clone`, `pattern`, `signal`, `symbol`).
- Pass 2: use the new measurement to choose the single hottest seam, then refactor one
  package behind narrower stable contracts. Default candidates are `diagnostic` or
  `coupling`, but let the measured hotspots decide.
- Use `gitnexus_impact` / `gitnexus_context` before editing to avoid guessing the blast
  radius.

Rejected alternatives:

- Lower volatility labels.
- Raise `coupling.min_severity`.
- Add broad waivers.
- Do a big-bang rewrite.

Likely files:

- `.archfit.yaml`
- `internal/model/diagnostic/**`
- `internal/model/coupling/**`
- `internal/model/graph/**`
- `internal/model/finding/**`
- `internal/model/signal/**`
- `internal/output/**` if DTO extraction is needed

Metrics / evidence to consume:

- `bc/imbalanced_coupling` advisory count and severity on `internal/model/**`
- `blast_radius`
- `local_coupling.complexity_share_pct`
- `file_facts.inbound_module_fanin`
- `classified_edges.tail_risk`
- `coupling_balance`

Task checklist:

- [ ] Split the broad `internal/model` measurement surface in config.
- [ ] Use graph evidence to rank the worst internal/model seam.
- [ ] Refactor one package at a time behind a narrower contract.
- [ ] Add or adjust import-boundary tests for the chosen seam.
- [ ] Re-run the architecture review and confirm the hub is smaller, not just renamed.

Validation:

```sh
gitnexus_impact internal/model/diagnostic --direction upstream || true
gitnexus_impact internal/model/coupling --direction upstream || true
go test ./internal/model/... ./internal/engine ./internal/score ./cmd/archfit
.bin/archfit analyze --full --json -c .archfit.yaml > /tmp/archfit-next-five-model.json
```

---

### Task 5: Add report-only distance-config candidates from runtime/dynamic evidence

Why this needs a fix:

- Runtime and dynamic-import evidence exists, but today it only helps human review in
  separate report-only sections.
- The repo still lacks a deterministic bridge from those signals to candidate config
  changes (`external_systems` / `deploy_unit`) without scoring them.
- This is the right place to improve distance evidence, not to auto-score runtime.

Advisor-selected solution:

- Add a report-only `distance_config_candidates[]` block.
- Source it from `runtime_async_edges`, `dynamic_imports`, and `dynamic_connascence_signals`.
- Each candidate should carry: source block, module, target, integration kind, count,
  evidence sites, and a suggested review action (`external_systems` or `deploy_unit`).
- Surface the same candidates in `config update` as review-only hints.
- Keep `runtime_async`, `dynamic_imports`, and `dynamic_connascence_signals` report-only.

Rejected alternatives:

- Auto-write `external_systems` into live config.
- Feed the candidates into `classify` or scoring.
- Treat dynamic connascence as measured runtime truth.

Likely files:

- `internal/model/diagnostic/diagnostic.go`
- `internal/engine/assemble.go`
- `cmd/archfit/update.go`
- `internal/initcfg/update.go`
- `internal/output/jsonout/jsonout.go` and tests
- `internal/output/markdown/markdown_metrics.go` and tests
- `docs/guide/metrics.md`
- `docs/guide/commands.md`

Metrics / evidence to consume:

- `runtime_async_edges[].count`
- `runtime_async_edges[].confidence`
- `dynamic_imports[].count`
- `dynamic_connascence_signals.signals`
- `distance_context.distance_basis`

Task checklist:

- [ ] Add the new report-only candidate block to the diagnostic model.
- [ ] Populate candidates from runtime/dynamic evidence.
- [ ] Render the candidates in JSON and Markdown.
- [ ] Add review-only hints to `config update` output.
- [ ] Keep scoring and gate verdicts unchanged.

Validation:

```sh
go test ./internal/engine ./cmd/archfit ./internal/initcfg ./internal/output/jsonout ./internal/output/markdown
```

## Done criteria

- Coverage never exceeds 100%.
- Empty-but-successful SCIP runs still show overlay visibility.
- Advisory repair guidance has a separate deterministic channel.
- `internal/model` is measurably smaller after the split-and-refactor pass.
- Runtime/dynamic evidence yields config review candidates but does not affect score
  or gate verdicts.
- `archfit analyze --gate --full` still passes.
