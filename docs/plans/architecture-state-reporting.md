# Plan: Replace Scalar Architecture Score with Deterministic Architecture State

> **Ralphex execution plan.** This plan is for review and approval only. Do not
> execute it until the owner approves the reviewed plan.
>
> Suggested executor:
>
> ```sh
> ralphex docs/plans/architecture-state-reporting.md
> ```
>
> The plan is intentionally incremental. Each task is independently committable
> and must pass its focused checks before the next task begins. The executor must
> stop on behavior drift, ambiguous product semantics, missing evidence, or a
> requested score/config change that cannot be justified by architecture truth.

## Overview

Replace the current repository-level `coupling_balance` scalar and
`poor/mixed/serviceable` headline with a structured, deterministic architecture
state report. Keep the scalar calculation only as a per-seam diagnostic for
ranking and explanation. Make the primary decision `HEALTHY`, `NEEDS_ATTENTION`,
or `BLOCKED`, based on explicit hard gates, diagnostic findings, and evidence
coverage—not on an averaged score.

Implement the nine agreed dimensions:

1. configured intent and gate conformance;
2. dependency structure and direction;
3. modularity, cohesion, hubs, and public surface;
4. coupling seam ledger;
5. change locality and domain volatility evidence;
6. code and cognitive complexity;
7. testability and boundary coverage;
8. operational topology and supply-chain/tool coverage;
9. erosion and drift against a comparable baseline.

The result must be deterministic for identical source, configuration, tool
versions, and input worktree. Every dimension reports its measurement status,
confidence, coverage denominator, findings, unknowns, and delta. Missing tools or
unobservable facts must be explicit `partial`/`unmeasured` states, never an
implicit green result.

Release acceptance includes running the built branch binary against the
repository corpus in `docs/test-corpus.md`: a fast representative matrix and the
full Go, TypeScript/JavaScript, Python, and Rust corpus. Each run uses a copied or
generated temporary config migrated by that same binary. Confirmed extractor,
config-migration, output-contract, or CLI defects must be fixed and covered by a
regression test before the migration can pass. Architecture findings in a target
project are evidence, not automatically Archfit defects.

This is a product and contract migration. Preserve existing finding IDs,
statuses, rule IDs, ordering, output facts, baseline behavior, `--base` behavior,
and process exit semantics. The primary output cutover is explicit: `--format
json`, `text`, `markdown`, `sarif`, and `scorecard` emit the new
`archfit.architecture-state.v1` contract; the default/`--format text` renderer is
its console presentation, and `--format legacy-json` is retained for
one documented migration window only. The legacy field is never used for
verdict, exit code, gate evaluation, or baseline acceptance. The breaking
configuration change is equally explicit: new configs use `version: 2`, and the
new binary's `config update --migration-only` command is the supported
v1-to-v2 migration path; its preview uses `--json` and its write uses `--apply`.

## Source artifacts and evidence

Approved architecture direction:

- `docs/design/20260823-archfit-capability-map.md`
- `docs/design/architecture-baseline.md`
- `docs/design/relationship-assessment-contract.md`
- `docs/design/model-contract-deletion.md`
- `docs/plans/complete-capability-architecture-migration.md`

Current implementation anchors:

- `internal/assessment/score/score.go:Synthesize` emits the only
  `coupling_balance` dimension and promotes it to `Scorecard.Overall`.
- `internal/assessment/evaluation/finalize.go:finalize` invokes score synthesis
  and coupling-gate evaluation.
- `internal/application/report.go:ProjectReport` projects assessment output to
  `report.Document`.
- `internal/application/baseline.go:BaselineService.Execute` persists a score
  snapshot and accepted finding fingerprints.
- `internal/application/base_compare.go` and
  `internal/assessment/decision/config_compare.go` compare base trees and
  configuration/model changes.
- Output adapters are in `internal/output/{console,jsonout,markdown,sarif,scorecard}`.
- CLI formats and exit mapping are in `cmd/archfit/{formats,analyze,check,main}.go`.
- Coupling inference and label overrides are in
  `internal/relationship/{classify,labels}` and `.archfit-labels.yaml`.
- Deterministic metrics are in `internal/assessment/metrics/**` and
  `internal/relationship/facts/**`.
- Cross-language acceptance inventory and mechanics are in
  `docs/test-corpus.md`, `scripts/eval/corpus_sweep.py`, and
  `skills/archfit-eval/**`.

Evidence from the final PR review and Fable review:

- **R1 — scalar scope:** `coupling_balance` is the sole scorecard dimension;
  repository score conflates one diagnostic with architecture health.
- **R2 — score evidence:** final PR has 369 scored internal edges, mean balance
  4.70/10, 74 critical-band edges, 85 high-or-worse edges, and zero distributed
  monolith edges. Main has 400 scored, mean 4.61, 123 critical-band, and 139
  high-or-worse. The tail improved while the scalar moved only from 40 to 41.
- **R3 — distance:** all final scored edges are `cross_module_same_owner`; raw
  owner/deploy/runtime facts are being collapsed into one distance category.
- **R4 — weighting:** evidence-adapters → evidence-contracts (41 edges),
  assessment-repair → evidence-contracts (24), relationship-analysis →
  evidence-contracts (18), and assessment-repair → relationship-analysis (16)
  dominate edge counts. One logical seam can be represented by many imports.
- **R5 — labels:** `.archfit-labels.yaml` is empty. Labels are ordered module-pair
  integration-strength overrides (`contract`, `model`, `functional`, `symmetric`,
  `intrusive`) and must not be used to lower volatility or excuse composition
  roots.
- **R6 — config:** `.archfit.yaml` has 18 capability modules with complete
  production owner/layer/subdomain/volatility metadata and one production deploy
  unit. `evidence-contracts` is described as a pinned published contract, not a
  frozen low-volatility artifact.
- **R7 — history:** volatility corroboration currently reports zero entries and
  must say `unmeasured`, not imply stable architecture.
- **R8 — testability:** testability and boundary-test coverage are not currently
  an Archfit dimension. Existing Go coverage and contract tests can supply the
  first deterministic implementation.
- **R9 — baseline:** baseline score persistence and comparison are coupled to the
  scalar; baseline generation has had idempotence issues. Baselines must store
  dimension snapshots and seam findings, not a decisive repository score.
- **R10 — safety:** GitNexus impact is critical for `Synthesize` (20 impacted
  symbols/processes, 3 modules) and `ProjectReport` (16 impacted symbols/processes,
  3 modules). Any production symbol change requires upstream impact analysis.
- **R11 — fusion review:** The expert panel found the first draft unsafe without
  explicit coupling-gate retirement, schema cutover, old-baseline behavior,
  existing-path verification, required-tool semantics, and task-4 slicing. These
  are resolved below as fixed decisions, not executor questions.
- **R12 — supported-language acceptance:** The existing corpus covers Go,
  TypeScript/JavaScript, Python, and Rust, but its helper still reads legacy
  scalar fields and does not fail the sweep on per-repository config/output
  errors. V1 is not complete until the branch binary migrates corpus config
  copies, produces valid architecture-state output on every supported language,
  exposes gaps honestly, and closes confirmed compatibility defects.

## Target contract

### Primary state

Introduce `report.ArchitectureState` in `internal/model/report` with this shape:

```json
{
  "schema_version": "archfit.architecture-state.v1",
  "verdict": "healthy|needs_attention|blocked",
  "decision": {
    "hard_gates": "pass|fail|unmeasured",
    "active_blockers": 0,
    "attention_dimensions": 3,
    "unknown_dimensions": 1
  },
  "comparison": {
    "status": "comparable|non_comparable|not_requested",
    "base_ref": "main",
    "config_hash": "...",
    "model_hash": "...",
    "labels_hash": "...",
    "rubric_version": "bc_score.v6",
    "reasons": []
  },
  "measurement": {
    "source_ref": "HEAD",
    "history_depth": 500,
    "history_window": "500 commits",
    "tool_versions": {}
  },
  "dimensions": {
    "intent": {
      "status": "measured",
      "confidence": "high",
      "gate": "pass",
      "coverage": {},
      "metrics": [],
      "findings": [],
      "unknown": [],
      "delta": {}
    },
    "structure": {},
    "modularity": {},
    "coupling": {},
    "change_locality": {},
    "complexity": {},
    "testability": {},
    "operations": {},
    "drift": {}
  },
  "coverage": { "measured": 8, "partial": 1, "unmeasured": 0, "tools": [] },
  "findings": [],
  "agent_tasks": []
}
```

Every dimension uses one explicit common envelope. On the wire, `metrics` is
always an array of typed metric records, never an object or a free-form map:

```json
{
  "name": "cycle_count",
  "value": 0,
  "unit": "count",
  "denominator": { "observed": 73, "total": 73 },
  "provenance": ["go/packages"]
}
```

The `measurement` block contains only deterministic source/tool/history facts.
It excludes wall-clock evaluation time, absolute local paths, process IDs, and
other run-specific values so repeated identical runs remain byte-identical.

```go
type DimensionState struct {
    Status     MeasurementStatus // measured, partial, unmeasured
    Confidence Confidence         // high, medium, low, unrated
    Gate       GateState          // pass, warn, fail, not_applicable
    Coverage   Coverage
    Metrics    []MetricValue
    Findings   []FindingRef
    Unknown    []UnknownFact
    Delta      *DimensionDelta
}
```

Dimension-specific data stays in typed metric blocks. Do not create a generic
`map[string]any` or a new all-purpose pipeline context. The common envelope is a
reporting contract; domain calculators retain ownership of their facts.

### Decision and exit semantics

- `BLOCKED`: a valid report contains at least one active hard-gate finding, or a
  required-tool policy failure is reportable. Failure to decode config/schema,
  malformed labels, or inability to produce a valid report is a command error
  (exit 3), not a reportable `BLOCKED` state (exit 1). No dimension-specific
  metric can block unless a named hard rule owns that decision.
- `NEEDS_ATTENTION`: no blocker, but at least one active diagnostic exists or any
  dimension is partial/unmeasured. This prevents missing evidence from becoming
  a false healthy result.
- `HEALTHY`: all nine dimension envelopes are measured, every hard gate passes,
  and no active diagnostic remains. This may be uncommon on day one; it must not
  be fabricated by treating unsupported collectors as zero.
