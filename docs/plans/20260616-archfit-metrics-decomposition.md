# Metrics decomposition + per-family typed inputs

## Overview

`internal/metrics` is archfit's largest package (~2025 LOC, ~11× the codebase median) and a top
co-change hotspot — flagged by archfit's own `structural_weight` and `hidden_coupling`. A metric
change today tends to touch `metrics`, `engine`, `model/signal`, tests, and config together.

Two root causes, fixed in two phases:

- **Phase A — god-package.** Split the flat `internal/metrics` into cohesive family packages so
  no package exceeds the god-module threshold. (archfit measures `structural_weight` per package,
  so only a real package split — not a file split — moves the metric.)
- **Phase B — god-input.** Replace the 14-field `signal.MetricInput` (passed whole to every
  metric) with narrow per-family input types behind a **generic adapter**, so the compiler
  forbids a metric from reading a signal it didn't declare. Also split the producer-side carrier
  `signal.ChangeHistory` (itself a god-struct) into the same typed groups, so neither the
  cmd→engine boundary nor the engine→metric boundary carries a god-struct.

Both phases are behavior-preserving. The byte-identical golden test and the arch-ring test are
the safety gates.

## Context (from discovery)

- 13 metrics, registered as an ordered slice by `metrics.New(cfg) []Metric`
  (`internal/metrics/metrics.go:616`). Interface `Metric{Name,Version,Calculate(MetricInput)}`
  at `metrics.go:48`. Engine builds one `signal.MetricInput` and iterates `Calculate`
  (`internal/engine/engine.go:128-147`). Output order = `New()` order.
- `metrics.MetricInput`/`ChangeHistory` are type aliases to `signal.*` (`metrics.go:57,62`);
  external code (`cmd/archfit/enrich.go`, `internal/engine/engine_test.go`) already uses
  `signal.*` directly, not the aliases.
- Producer carrier `signal.ChangeHistory` (`signal.go:57`) holds churn, co-change, LOC,
  complexity, fitness, clones, gitnexus, extra-coverage — built in `cmd/archfit/pipeline.go`,
  passed via `engine.RunInput.Change`, consumed once in `engine.go`. `golden_test.go:110` builds
  `signal.ChangeHistory{}`.
- Shared helpers: band/confidence model + `computeDelta`/`distanceRank` in `metrics.go`;
  graph/history helpers + `naCount` + `shortModule` + `modularitySmallN` in `modularity.go`.
  `naCount` (8 callers), `shortModule` (blast/amp/hidden/risk), `modularitySmallN`
  (`risk_hub.go:240`) cross family lines → must go to a shared package; the module-_graph_
  helpers (`blastRadius`, `moduleKey`, …) have callers only within the modularity family.
- `internal/arch_test.go` is CI gate 1: core-ring packages must not import `os`, `os/exec`, YAML,
  or adapter packages. `coreRingPkgs` lists `internal/metrics` by **exact path** (line 23); the
  forbidden-import subtest (line 99-111) only checks listed paths → new sub-packages would go
  unchecked. `model/signal` is in `modelPkgs` and is stdlib+model-only.
- CI gate 2: `TestGolden` (`internal/engine/golden_test.go`) asserts byte-identical JSON across
  two runs on `testdata/golang`.
- Self-scan: `.archfit.yaml` scans archfit; `.archfit-baseline.json` holds baseline values
  (`structural_weight: 2`, `hidden_coupling: 46`, …). Forbidden-dependency rules use
  `internal/metrics/**` globs (prefix-tolerant). Gate currently clean.
- Build: `Makefile` has `fmt` (gofmt+goimports), `lint` (golangci-lint), `test` (race+coverage),
  `build`, `all`. Module is `go 1.26` — generics available.

## Development Approach

- **Testing approach**: Regular (move/refactor first, then run the moved tests + gates).
- Behavior-preserving: **no metric math, scoring, or output-schema changes.** `TestGolden` must
  stay byte-identical at every task boundary; `TestArchImports` must stay green.
