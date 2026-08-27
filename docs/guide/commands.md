# Commands

This page is the command and flag reference for `archfit`.

Facts first:

- Bare `archfit` is the same as `archfit analyze`.
- `archfit analyze` is report-only. A successful run exits `0` even when it reports findings.
- `archfit check` is the CI gate. It exits `1` on violations, `2` on warnings, and `3` on usage, config, or runtime errors.
- Every command supports `-h, --help`.
- `archfit` also supports `-v, --version`.

## Breaking changes

The CLI changed in the 2026-07 redesign. Update old scripts before upgrading.

### Command migration

| Old command              | New command       | What changed                                        |
| ------------------------ | ----------------- | --------------------------------------------------- |
| `archfit analyze --gate` | `archfit check`   | Gate mode moved from a flag to a dedicated command. |
| `archfit --gate`         | `archfit check`   | Same behavior change as above.                      |
| `archfit analyze`        | `archfit analyze` | Still the local report command.                     |
| `archfit`                | `archfit analyze` | Bare `archfit` still runs local analysis.           |

### Flags

| Old surface               | New surface                                                 | Status                                                                  |
| ------------------------- | ----------------------------------------------------------- | ----------------------------------------------------------------------- |
| `--gate`                  | `archfit check`                                             | Removed.                                                                |
| `--full`                  | remove it                                                   | Removed. Full scan is always on.                                        |
| `--advisory`              | remove it, or use `--no-advisories` to hide advisories      | Removed and inverted.                                                   |
| `--severity`              | `--min-severity`                                            | Renamed.                                                                |
| `analyze --llm`           | `analyze --ai-summary`                                      | Renamed.                                                                |
| `explain --llm`           | `explain --ai-summary`                                      | Renamed to match `analyze`.                                             |
| `config init --llm`       | `config init --ai-classify`                                 | Renamed.                                                                |
| `config update --llm`     | `config update --ai-classify`                               | Renamed.                                                                |
| `--llm-provider`          | `--ai-provider`                                             | Renamed.                                                                |
| `--llm-model`             | `--ai-model`                                                | Renamed.                                                                |
| `--no-cache`              | `--refresh`                                                 | Renamed. New meaning: bypass cache reads and write fresh results back.  |
| `--no-config`             | initialize config first with `archfit config init --root .` | Removed.                                                                |
| `analyze --require-tools` | prefer `check --require-tools` in CI                        | Workflow moved. `analyze` still parses the flag, but stays report-only. |

## Command chooser

Use this when you know the job, not the command.

| I want to...                                                        | Run this                                                                                                      |
| ------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------- |
| review architecture locally without failing my shell step           | `archfit analyze -c .archfit.yaml`                                                                            |
| fail CI on blocking findings or warnings                            | `archfit check -c .archfit.yaml`                                                                              |
| accept the current findings as the baseline                         | `archfit baseline -c .archfit.yaml`                                                                           |
| understand one finding in detail                                    | `archfit explain <fingerprint-prefix> -c .archfit.yaml`                                                       |
| verify analyzers are installed, or install what archfit can install | `archfit doctor` or `archfit doctor --fix`                                                                    |
| create the first config file for a repo                             | `archfit config init --root .`                                                                                |
| sync an existing config to the current repo structure               | `archfit config update -c .archfit.yaml`                                                                      |
| read the config review from a script or an agent                    | `archfit config update --json -c .archfit.yaml`                                                               |
| see what a candidate config would measure on the same code          | `archfit config compare candidate.archfit.yaml -c .archfit.yaml`                                              |
| draft AI labels or module metadata for review                       | `archfit config enrich <kind>` where `<kind>` is `labels`, `abstained`, `owner`, `volatility`, or `subdomain` |

## Exit codes

`archfit check` is the only command that uses all four exit codes. Its exit code
is the architecture verdict, nothing else.

| Code | Meaning                                                                                                             | Commands that produce it                                                                                                                                                                                      |
| ---- | ------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `0`  | `healthy` — every dimension measured, every hard gate passing, no active diagnostic. For `analyze`, any valid report. | `archfit`, `archfit analyze`, `archfit check`, `archfit baseline`, `archfit explain`, `archfit doctor`, `archfit config init`, `archfit config update`, `archfit config compare`, `archfit config enrich ...` |
| `1`  | `blocked` — an active hard-gate finding, or a required analyzer that did not run under `--require-tools`.            | `archfit check`; `archfit analyze` never exits `1` on a successful run                                                                                                                                        |
| `2`  | `needs_attention` — no blocker, but an active diagnostic or a partial/unmeasured dimension.                          | `archfit check`                                                                                                                                                                                               |
| `3`  | Usage, parse, config, or runtime error. No valid report was produced.                                               | All commands                                                                                                                                                                                                  |

Notes:

- `archfit analyze` always exits `0` after a successful analysis, whatever the verdict.
- `archfit analyze --require-tools` only changes the rendered verdict. It does not change the exit code on success.
- `archfit baseline`, `archfit explain`, `archfit doctor`, and the `config` commands are success-or-error commands: `0` or `3`.
- Exit `0` is reachable when all nine dimensions are measured, hard gates pass,
  and no diagnostic is active. Missing supplied coverage, a non-comparable
  persisted baseline, or incomplete declared operational topology produces exit
  `2`, never a healthy zero. During adoption you may treat `0` and `2` as "not
  blocked" and gate on `1`; require `0` when complete evidence is your CI policy.
- A coupling advisory is a diagnostic, never a blocker: it can reach `2`, never
  `1`. The only coupling gate is `coupling.gate.distributed_monolith`, and it
  blocks only in `mode: fail` against a comparable reference.

## Output formats

These formats apply to `archfit analyze` and `archfit check`.

| Format            | Best for                                            | Notes                                                                                                                                     |
| ----------------- | --------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `text`            | terminal use, local review, quick CI logs           | Default. Headline, nine dimensions, unmeasured facts, seams, actionable findings, comparison.                                              |
| `json`            | automation, bots, custom dashboards, agent loops    | `archfit.architecture-state.v1` at the document root. Use this when a script needs `agent_tasks[]`, findings, dimensions, or the seam ledger. |
| `markdown` / `md` | saved audit reports, PR attachments, docs artifacts | Same facts as `json`, laid out for a human. Good for `archfit-report.md`.                                                                  |
| `sarif`           | GitHub code scanning and other SARIF consumers      | Findings keep their rule IDs and `archfit/v1` fingerprints; the state rides in `run.properties`.                                           |
| `scorecard`       | dimension-by-dimension review                       | The nine-dimension state scorecard: status, gate, confidence, denominator, metrics, and unknowns per dimension. No repository score.       |
| `legacy-json`     | one-release compatibility only                      | The pre-cutover diagnostic envelope, including the retired `score` block. Must be selected explicitly; never affects the verdict or exit code. |

