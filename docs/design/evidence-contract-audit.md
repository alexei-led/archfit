# Evidence contract implementation audit

Status: Task 2 characterization against
`docs/design/evidence-contract.md` at commit `95ba16c`.

This audit treats every current `measured` branch as unproven until its inputs,
denominator, and abstention paths establish the Task 1 predicate. It audits the
production path, not only the existing dimension tests. The classifications mean:

- **status-semantics change** — the run already carries enough evidence to make
  the contract decision, but the collector promotes, abstains, or names its
  denominator incorrectly;
- **implementation fix** — a producer already computes the necessary native
  evidence, but the current pipeline collapses, misattributes, mixes, or drops
  it before the dimension can apply the contract; and
- **new collector required** — no current dimension collector can produce the
  required in-claim fact set. The later owner task must add that collector while
  retaining any useful current diagnostics as out-of-claim evidence.

A row with `none` has no mismatch to classify. It was still audited from the
producer through the collector and is explicitly ratified rather than assumed
correct.

| Dimension | Current promotion shape | Mismatches | Classification |
| --- | --- | --- | --- |
| `intent` | declarations imply `measured` | `INT-1` | status-semantics change |
| `structure` | any classified edge implies `measured` | `STR-1`–`STR-4` | status-semantics change; implementation fix |
| `modularity` | any declared module implies `measured` | `MOD-1`, `MOD-2` | status-semantics change; implementation fix |
| `coupling` | candidate denominator, abstentions, and TS ceiling decide status | none | none (ratified) |
| `change_locality` | completed non-empty history sample decides status | none | none (ratified) |
| `complexity` | production-file size is always `partial` | `CPX-1` | new collector required |
| `testability` | static file split is always `partial` | `TST-1` | new collector required |
| `operations` | declarations and analyzer rows are always `partial` | `OPS-1` | new collector required |
| `drift` | comparable persisted seam reference decides status | none | none (ratified) |

GitNexus reports `Decide` as **CRITICAL** upstream risk: 5 production symbols
are impacted through 6 execution flows (`Execute` in analysis, baseline, and
explain; `scoreBaseTree`; `attachBaseComparison`; and `assess`). The two direct
production callers are `buildState` and `state.New`; depth-two callers are
`Score` and `result.New`. The index was 17 commits behind this worktree, so the
counts carry that staleness caveat, but symbol resolution was exact and matches
the plan's previously recorded 5-symbol/6-flow result. No audit remedy changes
`Decide`, its signature, or its rules. Status corrections still change verdicts,
all five state formats, and `check` exits through those existing rules, so Task 4
requires owner review and refreshed goldens.

`buildDimensions` resolves exactly to
`internal/assessment/evaluation/dimensions.go`; its direct production caller is
`buildState`, and it calls all nine collectors plus `gateFor`. The committed
scope companion lists only existing files needed by the status and native-fact
fixes below. The three `new collector required` bodies share `dimensions.go`,
but Task 4 must leave those three symbols unchanged as its plan gate requires.

## intent

**Contract definition:** The dimension applies when policy declares a module or
rule. `declared_intent_inventory` and one conformance result for every active
rule are required. A rule with `gate: off` is producer-proved inactive and its
conformance is out of claim. No declarations, or no resolved inventory, is
`unmeasured`; a resolved inventory with a missing active-rule result is
`partial`; only a complete inventory and complete active-rule results are
`measured`.

**Current behaviour and producing branches:** `intentDimension` returns
`unmeasured` only when both `PolicySnapshot.Topology.Modules` and
`PolicySnapshot.Gates.Rules.Rules` are empty. Otherwise it increments
`evaluated` once for every declaration whose gate is not `off`, assigns
`state.Measured`, and emits an out-of-claim unknown for the off declarations.
`NewRuleset` delegates to `rules.New`, `evaluate` invokes `Rule.Check` for every
compiled declaration, and `gatedRule.Check` suppresses findings for `gate: off`.

**Mismatch:** `INT-1` — `evaluated` is declaration arithmetic, not an observed
rule-result inventory. An active syntax rule can return no finding when
`Evidence.SyntaxFacts` is empty (`publicAPIMax.Check`, `publicAPIChange.Check`,
and `publicAPITypeLeak.Check` all have that abstention branch), which is
indistinguishable here from a successful check. Graph-backed rules have the same
problem when a primary extractor is partial or disabled: invoking the rule over
an incomplete relationship set is not complete conformance. The run already
carries primary analyzer names, tool coverage, and the `ast-grep/syntax`
coverage row, but `intentDimension` ignores them and promotes. The off-rule
unknown remains correctly out of claim.

**Mismatch classification:** `INT-1` — **status-semantics change**. Task 4 can
form the observed active-rule set from the declared rule family and existing
producer-completion rows; it does not need a new repository collector.

