# Design: coupling-measurement correctness + modularity

Status: approved (scope confirmed by user 2026-06-27) · review-driven remediation
Source: `reports/architecture-review-2026-06-27.md` (Findings F1–F5)
Implements on branch `fix/coupling-measurement-modularity`.

## Follow-up — F2 shipped via accurate strength hint (branch `fix/coupling-accurate-strength`)

F2 was held in the first PR because the SCIP-on hint was coarse. **Resolved:**
`enrichEdges` no longer lets SCIP overwrite a Go edge's compiler-grade `go/types`
hint (SCIP-go collapses imports to a blanket `functional`); SCIP stays the
strength source for TS/Py/Rust and for Go edges type-info left unresolved. The
glob-floor + hint-kind refinement (Contract 2) is re-applied on top.

Result on archfit-self (SCIP on): `by_strength` now diverse —
`contract:8, functional:154, model:177, symmetric:3` (model coupling — shared
concrete domain types like `Diagnostic`/`Graph`/`Finding` — is now visible, not
flattened). `coupling_balance` reads **36/poor, 139 critical edges**: model
coupling (s=3) into the high-volatility core (v=10) at d=4 → balance 2, the book's
verbatim complexity-zone verdict. This is honest and corroborated (blast_radius
model/\* 59–71%) — NOT a calibration artifact: the same 36/poor holds with or
without SCIP once the hint is accurate.

### Editorial fix — distributed-monolith label is now distance-gated