Format rules:

- Default output is `text`.
- Shorthands are `--json`, `--markdown`, and `--sarif`.
- Shorthands are mutually exclusive with each other.
- Shorthands are also mutually exclusive with `--format`.
- `--format` is repeatable.

## `archfit` / `archfit analyze`

Purpose:

- Run the full analysis pipeline locally.
- Print a decision, score, and findings.
- Keep the run report-only.

Use cases:

- local architecture review before a PR;
- generating Markdown, JSON, or SARIF artifacts;
- comparing the current branch against a base ref without failing the shell step;
- getting an AI summary after the deterministic report.

Synopsis:

```sh
archfit [flags]
archfit analyze [flags]
```

Behavior:

- Bare `archfit` is the same as `archfit analyze`.
- This command is report-only.
- On success it exits `0`, even when the rendered verdict says fail or warn.
- Use `archfit check` when you want exit codes to enforce a gate.

Flags:

| Flag              | Type        | Default                           | Effect                                                                                                        | Example                                                    |
| ----------------- | ----------- | --------------------------------- | ------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------- |
| `-c, --config`    | path        | `.archfit.yaml`                   | Config file to load.                                                                                          | `archfit analyze -c ./policy/.archfit.yaml`                |
| `--root`          | path        | directory of `--config`           | Repo root to analyze. Use when the policy file lives outside the checked-out repo.                            | `archfit analyze --root ../repo -c ./policy/.archfit.yaml` |
| `--base`          | git ref     | none                              | Compare the current run to a base ref and report the comparison and its comparability reasons.                                           | `archfit analyze --base origin/main`                       |
| `--ai-summary`    | bool        | `false`                           | Append an off-gate AI narrative review after the deterministic render. Requires `ai:` config.                 | `archfit analyze --ai-summary -c .archfit.yaml`            |
| `--refresh`       | bool        | `false`                           | Re-run extractors, bypass cached reads, and refresh the cache with fresh results.                             | `archfit analyze --refresh -c .archfit.yaml`               |
| `--json`          | bool        | `false`                           | Shorthand for `--format json`.                                                                                | `archfit analyze --json`                                   |
| `--markdown`      | bool        | `false`                           | Shorthand for `--format markdown`.                                                                            | `archfit analyze --markdown > archfit-report.md`           |
| `--sarif`         | bool        | `false`                           | Shorthand for `--format sarif`.                                                                               | `archfit analyze --sarif > archfit.sarif`                  |
| `--format`        | enum list   | `text` when no format flag is set | Output one or more formats: `json`, `text`, `markdown`, `md`, `sarif`, `scorecard`, `legacy-json`. Repeatable.               | `archfit analyze --format text --format json`              |
| `--no-advisories` | bool        | `false`                           | Hide informational Balanced Coupling advisories from the output.                                              | `archfit analyze --no-advisories`                          |
| `--min-severity`  | enum        | empty                             | Show only advisories at or above `low`, `medium`, `high`, or `critical`.                                      | `archfit analyze --min-severity high`                      |
| `--lang`          | string list | none                              | Force named analyzers on. Repeatable. See the language setup docs for valid names.                            | `archfit analyze --lang go --lang ts`                      |
| `--require-tools` | bool        | `false`                           | Mark missing required analyzer tools as fail in the rendered verdict. The command still exits `0` on success. | `archfit analyze --require-tools`                          |
| `--progress`      | enum        | `auto`                            | Progress reporting on stderr: `auto`, `plain`, or `none`.                                                     | `archfit analyze --progress plain`                         |
| `-q, --quiet`     | bool        | `false`                           | Suppress progress output.                                                                                     | `archfit analyze -q --json`                                |

Examples:

```sh
archfit
archfit analyze -c .archfit.yaml
archfit analyze --markdown -c .archfit.yaml > archfit-report.md
archfit analyze --json -c .archfit.yaml | jq .
archfit analyze --format scorecard -c .archfit.yaml
archfit analyze --base origin/main --format text -c .archfit.yaml
archfit analyze --ai-summary -c .archfit.yaml
archfit analyze --refresh --require-tools -c .archfit.yaml
```

## `archfit check`

Purpose:

- Run the same pipeline as `analyze`.
- Turn the result into CI-friendly exit codes.
- Use in CI, pre-push hooks, and agent validation loops.

Use cases:

- blocking merges on architecture drift;
- gating on missing analyzers with `--require-tools`;
- emitting JSON or SARIF for bots and code-scanning systems;
- comparing a branch to a base ref while still enforcing the gate.

Synopsis:

```sh
archfit check [flags]
```

Flags:

| Flag              | Type      | Default                           | Effect                                                                                          | Example                                                  |
| ----------------- | --------- | --------------------------------- | ----------------------------------------------------------------------------------------------- | -------------------------------------------------------- |
| `-c, --config`    | path      | `.archfit.yaml`                   | Config file to load.                                                                            | `archfit check -c .archfit.yaml`                         |
| `--root`          | path      | directory of `--config`           | Repo root to analyze.                                                                           | `archfit check --root ../repo -c ./policy/.archfit.yaml` |
| `--base`          | git ref   | none                              | Compare the current branch against a base ref and report the comparison and its comparability reasons.                    | `archfit check --base origin/main`                       |
| `--no-advisories` | bool      | `false`                           | Hide informational Balanced Coupling advisories from the output.                                | `archfit check --no-advisories`                          |
| `--min-severity`  | enum      | empty                             | Show only advisories at or above `low`, `medium`, `high`, or `critical`.                        | `archfit check --min-severity high`                      |
| `--refresh`       | bool      | `false`                           | Re-run extractors and refresh the cache. Use after installing or updating analyzer tools.       | `archfit check --refresh`                                |
| `--require-tools` | bool      | `false`                           | Exit non-zero when any required analyzer tool is missing.                                       | `archfit check --require-tools`                          |
| `--json`          | bool      | `false`                           | Shorthand for `--format json`.                                                                  | `archfit check --json`                                   |
| `--markdown`      | bool      | `false`                           | Shorthand for `--format markdown`.                                                              | `archfit check --markdown > archfit-report.md`           |
| `--sarif`         | bool      | `false`                           | Shorthand for `--format sarif`.                                                                 | `archfit check --sarif > archfit.sarif`                  |
| `--format`        | enum list | `text` when no format flag is set | Output one or more formats: `json`, `text`, `markdown`, `md`, `sarif`, `scorecard`, `legacy-json`. Repeatable. | `archfit check --format text --format json`              |
| `--progress`      | enum      | `auto`                            | Progress reporting on stderr: `auto`, `plain`, or `none`.                                       | `archfit check --progress plain`                         |
| `-q, --quiet`     | bool      | `false`                           | Suppress progress output.                                                                       | `archfit check -q --json`                                |

