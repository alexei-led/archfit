# Getting started

This guide installs archfit, sets up a repository with `archfit init`, and shows the daily loop: `rules`, `can`, `check`, and `request`. For full tool requirements, see [Language support](languages.md).

## Install archfit

On macOS or Linux, install the latest release with Homebrew:

```sh
brew install alexei-led/tap/archfit
```

With Go, install a tagged release:

```sh
go install github.com/alexei-led/archfit/v3/cmd/archfit@v3.2.0
```

You can also download a release binary from the [release page](https://github.com/alexei-led/archfit/releases). Check the file against `SHA256SUMS` before you install it.

## Check the tools

Run:

```sh
archfit doctor
```

This command reports the tools that archfit can find, in two groups: needed for the check, and optional, for the report. The check needs `git`, and `ast-grep` for languages other than Go. Install the tools of the first group for the languages in your repository. A missing tool makes the check `unknown` for the code it cannot read. See [Language support](languages.md#analyzer-tools).

For an agent loop, run `archfit doctor --fix` once in each checkout. It installs the hooks of Claude Code, Codex and git. See [hooks](agents.md#hooks).

## Set up the repository

From your repository, run:

```sh
archfit init
```

`init` writes the policy, `.archfit.yaml`, from the code: the modules, the part of each module that other modules use today (`public`), the dependencies between modules that exist (`depends_on`), the test and generated paths, and the owners from the GitHub `origin` remote. Each error that exists today becomes an exception, so the first check passes. `init` also writes five lines for agents between markers in `AGENTS.md`.

```text
$ archfit init
wrote .archfit.yaml: 4 modules, 0 allowed dependencies, 0 rules, 1 exception
  exception: duplication pkg_tc -> pkg_util
wrote AGENTS.md: the instruction block for agents
next: read .archfit.yaml, then open a pull request with it
```

Read the policy, then open one pull request with it. An owner removes an exception when the code is fixed, and writes rules with `why` and `instead`. Read [Configuration](configuration.md) for the fields and [Rules](rules.md) for the built-in rules, custom rules, and exceptions.

`init` never changes a version 3 policy. In a repository with an Archfit 3 policy (`.archfit.yaml` version 2), it converts the old files instead: it replaces the policy and removes the other Archfit 3 files. See [Migrate from Archfit 3 to 4](migration.md).

## Work on a change

1. Before you start, show the rules for the files you change:

   ```sh
   archfit rules
   archfit rules pkg/tc
   ```

2. Before a new dependency between modules, ask:

   ```text
   $ archfit can pkg/tc pkg/util
   pkg_tc -> pkg_util: graph: not an allowed dependency
     balance: 9/10 (min 7)
     next: archfit request -m "<why you need it>"
   ```

3. Before you finish, check the change against the target branch:

   ```sh
   archfit check
   ```

   The status line comes after the errors: `archfit: pass`, `archfit: fail, N errors`, or `archfit: unknown, no verdict`. The exit code is `0`, `1`, or `2`. Each error says where, which rule, what is wrong, and `next`: do what `next` says. `unknown` is never a pass: the record names what to restore. See [Results](results.md).

When no workflow uses the Archfit Action, every result carries the line `policy: not protected`: the result is a local guide. The protected check on pull requests comes with the Archfit App and Action 4.

## When a rule is in your way

Some rules need an owner: a new dependency between modules (`graph`), or a copy of a literal another module holds (`duplication`). Their `next` is `archfit request`. After `archfit check` (or `archfit can`), run:

```sh
archfit request -m "netem needs the shared retry helper"
```

The request takes the errors of the last check, so you write only the reason. Add `--expires 2026-12-10` to ask for an exception until a date. Without the App, `archfit request` prints the request and ends with `next: ask an owner`: show it to an owner. Do not copy the code to get around the rule.

## For owners: the state of the repository

```sh
archfit report
```

`report` judges the whole tree, not one change. Its state is `clean`, `drift` (an error no exception covers), or `expired` (an exception passed its date and the code still breaks the rule). It also lists the exceptions close to their date, the rules nothing checks, the balance of each allowed dependency, and the measurements of the code. It exits `0` whatever it shows.

## Run the check in CI

A pull request is checked against its target branch. Fetch the history and name the branch:

```sh
archfit check --base "origin/$TARGET_BRANCH"
```

Fail the job on any exit code except `0`. See [CI](ci.md) for the workflow.

## Use Docker

The release image runs archfit with a bundled toolchain:

```sh
docker run --rm -v "$(pwd):/repo" ghcr.io/alexei-led/archfit:v3.2.0 \
  check --config /repo/.archfit.yaml
```

The complete image includes Node, Python, Rust, the .NET SDK, syntax and clone tools, cargo-modules, and the Roslyn helper (`archfit-roslyn`).
Go is build-only and is not present in the final image.
No analyzer package installation is needed at run time.
See [Languages and tools](languages.md) for exact versions, optional execution, and limits.

The image runs as a non-root user and supports an arbitrary numeric UID with `HOME=/tmp`.
Target dependencies can still need network access. The image is not a network-isolated sandbox:
optional indexers and Rust build tools can execute target code. Do not forward credentials into it.

## More help

- [Commands](commands.md) — flags and exit codes.
- [Configuration](configuration.md) — the policy fields and their defaults.
- [Troubleshooting](troubleshooting.md) — common setup and analysis issues.