The "design improvement" request (wrap `Diagnostic` in a contract) was rejected as
cargo-cult: the 177 model edges are mostly imports of shared VALUE TYPES
(`MetricResult`/`Coverage`/`Finding`), not `Diagnostic` sharing; an interface over a
data aggregate is an anti-pattern; the output contract already exists
(`SchemaVersion`); and model coupling at low distance is high cohesion (BC's good
quadrant). The real, in-scope improvement was **dogfooding**: archfit's OWN advisory
layer was issuing that cargo-cult advice — labeling every critical-band edge
"distributed-monolith risk → introduce a contract," when per the book (and
`score_boundary_coupling.go`'s own comment) a distributed monolith is high strength ×
high **distance** × high volatility.

Fix (editorial only — the balance value/ordinals are untouched, so the honest 36
stands): a new `ClassifiedEdgeSummary.DistributedMonolith` counts critical edges
that are ALSO high distance (`coupling.DistanceIsHigh` = different owner / deploy
unit). The score summary, the value cap, and the per-edge advisory
(`bcRiskClause`) now name "distributed-monolith risk" only for those; critical
edges at `cross_module_same_owner` are framed as local high-strength/high-volatility
coupling whose cascade is cheap.

archfit-self result: **139 critical → 2 distributed-monolith** (e.g.
`extract/loc → model/diagnostic`: functional × `cross_module_different_owner`
[code-structure basis] × high). Those 2 are genuine high-strength × high-distance ×
high-volatility — the book's worst case — and there a contract genuinely helps
(`cheapest_move: reduce_strength`). The other 137 no longer carry the false label.
Value stays 36.

Open refinement (not done; maintainer's call): the 2 DM use a **code-structure**
distance basis, not a real deployment/ownership boundary. "Distributed monolith"
strictly means separately-deployed-but-coupled, which a single-binary sole-owner
repo cannot be. Gating DM on `DistanceBasis ∈ {deploy_unit, ownership}` (not the
code-structure proxy) would report 0 DM for archfit and reframe the 2 as "tight
coupling across an architectural boundary." Left as a follow-up — code structure is
a legitimate distance component per the book, so both readings are defensible.

## source_inputs

- Architecture review (this repo, 2026-06-27): F1 catch-all module shadowing,
  F2 strength-invariant coupling_balance, F5 god files.
- Verified empirically: removing the catch-all stanza moves coupling_balance
  78→40 and surfaces 257 high-volatility edges; layer rule is currently inert
  (0 findings) because every path resolves to the catch-all (`support` layer).
- Drift note: the self-config comment at `.archfit.yaml:394` documents intent
  (specific stanzas win) that the code does not implement. This design makes the
  code match the documented intent.

## domain_map (unchanged)

archfit's module subdomains/volatility are already declared and correct
(`.archfit.yaml`): `model/*` core/high, extractors supporting, `cmd` generic.
No domain re-classification — the bug is that resolution never _reaches_ those
declarations. No labels need human re-confirmation; this restores existing ones.

## Contract 1 — path→module resolution (F1, foundational)

**Problem.** Two independent resolvers — `config.ModuleMap.ModuleFor`
(modules.go:113, used by rules/layer/staleness) and
`classify.moduleIndex.moduleFor` (classify.go:111, used by coupling) — both
return the **first** glob match in `sort.Strings` order. A catch-all
(`internal/**`) sorts before and matches everything, shadowing every specific
module. Fragmented-resolver class of bug.

**Target contract.** Path→module resolution is **most-specific-match**: among all
modules whose path globs match, the one whose matching pattern has the longest
literal prefix (segments before the first wildcard) wins; ties break by
alphabetical module name (deterministic). Exact paths beat globs.

**Design.** One implementation. `config.ModuleMap.ModuleFor` becomes
most-specific via a pure helper `globSpecificity(pattern) int` (index of first
`* ? [ {`, else len). `classify.moduleIndex` **delegates** resolution to an
embedded `config.ModuleMap` (built with `config.BuildModuleMap`), removing the
duplicate logic. `classify` already imports `config`; no new dependency, core
ring intact. `staleness` map-review matcher is `warn`-gate and lower priority —
aligned in the same pass if cheap, else noted.

**Consequence (intended).** Specific modules resolve correctly; `model/*` edges
carry `volatility: high`; coupling_balance reports its honest ~40; the catch-all
absorbs only unmatched root files (`internal/arch_test.go`) — exactly the
documented intent. **The `layer_inversion` gate stops being inert** and begins
checking real layer ranks (see Risks).

## module_map delta (F5 — archfit's own cohesion)

Behavior-preserving file splits within existing packages (no API/boundary
change):

- `internal/rules/rules.go` (1037) → split per rule-type group (layer/dep, api,
  syntax/role, struct) keeping the `rules` package.
- `internal/score/score.go` (1021) → split per dimension scorer (done **after**
  F2/F3 logic edits land to avoid churn).
- `internal/output/markdown/markdown.go` (807) → split per report section.
- `internal/initcfg/yamledit.go` (745) → split editor vs serializer.
- `cmd/archfit/pipeline.go` (689) → split wiring stages.

Ordering: F1 → F4 → F3 → F2 → F5 (F5 last; it relocates code F2/F3 edit).

## test_specifications

- **Resolution (F1):** table test — modules `{internal: internal/**, internal/model:
internal/model/**}`, assert `ModuleFor("internal/model/x.go") == "internal/model"`
  and `ModuleFor("internal/arch_test.go") == "internal"`. Cover classify +
  config resolvers (or the shared one).
- **Strength kind (F2):** with a public-glob target and a struct-typed Go hint,
  assert strength == `model`; interface hint → `contract`; func → `functional`;
  `internal:`-glob → `intrusive`; no hint → `contract`.
- **Determinism:** `TestGolden_DoubleRun` must stay byte-identical (most-specific
  loop must be order-independent — guaranteed by sorted-name strict-greater
  tiebreak).
- **F5:** no new tests; existing package tests must pass unchanged (behavior
  preservation is the gate).

## fitness checks

- `TestArchImports` (ring) stays green.
- `make archfit` (dogfood) stays green — **now with an active `layer_inversion`
  gate** (the fix makes archfit's own layer rule enforce for real).
- Add the resolution table test as a permanent regression guard for the
  catch-all class of bug.

## self_review / risks

1. **Layer gate activation (significant).** F1 turns the inert `layer_inversion`
   gate live. If archfit's real layering violates the declared order, `make
archfit` fails (gate: fail). Mitigation: run `make archfit` immediately after
   F1; resolve any surfaced violation as either a real code fix or a config layer
   correction. Do not silence by reverting F1.
2. **Baseline delta (low).** `.archfit-baseline.json` tracks raw metrics, but
   delta bucketing is delta-mode only (`deltaReport` nil in full mode), so
   `--full` dogfood won't fail on the intended metric drop. Confirm; do not
   re-baseline unless verified necessary (known gotcha: re-baselining can flip
   PASS→WARN).
3. **F2 score churn (significant).** coupling_balance changes for all repos.
   Acceptance is the `engine→model/diagnostic == model` example + the strength
   table test, not a fixed score.

## Task #36 resolution — config redesign + scorer-calibration decision

Investigated the scorer to settle the F2/calibration question. Conclusions:

- **The scorer is book-verbatim and correct — no change.** `BookScorer` uses
  Khononov's Ch8–10 ordinals (strength contract=1/model=3/functional=8/intrusive=10;
  distance same=2/`cross_module_same_owner=4`/diff_owner=7/deploy=9; volatility
  low=3/high=10) with `balance = max(|S−D|, 10−V)+1`. **`cross_module_same_owner=4`
  is already the lowest cross-module distance**, so a sole owner is encoded as the
  cross-module minimum — there is nothing lower to claim. Re-weighting frozen
  ordinals to flatter archfit's own score would violate the project's
  adopt-theory-verbatim principle. **Decision: do not touch the scorer.**

- **The critical edges F2 surfaced were book-CORRECT, not false alarms.** model
  coupling (s=3) into a high-volatility core (v=10) at d=4 →
  `max(|3−4|, 0)+1 = 2` = critical: the book's complexity zone (strength≈distance,
  high volatility — a shared domain model that changes often, used across a
  boundary). archfit's `Diagnostic`/`graph`/`coupling` hub is exactly that
  (corroborated by blast_radius). The honest remedy is the book's:
  lower the core's volatility or wrap it in a contract — NOT suppress the metric.

- **F2's real defect was the strength INPUT, not the formula.** With SCIP on, the
  hint collapses Go imports to a blanket `functional` (scores _medium_, hiding the
  model-coupling criticality); only the Go type-info hint carries the accurate
  contract/model/functional split. Wiring the coarse hint into the verdict violates
  abstain-not-fake. **F2 unblocks by making the strength hint accurate (prefer the
  diverse Go type-info hint over SCIP's blanket functional), then applying the
  glob-floor + hint-kind refinement.** At that point coupling_balance will honestly
  rank model-coupling-to-core as the top concern. That is a separate, scoped change
  (touches SCIP/type-info hint precedence), offered as the next step.

- **`.archfit.yaml` is already book-aligned; updated to make the design explicit.**
  Added a design-rationale header (sole-owner → cross-module-minimum distance;
  domain-based volatility; how to read coupling_balance) and corrected the catch-all
  stanza comment (verified: zero production graph nodes resolve to it post-F1). No
  structural classification changes were needed — the subdomain/volatility/layer/
  owner assignments audit as correct per the book.