- **GitNexus discipline (CLAUDE.md).** Before editing a symbol run `impact({target,
direction:"upstream"})`; stop and warn on HIGH/CRITICAL. Targets for this work:
  `signal.MetricInput`, `signal.ChangeHistory`, `metrics.Metric`, `metrics.New`, `engine.Run`,
  the cmd pipeline runner (`cmd/archfit/pipeline.go`), `captureMetric`. Before each phase commit
  run `detect_changes({scope:"compare", base_ref:"main"})`. If the GitNexus MCP is unavailable to
  the runner, the arch + golden + unit gates are the safety net.
- **Verification commands**: `make fmt`, `make lint`, `make test` (race+coverage), `make build`;
  cache-bust the gates with `go test -count=1 ./internal/ -run TestArchImports` and
  `go test -count=1 ./internal/engine/ -run TestGolden`.
- Complete each task fully (code + tests green) before the next. Each phase ends on its own commit
  on a feature branch.
- Do **not** touch the unrelated working-tree changes: `.archfit.yaml` (only the documented
  `metrics.public` edit), `.claude/`, `.codegraph/`, `AGENTS.md`, `CLAUDE.md`.

## Testing Strategy

- **Unit tests**: each metric's test file moves with its implementation (1:1). Shared test
  helpers (`buildGraph`, `importKey`, `approxEqual`) move to a `internal/metrics/metricstest`
  package imported by family test files.
- **Gate tests**: `TestArchImports` and `TestGolden` run (cache-busted) after every task.
- **Regression tests (Phase B)**: keep explicit coverage that SCIP symbol graph reaches
  `risk_hub`; changed files reach `change_locality`; fitness nil-vs-empty behavior is unchanged;
  clones + co-change behavior in `functional_candidates` is unchanged.
- **Self-scan**: rebuild `./.bin/archfit` and run `check --config .archfit.yaml --full`; verdict
  must stay `pass`. Re-baseline `.archfit-baseline.json` when informational values shift.

## Progress Tracking

- Mark items `[x]` immediately when done. Newly discovered tasks get `➕`; blockers get `⚠️`.
- Keep the plan in sync with actual work.

---

## Implementation Steps

### Task 1: Extract shared `internal/metrics/internal/result` package

- [ ] create `internal/metrics/internal/result` (nested under `internal/` so Go's visibility rule
      makes it importable only within the `internal/metrics` subtree — compiler-enforced privacy)
- [ ] move the band/confidence model from `metrics.go` (`bandScore`, `bandRank`, `bandByRank`,
      `confidenceCapRank`, `applyConfidenceCap`, band-name constants, `computeDelta`) and export
      the cross-family ones
- [ ] move `naCount`, `shortModule` (from `modularity.go`) and the `modularitySmallN` constant
      here (the latter is read by `risk_hub.go:240`; keeping it in `modularity` would force a
      `risk → modularity` edge)
- [ ] update in-root references to call `result.*`; confirm it imports only `internal/model/*` +
      stdlib (no `os`/`os/exec`/yaml/adapters)
- [ ] move/author `result` unit tests (band + confidence-cap + delta + naCount)
- [ ] `go build ./... && go test ./internal/... && go test -count=1 ./internal/engine/ -run TestGolden`
      — must pass before Task 2

### Task 2: Extract `internal/metrics/boundary` package

- [ ] move `EncapsulationMetric`, `UnbalancedEdgeMetric`, `CycleMetric`, `CoverageMetric`
      (from `metrics.go`) and `ChangeLocalityMetric` (from `change_locality.go`)
- [ ] move their private helpers: `distanceRank`, `isClassifiedStrength`,
      `classificationConfidence`, `statusPriority`, `forwardReach`
- [ ] switch these files to import `internal/model/signal` directly (drop the `metrics.` alias)
      and `internal/metrics/internal/result`; update root `New()` to construct `boundary.*`
- [ ] move the matching cases out of `metrics_test.go`/`change_locality_test.go`
- [ ] run build + `go test ./...` + `TestArchImports` + `TestGolden` — must pass before Task 3

### Task 3: Extract `internal/metrics/modularity` package

- [ ] move `BlastRadiusMetric`, `ChangeAmplificationMetric`, `HiddenCouplingMetric`,
      `StructuralWeightMetric`, `FunctionalCandidatesMetric`
- [ ] keep the module-graph/history helpers private here (`moduleKey`, `blastRadius`,
      `tarjanSCC`, `dominantLanguage`, `fileToModuleKey`, `moduleChurn`, `orderedPair`,
      threshold constants); import `result` for `naCount`/`shortModule`/`modularitySmallN`
