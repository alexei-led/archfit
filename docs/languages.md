# Languages and tools

Archfit builds a dependency graph from Go, TypeScript and JavaScript, Python, and Rust projects. Each language uses its normal project tools.

Language adapters run when project markers and required tools are present. Set `languages.<language>.enabled: true` to require an adapter, or `false` to skip it. See [configuration](configuration.md) for all options.

## Language support

| Language | Project marker | Required tool | Graph detail | Missing or incomplete input |
| --- | --- | --- | --- | --- |
| Go | `go.mod` or a supported `go.work` workspace | `go` | File-to-package imports | Load errors produce partial evidence. Synthetic error packages are skipped. |
| TypeScript and JavaScript | Root `package.json` | `npx` or `bunx`, plus `dependency-cruiser` | File-to-file imports | Without the root marker, the adapter is absent. Tool errors produce partial evidence. |
| Python | `pyproject.toml`, `setup.py`, or `languages.python.package` | `uv`, or Python 3.12+ with `grimp` installed | Dotted module imports | Without a package marker, the adapter is absent. Import scan errors produce partial evidence. |
| Rust | Root `Cargo.toml`, or configured `languages.rust.manifest` | `cargo` | Crate dependencies | Cargo errors produce partial evidence. A one-crate graph has no internal crate edges. |

Optional tools add syntax evidence, clone evidence, or more precise coupling evidence. Their absence does not stop the analysis. See [metrics](metrics.md) for metric meaning and limits.

### Go

Archfit loads packages with `go/packages` and emits file-to-package imports. It reads `go.work` when present. It otherwise uses a root `go.mod`, then searches below the analysis root for modules.

Package loading uses the host `GOOS`, `GOARCH`, default build tags, and tags in `GOFLAGS`. Files excluded by those settings are not parsed. Run archfit under each platform or tag set that matters. `_test.go` files are not loaded.

For cgo files, archfit maps preprocessed imports back to the original file. If preprocessing fails, archfit reads imports from the original file and marks the package load partial.

Use `languages.go.modules` to select workspace members. Use `exclude` for files that must not count, such as ignored generators. See [configuration](configuration.md).

### TypeScript and JavaScript

Archfit runs `bunx depcruise` when `bunx` is available. Otherwise, it runs `npx depcruise`. The `dependency-cruiser` package must be available to the selected runner. A project-local development dependency gives repeatable results.

The adapter skips Node.js core modules and `node_modules` directories at any depth. Without a root `package.json`, TypeScript files outside that project do not enter dependency rule scope. Point `--root` at the TypeScript project to analyze it.

### Python

Archfit detects `uv` first and runs `grimp` in a temporary environment. Use Python 3.11 or later with `uv`. Without `uv`, archfit uses Python 3.12 or later and expects `grimp` to be installed.

Set `languages.python.package` when the package name differs from the repository directory name or when the project uses a `src/` layout. Module names and selectors use dotted names, such as `myapp.service`, rather than file paths.

Use `uv` for Python analysis. It resolves `grimp` in the temporary environment without a separate global install.

```sh
uv --version
```

### Rust

Archfit runs `cargo metadata --format-version 1 --no-deps`. Each workspace member becomes a crate node. Registry dependencies become external nodes. Dev-dependencies are skipped unless `languages.rust.include_dev_deps` is `true`.

The default graph is crate-level. One crate alone has no internal dependency edges. Set `languages.rust.manifest` to use a non-root manifest or to require Rust analysis when no root manifest exists. Set `languages.rust.features` to pass Cargo features to metadata discovery.

`archfit config init` and `archfit config update --apply` add Rust deep-analysis defaults when they find a root `Cargo.toml`. These defaults enable `analyzers.cargo_modules` and `analyzers.scip`.

`cargo-modules` adds intra-crate module nodes and `uses` edges. `rust-analyzer` produces SCIP symbols for more precise edge strength. Install `cargo-modules` to use the first pass. Install the Rust Analyzer component to use the second. `uv` is also required to read SCIP indexes. Missing tools create coverage gaps and do not fail the run.

Rust selectors use crate names from `Cargo.toml`, such as `grep-cli`. With `cargo-modules`, module names use Rust crate identifiers, such as `grep_cli::args`.

