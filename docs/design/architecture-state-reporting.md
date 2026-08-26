# Architecture-state reporting (`archfit.architecture-state.v1`)

Status: contract frozen, collectors pending.
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

The model kernel's exported surface is pinned by `TestModelSurfaceNoDrift`
(`internal/testdata/model_surface.golden`). The state contract is part of that
published surface; regenerating the golden is a contract change and is called out
in review.

## What is not built yet

The contract ships before its collectors. Every envelope currently reports
`unmeasured` with a named owner and a stated reason, and
`projectArchitectureState` (`internal/application/report.go`) carries only the
facts the assessment result already holds: source ref, history depth and window,
tool versions, per-tool coverage, config hash, rubric version, and the finding
and agent-task lists.

Reporting those envelopes as measured-and-empty would be exactly the implicit
green result this contract exists to prevent. Each collector deletes its own
`UnknownFact` entry as it lands, and the coverage counters follow mechanically
from `Dimensions.CountStatuses`.

The shadow verdict rule is likewise minimal and is replaced by the
assessment-owned aggregator: `blocked` when an active hard-gate finding or a
tripped tool gate exists, otherwise `needs_attention`. `healthy` is unreachable
by construction while any dimension is unmeasured.
