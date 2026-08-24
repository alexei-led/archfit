# Balanced Coupling measurement engine — design v6.0

Date: 2026-07-05. Status: SHIPPED. Supersedes `bc-measurement-v3.md` (archived
under `docs/archived/design/`). The v3 doc recorded the move to Khononov's
published formula; v4 changed classification feeding that formula; v5 changes
which deterministic coupling facts enter the headline rollup; v6 makes the
opt-in inferred-volatility cascade transitive. The formula, ordinals, bands, and
abstain discipline are unchanged. Delta notes: `20260702-bc-score-v4.md`,
`20260705-bc-score-v5.md`, and `20260705-bc-score-v6.md`.

Related plans: `docs/plans/completed/20260702-wave4-book-strength-distance.md` (Tasks 1–6),
`docs/plans/completed/20260705-wave1-deterministic-book-fidelity.md` (Tasks 2–5).

---

## 1. What shipped

archfit implements Vlad Khononov's _Balancing Coupling in Software Design_
**verbatim** — the published per-edge formula and ordinal anchors — since v3.
archfit owns only the _instrumentation_ — measuring code and placing each edge
on the book's scale.

`ScoreVersion = "bc_score.v6"` — a breaking metric change: **v6 scores are not
comparable to v5 scores.** v4 fixed three known-wrong classifications found by
the 2026-07-02 eval (`docs/archived/reports/eval-2026-07-02-v1.1.2/00-FINDINGS.md` §1
deviations 1–3), v5 closes the clone-only duplicated-knowledge score gap, and v6
closes the one-hop inferred-volatility cascade gap:

1. Go const/var reads score Model (S=3), not Functional (S=8) — pure data
   sharing per book Ch7. Rust gets the same fix via the rust-analyzer SCIP
   term kinds (const/static/field → model).
2. Pure-data Go DTOs referenced across a declared `public:` glob boundary
   reach Contract (S=1) — the book's canonical Contract example.
3. Frozen distance rung `DistanceExternal = 10` (book Ch10 Example 1,
   cross-vendor integration) for config-declared `external_systems:` seams.
4. Clone-only duplicated knowledge — cross-module clone pairs with no import
   edge — enters `coupling_balance` by default through
   `coupling.duplicated_knowledge: score`. Set the policy to `advisory` to
   preserve the v4 report-only behavior.
5. Inferred volatility (`coupling.volatility_cascade: true`) propagates to a
   deterministic fixpoint across strong deliberate coupling chains instead of
   stopping at one hop.

---

### Book alignment frame

Use these labels when changing code or docs:

- **Book-exact:** the Ch10 formula, published ordinal anchors, balance bands,
  and abstain discipline.
- **Sound adaptation:** deterministic tool facts mapped onto book vocabulary,
  such as compiler/object-kind strength hints, static connascence labels,
  same-module local-coupling reporting, and the fixpoint implementation of the
  Ch9 volatility-cascade idea.
- **Policy choice:** conservative defaults and boundaries archfit chooses for
  reproducibility: compressed D=3/D=5/D=6/D=8, declared-only external seams,
  clone-only duplicated-knowledge score/advisory policy, and undeclared V=10.
- **Out of scope:** dynamic connascence scoring, runtime/lifecycle distance
  scoring, churn-derived volatility, and LLM-only gate changes.

---

## 2. The book formula (authoritative — do not deviate)

From Khononov, Ch10 (MIN=1, MAX=10, higher = better balanced):

```
modularity = |S − D|                          // 0..9
balance    = max(modularity, 10 − V) + 1      // 1..10
```

