# Wave 4: Book Fidelity I — strength & distance classification (score-changing)

## Overview

Wave 4 of 7 from `reports/eval-2026-07-02-v1.1.2/00-FINDINGS.md` (§1 deviations 1–3) and `book-conformance.md`. Assumes Waves 1–3 merged (gate honest, output honest) — **do not run this wave earlier**: it moves coupling_balance scores on every repo, and score movement is only safe to ship when the gate and baselines around it are trustworthy.

The scorer formula is book-verbatim; these three deviations live in the classification feeding it:

- **const/var reads → Functional=8, book says Model=3.** Referencing another module's exported const/var is pure data sharing; Go's extractor maps the resolved object kind var/const to functional (`internal/extract/golang/golang.go:444-459`). A 5-point error on a 10-point ordinal scale, unfixable by config.
- **DTO ≠ Contract in Go.** A concrete integration DTO — the book's canonical Contract example — classifies as Model identically to a leaked domain object; Contract is reachable only via a language-level interface (`golang.go:446-459`, `internal/classify/classify.go:387-406`).
- **No D=10 rung.** The book's own Ch10 Example 1 (cross-vendor integration) has no distance representation; external-target edges are excluded from coupling_balance entirely (`internal/model/coupling/scorer_book.go:26-31`, `internal/engine/assemble.go:150-185`) rather than scored at the ladder's far end.

**Breaking-metric-change discipline (from CLAUDE.md):** ordinal assignment changes are a breaking metric change. This wave bumps `ScoreVersion` to `bc_score.v4` once (Task 1), and every subsequent task lands as its own commit with its own golden regeneration and its own corpus before/after attribution — never merge two score-moving changes into one diff.

## Context

- `internal/extract/golang/golang.go:349-352,444-459` (kind→strength hints), `internal/classify/classify.go:387-406` (public-glob floor semantics: floor refined by hint; internal-glob authoritative).
- `internal/model/coupling/scorer_book.go` (ordinals frozen as named constants), `internal/engine/assemble.go:150-185` (external exclusion), `internal/score/score_boundary_coupling.go` (denominator/evidence).
- SCIP override rules: Go type-info hints are NOT overridden by SCIP (deliberate — see CLAUDE.md "Go edge strength"); TS/Py/Rust heuristic hints ARE. Any hint-mapping change for Go must respect that asymmetry.
- Corpus attribution baseline: saved run JSONs and configs in `reports/eval-2026-07-02-v1.1.2/`.

## Development Approach

- Tests-first; every task = one commit = one golden regen = one corpus attribution measurement.
- Branch `fix/wave4-book-strength-distance`; `make test && make lint && make archfit` between tasks; PR at end (or one PR per task if diffs are large — prefer separate PRs for Tasks 2–4).
- Attribution protocol per score-moving task: run analyze on archfit(Go), ccgram(Py), herdr(Rust), storybook(TS) before and after the commit; record coupling_balance value/band/scored-count movement in a table appended to this plan.

## Implementation Steps

### Task 1: ScoreVersion bump + attribution harness

- [x] bump `ScoreVersion` to `"bc_score.v4"` (`internal/model/coupling/scorer.go` area) with a design note in `docs/design/` listing exactly the three classification changes this version covers (`docs/design/20260702-bc-score-v4.md`)
- [x] add a small script or make target (`make corpus-attrib`) that runs the built binary over the four attribution repos with their saved configs and prints repo → {score, band, scored, abstained, external} for the before/after table (`scripts/corpus-attrib.sh`)
- [x] write test asserting ScoreVersion appears in JSON output (consumers key on it) — `score_version` added to the JSON envelope (always present, not only on BC advisories); `TestJSONRenderer_ScoreVersion` pins the literal
- [x] run `make test && make lint`; commit

Task 1 attribution baseline (bc_score.v4 label, pre-Task-2 — classification still v3-identical):

| repo      | score | band  | scored | abstained | external |
| --------- | ----- | ----- | ------ | --------- | -------- |
| archfit   | 39    | poor  | 290    | 0         | 510      |
| ccgram    | 55    | mixed | 497    | 18        | 0        |
| herdr     | 25    | poor  | 630    | 16        | 21       |
| storybook | 48    | mixed | 310    | 0         | 181      |

### Task 2: const/var → Model (book Ch7 pure-data sharing)