- A coupling seam never blocks merely because its per-seam score is low. The
  current `coupling.gate.min_band` and `coupling.gate.max_drop` scalar gate is
  retired. Config v1 and existing keys fail analysis/check validation with a
  migration message; the config updater can still decode them for v2 migration. The
  replacement is the explicit `coupling.gate.distributed_monolith` rule; its
  `mode: warn` default is diagnostic and its `mode: fail` behavior is opt-in.
- JSON stores lower-case `healthy|needs_attention|blocked`; human formats display
  upper case.

Frozen exit table:

| Command/result                                         | Exit |
| ------------------------------------------------------ | ---: |
| `analyze` produced a valid report, regardless of state |    0 |
| `check`: `HEALTHY`                                     |    0 |
| `check`: `NEEDS_ATTENTION`                             |    2 |
| `check`: `BLOCKED`                                     |    1 |
| parser/config/tool execution error                     |    3 |

Legacy `pass/warn/fail` maps to `healthy/needs_attention/blocked` in
`legacy-json`. `ACCEPTABLE_WITH_WATCH_ITEMS` is presentation-only legacy score
state and maps to `needs_attention`; it never changes the new gate result.
`scripts/tests/cli_exit_contract_test.sh` remains the executable authority for
0/1/2/3 behavior.

### Coupling data

Retain the Balanced Coupling formula only in the coupling dimension and seam
ledger. Report:

- logical module-pair seam ID;
- observed edge count and edge denominator;
- strength, relative module-boundary distance, and raw owner/deploy/runtime facts;
- volatility and provenance;
- quadrant (`cohesive`, `loose`, `low_cohesion`, `tight`);
- per-seam score distribution (min, p10, median, mean, max), not only a mean;
- critical/high counts and shares;
- labels used, label evidence hash, and confidence;
- role expectation (`composition_root`, `adapter`, `core`, `shared_model`);
- balancing hypothesis (`reduce_strength`, `reduce_distance`, `declare_volatility`,
  or `leave_alone`).

Distance must expose both facts and analysis level. Do not silently change the
ordinal formula. A future distance change requires a rubric/version bump and a
fixture proving contract/model/functional/intrusive quadrants.

### Labels and configuration

- Keep `.archfit-labels.yaml` empty until a module pair is manually reviewed and
  every edge in that pair has one defensible strength.
- Add deterministic validation for module-pair existence, direction, approved
  status, valid strength, provenance, confidence, and duplicate entries.
  Malformed strength, unknown module, invalid direction, or duplicate pair is a
  hard config error (exit 3 because no valid report can be produced). A stale
  evidence hash disables the override and emits a deterministic diagnostic.
  Stale evidence is diagnostic-only in v1 and cannot
  be promoted to a blocking gate.
- Derive role-aware diagnostic expectations from the existing module `role`
  field. Do not add a second role configuration system or encode composition-root
  behavior in strength labels.
- Keep one production owner/deploy unit unless deployment or ownership facts
  change. Never invent owners/deploy units to improve output.
- Comparison is strict and built in: config/model/labels/rubric hash mismatch is
  `non_comparable`, never a numerical delta. No project option may weaken it.

### Decisions frozen before execution

These decisions are part of the approved contract, not questions for Ralphex:

- Immediate primary cutover to `archfit.architecture-state.v1`; `legacy-json`
  is available for exactly one release and must be explicitly selected.
- `Scorecard.Overall`, `OverallBand`, `min_band`, and `max_drop` cannot affect the
  new decision, exit code, baseline, or delta. The final self-config contains
  `coupling.gate.distributed_monolith.mode: warn` and `max_new_seams: 0`; it does
  not enable fail mode.
- Old baseline files remain readable for accepted finding fingerprints. Their
  scalar snapshot is ignored with reason `legacy_score_snapshot_ignored`; all
  state/dimension/seam comparisons against them are `non_comparable`; files are
  never auto-rewritten. A v2 baseline is the only valid reference for a new
  distributed-monolith seam delta.
- `agent_tasks` is projected from the existing Assessment-owned agent-task result;
  state aggregation never derives a second task list.
- Unsupported runtime coverage, runtime topology, or shallow/missing history is
  reported partial/unmeasured. The v1 product does not execute a target repo's
  test suite as measurement evidence. Release acceptance still runs the Archfit
  binary itself against the read-only cross-language corpus.
- SARIF keeps finding compatibility. Architecture state is stored in run
  properties; SARIF is exempt only from human-layout parity, not fact parity.
- Config schema `version: 2` is the only analysis/check schema for the new
  binary. `config init` emits v2. `config update --migration-only --json`
  previews and `config update --migration-only --apply` performs the supported
  v1-to-v2 migration. Ordinary `config update` remains structural synchronization
  only. The updater
  deterministically changes the top-level version, removes retired
  `coupling.gate.min_band/max_drop`, inserts
  `distributed_monolith: {mode: warn, max_new_seams: 0}`, and reports the
  semantic policy change. It never infers `mode: fail`; fail remains an explicit
  owner decision after a comparable v2 baseline reference exists. Structural
  synchronization remains ordinary `config update`; it is not part of the
  migration-only candidate. V1 is accepted
  only by the migration command; `check` and `analyze` reject it with the exact
  `archfit config update --migration-only --apply` hint. Running migration twice
  is byte-idempotent.
- Corpus sweeps never overwrite target configs. They retain before/after config,
  update-report, and diff artifacts. After a candidate passes, the actual
  `.archfit.yaml` in each owner-controlled dogfood repo named by
  `docs/test-corpus.md` is updated and validated in a separate controlled
  delivery pass. A dirty target worktree blocks and requires owner action; it is
  never overwritten. External corpus repos remain read-only. Cross-repository
  config changes never enter an Archfit repository commit.

### Nine-dimensional v1 contract

| Dimension       | Evidence owner and v1 facts                                                                 | Denominator/status rule                                                            | Gate posture                                                            |
| --------------- | ------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| Intent          | Policy + Assessment: config/schema validity, modules, rules, waivers                        | configured declarations/rules; unmeasured only on invalid config                   | config validity, expired waivers, configured fail rules are hard        |
| Structure       | Relationship facts: internal edges, packages/modules, cycles, layer direction               | classified internal edges / discovered internal edges                              | cycles, forbidden/layer violations, configured coverage floors are hard |
| Modularity      | Existing modularity/file/public facts: fan-in/out, hubs, public surface, local duplication  | discovered modules/packages/public symbols                                         | diagnostic in v1; only explicit public-surface rules gate               |
| Coupling        | Relationship Analysis: seams and underlying scored/abstained edges                          | scored + abstained = internal seam edge denominator                                | diagnostic; only explicit distributed-monolith policy gates             |
| Change locality | Existing bounded git-history corroboration: commits, module co-change, history depth/window | eligible commits and changed files; shallow/zero history = unmeasured              | diagnostic                                                              |
| Complexity      | Existing LOC, file class, syntax/file facts, large-file/public-surface tails                | applicable production files; cognitive complexity unavailable = partial            | diagnostic/ratchet only                                                 |
| Testability     | Static test-file classification and test-to-production module imports                       | discovered test and production files; runtime coverage absent = partial            | diagnostic; supplied coverage gates remain explicit future work         |
| Operations      | Declared owner/deploy/external systems plus tool coverage                                   | declared modules/tools; observed runtime topology unavailable = partial            | required-tool policy only                                               |
| Drift           | Comparable baseline/base findings, seams, dimensions, config/model/labels/rubric hashes     | comparable snapshots; absent baseline or hash mismatch = unmeasured/non-comparable | configured new hard findings only; drift magnitude diagnostic           |

Coverage summary counts must sum to nine. A dimension cannot be `measured`
without a nonzero or explicitly empty denominator and provenance. V1 does not add
runtime coverage execution, runtime discovery, or a new cognitive-complexity
analyzer; later collectors can enrich an existing envelope without changing the
contract.

Operations/tool coverage uses the existing applicable-tool rows: Go packages,
SCIP, SCIP symbols, ast-grep/syntax, jscpd, LOC, dependency-cruiser,
grimp/pydeps, and cargo-modules. A tool is required only when the target language
is applicable and the existing config/CLI tool-gate marks it fail/required.
Inapplicable language tools do not lower coverage. SBOM, vulnerability, and
observed-runtime facts are explicitly unmeasured in v1.

### Gate and diagnostic policy

| Classification | Rules/facts                                                                                                                                                                                                                                                     |
| -------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Hard gate      | reportable forbidden dependency/layer rule with `gate: fail`; cycles when configured fail; required-tool policy failure after a valid report is produced; expired waiver on an active fail rule; configured coverage floor; opt-in distributed-monolith rule    |
| Diagnostic     | coupling score/distribution; local same-owner cascades; hubs/fan-in/out; public-surface size without an explicit rule; complexity; static testability; change locality; operational declarations; drift magnitude; stale label evidence; optional missing tools |

Configuration/schema decoding, malformed/unknown/duplicate labels, and any
failure that prevents a valid report are command errors (exit 3), not dimension
gates. They must be represented in stderr and a machine-readable error envelope
when no report can be produced.

The decision aggregator may read only dimension status plus explicit gate and
finding classifications. It may not inspect dimension-specific metrics or derive
implicit thresholds. Policy loosening and config hash changes are themselves
reported as drift and make cross-model numerical deltas non-comparable.

### Distributed-monolith rule

The replacement config shape is versioned atomically in config types,
`archfit.schema.json`, decoder tests, config-update review, and docs:

```yaml
coupling:
  gate:
    distributed_monolith:
      mode: warn # warn | fail; default warn
      max_new_seams: 0
```

A distributed-monolith seam is one logical ordered module pair containing at
least one **active edge**: a current source-graph edge with a known source and
target module, a critical-band classification at high raw distance
(`different_owner` or `different_deploy_unit`), regardless of advisory display
filters, baseline acceptance, or waiver status. The rule counts seams, not import
edges. `mode: fail` blocks only when a comparable reference exists and the number
of newly introduced qualifying seams exceeds `max_new_seams`; without a
comparable reference it emits an unrated diagnostic. An absent stanza defaults to
`mode: warn`, `max_new_seams: 0`; the archfit self-config remains warn in v1.
Users opt into fail only after a report-only run against a comparable v2 baseline
shows the expected qualifying seam count.

