# Nine-dimension evidence contract

Status: approved input to the language-evidence implementation plan.

This document defines what each dimension in
`archfit.architecture-state.v1` claims. It is the source of truth for deciding
whether a dimension is `measured`, `partial`, or `unmeasured`; a collector's
current branch structure is not the definition.

## Shared promotion rule

Each dimension has a fixed set of required **fact kinds**. A fact kind may be
quantified over a run-time denominator, such as every declared module, but the
kind itself cannot be removed from the contract at run time. For dimension
`d`, let:

- `A(d)` be its **Applicable when** predicate;
- `R(d)` be its fixed set of in-claim required fact kinds;
- `O(d)` be the required fact kinds completely observed over their declared
  denominator; and
- `N(d)` be the required fact kinds proved not applicable by the fact
  producer's own probe.

Status is decided in this order:

```text
measured   iff A(d) and R(d) ⊆ O(d) ∪ N(d)
partial    iff A(d), R(d) ⊄ O(d) ∪ N(d), and O(d) is not empty
unmeasured iff not A(d), or O(d) is empty
```

A zero is observed only when a producer completed the relevant denominator and
reported zero. Absence of a row, an unavailable producer, or a failed producer
is not an observed zero. A config opt-out is not proof that a fact does not
apply. A per-language fact may be not applicable only when that language's own
applicability probe says the language is absent. A per-module fact may be not
applicable only when its owning producer has an equally explicit probe; the
absence of evidence is not such a probe.

The predicates are exhaustive because `measured` is evaluated first. Thus a
required set proved entirely not applicable is measured for an otherwise
applicable dimension, while a non-applicable dimension has no claim subject and
is unmeasured. Every `partial` or `unmeasured` result names each missing
in-claim fact as an `UnknownFact`.

A dimension's status reports evidence completeness, not whether its metric
values are desirable. Findings and gate posture remain separate fields.

## Lenient versus strict unknown facts

The contract adopts one conditional rule rather than a globally lenient or
globally strict interpretation:

- **Strict inside the claim:** a missing in-claim required fact prevents
  `measured` and produces a named `UnknownFact`.
- **Lenient outside the claim:** an out-of-claim diagnostic may be unknown while
  the dimension remains `measured`, because it cannot change the truth of the
  dimension's stated claim.

Therefore a measured dimension may carry an `UnknownFact` only when this
contract marks that fact out of claim. In particular, disabled-rule conformance,
external-edge structure, an inferred public API, and essential-versus-accidental
volatility are out of claim for `intent`, `structure`, `modularity`, and
`change_locality`. An abstained coupling edge is in claim and remains strict.
This reconciles the two meanings of `measured` identified in the plan without
weakening the abstain-not-fake rule.

## intent

**Claim:** The report inventories declared architecture intent and determines
conformance for every declared rule that is active in this run.

**Evidence owner:** `policy+assessment/evaluation`.

**Applicable when:** The resolved policy declares at least one module or one
rule.

**Required facts:** `declared_intent_inventory` (modules, layers, rules, and
waivers) and `active_rule_conformance` (one evaluation result for every declared
rule whose gate is not `off`). A tree with no active rule proves
`active_rule_conformance` not applicable only after the policy inventory is
observed.

**Denominator:** Active declared rules evaluated over all declared rules.
Rules with `gate: off` stay in the total and are disclosed as not evaluated.

**In-claim:** The declarations themselves, active-rule evaluation, waiver use,
and active findings routed to intent.

**Out-of-claim:** Conformance to a rule whose gate is `off`; the configuration
explicitly removed that rule from this run's conformance claim.

**Measured when:** `A(intent)` is true, `declared_intent_inventory` is observed,
and every declared rule is either covered by `active_rule_conformance` or is
proved inactive by its `gate: off` declaration.

**Partial when:** The declaration inventory is observed, but at least one active
declared rule has no evaluation result.

**Unmeasured when:** No module and no rule is declared, or no declaration
inventory was resolved.

## structure

**Claim:** The report describes dependency structure, direction, and layer
placement for dependencies whose endpoints resolve inside the declared module
map.

**Evidence owner:** `relationship/facts`.

