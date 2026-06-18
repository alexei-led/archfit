# Balanced Coupling measurement engine v2.0

## Overview

Make archfit's measurement engine a faithful, deterministic implementation of the
Balanced Coupling model (integration strength, distance, volatility) across Go,
TypeScript, and Python, so an AI coding agent gets architecture-level feedback the way
a linter gives code-level feedback. This closes the seven gaps found in the 2026-06-18
self-analysis, adds the standard coupling metrics the model leans on, and aligns the
report with Balanced Coupling vocabulary — without ever putting an LLM or
non-determinism on the gate path.

Adopted from `docs/plans/20260618-bc-measurement-v2.md` (architecture-plan format) and
the approved design `docs/design/bc-measurement-v2.md`.

## Context

- Source design: `docs/design/bc-measurement-v2.md` (approved 2026-06-18). Gaps and
  evidence cited there by `file:line`.
- Impacted packages (highest blast radius first): `internal/model/coupling`,
  `internal/classify`, `internal/config`, `internal/metrics/*`, `internal/engine`,
  `internal/ownership`, `internal/extract/*`, `cmd/archfit`.
- Hard invariants (must not regress): deterministic gate (no LLM/clock/network/random in
  `check`); LLM off-gate (only `enrich`/`explain`); core-ring purity enforced by
  `internal/arch_test.go`; golden output guarded by `TestGolden`; three languages each
  define a source per dimension or degrade to honest `unknown`.
- Resolved decisions (Perplexity + advisor, 2026-06-18): calibrate on archfit (Go) +
  `redwoodjs/redwood` (TS) + `saleor/saleor` (Py); ship the new standard metrics ON by
  default (report-only, never gate); one-shot `*.v2` metric bump + baseline reset shipped
  as **v0.3.0**, framed as a breaking change in scoring (not in verdict).

## Development Approach

- Testing approach: regular (code, then tests within the same task). Non-trivial logic
  leaves at least one runnable check that fails if the logic breaks.
- Complete each task fully before the next; every code-changing task ends by writing
  tests and running the project test suite green.
- **All project tests must pass before starting the next task** — no exceptions.
- Keep changes small and independently committable. Run on a feature branch, not `main`.
- **Run this plan PHASE BY PHASE.** The plan is grouped into five phases (bold markers
  below). After each phase's last task, stop for the human re-review gate listed in
  Post-Completion before starting the next phase. ralphex executes tasks autonomously and
  does not hard-pause mid-run, so the operator enforces the gate by running ralphex over
  one phase at a time.
- Determinism check after any gate-touching task: run `archfit check` twice on archfit
  and confirm byte-identical output.
- Update this plan file if scope changes during implementation.

## Testing Strategy

