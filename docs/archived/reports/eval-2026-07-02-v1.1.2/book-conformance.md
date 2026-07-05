# archfit book-conformance report — Khononov, _Balancing Coupling in Software Design_

Source: 13 chapter-level claim audits (foundations, ch04–ch14, appendices), 98 raw claim rows, cross-checked against `/Users/alexei/Workspace/archfit` @ `6140aba` (2026-07-01). Deduplicated to 76 distinct claims. Compared against the prior code-only audit `reports/eval-2026-06-30-v1.1.0/book-conformance-source-audit.md` (2026-06-30, no book text access).

## Verdict

**Verbatim formula, lossy instrumentation.** The scoring _math_ — the `bc_score.v3` balance formula, all three ordinal ladders (Strength 1/3/8/9/10, Distance 2/4/7/9, Volatility 1/3/6/10), and the dimensional separation (Strength ⊥ Distance ⊥ Volatility, merged only at scoring time) — is book-verbatim and independently confirmed against the book's own numeric worked examples (Ch10 Examples 1–4), not just its qualitative descriptions. This is the strongest and most load-bearing part of the tool, and it holds up.

The real conformance gaps sit one layer down, in the **edge-classification heuristics that feed the formula**, not in the formula itself:

- Go's `goObjectStrength` collapses two book-distinct cases into one ordinal: a referenced package-level `const`/`var` (pure data — should be Model, S=3) and a purpose-built integration DTO/command object (should be Contract, S=1) both land on Functional (S=8) or Model (S=3) respectively — never their book-correct values (`internal/extract/golang/golang.go:444-459`).
- Same-module edges are unconditionally excluded from scoring (`Scored:false`), even though the book's own Ch10 Example 2 computes a real, non-abstained balance for exactly this case (low distance, moderate strength) — and a design doc still documents the old, different behavior.
- Distance has no rung for the book's own flagship worked example (Ch10 Example 1: cross-vendor integration, D=10) — those edges are excluded from `coupling_balance` entirely rather than scored at the ladder's far end.
- The single-hop volatility cascade (Ch9's "inferred volatility") is book-faithful when enabled but ships **off by default**, so most configs never get it.
- `cheapest_move` (the Ch11 remediation advisory) offers "lower volatility" as a coequal fix alongside strength/distance reduction — a move Ch11's own quiz and case studies never sanction.

None of these corrupts the frozen formula or fabricates a number where none exists (archfit's abstain-not-fake discipline holds throughout); they instead misclassify or exclude specific edges _before_ the formula ever sees them, which quietly narrows what `coupling_balance` can actually detect.

**One correction of the prior (2026-06-30) audit:** that audit — code-only, no book text — flagged `StrengthSymmetric` (S=9) and `VolatilityFrozen` (V=1) as archfit inventions "beyond the book's published levels." With book text in hand, both are directly present in the book's own Ch10 numeric scale ("9 = Symmetric functional coupling," "1 = Legacy system that is not being evolved"). This is not an archfit extension; it's the prior audit under-crediting the tool without the source text to check against.

## Per-chapter table

| Chapter     | Rows (pre-dedup) | Verbatim | Adapted | Deviation | Missing | Out of scope |
| ----------- | ---------------- | -------- | ------- | --------- | ------- | ------------ |
| foundations | 4                | 0        | 2       | 0         | 0       | 2            |
| ch04        | 10               | 0        | 6       | 0         | 2       | 2            |
| ch05        | 9                | 0        | 1       | 0         | 1       | 7            |
| ch06        | 8                | 0        | 1       | 1         | 4       | 2            |
| ch07        | 8                | 2        | 1       | 2         | 1       | 2            |
| ch08        | 9                | 2        | 4       | 2         | 0       | 1            |
| ch09        | 9                | 1        | 3       | 2         | 2       | 1            |
| ch10        | 11               | 4        | 3       | 1         | 1       | 2            |
| ch11        | 4                | 2        | 0       | 1         | 1       | 0            |
| ch12        | 3                | 0        | 1       | 0         | 1       | 1            |
| ch13        | 10               | 3        | 3       | 0         | 4       | 0            |
| ch14        | 4                | 2        | 2       | 0         | 0       | 0            |
| appendices  | 9                | 2        | 2       | 0         | 0       | 5            |

_(Chapter breakdown counts each chapter's original claims once, before cross-chapter dedup; a claim later merged into a cross-chapter cluster is counted here under its originating chapter's raw status, not the resolved one, so this table shows per-chapter auditor coverage. The dedup counts below are the authoritative totals.)_

## Deduplicated counts (authoritative)

76 distinct claims after merging 9 cross-chapter clusters (22 raw rows → 9 resolved rows):

| status       | count |
| ------------ | ----- |
| adapted      | 25    |
| out_of_scope | 21    |
| missing      | 16    |
| deviation    | 7     |
| verbatim     | 7     |

## Cross-chapter contradictions resolved

Nine claims were reported by 2–5 different chapter auditors against the same code fact, sometimes with different verdicts. Resolved by reading the cited code directly:

### 1. Same-module edges (ch04 adapted / ch08 deviation-high / ch10 deviation-medium / appendices out_of_scope) → **resolved: deviation, medium**

`internal/model/coupling/scorer_book.go:79-82`:

```go
if c.Distance == DistanceSameModule {
    return EdgeScore{Scored: false, Reason: reasonBook}
}
```

This fires **before strength is even read**, so every same-module edge is excluded regardless of whether it's genuine cohesion (high strength, good) or Ch10's "local complexity" anti-pattern (low strength packaged too tightly, bad — Table 10.3's other quadrant). The book's own Ch10 Example 2 computes a real balance (7) for a same-namespace pair; archfit computes nothing. Confirmed doc/code drift: `docs/design/bc-measurement-v3.md:33-35` still says "Edge is still `Scored: true`" — the design doc was never updated when the code changed to unconditional abstention (git history: `222bcca` originally scored same-module edges as `Scored:true/Balance:10`; `5f633fb` changed it to full abstention without updating the doc).

Verdict: **deviation**, not `out_of_scope` — this isn't a clean, intentional scope line (the stale doc proves the team's own design intent was different), but it's an honest abstain surfaced in evidence counts, not a fabricated number, and it excludes only the intra-module edge population of a cross-module tool. **Medium**, not ch08's original "high" — it doesn't put a wrong number on the flagship metric, it just means one book-illustrated quadrant (same-module "local complexity") is invisible.

### 2–4. Three ordinal-table "verbatim" clusters (balance formula, strength scale, volatility scale) → **resolved: verbatim, high** (each)

Each was independently confirmed verbatim by 4 different chapter auditors citing the same `scorer_book.go` constants from different angles (Ch7/10 for strength, Ch9/10/13/14 for volatility, Ch10/11/14/appendices for the formula itself). No disagreement on substance, only redundant coverage — merged to one row per cluster. Kept severity **high** because these are the tool's flagship, load-bearing scoring path, correctly implemented.

### 5. Distance's third input — runtime coupling (ch07 verbatim / ch08 adapted+out_of_scope / ch11 verbatim / appendices out_of_scope) → **resolved: adapted, medium**

All rows describe one consistent design: `RuntimeAsync` is recorded (`internal/model/diagnostic/diagnostic.go:330-334`, doc comment: "Evidence only — never consumed by classify, score, or gate logic") and never adjusts distance or strength. This is simultaneously (a) correct per Ch7 ("sync/async doesn't affect integration strength" — verbatim) and (b) a gap against Ch8's explicit claim that runtime coupling reduces distance (deliberately rejected, `docs/design/bc-measurement-v3.md:277-279`: "runtime_adjust / +1 async distance ... rejected"). One design decision, two different book claims it touches — not a contradiction between chapters, just two sides of the same fact. Net: **adapted** (2 of 3 book-named distance factors — structure, ownership — implemented verbatim; the third, runtime, is a documented, consistent exclusion).

### 6. Inferred-volatility cascade default (ch09 deviation-medium / ch10 adapted-low / ch13 adapted-low / appendices verbatim) → **resolved: adapted, medium**

`coupling.volatility_cascade` (`internal/config/types.go:37-40`) defaults to `false`; archfit's own self-config turns it on (`.archfit.yaml:36`). All four auditors agree on every fact — implemented, single-hop, book-faithful when enabled, off by default, self-config opts in. They disagreed only on the label. Followed the majority + the stronger argument: the _mechanism_ is book-faithful (this isn't a wrong formula), and the opt-in has a documented false-positive rationale (coarse module granularity can let one strong edge over-taint a whole module, `docs/plans/20260701-multilang-reliability-fixes.md` B4). **Adapted**, but flagged medium because the practical consequence stands: a user running default config does not get Ch9's "inferred volatility" behavior, which the book presents as a fundamental property of the model, not an opt-in feature.

### 7. Distance ladder's missing top rung (ch08 deviation-high / ch10 missing-medium) → **resolved: deviation, high**

`bookDistanceOrdinal` tops out at `DistanceCrossDeployUnit=9` (`scorer_book.go:26-31`); there is no value ≥9 representing the book's D=10 ("systems implemented by different vendors"). Distance-unresolvable edges (third-party/vendor targets) are classified `DistanceUnknown` and excluded from `coupling_balance`'s numerator and denominator entirely (`internal/engine/assemble.go:150-185`). Kept **deviation/high**, not `missing`/`out_of_scope`: this is the book's own headline Example 1 (cross-vendor cloud ML integration), and archfit doesn't approximate it poorly — it structurally cannot represent the scenario, and the design choice to treat "external" as "not a coupling seam" directly contradicts a chapter whose entire opening worked example computes a real, non-trivial balance for exactly this case.

### 8. Distance ladder's middle values, 4 and 7 (ch08 adapted-low / ch10 adapted-medium) → **resolved: adapted, medium**

Merged — same fact (book gives no numeric distance table in Ch8 itself; only Ch10's worked examples surface D=2 and D=9; archfit interpolates 4 and 7 for same-owner/diff-owner). Both auditors agree this is a defensible, self-acknowledged approximation (`docs/design/bc-measurement-v3.md:69-74` labels it "Book anchor (approx.)").

### 9. Connascence static/dynamic split (ch06 missing / appendices out_of_scope) → **resolved: missing, low**

`internal/model/coupling/coupling.go:93-102`'s `Connascence` type has 3 values total (none/type/algorithm) — no static/dynamic split exists in the type system at all, and no current (non-archived) design doc declares the dynamic degrees out of scope (only a historical, superseded v0.4 spec mentions deferring them to "future" work). Resolved toward ch06's stricter reading: **missing**, not a clean documented exclusion — tempered to **low** severity because the whole `Connascence` field is dead code (see finding below), so the gap has no live effect on any output.

## All deviations (7, ranked by severity)

| # | Severity | Chapter(s) | Claim | Evidence |
| --- | --- | --- | --- | --- |
| D1 | **high** | ch07 | A concrete integration DTO/command object (the book's own canonical Contract-coupling example, Listing 7.2/7.3) is classified `StrengthModel`, identical to a leaked internal domain object — Contract is reachable only via a language-level `interface`. | `internal/extract/golang/golang.go:446-459` (`types.IsInterface` is the only path to Contract); `internal/classify/classify.go:387-406` |
| D2 | **high** | ch08+ch10 (merged) | No Distance rung exists for the book's own Example 1 (cross-vendor, D=10); those edges are excluded from `coupling_balance` entirely rather than scored at the ladder's far end. | `internal/model/coupling/scorer_book.go:26-31`; `internal/engine/assemble.go:150-185` |
| D3 | medium | ch07 | Referencing another module's exported `const`/`var` (pure data, book says must be Model) is classified `StrengthFunctional` — a jump of 5 ordinal points on the book's own 1–10 scale, and unfixable for Go by any config toggle (SCIP never overrides a Go type-info hint per CLAUDE.md). | `internal/extract/golang/golang.go:444-459` (default branch) |
| D4 | medium | ch04+ch08+ch10+appendices (merged) | Same-module edges are unconditionally excluded from scoring before strength is even read, hiding Ch10's "local complexity" anti-pattern (Table 10.3); a design doc still documents the old, different (`Scored:true`) behavior. | `internal/model/coupling/scorer_book.go:79-82`; `docs/design/bc-measurement-v3.md:33-35` (stale) |
| D5 | medium | ch11 | `cheapest_move` offers `lower_volatility` as a coequal remediation alongside strength/distance reduction — Ch11's quiz (Q2/Q3) and every case study never sanction volatility-reduction as a valid lever for a strength or volatility increase; only strength/distance are. Advisory-only today, but feeds `agent_tasks`, so an agent could act on it. | `internal/model/coupling/scorer_book.go:121-160,191-210`; `internal/engine/advisories.go:62-64` |
| D6 | low | ch06 | An edge is tagged `ConnascenceAlgorithm` precisely when a cross-module jscpd clone pair is found — the exact scenario the book calls out as a common misconception ("connascence of algorithm is not about duplication of code"). Kept low: the field is dead code (see below), so the mislabel reaches no output today. | `internal/classify/classify.go:501-522,506-512` |
| D7 | low | ch09 | The volatility-cascade exclusion for clone-derived edges is attributed in a code comment to a named Ch9 section ("Essential vs. Accidental (In)Volatility") that does not exist anywhere in Ch9's text — the underlying engineering judgment is sound, the citation is fabricated/misattributed. | `internal/classify/classify.go:725-732,759-761` |

## All "missing" findings (16), ranked by severity

| Severity | Chapter | Claim | Evidence |
| --- | --- | --- | --- |
| medium | ch06 | Connascence classification is fully **dead code**: never attached to `Diagnostic`, never in JSON output, never read by any metric — "report-only" currently means "reported nowhere." A named book concept with committed code and tests but zero observable effect. | `internal/model/coupling/coupling.go:120-125`; no production call sites outside `classify.go`/its own tests |
| medium | ch09 | Source-control churn as a volatility signal (with the book's own explicit false-positive/false-negative caveats) is not implemented even as report-only evidence, unlike `runtime_async`. The only justification is a code comment overstating the book ("Balanced Coupling forbids commit-history volatility") — Ch9 never uses the word "forbid." | `internal/history/git/git.go`; `internal/config/views.go:135-136` |
| medium | ch13 | Cross-service knowledge minimization via private/public event splitting (the book's flagship Case Study 1 fix) has no signal path for wire-only, cross-repo event/message schemas — only in-repo importable types are visible to any extractor. | `internal/rules/rules_dependency.go`; no schema/event extractor exists |
| low | ch08+ch10 (merged) | No rung for D=10 (different vendors) — see D2 above; listed here too since it is simultaneously an absent enum value. |  |
| low | ch04 | Ousterhout's deep-module interface:implementation ratio has no counterpart; `public_api_max` is a size ceiling, not a depth ratio. | `internal/rules/rules_api.go` |
| low | ch04 | Module's "Context" property (runtime/environment assumptions not in the public interface) has no config field or metric. | — |
| low | ch05 | Common/external coupling's actual mechanism (shared global mutable state / shared DB / shared object-storage file) produces zero edges in any extractor — invisible rather than abstained. | `internal/extract/{go,ts,py,rust}` |
| low | ch06 | Connascence's 9 named static+dynamic degrees collapse to 3 enum values (none/type/algorithm); no name/meaning/position, no dynamic degrees at all. | `internal/model/coupling/coupling.go:93-102` |
| low | ch06+appendices (merged) | Static/dynamic connascence categorization is entirely absent from the type system, with no current design doc declaring it out of scope. | `internal/model/coupling/coupling.go:93-102` |
| low | ch06 | V1 spec's own plan to implement meaning/name/position via concrete heuristics (magic status codes, positional tuples) was never built; only type and (mislabeled) algorithm exist. | `docs/spec/arch-fitness-spec-v0.4.md:774-799` (marked historical) |
| low | ch07 | No degree-level sub-classification within Functional/Model/Contract (sequential vs. transactional, name vs. meaning vs. position). | `internal/classify/classify.go:486-522` |
| low | ch09 | No config validation rejects a typo'd `subdomain:`/`volatility:` value (unlike `role:`, which does) — falls through silently to worst-case `VolatilityUndeclared` with no nudge. | `internal/config/config.go:168-174` |
| low | ch11 | No per-edge S/D/V delta between two refs — `analyze --base` diffs only the aggregate `coupling_balance` score, so archfit cannot itself say "this edge's distance grew" the way the book's case studies do by hand. | `internal/decision/decision.go:87-101,282-310`; `cmd/archfit/worktree.go` |
| low | ch12 | No distinct signal for "this module has structurally outgrown its boundary, split it" vs. "this edge is imbalanced, rebalance it" — the book treats these as different remedies for different failure modes. | `internal/engine/advisories.go` |
| low | ch13 | Aggregate pattern (entity-cluster transactional boundaries) and Interface Segregation via per-interface method distance are both below archfit's module/package grain — not detected. | `docs/guide/configuration-reference.md:667-672`; no aggregate concept exists |
| low | ch13 | Method-level code smells / per-function complexity (`analyzers.complexity`) was a real prior capability, actively removed in v1.0 — not merely never built. | `internal/config/config.go:93-95,144` |

## Notable verbatim strengths

- **The balance formula itself**, `balance = max(|S-D|, 10-V) + 1`, clamped 1..10, is byte-for-byte the book's Ch10 derivation, and it is validated against the book's own worked numeric examples as regression tests (`scorer_book_test.go`), not merely claimed — e.g. Example 3 (S=9,D=9,V=10 →
  1. and Example 4 (S=10,D=9,V=1 → 10) are literally encoded as passing test cases.
- **Strength ordinal table (1/3/8/9/10)** matches the book's own Ch10 numeric scale exactly, including the less-obvious `9 = Symmetric functional coupling` — and archfit's symmetric-from-clones detection is aimed at the book's own worked Example 3 ("the same business algorithm is duplicated").
- **Volatility ordinal table (1/3/6/10)**, subdomain-driven (core→10, supporting/generic→3), with `frozen`/`legacy`→1 matching the book's own legacy-system anchor — confirmed as book-native, not an archfit invention (correcting the prior audit).
- **`runtime_async` is genuinely report-only** — recorded, never consumed by classify/score/gate, verified by a dedicated regression test (`TestRun_RuntimeAsync_StaticGraphUnchanged`) asserting identical verdict/metrics with and without async evidence. This is an easy, plausible mistake to make (wire transport into distance) and archfit avoids it.
- **Abstain-not-fake discipline holds everywhere audited**: unknown strength/distance abstains (`Scored:false`) rather than defaulting to a mid-band number; external/vendor edges are excluded from the denominator and disclosed as a count, not silently folded into a fabricated `coupling_balance`. Even the deviations found (same-module exclusion, missing D=10 rung) are honest omissions, not wrong numbers.
- **Volatility × Strength interaction (Ch9 Fig 9.2/9.3)** is captured numerically: high strength into a high-volatility (core) module shrinks the `10-V` rescue term, dragging balance toward critical — matching the chapter's central point that identical coupling graphs carry different real risk depending on what they touch.
- **Distance/Strength dimensional independence** (classified from entirely separate signal sources — globs/hints/SCIP/labels for Strength; deploy-unit/ownership/code-structure for Distance — merged only inside `Score()`) faithfully mirrors the book's explicit "different aspects, merged only in Ch10" framing.

## What changed since the prior audit (2026-06-30, code-only)

The prior audit (`book-conformance-source-audit.md`) was produced without access to the book text — it could confirm code behavior but not check it against the book's actual claims, and said so explicitly ("Cannot confirm the book-literal integers without the text," item 6b). This audit had the text. Net effect: most "new" findings below are the same code, now checked against a source the earlier audit didn't have — not code regressions.

- **Corrected, not regressed**: item 3's "additive vocabulary beyond the book" claim (`StrengthSymmetric`, `VolatilityFrozen`) is overturned — both values are directly in the book's own Ch10 numeric scale. See "Notable verbatim strengths" above.
- **Confirmed, now with a mechanism**: item 6a's "doc/code inconsistency" (same-module comment says balance=10, code returns `Scored:false`) is the same bug, still present, but this audit found the _design doc_ also still documents the stale behavior (`bc-measurement-v3.md:33-35`), and connected it to a real book worked example (Ch10 Example 2) that the current code cannot reproduce — upgrading a "harmless but misleading" comment nit to a documented deviation (D4).
- **Confirmed unchanged**: `runtime_async` report-only (item 1) and volatility-from-config-not-churn (item 2) — both now independently reconfirmed against actual book chapters (Ch7, Ch9) rather than just internal code comments.
- **New material this audit surfaces that the prior one had no basis to check** (these are audit-depth deltas, not code changes since 06-30): Contract-vs-Model DTO misclassification (D1), const/var-as-Functional misclassification (D3), the missing D=10 vendor rung (D2), the default-off volatility cascade, and the `cheapest_move` volatility-as-remediation issue (D5).
- **Actual code changes in the window** (`927f8c4` v1.1.0 → `6140aba`, 2026-06-30 to 2026-07-01, per `git log`): CLI restructure, multi-language reliability fixes (A1/A2/B3/B4/B5/B6/H2/H3), and `coupling_balance` n/a-when-unmeasured honesty fixes — none of these touched `scorer_book.go`'s ordinals or formula, so the book-conformance surface audited here is materially the same code the prior audit saw, just examined with the source text this time.

## Notes on audit scope limits

- Several `out_of_scope` verdicts rest on book-text claims (Cynefin, Big Ball of Mud narrative, structured-design's six coupling levels, DB-schema-level worked examples) that have no formula or threshold in the book itself — these are correctly unscoreable by a static-facts tool by construction, not gaps.
- One claim (Appendix C's per-chapter quiz answer key) was deliberately not turned into audit rows — the answer letters exist without the corresponding question text in the provided source excerpt, so checking them would be unverifiable guesswork rather than evidence-based auditing.
- This audit is code-and-book-text based, not a live run of `archfit analyze` against a corpus; severity for `missing`/`out_of_scope` findings reflects impact on the tool's own stated charter (a deterministic coupling-boundary fitness function), not general software-quality importance.