## structure

**Contract definition:** The dimension applies when a primary dependency
extractor is applicable. It requires a completed inventory from every applicable
primary and complete module, direction, and layer classification for every
dependency whose endpoints resolve inside the declared module map. A completed
empty inventory is observed evidence. Missing all applicable inventories is
`unmeasured`; at least one completed inventory plus another missing inventory or
an incomplete internal classification is `partial`; complete inventories and
classifications are `measured`. Dependencies outside the declared map are
out-of-claim disclosures.

**Current behaviour and producing branches:** `structureDimension` returns
`unmeasured` when `Result.ClassifiedEdges` is nil or its `Total` is zero. Every
positive `Total` assigns `state.Measured`; it computes `internal` as
`Total - External`, appends the `cycle` metric when it is not `n/a`, and carries
`External` as an out-of-claim unknown. It does not inspect
`Result.PrimaryExtractorTools` or `Result.ToolCoverage`.
`analysis.buildClassifiedSummary` builds `Total` over every relationship edge
and score-bearing clone-only pair, while `addSummary` places only
unknown-distance edges in `External` and places declared external systems in
`DeclaredExternal`.

**Mismatches:**

- `STR-1` — a primary extractor that completed with zero edges is
  `unmeasured`, even though the contract requires the completed empty inventory
  to count as observed.
- `STR-2` — any positive summary is `measured` even when another applicable
  primary is disabled, failed, or partial. Existing primary tool coverage makes
  that incompleteness observable, but the collector never reads it.
- `STR-3` — the reported dependency denominator is not a dependency-only,
  inside-the-declared-map denominator. `Total` also includes containment edges
  and score-bearing clone-only pairs, and `Total - External` still includes
  `DeclaredExternal`. The current `internal_edges` metric and coverage numerator
  therefore over-count facts outside the structure claim.
- `STR-4` — the summary does not retain a completeness count for directed,
  layer-placed internal dependency edges. `analysis.buildSet` already resolves
  endpoint module names and preserves directed relationship kinds, and policy
  already carries layer declarations, but `buildClassifiedSummary` collapses
  those native facts before `structureDimension` can prove
  `internal_edge_classification` complete.

The unknown for undeclared external edges is correctly out of claim; it is not a
reason to lower an otherwise complete structure measurement.

**Mismatch classification:** `STR-1` and `STR-2` —
**status-semantics change**; `STR-3` and `STR-4` — **implementation fix**. Task 4
must preserve a dependency-only internal classification summary from the
existing relationship and policy facts, then apply the promotion rule to the
existing primary completion rows.

## modularity

**Contract definition:** The dimension applies when at least one module is
declared. It requires the declared-module inventory, explicit declared-module or
outside-map attribution for every first-party graph node, and every applicable
graph-shape fact. A metric producer may prove a graph metric not applicable when
its documented minimum signal is absent. No module inventory is `unmeasured`;
an observed inventory with missing attribution or an unavailable applicable
shape fact is `partial`; complete attribution and observed-or-producer-proved
not-applicable shape facts are `measured`. The intended public surface is
out-of-claim when none is declared.

**Current behaviour and producing branches:** `modularityDimension` returns
`unmeasured` only when the policy module map is empty. Any non-empty map assigns
`state.Measured`, regardless of `ClassifiedEdges` or metric producer status.
`metricValues` silently skips absent and `n/a` `blast_radius` or `encapsulation`
results, while the status remains measured. A zero public-surface declaration
adds an unknown but correctly leaves status unchanged.

The native graph path also uses a different denominator from the contract.
`analysis.buildSet` fills `relationship.Node.Module` with
`relationship.ModuleKey`, the extractor-convention collapse, while it resolves
only edge endpoint fields through the declared module map.
`modgraph.FirstPartyModules` and `modgraph.BlastRadius` use those structural keys
(and call `ModuleKey` again for edges), so `BlastRadiusMetric.Calculate` can
measure package/crate keys rather than the declared architecture boundaries.
No first-party-node declared-map attribution total survives in
`ClassifiedEdgeSummary`.

**Mismatches:**

- `MOD-1` — declaration alone promotes the dimension when graph acquisition is
  incomplete, node attribution is unavailable, or an applicable graph metric is
  absent. The collector also cannot distinguish a producer-proved `n/a` from an
  `n/a` caused by absent graph evidence without consulting the existing primary
  completion rows.
- `MOD-2` — boundary attribution is both unreported and, for blast radius,
  computed at the wrong module granularity. The graph, declared module map, and
  resolved edge modules already exist; the defect is their current collapse and
  projection, not a missing external evidence source.

The public-surface unknown is correctly out of claim and does not participate in
promotion.

