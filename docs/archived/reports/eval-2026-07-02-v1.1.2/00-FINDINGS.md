# archfit v1.1.2 — Book Conformance × Metric Quality × Corpus Experiments

**Date:** 2026-07-02 · **Binary:** v1.1.2 (commit 6140aba) · **Method:** 3 multi-agent workflows (19 + 29 + 16 agents) + independent adversarial verification of the 6 highest-impact claims (all 6 CONFIRMED at root cause, with repros).

**Sub-reports:** [book-conformance.md](book-conformance.md) · [metric-quality.md](metric-quality.md) · [corpus-experiments.md](corpus-experiments.md)

---

## Verdict in one paragraph

archfit is three layers, and their quality diverges sharply.

**Layer 1 — the scoring math — is book-verbatim**: `max(|S−D|, 10−V)+1`, all three ordinal ladders (S 1/3/8/9/10, D 2/4/7/9, V 1/3/6/10), validated by replaying the
book's own Ch10 worked examples through the scorer.

**Layer 2 — edge classification feeding the formula — is lossy**: Go DTOs can't reach Contract, const/var reads jump 5 ordinal points, no D=10 rung, same-module edges never scored.

**Layer 3 — gating and the AI-agent contract — is the weakest**: the flagship metric cannot gate by construction, metric-delta gating is sign-inverted for count metrics, the documented `metrics.*.gate` knobs are dead code, and `config init` emits a rule type that can never fire. The abstain-not-fake discipline holds everywhere (no fabricated numbers found across 12 repos) — the failures are honest omissions and dead wiring, not lies. For the stated goal — _fast feedback loop for AI agents to prevent architecture drift_ — the tool today delivers that loop only for explicit rule violations on small/medium repos; the Balanced Coupling dimension it is architected around is measured well but structurally disconnected from the gate and from `agent_tasks[]`.

---

## 1. Book conformance (76 deduped checkable claims)

| Status       | Count | Meaning                                                       |
| ------------ | ----- | ------------------------------------------------------------- |
| verbatim     | 7     | book value/rule exactly (incl. the entire Ch10 numeric model) |
| adapted      | 25    | same intent, different mechanics                              |
| out_of_scope | 21    | deliberate, documented exclusions                             |
| missing      | 16    | checkable but absent                                          |
| deviation    | 7     | contradicts the book                                          |

**Verbatim core, independently re-validated:** formula, ordinals, S/D/V separation, the 10−V rescue-term semantics (Ch9 Fig 9.2/9.3), runtime_async genuinely report-only, abstain-not-fake. Prior audit (2026-06-30) **corrected**: `Symmetric=9` and `Frozen=1` are in the book's own Ch10 scale, not archfit inventions — the earlier audit had no book text to check against.

**Deviations that matter (classification layer):**

