# Architecture Review: archfit

**Reviewer:** Independent (blind to self-scan output and arch-comparison tooling)
**Date:** 2026-06-20
**Target:** `github.com/alexei-led/archfit` @ HEAD (8243a08)
**Language:** Go (~214 .go files, 5174 lines in cmd/, ~8000 lines in internal/)
**Methodology:** Balanced Coupling (Khononov), hexagonal/ports-and-adapters fitness check, executable fitness-test verification

---

## 1. System Map

### Ring Layers (innermost → outermost)

```
model/*          stdlib-only value types
  └─ config      YAML parse + view projections (imports os + goccy/yaml + model/coupling)
       └─ core ring: classify, rules, metrics/*, status, staleness, facts, scope, score
            └─ ports       hexagonal port interfaces (imports config + model/*)
            └─ engine      orchestrator (imports core ring + ports + config + labels)
                 └─ adapters: toolrun, extract/*, history/git, output/*, labels/labelsio, fitness, baseline, calibrate, ownership
                      └─ cmd/archfit   composition root (kong CLI)
```

### Packages

| Layer | Packages |
|-------|----------|
| model | graph, finding, coupling, diagnostic, symbol, clone, pattern, signal |
| config | config (single package) |
| core ring | classify, rules, metrics, metrics/{boundary,modularity,risk,intramodule,internal/result}, status, staleness, facts, scope, score |
| ports | ports (interfaces: Extractor, PatternProvider, SymbolResolver, Renderer + Nop impls) |
| engine | engine |
| adapters | toolrun, extract/{go,ts,py,astgrep,clones,complexity,deployunit,dynimports,gitnexus,loc,scip}, history/git, output/{console,jsonout,markdown,sarif,scorecard}, labels, labels/labelsio, fitness, baseline, calibrate, ownership, agenttask, initcfg, llm |
| cmd | cmd/archfit (main + check, baseline, calibrate, score, enrich, explain, review, init, install, doctor, update, pipeline) |

### Dependency Direction (verified via `go list`)

```
model/* → (stdlib only)
config → model/coupling, goccy/go-yaml, doublestar, os
core ring → config, model/*
ports → config, model/*
engine → core ring + ports + config + labels
adapters → toolrun + model/* + config (never engine)
cmd → everything (composition root)
```

No import cycles detected (`go build ./...` exits 0).

### Entrypoints

`cmd/archfit/main.go` → `kong` CLI dispatcher → subcommands:
- `check` — full analysis pipeline
- `baseline` — snapshot current metrics
- `calibrate` — tune thresholds
- `score` — standalone scorecard
- `enrich` / `explain` — LLM-augmented (off-gate; only these touch internal/llm)
- `review` — LLM-assisted review
- `init`, `install`, `doctor`, `update` — tooling lifecycle

All check-path subcommands route through `cmd/archfit/pipeline.go:runPipeline()`.

---

## 2. Evidence with file:line Citations

### Import graph facts

