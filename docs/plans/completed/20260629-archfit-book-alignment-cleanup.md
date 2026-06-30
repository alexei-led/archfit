# archfit book-alignment cleanup — executable plan

Status: **READY FOR EXECUTION** — decisions D1–D5 accepted; plan-reviewed and
revised (7 priority + 3 minor fixes folded in). Execute in a fresh session.

## For a fresh session (self-contained context)

- **Repo / branch:** `/Users/alexei/Workspace/archfit`, branch `feat/analyze-cli`.
- **Prior commits this branch (already landed, green):** `b06a0f5` (config-schema
  redesign → languages/analyzers/ai/coupling/waivers), `48bdedc` (removed the
  syntax-role feature). This plan is the next unit of work.
- **Method that worked twice already:** green at every phase (`go build ./...` +
  `go test ./... -short`), regenerate goldens deliberately, finish with a repo-wide
  **orphan grep** as the no-debt gate (no verification workflow is queued).
- **Decisions D1–D5 are LOCKED** (below). Do not re-litigate; execute.
- **Execution mode:** phases are sequential (each must compile + pass before the
  next). Within a phase, independent metric/extractor deletions can fan out in
  parallel, but every phase ends on a single green build+test gate.

## Goal

Make archfit follow Khononov's _Balancing Coupling in Software Design_ closely and
apply KISS: keep the book's coupling judgment + standard structural architecture
rules + a **minimal** set of industry-recognized complementary metrics; remove
invented composite scores, metrics already covered by linters, and churn-based
metrics that contradict the book. Reduce external-tool dependency as a consequence.
Leave **no** technical debt (no orphaned signals/extractors/tools/docs).

## Validation basis

- **Book (Balanced Coupling):** the model is a _judgment_ — strength × distance ×
  volatility → balanced/unbalanced — plus DDD-subdomain volatility. It is **not** a
  6-dimension 0–100 report card. Volatility is **not** derived from git history.
- **Perplexity (industry/academia):** cycles, layering, encapsulation/public-API,
  blast radius/fan-in-out, Martin I/A/D, LCOM, propagation cost, temporal/co-change
  are recognized architecture/modularity metrics. Cyclomatic complexity, panic/
  unsafe/global-state/test density, god-struct field counts, clone/duplication,
  deprecated-dep counts are **code-quality/linter** concerns. Composite 0–100
  "architecture health" scores are an **anti-pattern** (false precision,
  non-actionable — Fowler, Martin, ISO 25010, SonarQube-rating critiques). Churn is
  a valid _independent diagnostic_ but **not** a volatility proxy.
- **Advisor + plan-review:** classification sound; the corrections below are folded
  into the phases.

## Safety invariants (hold for every phase)

1. **`coupling_balance` stays.** It feeds finding severity (`cl.Score.Band`,
   `classify.go`) — the single severity source. Do not touch the scorer formula
   (`ScoreVersion = "bc_score.v3"`) or the strength/distance/volatility ordinals.
2. **Exit code / gate verdict is unchanged; the report Band/headline WILL change —
   intentionally.** The gate exit code (0/1/2/3) comes from gate findings
   (`diag.Verdict`) and is unchanged — `make archfit` stays `PASS · 0 blocking`.
   BUT `internal/decision/decision.go` `decideBand()` reads `sc.OverallBand`, so
   collapsing 6 dimensions → 1 changes the top-level report Band/headline
   ("healthy" vs "acceptable…"). This is **expected and accepted** (the score is
   now `coupling_balance`-driven). Do **not** add a `decideBand()` rework to
   preserve the old Band; the changed headline is the desired outcome. (If a stable
   Band were ever required, that would be a separate decision.)
3. **SCIP is NOT removed.** It feeds `coupling_balance` strength hints
   (`StrengthHint` → `classify.enrichEdges`) for TS/Py/Rust. Only its _metric_
   consumers (`risk_hub`, `cohesion_lcom`) and the `symbol_dependants` derivation go.
4. **ast-grep (`sg`) STAYS** (D5 accepted — keep the encapsulation rules).
5. **Green at every phase:** `go build ./...` + `go test ./... -short` pass after
   each phase. After Phase 2 and Phase 3, explicitly run
   `go test ./internal/ -run TestArchImports` (the import-ring structural gate).
   `make all` + dogfood at the end.
6. **No orphans (the no-debt gate):** Phase 8 repo-wide grep for every removed
   identifier/metric-name/config-key/tool/doc-term returns clean.

---

## Decisions (ALL ACCEPTED — locked)

