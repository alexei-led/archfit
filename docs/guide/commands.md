# Commands

This page is the command and flag reference for `archfit`.

Facts first:

- Bare `archfit` is the same as `archfit analyze`.
- `archfit analyze` is report-only. A successful run exits `0` even when it reports findings.
- `archfit check` is the CI gate. It exits `1` on blocking violations, `2` when evidence or diagnostics need attention, and `3` on usage, config, or runtime errors.
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

| I want to...                                                         | Run this                                                                                                      |
| -------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------- |
| review architecture locally without failing my shell step            | `archfit analyze -c .archfit.yaml`                                                                            |
| fail CI on blocking findings or warnings                             | `archfit check -c .archfit.yaml`                                                                              |
| accept the current findings as the baseline                          | `archfit baseline -c .archfit.yaml`                                                                           |
| understand one finding in detail                                     | `archfit explain <fingerprint-prefix> -c .archfit.yaml`                                                       |
| verify analyzers are installed, or install what archfit can install  | `archfit doctor` or `archfit doctor --fix`                                                                    |
| create the first config file for a repo                              | `archfit config init --root .`                                                                                |
| sync an existing config to the current repo structure                | `archfit config update -c .archfit.yaml`                                                                      |
| read the config review from a script or an agent                     | `archfit config update --json -c .archfit.yaml`                                                               |
| find rules that can never fire and config values archfit cannot read | `archfit config lint -c .archfit.yaml`                                                                        |
| see what a candidate config would measure on the same code           | `archfit config compare candidate.archfit.yaml -c .archfit.yaml`                                              |
| draft AI labels or module metadata for review                        | `archfit config enrich <kind>` where `<kind>` is `labels`, `abstained`, `owner`, `volatility`, or `subdomain` |

## Exit codes

`archfit check` is the only command that uses all four exit codes. Its exit code
is the architecture verdict, nothing else.

| Code | Meaning                                                                                                                                                     | Commands that produce it                                                                                                                                                                                                             |
| ---- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `0`  | `healthy` — every dimension measured, every hard gate passing, no active diagnostic. For `analyze`, any valid report.                                       | `archfit`, `archfit analyze`, `archfit check`, `archfit baseline`, `archfit explain`, `archfit doctor`, `archfit config init`, `archfit config update`, `archfit config lint`, `archfit config compare`, `archfit config enrich ...` |
| `1`  | `blocked` — an active hard-gate finding, a required analyzer that did not run under `--require-tools`, or a tripped metric ratchet (`metrics.<name>.gate`). For `config lint`, at least one error diagnostic. | `archfit check`, `archfit config lint`; `archfit analyze` never exits `1` on a successful run |
| `2`  | `needs_attention` — no blocker, but an active diagnostic or a partial/unmeasured dimension.                                                                 | `archfit check`                                                                                                                                                                                                                      |
| `3`  | Usage, parse, config, or runtime error. No valid report was produced.                                                                                       | All commands                                                                                                                                                                                                                         |

Notes:

- `archfit analyze` always exits `0` after a successful analysis, whatever the verdict.
- `archfit analyze --require-tools` only changes the rendered verdict. It does not change the exit code on success.
- `archfit baseline`, `archfit explain`, `archfit doctor`, and the `config` commands are success-or-error commands: `0` or `3`.
  `archfit config lint` is the exception: it exits `1` when it reports an error diagnostic.
- Exit `0` is reachable when all nine dimensions are measured, hard gates pass,
  and no diagnostic is active. Missing supplied coverage, a non-comparable
  persisted baseline, incomplete declared operational topology, or an
  unevaluated required rule produces exit `2`, never a healthy zero. During
  adoption you may treat `0` and `2` as "not blocked" and gate on `1`; require
  `0` when complete evidence is your CI policy.
- A coupling advisory is a diagnostic, never a blocker: it can reach `2`, never
  `1`. The only coupling gate is `coupling.gate.distributed_monolith`, and it
  blocks only in `mode: fail` against a comparable reference.

## Output formats

These formats apply to `archfit analyze` and `archfit check`.

