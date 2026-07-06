# Monorepo acceptance record — 2026-06-27

Plan: `docs/plans/completed/20260626-archfit-monorepo-multimodule-resilience.md` (Tasks 1–16 done).
Binary: v0.11.0-dev (commit 76efecd).

## Acceptance criteria

### 1. go.work repo with no root go.mod builds a non-empty Go graph

Command: `archfit check --root ~/workspace/omni/server/shared --full --format json`

omni is a Go monorepo: `~/workspace/omni/go.work` lists 184 `go.mod` members,
no root `go.mod`. `server/shared` is a subtree with ~100 member directories.

Results:

| Criterion                     | Result                                                                   |
| ----------------------------- | ------------------------------------------------------------------------ |
| `go/packages` status          | **ok** (645 files loaded)                                                |
| Graph nodes (modules)         | **223**                                                                  |
| Total graph edges             | 2430 (208 scored first-party cross-module, 117 abstained, 1818 external) |
| Cross-module edges classified | ✓ (208 scored with distance; member auto-registration supplied distance) |
| `boundary_integrity`          | 50/100 (mixed) — measurable ✓                                            |
| `coupling_balance`            | 38/100 (poor) · mean balance 4.4/10 · confidence medium — measurable ✓   |
| `dependency_graph_health`     | 81/100 (strong) — measurable ✓                                           |
| `cohesion_modularity`         | 35/100 (poor) — measurable ✓                                             |
| `change_locality`             | 100/100 (full mode, strong — n/a expected in delta mode)                 |

Config-less run (no `.archfit.yaml`): `AugmentGoWorkspaceModules` registered
~100+ members as synthetic modules; undeclared volatility fell through to
conservative worst-case (v=10) and emitted advisory decision-tasks.

### 2. --root scopes file counts to the subtree

`loc: files=519` (subtree), `jscpd: files=797` (subtree). Coverage `140%`
(go/packages loads across member boundaries; denominator is loc files, numerator
is all extracted packages — correct and expected).

### 3. Single-module byte-identical

Verified by `TestByteIdentical_SingleModule` and `TestByteIdentical_OneMemberWorkspace`
(both hard assertions since Task 9). Pass green in `make test`.

### 4. Determinism

`TestGolden` (double-run determinism) green. `make archfit` dogfood green.

### 5. Non-git --full produces scorecard; delta mode without git errors

Verified by `TestNonGitFullMode` and `TestNonGitDeltaMode` in
`internal/scope/scope_test.go`.

## Workspace-root run

Command: `timeout 60 archfit check --root ~/workspace/omni --full`

Result: **timed out at ~60s** — 184 go.work members, full `NeedTypesInfo` load.
Expected. Mitigations shipped:

- `tools.go.modules.include/exclude` — scope to a member subset
- `tools.<x>.timeout` — per-analyzer watchdog
- `--root <subdir>` — reduce scope to a single service cluster

## Overall verdict

All acceptance criteria met. Tasks 1–16 are complete and green.