Same-module edges (`same_module` distance) score with the same formula at the
same-module distance rung (D=2), restoring the book's Ch10 "local complexity"
quadrant (low strength + low distance = low cohesion — the ball-of-mud corner;
was eval §1 deviation 4, fixed in Wave 5). High strength close together scores
high balance (cohesion, Ch10 Example 2: functional/same_module/high → 7); low
strength close together scores low balance (model/same_module/high → 2).
Fractal-level separation: same-module scores report in the per-module
`local_coupling` block ONLY — they never enter `coupling_balance`'s
scored/abstained denominator, never set edge `Severity` (no
`bc/imbalanced_coupling` advisory), and never gate. The block is report-only
in Wave 5; any gate wiring waits for a corpus shakedown. Abstain rules are
identical at both levels: unknown strength abstains same-module too
(`abstained_edges` in the block).

Severity bands derived from balance:

| balance | band       |
| ------- | ---------- |
| 1–2     | `critical` |
| 3–4     | `high`     |
| 5–6     | `medium`   |
| 7–8     | `low`      |
| 9–10    | `none`     |

Implemented in `internal/relationship/scoring/scorer_book.go` as `BookScorer`.
`ScoreVersion` is defined in `internal/relationship/coupling/coupling.go`.

---

## 3. Book ordinal anchors (frozen — cite the book before changing)

### Strength

| Constant             | Book anchor | Ordinal (S) |
| -------------------- | ----------- | ----------- |
| `StrengthContract`   | Contract    | 1           |
| `StrengthModel`      | Model       | 3           |
| `StrengthFunctional` | Functional  | 8           |
| `StrengthSymmetric`  | Symmetric   | 9           |
| `StrengthIntrusive`  | Intrusive   | 10          |

`StrengthSymmetric` (S=9) represents duplicated functionality (clone-detected
DRY violation) — see §4.

### Distance

| Constant                       | Book anchor (approx.)         | Ordinal (D) |
| ------------------------------ | ----------------------------- | ----------- |
| `DistanceSameModule`           | same object                   | 2           |
| `DistanceCrossModuleSameOwner` | different packages, same org  | 4           |
| `DistanceCrossModuleDiffOwner` | different packages, diff org  | 7           |
| `DistanceCrossDeployUnit`      | services / different deploy   | 9           |
| `DistanceExternal`             | cross-vendor (Ch10 Example 1) | 10          |

`DistanceExternal` (D=10) is new in v4 and reachable ONLY via a declared
`external_systems:` config entry — see §6.

The middle Ch8 range is deliberately compressed. archfit currently has stable,
deterministic facts for same-module (D=2), same-owner/near structure (D=4),
different-owner/far structure (D=7), deploy-unit boundary (D=9), and declared
vendor seam (D=10). It does **not** split D=3/D=5/D=6 or add D=8 from names,
directory taste, or dependency-manager package shape alone. JSON and Markdown
surface this as `classified_edges.by_distance_basis` plus
`classified_edges.distance_compression`, so a D=4/D=7 result is not mistaken for
full book-rung precision.

### Volatility

| Source                        | Book anchor           | Ordinal (V)       |
| ----------------------------- | --------------------- | ----------------- |
| `subdomain: core`             | core                  | 10                |
| explicit `volatility: high`   | core (override)       | 10                |
| `subdomain: supporting`       | supporting            | 3                 |
| `subdomain: generic`          | generic               | 3                 |
| explicit `volatility: low`    | supporting/generic    | 3                 |
| explicit `volatility: medium` | (interpolation)       | 6                 |
| explicit `volatility: frozen` | frozen/legacy         | 1                 |
| explicit `volatility: legacy` | alias for `frozen`    | 1                 |
| undeclared / unknown          | → abstain rescue term | 10 (conservative) |

`VolatilityFrozen` exists in code. `legacy` is a config alias that maps to it;
add a separate `VolatilityLegacy` constant only if it ever needs distinct
semantics.

`medium=6` is an interpolation on the acknowledged open 3–10 range in the book.

---

## 4. Instrumentation mapping (archfit signal → book anchor)

How archfit places each edge on the book scale:

### Strength mapping

