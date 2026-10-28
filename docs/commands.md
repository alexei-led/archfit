# Commands

This page lists the `archfit` commands and flags. It does not repeat the full configuration or output contracts.

Run `archfit <command> --help` for built-in help. Bare `archfit` runs `analyze`.

## Choose a command

| Task | Command |
| --- | --- |
| Review architecture locally | `archfit analyze` |
| Enforce the architecture gate | `archfit check` |
| Accept current findings as the gate baseline | `archfit baseline` |
| Explain one finding | `archfit explain <fingerprint-prefix>` |
| Ask which module owns a path | `archfit policy where <path>` |
| Ask whether a file can import a target | `archfit policy can-import <file> <target>` |
| Draw modules and seams | `archfit map` |
| Check or install analyzer tools | `archfit doctor` or `archfit doctor --fix` |
| Create or review the config | `archfit config init` or `archfit config update` |
| Compare two configs on one source tree | `archfit config compare <candidate>` |
| Check config defects | `archfit config lint` |
| Run an agent or Git hook | `archfit hook claude` or `archfit hook git` |
| Render the instruction block | `archfit agents-md --write` |
| Install the bundled agent skill | `archfit skill install` |

## Exit codes

Successful `analyze` runs exit `0` for every verdict. Use `check` when the verdict must set the process status.

| Code | `check` meaning | Other command behavior |
| --- | --- | --- |
| `0` | `healthy` | Success for report-only and setup commands. `analyze` also exits `0` for `needs_attention` and `blocked`. |
| `1` | `blocked` | `config lint` found an error; `policy can-import` denied a target; `hook git` found an in-scope repair or owner decision; `agents-md --check` found a missing or stale block. |
| `2` | `needs_attention` | `policy can-import` found a target with no decision. The Claude hook uses `2` to block a stop under its protocol. |
| `3` | Config, usage, or runtime error | General command error. `hook git` uses `3` when archfit cannot run. |

`check` exits `2` when there is no blocker, but evidence or diagnostics need attention. `--require-tools` makes missing required tools affect the verdict. `analyze` stays report-only.

The Claude Code hook has protocol-specific behavior. It exits `1` for malformed input, `2` to block a stop, and `0` for other events. An archfit error fails open and returns a `systemMessage`.

## Shared flags

These flags apply only to commands that list them below.

| Flag | Meaning |
| --- | --- |
| `-c, --config <file>` | Config path. Default: `.archfit.yaml` on commands that accept it. |
| `-r, --root <dir>` | Source root on commands that accept this shorthand. `analyze` and `check` use `--root`. |
| `--refresh` | Re-run extractors and refresh the fact cache. Config AI commands refresh AI results. |
| `--progress <mode>` | Progress on stderr: `auto`, `plain`, or `none`. Available on analysis and map commands. |
| `-q, --quiet` | Suppress progress output. Available on analysis and map commands. |

## Analysis commands

### `analyze`

`archfit analyze [flags]` prints a report and exits `0` after a successful run.

| Flag | Meaning |
| --- | --- |
| `--root <dir>` | Source root. Default: directory of `--config`. |
| `--base <ref>` | Compare the working tree with a Git ref. Adds origin and state comparison data. |
| `--format <name>` | Repeatable: `text`, `json`, `markdown` (`md`), `sarif`, `scorecard`, or `agent`. Default: `text`. |
| `--json`, `--markdown`, `--sarif` | Shorthand for one matching `--format`. Do not combine shorthands with `--format`. |
| `--no-advisories` | Omit advisory findings. Omitted findings do not count as diagnostics. |
| `--min-severity <level>` | Filter advisories: `low`, `medium`, `high`, or `critical`. |
| `--lang <name>` | Force language detection. Repeatable: `go`, `typescript` (`ts`), `python` (`py`), `rust` (`rs`). |
| `--require-tools` | Treat missing required analyzers as failures in the rendered verdict. It does not change `analyze` exit status. |
| `--ai-summary` | Add an off-gate AI summary. Requires AI settings in the config. |

Examples:

```sh
archfit analyze -c .archfit.yaml
archfit analyze --format json --base origin/main -c .archfit.yaml
archfit analyze --format scorecard -c .archfit.yaml
```

### `check`

