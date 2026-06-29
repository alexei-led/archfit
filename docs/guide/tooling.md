# Tooling reference

Use this page when `archfit doctor` reports a missing analyzer, when you build CI
images, or when a tool is installed but not found on `PATH`.

Version snapshot: 2026-06-23, from upstream docs/registries and Homebrew formulae.
Pin these versions in CI. For local development, using the package manager's latest
stable release is usually fine.

## Install policy

`archfit` discovers tools by running binaries on `PATH`. Keep the install source
predictable:

1. Prefer the platform package manager for platform tools.
   - macOS: Homebrew.
   - Debian/Ubuntu: `apt`.
   - Fedora/RHEL-like: `dnf`.
2. Use the language package manager for language-specific CLIs.
   - npm for Node CLIs (`dependency-cruiser`, `jscpd`, SCIP JS/Python indexers).
   - `uv tool` for Python CLIs.
   - `go install` for Go CLIs (`scip-go`).
   - `cargo install` for Rust CLIs (`cargo-modules`, `ast-grep` when Homebrew is
     unavailable).
   - `rustup` for the Rust toolchain and Rust components (`cargo`,
     `rust-analyzer`).
3. Use remote installer scripts only when they are the upstream documented path
   and you have reviewed the command. Do not hide `curl | sh` inside automation.
4. Do not mix installers for the same toolchain unless you understand PATH order
   (`brew node` + `nvm`, distro Rust + `rustup`, etc.).

`archfit doctor` is read-only. `archfit install` is intentionally conservative and
only bootstraps a few common tools. Use this matrix for the complete setup.

## Platform setup

### macOS

Use Homebrew for platform tools and formulae:

```sh
brew install git go node uv ast-grep
```

For Rust, prefer rustup-managed toolchains:

```sh
brew install rustup
rustup default stable
```

Homebrew prefixes to keep on `PATH`:

- Apple Silicon: `/opt/homebrew/bin` and `/opt/homebrew/sbin`.
- Intel macOS: `/usr/local/bin` and `/usr/local/sbin`.

### Debian / Ubuntu

Use `apt` for system tools when the distro version is new enough:

```sh
sudo apt update
sudo apt install git python3 nodejs npm
```

For Go, Node, uv, and Rust, check the distro version first. If it is behind the
version in the matrix, use the upstream docs instead of forcing an old package.

### Fedora / RHEL-like

Use `dnf` for system tools when the distro version is new enough:

```sh
sudo dnf install git python3 nodejs npm golang
```

For Go, Node, uv, and Rust, check the candidate version first. Use upstream docs
when the distro package lags the matrix.

## Core toolchain

| Tool                 | Required for                                         | Version / constraint                                   | macOS install                                  | Linux install                                                    | Homepage                                                  |
| -------------------- | ---------------------------------------------------- | ------------------------------------------------------ | ---------------------------------------------- | ---------------------------------------------------------------- | --------------------------------------------------------- |
| `git`                | repo root, delta mode, history metrics               | OS current (`2.54.0` Homebrew snapshot)                | `brew install git`                             | `sudo apt install git` / `sudo dnf install git`                  | <https://git-scm.com/downloads>                           |
| `go`                 | building `archfit`; Go repo analysis via `go list`   | `1.26.x` for this repo                                 | `brew install go`                              | distro package if `1.26.x`; otherwise <https://go.dev/dl/>       | <https://go.dev/>                                         |
| `node`, `npm`, `npx` | TS/JS tools, npm-based optional analyzers            | Node `24.x` preferred; `22+` for full optional toolset | `brew install node`                            | distro package if `>=22`; otherwise Node docs                    | <https://nodejs.org/en/download>                          |
| `bun`, `bunx`        | optional faster TS package runner                    | `1.3.14`                                               | `brew install bun`                             | official Bun installer, manual                                   | <https://bun.sh/docs/installation>                        |
| `uv` / `uvx`         | Python extraction; SCIP reader; Python tool installs | `0.11.23`                                              | `brew install uv`                              | distro package if current; otherwise Astral installer, manual    | <https://docs.astral.sh/uv/getting-started/installation/> |
| `python3`            | Python fallback when `uv` is unavailable             | `3.12+` for direct fallback; `3.11+` with `uv`         | `brew install python`                          | `sudo apt install python3` / `sudo dnf install python3`          | <https://www.python.org/downloads/>                       |
| `cargo`              | Rust analysis; Rust CLI installs                     | stable Rust toolchain                                  | `brew install rustup && rustup default stable` | distro `rustup` if available; otherwise rustup installer, manual | <https://rust-lang.org/tools/install/>                    |

## Analyzer and optional tool matrix