| Signal                                                                                     | Strength assigned                                               |
| ------------------------------------------------------------------------------------------ | --------------------------------------------------------------- |
| Target in `public:` globs or SCIP Protocol/ABC/interface symbol kind                       | `contract` (S=1)                                                |
| Go type-info resolved kind: interface                                                      | `contract` (S=1)                                                |
| Go pure-data DTO across a declared `public:` glob (see §4.2)                               | `contract` (S=1)                                                |
| Go type-info resolved kind: concrete type, **const, var** (v4; const/var was `functional`) | `model` (S=3)                                                   |
| SCIP struct/class symbol kind (shared data model)                                          | `model` (S=3)                                                   |
| rust-analyzer SCIP term kinds const/static/field (v4; was `functional`)                    | `model` (S=3)                                                   |
| Go type-info resolved kind: func; SCIP function/method; config-declared `functional`       | `functional` (S=8)                                              |
| Cross-module clone pair detected (see §4.1)                                                | `symmetric` (S=9)                                               |
| Target in `internal:` globs, Go `internal/`, SCIP private, or config-declared `intrusive`  | `intrusive` (S=10)                                              |
| Pinned approved label (`.archfit-labels.yaml`)                                             | label value wins — overrides extractor for `functional`/`model` |
| Nothing classifies the edge                                                                | `unknown` → **abstain** (see §5)                                |

`contract` and `intrusive` from config globs are config-authoritative: pinned
labels cannot override them. Go type-info hints are compiler-grade ground truth
and are NOT overridden by SCIP; TS/Py/Rust heuristic hints ARE (see CLAUDE.md
"Go edge strength"). TS const-only imports stay `functional` (object kinds are
invisible at import granularity) and grimp abstains (no object kinds) — both
pinned by documenting tests.

#### 4.1 Symmetric from clone detection

When the clone detector (`analyzers.clones.enabled: true`) finds a clone pair that
crosses a module boundary, any edge between those modules whose deterministic
strength is `functional` or `unknown` is upgraded to `StrengthSymmetric` (S=9).
Config-authoritative `contract` or `intrusive` assignments, and approved pinned
labels, are never overridden.

When the clone pair has **no** import edge in either direction, it is clone-only
duplicated knowledge: the same book Ch7 symmetric functional coupling exists,
but the import graph cannot carry it. v5 keeps detection pure in
`classify.CloneOnlyPairs` and lets `coupling.duplicated_knowledge` choose the
rollup policy:

- `score` (default): include the pair in `classified_edges` and
  `coupling_balance` as a symmetric-strength coupling fact.
- `advisory`: preserve v4 behavior; the pair can emit a
  `bc/duplicated_knowledge` advisory but stays out of the headline score.

JSON exposes the policy effect through `classified_edges.clone_only_scored` and
`classified_edges.clone_only_advisory`. Advisory filtering still applies
`coupling.min_severity`, approved labels, baseline status, and waivers. The
coupling gate promotes only `bc/imbalanced_coupling`, never the advisory.

S=9 with typical D=4 (same-owner) and V=3 (low volatility):
`max(|9−4|, 10−3) + 1 = max(5, 7) + 1 = 8` → `low` severity. Correct: same-owner
clone pairs are a DRY smell but not a distributed-monolith crisis. The critical
case is a cross-deploy-unit symmetric edge (S=9, D=9, V=10):
`max(0, 0) + 1 = 1` → `critical`.

#### 4.2 Pure-data DTO → Contract (v4, Go only)

Strict, statically decidable definition: an exported struct with at least one
field, every field exported, no behavior-carrying fields — func, chan, or
interface anywhere in the field's type structure; composite element types
(`[]func()`, `map[string]chan T`, `*Iface`) and nested struct fields are
recursed into — and an EMPTY method set (value + pointer receivers, promoted
methods from embedding included; zero-field marker structs are NOT DTOs). Referenced across
a config-declared `public:` glob boundary, it upgrades the public-glob floor
from not-intrusive to `contract` — the boundary declaration is what makes it a
contract, matching the book's "explicit integration contract". A DTO not
crossing a declared public boundary stays `model`; an internal-glob match stays
authoritative intrusive. The extractor emits a context-dependent hint `dto`
(`graph.StrengthHintDTO`, rank between contract and model); a qualifying DTO's
fields carry the same hint so composite-literal keys and selector reads don't
outrank the type reference. TS/Py/Rust have no equivalent static signal and
abstain (no fabricated contract upgrades) — Wave 7's LLM labels are the
designed path for those languages.