| Format            | Best for                                            | Notes                                                                                                                                                                           |
| ----------------- | --------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `text`            | terminal use, local review, quick CI logs           | Default. Headline, nine dimensions, unmeasured facts, seams, actionable findings, comparison.                                                                                   |
| `json`            | automation, bots, custom dashboards, agent loops    | `archfit.architecture-state.v1` at the document root. Use this when a script needs `agent_tasks[]`, findings, dimensions, comparison/gate-reference status, or the seam ledger. |
| `markdown` / `md` | saved audit reports, PR attachments, docs artifacts | Same facts as `json`, laid out for a human. Good for `archfit-report.md`.                                                                                                       |
| `sarif`           | GitHub code scanning and other SARIF consumers      | Findings keep their rule IDs and `archfit/v1` fingerprints; the state rides in `run.properties`.                                                                                |
| `scorecard`       | dimension-by-dimension review                       | The nine-dimension state scorecard: status, gate, confidence, denominator, metrics, and unknowns per dimension. No repository score.                                            |

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

| Flag              | Type        | Default                           | Effect                                                                                                                                   | Example                                                    |
| ----------------- | ----------- | --------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------- |
| `-c, --config`    | path        | `.archfit.yaml`                   | Config file to load.                                                                                                                     | `archfit analyze -c ./policy/.archfit.yaml`                |
| `--root`          | path        | directory of `--config`           | Repo root to analyze. Use when the policy file lives outside the checked-out repo.                                                       | `archfit analyze --root ../repo -c ./policy/.archfit.yaml` |
| `--base`          | git ref     | none                              | Compare the current run to a base ref and report the comparison and its comparability reasons.                                           | `archfit analyze --base origin/main`                       |
| `--ai-summary`    | bool        | `false`                           | Append an off-gate AI narrative review after the deterministic render. Requires `ai:` config.                                            | `archfit analyze --ai-summary -c .archfit.yaml`            |
| `--refresh`       | bool        | `false`                           | Re-run extractors, bypass cached reads, and refresh the cache with fresh results.                                                        | `archfit analyze --refresh -c .archfit.yaml`               |
| `--json`          | bool        | `false`                           | Shorthand for `--format json`.                                                                                                           | `archfit analyze --json`                                   |
| `--markdown`      | bool        | `false`                           | Shorthand for `--format markdown`.                                                                                                       | `archfit analyze --markdown > archfit-report.md`           |
| `--sarif`         | bool        | `false`                           | Shorthand for `--format sarif`.                                                                                                          | `archfit analyze --sarif > archfit.sarif`                  |
| `--format`        | enum list   | `text` when no format flag is set | Output one or more formats: `json`, `text`, `markdown` (`md` alias), `sarif`, `scorecard`. Repeatable.                                   | `archfit analyze --format text --format json`              |
| `--no-advisories` | bool        | `false`                           | Drop advisory findings: Balanced Coupling advisories and violations of `gate: warn` rules. Dropped findings do not count as diagnostics. | `archfit analyze --no-advisories`                          |
| `--min-severity`  | enum        | empty                             | Show only advisories at or above `low`, `medium`, `high`, or `critical`.                                                                 | `archfit analyze --min-severity high`                      |
| `--lang`          | string list | none                              | Force a language on: `go`, `typescript` (`ts`), `python` (`py`), `rust` (`rs`). Repeatable. Analyzer names are rejected.                 | `archfit analyze --lang go --lang ts`                      |
| `--require-tools` | bool        | `false`                           | Mark missing required analyzer tools as fail in the rendered verdict. The command still exits `0` on success.                            | `archfit analyze --require-tools`                          |
| `--progress`      | enum        | `auto`                            | Progress reporting on stderr: `auto`, `plain`, or `none`.                                                                                | `archfit analyze --progress plain`                         |
| `-q, --quiet`     | bool        | `false`                           | Suppress progress output.                                                                                                                | `archfit analyze -q --json`                                |

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

