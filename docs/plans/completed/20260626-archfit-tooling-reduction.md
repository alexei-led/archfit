# archfit tooling reduction: fewer external dependencies, same/more metrics

> **STATUS: COMPLETED 2026-06-26.** Executed via multi-agent workflows; validated
> lossless on all 7 repos (0 metric regressions — see report §7.4).
> Done: T1 baseline · T2–4 gitnexus drop (`c631195`) · T5–7 complexity backends
> (`60dcb42`) · T9 Go strength (`7c86d7f`) · T10 validation · T11–12 docs.
> **T8 SKIPPED** — cargo-modules/scip are complementary, demoting would regress Rust
> (report §5.3). depcruise `--ts-config` shipped earlier (`dd40bf7`).

## Overview

Apply the lossless-core recommendations from
`docs/archived/research/tooling-analysis-2026-06.md`: shrink archfit's external-tool
footprint **without losing any metric**, and recover one currently-missing signal
from a tool already required. Every cut routes its metric to a tool archfit already
runs (or to in-process code), so coverage is preserved or improved while the
prerequisite set drops.

**Changes (all lossless or additive):**

1. **Drop gitnexus entirely** (removes a Node runtime + a separate index build).
   Its only signal — a per-file distinct-dependant count from a static
   `CALLS/ACCESSES/IMPORTS/EXTENDS` graph — is a strict subset of the SCIP symbol
   graph archfit already resolves. Compute the same count in-process from
   `symbol.Graph` and feed `risk_hub` + the facts block from it. No metric lost.
2. **Remove the default Python pin from `complexity`**: use **gocyclo** for exact
   Go CCN (single static Go binary, zero extra runtime) and an **ast-grep
   decision-point proxy** for TS/Py/Rust (the `sg` prereq is already paid for
   `syntax`). Keep **lizard** as an opt-in backend for exact per-function CCN.
3. **Demote cargo-modules to a fallback** behind rust-analyzer SCIP for the
   intra-crate module graph (SCIP does the whole workspace in one pass and resolves
   `#[path]`/inline-mod/re-exports cargo-modules misses).
4. **Coverage gain (no new prereq): go/packages `NeedTypes`** → derive
   `StrengthHint` for Go internal edges (today Go sets only `EdgeKind`, never
   strength — `internal/extract/golang`), tightening `coupling_balance` for Go.

**Already done (committed this session, not in scope):** dependency-cruiser
`--ts-config` auto-detection (`fix(ts)`, commit `dd40bf7`).

**Deliberately out of scope (YAGNI):**

- **grimp `find_illegal_dependencies_for_layers`** — archfit already gates layering
  language-agnostically via `forbidden_layer_direction`; grimp's variant would
  duplicate that for Python only, adding code for marginal chain-detail gain.
- **Hygiene lane** (cargo-machete / deptry / knip) — these _add_ tools; they conflict
  with "less dependent" and are a separate opt-in metric class (report §8). Defer.
- **ast-grep pattern-path streaming** (`astgrep.go:86`) — small output, separate
  perf task, not a dependency change.

## Context (from discovery)

- **gitnexus**: extractor `internal/extract/gitnexus/`; called at
  `cmd/archfit/pipeline.go:228`; config `ToolGitnexus`/`GitnexusEnabled`/
  `GitnexusExplicitlyDisabled` in `internal/config/tools.go`; signal field
  `GitnexusImpact` in `internal/model/signal/signal.go` (both `Signals` and
  `SymbolSignals`); JSON field `gitnexus_impact` in
  `internal/model/diagnostic/diagnostic.go:153`. Two SCIP-gated consumers:
  `internal/metrics/risk/risk_hub.go` (bounded `[1.0,2.0]` factor) and
  `internal/facts/facts.go:138-148` (per-module `gitnexus_impact`). Both already
  return n/a when the SCIP symbol graph is empty, so gitnexus is only ever live
  alongside SCIP.
