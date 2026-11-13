# Commands

This page lists the `archfit` commands and flags. It does not repeat the full configuration or output contracts.

Run `archfit <command> --help` for built-in help. Bare `archfit` runs `analyze`.

## Policy version

`check`, `can`, `rules`, `request`, `init` and `report` are the Archfit 4 commands. `can`, `rules` and `request` need a version 3 `.archfit.yaml`; `report` gives the state of the repository only on a version 3 policy. `check` reads the version first: a version 3 policy runs the Archfit 4 check, and a version 2 policy (Archfit 3) runs the Archfit 3 check. In a repository with a version 2 policy, run `archfit init` to convert it to version 3 (see [migration](migration.md)).

## Choose a command

| Task | Command |
| --- | --- |
| Review architecture locally | `archfit analyze` |
| Check a change: pass, fail, or unknown | `archfit check` |
| Ask whether a dependency or library may be used | `archfit can <from> <to>` |
| Show the rules that apply | `archfit rules [PATH... \| RULE] [--graph]` |
| Ask an owner to change the policy | `archfit request -m "<why>" [--expires DATE]` |
| Set up a repository, or convert an Archfit 3 one | `archfit init` |
| See the state of the repository and its measurements | `archfit report` |
| Accept current findings as the gate baseline | `archfit baseline` |
| Explain one finding | `archfit explain <fingerprint-prefix>` |
| Ask which module owns a path | `archfit policy where <path>` |
| Ask whether a file can import a target | `archfit policy can-import <file> <target>` |
| Draw modules and seams | `archfit map` |
| Check the tools and hooks of this checkout, or install the hooks | `archfit doctor` or `archfit doctor --fix` |
| Create or review the config | `archfit config init` or `archfit config update` |
| Compare two configs on one source tree | `archfit config compare <candidate>` |
| Check config defects | `archfit config lint` |
| Render the instruction block | `archfit agents-md --write` |
| Install the bundled agent skill | `archfit skill install` |

## Exit codes

`check` with a version 3 policy, `can`, `rules`, `request`, `init`, and `report` use these codes:

| Code | `check` | `can` | `rules`, `init`, `report` | `request` |
| --- | --- | --- | --- | --- |
| `0` | `pass` | yes | done | sent (with the App, milestone 2) |
| `1` | `fail` | no | – | – |
| `2` | `unknown`, wrong usage, or a tool error | `unknown`, wrong usage, or a tool error | wrong usage or a tool error | not sent: no App, no `check` output for this tree, nothing to ask, wrong usage, or a tool error |

A flag or argument that the command line parser rejects, such as an unknown flag, still exits `3`, and so does a run stopped by `SIGINT` or `SIGTERM`. These two move to `2` when the Archfit 3 commands are removed.

The other commands, and `check` with no policy file or a version 2 policy, use the codes below. Successful `analyze` runs exit `0` for every verdict. Use `check` when the verdict must set the process status.

| Code | `check` meaning (version 2 policy) | Other command behavior |
| --- | --- | --- |
| `0` | `healthy` | Success for report-only and setup commands. `analyze` also exits `0` for `needs_attention` and `blocked`. |
| `1` | `blocked` | `config lint` found an error; `policy can-import` denied a target; `agents-md --check` found a missing or stale block. |
| `2` | `needs_attention` | `policy can-import` found a target with no decision. |
| `3` | Config, usage, or runtime error | General command error. A run stopped by `SIGINT` or `SIGTERM` also exits `3`: archfit stops its tools, removes its temporary copies, and reports the results as incomplete. A second signal exits at once. |

`check` exits `2` when there is no blocker, but evidence or diagnostics need attention. `--require-tools` makes missing required tools affect the verdict. `analyze` stays report-only.

