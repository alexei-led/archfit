# archfit language guidance

Use this when the task changes language-specific analyzer setup, config globs,
or coverage-gap handling. `archfit doctor` reports detected tools. This file
describes the expected setup.

## Cross-language defaults

- Detect repo markers first: `go.mod`, `package.json`, `pyproject.toml` /
  `setup.py`, `Cargo.toml`.
- Prefer repo-local, pinned analyzers over mutable global installs.
- Use `archfit doctor --fix --dry-run --lang <x>` to see the helper's expected setup.
- Optional analyzers run only when `enabled: true`. `auto` does not run them.
- Missing tools are coverage gaps, not healthy signal. Name the affected metrics,
  lower confidence, and avoid calling the run clean on exit 0 alone.
- For mixed repos, keep each language's modules distinct where possible.

## Go

Use when the repo has `go.mod`.

Checks:

```sh
go version
archfit doctor
archfit doctor --fix --lang go --dry-run
```

Guidance:

- Go analysis needs `go` on `PATH`. It does not need another Go analyzer.
- Config paths and `public` entries usually name package import paths, not
  individual `.go` files.
- Start with package-level module boundaries (`internal/domain`, `internal/http`,
  `cmd/...`) before adding narrower rules.
- If `go` is missing, report the gap and stop short of judging Go-derived metrics.

## TypeScript and JavaScript

Use when the repo has `package.json`.

Checks:

```sh
node --version || bun --version
archfit doctor
archfit doctor --fix --lang ts --dry-run
```

Guidance:

- `archfit` uses `bunx depcruise` when `bunx` exists, otherwise `npx depcruise`.
- Prefer a pinned repo-local `dependency-cruiser` install.
- Config paths and rule filters are repo-relative file globs.
- For higher-fidelity symbol evidence, SCIP needs installed dependencies
  (`node_modules`). If they are absent, edge strength classification falls back
  to heuristics and `coupling_balance` confidence drops.
- If Node/Bun or dependency-cruiser is missing, treat TS/JS architecture results
  as partial only.

## Python

Use when the repo has `pyproject.toml`, `setup.py`, or a clear top-level package.

Checks:

```sh
uv --version || python3 --version
archfit doctor
archfit doctor --fix --lang py --dry-run
```

Guidance:

- `uv` selects Python 3.14 and `grimp==3.17` without target project setup.
  Without `uv`, use a separate Python 3.14+ environment with `grimp==3.17`.
  Missing project dependencies do not invalidate static import literals.
- Set `languages.python.package` when the import root is not the repo name or
  the repo uses `src/` layout.
- Config paths and rule filters use dotted module globs (`myapp.domain**`).
- Keep slash-style `internal` globs too when you want `_internal` package access
  to show up as intrusive/internal-access edges.
- If `uv`/Python/`grimp` is missing, report Python coverage as incomplete rather
  than inferring health from other languages.

## Rust

Use when the repo has `Cargo.toml`.

Checks:

```sh
cargo --version
archfit doctor
archfit doctor --fix --lang rust --dry-run
```

Guidance:

- Base Rust analysis uses `cargo metadata`. The default graph is crate-level.
- Config paths and rule filters are crate-name globs from `Cargo.toml`, not file
  paths.
- A single crate has little structure at crate level. For module detail, enable
  `analyzers.cargo_modules.enabled: true`. For symbol-level strength, enable
  `analyzers.scip.enabled: true` and install `rust-analyzer`.
- If `cargo` is missing, Rust metrics stay `n/a`. Do not treat that as a pass.
- If `cargo-modules` or `rust-analyzer` is absent, call out the exact dimensions
  that remain coarse or unmeasured.

## Optional analyzers that often explain `n/a`

- `analyzers.scip.enabled: true` with a language SCIP indexer or `rust-analyzer`
  adds edge-strength detail and raises `coupling_balance` confidence.
- `analyzers.clones.enabled: true` with jscpd adds clone-detected `symmetric`
  facts (S=9). These facts do not represent import edges. They affect the
  `coupling_balance` distribution.
- `analyzers.cargo_modules.enabled: true` with `cargo-modules` adds Rust module
  depth for `encapsulation` and `cycle` signals.

When one is missing, say so explicitly. `n/a` means unmeasured, not strong.