### Connascence roadmap

Connascence is a report-only evidence vocabulary, not a scoring input.
`connascence.roadmap` makes the current book-alignment boundary explicit:

| Kind                            | Status                                                                                       | Gate effect |
| ------------------------------- | -------------------------------------------------------------------------------------------- | ----------- |
| name/type/meaning/algorithm     | deterministic static                                                                         | none        |
| position                        | unmeasured static gap unless deterministic argument/order evidence appears                   | none        |
| execution/timing/value/identity | unmeasured dynamic gap; `dynamic_imports` and `runtime_async_edges` are only related signals | none        |

An LLM may explain the roadmap or draft follow-up labels/docs, but it cannot turn
an unmeasured dynamic category into strength, distance, volatility, score,
finding, baseline, or verdict input. The upgrade trigger is a deterministic
source-module→runtime fact precise enough to cite and test.

### Distance mapping

Composite of three signals (first applicable in priority order), applied to
edges whose target resolves to a declared module:

1. Same module → `same_module` (D=2 — scored into `local_coupling` only, see §2).
2. Different `deploy_unit` values → `cross_deploy_unit` (D=9).
3. Ownership is informative (two or more distinct owners) → same owner →
   `cross_module_same_owner` (D=4); different owners → `cross_module_diff_owner` (D=7).
4. Code structure (always available) → sibling/parent-child subtree →
   `cross_module_same_owner` (D=4); unrelated subtrees →
   `cross_module_diff_owner` (D=7).

When the target resolves to NO module (module resolution always wins), a match
against a declared `external_systems:` entry assigns `declared_external`
(D=10) and the edge ENTERS scoring; otherwise the edge keeps the disclosed
external exclusion (§6).

Runtime async bridge: evidence recorded in `runtime_async` JSON field per
module and `runtime_async_edges` per source-module→runtime-target relation.
**Does not annotate graph edges, does not affect D, does not affect the score or
gate verdict.** Report-only by design.

Each classified cross-boundary edge records `distance_basis` when a concrete
signal selected the rung: `code_structure`, `ownership`, `deploy_unit`, or
`declared_external`. The run summary rolls these into
`classified_edges.by_distance_basis`. The companion
`classified_edges.distance_compression` block lists implemented rungs, omitted
compressed rungs, deterministic split decisions, and per-rung omission reasons.
D=8 remains omitted by policy: undeclared library/package imports stay excluded,
while explicitly declared `external_systems:` seams score at D=10. No D=8
library seam is invented from package-manager shape alone.

### Volatility mapping

`subdomain` or explicit `volatility` in config → book anchor (table in §3):
core → high, supporting → low, generic → low (medium is reachable only via an
explicit `volatility: medium` override). There is NO path/name guessing — volatility
is never inferred from a directory name. When neither `subdomain` nor `volatility`
is declared, the module is `undeclared`: the volatility rescue term (`10 − V`) is
computed with V=10 (conservative, cannot confirm low volatility) and an `agent_task`
is emitted asking the human to declare the subdomain.

A declared `external_systems:` entry defaults to `volatility: low` (the book's
generic-subdomain guidance) and accepts an explicit override
(high|medium|low|frozen).

The inferred-volatility cascade (opt-in, `coupling.volatility_cascade: true`, book
Ch9) propagates high volatility to a deterministic fixpoint across strongly-coupled
edges before scoring. It only raises effective volatility and excludes clone-only
pairs because duplicated code is accidental coupling evidence, not a domain/runtime
dependency.

### Repo rollup

