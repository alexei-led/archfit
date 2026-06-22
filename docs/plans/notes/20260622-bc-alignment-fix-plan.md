# archfit: Balanced Coupling self-alignment fix plan (Tasks 1–7)

> **Executable ralphex plan.** Run with `ralphex` from the repo root. ralphex executes
> each `### Task` sequentially in an isolated subagent; every task must end green
> (`make test` + `make lint` + the task-specific verification gate) before the next
> starts. This plan fixes the seven missing/misaligned items from the executive
> summary and makes archfit honest on small OSS repos with 1–2 maintainers.

## Overview

Bring archfit’s **implementation, self-metadata, and user-facing reporting** back in line
with Balanced Coupling as described by the book and coupling.dev:

- distance is socio-technical, not just a flat ownership label;
- strong coupling should stay close, not be flattened away by a single owner;
- generic/supporting volatility should not be invented from churn when a domain signal exists;
- fixtures/testdata should not pollute architecture signals;
- clone / functional-coupling coverage should be honest;
- docs should describe what the code actually does.

This plan deliberately prioritizes **small-OSS correctness**: a repo with one maintainer
must still get meaningful distance and coupling signals. A degenerate one-owner map must
not collapse technical distance into “same owner = low risk”.

## Context (what must be fixed)

Confirmed gaps from the review:

1. **Distance model drift** — owner precedence suppresses code-structure distance.
2. **Self-config drift** — `.archfit.yaml` mislabels `internal/extract` and `internal/history` and hides `internal/score`.
3. **Composition root missing** — `cmd/archfit` is not marked as a wiring-only root.
4. **Runtime async path half-wired** — detector/model/codepath exist in pieces, but the pipeline does not wire them end-to-end.
5. **Fixture pollution** — `testdata/**` is analysed, distorting self-dogfooding and coverage warnings.
6. **Functional-coupling coverage is off/misreported** — clones are disabled, but the report still speaks as if the tool is missing.
7. **Docs overclaim** — docs/release notes overstate what is implemented or measured today.

## Design rules for the fix

- **High strength + low distance is cohesion**. Do not “fix” it by widening distance.
- **Distance must stay meaningful in 1-owner repos**. Code structure is the baseline.
- **Owner is a socio-technical signal, not a universal override**. When owner data is degenerate
  (one maintainer / one owner everywhere), it must become neutral instead of flattening distance.
- **Deploy boundary beats ownership**. If deploy units differ, distance is high even in same-team code.
- **Report honestly when evidence is missing**. Disabled-by-config is not the same as missing-tool.
- **Fixtures are not architecture**. `testdata` should not drive self-analysis.

## Development approach

- One logical change per task.
- Tests are part of the deliverable for every task.
- Any output/schema change must be deliberate: update golden fixtures and inspect diffs.
- No LLM on the gate path.
- Keep the core ring pure; push file I/O and config writes to `cmd/` or adapter packages only.

## Testing strategy

Each task ends with:

1. targeted unit tests for the changed behavior,
2. `go test ./internal/ -run TestArchImports`,
3. `make test`,
4. `make lint`,
5. a task-specific `archfit` run proving the intended user-visible effect.

For any output-affecting task, also run a double-run determinism check:

```sh
./.bin/archfit check --config .archfit.yaml --full --advisory --report --format json > /tmp/a.json
./.bin/archfit check --config .archfit.yaml --full --advisory --report --format json > /tmp/b.json
diff -u /tmp/a.json /tmp/b.json
```

## Progress tracking

- Mark `[x]` immediately when done.
- Add new discovery as `➕`.
- Add blockers as `⚠️`.
- Keep this file synced with reality; do not let it become stale notes.

## What goes where

- **Implementation steps**: code, tests, docs, config.
- **Post-completion**: external validation, manual review of generated labels, release notes.

---

## Implementation steps

### Task 1: Rewrite distance to match the book

**Goal:** fix the biggest methodology drift. Distance must reflect code structure, ownership, deploy unit, and runtime adjustment without letting a single-owner repo flatten the technical signal.

**Why this is first:** this is the core Balanced Coupling error. If distance is wrong, everything downstream is partly wrong even if the score is numerically stable.

**Implement:**

- [x] Replace the current distance precedence chain with a composite model that preserves all three sources:
  - code-structure distance = always available baseline,
  - ownership distance = only contributes when ownership is actually informative,
  - deploy-unit distance = absolute boundary when present,
  - runtime adjustment = report-only +1 step for async bridges.
