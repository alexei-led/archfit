# Architecture-state reporting (`archfit.architecture-state.v1`)

Status: shipped. Contract pinned, all nine collectors landed.
Plan: `docs/plans/architecture-state-reporting.md`.

Archfit's headline is being replaced. The repository-level `coupling_balance`
scalar and its `poor/mixed/serviceable/strong` band are demoted to a per-seam
diagnostic; the primary decision becomes `HEALTHY`, `NEEDS_ATTENTION`, or
`BLOCKED`, derived from explicit hard gates, active diagnostics, and evidence
coverage.

This document records the approved contract, its decision semantics, and the
compatibility policy for the migration. It is the reference the code comments in
`internal/model/report/state.go`, `internal/model/report/document.go`, and
`cmd/archfit/format_matrix_test.go` point at.

## Why a state instead of a score

An averaged number answers the wrong question. `coupling_balance` folds a
mean-of-seams into one 0–100 value, so a repository with one catastrophic seam
and ninety healthy ones reports the same headline as one with ninety mediocre
seams. Worse, the average is defined over whatever was measured, so a missing
extractor silently shrinks the denominator and the number goes *up*. A reader
cannot tell a green result from an unlooked-at one.

The state contract fixes both by construction:

- there is no repository-level scalar to average — the verdict reads only
  classifications, never magnitudes;
- every dimension publishes its own measurement status, denominator, and the
  facts it could not observe, so "we did not look" is a first-class output.

## The contract

`report.ArchitectureState`, `internal/model/report/state.go`. Schema version is
`archfit.architecture-state.v1`, pinned by `TestStateSchemaVersionIsPinned` and
carried in `StateSchemaVersion`. It is versioned independently of the diagnostic
envelope's `SchemaVersion`: the two cut over on different schedules.

Top-level blocks:

| Block         | Holds                                                           |
| ------------- | --------------------------------------------------------------- |
| `verdict`     | `healthy` \| `needs_attention` \| `blocked`                      |
| `decision`    | the explicit inputs to the verdict, and nothing else             |
| `comparison`  | what this run was compared against, and whether that is legal    |
| `measurement` | deterministic source, history, and tool-version facts            |
| `dimensions`  | the nine envelopes                                               |
| `coverage`    | measured/partial/unmeasured counts plus per-tool coverage rows   |
| `findings`    | the run's findings, unchanged                                    |
| `agent_tasks` | the Assessment-owned task list, projected verbatim               |

### The nine dimensions

`Dimensions` is a struct with nine named fields, not a map. Two consequences are
deliberate: each dimension has a compile-time name and evidence owner, and the
wire order is fixed by declaration order instead of by map iteration — so the
encoding is byte-stable without a sorting pass.

| Dimension        | Evidence owner                  |
| ---------------- | ------------------------------- |
| `intent`         | `policy+assessment/evaluation`  |
| `structure`      | `relationship/facts`            |
| `modularity`     | `assessment/metrics`            |
| `coupling`       | `relationship/analysis`         |
| `change_locality`| `history/git`                   |
| `complexity`     | `syntax+evidence/acquisition`   |
| `testability`    | `syntax/fileclass`              |
| `operations`     | `policy+evidence/acquisition`   |
| `drift`          | `assessment/decision`           |

Every envelope is the same `DimensionState`: name, owner, measurement status,
confidence, gate posture, coverage denominator, typed metrics, finding
references, unknown facts, and an optional delta.

Dimension-specific facts live in `[]MetricValue` — a typed record with a name,
value, unit, optional observed/total denominator, and provenance. On the wire
`metrics` is always an array, never an object and never a free-form map. The
envelope is a reporting contract; the domain calculators keep ownership of their
facts. `TestArchitectureStateFieldsAreVersioned` fails any new field that enters
the schema without an explicit JSON tag.

Findings are never duplicated per dimension. An envelope carries `FindingRef`
values pointing into the state's single `findings` list, so finding identity,
status, and ordering keep exactly one owner.

`UnknownFact` is how a partial or unmeasured dimension stays honest: it names the
fact, the reason it is missing, and the capability that would have to produce it.