- **D1 — Scorecard: shrink to book (option A).** Dismantle the composite
  6-dimension 0–100 scorecard. Keep `coupling_balance` as the scored dimension;
  headline = `coupling_balance` band + structural-rule pass/fail verdict + per-module
  coupling findings + coarse counts (cycles, forbidden-dep violations, unbalanced
  seams). No 0–100 index.
- **D2 — Complementary metrics: minimal.** Keep `cycle`, `blast_radius`,
  `encapsulation` (report-only). **Cut** Martin `instability`/`abstractness`/
  `martin_distance` and `propagation_cost` (strict KISS — accepted).
- **D3 — Churn: cut all three** (`change_amplification`, `hidden_coupling`,
  `change_coupling`).
- **D4 — SCIP: keep** as opt-in strength enricher; remove only `risk_hub`,
  `cohesion_lcom`, and the `symbol_dependants` fact/field.
- **D5 — ast-grep: keep the encapsulation rules** (`public_api_change`,
  `public_api_type_leak`, `public_api_max`) → `sg` stays opt-in. **Cut** the
  code-quality syntax rules `struct_field_max` and `test_in_production`.

### Why the score is dropped but the drift-gate is kept (feedback-loop intent)

The numeric score was wanted as a machine-actionable gate for CI/agent loops. That
need is met better by deterministic primitives archfit already has, which this
cleanup MUST preserve as first-class:

1. **Verdict + exit codes** (`0` pass / `1` fail / `2` warn / `3` error) — accept/decline.
2. **Baseline + `--base <ref>` delta** — gate on _new_ violations only (a change
   that adds a forbidden dep, a cycle, or a new unbalanced seam beyond
   `.archfit-baseline.json` fails). This is the drift gate.
3. **`coupling_balance` band** (balanced/mixed/unbalanced) per edge/module.
4. **Machine-readable JSON / SARIF / `agent_tasks`.**

Acceptance: the loop `archfit analyze --base <ref> → exit 1 + JSON on a new
violation` works end-to-end after the cleanup.

---

## Finalized classification

### KEEP — book + structural canon + minimal complement

| Item                                                          | Kind                 | Why                                                          |
| ------------------------------------------------------------- | -------------------- | ------------------------------------------------------------ |
| `coupling_balance` (+ `unbalanced_edge`)                      | metric / scored dim  | THE book judgment (S×D×V)                                    |
| `forbidden_dependency`, `forbidden_layer_direction`           | rule                 | structural canon                                             |
| `public_api_only`, `internal_api_access`                      | rule                 | encapsulation boundary (graph+glob, no ast-grep)             |
| `new_cross_module_dependency`                                 | rule                 | structural drift, baseline-aware                             |
| `cycle`                                                       | rule + metric        | structural canon                                             |
| `blast_radius`, `encapsulation`                               | metric (report-only) | recognized; support the book (D2)                            |
| `public_api_change`, `public_api_type_leak`, `public_api_max` | rule (ast-grep)      | encapsulation / API-boundary coupling (D5 keep → `sg` stays) |

### CUT — invented composites / meta (D1)

`boundary_integrity`, `dependency_graph_health`, `architecture_fitness`,
`analysis_confidence`, `risk_hub`, `cohesion_modularity` (it aggregates
`structural_weight`/`hidden_coupling`/`functional_candidates` — all cut, so it is
cut, not redefined), `change_locality`. → collapses the 6-dim scorecard to 1.

### CUT — code-quality / linter (Perplexity-confirmed)

metrics: `complexity`, `panic_density`, `unsafe_density`, `global_state_density`,
`test_density`, `struct_field_density`, `functional_candidates`, `structural_weight`,
`file_structural_weight`, `file_mutual_import`, `deprecated_dep_count`.
rules: `struct_field_max`, `test_in_production` (D5).

### CUT — churn (book-contra, D3)

`change_amplification`, `hidden_coupling`, `change_coupling`.

### CUT — beyond-book complement (D2)

`instability`, `abstractness`, `martin_distance`, `propagation_cost`.

### CUT — dead / invented

