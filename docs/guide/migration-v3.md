# Migrate to v3.0.0

v3.0.0 is a breaking release. This page lists what you must do, in order.
The [release notes](release-notes.md) list every change.

## Who must do what

| You use                                 | You must                                                      |
| --------------------------------------- | ------------------------------------------------------------- |
| `go install` or import archfit packages | Use the `/v3` module path (step 1)                            |
| A stored `.archfit-baseline.json`       | Re-anchor it (step 3)                                         |
| JSON, SARIF or agent output in a tool   | Read the new and removed keys (step 5, step 6)                |
| `public:` in `.archfit.yaml`            | Check that each entry names a real published surface (step 2) |
| Metric ratchets (`metrics.<name>.gate`) | Expect a `metric/<name>` finding, and exit 2 on a stale baseline (step 4) |
| The archfit GitHub App or Action        | Use versions that know the v3 wire (step 7)                   |

## 1. Install with the new module path

The Go module path is `github.com/alexei-led/archfit/v3`.

```sh
go install github.com/alexei-led/archfit/v3/cmd/archfit@v3.0.0
```

Release binaries, the Homebrew tap and the container image
`ghcr.io/alexei-led/archfit:v3.0.0` do not change their names.
The `internal/` packages are not a public API. If you import
`github.com/alexei-led/archfit/...`, change the import to `.../v3/...`.

## 2. Review `.archfit.yaml`

The config schema is still version 2. No key is removed. Two meanings change.

- A `public:` target is the integration contract (`bc_score.v7`). A call to
  it counts as contract coupling and does not raise the score. Keep an entry
  only for a surface you publish.
- `archfit config init` and `config update` write no `public:` for Go or
  TypeScript modules. An old config that has whole-module `public:` globs still
  loads, but it now hides coupling inside that module. Narrow the globs.

Other changes:

- The rule ID prefix `metric/` is reserved. A rule with that ID is a config
  error.
- A waiver on `metric/<name>` takes no `from` or `to`.
- Declare `volatility:` or `subdomain:` on each module. A module with none is
  "unrated". Coupling stays `partial` and `check` exits 2 until you declare it.
- The distance token `cross_module_same_owner` is now `cross_module`. Owner and
  deploy unit no longer change severity.
- `role:` changes meaning. A cohesive role (`composition_root`, `generated`,
  `test`) no longer lowers distance or severity. It only stops a non-intrusive
  edge from that module from qualifying a seam. If you used `role:` to quiet
  advisories, they can come back.

## 3. Re-anchor the baseline

The engine rejects `archfit.baseline.v2`. Your baseline is also on an older
score and measurement profile, so it does not compare with a v3 run.

```sh
archfit baseline --reanchor --config .archfit.yaml
```

`--reanchor` carries the accepted debt to the new epoch. It accepts no new
finding. It keeps a finding only if the stored file has the same finding ID.
It keeps a qualifying seam only if the stored file has it too.
The new file is written next to the config, as a full capture is.

The command prints:

| Line                               | Meaning                                                             |
| ---------------------------------- | ------------------------------------------------------------------- |
| `re-anchor: kept N …, dropped M …` | Totals                                                              |
| `drift: …`                         | Why the stored reference did not compare                            |
| `dropped: <rule> <id>`             | Stored debt that the new run no longer reports                      |
| `not accepted: <rule> <id> …`      | A current finding that the stored file did not accept. It stays new |
| `seam no longer qualifies: <id>`   | A stored seam that stopped qualifying                               |
| `seam qualifies only now (stays new): <id>` | A new seam. It stays new |
| `metric worsened: <name> …`        | The new file records the worse value                                |

Many `dropped` lines are normal. Under `bc_score.v7` the engine flags fewer
edges. On the archfit repository, the same tree went from 26 to 13 critical
seams, and on Prometheus from 51 to 3. The seams that qualify for the gate can
rise: archfit went from 0 to 5.

A baseline from v2.5.0 holds one edge for each Balanced Coupling advisory group.
`--reanchor` keeps only stored IDs, so the other edges of such a group show as
`not accepted`. This is expected. Review them, then run a full
`archfit baseline` if you accept the group.
Decide each `not accepted` line: fix it, or accept it with a full
`archfit baseline`. Use `--from <path>` to read the stored file from another
place. `--from` without `--reanchor` is an error. So is `--reanchor` with
`--no-advisories`. Without a stored file the command exits 3.

Then run `archfit check`. `gate_reference.status` must be `comparable`.

## 4. Metric ratchets are findings

