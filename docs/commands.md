# Commands

Archfit has seven public commands. Run `archfit <command> --help` for their flags. Bare `archfit` prints a usage error; it runs no check or report.

## Policy version

`check`, `can`, `rules` and `request` need a version 3 `.archfit.yaml`. In an Archfit 3 repository, run `archfit init` to convert it before checking a change. See [Migration](migration.md). `report` can still read a version 2 policy for its measurements only.

## Choose a command

| Task | Command |
| --- | --- |
| Check a change: pass, fail, or unknown | `archfit check` |
| Ask whether a dependency or library may be used | `archfit can <from> <to>` |
| Show the rules that apply | `archfit rules [PATH... \| RULE] [--graph]` |
| Ask an owner to change the policy | `archfit request -m "<why>" [--expires DATE]` |
| Set up a repository, or convert an Archfit 3 one | `archfit init` |
| See the state of the repository and its measurements | `archfit report` |
| Check tools and hooks, or install what is missing | `archfit doctor` or `archfit doctor --fix` |

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | `pass`, yes, or done. `report` exits 0 whatever the repository state shows. A sent `request` prints its comment or issue URL. |
| `1` | `fail` or no. |
| `2` | `unknown`, wrong usage, a tool error, or a run interrupted by `SIGINT` or `SIGTERM`. A request was not sent, or its send outcome is unknown. |

Unknown commands and flags also exit `2`. Diagnostics go to stderr; normal output goes to stdout. The hidden hooks use their harness's protocol, described below.

## Command details

### `check`

`archfit check [flags]` gives the status of the change in the working tree: `pass`, `fail`, or `unknown`. It needs a version 3 policy. If the policy is missing or version 2, run `archfit init` (see [migration](migration.md)).

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

The default text record is also the agent workflow: run `archfit check` without a format flag and follow `next:`. Exit `0` means `pass`, `1` means `fail`, and `2` means `unknown`, wrong usage, or a tool error.

The record goes to stdout. Notes, such as a target branch with no policy, and tool errors go to stderr. When no workflow in `.github/workflows/` has a step that uses `alexei-led/archfit-action` (`uses: alexei-led/archfit-action@<ref>`), every result carries the line `policy: not protected`, after the status line: the result is a local guide, not change control.

Each run except a `--policy` preview keeps its JSON output and the git tree id of the working tree in `.archfit-cache/last.json`. `archfit request` reads it while the tree and `HEAD` are the same. The same tree, policy, engine version, and date give the same output.

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

With a GitHub remote, `archfit request` posts `/archfit request <why> [--expires DATE]` as a comment on the one open pull request for the current branch. The target repository is `origin`, or a configured GitHub `upstream` for a fork. The head uses the push remote's owner, falling back to the `origin` owner. Several matching pull requests means nothing is sent: the error names their numbers. A detached HEAD or a branch without an upstream has no matching pull request.

Without a pull request, only a request from `archfit can` opens an issue. Its body holds the request line and the `can` record; the App marks the numbers "as reported" until a pull request confirms them. With `check` output, nothing is opened: `next: open a pull request first`.

Credentials are tried in order: `GH_TOKEN`, `GITHUB_TOKEN`, then `gh auth token`. No credentials means exit `2` and `next: gh auth login`. Tokens are never printed. Success exits `0` and prints the comment or issue URL. API errors exit `2` without printing a successful request or changing the last result. HTTP calls time out after 30 seconds. A POST is never retried; when the send outcome is unknown (for example, a lost connection after sending), check GitHub before trying again.

Without a GitHub remote, the request is printed and exits `2`; show it to an owner. The last line is `next: ask an owner`:

```text
$ archfit request -m "netem needs the shared retry helper"
pkg/tc/netem.go:12: graph: pkg_tc -> pkg_util
  balance: 9/10 (min 7)
request: netem needs the shared retry helper
next: ask an owner     (open a pull request first if there is none)
```

| Flag                    | Meaning                                                                                                                                                                                                                       |
| ----------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `-m <why>`              | The reason. Required. On a terminal, `archfit request` asks for it when `-m` is not given. Use one line of at most 500 characters. The reason must not end in `--expires` or `--expires <word>`: give the date with the flag. |
| `--expires <date>`      | Ask for an exception until this day, `YYYY-MM-DD`: a date after today and after the `check`, at most 365 days ahead. Without it the request asks for a permanent change.                                                      |
| `-C, --directory <dir>` | Run in `<dir>` instead of the current directory.                                                                                                                                                                              |

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

On a version 2 policy the report shows the measurements only, says to run `archfit init`, and has no `policy:` line. On a version 3 policy, the report does not yet place Python code in its modules, so the coupling measurements of a Python repository are `n/a`.

`--format json` writes the `archfit.architecture-state.v1` document of `archfit.report.schema.json`. On a version 3 policy, its `repository` object holds the state and each list above.

| Flag | Meaning |
| --- | --- |
| `--format <name>` | `text` (default) or `json`. |
| `-C, --directory <dir>` | Run in `<dir>` instead of the current directory. |

### `doctor`

`archfit doctor [flags]` checks this checkout: the tools archfit uses and the hooks of the agent loop. `--fix` installs the hooks.

The tools come in two tables:

| Group | Tools | Without the tool |
| --- | --- | --- |
| Needed for the check | `git` for every language. `ast-grep` for TypeScript, JavaScript and Python imports, and for the literals of every language but Go. `cargo` for Rust. `dotnet` for C#. | The check is `unknown` for the code it cannot read. |
| Optional, for the report | `go`, `cargo-modules`, `archfit-roslyn` | `report` measures less. The check never runs these tools. |

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

## Internal hooks

`archfit doctor --fix` installs these entry points. They are not public setup commands.

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

## Version and help

Every command accepts `-h, --help`. The top-level command accepts `-v, --version`.

See [configuration](configuration.md) for config fields, [results](results.md) for output contracts, and [CI](ci.md) for gate setup.
