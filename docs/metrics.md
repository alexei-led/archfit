# Metrics

Metrics describe code structure and evidence. They do not replace rules or
determine architecture-state completeness. See [rules](rules.md) for rule gates,
[results](results.md) for verdicts and output fields, and [configuration](configuration.md)
for metric thresholds and gate settings.

## Architecture-state dimensions

The state report has nine dimensions. Each reports `measured`, `partial`, or
`unmeasured` from required evidence. These statuses do not rate quality.

| Dimension | Claim |
| --- | --- |
| `intent` | Declared modules, layers, rules, waivers, and active-rule results. |
| `structure` | Dependencies, direction, module placement, and cycles. |
| `modularity` | Boundary shape, cohesion, hubs, and encapsulation. |
| `coupling` | Strength, distance, volatility, and balanced coupling evidence. |
| `change_locality` | How change impact relates to module boundaries. |
| `complexity` | Declared-module dependency depth and degree. |
| `testability` | Supplied execution coverage attributed to declared modules. |
| `operations` | Declared owners and deploy units, with independent evidence. |
| `drift` | Change in distributed-monolith seams against a comparable baseline. |

The [evidence contract](evidence.md) defines applicability, required facts,
denominators, and the exact status rules for each dimension.

Dimension metrics report facts within each claim. `complexity` includes chain
depth and module fan-in/fan-out percentiles. `testability` includes supplied
covered and total execution units. `operations` includes owner and deploy-unit
counts. `drift` includes new and resolved seams. Function-size metrics are
diagnostic. They do not determine `complexity` completeness.

## Baseline metrics