| Flag              | Type      | Default                           | Effect                                                                                                                                   | Example                                                  |
| ----------------- | --------- | --------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------- |
| `-c, --config`    | path      | `.archfit.yaml`                   | Config file to load.                                                                                                                     | `archfit check -c .archfit.yaml`                         |
| `--root`          | path      | directory of `--config`           | Repo root to analyze.                                                                                                                    | `archfit check --root ../repo -c ./policy/.archfit.yaml` |
| `--base`          | git ref   | none                              | Compare the current branch against a base ref and report the comparison and its comparability reasons.                                   | `archfit check --base origin/main`                       |
| `--no-advisories` | bool      | `false`                           | Drop advisory findings: Balanced Coupling advisories and violations of `gate: warn` rules. Dropped findings do not count as diagnostics. | `archfit check --no-advisories`                          |
| `--min-severity`  | enum      | empty                             | Show only advisories at or above `low`, `medium`, `high`, or `critical`.                                                                 | `archfit check --min-severity high`                      |
| `--refresh`       | bool      | `false`                           | Re-run extractors and refresh the cache. Use after installing or updating analyzer tools.                                                | `archfit check --refresh`                                |
| `--require-tools` | bool      | `false`                           | Exit non-zero when any required analyzer tool is missing.                                                                                | `archfit check --require-tools`                          |
| `--json`          | bool      | `false`                           | Shorthand for `--format json`.                                                                                                           | `archfit check --json`                                   |
| `--markdown`      | bool      | `false`                           | Shorthand for `--format markdown`.                                                                                                       | `archfit check --markdown > archfit-report.md`           |
| `--sarif`         | bool      | `false`                           | Shorthand for `--format sarif`.                                                                                                          | `archfit check --sarif > archfit.sarif`                  |
| `--format`        | enum list | `text` when no format flag is set | Output one or more formats: `json`, `text`, `markdown` (`md` alias), `sarif`, `scorecard`. Repeatable.                                   | `archfit check --format text --format json`              |
| `--progress`      | enum      | `auto`                            | Progress reporting on stderr: `auto`, `plain`, or `none`.                                                                                | `archfit check --progress plain`                         |
| `-q, --quiet`     | bool      | `false`                           | Suppress progress output.                                                                                                                | `archfit check -q --json`                                |

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
  and the `measurement_profile` (producer semantics, tool versions, statuses,
  and settings hash)
  together with the facts they qualify — `hard_gate_finding_ids`,
  `qualifying_seam_ids`, and a snapshot of the nine dimensions. A dimension or
  seam delta is claimed only when all four fingerprints and the measurement
  profile still match; any fingerprint or profile mismatch is reported as
  non-comparable
  and names the input that moved.
- Stores **no repository score**. Schema v2 retired the scalar gate, so a stored
  score would anchor nothing.

An older baseline such as `archfit.baseline.v1` is rejected with exit `3`.
Do not change only `schema_version`: v2 requires an architecture-state reference
that cannot be derived safely from the old file. Keep the old baseline for owner
review, inspect the current findings, then regenerate deliberately with
`archfit baseline`. The exact review-and-regenerate procedure is in
[`skills/archfit/references/migration.md`](../../skills/archfit/references/migration.md).

Capture is a pure function of the tree and the config: the run reads an empty
accepted set, so two captures over an unchanged tree are byte-identical. Active
`waived` findings are excluded from permanent acceptance, and the command
prints how many temporary findings it skipped. This is a disclosure, not a
blanket migration path: review the complete capture before committing it.

Flags:

| Flag              | Type | Default                 | Effect                                                                                                          | Example                                                 |
| ----------------- | ---- | ----------------------- | --------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------- |
| `-c, --config`    | path | `.archfit.yaml`         | Config file.                                                                                                    | `archfit baseline -c .archfit.yaml`                     |
| `-r, --root`      | path | directory of `--config` | Repo root to analyze.                                                                                           | `archfit baseline -r ../repo -c ./policy/.archfit.yaml` |
| `--no-advisories` | bool | `false`                 | Exclude advisory findings from the baseline: Balanced Coupling advisories and violations of `gate: warn` rules. | `archfit baseline --no-advisories`                      |
| `--refresh`       | bool | `false`                 | Re-run extractors and refresh the cache.                                                                        | `archfit baseline --refresh`                            |

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
- Language presence comes from each extractor's own applicability check, the
  same one analysis uses. A present language is written `enabled: true` (Rust:
  `auto`); an absent one stays `auto`. Init never writes `enabled: false`.