| Tool                               | Powers                                                       | Version                                           | Preferred install                                                                                                            | Homepage / docs                                          | Notes                                                                              |
| ---------------------------------- | ------------------------------------------------------------ | ------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------- | ---------------------------------------------------------------------------------- |
| `dependency-cruiser` (`depcruise`) | TS/JS dependency graph                                       | `17.4.3`                                          | `npm install --save-dev dependency-cruiser@17.4.3`                                                                           | <https://github.com/sverweij/dependency-cruiser>         | Prefer project-local dev dependency; `archfit` invokes it through `bunx` or `npx`. |
| `grimp`                            | Python import graph                                          | `3.14`                                            | no global install with `uv`; direct fallback: `python3.12 -m pip install 'grimp==3.14'`                                      | <https://pypi.org/project/grimp/>                        | `archfit` normally runs `uv run --with grimp`.                                     |
| `sg` / ast-grep                    | structural patterns                                          | `0.44.0`                                          | macOS: `brew install ast-grep`; otherwise `cargo install ast-grep --version 0.44.0` or `npm install -g @ast-grep/cli@0.44.0` | <https://ast-grep.github.io/guide/quick-start.html>      | Verify `sg --version` says `ast-grep`; Linux can have util-linux `sg`.             |
| `jscpd`                            | `coupling_balance` edge-strength (symmetric-coupling signal) | `5.0.11`                                          | `npm install -g jscpd@5.0.11`                                                                                                | <https://jscpd.dev/>                                     | Current extractor probes `jscpd`. PMD/CPD is only a manual alternative.            |
| `scip-go`                          | Go edge-strength precision for `coupling_balance`            | `v0.2.7`                                          | `go install github.com/sourcegraph/scip-go/cmd/scip-go@v0.2.7`                                                               | <https://github.com/sourcegraph/scip-go>                 | Ensure `$(go env GOPATH)/bin` or `GOBIN` is on `PATH`.                             |
| `scip-typescript`                  | TS/JS edge-strength precision for `coupling_balance`         | `0.4.0`                                           | `npm install -g @sourcegraph/scip-typescript@0.4.0`                                                                          | <https://github.com/sourcegraph/scip-typescript>         | Run `npm ci` / `bun install` first; it needs `node_modules` to resolve imports.    |
| `scip-python`                      | Python edge-strength precision for `coupling_balance`        | `0.6.6`                                           | `npm install -g @sourcegraph/scip-python@0.6.6`                                                                              | <https://github.com/sourcegraph/scip-python>             | Despite the name, the published CLI is npm-based and requires Node.                |
| `rust-analyzer`                    | Rust SCIP symbol facts; edge-strength for `coupling_balance` | rustup component / Homebrew snapshot `2026-06-22` | `rustup component add rust-analyzer`; macOS alternative: `brew install rust-analyzer`                                        | <https://rust-analyzer.github.io/book/installation.html> | If using rustup, the component keeps it aligned with the toolchain.                |
| `cargo-modules`                    | Rust intra-crate module graph                                | `0.26.0`                                          | `cargo install cargo-modules --version 0.26.0`                                                                               | <https://docs.rs/crate/cargo-modules/>                   | Installs a cargo plugin; `archfit` probes `cargo-modules`.                         |

## PATH checks

When a package manager says a tool is installed but `archfit doctor` reports it
missing, inspect PATH before reinstalling.

```sh
command -v <tool>
which -a <tool>
echo "$PATH" | tr ':' '\n'
```

Known bin directories:

```sh
# Homebrew
brew --prefix                     # add <prefix>/bin and <prefix>/sbin

# npm global CLIs
npm config get prefix             # add <prefix>/bin

# Go CLIs installed by `go install`
go env GOBIN GOPATH               # add GOBIN, or GOPATH/bin when GOBIN is empty

# Rust CLIs installed by rustup/cargo
echo "${CARGO_HOME:-$HOME/.cargo}/bin"

# uv tools installed by `uv tool install`
uv tool dir --bin
```

Typical user-level PATH additions:

```sh
# zsh/bash example; add only directories that exist on your machine
export PATH="/opt/homebrew/bin:/opt/homebrew/sbin:$PATH"
export PATH="$HOME/.cargo/bin:$PATH"
export PATH="$HOME/go/bin:$PATH"        # replace with `go env GOPATH`/bin if different
export PATH="$HOME/.local/bin:$PATH"    # common uv/pipx user-tool bin
```

Resolve dynamic paths once (`go env GOPATH`, `uv tool dir --bin`) and add the
literal directory to your shell profile. Do not run package-manager commands on
every shell startup. Do not add duplicate entries blindly. Prefer one Node manager,
one Rust toolchain, and one Python tool installer per shell profile.

## CI and containers

- Pin npm/cargo/go/uv tool versions in CI commands.
- Prefer project-local `dependency-cruiser` so repository analysis is reproducible.
- Run `archfit doctor` in CI as a diagnostic step, but use
  `archfit analyze --gate --require-tools` or per-tool `gate: fail` (under
  `languages.<x>` or `analyzers.<x>`) when missing tools must fail the build.
- The official Docker image bundles the archfit binary, Go SDK, Node, npm,
  dependency-cruiser, ast-grep, uv, and Python. It intentionally does not bundle
  the Rust toolchain; extend the image or run Rust repos on a host with
  `cargo`/`rust-analyzer`.
