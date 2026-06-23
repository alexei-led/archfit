# Align archfit with the Balanced Coupling book (formula, anchors, LLM-as-judge)

## Overview

Make archfit's coupling scorer implement Vlad Khononov's _Balancing Coupling in
Software Design_ **verbatim** — the published formula and ordinal anchors — instead
of archfit's homegrown multiplicative scorer. The bespoke formula
(`risk = (1 − |Sₙ − Dₙ|) × Vₙ`, ordinals `0/2/3/5/8`) has no published theory behind
it: it is "somewhat similar but still different" and would need its own book to
justify. The book's model already comes with theory, worked examples, and
justification. Adopting it **removes** invented surface rather than adding more.

**Guiding razor:** the book owns the _formula_ and the _ordinal anchors_; archfit
owns only the _instrumentation_ — measuring code and placing each edge on the book's
scale (the tool the book explicitly asked for). Anywhere archfit's numbers deviate
from the book is either instrumentation (keep) or an invented mini-formula (delete).

**The book's per-edge model (Ch10), MIN=1, MAX=10:**

```
modularity = |S − D|                          // 0..9  (high gap = modular)
balance    = max(modularity, MAX − V) + MIN   // 1..10 (higher = better balanced)
```

- **Strength** anchors: Contract=1, Model=3, Functional=8, **Symmetric=9**, Intrusive=10.
- **Distance** anchors (book): same object=1, same package=2, diff packages=3–7,
  diff libraries=8, services=9, diff vendors=10.
- **Volatility** anchors (book): frozen/legacy=1, supporting/generic=3, core=10.
- The book has **no "unknown"** level. Where archfit cannot place an edge on the
  scale, it must **abstain** (exclude the edge from scoring and lower confidence) —
  never fake an ordinal.

**Judgment inputs (book assumes a human judges these):** strength of the
model-vs-functional middle and all currently-"unknown" edges; subdomain/volatility of
undeclared modules. For each such input the design is:

> deterministic measurement where possible → otherwise a **human-editable surface**
> (config / labels) is the source of truth → an **optional LLM draft** populates that
> surface for human approval → if neither LLM nor human supplies a value, the scorer
> **abstains and emits an actionable decision task** telling the human what to decide.

LLM stays strictly **off-gate** (only `enrich`/`review`/`explain`/`autopilot` touch
it; `check` consumes only deterministic, human-approved, pinned facts). This is
already archfit's architecture — we extend it, we do not breach it.

### Problem it solves

- archfit under-detects the worst BC anti-pattern (distributed monolith ≈ 6/10 today;
  book says 1/10 = worst) because it lacks the Symmetric level and its functional
  ordinal sits mid-scale.
- `coupling_balance` is stuck at `60/mixed/low` on well-structured repos because the
  per-edge balance archfit already computes is discarded; a clean repo is
  indistinguishable from an unanalyzed one.
- archfit's volatility lacks the book's Ch9 inferred-volatility cascade and the
  frozen-legacy anchor; runtime (async) coupling does not affect distance per Ch8.
- The scorer carries a bespoke formula that needs its own justification.

## Context (from discovery)

Files/components involved (verified against current code):

- `internal/model/coupling/coupling.go` — `Strength`/`Distance`/`Volatility` consts,
  `Classification`, `Index`, `Severity`, `EdgeScore` location.
- `internal/model/coupling/scorer.go` — `strengthOrdinal*`/`distanceOrdinal*` consts,
  `ScoreVersion = "bc_score.v2"`, `EdgeScore`, `ScoreBreakdown`.
- `internal/model/coupling/scorer_multiplicative.go` (+ `scorer_additive.go`) — the
  combination logic to replace.
- `internal/classify/classify.go` — `classifyStrength` (3-tier: config globs →
  approved pinned label → SCIP hint), `classifyDistance` (composite + `DistanceBasis`),
  `classifyVolatility` (explicit → subdomain → path heuristic → undeclared),
  `classifyConnascence` (clone pair → `ConnascenceAlgorithm`). Strength is computed
  **before** connascence — needs reordering for Symmetric.