**Applicable when:** At least one primary dependency extractor is applicable to
the tree according to that extractor's own language probe.

**Required facts:** `primary_dependency_inventory` (a completed discovered-edge
inventory from every applicable, enabled primary extractor) and
`internal_edge_classification` (module endpoints, direction, and layer for every
edge that resolves inside the module map). A completed empty edge inventory is
an observed zero, not missing evidence.

**Denominator:** Discovered edges resolved inside the declared module map over
all discovered dependency edges.

**In-claim:** The complete primary edge inventory and the module, direction,
layer, and cycle facts for every internally resolved edge.

**Out-of-claim:** Direction and layer of edges whose targets leave the declared
module map; those edges remain counted and disclosed, but they make no claim
about this repository's internal structure.

**Measured when:** `A(structure)` is true, every applicable primary dependency
inventory completed, and every internally resolved edge has its in-claim
classification. An empty completed inventory and an inventory containing only
external edges both satisfy the predicate.

**Partial when:** At least one applicable primary inventory completed, but
another applicable primary inventory is unavailable or at least one internally
resolved edge lacks an in-claim classification.

**Unmeasured when:** No primary dependency extractor is applicable, or no
applicable primary extractor completed an edge inventory.

## modularity

**Claim:** The report describes the shape and graph-derived cohesion of the
boundaries in the declared module set.

**Evidence owner:** `assessment/metrics`.

**Applicable when:** At least one module is declared.

**Required facts:** `declared_module_inventory`, `module_boundary_attribution`
(one module or an explicit outside-the-map result for every first-party graph
node), and `module_graph_shape` (the graph facts needed to compute the applicable
cohesion, hub, and encapsulation metrics). A metric producer may prove a metric
not applicable when its documented minimum graph signal is absent; an absent or
disabled producer is not such proof.

**Denominator:** Declared modules with an explicitly declared public surface
over all declared modules. This denominator reports declaration coverage; its
ratio does not by itself decide measurement status.

**In-claim:** The declared module inventory, complete boundary attribution, and
applicable graph-derived modularity facts.

**Out-of-claim:** The intended public surface when none is declared. Zero public
surface declarations is an observed policy fact; inferring what should have
been public would require an intent source this dimension does not own.

**Measured when:** `A(modularity)` is true, the declared module inventory and
boundary attribution are complete, and every graph-derived modularity fact is
either observed or proved not applicable by its metric producer.

**Partial when:** The module inventory is observed, but boundary attribution is
incomplete or an applicable graph-derived modularity fact is unavailable.

**Unmeasured when:** No module is declared, or no declared-module inventory was
resolved.

## coupling

**Claim:** The report measures the balance and actionable seam shape of every
classified cross-boundary coupling candidate in the declared architecture,
subject to the documented TypeScript unresolved-specifier ceiling.

**Evidence owner:** `relationship/analysis`.

**Applicable when:** Relationship analysis identifies at least one in-scope
cross-boundary candidate: an internal edge, a declared-external edge, or a
score-bearing clone-only pair.

**Required facts:** `coupling_candidate_inventory`, `coupling_strength`,
`coupling_distance`, and `extractor_resolution_within_ceiling`. Strength and
distance are required for every candidate in the scored-plus-abstained
denominator. TypeScript resolution satisfies the last fact only when unresolved
specifiers are no more than 10 percent of specifiers seen.

**Denominator:** Scored cross-boundary candidates over scored plus abstained
cross-boundary candidates. Same-module and undeclared-external edges are outside
this denominator.

**In-claim:** Candidate identity, integration strength, architectural distance,
balance, seam aggregation, and material extractor incompleteness that can omit
candidates from the denominator.

**Out-of-claim:** Same-module coupling and dependencies on undeclared external
systems. They are separate local-coupling or external-hygiene facts and cannot
establish cross-boundary balance.

**Measured when:** `A(coupling)` is true, the candidate inventory is observed,
every denominator member has both strength and distance, and TypeScript
resolution is within the 10 percent ceiling or proved not applicable.

