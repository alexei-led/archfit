# Install

Install from source with Go:

```sh
go install github.com/alexei-led/archfit/cmd/archfit@latest
```

Check the binary and available analyzers:

```sh
archfit doctor
```

## Tool summary

- CLI install: Go 1.26+ builds and installs the binary.
- Git: `git` resolves changed files and refs for diff mode.
- Go analysis: `go` loads packages with `go/packages`.
- TypeScript analysis: Node.js, `npx` or `bunx`, and dependency-cruiser extract
  the TS/JS dependency graph.
- Python analysis: `uv` or Python 3.12+ with `grimp` extracts the Python import
  graph.
- Structural patterns: `sg` from ast-grep runs configured patterns.
- Symbol indexers: `scip-typescript`, `scip-python`, and `scip-go` are optional
  and reserved for higher-fidelity symbol resolution.

`archfit doctor` reports all detected tools.

## Install helper

`archfit install` checks or installs the common external tools it knows how to
bootstrap. Use `--dry-run` first:

```sh
archfit install --lang go --lang ts --lang py --dry-run
```

Current behavior:

- `--lang go` checks for `go` and prints the Go download URL if missing.
- `--lang ts` checks for Node.js and installs it with Homebrew when available.
- `--lang py` checks for `uv` and installs it with Homebrew when available.

For deterministic CI, prefer explicit package-manager setup in your workflow.

## TypeScript tools

Install Node.js 22+ or Bun, then add dependency-cruiser to the repository:

```sh
npm install --save-dev dependency-cruiser
# or
bun add --dev dependency-cruiser
```

`archfit` runs dependency-cruiser through `bunx depcruise` when `bunx` is found,
otherwise through `npx depcruise`.

## Python tools

Recommended setup:

```sh
brew install uv
```

With `uv`, `archfit` runs the Python extractor with `uv run --with grimp`, so the
project does not need to add `grimp` as a dependency.

Without `uv`, use Python 3.12+ and install `grimp` in the environment used by the
repo:

```sh
python3.12 -m pip install grimp
```

## Go tools

Go repositories need the `go` command available on `PATH`:

```sh
go version
```

No extra Go analyzer is installed. The Go adapter uses the Go package loader.

## Docker

Use Docker when you want the bundled toolchain instead of installing language
analysis tools on the host:

```sh
docker run --rm -v "$(pwd):/repo" ghcr.io/alexei-led/archfit:latest \
  check --config /repo/.archfit.yaml --full
```

The image includes the `archfit` binary, Node.js, dependency-cruiser, Python,
`uv`, and `grimp`.