- [x] failing table test in the Go extractor: resolved object kind `const`/`var` (pure data reference) must yield strength hint `model`, not `functional`; func stays `functional`; interface stays `contract`; concrete type stays `model`
- [x] fix the kind mapping (`golang.go:444-459`); check whether TS/Py/Rust hint sources have an equivalent misclassification: grimp has no object kinds (Python abstains — assert unchanged), dependency-cruiser heuristics (check `internal/extract/ts/`), rust-analyzer SCIP strength map (check `internal/extract/scip/scip_strength.go` kind table) — fix only where the same pure-data case is provably mapped to functional; otherwise add a documenting test of current behavior per language
- [x] regen goldens; run corpus attribution; append the before/after table to this plan
- [x] `make test && make lint && make archfit`; commit

Task 2 findings and attribution:

- The SCIP kind table lives in `scip_reader.py` (`_classify`, extracted from the
  former `classify` closure), not `scip_strength.go`. Fixed for **rust only**:
  rust-analyzer terms (`X.`) are const/static/field — provably pure data. TS/Py
  SCIP terms stay `functional` (they can bind callables: arrow-function exports,
  module-level partials) and scip-go never overrides Go type-info hints —
  documented in `scip_reader_test.py`. `_suffix` now separates real terms (`X.`)
  from namespaces/macros so only true data reads move.
- Python (grimp): abstains, unchanged — pinned by the new
  `const import → abstained` case in `py_test.go`. TS (dependency-cruiser):
  const-only module import stays `functional` (object kinds invisible at import
  granularity) — pinned in `ts_test.go` `TestExtract_EdgeTypes`.
- No stored goldens exist to regenerate: `TestGolden_DoubleRun` is a determinism
  gate over the live `testdata/golang` fixture and passes with the new
  const/var fixture files.

| repo      | score | band  | scored | abstained | external | Δ vs Task 1 baseline                                              |
| --------- | ----- | ----- | ------ | --------- | -------- | ----------------------------------------------------------------- |
| archfit   | 40    | poor  | 290    | 0         | 510      | +1 — Go const/var reads → model                                   |
| ccgram    | 55    | mixed | 497    | 18        | 0        | none — grimp abstains (documented)                                |
| herdr     | 29    | poor  | 630    | 16        | 21       | +4 — rust-analyzer SCIP terms (const/static/field) → model        |
| storybook | 48    | mixed | 310    | 0         | 181      | none — depcruise heuristic + TS SCIP terms unchanged (documented) |

### Task 3: DTO reaches Contract in Go

- [x] definition (keep strict, statically decidable): an exported struct with only data fields and an empty method set, referenced across a config-declared `public:` glob boundary, upgrades the public-glob floor from not-intrusive to `contract`; anything with methods or unexported fields stays `model`
- [x] failing tests: DTO fixture (pure-data struct via public glob → contract) vs domain-entity fixture (struct with methods → model) vs DTO not via public glob (→ model; the boundary declaration is what makes it a contract, matching the book's "explicit integration contract")
- [x] implement using type info already loaded (`NeedTypesInfo`); respect the floor-refinement semantics in `classify.go:387-406` (public glob = floor, hint refines; internal glob stays authoritative-intrusive)
- [x] language checks: TS/Py/Rust have no equivalent static signal — document per-language abstention in tests (no fabricated contract upgrades); note that Wave 7's LLM labels are the designed path for those languages
- [x] regen goldens; corpus attribution table; `make test && make lint && make archfit`; commit

Task 3 findings and attribution:

- Definition shipped (strict, statically decidable): exported struct, ≥1 field,
  every field exported, no func-/chan-typed fields (behavior carriers), EMPTY
  method set via `typeutil.MethodSetCache` over the pointer type (value +
  pointer receivers, promoted methods from embedding included). Zero-field
  marker structs (`struct{}` sentinels) are NOT DTOs — they carry no data model.
- The extractor emits a context-dependent hint `dto` (`graph.StrengthHintDTO`,
  rank 2: between contract and model). classify resolves it: public-glob floor
  stands at `contract`; no glob → `model` (strengthFromHint); internal glob
  stays authoritative intrusive. A DTO's FIELDS carry the same hint (package-
  wide pre-pass registry in `buildStrengthHints` — a field `*types.Var` has no
  owner pointer in go/types), otherwise composite-literal keys (`UserDTO{ID:…}`)
  and selector reads would outrank the type reference to model and the upgrade
  would never fire on real consumers.
- Per-language abstention documented in tests: grimp has no object kinds
  (`py_test.go` DTO-shaped class import → abstained), dependency-cruiser has no
  type shapes (`ts_test.go` DTO-only module value import → functional;
  `import type` already maps to contract via type-only), SCIP indexes carry no
  method-set/field-visibility info (`scip_reader_test.py`: type symbols → model
  in every language, no `dto` in RANK; scip-go never overrides Go hints).
