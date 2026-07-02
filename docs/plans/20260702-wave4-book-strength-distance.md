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

- [ ] bump `ScoreVersion` to `"bc_score.v4"` (`internal/model/coupling/scorer.go` area) with a design note in `docs/design/` listing exactly the three classification changes this version covers
- [ ] add a small script or make target (`make corpus-attrib`) that runs the built binary over the four attribution repos with their saved configs and prints repo → {score, band, scored, abstained, external} for the before/after table
- [ ] write test asserting ScoreVersion appears in JSON output (consumers key on it)
- [ ] run `make test && make lint`; commit

### Task 2: const/var → Model (book Ch7 pure-data sharing)

- [ ] failing table test in the Go extractor: resolved object kind `const`/`var` (pure data reference) must yield strength hint `model`, not `functional`; func stays `functional`; interface stays `contract`; concrete type stays `model`
- [ ] fix the kind mapping (`golang.go:444-459`); check whether TS/Py/Rust hint sources have an equivalent misclassification: grimp has no object kinds (Python abstains — assert unchanged), dependency-cruiser heuristics (check `internal/extract/ts/`), rust-analyzer SCIP strength map (check `internal/extract/scip/scip_strength.go` kind table) — fix only where the same pure-data case is provably mapped to functional; otherwise add a documenting test of current behavior per language
- [ ] regen goldens; run corpus attribution; append the before/after table to this plan
- [ ] `make test && make lint && make archfit`; commit

### Task 3: DTO reaches Contract in Go

- [ ] definition (keep strict, statically decidable): an exported struct with only data fields and an empty method set, referenced across a config-declared `public:` glob boundary, upgrades the public-glob floor from not-intrusive to `contract`; anything with methods or unexported fields stays `model`
- [ ] failing tests: DTO fixture (pure-data struct via public glob → contract) vs domain-entity fixture (struct with methods → model) vs DTO not via public glob (→ model; the boundary declaration is what makes it a contract, matching the book's "explicit integration contract")
- [ ] implement using type info already loaded (`NeedTypesInfo`); respect the floor-refinement semantics in `classify.go:387-406` (public glob = floor, hint refines; internal glob stays authoritative-intrusive)
- [ ] language checks: TS/Py/Rust have no equivalent static signal — document per-language abstention in tests (no fabricated contract upgrades); note that Wave 7's LLM labels are the designed path for those languages
- [ ] regen goldens; corpus attribution table; `make test && make lint && make archfit`; commit

### Task 4: D=10 rung for declared external systems

- [ ] design decision (documented in the code and guide): scoring EVERY library import at D=10 would flood the metric with vendor noise — the book's Example 1 is a _declared integration seam_. Add config `external_systems:` — named entries with target globs (e.g. a vendor SDK package, a generated client) and optional `volatility:` (default low, per book's generic-subdomain guidance)
- [ ] edges matching a declared external system get `DistanceExternal = 10` (new frozen named constant) and ENTER scoring; undeclared external edges keep today's disclosed exclusion (`classified_edges.external`) — the deviation shrinks to the undeclared case and is documented as such
- [ ] failing tests: declared external edge scored at D=10 with the book formula; undeclared unchanged; BandNA logic unaffected when nothing is declared
- [ ] four-language check: the match happens on the classified edge target (language-independent); add one fixture edge per language exercising the glob match (Go import path, TS package specifier, Python dotted target, Rust crate name)
- [ ] docs: `configuration-reference.md` new section with the Ch10 Example 1 rationale
- [ ] regen goldens; corpus attribution (expect no movement — nobody has `external_systems:` declared yet); `make test && make lint && make archfit`; commit

### Task 5: Corpus verification & re-baseline

- [ ] full four-repo attribution table complete (Tasks 2–4 rows); sanity-check every movement is explained by the change that caused it — unexplained movement blocks the PR
- [ ] re-run `make archfit` on self: if the self-score moved bands, update `.archfit-baseline.json` deliberately IN ITS OWN COMMIT (re-baseline gotcha: verify no phantom deltas — Wave 1's n/a-aware tests should hold)
- [ ] run the herdr/yazi Rust checks: scores move only per Task 2/3 attribution; `encapsulation` stays n/a (correct-by-design for Rust privacy)
- [ ] `make all`; PR(s)

### Task 6: [Final] Documentation

- [ ] update `docs/design/bc-measurement` doc for v4 semantics; changelog entry: bc_score.v4 with the three-change list and migration note (scores are not comparable across ScoreVersion)
- [ ] mark findings-report deviations 1–3 fixed with commit refs

## Technical Details

- New ordinal constant `DistanceExternal = 10` joins the frozen table — never reuse an existing rung.
- `external_systems` entries participate in evidence strings (`classified_edges` gains a `declared_external` bucket) so the disclosed-exclusion count stays honest.
- Abstain rules unchanged: unknown strength still abstains; this wave only corrects _known-wrong_ classifications.

## Post-Completion

- Re-run the full 12-repo corpus experiment; archive as `reports/eval-<date>-bc_score.v4/` — this becomes the new comparison baseline for Waves 5–7.
- Downstream consumers pinned to `bc_score.v3` semantics (if any) must be notified via release notes.
