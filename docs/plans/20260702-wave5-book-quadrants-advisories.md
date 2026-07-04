# Wave 5: Book Fidelity II — missing quadrants, honest advisories

## Overview

Wave 5 of 7 from `reports/eval-2026-07-02-v1.1.2/00-FINDINGS.md` (§1 deviations 4–6, §3 new bug 4) and `book-conformance.md`. Assumes Waves 1–4 merged (gate honest, output honest, bc_score.v4 shipped). This wave adds the book's coverage that archfit currently lacks and removes advisory content the book contradicts.

- **Same-module edges unconditionally unscored.** `scorer_book.go:79-82` drops same-module edges before strength is read, hiding Ch10's "local complexity" quadrant (low strength + low distance = low cohesion / big ball of mud). The design doc `bc-measurement-v3.md:33-35` still documents the OLD `Scored:true` behavior — stale doc plus a real book worked example (Ch10 Example 2) the code cannot reproduce.
- **Clone-without-import-edge drift is invisible end-to-end.** Cross-module jscpd clone pairs only upgrade strength on an _already existing_ graph edge; duplication between modules with no import relationship changes nothing — no metric, no finding, no score (verified by the drift-injection probe). This is duplicated knowledge, the book's own extreme case of functional coupling (Ch7), and exactly the drift an AI agent doing copy-paste extraction produces.
- **`cheapest_move` offers "lower volatility".** Ch11's quiz and case studies never sanction volatility-reduction as a remediation lever (`scorer_book.go:121-160`, `internal/engine/advisories.go:62-64`).
- **Dead/incorrect scraps:** ConnascenceAlgorithm is tagged precisely when a jscpd clone is found — the exact scenario the book calls a misconception; the field is dead code reaching no output (`classify.go:501-522`). The volatility-cascade clone-exclusion comment cites a Ch9 section that does not exist (`classify.go:725-732,759-761`) — sound judgment, fabricated citation.
- **Volatility triage cliff** (corpus finding, H4 residue): herdr shows 100% inherited `high` volatility across ~179 synthetic submodules; tokio 99.7% uniform `medium` — an agent cannot prioritize when every edge carries the same volatility.

## Context

- `internal/model/coupling/scorer_book.go` (:79-82 same-module drop, :121-160 cheapest_move), `internal/classify/classify.go` (:501-522, :725-761), `internal/engine/advisories.go`.
- Clone facts: `analyzers.clones` (jscpd) pipeline; symmetric-upgrade path in classify.
- Volatility: config-declared + `AugmentModulesFromGraph`/`AugmentGoWorkspaceModules` owner/volatility inheritance for synthetic modules.
- Fractal-levels framing (book Ch on fractal geometry): cross-module coupling and intra-module cohesion are different abstraction levels — keep them in separate reported numbers.

## Development Approach

- Tests-first; one behavior change per commit with its own golden regen.
- Same-module scoring and the new advisory are REPORT-ONLY in this wave (no gate wiring) — they enter the gate, if ever, only after a corpus shakedown; state this in the output docs.
- Branch `fix/wave5-book-quadrants`; `make test && make lint && make archfit` between tasks; PR at end.

## Implementation Steps

### Task 1: local_coupling report block (Ch10 local-complexity quadrant)

- [x] design note first (in-code + docs/design): same-module edges are scored with the book formula at the same-module distance rung, but reported in a NEW `local_coupling` block per module — they do NOT enter coupling_balance's denominator (fractal level separation; coupling_balance stays the cross-module number consumers already track) — in-code notes on `BookScorer`/`classify.Run`/`buildLocalCoupling`/`LocalCouplingModule`; `bc-measurement-v4.md` §2
- [x] failing tests: same-module low-strength edge lands in `local_coupling`, not in coupling_balance (`TestBuildLocalCoupling`); Ch10 Example 2's worked numbers reproduce through the scorer (functional/same_module/high → balance 7, `TestBookScorer_BookExamples`)
- [x] remove the pre-strength drop at `scorer_book.go:79-82`; route same-module scored edges to the new block; abstain rules identical (`Severity` stays `SeverityNone` for same-module — advisory pipeline remains cross-boundary, `TestRun_SameModuleScoredSeverityNone`)
- [x] update the stale design doc (`bc-measurement-v3.md:33-35`) to the actual v4 behavior — v3 is archived; the live `bc-measurement-v4.md` §2 same-module paragraph and the distance-mapping line updated instead
- [x] four-language check: fixture with a same-module edge per language (Go package-internal import, TS relative import within module glob, Python dotted intra-package, Rust intra-crate `::` module when cargo_modules enabled) — each appears in `local_coupling`, none in coupling_balance (`TestBuildLocalCoupling_FourLanguages`)
- [x] regen goldens; `make test && make lint && make archfit`; commit — no stored golden changed (TestGolden is a determinism double-run; byteidentical baselines have no same-module edges); full suite + lint + dogfood gate green, self-scan verdict pass with `local_coupling` populated (3 modules, 19 same-module edges; coupling_balance denominator unchanged at 299)