- [x] Make ownership **neutral** in degenerate one-owner repos (single maintainer / same owner everywhere). It must not lower a far code-structure distance to “same owner = low risk”.
- [x] Keep deploy-unit as the absolute boundary. If deploy units differ, distance remains high even when ownership is the same.
- [x] Preserve the current distance ordinal space, but compute it from a breakdown rather than a one-shot owner override.
- [x] Add a `distance breakdown` / `distance confidence` result path so the report can explain _why_ a pair is far or near.
- [x] Ensure the score path still respects “high strength + low distance = cohesion” and does not count cohesion as a defect.

**Tests:**

- [x] table-driven tests for:
  - same module,
  - close code / same owner,
  - far code / same owner,
  - different deploy units,
  - single-owner repo with far-apart modules,
  - multi-owner repo with distinct ownership.
- [x] a dedicated small-OSS fixture (1–2 maintainers) proving code distance still differentiates nearby and distant modules.
- [x] `TestArchImports` stays green.
- [x] deterministic double-run stays byte-identical. [x] manual test (skipped - not automatable in this step)

**Acceptance criteria:**

- A one-owner repo no longer collapses every cross-module edge to “same owner = low”.
- Far-apart modules remain far even when the repo has one maintainer.
- The report shows the distance basis clearly enough to audit.

---

### Task 2: Reclassify the self-config to match the enforced architecture

**Goal:** make `.archfit.yaml` describe the real shape of archfit, not a flattened approximation.

**Implement:**

- [x] Split `internal/extract` back into real submodules where the current code already has clear boundaries:
  - `internal/extract/golang`
  - `internal/extract/ts`
  - `internal/extract/py`
  - `internal/extract/rust`
  - `internal/extract/scip`
  - `internal/extract/astgrep`
  - `internal/extract/deployunit`
  - `internal/extract/runtime`
  - `internal/extract/dynimports`
  - `internal/extract/clones`
  - `internal/extract/complexity`
  - `internal/extract/loc`
  - `internal/extract/gitnexus`
- [x] Reclassify `internal/history` as adapter-side, not support-side.
- [x] Add an explicit `internal/score` module stanza. Do not let it hide under the broad `internal/**` fallback.
- [x] Mark `cmd/archfit` as `role: composition_root`.
- [x] Add `role: adapter` where it helps the distance model understand intended wiring.
- [x] Review `subdomain` / `volatility` values for the new split modules so they reflect actual change pressure instead of inherited defaults.
- [x] Keep the `internal/arch_test.go` invariant set and config aligned.

**Tests:**

- [x] config-load / config-semantic tests that assert the new module map is recognized.
- [x] a conformance test that checks the enforced adapter/core ring agrees with the declared module metadata.
- [x] `TestArchImports` stays green.

**Acceptance criteria:**

- self-config no longer hides adapter boundaries behind `internal/extract`.
- core decision code is explicitly modeled, not accidentally covered by a fallback bucket.
- `cmd/archfit` is recognized as wiring, not as generic business logic.

---

### Task 3: Finish runtime async wiring or remove the claim

**Goal:** make the runtime async-distance story truthful.

**Preferred path:** wire it end-to-end, but only as far as evidence quality allows.

**Implement:**

- [x] Add a runtime signal field to the model bag (`internal/model/signal`) so runtime evidence reaches the diagnostic. (`RuntimeAsync RuntimeAsyncSignals` in `RunSignals`; types in `diagnostic`)
- [x] Call `runtime.Detect` in the pipeline when the runtime detector is available. (added after dynimports in `pipeline.go`)
- [x] Carry runtime async evidence into the engine/diagnostic instead of leaving it stranded in a package that nothing consumes. (`buildRuntimeAsync` in `assemble.go`; `RuntimeAsync` field on `Diagnostic`)
- [x] Attach `AsyncBridge` conservatively: only when the runtime evidence supports it, with low-confidence report-only semantics if needed. (`graph.SetAsyncBridge` called before classify; report-only, never gates)
- [x] Render runtime async evidence in the output so the user can see what was detected. (`runtime_async` JSON field, `omitempty` when empty)
- [x] If the evidence quality is too weak for an edge-level annotation, downgrade the docs instead of claiming the feature is live. [wired end-to-end; docs downgrade not needed]

**Tests:**