- **Symbol graph**: `internal/model/symbol/graph.go` — `Refs[from]=set(to)`,
  `Path[sym]` (repo-relative defining file), `Module[sym]`. Resolved by
  `internal/extract/scip/symbols.go` (the only `SymbolResolver`), surfaced in the
  engine as `ex.scipSymbols` (`internal/engine/engine.go:330,186,273`).
- **complexity**: `internal/extract/complexity/complexity.go` (lizard via PATH or
  `uvx`); record shape `signal.ComplexityFunc{File,Name,CCN,NLOC,Line}`; called at
  `cmd/archfit/pipeline.go:210`; metric `internal/metrics/.../complexity.go`
  (per-function CCN>15). ast-grep adapter + rules live in
  `internal/extract/astgrep/` (`syntax.go`, `rules/`).
- **rust**: `internal/extract/rust/rust.go` + `modules.go` (cargo-modules per
  crate); SCIP via the shared scip resolver; config `ToolCargoModules`, `ToolScip`.
- **golden tests**: `internal/engine` `TestGolden`; import ring `arch_test.go`
  `TestArchImports`; dogfood gate `make archfit` against `.archfit.yaml` (which
  currently enables gitnexus).
- **Validation repos** (all local under `/Users/alexei/Workspace`): Go — archfit,
  pumba, spotinfo; TS — codegraph; Python — ccgram; Rust — herdr, yazi.

## Development Approach

- **Testing approach**: Regular (code, then tests in the same task) — matches the
  repo's table-driven + moq style.
- Complete each task fully before the next; small, focused changes.
- **Every task includes new/updated tests** (success + error/edge cases) as separate
  checklist items.
- **All tests pass before starting the next task.** Run `make test` (or the scoped
  package) after each change.
- Golden output changes are **never automatic**: regenerate `TestGolden` deliberately
  and inspect the diff (per CLAUDE.md).
- Keep the import ring green (`go test ./internal/ -run TestArchImports`) — core
  packages must not import `os`/`exec`/adapters.
- Maintain graceful degradation: a metric with no signal must read `n/a`, never a
  false zero.

## Testing Strategy

- **Unit tests**: required every task. Fake the `toolrun.Runner` (moq) for any
  subprocess; never shell out in unit tests.
- **Golden test**: update `internal/engine` `TestGolden` whenever output shape
  changes (gitnexus removal, field rename, schema bump).
- **Structural gates**: `TestArchImports` + `make archfit` (dogfood) must stay green.
- **No project e2e/UI tests** — the cross-repo validation in Task 10 is the
  integration gate (before/after metric diff).

## Progress Tracking

- Mark completed items `[x]` immediately when done.
- Add newly discovered tasks with ➕ prefix; blockers with ⚠️ prefix.
- Update this plan if scope changes during implementation.

## What Goes Where

- **Implementation Steps** (`[ ]`): code, tests, config, docs achievable in-repo.
- **Post-Completion** (no checkboxes): release tagging, consumer-side notes.

## Implementation Steps

### Task 1: Baseline metric snapshot + diff harness (run before any code change)

- [ ] write `scratchpad/metric-baseline.sh`: for each validation repo, run the
      current `.bin/archfit` (`make build` at HEAD first) with `check --full --format
json`, using the repo's `.archfit.yaml` if present, else a generated minimal
      config that enables the repo's language + `scip` + `complexity` + `clones`
      (so the gitnexus-replacement and complexity paths are exercised); save to
      `scratchpad/baseline/<repo>.json`
- [ ] write `scratchpad/metric-diff.sh <before.json> <after.json>`: report any
      dimension/metric that regressed (band dropped, value→n/a, confidence dropped)
      or newly appeared, and the per-run tool list (to measure dependency reduction)
- [ ] run the baseline script; confirm `scratchpad/baseline/*.json` exists for all 7
      repos and note which dimensions are n/a per repo (the pre-change reference)