### Verdict semantics

The aggregator reads `StateDecision` plus dimension status. It may not inspect a
dimension's metrics or derive an implicit threshold from them.

- **`BLOCKED`** — at least one active hard-gate finding, or a reportable
  required-tool policy failure. No dimension-specific metric can block unless a
  named hard rule owns that decision. A failure to produce a valid report at all
  (config decode, malformed labels, schema error) is a command error, exit 3 —
  not a reportable `BLOCKED`.
- **`NEEDS_ATTENTION`** — no blocker, but at least one active diagnostic exists
  or any dimension is `partial`/`unmeasured`. This is what stops missing evidence
  from reading as a healthy result.
- **`HEALTHY`** — all nine envelopes measured, every hard gate passing, no active
  diagnostic. It must never be fabricated by treating an unsupported collector as
  zero. `NewArchitectureState` constructs nine unmeasured envelopes precisely so
  a caller that measures nothing cannot publish green;
  `TestNewArchitectureStateIsHonestlyUnmeasured` pins that.

JSON stores lower case; human formats display upper case.

Frozen exit table:

| Command / result                                        | Exit |
| ------------------------------------------------------- | ---: |
| `analyze` produced a valid report, whatever the state   |    0 |
| `check`: `HEALTHY`                                      |    0 |
| `check`: `NEEDS_ATTENTION`                              |    2 |
| `check`: `BLOCKED`                                      |    1 |
| parser / config / tool execution error                  |    3 |

`scripts/tests/cli_exit_contract_test.sh` stays the executable authority for the
0/1/2/3 table.

### Comparison is strict

Any `config_hash` / `model_hash` / `labels_hash` / `rubric_version` mismatch makes
the run `non_comparable` with stated reasons — never a numerical delta computed
across incomparable models. No project option may weaken this. A module rename
changes the model hash, so it cannot surface as a comparable resolved/new seam.

**`model_hash` covers the RESOLVED module map, not the declared one.** It is
taken after `PolicySnapshot.WithResolvedTopology`, so it includes the owners
CODEOWNERS/git history filled in and the deploy units detection mapped — not
only what `.archfit.yaml` declares. Task 3 deferred this choice to 4A; it is
settled here, and `TestModelHashCoversResolvedTopology` pins it.

The reason is the direction of the failure. A seam qualifies for the
distributed-monolith gate at high distance, and distance is computed from owner
and deploy unit. Hashing only the declared map would leave a resolved-owner
change — a CODEOWNERS edit, a new commit by a different author, or an
`ownership.Resolve` git timeout — silently re-qualifying an existing seam, and
`mode: fail` would block a no-op commit. Including it turns the same event into
an ABSTENTION with a named cause, which is the safe direction and matches the
abstain-not-fake rule the rest of the scorer follows.

The cost is real and accepted: an ownership or deploy-unit split — one canonical
way a distributed-monolith seam appears — makes the stored baseline
non-comparable, so `mode: fail` blocks only on seams introduced by a code-edge
change. Re-run `archfit baseline` after a topology change to restore a
comparable reference.

### Determinism

`StateMeasurement` carries only deterministic source, history, and tool facts. It
deliberately excludes wall-clock evaluation time, absolute local paths, and
process IDs, so two identical runs over an identical tree encode identically.
`TestArchitectureStateSerializationIsDeterministic` and
`TestShadowStateProjectionIsDeterministic` pin it at the contract and projection
layers; `TestFormatMatrix_DoubleRunIsStable` pins it per renderer.

## Approved product decisions

These were settled before execution. They are not open questions.

- **Primary formats cut over** to `archfit.architecture-state.v1`.
  `legacy-json` is available for exactly one release and must be explicitly
  selected.