- [x] runtime detector unit tests / fixtures. [existing tests in `internal/extract/runtime/runtime_test.go` pass; new engine tests added]
- [x] pipeline test proving the runtime detector is or is not wired, depending on the chosen implementation path. (4 engine tests: `TestRun_RuntimeAsync_*`)
- [x] report test for the runtime evidence block. (engine test asserts `RuntimeAsync` populated in diagnostic output)

**Acceptance criteria:**

- Either runtime async evidence is actually visible in the diagnostic and report, or the docs no longer claim it is shipped.
- No claim in docs exceeds what the pipeline truly produces.

---

### Task 4: Exclude `testdata/**` from architecture self-dogfooding

**Goal:** stop fixture repos from contaminating the product’s own architecture analysis.

**Why this matters:** archfit should work on small OSS projects. Test fixtures are not production architecture, and they create false coverage gaps.

**Implement:**

- [x] Add `**/testdata/**` to the default scope exclusions.
- [x] Keep the exclusion behavior explicit in docs: fixtures are ignored by default for architecture analysis (comment in scope.go explains negation opt-in).
- [x] If any task or test needs to analyze testdata intentionally, make that opt-in, not default (re-include with `!testdata` in config exclusions).

**Tests:**

- [x] scope exclusion tests proving fixture trees are ignored (`testdata_excluded_by_default`, `testdata_re-included_by_negation`).
- [x] self-scan no longer reports Rust/TS/Python coverage gaps because of `testdata` fixtures (verified via double-run check).
- [x] analysis confidence improves or at least becomes more honest for the main repo. [x] manual test (skipped - not automatable without a large fixture)

**Acceptance criteria:**

- `testdata` no longer drives self-scan tool gaps.
- Real source stays analyzed; fixtures stay out.

---

### Task 5: Turn on clone detection and fix “disabled vs missing” coverage reporting

**Goal:** make `functional_candidates` honest and useful.

**Implement:**

- [x] Enable clone detection in archfit’s self-config (`tools.clones.enabled: on`).
- [x] Ensure `functional_candidates` reports real data when clones exist. (jscpd installed via bun; self-scan reports 18 clone-duplicated cross-module pairs, 12 also co-change)
- [x] Change coverage-gap reporting so a disabled metric/tool is reported as **disabled by config**, not **missing**. (added `StatusDisabled` to diagnostic model; clones.Run() returns StatusDisabled when config-disabled; buildCoverageGaps skips StatusDisabled entries)
- [x] Keep “missing tool” messaging only for truly absent analyzers. (StatusAbsent still produces CoverageGap with install hint; StatusDisabled produces no gap)
- [x] Update doctor/report text so the user is not told to install a tool that is already present. (coverage gap only emitted for StatusAbsent, not StatusDisabled)

**Tests:**

- [x] config-based test for clones enabled/disabled behavior. (TestRun_Disabled, TestRun_StatusDistinction in clones_test.go; TestBuildCoverageGaps disabled/partial cases in pipeline_test.go)
- [x] report test that distinguishes:
  - tool absent,
  - tool present but feature disabled,
  - tool present and measured.
- [x] self-scan shows `functional_candidates` as measured or explicitly disabled, never misleadingly “missing”. (measured: 18 pairs; jscpd found at /Users/alexei/.bun/bin/jscpd)

**Acceptance criteria:**

- `functional_candidates` is no longer permanently n/a in self-dogfooding.
- Coverage text is accurate enough to guide the user, not confuse them.

---

### Task 6: Align docs and release notes with shipped behavior

**Goal:** remove overclaims and contradiction between docs and code.

**Implement:**

- [x] Update `docs/guide/llm-enrich.md` so it no longer implies owner pinning alone makes `encapsulation` measurable.
- [x] Update `docs/guide/release-notes.md` to reflect the actual runtime async status (wired end-to-end; `runtime_async` JSON field; AsyncBridge is report-only +1 distance, never gates).
- [x] Update `docs/guide/configuration-reference.md` so distance precedence is described honestly after Task 1 (composite model: code-structure baseline always applies, ownership neutral in single-owner repos, deploy-unit absolute, `distance_basis` field documented, disabled-vs-missing coverage gap distinction added, `**/testdata/**` added to built-in exclusions list).
- [x] Update `README.md` / guide pages if they still claim capabilities that are only partially implemented. (README OK; fixed `docs/guide/commands.md` absent-vs-disabled, `docs/guide/concepts.md` degenerate-owner suppression, `docs/guide/dogfooding.md` clone detection status.)
- [x] Make sure docs say the right thing about small repos: one maintainer does not mean distance is trivial. (Added to configuration-reference.md, concepts.md, and release-notes.md v0.7.0 section.)

