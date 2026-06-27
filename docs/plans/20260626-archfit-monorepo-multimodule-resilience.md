# Monorepo & multi-module resilience — scanRoot/gitRoot decoupling + Go go.work + n/a-reduction

## Overview

Make archfit analyze **sub-projects of a monorepo** and **multi-module Go
workspaces** (`go.work`, no root `go.mod`), and stop reporting `n/a` on metrics
when the tools are present. Evidence base: the omni exercise
(`docs/notes/omni-monorepo-assessment-v0.10.0.md`) — 184 `go.mod` under
`go.work`, no root `go.mod`; every structural dimension came back "fewer than two
connected first-party modules" and `go/packages: absent`, while file metrics
scanned the **whole** repo regardless of `--root`.

Two confirmed root causes:

1. **Scan root is hard-anchored to git toplevel.** `scope.Resolve` sets
   `Scope.Root` from `Resolver.RepoRoot` = `git rev-parse --show-toplevel`
   (`internal/scope/scope.go:121`). `--root` only sets the git command's workdir,
   which still resolves _up_ to the toplevel. Every extractor then walks
   `s.Root` (`cmd/archfit/pipeline.go:140-216`).
2. **Go assumes one module per repo.** `internal/extract/golang/golang.go:82`
   runs `packages.Load({Dir: s.Root}, "./...")` and derives one `modPath` from
   the first package. At a `go.work` root this returns a single synthetic error
   package (`err==nil`, `pkg.Module==nil`, `len(pkg.Errors)>0`) → empty graph.

The keystone is **A (decouple scan root from git toplevel)**; **B (Go go.work)**
and the n/a/resilience work build on it. Tier 0 (Tasks 1–9) is the clean ship
line — it includes member→module auto-registration (Task 8) so the **config-less**
monorepo case measures `coupling_balance` _before_ the acceptance gate (Task 9).
Tiers 1–3 close the remaining findings.

Design basis: this plan's "Technical Details" + the two research briefs captured
during discovery (go/packages workspace loading; depcruise/grimp scoping).

## Context (from discovery)

- **Scope plumbing**
  - `internal/scope/scope.go` — `Scope` struct (`:86`, field `Root`), `Resolver`
    interface (`:104`), `Resolve` (`:120`). `Resolve` treats a `RepoRoot` error
    as a hard exit-3 (`:122`). `scope` is **core ring** (arch_test) — may gain a
    `GitRoot` field but must not import `os`/git; the `Resolver` interface is the
    seam.
  - `internal/history/git/git.go` — `RepoRoot` (`git rev-parse --show-toplevel`),
    `HeadRef` (`rev-parse HEAD`), `Changed` (`git diff --name-only base..head`),
    `History` (`git log -n1000 --name-only` — **no `-- <path>` filter**, whole
    repo relative to workDir). All paths are git-toplevel-relative.
  - `cmd/archfit/pipeline.go` — `scanDir := root` else `configDir` (`:80`);
    `scope.Resolve(... gitResolver{workDir: scanDir})` (`:93`). Then `s.Root`
    flows to `git.History` (`:140`), `loc.Run` (`:148`), `fitness.Detect`
    (`:153`), `dynimports`/`manifest`/`runtime`/`ownership`/`deployunit`/
    `complexity`/`clones` (`:158-216`), `buildCoverageGaps` (`:307`).
  - `cmd/archfit/check.go` — `CheckCmd.Root` (`type:"path"`) + `ScanCmd.Root`;
    passed as the `root` arg to `runPipeline`.
  - `internal/config` — `ScopeConfig{Base, Full, Exclusions, WorkDir}` (no
    `Root`); `ForScope()` in `views.go`. No `root:` YAML key; strict decoder
    (`DisallowUnknownField`) rejects unknowns.
- **Engine / delta** — `internal/engine` forwards the whole `scope.Scope` to
  `Extractor.Extract(ctx, s)` (interface in `internal/ports/ports.go:28-36`);
  delta bucketing in `internal/status/status.go` uses `pathRelated` (`:249`) —
  a **root-unaware raw prefix match** over `Changed` vs finding paths.