A worse metric against the baseline is now one finding, `metric/<name>`. It
shows the value before and after, the threshold, and a repair task. It counts
in the exit code, SARIF, text and Markdown output. `gate: warn` gives an
advisory.

A ratchet decides only against a comparable reference. Against a reference
that does not compare, the run lists one unevaluated entry, `metric_ratchets`.
Then `hard_gates` is `unmeasured` and the exit code is 2, not 1.
With no baseline file there is no ratchet.

In `--format agent`, an unmeasured ratchet gives `next_action: ask_owner`.
`archfit hook claude` shows it as a `systemMessage` and does not block.

Removed: the `METRIC RATCHET` section of text and Markdown output, and the
`worsened_metrics` field of `archfit.agent-result.v1`.

## 5. Report (JSON state) changes

The state schema ID is still `archfit.architecture-state.v1`.

| Change   | Key                                                                                                                        |
| -------- | -------------------------------------------------------------------------------------------------------------------------- |
| New      | `comparison.classification_hash`                                                                                           |
| New      | `comparison.drift[]` and `gate_reference.drift[]` (input classes that broke comparison)                                    |
| New      | `gate_reference.baseline_present`                                                                                          |
| New | `findings[].origin` with `--base`: `introduced`, `pre_existing`, `unknown`. `agent_tasks[].origin` copies it | 
| New      | `comparison.introduced_finding_ids`, `comparison.resolved_finding_ids` (present only with `--base`)                        |
| New      | Finding `rule_id` of the form `metric/<name>`; unevaluated entry `metric_ratchets`                                         |
| New      | Metrics `library_edges`, `unmapped_first_party_edges`, `qualifying_edges`                                                  |
| Renamed  | `comparison.task_origin_status` → `comparison.origin_status`                                                               |
| Renamed  | `comparison.task_origin_reasons` → `comparison.origin_reasons`                                                             |
| Replaced | Metric `external_edges` → `library_edges` + `unmapped_first_party_edges` (their sum is the old value)                      |
| Replaced | Metric `critical_high_distance_edges` → `qualifying_edges`                                                                 |
| Changed | `findings[].matched_by.cheapest_move` → `matched_by.hypothesis`; `distance_basis` `code_structure` → `module_boundary` |
| Removed  | `dimensions.<name>.delta.new_findings` and `resolved_findings`                                                             |
| Removed  | `worsened_metrics` in `archfit.agent-result.v1`                                                                            |
| Changed | `seams[].distance` can be `cross_module`; `seams[].raw_distance.basis` is `<boundary>@<container>`; `seams[].hypothesis` uses the new vocabulary |

Other outputs also change. In Markdown and in `config compare --json`, the
`distance_compression` fields `code_structure_*` are now `containment_*`, and
`strength_inferred_edges` is removed from the connascence report.

`config_hash` stays as the identity of the file. The engine does not compare it
any more. `measurement.source_ref` is always `worktree`.
Comparison uses `classification_hash`, `model_hash`, `labels_hash`,
`rubric_version` and the measurement profile `archfit.measurement.v2`.
A comment, waiver, rule, gate, `layers`, `min_severity`, `depends_on`,
`visible_to` or `reviewed_at` edit does not break comparison. An edit to a
rule's `patterns:` still does, because the pattern pass is a measurement input.

The `required` coupling fact `coupling volatility` is new. See step 2.

## 6. SARIF changes

- Each result has `partialFingerprints.primaryLocationLineHash` (the finding
  ID).
- With a loaded baseline file, each result has `baselineState`: `unchanged`,
  `absent` or `new`. A baselined or waived result has an `external`,
  `accepted` suppression.
- With `--base`, each result has `properties.origin`.

## 7. App and Action

The archfit GitHub App and Action must read the keys in step 5 and know
`baseline --reanchor`. Use their v3-ready versions with engine v3.0.0.
The engine does not run the target repository's tests, and the wire contract
is the state document, not the CLI text.

## What the score change means

`bc_score.v7` keeps the formula `balance = max(|S − D|, 10 − V) + 1`. The inputs
change: any module boundary is D=9, interface calls are contract coupling, and
functional and symmetric edges use the worse volatility of both modules.
A seam qualifies for the distributed-monolith gate in fewer, clearer cases.
The self-config stays at `mode: warn`. Run a trial before you set `mode: fail`.
See [bc-measurement-v7](../design/bc-measurement-v7.md).

## Other behavior changes

- `archfit hook git` judges the staged content, not the files on disk.
- `archfit baseline` accepts every edge of a Balanced Coupling advisory group.
  A baseline from an older engine kept only one edge per group.
- A rule `rationale`, `docs` or `alternatives` is one line everywhere.
