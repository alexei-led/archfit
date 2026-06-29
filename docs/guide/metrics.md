# Metrics reference

This page documents every signal `archfit` computes: what it represents, why it
exists, how it is scored, and whether it can affect the verdict. For the theory
behind the strength / distance / volatility vocabulary used throughout, read
[Concepts](concepts.md) first.

`archfit` measures **Balanced Coupling** (`coupling_balance`) plus a minimal set of
complementary metrics. They split into two roles:

- **Verdict-affecting (2):** `coupling_balance` (scored 0–10, the headline), and
  `unbalanced_edge` (companion count).
- **Report-only (4):** `cycle`, `blast_radius`, `encapsulation`, `coverage`. Band
  `info`; surface facts for humans and agents, never change the verdict.

No metric ever fails the build on its own. Only explicit **gate rules**
(forbidden dependency, public-API-only, layer direction, cycle-as-fail, expired
exception) produce a `fail`. Metrics inform; rules gate. This separation is
deliberate — see [Concepts → How archfit operationalizes the model](concepts.md#how-archfit-operationalizes-the-model).

---

## The verdict

After all metrics run, the run gets one verdict
(`internal/engine/engine.go`, `computeVerdict`):

```text
fail  → any gate finding with status "new" or "expired_waiver"
warn  → otherwise, any metric whose delta vs baseline is negative
pass  → otherwise
```

Exit codes: `0` pass, `1` gate failed, `2` warnings/regressions (non-blocking by
default; use `--report` to never exit non-zero on these), `3` tool/config error.

---

## Scoring model

The verdict-affecting metrics produce a 0–10 value, a band, and a confidence.

### Bands

| Band          | Score    | Meaning                                                                                   |
| ------------- | -------- | ----------------------------------------------------------------------------------------- |
| `strong`      | 9.0–10.0 | Healthy.                                                                                  |
| `serviceable` | 7.0–8.9  | Acceptable.                                                                               |
| `mixed`       | 5.0–6.9  | Watch.                                                                                    |
| `poor`        | 3.0–4.9  | Problem.                                                                                  |
| `critical`    | 0.0–2.9  | Measured and bad.                                                                         |
| `n/a`         | —        | No signal to measure. Not good, not bad — _no evidence_. Never conflated with `critical`. |
| `info`        | —        | Report-only fact; asserts no quality verdict.                                             |

The `n/a` vs `critical` distinction is load-bearing. A repo that declares no
public/internal API surface has nothing to score for encapsulation; that is
`n/a`, not a `critical` failure. Reporting a confident bad score on absent
evidence is a false alarm the tool refuses to raise.

### Confidence caps the band

Confidence (`high` / `medium` / `low`) reflects how much of the needed evidence
was actually available (coverage, classified fraction, sample size). It can only
_lower_ the reported band, never raise it
(`internal/metrics/internal/result`, `ApplyConfidenceCap`):

```text
high   → band may reach "strong"
medium → band capped at "serviceable"
low    → band capped at "mixed"
```

So a metric cannot claim `strong` on thin evidence. This is why low extraction
coverage quietly pulls every dependent metric's ceiling down instead of letting
the tool over-claim.

### Deltas

When a committed baseline exists (`.archfit-baseline.json`, written by `archfit
baseline`), each scored metric is compared with that snapshot; a negative delta
(the metric got worse) sets the run to `warn`. Report-only metrics carry no delta
and never warn. The `--base <ref>` flag compares `coupling_balance` and metric
scores against a git ref.

---

## Verdict-affecting metrics

### `coupling_balance` (headline metric)

> **Scorer version:** `bc_score.v3` — Khononov Ch10 book formula.

- **Represents:** how well the distribution of coupling across module boundaries
  respects the strength × distance × volatility balance rule. High score means
  most edges carry low maintenance cost; low score means expensive, high-risk
  couplings dominate.
- **Formula:** for each scored internal edge,
  `balance = max(|S − D|, 10 − V) + 1` where `S` = strength ordinal, `D` =
  distance ordinal, `V` = volatility ordinal (Khononov Ch10 verbatim). Ordinals
  are frozen named constants — changing any is a breaking metric change.
- **Abstain-not-fake:** when strength OR distance is `unknown`, the edge is
  unscored (`EdgeScore.Scored = false`). No invented ordinals. Genuine internal
  edges with unknown strength stay in the `abstained` bucket (lowers confidence).
  External/library edges (`DistanceUnknown`) are excluded from the denominator
  entirely and counted in `classified_edges.external`.
- **Scored:** the distribution of balance values across scored edges → band.
  Confidence scales with the fraction of classified edges; empty edges with low
  coverage → low confidence, band capped at `mixed`.
- **Affects verdict:** `warn` when `coupling_balance` drops vs baseline.
- **Opt-in cascade:** `coupling.volatility_cascade: true` enables a single-hop
  propagation pass (book Ch9) that raises effective volatility to `high` for
  modules strongly coupled to a `core` module.

### `unbalanced_edge`

> **Breaking change (v0.3.0):** `metric_version` bumped to `unbalanced_edge.v2` — the
> distance composite (code-structure + deploy-unit + degenerate-owner suppression) and
> the removal of git-churn from gate volatility changed the metric's input semantics.
> Re-run `archfit baseline` if you have a pinned baseline from v0.2.x or earlier.

- **Represents:** count of **new, high-risk** imbalanced edges — the worst corner
  of the balance rule.
- **Computed:** an edge qualifies when it is `intrusive` **and** at distance
  ≥ `cross_module_different_owner` **and** `high` volatility **and** new (not
  already in the baseline). The reported value is the count of such edges.
- **Scored:** `0` qualifying edges → `strong`; any → `critical`. If intrusive
  cross-module candidates exist but none has known volatility → `n/a` (honest
  indeterminate, not a clean zero).
- **Affects verdict:** `warn` when the count rises vs baseline. Can be promoted to
  a hard gate by configuring `gate: fail` on the rule.
- **Balanced Coupling:** the most direct encoding of the model — all three
  dimensions at their high settings.

---

## Report-only metrics

These always report band `info`. They never set a delta and never change the
verdict.

### `cycle`

- **Represents:** number of import cycles among modules/packages.
- **Computed:** Tarjan strongly-connected components; each SCC of size > 1 is one
  cycle.
- **Band:** always `info`. Confidence always `high` (cycles are a fact, not an
  inference). The `cycle` rule with `gate: fail` makes new cycles a hard failure.
- **Balanced Coupling:** none — a graph-topology fact, not a strength/distance call.

### `blast_radius`

- **Represents:** structural change-impact concentration — modules that a large
  fraction of the codebase transitively depends on.
- **Computed:** for each first-party module, the count of other modules that
  transitively reach it (over the cycle-condensed DAG, so cycles don't inflate
  it); flagged a hub when that share ≥ **0.30** of modules.
- **Why report-only:** a hub is a fact, not a defect. A widely-used stable utility
  is good design. The number tells you _where_ a change ripples, not that anything
  is wrong.

### `encapsulation`

- **Represents:** of the cross-boundary edges that take a stance on boundary
  respect, the fraction that go through a declared contract instead of reaching
  into internals.
- **Computed:** `contract_cross / (contract_cross + intrusive_cross)`, counting
  only edges classified `contract` or `intrusive` at a real cross-boundary
  distance. `functional` and `model` (normal public coupling) and `unknown`
  (no evidence) are excluded from the denominator, not counted against the score.
- **Why this denominator:** counting every public function call as a strike would
  crush the ratio for any normal codebase. The metric asks one question — _when
  code crosses a boundary, does it use the front door?_ — and ignores edges that
  are neither a contract nor a leak.
- **Band:** always `info`. No cross-boundary edges → ratio `1.0`. Cross-boundary
  edges exist but none is contract/intrusive → `n/a`. Contract-only with zero
  intrusive on a compiler that forces exported access (Go/TS) → `n/a`, because
  100% there is not _earned_. Confidence scales with the classified fraction of
  cross-boundary edges.
- **Balanced Coupling:** strength (`contract` vs `intrusive`), filtered by distance.

### `coverage`

- **Represents:** the fraction of applicable files the extractors actually
  processed — the trust signal for every other metric.
- **Computed:** `extracted / applicable` across all tool-coverage records. Zero
  applicable (extractors ran, nothing matched) → `1.0`. **No extractor ran at
  all** (no coverage record) → `n/a`, not `1.0` — absence of evidence is never
  scored as full coverage. This is the load-bearing distinction that stops an
  unanalysed repo from reporting confident health.
- **Band:** always `info`. Confidence from the unresolved ratio (≤5% → high,
  ≤20% → medium, else low). Low coverage caps the band of every metric that
  depends on the missing evidence.

---

## Coupling classification reference

Every cross-boundary edge is classified on the four lenses below
(`internal/model/coupling/coupling.go`). These power the
`bc/imbalanced_coupling` advisories and feed `encapsulation` and
`unbalanced_edge`.

| Lens         | Values (ordered)                                                                                              | Derived from                                                                                       |
| ------------ | ------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| Strength     | `contract` < `model` < `functional` < `intrusive` (+`unknown`)                                                | public/internal globs, visibility, SCIP symbol kind, pinned labels                                 |
| Distance     | `same_module` < `cross_module_same_owner` < `cross_module_different_owner` < `cross_deploy_unit` (+`unknown`) | module map, `owner`, `deploy_unit`                                                                 |
| Volatility   | `low` < `medium` < `high` (+`undeclared`, `unknown`)                                                          | explicit `volatility:`, then `subdomain:`; else `undeclared` (no path/name guessing, no git churn) |
| Explicitness | `explicit`, `implicit` (+`unknown`)                                                                           | strength (contract→explicit, intrusive→implicit) or AST hint                                       |
| Severity     | (none) < `low` < `medium` < `high` < `critical`                                                               | the balance rule over the four above                                                               |

For the full severity table and the reasoning, see
[Concepts → The balance rule](concepts.md#the-balance-rule).

---

## Rules reference

Gate rules are the only mechanism that produces a `fail`. Metrics inform; rules
gate. Rules with `gate: warn` are advisory (non-blocking); rules with `gate: fail`
block the build. An unknown `type` value is a config error.

Rules kept in this release:

| Rule                          | Default gate | Notes                                                                                                                                                                                                                     |
| ----------------------------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `forbidden_dependency`        | `fail`       | Blocks explicit banned import pairs.                                                                                                                                                                                      |
| `forbidden_layer_direction`   | `fail`       | Enforces layer ordering (lower layers must not import higher).                                                                                                                                                            |
| `public_api_only`             | `fail`       | Enforces that cross-module callers use only declared public API.                                                                                                                                                          |
| `internal_api_access`         | `fail`       | Blocks access to internal API from outside the declared boundary.                                                                                                                                                         |
| `new_cross_module_dependency` | `fail`       | Fires on cross-module edges; baseline suppresses known ones so only new edges fire.                                                                                                                                       |
| `cycle`                       | `fail`       | New import cycles. Demote to `warn` with `gate: warn` if desired.                                                                                                                                                         |
| `public_api_change`           | `warn`       | Detects breaking changes in declared public API; defaults to `warn` when unset.                                                                                                                                           |
| `public_api_type_leak`        | `warn`       | Fires when a public API exposes a type from an external framework package in a signature. Defaults to `warn`. Requires `analyzers.syntax.enabled: true`. Only fires on repos with Go-style dotted external package nodes. |
| `public_api_max`              | `fail`       | Caps the number of public symbols on a module.                                                                                                                                                                            |

For full rule configuration syntax and examples, see
[Configuration reference](configuration-reference.md#rules).

---

## Per-language behavior

The deterministic gates and core metrics work from the built-in extractors plus
`git`. Supported languages and setup live in [Language support](languages.md).
Coverage and the optional metrics differ by language; when a tool is missing the
dependent metric reports `n/a` **with the reason and enable step** — never a
false failure.

| Signal                       | Go            | TypeScript / JS                          | Python                                   |
| ---------------------------- | ------------- | ---------------------------------------- | ---------------------------------------- |
| Dependency graph + gates     | `go/packages` | dependency-cruiser                       | `grimp` (dotted modules)                 |
| Node-ID scheme               | `file:`       | `file:`                                  | `module:` (incl. `src/` layout)          |
| SCIP edge strength           | `scip-go`     | `scip-typescript` (needs `node_modules`) | `scip-python`                            |
| type-only vs runtime edges   | n/a           | tagged (→ Contract strength)             | n/a                                      |
| Dynamic / lazy import signal | n/a           | `require()` / dynamic `import()`         | in-function / `importlib` / `__import__` |

SCIP refines edge strength feeding `coupling_balance` and `encapsulation` — it is
not needed for gate rules, which work from the built-in extractor graph. For
install matrix and per-language tool setup, see
[Language support → optional analyzers](languages.md#optional-analyzers-per-language)
and [Tooling reference](tooling.md).

---

## Tool requirements

Most metrics work from the built-in extractors and `git`. A few need an opt-in
tool and report `n/a` (with a coverage note) when it is absent — never a false
failure.

| Metric(s)                                                                       | Needs                                             |
| ------------------------------------------------------------------------------- | ------------------------------------------------- |
| coupling_balance, unbalanced_edge, cycle, blast_radius, encapsulation, coverage | built-in extractors + `git`                       |
| coupling_balance (strength refinement)                                          | SCIP index (`analyzers.scip.enabled: true`)       |
| coupling_balance (clone → symmetric strength)                                   | clone detector (`analyzers.clones.enabled: true`) |
| public_api_max, public_api_change, public_api_type_leak (rules)                 | `sg` (ast-grep); `analyzers.syntax.enabled: true` |

The `llm` tool is used only by `archfit enrich`, `archfit explain --llm`,
`archfit analyze --llm`, `archfit autopilot`, `archfit init --llm`, and
`archfit update --llm`. It is **never** consumed by the deterministic gate path —
gate verdicts and metric values stay deterministic. See
[LLM enrichment](llm-enrich.md).

---

## Design rationale: what archfit does not measure

`archfit` deliberately omits some popular signals. Recording why is part of being
honest about what the numbers mean.

- **No composite architecture score.** A blended 0–100 score hides detected edges,
  declared volatility, inferred distance, missing coverage, and accepted exceptions
  behind one figure. False precision is an anti-pattern. archfit reports
  `coupling_balance` as a band, lists every advisory edge, and lets the gate rules
  produce binary pass/fail. Each signal stands on its own evidence.
- **Code-quality concerns are delegated to linters by design.** Cyclomatic
  complexity, duplication, panic/unsafe/global-state density, god-struct counts,
  and test density are better served by purpose-built linters (golangci-lint,
  clippy, flake8, eslint). archfit focuses on inter-module coupling structure, not
  intra-module code quality.
- **Volatility comes from DDD subdomain classification, never git churn.** Git
  churn is accidental volatility; essential volatility is determined by business
  domain. Mixing them produces a metric that punishes active development, not
  structural risk.
- **`cohesion_spread`, `shared_state_hub`.** Prototyped, then removed — they did
  not rank real problems better than the metrics that shipped.

See [Concepts](concepts.md) for the model these metrics serve.
