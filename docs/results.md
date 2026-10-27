# Results and JSON contracts

This page explains how to read archfit output and how to consume its JSON contracts. Metric meanings live in [metrics](metrics.md). Coupling terms live in [coupling](coupling.md).

## Choose an output

`analyze` and `check` accept repeatable `--format` flags. The default is `text`.

| Format | Use | Contract |
| --- | --- | --- |
| `text` | Terminal and CI logs | `analyze`: the human report, all blockers and a bounded number of other details. `check`: linter records, see below. |
| `markdown` or `md` | Saved reports | Same report sections in Markdown. |
| `json` | Automation and dashboards | `archfit.architecture-state.v1`. |
| `sarif` | Code scanning tools | SARIF 2.1.0. State metadata is in `run.properties`. |
| `scorecard` | Dimension review | Nine dimensions with status, gate, confidence, coverage, metrics, and unknowns. |
| `agent` | Coding-agent loops | Compact `archfit.agent-result.v1`. |

`--json`, `--markdown`, and `--sarif` are shorthands. Do not combine a shorthand with `--format`. Use repeated `--format` flags to write multiple formats.

Multiple formats concatenate on stdout. Select only JSON when a consumer parses stdout as one JSON document.

`check` text reads like a linter. Each blocker is one record, with its explanation indented under it:

```text
pkg/a/a.go:3: no_b_impl: Remove the forbidden dependency from pkg/a/a.go on pkg/b/impl.
  why: callers must not see storage
  instead: use pkg/b/api
  status: introduced, code_change

FAIL  1 new, 0 unknown, 3 accepted
engine v3.3.0 · policy 0f7b1dd7cce2 · baseline … · tree … · profile …

repository attention (not this change):
  unmeasured_ratchet: reference not comparable (drift: measurement_profile): …
```

The footer word is `PASS`, `FAIL`, or `UNKNOWN`, the same as `change.status` in the agent result. `why` is the rule's rationale, or its constraint when it has none; `instead` lists the alternatives the rule's owner declared. The coupling gate's reasons, which older versions printed on stderr, are repository attention. `check` prints nothing on stderr unless you pass `--progress`. The scorecard, dimensions, and seam ledger are not in `check` text: read them with `analyze`.