`archfit check [flags]` runs the gate and uses the verdict exit codes above. It accepts the same analysis flags as `analyze`, except `--ai-summary`. Its default text output is one linter-style record per blocker and a `PASS`, `FAIL`, or `UNKNOWN` footer (see [results](results.md)). Progress goes to stderr only with `--progress`. For the architecture report, with dimensions, seams, and coupling bands, run `analyze`.

```sh
archfit check -c .archfit.yaml
archfit check --format agent --base HEAD -c .archfit.yaml
```

| Flag | Meaning |
| --- | --- |
| `--base <ref>` | What the change is compared with. Default `auto`: the merge-base of `HEAD` and the target branch. On the target branch itself the whole tree is the change. Any other value is a git ref. |
| `--target <branch>` | The branch the change lands on. Default: `target_branch` from the config, then `origin/HEAD`, then `main` or `master` when exactly one exists. A bare name is looked up on `origin` first. |

`check` never falls back to `HEAD` when it cannot resolve the target or the base. It reports `change.status: unknown` with `reason: scope_unresolved` and exits `2`. Fetch the history (`fetch-depth: 0`) or pass `--target` or `--base`.

Before it measures, `check` compares `.archfit.yaml`, `.archfit-baseline.json`, and `.archfit-labels.yaml` with the target branch's tip. If any differs, the branch cannot grade itself under rules it wrote: the change is `unknown` with `reason: policy_changed_on_branch`, the exit code is `2`, and the result shows both the branch's findings under its own policy and the findings under the target's policy, which is the one the gate will use. If the branch did not touch the file and the target changed it after the fork, the change is `unknown` with `reason: policy_stale_on_branch`: merge or rebase the target. A policy directory outside the repository is not compared, and a target that holds no config has no policy to protect, so nothing is compared.

### `baseline`

`archfit baseline [flags]` accepts current findings as the gate baseline.

| Flag | Meaning |
| --- | --- |
| `--root <dir>` | Source root. Default: directory of `--config`. |
| `--no-advisories` | Exclude advisory findings from the baseline. |
| `--refresh` | Re-run extractors and refresh the cache. |
| `--reanchor` | Carry accepted debt into the current engine epoch without accepting new findings. |
| `--from <file>` | Baseline to re-anchor. Requires `--reanchor`. Default: baseline beside the config. |

### `explain <fingerprint-prefix>`

`archfit explain <fingerprint-prefix> [flags]` prints one finding and its evidence.

| Flag | Meaning |
| --- | --- |
| `-r, --root <dir>` | Source root. Default: directory of `--config`. |
| `--ai-summary` | Add an off-gate AI narrative. Requires AI settings. |
| `--refresh` | Re-run extractors and refresh the cache. |

### `policy where <path>...`

Reports the module, layer, owner, public surface, allowlists, and selected rules for each repo-relative path.

| Flag | Meaning |
| --- | --- |
| `-c, --config <file>` | Config path. |
| `--root <dir>` | Analysis root. Default: Git top-level, then config directory. |

### `policy can-import <file> <target>...`

Reports whether a source file can import each target. A denied target exits `1`. A target without a decision exits `2`.

| Flag | Meaning |
| --- | --- |
| `-c, --config <file>` | Config path. |
| `--root <dir>` | Analysis root. Default: Git top-level, then config directory. |

### `map`

`archfit map [flags]` writes a module and seam graph. The default format is Mermaid.

| Flag | Meaning |
| --- | --- |
| `--root <dir>` | Source root. |
| `--format <name>` | `mermaid` or `text`. |
| `--focus <module>` | Keep a module, its direct neighbors, and touching seams. Repeatable. |
| `--refresh` | Re-run extractors and refresh the cache. |

## Setup and config commands

### `doctor`

`archfit doctor [flags]` checks analyzer availability. `--fix` installs tools that archfit can install.

| Flag | Meaning |
| --- | --- |
| `--fix` | Install missing supported tools. `--install` is an alias. |
| `--lang <name>` | Limit installation to `go`, `ts`, `py`, or `rust`. Repeatable. Default: all. |
| `-n, --dry-run` | Print install commands without running them. Requires `--fix`. |

### `config init`

Creates a starter config. `--ai-classify` drafts AI classifications.