- **Score-gate fields are rejected.** `Scorecard.Overall`, `OverallBand`,
  `coupling.gate.min_band`, and `coupling.gate.max_drop` cannot affect the
  verdict, the exit code, gate evaluation, baseline acceptance, or a delta. The
  scalar gate is retired and replaced by the explicit
  `coupling.gate.distributed_monolith` rule, default `mode: warn`,
  `max_new_seams: 0`. `TestArchitectureStateCarriesNoRepositoryScalar` is the
  structural guard: an `overall`, `overall_band`, `score`, or `band` field
  reachable from `ArchitectureState` fails the build.
- **Diagnostics-only** means `analyze` exits 0 and `check` exits 2.
- **Old baselines stay readable** for accepted finding fingerprints. Their scalar
  snapshot is ignored with reason `legacy_score_snapshot_ignored`, every
  state/dimension/seam comparison against them is `non_comparable`, and they are
  never auto-rewritten.
- **`agent_tasks` is projected**, not re-derived. State aggregation never builds a
  second task list.
- **Unobservable is unobservable.** Unsupported runtime coverage, runtime
  topology, and shallow or missing history report `partial`/`unmeasured` with
  named missing tools. V1 does not execute a target repository's test suite as
  measurement evidence.
- **SARIF keeps finding compatibility.** Architecture state goes in run
  properties. SARIF is exempt from human-layout parity, not from fact parity.

## Compatibility policy during the migration

The migration lands in tasks. Until the format cutover, **the current output does
not move a byte.**

`Document.State` is tagged `json:"-"`, for the same reason `Score` and `Decision`
are: the diagnostic envelope's wire shape is frozen until cutover, so populating
the field cannot change existing output. `TestShadowStateIsNotOnTheDiagnosticWire`
asserts no state key leaks into the encoded document.

The compatibility matrix is five committed baselines, one per renderer:

| Format      | Baseline                                                       | Owner                     |
| ----------- | -------------------------------------------------------------- | ------------------------- |
| `json`      | `internal/extract/golang/testdata/single-module/baseline.json`  | `byteidentical_test.go`   |
| `text`      | `cmd/archfit/testdata/format-matrix/text.txt`                   | `format_matrix_test.go`   |
| `markdown`  | `cmd/archfit/testdata/format-matrix/markdown.md`                | `format_matrix_test.go`   |
| `sarif`     | `cmd/archfit/testdata/format-matrix/sarif.json`                 | `format_matrix_test.go`   |
| `scorecard` | `cmd/archfit/testdata/format-matrix/scorecard.txt`              | `format_matrix_test.go`   |

The task that cuts over must move these bytes deliberately, in the same commit
that changes the contract. Adding a renderer without adding its matrix row leaves
that format's cutover unwitnessed.

Two capture rules, both learned the hard way:

- **Baselines live outside the analysed fixture tree.** `materializeFixtureRepo`
  copies the whole fixture directory, so a baseline written beside the fixture
  becomes an input to the next run that copies it — and four parallel subtests
  bootstrapping into one shared source directory race over what each one
  analyses. `baseline.json` predates this and is a self-referential fixed point;
  the four format baselines are not, and sit under `cmd/archfit/testdata/`.
- **Never capture a baseline in a degraded environment.** A sandbox that blocks
  Go module-cache writes makes `packages.Load` fail per package, which raises
  `Coverage.Unresolved`, which drops the `coverage` metric to low confidence,
  which caps its band from `strong` to `mixed`. Only the scorecard carries that
  band, so the JSON envelope stays byte-identical while every scorecard-bearing
  renderer moves — the diff looks exactly like a rendering regression. The
  captured "before" then encodes the sandbox, not the code.

  `requireHealthyExtraction` in `format_matrix_test.go` checks the `go/packages`
  coverage row before the matrix compares anything and fails with the
  environmental cause named. The pre-existing `TestByteIdentical_*` baselines
  have the same sensitivity and no such guard: run the whole `cmd/archfit`
  baseline suite with Go module-cache writes permitted.

Never delete a committed baseline to get green: that re-records whatever the code
now emits and makes the gate vacuous.

## Report boundary

Report adapters render a finished contract. `internal/output/**` and
`internal/report/ports` may import `internal/model/report` and
`internal/report/ports` and nothing else under `internal/` — reaching into
Assessment or Relationship internals would let a renderer re-derive a fact
instead of presenting the one the pipeline decided.