`check` sets its exit code from the verdict. `analyze` exits `0` after every successful run. See [commands](commands.md#exit-codes).

## Read the verdict

The architecture-state verdict has three values:

| Verdict | Meaning |
| --- | --- |
| `healthy` | All dimensions are measured, hard gates pass, and no active diagnostic exists. |
| `needs_attention` | No blocker exists, but diagnostics are active or a dimension is partial or unmeasured. |
| `blocked` | At least one active hard-gate finding exists. |

The verdict is not an average score. Missing evidence stays explicit. archfit does not convert unknown values to zero.

Each of the nine dimensions has its own `status`, `confidence`, `gate`, `coverage`, `metrics`, `findings`, and `unknown` fields. Status is `measured`, `partial`, or `unmeasured`. A dimension gate is `pass`, `warn`, `fail`, or `not_applicable`. See [metrics](metrics.md) for the dimensions and their measures.

Findings appear once in the top-level `findings` array. Dimension `findings` arrays refer to those records by ID. A finding's status and severity describe its gate role and urgency. See [rules](rules.md) for rule behavior.

## Baseline comparison

`comparison` reports the separate Git comparison requested with `--base`.
Without `--base`, its status is `not_requested`, even when a stored baseline exists.
Other statuses are `comparable` and `non_comparable`.
Numeric deltas require compatible classification, model, labels, rubric, and measurement-profile fingerprints.
The raw configuration hash identifies file bytes. It is not a comparison fingerprint.

`gate_reference` reports the reference used by hard gates. It can differ from `comparison`. For example, `--base` adds a Git comparison but does not replace the stored gate reference.

A dimension can carry an optional `delta` when its comparison has compatible evidence.
A non-comparable reference carries reasons instead of numeric changes.
Read the status before reading a delta.

### Finding origin is not debt status

With `--base`, `findings[].origin` compares observed findings with the base-tree
run. `status: new` means unaccepted debt. It does not mean the PR added it.
A finding can be both `new` and `pre_existing`. Origin is report-only and never
changes gate acceptance or the exit code.

- `pre_existing`: the finding ID, or every current member ID of a grouped
  advisory, was observed in base. A seam-gate finding instead matches a
  qualifying ordered module pair.
- `introduced`: at least one current member finding was not observed in base,
  and the active analyzer evidence permits origin comparison. This is a finding
  difference, not proof that every dependency or displayed source line was added.
- `unknown`: the evidence cannot establish introduction. Known matches can still
  be `pre_existing`. Unmatched findings have unknown origin and no finding is
  claimed resolved.
  Accepted-baseline metric ratchets always have unknown base-tree origin.

Read `comparison.origin_status` and `origin_reasons` for analyzer pairing.
Numeric `comparison.status` is separate: fingerprint differences can suppress
numeric deltas while origin still compares observed IDs. Neither status proves
that installed dependencies, generated files, or indexer resolution were identical.
A zero tracked Git diff alone does not establish equivalent measurement inputs.

Grouped coupling advisories match **all** in-memory member IDs, not the
first eight exported in `matched_by.group_members`. Their `why` gives a compact full-membership match summary.
`matched_by.origin_basis` adds up to eight unmatched IDs. Adding a member can mark an old group `introduced` without changing its ID.
The displayed path pair belongs to one member. Locations merge all member sites,
including test sites when those are in scope. Neither the representative nor the
first site necessarily identifies the unmatched member. A missing base observation
is not proof that the source dependency was absent.

## Config comparison

`config compare` measures the same source tree with two configs. It uses no accepted baseline and does not write config or baseline files. Both measurements share the current config's cache and labels.

The text report compares changed finding IDs, classified-edge counts, and classification mix. It does not print a repository score. JSON includes each side's scorecard and `score_delta`. A null delta means that at least one side did not measure `coupling_balance`.

Coverage warnings qualify the comparison. A difference does not show that one config is better. See [commands](commands.md) for invocation.

## Read findings, tasks, and seams

The state JSON includes full findings, `agent_tasks`, and the coupling seam ledger. Text and Markdown cap some non-blocking details. Use JSON when a consumer needs the complete lists.

An `agent_task` gives a constrained repair or an owner decision. It includes the finding and rule IDs, goal, constraints, files, and validation commands. Do not infer that an empty task list means the run is healthy. Check `verdict`, `decision`, diagnostics, and unmeasured required rules.

Each seam record describes a cross-module relationship and its policy. The `policy` value distinguishes `violation`, `accepted`, `advisory`, `allowed`, and `observed`. A seam is not itself a finding. Use the associated finding IDs and gate result to identify blockers. See [coupling](coupling.md).

## JSON contracts

The `schema_version` field identifies a wire contract. It does not identify the project release or configuration version.

| Contract | Produced by | Published schema |
| --- | --- | --- |
| `archfit.architecture-state.v1` | `analyze --format json`, `check --format json` | [`archfit.state.schema.json`](../archfit.state.schema.json) |
| `archfit.agent-result.v1` | `analyze --format agent`, `check --format agent` | [`archfit.agent-result.schema.json`](../archfit.agent-result.schema.json) |
| `archfit.config-review.v1` | `config update --json` | Go contract in `internal/initcfg`; no published JSON Schema. |
| `archfit.config-compare.v1` | `config compare --json` | Go contract in `cmd/archfit/config_compare.go`; no published JSON Schema. |

Every state and agent result carries an `identity` block (`archfit.result-identity.v1`): the sha256 of the policy, baseline, and labels files as read from disk, the base commit, the git tree of the measured head (with `+dirty` for uncommitted edits), the engine version, and a hash of the measurement profile. A value the run could not read is omitted, never empty. The text footer, the SARIF run properties, and the hook message repeat it in short form.

The agent result splits into `change` and `repository`. `change.status` is `pass`, `fail`, or `unknown`; `change.blockers` are the in-scope repairs. `unknown` means evidence is missing and never passes. `repository.attention` holds what the change did not cause: `existing_violation`, `unmeasured_ratchet`, `dead_selector`, `blocked_without_finding`. `next_action` follows `change` only, and the hooks stop a change only on `fail`. The process exit code still follows the verdict, so a clean change in a repository with attention exits `2`.

`archfit.schema.json` describes configuration input. It is not a report schema. Configuration files use numeric `version: 2`. See [configuration](configuration.md) for fields and validation.

The coverage sidecar uses numeric schema version `1`. Its shape and source rules belong to the configuration reference because they define input data. The sidecar is separate from the configuration file and report contracts.

The persisted `.archfit-baseline.json` uses the string `schema_version: "archfit.baseline.v3"`.
It stores accepted findings and comparison snapshots. It is separate from state-report JSON and has no published JSON Schema.
Use [Migration](migration.md) to review and re-anchor older baselines. Do not change the version field by hand.

## Consume JSON safely

Use the published schema for state and agent output. The schemas use JSON Schema Draft 2020-12. The state schema is generated from the report types and checked for drift. The agent schema is generated from the agent result types and checked for drift.

Decode the exact `schema_version` that your consumer supports. Reject unknown enum values and unknown contract versions. The output contracts do not promise that a future major contract will keep the same fields.

The published state and agent schemas reject extra object properties. Some properties can be absent when marked optional by the schema. Most state fields are required, including all nine dimensions. Consumers must handle explicit nulls where the schema permits them.

`config update --json` reports a review. An empty `issues` list does not prove that every configured module was checked. Read `unchecked_modules` and `status` as well as `issues`.

`config compare --json` reports two measurements of one source tree. It has `current`, `candidate`, `score_delta`, finding ID groups, coverage details, and warnings. `score_delta` is `null` when either side cannot measure coupling balance. It is a report-only comparison and does not say that one config is better. The text output has no repository score, but JSON keeps the scorecards and `score_delta`.

## Compatibility limits

Treat each named contract as a versioned API. Do not use `schema_version` alone to infer a project release or config version.

The configuration schema helps editors validate config shape. Runtime validation has additional rules, including checks that the schema cannot express. The CLI is the final config validator.

The config review and config comparison documents have version fields, but they do not have published JSON Schema files. Pin consumers to those version fields and test them against sample output from the supported binary.

See [agents](agents.md) for agent-loop behavior and [CI](ci.md) for gate usage.