Comparison reference precedence is fixed: an explicit `--base` reference wins;
otherwise the stored v2 baseline is used; otherwise no “new seam” claim is made.
A pre-state baseline is non-comparable for dimensions/seams even though accepted
finding fingerprints remain usable. `config_hash` covers normalized config,
`model_hash` covers canonical module map/public surfaces, and `labels_hash` covers
canonical label entries; all three plus `rubric_version` are required for
comparable seam deltas.

Seam ID is `sha256("seam.v1\x00" + fromModule + "\x00" + toModule)`. Module-map
hash mismatch makes comparison non-comparable, so a module rename cannot appear
as a comparable resolved/new seam. The existing edge-level `DistributedMonolith`
score-cap field and `DistributedMonolithEdges` report count are reconciled in v1
as `critical_high_distance_edges`; both remain diagnostic facts and neither
implements the seam gate. The new `distributed_monolith` seam gate is a separate
policy result. Scores are sorted integers; nearest-rank percentiles use
`ceil(p*n)-1`; p10/p90 are null when `n < 10`. Seam severity is its worst active
underlying edge band, not a second repository scalar.

The executable dry run runs `make build` first, then the freshly built binary,
with the self-config in `mode: warn`; it records total qualifying seams and, only
when the reference is comparable, new qualifying seams. It must not claim zero
new seams against a non-comparable legacy baseline.

## Success criteria

- New JSON schema is versioned and has no decisive repository-level score.
- Text/console, Markdown, SARIF, scorecard, and JSON all show the same decision,
  nine dimensions, coverage/unknown state, attention summary, and comparable
  delta semantics.
- Legacy score is absent from new primary output. Any temporary compatibility
  representation is explicitly named `legacy` and never drives verdict/exit code.
- Hard-gate findings and diagnostics are separate in the domain model, baseline,
  reports, and exit mapping.
- Config/model/labels/rubric hash mismatch produces `comparison.status=non_comparable` and
  explains why; no numerical score delta is emitted as if comparable.
- Coupling reports seam-level distributions and raw distance context; the
  repository mean is not a decision.
- All nine dimensions have deterministic measurement definitions, denominator,
  status, confidence, unknown handling, and owner.
- History with zero observations is `unmeasured`; absent test/ops tools are
  `partial` or `unmeasured` with named missing tools.
- Labels are validated and provenance-bearing. No label is added merely to move a
  score or severity band.
- Existing finding IDs/statuses/order, output evidence blocks, baseline accepted
  findings, enrich/config schemas, `--base`, and exit codes remain compatible or
  are covered by a deliberate schema migration note.
- New hard gates fail on a fixture before the fix and pass after the fix.
- Existing owner-local and output contract tests remain green; new state/report
  tests are deterministic across repeated runs.
- `.archfit.yaml`, `.archfit-labels.yaml`, architecture docs, baseline schema, and
  CLI help describe the same implemented behavior.
- No module/package cycles, dead rules, uncovered production paths, or stale
  labels remain.

## Validation commands

Per-task commands are mandatory. Whole-plan validation is:

```sh
make fmt
make lint
go vet ./...
make test
make build
make archfit
git diff --check

.bin/archfit doctor
set +e
.bin/archfit check --config .archfit.yaml --root "$PWD" > /tmp/archfit-state-check.out 2>&1
check_rc=$?
set -e
test "$check_rc" -eq 0 || test "$check_rc" -eq 2
.bin/archfit analyze --json --config .archfit.yaml --root "$PWD" \
  > /tmp/archfit-state-head.json
.bin/archfit analyze --json --base main --config .archfit.yaml --root "$PWD" \
  > /tmp/archfit-state-delta.json

for format in json text markdown sarif scorecard; do
  .bin/archfit analyze --format "$format" --config .archfit.yaml --root "$PWD" \
    > "/tmp/archfit-state-$format.out"
done

bash scripts/tests/cli_exit_contract_test.sh
python3 internal/extract/scip/scip_reader_test.py
node .gitnexus/run.cjs analyze
git status --short
```

For every task, run:

```text
gitnexus_impact(target=<changed symbol>, direction="upstream", depth=4,
                include_tests=true, repo="archfit")
gitnexus_detect_changes(scope="all", repo="archfit")
```

Use `gitnexus_rename(..., dry_run=true)` for symbol renames. If GitNexus is
unavailable, record the fallback `git diff --name-only`, `go list`, and direct
import-graph command; do not claim exact graph coverage.

## Implementation Steps

### Task 1: Freeze the state contract and behavior compatibility matrix

Justification: R1, R8, R9, R10; the `Synthesize` and `ProjectReport` blast radius
is critical. Define the new contract before changing scoring or renderers.

Files:

- `internal/model/report/document.go`, `internal/model/report/report.go`, and
  new focused report contract files — add `ArchitectureState`, `DimensionState`,
  coverage/confidence/unknown/delta types, schema version, and stable JSON tags.
- `internal/assessment/result/**` and `internal/assessment/score/**` — separate
  gate result, diagnostic result, and legacy coupling diagnostics without yet
  removing behavior.
- `internal/application/report.go` — project explicit dimension blocks and
  preserve all existing evidence blocks.
- `internal/output/{jsonout,console,markdown,sarif,scorecard}/*` — add contract
  fixtures and adapters behind the new report state; do not remove old output
  until migration tests exist.
- `cmd/archfit/{formats,analyze,check,main}.go` — define new format/version
  selection and keep old flags/exit mapping explicit.
- `cmd/archfit/*_test.go`, `internal/model/report/report_test.go`, and
  `internal/application/{golden_test,report_test,report_blocks_test}.go` — add
  schema fixtures, deterministic double-run checks, ordering checks, and a
  compatibility matrix for all five formats.
- `docs/design/architecture-state-reporting.md` — record the approved contract,
  decision semantics, and compatibility policy.

Preconditions: current `make all`, golden tests, and GitHub CI are green; current
JSON/Markdown/SARIF/scorecard/console samples are captured outside the repository.

Postconditions: new report types compile and are populated in a shadow field or
feature-selected path; current output is unchanged by default; every field has an
owner and deterministic serialization rule.

Fitness gate: add an architecture-test fixture that fails when report adapters
import Assessment/Relationship internals or when a new unversioned report field
is added. Existing `assessment_no_report_dtos` and relationship report rules stay
passing.

Impact commands:

- `gitnexus_impact(target="Synthesize", direction="upstream", depth=4, include_tests=true, repo="archfit")` — CRITICAL, 20 impacted.
- `gitnexus_impact(target="ProjectReport", direction="upstream", depth=4, include_tests=true, repo="archfit")` — CRITICAL, 16 impacted.
- `gitnexus_impact(target="Struct:internal/model/report/document.go:Document", direction="upstream", depth=4, include_tests=true, repo="archfit")`.
- `gitnexus_detect_changes(scope="all", repo="archfit")`.

Verification commands:

```sh
go test ./internal/model/report ./internal/application ./internal/output/... -count=1
go test ./cmd/archfit -run 'Test.*(JSON|Markdown|SARIF|Scorecard|Console|Golden|Exit)' -count=1
go test ./internal/ -run 'TestArchImports|TestModelSurfaceNoDrift' -count=1
make archfit
git diff --check
```

Manual checks:

- Confirm the explicit product decisions: primary formats cut over to
  `archfit.architecture-state.v1` now; `legacy-json` is opt-in for one release;
  score-gate fields are rejected; diagnostics-only `analyze` exits 0 and
  diagnostics-only `check` exits 2.
- Confirm no legacy score field can affect verdict, exit code, gate evaluation, or
  baseline acceptance.
- Confirm every dimension has a named evidence owner and no generic map is used.

- [x] Capture current format fixtures and exit-code behavior.
- [x] Add versioned `ArchitectureState` and common dimension envelope.
- [x] Add report projection and shadow population without changing default output.
- [x] Add deterministic format/ordering/schema/compatibility tests.
- [x] Add report-boundary architecture tests.
- [x] Record the approved output migration decision and commit the contract.

### Task 2: Separate results, populate nine envelopes from existing facts, and aggregate state

Justification: R1, R7, R8, R11; current `finalize` mixes score synthesis,
coupling-gate application, findings, metrics, and repair tasks. The safe v1
reuses existing facts and marks unsupported evidence partial/unmeasured instead
of building several new analyzers at once.

Files:

- `internal/assessment/evaluation/{evaluation,finalize,assess}.go` — expose the
  existing active hard-gate findings separately from diagnostics, while keeping
  finding identity/status assignment and agent-task construction unchanged.
- new `internal/assessment/state/{state,decision}.go` — own only common envelope
  types and deterministic `healthy/needs_attention/blocked` aggregation. The
  aggregator may read statuses, finding kinds, and explicit gate results; it may
  not inspect dimension-specific metrics.
- `internal/assessment/result/**` — carry hard-gate, diagnostic, metric, coverage,
  and existing agent-task results without a second task derivation.
- existing `internal/assessment/metrics/{boundary,modularity}/**` and
  `internal/relationship/facts/**` — provide v1 facts/denominators for intent,
  structure, modularity, coupling, and partial complexity/testability/operations.
- existing history/volatility evidence — populate change-locality or mark it
  unmeasured with fixed window/depth reasons.
- `internal/assessment/decision/**` — retain existing finding/config/git delta
  logic; add only dimension status/delta mapping that belongs there.
- `internal/application/analysis.go` and `internal/application/report.go` — pass
  the explicit state result and project the report contract.
- Focused tests in `internal/assessment/{evaluation,state,result,decision}` and
  existing metric/fact packages — table-driven measured, partial, unmeasured,
  pass, warn, fail, unknown, no-findings, and missing-tool cases.

Preconditions: Task 1 contract exists; current score/gate behavior is captured;
all current rules, findings, agent tasks, and exit mappings are known.

Postconditions: no score field is consulted by the new state aggregator; all nine
envelopes exist; unsupported runtime/test/history facts are explicit; hard gates
and diagnostics are separate; the old output remains available until Task 4.

Required commit slices inside this task:

- **2A:** separate gate/diagnostic results with no output or exit change;
- **2B:** populate nine envelopes from existing deterministic facts, using
  partial/unmeasured for unsupported collectors;