- `internal/score/score.go` — `Synthesize`, `couplingBalance` (zero-edge → 60/mixed/low),
  `bcEdges` (reads only `bc/imbalanced_coupling` findings), `boundaryIntegrity`,
  `finalize` (low-confidence cap at 60).
- `internal/model/diagnostic/diagnostic.go` — `Diagnostic`; carries only post-filter
  findings, no classified-edge distribution. (stdlib-only by `arch_test`.)
- `internal/engine/engine.go`, `internal/engine/advisories.go` — `classify.Run`
  produces `coupling.Index`; `collectAdvisories` applies `BCAdvisoryMinSeverity`.
- `internal/labels/labels.go` + `internal/labels/labelsio` — `Label{From,To,Strength,
Rationale,EvidenceHash,Status}` (no confidence/provenance); `Approved()` filters.
- `cmd/archfit/enrich.go`, `enrich_values.go` — LLM drafts strength (per edge),
  volatility, owner; subdomain draft is deterministic today. draft → approve → pin.
- `cmd/archfit/pipeline.go` — `buildCoverageGaps` (suppresses disabled primary tools
  only; no project-marker check).
- `internal/config/modules.go` — `ModuleDef{Subdomain,Volatility,Owner,DeployUnit,Role,…}`.
- `Makefile` — `make calibrate` (Go-only), structural gates.

Related patterns found: draft→approve→pin→deterministic-consume (enrich); scorer-variant
seam (additive/multiplicative); composite distance with `DistanceBasis`; `agent_tasks`

- coverage gaps for actionable human follow-up.

Dependencies / constraints (from `internal/arch_test.go` + CLAUDE.md):

- `internal/score` is in the **core ring**: no `os`/`os/exec`/YAML/adapter imports. It
  already imports `internal/model/coupling` and `internal/model/diagnostic` (fine).
- `internal/model/*` is **stdlib-only** — `ClassifiedEdgeSummary` must be primitive counts.
- No `internal/*` may import `internal/llm` — LLM extensions live in `cmd` only.
- Golden output (`internal/engine` `TestGolden`) changes on every scoring change —
  regenerate deliberately and inspect; never automatic.
- Keep green: `make test`, `make lint`, `TestArchImports`, `make archfit`, byte-identical
  double-run.

### Non-goals — explicitly rejected (do NOT implement)

- **Award `boundary_integrity` points for having passing gate rules** (recommendation
  Gap 2). Rewards writing rules, not boundary health; no book basis. The honest lift
  for `boundary_integrity` is making encapsulation measurable via real strength
  classification (Tasks 3/6/7), not crediting rule presence.
- **Blanket `internal/` import → intrusive** (Gap 3b). Go's compiler already enforces
  the `internal/` visibility rule; within one module there are no illegal internal
  imports and archfit lives entirely under `internal/`. This would mislabel the
  standard layout. archfit's intrusive signal correctly comes from per-module config
  `internal:` globs and SCIP private resolution.
- **Role-aware god-module exemption** (Gap 6). Exempting `composition_root`/`generated`
  modules from the size-based god-module flag raises archfit's own score and is
  off-theme for book fidelity. Keep the scorer honest. (Revisit separately if desired.)

## Development Approach

- **Testing approach**: Regular (code, then tests) — the codebase is golden/regression
  driven; targeted unit tests encode book-justified expected values so golden regen is
  validated, not rubber-stamped.
- Complete each task fully before the next. Small, focused changes.
- **CRITICAL: every task includes new/updated tests** (success + failure/edge).
- **CRITICAL: all gates pass before starting the next task** — `make test`, `make lint`,
  `TestArchImports`; for any task that changes scoring, **regenerate goldens and inspect
  the diff against book-justified expectations** within that task.
- Maintain backward compatibility of the JSON schema (additive fields; consumers ignore
  unknown fields). Bump `ScoreVersion` once (Task 1) to `bc_score.v3`.

## Testing Strategy

- **Unit tests**: required every task. Scorer tasks reproduce the book's Ch10 worked
  examples and four corners as table-driven cases.