- Go modules come from every Go member the extractor loads: `go.work` members,
  a root `go.mod`, or nested `go.mod` directories. A `go.work` monorepo with no
  root `go.mod` is discovered member by member.
- Init proposes a module only where the source inventory that `config lint`
  and `check` read holds production code for it. A `mocks/` package, a
  package of only generated or only test files, and a tree excluded by default
  (`reports/`, `testdata/`, `build/`, `dist/`, `vendor/`) get no module, even
  when `go list` or a `src/` subdirectory scan finds them. When Go and
  TypeScript discovery propose the same directory, the Go module keeps it.
  Rust crates from `cargo metadata` are always kept.
- `public:` is written only for a directory that is itself a Go package with
  production code; a bare grouping directory names no graph node.
- Starter rules: `module_cycle` (`no-module-cycles`) always, and
  `forbidden_layer_direction` (`no-layer-back-edges`) when discovery inferred
  two or more layers. Each gets `gate: fail` when the init-time import graph
  covers every module and shows no violation, and `gate: warn` with a comment
  otherwise: the current violation count, or "no complete import graph" with a
  `Why:` line. The graph is incomplete when TypeScript or Python modules exist
  (their discovery builds no graph), when Rust is present (analysis adds
  intra-crate modules), or when a Go module also owns source in another
  language, such as TypeScript under `web/ui/`, which init's Go graph omits.
- Init does not guess layers from directory or package names, in any language.
  The one exception is evidence, not a name: for a Rust workspace, each crate
  is put in a tier of the `cargo metadata` dependency graph (`layer-0` for
  crates that depend on no other member, then one tier above the deepest crate
  it depends on). Dev-dependencies are left out, as in analysis, so every
  current crate edge points to an earlier tier and the rule starts with zero
  back-edges. With fewer than two layers, init writes a commented `layers:`
  example and a commented `no-layer-back-edges` rule: list your layers
  innermost first, set `layer:` on each module, and uncomment the rule.
- The generated config passes `archfit config lint`.

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
- reviewing AI proposals for new modules without changing the gate behavior.

Synopsis:

```sh
archfit config update [flags]
```

Notes:

- Without `--apply`, this command is report-only.
- With `--apply`, only added modules, path drift, and settings are written live.
- `--apply` never deletes, comments out, or re-keys a configured module stanza.
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
| `added_modules`   | yes                   | Discovered modules holding source no configured module owns.                          |
| `path_drift`      | yes                   | Declared modules whose configured paths differ from the discovered paths.             |
| `settings`        | yes                   | Non-module settings, such as the Rust deep-analysis defaults.                         |
| `name_drift`      | no                    | A configured module and a discovered module own the same paths under different names. |
| `removed_modules` | no                    | Configured modules discovery did not emit.                                            |

A discovered module whose every source (Go package, Python package) a
configured module already owns, by the same most-specific match analysis uses,
is not added: the curated map just groups the code more finely or more coarsely
than discovery. Its owning stanzas are not reported as removed, and their fields
are checked. TypeScript and Rust modules carry no source list and are matched by
name.

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

## `archfit config lint`

Purpose:

- Report config defects that loading accepts but that silently weaken the policy.
- Judge them against the source tree that `check` scans, without running any analyzer.

Use cases:

- catching a rule that can never fire after a directory rename;
- catching a typo in `volatility:`, `subdomain:`, or `layer:` before it drops a finding;
- a fast CI or pre-commit step next to `archfit check`.

Synopsis:

```sh
archfit config lint [flags]
```

Exit codes:

| Code | Meaning                                                       |
| ---- | ------------------------------------------------------------- |
| `0`  | No error diagnostic. Warnings and info lines may still print. |
| `1`  | At least one error diagnostic.                                |
| `3`  | The config cannot be read, parsed, or validated.              |

