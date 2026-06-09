# archfit Hybrid — Tranche 1 (Deterministic) + Tranche 2 Outline

## Overview

Evolve archfit into a hybrid: a deterministic meta-linter (language tools +
external architecture linters) plus a **selective, off-gate** LLM layer. This plan
implements **Tranche 1 in full** (all deterministic features) and **outlines
Tranche 2** (the LLM layer), which is built only after a validation spike.

Source of truth: `docs/design/hybrid-llm-strength-v0.1.md`.

**Hard constraint (whole plan):** `check` — the CI gate — stays pure deterministic
and reproducible. No LLM call ever on the gate path. LLM lives only in `init`/`enrich`
and `explain`, cached. This preserves the baseline/delta verdict.

Tranche 1 features:

1. **Symbol-level impact ranking** (`risk_hub` metric) — surface the _risky_ hub
   (e.g. ccgram `window_state_store`) that module-level `blast_radius` misses.
2. **`architecture_fitness` self-measure** — detect whether architecture intent is
   actually enforced (arch tests / import-linter / archfit-in-CI).
3. **Socio-technical distance** — CODEOWNERS (git-author overlap fallback) refines
   Balanced-Coupling distance.
4. **Functional-coupling candidates** — clone detection + co-change produce a
   deterministic candidate list (no semantic label yet).
5. **gitnexus optional provider** — enrich symbol impact when enabled; SCIP-only otherwise.

## Context (from discovery)

- **Language/stack:** Go, kong CLI, hexagonal. Packages: `internal/model`,
  `internal/metrics`, `internal/extract`, `internal/engine`, `internal/config`,
  `internal/classify`, `internal/output`, `cmd/archfit`.
- **Metric contract** (`internal/metrics/metrics.go:47`):
  `Metric{ Name() string; Version() string; Calculate(MetricInput) diagnostic.MetricResult }`,
  registered as a slice in `metrics.New()` (`metrics.go:632`). 9 metrics today.
- **`MetricInput`** (`metrics.go:63`): `Graph`, `Classifications` (coupling.Index),
  `Findings`, `Baseline`, `ToolCoverage`, `FileChurn`, `CoChange`, `FileLOC`,
  `Complexity`. New inputs append here and via `ChangeHistory` (`metrics.go:55`).
- **Reusable pattern** (`internal/metrics/modularity.go`): `blastRadius()` reverse
  reachability + `tarjanSCC()` SCC condensation + `moduleChurn()`/`fileToModuleKey()`
  - `naCount()`/`naResult()` n/a helpers + `bandScore`/`applyConfidenceCap`.
- **SCIP** (`internal/extract/scip/`): `scip_strength.go` runs an indexer and the
  embedded `scip_reader.py` parses `.scip`, but **collapses symbol references to
  per-module-edge strength** (`readerOutput.Edges` = `{from,to,strength}` keyed by
  module path). `scip_reader.py` already walks `doc.occurrences` and classifies the
  four strength levels (`_is_private`→intrusive, Protocol/ABC→contract, type-ref→model,
  call-ref→functional). The symbol graph is parsed and discarded — Tranche 1 retains it.
- **Distance levels** already include `cross_module_different_owner` (driven by config
  `owner`); socio-technical distance extends the _owner source_, not the level model.
- **Volatility** (`internal/config/volatility.go`): falls back to churn when no
  `subdomain`/`volatility` is authored — the churn double-count risk for `risk_hub`.

## Development Approach

