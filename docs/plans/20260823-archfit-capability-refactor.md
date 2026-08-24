# Capability-boundary refactor plan

Date: 2026-08-23
Status: IN PROGRESS
Design: [`docs/design/20260823-archfit-capability-map.md`](../design/20260823-archfit-capability-map.md)

This plan follows observed dependency and change seams. Each phase keeps the
current score inputs and gate semantics unchanged.

Completed in the current worktree: evidence, report, and scan contract moves;
compatibility aliases; capability module reassignment; score reporting; and
regression fixtures. The measured result is 54/100 with 14 critical edges,
compared with 40/100 and 123 critical edges before the refactor.

## Phase 0: freeze behavior and boundaries

- Keep the current `.archfit.yaml` module map.
- Keep byte-identical JSON fixtures for cold, warm, and refresh runs.
- Add import tests for the target dependency direction.
- Record the current report evidence, score, findings, and exit codes.

Gate: `make all`, `go vet ./...`, byte-identical tests, and no new blocking
architecture finding.

## Phase 1: finish the report contract — mostly complete

- Change the renderer port in `internal/ports` to accept the completed
  `internal/model/report` projection.
- Update console, JSON, Markdown, SARIF, and scorecard adapters to consume the
  report contract or an explicitly named detail contract.
- Keep `diagnostic.Diagnostic` inside the pipeline and diagnostic-specific
  persistence paths.
- Remove renderer imports of `internal/model/diagnostic`.

Tests:

- Renderer contract tests for every format.
- JSON and Markdown golden tests for raw score, cap, drivers, and top pairs.
- A compile-time architecture test forbidding output-to-diagnostic imports.

Gate: output parity except for the intentional contract fields; `make all`.

## Phase 2: separate pipeline state from orchestration — next

- Identify state structs and assembly helpers in `internal/engine`.
- Move only stage-result and assembly-state types into `pipeline-state`.
- Keep `engine.Run` as the ordered composition root.
- Make stage transitions explicit: extract, derive, evaluate, verdict, project.

Tests:

- Existing end-to-end pipeline tests.
- Stage tests that pass state through each transition.
- Error and cancellation tests at each external-tool boundary.

Gate: no policy or rendering import from the orchestration package; report and
exit-code parity.

## Phase 3: split evaluation-core by responsibility — next

- Establish `fact-derivation` around `classify` and `facts`.
- Establish `policy-evaluation` around `rules`, `metrics`, and `score`.
- Establish `verdict` around `status`, `decision`, and `agenttask`.
- Keep `staleness` with the baseline/change-origin seam until its callers are
  measured; do not move it by name alone.
- Extract only contracts needed to stop reverse imports. Do not create a
  generic `common` package.

Tests:

- Package-level import direction tests.
- Focused behavior tests for classification, metric calculation, score caps,
  verdict synthesis, and repair-task output.
- Full CLI byte-identical fixtures after each move.

Gate: no new cross-direction imports; unchanged score inputs, findings, and
exit codes.

## Phase 4: reduce composition-root coupling

- Keep `cmd/archfit` as a composition root.
- Move command-specific policy and lifecycle code behind config lifecycle,
  stores, adapters, or pipeline contracts.
- Do not move command registration or flag parsing into domain packages.
- Use the top module-pair evidence to select one seam at a time, starting with
  `archfit-cli -> fact-adapters` and `archfit-cli -> evaluation-model`.

Tests:

- Command contract tests for analyze, check, baseline, and config commands.
- Integration tests for missing tools, cache hits, and report-only failures.

Gate: CLI behavior parity, no direct extractor construction outside the
composition root, and no increase in report-only diagnostic leakage.

## Phase 5: recalibrate and publish

- Run Archfit without changing labels or baselines to compare the structural
  result.
- Review the new edge distribution and module-pair concentration.
- Update the architecture report and docs with the actual change, not the
  intended map.
- Run an independent architecture review.
- Release only after all gates, tests, report generation, and publication checks
  pass.

Success means fewer distant high-strength seams and clearer ownership. A higher
`coupling_balance` number alone is not success.