Examples:

```sh
archfit check -c .archfit.yaml
archfit check --json -c .archfit.yaml
archfit check --base origin/main --sarif -c .archfit.yaml > archfit.sarif
archfit check --require-tools -c .archfit.yaml
archfit check --refresh --format scorecard -c .archfit.yaml
archfit check --min-severity high -c .archfit.yaml
```

## `archfit baseline`

Purpose:

- Accept the current findings as the baseline.
- Let future `check` runs block only new drift.

Use cases:

- first-time rollout;
- re-anchoring after a deliberate cleanup wave;
- re-anchoring after a scorer or rubric change.

A baseline is always the accepted state of the tree as checked out. There is no
git-base mode; compare against a ref with `archfit check --base` instead.

Synopsis:

```sh
archfit baseline [flags]
```

What it writes (`schema_version: archfit.baseline.v2`):

- Saves the baseline beside the config as `.archfit-baseline.json`.
- Keeps the accepted finding fingerprints and the metric snapshot, so later runs
  can detect fixed findings.
- Keeps the architecture-state reference under `state`: the four comparison
  fingerprints (`config_hash`, `model_hash`, `labels_hash`, `rubric_version`)
  together with the facts they qualify — `hard_gate_finding_ids`,
  `qualifying_seam_ids`, and a snapshot of the nine dimensions. A dimension or
  seam delta is claimed only when all four fingerprints still match; any
  mismatch is reported as non-comparable and names the input that moved.
- Stores **no repository score**. Schema v2 retired the scalar gate, so a stored
  score would anchor nothing.

Reading an older baseline (`archfit.baseline.v1`):

- Its accepted finding fingerprints stay usable — those are acceptance decisions
  the owner made.
- Its scalar snapshot is ignored, reported as `legacy_score_snapshot_ignored`.
- Every state, dimension, and seam comparison against it is non-comparable: a
  file written before the seam ledger existed records no seams, and reading that
  as "there were none" would report every existing seam as newly introduced.
- It is never rewritten on read. Re-baseline deliberately to upgrade it.

Capture is a pure function of the tree and the config: the run reads an empty
accepted set, so two captures over an unchanged tree are byte-identical.

Flags:

| Flag              | Type | Default                 | Effect                                                                | Example                                                 |
| ----------------- | ---- | ----------------------- | --------------------------------------------------------------------- | ------------------------------------------------------- |
| `-c, --config`    | path | `.archfit.yaml`         | Config file.                                                          | `archfit baseline -c .archfit.yaml`                     |
| `-r, --root`      | path | directory of `--config` | Repo root to analyze.                                                 | `archfit baseline -r ../repo -c ./policy/.archfit.yaml` |
| `--no-advisories` | bool | `false`                 | Exclude informational Balanced Coupling advisories from the baseline. | `archfit baseline --no-advisories`                      |
| `--refresh`       | bool | `false`                 | Re-run extractors and refresh the cache.                              | `archfit baseline --refresh`                            |

Examples:

```sh
archfit baseline -c .archfit.yaml
archfit baseline --no-advisories -c .archfit.yaml
archfit baseline --refresh -r . -c .archfit.yaml
```

## `archfit explain <fingerprint>`

Purpose:

- Re-run the pipeline.
- Find one finding by fingerprint prefix.
- Print the rule, severity, edge, modules, locations, and constraint for that finding.

Use cases:

- understanding a CI failure;
- sharing a single finding in review;
- appending an AI narrative to one finding instead of to the whole report.

Synopsis:

```sh
archfit explain <fingerprint-prefix> [flags]
```

Tips:

- Use the fingerprint prefix from `archfit check --json` or `archfit analyze --json`.
- The prefix must match exactly one finding in the current pipeline output.

Flags:

| Flag           | Type | Default                 | Effect                                                                   | Example                                                         |
| -------------- | ---- | ----------------------- | ------------------------------------------------------------------------ | --------------------------------------------------------------- |
| `-c, --config` | path | `.archfit.yaml`         | Config file.                                                             | `archfit explain ab12cd34 -c .archfit.yaml`                     |
| `-r, --root`   | path | directory of `--config` | Repo root to analyze.                                                    | `archfit explain ab12cd34 -r ../repo -c ./policy/.archfit.yaml` |
| `--ai-summary` | bool | `false`                 | Append an off-gate AI narrative for this finding. Requires `ai:` config. | `archfit explain ab12cd34 --ai-summary -c .archfit.yaml`        |
| `--refresh`    | bool | `false`                 | Re-run extractors and refresh the cache.                                 | `archfit explain ab12cd34 --refresh -c .archfit.yaml`           |

Examples:

```sh
archfit explain 5fd7d1c9 -c .archfit.yaml
archfit explain 5fd7d1c9 --ai-summary -c .archfit.yaml
archfit explain 5fd7d1c9 --refresh -c .archfit.yaml
archfit explain 5fd7d1c9 -r ../repo -c ./policy/.archfit.yaml
```

## `archfit doctor`

Purpose:

- Check which analyzer tools are available.
- Show install hints for missing tools.
- Optionally install what archfit can install.

Use cases:

- setting up a new machine;
- debugging analyzer coverage gaps;
- previewing install commands with `--dry-run`.

Synopsis:

```sh
archfit doctor [flags]
```

What it reports:

- a tool status table;
- off-gate AI provider status from `.archfit.yaml` when present;
- `.archfit-cache/llm` entry count when AI is configured;
- config-load errors for `.archfit.yaml` when the default config exists but is invalid.

Flags:

| Flag            | Type      | Default                              | Effect                                                                 | Example                                    |
| --------------- | --------- | ------------------------------------ | ---------------------------------------------------------------------- | ------------------------------------------ |
| `--fix`         | bool      | `false`                              | Install missing analyzer toolchains that archfit knows how to install. | `archfit doctor --fix`                     |
| `--lang`        | enum list | all languages when used with `--fix` | Scope installs to `go`, `ts`, `py`, or `rust`. Repeatable.             | `archfit doctor --fix --lang ts --lang py` |
| `-n, --dry-run` | bool      | `false`                              | With `--fix`, print install commands without running them.             | `archfit doctor --fix --dry-run`           |