### Task 2: bc/duplicated_knowledge advisory (clone pair without an edge)

- [x] failing test: two modules with a cross-module jscpd clone pair and NO import edge → new advisory finding `bc/duplicated_knowledge` with both locations, strength labeled symmetric-functional (book Ch7/Ch10 Example 3 framing), report-only, `Kind: advisory` (`TestDuplicatedKnowledgeAdvisory`, `TestCloneOnlyPairs`)
- [x] implement: when clones report a cross-module pair and no graph edge exists between the modules, emit the advisory (do NOT invent a graph edge — the scorer's inputs stay tool-derived facts) — `classify.CloneOnlyPairs` scores the pair (symmetric × module-pair distance × worst-of-pair volatility through the book formula); `engine.duplicatedKnowledgeAdvisories` emits the finding; never promoted (coupling gate matches `RuleIDBCImbalanced` only), baseline persists it with native advisory kind
- [x] respect existing suppression paths: `.archfit-labels.yaml` approved labels can accept a pair (either direction; mirrors the fromPin guard); tested — plus the `coupling.min_severity` floor and the SeverityNone balanced case (e.g. frozen pair) emit nothing
- [x] test the existing behavior is unchanged when an edge DOES exist (symmetric upgrade path, no duplicate advisory) — `TestDuplicatedKnowledgeAdvisory_EdgeExists`
- [x] regen goldens; commit — no stored golden changed (no golden fixture carries clone clusters); `make test && make lint && make archfit` green; self-scan verdict pass with 3 real duplicated-knowledge advisories surfaced (extractor copy-paste pairs with no import edge)

### Task 3: cheapest_move honesty + dead-code removal

- [x] remove `lower_volatility` from cheapest_move levers (`scorer_book.go:121-160`, advisory wording in `advisories.go:62-64`); remaining levers: reduce strength, reduce distance — per Ch11; failing test first (no advisory may name volatility as the move) — `bookCheapestMove` drops the volatility branch (both `lower_volatility` and `declare_volatility`); advisories copy the scorer label verbatim, so nothing downstream can name volatility; exhaustive `TestBookCheapestMove_NoVolatilityLever` (failed pre-fix on 50+ combos) + `TestBookCheapestMove_Cases`; legacy calibration scorers (additive/multiplicative, cmd/calibrate only) keep their levers
- [x] delete the dead ConnascenceAlgorithm tagging (`classify.go:501-522`) — it reaches no output and encodes a book-contradicting inference; delete, do not comment out — whole dead Connascence facility removed (field, type, constants, `classifyConnascence`, classifyEdge block); `connascencePairKey` renamed `modulePairKey` (still used by clone upgrade/cascade/duplicated-knowledge); stale CoA comments updated in config/types.go, engine/labels.go, engine/engine.go
- [x] fix the fabricated Ch9 citation comment (`classify.go:725-732,759-761`) to describe the actual reasoning without a fake section reference — `computeEffectiveVolatility` doc now states the essential-rate-of-change reasoning citing Ch9 generically, no invented section title
- [x] regen goldens (cheapest_move text changes); commit — no stored golden carries cheapest_move text, so regen is a no-op (TestGolden green, byte-identical); `make test && make lint && make archfit` green, dogfood gate PASS 0 blocking

### Task 4: Volatility triage disclosure

- [x] failing test: coupling_balance evidence string discloses volatility provenance counts — `declared: N, inherited: M, cascade: K` — so a uniform-volatility repo is visibly uniform-by-inheritance, not measured — `diagnostic.VolatilityProvenance` (module counts, `classified_edges.volatility_provenance`), `classify.ComputeVolatilityProvenance` (pre- vs post-augmentation module maps + cascade overlay recompute), evidence line appends `, undeclared: U` when nonzero; `TestCouplingBalance_VolatilityProvenanceEvidence` (failed pre-fix) + `TestComputeVolatilityProvenance` + wiring assert in `TestRun_DiagnosticShape`
- [x] allow per-module volatility override to reach synthetic modules: config `modules:` entries whose `paths:` glob matches a synthetic module key (e.g. `mycrate::submod`) apply their `volatility:`; test on a herdr-shaped fixture (parent declared high, one submodule overridden low) — already reachable: an exact `::` glob is maximally specific in `ModuleMap.ModuleFor`, so it pre-empts synthetic registration in `AugmentModulesFromGraph` and wins edge resolution; locked by `TestSyntheticModuleVolatilityOverride_HerdrShape` (override low, sibling keeps inherited high)
- [x] do NOT invent differentiation (no churn-derived volatility — book: volatility comes from the domain, not commit history); the LLM-assisted role assignment is Wave 7's job — disclosure-only change; constraint stated on the `VolatilityProvenance` doc comment
- [x] regen goldens; `make test && make lint && make archfit`; commit — only the two byte-identical CLI baselines changed (new `volatility_provenance` block + evidence line; diff inspected); TestGolden double-run green; full suite + lint + dogfood gate PASS 0 blocking; self-scan discloses `declared: 39, inherited: 0, cascade: 14`

### Task 5: Corpus verification (four languages)

- [x] Rust — herdr: evidence shows `inherited:` dominance explicitly; overriding one submodule's volatility in a scratch config changes only its edges — evidence line reads `volatility provenance (modules): declared: 1, inherited: 178, cascade: 0`; scratch config module keyed `herdr::persist::io` (exact `::` glob, volatility low): exactly 1 of 630 advisories changed (high/score 5/medium → low/score 8/low), 0 added/removed, siblings `::restore`/`::snapshot` keep inherited high, provenance became declared 2/inherited 177
- [x] Rust — yazi: `local_coupling` block present; coupling_balance value unchanged vs post-Wave-4 baseline (denominator untouched) — shipped config (cargo_modules off) has 0 same-module edges by construction (crate-level graph, block correctly omitted); old-vs-new binary byte-identical (60/mixed, 66 scored, 144 abstained); cargo_modules scratch config with underscore lib-name globs (`yazi_fs::*` — dash package-name globs match nothing) yields 2526 same-module edges, block present, coupling_balance STILL 60/66 scored; all same-module entries abstain on unknown SCIP strength (pre-existing yazi SCIP-strength gap, abstain-not-fake)
- [x] Go — archfit self: dogfood gate still PASS; new `bc/duplicated_knowledge` — inject a scratch cross-module copy-paste in a worktree, verify the advisory fires with both locations, then discard — staleness helpers copied into `internal/extract/dynimports` (no import edge between the pair, compiles) fired `bc/duplicated_knowledge` `internal/extract/dynimports<->internal/staleness` with both files in `locations[]`; worktree removed
- [x] Python — ccgram / prefect: `local_coupling` populated from dotted intra-package edges; no new gate trips — ccgram 16 / prefect 15 modules (e.g. `prefect.blocks.abstract→prefect.blocks.core`); 0 gate findings on both; ccgram old-vs-new binary byte-identical (warn/55/497 scored); prefect matches the v1.1.2 eval headline (pass/53/935 scored)
- [x] TypeScript — storybook: run completes; advisory counts sane (no clone-pair flood — if jscpd floods on generated code, verify FileClass Production filtering applies to clone facts) — exit 0/pass on the code/ subtree; 15 `bc/duplicated_knowledge` + 39 imbalanced advisories, no flood; jscpd ok (3468 files); local_coupling 18 modules; 48/mixed, 310 scored matches the v1.1.2 eval (scip honestly `absent` — no node_modules)
- [x] corpus clean; `make all`; PR — prefect/storybook left spotless, herdr/yazi untracked files predate these runs (Jun 26 mtimes); make all green (fmt, lint, test, dogfood PASS); branch pushed → PR #25 carries Wave 5

### Task 6: [Final] Documentation

- [ ] output reference: `local_coupling` block, `bc/duplicated_knowledge` advisory, volatility provenance counts
- [ ] mark findings deviations 4–6 + invisible-drift gap fixed with commit refs

## Technical Details

- `local_coupling` per-module summary: count of scored same-module edges, share in the complexity quadrant (S low ∧ D low), worst offenders with locations — enough for an agent to act, small enough not to bloat the JSON.
- The advisory's severity derives from the scored pair (symmetric strength 9 × distance × volatility through the standard formula), not a hardcoded level.

## Post-Completion

- After a few weeks of corpus/self dogfooding, decide whether `bc/duplicated_knowledge` earns a gate default (`warn`); revisit with real false-positive data.
- Shakedown observation (Task 5, yazi + cargo_modules): `buildLocalCoupling` emits an entry for every module with same-module edges even when ALL of them abstained — 597 zero-`scored_edges` entries on yazi. Consider omitting zero-scored modules to honor the "small enough not to bloat the JSON" design note.
- Shakedown observation (Task 5, storybook): `local_coupling` worst-offenders surface test→prod same-module edges (`a11yRunner.test.ts → a11yRunner.ts`); decide whether same-module scoring should filter to Production files like the size metrics do.