**Partial when:** The candidate inventory is observed, but one or more
candidates abstain for unknown strength or distance, or TypeScript unresolved
specifiers exceed the 10 percent ceiling.

**Unmeasured when:** No in-scope cross-boundary candidate was identified, or no
coupling candidate inventory was produced.

## change_locality

**Claim:** The report measures how broadly a completed bounded git-history
sample touched the declared module set.

**Evidence owner:** `history/git`.

**Applicable when:** At least one module is declared and the analysis root has a
git history source that can be sampled.

**Required facts:** `eligible_commit_sample` (including its bound and completion
status) and `commit_module_attribution` for every commit in that sample.

**Denominator:** Declared modules touched in the sampled history window over the
declared module set. If attribution discovers more modules than the resolved
policy contains, the larger set is the total so observed coverage cannot exceed
it.

**In-claim:** Eligible commits in the configured bounded window, their module
attribution, and the resulting co-change/locality distribution.

**Out-of-claim:** Essential versus accidental volatility. Commit frequency can
corroborate a declared volatility but cannot distinguish domain change from
design churn.

**Measured when:** `A(change_locality)` is true, at least one eligible commit was
observed, the history producer reports `ok`, and every sampled commit has a
complete module attribution, including an observed zero for untouched modules.

**Partial when:** At least one eligible commit and some attribution were
observed, but the scan is truncated, timed out, or has incomplete attribution.

**Unmeasured when:** No declared module or git history source is available, or
the scan produced no eligible commit.

## complexity

**Claim:** The report measures architecture-level complexity as module
dependency-chain depth and the module fan-in/fan-out degree tail.

**Evidence owner:** `syntax+evidence/acquisition` for supporting syntax facts and
`relationship/analysis` for the promoting module-graph facts.

**Applicable when:** At least one declared module contains production source.

**Required facts:** `declared_module_graph`, `dependency_chain_depth`,
`module_fan_in_distribution`, and `module_fan_out_distribution`, each complete
over the declared module denominator. A completed graph with no edges has
observed zero depths and degrees.

**Denominator:** Declared modules with chain-depth and fan-in/fan-out values over
all declared modules.

**In-claim:** The complete declared module graph and its chain-depth and degree
distributions. Dependency-chain depth is the longest path in dependency edges
through the graph's strongly connected component (SCC) condensation DAG. An
edgeless graph has depth zero, edges inside one SCC add no depth, and an edge
between SCCs adds one; cycle severity remains owned by the existing cycle
evidence.

**Out-of-claim:** File LOC, function/method length distributions, and cognitive
complexity. Size tails locate code hotspots but one large function does not
change the architecture-level claim; no cognitive-complexity analyzer is
claimed.

**Measured when:** `A(complexity)` is true and all four required module-graph
fact kinds are observed over every declared module.

**Partial when:** The declared module graph or at least one graph distribution
is observed, but another required graph fact is incomplete.

**Unmeasured when:** No declared module contains production source, or no
required module-graph fact was observed. Out-of-claim LOC or syntax facts alone
do not promote this dimension.

## testability

**Claim:** The report measures exercised-code attribution: how much production
code represented by supplied coverage artifacts was exercised, and which
applicable declared modules that evidence covers.

**Evidence owner:** `syntax/fileclass` for the source denominator and supplied
coverage ingestion for the promoting execution evidence.

**Applicable when:** The tree contains production source in at least one
supported language, as established by that language producer's own probe.

**Required facts:** `production_source_inventory`, `supplied_coverage_units`,
`coverage_path_resolution`, `coverage_module_attribution`, and
`coverage_freshness`. Required coverage is quantified per applicable language
and declared module. An absent language is not applicable; an applicable
language with no supplied artifact is missing. Freshness is observed only when
a valid sidecar's source hashes match the scanned bytes.

**Denominator:** Applicable declared modules represented by attributed coverage
over all applicable declared modules, plus covered units over total units within
each compatible single-unit coverage family. Unsupported or absent languages
are never averaged into either denominator.

**In-claim:** Production-source classification, parsed coverage units, complete
ScanRoot-relative path resolution, module attribution, and sidecar-to-source
freshness.