**Mismatch classification:** `MOD-1` — **status-semantics change**; `MOD-2` —
**implementation fix**. Task 4 must preserve first-party node attribution to the
declared map, make graph-shape consumers use declared module identities, and
promote only after primary completion and graph-fact applicability are known.

## coupling

**Contract definition:** The dimension applies when relationship analysis
identifies an internal edge, declared-external edge, or score-bearing clone-only
pair. It requires the candidate inventory, strength and distance for each
candidate, and TypeScript unresolved-specifier coverage at or below the 10
percent ceiling (or TypeScript proved absent). No candidate inventory is
`unmeasured`; any abstained candidate or material TypeScript resolution gap is
`partial`; a complete scored denominator within the ceiling is `measured`.
Same-module and undeclared-external relationships are out of claim.

**Current behaviour and producing branches:** `couplingDimension` returns
`unmeasured` when `ClassifiedEdges` is nil or `Scored + Abstained` is zero. It
starts positive denominators as measured, changes to partial and names the
missing balance when `Abstained` is positive, then also changes to partial when
`score.TSUnresolvedPartial` reports a dependency-cruiser partial row above the
strictly-greater-than 10 percent ceiling. `analysis.addSummary` excludes
same-module and unknown-distance external edges from `Scored + Abstained`, while
including declared-external edges and score-bearing clone-only pairs.

**Mismatch:** None. Candidate identity is established by relationship analysis,
every candidate enters exactly one of scored or abstained, and the TypeScript
ceiling is shared with score confidence through `TSUnresolvedPartial`. The
absence of another primary language's possible edges is a structure inventory
issue under this contract; coupling's stated claim is over identified
candidates, with the explicit TypeScript resolution ceiling.

**Mismatch classification:** none — current behaviour is ratified. Task 4 may
route the same observed set through `Promote`, but it must preserve these
branches and output semantics.

## change_locality

**Contract definition:** The dimension applies when at least one module is
declared and bounded git history can be sampled. It requires a non-empty eligible
commit sample with completion status and complete commit-to-module attribution.
No eligible commit is `unmeasured`; an observed but truncated or incompletely
attributed sample is `partial`; a completed, fully attributed sample is
`measured`. Essential-versus-accidental volatility is out of claim.

**Current behaviour and producing branches:** `changeLocalityDimension` returns
`unmeasured` when `VolatilityCorroboration` is nil or `CommitsScanned` is zero. A
non-empty sample starts measured and changes to partial when its status is not
`evidence.StatusOK`. Its denominator uses the larger of declared modules and
observed touched modules. `history/git.runTouchCounts` parses every path in every
sampled commit, records an explicit zero when no path maps to a declared module,
and returns no partial stdout on a timeout. `buildVolatilityCorroboration`
preserves the sample status and commit count.

**Mismatch:** None. The bounded 500-commit pass is a completed sample when it
returns `ok`; the full-history fallback is also explicit. Runtime timeout output
contains no observed commit sample, so the resulting unmeasured branch agrees
with the contract. Synthetic input with observed commits and a non-`ok` status
uses the defensive partial branch. The essential-versus-accidental unknown is
correctly out of claim.

**Mismatch classification:** none — current behaviour is ratified. Task 4 may
route it through `Promote` without changing status or denominator output.

## complexity

**Contract definition:** The dimension applies when a declared module contains
production source. It requires a declared-module graph plus complete dependency
chain depth, fan-in distribution, and fan-out distribution over every declared
module. No required graph fact is `unmeasured`; some but not all graph facts are
`partial`; all four complete graph fact kinds are `measured`. File LOC,
function/method length, and cognitive complexity are out of claim.

**Current behaviour and producing branches:** `complexityDimension` returns
`unmeasured` only when `Observations.FileLOC` is empty. Otherwise it computes
production file count, total LOC, and largest file, then assigns
`state.Partial` unconditionally and names cognitive complexity as missing. A
walk containing only test, generated, or vendor files therefore still reports
partial with zero production files. The function contains no assignment of
`state.Measured` and receives no module graph depth or degree facts.

**Mismatch:** `CPX-1` — the current collector measures the wrong claim, treats an
out-of-claim cognitive metric as the promoting unknown, applies on trees with no
production source, and has none of the four required architecture graph facts.
Its size metrics remain useful diagnostics but cannot promote complexity.

**Mismatch classification:** `CPX-1` — **new collector required**. Task 7 owns
the module-graph collector and may retain LOC and syntax length tails only as
out-of-claim diagnostics. Task 4 must not edit `complexityDimension`.

## testability

