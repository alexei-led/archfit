# Install

Install from source with Go. Use a release tag, not `@latest`, in scripts and
repeatable docs:

```sh
go install github.com/alexei-led/archfit/cmd/archfit@v0.5.0
```

Check the binary and available analyzers:

```sh
archfit doctor
```

## Tool summary

- CLI install: Go 1.26+ builds and installs the binary.
- Git: `git` resolves changed files and refs for diff mode.
- Go analysis: `go` loads packages with `go/packages`.
- TypeScript analysis: Node.js 24 LTS, `npx` or `bunx`, and dependency-cruiser
  extract the TS/JS dependency graph.
- Python analysis: `uv` or Python 3.14 recommended / Python 3.12+ minimum with
  `grimp` extracts the Python import graph.
- Rust analysis: `cargo` (from rustup) extracts the crate dependency graph with
  `cargo metadata`.
- Structural patterns: `sg` from ast-grep runs configured patterns.
- Symbol indexers: `scip-typescript`, `scip-python`, `scip-go`, and
  `rust-analyzer` are optional and reserved for higher-fidelity symbol resolution.

`archfit doctor` reports all detected tools.

## Install helper

`archfit install` checks or installs the common external tools it knows how to
bootstrap. Use `--dry-run` first:

```sh
archfit install --lang go --lang ts --lang py --lang rust --dry-run
```

Current behavior:

- `--lang go` checks for `go` and prints the Go download URL if missing.
- `--lang ts` checks for Node.js and installs it with Homebrew when available.
- `--lang py` checks for `uv` and installs it with Homebrew when available.
- `--lang rust` checks for `cargo` and prints the rustup URL if missing.

For deterministic CI, prefer explicit package-manager setup in your workflow.

## TypeScript tools

Install Node.js 24 LTS or Bun, then add dependency-cruiser to the repository.
Pin the package version in examples, CI, and lockfiles:

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
brew install uv
```

With `uv`, `archfit` injects `grimp` for the extractor run, so the project does
not need to add `grimp` as a dependency.

Without `uv`, use Python 3.14 when available, or Python 3.12+ minimum, and
install a pinned `grimp` in the environment used by the repo:

```sh
python3.14 -m pip install 'grimp==3.13'
```

## Go tools

Go repositories need the `go` command available on `PATH`:

```sh
go version
```

No extra Go analyzer is installed. The Go adapter uses the Go package loader.

## Rust tools

Rust repositories need the `cargo` command on `PATH`. Install the toolchain with
rustup:

```sh
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
cargo --version
```

`archfit` runs `cargo metadata` to extract the crate dependency graph. Two opt-in
tools add intra-crate module depth (important for single-crate repos, which are one
node at crate level):

- **`cargo-modules`** (`tools.cargo-modules.enabled: on`) builds the intra-crate
  module graph (`<crate>::<mod>` nodes + `uses` edges):

  ```sh
  cargo install cargo-modules
  ```

- **`rust-analyzer`** (`tools.scip.enabled: on`) adds symbol-level integration
  strength via `rust-analyzer scip`:

  ```sh
  rustup component add rust-analyzer
  ```

With either on, per-file LOC/churn resolves to module granularity, so `structural_weight`
flags god _files/modules_ (not just god crates) and the history metrics measure inside a
single crate.

## Optional analysis tools

These power the report-only metrics and are off by default. Install them only when
you enable the matching `tools.*` key in `.archfit.yaml`.

- **SCIP** (powers `risk_hub`) — install a SCIP indexer for your language
  (`scip-go`, `scip-python`, `scip-typescript`, or `rust-analyzer` for Rust) plus
  [`uv`](https://docs.astral.sh/uv/). Enable with `tools.scip.enabled: on`.
- **Clone detectors** (power `functional_candidates`) — `npm install -g jscpd@5.0.9`
  for JS/TS, or install [PMD](https://pmd.github.io/) (includes CPD) for Go/Python.
  Enable with `tools.clones.enabled: on`.
- **gitnexus** (enriches `risk_hub`) — install the `gitnexus` binary on `PATH`.
  Enable with `tools.gitnexus.enabled: on`, or leave it unset/`auto` to use a
  present `.gitnexus`/`.codegraph` index automatically.
- **lizard** (powers `complexity`) — `pip install lizard` or
  `uv tool install lizard`. Supports Go, Python, TypeScript, and TSX. Enable
  with `tools.complexity.enabled: on`; `complexity` is `n/a` when not enabled.

When a tool is absent, the dependent metric reports `n/a` — the run never fails.

## Docker

Use Docker when you want the bundled toolchain instead of installing language
analysis tools on the host:

```sh
docker run --rm -v "$(pwd):/repo" ghcr.io/alexei-led/archfit:v0.5.0 \
  check --config /repo/.archfit.yaml --full
```

The image bundles the `archfit` binary plus the full analysis toolchain: the Go
SDK (Go targets need it at runtime), Node.js 24 with dependency-cruiser and
ast-grep (`sg`), and `uv` with Python 3 (`grimp` is resolved at runtime via
`uv run --with grimp`).

The image does not bundle the Rust toolchain. Run Rust targets on a host with
`cargo`/`rust-analyzer`, or extend the image with rustup; Rust analysis reports
`n/a` (never fails) when `cargo` is absent.