`coupling_balance` dimension value = `round(100 × (mean book balance − 1) / 9)`
over all scored internal cross-boundary coupling facts: graph edges plus
clone-only duplicated-knowledge pairs when `coupling.duplicated_knowledge: score`.
Confidence starts from the internal scored fraction (see §5), then high
confidence is disallowed for tiny samples: fewer than 5 scored internal
cross-boundary facts or fewer than 3 connected modules caps the dimension at
medium confidence and appends an evidence line. The cap changes confidence only;
it does not change the numeric score, band, or `coupling.gate` decision.

The mean is paired with `classified_edges.tail_risk`: worst balance,
lower-decile balance, high-or-worse share, critical count, and
distributed-monolith count. Clone-only duplicated-knowledge pairs that enter the
score under `coupling.duplicated_knowledge: score` are counted in the same tail
summary with clone-only subcounts, so copy-paste evidence is visible without
inventing graph edges or changing the book formula. Worst edges still surface as
advisories. This is transparent aggregation of the book's own per-edge score,
not a new coupling model.

---

## 5. The ABSTAIN rule

The book has no "unknown" level. Where archfit cannot place an edge, it
**abstains** — the edge is excluded from scoring (`EdgeScore.Scored = false`).
It is never assigned an invented ordinal.

Abstain conditions:

- `Strength == StrengthUnknown` **OR** `Distance == DistanceUnknown` → edge
  unscored. Both conditions must be resolvable to score the edge. A
  declared-external distance (D=10) does not rescue an unknown strength — the
  edge still abstains.
- `Distance == DistanceUnknown` additionally means the target is an
  external/library import (stdlib, third-party package, undeclared module) not
  covered by any `external_systems:` declaration — see §6.

When strength is unknown and distance is known (genuine internal edge with
unclassified strength), the edge stays in the internal `abstained` bucket and
lowers `coupling_balance` confidence. An `agent_task` is emitted to prompt
strength classification.

---

## 6. External edges: declared seams score, the rest are excluded

Edges whose target does not resolve to a declared module fall in two buckets:

- **Declared** (v4): the target matches a `external_systems:` entry — named
  config entries with `targets:` globs matched against the classified edge
  target (language-independent: Go import path, TS resolved package path or
  bare specifier, Python dotted module, Rust crate name) and optional
  `volatility:` (default low). These edges get `DistanceExternal = 10`
  (`distance_basis: declared_external`) and ENTER scoring — the book's Ch10
  Example 1 (cross-vendor integration) lives at the far end of the ladder, not
  outside it. Their count surfaces in `classified_edges.declared_external`.
- **Undeclared**: excluded from the `coupling_balance` scored/abstained
  distribution. They are NOT abstained internal edges — they are external
  imports (stdlib, third-party packages, undeclared internal packages treated
  as external). Scoring EVERY library import at D=10 would flood the metric
  with vendor noise; the book's example is a _declared integration seam_.
  External dependency hygiene belongs in linter/dependency tooling. The count
  is surfaced transparently in `classified_edges.external` (JSON), Markdown
  **Distance confidence**, and the `coupling_balance` evidence string.

Language-agnostic: both buckets key on `DistanceUnknown`, which every language
extractor sets for unresolved targets; the `external_systems:` match runs only
on those edges.

Strength sourcing for declared seams (abstain-not-fake applies — a declared
edge whose strength stays `unknown` is counted in `declared_external` and
`abstained`, lowering confidence, but is never scored with an invented
ordinal):

- **Go**: type-info strength hints cover external targets too (stdlib and
  third-party symbols resolve through the same `TypesInfo.Uses` pass as
  first-party ones), so a declared Go seam scores with compiler-grade strength.
- **TS**: dependency-cruiser hints apply to every dependency (`import type` →
  contract, value import → functional), external targets included.
- **Python**: grimp has no object kinds — declared seams abstain, same as every
  other Python edge.
- **Rust**: `cargo metadata` is manifest-granularity; a `Cargo.toml` dependency
  edge carries no symbol information, so a declared Rust seam abstains on
  strength. Deriving a hint from real symbol use (rust-analyzer SCIP) is the
  upgrade path; fabricating a conservative default is not.