1. **HIGH — DTO ≠ Contract in Go.** — ✅ **FIXED** wave 4 (`1cb538b`, bc_score.v4): a pure-data DTO (exported struct, ≥1 field, all fields exported, no func/chan fields, empty method set) crossing a declared `public:` glob boundary upgrades the floor to Contract; TS/Py/Rust abstain (no static signal — documented in tests). Was: a concrete integration DTO (the book's canonical Contract example) classifies as Model; Contract is reachable only via a language interface (`internal/extract/golang/golang.go:446-459`, `internal/classify/classify.go:387-406`).
2. **HIGH — no D=10 rung.** — ✅ **FIXED** wave 4 (`e64b77d`, bc_score.v4): new frozen `DistanceExternal = 10`; edges matching a declared `external_systems:` config entry enter scoring at D=10 (book Ch10 Example 1), undeclared external edges keep the disclosed exclusion (`classified_edges.external`, declared count in `classified_edges.declared_external`). Was: the book's own Example 1 (cross-vendor integration) had no distance representation; such edges were excluded from coupling_balance instead of scored at the far end (`internal/model/coupling/scorer_book.go:26-31`).
3. **MEDIUM — const/var reads score Functional=8** where the book demands Model=3 — a 5-point jump on a 10-point scale, unfixable by config for Go (`golang.go:444-459`). — ✅ **FIXED** wave 4 (`25d4e4b`, bc_score.v4): Go resolved object kinds const/var → `model`; rust-analyzer SCIP const/static/field terms → `model`; TS/Py unchanged (documented abstentions).
4. **MEDIUM — same-module edges unconditionally unscored**, hiding Ch10's "local complexity" quadrant. — ✅ **FIXED** wave 5 (`d002ae8`): same-module edges now score with the book formula at the same-module rung into a new report-only `local_coupling` block (per-module scored/abstained counts, complexity-quadrant share, worst offenders with locations); they never enter coupling_balance's denominator (fractal-level separation) and Ch10 Example 2's worked numbers reproduce through the scorer. Design doc updated (`bc-measurement-v3.md` archived; live `bc-measurement-v4.md` §2). Was: dropped pre-strength at `scorer_book.go:79-82` with the stale v3 doc still documenting `Scored:true`.
5. **MEDIUM — `cheapest_move` offers "lower volatility"** as a remediation lever Ch11 never sanctions (`scorer_book.go:121-160`, `internal/engine/advisories.go:62-64`). — ✅ **FIXED** wave 5 (`db1e95d`): volatility branch removed from `bookCheapestMove` (both `lower_volatility` and `declare_volatility`); remaining levers are reduce-strength and reduce-distance per Ch11; advisories copy the scorer label verbatim, so nothing downstream can name volatility as the move.
6. LOW — ConnascenceAlgorithm tagged from jscpd clones (the book's named misconception); dead code, reaches no output. LOW — cascade clone-exclusion comment cites a Ch9  section that doesn't exist (sound logic, fabricated citation). — ✅ **FIXED** wave 5 (`db1e95d`): the whole dead Connascence facility deleted (field, type, constants, `classifyConnascence`); the cascade comment now states the essential-rate-of-change reasoning citing Ch9 generically, no invented section title.

---

## 2. Metric quality (24 emitted items audited)

Distribution: **0 solid · 13 acceptable · 11 weak.** Every item carried at least one defect; the systemic ones below were independently re-verified (static trace + sandbox repros in all six cases).

### Confirmed systemic defects (all re-verified)

| # | Defect | Root cause | Failure scenario |
| --- | --- | --- | --- |
| V1 | **Metric-delta sign inversion** — ✅ **FIXED** wave 1 (PR #25, `cd1f1e7` + harness `f12bd22`): per-metric `direction` field, worsening-move-only regression | `computeVerdict`: `Delta<0 → WARN` unconditionally (`engine.go:452-467`); correct for ratio metrics, inverted for count metrics (cycle, unbalanced_edge) | New cycle → Delta=+1 → **silent PASS**; fixing a cycle → false WARN. No `TestComputeVerdict` exists |
| V2 | **coupling_balance can never gate** — ✅ **FIXED** wave 1 (PR #25, `f4f00f6`): `coupling.gate: {min_band, max_drop}` runs inside the pipeline; trips promote active BC advisories to `agent_tasks[]`; BandNA never gates | Not one of the 5 registered metrics (`metrics.go:52-58`); score synthesized _after_ verdict finalized (`cmd/archfit/analyze.go:188→198`); BC advisories excluded from `computeVerdict` (`engine.go:259-261`) | Flagship metric drops 78→40 → exit 0, no config can change that |
| V3 | **Dead gate config fields** — ✅ **FIXED** wave 1 (PR #25, `8e6c45e`): `gate`/`max_new`/`min_delta` wired into the verdict; `max_new_high` deleted from schema/docs/spec; wrong-kind knobs rejected | `metrics.*.gate/min_delta/max_new/max_new_high` schema-validated, zero consumers; `ForMetric()` view has no callers | Docs (`configuration-reference.md:108-114,750-766`) present them as working knobs |
| V4 | **`config init` emits a rule that can never fire** — ✅ **FIXED** wave 2 (`64bf176`): generator now emits `type: forbidden_layer_direction`; vestigial `from_layer:/to_layer:` keys dropped from both the inferred-rules and placeholder branches | Emits `type: forbidden_dependency` with `from_layer:/to_layer:` keys (`initcfg.go:352-370`); that checker reads `from:/to:` globs, so empty globs match nothing | Onboarding path + its own `# TODO: promote to gate: fail` comment → permanently vacuous blocking gate |
| V5 | **public_api_only mislabels same-module access** — ✅ **FIXED** wave 2 (`ddc1db4`): `Check` now resolves both endpoints through the module map and skips when `from.module == to.module`; `Why` text no longer claims "Cross-module" for a same-module edge | No module-map check; fires on lexical `/internal/` (`rules_dependency.go:63-92`) with hardcoded "Cross-module" text; default gate `fail` | Idiomatic Go `domain/` importing its own `domain/internal/` → blocking false positive |
| V6 | **agent_tasks disconnected from coupling drift** | `Kind != "gate" → skip` (`agenttask.go:48`) — _deliberate_ per code comment ("signals, not orders"), unchanged; `Files[]` carrying bare module keys / dotted Python IDs, not file paths (`rules_api.go:88-91`, `py.go:299-336`) — ✅ **FIXED** wave 3 (`e1233b6`, `51b2fff`): `rules_api.go` findings now carry real `Locations`; `filesFor` resolves Go/Python/Rust module keys to on-disk paths (drops unresolvable entries, falls back to the module's config `paths:` root dir); every `agent_tasks[].files[]` entry is now `os.Stat`-verified in tests | Poor coupling score with dozens of critical findings → `agent_tasks: []` on all 12 corpus repos (unchanged: the `Kind != "gate"` skip is deliberate) |

### Trust ranking for an AI agent today

- **Trustworthy when firing:** `forbidden_dependency`, `new_cross_module_dependency`, `classified_edges` counts, `coupling_balance` _value_ read from `--json` (not the gate, not `--base` delta), `bc/imbalanced_coupling` findings' `locations[]`.
- **Advisory-only, verify before acting:** encapsulation, public_api_change/type_leak, staleness advisories, tool_coverage.
- **Noise / do not trust:** unbalanced_edge (false-confident 0 without hand-authored `internal:` globs), cycle-metric _deltas_ (V1), coverage metric (can fabricate >100%; documented 125% case), blast_radius (absence silently forces coupling_balance to n/a), public_api_max counts (61–89% test-file declarations), `agent_tasks[].files` for non-Go findings (✅ **FIXED** wave 3, `e1233b6`/`51b2fff` — see V6).

---

## 3. Corpus experiments (12 repos, v1.1.2)

| Repo           | Lang    | Wall    | Verdict | coupling_balance                | Top issue                                                                                |
| -------------- | ------- | ------- | ------- | ------------------------------- | ---------------------------------------------------------------------------------------- |
| archfit (self) | Go      | 8.2s    | pass    | 39/poor, high conf, 284 scored  | cascade drives 100 critical edges; 7 genuinely distributed-monolith-shaped               |
| pumba          | Go      | 7.1s    | pass    | 45/mixed, 70 scored             | critical findings never gate                                                             |
| spotinfo       | Go      | 2.6s    | pass    | 44/mixed, 4 scored              | clean run                                                                                |
| ccgram         | Py      | 27.2s   | warn    | 55/mixed, 497 scored            | **narrative bug:** "high-strength" prose on 15/16 critical edges whose strength=model    |
| yazi           | Rust    | 34.9s   | pass    | 60/mixed, low conf (31% scored) | phantom `cohesion` in coverage_gaps; test clone inflates production edge                 |
| herdr          | Rust    | 79.3s   | pass    | 25/poor, 97% scored             | `cross_module_different_owner` label reused for non-ownership distance (390/630 edges)   |
| omni (subtree) | Go      | 36.2s   | warn    | 40/poor (correct case)          | **NEW BUG:** lowercase `--root` silently flips to false 58/mixed                         |
| prometheus     | Go      | 86.5s   | pass    | 56/mixed, 620 scored            | 23 critical findings, exit 0, agent_tasks=0                                              |
| ruff*          | Rust+Py | —       | pass    | 23 scored, 100% clone-derived   | SCIP still 0 files; honest low-conf, not fabricated                                      |
| storybook      | TS      | (fixed) | pass    | partial coverage                | depcruise `partial`, 5612 unresolved specifiers, **no warning**, confidence still `high` |
| prefect        | Py      | 493s    | —       | 935 scored (was 0)              | grouped findings' `edge.path` points at wrong file (2/2 sampled)                         |
| tokio          | Rust    | —       | pass    | measured                        | 99.7% uniform `medium` volatility → triage cliff                                         |

\* ruff's run agent died on an API stall; row reconstructed from its saved JSON artifacts — lower confidence.

**Prior-gap scorecard (v1.1.0 → v1.1.2):**

- **H1 fabricated 60/mixed — FIXED** (prefect 0→935 scored; ruff honestly low-confidence now; its SCIP coverage gap remains).
- **H2 storybook crash — FIXED** (exact repro config now exits 0, depcruise degrades to `partial`).
- **H3 tokio git-owner flood — FIXED** (owner_source=git, 8/979 diff-owner edges).
- **H4 volatility floods — FIXED as specified**, but the triage cliff re-emerged via uniform inherited volatility (herdr 100% high across ~179 synthetic submodules). Wave 5 (`ba9d250`) made the cliff visible instead of silent: coupling_balance evidence now discloses `volatility provenance (modules): declared/inherited/cascade[, undeclared]` (herdr reads `declared: 1, inherited: 178`), and an exact `::` module glob can override a synthetic submodule's volatility (verified on herdr: 1 of 630 advisories changed, siblings untouched). ✅ **DONE** wave 7 (`041e2ee`, `c267f66`): `config update --llm` now proposes review-only `role: core|supporting|generic` plus derived volatility for declared and synthetic modules, and the corpus spot check differentiated herdr/tokio volatility with 5/5 agreement. Volatility remains domain-declared, never churn-derived; the final 12-repo before/after corpus rerun remains the post-completion validation pass.
- **M1 agent_tasks on coupling drift — UNCHANGED** (confirmed deliberate; see V6).
- **TS coverage dishonesty (storybook) — FIXED** wave 3 (`b9e1657`): `Coverage.Status: partial` now carries a reason string (unresolved count) plus a stderr warning, and `coupling_balance` confidence downgrades one band once the unresolved ratio crosses the named threshold. tsconfig autodetect (shipped pre-wave-3, `b8f0392`/`038366e`) already drove storybook's own unresolved-specifier count to 0, so the mechanism is exercised by Task 4's unit tests (fake depcruise output) rather than re-triggered live on this repo.

**New bugs found this run:**

1. **macOS case-variant `--root` ownership corruption (omni)** — ✅ **FIXED** wave 2 (`6126793`): CODEOWNERS subtree-prefix resolution now applies the same `os.SameFile` canonicalization as `snapScanRoot`, and owner-source degradation (`codeowners→git→none`) emits a stderr warning naming the cause plus surfaces `owner_source` in JSON. Lowercase `--root` now reproduces the correct-case run byte-identically (`owner_source: codeowners`, 40/poor). Previously: lowercase path drops `SubtreePrefix` → `owner_source: none` → verdict flips 40/poor → 58/mixed with zero warning. Task 25's `snapScanRoot` fixed scope resolution but not CODEOWNERS subtree resolution — an incomplete fix of a previously-fixed bug class.
2. **Hardcoded severity narrative** (`bcRiskClause`, advisories.go): says "high-strength coupling" for every critical edge at non-high distance regardless of actual strength. — ✅ **FIXED** wave 3 (`5909ea9`): clause composed from `matched_by.strength`/`distance`/`volatility`, no fixed string; re-verified on ccgram — 3/3 critical findings' `why` now names "model integration strength".
3. **Grouped finding `edge.path` is an arbitrary representative** — frequently a different file than `locations[]` (2/2 sampled on prefect at group_count 17 and 38). — ✅ **FIXED** wave 3 (`26b681e`): `edge.from`/`edge.to.path` now set from `locations[0]`'s owner side; re-verified on prefect — 0/73 grouped findings mismatched.
4. **Clone-without-import-edge drift is invisible end-to-end** — cross-module duplication only upgrades strength on an _existing_ graph edge; copy-paste between unconnected modules changes nothing (exactly the drift an AI agent doing extract-and-duplicate produces). — ✅ **FIXED** wave 5 (`bbb5760`): a cross-module jscpd clone pair with no graph edge now emits a report-only `bc/duplicated_knowledge` advisory with both locations, scored symmetric strength × module-pair distance × worst-of-pair volatility through the book formula (Ch7 duplicated knowledge); respects approved labels (either direction) and `coupling.min_severity`; never promoted by `coupling.gate`. Verified live: archfit self-scan surfaced 3 real extractor copy-paste pairs; the drift-injection probe re-run in a worktree fired the advisory with both files in `locations[]` (`ea55ad6`).

**Drift-injection probe (the core use-case test):** injected forbidden import in a worktree → caught in **~5.3s warm** with exit 1 and file:line-precise `agent_tasks[]` including fix goal and rerun command — an agent could fix it from JSON alone. That loop is real and good. It just only exists for rule violations.

**Latency:** archfit self 5.3–8.2s warm, small Go repos 2–8s, medium 12–37s, large 47s–8m13s (prefect). No gate-level cache (cold≈warm everywhere); `--base` re-scans both sides in full (≈2× cost). Fine for per-PR gating on small/medium repos; not per-edit on large ones. — ✅ **FIXED** wave 6 (`328923e` design, `c8fe2d3` store+Runner decorator, `fbf8179` per-language wiring, `6a8c4eb` incremental `--base`, `4299bf4` bench-gate): content-keyed extractor fact cache (`.archfit-cache/facts/`, design `docs/design/fact-cache.md`), byte-identical warm vs `--no-cache` verified on archfit + ccgram. Measured cold → warm gate: archfit 8.8→3.0s, ccgram 29.8→8.7s, herdr 49.8→10.9s, storybook 11.2→3.6s (all ≥3× targets met); prefect 773.3→720.3s (1.07× — jscpd+SCIP end partial/timed-out there and are vetoed from caching by design; targeted follow-up per plan Post-Completion). `--base` now reuses base-side facts by commit SHA (second same-ref run: zero base-side subprocess work).

---

## 4. Fix backlog (priority-ranked)

**P1 — gate integrity (the product promise):**

1. ✅ **DONE** (wave 1, PR #25, `cd1f1e7` + `f12bd22`) — V1 delta-sign: add per-metric direction to `computeVerdict` + `TestComputeVerdict` table test (new-cycle-must-warn, fixed-cycle-must-not).
2. ✅ **DONE** (wave 1, PR #25, `f4f00f6`) — V2/M1: give coupling_balance a gate path — score-band floor and/or negative-delta gate in config (`coupling.gate: {min_band, max_drop}`), and optionally emit agent_tasks from critical BC findings behind a flag. Shipped without a flag: a tripped gate promotes active BC advisories to `Kind: "gate"`, which is the existing agent_tasks filter. This is the single change that connects the flagship metric to the stated product goal.
3. ✅ **DONE** (wave 2, `64bf176`) — V4 config init: emit `type: forbidden_layer_direction` (one string); drop vestigial `from_layer/to_layer` from the generator.

**P2 — trust in emitted facts:** 4. ✅ **DONE** (wave 2, `6126793`) — omni case-path: make CODEOWNERS subtree resolution case-insensitive-safe (same `os.SameFile` treatment as `snapScanRoot`); warn when owner_source degrades. 5. ✅ **DONE** (wave 1, PR #25, `8e6c45e`) — V3: wire `metrics.*.gate/max_new/min_delta` or delete them from schema + docs (dead knobs are worse than absent knobs) — wired all three; `max_new_high` deleted. 6. ✅ **DONE** (wave 2, `ddc1db4`) — V5: public_api_only must consult the module map before claiming "cross-module". 7. ✅ **DONE** (wave 3, `5909ea9`) — bcRiskClause: derive narrative from `matched_by.strength`, never a fixed string. 8. ✅ **DONE** (wave 3, `26b681e`) — Grouped findings: `edge.path` now the first `locations[]` file (owner side), never an arbitrary representative. 9. ✅ **DONE** (wave 3, `e1233b6`, `51b2fff`) — V6 `Files[]` half: `agent_tasks[].files[]` entries now resolve to real on-disk paths for Go/Python/Rust/TS, dropping unresolvable entries rather than fabricating a bare module key or dotted ID. 10. ✅ **DONE** (wave 3, `b9e1657`) — TS coverage honesty: `partial` status carries a reason + stderr warning, and `coupling_balance` confidence downgrades on high unresolved-specifier ratio.

**P3 — book fidelity (classification layer):** 11. ✅ **DONE** (wave 4, `1cb538b` DTO, `25d4e4b` const/var) — Contract reachable for Go DTOs (pure-data struct with no methods crossing via a declared public glob → Contract floor); const/var reads → Model. 12. ✅ **DONE** (wave 4, `e64b77d`) — D=10 rung for declared `external_systems:` edges (undeclared remainder stays a disclosed exclusion). 13. ✅ **DONE** (wave 5, `d002ae8`) — same-module edges scored into the report-only `local_coupling` block (Ch10 local-complexity quadrant as a per-module fact); coupling_balance denominator unchanged. 14. ✅ **DONE** (wave 5, `db1e95d`) — `lower_volatility` lever removed (Ch11 levers only: reduce strength, reduce distance); fabricated Ch9 citation rewritten; dead ConnascenceAlgorithm facility deleted. 15. ✅ **DONE** (wave 5, `bbb5760`) — clone-pair drift surfaces as the report-only `bc/duplicated_knowledge` advisory even without an import edge (never gate-promoted).

**Performance (§3 latency, wave 6):** 16. ✅ **DONE** (wave 6, `328923e` `c8fe2d3` `fbf8179` `6a8c4eb` `4299bf4`) — extractor fact cache keyed by content + tool version + config slice (never scores; timed-out/partial results never cached), incremental `--base` via per-SHA worktree path, `make bench-gate`. Cold → warm gate: archfit 8.8→3.0s, ccgram 29.8→8.7s, herdr 49.8→10.9s, storybook 11.2→3.6s; prefect 773.3→720.3s (miss — partial jscpd/SCIP vetoed from cache by design, follow-up open). Warm output byte-identical to `--no-cache` (archfit 455 KB, ccgram 1.7 MB diffs empty).

**New (found in wave-6 review, deferred — metric-semantics change):** 17. `ownershipDistance("","")` returns `DistanceSameModule` (`internal/classify/distance_structure.go:99`, pre-existing since `63a4c19` 2026-06-18). In a NON-degenerate-owner repo (≥2 distinct owners), moduleDistance Step 3 is authoritative for every edge, so an edge between two UNOWNED modules scores the lowest distance rung (D=2) instead of falling through to code structure — flattening real cross-module coupling on partially-owned multi-team repos (omni/tokio class). Wave-5's `moduleDistance` extraction reused it for `bc/duplicated_knowledge`, widening the reach. Candidate fix: both-owners-empty ⇒ per-pair fallthrough to `codeStructureDistance`. NOT fixed in the wave-6 review pass: it changes `coupling_balance` scores and needs corpus verification + golden inspection per the "output changes are never automatic" gate.

---

## 5. Answer to the original questions

**How close is archfit to the book?** The numeric model is the book, verbatim, provably (worked examples replay). Around it: 25 faithful adaptations, 21 documented exclusions, and the 7+16 deviations/missing items concentrated in per-language strength/distance heuristics — i.e., conformance is limited by instrumentation, not by the model. No other tool the book community has produced measures this at all; the honest-abstention discipline is the differentiator worth protecting.

**Is it a fast feedback loop for AI agents against drift?** Today: yes for rule violations (5s, file:line, fix-goal), no for coupling drift (measured accurately, gates never, agent_tasks never) and no for large repos (minutes, no cache — ✅ addressed wave 6: warm gate 3× or better on the corpus except prefect, see item 16). The P1 backlog above is precisely the gap between "deterministic architecture linter" and the stated product goal.