- [ ] update root `New()` to construct `modularity.*`
- [ ] move `modularity_test.go` and the relevant `functional_candidates_test.go` cases
- [ ] run build + `go test ./...` + `TestArchImports` + `TestGolden` — must pass before Task 4

### Task 4: Extract `internal/metrics/risk` package

- [ ] move `RiskHubMetric` + `volatilityBandMultiplier`, `moduleSurfaceBreadth`,
      `moduleImpactFromFiles`, `gitnexusImpactFactor`, vol-band constants
- [ ] move `newRiskHubMetric(cfg)` here as `risk.NewMetric(cfg)`; import `result`
- [ ] update root `New()` to call `risk.NewMetric(cfg)`; move `risk_hub_test.go`
- [ ] run build + `go test ./...` + `TestArchImports` + `TestGolden` — must pass before Task 5

### Task 5: Extract `internal/metrics/intramodule` package

- [ ] move `ComplexityMetric`, `ArchitectureFitnessMetric`, `shortFile`
- [ ] delete the `ComplexityFunc` alias (`complexity.go:21`) — use `signal.ComplexityFunc`
- [ ] update root `New()` to construct `intramodule.*`; move `complexity_test.go` + `arch_fitness_test.go`
- [ ] run build + `go test ./...` + `TestArchImports` + `TestGolden` — must pass before Task 6

### Task 6: Slim root `internal/metrics` + shared test helpers

- [ ] reduce root to: `Metric` interface, `New(cfg)` (preserving the exact metric order from
      `metrics.go:617-631`), `doc.go`; delete the `MetricInput`/`ChangeHistory` aliases
- [ ] create `internal/metrics/metricstest` exporting `BuildGraph`, `ImportKey`, `ApproxEqual`;
      update family `_test.go` files to use it
- [ ] keep `TestNew_ReturnsAllMetrics` (count=13 + names) with `New()` in root
- [ ] run build + `go test ./...` + `TestArchImports` + `TestGolden` — must pass before Task 7

### Task 7: Update structural guards for the split

- [ ] in `internal/arch_test.go`, change `core_ring_no_forbidden_imports` to a **prefix scan**
      over all loaded packages matching the core-ring prefixes (incl. `internal/metrics`),
      mirroring `llm_ring_unreachable_from_internal`; keep `core_ring_packages_present` as exact
      paths and add the new sub-packages there
- [ ] in `.archfit.yaml`, add the family + shared sub-packages
      (`boundary`, `modularity`, `risk`, `intramodule`, `internal/result`) to the `metrics`
      module `public:` list
- [ ] add a negative arch-test case: a metrics sub-package importing an adapter is rejected
- [ ] run `go test -count=1 ./internal/ -run TestArchImports` + `TestGolden` — must pass before Task 8

### Task 8: Phase A verification + re-baseline + commit

- [ ] `make fmt && make lint && make test && make build`
- [ ] `./.bin/archfit check --config .archfit.yaml --full` — verdict `pass`; confirm
      `structural_weight` dropped (metrics no longer a god-module); note any `hidden_coupling` shift
- [ ] regenerate `.archfit-baseline.json` to absorb new informational values
- [ ] `detect_changes({scope:"compare", base_ref:"main"})` — only expected symbols/flows changed
- [ ] commit Phase A on the feature branch

### Task 9: Add narrow signal types (`internal/model/signal/signal.go`)

- [ ] add `CommonInput{Graph, Classifications, Findings, Baseline, ToolCoverage, ChangedFiles}`
- [ ] add signal groups: `HistorySignals{FileChurn, CoChange}`, `SymbolSignals{Graph,
    GitnexusImpact}`, `SizeSignals{FileLOC}`, `ComplexitySignals{Funcs}`,
      `DuplicationSignals{Clusters}` (Fitness reuses existing `Signals`)