Examples:

```sh
archfit doctor
archfit doctor --fix
archfit doctor --fix --dry-run
archfit doctor --fix --lang go --lang rust
```

## `archfit config init`

Purpose:

- Discover project structure.
- Write a starter `.archfit.yaml`.
- Optionally add an off-gate AI classification pass.

Use cases:

- first setup in a repo;
- generating a draft to review before saving;
- creating an AI-classified draft without touching the live config.

Synopsis:

```sh
archfit config init [flags]
```

Notes:

- Relative output paths resolve against `--root`.
- Use `-o -` to write the rendered config to stdout.
- Without `--force`, an existing valid config is left untouched.
- `--apply` requires `--ai-classify`.

Flags:

| Flag            | Type        | Default           | Effect                                                                                    | Example                                                               |
| --------------- | ----------- | ----------------- | ----------------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| `-r, --root`    | path        | `.`               | Project root directory to inspect.                                                        | `archfit config init -r .`                                            |
| `-o, --output`  | path or `-` | `.archfit.yaml`   | Output file. Relative paths resolve against `--root`. Use `-` for stdout.                 | `archfit config init -r . -o draft.yaml`                              |
| `--force`       | bool        | `false`           | Overwrite an existing config and keep a timestamped backup.                               | `archfit config init --force -r .`                                    |
| `--ai-classify` | bool        | `false`           | Run an off-gate AI classification pass. Requires `ai:` config or provider override flags. | `archfit config init --ai-classify -r .`                              |
| `--apply`       | bool        | `false`           | With `--ai-classify`, write AI classifications directly into the config.                  | `archfit config init --ai-classify --apply -r .`                      |
| `--ai-provider` | enum        | `anthropic`       | Override AI provider: `anthropic`, `openai`, or `ollama`.                                 | `archfit config init --ai-classify --ai-provider openai -r .`         |
| `--ai-model`    | string      | `claude-opus-4-8` | Override the AI model.                                                                    | `archfit config init --ai-classify --ai-model claude-sonnet-4-5 -r .` |
| `--refresh`     | bool        | `false`           | Re-run AI calls and refresh the AI cache.                                                 | `archfit config init --ai-classify --refresh -r .`                    |

Examples:

```sh
archfit config init --root .
archfit config init --root . --output draft.yaml
archfit config init --root . --output -
archfit config init --ai-classify --root . --output draft.yaml
archfit config init --ai-classify --apply --root .
archfit config init --force --root .
```

## `archfit config update`

Purpose:

- Re-discover project structure.
- Diff it against the existing config.
- Print a drift report and a config review, or apply structural edits.

Use cases:

- after adding, removing, or moving modules;
- after enabling more language analyzers;
- checking which config fields still need a decision;
- reviewing AI proposals for new modules without changing the gate behavior;
- migrating a config to the current schema version (`--migration-only`).

Synopsis:

```sh
archfit config update [flags]
```

Notes:

- Without `--apply`, this command is report-only.
- With `--apply`, only added modules, path drift, and settings are written live.
- `--apply` never deletes, comments out, or re-keys a configured module stanza.
- `--migration-only` is a different job: a mechanical schema rewrite of the one
  config file, with no discovery, no tool calls, and no cache access. It cannot
  be combined with `--ai-classify` or `--refresh` (exit 3), and `--json`
  previews while `--apply` writes — combining those two is also exit 3.
- AI semantic proposals remain review-only even when `--apply` is used.
- The report leads with one status line: `action_required`, `review_available`,
  or `no_known_issues`. `no_known_issues` means these checks found nothing; it is
  not a claim that the config is complete.
- With `--apply`, a run with nothing to write still prints the same status line
  and report as the preview. A structure with no pending edits is not a clean
  config: module gaps, unclassifiable modules, and unchecked stanzas are still
  reported.
- With `--apply`, a run that DOES write an edit reports the review-only half too:
  the edit it made, then module gaps, unclassifiable modules, naming
  differences, unmatched stanzas, and unchecked stanzas. Having an edit to apply
  never hides what apply refuses to write.
- The report lists every change `--apply` would write, including non-module
  settings such as the Rust deep-analysis defaults.

Review model:

| Field                | Meaning                                                                                                                                                      |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `structure`          | Discovery differences: pending edits plus the review-only buckets below.                                                                                     |
| `issues`             | Per-module config gaps that need a decision, each with a reason and a next action.                                                                           |
| `unchecked_modules`  | Configured modules the per-module checks did NOT evaluate, with the reason. An empty `issues` list is only "clean" for the modules that are not listed here. |
| `review_suggestions` | Deterministic deploy-unit and distance-config proposals. `--apply` never writes them.                                                                        |

`structure` fields:

| Field             | Applied by `--apply`? | Meaning                                                                               |
| ----------------- | --------------------- | ------------------------------------------------------------------------------------- |
| `added_modules`   | yes                   | Modules discovery found that the config does not declare.                             |
| `path_drift`      | yes                   | Declared modules whose configured paths differ from the discovered paths.             |
| `settings`        | yes                   | Non-module settings, such as the Rust deep-analysis defaults.                         |
| `name_drift`      | no                    | A configured module and a discovered module own the same paths under different names. |
| `removed_modules` | no                    | Configured modules discovery did not emit.                                            |

`name_drift` and `removed_modules` are review-only. Resolving either means
re-keying or deleting a stanza, which discards its `owner`, `subdomain`,
`volatility`, `layer`, and `public` values, so both stay human decisions.
Neither raises the status to `action_required` — only pending edits and module
`issues` do. A report whose findings are review items alone reads
`review_available`, and those items are not just these two buckets: modules
archfit cannot classify and the stanzas in `unchecked_modules` count as well.

Issue codes:

| Code                       | Condition                                                                          |
| -------------------------- | ---------------------------------------------------------------------------------- |
| `missing_owner`            | The module has no `owner:`, so cross-module distance falls back to code structure. |
| `missing_volatility_input` | The module has neither `subdomain:` nor `volatility:`. Either field closes it.     |
| `missing_layer`            | The module has no `layer:` while a `forbidden_layer_direction` rule is not `off`.  |

JSON:

- `--json` emits the review as `archfit.config-review.v1`.
- Every list is a JSON array, never `null`.
- Issues sort by module, then by code. All other lists keep their report order.
- `--json` is report-only and cannot be combined with `--apply`, `--ai-classify`,
  or `--refresh`. Those combinations exit `3` before any discovery or write.

Flags:

| Flag            | Type   | Default                 | Effect                                                                   | Example                                                                     |
| --------------- | ------ | ----------------------- | ------------------------------------------------------------------------ | --------------------------------------------------------------------------- |
| `-c, --config`  | path   | `.archfit.yaml`         | Config file path.                                                        | `archfit config update -c .archfit.yaml`                                    |
| `-r, --root`    | path   | directory of `--config` | Project root directory to scan.                                          | `archfit config update -r . -c .archfit.yaml`                               |
| `--ai-classify` | bool   | `false`                 | Run AI classification for unclassified modules. Off-gate.                | `archfit config update --ai-classify -c .archfit.yaml`                      |
| `--apply`       | bool   | `false`                 | Write structural changes live into `.archfit.yaml`. Backups are created. | `archfit config update --apply -c .archfit.yaml`                            |
| `--json`        | bool   | `false`                 | Emit the review as JSON. Report-only.                                    | `archfit config update --json -c .archfit.yaml`                             |
| `--migration-only` | bool | `false`                | Migrate the config to the current schema version and nothing else.       | `archfit config update --migration-only --apply -c .archfit.yaml`           |
| `--refresh`     | bool   | `false`                 | Re-run AI calls and refresh the AI cache.                                | `archfit config update --ai-classify --refresh -c .archfit.yaml`            |
| `--ai-provider` | string | `anthropic`             | Override the AI provider.                                                | `archfit config update --ai-classify --ai-provider ollama -c .archfit.yaml` |
| `--ai-model`    | string | `claude-opus-4-8`       | Override the AI model.                                                   | `archfit config update --ai-classify --ai-model llama3.1 -c .archfit.yaml`  |

Examples:

```sh
archfit config update -c .archfit.yaml
archfit config update --json -c .archfit.yaml
archfit config update --apply -c .archfit.yaml
archfit config update --ai-classify -c .archfit.yaml
archfit config update --ai-classify --apply -c .archfit.yaml
archfit config update --ai-classify --refresh -c .archfit.yaml
```

## `archfit config compare <candidate>`

Purpose:

- Measure one source tree twice: once under the current config, once under a candidate config.
- Report the finding, coverage, and measurement differences between the two.

Use cases:

- checking what a proposed module split, rule change, or analyzer switch would report;
- checking whether a config edit measured more of the tree, or simply less of it;
- reviewing a config change without touching the baseline or the gate.

Synopsis:

```sh
archfit config compare <candidate> [flags]
```

Notes:

- Report-only. Exit `0` after a successful comparison, exit `3` on an input or
  runtime error. Findings never change the exit code.
- Both runs use an empty accepted baseline. `.archfit-baseline.json` records
  findings accepted under the current config, so applying it would silence the
  candidate's findings by the current config's history.
- Both runs measure the same tree with the same pinned labels and fact cache.
  Only the config file differs, so a candidate stored outside the repo is fine.
- Nothing is written. Config, baseline, labels, candidate, and policy files stay
  byte-identical; normal fact-cache reads and writes still happen.
- The report never states that a candidate config is better. A config that scores
  higher because it measured less of the tree is a measurement loss, not an
  improvement.

Report model:

| Section                 | Meaning                                                                                                                                                        |
| ----------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `coverage evidence`     | Whether the two runs rest on comparable analyzer evidence. Graded, and reported separately from the differences.                                               |
| measurement differences | Only what changed: the overall score, the one-sided finding IDs, the classified-edge counts, and the classification mix (strength, distance, distance basis, volatility, severity, and volatility provenance). Nothing changed prints `No change in score, findings, edge counts, or classification mix.` — a claim about those measurements, not a claim that the two configs are equivalent. |
| measurement loss        | Warnings raised when the candidate measured less of the same tree.                                                                                             |

Coverage grades:

| Grade                  | Condition                                                                                                                                                                                                                     |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `comparable`           | Every compared analyzer ran on both sides. A per-language analyzer absent on both sides with no coverage gap drops out of the comparison entirely — that language is simply not in the tree. A language the repo contains but the config switched off reports `disabled`, not absent, so it never drops out this way.                                  |
| `comparable_with_gaps` | The sides agree, but at least one analyzer was absent, disabled, or left import specifiers unresolved on **both** sides. The blindness is shared, so the comparison rests on it — each such analyzer is listed with a reason. |
| `not_comparable`       | An analyzer's evidence differs between the sides, did not finish (timed out, or partial from a run that did not complete), was absent on both sides but expected by only one, or its coverage row is missing or duplicated.   |

A `not_comparable` grade is about evidence, not about the configs: it can appear
on a run that reports no measurement differences at all. Read the grade first,
then the differences.

An `owner:` or `deploy_unit:` edit typically moves edges between distance rungs
without moving the score: with `balance = max(|S−D|, 10−V)` the `10−V` term
dominates for every low-volatility target. That shift shows up as a
classification-mix line; `--json` carries the full histograms on both sides under
`current.classified_edges` and `candidate.classified_edges`.

Measurement-loss warnings:

| Code                   | Condition                                                                                                |
| ---------------------- | -------------------------------------------------------------------------------------------------------- |
| `scored_fraction_fell` | The candidate placed a smaller share of cross-boundary edges on a concrete balance.                      |
| `abstained_edges_rose` | The candidate abstained on more cross-boundary edges.                                                    |
| `external_edges_rose`  | The candidate pushed more edges outside the declared module set, excluding them from `coupling_balance`. |

JSON:

- `--json` emits the comparison as `archfit.config-compare.v1`.
- Every list is a JSON array, never `null`. All ID lists are sorted.
- `score_delta` is `null` when either side could not measure `coupling_balance`.
  `null` means unknown; `0` means measured and unchanged.
- `classified_edges` is `null` when a side classified no edges at all.
- `finding_count` counts the distinct observed finding IDs of that side, with
  fixed entries excluded.
- Findings are bucketed as `current_only_ids`, `candidate_only_ids`, and
  `both_ids`. Alternative configs have no time order, so nothing is labelled
  introduced or resolved. Gate and advisory forms of one finding share one ID.

Flags:

| Flag           | Type | Default                 | Effect                                                    | Example                                                         |
| -------------- | ---- | ----------------------- | --------------------------------------------------------- | --------------------------------------------------------------- |
| `<candidate>`  | path | required                | Candidate config file to measure against the current one. | `archfit config compare candidate.archfit.yaml`                 |
| `-c, --config` | path | `.archfit.yaml`         | Current config file path.                                 | `archfit config compare cand.yaml -c .archfit.yaml`             |
| `--root`       | path | directory of `--config` | Repository root to analyze.                               | `archfit config compare cand.yaml --root . -c ci/.archfit.yaml` |
| `--json`       | bool | `false`                 | Emit the comparison as JSON. Report-only.                 | `archfit config compare cand.yaml --json -c .archfit.yaml`      |

Examples:

```sh
archfit config compare candidate.archfit.yaml -c .archfit.yaml
archfit config compare candidate.archfit.yaml --json -c .archfit.yaml | jq .
archfit config compare candidate.archfit.yaml --root . -c ci/.archfit.yaml
```

## `archfit config enrich ...`

Purpose:

- Draft off-gate AI annotations for review.
- Keep the deterministic gate separate from AI judgment.
- Write drafts into sidecar files, then pin approved values into `.archfit.yaml` where supported.

Group synopsis:

```sh
archfit config enrich labels [flags]
archfit config enrich abstained [flags]
archfit config enrich owner [flags]
archfit config enrich volatility [flags]
archfit config enrich subdomain [flags]
```

Common behavior:

- These commands are review workflows, not gate workflows.
- `labels` and `abstained` write to `.archfit-labels.yaml`.
- `owner` writes drafts to `.archfit-owners.yaml`.
- `volatility` writes drafts to `.archfit-volatility.yaml`.
- `subdomain` writes drafts to `.archfit-subdomains.yaml`.
- `owner`, `volatility`, and `subdomain` can later pin approved entries into `.archfit.yaml` with `--apply`.

### `archfit config enrich labels`

Purpose:

- Draft coupling-strength labels for cross-module edges.
- Focus on pairs that look refinable from the deterministic evidence.

Synopsis:

```sh
archfit config enrich labels [flags]
```

Flags:

| Flag           | Type | Default                 | Effect                                   | Example                                                   |
| -------------- | ---- | ----------------------- | ---------------------------------------- | --------------------------------------------------------- |
| `-c, --config` | path | `.archfit.yaml`         | Config file.                             | `archfit config enrich labels -c .archfit.yaml`           |
| `-r, --root`   | path | directory of `--config` | Repo root to analyze.                    | `archfit config enrich labels -r . -c .archfit.yaml`      |
| `--refresh`    | bool | `false`                 | Re-run extractors and refresh the cache. | `archfit config enrich labels --refresh -c .archfit.yaml` |

Examples:

```sh
archfit config enrich labels -c .archfit.yaml
archfit config enrich labels -r . -c .archfit.yaml
archfit config enrich labels --refresh -c .archfit.yaml
```

### `archfit config enrich abstained`

Purpose:

- Draft labels only for abstained cross-module edges.
- Use code snippets when the deterministic scorer could not classify strength.

Synopsis:

```sh
archfit config enrich abstained [flags]
```

Flags:

| Flag           | Type | Default                 | Effect                                   | Example                                                      |
| -------------- | ---- | ----------------------- | ---------------------------------------- | ------------------------------------------------------------ |
| `-c, --config` | path | `.archfit.yaml`         | Config file.                             | `archfit config enrich abstained -c .archfit.yaml`           |
| `-r, --root`   | path | directory of `--config` | Repo root to analyze.                    | `archfit config enrich abstained -r . -c .archfit.yaml`      |
| `--refresh`    | bool | `false`                 | Re-run extractors and refresh the cache. | `archfit config enrich abstained --refresh -c .archfit.yaml` |

Examples:

```sh
archfit config enrich abstained -c .archfit.yaml
archfit config enrich abstained -r . -c .archfit.yaml
archfit config enrich abstained --refresh -c .archfit.yaml
```

### `archfit config enrich owner`

Purpose:

- Draft a module owner per module.
- Use CODEOWNERS context when available.

Synopsis:

```sh
archfit config enrich owner [flags]
```

Flags:

| Flag            | Type   | Default                 | Effect                                                           | Example                                                                      |
| --------------- | ------ | ----------------------- | ---------------------------------------------------------------- | ---------------------------------------------------------------------------- |
| `-c, --config`  | path   | `.archfit.yaml`         | Config file.                                                     | `archfit config enrich owner -c .archfit.yaml`                               |
| `-r, --root`    | path   | directory of `--config` | Repo root to analyze.                                            | `archfit config enrich owner -r . -c .archfit.yaml`                          |
| `--refresh`     | bool   | `false`                 | Re-run extractors and refresh the cache.                         | `archfit config enrich owner --refresh -c .archfit.yaml`                     |
| `--apply`       | bool   | `false`                 | Read approved draft entries and write them into `.archfit.yaml`. | `archfit config enrich owner --apply -c .archfit.yaml`                       |
| `--reviewed-by` | string | empty                   | Stamp the reviewer identity on applied entries.                  | `archfit config enrich owner --apply --reviewed-by @alexei -c .archfit.yaml` |

Examples:

```sh
archfit config enrich owner -c .archfit.yaml
archfit config enrich owner --refresh -c .archfit.yaml
archfit config enrich owner --apply -c .archfit.yaml
archfit config enrich owner --apply --reviewed-by @you -c .archfit.yaml
```

### `archfit config enrich volatility`

Purpose:

- Draft module volatility values.
- Pin approved values into the config later.

Synopsis:

```sh
archfit config enrich volatility [flags]
```

Flags:

| Flag            | Type   | Default                 | Effect                                                           | Example                                                                           |
| --------------- | ------ | ----------------------- | ---------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| `-c, --config`  | path   | `.archfit.yaml`         | Config file.                                                     | `archfit config enrich volatility -c .archfit.yaml`                               |
| `-r, --root`    | path   | directory of `--config` | Repo root to analyze.                                            | `archfit config enrich volatility -r . -c .archfit.yaml`                          |
| `--refresh`     | bool   | `false`                 | Re-run extractors and refresh the cache.                         | `archfit config enrich volatility --refresh -c .archfit.yaml`                     |
| `--apply`       | bool   | `false`                 | Read approved draft entries and write them into `.archfit.yaml`. | `archfit config enrich volatility --apply -c .archfit.yaml`                       |
| `--reviewed-by` | string | empty                   | Stamp the reviewer identity on applied entries.                  | `archfit config enrich volatility --apply --reviewed-by @alexei -c .archfit.yaml` |