- [ ] commit the baseline JSONs + scripts to `scratchpad/` (gitignored is fine; keep
      locally) so Task 10 can diff against them
- [ ] verification: diff harness runs clean on a repo compared against itself (zero
      regressions reported)

### Task 2: Compute per-file dependant count from the SCIP symbol graph

- [ ] add `DependantsFromSymbolGraph(g symbol.Graph) map[string]int` (in
      `internal/model/symbol` or a small core helper): for each `from→to` in
      `g.Refs`, with `fa=g.Path[from]`, `fb=g.Path[to]`, when `fa!=""`, `fb!=""`,
      `fa!=fb`, add `fa` to the dependant set of `fb`; return `file → len(distinct
dependant files)` — exactly the gitnexus cypher semantics, in-process
- [ ] ensure it returns nil/empty for an empty graph (no false zeros) and is
      deterministic (stable over map iteration — counts only, so order-independent)
- [ ] keep it in the core ring (stdlib only; no os/exec) so `TestArchImports` stays
      green
- [ ] write table-driven tests: multi-file cross-module refs, self-reference
      excluded, intra-file refs excluded, empty graph → nil, multi-referrer dedup
- [ ] run tests — must pass before Task 3

### Task 3: Rewire risk_hub + facts to SCIP dependants; rename GitnexusImpact → SymbolDependants everywhere

- [ ] rename the signal field `GitnexusImpact` → `SymbolDependants` in **both**
      `signal.Signals` and `signal.SymbolSignals` (`internal/model/signal/signal.go`);
      in `engine.go` compute it once via `DependantsFromSymbolGraph(ex.scipSymbols)`
      and pass to the risk_hub `SymbolSignals` and `facts.Build` — delete the old
      `in.Signals.GitnexusImpact` pass-through (engine.go:186,273)
- [ ] `internal/metrics/risk/risk_hub.go`: read `in.Symbol.SymbolDependants`; rename
      every gitnexus identifier (`gitnexusFactor`→`dependantFactor`,
      `gitnexusImpactFactor`→`dependantFactor` func, `gitnexusPresent`→
      `dependantsPresent`, `moduleImpactFromFiles`→`moduleDependantsFromFiles`);
      relabel display `gn×` → `dep×`; purge "gitnexus"/"historical impact" wording from
      comments; keep the `[1.0,2.0]` formula and the "factor 1.0 when absent" behavior
      byte-identical
- [ ] `internal/facts/facts.go`: rename the `gitnexusImpact` param → `symbolDependants`
      and update the doc comment (SCIP-derived; drop "gitnexus")
- [ ] `internal/model/diagnostic/diagnostic.go`: rename the `GitnexusImpact` field +
      JSON tag `gitnexus_impact` → `SymbolDependants`/`symbol_dependants`; bump
      `schema_version`; update the comment (drop "historical change-impact")
- [ ] `internal/score/score.go`: drop `"gitnexus"` from `semanticTools` (~line 712)
      and from the absent-semantic-tools comment (~line 620) — gitnexus is no longer a
      tool, so it must not sit in the `analysis_confidence` denominator
- [ ] update every touched test for the renames: `risk_hub_test.go` (rename
      `TestRiskHub_GitnexusImpactRefinesRanking`), `facts_test.go`
      (`TestBuild_GitnexusImpact`), `jsonout_test.go`, `console_test.go`,
      `markdown_test.go`, `scorecard_test.go`, `score_test.go`, signal/diagnostic
      tests; assert the factor still refines ranking and nil dependants ==
      byte-identical to the no-factor output
- [ ] regenerate `TestGolden` deliberately; inspect the diff (field rename + schema
      bump + one fewer semanticTool; no metric value change where SCIP was already on)
- [ ] run `make test` — must pass before Task 4

### Task 4: Delete the gitnexus tool and purge every leftover (verified clean)