`assertReportAdaptersNoDomainImports` enforces it over the real import graph.
`TestReportBoundaryRuleFiresOnDomainImports` is its executable fixture: it proves
the predicate rejects the imports the migration must keep out of renderers,
rather than passing vacuously because no adapter happens to violate it today.

The model kernel is a pinned published contract. Its exported surface is
held by `TestModelSurfaceNoDrift`
(`internal/testdata/model_surface.golden`). The state contract is part of that
published surface; regenerating the golden is a contract change and is called out
in review.

## What v1 measures, and what it does not

All nine collectors have landed. Each envelope reports what this run actually
observed and names, as an `UnknownFact`, whatever it could not.

| Dimension         | v1 status         | Denominator                                    | Named unknowns                                                     |
| ----------------- | ----------------- | ---------------------------------------------- | ------------------------------------------------------------------ |
| `intent`          | measured          | declared rules evaluated / declared rules       | conformance to rules declaring `gate: off`                          |
| `structure`       | measured          | edges resolved to a declared module / all edges | direction and layer of edges leaving the module map                 |
| `modularity`      | measured          | modules with a public surface / declared modules| —                                                                   |
| `coupling`        | measured; partial while any edge abstains | scored / scored+abstained cross-boundary edges | the balance of each abstained edge  |
| `change_locality` | measured; unmeasured with no history | declared modules touched / declared modules | essential vs accidental volatility           |
| `complexity`      | **always partial**| production files / walked files                 | cognitive complexity — v1 ships no analyzer for it                  |
| `testability`     | **always partial**| test+production files / classified files        | executed test coverage; which boundaries a test exercises           |
| `operations`      | **always partial**| analyzers reporting / analyzer rows             | observed runtime topology; SBOM and vulnerability state; each gap   |
| `drift`           | measured only against a comparable v2 reference | qualifying seams compared | everything, when the reference is non-comparable |

Three dimensions are partial **by contract**, not by omission. Archfit does not
execute a target repository's test suite, does not observe what runs, and ships
no cognitive-complexity analyzer. Reporting those as measured-and-empty is the
implicit green result this contract exists to prevent — so a clean repository
lands on `NEEDS_ATTENTION` (exit 2 for `check`), and that is correct, not a bug
to configure away. `make archfit` accepts 0 or 2; only 1 fails it.

A later collector enriches an existing envelope and deletes its own
`UnknownFact`. Neither the contract nor the coverage counters change: the counts
follow mechanically from `Dimensions.CountStatuses`.

### Measurement is a property of the tree, not of the run

`StateMeasurement` publishes `source_ref`, `history_depth`, `history_window`, and
`tool_versions` — and nothing else. A full run measures files on disk and reports
`source_ref: worktree`; naming a commit there would claim the measured bytes
equal it, which is false the moment the tree is dirty. Only a delta run, which
really did diff against a resolved SHA, publishes one.

A run that scanned no history records `history_window: unavailable` with depth 0
rather than leaving both blank, so "there is no history here" stays
distinguishable from "nobody wired the scan up".

The four comparability fingerprints live **only** in the root `comparison` block.
A second copy anywhere in the document is a second answer to "may these two runs
be compared", and the copies would drift.
`TestFingerprintsLiveOnlyInTheComparisonBlock` walks the serialised wire form and
pins that. `labels_hash` is absent when no label is approved — empty compares
equal to empty, so two unlabelled repositories stay comparable, and a malformed
labels file is exit 3 long before any report exists.

## Label policy

`.archfit-labels.yaml` stays empty until a module pair has been reviewed by hand
and every edge in it has one defensible strength. A label overrides measured
integration strength, which moves seam severity and distributed-monolith
qualification, so it is an architecture decision — never metadata cleanup.

Validation is structural and hard. A self-pair, a duplicate ordered pair, an
undeclared endpoint, an invalid strength/status/confidence/provenance is exit 3:
an override that applies to nothing, or two answers to one question decided by
file order, cannot produce a valid report. A **stale** evidence hash is not an
error — it disables the override and emits the `labels/stale` diagnostic, which
can never be promoted to a gate.

