# Metrics reference

This page documents every signal `archfit` computes: what it represents, why it
exists, how it is scored, and whether it can affect the verdict. For the theory
behind the strength / distance / volatility vocabulary used throughout, read
[Concepts](concepts.md) first.

`archfit` measures **Balanced Coupling** (`coupling_balance`) plus a minimal set of
complementary metrics. They split into three roles:

- **Headline (1):** `coupling_balance` (scored 0–100, linearly rescaled from
  the book's 1–10 per-edge balance). Report-only unless you
  configure the opt-in [`coupling.gate`](configuration-reference.md#couplinggate)
  block, which fails the build on a band floor or a score drop.
- **Baseline-delta gated (4):** `unbalanced_edge`, `cycle`, `encapsulation`,
  `coverage`. Each is compared against the committed baseline; a worsening
  delta **fails the build by default** (`metrics.<name>.gate` unset = `fail`;
  downgrade with `warn`, disable with `off`).
- **Report-only metric (1):** `blast_radius`. Carries no delta and never
  changes the verdict.

Separate JSON/Markdown report-only blocks, including `connascence`,
`local_coupling`, `runtime_async`, and `classified_edges` summaries, explain the
score inputs. They are evidence, not metrics, and never gate on their own.

A metric's **absolute value** never fails the build — only a _regression_
against the baseline you accepted (or a tripped `coupling.gate`) does. Gate
rules (forbidden dependency, public-API-only, layer direction, cycle-as-fail,
expired exception) remain the only checks that fail on current structure alone.
See [Concepts → How archfit operationalizes the model](concepts.md#how-archfit-operationalizes-the-model).

---

## The verdict

After all metrics run, the run gets one verdict
(`internal/engine/engine.go`, `computeVerdict`):

```text
fail  → any gate finding with status "new" or "expired_waiver", or any metric
        delta that worsens past its threshold with gate fail/unset
warn  → otherwise, any worsening metric delta capped by gate: warn, or any
        active gate:warn rule advisory
pass  → otherwise
```

"Worsens" is direction-aware: count metrics (`cycle`, `unbalanced_edge`) worsen
upward (delta > `max_new`), ratio metrics (`encapsulation`, `coverage`) worsen
downward (drop > `min_delta`). Per-metric `gate`/threshold knobs are documented
in the [configuration reference](configuration-reference.md#metrics).

Exit codes: `0` pass, `1` gate failed, `2` warnings/regressions, `3` tool/config
error. Without `--gate`, `archfit analyze` is report-only and exits `0` on any
verdict.

---

## Scoring model

The scorecard dimension produces a 0–100 value, a band, and a confidence.

### Bands

| Band          | Score  | Meaning                                                                                   |
| ------------- | ------ | ----------------------------------------------------------------------------------------- |
| `strong`      | 90–100 | Healthy.                                                                                  |
| `serviceable` | 70–89  | Acceptable.                                                                               |
| `mixed`       | 50–69  | Watch.                                                                                    |
| `poor`        | 30–49  | Problem.                                                                                  |
| `critical`    | 0–29   | Measured and bad.                                                                         |
| `n/a`         | —      | No signal to measure. Not good, not bad — _no evidence_. Never conflated with `critical`. |
| `info`        | —      | Report-only fact; asserts no quality verdict.                                             |

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

`coupling_balance` confidence starts with the scored fraction of classified
internal cross-boundary facts, then applies evidence caps:

- fewer than **5** scored internal cross-boundary facts ⇒ high confidence is
  disallowed;
- fewer than **3** connected modules in the scored/abstained coupling sample ⇒
  high confidence is disallowed;
- `dependency-cruiser` `partial` with unresolved-specifier ratio above **10%**
  (`tsUnresolvedRatioCeiling`, `internal/score/score.go`) ⇒ high confidence is
  disallowed, because unresolved specifiers land in the `external` bucket,
  outside `coupling_balance`'s denominator;
- `cargo-modules` `partial` on Rust ⇒ high confidence is disallowed.

These caps lower `high` to `medium` and append evidence lines. They do not lower
the numeric score or trip `coupling.gate` by themselves. Existing low confidence
from a poor scored fraction remains low. Confidence downgrades (provenance,
coverage, per-language partial extraction, sample size) compose by taking the
minimum confidence — they never stack.

### Deltas

When a committed baseline exists (`.archfit-baseline.json`, written by `archfit
baseline`), each metric is compared with that snapshot. A worsening delta —
direction-aware, past the metric's threshold knob — sets the run to `fail`
unless that metric's `gate` says `warn` or `off`. `blast_radius` carries no
delta and never gates. The `--base <ref>` flag compares `coupling_balance` and
metric scores against a git ref.

---

## Verdict-affecting metrics

### `coupling_balance` (headline metric)

> **Scorer version:** `bc_score.v6` — Khononov Ch10 book formula, with clone-only duplicated knowledge scored by default and transitive inferred-volatility cascade when enabled.

- **Represents:** how well the distribution of coupling across module boundaries
  respects the strength × distance × volatility balance rule. High score means
  most edges carry low maintenance cost; low score means expensive, high-risk
  couplings dominate.
- **Formula:** `balance = max(|S − D|, 10 − V) + 1` (Khononov Ch10 verbatim);
  see [Concepts → The balance rule](concepts.md#the-balance-rule) for ordinals,
  abstain semantics, and confidence. `coupling.volatility_cascade: true` enables
  a deterministic fixpoint cascade (book Ch9).
- **Affects verdict:** only through the opt-in
  [`coupling.gate`](configuration-reference.md#couplinggate) block — `min_band`
  (band floor) and `max_drop` (points below the baselined score) fail the run.
  No block ⇒ report-only. Band `n/a` never trips (abstain ≠ fail).
- **Denominator:** cross-module coupling facts only. Same-module edges score into
  the report-only [`local_coupling`](#local_coupling) block and never enter this
  metric. Clone-only duplicated-knowledge pairs (cross-module clones with no
  import edge) enter by default through `coupling.duplicated_knowledge: score`;
  set `advisory` to hold them out of the headline score. The evidence line
  discloses `clone-only duplicated-knowledge pairs: S scored, A advisory-only`,
  and JSON exposes the same counters as `classified_edges.clone_only_scored` and
  `classified_edges.clone_only_advisory`. The evidence line also discloses
  volatility provenance — `volatility provenance (modules): declared: N,
inherited: M, cascade: K` (plus `undeclared: U` when nonzero; JSON:
  `classified_edges.volatility_provenance`) — so a repo whose volatility is
  uniform because synthetic submodules inherited it reads as
  uniform-by-inheritance, not as a measured fact. JSON also exposes
  `classified_edges.connected_modules`, `classified_edges.by_distance_basis`,
  `classified_edges.distance_compression`, and `classified_edges.tail_risk`;
  Markdown renders the same in **Distance confidence**. These fields show which
  deterministic distance signal selected each rung, which middle Ch8 rungs
  remain compressed, and whether the mean hides a lower-tail hot spot. Tail risk
  reports worst balance, lower-decile balance, high-or-worse share, critical and
  distributed-monolith counts, plus clone-only subcounts when scored clone-only
  duplicated knowledge contributes to the tail.

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
- **Affects verdict:** a count rise past `max_new` vs baseline **fails by
  default**; set `metrics.unbalanced_edge.gate: warn` (or `off`) to downgrade.
- **Balanced Coupling:** the most direct encoding of the model — all three
  dimensions at their high settings.

---

## Info-band metrics

These always report band `info` — they assert no quality band on their own.
`cycle`, `encapsulation`, and `coverage` still carry a baseline delta, and a
worsening delta gates like any other metric (fail unless downgraded per metric);
`blast_radius` carries no delta and never affects the verdict.

### `cycle`

- **Represents:** number of import cycles among modules/packages.
- **Computed:** Tarjan strongly-connected components; each SCC of size > 1 is one
  cycle.
- **Band:** always `info`. Confidence always `high` (cycles are a fact, not an
  inference). A cycle-count rise vs baseline fails by default via the metric
  delta (`metrics.cycle.gate`); the `cycle` rule with `gate: fail` makes new
  cycles a hard failure.
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
- **`partial` always carries a reason.** A tool-coverage record with
  `status: partial` names the cause in its `reason` field (e.g. TypeScript's
  unresolved-specifier count/ratio) and, for TS, `archfit analyze` also emits a
  matching stderr warning — never a silent partial with no explanation. For
  dependency-cruiser the record also carries `specifiers_seen` (total import
  specifiers examined), the denominator of both the disclosed ratio and the
  confidence cap — the two can never disagree.

---

## Report-only blocks

### `connascence`

- **Represents:** deterministic, static Ch6 connascence evidence observed on
  dependency edges. It is explanatory evidence for the strength classification,
  not a scored metric.
- **Computed:** extractors attach edge evidence only where the source fact is
  deterministic. Go `go/types` can report name, type, meaning (const/var/data),
  and algorithm (function/method/callable value). TypeScript dependency-cruiser
  reports name for runtime imports and type for `import type`. Python grimp
  reports dotted/private import name evidence only; it does not invent class,
  const, or function meaning from names. SCIP reports name/type/meaning/algorithm
  where symbol descriptors prove them.
- **Output:** JSON `connascence` contains `edges_with_evidence`,
  `abstained_edges`, `total_evidence`, `by_kind`, `by_source`, and `unmeasured`.
  Markdown renders the same compact summary.
- **Unmeasured by design:** position, execution, timing, runtime value, and
  identity are disclosed as unmeasured unless a later deterministic source proves
  them. Dynamic/lazy imports and runtime async bridges remain separate report-only
  evidence blocks and never become connascence guesses.
- **Report-only by design:** never consumed by `coupling_balance`, findings,
  baselines, or gate verdicts.

### `runtime_async` and `runtime_async_edges`

- **Represents:** deterministic async/message-bus/task integration sites. This is
  runtime/lifecycle coupling evidence for human review, not a scored distance
  adjustment.
- **Computed:** the runtime detector scans Go, TypeScript, and Python for known
  async libraries and high-signal async framework patterns. Missing async evidence
  never implies synchronous coupling.
- **Output:** JSON `runtime_async` keeps the historical per-module rollup.
  JSON `runtime_async_edges` groups the same concrete sites by
  source module → runtime target (`library`, decorator, or async primitive), with
  a capped site sample and a true `count`. Markdown renders the relationship-level
  summary and points to JSON for the full list.
- **Report-only by design:** neither block annotates graph edges, changes distance,
  enters `coupling_balance`, affects baselines, or changes the gate verdict.

### `local_coupling`

- **Represents:** intra-module cohesion — the book's Ch10 "local complexity"
  quadrant (low strength at low distance = low cohesion, the "ball of mud"
  corner). One JSON entry per module that has same-module edges.
- **Computed:** same-module edges score with the standard book formula at the
  `same_module` distance rung. Per module: `scored_edges`, `abstained_edges`
  (unknown strength — abstain-not-fake applies at this level too),
  `complexity_edges` and `complexity_share_pct` (scored edges in the
  local-complexity quadrant), `mean_balance`, and a capped, deterministic
  `worst_offenders` sample with source locations.
- **Fractal levels:** cross-module coupling and intra-module cohesion are
  different abstraction levels, so they stay separate reported numbers —
  same-module edges never enter `coupling_balance`'s denominator, and this
  block asserts no band.
- **Report-only by design:** never consumed by the verdict or any gate; a gate
  path, if one is ever added, comes only after real-world shakedown data.

---

## Coupling classification reference

Every cross-boundary edge is classified on the four lenses below
(`internal/model/coupling/coupling.go`). These power the
`bc/imbalanced_coupling` advisories and feed `encapsulation` and
`unbalanced_edge`.

| Lens         | Values (ordered)                                                                                                                    | Derived from                                                                                                                         |
| ------------ | ----------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| Strength     | `contract` < `model` < `functional` < `intrusive` (+`unknown`)                                                                      | public/internal globs, visibility, SCIP symbol kind, pinned labels                                                                   |
| Distance     | `same_module` < `cross_module_same_owner` < `cross_module_different_owner` < `cross_deploy_unit` < `declared_external` (+`unknown`) | module map, `owner`, `deploy_unit`, declared `external_systems` seam                                                                 |
| Volatility   | `low` < `medium` < `high` (+`undeclared`, `unknown`)                                                                                | explicit `volatility:`, then `subdomain:`; optional strong-coupling cascade; else `undeclared` (no path/name guessing, no git churn) |
| Explicitness | `explicit`, `implicit` (+`unknown`)                                                                                                 | strength (contract→explicit, intrusive→implicit) or AST hint                                                                         |
| Severity     | (none) < `low` < `medium` < `high` < `critical`                                                                                     | the balance rule over the four above                                                                                                 |

For the full severity table and the reasoning, see
[Concepts → The balance rule](concepts.md#the-balance-rule).

---

## Rules reference

Gate rules fail on current structure; metric deltas and `coupling.gate` fail on
regression vs baseline (see [The verdict](#the-verdict)). Rules with `gate: warn`
are advisory (non-blocking); rules with `gate: fail` block the build. An unknown
`type` value is a config error.

For the full rule list, default gates, and configuration syntax, see
[Configuration reference → rules](configuration-reference.md#rules).

---

## Per-language behavior

The deterministic gates and core metrics work from the built-in extractors plus
`git`. Supported languages and setup live in [Language support](languages.md).
Coverage and the optional metrics differ by language; when a tool is missing the
dependent metric reports `n/a` **with the reason and enable step** — never a
false failure.

| Signal                       | Go                                          | TypeScript / JS                                                    | Python                                                    |
| ---------------------------- | ------------------------------------------- | ------------------------------------------------------------------ | --------------------------------------------------------- |
| Dependency graph + gates     | `go/packages`                               | dependency-cruiser                                                 | `grimp` (dotted modules)                                  |
| Node-ID scheme               | `file:`                                     | `file:`                                                            | `module:` (incl. `src/` layout)                           |
| SCIP edge strength           | `scip-go`                                   | `scip-typescript` (needs `node_modules`)                           | `scip-python`                                             |
| type-only vs runtime edges   | n/a                                         | tagged (→ Contract strength)                                       | n/a                                                       |
| Connascence evidence         | name/type/meaning/algorithm from `go/types` | name/type from dependency-cruiser; symbol-kind refinements by SCIP | name/private import only; symbol-kind refinements by SCIP |
| Dynamic / lazy import signal | n/a                                         | `require()` / dynamic `import()`                                   | in-function / `importlib` / `__import__`                  |

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

| Metric(s)                                                                            | Needs                                             |
| ------------------------------------------------------------------------------------ | ------------------------------------------------- |
| coupling_balance, unbalanced_edge, cycle, blast_radius, encapsulation, coverage      | built-in extractors + `git`                       |
| coupling_balance (strength refinement)                                               | SCIP index (`analyzers.scip.enabled: true`)       |
| coupling_balance (clone → symmetric strength, including clone-only pairs by default) | clone detector (`analyzers.clones.enabled: true`) |
| `bc/duplicated_knowledge` advisory                                                   | clone detector (`analyzers.clones.enabled: true`) |
| public_api_max, public_api_change, public_api_type_leak (rules)                      | `sg` (ast-grep); `analyzers.syntax.enabled: true` |

The `llm` tool is used only by `archfit config enrich`, `archfit explain --llm`,
`archfit analyze --llm`, `archfit config init --llm`, and
`archfit config update --llm`. It is **never** consumed by the deterministic gate path —
gate verdicts and metric values stay deterministic. See
[LLM enrichment](llm-enrich.md).

---

## Design rationale: what archfit does not measure

`archfit` deliberately omits some popular signals. Recording why is part of being
honest about what the numbers mean.

- **No composite architecture score.** A blended multi-signal score hides detected edges,
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