These metrics can carry deltas. A regression can block `check` when its gate is
`fail` or unset. `warn` reports an advisory. `off` disables the metric gate.
The [configuration reference](configuration.md#metrics) owns settings.

| Metric | Value and direction | Meaning |
| --- | --- | --- |
| `unbalanced_edge` (`unbalanced_edge.v3`) | Count. Higher is worse. | New intrusive edges across a module boundary into high volatility. External systems do not count. Zero is strong. One or more is critical. |
| `cycle` (`cycle.v1`) | Count. Higher is worse. | Strongly connected dependency components. Cycles built only from Rust edges (intra-crate module cycles) or only from .NET edges (intra-project file cycles) receive a `poor` band: cargo and MSBuild forbid cycles between crates and projects, so the language permits the cycle. Any other cycle receives a `critical` band. `module_cycle` handles cycles between declared modules. |
| `encapsulation` (`encapsulation.v2`) | Ratio from 0 to 1. Lower is worse. | Contract edges divided by contract plus intrusive edges across module boundaries. Other strengths and unknown edges do not enter the denominator. |
| `coverage` (`coverage.v1`) | Ratio from 0 to 1. Higher is better. | Extracted files divided by applicable files across contributing tool-coverage records. This is analyzer coverage, not test coverage. |

A metric with no signal reports `n/a`, not a clean zero. `unbalanced_edge` is
`n/a` when intrusive cross-module candidates exist but none has known volatility.
`coverage` is `n/a` when no applicable extractor files contribute. Tool rows
with zero applicable files do not affect its ratio. Supplied execution coverage
is a separate input to `testability`. See [evidence](evidence.md#testability)
for its attribution and freshness limits. Analyzer coverage confidence is high
at up to 5% unresolved files, medium at up to 20%, and low above 20%. A partial
tool row must report its reason.

`encapsulation` reports `n/a` when cross-boundary edges exist but none is
classified as contract or intrusive. Compiler-enforced exports alone do not
earn a perfect score. Its confidence depends on the classified share of
cross-boundary edges.

## Diagnostic metrics

| Metric | Range or unit | Meaning |
| --- | --- | --- |
| `coupling_balance` | 0–100. Higher is better. | Rescaled mean balance of scored cross-module coupling facts. It is diagnostic and does not gate directly. |
| `blast_radius` (`blast_radius.v2`) | Count of hubs | Modules that at least 30% of other declared modules can reach through dependencies. It reports impact concentration, not a defect. |

`blast_radius` has no baseline delta or gate. It reports `n/a` when the graph
has no useful multi-module signal.

The `coupling_balance` metric version is `bc_score.v7`. It linearly rescales
per-edge balance values from 1–10 into 0–100. Its bands are `critical` (0–20), `poor` (21–40),
`mixed` (41–60), `serviceable` (61–80), and `strong` (81–100). Confidence can
cap the value and band. These bands do not apply to the nine dimensions.

Confidence comes from the scored share of module and declared-external facts: at
least 80% is high, 50–79% is medium, and less than 50% is low. Fewer than five
scored facts or fewer than three connected modules disallows high confidence.
Zero scored facts gives `n/a` with low confidence. A degenerate graph can still
have a measured score when clone-only facts provide coupling evidence.

Critical edges cap the value at 60. Distributed-monolith edges cap it at 40
when they reach at least 5% of scored and abstained facts. Low confidence caps
the band at `mixed`. Reported `raw_value` and `cap_applied` show the uncapped
value and the applied limit.

For standalone metric bands, high, medium, and low confidence cap the band at
`strong`, `serviceable`, and `mixed`, respectively. These caps never raise a
band. `coupling_balance` also reports confidence limits for small samples,
unresolved TypeScript specifiers, and partial Rust module graphs.
Applied non-high-confidence LLM labels can lower coupling confidence by one
level. Undeclared external edges do not enter the scored share.

## Deltas and gates

Metrics compare with a stored baseline only when their metric versions and
comparison context match. `cycle` and `unbalanced_edge` worsen when counts
increase. `encapsulation` and `coverage` worsen when ratios decrease.
Configured `max_new` or `min_delta` thresholds define tolerated change.

`unbalanced_edge` is a live metric and supports baseline ratchets. A blocking
delta becomes a finding with before, after, and threshold values. A rule-only
waiver can accept that finding for a limited period. A baseline capture cannot
evaluate a delta because it has no prior baseline.

`coupling_balance` does not ratchet. The `distributed_monolith` coupling gate
counts new qualifying seams against a comparable reference. Other hard gates can
block on current structure. See [commands](commands.md) for exit codes and
[results](results.md) for finding fields.

## Supporting evidence

The following report blocks explain inputs or expose limits. None gates by itself.

| Block | Meaning and limit |
| --- | --- |
| `connascence` | Deterministic static evidence for shared knowledge. It never sets strength. Dynamic categories remain unmeasured. |
| `dynamic_imports`, `classified_external_edges` | Dynamic import sites and excluded external edges. They inform review but do not enter internal coupling scoring. |
| `dynamic_connascence_signals` | Review hints from dynamic imports and runtime async relations. Each signal is unmeasured and report-only. |
| `distance_config_candidates` | Static external, dynamic, and async evidence that can prompt config review. It does not change distance or config. |
| `volatility_corroboration` | Git commit counts by declared module. It supports review but does not set volatility. |
| `semantic_strength_overlay` | SCIP strength refinements by language. Go is excluded because `go/types` supplies its strength. |
| `runtime_async`, `runtime_async_edges` | Detected async integration sites. Missing evidence does not prove synchronous coupling. These sites do not affect score or distance. |
| `local_coupling` | Same-module coupling and cohesion signals. Unknown strength abstains. These edges do not enter `coupling_balance`. |
| `distance_context`, `classified_edges` | Distance basis and scoring inputs, abstentions, volatility provenance, drivers, module-pair concentration, and tail risk. These explain the score; they do not create another gate. |

`classified_edges` includes clone counts. Score mode counts clone-only pairs in
`coupling_balance`. Advisory mode omits them from that score. The seam ledger
includes only pairs with import edges.

Coverage, tool availability, abstained edges, confidence, and incomplete facts
can limit a claim. A missing fact is not a zero. See [evidence](evidence.md)
for the required-fact boundaries and [configuration](configuration.md) for
supplied test-coverage inputs and metric policy. Metric gates use direction-aware
thresholds. See [configuration](configuration.md#metrics) for defaults and
field behavior.