The hooks (`hook run`) follow the protocol of their host, not the table above. The agent hooks (`session-start`, `post-tool-use`, `stop`) exit `2` to give the agent feedback (a `no` from `can`, or a `fail` that stops the agent), `1` for malformed input, and `0` otherwise. `pre-commit` exits `1` on a `fail`, which stops the commit, and `0` otherwise. A hook that fails for its own reasons shows why and exits `0`.

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
| `--lang <name>` | Force language detection. Repeatable: `go`, `typescript` (`ts`), `python` (`py`), `rust` (`rs`), `dotnet` (`csharp`). |
| `--require-tools` | Treat missing required analyzers as failures in the rendered verdict. It does not change `analyze` exit status. |
| `--ai-summary` | Add an off-gate AI summary. Requires AI settings in the config. |

Examples:

```sh
archfit analyze -c .archfit.yaml
archfit analyze --format json --base origin/main -c .archfit.yaml
archfit analyze --format scorecard -c .archfit.yaml
```

### `check`

`archfit check [flags]` gives the status of the change in the working tree: `pass`, `fail`, or `unknown`. It reads the version of `.archfit.yaml` first. A version 3 policy, or `--policy`, runs the check below. No policy file, or a version 2 policy (Archfit 3), runs the Archfit 3 check of [the next section](#check-with-a-version-2-policy) until you convert the repository (see [migration](migration.md)).

```sh
archfit check
archfit check --base origin/main
archfit check --policy candidate.yaml
archfit check --format sarif > archfit.sarif
```

| Flag | Meaning |
| --- | --- |
| `--base <ref>` | The target branch. Default: the remote's default branch, `origin/HEAD`. |
| `--policy <file>` | Judge the change with this policy file instead of `.archfit.yaml`, and do not compare it with the policy of the target branch. A preview: nothing is kept for `archfit request`. A relative path is read from the directory of `-C`, else from the current directory. |
| `--format <name>` | `text` (default), `json`, or `sarif`. |
| `--verbose` | Add the policy digest and the engine version after the status line. |
| `-C, --directory <dir>` | Run in `<dir>` instead of the current directory. |

The run:

1. The target branch is `--base`, else `origin/HEAD`. With neither, the status is `unknown` and the record says to pass `--base`. A CI checkout often has no `origin/HEAD`: see [CI](ci.md).
2. The base is the merge base of `HEAD` and the target branch. When `HEAD` is the tip of the target branch, the base is the first parent of `HEAD`: the change is the last commit. A clone without the merge base, such as a shallow clone, is `unknown`, and the record names the fetch.
3. The policy on the target branch is the protected one. When `.archfit.yaml` differs from it, the status is `unknown`, the record names the command that restores the file, and nothing else is checked. When the target branch has no `.archfit.yaml`, or a version 2 one (the pull request that converts the repository), the check uses the local policy and says so on stderr.
4. The policy must fit the tree. A path glob that matches no file, or another policy error, is `unknown` with the message.
5. The check reads the base and the working tree and counts the errors of each by rule, modules, and symbol. An error the base does not have, or has fewer times, is added by the change: the status is `fail`. An expired exception fails every change, and the status line ends with `(not from this change)`. See [results](results.md).
6. On a push (`HEAD` is the tip of the target branch) with a `pass` or `fail` status, the JSON result adds the state of the repository. See [results](results.md#json).

The record goes to stdout. Notes, such as a target branch with no policy, and tool errors go to stderr. When no workflow in `.github/workflows/` has a step that uses `alexei-led/archfit-action` (`uses: alexei-led/archfit-action@<ref>`), every result carries the line `policy: not protected`, after the status line: the result is a local guide, not change control.

Each run except a `--policy` preview keeps its JSON output and the git tree id of the working tree in `.archfit-cache/last.json`. `archfit request` reads it while the tree and `HEAD` are the same. The same tree, policy, engine version, and date give the same output.

### `check` with a version 2 policy

With no policy file or a version 2 policy, `check` runs the Archfit 3 check and uses the version 2 exit codes above. It accepts the same analysis flags as `analyze`, except `--ai-summary`; `check --help` does not list them. Its default text output is one linter-style record per blocker and a `PASS`, `FAIL`, or `UNKNOWN` footer (see [results](results.md)). Progress goes to stderr only with `--progress`. For the architecture report, with dimensions, seams, and coupling bands, run `analyze`.

```sh
archfit check -c .archfit.yaml
archfit check --format agent --base HEAD -c .archfit.yaml
```

| Flag | Meaning |
| --- | --- |
| `--base <ref>` | What the change is compared with. Default `auto`: the merge-base of `HEAD` and the target branch. On the target branch itself the whole tree is the change. Any other value is a git ref. |
| `--target <branch>` | The branch the change lands on. Default: the repository's default branch (`origin/HEAD`, else `main` or `master` when exactly one exists), or the `target_branch` that branch's own `.archfit.yaml` declares. A name is looked up on `origin` first. |

`check` never falls back to `HEAD` when it cannot resolve the target or the base. It reports `change.status: unknown` with `reason: scope_unresolved` and exits `2`. Fetch the history (`fetch-depth: 0`) or pass `--target` or `--base`.

Before it measures, `check` compares `.archfit.yaml`, `.archfit-baseline.json`, and `.archfit-labels.yaml` with the target branch's tip. If any differs, the branch cannot grade itself under rules it wrote: the change is `unknown` with `reason: policy_changed_on_branch`, the exit code is `2`, and the result shows both the branch's findings under its own policy and the findings under the target's policy, which is the one the gate will use. If the branch did not touch the file and the target changed it after the fork, the change is `unknown` with `reason: policy_stale_on_branch`: merge or rebase the target. A policy directory outside the repository is not compared, and a target that holds no config has no policy to protect, so nothing is compared.

### `can <from> <to>`

`archfit can <from> <to> [flags]` answers before you write code: may `<from>` depend on `<to>`? It reads `.archfit.yaml` (version 3) and applies the rules `check` applies to one new dependency: `graph`, `coupling`, `layers`, and the custom `deny` and `only` rules.

```text
$ archfit can pkg/tc pkg/util
pkg_tc -> pkg_util: graph: not an allowed dependency
  balance: 9/10 (min 7)
  next: archfit request -m "<why you need it>"
```

Each side is a module name, a file or a directory relative to the repository root, or a Go import path that a `go.mod` of the checkout maps. `<to>` can also be a library, such as `github.com/spf13/cobra`. A module name as `<to>` means the public part of the module. A path that does not exist yet is judged by the module its glob gives.

| Answer | Exit | The record shows |
| --- | --- | --- |
| yes | `0` | The balance, when the two sides are different modules. |
| no | `1` | The rule, its `why` for a custom rule, `instead`, the balance, and `next`. |
| unknown | `2` | A path in no module, or a question only a file can answer, and the command to run next. |

A directory is judged one file at a time, as `check` judges it. When its files answer differently, the answer is `unknown`.

The answer is approximate in two cases. It applies no exception, and it does not see a cycle that the new dependency would close. `check` judges both: a `yes` can still fail `check` when the dependency closes a cycle, and a `no` can pass `check` when an exception that has not expired covers it.

`can` keeps its JSON answer in `.archfit-cache/last.json`, as `check` does, for `archfit request`.

| Flag | Meaning |
| --- | --- |
| `--format <name>` | `text` (default) or `json`. |
| `--verbose` | Add the policy digest and the engine version. |
| `-C, --directory <dir>` | Run in `<dir>` instead of the current directory. |

### `rules [PATH... | RULE]`

`archfit rules` shows the rules of `.archfit.yaml` (version 3) in plain words. Paths are relative to the repository root.

| Form | Shows |
| --- | --- |
| `archfit rules` | The paths form for the files the change adds lines to, against the base `check` uses. |
| `archfit rules <path>...` | For each path: its module, the public part, volatility, layer, and allowed dependencies, then the rules that apply with their modes and the exceptions that apply with their dates. A path in no module, a declared test, or a generated path says so. |
| `archfit rules <rule-id>` | One rule in full, with its exceptions. Built-in rule ids work too. |
| `archfit rules --graph` | The modules by layer, inner first, with their allowed dependencies. |

The notes of the policy are listed as "not checked". Dates show the days left, such as `2026-11-19 (9 days)`.

| Flag | Meaning |
| --- | --- |
| `--graph` | The module graph. It takes no path or rule id. |
| `-C, --directory <dir>` | Run in `<dir>` instead of the current directory. |

### `request`

`archfit request -m "<why>" [--expires DATE] [flags]` asks an owner to change the policy. It takes the errors of the last `archfit check`, or the dependency of the last `archfit can` answer, from `.archfit-cache/last.json`, so you write only the reason. The output is used only while `HEAD` and the working tree are the ones it was produced on: after an edit, run `archfit check` again.

Only an error whose `next` is `archfit request` joins the request: an error of a rule an owner may allow. An error that says `fix the code`, or `wait for an owner: #n`, does not.

Without the App, `archfit request` prints the request it built and exits `2`. The last line is `next: ask an owner`. A request built from `check` output adds a note: the App joins it to a pull request, so open one first if there is none. A request built from a `can` answer comes before a pull request.

```text
$ archfit request -m "netem needs the shared retry helper"
pkg/tc/netem.go:12: graph: pkg_tc -> pkg_util
  balance: 9/10 (min 7)
request: netem needs the shared retry helper
next: ask an owner     (open a pull request first if there is none)
```

| Flag | Meaning |
| --- | --- |
| `-m <why>` | The reason. Required. On a terminal, `archfit request` asks for it when `-m` is not given. The reason must not end in `--expires` or `--expires <word>`: give the date with the flag. |
| `--expires <date>` | Ask for an exception until this day, `YYYY-MM-DD`: a date after today and after the `check`, at most 365 days ahead. Without it the request asks for a permanent change. |
| `-C, --directory <dir>` | Run in `<dir>` instead of the current directory. |

`archfit request` exits `2` with the reason on stderr when there is no `check` or `can` output for this tree (`run archfit check first`), when the output is `pass` or has nothing an owner may allow (`nothing to ask`), when it is `unknown`, or when `-m` or `--expires` is wrong.

### `init`

`archfit init [flags]` sets up a repository. It writes what is missing and never changes a version 3 policy. It writes no workflow file: the workflow comes with Action 4.

| The repository has | `archfit init` |
| --- | --- |
| No `.archfit.yaml` | Writes a version 3 policy from the code: a module for each directory of code cut to two levels, such as `pkg/chaos` (for Python, below `src/`), with `paths` and `public` (the packages other modules use today); `depends_on` as the dependencies that exist; `tests` and `generated`; `owners` from the GitHub `origin` remote; an `only` rule for network, process, or database code that only some modules use; and an exception for each error that exists today, so the first `check` passes. |
| A version 2 `.archfit.yaml` (Archfit 3) | Converts it, with `.archfit-baseline.json`, `.archfit-labels.yaml` and `.archfit-app.yaml`, into one version 3 policy with `tests` and `generated` as for a new policy, removes the old files, and lists what it could not convert. See [migration](migration.md). |
| A version 3 `.archfit.yaml` | Leaves it as it is. |

In each case it writes the five lines for agents between the `<!-- archfit:start -->` and `<!-- archfit:end -->` markers in `AGENTS.md`, and creates the file when it is missing. Text outside the markers stays as it is. When the block is already there, nothing is written and the command still exits `0`.

```text
$ archfit init
wrote .archfit.yaml: 4 modules, 0 allowed dependencies, 0 rules, 1 exception
  exception: duplication pkg_tc -> pkg_util
wrote AGENTS.md: the instruction block for agents
next: read .archfit.yaml, then open a pull request with it
```

Read the policy and its exceptions before you open the pull request. A place in the code that `init` could not read is listed with `not read:` and its fix. A missing tool, unbalanced markers in `AGENTS.md`, or no GitHub `origin` remote stops `init` before it writes anything, with exit `2`.

| Flag | Meaning |
| --- | --- |
| `-C, --directory <dir>` | Run in `<dir>` instead of the current directory. |

### `report`

`archfit report [flags]` gives owners the repository-wide picture. It decides nothing: it exits `0` whatever it shows, and `2` only on wrong usage or a tool error. It never runs on the path of `check`.

On a version 3 policy, it first judges the whole working tree with the rules and exceptions of `.archfit.yaml` on today's date. The state is one of:

| State | Meaning |
| --- | --- |
| `clean` | No error outside the exceptions. |
| `drift` | An error exists that no exception covers. The report lists each one. |
| `expired` | An exception passed its date and the code still breaks the rule. Every change fails until an owner extends the exception or the code is fixed. |

Beside the state, the report lists the places the check could not read (the state does not cover them), the exceptions within 14 days of their date, the rules nothing checks, and the balance of each allowed dependency. Then come the measurements of the code. A tool whose evidence is partial or timed out, or that the code needs and is not installed, is listed first under `evidence not complete:`: a measurement it feeds can show a zero that was not measured. A policy that does not fit the tree, such as a path glob that matches no file, stops the report with exit `2` before the measurements.

```text
$ archfit report
repository: drift, 1 error no exception covers
pkg/tc/netem.go:12: graph: pkg_tc -> pkg_util is not an allowed dependency
  why: a new dependency between modules needs an owner
  balance: 9/10 (min 7)
exceptions close to their date:
  duplication pkg_tc -> pkg_util: 2026-11-19 (9 days)
not checked:
  keep-docs: Each module has a README.
allowed dependencies:
  pkg_chaos -> pkg_runtime: 9/10 (min 7)
measurements:
  cycle: 0 import cycles (strong)
policy: not protected
```

On a version 2 policy the report shows the measurements only, says to run `archfit init`, and has no `policy:` line, as the version 2 `check` has none. On a version 3 policy, the report does not yet place Python code in its modules, so the coupling measurements of a Python repository are `n/a`.

`--format json` writes the `archfit.architecture-state.v1` document of `archfit.state.schema.json`. On a version 3 policy, its `repository` object holds the state and each list above.

| Flag | Meaning |
| --- | --- |
| `--format <name>` | `text` (default) or `json`. |
| `-C, --directory <dir>` | Run in `<dir>` instead of the current directory. |

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

Reports whether a source file can import each target. A denied target exits `1`. A target without a decision exits `2`. A denial also carries `instead` (the alternatives the rule's owners declared) and `candidates`: each alternative that names a package a module owns, judged again from the same file.

The answer runs no analyzer. It spells the edge the language's extractor would emit and runs the rule pass on that edge. Write the target in the language's graph vocabulary:

| Language | Target |
| --- | --- |
| Go | A package directory or an import path. |
| TypeScript | A source file. |
| Python | A dotted module or a `.py` file inside the packages grimp builds. |
| .NET | A `.cs` file or an external assembly name, such as `Microsoft.EntityFrameworkCore`. |
| Rust | Not decided: crate roots need `cargo metadata`. |

A Go file the go command never reads is `unconstrained` with the reason: a `_test.go` file, a file under `testdata`, `vendor`, or a directory that starts with `.` or `_`, a file the build constraints exclude, a file of a module the run does not load, and an excluded file. With `CGO_ENABLED` unset, a Go file that only one cgo setting reads is not decided.

A rule that needs the whole graph, such as `module_cycle`, leaves a target not decided unless another rule denies it. It outranks an allowlist or the layer order: those permit the pair, and only the whole graph can say whether the pair closes a cycle. `can-import` runs no analyzer, so it cannot answer that.

.NET answers are not decided when `analyzers.roslyn.enabled` is `false` (check then reads project edges), when the file is in no selected project's directory, and when the target names a first-party assembly or a project computes its `AssemblyName`. A file that check never reads is `unconstrained` with the reason: an excluded file, a file under `obj/` or `bin/`, or a file of a project with a `test` or `tests` path segment. A test project marked only by `IsTestProject` needs MSBuild evaluation, so its files are judged like production files.

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

`archfit doctor [flags]` checks this checkout: the tools archfit uses and the hooks of the agent loop. `--fix` installs the hooks.

The tools come in two tables:

| Group | Tools | Without the tool |
| --- | --- | --- |
| Needed for the check | `git` for every language. `ast-grep` for TypeScript, JavaScript and Python imports, and for the literals of every language but Go. `cargo` for Rust. `dotnet` for C#. | The check is `unknown` for the code it cannot read. |
| Optional, for the report | `go`, `scip-go`, `node`, `depcruise`, `scip-typescript`, `uv`, `python3`, `scip-python`, `rust-analyzer`, `cargo-modules`, `archfit-roslyn`, `jscpd` | `report` measures less. The check never runs these tools. |

A Go repository needs only `git`. Each row shows the tool, its status (`ok`, `missing`, or `invalid`), the languages that use it, and its path or install hint. For .NET, doctor prints the SDK and helper versions that the current directory's `global.json` selects, and fails when that pin cannot be satisfied.

After the tools and the LLM settings, doctor shows the hooks. For each harness (Claude Code, Codex, git), it shows whether the harness is here and whether each archfit hook runs: `active`, or `inactive` with the reason. Reasons are: not installed; for Claude Code, `disableAllHooks` in `.claude/settings.json` or `.claude/settings.local.json`; for Codex, not trusted yet. Codex keeps hook trust in `~/.codex/config.toml` (`CODEX_HOME`). doctor cannot recompute the trusted hash, so a trusted Codex hook is `active (hash not verified)`. When git ignores the settings file of a harness it found, doctor warns: other checkouts and worktrees do not get its hooks. A tracked settings file reaches every clone, so it gets no warning. Then doctor shows the target branch, the scope of the check, and whether a policy file differs from the target.

`doctor --fix` writes the hooks of each harness it finds, at the root of the repository:

| Harness | Found when | File | Events |
| --- | --- | --- | --- |
| Claude Code | `.claude/` is in the repository, or `claude` is on `PATH` | `.claude/settings.json` | `SessionStart`, `PostToolUse` (after `Edit`, `Write`, `MultiEdit`, `NotebookEdit`, `Bash`), `Stop`, `SubagentStop` |
| Codex | `codex` is on `PATH`, or `CODEX_HOME` (default `~/.codex`) exists | `.codex/hooks.json` | `SessionStart`, `PostToolUse` (after `apply_patch`, `Bash`), `Stop` |
| git | the directory is in a git repository | `pre-commit` in the hooks directory git uses (`core.hooksPath` when set) | the pre-commit hook |

It keeps every other setting and hook in a settings file and replaces its own earlier entries, so a second run changes nothing. It never replaces a pre-commit hook that archfit did not write, and never writes into a hooks directory outside the repository, such as a global `core.hooksPath`: it prints a warning with the line to add to that hook. The hooks read `.archfit.yaml` at the repository root; doctor warns when it does not exist. Codex runs the hooks only after you trust them. Outside a git repository, `--fix` writes no hook and says so. After the hooks, `--fix` installs the analyzer toolchains that archfit can install. A failed toolchain install keeps the hooks and makes doctor exit non-zero.

| Flag | Meaning |
| --- | --- |
| `--fix` | Install the hooks of the harnesses found here, and the missing toolchains archfit can install. `--install` is an alias. |
| `--lang <name>` | Limit the toolchain install to `go`, `ts`, `py`, or `rust`. Repeatable. Default: all. |
| `-n, --dry-run` | With `--fix`: print the install commands and the hook files, and change nothing. |
| `--format <name>` | `text` (default) or `json`. `json` prints the tool rows and the hooks as `archfit.doctor.v1`. Each tool row has `group` (`check` or `report`), `languages` (absent: every language), and `status` (`ok`, `missing`, or `invalid` with its `error`). A harness whose settings file git ignores has `ignored: true`. `json` cannot be combined with `--fix`. An `invalid` tool fails `doctor` in both formats, after the full report. |

### `config init`

Creates a starter config for a version 2 policy (Archfit 3). For a version 3 policy, run [`archfit init`](#init). `--ai-classify` drafts AI classifications.

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
| `--strict` | Report a blocking rule without a `rationale` as an error. Without it, `missing_rationale` is a warning. |

### `config compare <candidate>`

Measures one source tree under the current and candidate configs. It is report-only and does not write config or baseline files.

| Flag | Meaning |
| --- | --- |
| `-c, --config <file>` | Current config. Default: `.archfit.yaml`. |
| `--root <dir>` | Source root. Default: current config directory. |
| `--json` | Emit `archfit.config-compare.v1`. |

The text report shows changed findings, edge counts, and classification mix. It has no repository score. JSON includes `score_delta`. See [results](results.md#config-comparison).

Both forms also classify the policy change from the two configs' declarations, as they take effect, with no measurement involved. JSON `policy_change` has `direction`: `none`, `tightening`, `loosening`, or `mixed`. A loosening always needs a human approver. Only a difference the classifier can order counts as a tightening; every other difference counts as loosening.

- Loosening lists: `rules_removed`; `rules_loosened` (`gate` lowered, `max` raised, `declaration` for any other change of what a rule checks, such as a selector, patterns, or `guard`; for a metric as `metric/<name>`: a lowered gate, a larger `max_new` or `min_delta`; a metric with no entry ratchets at `fail`); `waivers_added`; `allowlists_widened` (`depends_on` or `visible_to`: `any` is an absent key, `none` an empty list); `coverage_lost` (`exclude_added` on the effective exclusions, defaults and re-includes applied; `language_disabled`; `language_mode_lowered` such as `on` to `auto`; `analyzer_disabled`, where only `on` runs an optional analyzer; `module_removed`; `module_paths_removed`; `module_review_gate_lowered`, where an absent block or no `stale_after` with no gate means the pass is off); `declarations_changed` (any other difference: a module added or given a new path, a module's other fields as `modules[<name>]`, and every other configuration key except `ai`, `outputs` and `version`). The last list is built by subtraction, so a configuration key added later is compared from the day it exists.
- Tightening lists: `rules_added`, `rules_tightened`, `waivers_removed`, `allowlists_narrowed`, `coverage_gained`. A rule's `rationale`, `alternatives` and `docs` never change a verdict and are not compared.

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

### Hooks

`archfit doctor --fix` installs the hooks. The `archfit hook` command is internal: it is not in the command list, and the installed hooks call `archfit hook run <event>`. See [coding agent workflows](agents.md#hooks).

### `hook run <event>`

`archfit hook run <event> [flags]` is what the installed hooks call. Events are `session-start`, `post-tool-use`, `stop`, and `pre-commit`. Every hook reads `.archfit.yaml` at the repository root, as `check` does. The first three read the hook event (`session_id`, `cwd`, `hook_event_name`, `stop_hook_active`) on stdin and keep the session in `.archfit-cache/session/<session_id>.json`.

| Event | Runs | Stops when |
| --- | --- | --- |
| `session-start` | Records `HEAD` as the session base and prints the five lines that `init` writes for agents. | never |
| `post-tool-use` | `can` on each import on a line added since the session base, when the working tree changed, with the errors that exceptions cover left out. It stores the answer of the first `no` for `archfit request`. | `can` answers `no`, and the session has not shown it yet. |
| `stop` | `check` | The change fails, with errors other than the set it stopped on last. A pass or an `unknown` forgets that set. |
| `pre-commit` | `check` on the staged content against `HEAD` | The commit adds an error, or an exception is past its date. |

| Flag | Meaning |
| --- | --- |
| `--harness <name>` | `claude` (default) or `codex`. Both use the same protocol. |

`pre-commit` also accepts hidden `--index-file` for Git hook integration. `GIT_INDEX_FILE` supplies its default. `archfit hook claude` and `archfit hook git` are removed, and so is `-c`: run `archfit doctor --fix` again to rewrite installed hooks.

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
