# Balanced Coupling measurement engine — design v3.0

Date: 2026-06-23. Status: SHIPPED. Supersedes `bc-measurement-v2.md` (the v2
design described the measurement gaps and two prototype scorers; v3 records what
was actually built and why).

Related plan: `docs/plans/20260623-bc-book-alignment.md` (Tasks 1–16).

---

## 1. What shipped

archfit now implements Vlad Khononov's _Balancing Coupling in Software Design_
**verbatim** — the published per-edge formula and ordinal anchors — instead of
a homegrown multiplicative scorer. The bespoke v2 formula
(`risk = (1 − |Sₙ − Dₙ|) × Vₙ`, invented ordinals `0/2/3/5/8`) has been
replaced. archfit owns only the _instrumentation_ — measuring code and placing
each edge on the book's scale.

`ScoreVersion = "bc_score.v3"` — a breaking metric change: scores shift by design.

---

## 2. The book formula (authoritative — do not deviate)

From Khononov, Ch10 (MIN=1, MAX=10, higher = better balanced):

```
modularity = |S − D|                          // 0..9
balance    = max(modularity, 10 − V) + 1      // 1..10
```

Same-module edges (`same_module` distance): balance = 10 (cohesion, not
cross-boundary coupling). Edge is still `Scored: true` so it counts in the
distribution but never as a penalty.

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

`StrengthSymmetric` (S=9) is new in v3; it represents duplicated functionality
(clone-detected DRY violation) — see §4.

### Distance

| Constant                       | Book anchor (approx.)        | Ordinal (D) |
| ------------------------------ | ---------------------------- | ----------- |
| `DistanceSameModule`           | same object                  | 2           |
| `DistanceCrossModuleSameOwner` | different packages, same org | 4           |
| `DistanceCrossModuleDiffOwner` | different packages, diff org | 7           |
| `DistanceCrossDeployUnit`      | services / different deploy  | 9           |

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

| Signal                                                                                    | Strength assigned                                               |
| ----------------------------------------------------------------------------------------- | --------------------------------------------------------------- |
| Target in `public:` globs or SCIP Protocol/ABC/interface symbol kind                      | `contract` (S=1)                                                |
| SCIP struct/class symbol kind (shared data model)                                         | `model` (S=3)                                                   |
| SCIP function/method reference or config-declared `functional`                            | `functional` (S=8)                                              |
| Cross-module clone pair detected (see §4.1)                                               | `symmetric` (S=9)                                               |
| Target in `internal:` globs, Go `internal/`, SCIP private, or config-declared `intrusive` | `intrusive` (S=10)                                              |
| Pinned approved label (`.archfit-labels.yaml`)                                            | label value wins — overrides extractor for `functional`/`model` |
| Nothing classifies the edge                                                               | `unknown` → **abstain** (see §5)                                |

`contract` and `intrusive` from config globs are config-authoritative: pinned
labels cannot override them.

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

### Distance mapping

Composite of three signals (first applicable in priority order):

1. Same module → `same_module` (D=2; balance=10, scored as cohesion).
2. Different `deploy_unit` values → `cross_deploy_unit` (D=9).
3. Ownership is informative (two or more distinct owners) → same owner →
   `cross_module_same_owner` (D=4); different owners → `cross_module_diff_owner` (D=7).
4. Code structure (always available) → sibling/parent-child subtree →
   `cross_module_same_owner` (D=4); unrelated subtrees →
   `cross_module_diff_owner` (D=7).

Runtime async bridge: evidence recorded in `runtime_async` JSON field per
module. **Does not annotate graph edges, does not affect D, does not affect the
score or gate verdict.** Report-only by design (Task 4 decision).

### Volatility mapping

`subdomain` or explicit `volatility` in config → book anchor (table in §3):
core → high, supporting → low, generic → low (medium is reachable only via an
explicit `volatility: medium` override). There is NO path/name guessing — volatility
is never inferred from a directory name. When neither `subdomain` nor `volatility`
is declared, the module is `undeclared`: the volatility rescue term (`10 − V`) is
computed with V=10 (conservative, cannot confirm low volatility) and an `agent_task`
is emitted asking the human to declare the subdomain.

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
  unscored. Both conditions must be resolvable to score the edge.
- `Distance == DistanceUnknown` additionally means the target is an
  external/library import (stdlib, third-party package, undeclared module) — see §6.

When strength is unknown and distance is known (genuine internal edge with
unclassified strength), the edge stays in the internal `abstained` bucket and
lowers `coupling_balance` confidence. An `agent_task` is emitted to prompt
strength classification.

---

## 6. External/library edge exclusion

Edges whose target does not resolve to a declared module (`Distance ==
DistanceUnknown`) are **excluded from** the `coupling_balance` scored/abstained
distribution. They are NOT abstained internal edges — they are external imports
(stdlib, third-party packages, undeclared internal packages treated as external).

External dependency coupling is outside the `coupling_balance` measurement
boundary — it belongs in linter and dependency-hygiene tooling. Mixing them
artificially deflated the scored fraction and lowered confidence on what is
actually a well-classified internal graph.

The count is surfaced transparently in `classified_edges.external` (JSON) and in
the `coupling_balance` evidence string. Language-agnostic: keys on
`DistanceUnknown`, which every language extractor sets for unresolved targets.

Self-scan result: 447 external edges excluded, 89 internal edges scored (89/89
scored fraction → `high` confidence, value 78/100).

---

## 7. LLM placement (unchanged from v2, now enforced structurally)

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

Labels in `.archfit-labels.yaml` carry two new fields (added in Task 6):

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

## 8. archfit self-scorecard (Task 15, 2026-06-23)

After the full book-model alignment (Tasks 1–15):

| Dimension        | Score | Band        | Confidence |
| ---------------- | ----- | ----------- | ---------- |
| coupling_balance | 78    | serviceable | high       |

Edge distribution (89 scored internal cross-boundary edges):

- Strength: all `contract` or `symmetric` — no `functional` or `intrusive`.
- Distance: all `cross_module_same_owner` (single-owner repo; code-structure
  baseline applies).
- Volatility: all `low` (supporting/generic subdomains).
- Severity: 84 `none`, 5 `low` (symmetric clone edges). No criticals.

---

## 9. Non-goals and rejected designs

- **runtime_adjust / +1 async distance:** runtime async detection is
  report-only. Never modifies distance, never annotates edges, never gates.
  (Task 4 decision — `runtime_async` JSON field is evidence only.)
- **Invented ordinals for unknown edges:** abstain instead. No `strengthOrdinalUnknown`
  or `distanceOrdinalUnknown` invented values remain.
- **VolatilityFrozen/VolatilityLegacy in code today:** book anchor=1, but no
  constant exists yet — all archfit modules are in active development. Add when a
  genuinely stable module requires it.
- **gap-6 exemption (composition_root god-module):** explicitly rejected to keep
  the scorer honest. `cmd/archfit` is rightly flagged as a god module.
