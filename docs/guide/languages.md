# Language support

This page is the canonical list of currently supported language adapters.
`archfit` can analyze Go, TypeScript/JavaScript, Python, and Rust in the same run.
Enable languages in `.archfit.yaml` under `languages.<language>.enabled`.

```yaml
languages:
  go:
    enabled: auto
  typescript:
    enabled: auto
  python:
    enabled: auto
  rust:
    enabled: auto
```

`enabled` accepts `auto` (run when project markers and tools are found), `true`
(require; missing tools are errors), or `false` (skip); legacy `"on"`/`"off"` are
a hard error. See [configuration reference](configuration-reference.md) for details.

## Supplied test coverage

Archfit can attribute execution coverage to declared modules, but it does not run
any test command. Produce the artifact in the repository's normal CI test step,
produce its [freshness sidecar](configuration-reference.md#coverage-freshness-sidecar),
and then enable the top-level
[`coverage:` block](configuration-reference.md#coverage).

Use one concrete producer command per language:

| Language | Coverage command | Artifact and `format` |
| --- | --- | --- |
| Go | `go test ./... -coverprofile=coverage.out` | `coverage.out`, `go-coverprofile` |
| TypeScript/JavaScript | `npx c8 --reporter=lcov --reports-dir=coverage npm test` | `coverage/lcov.info`, `lcov` |
| Python | `python3 -m pytest --cov=. --cov-report=json:coverage.json` | `coverage.json`, `coverage-py-json` |
| Rust | `cargo llvm-cov --lcov --output-path coverage.lcov` | `coverage.lcov`, `lcov` |

The commands are examples for the named, established producers; keep the test
runner and flags your repository already uses as long as the resulting format is
one Archfit accepts. Rust may instead use
`cargo llvm-cov --json --summary-only --output-path coverage.json` with
`format: llvm-cov-json`. TypeScript projects using Vitest or Jest may emit LCOV
directly instead of wrapping their test command in c8.

Go coverprofiles and coverage.py/LLVM JSON can contain import-prefixed or absolute
paths; LCOV preserves whatever its producer wrote. Archfit normalizes paths under
the analysis root and reports every unresolved path. Every declared module must
be represented by complete, fresh supplied coverage. Language selects the parser
and unit; it is not a separate completeness denominator.

The artifact alone is not fresh evidence. In the same CI step, write
`<artifact>.sidecar.json` with schema version 1, declared module identities, and
lowercase SHA256 hashes for every source represented by the artifact. A commit
SHA in `source_ref` is metadata, not a substitute for those byte hashes. The
producer must enumerate the complete covered universe; read the
[producer-attestation ceiling](configuration-reference.md#coverage-freshness-sidecar)
before enabling coverage in CI.

Multiple artifacts aggregate only when their units are compatible: Go and
coverage.py use statements, while LCOV and llvm-cov JSON use lines. Unit
conflicts keep testability partial instead of being averaged.

## Go

Requirements:

- `go` on `PATH`;
- `go.mod` at or under the analysis root, or a `go.work` workspace file anywhere
  in or above `--root`.

How extraction works:

- discovers workspace members from `go.work` when present (parsed without
  subprocess using `golang.org/x/mod/modfile`), or falls back to a single
  `go.mod`, then walks the tree for `go.mod` files when neither is at the root;
- loads each member's packages with `go/packages` (`./...`) concurrently, bounded
  to `GOMAXPROCS`;
- emits file-to-package import edges;
- strips each package's module prefix per `pkg.Module.Dir` so imports become
  ScanRoot-relative paths;
- records import locations when available;
- detects and skips synthetic error packages (`Module==nil && len(Errors)>0`)
  rather than failing.

**Workspace (go.work) support:** on a repo with `go.work` and no root `go.mod`,
archfit loads each `use` directory that falls inside `--root` and is not
exclusion-matched, then merges the graphs. When ≥2 members are loaded, archfit
auto-registers each member as a synthetic module (mirroring the Rust
`AugmentModulesFromGraph` pattern, gated on ≥2 to leave single-module repos and
archfit's own self-scan byte-identical). Members whose `RelDir` is `"."` (the
workspace root itself) are skipped during auto-registration — a workspace root
without its own `go.mod` has no module identity and archfit treats it as a loader
entry point only. Single surviving members collapse to the existing single-module
path with no change in output.

**go.work member scoping:** use `languages.go.modules` to restrict which workspace
members are loaded (see
[configuration reference](configuration-reference.md#languagesgomodules)).

Install/check:

```sh
go version
archfit doctor
```

Example config:

```yaml
languages:
  go:
    enabled: true
  typescript:
    enabled: false
  python:
    enabled: false

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

Manage Node with one version manager such as fnm, or use Bun directly. Then
install a pinned dependency-cruiser version in the target repository:

```sh
fnm install 24
fnm default 24
npm install --save-dev dependency-cruiser@17.4.3
# or, without Node/npm
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
languages:
  go:
    enabled: false
  typescript:
    enabled: true
  python:
    enabled: false

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

- `pyproject.toml`, `setup.py`, or configured `languages.python.package`;
- `uv` (preferred), or Python `3.12+` with `grimp` installed.

Recommended install on macOS and Linux:

```sh
curl -LsSf https://astral.sh/uv/install.sh | sh
uv --version
```

This is Astral's official standalone installer. A current package-manager
installation is also supported; keep one `uv` on `PATH`.

How extraction works:

- detects `uv` first;
- with `uv`, injects `grimp` for the helper run;
- without `uv`, runs Python 3.12+ and expects a pinned `grimp` to be installed;
- emits dotted module-to-module import edges, such as `myapp.service`.

Set `languages.python.package` when the top-level package is not the repository
directory name, or when the repo uses `src/` layout.

```yaml
languages:
  go:
    enabled: false
  typescript:
    enabled: false
  python:
    enabled: true
    package: myapp
```

Example config:

```yaml
languages:
  python:
    enabled: true
    package: myapp

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

Recommended install uses the official rustup installer, without Homebrew or
`sudo`:

```sh
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
. "$HOME/.cargo/env"
rustup toolchain install 1.98.0 --profile minimal --component rust-analyzer
rustup default 1.98.0
cargo install cargo-modules --version 0.26.0
cargo --version
```

The owned corpus harness pins Rust `1.98.0`. A target repository's
`rust-toolchain.toml`, or an explicit `RUSTUP_TOOLCHAIN`, may select another
version. See <https://rust-lang.org/tools/install/>.

How extraction works:

- runs `cargo metadata --format-version 1 --no-deps` in the project root;
- emits one `package:<crate>` node per workspace member;
- emits an `external:<crate>` node for each registry dependency;
- emits `depends_on` edges located at `Cargo.toml`;
- skips dev-dependencies unless `languages.rust.include_dev_deps: true`.

Granularity is **crate-level by default**: each workspace member is one node, so a
single-crate repo yields exactly one `package:` node and no intra-workspace edges. The
scorecard treats such a degenerate (<2-node) graph honestly — it never scores `strong`
on it.

For finer resolution, enable both Rust deep-analysis passes. `archfit config init`
and `archfit config update --apply` add these stanzas for projects with a root
`Cargo.toml`:

```yaml
languages:
  rust:
    enabled: auto
analyzers:
  cargo_modules:
    enabled: true
  scip:
    enabled: true
```

- `analyzers.cargo_modules.enabled: true` runs `cargo-modules` to emit
  `<crate>::<mod>` nodes and aggregated `uses` edges.
- `analyzers.scip.enabled: true` runs `rust-analyzer scip` for symbol-level
  integration strength on those module edges, improving `coupling_balance`
  precision.

These passes can be slow and require extra tools (`cargo-modules`, `rust-analyzer`,
and `uv` for SCIP reading). Missing tools are reported as coverage gaps; they do
not crash the run. With either pass on, per-file LOC also resolves to module
granularity (via the crate roots cargo metadata provides), so size-based metrics
cover individual modules rather than entire crates. See [Optional analyzers](#optional-analyzers-per-language) below.

Optional config:

- `languages.rust.manifest` — path to a non-root `Cargo.toml` (empty = auto, root manifest).
  It is also the applicability marker: with it set, a project needs no root
  `Cargo.toml` at all, and a missing `cargo` still reports a coverage gap rather
  than "no Rust here";
- `languages.rust.features` — cargo features to activate for the metadata run;
- `languages.rust.include_dev_deps` — include dev-dependencies as crate edges.

Example config (multi-crate workspace):

```yaml
languages:
  go:
    enabled: false
  typescript:
    enabled: false
  python:
    enabled: false
  rust:
    enabled: auto

analyzers:
  cargo_modules:
    enabled: true
  scip:
    enabled: true

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

## File classification per language

archfit classifies every source file as `Production`, `Test`, `Generated`, or
`Vendor`. Classification drives vendor/generated exclusion in the LOC walk and
in structural metrics. The per-language detection patterns are:

| Language      | Test patterns                                        | Generated patterns                                                                                       |
| ------------- | ---------------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| Go            | `*_test.go`                                          | `// Code generated .* DO NOT EDIT` header; `*.pb.go`; `*_gen.go`; `mock_*.go`; `*_mock.go`; `mocks/` dir |
| TypeScript/JS | `*.test.ts`, `*.spec.ts`, `*.test.tsx`, `*.spec.tsx` | `*.gen.ts` (e.g. `api.gen.ts`); moq-style header                                                         |
| Python        | `test_*.py`, `*_test.py`                             | `_pb2.py`; `// Code generated` header                                                                    |
| Rust          | `tests/` directory path segment                      | `*.gen.rs`; moq-style header                                                                             |

`Vendor` covers `vendor/`, `node_modules/`, and `pkg/mod/` regardless of language.

Auto-detection runs first; the `file_class:` config key (`generated_globs`,
`test_globs`, `mock_frameworks`) adds project-specific patterns on top. See
[Configuration reference → file_class](configuration-reference.md#file_class)
for the config override syntax.

## Optional analyzers per language

The deterministic gates need only the language adapter above. The optional tools
improve `coupling_balance` precision or add complementary metrics. When a tool is
missing, dependent metrics report `n/a` **with the reason and enable step** — the
run never fails.

| Tool                | Powers                                               | Go  | TS/JS | Python | Rust | Setup                                                                                   |
| ------------------- | ---------------------------------------------------- | --- | ----- | ------ | ---- | --------------------------------------------------------------------------------------- |
| SCIP indexer + `uv` | `coupling_balance` edge-strength precision           | yes | yes   | yes    | yes  | `analyzers.scip.enabled: true`; see notes below                                         |
| clone detector      | `coupling_balance` symmetric-coupling signal (jscpd) | yes | yes   | yes    | yes  | `analyzers.clones.enabled: true`; `npm install -g jscpd@5.0.11`                         |
| `cargo-modules`     | intra-crate module graph (Rust)                      | —   | —     | —      | yes  | `analyzers.cargo_modules.enabled: true`; `cargo install cargo-modules --version 0.26.0` |

Notes that bite most often:

- **SCIP indexers are language-specific.** Use `go install github.com/scip-code/scip-go/cmd/scip-go@v0.2.7`,
  `npm install -g @sourcegraph/scip-typescript@0.4.0`,
  `npm install -g @sourcegraph/scip-python@0.6.6`, or
  `rustup component add rust-analyzer`, plus `uv` for archfit's embedded SCIP reader.
- **SCIP for TypeScript needs `node_modules`.** `scip-typescript` resolves
  imports through installed dependencies, so run `npm ci` (or `bun install`)
  before the run. If `node_modules` is absent, archfit reports the edge-strength
  pass as `n/a` with exactly that reason instead of silently skipping it.
- **SCIP for Rust uses `rust-analyzer`.** With `analyzers.scip.enabled: true` and
  `rust-analyzer` on `PATH`, archfit runs `rust-analyzer scip` to add symbol-level
  strength on top of the crate-level `cargo metadata` graph. When the binary is
  absent the pass no-ops cleanly — no error, `coupling_balance` confidence stays
  at whatever the type-info heuristic provides.
- **Rust module depth needs `cargo-modules`.** A single crate is one node at crate
  level, so cycle/blast-radius/encapsulation go `n/a`. `analyzers.cargo_modules.enabled: true`
  (with `cargo install cargo-modules --version 0.26.0`) adds the `<crate>::<mod>` graph;
  archfit then maps per-file LOC to module keys so size metrics cover individual
  modules rather than entire crates. On a workspace, crates whose `cargo-modules`
  run fails (proc-macro/codegen) are named in the coverage reason and confidence
  drops to medium — partial, never silent.
- **Clone detection is opt-in.** `analyzers.clones.enabled: true` plus `jscpd`
  (`npm install -g jscpd@5.0.11`) upgrades cross-module clone pairs to
  `StrengthSymmetric` (S=9) in the BC scorer. Clone-only pairs with no import
  edge enter `coupling_balance` by default under `coupling.duplicated_knowledge: score`.
  `false` or absent → no clone signal.
- **Strength precision is language-asymmetric (bc_score.v6).** Go classifies
  `const`/`var` uses as `model` (pure-data sharing) and pure-data DTO structs
  crossing a declared `public:` boundary as `contract` directly from compiler
  type info. Rust gets const/static→model precision via `analyzers.scip`
  (rust-analyzer terms). Python and TypeScript built-in extractors stay
  conservative, but SCIP symbol-kind metadata can refine interface/protocol/trait
  references to `contract`, concrete data symbols to `model`, and functions to
  `functional`. None of the non-Go paths emit a DTO upgrade — see
  [Concepts → the balance rule](concepts.md#the-balance-rule).

See [Install → optional analysis tools](install.md#optional-analysis-tools) for
quick setup and [Tooling reference](tooling.md) for platform-specific package
manager choices, versions, home pages, and PATH checks.

## Mixed repositories

For a repo with more than one language, keep each language's paths in distinct
modules where possible:

```yaml
languages:
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