**Out-of-claim:** Assertion quality, mutation resistance, boundary-test
semantics, and whether a test suite is broadly well designed. Exercising an
instrumentation point proves none of those properties.

**Measured when:** `A(testability)` is true, every applicable module/language has
a valid supplied coverage fact, all paths and modules are resolved, compatible
unit denominators are complete, unresolved path count is zero, and freshness is
`matched`.

**Partial when:** Production-source inventory or some valid coverage facts are
observed, but an applicable module/language lacks coverage, a source is invalid,
units conflict, a path or module is unresolved, or freshness is `stale` or
`unverified`. No configured coverage artifact is partial, not not-applicable,
when production source exists.

**Unmeasured when:** No supported production source is present, or neither a
production-source inventory nor a valid coverage fact was observed.

## operations

**Claim:** The report measures declared operational-topology completeness:
every declared module has an independently corroborated deploy unit and an
owner backed by an ownership statement.

**Evidence owner:** `policy+evidence/acquisition`.

**Applicable when:** At least one module is declared.

**Required facts:** `declared_operational_topology`,
`corroborated_deploy_unit`, `owner_provenance`, and
`declared_corroborated_reconciliation`, quantified over every declared module.
A declared deploy unit is not its own corroboration. `declared` and
`codeowners` are ownership statements; `git_author` is observed fallback
provenance but does not satisfy the owner requirement. No module is implicitly
non-deployable without an explicit producer probe.

**Denominator:** Declared modules with both a corroborated deploy unit and a
qualifying owner over all declared modules. Declared and corroborated deploy
unit counts remain separate metrics and are never merged.

**In-claim:** The resolved declared topology, independent deploy-unit evidence,
owner provenance, and reconciliation of declared versus corroborated values.

**Out-of-claim:** Observed runtime topology, SBOM/vulnerability state, and
analyzer tool health. Runtime and supply-chain facts are separate report
families; analyzer gaps remain disclosed and may trip their own required-tool
gate without changing the declared-topology measurement claim.

**Measured when:** `A(operations)` is true and every declared module has a
qualifying owner, an independently corroborated deploy unit, and an observed
reconciliation between its declared and corroborated topology.

**Partial when:** The declared topology is observed, but at least one declared
module lacks qualifying owner provenance, corroborated deploy evidence, or a
reconciliation result.

**Unmeasured when:** No module is declared, or no declared operational-topology
inventory was resolved. Analyzer coverage rows alone do not make the declared
operational-topology claim applicable.

## drift

**Claim:** The report measures changes in qualifying distributed-monolith seams
against a persisted architecture-state baseline produced under the same config,
resolved model, approved labels, and rubric.

**Evidence owner:** `assessment/decision`.

**Applicable when:** A persisted state baseline contains a seam ledger and all
four comparison fingerprints match the current run.

**Required facts:** `admissible_persisted_reference` and
`complete_two_sided_seam_identity`. A reference is admissible only after all
four fingerprints match. A current seam set, a legacy baseline, or a mismatched
baseline alone is not a partial observation of either required fact because no
legal drift comparison exists.

**Denominator:** The union of qualifying distributed-monolith seam identities
in the current and persisted ledgers; every identity in either side is compared.

**In-claim:** Reference admissibility and new/resolved status for every
qualifying seam in the two-sided denominator.

**Out-of-claim:** Whether a separate `--base` comparison was requested and
finding-lifecycle buckets attached to the delta. They are report context, not
the persisted seam-drift measurement.

**Measured when:** `A(drift)` is true, the persisted reference is admissible,
and every qualifying seam identity on both sides was compared.

**Partial when:** An admissible persisted reference is observed and at least one
qualifying seam identity was compared, but another in-claim seam identity cannot
be read or compared. This is a defensive state; current validated baseline
loading normally makes the comparison complete or rejects it entirely.

**Unmeasured when:** No persisted state baseline exists, the baseline is legacy,
any comparison fingerprint differs, or no admissible reference fact was
observed.

## Drift lifecycle decision

The root `comparison.status: not_requested` says that the user did not request a
separate base-versus-head report. It does **not** make drift not applicable and
is not evidence of architectural stability. Drift is baseline-driven only:
without a persisted, fingerprint-comparable state baseline it remains
`unmeasured`, whether or not `--base` was requested.