An approved label owes its evidence: `evidence_hash`, `rationale`, `provenance`,
and `confidence`. Without an evidence hash the freshness check is skipped
forever, so the label silences its pair permanently and nothing in the report
says a human decision is doing it. `label_evidence_required` gates that.

## Gate policy

Only a named hard rule blocks. Hard gates are reportable forbidden-dependency and
layer-direction rules at `gate: fail`, cycles when configured to fail, a
required-tool policy failure, an expired waiver on an active fail rule, a
configured coverage floor, and the opt-in distributed-monolith rule.

Everything else is diagnostic: coupling scores and distributions, local
same-owner cascades, hubs and fan-in/out, public-surface size with no explicit
rule, complexity, static testability, change locality, operational declarations,
drift magnitude, stale label evidence, and optional missing tools. A coupling
seam never blocks merely because its score is low.

The self-config keeps `coupling.gate.distributed_monolith` at `mode: warn` with
`max_new_seams: 0`. Archfit is a single-owner, single-deploy-unit monolith, so no
seam qualifies; enabling `fail` here would gate on a condition the repository
cannot currently reach, which teaches readers to ignore the gate.

## Erosion gates

Six named checks keep the contract from decaying back into the scalar report it
replaced. CI runs them as an explicit gate step; each has one executable owner
and each owner proves it fires on a fixture that violates it, so none can pass
vacuously.

| Check                       | Owner                                 | What it prevents                                                   |
| --------------------------- | ------------------------------------- | ------------------------------------------------------------------ |
| `no_scalar_decision`        | `internal.TestErosion_NoScalarDecision` | an averaged score re-entering the path from evidence to exit code |
| `no_dead_archfit_rule`      | `internal.TestErosion_NoDeadArchfitRule` | a rule reporting "0 violations" for a boundary nobody checks     |
| `dimension_status_required` | `cmd/archfit.TestErosion_DimensionStatusRequired` | an envelope with no status reading as an empty, healthy result |
| `config_hash_required`      | `cmd/archfit.TestErosion_ConfigHashRequired` | a delta taken across a config edit blaming the code           |
| `label_evidence_required`   | `cmd/archfit.TestErosion_LabelEvidenceRequired` | an unevidenced approval silencing a seam permanently       |
| `baseline_idempotent`       | `cmd/archfit.TestErosion_BaselineIdempotent` | a self-referential capture reporting drift that is not there  |

`no_scalar_decision` scopes `internal/application/analysis.go` to the decision
functions rather than the whole file, deliberately: the run result still
**carries** the scorecard for the legacy renderers, and carrying a retired fact
for one release is not the same defect as deciding from it. The scoped rule fails
loudly if its target function is renamed away, so it cannot silently check
nothing.

## Maintenance recipes

- **Adding a collector to an existing envelope.** Extend the dimension function
  in `internal/assessment/evaluation/dimensions.go`, add its `MetricValue` with a
  provenance string, and delete the `UnknownFact` it retires. The contract, the
  coverage counters, and the renderers need no change.
- **Adding a metric to the state.** It is a `MetricValue` inside an existing
  envelope. A new top-level field is a contract change: it needs a JSON tag, a
  `TestModelSurfaceNoDrift` golden regeneration, and a review that says so.
- **Changing a dimension's status rule.** Update the collector and its
  `dimensions_test.go` case together. `dimension_status_required` rejects the
  shapes that read as healthy-by-omission; it does not decide which status is
  right for a given evidence set.
- **Changing the distance ordinals or the balance formula.** Bump
  `ScoreVersion`. It is a comparability fingerprint, so every stored reference
  becomes non-comparable — which is the point: two runs under different formulas
  must not subtract cleanly.
- **Moving a package.** Update the owning `paths:` in `.archfit.yaml` in the same
  commit. `TestSelfModel*` fails on a dead glob, an unowned package, an
  equal-specificity ownership tie, or a rule aimed at a path that does not exist.
