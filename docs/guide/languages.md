# Language support

This page is the canonical list of currently supported language adapters.
`archfit` can analyze Go, TypeScript/JavaScript, Python, and Rust in the same run.
Enable languages in `.archfit.yaml` with `tools.<language>.enabled`.

```yaml
tools:
  go:
    enabled: auto
  typescript:
    enabled: auto
  python:
    enabled: auto
  rust:
    enabled: auto
```

Tool modes:

- `auto` — use the adapter when project markers and tools are found.
- `on` — require the adapter; missing project markers or tools are errors.
- `off` — skip the adapter.

Use `auto` for mixed repos while calibrating. Use `on` in CI when a language must
be analyzed.

## Go

Requirements:

- `go` on `PATH`;
- `go.mod` at the repository root.

How extraction works:

- loads packages with `go/packages`;
- emits file-to-package import edges;
- strips the module path so internal imports become repo-relative paths;
- records import locations when available.

Install/check:

```sh
go version
archfit doctor
```

Example config:

```yaml
tools:
  go:
    enabled: on
  typescript:
    enabled: off
  python:
    enabled: off

layers: [model, core, adapter, cmd]
modules:
  domain:
    paths: [internal/domain/**]
    public: [internal/domain]
    internal: [internal/domain/internal/**]
    layer: model
    subdomain: core
  http:
    paths: [internal/http/**]
    public: [internal/http]
    layer: adapter
    subdomain: supporting
rules:
  - id: domain_no_http
    type: forbidden_dependency
    from: internal/domain/**
    to: internal/http
```

For Go, `public` usually names package import paths, such as `internal/domain`,
not individual `.go` files.

## TypeScript and JavaScript

Requirements:

- `package.json` at the repository root;
- Node.js `24.x` preferred (`22+` for the full optional npm toolset) with `npx`,
  or Bun with `bunx`;
- dependency-cruiser available locally or through the package runner.

Install a pinned dependency-cruiser version:

```sh
npm install --save-dev dependency-cruiser@17.4.3
# or
bun add --dev dependency-cruiser@17.4.3
```

How extraction works:

- runs `bunx depcruise` when `bunx` is available;
- otherwise runs `npx depcruise`;
- reads dependency-cruiser JSON output;
- skips Node.js core modules;
- emits file-to-file dependency edges.

Example config:

```yaml
tools:
  go:
    enabled: off
  typescript:
    enabled: on
  python:
    enabled: off

layers: [domain, app, ui]
modules:
  domain:
    paths: [src/domain/**]
    public: [src/domain/index.ts]
    internal: [src/domain/internal/**]
    layer: domain
    subdomain: core
  ui:
    paths: [src/ui/**]
    public: [src/ui/index.ts]
    layer: ui
    subdomain: supporting
rules:
  - id: domain_no_ui
    type: forbidden_dependency
    from: src/domain/**
    to: src/ui/**
    gate: fail
  - id: no_internal_imports
    type: public_api_only
    gate: fail
```

For TypeScript, module paths and rule filters are repo-relative file path globs.

## Python

Requirements:

- `pyproject.toml`, `setup.py`, or configured `python_package`;
- `uv` (preferred), or Python `3.12+` with `grimp` installed.

Recommended install:

```sh
brew install uv              # macOS
# Linux: use a current distro package or the Astral installer; see tooling.md
```

How extraction works:

- detects `uv` first;
- with `uv`, injects `grimp` for the helper run;
- without `uv`, runs Python 3.12+ and expects a pinned `grimp` to be installed;
- emits dotted module-to-module import edges, such as `myapp.service`.

Set `python_package` when the top-level package is not the repository directory
name, or when the repo uses `src/` layout.

```yaml
python_package: myapp
tools:
  go:
    enabled: off
  typescript:
    enabled: off
  python:
    enabled: on
```

Example config:

```yaml
python_package: myapp
tools:
  python:
    enabled: on

layers: [domain, app, adapter]
modules:
  domain:
    paths: [myapp.domain**]
    public: [myapp.domain.api**]
    internal:
      - myapp.domain._internal**
      - myapp/domain/_internal/**
    layer: domain
    subdomain: core
  web:
    paths: [myapp.web**]
    public: [myapp.web**]
    layer: adapter
    subdomain: supporting
rules:
  - id: domain_no_web
    type: forbidden_dependency
    from: myapp.domain**
    to: myapp.web**
    gate: fail
  - id: no_private_python_imports
    type: internal_api_access
    gate: fail
```

Python notes:

- dependency nodes are dotted module names;
- use dotted globs for `modules.paths`, `public`, and rule `from`/`to` filters;
- include slash-style `internal` globs too when you want the extractor to mark
  `_internal` packages as internal-access edges;
- imports of underscore-prefixed modules, such as `myapp._internal`, are treated
  as intrusive coupling signals.

## Rust

Requirements:

- `cargo` on `PATH`;
- `Cargo.toml` at the repository root.

Recommended install:

```sh
rustup default stable
cargo --version
```

If rustup is not installed, use your platform package manager where available, or
follow the official rustup installer at <https://rust-lang.org/tools/install/>.

How extraction works:

- runs `cargo metadata --format-version 1 --no-deps` in the project root;
- emits one `package:<crate>` node per workspace member;
- emits an `external:<crate>` node for each registry dependency;
- emits `depends_on` edges located at `Cargo.toml`;
- skips dev-dependencies unless `rust_include_dev_deps: true`.

Granularity is **crate-level by default**: each workspace member is one node, so a
single-crate repo yields exactly one `package:` node and no intra-workspace edges. The
scorecard treats such a degenerate (<2-node) graph honestly — it never scores `strong`
on it.

For finer resolution, two opt-in passes add the **intra-crate module graph** so
single-crate repos get real cycle, blast-radius, cohesion, and god-file signal:

- `tools.cargo-modules.enabled: on` runs `cargo-modules` to emit `<crate>::<mod>` nodes
  and aggregated `uses` edges.
- `tools.scip.enabled: on` runs `rust-analyzer scip` for symbol-level integration
  strength on those module edges.