Diagnostics:

| Code                     | Severity                       | Meaning                                                                                                                                                                                                                                                        |
| ------------------------ | ------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `dead_selector`          | error (warning on `gate: off`) | A `forbidden_dependency`, `public_api_only`, `internal_api_access`, or `forbidden_pattern` (`from:` only) selector matches no scanned source, or (dependency rules) only source no dependency producer analyses. A `module_dependencies` rule when no module declares `depends_on` or `visible_to`. `check` lists the same rule as not evaluated. |
| `guard_rule`             | info                           | A `guard: true` rule whose selector matches nothing, as intended.                                                                                                                                                                                              |
| `guard_matches_source`   | warning                        | A `guard: true` rule whose selectors all match source: the guarded path exists again.                                                                                                                                                                          |
| `unknown_volatility`     | error                          | `modules.<m>.volatility` is not `high`, `medium`, `low`, `frozen`, or `legacy`.                                                                                                                                                                                |
| `unknown_subdomain`      | error                          | `modules.<m>.subdomain` is not `core`, `supporting`, or `generic`.                                                                                                                                                                                             |
| `undeclared_layer`       | error                          | `modules.<m>.layer` is not declared in `layers:`.                                                                                                                                                                                                              |
| `public_outside_module`  | error                          | A `public:` entry is outside the module's own `paths:`.                                                                                                                                                                                                        |
| `public_matches_nothing` | error                          | A `public:` entry names no scanned package or module.                                                                                                                                                                                                          |
| `ambiguous_ownership`    | error                          | Two modules claim the same source at equal glob specificity; the first by name silently wins.                                                                                                                                                                  |
| `unknown_module`         | error                          | A `modules.<m>.depends_on` or `modules.<m>.visible_to` entry names no declared module, so it allows nothing. `check` prints the same text as a config warning.                                                                                               |

Notes:

- A dead selector is decided by the predicate rule evaluation uses, over the
  same source inventory, so lint and a `check` of the same config agree, with
  two exceptions. Lint runs no `cargo metadata`, so it leaves selectors a Rust
  crate could spell undecided where `check` can list them as not evaluated.
  `check --lang` turns on a language the config switches off, which changes
  the source in scope; lint reads the config as written. A selector spelled
  with the Go module path (`example.com/shop/internal/x`), a leading `./`,
  `../`, `/` or `!`, or an extglob `!(...)` never matches. Dependency-rule
  selectors are read in each language's edge spelling; see
  [Selectors that match nothing](configuration-reference.md#selectors-that-match-nothing).
  A `to:` selector that names no first-party source, such as `net/http` or
  `github.com/...`, is an external ban and is never dead. Neither is a `to:` selector that names a Go
  standard-library package (as listed by `go list std` for the toolchain in the
  repository) when a first-party directory shares its first segment, such as
  `database/sql` beside a top-level `database/`.
- Mark a rule that matches nothing on purpose, such as a ban on re-introducing
  a deleted package, with `guard: true`.
- Unknown `volatility`, `subdomain`, and `layer` values still load in config
  schema v2. `analyze` and `check` print them as config warnings.
- Rust crate selectors are not judged: crate names need `cargo metadata`, which
  lint does not run.
- JSON output (`--json`) is `archfit.config-lint.v1`:

```json
{
  "schema_version": "archfit.config-lint.v1",
  "diagnostics": [
    {
      "code": "dead_selector",
      "severity": "error",
      "path": "rules[billing_not_catalog].to",
      "message": "to: internal/catalog/** matches no scanned source; fix the selector or set guard: true"
    }
  ]
}
```

- Text output prints one line per diagnostic: `<severity> <code> <path>: <message>`.
  A clean config prints nothing.

Flags:

| Flag           | Type | Default                     | Description                                                       | Example                                        |
| -------------- | ---- | --------------------------- | ----------------------------------------------------------------- | ---------------------------------------------- |
| `-c, --config` | path | `.archfit.yaml`             | Config file path.                                                 | `archfit config lint -c .archfit.yaml`         |
| `-r, --root`   | path | analysis root of `--config` | Repository root to lint against, resolved as `check` resolves it. | `archfit config lint -r . -c ci/.archfit.yaml` |
| `--json`       | bool | `false`                     | Emit the diagnostics as JSON.                                     | `archfit config lint --json`                   |

Examples:

```sh
archfit config lint
archfit config lint --json -c .archfit.yaml | jq '.diagnostics[] | select(.severity == "error")'
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
- Both runs also publish a `measurement_profile` containing the extractor
  semantics, tool versions/statuses, and settings hash. A profile mismatch is a
  `not_comparable` coverage result with named details, and no score delta is
  reported.
- Nothing is written. Config, baseline, labels, candidate, and policy files stay
  byte-identical; normal fact-cache reads and writes still happen.
- The report never states that a candidate config is better. A config that scores
  higher because it measured less of the tree is a measurement loss, not an
  improvement.

Report model:

| Section                 | Meaning                                                                                                                                                                                                                                                                                                                                                                                        |
| ----------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `coverage evidence`     | Whether the two runs rest on comparable analyzer evidence. Graded, and reported separately from the differences.                                                                                                                                                                                                                                                                               |
| measurement differences | Only what changed: the overall score, the one-sided finding IDs, the classified-edge counts, and the classification mix (strength, distance, distance basis, volatility, severity, and volatility provenance). Nothing changed prints `No change in score, findings, edge counts, or classification mix.` — a claim about those measurements, not a claim that the two configs are equivalent. |
| measurement loss        | Warnings raised when the candidate measured less of the same tree.                                                                                                                                                                                                                                                                                                                             |

Coverage grades:

| Grade                  | Condition                                                                                                                                                                                                                                                                                                             |
| ---------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `comparable`           | Every compared analyzer ran on both sides. A per-language analyzer absent on both sides with no coverage gap drops out of the comparison entirely — that language is simply not in the tree. A language the repo contains but the config switched off reports `disabled`, not absent, so it never drops out this way. |
| `comparable_with_gaps` | The sides agree, but at least one analyzer was absent, disabled, or left import specifiers unresolved on **both** sides. The blindness is shared, so the comparison rests on it — each such analyzer is listed with a reason.                                                                                         |
| `not_comparable`       | An analyzer's evidence differs between the sides, did not finish (timed out, or partial from a run that did not complete), was absent on both sides but expected by only one, or its coverage row is missing or duplicated.                                                                                           |

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
- `archfit config lint`
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
- short and long form: `archfit baseline`, `archfit explain`, `archfit config init`, `archfit config update`, `archfit config lint`, `archfit config enrich ...`

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
- Adds the canonical base comparison and comparability reasons to the normal output.
- In JSON, classifies each current `agent_tasks[]` entry with optional `origin`:
  `introduced`, `pre_existing`, or conservative `unknown`. Evidence differences
  are named in `comparison.task_origin_reasons`; there is no parallel task list
  or separate delta schema. See
  [Task origin with `--base`](agent-feedback.md#task-origin-with---base).
- The root `comparison` block describes this base comparison and carries the
  current run's `measurement_profile`. An unknown or incompatible profile makes
  the comparison `non_comparable` and keeps affected task origins `unknown`.
  The persisted baseline used for hard-gate and drift comparisons is reported
  separately as `gate_reference`; `--base` never replaces it.
- Never changes the verdict or exit code. A base worktree or pipeline error exits
  `3` and prints no partial output.
- Not accepted by `archfit baseline`, which always records the checked-out tree.

Examples:

```sh
archfit analyze --base origin/main -c .archfit.yaml
archfit check --base main --json -c .archfit.yaml
```

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

## Built-in help and version flags

These are small, but they are still part of the surface.

| Flag            | Where it works                             | Effect                                |
| --------------- | ------------------------------------------ | ------------------------------------- |
| `-h, --help`    | every command                              | Show context-sensitive help and exit. |
| `-v, --version` | top-level command and command help surface | Print version and exit.               |