`cohesion_lcom` (already disabled, failed its eval — delete, don't carry);
`layer_role_divergence` rule (invented).

---

## Phase 0 — dependency map + sign-off (no code change)

Verify and complete the `metric → signal family → extractor → external tool` map
(read `internal/metrics/metrics.go`, `internal/model/signal/signal.go`):

| metric/dim                                                                     | signal family                                     | extractor                                    | external tool                     |
| ------------------------------------------------------------------------------ | ------------------------------------------------- | -------------------------------------------- | --------------------------------- |
| change_amplification, hidden_coupling, change_coupling                         | `AsHistory`                                       | `internal/history/git` `History()`/`Churn()` | git log/blame                     |
| complexity                                                                     | `AsComplexity`                                    | `internal/extract/complexity`                | **gocyclo, lizard**               |
| functional_candidates                                                          | `AsDuplication` (carries `History` co-change too) | `internal/extract/clones`                    | **jscpd**                         |
| risk_hub, cohesion_lcom                                                        | `AsSymbol`                                        | `internal/extract/scip`                      | SCIP indexers (KEEP for strength) |
| architecture_fitness                                                           | `AsFitness`                                       | `internal/fitness`                           | none (reads CI/Makefile)          |
| structural_weight, panic_density, file_structural_weight, struct_field_density | `AsSize`/`AsCommon`                               | `internal/extract/loc`                       | none (in-process)                 |
| `facts.Build` SCIP file-facts (report-only)                                    | `FileLOC`                                         | `internal/extract/loc`                       | none — **loc SURVIVES with SCIP** |
| coupling_balance, cycle, blast_radius, encapsulation                           | `AsCommon`                                        | go/packages, dep-cruiser, grimp, cargo       | (core — KEEP)                     |

**Notes (from plan-review):** `loc` is **not** orphaned — `facts.Build` (engine.go
~289) consumes `FileLOC` for SCIP file-facts even after all size-metrics are cut, so
`internal/extract/loc` stays. `internal/history/git/git.go` holds BOTH churn
(`History`/`Churn`) AND scope helpers (`RepoRoot`/`HeadRef`/`Changed`) used by
`pipeline_run.go` + `worktree.go` — so remove only the churn functions, keep the
package.

**Acceptance:** map confirmed; the four pipeline_run.go wiring points located:
`git.History` (~166), `fitness.Detect` (~181), `complexity.Run` (~245),
`clones.Run` (~258).

## Phase 1 — scorecard reduction (D1)

Edit targets: `internal/score/` (reduce `dims` in `score.go` to `couplingBalance`
only; delete the 5 dead dimension functions/files + their `Dim*` constants:
`DimBoundaryIntegrity`, `DimDependencyGraphHealth`, `DimCohesionModularity`,
`DimChangeLocality`, `DimArchitectureFitness`, `DimAnalysisConfidence`);
`internal/decision/decision.go` (it references removed dims; `decideBand()` Band
change is expected per invariant #2 — do not preserve old Band);
**`cmd/archfit/llmreview.go`** (the `validDimNames` map ~228–236 hardcodes all 6
deleted Dim constants — update it); console/markdown/json/sarif/scorecard renderers.
**Verify:** `go build ./...`; `go test ./internal/score/ ./internal/decision/ ./internal/output/... -short`.

## Phase 2 — remove cut metrics

Delete cut metric calculators under `internal/metrics/**` + their `New()` entries in
`internal/metrics/metrics.go` + their tests. Keep the registry/`adapt` machinery.
**`internal/arch_test.go`**: remove `internal/metrics/intramodule` and
`internal/metrics/risk` from `coreRingPkgs` (~lines 27–28) — they are deleted, and
`TestArchImports` will fail to compile otherwise. Update `internal/metrics` pkg doc.
**Verify:** `go build ./...`; `go test ./internal/metrics/... ./internal/ -run TestArchImports -short`.

## Phase 2b — remove cut RULES + their config references (ATOMIC — one commit)

Unknown rule `type` is a hard config error, and `.archfit.yaml` wires
`arch_struct_field_max` as a **fail gate** — so the rule code and its config
references must be removed in the **same** commit or every config-load (incl.
`go test`) breaks. Remove: in `internal/rules/rules.go` the `struct_field_max`,
`test_in_production`, `layer_role_divergence` switch cases + their `defaultGateForType`
entries; the `structFieldMax`/`layerRoleDivergence` impls (`rules_api.go`/`rules.go`)

- `testInProduction` (`rules_syntax.go`); AND the matching rule entries in
  `.archfit.yaml` (`arch_struct_field_max`) + any `examples/*.archfit.yaml` + the 6
  external configs (working-tree). Delete the corresponding rule tests.
  **Verify:** `archfit analyze --config .archfit.yaml` parses + gates; `go test ./internal/rules/... -short`.

## Phase 3 — remove orphaned signals + extractors

Remove now-consumerless signal families from `internal/model/signal/signal.go`
(`AsHistory`, `AsComplexity`, `AsDuplication`, `AsFitness`; **keep `AsSymbol`** — SCIP
strength; **keep `AsSize`/loc**). Delete packages `internal/extract/complexity`,
`internal/extract/clones`, `internal/fitness`. In `internal/history/git/git.go`
remove **only** `History()` + `Churn()` (keep `RepoRoot`/`HeadRef`/`Changed` — used by
`pipeline_run.go`/`worktree.go`). Remove the 4 wiring points in
`cmd/archfit/pipeline_run.go` (`git.History`, `fitness.Detect`, `complexity.Run`,
`clones.Run`) + any `internal/engine` wiring.
**Verify:** `go build ./...`; `go test ./... ./internal/ -run TestArchImports -short`.

## Phase 4 — drop now-unused external tools

Tools dropped: **gocyclo, lizard, jscpd** (+ git-history churn reader). SCIP and
`sg` STAY. Remove them from `cmd/archfit/registry.go`, `consts.go`,
`pipeline_coverage.go` (coverage gaps + install hints), `cmd/archfit/doctor.go`
(tool checks), `.github`/CI workflow if referenced, and the `analyzers.complexity`/
`analyzers.clones` config keys. (Dockerfile does **not** install
gocyclo/lizard/jscpd — no Dockerfile change for those; instead update docs:
`docs/guide/tooling.md`, `docs/guide/configuration-reference.md`.)
**Verify:** `go build ./...`; `.bin/archfit doctor` runs; `go test ./cmd/... -short`.

## Phase 5 — config, self-config, examples (remaining keys)

Remove dropped `analyzers.*` keys + dropped `metrics.*` entries from the config
structs (`internal/config`), `.archfit.yaml`, `examples/*.archfit.yaml`, the
`initcfg` generator, and the 6 external repo configs (working-tree only). (Stale
`metrics.<name>` map entries are harmless — unknown metric names don't error — but
remove for cleanliness.) Update config validation for removed analyzer keys.
**Verify:** config tests; `archfit analyze --config .archfit.yaml` parses.

## Phase 6 — docs / skills / spec / CLAUDE.md

Update `docs/guide/{configuration-reference,metrics,tooling,concepts,commands}.md`,
`docs/design/*`, `docs/spec/arch-fitness-spec-v0.4.md`, `skills/archfit/*`,
`CLAUDE.md` (Invariants: metric list, tool list, scorecard description). State
plainly: archfit measures Balanced-Coupling + structural rules; code-quality
(complexity, duplication, density) is delegated to linters by design.
**Verify:** Phase 8 doc grep.

## Phase 7 — regenerate schema + goldens + baselines

`make schema` (config structs changed). Regenerate: `internal/engine/testdata`
golden; the two `internal/extract/golang/testdata/*/baseline.json`; **and the
root `.archfit-baseline.json` dogfood baseline** (it carries entries for cut
metrics — regenerate via the baseline-update path and confirm whether stale entries
error or are ignored). **Eyeball every diff** — it must be ONLY removed
metric/dimension/field/baseline entries, never a verdict flip.
**Verify:** `go test ./internal/engine/ -run TestGolden`; byteidentical tests.

## Phase 8 — full verification + no-debt acceptance

1. `make all` green (fmt, lint, race tests, dogfood).
2. `make archfit` dogfood = `PASS · 0 blocking` (same exit verdict; report Band may
   differ per invariant #2 — expected).
3. **Orphan grep clean** (only intentional "delegated to linters" doc mentions remain):
   `grep -rInE 'risk_hub|architecture_fitness|analysis_confidence|boundary_integrity|dependency_graph_health|cohesion_modularity|cohesion_lcom|change_locality|change_amplification|hidden_coupling|change_coupling|functional_candidates|complexity|panic_density|unsafe_density|global_state_density|test_density|struct_field_(max|density)|structural_weight|file_structural_weight|file_mutual_import|deprecated_dep_count|layer_role_divergence|test_in_production|propagation_cost|martin_distance|instability|abstractness|gocyclo|lizard|jscpd' cmd/ internal/ docs/guide docs/design docs/spec skills/ examples/ CLAUDE.md .archfit.yaml archfit.schema.json`
   (note: `complexity` will also hit `analyzers.complexity` removal sites and any
   cyclomatic prose — review hits, don't blindly expect zero.)
4. `go vet ./...` clean; no unused exports (revive/unused).
5. JSON-schema no-drift test passes.

## Commit strategy

One commit per phase (Phase 2b is its own atomic commit), each green, on
`feat/analyze-cli`, `refactor!:` prefix. Final commit summarizes the keep-set and
the dropped tools (gocyclo, lizard, jscpd, git-churn reader).

## Out of scope / follow-ups

- The `archfit check`/`scan` leftovers in spec + release-notes (separate
  command-rename propagation from commit `4cdbc0f`).