Examples:

```sh
archfit config enrich volatility -c .archfit.yaml
archfit config enrich volatility --refresh -c .archfit.yaml
archfit config enrich volatility --apply -c .archfit.yaml
archfit config enrich volatility --apply --reviewed-by @you -c .archfit.yaml
```

### `archfit config enrich subdomain`

Purpose:

- Draft module subdomains.
- Draft volatility alongside subdomain when the model provides it.
- Pin approved values into the config later.

Synopsis:

```sh
archfit config enrich subdomain [flags]
```

Flags:

| Flag            | Type   | Default                 | Effect                                                           | Example                                                                          |
| --------------- | ------ | ----------------------- | ---------------------------------------------------------------- | -------------------------------------------------------------------------------- |
| `-c, --config`  | path   | `.archfit.yaml`         | Config file.                                                     | `archfit config enrich subdomain -c .archfit.yaml`                               |
| `-r, --root`    | path   | directory of `--config` | Repo root to analyze.                                            | `archfit config enrich subdomain -r . -c .archfit.yaml`                          |
| `--refresh`     | bool   | `false`                 | Re-run extractors and refresh the cache.                         | `archfit config enrich subdomain --refresh -c .archfit.yaml`                     |
| `--apply`       | bool   | `false`                 | Read approved draft entries and write them into `.archfit.yaml`. | `archfit config enrich subdomain --apply -c .archfit.yaml`                       |
| `--reviewed-by` | string | empty                   | Stamp the reviewer identity on applied entries.                  | `archfit config enrich subdomain --apply --reviewed-by @alexei -c .archfit.yaml` |

Examples:

```sh
archfit config enrich subdomain -c .archfit.yaml
archfit config enrich subdomain --refresh -c .archfit.yaml
archfit config enrich subdomain --apply -c .archfit.yaml
archfit config enrich subdomain --apply --reviewed-by @you -c .archfit.yaml
```

## Shared flags

These flags repeat across multiple commands.

### `-c, --config`

Used by:

- `archfit analyze`
- `archfit check`
- `archfit baseline`
- `archfit explain`
- `archfit config update`
- `archfit config compare`
- `archfit config enrich ...`

Rules:

- Default path is `.archfit.yaml`.
- `analyze` and `check` expect a real config file at that default path.
- `baseline`, `explain`, and the `config` subcommands resolve sidecar files beside the config.
- For `config compare`, `-c` is the CURRENT config. The candidate is the positional
  argument, and sidecar files still resolve beside the current config.

Examples:

```sh
archfit check -c .archfit.yaml
archfit explain ab12cd34 -c ./policy/.archfit.yaml
archfit config update -c ./policy/.archfit.yaml
```

### `--root` / `-r, --root`

Used by:

- long form only: `archfit analyze`, `archfit check`, `archfit config compare`
- short and long form: `archfit baseline`, `archfit explain`, `archfit config init`, `archfit config update`, `archfit config enrich ...`

Effect:

- Sets the analysis boundary.
- Lets a policy file live outside the repo being scanned.
- Changes what files, edges, and coverage counts are inside scope.

Examples:

```sh
archfit analyze --root ../repo -c ./policy/.archfit.yaml
archfit check --root ./server/shared -c ./policies/.archfit.yaml
archfit config enrich labels -r . -c .archfit.yaml
```

### `--base`

Used by:

- `archfit analyze`
- `archfit check`

Effect:

- Compares the current branch against a git ref such as `main` or `origin/main`.
- Adds a base-vs-head delta to the normal output.
- Adds `git_finding_delta` to `--format legacy-json` output: which current repair
  tasks the change introduced, which pre-date the base ref, and which could not
  be placed. The block is diagnostic-only and is **not** part of
  `archfit.architecture-state.v1`, so `--json` does not carry it.
- Never changes the verdict or the exit code. A base worktree or base pipeline
  error exits `3` and prints no partial output.
- Not accepted by `archfit baseline`, which always records the tree as checked out.

Examples:

```sh
archfit analyze --base origin/main -c .archfit.yaml
archfit check --base main --format legacy-json -c .archfit.yaml
```

Reading the origin block (`legacy-json` only, and only for one release — see
[release notes](release-notes.md)):

```sh
archfit check --base main --format legacy-json -c .archfit.yaml \
  | jq '.git_finding_delta.introduced_finding_ids'
```

A task is called introduced only when every active analyzer covered both sides
equivalently. A missing or duplicated coverage row, a timeout, a partial from a
run that did not complete, and any one-sided analyzer evidence all put the task
in `unknown_origin_finding_ids` and name the family in `comparison_reasons`.

