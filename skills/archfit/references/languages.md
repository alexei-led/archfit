# archfit language guidance

Use this when the task changes language-specific analyzer setup, config globs,
or coverage-gap handling. `archfit doctor` tells you what was detected; this file
tells you what "good" looks like.

## Cross-language defaults

- Detect repo markers first: `go.mod`, `package.json`, `pyproject.toml` /
  `setup.py`, `Cargo.toml`.
- Prefer repo-local, pinned analyzers over mutable global installs.
- Use `archfit install --dry-run --lang <x>` to see the helper's expected setup.
- Missing tools are coverage gaps, not healthy signal. Name the affected metrics,
  lower confidence, and avoid calling the run clean on exit 0 alone.
- For mixed repos, keep each language's modules distinct where possible.

## Go

Use when the repo has `go.mod`.

Checks:

```sh
go version
archfit doctor
archfit install --lang go --dry-run
```

Guidance:

- Go analysis needs `go` on `PATH`; no extra Go analyzer is installed.
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
archfit install --lang ts --dry-run
```

Guidance:

- `archfit` uses `bunx depcruise` when `bunx` exists, otherwise `npx depcruise`.
- Prefer a pinned repo-local `dependency-cruiser` install.
- Config paths and rule filters are repo-relative file globs.
- For higher-fidelity symbol evidence, SCIP needs installed dependencies
  (`node_modules`). If they are absent, `risk_hub` stays `n/a`.
- If Node/Bun or dependency-cruiser is missing, treat TS/JS architecture results
  as partial only.

## Python

Use when the repo has `pyproject.toml`, `setup.py`, or a clear top-level package.

Checks:

```sh
uv --version || python3 --version
archfit doctor
archfit install --lang py --dry-run
```

Guidance:

- Prefer `uv`; without it, use Python 3.12+ with pinned `grimp`.
- Set `python_package` when the import root is not the repo name or the repo uses
  `src/` layout.
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
archfit install --lang rust --dry-run
```

Guidance:

- Base Rust analysis uses `cargo metadata`; the default graph is crate-level.
- Config paths and rule filters are crate-name globs from `Cargo.toml`, not file
  paths.
- Single-crate repos are structurally shallow at crate level. For real module
  depth, enable `tools.cargo-modules.enabled: on`; for symbol-level strength,
  enable `tools.scip.enabled: on` with `rust-analyzer` available.
- If `cargo` is missing, Rust metrics stay `n/a`; do not treat that as a pass.
- If `cargo-modules` or `rust-analyzer` is absent, call out the exact dimensions
  that remain coarse or unmeasured.

## Optional analyzers that often explain `n/a`

- `tools.complexity.enabled: on` + `lizard` → `complexity`
- `tools.scip.enabled: on` + language SCIP indexer / `rust-analyzer` → `risk_hub`
- `tools.clones.enabled: on` + clone detector → `functional_candidates`
- `tools.cargo-modules.enabled: on` + `cargo-modules` → Rust intra-crate depth

When one is missing, say so explicitly. `n/a` means unmeasured, not strong.