- No stored goldens to regenerate (same as Task 2): engine goldens pass
  unchanged; `TestGolden_DoubleRun` passes over the extended `testdata/golang`
  fixture (new DTO/entity/partial/callback consumer files).

| repo      | score | band  | scored | abstained | external | Δ vs Task 2                                                                                                                                                                                                                                                                                |
| --------- | ----- | ----- | ------ | --------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| archfit   | 40    | poor  | 290    | 0         | 511      | by_strength moved 17 edges model→contract (contract 8→25, model 155→138) — DTO upgrades across self-config public globs; aggregate below score granularity. external +1 is the extractor's own new `x/tools/go/types/typeutil` import (verified by stash A/B), not a classification change |
| ccgram    | 55    | mixed | 497    | 18        | 0        | none — grimp abstains (documented)                                                                                                                                                                                                                                                         |
| herdr     | 29    | poor  | 630    | 16        | 21       | none — SCIP carries no method-set/field info, no dto fabrication (documented)                                                                                                                                                                                                              |
| storybook | 48    | mixed | 310    | 0         | 181      | none — depcruise type shapes invisible (documented)                                                                                                                                                                                                                                        |

### Task 4: D=10 rung for declared external systems

- [x] design decision (documented in the code and guide): scoring EVERY library import at D=10 would flood the metric with vendor noise — the book's Example 1 is a _declared integration seam_. Add config `external_systems:` — named entries with target globs (e.g. a vendor SDK package, a generated client) and optional `volatility:` (default low, per book's generic-subdomain guidance)
- [x] edges matching a declared external system get `DistanceExternal = 10` (new frozen named constant) and ENTER scoring; undeclared external edges keep today's disclosed exclusion (`classified_edges.external`) — the deviation shrinks to the undeclared case and is documented as such
- [x] failing tests: declared external edge scored at D=10 with the book formula; undeclared unchanged; BandNA logic unaffected when nothing is declared
- [x] four-language check: the match happens on the classified edge target (language-independent); add one fixture edge per language exercising the glob match (Go import path, TS package specifier, Python dotted target, Rust crate name)
- [x] docs: `configuration-reference.md` new section with the Ch10 Example 1 rationale
- [x] regen goldens; corpus attribution (expect no movement — nobody has `external_systems:` declared yet); `make test && make lint && make archfit`; commit

Task 4 findings and attribution:

- Design shipped: top-level `external_systems:` map — named entries with
  `targets:` globs matched against the classified edge target
  (language-independent: Go import path, TS resolved package path or bare
  specifier, Python dotted module, Rust crate name) and optional `volatility:`
  (default low, book generic-subdomain guidance; validated enum
  high|medium|low|frozen, ≥1 valid target glob required). The match runs only
  when the target resolves to NO module (module resolution always wins) and
  upgrades DistanceUnknown to the new frozen book ordinal
  `bookDistanceExternal = 10` (`Distance = "declared_external"`,
  `distance_basis: declared_external`). Undeclared external edges keep the
  disclosed exclusion; unknown strength still abstains (pinned in tests).
- `classified_edges` gains `declared_external` (edges that ENTERED scoring);
  `external` now counts only the undeclared remainder; couplingBalance evidence
  discloses the declared count. `DistanceIsHigh` includes the new rung
  (critical at D=10 = vendor-lock distributed monolith). encapsulation and
  unbalanced_edge (frozen v2) deliberately exclude declared-external edges;
  legacy calibration scorers get `distanceOrdinalExternal = 6`. JSON schema
  regenerated (`make schema`) with minItems/enum mirroring validate().
- Four-language fixture edges pinned in `classify/external_systems_test.go`;
  book-formula values pinned in `TestBookScorer_DeclaredExternal`
  (contract/low → 10 none — the book's Example 1 shape; functional/low → 8;
  intrusive/high → 1 critical vendor lock).
- No stored goldens to regenerate (same as Tasks 2–3); engine goldens pass
  unchanged — no corpus repo declares `external_systems:`.

| repo      | score | band  | scored | abstained | external | Δ vs Task 3                                                                                                                                                      |
| --------- | ----- | ----- | ------ | --------- | -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| archfit   | 40    | poor  | 292    | 0         | 513      | +2 scored (model) / +2 external — archfit's own new source files (external_systems.go + tests), verified by stash A/B (HEAD = 290/511); no classification change |
| ccgram    | 55    | mixed | 497    | 18        | 0        | none — nothing declared                                                                                                                                          |
| herdr     | 29    | poor  | 630    | 16        | 21       | none — nothing declared                                                                                                                                          |
| storybook | 48    | mixed | 310    | 0         | 181      | none — nothing declared                                                                                                                                          |