- **2C:** add state aggregation and mapping to the shadow report contract.

Each slice runs focused tests and creates its own commit before continuing.

Fitness gate:

- An imbalanced coupling diagnostic without a hard gate produces
  `NEEDS_ATTENTION`; `analyze` maps it to 0 and `check` to 2 in Task 4.
- A forbidden dependency/cycle/required-tool failure produces `BLOCKED` and maps
  to existing hard-gate exit 1 in Task 4.
- No history or runtime test profile produces `PARTIAL`/`UNMEASURED` and
  `NEEDS_ATTENTION`, never `HEALTHY` by omission.
- Coverage counts always sum to nine; no measured dimension lacks a denominator.
- Add `assessment_no_score_decision` and `dimension_coverage_required` rules.

Impact commands:

- `gitnexus_impact(target="finalize", direction="upstream", depth=4, include_tests=true, repo="archfit")`.
- `gitnexus_impact(target="evaluate", file_path="internal/assessment/evaluation/evaluation.go", direction="upstream", depth=4, include_tests=true, repo="archfit")`.
- `gitnexus_impact(target="Synthesize", direction="upstream", depth=4, include_tests=true, repo="archfit")`.
- `gitnexus_detect_changes(scope="all", repo="archfit")`.

Verification commands:

```sh
go test ./internal/assessment/evaluation ./internal/assessment/result \
  ./internal/assessment/decision ./internal/assessment/metrics/... \
  ./internal/relationship/facts ./internal/relationship/analysis -count=1
# Run after slice 2C creates the path:
go test ./internal/assessment/state -count=1
go test ./cmd/archfit -run 'Test.*(Score|Gate|Warning|Coverage|Delta)' -count=1
make archfit
```

Manual checks:

- Confirm diagnostics cannot change exit status through an accidental shared
  `Verdict` or score threshold.
- Confirm an unmeasured dimension lowers confidence/decision only according to
  explicit policy, not a hidden default.
- Confirm dimension ownership does not introduce a generic `state` god package.

- [x] Slice 2A: separate hard-gate and diagnostic results; project existing
      agent tasks; prove output and exit behavior are unchanged; commit.
- [x] Slice 2B: populate all nine envelopes from existing facts and explicit
      partial/unmeasured states; prove denominators/order; commit.
- [x] Slice 2C: implement metric-blind state aggregation and shadow report
      mapping; prove healthy/attention/blocked fixtures; commit.
- [x] Add fail-before/pass-after architecture fixtures preventing score reads and
      missing dimension status.
- [x] Preserve finding identity, status, ordering, and exit behavior until the
      explicit Task 4 cutover.

### Task 3: Replace scalar coupling with seam-ledger diagnostics and honest labels

Justification: R2-R6; current scalar is edge-count sensitive, distance-compressed,
and role-blind. Coupling remains valuable at seam level but must not decide repo
health.

Files:

- `internal/assessment/score/score.go` and
  `internal/assessment/score/score_boundary_coupling.go` — retain per-edge/per-seam
  calculation for diagnostics; remove `Overall` as a primary decision input;
  compute distribution/tail metrics and stable logical seam IDs.
- `internal/relationship/classify/classify.go` and
  `internal/relationship/coupling/**` — expose strength evidence, analysis level,
  relative boundary distance, raw owner/deploy/runtime facts, and abstention
  reasons. Preserve formula/rubric version until separately approved.
- `internal/relationship/analysis/**` and `internal/relationship/facts/**` — group
  edges by ordered module pair and emit seam denominators, distributions, role,
  provenance, and duplicate-knowledge facts.
- `internal/relationship/labels/**`, `internal/labels/labelsio/**`, and
  `.archfit-labels.yaml` — validate approved label direction, module existence,
  evidence hash, status, strength, provenance, confidence, and duplicate pairs.
- `internal/config/**`, `internal/initcfg/**`, `internal/configschema/**`,
  `cmd/archfit/config_update*.go`, `cmd/archfit/update_test.go`,
  `archfit.schema.json`, `.archfit.yaml`, config-update review, and configuration
  docs — add the explicit config v1-to-v2 migration, make `config init` emit v2,
  and atomically replace `coupling.gate.min_band/max_drop` with the frozen
  `coupling.gate.distributed_monolith.{mode,max_new_seams}` schema. Derive
  role-aware diagnostics from existing `module.role`; do not add a duplicate role
  key or change owner/deploy/volatility for reporting.
- `internal/assessment/decision/config_compare.go` and
  `internal/application/base_compare.go` — make coupling comparisons
  non-comparable when config/model/labels/rubric hashes differ; compare seam IDs and
  distributions when comparable.
- `internal/assessment/score/*_test.go`, `internal/relationship/{classify,analysis,
labels}/*_test.go`, `cmd/archfit/config_compare_test.go` — fixtures for local
  cohesive coupling, loose contract coupling, tight cross-owner coupling,
  distributed monoliths, edge-count aggregation, stale labels, empty labels,
  role-aware composition roots, and model changes.

Preconditions: Task 2 emits the coupling envelope through the shadow state path;
current legacy score fixtures and PR-vs-main evidence are preserved. The self
configuration still uses the default `mode: warn`; no fail gate is enabled by
this task.

Postconditions: coupling output is a seam ledger with distributions and raw
context; the repository scalar is absent from the primary report; labels affect
only valid approved strength overrides; same-owner local cascades are not called
distributed monoliths.

Fitness gate:

- A fixture introducing one critical-band seam across different owners or deploy
  units must produce one distributed-monolith seam. With comparable baseline,
  `mode: fail`, and `max_new_seams: 0`, it blocks; `mode: warn` is diagnostic.
- The current self-config is measured report-only with `make build` first. It
  records total qualifying seams and reports new seams only when the selected
  reference is comparable; no fail mode is enabled in this task. The v1 self
  config uses `mode: warn`, `max_new_seams: 0`.
- A same-owner strong Relationship → Assessment fixture remains diagnostic.
- Malformed/unknown-direction/duplicate labels fail config validation. A stale
  evidence hash disables the override and emits a deterministic diagnostic.
- `check`/`analyze` accept only config v2 and reject v1 or retired
  `min_band/max_drop` with the exact `archfit config update --migration-only --apply`
  migration message. The migration-only preview/apply path can decode v1,
  reports and applies the version bump plus deterministic warn-mode replacement,
  preserves unrelated authored YAML, reports the semantic policy change, and is
  byte-idempotent. Ordinary config update remains structural synchronization.
- Config/model/labels/rubric mismatch emits `non_comparable` and no numerical delta.

Impact commands:

- `gitnexus_impact(target="Run", file_path="internal/relationship/classify/classify.go", direction="upstream", depth=4, include_tests=true, repo="archfit")`.
- `gitnexus_impact(target="Synthesize", direction="upstream", depth=4, include_tests=true, repo="archfit")`.
- `gitnexus_impact(target="Score", file_path="internal/relationship/scoring/scorer.go", direction="upstream", depth=4, include_tests=true, repo="archfit")`.
- `gitnexus_detect_changes(scope="all", repo="archfit")`.

Verification commands:

```sh
go test ./internal/relationship/... ./internal/assessment/score \
  ./internal/assessment/decision ./internal/config ./internal/configschema -count=1
go test ./cmd/archfit -run 'Test.*(Label|Coupling|ConfigCompare|Score)' -count=1
make build
# Primary JSON v1 is owned by Task 4B. Task 3 validates the domain seam facts
# without asserting a renderer contract that does not exist yet.
go test ./internal/relationship/... ./internal/assessment/score \
  -run 'Test.*(Seam|Coupling|Distributed|Determin|Label)' -count=1
make archfit
```

Manual checks:

- Review the top module-pair seams by reading the actual crossing types. Do not
  relabel a mixed pair globally.
- Confirm the distance level is explicit and no score change is attributed to a
  fabricated owner/deploy boundary.
- Confirm empty labels means “no overrides”, not “all edges are contract”.
- Confirm the scalar may appear only under a seam’s diagnostic details or a
  versioned legacy field.

- [x] Implement logical seam grouping and denominator/distribution metrics.
- [x] Add raw distance context and explicit analysis-level fields.
- [x] Add config v2 init/load/schema plus the exact distributed-monolith
      config-update report/apply/idempotence path, v1/retired-field migration
      error, and current-HEAD dry-run before enabling fail mode.
- [x] Separate diagnostic coupling from the explicit per-seam gate policy.
- [x] Add hard structural label validation plus diagnostic stale-evidence handling.
- [x] Derive composition-root/adapter expectations from existing roles without
      label hacks or new role keys.
- [x] Make config/model/labels/rubric comparison explicit and non-comparable when needed.
- [x] Add seam fixtures and preserve per-edge explain evidence.
- [x] Remove repository `Scorecard.Overall` from the new primary decision path.
- [x] Run focused tests and commit the seam-ledger implementation.

Current-HEAD dry run (2026-08-26, `make build` then the freshly built binary,
self-config in `mode: warn`, `max_new_seams: 0`): 380 scored internal
cross-boundary edges, **all** at `cross_module_same_owner`, 78 in the critical
band, **0** at high distance — therefore **0 qualifying distributed-monolith
seams**. This matches R3 exactly. No new-seam count is claimed: the stored
baseline predates the seam ledger and is non-comparable, so the gate abstains.
`analyze` prints nothing, which is correct — with no qualifying seam there is no
claim to withhold. Fail mode is NOT enabled: on this tree it would gate on
nothing while reading as a control that exists.

### Task 4: Migrate all formats, baseline, delta, and CLI behavior

Justification: R1, R9-R11; all supported output adapters and baseline flows
currently depend on `Scorecard.Overall`. The first fusion draft made this one
unsafe mega-commit, so this task has five mandatory independently revertible
commit slices.

Files:

- `internal/application/baseline.go` and the existing baseline adapter/model —
  persist schema version, config/model/labels/rubric hashes, hard-gate finding
  IDs, dimension snapshots, seam IDs, and accepted findings. Reading a pre-state
  baseline preserves accepted finding fingerprints, ignores its score snapshot
  with `legacy_score_snapshot_ignored`, marks all state/dimension/seam deltas
  `non_comparable`, and never auto-rewrites the file.