- [ ] delete the package `internal/extract/gitnexus/` (gitnexus.go + gitnexus_test.go)
- [ ] `cmd/archfit/pipeline.go`: remove the `internal/extract/gitnexus` import, the
      `gitnexus.Run(...)` call, `gitnexusCov`, the `change.GitnexusImpact` assignment,
      `noteToolErr("gitnexus", …)`, and the gitnexus comments — the dependant map now
      comes from the engine (Task 3), not a pipeline tool
- [ ] `cmd/archfit/consts.go`: remove `toolGitnexus = "gitnexus"`;
      `cmd/archfit/doctor.go`: remove gitnexus from the doctor tool inventory;
      `cmd/archfit/main_test.go` + `cmd/archfit/pipeline_test.go`: drop gitnexus refs
- [ ] `internal/config/tools.go`: remove `ToolGitnexus`, `GitnexusEnabled`,
      `GitnexusExplicitlyDisabled`; `internal/config/config_test.go` +
      `internal/config/testdata/new_tools_metrics.yaml`: drop the gitnexus tool entry
- [ ] `internal/extract/rust/rust.go:145`: update the comment listing
      "complexity/clones/gitnexus" (drop gitnexus)
- [ ] self-config cleanup: remove the `gitnexus:` tool stanza (`.archfit.yaml` ~line
      610), the `internal/extract/gitnexus:` module def + globs (`.archfit.yaml`
      ~315-320), and the two `internal/extract/gitnexus` label entries in
      `.archfit-labels.yaml` (~253, ~317) — the package no longer exists
- [ ] **KEEP (not a leftover)** `internal/scope/scope.go:25-26`
      (`**/.gitnexus/**`, `**/.codegraph/**` walk-exclusions) and its `scope_test.go`
      cases: archfit must still avoid scanning a present index dir even though it no
      longer runs the tool. Add a one-line comment: "ignore index dirs; tool removed."
- [ ] current user-facing docs: purge gitnexus from `CLAUDE.md`, `README.md`, and
      `docs/guide/{install,configuration-reference,tooling,languages,metrics,
troubleshooting,agent-feedback,release-notes}.md` (note the removal; risk_hub /
      `symbol_dependants` are SCIP-derived). **Leave historical records unchanged**:
      `docs/plans/completed/*` (incl. `gitnexus-adapter-decision.md`) and
      `docs/archived/*` are history, not current behavior.
- [ ] confirm `TestArchImports` green (gitnexus package gone from the ring)
- [ ] **verification gate**: `grep -rin gitnexus internal/ cmd/ .archfit.yaml
.archfit-labels.yaml CLAUDE.md README.md docs/guide/` returns **only** the two
      intentional `scope.go` index-dir exclusions — zero other code/config/test/
      current-doc hits
- [ ] run `make test` + `make lint` + `make archfit` (dogfood, gitnexus-free) — must
      pass before Task 5

### Task 5: gocyclo backend for exact Go CCN

- [ ] add a gocyclo backend in `internal/extract/complexity` (new file): detect
      `gocyclo`, run `gocyclo <root>` (or `-over 0` for all funcs), parse
      `<ccn> <pkg> <func> <file>:<line>` into `[]signal.ComplexityFunc`
      (`File,Name,CCN,Line`; NLOC stays 0 — gocyclo has no NLOC, the pure-Go `loc`
      already covers size)
- [ ] route Go files through gocyclo when present; the backend is selected per the
      Task 7 backend policy (default auto, lizard opt-in)
- [ ] graceful degradation: gocyclo absent → fall through to ast-grep proxy / lizard;
      coverage reason names the chosen backend
- [ ] write tests with a captured gocyclo-output fixture: parse correctness, line
      attribution, empty output, malformed line skipped
- [ ] run tests — must pass before Task 6

### Task 6: ast-grep decision-point CCN proxy for TS/Py/Rust