## Analyzer tools

Enable optional tools under `analyzers`. These tools report absent or partial evidence when unavailable or when a run fails.

| Tool | Evidence | Required program |
| --- | --- | --- |
| `ast-grep` (`sg`) | Structural patterns and embedded syntax rules | `sg` executable from ast-grep |
| `scip` | Language-specific symbol and edge-strength evidence | `scip-go`, `scip-typescript`, `scip-python`, or `rust-analyzer`, plus `uv` |
| `clones` | Duplicate-code evidence | `jscpd` |
| `cargo_modules` | Rust intra-crate module graph | `cargo-modules` |

SCIP indexers are language-specific. Install the matching indexer for the language under analysis. TypeScript SCIP needs installed project dependencies to resolve imports. Archfit invokes tools found on `PATH`.

The Docker image includes `ast-grep` and `dependency-cruiser`. It does not include Rust tools, SCIP indexers, or `jscpd`. See the repository [Dockerfile](../Dockerfile) for the current image contents.

Use `archfit doctor` to inspect tool discovery. A missing optional tool leaves its dependent evidence unavailable. A missing required language tool makes that language's evidence incomplete or causes the required-tool check to fail.

## Supplied test coverage

Archfit reads coverage artifacts from the test job. It does not run test commands. Enable supplied coverage with the top-level `coverage` configuration. See [configuration](configuration.md) for accepted formats and the freshness sidecar. See [metrics](metrics.md) for how coverage affects metric results.

| Language | Example producer | Artifact | Format |
| --- | --- | --- | --- |
| Go | `go test ./... -coverprofile=coverage.out` | `coverage.out` | `go-coverprofile` |
| TypeScript or JavaScript | `npx c8 --reporter=lcov --reports-dir=coverage npm test` | `coverage/lcov.info` | `lcov` |
| Python | `uv run --with pytest-cov python -m pytest --cov=. --cov-report=json:coverage.json` | `coverage.json` | `coverage-py-json` |
| Rust | `cargo llvm-cov --lcov --output-path coverage.lcov` | `coverage.lcov` | `lcov` |

These commands show common producers. Keep the test runner already used by the repository when it emits a supported format. Vitest and Jest can emit LCOV directly. Rust can also use `cargo llvm-cov --json --summary-only --output-path coverage.json` with `llvm-cov-json`.

Write `<artifact>.sidecar.json` in the same CI step. The sidecar must list declared module identities and lowercase SHA256 hashes for every represented source file. `source_ref` is metadata. It does not replace source hashes. The producer must enumerate the complete set of covered files.

Coverage formats use different units. Go and Python use statements. LCOV and LLVM JSON use lines. Archfit does not combine incompatible units. Unit conflicts keep testability partial.

Archfit normalizes paths under the analysis root and reports unresolved paths. Every declared module must have complete, fresh supplied coverage. Language selects the parser and unit. It does not create another completeness denominator.

## File classification

Archfit classifies source files as `Production`, `Test`, `Generated`, or `Vendor`. This affects source counts and structural metrics. Project-specific patterns can extend built-in classification through `file_class` settings. See [configuration](configuration.md).

| Language | Test patterns | Generated patterns |
| --- | --- | --- |
| Go | `*_test.go` | `*.pb.go`, `*_gen.go`, `mock_*.go`, `*_mock.go`, `mocks/`, or a `Code generated ... DO NOT EDIT` header |
| TypeScript and JavaScript | `*.test.ts`, `*.spec.ts`, `*.test.tsx`, `*.spec.tsx` | `*.gen.ts` or a recognized mock header |
| Python | `test_*.py`, `*_test.py` | `_pb2.py` or a `Code generated` header |
| Rust | `tests.rs`, `*_test.rs`, `*_tests.rs`, `tests/`, `benches/`, `*_tests/`, `*-tests/` | `*.gen.rs` or a recognized mock header |

`vendor/`, `node_modules/`, and `pkg/mod/` are vendor paths. Other project conventions can use `test_globs` and `generated_globs`.

Classification is file-level. An inline Rust `#[cfg(test)]` module remains part of its production file. Move tests to a separate file or narrow the rule that matches them.