- **Testing approach: Regular** (code first, then table-driven stdlib tests; `moq`
  for tool/subprocess boundaries — matches archfit's existing metric/extractor tests).
- Complete each task fully before the next; small focused changes.
- **Every task includes new/updated tests** (success + error/edge), as separate
  checklist items. **All tests pass before the next task.**
- New metrics are **report-only (`band: info`) first** — they never gate until
  explicitly promoted, preserving the current verdict semantics.
- Maintain backward compatibility: absent tool/data → `n/a` (info), never a false zero.

## Testing Strategy

- **Unit tests:** required every task. Table-driven for metric input→output matrices;
  `moq`-generated fakes for `toolrun.Runner` and resolver ports (no real subprocess).
- **Python reader tests:** extend `internal/extract/scip/scip_reader_test.py` for the
  new symbol output (run via `uv`).
- **Acceptance (Task A5):** real-repo validation across 4 repos (ccgram, codegraph,
  pumba, spotinfo) — a gate, distinct from unit tests.
- **Determinism check (Task: Verify):** two `check` runs on the same commit/config
  produce identical diagnostics; grep the `check` path for any LLM dependency (must
  be none in Tranche 1).
- No e2e/UI tests (CLI project).

## Progress Tracking

- Mark completed items `[x]` immediately when done.
- New tasks: `➕` prefix. Blockers: `⚠️` prefix.
- Keep this file in sync; update scope if implementation deviates.

## Solution Overview

Tranche 1 adds four deterministic metrics/inputs and one optional provider, all
feeding the existing engine → metrics → renderer pipeline:

- **Symbol graph:** extend the SCIP reader to emit per-symbol fan-in + symbol→symbol
  reference edges; Go builds a `SymbolGraph`; `risk_hub` runs reverse-reachability and
  ranks by `symbol_impact × volatility` (explicit-subdomain volatility only).
- **Fitness signals:** a deterministic repo scan (arch tests, import-linter config,
  arch-linter in CI) computed in `cmd`/engine, passed as `MetricInput.FitnessSignals`.
- **Ownership:** a resolver (CODEOWNERS → git-author fallback) fills module `owner`
  gaps before `classify`, refining distance.
- **Functional candidates:** a clone-detection runner + existing co-change produce a
  report-only candidate list.
- **gitnexus:** optional provider that enriches symbol impact when enabled.

Metrics stay pure (operate on `MetricInput`); all I/O (tools, FS scans, git) happens
in `cmd`/engine and is handed in — consistent with the current architecture.

## Technical Details

- **New `MetricInput` fields:** `SymbolGraph` (symbol→module, symbol fan-in,
  symbol-ref adjacency), `FitnessSignals` (struct), `CloneClusters` ([]Cluster),
  `Ownership` (module→owner, for diagnostics; distance uses merged config owner).
- **New ports/providers:** extend SCIP adapter with `Symbols(ctx, scope)`; new
  `gitnexus` provider behind `tools.gitnexus.enabled`; new clone-detection runner
  behind `tools.clones.enabled`. All via `toolrun.Runner`, all opt-in/`auto` with
  graceful `n/a`.
- **New config keys:** `tools.gitnexus.enabled`, `tools.clones.enabled`, and
  `metrics.{risk_hub,architecture_fitness,functional_candidates}` entries.
- **risk_hub volatility:** read **explicit** `subdomain`/`volatility` config only;
  unset module → neutral `1.0`. Never use churn-derived volatility (that double-counts
  with `change_amplification`).

## What Goes Where

- **Implementation Steps** (`[ ]`): all code, tests, config, docs in this repo.
- **Post-Completion** (no checkboxes): the LLM spike (needs an LLM), Tranche 2 build,
  and per-repo manual validation notes.

## Implementation Steps

### Task 1: Config schema for new tools and metrics

**Files:**

- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `.archfit.yaml` (self-config: register new metric entries, default off)

- [x] add `Gitnexus` and `Clones` tool-mode entries under `tools` (enabled: on|off|auto, default off/auto)
- [x] accept `metrics.risk_hub`, `metrics.architecture_fitness`, `metrics.functional_candidates` entries (reuse existing `MetricEntry`)
- [x] keep unknown-key tolerance and defaults consistent with existing `tools.scip`/`tools.complexity`
- [x] write tests: load config with new keys (success), missing keys default correctly, invalid values rejected
- [x] run tests — must pass before Task 2

### Task 2: Extend SCIP reader to emit symbol fan-in + references

**Files:**

- Modify: `internal/extract/scip/scip_reader.py`
- Modify: `internal/extract/scip/scip_reader_test.py`

- [x] add a `symbols` array to the reader JSON: per internal definition symbol → `{symbol, path, module, fan_in}` where `fan_in` = count of distinct referencing documents (from the existing `doc.occurrences` walk)
- [x] add a `symbol_refs` array: `{from_symbol, to_symbol}` cross-module reference edges (reuse `_is_internal`/`_to_path` already present)
- [x] keep the existing `edges` (per-module strength) output unchanged — additive only
- [x] write Python tests for the new `symbols`/`symbol_refs` output on a small fixture index (success + empty-index + no-internal-symbols cases)
- [x] run reader tests via `uv` — must pass before Task 3

### Task 3: Parse symbol output into a Go SymbolGraph

**Files:**

- Create: `internal/extract/scip/symbols.go`
- Create: `internal/extract/scip/symbols_test.go`
- Modify: `internal/extract/scip/scip_strength.go` (extend `readerOutput` struct)

- [ ] extend `readerOutput` to unmarshal `symbols` + `symbol_refs`
- [ ] add `func (a *Adapter) Symbols(ctx, scope.Scope) (SymbolGraph, diagnostic.Coverage, error)` — runs the indexer+reader once, returns a `SymbolGraph{ Module map[string]string; FanIn map[string]int; Refs map[string]map[string]struct{} }`
- [ ] absent indexer/uv or any failure → empty graph + absent/partial coverage, never an error (match `Strengths` behavior)
- [ ] write tests with a `moq` `toolrun.Runner` returning canned reader JSON (success, absent tool, malformed output)
- [ ] run tests — must pass before Task 4

### Task 4: Wire SymbolGraph through engine into MetricInput

**Files:**

- Modify: `internal/metrics/metrics.go` (add `SymbolGraph` to `MetricInput` + `ChangeHistory`)
- Modify: `internal/engine/engine.go` (forward SymbolGraph)
- Modify: `internal/engine/ports.go` (resolver port gains `Symbols` or a new port)
- Modify: `cmd/archfit/main.go` (call `scip.Symbols` when `tools.scip.enabled: on`)
- Modify: `internal/engine/engine_test.go`

- [ ] add `SymbolGraph` field to `MetricInput` (and the engine carrier) with doc comment; empty when SCIP off/absent
- [ ] wire `cmd` to collect the symbol graph alongside existing strength hints (single indexer run if practical)
- [ ] thread it through the engine to `Calculate`
- [ ] write tests: engine forwards a populated SymbolGraph; empty when resolver absent
- [ ] run tests — must pass before Task 5

### Task 5: Implement risk_hub metric (symbol-level impact × volatility)

**Files:**

- Create: `internal/metrics/risk_hub.go`
- Create: `internal/metrics/risk_hub_test.go`
- Modify: `internal/metrics/metrics.go` (register `RiskHubMetric{}` in `New()`)

- [ ] implement `symbolImpact(SymbolGraph)` — reverse-reachability over `Refs` (mirror `blastRadius`/`tarjanSCC`; condense SCC so cycles don't inflate), returns per-symbol transitive dependents
- [ ] implement `RiskHubMetric.Calculate`: rank symbols by `impact × volatility`, where volatility comes **only** from explicit `subdomain`/`volatility` config (unset → `1.0`); aggregate to owning module for display; `band: info`
- [ ] `naResult` when `SymbolGraph` is empty (no SCIP) — never a false zero
- [ ] register in `metrics.New()`; ensure renderers tolerate the new metric (Task 14 verifies output)
- [ ] write tests: known symbol hub ranks top; cyclic refs don't inflate; churn does NOT affect score (distinct from `change_amplification`); n/a when no graph
- [ ] run tests — must pass before Task 6

### Task 6: Validate risk_hub across 4 repos (acceptance gate)

**Files:**

- Create: `docs/plans/notes/risk_hub-validation.md` (results log)

- [ ] run `archfit scan` (scip on) on ccgram → confirm `window_state_store` surfaces as a top risk_hub (not buried as in `blast_radius`)
- [ ] run on codegraph, pumba, spotinfo → confirm known hubs surface; record top-5 per repo
- [ ] compare against the architect reviews where available; note false positives/negatives
- [ ] ⚠️ if a known hub does NOT surface, adjust ranking (impact normalization / aggregation) and re-run before proceeding
- [ ] write the results log; mark gate pass/fail

### Task 7: Architecture-fitness signal collector

**Files:**

- Create: `internal/fitness/fitness.go`
- Create: `internal/fitness/fitness_test.go`

- [ ] implement `Detect(root string) Signals` — deterministic scan for: arch test files (e.g. `*import*cycle*`, `*arch*test*`, tests importing archfit/import-linter), import-linter config (`.importlinter`, `setup.cfg [importlinter]`, `pyproject.toml [tool.importlinter]`), arch-linter in CI (`.github/workflows/*` referencing `archfit`/`import-linter`/`deptry`/`dependency-cruiser`)
- [ ] return a `Signals` struct with booleans + the matched evidence paths (for explainability)
- [ ] write tests over fixture dirs: all-present, none-present, partial (table-driven)
- [ ] run tests — must pass before Task 8

### Task 8: architecture_fitness metric

**Files:**

- Create: `internal/metrics/arch_fitness.go`
- Create: `internal/metrics/arch_fitness_test.go`
- Modify: `internal/metrics/metrics.go` (`FitnessSignals` field on `MetricInput`; register metric)
- Modify: `cmd/archfit/main.go` (call `fitness.Detect`, pass into input)

- [ ] add `FitnessSignals` to `MetricInput`; wire `fitness.Detect` in `cmd`
- [ ] implement `ArchitectureFitnessMetric.Calculate` — score by count/weight of enforcement signals present; `band: info` first (report-only)
- [ ] include matched evidence paths in the display string
- [ ] write tests: no-enforcement → low, full-enforcement → high, partial → mid; n/a when scan unavailable
- [ ] run tests — must pass before Task 9

### Task 9: Ownership resolver (CODEOWNERS + git-author fallback)

**Files:**

- Create: `internal/ownership/ownership.go`
- Create: `internal/ownership/ownership_test.go`

- [ ] parse CODEOWNERS (`.github/CODEOWNERS`, `CODEOWNERS`, `docs/CODEOWNERS`) → glob→owner rules; match files to owners
- [ ] git-author fallback: dominant author per file from `git log --format` (reuse `internal/history/git` pass if practical) → module owner; used only when no CODEOWNERS
- [ ] aggregate file owners → module owner (via `fileToModuleKey`); return empty when neither source exists (no fabrication)
- [ ] write tests: CODEOWNERS precedence, git-author fallback, neither present → empty (use `moq`/fixtures, no real git in unit tests)
- [ ] run tests — must pass before Task 10

### Task 10: Feed resolved owners into Balanced-Coupling distance

**Files:**

- Modify: `cmd/archfit/main.go` (merge resolved owners into config module metadata before classify)
- Modify: `internal/config/config.go` (helper to fill missing `owner` from resolved map; config owner wins)
- Modify: `internal/classify/classify_test.go`

- [ ] merge resolved ownership into module metadata: explicit config `owner` wins; resolver fills gaps; unset → unchanged (distance stays as today)
- [ ] confirm `classify` raises distance to `cross_module_different_owner` for differing owners via the existing logic (no new distance level)
- [ ] write tests: differing resolved owners raise distance; config owner overrides resolver; no ownership → no distance change
- [ ] run tests — must pass before Task 11

### Task 11: Clone-detection runner

**Files:**

- Create: `internal/extract/clones/clones.go`
- Create: `internal/extract/clones/clones_test.go`

- [ ] implement a `toolrun`-based runner: detect/invoke a clone detector per language (jscpd for JS/TS, PMD-CPD for multi/Java/Go/Py where available); behind `tools.clones.enabled`
- [ ] parse output → `[]Cluster{ Files []string; Lines int }`; absent tool → empty + absent coverage, never error
- [ ] map clusters to module pairs via `fileToModuleKey`
- [ ] write tests with `moq` runner returning canned detector output (success, absent tool, malformed)
- [ ] run tests — must pass before Task 12

### Task 12: functional_candidates metric (report-only)

**Files:**

- Create: `internal/metrics/functional_candidates.go`
- Create: `internal/metrics/functional_candidates_test.go`
- Modify: `internal/metrics/metrics.go` (`CloneClusters` field; register metric)
- Modify: `cmd/archfit/main.go` (wire clones runner output)

- [ ] add `CloneClusters` to `MetricInput`; wire from the clones runner
- [ ] implement `FunctionalCandidatesMetric.Calculate` — count module pairs that share duplicated logic (clone clusters), cross-referenced with `CoChange`; `band: info`
- [ ] explicitly distinct from `hidden_coupling` (co-change-without-edge): this is duplication-based; note the distinction in the metric definition string
- [ ] write tests: clone clusters → candidate pairs; no clones → n/a; dedup pairs
- [ ] run tests — must pass before Task 13

### Task 13: gitnexus optional symbol-impact provider

**Files:**

- Create: `internal/extract/gitnexus/gitnexus.go`
- Create: `internal/extract/gitnexus/gitnexus_test.go`
- Modify: `cmd/archfit/main.go` (enrich SymbolGraph when `tools.gitnexus.enabled: on`)
- Modify: `internal/metrics/risk_hub.go` (consume enriched impact if present)

- [ ] implement a `toolrun`-based gitnexus provider: when enabled + present, query impact/fan-in to enrich the symbol-impact map; never auto
- [ ] precedence: gitnexus impact when available, else SCIP fan-in; record which source in coverage
- [ ] absent/disabled → SCIP-only, unchanged behavior
- [ ] write tests with `moq` runner (enabled+present enriches, disabled ignored, absent falls back)
- [ ] run tests — must pass before Task 14

### Task 14: Render new metrics in all outputs

**Files:**

- Modify: `internal/output/console/console.go`
- Modify: `internal/output/markdown/markdown.go`
- Modify: `internal/output/jsonout/jsonout.go` (usually automatic via struct)
- Modify: corresponding `_test.go` files

- [ ] confirm `risk_hub`, `architecture_fitness`, `functional_candidates` render with band/confidence/display in console + markdown
- [ ] ensure top-N truncation + `n/a` rendering match existing info metrics
- [ ] add `tool_coverage` rows for scip-symbols, clones, gitnexus
- [ ] write/Update renderer tests for the new metrics (present + n/a)
- [ ] run tests — must pass before Task 15

### Task 15: Verify acceptance criteria

- [ ] verify all 5 Tranche-1 features implemented and registered
- [ ] **determinism gate:** two `check` runs on the same commit/config produce byte-identical diagnostics; grep `check` path — confirm zero LLM dependencies in Tranche 1
- [ ] run full suite: `make test` (or `go test ./...`)
- [ ] run lint: `make lint` (golangci-lint) — clean
- [ ] run `archfit scan` on this repo and the 4 validation repos — no panics, sensible output
- [ ] confirm baseline/delta still works (new metrics report-only, do not change verdict)

### Task 16: Documentation

**Files:**

- Modify: `README.md`, `docs/design/hybrid-llm-strength-v0.1.md` (mark Tranche 1 status), `CLAUDE.md` (new metric/provider patterns)

- [ ] document the new metrics, config keys (`tools.gitnexus`, `tools.clones`), and SCIP-symbol/clone/gitnexus coverage
- [ ] update the design doc status to "Tranche 1 implemented; Tranche 2 spike-gated"
- [ ] note new patterns in CLAUDE.md (pure-metric + I/O-in-cmd, opt-in tool with n/a fallback)
- [ ] move this plan to `docs/plans/completed/` when Tranche 1 is done

## Spike (gate before Tranche 2 — Post-Completion)

Before any LLM code is written, run a cheap classification spike:

- LLM-classify ccgram cross-boundary edges (model-vs-functional refinement) and module
  subdomains (core/supporting/generic), then diff against the architect review's
  "Coupling review" section in `docs/architecture-review/2026-05-23-ccgram-full.md`.
- **Pass:** broad agreement on subdomain class and the unbalanced-relationship list →
  proceed to detail + build Tranche 2.
- **Fail:** the LLM tranche is re-thought (prompt, evidence packaging, or whether
  per-edge LLM labeling is worthwhile) before it is planned in detail.

## Tranche 2 — LLM layer (outline only; build after spike passes)

Off-gate, cached, provider-pluggable. Not detailed until the spike passes.

- **Provider interface:** thin Go `Classifier{ Classify(...) }` + `Explainer{ Explain(...) }`;
  impls `ollama` (local-first), `openai`, `anthropic`; selected via `tools.llm`.
  Results cached by content hash; **never invoked from `check`**.
  - **Library decision (perplexity-researched):** no multi-LLM framework. Use official
    SDKs — `openai-go` (also drives **Ollama** via its OpenAI-compatible `/v1` endpoint,
    base-URL swap) + `anthropic-sdk-go`. ~2 deps + stdlib. Rejected any-llm-go (no
    first-class Anthropic/Ollama), gollm (unmaintained pace), langchaingo (framework
    overkill). Re-verify SDK status at build time. See design §6.
- **`enrich` command:** LLM drafts `subdomain`/`volatility` and refines
  model-vs-functional labels into `.archfit.yaml`; human reviews and commits; the gate
  runs on the pinned config (LLM ran once, output version-controlled).
- **`explain` upgrade:** LLM narrative over already-collected evidence + ranked
  `risk_hub` hubs (the "why it hurts / cascading scenario / smallest fix" story).

## Post-Completion

_No checkboxes — external action or later tranche._

**LLM spike** (needs an LLM + the existing architect review): the gate above.

**Tranche 2 build:** provider interface, `enrich`, `explain` — after the spike passes.

**Manual validation:** per-repo review of risk_hub / functional_candidates output for
false positives; tune thresholds in config rather than code where possible.

**External tooling:** clone detectors (jscpd, PMD-CPD) and gitnexus are opt-in external
tools; document install via `archfit doctor`/`install` and degrade to `n/a` when absent.