Self-scan result (v5 final validation, 2026-07-05): 572 external edges excluded
(0 declared — archfit declares no external systems), 350 internal edges scored.

---

## 7. LLM placement (unchanged from v2, enforced structurally)

| Stage                     | LLM?          | Role                                                          |
| ------------------------- | ------------- | ------------------------------------------------------------- |
| `check`                   | **never**     | reads pinned config + tool facts only                         |
| `config enrich labels`    | yes, off-gate | draft → human review → approved → `analyze` consumes          |
| `config enrich subdomain` | yes, off-gate | draft subdomain/volatility → human review → apply into config |
| `explain <fingerprint>`   | yes, off-gate | narrate finding in prose                                      |
| `analyze --ai-summary`    | yes, off-gate | LLM architecture narrative from gathered evidence             |

`arch_test.go` structurally forbids any `internal/*` package from importing
`internal/llm`, so LLM code is confined to `cmd/archfit`.

### Label confidence and provenance

Labels in `.archfit-labels.yaml` carry confidence and provenance:

```yaml
- from: internal/engine
  to: internal/classify
  strength: functional
  status: approved
  confidence: medium # high | medium | low
  provenance: llm # human | llm | tool
  reviewed_by: "@architect"
  reviewed_at: "2026-06-23"
```

- `provenance: llm` with `confidence` below `high` causes `coupling_balance`
  confidence to drop by one band (implemented via `labels.LLMApprovedCount`).
  Rationale: LLM judgment has been human-reviewed but is not as certain as a
  config-glob or SCIP symbol-kind classification.
- `provenance: human` or `provenance: tool` does not lower confidence.
- `analyze` consumes only `status: approved` labels. Draft labels are never read
  by the gate.

### Decision tasks (abstain → agent_task)

When an edge abstains due to unknown strength (internal edge, distance known):
an `agent_task` is emitted in JSON/SARIF with the from→to pair and a prompt to
add a label in `.archfit-labels.yaml`. This surfaces classification gaps to the
human or LLM operator without blocking the gate.

Similarly, undeclared subdomain/volatility modules emit a decision task prompting
`subdomain:` declaration or `archfit config enrich subdomain`.

---

## 8. archfit self-scorecard (Wave 1 final validation, 2026-07-05)

| Dimension        | Score | Band  | Confidence |
| ---------------- | ----: | ----- | ---------- |
| coupling_balance |    43 | mixed | high       |

354 scored internal cross-boundary edges, 0 abstained, 591 external/library edges
excluded, 11 clone-only pairs scored, and 38 connected modules in the coupling
sample. Movement from the Wave 1 deterministic baseline is 42 → 43 with no band
change; v5's gain came from clone-only duplicated-knowledge scoring, and v6 keeps
the score stable while making inferred volatility transitive. The v3/v4/v5/v6
score versions are not comparable by design; accept v6 by reviewing the attribution
table and re-running `archfit baseline` only when your configured gates need a new
anchor.

---

## 9. Non-goals and rejected designs

- **runtime_adjust / +1 async distance:** runtime async detection is
  report-only. Never modifies distance, never annotates graph edges, never gates.
  (`runtime_async` and `runtime_async_edges` JSON fields are evidence only.)
- **Invented ordinals for unknown edges:** abstain instead. No `strengthOrdinalUnknown`
  or `distanceOrdinalUnknown` invented values remain.
- **Scoring every library import at D=10:** rejected as vendor noise — the
  D=10 rung is reachable only through a declared `external_systems:` entry;
  the undeclared remainder stays a disclosed exclusion.
- **Separate VolatilityLegacy constant:** rejected for now. `legacy` is accepted
  as a config alias for `VolatilityFrozen`; add a separate constant only if it
  needs distinct scoring or reporting semantics.
- **gap-6 exemption (composition_root god-module):** explicitly rejected to keep
  the scorer honest. `cmd/archfit` is rightly flagged as a god module.