- **Go extractor** — `golang.go:64` `Extract`; per-package loop `:140`;
  `stripModPath` `:101`; `buildStrengthHints` (`:257`, needs `NeedTypesInfo`).
  LoadMode currently full (`:70-76`).
- **Coupling scorer (decides what "measurable" needs)** —
  `internal/model/coupling/scorer_book.go:78` `BookScorer.Score`: abstains
  (`Scored=false`) **only** on same-module distance or **unknown strength/
  distance**. Undeclared/unknown **volatility** falls through to worst-case
  `v=10` (`scorer.go:92-93`, `scorer_book.go:91-95`) and **still scores**. ⇒
  Auto-registered members (distance) + Go type-info (strength) yield a real
  `coupling_balance` on config-less omni; undeclared volatility only lowers the
  balance and emits an advisory decision-task (`pipeline.go:552`
  `buildJudgmentDecisionTasks`).
- **Module auto-registration precedent** — Rust registers auto-discovered
  `crate::mod` nodes as modules via `classify.AugmentModulesFromGraph`, **gated on
  the `::` separator** so Go/TS/Python are untouched (CLAUDE.md "Rust analysis
  granularity"). The Go analogue must gate on **≥2 workspace members loaded** so
  single-module repos and archfit's own self-config are byte-identical.
- **Per-language extractors** — TS `resolveTSConfig` probes only `s.Root`
  (`internal/extract/ts/ts.go`); Python `grimp_helper.py` fetches
  `get_import_details().line_contents` but **discards** it (emits only
  `line_number`); `py.go` sets `StrengthHint` **only** `intrusive` (underscore
  target) → BC abstains for most Python edges. Rust handles workspaces natively
  via `workspace_members`.
- **archfit's own `go.work`** = `use .`, `use ./testdata/fixture-go`,
  `use ./testdata/golang`. After the `**/testdata/**` default exclusion + the
  subtree filter, **one** member (`.`) survives → must collapse to today's
  single-module path. NOTE: archfit-self output **legitimately changes** under
  this plan (Tasks 2–6 add files/imports → new nodes/edges/metrics), so the
  one-member collapse is guarded by a **frozen one-member `go.work` fixture**
  (Task 1) + `make archfit`/`TestGolden` determinism — **not** by a byte-diff of
  archfit's own self-scan.
- **Tests / gates** — `internal/scope/scope_test.go` (Resolve full/delta/
  resolver-error, property-based); `internal/extract/golang/*_test.go`
  (property-based, no JSON snapshot); `internal/engine TestGolden` is a
  **double-run determinism** test (no snapshot file); `make archfit` dogfoods;
  `TestArchImports` enforces the ring. **No before/after byte-identical guard
  exists today** — Task 1 adds one.

## Development Approach

- **Testing approach:** Regular (code first, then tests) — matches existing
  adapter/extractor style. Every task ships new/updated tests (success +
  error/edge). All tests pass before the next task.
- **Smallest correct change.** Reuse seams: keep `Scope.Root` meaning "analysis
  boundary"; add `GitRoot` rather than renaming 12 callsites. Mirror the Rust
  `CrateRoots`/`AugmentModulesFromGraph` pattern for Go members.
- **Structural gates stay green every task:**
  `go test ./internal/ -run TestArchImports`;
  `go test ./internal/engine/ -run TestGolden` (deliberate, inspect diff);
  `make archfit` (dogfood).
- **Byte-identical is a hard gate, not a hope** — Task 1 captures the baselines,
  Task 9 diffs them. Update this plan when scope changes (`➕` new task, `⚠️`
  blocker).

## Testing Strategy

- **Unit:** scope resolution (scanRoot≠gitRoot, prefix, non-git per-mode);
  git rebasing (filter+rebase to subtree, no-op when equal); Go member discovery
  (go.work / single go.mod / multi go.mod walk / exclusion+subtree filter); Go
  per-package strip across 2+ modules; first-party-by-member; synthetic-error
  detection. Pure/table-driven where possible.
- **Fixture:** a `go.work` + 2-module workspace with a cross-module import,
  located in a **subdir that is not the git root**, under
  `internal/extract/golang/testdata/` (re-included from the default
  `**/testdata/**` exclusion inside the test).
- **Golden / byte-identical:** Task 1 baselines **two frozen fixtures** (volatile
  fields normalized) — a single-module repo AND a one-real-member `go.work` (with
  excluded members) that directly tests the "collapses to 1 member = identical"
  refactor claim — diffed in Task 9. **Not** archfit-self (its output
  legitimately changes here). archfit-self is guarded by `make archfit` +
  `TestGolden` double-run determinism instead.
- **Acceptance:** real runs against `~/workspace/omni` — `--root <go-subdir>` and
  the workspace root — asserting `go/packages: ok`, ≥2 connected first-party
  modules, classified cross-module edges, measurable
  boundary_integrity/coupling_balance/dependency_graph_health/cohesion, and
  file-count scoping to the subtree.
- **No e2e/UI** — CLI project. Integration tests skip cleanly when a tool is
  absent.

## What Goes Where

- **Implementation Steps (`[ ]`):** code, tests, docs in this repo.
- **Post-Completion (no checkboxes):** manual verification on omni + downstream.

## Implementation Steps

### Task 1: Byte-identical baseline harness (do FIRST, before any refactor)

- [x] add **two frozen fixtures** under `internal/extract/golang/testdata/`: (a) a
      single-module repo, (b) a one-real-member `go.work` (the member + ≥1
      excluded/testdata member) — (b) directly exercises the "collapses to 1
      member = identical" path that is the core refactor risk. Do **NOT** baseline
      archfit-self: its output legitimately changes under this plan (Tasks 2–6 add
      files/imports) — it is covered by `make archfit` + `TestGolden` instead
- [x] add a test helper that runs `archfit check --full --format json` on each
      fixture and normalizes volatile fields (timestamps, durations, absolute
      paths) to stable placeholders; commit the normalized outputs as baseline
      artifacts beside the fixtures
- [x] add `TestByteIdentical_SingleModule` and `TestByteIdentical_OneMemberWorkspace`
      that re-run + normalize + diff against the committed baselines (xfail-tolerant
      until Task 9 flips them to hard assertions)
- [x] document the known divergence risk: old code derives one `modPath` from the
      first non-nil-`Module` package; new code uses each `pkg.Module.Dir` — they
      can diverge on `Module==nil` (nested/vendored/synthetic) packages
- [x] run tests — must pass before next task

### Task 2: Scope plumbing — GitRoot + ScanRoot + non-fatal RepoRoot

- [x] add `GitRoot string` to `scope.Scope` (`internal/scope/scope.go`); keep
      `Root` = the **analysis boundary** (ScanRoot); update the doc comment
- [x] add `Root string` to `config.ScopeConfig` (the absolute `--root`, empty =
      default) + thread through `ForScope()` consumers in `cmd`
- [x] rework `Resolve`: resolve `gitRoot` via `RepoRoot`; on error, in **full
      mode** set `GitRoot=""` and continue (non-git is analyzable); in **delta
      mode** keep the hard error (no git → no diff). Set
      `Root = abs(cfg.Root)` when non-empty, else `gitRoot` (when both empty,
      fall back to `abs(cfg.WorkDir)`). Compute and store the subtree prefix
      `rel(GitRoot, Root)` (helper returns `""` when equal or non-git)
- [x] write tests: scanRoot≠gitRoot resolution; `--root` absent → scanRoot=gitRoot
      (prefix ""); non-git full mode (GitRoot="", no error); non-git delta mode
      (hard error); prefix computation
- [x] run `TestArchImports` + scope tests — must pass before next task

### Task 3: Git history/changed re-basing to the ScanRoot subtree

- [x] `git.History` / `git.Changed`: run in `GitRoot` (workDir) but add a
      path-scope arg (`-- <prefix>`) and **re-base** returned paths to
      ScanRoot-relative, dropping paths outside the prefix; both are no-ops when
      prefix is `""`
- [x] `cmd/archfit/pipeline.go`: call history/changed with `GitRoot` + prefix,
      keep all extractors on `s.Root` (ScanRoot); ensure `Scope.Changed` is
      ScanRoot-relative so `status.pathRelated` matches finding paths
- [x] write tests: history filtered+rebased to a subtree; changed-file filtering +
      re-basing in delta mode; prefix "" is byte-identical to today; a changed
      file outside the subtree is excluded
- [x] run tests — must pass before next task

### Task 4: cmd wiring + non-git full-mode path

- [x] pass `c.Root` (abs) into `ScopeConfig.Root`; confirm `--root` now sets the
      analysis boundary (not just git workdir) for `check`/`scan`/`explain`
- [x] confirm `outputInsideRootWarning` and `MergeExclusions` resolve against
      ScanRoot (already via `s.Root`); add a regression test
- [x] write tests: `--root <subdir>` scopes loc/fitness file counts to the subtree
      (assert `FilesSeen` matches subtree, not whole repo); non-git dir with
      `--full` produces a scorecard (no exit-3)
- [x] run tests — must pass before next task

### Task 5: Go member discovery (go.work + multi-go.mod + single)

- [x] add member discovery to `internal/extract/golang`: detect `go.work`
      (`os.Stat(scanRoot/go.work)`, else `go env GOWORK` / walk up); parse `use`
      dirs with `golang.org/x/mod/modfile.ParseWork` (no subprocess); keep members
      that are **under ScanRoot** and **not exclusion-matched** (globs relative to
      ScanRoot)
- [x] fallbacks: no go.work (or 0 in-scope members) → `[ScanRoot]` if it has
      `go.mod`; else walk ScanRoot for `go.mod` dirs (exclusion-filtered); else no
      members (→ `absent`)
- [x] add `golang.org/x/mod` to go.mod; keep the change inside the `extract/golang`
      adapter (ring-safe)
- [x] write table-driven tests: go.work-with-members, single-go.mod, multi-go.mod
      walk, exclusion+subtree filtering, archfit-self collapses to one member
- [x] run tests — must pass before next task

### Task 6: Go per-member concurrent load + per-package strip + first-party

- [x] replace the single `packages.Load({Dir: s.Root}, "./...")` with a
      per-member load (`Dir=memberDir, "./..."`), concurrent via `errgroup` bounded
      to `GOMAXPROCS`/NumCPU; merge nodes/edges deterministically (sorted)
- [x] strip the module prefix **per package** via `pkg.Module.Dir` rel ScanRoot
      (not one global `modPath`); first-party = the import's `Module.Path` ∈ the
      loaded member set; detect the synthetic-error package
      (`Module==nil && len(Errors)>0`) → `unresolved++`, never fatal
- [x] **CRITICAL — strength guard:** `buildStrengthHints`' `isInModule` predicate
      is built from the single `modPath`; in workspace mode there is no single
      `modPath`, so it returns false for every target → **zero StrengthHints → every
      Go edge abstains on strength → `coupling_balance` collapses even when distance
      is present**. Replace `isInModule` with the **member-set predicate** (Task 5)
      for StrengthHints too, not just edge classification
- [x] keep StrengthHints (full LoadMode) per member; **1 member ⇒ identical code
      path/output to today**; attach loaded members to `graph.Facts`
      (`GoModules []GoModule{Path, RelDir}`, mirroring Rust `CrateRoots`) for Task 8
- [x] write tests: 2-module strip produces ScanRoot-relative IDs; cross-module edge
      is first-party **and carries a StrengthHint** (guards the predicate above);
      missing-dep module → `partial` not empty; merged output sorted
- [x] run tests — must pass before next task

### Task 7: Workspace fixture + extractor integration

- [x] add `internal/extract/golang/testdata/workspace/` — `go.work` + 2 modules +
      a cross-module import, placed so the workspace root is **not** the git root
- [x] add a test loading that fixture: `go/packages: ok`, ≥2 first-party module
      nodes, the cross-module edge classified first-party (carries a StrengthHint),
      node IDs ScanRoot-relative
- [x] write the negative case: load at the workspace root pre-fix would be
      empty/synthetic (guards the regression)
- [x] run tests — must pass before next task

### Task 8: Auto-register go.work members as modules (≥2-member gate)

Must precede the config-less acceptance gate (Task 9): on config-less omni the
graph has **no modules** until members are registered, so cross-module edges stay
unclassified and `coupling_balance` reads `n/a`. This task supplies the
**distance** half of the chain (Task 6's strength guard supplies the other half).

- [x] add a Go analogue of `classify.AugmentModulesFromGraph`: when **≥2**
      workspace members were loaded, register each member's `RelDir` as a module
      (path glob `<relDir>/**`) **unless** already covered by a config module glob;
      gate strictly on ≥2 members so single-module repos + archfit-self are
      untouched (mirrors the Rust `::` gate)
- [x] consume `graph.Facts.GoModules` (Task 6) in the classify stage; verify
      cross-member edges now classify with a real `Distance` so `BookScorer`
      scores them (volatility stays undeclared/conservative — emits the existing
      advisory decision-task, not a blocker)
- [x] write tests: ≥2 members auto-register + cross-member edge gets distance +
      `coupling_balance` measures; 1 member does not augment; config module globs
      win over auto-registration
- [x] run `make archfit` + tests — must stay green (1-member self path unchanged)

### Task 9: Byte-identical proof + omni Go acceptance (Tier 0 gate)

- [x] flip Task 1's `TestByteIdentical_SingleModule` + `_OneMemberWorkspace` to
      hard assertions; diff the two **frozen fixtures** against the committed
      baselines; investigate and resolve any `Module==nil` divergence. (archfit-self
      is NOT byte-diffed — it is covered by the next bullet)
      **Result:** both tests passed byte-identical without any extractor fix needed
      — no `Module==nil` divergence observed on single-module or 1-member-workspace
      fixtures after Tasks 2-8.
- [x] run `make archfit` (dogfood) + `TestGolden` — stay green; regenerate only if
      a block legitimately changed (inspect diff)
      **Result:** both green, no regeneration needed.
- [x] acceptance on `~/workspace/omni` (config-less, relying on Task 8
      auto-registration): `archfit check --root <go-subdir> --full` and at the
      workspace root → `go/packages: ok`, ≥2 connected first-party modules,
      **classified** cross-module edges, **measurable** `boundary_integrity` /
      `coupling_balance` / `dependency_graph_health` / `cohesion`; `--root <subdir>`
      restricts loc/complexity/clones/fitness counts to that subtree (record numbers)
      **Result (--root ~/workspace/omni/server/shared):** - go/packages: ok, 645 files_seen - N=223 modules; 2430 total edges (208 scored first-party cross-module, 117 abstained) - classified cross-module edges: ✓ (208 scored with distance) - boundary_integrity: 50/100 (mixed) ✓ measurable - coupling_balance: 38/100 (poor) · mean balance 4.4/10 ✓ measurable - dependency_graph_health: 81/100 (strong) ✓ measurable - cohesion_modularity: 35/100 (poor) ✓ measurable - loc: 519 files (subtree-scoped); jscpd: 802 files (subtree-scoped)
      **Workspace root (~178 members):** timed out at 5 min — scale issue, no
      output produced; motivates Task 11 (module scoping) + Task 12 (timeouts).
- [x] run full suite (`make test`, `TestArchImports`, `make lint`) — all green

### Task 10: Flag vacuous history-fed dimensions

- [x] when the (subtree-scoped) history is empty/shallow (<2 commits, or
      `git log` returns nothing for the prefix), mark `change_locality` and other
      churn-fed dimensions `n/a (no history)` instead of printing a strong number
      (the omni `96/strong` on a 1-commit checkout artifact)
- [x] write tests: empty-history subtree → `n/a (no history)`; populated history →
      unchanged
- [x] run tests — must pass before next task

### Task 11: Go module-scoping config + scale guardrails

- [x] add `tools.go.modules` (include/exclude member globs) to scope a run to a
      subset of workspace members; default = all in-scope members
- [x] confirm bounded concurrency (Task 6) and benchmark a full omni workspace
      run; record wall-clock + peak; if full LoadMode is too slow, document the
      ceiling and the import-graph-only fallback (`NeedName|NeedImports|NeedModule`,
      drops StrengthHints) as a follow-up knob — do not build it unless the
      benchmark requires it
      **RESULT:** omni full run completed in ~77s wall clock (errcpd TS error unrelated
      to Go load). Full NeedTypesInfo LoadMode is acceptable at this scale. Import-graph-
      only fallback NOT built (benchmark did not require it). Ceiling documented in
      `golang.go` comment: scale mitigation is `tools.go.modules` scoping + Task 12 timeout.
- [x] write tests: include/exclude member selection; empty selection → `absent`
- [x] run tests — must pass before next task

### Task 12: Per-analyzer timeout / watchdog (resilience)

- [x] wrap the subprocess analyzers (scip, jscpd/clones, complexity) with a
      per-analyzer context timeout; on timeout degrade to `n/a (timed out)`
      coverage, never hang the run (the focus-analytics >55-min hang on a
      122k-LOC generated file)
- [x] make the timeout configurable (`tools.<x>.timeout`, sensible default); the
      timeout is the **primary** mitigation (scip-go indexes via the Go build, not
      a file list, so it cannot be pre-filtered)
- [x] write tests (fake Runner that blocks): timeout fires → `n/a (timed out)`,
      verdict unaffected, no deadlock
- [x] run tests — must pass before next task

### Task 13: Best-effort exclusion propagation to jscpd

- [x] pass the effective exclusion globs to jscpd (`--ignore`/glob) so excluded /
      generated files are skipped before duplication scanning
- [x] document explicitly that scip-go cannot honor file-level exclusions
      (build-based indexing) — Task 12's timeout is its guard; this task is
      jscpd-only / best-effort
- [x] write tests: excluded file absent from jscpd input
- [x] run tests — must pass before next task

### Task 14: Python intrusive strength at symbol level (cheap, no heuristics)

- [ ] `grimp_helper.py`: emit `line_contents` from `get_import_details()` (it is
      already fetched and discarded)
- [ ] `py.go`: parse `from x import _sym` → `StrengthIntrusive` at **symbol**
      level (extends today's module-level-only underscore check). Do **not** use
      PascalCase/snake_case naming heuristics for model/functional — that violates
      abstain-not-fake; leave non-intrusive edges abstaining until Task 15
- [ ] write tests: `from pub import _priv` → intrusive; normal import stays
      abstained; existing module-level intrusive still detected
- [ ] run tests — must pass before next task

### Task 15: Python strength via scip-python (phase 2, opt-in)

- [ ] when `tools.scip` is enabled for Python, classify non-intrusive edges from
      the SCIP symbol graph by target symbol kind: function → functional, class →
      model, `abc.ABC`/`typing.Protocol`/TypedDict/@dataclass → contract/model;
      `_`-prefixed → intrusive. Genuine evidence only (abstain when SCIP absent)
- [ ] write tests (canned SCIP fixture): each kind → expected strength; SCIP
      absent → abstain (unchanged)
- [ ] run tests — must pass before next task

### Task 16: TS/Python multi-package scoping (depcruise/grimp)

- [ ] TS: scope depcruise to the analysis subtree with `--include-only ^<rel>` +
      `--exclude ^node_modules`; confirm `resolveTSConfig` finds the
      package-local tsconfig under ScanRoot (fixed transitively by Task 4); add a
      multi-package/workspace path (multiple roots → one merged graph) where a
      single tsconfig covers the aliases
- [ ] Python: extend `grimp_helper.py` to accept multiple package names
      (`build_graph('a','b',...)`); discover top-level packages under ScanRoot.
      **Document the shared-venv constraint**: packages must be co-importable from
      one environment — omni's ~42 services are not, so cross-service Python
      coupling stays out of reach without per-service venvs (note, don't promise)
- [ ] write tests: depcruise include-only/exclude args; grimp multi-package arg
      construction; tsconfig resolution under a subdir
- [ ] run tests — must pass before next task

### Task 17: Docs, CLAUDE.md, final acceptance

- [ ] update `docs/guide`: `--root` as the analysis boundary, `go.work` support,
      `tools.go.modules`, `tools.<x>.timeout`, `n/a (no history)` / `n/a (timed
out)` semantics
- [ ] update `CLAUDE.md` Invariants: scanRoot vs gitRoot; Go workspace loading +
      ≥2-member module auto-registration; per-analyzer timeout
- [ ] re-verify every acceptance criterion (below) on omni; record results in
      `docs/notes/`
- [ ] run `make all` (fmt → lint → test → archfit) — all green

## Acceptance criteria (verified in Tasks 9 & 17)

- On a `go.work` repo with no root `go.mod`, `archfit check --root <subdir>
--full` builds a non-empty Go graph: `go/packages: ok`, ≥2 connected
  first-party modules, classified cross-module edges, and measurable
  `boundary_integrity` / `coupling_balance` / `dependency_graph_health` /
  `cohesion`.
- `--root <subdir-of-monorepo>` restricts loc/complexity/clones/fitness file
  counts to that subtree (`FilesSeen` matches the subtree, not the whole repo).
- Single-module repos produce **byte-identical** output to before (Task 1/8 diff).
- Determinism preserved (sorted nodes/edges/changed files; `TestGolden` green).
- A non-git directory analyzed with `--full` produces a scorecard (no exit-3);
  delta mode without git still hard-errors.

## Technical Details

- **Two roots.** `Scope.Root` = ScanRoot (analysis boundary, all extractors);
  `Scope.GitRoot` = `git rev-parse --show-toplevel` (git ops only). `prefix =
filepath.Rel(GitRoot, Root)` — no extra `git rev-parse --show-prefix` call.
  `--root` absent ⇒ ScanRoot=GitRoot, prefix="" ⇒ byte-identical.
- **Non-fatal git is per-mode.** Full mode proceeds with `GitRoot=""` (history
  empty); delta mode without git is a hard error.
- **Go member set.** go.work (`modfile.ParseWork`) ∩ ScanRoot ∩ not-excluded;
  else single `go.mod`; else walk for `go.mod` dirs. Per-package strip via
  `pkg.Module.Dir`; first-party via member-set membership; synthetic-error
  package detected, not fatal. **1 surviving member = today's path.**
- **Out-of-the-box coupling.** Undeclared volatility does **not** abstain
  (`scorer_book.go:91-95`); ≥2-member auto-registration supplies distance, Go
  type-info supplies strength ⇒ `coupling_balance` scores. Volatility stays the
  conservative worst case + an advisory decision-task.
- **Determinism.** Concurrent member loads merge into sorted node/edge slices;
  member discovery sorts use-dirs; git rebasing preserves git's order then sorts.
- **Resilience.** Per-analyzer timeout is the primary guard for pathological
  inputs; jscpd exclusion-propagation is best-effort; scip-go is build-based and
  unfilterable by file.

## Non-goals / Future (explicitly out of scope here)

- **Projects mode** — declaring N sub-projects and emitting per-project
  scorecards **plus an aggregate** in one invocation (omni doc Tier 1 #3). This
  plan delivers one **unified** cross-module graph per `--root`, which already
  measures cross-component coupling; multi-scorecard orchestration is a separate
  plan.
- **Cross-deploy-unit inter-project graph** beyond the existing `deploy_unit`
  distance (omni doc #4).
- **LLM resilience** (`enrich`/`autopilot` JSON-abort, omni doc #6) — off-gate,
  separate concern; not touched here.
- **`init` grouping heuristics / default-on analyzers** (omni doc #7–#8) —
  ergonomics; out of scope. (Honest-`n/a` is addressed by Task 10.)
- **Python cross-service coupling on omni** — blocked by per-service venvs
  (Task 16 note), not by archfit.

## Post-Completion

_Manual/external only — no checkboxes._

- Run `archfit check --root <subdir>` and at the workspace root on `~/workspace/
omni` (Go), the `client/` TS app, and a Python service; eyeball cross-module
  edges and the scoped file counts.
- Benchmark a full omni `go.work` run; decide whether the import-graph-only
  LoadMode fallback (Task 11) is warranted.
- Confirm a downstream monorepo behaves as intended before promoting any
  `tools.<x>.gate` to `fail`.