Two **symmetric** degradations still pair, so the origin block stays usable in
the environments where they are normal: a partial from unresolved import
specifiers (the steady state for dependency-cruiser and grimp) and an analyzer
that was unavailable on both sides (its tool is not installed). Both keep the
status `comparable` and both always name themselves in `comparison_reasons`,
with each side's magnitude — they are never paired silently. See
[the agent feedback loop](agent-feedback.md#git_finding_delta--which-repair-tasks-this-change-introduced).

### `--format` and format shorthands

Used by:

- `archfit analyze`
- `archfit check`

Choices:

- `text`
- `json`
- `markdown`
- `md`
- `sarif`
- `scorecard`

Shorthands:

- `--json`
- `--markdown`
- `--sarif`

Rules:

- Use one shorthand, or use `--format`.
- Do not mix shorthands with `--format`.
- Repeat `--format` when you need more than one output.

Examples:

```sh
archfit analyze --json -c .archfit.yaml
archfit check --markdown -c .archfit.yaml
archfit analyze --format text --format json -c .archfit.yaml
```

### `--progress` and `-q, --quiet`

Used by:

- `archfit analyze`
- `archfit check`

Effect:

- Progress always goes to stderr.
- `--progress auto` shows live progress only on a TTY.
- `--progress plain` prints log-safe lines.
- `--progress none` disables progress.
- `-q, --quiet` suppresses progress output.

Examples:

```sh
archfit analyze --progress plain -c .archfit.yaml
archfit check --progress none --json -c .archfit.yaml
archfit check -q --json -c .archfit.yaml
```

## Warnings reference

These are the five active stderr health warnings emitted by the pipeline.

| Warning text                                                                    | Trigger condition                                                                                                        | Next command                                  |
| ------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------- |
| `analyzer coverage gap — some edges may be unscored`                            | `diag.CoverageGaps` is not empty.                                                                                        | `archfit doctor --fix`                        |
| `0 of N edges scored — coupling strength is unknown`                            | Classified edges exist, `Total > 0`, and `Scored == 0`.                                                                  | `archfit config update -c <config>`           |
| `all N cross-module edges have unknown strength`                                | `Scored == 0`, `Abstained > 0`, and `External == 0`.                                                                     | `archfit config enrich abstained -c <config>` |
| `no internal edges found — module paths may not match source layout`            | Python all-edges-external case: `grimp` coverage status is `ok`, `Scored == 0`, and every cross-module edge is external. | `archfit config update -c <config>`           |
| `no source files matched declared module paths — check --root and module globs` | The config declares module paths, but no source file under the scan root maps to any module.                             | `archfit check --root . -c <config>`          |

Notes:

- Warnings are hints, not parser errors.
- They are meant to stop false confidence after a technically successful run.
- The command shown after `→ run:` is the recommended next step.

## Migration table

This repeats the redesign changes, but adds the exact error you see today.

| Old surface                             | Use now                                                     | Error you see today                                    | Notes                                                                                                           |
| --------------------------------------- | ----------------------------------------------------------- | ------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------- |
| `archfit analyze --gate`                | `archfit check`                                             | `archfit: unknown flag --gate, did you mean "--base"?` | Gate mode is now a command, not a flag.                                                                         |
| `archfit --gate`                        | `archfit check`                                             | `archfit: unknown flag --gate, did you mean "--base"?` | Same as above.                                                                                                  |
| `--gate`                                | `archfit check`                                             | `archfit: unknown flag --gate, did you mean "--base"?` | Applies anywhere you try the old flag.                                                                          |
| `--full`                                | remove it                                                   | `archfit: unknown flag --full`                         | Full scan is unconditional now.                                                                                 |
| `--advisory`                            | remove it, or use `--no-advisories` to hide advisories      | `archfit: unknown flag --advisory`                     | Advisories are on by default now.                                                                               |
| `--severity`                            | `--min-severity`                                            | `archfit: unknown flag --severity`                     | Threshold name changed.                                                                                         |
| `analyze --llm`                         | `analyze --ai-summary`                                      | `archfit: unknown flag --llm`                          | Rename from `llm` to `ai-summary`.                                                                              |
| `explain --llm`                         | `explain --ai-summary`                                      | `archfit: unknown flag --llm`                          | Same rename pattern as `analyze`.                                                                               |
| `config init --llm`                     | `config init --ai-classify`                                 | `archfit: unknown flag --llm`                          | AI config drafting still exists. Only the flag name changed.                                                    |
| `config update --llm`                   | `config update --ai-classify`                               | `archfit: unknown flag --llm`                          | Same rename as `config init`.                                                                                   |
| `--llm-provider`                        | `--ai-provider`                                             | `archfit: unknown flag --llm-provider`                 | Provider override name changed.                                                                                 |
| `--llm-model`                           | `--ai-model`                                                | `archfit: unknown flag --llm-model`                    | Model override name changed.                                                                                    |
| `--no-cache`                            | `--refresh`                                                 | `archfit: unknown flag --no-cache`                     | `--refresh` re-runs and writes fresh cache entries back.                                                        |
| `--no-config`                           | initialize config first with `archfit config init --root .` | `archfit: unknown flag --no-config`                    | The flag is gone.                                                                                               |
| `check --ai-summary`                    | use `analyze --ai-summary` or `explain --ai-summary`        | `archfit: unknown flag --ai-summary`                   | AI summary is report-only, not a gate feature.                                                                  |
| `analyze --require-tools` in CI scripts | `check --require-tools`                                     | no parser error                                        | `analyze` still accepts the flag, but a successful run still exits `0`. Use `check` when the exit code matters. |

## Built-in help and version flags

These are small, but they are still part of the surface.

| Flag            | Where it works                             | Effect                                |
| --------------- | ------------------------------------------ | ------------------------------------- |
| `-h, --help`    | every command                              | Show context-sensitive help and exit. |
| `-v, --version` | top-level command and command help surface | Print version and exit.               |

### `archfit config update --migration-only`

Migrates one config file to the current schema version. It is the supported
v1-to-v2 path, and the only thing `analyze` and `check` accept as an answer to a
v1 config or a retired `coupling.gate.min_band` / `max_drop` key.

```sh
archfit config update --migration-only --json -c .archfit.yaml   # preview
archfit config update --migration-only --apply -c .archfit.yaml  # write
```

What it does, deterministically:

- sets the top-level `version:` to the current schema version;
- removes the retired `coupling.gate.min_band` and `coupling.gate.max_drop`
  keys, together with the comment lines documenting them;
- inserts `coupling.gate.distributed_monolith` with `mode: warn` and
  `max_new_seams: 0`;
- reports the semantic policy change that follows from retiring the old gate.

What it deliberately does not do:

- infer `mode: fail`. Fail blocks a build, so enabling it stays an owner
  decision taken after a report-only run against a comparable reference;
- supply a missing root `version:`. A file that declares none loads under no
  archfit schema, and prepending a version line is not a safe mechanical edit —
  above a `---` document marker it would splice a second YAML document and
  silently drop the config below it. Such a file reports `unversioned`; add
  `version: 2` at column zero yourself and re-run;
- touch any other key, comment, or line. It is a line transform, not a YAML
  round-trip, so authored formatting survives;
- run discovery, analyzers, or the fact cache;
- structural synchronization — that is ordinary `config update`.

Running it twice is byte-identical: after the first pass there is nothing left
to migrate.

`--json` emits `archfit.config-migration.v1`:

```json
{
  "schema_version": "archfit.config-migration.v1",
  "status": "migration_required",
  "from_version": 1,
  "to_version": 2,
  "config": ".archfit.yaml",
  "changes": [
    { "key": "version", "action": "set", "detail": "1 → 2" },
    { "key": "coupling.gate.min_band", "action": "removed", "detail": "retired in schema v2" },
    { "key": "coupling.gate.distributed_monolith", "action": "inserted", "detail": "mode: warn, max_new_seams: 0" }
  ],
  "policy_changes": ["..."]
}
```

`status` is `migration_required`, `already_current`, or `unversioned` (no root
`version:` key, so there is no schema to migrate from). All three exit 0 under
`--json` and in preview. `--apply` exits 3 on `unversioned`, and on a result the
post-write validation rejects — a line transform cannot reach a retired key
written inside a flow mapping (`gate: {min_band: …}`), so that file is reported
and left untouched rather than stamped v2 while still unloadable. An input or
usage error also exits 3.
