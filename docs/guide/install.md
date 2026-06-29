# Install

Install the `archfit` binary first, then install only the analyzer tools needed for
your repository. For the full platform/tool matrix, versions, home pages, package
manager choices, and PATH checks, see [Tooling reference](tooling.md).

## Install the archfit CLI

Install from source with Go. Use a release tag, not `@latest`, in scripts and
repeatable docs:

```sh
go install github.com/alexei-led/archfit/cmd/archfit@v0.6.1
```

Check the binary and available analyzers:

```sh
archfit doctor
```

## Analyzer summary

- Git: `git` resolves changed files and refs for diff mode.
- Go analysis: `go` loads packages with `go/packages`; this repo targets Go
  `1.26.x`.
- TypeScript/JavaScript analysis: Node/npm or Bun plus dependency-cruiser extract
  the import graph. Node `24.x` is preferred; Node `22+` keeps the optional npm
  tools compatible.
- Python analysis: `uv` is preferred. With `uv`, archfit injects `grimp` for the
  run; without `uv`, use Python `3.12+` with `grimp` installed.
- Rust analysis: `cargo` (normally from rustup) extracts the crate graph with
  `cargo metadata`.
- Structural patterns: `sg` must be the ast-grep binary, not the util-linux `sg`.
- Optional depth tools: SCIP indexers, `jscpd`, and `cargo-modules`
  feed report-only metrics and coupling-balance edge-strength precision.

## Platform setup quick start

macOS:

```sh
brew install git go node uv ast-grep
brew install rustup
rustup default stable
```

Debian / Ubuntu:

```sh
sudo apt update
sudo apt install git python3 nodejs npm
```

Fedora / RHEL-like:

```sh
sudo dnf install git python3 nodejs npm golang
```

Check distro package versions before relying on them for Go, Node, uv, or Rust.
If the package is too old for the constraints above, use the upstream install docs
linked from [Tooling reference](tooling.md).

## Install helper

`archfit install` checks or installs the common external tools it knows how to
bootstrap. It is intentionally conservative and not a complete installer for every
optional analyzer. Use `--dry-run` first:

```sh
archfit install --lang go --lang ts --lang py --lang rust --dry-run
```

Current behavior:

- `--lang go` checks for `go` and prints the Go download URL if missing.
- `--lang ts` checks for Node.js and installs it with Homebrew when available.
- `--lang py` checks for `uv` and installs it with Homebrew when available.
- `--lang rust` checks for `cargo` and prints the rustup URL if missing.

For deterministic CI, prefer explicit package-manager setup from
[Tooling reference](tooling.md), then run `archfit doctor` as a diagnostic step.

## TypeScript and JavaScript tools

Install Node.js or Bun, then add dependency-cruiser to the repository. Project-local
is preferred over a global install because it keeps the analyzer version in the
repo lockfile.

```sh
npm install --save-dev dependency-cruiser@17.4.3
# or
bun add --dev dependency-cruiser@17.4.3
```

`archfit` runs dependency-cruiser through `bunx depcruise` when `bunx` is found,
otherwise through `npx depcruise`.

## Python tools

Recommended setup:

```sh
brew install uv              # macOS
# Linux: use a current distro package or the Astral installer; see tooling.md
```

With `uv`, `archfit` injects `grimp` for the extractor run, so the project does
not need to add `grimp` as a dependency.

Without `uv`, use Python `3.12+` and install a pinned `grimp` in the environment
used by the repo:

```sh
python3.12 -m pip install 'grimp==3.14'
```

## Go tools

Go repositories need the `go` command available on `PATH`:

```sh
go version
```

No extra Go analyzer is installed. The Go adapter uses the Go package loader.
Optional SCIP support uses `scip-go`; see [Optional analysis tools](#optional-analysis-tools).

## Rust tools

Rust repositories need the `cargo` command on `PATH`. Prefer rustup-managed
stable Rust so `cargo`, `rustc`, and components stay aligned:

```sh
rustup default stable
cargo --version
```

If rustup is not installed, use your platform package manager where available, or
the official rustup installer from <https://rust-lang.org/tools/install/>.

`archfit` runs `cargo metadata` to extract the crate dependency graph. Two opt-in
tools add intra-crate module depth (important for single-crate repos, which are one
node at crate level):

- **`cargo-modules`** (`analyzers.cargo_modules.enabled: true`) builds the intra-crate
  module graph (`<crate>::<mod>` nodes + `uses` edges):

  ```sh
  cargo install cargo-modules --version 0.26.0
  ```

- **`rust-analyzer`** (`analyzers.scip.enabled: true`) adds symbol-level integration
  strength via `rust-analyzer scip`:

  ```sh
  rustup component add rust-analyzer
  ```

With either on, per-file LOC resolves to module granularity, so size-based
metrics cover individual modules rather than entire crates.

## Optional analysis tools

These power report-only metrics and are off by default. Install them only when you
enable the matching key in `.archfit.yaml` (`analyzers.*` or `languages.*`).

- **SCIP** (improves `coupling_balance` edge-strength precision for TS/Py/Rust;
  adds Rust module-level strength) — install the indexer for your language plus
  `uv` for the embedded SCIP reader:

  ```sh
  go install github.com/sourcegraph/scip-go/cmd/scip-go@v0.2.7
  npm install -g @sourcegraph/scip-typescript@0.4.0
  npm install -g @sourcegraph/scip-python@0.6.6
  rustup component add rust-analyzer
  ```

  Enable with `analyzers.scip.enabled: true`. TypeScript also needs project dependencies
  installed (`npm ci` or `bun install`).

- **Clone detection** (`coupling_balance` symmetric-coupling signal) — current
  extractor probes `jscpd`:

  ```sh
  npm install -g jscpd@5.0.11
  ```

  Enable with `analyzers.clones.enabled: true`.

When a tool is absent, the dependent metric reports `n/a` — the run never fails
unless you opt in with `--require-tools` or `analyzers.<x>.gate: fail`.

## Docker

Use Docker when you want the bundled toolchain instead of installing language
analysis tools on the host:

```sh
docker run --rm -v "$(pwd):/repo" ghcr.io/alexei-led/archfit:v0.6.1 \
  analyze --config /repo/.archfit.yaml --full
```

The image bundles the `archfit` binary plus the full non-Rust analysis toolchain:
the Go SDK (Go targets need it at runtime), Node.js 24 with dependency-cruiser and
ast-grep (`sg`), and `uv` with Python 3 (`grimp` is resolved at runtime via
`uv run --with grimp`).

The image does not bundle the Rust toolchain. Run Rust targets on a host with
`cargo`/`rust-analyzer`, or extend the image with rustup; Rust analysis reports
`n/a` (never fails) when `cargo` is absent.
