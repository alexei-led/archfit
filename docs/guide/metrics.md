# Metrics reference

This page documents every signal `archfit` computes: what it represents, why it
exists, how it is scored, and whether it can affect the verdict. For the theory
behind the strength / distance / volatility vocabulary used throughout, read
[Concepts](concepts.md) first.

`archfit` ships **26 metrics**. They split into two roles:

- **Verdict-affecting** (4): scored 0–10, can `warn` the run when they regress.
- **Report-only** (22): band `info`; surface facts for humans and agents, never
  change the verdict.

A metric absent from the config is enabled by default; only an explicit
`metrics.<name>.enabled: false` disables it. The
[scorecard](#scorecard-dimensions) synthesizes these metrics (plus gates) into
the architect's seven banded dimensions.

No metric ever fails the build on its own. Only explicit **gate rules**
(forbidden dependency, public-API-only, layer direction, cycle-as-fail, expired
exception) produce a `fail`. Metrics inform; rules gate. This separation is
deliberate — see [Concepts → How archfit operationalizes the model](concepts.md#how-archfit-operationalizes-the-model).

---

## The verdict

After all metrics run, the run gets one verdict
(`internal/engine/engine.go`, `computeVerdict`):

```text
fail  → any gate finding with status "new" or "expired_exception"
warn  → otherwise, any metric whose delta vs baseline is negative
pass  → otherwise
```

Exit codes: `0` pass, `1` gate failed, `2` warnings/regressions (non-blocking by
default; use `--report` to never exit non-zero on these), `3` tool/config error.

---

## Scoring model

Every scored (non-`info`) metric produces a 0–10 value, a band, and a confidence.

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

In delta mode (`check --base <ref>`) each scored metric is compared with the
baseline snapshot. A negative delta (the metric got worse) sets the run to
`warn`. Report-only metrics carry no delta and never warn.

---

## Verdict-affecting metrics

### `encapsulation` (headline metric)

- **Represents:** of the cross-boundary edges that take a stance on boundary
  respect, the fraction that go through a declared contract instead of reaching
  into internals.
- **Computed:** `contract_cross / (contract_cross + intrusive_cross)`, counting
  only edges classified `contract` or `intrusive` at a real cross-boundary
  distance. `functional` and `model` (normal public coupling) and `unknown`
  (no evidence) are excluded from the denominator, not counted against the score.
- **Why this denominator:** counting every public function call as a strike would
  crush the ratio for any normal codebase and manufacture a false `critical` once
  symbol-level strength lands. The metric asks one question — _when code crosses a
  boundary, does it use the front door?_ — and ignores edges that are neither a
  contract nor a leak.
- **Scored:** `value × 10` → band. No cross-boundary edges → `1.0` (vacuously
  encapsulated). Cross-boundary edges exist but none is contract/intrusive →
  `n/a`. Contract-only with zero intrusive on a compiler that forces exported
  access (Go/TS) → `n/a`, because 100% there is not _earned_. Confidence scales
  with the classified fraction of cross-boundary edges.
- **Affects verdict:** `warn` when encapsulation drops vs baseline.
- **Balanced Coupling:** strength (`contract` vs `intrusive`), filtered by distance.

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

### `cycle`

- **Represents:** number of import cycles among modules/packages.
- **Computed:** Tarjan strongly-connected components; each SCC of size > 1 is one
  cycle.
- **Scored:** `0` → `strong`; any → `critical`. Confidence always `high` (cycles
  are a fact, not an inference).
- **Affects verdict:** `warn` on a new cycle vs baseline; the `cycle` rule with
  `gate: fail` makes new cycles a hard failure.
- **Balanced Coupling:** none — a graph-topology fact, not a strength/distance call.

### `coverage`

- **Represents:** the fraction of applicable files the extractors actually
  processed — the trust signal for every other metric.
- **Computed:** `extracted / applicable` across all tool-coverage records. Zero
  applicable (extractors ran, nothing matched) → `1.0`. **No extractor ran at
  all** (no coverage record) → `n/a`, not `1.0` — absence of evidence is never
  scored as full coverage. This is the load-bearing fix that stops an unanalysed
  repo from scoring `strong`.
- **Scored:** `value × 10`. Confidence from the unresolved ratio (≤5% → high,
  ≤20% → medium, else low).
- **Affects verdict:** `warn` when coverage drops. More importantly, low coverage
  caps the band of every metric that depends on the missing evidence. When
  coverage is `n/a`, `analysis_confidence` starts at 60 and loses 15 per absent
  primary extractor (go/packages, dependency-cruiser, grimp), so an all-absent
  repo lands ≈ 0/critical rather than reporting confident health.
- **Balanced Coupling:** none — it modulates confidence system-wide.

---

## Report-only metrics

These always report band `info`. They never set a delta and never change the
verdict. They exist because the most expensive coupling is often the kind a
pass/fail gate cannot honestly judge: a stable hub is fine, a god-module might be
intentional, hidden coupling needs a human to confirm. Reporting beats gating
here.

### `blast_radius`

- **Represents:** structural change-impact concentration — modules that a large
  fraction of the codebase transitively depends on.
- **Computed:** for each first-party module, the count of other modules that
  transitively reach it (over the cycle-condensed DAG, so cycles don't inflate
  it); flagged a hub when that share ≥ **0.30** of modules.
- **Why report-only:** a hub is a fact, not a defect. A widely-used stable utility
  is good design. The number tells you _where_ a change ripples, not that anything
  is wrong.

### `change_amplification`

- **Represents:** expected change cost — blast radius weighted by how often the
  module actually changes (accidental volatility).
- **Computed:** `(blast_share) × (churn / max_churn)` per module; hub when
  ≥ **0.15**. Uses git churn, mapped to modules per language.
- **Why it matters:** blast radius says a change _could_ ripple; this says it
  _both_ ripples _and_ happens often — the modules where imbalance is paid down
  repeatedly. This is the one metric built on churn-derived volatility.

### `hidden_coupling`

- **Represents:** module pairs that change together in git history but have **no**
  static import edge — coupling the dependency graph cannot see (shared implicit
  contracts, temporal coupling, mediated state).
- **Computed:** for each unlinked pair, `co_change / min(churn_a, churn_b)`;
  counted when co-change ≥ **4** and that logical-coupling ratio ≥ **0.50**.
- **Why report-only:** this is the operational hint for `functional` / `model`
  coupling that imports miss. It surfaces candidates for human or LLM review, not
  verdicts.

### `structural_weight`

- **Represents:** size skew — modules far larger than the codebase median
  (god-module smell).
- **Computed:** flagged when module LOC ≥ `max(median × 4, 400)`.
- **Why LOC:** LCOM4 and "vocabulary mixing" were prototyped and dropped — they
  misranked (data registries scored worst, real god-files scored best). Raw size
  is the honest proxy. Known blind spot: a god-module split across many small
  files is not flagged.

### `complexity`

- **Represents:** functions whose cyclomatic complexity exceeds a threshold — the
  intra-module risk that module-size metrics cannot see.
- **Computed:** count of functions with CCN > **15**, via the `lizard` tool. Shows
  the top 5 hotspots with file and line.
- **Requires:** `tools.complexity.enabled: on` (opt-in; config-driven for
  determinism, not PATH presence).

### `risk_hub`

- **Represents:** symbol-level risk hubs — modules with the widest externally-used
  symbol surface, weighted by _declared_ volatility.
- **Computed:** `breadth × volatility_multiplier`, where breadth is the count of a
  module's own symbols referenced from other modules' symbols, and the multiplier
  is `high 1.0 / medium 0.66 / low 0.33 / unset 1.0`. An optional GitNexus factor
  (1.0–2.0) refines but cannot dominate.
- **Distinct from `blast_radius`:** blast radius counts _how many modules depend on
  M_; risk*hub counts \_how many of M's own symbols are used externally*. A state
  store with 73 externally-read fields outranks a utility with one widely-called
  function.
- **Distinct from `change_amplification`:** risk_hub uses only hand-authored
  volatility (captured before churn is applied), so the two never double-count.
- **Requires:** a SCIP index (`tools.scip.enabled: on`; `scip-go`,
  `scip-typescript`, `scip-python`).

### `architecture_fitness`

- **Represents:** how much of the architecture intent is _actively enforced_, not
  just documented.
- **Computed:** fraction of three enforcement signals present — (1) arch test
  files, (2) import-linter config, (3) an arch-linter in CI. Display shows
  `present/3 × 10` with the matched evidence paths. Detection skips
  `<root>/pkg/mod/**` (the Go module cache) and `**/testdata/**`, so a vendored
  module-cache `_test.go` is never miscounted as an architecture test.
- **Scoring note:** the display carries a 0–10 number for legibility, but the band
  is always `info` — it never gates. As a **scorecard dimension** it distinguishes
  "scan didn't run" from "ran and found nothing": `n/a` → poor (≈40/low), not
  critical; `critical` is reserved for a real 0/3 on a repo that was analysed. A
  genuine 0/3 (no enforcement signals) on a small healthy repo is correct, not a
  false positive.
- **Why it matters:** an architecture that is enforced by executable checks resists
  drift; one that lives only in a wiki rots. This metric measures the enforcement
  posture itself.

### `functional_candidates`

- **Represents:** cross-module pairs with duplicated logic (copy-paste of business
  rules) — a proxy for `functional` coupling.
- **Computed:** count of cross-module pairs sharing ≥1 duplicated code block (clone
  detector, e.g. `jscpd`), annotated with how many also co-change.
- **Distinct from `hidden_coupling`:** hidden*coupling is co-change \_without* an
  import edge; functional*candidates is \_duplication*, whether or not the modules
  import each other. A pair can appear in both.
- **Requires:** `tools.clones.enabled: on` (opt-in).

### `change_locality`

> **Breaking change (v0.3.0):** `metric_version` bumped to `change_locality.v2` — the
> distance composite (code-structure + deploy-unit + degenerate-owner suppression) and
> the removal of git-churn from gate volatility changed the metric's input semantics.
> Re-run `archfit baseline` if you have a pinned baseline from v0.2.x or earlier.

- **Represents:** per-change drift — how far a change reaches beyond the modules it
  touches.
- **Computed:** count of cross-module edges originating from changed files, plus
  the forward graph reach (distinct files reachable from the changed set). Always
  `n/a` in full mode (no diff) — never a false zero.
- **Why it matters:** this is the bridge to agent cost. A change that stays local
  is cheap to make and to verify; one that reaches across many modules predicts
  larger token burn and more repair iterations. The `new_cross_module_dependency`
  _rule_ is the gate equivalent; this metric quantifies the blast surface for the
  agent loop. See [Agent feedback loop](agent-feedback.md).

### `cohesion_lcom`

- **Represents:** lack-of-cohesion within a module — whether its definition
  symbols form one connected unit or split into several unrelated clusters
  (LCOM4 / connected-components proxy).
- **Computed:** over the SCIP symbol graph, per measurable module, the undirected
  graph of definition symbols connected by same-module references; counts
  connected components (1 = cohesive, >1 = fragmented).
- **Caveat (load-bearing):** SCIP indexers do not populate `enclosing_range`, so
  reference attribution is document-scoped. The proxy trusts only cross-document
  structure and measures only modules spanning ≥2 documents (and ≥4 symbols).
  For `scip-python` / `scip-typescript` a "module" is keyed per source file, so
  almost every module is single-document → **`n/a`, never a false verdict**. Go
  packages span multiple files and are measurable.
- **Status:** report-only, and **disabled in archfit's own config**. It failed
  its eval (blind for single-file Python/TS modules — exactly where the LOC-skew
  proxy already diverged from expert judgment); kept because the Go-package
  fragmentation signal is honest where it applies. See
  [`gap-closure-task20-cohesion-eval.md`](../plans/notes/gap-closure-task20-cohesion-eval.md).

### Beyond Balanced Coupling (supporting / non-BC)

These are standard structural metrics that complement, but are not part of, the
Balanced Coupling model. They are clearly labelled non-BC, always report-only,
and never re-use BC vocabulary. They exclude external / unresolved nodes (node
builtins, uninstalled packages) so they never flag third-party names.

- **`instability`** — per-module Martin instability `I = Ce / (Ca + Ce)`: the
  fraction of a module's couplings that are outgoing. High `I` = depends on many,
  depended on by few.
- **`abstractness`** — per-module Martin abstractness
  `A = abstract_inbound / (abstract + concrete_inbound)`: the fraction of inbound
  edges targeting an interface/protocol (SCIP strength hint).
- **`martin_distance`** — distance from the main sequence `Dms = |A + I − 1|`;
  `> 0.5` is the "zone of pain" (too concrete + stable) or "uselessness" (too
  abstract + unstable). When confidence is low (proxy-derived without SCIP type
  kinds) it is footnoted, not headlined, in the human report — full values stay
  in JSON.
- **`propagation_cost`** — `PC = reachable_pairs / (n·(n−1))`: the fraction of
  ordered module pairs `(i, j)` where `j` is transitively reachable from `i`;
  reported system-wide and per-module.
- **`change_coupling`** — CodeScene's temporal-coupling formula: module pairs that
  co-change in ≥ **65%** of commits touching either (min support applies),
  whether or not they share a static import edge. Complements `hidden_coupling`
  (which filters out importing pairs).

### Syntax-surface metrics (ast-grep facts)

These metrics are derived from `tools.syntax` (ast-grep) facts.
All are report-only (`info`); none changes the verdict.
They require `tools.syntax.enabled: on`; when absent the metric reports `n/a`.

- **`unsafe_density`** — count of unsafe operations per module (Rust): `unsafe {}`
  blocks, `UnsafeCell`, `transmute`, and raw-pointer casts (`as *mut`/`as *const`).
  A high count surfaces candidate modules for manual soundness review; `archfit` does
  not judge acceptability — route to `archfit review` or a human for that.
- **`panic_density`** — count of panic/unwrap operations per module in production
  code. Counts `unwrap()`/`expect()` (Rust) and `panic(` (Go). Test files are
  excluded using the same heuristic as `test_in_production` — the production-only
  count is materially lower than the naive module-wide total.
- **`struct_field_density`** — per-module count of struct definitions with ≥1 field
  (Go and Rust). High field counts surface god-struct candidates; the opt-in
  `struct_field_max` rule lets a project gate on a ceiling.
- **`test_density`** — per-module count of test functions (`func Test…` in Go,
  `#[test]` in Rust, `def test_*` in Python). A proxy for test coverage presence;
  real coverage percentages need a dedicated coverage tool and stay in the LLM/human
  review path.

### public_api_type_leak (rule)

Fires when a public API exposes a type from an external framework package directly in a function or method signature. Requires `tools.syntax.enabled: on`.

Defaults to `gate: warn` when unset (advisory, non-blocking). Fires once per unique module+type combination.

**Limitation:** only fires on repos with Go-style dotted external package nodes in the dependency graph. Rust/TypeScript/Python repos may have type_leak facts but no matching external nodes — see [configuration reference](configuration-reference.md#rules).

### Manifest and graph metrics

- **`deprecated_dep_count`** — count of locally-declared deprecation/retraction
  markers found in manifest files: `retract` directives in `go.mod`, `deprecated`
  fields in `package.json`. Report-only; never gates. Live-version EOL checking
  (registry queries) is out of scope — route that to `archfit enrich`.
- **`file_mutual_import`** — count of file pairs that mutually import each other
  (file-level bidirectional cycles not caught by the module-level `cycle` metric).
  Detected by running Tarjan SCC over the file-node projection of the dependency
  graph. Currently only TypeScript has file→file edges; Go and Python produce
  file→package or module→module edges, which yield no false positives here.
  Report-only; never gates.

---

## Coupling classification reference

Every cross-boundary edge is classified on the four lenses below
(`internal/model/coupling/coupling.go`). These power the
`bc/imbalanced_coupling` advisories and feed `encapsulation` and
`unbalanced_edge`.

| Lens         | Values (ordered)                                                                                              | Derived from                                                                                                     |
| ------------ | ------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| Strength     | `contract` < `model` < `functional` < `intrusive` (+`unknown`)                                                | public/internal globs, visibility, SCIP symbol kind, pinned labels                                               |
| Distance     | `same_module` < `cross_module_same_owner` < `cross_module_different_owner` < `cross_deploy_unit` (+`unknown`) | module map, `owner`, `deploy_unit`                                                                               |
| Volatility   | `low` < `medium` < `high` (+`undeclared`, `unknown`)                                                          | explicit `volatility:`, then `subdomain:`, then a deterministic path heuristic; else `undeclared` (no git churn) |
| Explicitness | `explicit`, `implicit` (+`unknown`)                                                                           | strength (contract→explicit, intrusive→implicit) or AST hint                                                     |
| Severity     | (none) < `low` < `medium` < `high` < `critical`                                                               | the balance rule over the four above                                                                             |

For the full severity table and the reasoning, see
[Concepts → The balance rule](concepts.md#the-balance-rule).

---

## Scorecard dimensions

`archfit score` (and `archfit check --format scorecard`) synthesizes the metrics
above plus the gate findings into the **seven-dimension architect rubric**. The
synthesis is a pure, deterministic decision over the already-computed evidence —
no tools, no I/O, no LLM. Each dimension carries a 0–100 value, a band, a
confidence, and evidence refs; the overall is the mean of the six non-meta
dimensions.

| Dimension                 | Derived from                                                                                                                                                                                                                                      |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `boundary_integrity`      | gate-violation count over classified cross-boundary edges (low confidence when none classified; an explicit note when `encapsulation` is `n/a` rather than a fabricated value)                                                                    |
| `coupling_balance`        | strictly the BC advisory rollups — strength × distance × volatility maintenance-effort distribution + worst-case (high/high/high) count. Empty edges with low coverage → 50/low (not a blanket 90); the evidence states the classified-edge count |
| `dependency_graph_health` | cycles, blast-radius hubs, instability/abstractness shape                                                                                                                                                                                         |
| `cohesion_modularity`     | god-modules, hidden coupling, duplication — **high-strength + low-distance cohesion is never penalised**                                                                                                                                          |
| `change_locality`         | the `change_locality` metric (delta mode); `n/a` in full mode                                                                                                                                                                                     |
| `architecture_fitness`    | the `architecture_fitness` enforcement metric (`n/a` → poor, not critical)                                                                                                                                                                        |
| `analysis_confidence`     | meta dimension — how much evidence backed the review (coverage, classified fraction, semantic tools present); drops to critical when the primary extractors are absent                                                                            |

Band thresholds match the rubric: critical 0–20, poor 21–40, mixed 41–60,
serviceable 61–80, strong 81–100. Band-matches-value, evidence-per-score, and
low-confidence caps are enforced and covered by a stored golden. The scorecard is
**off-gate** — it never changes the `check` verdict.

**Fail-loud, not false-green.** None of these dimensions report `strong` from
absence of evidence. A repo no extractor analysed scores `n/a`/critical with a
coverage gap, not a confident pass — see
[commands.md](commands.md#coverage-gaps-and-required-tools) and the
[coverage-gate design doc](../design/coverage-gate-and-autopilot-v0.1.md).

## Per-language behavior

The deterministic gates and core metrics work from the built-in extractors plus
`git`. Supported languages and setup live in [Language support](languages.md).
Coverage and the optional metrics differ by language; when a tool is missing the
dependent metric reports `n/a` **with the reason and enable step** — never a
false failure.

| Signal                              | Go            | TypeScript / JS                          | Python                                                   |
| ----------------------------------- | ------------- | ---------------------------------------- | -------------------------------------------------------- |
| Dependency graph + gates            | `go/packages` | dependency-cruiser                       | `grimp` (dotted modules)                                 |
| Node-ID scheme                      | `file:`       | `file:`                                  | `module:` (incl. `src/` layout)                          |
| `complexity` (lizard)               | yes           | yes                                      | yes                                                      |
| `risk_hub` / `cohesion_lcom` (SCIP) | `scip-go`     | `scip-typescript` (needs `node_modules`) | `scip-python` (per-file modules → cohesion mostly `n/a`) |
| type-only vs runtime edges          | n/a           | tagged (→ Contract strength)             | n/a                                                      |
| Dynamic / lazy import signal        | n/a           | `require()` / dynamic `import()`         | in-function / `importlib` / `__import__`                 |

Key behaviors fixed in the v0.x gap-closure program (see
[the gap-closure result](../notes/v0.x-tool-vs-expert-gap-closure.md)):

- `change_locality` matches changed files across all node-ID schemes — no more
  Python false-0.
- Beyond-BC graph metrics exclude external/unresolved nodes — TS martin no longer
  flags node builtins (`fs`, `crypto`, `commander`).
- BC advisories are grouped into scored rollups; the dynamic-import signal points
  at cycles the static graph cannot see (the lazy-import class common in Python
  and TS).

For the install matrix and per-language tool setup, see
[Language support → optional analyzers](languages.md#optional-analyzers-per-language)
and [Tooling reference](tooling.md).

## Tool requirements

Most metrics work from the built-in extractors and `git`. A few need an opt-in
tool and report `n/a` (with a coverage note) when it is absent — never a false
failure.

| Metric(s)                                                                                                               | Needs                                                               |
| ----------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------- |
| encapsulation, unbalanced_edge, cycle, coverage, blast_radius, structural_weight, architecture_fitness, change_locality | built-in extractors + `git`                                         |
| change_amplification, hidden_coupling, change_coupling                                                                  | `git` history (churn / co-change)                                   |
| instability, abstractness, martin_distance, propagation_cost                                                            | built-in extractors (SCIP refines abstractness)                     |
| risk_hub, cohesion_lcom                                                                                                 | SCIP index (`tools.scip.enabled: on`)                               |
| complexity                                                                                                              | `lizard` (`tools.complexity.enabled: on`)                           |
| functional_candidates                                                                                                   | clone detector (`tools.clones.enabled: on`)                         |
| risk_hub (refinement only)                                                                                              | GitNexus (`tools.gitnexus.enabled: on`)                             |
| unsafe_density, panic_density, struct_field_density, test_density                                                       | `sg` (ast-grep); `tools.syntax.enabled: on`                         |
| deprecated_dep_count                                                                                                    | manifest files (`go.mod`, `package.json`) — built-in; no extra tool |
| file_mutual_import                                                                                                      | built-in (TS file→file graph); no extra tool                        |

The `llm` tool is used only by `archfit enrich`, `archfit explain --llm`,
`archfit review`, `archfit autopilot`, `archfit init --llm`, and
`archfit update --llm`. It is **never** consumed by `check` — gate verdicts and
metric values stay deterministic. See [LLM enrichment](llm-enrich.md).

---

## Dropped and rejected metrics

`archfit` deliberately omits some popular metrics. Recording why is part of being
honest about what the numbers mean.

- **No single blended architecture score.** Martin's I/A/D are now reported as
  separate report-only metrics (`instability`, `abstractness`, `martin_distance`),
  but `archfit` never blends them — or any metrics — into one verdict number. A
  composite hides detected edges, declared volatility, inferred distance, missing
  coverage, and accepted exceptions behind one figure. The
  [scorecard](#scorecard-dimensions) bands each dimension separately for exactly
  this reason.
- **`cohesion_spread` and `shared_state_hub`.** Prototyped, then removed — they did
  not rank real problems better than the metrics that shipped. The later
  `cohesion_lcom` (connected-components LCOM4 proxy) also failed its eval but is
  retained report-only and disabled by default; see its entry above.
- **LCOM4 / "vocabulary mixing"** for `structural_weight`. Prototyped and dropped
  for misranking; raw LOC is the honest proxy (see `structural_weight` above).

See [Concepts](concepts.md) for the model these metrics serve, and
`docs/spec/arch-fitness-spec-v0.4.md` §10 for the original measurement design.