**Contract definition:** The dimension applies when a supported-language probe
finds production source. It requires the production-source inventory, valid
supplied coverage units for every applicable module/language, complete path and
module attribution, compatible denominators, and sidecar-to-worktree freshness
`matched`. No production inventory and no valid coverage fact is `unmeasured`;
some production or coverage evidence with any missing, invalid, unresolved,
conflicting, stale, or unverified fact is `partial`; only the complete matched
set is `measured`. Assertion quality and boundary-test semantics are out of
claim.

**Current behaviour and producing branches:** `testabilityDimension` returns
`unmeasured` only when `Observations.FileClassIndex` is empty. Otherwise it
counts static test and production files, assigns `state.Partial`
unconditionally, and names executed coverage and boundary-test coverage as
unknown. A classification containing no production source is still partial.
The function contains no assignment of `state.Measured`; no coverage artifact,
path attribution, module attribution, units, sidecar, or freshness value reaches
it.

**Mismatch:** `TST-1` — the static file split is supporting inventory, not the
contract's exercised-code claim. The collector has no supplied-coverage path or
measured branch, applies too broadly when no supported production source exists,
and treats an out-of-claim boundary-test diagnostic as if it helped explain the
promoting gap.

**Mismatch classification:** `TST-1` — **new collector required**. Tasks 8–10
own supplied ingestion, parsing, attribution, freshness, and promotion. Task 4
must not edit `testabilityDimension`.

## operations

**Contract definition:** The dimension applies when a module is declared. It
requires the declared topology, an independently corroborated deploy unit, a
qualifying ownership statement (`declared` or `codeowners`), and declared-versus-
corroborated reconciliation for every declared module. No topology inventory is
`unmeasured`; an inventory with any missing corroboration, provenance, or
reconciliation is `partial`; complete per-module facts are `measured`. Runtime
topology, supply-chain state, and analyzer health are out of claim.

**Current behaviour and producing branches:** `operationsDimension` returns
`unmeasured` only when both the declared module map and analyzer rows are empty.
Otherwise it counts non-empty resolved owner and deploy-unit values, builds a
coverage denominator from applicable analyzer rows, assigns `state.Partial`
unconditionally, and names runtime topology, supply-chain inventory, and each
analyzer gap as unknown. It contains no assignment of `state.Measured`.

The native sources already run, but their promoting provenance does not reach
this collector. `acquire.Collect` produces `DeployUnitsByModule`; acquisition
then calls `PolicySnapshot.WithResolvedTopology`, which fills empty declaration
fields and collapses declared and detected units into the resolved module value.
Only a detected-module count remains in distance context. `ownership.Resolve`
returns a map plus one source for the resolution pass, but the collector counts
resolved owner strings without distinguishing a config or CODEOWNERS statement
from git-author fallback. No per-module declared/corroborated reconciliation is
retained.

**Mismatch:** `OPS-1` — the current denominator and unknowns describe analyzer
health and the rejected runtime/SBOM claim, not declared-topology completeness.
It can become partial with zero modules merely because a tool row exists, loses
the independent deploy-unit evidence during topology backfill, loses the owner
qualification needed per module, never reconciles the two sides, and has no
measured branch.

**Mismatch classification:** `OPS-1` — **new collector required**. Task 6 owns a
collector over retained deploy detection and ownership provenance. Existing
native detection avoids a new subprocess, but a new operations fact collector
is still required. Task 4 must not edit `operationsDimension`.

## drift

**Contract definition:** The dimension applies only when a persisted
architecture-state baseline carries a seam ledger and all config, resolved
model, approved-label, and rubric fingerprints match. It requires an admissible
reference and complete two-sided seam identity. Missing, legacy, or mismatched
references are `unmeasured`; an admissible reference with an unreadable identity
would defensively be `partial`; a complete union comparison is `measured`.
Whether `--base` was requested is out of claim.

**Current behaviour and producing branches:** `driftDimension` tests only
`BaselineAnchor.SeamsComparable`. False produces an unmeasured dimension and a
non-comparable delta with `driftReasons`; true compares the union of current and
stored qualifying seam IDs, assigns `state.Measured`, and reports equal observed
and total counts. `application.seamAnchor` sets `SeamsComparable` only for a
non-legacy persisted state whose four fingerprints pass
`decision.CompareFingerprints`. The root report's independent
`comparison.status: not_requested` is never an input to this collector.

**Mismatch:** None. Validated baseline loading and the typed ID slices make the
contract's defensive partial state unreachable today: input is either rejected
as non-comparable or supplied as a complete slice. An empty union under a valid
reference is a completed zero, not missing evidence. The no-baseline and
fingerprint-mismatch reasons remain explicit.

**Mismatch classification:** none — current behaviour and the Task 1 lifecycle
policy are ratified. Task 5 adds lifecycle coverage but requires no production
collector change; Task 4 may route the same facts through `Promote`.