Consequently, with passing hard gates and no active diagnostic, `check` still
returns `needs_attention` (exit 2) when no comparable persisted baseline exists.
`healthy` (exit 0) requires measured drift in addition to the other eight
measured dimensions. A missing, legacy, or fingerprint-incomparable baseline is
also `unmeasured`; the reason distinguishes the workflow case, but does not
change the drift claim. A blocked run remains exit 1, and command/config errors
remain exit 3.

## Reconciliation with current behavior

The table accounts for all nine collectors in
`internal/assessment/evaluation/dimensions.go`. “No deviation” means the current
status branches implement this contract for the facts they can receive; Task 2
still audits and ratifies that conclusion. Task 3 characterizes end-to-end
reachability but does not own a production status change.

| Dimension | Current behavior versus this contract | Deviation owner |
| --- | --- | --- |
| `intent` | **No observed deviation.** `intentDimension` is unmeasured with no declarations, otherwise measured, and treats `gate: off` conformance as an unknown footnote. Its inputs do not currently represent a missing active-rule result. | Task 2 ratifies or identifies an unrepresented partial path. |
| `structure` | **Deviation.** `structureDimension` reports measured whenever `ClassifiedEdges.Total > 0`, without proving every applicable primary inventory completed or every internal classification is complete. It also reports unmeasured for a completed zero-edge inventory because completion and absence are not distinguishable in its input. External-edge unknowns are correctly out of claim. | Task 2 classifies both status-semantics gaps; Task 4 implements only approved fixes. |
| `modularity` | **Deviation.** `modularityDimension` reports measured from a non-empty module declaration alone, even when boundary attribution or applicable graph metrics are absent/`n/a`. Its public-surface unknown is correctly out of claim. | Task 2 classifies the gap; Task 4 implements an approved status-semantics or input fix. |
| `coupling` | **No observed deviation.** `couplingDimension` is unmeasured without a candidate denominator, partial for abstained edges or TypeScript resolution above the 10 percent ceiling, and measured only when the required candidate facts meet that rule. | Task 2 ratifies; Task 4 routes the existing rule through the common promotion mechanism without weakening it. |
| `change_locality` | **No observed deviation.** `changeLocalityDimension` is unmeasured for no eligible commit, partial for a non-`ok` sample, and measured for a completed non-empty sample. Essential-versus-accidental volatility is correctly out of claim. | Task 2 ratifies; Task 4 routes the rule through common promotion. |
| `complexity` | **Deviation.** `complexityDimension` uses production-file LOC as its denominator, names cognitive complexity as missing, and is hardcoded partial whenever files exist. It has no module-chain or degree facts and no measured path under the architecture-level claim. | Task 2 classifies `new collector required`; Task 7 supplies the graph collector and keeps size diagnostics out of claim. |
| `testability` | **Deviation.** `testabilityDimension` observes only the static test/production file split and is hardcoded partial. It has no supplied coverage, path attribution, module denominator, or freshness fact, so it cannot satisfy the exercised-code claim. | Task 2 classifies `new collector required`; Tasks 8–10 add ingestion, parsers, attribution, and promotion. |
| `operations` | **Deviation.** `operationsDimension` is hardcoded partial, uses applicable analyzer rows as its envelope denominator, discards deploy detection and owner provenance, and treats runtime/SBOM absence as if it were inside the claim. It has no measured path for declared-topology completeness. | Task 2 classifies `new collector required`; Task 6 retains corroboration/provenance, changes the denominator, and promotes only the narrowed claim. |
| `drift` | **No observed status deviation.** `driftDimension` is measured only from `SeamsComparable` and otherwise unmeasured with a named reason. The root report's independent `not_requested` status does not feed the collector. | Task 2 ratifies the policy; Task 5 adds lifecycle tests and makes no collector change. |

The deviations above are exhaustive for current status promotion against this
contract. Metric presentation, confidence lowering, gate posture, finding
routing, and comparison rendering are separate contracts and are not silently
reclassified as evidence-completeness deviations here.