- `internal/application/base_compare.go`, `internal/application/compare.go`, and
  `internal/assessment/decision/{git_finding_delta,config_compare}.go` — compare
  dimensions, findings, seams, coverage, and comparability reasons; any config,
  model, labels, or rubric hash mismatch
  is non-comparable.
- `internal/application/report.go` and
  `internal/application/relationship_report.go` — project the complete state and
  preserve existing evidence blocks without leaking domain DTOs.
- `internal/output/console/report.go` (`--format text`, also the default) — implement the headline:
  `ARCHITECTURE STATE / VERDICT / BLOCKING / ATTENTION / COVERAGE`, followed by
  nine dimensions and top actionable findings.
- `internal/output/jsonout/**` — emit `archfit.architecture-state.v1` with stable
  keys, explicit null/empty/unmeasured semantics, and no repo scalar.
- `internal/output/markdown/**` — render the same headline, dimension table,
  coverage table, seam ledger, delta/comparability, and unknowns.
- `internal/output/sarif/**` — keep SARIF finding compatibility; encode gate versus
  diagnostic properties, dimension, confidence, coverage, and seam metadata.
- `internal/output/scorecard/**` — replace scalar scorecard with nine-dimensional
  state scorecard; do not call it an architecture score.
- `cmd/archfit/{analyze,check,baseline,config_compare,explain,main}.go` — update
  help, format dispatch, baseline options, report-only behavior, exit mapping,
  and the frozen one-release `legacy-json` selection.
- All format, baseline, comparison, golden, byte-identical, explain, and exit
  tests under `cmd/archfit`, `internal/application`, `internal/output`, and
  `internal/model/report`.
- `README.md`, `CLAUDE.md`, `CONTRIBUTING.md`, and command docs — remove scalar
  headline language and document state/diagnostic/gate semantics.

Preconditions: Tasks 1-3 pass and all frozen compatibility decisions are present
in tests; no new decision path reads the scalar; legacy adapters remain green
until their migration slice.

Postconditions: all five primary formats show the same structured state; opt-in
`legacy-json` is isolated; baseline and `--base` compare dimensions only when
hashes match; old accepted finding IDs/statuses remain stable; the frozen
0/1/2/3 exit table passes.

Required commit slices:

- **4A — baseline and compare:** fix idempotence; add baseline v2 read/write and
  old-baseline read behavior; no renderer cutover.
- **4B — JSON:** cut `--format json` to architecture-state v1; add
  `legacy-json`; byte-identical double-run and ordering tests.
- **4C — text/console and Markdown:** add the headline and dimension tables; normalized
  parity with JSON.
- **4D — SARIF and scorecard:** add SARIF run properties and replace scalar
  scorecard presentation; preserve SARIF finding/rule identity.
- **4E — CLI/help/docs:** freeze format dispatch, `analyze`/`check` exit mapping,
  config/baseline migration help, and user docs.

Each slice runs its focused test set and creates one commit. Do not proceed to a
later slice with a red earlier adapter.

Fitness gate:

- Add cross-format fixture tests that normalize JSON/console/Markdown/scorecard
  and compare decision, dimensions, counts, and finding IDs.
- SARIF must retain every gate finding and must distinguish diagnostics from gates.
- A report containing only coupling advisories renders `NEEDS_ATTENTION`;
  `analyze` exits 0 and `check` exits 2, never 1.
- Baseline with a config/model/labels/rubric hash mismatch reports
  non-comparable and emits no false numerical delta. A pre-state baseline is
  always non-comparable for state/dimension/seam deltas, even when fingerprints
  are accepted.
- Two identical binary runs produce byte-identical JSON; ordering is fixed for
  dimensions, findings, seams, reasons, unknowns, and tools.
- All nine dimension keys exist in every primary format; measured/partial/
  unmeasured counts sum to nine. SARIF may differ in layout only.

Impact commands:

- `gitnexus_impact(target="ProjectReport", direction="upstream", depth=4, include_tests=true, repo="archfit")`.
- `gitnexus_impact(target="BaselineService.Execute", direction="upstream", depth=4, include_tests=true, repo="archfit")`.
- `gitnexus_impact(target="scoreBaseTree", file_path="internal/application/base_compare.go", direction="upstream", depth=4, include_tests=true, repo="archfit")`.
- `gitnexus_detect_changes(scope="all", repo="archfit")`.

Verification commands:

```sh
go test ./internal/model/report ./internal/application ./internal/output/... -count=1
go test ./cmd/archfit -run 'Test.*(Main|Analyze|Check|Baseline|Compare|Explain|Format|Golden|Byte|Exit)' -count=1
bash scripts/tests/cli_exit_contract_test.sh
make build
for format in json text markdown sarif scorecard; do
  .bin/archfit analyze --format "$format" --config .archfit.yaml --root "$PWD" \
    > "/tmp/state-$format.out"
done
jq -e '{verdict,decision,dimension_keys:(.dimensions|keys)} | select(.verdict != null and (.dimension_keys|length) == 9)' /tmp/state-json.out
make test
make archfit
```

Manual checks:

- Inspect representative reports for readability and equal facts across formats.
- Confirm SARIF consumers still receive stable finding IDs and rule IDs.
- Confirm `baseline` never silently stores or compares a non-comparable model.
- Confirm report-only mode still renders a failing gate while preserving command
  exit semantics.

- [ ] Slice 4A: reproduce baseline non-idempotence, fix it, implement baseline v2
      plus old-baseline reads, compare semantics, focused tests, and commit.
- [ ] Slice 4B: implement primary JSON v1 plus `legacy-json`, double-run byte
      identity, stable ordering, schema tests, and commit. Use `jq -e` for
      required fields so absent state cannot pass verification.
- [ ] Slice 4C: implement text/console and Markdown headline and normalized parity; commit.
- [ ] Slice 4D: implement SARIF run properties and state scorecard while
      preserving finding identity; commit.
- [ ] Slice 4E: update CLI dispatch/help/exit contract and user docs; run
      cross-format, golden, compare, explain, and shell exit tests; commit.

### Task 5: Finalize v1 evidence status, self-config, and erosion controls

Justification: R3-R9, R11; the state report is useful only when it discloses
measurement limits. V1 does not execute foreign tests or claim observed runtime
facts. Unsupported collectors remain explicit partial/unmeasured envelopes for a
later approved plan.

Files:

- Existing LOC/file-class/syntax facts, test-file classification, history
  corroboration, module/deploy declarations, and tool-coverage projections — map
  only observed v1 facts into complexity, testability, change-locality, and
  operations envelopes. Cognitive complexity, runtime coverage, observed runtime
  topology, SBOM, and vulnerability state remain named unknowns unless supplied.
- `internal/application/analysis.go` and evidence acquisition ports — supply
  fixed tool-version, history depth/window, and canonical source-ref metadata in
  the root `measurement` block. The canonical `config_hash`, `model_hash`,
  `labels_hash`, and `rubric_version` live only in the root `comparison` block
  defined above. Evaluation wall-clock time is
  excluded from primary serialization,
  semantic hashes, and byte-identity tests; it may appear only as an explicitly
  non-deterministic diagnostic field.
- `internal/config/**`, `internal/initcfg/**`, `internal/configschema/**`,
  `archfit.schema.json`, and `.archfit.yaml` — verify config v2, the
  distributed-monolith schema, current module roles, required tool gates,
  comparison behavior, and
  exact generated schema. Correct “frozen” to “pinned published contract”.
- `.archfit-labels.yaml`, `internal/labels/labelsio/**`, and label tests — keep
  empty unless evidence-backed approved pairs exist; structural errors fail,
  stale hashes diagnose and disable the override.
- `.archfit-baseline.json` — do not write the final baseline during Task 5.
  Preserve the old version in git history and do not retain a decisive scalar
  snapshot. Task 6 owns corpus acceptance, final architecture re-review, and the
  post-GO baseline write.
- `internal/arch_test.go`, architecture fixture tests, and CI workflow — enforce
  no cycles/inversions, complete paths, no dead rules, valid labels, no
  score-based decisions, required tool coverage, nine dimension keys/statuses,
  and approved output-schema drift.
- `docs/design/architecture-state-reporting.md`,
  `docs/design/architecture-baseline.md`,
  `docs/design/20260823-archfit-capability-map.md`, `README.md`, `CONTRIBUTING.md`,
  `CLAUDE.md`, and `AGENTS.md` — document the nine dimensions, ownership,
  confidence/coverage semantics, label policy, gate policy, maintenance recipes,
  and accepted coupling.

Preconditions: Tasks 1-4 pass; output migration is complete; current config and
label fixtures are captured; tool availability is known on local and CI runners.

Postconditions: all dimensions have truthful measurement/unknown handling;
complexity/testability/operations do not overclaim runtime evidence; configuration
matches physical modules and intent; labels are deterministic; CI blocks erosion
without a scalar threshold. The branch binary and self-config are ready for Task
6 corpus acceptance. No final baseline has been written.

Fitness gate:

- The new `no_scalar_decision`, `dimension_status_required`, `config_hash_required`,
  `label_evidence_required`, `baseline_idempotent`, and `no_dead_archfit_rule`
  checks fail on pre-fix fixtures and pass on final source.
- `archfit check` has zero unexpected blocking findings, zero uncovered
  production paths, zero dead modules/rules/globs, and no structurally invalid
  labels. Optional missing tools remain visible and may create attention.
- A shallow/no-history fixture is unmeasured with recorded window/depth. A
  controlled-PATH fixture proves optional versus required tool behavior.
- Complexity/testability/history/operations are diagnostic in v1; no implicit
  thresholds or runtime claims are added.

Impact commands:

- `gitnexus_impact(target="Build", file_path="internal/relationship/facts/facts.go", direction="upstream", depth=4, include_tests=true, repo="archfit")`.
- `gitnexus_impact(target="Load", file_path="internal/labels/labelsio/labelsio.go", direction="upstream", depth=4, include_tests=true, repo="archfit")`.
- `gitnexus_impact(target="BaselineService.Execute", direction="upstream", depth=4, include_tests=true, repo="archfit")`.
- `gitnexus_detect_changes(scope="compare", base_ref="main", repo="archfit")`.
- `gitnexus_detect_changes(scope="all", repo="archfit")`.

Verification commands:

```sh
make fmt
make lint
go vet ./...
make test
make build
make archfit
git diff --check

.bin/archfit doctor
set +e
.bin/archfit check --json --config .archfit.yaml --root "$PWD" > /tmp/state-check.json 2>&1
check_rc=$?
set -e
test "$check_rc" -eq 0 || test "$check_rc" -eq 2
.bin/archfit analyze --json --config .archfit.yaml --root "$PWD" > /tmp/state-final.json
.bin/archfit analyze --json --base main --config .archfit.yaml --root "$PWD" > /tmp/state-delta.json
.bin/archfit baseline --help
.bin/archfit config update --json --config .archfit.yaml > /tmp/config-review.json
bash scripts/tests/cli_exit_contract_test.sh
python3 internal/extract/scip/scip_reader_test.py
node .gitnexus/run.cjs clean
node .gitnexus/run.cjs analyze
```

Manual checks:

- Review every module’s owner, deploy unit, role, layer, subdomain, volatility,
  public surface, and paths against the source tree.
- Review every label entry against current evidence and reject score-driven
  rationale.
- Review each dimension’s denominator and unknown bucket; no empty list may mean
  “healthy” without a measurement status.
- Run repeated config/report commands and compare bytes and semantic hashes;
  record both update exits and require the second migration exit to be 0.
- Confirm `.archfit-baseline.json` was not regenerated; Task 6 performs the final
  architecture review and post-GO baseline write after corpus acceptance.
- Run five cold/warm `scripts/bench-gate.sh` trials before and after the migration;
  report p50 and max memory. A regression above 25% requires investigation, not
  an automatic architecture verdict.
- Review representative change scenarios: new rule, new analyzer, relationship
  semantic change, renderer field, and new CLI use case. Record touched modules
  and whether the delta is comparable.

- [ ] Map existing static facts into complexity, testability, operations, and
      history envelopes; list unsupported runtime evidence as unknown. Populate
      only deterministic root `measurement` fields; keep the four comparability
      hashes only in the root `comparison` block.
- [ ] Finalize drift/config/model/labels/rubric hashes and reuse Task 4 baseline
      idempotence/comparability checks without writing the final baseline yet.
- [ ] Finalize truthful `.archfit.yaml`, `archfit.schema.json`, generated schema,
      and label validation atomically.
- [ ] Add all erosion-prevention architecture fixtures and CI gates.
- [ ] Update architecture and contributor documentation.
- [ ] Refresh GitNexus and record Task 5 compare/all change evidence.
- [ ] Run full self-validation without regenerating the final baseline.
- [ ] Commit Task 5 so the branch binary and self-config are stable inputs to the
      corpus acceptance task.

### Task 6: Validate v1 across the cross-language corpus and close compatibility gaps

Justification: R12; unit fixtures and self-dogfooding cannot prove that config
migration, extraction, state projection, format output, and exit semantics work
on the supported Go, TypeScript/JavaScript, Python, and Rust adapters. The
existing corpus workflow is the product-level acceptance gate.

Files:

- `scripts/eval/corpus_sweep.py` and a focused stdlib Python test file — replace
  legacy `score_overall/score_band` parsing with the v1 state contract; add a
  strict mode, a frozen nested per-repo result record, config before/after/
  update-report/diff artifacts, config-update idempotence checks, expected
  exit/verdict validation, per-language format parity, and deterministic repeat
  comparison. The migration-only candidate is immutable input; an AI-summary
  overlay is a separate temporary copy and can never be a delivery candidate.
  `cmd/archfit/update.go`, `internal/application/config_update.go`, and
  `internal/initcfg/**` own the explicit `config update --migration-only`
  preview/apply path; the helper must invoke that path for this
  candidate; structural synchronization is a separate review artifact and is
  never silently bundled into release-config delivery.
- `docs/test-corpus.md`, `skills/archfit-eval/SKILL.md`,
  `skills/archfit-eval/references/corpus-sweep.md`, and
  `docs/book-alignment-review-prompt.md` — document v1 fields, strict acceptance,
  config candidate delivery, exact representative/full commands, and the
  distinction between product defects, target architecture findings, missing
  optional tools, and config-policy decisions.
- `/tmp/archfit-corpus-eval/` and `/tmp/archfit-corpus-results.json` — ephemeral
  sweep artifacts only. For every repo retain config source hash, original
  config, migration-only candidate, optional AI overlay in a different file,
  `config update --json` report, applied diff, second-update idempotence result,
  command exits/stderr, parsed state summary, format facts, and repeat
  comparison. Delivery receipts and target-repo diffs live under the separate
  `/tmp/archfit-corpus-delivery-receipts/` tree because the sweep may delete
  per-repo output directories. Never add these artifacts to git.
- Corpus target configs — read-only during the sweep. For each of the three
  owner-controlled dogfood repos named in `docs/test-corpus.md` (`spotinfo`,
  `pumba`, and `ccgram`), require a clean worktree and the exact swept HEAD and
  source-config hash, then apply only the immutable migration-only candidate to
  its actual `.archfit.yaml` in a separate two-phase delivery pass. Stage and
  validate all three candidates before replacing any target file; retain
  backups, receipts, and target-repo diffs outside the sweep output tree, and
  restore all files if post-delivery validation fails. External corpus repos
  remain read-only.
- Production/extractor/config/output files discovered by a failing corpus case —
  edit only after reproducing the failure, classifying it as an Archfit defect,
  running GitNexus upstream impact on every selected production symbol, and
  adding a focused regression fixture. Do not alter target-project architecture,
  labels, owners, deploy units, volatility, baselines, or rules to make output
  look better.

Preconditions: Tasks 1-5 pass and are committed; `.bin/archfit` is built from the
current branch; `doctor` output is captured; corpus roots match
`docs/test-corpus.md`; target worktree status and config hashes are recorded
before any command. A missing mandatory representative blocks this task rather
than becoming a silent skip.

Postconditions: the current branch binary has migrated config candidates and
valid v1 results for every available listed corpus repo, including at least one
mandatory Go, TypeScript/JavaScript, Python, and Rust representative. The full
corpus has no unexplained config-update error, schema/parse error, crash, exit
mismatch, or nondeterministic repeat. Each confirmed Archfit defect has a
regression test and focused fix; each remaining gap is explicitly classified.
Every available corpus repo has a valid v1 result, or an explicitly recorded
and owner-accepted unverified gap that is not counted as a pass. Every
owner-controlled dogfood repo config matches its validated migration-only
candidate and passes the branch binary; those cross-repository diffs remain
separate from the Archfit commit. Task 6 acceptance is corpus acceptance, final
repository validation, and successful controlled delivery; the final review and
baseline creation follow those completed prerequisites.

Fitness gate:

- Fast mandatory matrix: `spotinfo` (Go), `storybook` (TypeScript/JavaScript),
  `ccgram` (Python), and `herdr` (Rust). Each config migrates to `version: 2`
  with the branch binary, loads after migration, and is byte-idempotent on a
  second update.
- Full corpus: `spotinfo,pumba,omni/scheduled-tasks,prometheus,ccgram,prefect,storybook,yazi,herdr,ruff,tokio`.
  Missing repos are reported as `unverified` and block strict mode. An explicit
  owner-supplied `--allow-unverified label=reason` may change that record to
  `accepted_unverified` and allow strict mode to continue, but it never changes
  it to `pass`; the final review must disclose the accepted gap. The full corpus
  gate still requires exactly 11 records.
- Strict mode returns exit 0 only when every record is `pass`, or when every
  non-pass record is an explicitly allowed `accepted_unverified` gap and all
  deterministic checks for available repos pass. It returns nonzero for any
  unclassified, failed, or unaccepted-unverified record. AI credential failures
  are non-blocking only when AI was selected and all deterministic checks pass.
- Each repo record has this frozen shape: `{label, root, language, status,
failures[], unverified{reason}, config{source,source_config_sha256,
candidate_sha256,target_head,version,update_exit,second_update_exit,
second_update_changed}, analyze{exit,
schema_version,verdict,dimension_keys}, check{exit,verdict}, formats{
json,text,markdown,sarif,scorecard}, determinism{json_byte_identical},
ai{requested,exit}}`; absent values are explicit `null` or empty arrays.
- Every successful `analyze --json` exits 0 and validates
  `archfit.architecture-state.v1`, all nine dimensions, coverage sums, and typed
  metric arrays. Every `check` exit matches its decision: healthy 0,
  needs-attention 2, blocked 1, and no-valid-report error 3.
- The four mandatory representatives emit JSON, text, Markdown, SARIF, and
  scorecard with normalized decision, dimension statuses/counts, comparison
  state, coverage facts, and the complete canonical finding-ID/status/rule-ID
  sequence in parity. Text and Markdown may put actionable findings first, but
  must append every finding ID in canonical order; abbreviated output without
  that appendix fails parity. Human layout may differ.
- Repeat JSON for all four mandatory representatives is byte-identical with no
  field exclusions: wall-clock/run-local values are not in the primary contract.
  Config update is also byte-idempotent. The canonical finding sequence is
  identical across all five formats, including accepted and waived statuses.
- Missing optional tools or unavailable evidence remain explicit coverage gaps;
  they do not crash, disappear, or become healthy. Required-tool failures retain
  their configured hard behavior. Strict mode gates deterministic config/state/
  format behavior; selected AI summaries remain advisory. Credential/provider
  failures are recorded separately, while an AI-path schema/projection defect is
  still a product defect.
- A corpus result can trigger an Archfit fix only with a minimal reproduction and
  regression test. Target-project architecture findings are recorded but do not
  cause product changes unless the deterministic contract is wrong. Structural
  config synchronization is a separate reviewed candidate from the v1 schema
  migration; it must not obscure migration-only idempotence or alter unrelated
  authored settings.

Impact commands:

- Before editing any production symbol selected from a corpus failure, run
  `gitnexus_impact(target="<exact-symbol>", file_path="<exact-file>", direction="upstream", depth=4, include_tests=true, repo="archfit")`; stop and report HIGH/CRITICAL risk before editing.
- For config migration defects, re-run impact on the exact updater/load symbols
  in `internal/initcfg` or `internal/config` before editing.
- For language defects, re-run impact on the exact Go, TypeScript, Python, or
  Rust extractor symbol before editing.