| Flag | Meaning |
| --- | --- |
| `-r, --root <dir>` | Project root. Default: `.`. |
| `-o, --output <file>` | Output path. Default: `.archfit.yaml`. Use `-` for stdout. |
| `--force` | Replace an existing config and keep a timestamped backup. |
| `--ai-classify` | Run an off-gate AI classification pass. |
| `--apply` | Write AI classifications directly. Requires `--ai-classify`. Review them before gating. |
| `--ai-provider <name>` | Provider override. See [AI providers](ai.md#configure-a-provider). |
| `--ai-model <name>` | Model override. Configured values precede provider defaults. See [override precedence](ai.md#initupdate-overrides). |
| `--refresh` | Re-run AI calls and refresh the AI cache. |

### `config update`

Reviews config structure against the source tree. `--apply` writes safe structural changes and creates a backup.

| Flag | Meaning |
| --- | --- |
| `-c, --config <file>` | Config path. |
| `-r, --root <dir>` | Project root. Default: config directory. |
| `--apply` | Write structural changes. AI proposals remain review-only. |
| `--json` | Emit `archfit.config-review.v1`. Report-only; cannot combine with `--apply`, `--ai-classify`, or `--refresh`. |
| `--ai-classify` | Draft AI classifications for unclassified modules. |
| `--ai-provider <name>` | Provider override. See [AI providers](ai.md#configure-a-provider). |
| `--ai-model <name>` | Model override. Configured values precede provider defaults. See [override precedence](ai.md#initupdate-overrides). |
| `--refresh` | Re-run AI calls and refresh the AI cache. |

### `config lint`

Checks config rules against the source tree. It exits `1` when it reports an error diagnostic.

| Flag | Meaning |
| --- | --- |
| `-c, --config <file>` | Config path. |
| `--root <dir>` | Project root. |
| `--json` | Emit `archfit.config-lint.v1`. |

### `config compare <candidate>`

Measures one source tree under the current and candidate configs. It is report-only and does not write config or baseline files.

| Flag | Meaning |
| --- | --- |
| `-c, --config <file>` | Current config. Default: `.archfit.yaml`. |
| `--root <dir>` | Source root. Default: current config directory. |
| `--json` | Emit `archfit.config-compare.v1`. |

The text report shows changed findings, edge counts, and classification mix. It has no repository score. JSON includes `score_delta`. See [results](results.md#config-comparison).

### `config enrich <kind>`

Drafts or applies AI annotations. Kinds are `labels`, `abstained`, `owner`, `volatility`, and `subdomain`.

| Flag | Meaning |
| --- | --- |
| `-c, --config <file>` | Config path. |
| `-r, --root <dir>` | Project root. Default: config directory. |
| `--refresh` | Re-run AI calls and refresh the AI cache. |
| `--apply` | Apply reviewed entries for `owner`, `volatility`, or `subdomain`. |
| `--reviewed-by <name>` | Record the reviewer when applying entries. |

`labels` and `abstained` write drafts to `.archfit-labels.yaml`. `owner`, `volatility`, and `subdomain` write kind-specific draft files and support `--apply`. Run `archfit config enrich <kind> --help` for kind-specific flags and paths.

## Integration commands

### `hook claude` and `hook git`

`hook claude` implements the Claude Code Stop and SubagentStop protocol. `hook git` checks staged content. Both stop a change only when `change.status` is `fail`.

| Flag | Meaning |
| --- | --- |
| `-c, --config <file>` | Config path, relative to the hook working directory. |
| `--base <ref>` | Scope blockers against a Git ref. Default: `HEAD`. |

`hook git` also accepts hidden `--index-file` for Git hook integration. `GIT_INDEX_FILE` supplies its default.

### `agents-md`

Renders the archfit instruction block in `AGENTS.md` or another file.

| Flag | Meaning |
| --- | --- |
| `-c, --config <file>` | Config path. |
| `--file <path>` | Instruction file. Default: `AGENTS.md`. |
| `--write` | Replace or append the marked block. |
| `--check` | Exit `1` if the block is absent or stale. |

### `skill install`

Installs the agent skill bundled with this binary.

| Flag | Meaning |
| --- | --- |
| `--dir <path>` | Skills directory. Default: `.claude/skills`. |
| `--force` | Replace files that differ from the bundled copy. |

## Version and help

Every command accepts `-h, --help`. The top-level command accepts `-v, --version`.

See [configuration](configuration.md) for config fields, [results](results.md) for output contracts, and [CI](ci.md) for gate setup.