With either on, per-file LOC and git churn also resolve to module granularity (via the
crate roots cargo metadata provides), so `structural_weight` flags god _files/modules_ by
size and the change-history metrics measure inside a single crate — not just across
crates. See [Optional analyzers](#optional-analyzers-per-language) below.

Optional config:

- `rust_manifest` — path to a non-root `Cargo.toml` (empty = auto, root manifest);
- `rust_features` — cargo features to activate for the metadata run;
- `rust_include_dev_deps` — include dev-dependencies as crate edges.

Example config (multi-crate workspace):

```yaml
tools:
  go:
    enabled: off
  typescript:
    enabled: off
  python:
    enabled: off
  rust:
    enabled: on

layers: [core, cli]
modules:
  core:
    paths: [grep-*]
    layer: core
    subdomain: core
  cli:
    paths: [ripgrep]
    layer: cli
    subdomain: supporting
rules:
  - id: core_no_cli
    type: forbidden_dependency
    from: grep-*
    to: ripgrep
    gate: fail
```

For Rust, module paths and rule filters are crate-name globs (the crate name from
`Cargo.toml`, such as `grep-cli`), not file paths.

## Optional analyzers per language

The deterministic gates need only the language adapter above. The report-only
metrics need extra tools, and several are language-specific. When a tool is
missing the dependent metric reports `n/a` **with the reason and enable step** —
the run never fails — but the metric stays blind until you install it.

| Tool                | Powers                          | Go  | TS/JS | Python | Rust | Setup                                                                             |
| ------------------- | ------------------------------- | --- | ----- | ------ | ---- | --------------------------------------------------------------------------------- |
| `lizard`            | `complexity`                    | yes | yes   | yes    | yes  | `tools.complexity.enabled: on`; `uv tool install 'lizard==1.23.0'`                |
| SCIP indexer + `uv` | `risk_hub`                      | yes | yes   | yes    | yes  | `tools.scip.enabled: on`; see notes below                                         |
| clone detector      | `functional_candidates`         | yes | yes   | yes    | yes  | `tools.clones.enabled: on`; `npm install -g jscpd@5.0.11`                         |
| `gitnexus`          | enriches `risk_hub`             | yes | yes   | yes    | yes  | `tools.gitnexus.enabled: on` (or auto-detect); `npm install -g gitnexus@1.6.8`    |
| `cargo-modules`     | intra-crate module graph (Rust) | —   | —     | —      | yes  | `tools.cargo-modules.enabled: on`; `cargo install cargo-modules --version 0.26.0` |

Notes that bite most often:

- **Complexity needs PyPI `lizard` and `tools.complexity.enabled: on`.** Without
  both, `complexity` is `n/a` for every language. lizard supports Go, Python,
  TypeScript, TSX, and Rust. Install it with `uv tool install 'lizard==1.23.0'` and
  set the config flag, then re-run. Do **not** use `brew install lizard`; that is
  a compression tool with the same command name.
- **SCIP indexers are language-specific.** Use `go install github.com/sourcegraph/scip-go/cmd/scip-go@v0.2.7`,
  `npm install -g @sourcegraph/scip-typescript@0.4.0`,
  `npm install -g @sourcegraph/scip-python@0.6.6`, or
  `rustup component add rust-analyzer`, plus `uv` for archfit's embedded SCIP reader.
- **SCIP for TypeScript needs `node_modules`.** `scip-typescript` resolves
  imports through installed dependencies, so run `npm ci` (or `bun install`)
  before the run. If `node_modules` is absent, archfit reports `risk_hub` as
  `n/a` with exactly that reason instead of silently skipping it.
- **SCIP for Rust uses `rust-analyzer`.** With `tools.scip.enabled: on` and
  `rust-analyzer` on `PATH`, archfit runs `rust-analyzer scip` to add symbol-level
  strength on top of the crate-level `cargo metadata` graph. When the binary is
  absent the pass no-ops cleanly — no error, `risk_hub` stays `n/a` with the
  reason.
- **Rust module depth needs `cargo-modules`.** A single crate is one node at crate
  level, so cycle/blast-radius/cohesion go `n/a`. `tools.cargo-modules.enabled: on`
  (with `cargo install cargo-modules --version 0.26.0`) adds the `<crate>::<mod>` graph; archfit then
  maps per-file LOC/churn to module keys so `structural_weight` flags god _files_ and
  the change-history metrics measure within the crate. On a workspace, crates whose
  `cargo-modules` run fails (proc-macro/codegen) are named in the coverage reason and
  the structural dimensions drop to medium confidence — partial, never silent.
- **Clone detection is opt-in.** `tools.clones.enabled: on` plus `jscpd`
  (`npm install -g jscpd@5.0.11`) turns `functional_candidates` on. Off or absent
  → `n/a`.
- **gitnexus auto-detects a present index.** With `tools.gitnexus.enabled`
  unset/`auto`, a present `.gitnexus/` or `.codegraph` index is used
  automatically; `on` always queries; `off` opts out but still reports that an
  index is present so you can flip the flag. Refresh the index with
  `node .gitnexus/run.cjs analyze --index-only`. archfit only reads it.

See [Install → optional analysis tools](install.md#optional-analysis-tools) for
quick setup and [Tooling reference](tooling.md) for platform-specific package
manager choices, versions, home pages, and PATH checks.

## Mixed repositories

For a repo with more than one language, keep each language's paths in distinct
modules where possible:

```yaml
tools:
  go:
    enabled: auto
  typescript:
    enabled: auto
  python:
    enabled: auto

modules:
  api_go:
    paths: [internal/api/**]
    layer: adapter
  web_ts:
    paths: [src/web/**]
    layer: adapter
  jobs_py:
    paths: [myapp.jobs**]
    layer: adapter
```

Run `archfit doctor` before blaming config. Missing optional tools in `auto` mode
produce absent coverage, not a hard failure.