- `gitnexus_detect_changes(scope="compare", base_ref="main", repo="archfit")`.
- `gitnexus_detect_changes(scope="all", repo="archfit")`.

Verification commands:

The harness exposes `--strict` and returns zero only when every selected repo
passes the deterministic contract, except explicitly owner-accepted
`accepted_unverified` gaps in the full corpus. It returns nonzero for every
failed or unaccepted-unverified deterministic record. The nested per-repo record
shape above is stable and is the contract asserted below.

```bash
# Run this delivery block with Bash, not POSIX sh.
set -euo pipefail
make build
.bin/archfit doctor
python3 -m unittest discover -s scripts/eval -p '*corpus_sweep_test.py'

python3 scripts/eval/corpus_sweep.py \
  --repos spotinfo,storybook,ccgram,herdr \
  --ai-repos '' --migration-only \
  --repeat-repos spotinfo,storybook,ccgram,herdr \
  --format-repos spotinfo,storybook,ccgram,herdr \
  --strict --max-workers 1 \
  --output-dir /tmp/archfit-corpus-eval-fast \
  --summary-file /tmp/archfit-corpus-results-fast.json
jq -e 'length == 4 and all(.[]; .status == "pass" and
  .config.update_exit == 0 and .config.version == 2 and
  .config.second_update_exit == 0 and .config.second_update_changed == false and .analyze.exit == 0 and
  .analyze.schema_version == "archfit.architecture-state.v1")' \
  /tmp/archfit-corpus-results-fast.json

python3 scripts/eval/corpus_sweep.py \
  --repos spotinfo,pumba,omni/scheduled-tasks,prometheus,ccgram,prefect,storybook,yazi,herdr,ruff,tokio \
  --ai-repos spotinfo,ccgram,herdr,storybook --migration-only \
  --repeat-repos spotinfo,storybook,ccgram,herdr \
  --format-repos spotinfo,storybook,ccgram,herdr \
  --strict --max-workers 4 \
  --output-dir /tmp/archfit-corpus-eval \
  --summary-file /tmp/archfit-corpus-results.json

jq -e 'length == 11 and all(.[];
  (.status == "pass" and .config.update_exit == 0 and .config.version == 2 and
   .config.second_update_exit == 0 and .config.second_update_changed == false and .analyze.exit == 0 and
   .analyze.schema_version == "archfit.architecture-state.v1" and
   (.analyze.dimension_keys | length) == 9 and
   (.check.exit == 0 or .check.exit == 1 or .check.exit == 2)) or
  (.status == "accepted_unverified" and
   (.unverified.reason | type) == "string"))' \
  /tmp/archfit-corpus-results.json

# Produce migration-only candidates after the final full sweep. AI overlays are
# never eligible for delivery, and delivery receipts use a separate tree.
python3 scripts/eval/corpus_sweep.py \
  --repos spotinfo,pumba,ccgram \
  --ai-repos '' --migration-only \
  --repeat-repos spotinfo,pumba,ccgram \
  --strict --max-workers 1 \
  --output-dir /tmp/archfit-corpus-delivery-candidates \
  --summary-file /tmp/archfit-corpus-delivery-candidates.json
jq -e 'length == 3 and all(.[]; .status == "pass" and
  .ai.requested == false and .config.version == 2 and
  .config.second_update_exit == 0 and .config.second_update_changed == false)' \
  /tmp/archfit-corpus-delivery-candidates.json

# Final Archfit validation must finish before any target config is touched.
make fmt
make lint
go vet ./...
make test
make build
make archfit
git diff --check
node .gitnexus/run.cjs clean
node .gitnexus/run.cjs analyze

# Deliver only during an owner-confirmed exclusive delivery window. No one may
# edit the three target worktrees until this command exits. The atomic lock makes
# concurrent delivery attempts fail closed; the owner window protects the
# check-to-write interval from editors that do not participate in this script.
# Abort before staging if any target worktree, HEAD, or source-config hash changed.
# Stage and validate all three, then replace all three actual configs. Restore
# every original on failure without overwriting a concurrent change.
delivery_receipts=$(mktemp -d "${TMPDIR:-/tmp}/archfit-corpus-delivery-receipts.XXXXXX")
# Receipts are unique per run; the lock name is global so concurrent deliveries
# contend on one atomic resource.
delivery_lock="${TMPDIR:-/tmp}/archfit-corpus-delivery.lock"
lock_owned=false
# Install ownership-aware cleanup before acquiring the lock or running any
# fail-closed preflight. A failed mkdir must not remove another process's lock.
cleanup_lock() {
  rc=$?
  if test "$lock_owned" = true; then
    if ! rmdir "$delivery_lock"; then
      printf 'delivery lock cleanup failed: %s\n' "$delivery_lock" >&2
      rc=1
    fi
  fi
  exit "$rc"
}
trap cleanup_lock EXIT
if mkdir "$delivery_lock"; then
  lock_owned=true
else
  printf 'delivery lock is already held: %s\n' "$delivery_lock" >&2
  exit 1
fi
repos=(spotinfo pumba ccgram)
for repo in "${repos[@]}"; do
  target="$HOME/workspace/$repo"
  candidate="/tmp/archfit-corpus-delivery-candidates/$repo/archfit.yaml"
  test "$(git -C "$target" rev-parse --is-inside-work-tree)" = true
  test -z "$(git -C "$target" status --porcelain --untracked-files=all)"
  test -f "$target/.archfit.yaml" -a -f "$candidate"
  test "$(git -C "$target" rev-parse HEAD)" = "$(jq -er --arg r "$repo" '.[] | select(.label == $r) | .config.target_head' /tmp/archfit-corpus-delivery-candidates.json)"
  test "$(shasum -a 256 "$target/.archfit.yaml" | awk '{print $1}')" = "$(jq -er --arg r "$repo" '.[] | select(.label == $r) | .config.source_config_sha256' /tmp/archfit-corpus-delivery-candidates.json)"
  test "$(shasum -a 256 "$candidate" | awk '{print $1}')" = "$(jq -er --arg r "$repo" '.[] | select(.label == $r) | .config.candidate_sha256' /tmp/archfit-corpus-delivery-candidates.json)"
done
restore_configs() {
  rc=$?
  set +e
  rollback_failed=0
  if test "$rc" -ne 0; then
    for repo in "${repos[@]}"; do
      target="$HOME/workspace/$repo"
      next="$target/.archfit.yaml.archfit-next"
      original="$delivery_receipts/$repo.original.archfit.yaml"
      candidate_receipt="$delivery_receipts/$repo.candidate.archfit.yaml"
      if test -e "$next"; then
        if next_hash=$(shasum -a 256 "$next" | awk '{print $1}') &&
           expected_hash=$(shasum -a 256 "$candidate_receipt" | awk '{print $1}'); then
          if test "$next_hash" = "$expected_hash"; then
            rm -f "$next" || { printf 'rollback failed removing %s\n' "$next" >&2; rollback_failed=1; }
          else
            printf 'rollback conflict: staged file changed: %s\n' "$next" >&2
            rollback_failed=1
          fi
        else
          printf 'rollback failed hashing staged file: %s\n' "$next" >&2
          rollback_failed=1
        fi
      fi
      if test ! -f "$original"; then
        printf 'rollback missing original: %s\n' "$original" >&2
        rollback_failed=1
        continue
      fi
      if current_hash=$(shasum -a 256 "$target/.archfit.yaml" | awk '{print $1}') &&
         original_hash=$(shasum -a 256 "$original" | awk '{print $1}') &&
         candidate_hash=$(shasum -a 256 "$candidate_receipt" | awk '{print $1}') ; then
        if test "$current_hash" = "$original_hash" || test "$current_hash" = "$candidate_hash"; then
          cp "$original" "$target/.archfit.yaml" || { printf 'rollback failed restoring %s\n' "$target/.archfit.yaml" >&2; rollback_failed=1; }
        else
          printf 'rollback conflict: %s changed after delivery started\n' "$target" >&2
          rollback_failed=1
        fi
      else
        printf 'rollback failed hashing live or receipt config: %s\n' "$target" >&2
        rollback_failed=1
      fi
    done
  fi
  if test "$lock_owned" = true; then
    if rmdir "$delivery_lock"; then
      lock_owned=false
    else
      printf 'rollback failed removing delivery lock: %s\n' "$delivery_lock" >&2
      rollback_failed=1
    fi
  fi
  if test "$rollback_failed" -ne 0; then rc=1; fi
  exit "$rc"
}
trap restore_configs EXIT
for repo in "${repos[@]}"; do
  target="$HOME/workspace/$repo"
  candidate="/tmp/archfit-corpus-delivery-candidates/$repo/archfit.yaml"
  cp "$target/.archfit.yaml" "$delivery_receipts/$repo.original.archfit.yaml"
  cp "$candidate" "$delivery_receipts/$repo.candidate.archfit.yaml"
  cp "$candidate" "$target/.archfit.yaml.archfit-next"
done
# Recheck the swept revision, clean-worktree precondition, candidate identity,
# and live source config immediately before any target file is replaced.
for repo in "${repos[@]}"; do
  target="$HOME/workspace/$repo"
  candidate="/tmp/archfit-corpus-delivery-candidates/$repo/archfit.yaml"
  test "$(git -C "$target" rev-parse HEAD)" = "$(jq -er --arg r "$repo" '.[] | select(.label == $r) | .config.target_head' /tmp/archfit-corpus-delivery-candidates.json)"
  test "$(shasum -a 256 "$target/.archfit.yaml" | awk '{print $1}')" = "$(jq -er --arg r "$repo" '.[] | select(.label == $r) | .config.source_config_sha256' /tmp/archfit-corpus-delivery-candidates.json)"
  test "$(shasum -a 256 "$candidate" | awk '{print $1}')" = "$(jq -er --arg r "$repo" '.[] | select(.label == $r) | .config.candidate_sha256' /tmp/archfit-corpus-delivery-candidates.json)"
  test "$(git -C "$target" status --porcelain --untracked-files=all)" = "?? .archfit.yaml.archfit-next"
done
for repo in "${repos[@]}"; do
  target="$HOME/workspace/$repo"
  .bin/archfit config update --migration-only --json --config "$target/.archfit.yaml.archfit-next" --root "$target" > "$delivery_receipts/$repo.staged-update.json"
  .bin/archfit analyze --json --config "$target/.archfit.yaml.archfit-next" --root "$target" > "$delivery_receipts/$repo.staged-analyze.json"
done
# Recheck live state, candidate identity, and the staged file immediately before
# each atomic replacement. The owner-exclusive window makes this check-to-write
# interval safe; any concurrent edit aborts and triggers guarded rollback.
for repo in "${repos[@]}"; do
  target="$HOME/workspace/$repo"
  candidate="/tmp/archfit-corpus-delivery-candidates/$repo/archfit.yaml"
  expected_candidate=$(jq -er --arg r "$repo" '.[] | select(.label == $r) | .config.candidate_sha256' /tmp/archfit-corpus-delivery-candidates.json)
  test "$(git -C "$target" rev-parse HEAD)" = "$(jq -er --arg r "$repo" '.[] | select(.label == $r) | .config.target_head' /tmp/archfit-corpus-delivery-candidates.json)"
  test "$(shasum -a 256 "$target/.archfit.yaml" | awk '{print $1}')" = "$(jq -er --arg r "$repo" '.[] | select(.label == $r) | .config.source_config_sha256' /tmp/archfit-corpus-delivery-candidates.json)"
  test "$(shasum -a 256 "$candidate" | awk '{print $1}')" = "$expected_candidate"
  test "$(git -C "$target" status --porcelain --untracked-files=all)" = "?? .archfit.yaml.archfit-next"
  test "$(shasum -a 256 "$target/.archfit.yaml.archfit-next" | awk '{print $1}')" = "$expected_candidate"
  mv "$target/.archfit.yaml.archfit-next" "$target/.archfit.yaml"
done
for repo in "${repos[@]}"; do
  target="$HOME/workspace/$repo"
  .bin/archfit config update --migration-only --json --config "$target/.archfit.yaml" --root "$target" > "$delivery_receipts/$repo.final-update.json"
  .bin/archfit analyze --json --config "$target/.archfit.yaml" --root "$target" > "$delivery_receipts/$repo.final-analyze.json"
  set +e
  .bin/archfit check --json --config "$target/.archfit.yaml" --root "$target" > "$delivery_receipts/$repo.final-check.json"
  owner_check_rc=$?
  set -e
  test "$owner_check_rc" -eq 0 || test "$owner_check_rc" -eq 1 || test "$owner_check_rc" -eq 2
  git -C "$target" diff --check -- .archfit.yaml
  git -C "$target" diff -- .archfit.yaml > "$delivery_receipts/$repo.target-config.diff"
done
# Keep original configs in the receipt tree for manual recovery. Disable
# rollback only after every post-delivery validation has passed.
if ! rmdir "$delivery_lock"; then
  printf 'delivery completed but lock cleanup failed: %s\n' "$delivery_lock" >&2
  exit 1
fi
lock_owned=false
trap - EXIT
```