- Unit tests required for every code-changing task (table-driven where there is an
  input/output matrix — e.g. the scorer's strength×distance×volatility cube).
- After each task: `make test` (race) green; for gate-touching tasks also
  `go test ./internal/ -run TestArchImports` and `go test ./internal/engine/ -run
TestGolden` (regenerate golden deliberately and inspect the diff — output changes are
  never automatic).
- Whole-plan gates (run before declaring a phase done): `make all` (fmt → lint → test →
  build); `go test -race ./...`; `archfit doctor`.
- Calibration harness `make calibrate` is informational (not a CI gate).

## Progress Tracking

- Mark completed checkboxes with `[x]` immediately when done.
- Add newly discovered tasks with a `➕` prefix; record blockers with `⚠️`.
- Keep this plan in sync with actual work; update scope notes if the design changes.

## Technical Details

Per-edge continuous score (replaces the binary `BalanceResult` in
`internal/model/coupling/coupling.go`). A `Scorer` port ships TWO implementations;
calibration (Task 16) locks one and freezes its constant tables.

- Ordinal tables (frozen consts, cited to Balanced Coupling):
  - strength: contract 0, model 2, functional 5, intrusive 8, unknown 3
  - distance: same_module 0, cross_module_same_owner 1, cross_module_diff_owner 3,
    cross_deploy_unit 5, unknown 2; plus +1 when the edge is an async bridge
  - volatility: low / medium / high / unknown
- Additive impl: `raw = strength + distance − vol_discount{low2,med1,high0,unk0}`,
  clamp [0,10].
- Multiplicative (Balanced-Coupling-pure) impl: map levels to [0,1];
  `R_mod = 1 − abs(S − D)` (continuous XOR); `R_edge = R_mod × V`;
  `score = round(10 × R_edge)`; structural penalties (+cycle, +unstable-dependency,
  +change-coupling ≥ 65%); intrusive floor = at least band `low` regardless of volatility.
- Bands: 0–2 none · 3–4 low · 5–6 medium (default gate) · 7–8 high · 9–10 critical.
- Every edge carries: integer score, band, a machine-parseable reason string
  (`intrusive(+8) cross_deploy(+5) vol_high(-0)=13→10`), and a deterministic
  `cheapest_move` (the single dimension change that drops the band most).

Distance becomes a composite (design §4.2):
`distance = max(code_structure, ownership, deploy) + runtime_adjust`. `code_structure`
(new, always-available tree distance) is the deterministic baseline; ownership is
suppressed when degenerate (one owner for the whole repo); deploy units are auto-detected.

Volatility is two-axis (design §4.3): domain volatility (subdomain/config/heuristic →
the gate, zero churn) vs implementation volatility (git churn → report-only metrics only).

## Implementation Steps

**Phase 1 — Correctness, no new data.**

### Task 1: Fix XOR severity grading

Gap 5 (design §3#5, §5): `coupling.go:137-138` returns `SeverityLow` for the two
Balanced-Coupling-modular quadrants, which should be `SeverityNone`.

- [x] In `internal/model/coupling/coupling.go` `BalanceResult`, return `SeverityNone` for
      the asymmetric (high-strength+low-distance cohesive; low-strength+high-distance
      loose) cases; keep the over-decoupled-volatile-seam (low+low+high-volatility) at
      `SeverityMedium`; leave the intrusive-escalation paths unchanged.
- [x] Grep `internal/rules` for consumers that relied on the old `SeverityLow` for XOR
      edges and adjust if any exist. (rules uses `finding.SeverityLow`, not `coupling.SeverityLow` — no change needed)
- [x] Write a four-quadrant table test in `coupling_test.go` (none/none/medium-if-volatile/
      critical-if-volatile). Updated with full XOR quadrant coverage + intrusive cases.
- [x] Regenerate `TestGolden` if the severity change shifts output; inspect the diff. (no change — archfit's own edges don't hit XOR case)
- [x] Run project tests (incl. `TestArchImports`) — must pass before next task.

### Task 2: Remove git churn from gate volatility

Gap 3 (design §3#3, §4.3): `classify.go:224-253` consumes churn-derived volatility via
`effectiveModules()`; Balanced Coupling forbids commit-history volatility on the gate.

- [x] Split the config views in `internal/config/config.go`: a classify view with NO
      churn vs a metrics/churn view carrying implementation volatility.
      (`ForClassify()` now uses `c.Modules` directly; `effectiveModules()` removed as no
      longer needed — the separate `derivedVolatility` store is kept for future report-only
      metric consumers)
- [x] In `internal/classify/classify.go`, make `classifyVolatility` read explicit
      `volatility` → `subdomain` → `VolatilityUnknown` only (no churn).
      (`classifyVolatility` was already correct; the bug was in `ForClassify()` passing
      churn-merged modules — fixed by using `c.Modules` directly)
- [x] In `internal/config/volatility.go`, keep `DeriveVolatility`/`ApplyVolatility` but
      route churn only to report-only metric inputs (`risk_hub`, `change_amplification`);
      keep `c.Modules` hand-authored-only.
- [x] Write tests: gate volatility ignores churn; `risk_hub`/`change_amplification`
      outputs unchanged on archfit (churn path intact for report-only).
      (`TestForClassify_ChurnExcludedFromGate`, `TestApplyVolatility_ChurnPathIntact`,
      `TestRun_ChurnVolatilityIgnoredOnGate` added; existing
      `TestApplyVolatility_RespectsExplicitConfig` updated to assert new behavior)
- [x] Run project tests — must pass before next task.

### Task 3: Domain-volatility heuristic + generic-subdomain advisory

Gap 3/6 (design §4.3): replace the removed churn fallback with a deterministic domain
default, and add the generic-subdomain contract advisory (closes coverage gap).

- [x] Add `internal/classify/volatility_heuristic.go`: path-pattern table — generic
      (`vendor/`,`lib/`,`util/`,`pkg/common/`) → low; supporting
      (`infra/`,`platform/`,`storage/`,`db/`) → medium; else `unknown` (never guess
      core/high). Wire as source priority 3, after explicit config, before `unknown`.
- [x] Emit a "contract recommended" advisory when a generic-subdomain target is reached
      via non-contract strength (design §4.3 — the anti-corruption-layer guidance).
- [x] Write tests for the heuristic table (incl. the `unknown` default) and the
      generic-subdomain advisory trigger.
- [x] Run project tests — must pass before next task.

### Task 4: Scorer port + additive and multiplicative impls

Gap 4 (design §3#4, §5): collapse-to-2-bits is the core scoring defect. Add a swappable,
testable continuous scorer; default keeps current bands until calibration (Task 16).

- [x] Add `internal/model/coupling/scorer.go`: `Scorer` interface and `EdgeScore`
      (`Value int`, `Band Severity`, `Reason string`, `Breakdown`, `CheapestMove string`)
      with the frozen ordinal→number const tables (Technical Details) and BC-rationale
      comments.
- [x] Add `scorer_additive.go` and `scorer_multiplicative.go` (integer math;
      multiplicative carries the intrusive floor + structural-penalty hooks).
- [x] In `internal/classify/classify.go`, attach the score via the configured scorer;
      default to a shim reproducing the post-Task-1 bands so output stays stable.
- [x] Write table-driven tests over the strength×distance×volatility cube: XOR=none,
      intrusive floor, clamp [0,10], deterministic `cheapest_move`.
- [x] Run project tests (incl. `TestArchImports`, `TestGolden`) — must pass before next task.

**Phase 2 — Distance signal.**

### Task 5: code_structure_distance baseline

Gap 1 (design §4.2): the always-available distance signal archfit lacks.

- [x] Add `internal/classify/distance_structure.go`: deterministic tree-distance from two
      module paths (siblings near; fewer shared path segments → farther; normalized by
      tree depth).
- [x] Fold it into the `max(...)` distance composite in `classify.go`.
- [x] Write tests: siblings read closer than distant trees (e.g. `metrics/boundary` ↔
      `metrics/modularity` closer than `cmd/archfit` ↔ `extract/py`).
- [x] Run project tests — must pass before next task.

### Task 6: Degenerate-owner suppression

Gap 1 (design §4.2): a single git-author owner must not read as "same owner everywhere =
low distance".

- [x] When the owner resolver yields one owner for the whole repo, suppress ownership's
      distance contribution (let code-structure dominate); leave multi-owner CODEOWNERS
      behavior intact. Change in `internal/ownership/ownership.go` and/or its consumer in
      `cmd/archfit/pipeline.go`.
- [x] Write tests both ways: degenerate single-owner suppressed; a multi-team CODEOWNERS
      fixture still yields owner distance.
- [x] Run project tests — must pass before next task.

### Task 7: Deploy-unit detector

Gap 2 (design §4.2): `cross_deploy_unit` is unreachable today.

- [x] Add `internal/extract/deployunit/` (adapter via `toolrun.Runner`): Go `main` pkgs
      (`go list`), TS `package.json` workspaces, Python `pyproject.toml`, `Dockerfile`,
      k8s `Deployment`/`StatefulSet`. Record `deploy_unit_confidence: inferred|config`.
- [x] Add `FillMissingDeployUnits` in `internal/config/config.go` mirroring
      `FillMissingOwners`; wire it in `cmd/archfit/pipeline.go` (config always wins).
- [x] Extend `internal/arch_test.go` to forbid `os`/`exec` in any new core-ring code. (deployunit is an extractor adapter using toolrun.Runner — no direct os/exec, verified; no new core-ring code; TestArchImports green)
- [x] Write per-language detector tests with fixtures; confirm `archfit doctor` still green.
- [x] Run project tests — must pass before next task.

### Task 8: Calibration harness + golden refresh

Design §5: needed to choose the scorer (Task 16).

- [x] Add a `calibrate` target to the `Makefile` + `scripts/calibrate.*` that runs both
      scorers on archfit (Go) + pinned `redwoodjs/redwood` (TS) + pinned `saleor/saleor`
      (Py) and emits a per-edge band-agreement report.
- [x] Regenerate `TestGolden` for the distance-composite changes; inspect the diff.
      (TestGolden passed without regeneration — no distance/score shift in engine output)
- [x] Write a smoke test that `make calibrate` produces a report artifact.
      (internal/calibrate/calibrate_test.go: 6 unit tests; make calibrate smoke-runs on archfit: 147 edges, 70% agreement)
- [x] Run project tests — must pass before next task.

**Phase 3 — Standard metrics, report-only (ON by default, never gate).**

### Task 9: Instability / Abstractness / Distance-from-main-sequence

Design §6.

- [x] Add `internal/metrics/modularity/martin.go`: `I = Ce/(Ca+Ce)`,
      `A = abstract/(abstract+concrete)`, `Dms = abs(A+I-1)` from the existing dependency
      graph + SCIP exports; expose an unstable-dependency (`I(to) > I(from)`) penalty hook
      for the multiplicative scorer.
- [x] Label these "beyond Balanced Coupling" in output; never gate.
- [x] Write tests incl. the shared-DTO trap note (high-Ca low-I is not auto-"good").
- [x] Run project tests — must pass before next task.

### Task 10: Propagation cost

Design §6.

- [x] Add `internal/metrics/modularity/propagation.go`: `PC = reachable_pairs/(N^2-N)`
      via transitive closure; report system-level and per-module.
- [x] Write tests on a small known graph (hand-computed PC).
- [x] Run project tests — must pass before next task.

### Task 11: Change-coupling formula

Design §6: sharpen `hidden_coupling` with CodeScene's exact formula.

- [x] Add/extend `internal/metrics/modularity/change_coupling.go`:
      `CC(A,B) = C_AB / min(C_A, C_B)`, flag `≥ 65%`; source git/gitnexus.
- [x] Write tests for the formula and the threshold flag.
- [x] Run project tests — must pass before next task.

**Phase 4 — Speculative signals, report-only (never gate in v1).**

### Task 12: Runtime async detection

Design §4.2 (runtime_adjust).

- [x] Add `internal/extract/runtime/` (ast-grep patterns per language) + an off-gate
      `library → integration-kind` YAML table; set an `AsyncBridge` annotation on
      `graph.Edge`; scorer adds +1 distance level when set (report-only).
- [x] Record `confidence: low` when no async signal is found ("absence of evidence ≠
      synchronous"); never gate.
- [x] Extend `internal/arch_test.go` to forbid `os`/`exec` in any new core-ring code.
- [x] Write per-language detector tests with fixtures.
- [x] Run project tests — must pass before next task.

### Task 13: Connascence CoT/CoA tags

Design §4.1 (report-only, not scored).

- [ ] In `internal/classify`, tag edges `connascence: type` (SCIP struct/interface use)
      and `connascence: algorithm` (clone pair crossing a module boundary).
- [ ] Write tests for both tag derivations.
- [ ] Run project tests — must pass before next task.

### Task 14: `enrich --subdomains` (LLM draft → review → pin)

Design §7 (LLM off-gate only).

- [ ] Extend `cmd/archfit/enrich.go` + `internal/initcfg` to draft a subdomain per module
      (LLM) and pin it to config with `reviewed_at`/`reviewed_by`; wire existing staleness.
- [ ] Confirm `internal/arch_test.go` still proves `check` imports no LLM SDK.
- [ ] Write tests for the pin-writer and the gate-purity assertion.
- [ ] Run project tests — must pass before next task.

**Phase 5 — Report restructure + lock the scorer + version bump.**

### Task 15: Balanced-Coupling-aligned report

Gap 7 (design §8).

- [ ] Restructure the `internal/engine` markdown/JSON writers into lint-message format
      (`ARCHFIT[id sev] from→to`, strength/distance/volatility line, score breakdown, why,
      cheapest-move), add `config_hash`, a "beyond Balanced Coupling" metrics section, and
      a distance-confidence section.
- [ ] Regenerate `TestGolden`; inspect the diff.
- [ ] Write tests for the lint-message format and `config_hash` stability.
- [ ] Run project tests — must pass before next task.

### Task 16: Run calibration, lock the scorer

Design §5.

- [ ] Run `make calibrate`; choose the winning scorer by band-agreement vs hand-judged
      edges; set it as the default; freeze its const tables; remove the legacy shim.
- [ ] Record the decision + agreement numbers in `docs/design/bc-measurement-v2.md`.
- [ ] Write/adjust tests so the locked scorer is the asserted default.
- [ ] Run project tests — must pass before next task.

### Task 17: `*.v2` bump, baseline reset, docs, v0.3.0 release note

Design §11 risk 1 + versioning decision.

- [ ] Bump affected `metric_version` strings to `*.v2`; regenerate golden and reset
      baselines deliberately.
- [ ] Update `docs/guide`, `CLAUDE.md` (commands/metrics), and the self-analysis report
      `archfit-analysis.md`.
- [ ] Write a v0.3.0 release note: breaking change in SCORING (continuous scorer replaces
      `BalanceResult`; old `*.v1` baselines invalid — re-run `archfit baseline`); explicitly
      NOT a change in verdict/gate behavior.
- [ ] Run project tests — must pass before next task.

### Task 18: Verify acceptance criteria

- [ ] Verify every requirement in Overview/Context is implemented: distance varies on
      archfit (not constant); gate volatility has zero churn input; scoring uses all four
      strength + four distance levels; XOR edges produce no finding; tight+volatile =
      critical; deploy units detected for Go/TS/Py; standard metrics present and labeled
      beyond-BC; runtime/connascence report-only with confidence.
- [ ] Confirm `internal/arch_test.go` proves `check` imports no LLM/os/exec in the core ring.
- [ ] Determinism: run `archfit check` twice on archfit — byte-identical output.
- [ ] Run the full project test suite (`make all`, `go test -race ./...`, `TestArchImports`,
      `TestGolden`) — all green.
- [ ] Run the project linter — all issues fixed.

## Post-Completion

_Items requiring human judgment or external action — no checkboxes, informational only._

**Phase re-review gates (run the plan one phase at a time; stop here for human review):**

- After Task 4 (end of Phase 1): human architecture-review of `classify` + `model/coupling`
  before starting Phase 2. Confirm gate volatility shows no churn dependency and the XOR fix
  is verified on archfit (expect still 0 gate findings, advisories now correctly graded).
- After Task 8 (end of Phase 2): human review of the distance composite + deploy-unit
  detection; eyeball that archfit's edges now show varied distance.
- After Task 11 (end of Phase 3): human review of the new standard metrics for face validity.
- After Task 14 (end of Phase 4): human review of runtime/connascence signal quality and a
  dry-run of `enrich --subdomains` with a real human approving one module.
- After Task 17 (end of Phase 5): final human review of the Balanced-Coupling report on
  archfit; confirm it reads in BC vocabulary with correct gradings.

**Manual verification:**

- Spot-check the volatility heuristic pattern table against real module paths.
- Review the calibration agreement report before locking the scorer (Task 16).
- Confirm `deploy_unit_confidence` is recorded and config-authored values win.

**External / release:**

- Communicate the v0.3.0 breaking-scoring change in release notes; downstream consumers
  with pinned baselines must re-run `archfit baseline`.
- Consider adding the new detectors' external tools to the Docker runtime image + `doctor`
  if any new tool is introduced (current plan hand-rolls on the existing toolset).

**Re-review (full):** after Task 18, run a scoped architecture-review over `classify`,
`model/coupling`, `config`, `extract/deployunit`, `extract/runtime`, and `engine` to verify
the implementation matches `docs/design/bc-measurement-v2.md` and that determinism +
core-ring purity hold; re-run `archfit scan` on archfit.