- [ ] add ast-grep decision-point rules per language under
      `internal/extract/astgrep/rules/` (count `if/for/while/case/catch/&&/||/?:`
      and language equivalents — `match` for Rust, `elif/except` for Python),
      reusing the validated Go rule shape (report §7.1)
- [ ] add a complexity provider that streams the rule matches (reuse
      `Runner.Stream`/`decodeSyntaxStream`), assigns each match to its enclosing
      function by line range, and emits `[]signal.ComplexityFunc` with
      `CCN = 1 + decisionPoints`
- [ ] route TS/Py/Rust through the proxy when `sg` is present (and lizard is not the
      chosen backend); keep it report-only-grade (over-flagging is safe)
- [ ] write per-language tests against small fixtures with known decision-point
      counts; assert per-function CCN and enclosing-function attribution
- [ ] run tests — must pass before Task 7

### Task 7: Backend policy + validate proxy vs lizard; keep lizard opt-in

- [ ] add a `tools.complexity.backend` selector (`auto` default = gocyclo for Go +
      ast-grep proxy for others; `lizard` = force exact lizard; `off` unchanged);
      wire it in `internal/config/tools.go` + the pipeline; remove the default
      Python pin (lizard only runs when explicitly chosen)
- [ ] write `scratchpad/ccn-validate.sh`: on codegraph (TS), ccgram (Py), yazi (Rust),
      compare proxy CCN>15 hotspots vs `lizard` ground truth; record recall (target
      ≈1.00, zero misses) and precision; document results inline in this plan
- [ ] if any language's recall < 1.0, tighten that language's rule before proceeding;
      record the final recall/precision per language
- [ ] write tests for backend selection (auto → gocyclo for Go, proxy for TS/Py/Rust;
      `lizard` forces lizard; absent tool → graceful n/a)
- [ ] update docs (`CLAUDE.md`, guide): complexity backends + that Python is no
      longer pinned by default
- [ ] run `make test` — must pass before Task 8

### Task 8: ~~Demote cargo-modules to a fallback behind rust-analyzer SCIP~~ — SKIPPED (would regress Rust; see §5.3)

⚠️ **SKIPPED after code investigation.** cargo-modules and scip are **complementary,
not substitutes**: cargo-modules is the _sole_ source of `<crate>::<mod>` module-graph
**structure** (`runModuleGraph` appends the nodes/edges; `AugmentModulesFromGraph`
only promotes existing `::` nodes), while scip only _enriches existing edges_ with
`StrengthHint` and feeds symbol-level signals (`engine.go:342`). Demoting
cargo-modules when scip is on would leave single-crate Rust with no module nodes →
`cycle`/`blast_radius`/`cohesion_lcom` regress to n/a, and scip's strength map would
have nothing to attach to. It also removes no prerequisite (cargo-modules is already
opt-in). Not lossless → not done. The report's §5.3 has been corrected accordingly.

Optional future feature (additive, NOT this plan): derive `<crate>::<mod>` module
edges from the scip symbol graph's `Refs` so scip can supply structure too — only
then could cargo-modules become genuinely optional.

### Task 9: go/packages NeedTypes → StrengthHint for Go internal edges (coverage gain)

- [ ] extend `internal/extract/golang` to load `NeedTypes|NeedTypesInfo` and derive a
      `StrengthHint` for `uses_internal` edges (e.g. function-call/type-reference
      density between packages), where today only `EdgeKind` is set (memory: Go sets
      no StrengthHint)
- [ ] keep it within the existing go/packages load (no new tool); guard cost — only
      request types when the Go extractor runs, and degrade to today's behavior if
      type info is unavailable (build errors)
- [ ] confirm `coupling_balance` consumes the new Go strength (distance/balance now
      measures where it abstained for Go)
- [ ] write tests: edges get a strength hint from a typed fixture; missing type info
      → unchanged (no strength, no panic); strength feeds the scorer
- [ ] run `make test` — must pass before Task 10

### Task 10: Full before/after validation on the 7 repos