- **Regression**: extend `internal/engine` regression tests (small-OSS BC patterns) and
  add a dedicated **book-examples regression test** that encodes Ch10 examples 1–4.
- **Determinism**: byte-identical double-run verified in the final verification task.
- **No e2e/UI** in this repo.

## Progress Tracking

- Mark completed items `[x]` immediately. Add discovered tasks with ➕. Blockers with ⚠️.
- Keep this file in sync with actual work.

## What Goes Where

- **Implementation Steps** (`[ ]`): code, tests, golden regen, doc updates in-repo.
- **Post-Completion** (no checkboxes): external validation on third-party repos,
  human calibration review, release tagging.

## Implementation Steps

### Task 1: Replace the scorer with the book's formula + ordinal anchors

- [x] In `internal/model/coupling/coupling.go` add `StrengthSymmetric Strength = "symmetric"`.
- [x] In `internal/model/coupling/scorer.go` set book ordinal anchors: strength
      `Contract=1, Model=3, Functional=8, Symmetric=9, Intrusive=10`; distance
      `same_module=2, cross_module_same_owner=4, cross_module_diff_owner=7,
cross_deploy_unit=9`; volatility `frozen=1, supporting=3, generic=3, core=10`,
      explicit override `low=3, medium=6, high=10` (document `medium=6` as an
      interpolation on the book's acknowledged-open 3–10 range, not a new formula).
- [x] Replace the combination in the active scorer with the book model:
      `modularity = |S − D|`, `balance = max(modularity, 10 − V) + 1` (range 1..10,
      higher = better). Store book `balance` on `EdgeScore` (keep a derived severity
      band: balance 1–2 → critical, 3–4 → high, 5–6 → medium, 7–8 → low, 9–10 → none —
      tune bands against examples in tests). Bump `ScoreVersion` to `"bc_score.v3"`.
- [x] Introduce explicit **abstain**: when strength or distance is `unknown`, the edge
      is **unscored** (return a sentinel / `Scored=false` on `EdgeScore`), not assigned
      a faked ordinal. Remove `strengthOrdinalUnknown`/`distanceOrdinalUnknown` invented
      values. Volatility `undeclared` → abstain on the volatility rescue term only
      (treat as "cannot confirm low volatility").
- [x] Delete (or clearly mark deprecated and unused) the bespoke multiplicative/additive
      combination so only the book scorer remains the source of truth.
- [x] Write table-driven tests reproducing **book Ch10 examples**: distributed monolith
      (Symmetric 9 / cross_deploy 9 / core 10 → balance **1**), frozen legacy (Intrusive
      10 / cross_deploy 9 / frozen 1 → **10**), model-over-distance (Model 3 / cross_deploy
      9 / core 10 → **8**), transactional cohesion (Functional 8 / same_module 2 / core 10
      → **7**); plus four corners (cohesion, loose, ball-of-mud, distributed monolith);
      plus abstain cases (unknown strength/distance → unscored).
- [x] Regenerate goldens, inspect diff against expectations; run `make test`, `make lint`,
      `TestArchImports` — must pass before Task 2.

### Task 2: Surface the per-edge book-balance distribution into `coupling_balance`

- [x] Add stdlib-only `ClassifiedEdgeSummary` to `internal/model/diagnostic/diagnostic.go`:
      counts `ByStrength`/`ByDistance`/`ByVolatility`/`BySeverity`, `Total`, `Scored`,
      `Abstained`, and an aggregate `MeanBalance` (or the balance histogram) over scored
      cross-boundary edges. Add `Diagnostic.ClassifiedEdges *ClassifiedEdgeSummary`.
- [x] In `internal/engine/engine.go`, after `classify.Run`, populate the summary from the
      full `coupling.Index` (a pure read; skip `same_module` and abstained edges from the
      balance aggregate but count them). Keep `collectAdvisories`/`BCAdvisoryMinSeverity`
      unchanged — advisories stay noise-controlled; the **dimension** sees everything.
- [x] In `internal/score/score.go`, pass the summary into `Synthesize` and rewrite
      `couplingBalance` to value from the distribution: `value = round(100 × (MeanBalance
− 1) / 9)` (a transparent linear rescale of the book's own 1–10 score — not a new
      model); confidence scales with scored fraction (high ≥80% scored, medium 50–79%,
      low <50% or unknown-heavy); **zero scored edges → keep `60/mixed/low` (unanalyzed)**;
      worst edges remain surfaced via existing advisories.
- [x] Write tests: all-balanced repo with N≥50 scored edges → high value + high
      confidence (and distinct from the zero-edge unanalyzed case); unknown-heavy repo →
      lower confidence; zero edges → 60/low; verify advisory tail still flags the worst
      edges independently of the mean.
- [x] Regenerate goldens + inspect; run gates — must pass before Task 3.

### Task 3: Assign the Symmetric strength level from clone detection

- [x] In `internal/classify/classify.go` reorder so clone-pair membership
      (`CrossModuleClonePairs` → `ConnascenceAlgorithm`) is known before strength is
      finalized (compute connascence first, or post-adjust strength).
- [x] When a cross-module clone pair exists for an edge and the deterministic strength is
      `functional` or `unknown`, upgrade strength to `StrengthSymmetric` (book level 9 =
      duplicated functionality / DRY violation). Do not override `contract`/`intrusive`
      (config-authoritative) or an approved pinned label.
- [x] Write tests: clone pair between modules → `StrengthSymmetric`; no clone → unchanged;
      config glob / pinned label still wins; integration — duplicated logic across deploy
      units yields balance ≈ 1 (distributed monolith now detected).
- [x] Regenerate goldens + inspect; run gates — must pass before Task 4.

### Task 4: Runtime coupling → distance (book Ch8) via the AsyncBridge signal

- [x] Per the book (async integration increases lifecycle independence = higher effective
      distance), wire the existing `Classification.AsyncBridge` signal into distance:
      an async edge raises effective distance by one book anchor (capped at the
      cross_deploy/services anchor). This replaces the dead AsyncBridge infra (previously
      annotated but never consumed by the scorer) with book-faithful behavior.
- [x] If, on inspection, async detection is too unreliable to drive scoring, instead
      **delete** the dead AsyncBridge annotation path rather than leave inert infra —
      decide in-task and record which. **Decision: do not wire.** `Classification.AsyncBridge`
      was never implemented as an edge-level field. `RuntimeAsync` operates at module
      granularity (not edge granularity), is explicitly report-only by contract test
      `TestRun_RuntimeAsync_StaticGraphUnchanged`, and is confidence=low|medium. No edge
      annotation path exists to delete — it was never built, which is correct. Status quo kept.
- [x] Write tests: async edge gets the higher distance anchor and a correspondingly
      higher balance (looser lifecycle); sync edge unchanged; capped at max anchor.
      (skipped — no implementation; existing TestRun_RuntimeAsync_StaticGraphUnchanged
      encodes the correct contract that async signal does not affect scoring)
- [x] Regenerate goldens + inspect; run gates — must pass before Task 5.
      (no code changes; all gates pass: make test, make lint, TestArchImports green)

### Task 5: Inferred-volatility cascade (book Ch9)

- [x] In `internal/classify`, after base module volatility is assigned, add a deterministic
      propagation pass: a module strongly coupled (strength ≥ functional) to a
      high-volatility module inherits raised effective volatility (book: "high integration
      strength with volatile components makes it highly volatile as well"). Bound the
      propagation (single hop or fixpoint with an explicit cap) and make it config-gateable.
- [x] Edges consume the to-module's **effective** (post-cascade) volatility.
- [x] Write tests reproducing the book's inferred-volatility example: a supporting module
      (would be low) strongly coupled to a core module scores as high-volatility;
      weak coupling to a volatile module does **not** propagate; cascade is deterministic
      and terminates.
- [x] Regenerate goldens + inspect; run gates — must pass before Task 6.

### Task 6: Human-editable judgment surfaces + abstain-to-decision (the human fallback)

- [x] Add `Confidence` (e.g. `high|medium|low`) and `Provenance` (`human|llm|tool`) to the
      `Label` record (`internal/labels/labels.go`) and to the owner/volatility/subdomain
      value-draft records; round-trip through `labelsio`. Keep schema backward-compatible.
- [x] Make the gate treat judgment inputs honestly: a low-confidence or unreviewed
      (`status: draft`) label is **not** consumed by `check` (only `approved` is, as
      today); an `approved` label of `provenance: llm` lowers the affected dimension's
      confidence by one band unless confirmed `human`. Config-declared values
      (subdomain/volatility/owner/globs) are the authoritative human surface.
- [x] When a judgment input is **undeclared and undrafted** (unknown strength edge with no
      label; module with no subdomain/volatility), the scorer abstains AND archfit emits
      an actionable item (coverage gap / `agent_task`): "declare subdomain for module X" /
      "classify strength for edge A→B" — so a human knows exactly what to decide and where
      to edit (`.archfit.yaml` / `.archfit-labels.yaml`).
- [x] Write tests: confidence/provenance round-trip; draft labels ignored by gate; llm
      provenance lowers dimension confidence; undeclared inputs abstain and emit a decision
      item pointing at the right file/key.
- [x] Run gates (this task may not change goldens; regen only if scores move) — must pass
      before Task 7.

### Task 7: LLM-as-judge extensions in `enrich` (off-gate)

- [x] Broaden strength enrichment: `selectRefinablePairs` currently requires SCIP and
      targets model/functional. Extend it to also target `unknown`-strength cross-module
      edges (where the book most needs human/LLM judgment), still emitting drafts to
      `.archfit-labels.yaml` with `provenance: llm`, `confidence`, and `EvidenceHash`.
- [x] Add an **LLM subdomain draft** path (today `--subdomains` is deterministic): an
      opt-in LLM draft proposing `core|supporting|generic` per module with rationale,
      written to `.archfit-subdomains.yaml` for human approval + `--pin`. Keep the
      deterministic classifier as the default/fallback. (Already implemented: `--subdomains`
      uses the LLM via `runSubdomainDraft`; `--pin` applies approved entries; covered by
      `enrich_subdomains_test.go`.)
- [x] Keep everything off-gate: changes live in `cmd/archfit` only; no `internal/*`
      imports `internal/llm` (verified by `TestArchImports`). Approval remains mandatory
      before `check` consumes anything.
- [x] Write tests with a fake `llm.Provider`: unknown edges selected for strength enrich;
      drafts carry provenance/confidence; subdomain LLM draft → file → pin into
      `.archfit.yaml`; no SCIP required for the unknown-edge path.
- [x] Run gates — must pass before Task 8.

### Task 8: Noise suppression + reliability/efficiency

- [x] In `cmd/archfit/pipeline.go` `buildCoverageGaps`, suppress tool-coverage-gap noise
      for a language whose project marker is absent (e.g. no `Cargo.toml` → no
      cargo/cargo-modules gap) in addition to the existing disabled-primary-tool
      suppression; do not suppress when the marker is present. Keep `StatusDisabled`
      distinct from `StatusAbsent`.
- [x] Fix the O(E·M) owner-map rebuild in `classifyDistance` (rebuild the owner index once,
      O(M+E), not per-edge) — behavior-preserving.
- [x] Write tests: pure-Go repo (no Cargo.toml) reports no cargo gaps; mixed Go+Rust repo
      with Cargo.toml still does; owner-map fix preserves identical distance classification
      on a fixture with multiple owners.
- [x] Run gates — must pass before Task 9.

### Task 9: Verify acceptance criteria, goldens, determinism, dogfood

- [ ] Add a dedicated **book-examples regression test** (`internal/engine` or
      `internal/model/coupling`) encoding Ch10 examples 1–4 with their exact balance
      values, as the durable proof archfit matches the book.
- [ ] Review the cumulative golden diff deliberately; confirm every changed value is
      explained by a book-justified reason; commit regenerated goldens.
- [ ] Verify determinism: byte-identical double-run of `check`/`scan` on the repo.
- [ ] Run the full gate set: `make test`, `make lint`, `TestArchImports`, `TestGolden`,
      `make archfit` (dogfood). All green.
- [ ] Confirm acceptance: distributed monolith scores ≈1; frozen legacy ≈10; a
      well-structured repo's `coupling_balance` reads high/high (not `60/mixed/low`) and
      is distinct from an unanalyzed repo; `ScoreVersion == bc_score.v3`; no LLM import
      reachable from `internal/*`.

### Task 10: [Final] Documentation

- [ ] Rewrite `docs/design/bc-measurement-v2.md` (or supersede with a `-v3`): state that
      archfit implements Khononov's published formula and ordinal anchors verbatim;
      document the instrumentation mapping (measured signal → book anchor), the abstain
      rule, and the LLM-as-judge-off-gate + human-editable-fallback design.
- [ ] Update `docs/guide/configuration-reference.md` (new label `confidence`/`provenance`
      fields; frozen/legacy volatility; LLM subdomain draft; abstain → decision tasks),
      `concepts.md`, and `release-notes.md`.
- [ ] Update `CLAUDE.md` with the new patterns (book-verbatim scorer, abstain-not-fake,
      provenance-lowers-confidence, async→distance, volatility cascade) and the bumped
      `ScoreVersion`.

_Note: ralphex automatically moves completed plans to `docs/plans/completed/`._

## Technical Details

### The book formula (authoritative — do not deviate)

```
S, D, V = book ordinals for the edge (abstain if S or D unknown)
modularity = |S − D|                         # 0..9
balance    = max(modularity, 10 − V) + 1     # 1..10, higher = better
```

### Instrumentation mapping (archfit signal → book anchor)

- Strength: contract glob / SCIP interface → 1; SCIP/struct data model or pinned `model`
  → 3; call edge / pinned `functional` → 8; **clone pair → symmetric 9**; internal glob /
  SCIP private / pinned `intrusive` → 10; otherwise **unknown → abstain**.
- Distance: same_module → 2; cross_module_same_owner → 4; cross_module_diff_owner → 7;
  cross_deploy_unit → 9; async bridge → +1 anchor (cap 10); unknown → abstain.
- Volatility: subdomain core → 10; supporting/generic → 3; explicit frozen/legacy → 1;
  explicit low/medium/high → 3/6/10; undeclared → abstain rescue term + emit decision task.
- Repo rollup (`coupling_balance` dimension): `round(100 × (mean book balance − 1)/9)` over
  scored cross-boundary edges; confidence from scored fraction; worst edges via advisories.
  (Transparent aggregation of the book's own per-edge score, not a new coupling model.)

### Validation anchors (must reproduce)

| Case                    | S   | D   | V   | book balance |
| ----------------------- | --- | --- | --- | ------------ |
| Distributed monolith    | 9   | 9   | 10  | **1**        |
| Frozen legacy intrusive | 10  | 9   | 1   | **10**       |
| Model over distance     | 3   | 9   | 10  | **8**        |
| Transactional cohesion  | 8   | 2   | 10  | **7**        |
| Loose coupling          | 1   | 9   | 10  | **9**        |
| Ball of mud             | 3   | 2   | 10  | **2**        |

## Post-Completion

_No checkboxes — external/human actions._

**Calibration & validation (human-in-the-loop):**

- Run the book scorer on external repos (e.g. herdr, a TS api+web deploy-unit repo, a
  Python DDD repo) and manually confirm band agreement with expert judgment on a sample;
  the book stresses the scale is judgment-assisted, so this is human validation, not
  auto-tuning. Adjust only the instrumentation mapping (signal→anchor), never the formula.

**External system updates:**

- Consuming configs (`.archfit.yaml`) gain optional frozen/legacy volatility and richer
  labels; communicate the `ScoreVersion` bump (`bc_score.v3`) in release notes — scores
  shift by design.

**Release:**

- Tag `vX.Y.Z` only after PR merge to protected `main` (tag-triggered release workflow).
