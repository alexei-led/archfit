# Results and JSON contracts

This page explains the output of `archfit check` and `archfit can` with a version 3 policy: the text record, the statuses, the exit codes, JSON, and SARIF. A version 2 policy (Archfit 3) has other outputs: see [Version 2 (Archfit 3)](#version-2-archfit-3).

## The record

`check` prints one record per error, in the same order everywhere: where, which rule, what; then why, instead, balance, next. The status line comes last, as in a linter.

```text
path:line: rule-id: what is wrong
  why: the reason the rule exists
  instead: what the owner says to do          (when the rule has it)
  balance: 9/10 (min 7)                       (when the rule is about coupling)
  next: one command, or one short instruction
archfit: status, count
```

A pass is one line:

```text
$ archfit check
archfit: pass
```

A dependency only an owner can allow:

```text
$ archfit check
pkg/tc/netem.go:12: graph: pkg_tc -> pkg_util is not an allowed dependency
  why: a new dependency between modules needs an owner
  balance: 9/10 (min 7)
  next: archfit request -m "<why you need it>"     (do not copy the code)
archfit: fail, 1 error
```

No verdict:

```text
$ archfit check
.archfit.yaml: policy: differs from origin/main
  why: only owners change the policy
  next: git checkout origin/main -- .archfit.yaml
archfit: unknown, no verdict
```

- `next` is `fix the code`, a command to run, `wait for an owner: #n`, or a short instruction such as `git fetch --unshallow`. A note in parentheses can follow it.
- An error with no file, such as an expired exception, names `.archfit.yaml` and no line.
- A reason for `unknown` names its cause instead of a rule: `policy`, `base`, `tool`, or `import`.
- `balance` is shown only for a rule about coupling, as `n/10 (min m)`.
- Dates show the days left, such as `2026-11-19 (9 days)`.
- `(not from this change)` ends the status line when every error comes from an expired exception, not from the change.
- `policy: not protected` follows the status line when no workflow in `.github/workflows/` uses `alexei-led/archfit-action`. The result is then a local guide, not change control.
- `--verbose` adds the policy path and digest, and the engine version, after the status line.

`can` prints one answer and no status line:

```text
$ archfit can pkg/tc pkg/util
pkg_tc -> pkg_util: graph: not an allowed dependency
  balance: 9/10 (min 7)
  next: archfit request -m "<why you need it>"
```

The record goes to stdout. Notes and tool errors go to stderr.

## Statuses

| Status | Meaning | The next step |
| --- | --- | --- |
| `pass` | The change adds nothing a rule forbids. | None. There is no warning on `pass`. |
| `fail` | The change adds an error, or an exception has expired. | Fix the code, or ask, as the error says. |
| `waiting` | The errors are covered by an open request. Set by the App on a pull request only; the CLI never prints it. | Wait for the owner. |
| `unknown` | No verdict. | Restore what the record names, then run `check` again. |

The status is `unknown` when:

- the local `.archfit.yaml` differs from the policy of the target branch, or is not valid;
- there is no target branch, no merge base, or the base cannot be read;
- a tool the check needs is missing;
- a first-party import in a changed file cannot be resolved, or the change adds an import that cannot be resolved.

The order: a comparison that cannot be made at all (the policy differs or is not valid, there is no merge base, or the base cannot be read) is `unknown` with only its reasons. Otherwise an added error or an expired exception is `fail`, and the other reasons for `unknown` are still listed. Otherwise any reason for `unknown` gives `unknown`. Otherwise the status is `pass`.

The same tree, policy, engine version, and date give the same output.

## Exit codes

| Code | `check` | `can` |
| --- | --- | --- |
| `0` | `pass` | yes |
| `1` | `fail` (and `waiting`) | no |
| `2` | `unknown`, wrong usage, or a tool error | `unknown`, wrong usage, or a tool error |

`unknown` is not a pass: in CI, fail the job on every code except `0`. See [commands](commands.md#exit-codes) for the other commands.

## JSON

`--format json` writes one JSON document, `archfit.result.v1`, described by [`archfit.result.schema.json`](../archfit.result.schema.json). It holds the same facts as the record, in the same order. An agent needs no JSON to act: the format is for scripts and for the upload to the App.

| Field | Meaning |
| --- | --- |
| `schema` | `archfit.result.v1`. |
| `status` | `pass`, `fail`, `waiting`, or `unknown`. |
| `summary` | The status line of the record. |
| `errors[]` | The errors in record order. |
| `unknown[]` | The reasons for `unknown`: `path`, `line`, `cause` (`policy`, `base`, `tool`, `import`), `what`, `why`, `next`. |
| `repository` | The state of the repository, on a push only. See below. |
| `policy` | `path`, `digest`, `protected`, `differs` (the local file differs from the target's), and `target` (the branch it was compared with). |
| `base`, `head` | The commits compared. |
| `date` | The date the exceptions were judged on, `YYYY-MM-DD`. |
| `engine` | The archfit version. |

Each error has:

| Field | Meaning |
| --- | --- |
| `path`, `line` | Where. An expired exception has `.archfit.yaml` and no line. |
| `rule` | The rule id, or `exception` for an expired exception. |
| `what`, `why`, `instead` | As in the record. |
| `balance` | `value` and `min`, for a rule about coupling. |
| `next` | `kind` and `text`, and `note` when there is one. `kind` is `fix`, `command` (only an error of an `ask` rule), `wait`, or `restore`. |
| `from`, `to` | The modules. |
| `key` | The rule, the modules, and the symbol the error is counted by. |
| `from_change` | `false` for an expired exception that another change added. |
| `exception` | For an expired exception: the `rule` it covers and the date it `expires`. |

On a push, the result also has a `repository` object: the whole tree of `HEAD`, judged on its own with the exceptions on `date`. A push is a run where `HEAD` is the tip of the target branch, and where the status is `pass` or `fail`.

| Field | Meaning |
| --- | --- |
| `state` | `clean` (no error outside the exceptions), `drift` (an error that no exception covers), or `expired` (an exception passed its date and the code still breaks its rule). |
| `drift[]` | The errors that no exception covers, in the fields of an error, each with `from_change: false`. |
| `expiring[]` | The exceptions within 14 days of their date that still cover an error: `rule`, `from`, `to`, `expires` (the last day it holds), `why`. |

An expired exception is not in `repository`. `errors[]` lists it, as on every run. The state does not change the status: drift that the change did not add still gives `pass`.

A pull-request run has no `repository`. A push also has none when its status is `unknown`, when a later push moved the tip before the run read it, or when it is a `--policy` preview. A missing `repository` means "no state". It never means `clean`.

`archfit can --format json` writes `archfit.can.v1`, in the same schema file: `from`, `to`, `answer` (`yes`, `no`, `unknown`), `rule`, `what`, `why`, `instead`, `balance`, `next`, `policy`, `head`, `date`, and `engine`.

Both commands keep their JSON document, with the git tree id of the working tree, in `.archfit-cache/last.json` for `archfit request`. A `check --policy` preview keeps nothing.

## SARIF

`--format sarif` writes a SARIF 2.1.0 log for code scanning. Each error is one result with level `error`, the rule id, and the location. Its message holds the lines of the record: what, then why, instead, balance, and next. A reason for `unknown` is a tool notification, not a result, and an `unknown` status is an execution that did not succeed. A pass has no result.

## The report

`archfit report --format json` writes the `archfit.architecture-state.v1` document of [`archfit.state.schema.json`](../archfit.state.schema.json). On a version 3 policy it has a `repository` object: `state` (`clean`, `drift`, or `expired`), `drift` and `expired` (the errors, in the fields of the record), `expiring` (exceptions within 14 days of their date), `not_checked` (the notes), `balances` (each allowed dependency), `unread` (the places the check could not read, which `state` does not cover), and `protected`. A reader that shows `state` shows `unread` beside it. The report decides nothing and exits `0` whatever it shows. Until the Archfit 3 outputs are removed, the document still carries their `verdict` and `decision`: read `repository` instead. See [commands](commands.md#report).

## Version 2 (Archfit 3)

With a version 2 policy, `archfit check`, `analyze`, and the other Archfit 3 commands write the Archfit 3 outputs: `text`, `markdown`, `json` (`archfit.architecture-state.v1`), `sarif`, `scorecard`, and `agent` (`archfit.agent-result.v1`), with the verdicts `healthy`, `needs_attention`, and `blocked`. The full reference is the [Results page of v3.4.0](https://github.com/alexei-led/archfit/blob/v3.4.0/docs/results.md). Convert the repository with `archfit init` (see [migration](migration.md)).

### Baseline comparison

`archfit.architecture-state.v1` compares findings with the stored baseline and with `--base`. See [v3.4.0](https://github.com/alexei-led/archfit/blob/v3.4.0/docs/results.md#baseline-comparison).

### Config comparison

`archfit config compare` writes `archfit.config-compare.v1`. See [v3.4.0](https://github.com/alexei-led/archfit/blob/v3.4.0/docs/results.md#config-comparison).

### JSON contracts

The JSON Schemas of version 2 are [`archfit.state.schema.json`](../archfit.state.schema.json) and [`archfit.agent-result.schema.json`](../archfit.agent-result.schema.json). See [v3.4.0](https://github.com/alexei-led/archfit/blob/v3.4.0/docs/results.md#json-contracts).