- `internal/ports/ports.go:11` — `import "github.com/alexei-led/archfit/internal/config"`
- `internal/engine/engine.go:13-29` — engine imports classify, config, facts, labels, metrics, model/*, ports, rules, scope, staleness, status (20 imports, no LLM, no os/exec)
- `internal/toolrun/toolrun.go:1-8` — package declares `Runner` interface AND `ToolRunner` concrete type in the same package; imports `os/exec`
- `internal/config/config.go:14-16` — imports `os`, `goccy/go-yaml`, `doublestar` — NOT stdlib-only
- `cmd/archfit/pipeline.go:13-47` — 25 internal package imports in the composition root
- `internal/arch_test.go:~80` — `allowed` list for core ring explicitly includes `internal/config`
- `internal/arch_test.go:adapters_no_engine_import` — checks that extract/*, history/*, output/* do NOT import engine; does NOT check whether they import toolrun directly

### Test results

All 30+ packages `ok` (0 `FAIL`). The arch fitness test (`go test ./internal/ -run TestArchImports`) uses `golang.org/x/tools/go/packages` for real import-graph traversal — not string matching, not static analysis of comments, not a linter config.

### config transitive path into core

```
classify → config → os (file I/O), goccy/go-yaml
rules → config → os, goccy/go-yaml
metrics/* → config → os, goccy/go-yaml
scope → config → os, goccy/go-yaml
status → config → os, goccy/go-yaml
```

The arch test forbids `os` and `os/exec` as **direct** imports in core ring packages (`isForbiddenForCore` checks `imp == "os"`). It does not check transitive imports. Because `config` is explicitly on the allowed list, core ring packages can reach `os` and `goccy/go-yaml` one hop away via config.

### Runner interface placement

```
internal/ports/ports.go   → Extractor, PatternProvider, SymbolResolver, Renderer  ✓
internal/toolrun/toolrun.go → Runner interface + ToolRunner concrete implementation  ✗ (not in ports)
```

Adapters that use subprocess execution (`extract/golang`, `extract/ts`, `extract/py`, `extract/astgrep`, `extract/clones`, `extract/gitnexus`, `history/git`) must import `internal/toolrun` directly — they depend on the concrete package, not a neutral interface package.

Evidence: `internal/extract/astgrep/astgrep.go` (and all other extract packages) import `internal/toolrun`.

### cmd single-package size

```
5174 lines total across cmd/archfit/
  570  enrich.go
  391  review.go
  338  update.go
  303  pipeline.go
  237  init.go
  142  main.go
  136  check.go
```

Single Go package (`package main`) for all CLI commands. No internal subdivision.

### fitness package gap

`internal/fitness/fitness.go` imports `os` directly (filesystem scan) and is NOT listed in `adapterPrefixes` in arch_test.go. This is architecturally correct — fitness is a leaf adapter — but the test does not verify it cannot import engine. Low risk today; would become a risk if someone accidentally imported engine from fitness.

---

## 3. Balanced Coupling Analysis

### Coupling classification framework

Per Khononov: **Balance Rule** = (STRENGTH XOR DISTANCE) OR NOT VOLATILITY.
Unbalanced = high strength AND high distance AND high volatility simultaneously.

### Significant Couplings

#### C1: core ring → config (classify, rules, metrics, scope, status → internal/config)

| Dimension | Assessment |
|-----------|-----------|
| Integration strength | Functional — core packages call `config.ClassifyConfig`, `config.RulesConfig`, etc. by value |
| Distance | Low — one hop inward within the same module, adjacent in ring hierarchy |
| Volatility | Medium — config schema changes affect all importers |

**Balance:** Strength=functional, Distance=low → balanced by low distance. Not flagged.
**Latent risk:** config is treated as "pure view types" but it also owns YAML parsing and `os` file I/O. If config evolves to add more I/O behavior, the core ring's purity degrades without a test catching it (transitive imports not checked).

#### C2: ports → config (internal/ports/ports.go:11)

| Dimension | Assessment |
|-----------|-----------|
| Integration strength | Model — config view types appear in port method signatures (e.g. `PatternConfig` in `PatternProvider.Find`) |
| Distance | Low — ports is a neutral layer, config is one level inward |
| Volatility | Medium |

**Balance:** Model strength + low distance → balanced. Accepted design choice.
**Consequence:** Every adapter implementing a port must also import config. A change to `config.PatternConfig` (e.g., adding a field) propagates to `PatternProvider.Find` signature and all adapters simultaneously. This is a high-fan-out change surface for a `model`-strength coupling — note-worthy but not critical.

#### C3: adapters → toolrun (extract/*, history/git → internal/toolrun)

| Dimension | Assessment |
|-----------|-----------|
| Integration strength | Functional — adapters call `toolrun.Runner.Run()` and `toolrun.Runner.Detect()`; they depend on `toolrun.ToolCmd`, `toolrun.Output` concrete types |
| Distance | Low — within the adapter ring |
| Volatility | Low — subprocess execution contract is stable |

**Balance:** Functional strength + low distance + low volatility → balanced by all three dimensions.
**Architectural concern:** The `Runner` interface is co-located with `ToolRunner` concrete implementation in `toolrun` package. This violates the hexagonal principle that interfaces should be owned by the consumer or defined in a neutral package. Currently `Extractor`, `PatternProvider`, `SymbolResolver`, and `Renderer` are properly in `ports`; `Runner` is not. This is an **asymmetry in the hexagonal model** that the arch test does not catch. Balanced coupling-wise: acceptable (low distance, low volatility). Architecturally: inconsistent with the stated pattern.

#### C4: cmd/archfit → all internal packages (25+ imports in pipeline.go)

| Dimension | Assessment |
|-----------|-----------|
| Integration strength | Intrusive — cmd constructs concrete types from every adapter package |
| Distance | Maximum — outermost shell to all inner rings |
| Volatility | Expected high — composition root changes with every new feature |

**Balance:** High strength + max distance + high volatility → technically unbalanced by Balanced Coupling rule. However, this is the **intentional composition root** pattern in hexagonal architecture. cmd's sole responsibility is wiring — it is the only place where concrete types are instantiated. This is the correct design; the apparent imbalance is a known property of composition roots, not a defect.
**Risk:** cmd is a single `package main` with no internal subdivision. At 5174 lines with 12+ subcommand files, it is manageable now but will become harder to navigate as commands grow.

#### C5: engine → labels (internal/engine imports internal/labels, not labels/labelsio)

| Dimension | Assessment |
|-----------|-----------|
| Integration strength | Functional — engine uses `labels.Label` and pure matching logic |
| Distance | Low |
| Volatility | Low — labels is pure (stdlib only: crypto/sha256, encoding/hex, sort) |

**Balance:** Balanced (functional + low distance + low volatility). Correctly split: `labels` (pure) vs `labels/labelsio` (YAML I/O, cmd-only). The arch test enforces `labelsio` is unreachable from internal.

---

## 4. Hexagonal / Ports-and-Adapters Fitness

**Verdict: ~85% faithful hexagonal architecture.**

### What holds

- `internal/ports` defines `Extractor`, `PatternProvider`, `SymbolResolver`, `Renderer` interfaces — all adapters implement these, never importing engine.
- `arch_test.go:adapters_no_engine_import` enforces the boundary at CI time with real package graph traversal.
- `arch_test.go:llm_ring_unreachable_from_internal` — LLM is off-gate; only `cmd` can import `internal/llm`. This is structurally enforced, not just a convention.
- `arch_test.go:labelsio_unreachable_from_internal` — the YAML I/O adapter for labels is cmd-only.
- Nop implementations in ports (`NopPatternProvider`, `NopSymbolResolver`) mean the engine works without all tools present — graceful degradation built into the port layer.

### What does not hold

- `toolrun.Runner` interface is defined inside the `toolrun` concrete package, not in `ports`. Adapters depend on `internal/toolrun`, not a neutral interface. This is an asymmetry: three of the four major port interfaces are in `ports`, but the subprocess execution port is not.
- `config` has dual identity: it owns YAML parsing (I/O concern) AND is used as shared types by core ring and ports (pure domain concern). A cleaner separation would extract `config/views` (pure types) from `config` (parse + validate). The current design accepts this to avoid a third shared-types package.

### Import ring fitness test: real or cosmetic?

**Real.** Evidence:
1. Uses `golang.org/x/tools/go/packages` with `NeedName|NeedImports` mode — actual compiler package graph, not string matching.
2. Sub-tests are individually named and independently fail.
3. The `inCoreRing` prefix matcher has its own unit test (`TestInCoreRing`) covering edge cases like `internal/metricsx` not matching `internal/metrics`.
4. CI YAML runs `go test ./internal/ -run TestArchImports` explicitly as a named gate.

**Gap:** The test checks only **direct** imports, not transitive. `config`'s transitive reach into `os` and `goccy/go-yaml` via core ring packages is not caught. This is a known design acceptance, not an oversight, but it means the "core ring has no os dependency" guarantee is weaker than stated — it's "no direct os dependency."

---

## 5. Banded Scorecard

| Dimension | Score | Band | Confidence | Key Evidence |
|-----------|-------|------|-----------|-------------|
| modularity / cohesion | 8/10 | Good | High | 8 model packages (stdlib-only); metrics split into 5 families; clean adapter subdirectories. Minor: config straddles model/parse concerns. |
| coupling_balance | 7/10 | Good | High | Hexagonal ports pattern holds for 4/5 major boundaries. Runner interface in toolrun (not ports) is the asymmetry. Config→core fan-out is accepted but watch volatility. |
| dependency_direction / cycles | 9/10 | Excellent | High | Strict inward flow verified via `go list`. Zero cycles. LLM isolation enforced by test. labelsio isolation enforced by test. |
| encapsulation | 8/10 | Good | High | `internal/` Go module boundary hides all implementation. Public surface is cmd binary only. No exported packages for external consumers. |
| blast_radius / change_locality | 7/10 | Good | Medium | config changes fan out to 8+ core packages + ports. Runner changes fan out to all 11 extract adapters + history/git. New extractor = only pipeline.go change. |
| testability | 8/10 | Good | High | All 30+ packages pass. moq fakes for 3 port interfaces. Core ring is pure-functional (no I/O). Engine uses injected fakes. cmd has integration tests. |
| architecture_fitness | 9/10 | Excellent | High | arch_test.go is executable AST-level enforcement (not prose/linter config). 5 sub-tests cover critical boundaries. Self-referential: archfit enforces its own ring rules on itself. |

**Overall band: Good (8.0/10 average)**

---

## 6. Top Findings

### F1 — Runner interface not in ports (coupling_balance, architecture_fitness)

**File:** `internal/toolrun/toolrun.go` (Runner interface at line ~40); `internal/ports/ports.go` (Runner absent)
**BC dimension:** Structural asymmetry — 3 of 4 major ports are in `ports`, subprocess execution port is not.
**Impact:** All extract adapters and history/git import `internal/toolrun` (concrete package). A change to `toolrun.ToolCmd` or `toolrun.Output` propagates to all 12 adapters. The arch test's `adapters_no_engine_import` does not detect if an adapter imports toolrun unnecessarily.
**Fix:** Move `Runner` interface (and supporting types `ToolCmd`, `Output`, `ToolInfo`) to `internal/ports`. Keep `ToolRunner` concrete impl in `toolrun`. Add `adapters_no_toolrun_direct` sub-test to arch_test.go (adapters should import ports.Runner, not toolrun.Runner).

### F2 — config is dual-identity: parse + shared-types (modularity/cohesion, blast_radius)

**File:** `internal/config/config.go:14-16` (imports `os`, `goccy/go-yaml`, `doublestar`)
**BC dimension:** config is classified as "allowed" in core ring but also does I/O — this makes the "core ring is pure" guarantee transitive-only.
**Impact:** Arch test direct-import check does not catch that `classify → config → os`. The guarantee holds for direct imports but not for transitive ones.
**Options:** (a) Accept and document as a known tradeoff. (b) Split `config/views` (pure types) from `config` (parse). Option (a) is reasonable given the stability of config's I/O behavior.

### F3 — cmd/archfit is a single large package (modularity/cohesion)

**File:** `cmd/archfit/pipeline.go:1-303`; all cmd files total 5174 lines in `package main`
**BC dimension:** The composition root handles wiring, flag parsing, output formatting, and domain-specific command logic (enrich, review, update) in one flat package.
**Impact:** `enrich.go` (570 lines) and `review.go` (391 lines) contain substantial business logic that could be in internal packages, tested independently of the CLI. Currently testable only via `main_test.go` integration tests.
**Severity:** Moderate. Manageable now; track growth. `enrich.go`'s subdomain-classification logic would benefit from extraction into `internal/` with its own unit tests.

### F4 — fitness package not in adapterPrefixes (architecture_fitness gap)

**File:** `internal/arch_test.go:adapterPrefixes`; `internal/fitness/fitness.go:8-12` (imports `os`)
**BC dimension:** Minor coverage gap in the fitness test.
**Impact:** If someone accidentally imports `internal/engine` from `internal/fitness`, the arch test won't catch it. Low risk today (fitness is a simple leaf scanner).
**Fix:** Add `modulePrefix + "internal/fitness"` to `adapterPrefixes` in arch_test.go.

### F5 — ports imports config (creating config→adapters transitive coupling) (coupling_balance)

**File:** `internal/ports/ports.go:11`
**BC dimension:** Config types appear in port method signatures (`config.PatternConfig` in `PatternProvider.Find`). Every adapter implementing a port becomes transitively coupled to config's full type set.
**Impact:** A field added to `config.PatternConfig` requires updating `PatternProvider.Find` signature AND all implementing adapters. This is a moderate fan-out for a model-strength coupling.
**Options:** (a) Accept: config is stable and this is a deliberate design choice. (b) Introduce `ports.PatternQuery` as an independent parameter type, decoupling ports from config. Option (a) is pragmatic for a single-binary tool.

---

## 7. Biggest Structural Risk

**Runner interface in toolrun (F1) is the highest-impact structural gap.** Because `toolrun.Runner`, `toolrun.ToolCmd`, and `toolrun.Output` are the shared contract used by all 12 extract adapters and history/git, any change to this contract is a module-wide coordinated change. Unlike the other three port interfaces (Extractor, PatternProvider, SymbolResolver) — which are in `ports` and therefore protected by `adapters_no_engine_import` — the Runner contract has no arch test protection. A developer can add a new adapter that imports `toolrun` for reasons other than the Runner interface (e.g., ToolInfo) without any test catching the coupling.

Second risk: `config`'s dual identity means the "core ring is I/O-free" property is contingent on `config` never adding more stateful behavior. If a future developer adds caching, HTTP calls, or mutable state to `config`, all core ring packages inherit that behavior silently. The arch test only checks direct imports.

---

## 8. Coverage Gaps and Low-Confidence Areas

- **Transitive import analysis:** go tools used here check direct imports only. A full transitive dependency graph (e.g., via `goda`) would give higher confidence on the "core ring is pure" claim.
- **Test coverage percentage:** Tests all pass (`ok` for 30+ packages) but per-package coverage percentages were not collected. The `go test -coverprofile` run was initiated but output not captured per-package. High confidence on pass/fail; medium confidence on coverage depth within individual packages.
- **cmd/archfit integration tests:** `main_test.go` and `pipeline_test.go` exist but their scope (unit vs. end-to-end) was not fully reviewed. High-level structure visible; inner test quality not assessed.
- **LLM package internals:** `internal/llm` was not read. Its isolation from the check gate is enforced and verified; its internal design is not assessed.
- **gitnexus/codegraph index:** No `.gitnexus/` or `.codegraph/` index was available. Change-coupling (GitNexus-style) analysis was not performed — volatility assessments above are based on structural role, not commit-frequency data.

---

## Summary

archfit has a well-executed, genuinely enforced hexagonal architecture. The import ring fitness test is real — using `golang.org/x/tools/go/packages` for AST-level graph traversal, not prose or linter rules. The LLM off-gate and labelsio isolation are structurally guaranteed, not just conventional. The dependency direction (model→config→core→ports→engine→adapters→cmd) is clean and cycle-free.

The main structural gap is `toolrun.Runner` not being in `ports` — an asymmetry the arch test does not cover. The second concern is `config`'s dual role as both I/O owner and shared-types provider for the core ring, which makes the "core ring is pure" guarantee direct-import-only rather than truly transitive. Both are known design tradeoffs rather than accidents, but only the first poses meaningful blast-radius risk as the codebase grows.

**Overall: Good (8.0/10).** Architecture fitness enforcement is the standout strength. The hexagonal boundary holds for the domain logic; the subprocess execution boundary has a small gap.