### Task 5: Corpus verification & re-baseline

- [x] full four-repo attribution table complete (Tasks 2–4 rows); sanity-check every movement is explained by the change that caused it — unexplained movement blocks the PR
- [x] re-run `make archfit` on self: if the self-score moved bands, update `.archfit-baseline.json` deliberately IN ITS OWN COMMIT (re-baseline gotcha: verify no phantom deltas — Wave 1's n/a-aware tests should hold)
- [x] run the herdr/yazi Rust checks: scores move only per Task 2/3 attribution; `encapsulation` stays n/a (correct-by-design for Rust privacy)
- [x] `make all`; PR(s)

Task 5 verification results (2026-07-04, HEAD after Task 4):

- Fresh `make corpus-attrib` re-run reproduces the Task 4 table byte-identical
  (archfit 40/poor/292/0/513, ccgram 55/mixed/497/18/0, herdr 29/poor/630/16/21,
  storybook 48/mixed/310/0/181). Full movement chain vs the Task 1 baseline is
  attributed: archfit 39→40 (Task 2 const/var→model) + 17 model→contract edges
  (Task 3 DTO, below score granularity) + 290→292/511→513 (Task 4's own new
  source files, stash-verified); herdr 25→29 (Task 2 rust-analyzer SCIP terms
  only); ccgram and storybook flat with documented per-language abstentions.
  No unexplained movement.
- Self dogfood: coupling_balance 40/poor — band unchanged (pre-wave 39/poor),
  `make archfit` PASS exit 0, no phantom metric deltas → the conditional
  re-baseline is NOT triggered; `.archfit-baseline.json` untouched.
- Rust checks: herdr 29/poor (Task 2 movement only, nothing from Tasks 3–4);
  yazi 60/mixed low-conf — raw mean 7.0/10 → 67, capped to 60, 66 scored /
  144 abstained, byte-identical to the v1.1.2 eval baseline (no movement).
  `encapsulation` reports `n/a` on both (correct-by-design for Rust privacy).
- `make all` green. PR #25 (branch `fix/wave1-gate-integrity`, open) carries
  Waves 1–4; this commit is pushed onto it — no separate Wave-4 PR.

### Task 6: [Final] Documentation

- [x] update `docs/design/bc-measurement` doc for v4 semantics; changelog entry: bc_score.v4 with the three-change list and migration note (scores are not comparable across ScoreVersion)
- [x] mark findings-report deviations 1–3 fixed with commit refs

Task 6 notes:

- `docs/design/bc-measurement-v4.md` written (v3 archived to
  `docs/archived/design/`, mirroring the v2→v3 supersede pattern); v4 doc adds
  the D=10 rung to the frozen distance table, const/var→model and DTO→contract
  to the strength mapping, the declared/undeclared external split (§6), the
  Wave-4 self-scorecard (40/poor, 292 scored / 513 external), and — instead of
  carrying over v3's stale `Scored: true` claim — documents same-module edges
  as unscored with a pointer to the Wave 5 plan (eval deviation 4).
  `docs/guide/concepts.md` link retargeted to the v4 doc; CLAUDE.md layout
  count 3→4 files.
- Changelog: the repo has no CHANGELOG.md — releases are tag-triggered and the
  annotated tag message IS the release notes (`release.yaml`). The curated
  entry (three-change list + not-comparable migration note + top-level
  `score_version` consumer note) is drafted in
  `docs/design/20260702-bc-score-v4.md` "Changelog entry" section, ready to
  paste into the next `git tag -a` message.
- Findings report `reports/eval-2026-07-02-v1.1.2/00-FINDINGS.md`: §1
  deviations 1–3 marked ✅ FIXED wave 4 (`1cb538b` DTO, `e64b77d` D=10,
  `25d4e4b` const/var) and §4 P3 backlog items 11–12 marked ✅ DONE with the
  same refs.

## Technical Details

- New ordinal constant `DistanceExternal = 10` joins the frozen table — never reuse an existing rung.
- `external_systems` entries participate in evidence strings (`classified_edges` gains a `declared_external` bucket) so the disclosed-exclusion count stays honest.
- Abstain rules unchanged: unknown strength still abstains; this wave only corrects _known-wrong_ classifications.

## Post-Completion

- Re-run the full 12-repo corpus experiment; archive as `reports/eval-<date>-bc_score.v4/` — this becomes the new comparison baseline for Waves 5–7.
- Downstream consumers pinned to `bc_score.v3` semantics (if any) must be notified via release notes.