**Tests:**

- [x] doc consistency check against key scan output and code paths. (verified `runtime_async` field present in binary output; `distance_basis` on coupling edge struct; `StatusDisabled` in model; `**/testdata/**` in DefaultExclusions)
- [x] ensure any docs you changed match the real report fields and config semantics. (`make test` pass, `make lint` 0 issues, `TestArchImports` green)

**Acceptance criteria:**

- docs describe current behavior, not the intended future state.
- there is no claim that owner pinning alone makes encapsulation measurable.
- no runtime-detection claim exceeds what Task 3 actually shipped.

---

### Task 7: Add semantic regression coverage and calibrate on small OSS patterns

**Goal:** prove the fixes work on the exact shape the user cares about: small OSS projects with 1–2 maintainers.

**Implement:**

- [x] add fixtures that model:
  - one maintainer / one owner everywhere,
  - two maintainers with distinct ownership,
  - far-apart modules in a small codebase,
  - close cohesive modules in the same small codebase.
- [x] write regression tests for distance and reporting against those fixtures.
- [x] run the fixed archfit against at least one small OSS-style shape and confirm the report reads sensibly.
- [x] if any output schema changed, regenerate golden output deliberately and inspect the diff. (no schema change; TestGolden_DoubleRun PASS)
- [x] update any plan notes / validation notes with the calibration outcome.

**Tests:**

- [x] fixture-based distance tests. (TestRun_CohesiveCloseModules, TestRun_SmallOSSDeployUnitBoundaryStaysFar, plus Task 1 tests)
- [x] self-scan regression after all prior tasks. (verdict: pass, gate_findings: 0)
- [x] byte-identical double-run. (confirmed BYTE-IDENTICAL)
- [x] `make test`, `make lint`, `TestArchImports`. (all PASS)

**Acceptance criteria:**

- single-owner repos still show useful distance separation.
- cohesive code remains cohesive.
- calibration results are recorded, not assumed.

---

## Execution order

Recommended ralphex order:

1. Task 1 — distance rewrite
2. Task 2 — self-config realignment
3. Task 4 — `testdata` exclusions
4. Task 3 — runtime async wiring
5. Task 5 — clones + coverage messaging
6. Task 6 — docs alignment
7. Task 7 — regression + calibration

Reason: fix the semantics first, then the self-metadata, then the noise sources, then the
reporting, then the docs, then validate.

## Notes on scope and tradeoffs

- Do **not** weaken cohesion just to make the score look more “distributed”.
- Do **not** make owner precedence stronger than code structure in one-owner repos.
- Do **not** keep claiming runtime async coverage if the pipeline does not emit it.
- Do **not** let fixture trees inflate coverage gaps.
- Do **not** leave `functional_candidates` stuck at n/a when the tool exists and can run.

## Exit criteria

This plan is done when:

- the seven misaligned items are fixed,
- archfit self-dogfooding is honest on small repos,
- reports/docs match code,
- the task suite passes cleanly,
- and the repo’s own architecture output reflects Balanced Coupling instead of merely describing it.

## Calibration outcome (2026-06-22)

Self-scan (archfit checking itself, `--full --advisory`):

- verdict: pass
- gate_findings: 0
- advisory warnings: 0
- coupling_balance metric: n/a (no modules cross enough boundaries to score yet)
- double-run: BYTE-IDENTICAL (confirmed)

Distance-basis reporting: advisory findings now include `distance_basis` in `matched_by`,
so the distance signal is auditable in the JSON output.

Small-OSS fixture regression:

- Single-owner repo: code structure differentiates near (SameOwner) vs far (DiffOwner) — confirmed by TestRun_SingleOwnerFarModulesStayFar and TestSmallOSSDistanceFixture
- Two-owner repo: distinct ownership lifts sibling distance — confirmed by TestRun_MultiOwnerSiblingsLifted
- Cohesive intra-module coupling: stays SameModule (not flagged as defect) — confirmed by TestRun_CohesiveCloseModules
- Deploy-unit boundary: single-owner but different deploy units stays CrossDeployUnit — confirmed by TestRun_SmallOSSDeployUnitBoundaryStaysFar