- [ ] add per-family inputs embedding `CommonInput`: `HistoryInput`, `SymbolInput`, `SizeInput`,
      `ComplexityInput`, `FitnessInput`, `DuplicationInput` (the last also carries `History` for
      `functional_candidates`' co-change)
- [ ] add `CollectedSignals{Common, History, Symbol, Size, Complexity, Fitness, Duplication}`
      with `As*` projector methods returning each per-family input
- [ ] keep the old flat `MetricInput` for now (additive task); confirm `model_stdlib_only` holds
- [ ] run `go build ./... && go test -count=1 ./internal/ -run TestArchImports` — pass before Task 10

### Task 10: Swap to generic per-family dispatch (consumer side)

- [ ] in root `metrics`: change `Metric` to `{Name; Version; Calculate(signal.CollectedSignals)}`;
      add generic `Calculator[In]`, `wrapped[In]`, `adapt[In]` (see Technical Details)
- [ ] convert each metric's method to its typed family signature (e.g. `Calculate(signal.HistoryInput)`)
      reading `in.History.FileChurn`, `in.Symbol.Graph`, `in.Size.FileLOC`, `in.Complexity.Funcs`,
      `in.Duplication.Clusters`, `in.Fitness`, etc. (no math changes)
- [ ] rewrite `New()` to register via `adapt(metric, signal.CollectedSignals.As<Family>)`,
      preserving order (wrong family↔projection pairing must be a compile error)
- [ ] rewrite engine dispatch (`engine.go:128-147`): build one `signal.CollectedSignals` from the
      existing `in.Change.*` + `ex.*` + `in.Scope`, then
      `for _, m := range in.Metrics { append(results, m.Calculate(collected)) }` — no type switch
- [ ] update `cmd/archfit/enrich.go` `captureMetric` to `Calculator[signal.CommonInput]`;
      delete the old flat `MetricInput`
- [ ] update metric unit + engine spy tests to the new input types; keep the Phase B regression
      checks (SCIP→risk_hub, changed-files→change_locality, fitness nil-vs-empty, clones+co-change)
- [ ] run `make test` + `TestArchImports` + `go test -count=1 ... TestGolden` (byte-identical)
      — must pass before Task 11

### Task 11: Split the producer carrier (eliminate `ChangeHistory`)

- [ ] add `signal.RunSignals{History, Size, Complexity, Fitness, Duplication, GitnexusImpact,
    ExtraCoverage}` and replace `engine.RunInput.Change signal.ChangeHistory` with the typed
      groups; delete `signal.ChangeHistory`
- [ ] update `cmd/archfit/pipeline.go` to populate the typed groups directly from the producers
      (git history, loc, complexity, fitness, clones, gitnexus)
- [ ] update `engine.go` to assemble `CollectedSignals` from the typed `RunInput` groups +
      `ex.g`/`ex.scipSymbols`/`ex.coverages` (no behavior change)
- [ ] update `golden_test.go:110` and other tests that build `signal.ChangeHistory{}`
- [ ] run `make test` + `TestArchImports` + `go test -count=1 ... TestGolden` — pass before Task 12

### Task 12: Documentation

- [ ] update `internal/metrics/doc.go` and the `internal/model/signal` package comment to describe
      the per-family input model + typed producer carrier
- [ ] update any design doc that states "every metric receives the full `MetricInput`"
- [ ] run `go test ./...` — must pass before Task 13

### Task 13: Verify acceptance criteria + Phase B commit

- [ ] acceptance: same 13 metric names in the same order; `TestGolden` byte-identical; **no metric
      implementation receives all signals** (each takes one family input); `internal/metrics` root
      is no longer the LOC hotspot; **no new gate findings**
- [ ] `make fmt && make lint && make test && make build`
- [ ] `./.bin/archfit check --config .archfit.yaml --full` — verdict `pass`; re-baseline if an
      informational value shifted
- [ ] `detect_changes({scope:"compare", base_ref:"main"})`; commit Phase B

---

## Technical Details

### Phase A package layout & DAG

```
internal/metrics/
  metrics.go        Metric interface + New(cfg) []Metric            (root / registry facade)
  doc.go
  metrics_test.go   TestNew_ReturnsAllMetrics
  internal/result/  band/confidence model, computeDelta, naCount, shortModule, modularitySmallN
                    (nested under internal/ → importable only within internal/metrics/...)
  boundary/         encapsulation, unbalanced_edge, cycle, coverage, change_locality
  modularity/       blast_radius, change_amplification, hidden_coupling, structural_weight,
                    functional_candidates  (+ private module-graph/history helpers)
  risk/             risk_hub                (+ NewMetric(cfg))
  intramodule/      complexity, architecture_fitness
  metricstest/      BuildGraph, ImportKey, ApproxEqual  (test helpers)
```

DAG (acyclic): `root → {boundary, modularity, risk, intramodule} → internal/result → model/*`.
`result` never imports root; family packages satisfy `Metric`/`Calculator` **structurally** (no
import of root). Package membership and Phase-B input family are independent axes — e.g.
`blast_radius` is in the `modularity` package (shares `blastRadius`) but is a
`Calculator[CommonInput]` (needs only the graph).

### Phase B generic core

```go
// metrics root
type Metric interface { Name() string; Version() string; Calculate(signal.CollectedSignals) diagnostic.MetricResult }
type Calculator[In any] interface { Name() string; Version() string; Calculate(In) diagnostic.MetricResult }
type wrapped[In any] struct { c Calculator[In]; project func(signal.CollectedSignals) In }
func (w wrapped[In]) Name() string    { return w.c.Name() }
func (w wrapped[In]) Version() string { return w.c.Version() }
func (w wrapped[In]) Calculate(s signal.CollectedSignals) diagnostic.MetricResult { return w.c.Calculate(w.project(s)) }
func adapt[In any](c Calculator[In], project func(signal.CollectedSignals) In) Metric { return wrapped[In]{c, project} }

// New(): order preserved == golden order; mis-paired family/projection won't compile
adapt(boundary.CycleMetric{},               signal.CollectedSignals.AsCommon)
adapt(modularity.HiddenCouplingMetric{},    signal.CollectedSignals.AsHistory)
adapt(risk.NewMetric(cfg),                  signal.CollectedSignals.AsSymbol)
adapt(modularity.StructuralWeightMetric{},  signal.CollectedSignals.AsSize)
adapt(intramodule.ComplexityMetric{},       signal.CollectedSignals.AsComplexity)
adapt(intramodule.ArchitectureFitnessMetric{}, signal.CollectedSignals.AsFitness)
adapt(modularity.FunctionalCandidatesMetric{}, signal.CollectedSignals.AsDuplication)
```

Generics replace a 7-case type switch + runtime `default` error + guard test with one
compile-safe adapter. The metric's `Calculate` only ever sees its family input — the structural
guarantee. `CollectedSignals` is the engine's producer-side bag and never reaches metric logic.

### Producer carrier

`signal.ChangeHistory` is replaced by `signal.RunSignals` (the cmd-produced typed groups:
History, Size, Complexity, Fitness, Duplication, GitnexusImpact, ExtraCoverage). The engine
combines `RunSignals` with its extract outputs (`ex.g`, `ex.scipSymbols`, `ex.coverages`,
classifications, findings, baseline, changed files) into `CollectedSignals`. No god-struct remains
on either boundary.

### Metric → input family

| family (interface)             | metrics                                                                        |
| ------------------------------ | ------------------------------------------------------------------------------ |
| `Calculator[CommonInput]`      | encapsulation, unbalanced_edge, cycle, coverage, blast_radius, change_locality |
| `Calculator[HistoryInput]`     | change_amplification, hidden_coupling                                          |
| `Calculator[SizeInput]`        | structural_weight                                                              |
| `Calculator[SymbolInput]`      | risk_hub                                                                       |
| `Calculator[ComplexityInput]`  | complexity                                                                     |
| `Calculator[FitnessInput]`     | architecture_fitness                                                           |
| `Calculator[DuplicationInput]` | functional_candidates                                                          |

Notes: `ChangedFiles` stays in `CommonInput` (delta-scope, not git history) → `change_locality`
is a `CommonMetric`. `GitnexusImpact` lives in `SymbolSignals` (only `risk_hub` reads it). The
`Calculator[ComplexityInput]` alias and the `intramodule.ComplexityMetric` struct are in different
packages (no Go collision).

## Post-Completion

_Informational — no checkboxes._

- **Out of scope** (separate plans): review items 3–6 (model/config hub discipline,
  `hidden_coupling` triage, labels persistence split, CI self-check step).
- **Manual check**: skim the post-split `archfit check --full` report to confirm the metrics
  module no longer appears in `structural_weight` and that no new HIGH coupling findings appear.