Manual checks:

- Inspect every migrated config diff. Preserve project-authored modules, paths,
  rules, owners, roles, deploy units, volatility, labels, baselines, comments,
  and unrelated formatting. Reject any silent policy weakening or invented fact.
- Inspect all four language representatives, every nonzero exit, stderr tail,
  partial/unmeasured reason, and normalized format-parity result.
- Classify every anomaly as product defect, config migration defect, expected
  target architecture finding, optional/required tool gap, environment failure,
  or docs/UX defect. Do not leave an `unknown` classification at final GO.
- Re-run an affected repo immediately after each fix, then rerun the fast matrix;
  rerun the full corpus after the last fix.
- Obtain an owner-confirmed exclusive delivery window and verify the atomic
  delivery lock is absent before applying any target config. Keep the window
  through replacement, post-delivery validation, or guarded rollback.
- For each owner-controlled dogfood repo, require a clean worktree, apply the
  exact validated candidate to `.archfit.yaml`, rerun
  `config update --migration-only --json`,
  `check`, and `analyze` with the branch binary, and retain the diff and command
  receipt. Stop for owner action on a dirty worktree. Do not edit or commit an
  external corpus repo.
- Run the scoped architecture re-review only after corpus acceptance, final
  repository validation, and successful controlled config delivery. After GO,
  regenerate the v2 baseline and run final baseline-read/compare validation.

- [ ] Slice 6A: update and test the corpus harness for v1 state, strict exits,
      format parity, config migration artifacts, and idempotence; commit.
- [ ] Run the mandatory four-language fast matrix and classify every anomaly.
- [ ] Fix each confirmed Archfit/config-migration defect with impact evidence, a
      minimal regression test, and one focused commit; rerun the affected repo
      and fast matrix after each fix.
- [ ] Run the full corpus and inspect every config/output/coverage result.
- [ ] Update corpus/evaluation docs from observed v1 behavior; commit.
- [ ] Rerun the full corpus, full repository validation, and GitNexus change
      detection after the last fix or documentation change.
- [ ] Obtain an owner-confirmed exclusive delivery window; acquire the atomic
      lock; apply and validate the exact migration-only `.archfit.yaml` in every
      owner-controlled dogfood repo as the final separate cross-repository
      delivery; retain diffs and receipts and stop rather than overwrite a dirty
      worktree or concurrent owner edit.
- [ ] Run the final scoped architecture re-review; require GO.
- [ ] After and only after GO, regenerate the v2 baseline, validate baseline
      read/compare and byte identity, then commit and push the final handoff.

## Acceptance criteria

- The command headline and every supported format use structured architecture
  state, not a repository-level score.
- Nine dimensions are present and independently marked measured, partial, or
  unmeasured with confidence, coverage, findings, unknowns, and deltas.
- `HEALTHY`, `NEEDS_ATTENTION`, and `BLOCKED` are deterministic and explainable.
- Hard gates alone determine blocking/exit behavior; diagnostics never become
  blockers through scalar aggregation.
- Coupling score remains available only per seam/per edge for explanation and
  prioritization. It is not averaged into architecture health.
- Same-owner modular-monolith seams, composition roots, and adapters are handled
  by explicit role/raw-context policy, not fabricated labels or deployment data.
- Config schema v2 is explicit: init emits v2; `config update
--migration-only --json/--apply` previews/applies v1-to-v2 without losing
  authored data; analysis/check reject unmigrated v1; the second migration
  update exits 0 and is byte-idempotent. Ordinary config update remains
  structural synchronization.
- Config changes make comparisons non-comparable when required and expose the
  reason.
- Labels are optional, valid, evidence-hashed, provenance-bearing, and never used
  for score manipulation.
- JSON, text/console, Markdown, SARIF, and scorecard outputs agree on decision,
  dimension status, counts, coverage semantics, and the complete canonical
  finding-ID/status/rule-ID sequence. Text and Markdown may summarize actionable
  findings only in the headline area, but must include a complete ID appendix.
- Baseline storage is versioned, idempotent, and stores dimensions/seams/gates
  rather than a decisive scalar.
- All existing behavior contracts and exit codes pass their compatibility tests.
- The branch binary passes the mandatory Go, TypeScript/JavaScript, Python, and
  Rust matrix and the full listed corpus with no unexplained config migration,
  schema, crash, exit, format-parity, or determinism failure.
- Every corpus config is migrated to `version: 2` on a temp copy by the branch
  binary, loads successfully, and is byte-idempotent on a second update. Every owner-controlled
  dogfood repo's actual `.archfit.yaml` matches its validated candidate and
  passes the branch binary; external corpus configs remain read-only.
- Every corpus anomaly is classified. Confirmed Archfit defects have regression
  tests; target-project findings and optional-tool gaps remain truthful evidence.
- All architecture gates, tests, lint, vet, build, and CI pass.
- Architecture docs and `.archfit.yaml` describe implemented source paths and
  maintenance rules.
- A final architecture review returns GO with residual risks explicitly listed.

## Safety notes

This plan changes the central assessment/report contract. GitNexus reports
critical blast radius for `Synthesize` and `ProjectReport`; do not batch unrelated
cleanup into those commits. Run upstream impact analysis before every production
symbol edit and stop on HIGH/CRITICAL findings until the affected callers and
flows are understood.

No baseline, label, or output fixture may be regenerated merely to make checks
pass. Preserve old artifacts before migration. Do not use `--no-verify`, force
push, blanket waivers, lowered volatility, fabricated deploy units, or invented
labels.

The old scalar remains only in the clearly versioned `legacy-json` compatibility
format for exactly one release. It must never drive the new verdict, exit code,
baseline acceptance, or hard-gate evaluation. Remove that compatibility format
at the end of that release; do not extend the window based on executor judgment.

Rollback is task-commit based. Tasks 1-3 can be reverted without changing the
CLI default if the shadow path is retained. Task 4 is a user-visible schema
migration and must be rolled back as a complete output contract. Task 5 can be
reverted before corpus acceptance without touching target repos. Task 6 harness
and defect fixes use focused commits; the final baseline write is reversible
through git, but never overwrite the previous baseline without preserving it.
Config deliveries for owner-controlled dogfood repos are separate changes in
those repositories and must be reviewed, committed, or reverted there, never
through this repository.

The executor must stop and ask the owner when product semantics are ambiguous,
when deterministic evidence is unavailable, or when a proposed fix improves a
score without improving the architecture. This plan is approved for execution
only after the owner reviews the fusion findings and accepts the contract/version
and exit-code decisions.

## Re-review and handoff

After Task 6 corpus acceptance, final repository validation, and successful
controlled config delivery, run a fresh scoped architecture review against:

- `docs/design/20260823-archfit-capability-map.md`;
- `docs/design/architecture-baseline.md`;
- this plan’s nine-dimensional contract;
- the final `.archfit.yaml` and `.archfit-labels.yaml`;
- `/tmp/archfit-corpus-results.json` plus the four representative format and
  deterministic-repeat artifacts;
- the config migration diff/receipt for every owner-controlled dogfood repo.

The review must independently verify DDD ownership, modularity, Balanced
Coupling, complexity, testability, change locality, configuration honesty,
coverage/confidence handling, all output formats, erosion controls,
cross-language compatibility, and config migration honesty. It must return GO.
Only then may the executor perform the final baseline write as a post-review
handoff; a baseline is never a prerequisite for this review.

After approval, execute with:

```sh
ralphex docs/plans/architecture-state-reporting.md
```
