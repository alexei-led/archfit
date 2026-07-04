# Balanced Coupling measurement engine — design v4.0

Date: 2026-07-04. Status: SHIPPED. Supersedes `bc-measurement-v3.md` (archived
under `docs/archived/design/`). The v3 doc recorded the move to Khononov's
published formula; v4 changes ONLY the classification feeding that formula —
the formula, bands, and abstain discipline are unchanged. Delta note with the
exact three-change list: `20260702-bc-score-v4.md`.

Related plan: `docs/plans/20260702-wave4-book-strength-distance.md` (Tasks 1–6).

---

## 1. What shipped

archfit implements Vlad Khononov's _Balancing Coupling in Software Design_
**verbatim** — the published per-edge formula and ordinal anchors — since v3.
archfit owns only the _instrumentation_ — measuring code and placing each edge
on the book's scale.

`ScoreVersion = "bc_score.v4"` — a breaking metric change: **v4 scores are not
comparable to v3 scores.** v4 fixes three known-wrong classifications found by
the 2026-07-02 eval (`reports/eval-2026-07-02-v1.1.2/00-FINDINGS.md` §1
deviations 1–3):

1. Go const/var reads score Model (S=3), not Functional (S=8) — pure data
   sharing per book Ch7. Rust gets the same fix via the rust-analyzer SCIP
   term kinds (const/static/field → model).
2. Pure-data Go DTOs referenced across a declared `public:` glob boundary
   reach Contract (S=1) — the book's canonical Contract example.
3. New frozen distance rung `DistanceExternal = 10` (book Ch10 Example 1,
   cross-vendor integration) for config-declared `external_systems:` seams.

---

## 2. The book formula (authoritative — do not deviate)

From Khononov, Ch10 (MIN=1, MAX=10, higher = better balanced):

```
modularity = |S − D|                          // 0..9
balance    = max(modularity, 10 − V) + 1      // 1..10
```

Same-module edges (`same_module` distance) are cohesion, not cross-boundary
coupling: `BookScorer.Score` returns them unscored (`Scored: false`), so they
never enter the distribution. This hides the book's Ch10 "local complexity"
quadrant (low strength + low distance) — a known deviation (2026-07-02 eval §1
deviation 4) scheduled for Wave 5
(`docs/plans/20260702-wave5-book-quadrants-advisories.md`).

Severity bands derived from balance:

| balance | band       |
| ------- | ---------- |
| 1–2     | `critical` |
| 3–4     | `high`     |
| 5–6     | `medium`   |
| 7–8     | `low`      |
| 9–10    | `none`     |

Implemented in `internal/model/coupling/scorer_book.go` as `BookScorer`.
`ScoreVersion` is defined in `internal/model/coupling/scorer.go`.

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

### Volatility

| Source                        | Book anchor           | Ordinal (V)       |
| ----------------------------- | --------------------- | ----------------- |
| `subdomain: core`             | core                  | 10                |
| explicit `volatility: high`   | core (override)       | 10                |
| `subdomain: supporting`       | supporting            | 3                 |
| `subdomain: generic`          | generic               | 3                 |
| explicit `volatility: low`    | supporting/generic    | 3                 |
| explicit `volatility: medium` | (interpolation)       | 6                 |
| `frozen` / `legacy` (future)  | frozen/legacy         | 1                 |
| undeclared / unknown          | → abstain rescue term | 10 (conservative) |

Note: `VolatilityFrozen` / `VolatilityLegacy` constants do not yet exist in the
codebase (all archfit modules are actively developed). The table documents the
mapping for when they are added.

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

### Distance mapping

Composite of three signals (first applicable in priority order), applied to
edges whose target resolves to a declared module:

1. Same module → `same_module` (cohesion — unscored, see §2).
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
module. **Does not annotate graph edges, does not affect D, does not affect the
score or gate verdict.** Report-only by design.

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
Ch9) propagates high volatility one hop across strongly-coupled edges before scoring.

### Repo rollup

`coupling_balance` dimension value = `round(100 × (mean book balance − 1) / 9)`
over all scored internal cross-boundary edges. Confidence from the internal scored
fraction (see §5). Worst edges surface as advisories. This is transparent
aggregation of the book's own per-edge score, not a new coupling model.

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
  is surfaced transparently in `classified_edges.external` (JSON) and in the
  `coupling_balance` evidence string.

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

Self-scan result (v4, 2026-07-04): 513 external edges excluded (0 declared —
archfit declares no external systems), 292 internal edges scored.

---

## 7. LLM placement (unchanged from v2, enforced structurally)

| Stage                     | LLM?          | Role                                                          |
| ------------------------- | ------------- | ------------------------------------------------------------- |
| `analyze --gate`          | **never**     | reads pinned config + tool facts only                         |
| `config enrich labels`    | yes, off-gate | draft → human review → approved → `analyze` consumes          |
| `config enrich subdomain` | yes, off-gate | draft subdomain/volatility → human review → apply into config |
| `explain <fingerprint>`   | yes, off-gate | narrate finding in prose                                      |
| `analyze --llm`           | yes, off-gate | LLM architecture narrative from gathered evidence             |

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

## 8. archfit self-scorecard (Wave 4 Task 5, 2026-07-04)

| Dimension        | Score | Band | Confidence |
| ---------------- | ----- | ---- | ---------- |
| coupling_balance | 40    | poor | high       |

292 scored internal cross-boundary edges, 0 abstained, 513 external excluded.
The pre-wave value was 39/poor (same band — no re-baseline needed); the +1 is
Task 2's const/var → model fix, and Task 3's DTO upgrade moved 17 edges
model → contract (below score granularity). The v3-era 78/serviceable
self-score predates the opt-in volatility cascade (now enabled in self-config)
and later classification fixes — v3 and v4 numbers are not comparable by
design.

---

## 9. Non-goals and rejected designs

- **runtime_adjust / +1 async distance:** runtime async detection is
  report-only. Never modifies distance, never annotates edges, never gates.
  (`runtime_async` JSON field is evidence only.)
- **Invented ordinals for unknown edges:** abstain instead. No `strengthOrdinalUnknown`
  or `distanceOrdinalUnknown` invented values remain.
- **Scoring every library import at D=10:** rejected as vendor noise — the
  D=10 rung is reachable only through a declared `external_systems:` entry;
  the undeclared remainder stays a disclosed exclusion.
- **VolatilityFrozen/VolatilityLegacy in code today:** book anchor=1, but no
  constant exists yet — all archfit modules are in active development. Add when a
  genuinely stable module requires it.
- **gap-6 exemption (composition_root god-module):** explicitly rejected to keep
  the scorer honest. `cmd/archfit` is rightly flagged as a god module.