- [ ] `make build`; re-run `check --full --format json` on all 7 repos with the same
      configs as Task 1; save to `scratchpad/after/<repo>.json`
- [ ] run `scratchpad/metric-diff.sh` baseline→after per repo; **assert no metric
      regressed** — specifically `risk_hub` and `symbol_dependants` still present on
      scip-on repos, `complexity` still flags the same hotspot families, Rust module
      coupling still measures
- [ ] record the **dependency-count reduction** per repo (Node dropped on non-TS via
      gitnexus removal; Python dropped on non-Python via lizard de-pinning) in this
      plan's results section
- [ ] investigate and resolve any regression or new n/a (fix forward; if a metric
      genuinely can't be preserved, stop and update the plan with a ⚠️)
- [ ] verification: 7/7 repos show zero metric regressions and a measured tool-count
      reduction

### Task 11: Verify acceptance criteria

- [ ] verify each Overview change is implemented and lossless (no metric removed)
- [ ] run full `make test` (race + coverage); `make lint`; `TestArchImports`;
      regenerate + inspect `TestGolden`; `make archfit` dogfood gate green
- [ ] verify coverage meets project standard (80%+) for changed packages
- [ ] update `docs/archived/research/tooling-analysis-2026-06.md`: mark §5.1/§5.2/§5.3/§9.1
      recommendations as implemented, with the Task 10 validation numbers

### Task 12: [Final] Documentation sync

- [ ] update `CLAUDE.md` (tool/runtime list, gitnexus removed, complexity backends,
      cargo-modules fallback, `symbol_dependants` schema field), `README`/guide, and
      `archfit doctor` tool inventory
- [ ] update project knowledge docs if new patterns emerged (SCIP-derived dependant
      count, ast-grep complexity proxy)

## Technical Details

- **Dependant-count algorithm** (Task 2): mirrors the retired gitnexus cypher —
  per file F, count distinct _other_ files whose symbols reference symbols defined in
  F. Source: `symbol.Graph.Refs` joined through `symbol.Graph.Path`. Same numeric
  signal, no Node, no index build.
- **risk_hub formula unchanged**: `breadth × config-volatility × dep-factor`, where
  `dep-factor = 1 + clamp(dependants/maxDependants, 0, 1) ∈ [1.0, 2.0]`. Only the
  _source_ of `dependants` changes (gitnexus → SCIP); behavior with the factor
  absent stays byte-identical.
- **ComplexityFunc** shape is fixed (`File,Name,CCN,NLOC,Line`); gocyclo and the
  ast-grep proxy must emit the same records so the `complexity` metric is unchanged.
  CCN proxy = `1 + decision-points` assigned to the enclosing function by line range.
- **Backend policy** (Task 7): `auto` (default) = gocyclo(Go) + ast-grep(TS/Py/Rust);
  `lizard` = force lizard (exact, re-pins Python); `off` = disabled. Default removes
  the Python pin.
- **Schema bump** (Task 3): `gitnexus_impact` → `symbol_dependants` is a breaking
  JSON change → bump `schema_version` and update golden + round-trip tests.

## Post-Completion

_Items requiring manual intervention or external systems — informational only._

**Manual verification:**

- Spot-check `risk_hub` rankings on archfit's own repo before/after to confirm the
  SCIP-derived factor ranks the same hubs as gitnexus did (report §7.2 found 7/10
  top-hub agreement; expect equal-or-better since SCIP is a superset).
- Confirm on a cold CI runner that removing gitnexus + de-pinning lizard actually
  drops the Node/Python install steps (the report's "first-run-on-fresh-CI" risk).

**Release:**

- This is a metric-source + schema change (`schema_version` bump). Tag a release
  only via the tag-triggered flow (`git tag -a vX.Y.Z`), never `gh release create`.
- Note the `gitnexus_impact` → `symbol_dependants` rename in release notes for any
  JSON consumers.
