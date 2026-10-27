# Languages and tools

Archfit builds a dependency graph from Go, TypeScript and JavaScript, Python, Rust, and .NET projects. Each language uses its normal project tools.

Language adapters run when project markers and required tools are present. Set `languages.<language>.enabled: true` to require an adapter, or `false` to skip it. See [configuration](configuration.md) for all options.

## Language support

| Language | Project marker | Required tool | Graph detail | Missing or incomplete input |
| --- | --- | --- | --- | --- |
| Go | `go.mod` or a supported `go.work` workspace | `go` | File-to-package imports | Load errors produce partial evidence. Synthetic error packages are skipped. |
| TypeScript and JavaScript | Root `package.json` | `npx` or `bunx`, plus `dependency-cruiser` | File-to-file imports | Without the root marker, the adapter is absent. Tool errors produce partial evidence. |
| Python | `pyproject.toml`, `setup.py`, or `languages.python.package` | `uv`, or Python 3.14+ with `grimp==3.17` installed | Dotted module imports | Without a package marker, the adapter is absent. Source or graph extraction gaps produce partial evidence. |
| Rust | Root `Cargo.toml`, or configured `languages.rust.manifest` | `cargo` | Crate dependencies | Cargo errors produce partial evidence. A one-crate graph has no internal crate edges. |
| .NET (C#, SDK-style projects, net8.0 and later) | A `.slnx` or `.sln` at the root, or any `.csproj` | `dotnet` SDK; Roslyn helper for the type graph | Project graph per target framework; file-to-file type references with compiler-resolved coupling strength; `internal` use across assemblies as intrusive hints; LOC, test and generated classification, route rules, Cobertura coverage | Projects that do not evaluate or load are reported `failed` or `partial` with the cause. Unresolved symbols keep coupling from `measured`. See [.NET](#net). |

Optional tools add syntax evidence, clone evidence, or more precise coupling evidence. Their absence does not stop the analysis. See [metrics](metrics.md) for metric meaning and limits.

## Before you run

Each adapter drives the project's own tooling. Archfit reuses what is installed and reports what it could not read; it never installs dependencies into your repository.

| Language | Do this first | Configure when auto-detection is wrong |
| --- | --- | --- |
| Go | a `go.mod` (or a supported `go.work`) and a working `go` | `languages.go.modules` to select workspace members, `exclude` for generated trees |
| TypeScript / JavaScript | `npm ci` (or `pnpm install` / `yarn`) in the project root, the workspace root for a monorepo, so `node_modules` and the `tsconfig` `paths`/`baseUrl` resolve | `analyzers.scip` for symbol-level strength, `exclude` for generated code |
| Python | a package marker (`pyproject.toml` or `setup.py`) and Python 3.14+ with `grimp==3.17` | `languages.python.package` when the importable package name is not obvious |
| Rust | a root `Cargo.toml` and `cargo` | `languages.rust.manifest`, `languages.rust.features`, `languages.rust.include_dev_deps` |
| .NET | the `dotnet` SDK on `PATH`, and `ARCHFIT_DOTNET_ROSLYN` naming the built Roslyn helper (`make dotnet-roslyn`). Network is optional, but without it MSBuild SDKs resolved through NuGet cannot load | `languages.dotnet` to select solutions or projects, `analyzers.roslyn.enabled: false` for the project-only graph |

The shipped image already contains every tool, at the versions pinned in `tools/manifest.json`: Go, Node with dependency-cruiser and TypeScript, Python with grimp, Rust with rust-analyzer, the .NET SDK with the Roslyn helper, ast-grep, jscpd, scip indexers, and cargo-modules. Running archfit from the image removes the "install the analyzer" failure mode entirely; what remains is the target repository's own prerequisites (the second and third columns).

## When evidence is absent, disabled, or partial

Every analyzer reports one status per tool in `coverage.tools[]`. The status tells you what is left for you to do:

| Status | Meaning | What to do |
| --- | --- | --- |
| `ok` | the tool ran and its evidence was complete | nothing |
| `absent` | the tool does not apply to this repository's languages, or its project marker is missing | check the marker for that language |
| `disabled` | the analyzer is opt-in and your config did not enable it | set `analyzers.<name>.enabled: true` when you want that evidence |
| `partial` | the tool ran, but its evidence is incomplete | read `reason`: it names the missing input and the command or key that fixes it |
| `timeout` | the pass exceeded its budget | raise `analyzers.<name>.timeout`, or narrow the scope |

The two partial results people hit first:

- **TypeScript / JavaScript.** `N of M import specifiers unresolved` means the target's dependencies are not installed: install them in the project root (`npm ci`, `pnpm install`, or `yarn`; the workspace root for a monorepo), then check `tsconfig` `paths`/`baseUrl`. `scip` stays `absent` until `node_modules` exists, because scip-typescript resolves packages through it.
  `dependency-cruiser environment incomplete` means the analyzer environment has no compatible TypeScript compiler: run archfit from the shipped image, or install TypeScript 5-6 next to dependency-cruiser.
- **Rust.** `Rust target attribution partial` means rust-analyzer saw symbols it cannot pin to exactly one Cargo target (macros, generated code, several targets). Crate-level structure is still measured. Narrow `languages.rust.manifest` to the workspace you care about, or set `analyzers.scip.enabled: false` to rely on crate-level coupling only.

### Go

Archfit loads packages with `go/packages` and emits file-to-package imports. It reads `go.work` when present. It otherwise uses a root `go.mod`, then searches below the analysis root for modules.

Package loading uses the host `GOOS`, `GOARCH`, default build tags, and tags in `GOFLAGS`. Files excluded by those settings are not parsed. Run archfit under each platform or tag set that matters. `_test.go` files are not loaded.

For cgo files, archfit maps preprocessed imports back to the original file. If preprocessing fails, archfit reads imports from the original file and marks the package load partial.

Use `languages.go.modules` to select workspace members. Use `exclude` for files that must not count, such as ignored generators. See [configuration](configuration.md).

### TypeScript and JavaScript

Archfit invokes an installed dependency-cruiser entry through an explicitly resolved Node interpreter.
Both must be outside the target project and Git root. Project-local executables, package-manager
launchers, Node preload hooks, and `.dependency-cruiser.*` configuration do not select or configure
the judge. Supported data-only `tsconfig` resolution still applies. Type-only TypeScript imports
count as dependencies.

The supported image pair is dependency-cruiser 18.7.0 with TypeScript 6.0.3. TypeScript 7 is not
supported by that producer. Missing compilers, unreadable source inventories, omitted eligible
files, and unusable graphs leave required rules unevaluated; an empty graph is not proof of no
violations. A genuinely empty input tree remains distinct.

The adapter skips Node.js core modules and `node_modules` directories at any depth. Without a root `package.json`, TypeScript files outside that project do not enter dependency rule scope. Point `--root` at the TypeScript project to analyze it.

### Python

The image sets `ARCHFIT_TOOL_PYTHON` to its installed isolated Python environment.
An explicit setting must name a valid absolute interpreter outside the target; invalid settings
fail visibly rather than falling back. Without that setting, local CLI execution detects `uv`
and selects Python 3.14 with `grimp==3.17` in an isolated environment. Project uv configuration and interpreter requests do not override this selection. Without `uv`, put a Python 3.14+ virtual environment containing `grimp==3.17` on `PATH`. Keep that tool environment separate from the target repository.

Leave `languages.python.package` unset for automatic discovery of flat or `src/` packages. Pin it only when that single package is the complete intended source. Selecting one of several discovered roots leaves the others unscanned. Module names and selectors use dotted names, such as `myapp.service`, rather than file paths.

Project dependencies do not need to be installed. The analyzer runs outside the target directory with an isolated environment; target virtual environments, `PYTHONPATH`, and user-site packages are ignored. Repository Grimp caches are neither read nor written. The producer binds selected packages to repository source without executing package initializers. Namespace roots, separate service roots, source symlinks, and package names that collide with loaded Python modules leave evidence incomplete.

Third-party imports become low-confidence external literals. Explicit first-party targets absent from the tree remain low-confidence dotted dependencies. These record source statements, without verifying target files, symbols, or runtime availability. Dependency bans still match them, including imports inside `try/except`. Missing generated version modules need no special exception. `from package import name` can name an attribute, so an absent `name` is not invented as a submodule.

Unreadable, unparseable, or unscanned source leaves evidence incomplete and required rules unevaluated. A known first-party package omitted by package selection is unscanned input even when selected files do not import it. Automatic discovery can select all roots. Literal targets do not count as scanned files. Standard-library imports and existing `TYPE_CHECKING` guard handling remain outside the inventory.

Loose Python files outside the scanned packages do not become verified source. Rules whose selectors include them remain unevaluated, including broad selectors that also match scanned files. Rules limited to scanned packages can still evaluate. A whole-project cycle rule therefore needs every in-scope Python file covered by the package scan.

If multiple files map to the same dotted module, such as `app.py` and `app/__init__.py`, their bodies are ambiguous and do not count as verified source. Excluded files do not contribute to that ambiguity. A rule whose target vocabulary cannot contain Python nodes does not wait for omitted Python source.

The v3.1.1 image supplies Python 3.14. CI checks the src-layout `testdata/python314` fixture; releases also check each immutable platform image before publishing image version tags. Images through v3.1.0 contain Python 3.11; caller-side Python setup cannot change the container interpreter. See [migration](migration.md#python-producer-semantics) for baseline compatibility.

Use `uv` to resolve the analyzer without a global install:

```sh
uv --version
```

### Rust

Archfit runs `cargo metadata --format-version 1 --no-deps`. Each workspace member becomes a crate node. Registry dependencies become external nodes. Dev-dependencies are skipped unless `languages.rust.include_dev_deps` is `true`.

The default graph is crate-level. One crate alone has no internal dependency edges. Set `languages.rust.manifest` to use a non-root manifest or to require Rust analysis when no root manifest exists. Set `languages.rust.features` to pass Cargo features to metadata discovery.

`archfit config init` and `archfit config update --apply` add Rust deep-analysis defaults when they find a root `Cargo.toml`. These defaults enable `analyzers.cargo_modules` and `analyzers.scip`.

`cargo-modules` adds intra-crate module nodes and `uses` edges. `rust-analyzer` produces SCIP symbols for more precise edge strength. Install `cargo-modules` to use the first pass. Install the Rust Analyzer component to use the second. `uv` is also required to read SCIP indexes. Missing tools create coverage gaps and do not fail the run.

Rust selectors use package names from `Cargo.toml`, such as `grep-cli`. With cargo-modules,
module names use target crate identifiers, such as `grep_cli::args`; custom lib/bin target names
can differ from the package name. Cover both vocabularies when the policy uses both graph levels.
SCIP joins use Cargo target metadata. When rust-analyzer cannot distinguish multi-target symbols,
Archfit abstains and reports partial attribution instead of inventing a module identity.
Bench/example-only targets can lack a supported module graph; read the named coverage gap.

### .NET

Archfit evaluates each C# project with `dotnet msbuild -getItem -getProperty`, once per project and once per target framework, and reads `.slnx` and `.sln` membership. The Roslyn helper (`archfit-roslyn`) then loads the projects without compiling and emits file-to-file type references. Archfit never runs `dotnet restore` and never asks the helper to. Non-C# projects in a solution, such as `.sqlproj`, are skipped and counted in coverage detail.

Node IDs are repo-relative slash paths, as for Go and TypeScript. Project nodes use the `.csproj` path. Namespaces are not graph nodes. Declare modules with globs; `archfit config init` writes one module per evaluated project directory. A file compiled into several projects through a linked `Compile Include` is one node with the union of its edges.

Coupling strength comes from compiler symbols. The extractor emits hints and does not classify; `internal/relationship` decides strength with the precedence in [coupling](coupling.md).

| Reference from file A to a symbol in file B | Hint |
| --- | --- |
| Implements or uses an interface or abstract type | `contract` |
| Uses a concrete type, field, property value, or constant as data only | `model` |
| Uses a record or struct with no behavior as data only | `dto` |
| Calls a method, property, event, or constructor of a concrete type | `functional` |
| Inherits a concrete class | `functional` |
| Uses a symbol that is `internal` or `private protected`, declared in another assembly | `intrusive` |

The `intrusive` hint feeds the `encapsulation` metric. `InternalsVisibleTo` declarations are reported as coverage detail only; a declaration is not a usage edge.

#### Project status

The helper classifies each project from workspace and compilation diagnostics. A multi-targeting project takes the worst status across its target frameworks, and its edges are the union.

| Status | Condition | Edges |
| --- | --- | --- |
| `complete` | The project loaded and no symbol is unresolved. | File edges. |
| `partial` | The project loaded, but some symbols are unresolved (missing NuGet packages or reference packs). | File edges for resolved symbols. Coverage names the counts. |
| `failed` | The project loaded with 0 source documents, and the workspace reports a failure for its path. | None from the helper. |

`failed` is measured, not assumed: MSBuildWorkspace keeps a project whose evaluation failed as an empty project. A workspace failure on a project that did load its documents is `partial`. A `Tests` project that references a broken project can therefore be `partial`, not `failed`. The reason names the cause and the fix, for example `MSB4236: SDK 'Bitwarden.Server.Sdk' not resolvable offline; run with NuGet access and a warm cache` or `global.json requests SDK X, which is not installed; install a matching SDK or relax the pin`.

An unsatisfiable `global.json` makes the helper exit with an error that names `global.json`, and the run reports no helper evidence.

#### Fallback to the project graph

One source project contributes one scored edge set. When the helper loaded a project, only its file edges are emitted. When the helper is absent, disabled, or failed for a project, and MSBuild evaluation supplied its `ProjectReference` items, only those project edges are emitted. A project that fails both has no edges and is reported as missing inventory.

Fallback limits:

- Project edges have unknown strength. The scorer abstains unless a policy glob or an approved label supplies strength.
- File selectors (`public:`, `internal:` file globs) do not match project edges.
- Edge counts become project pairs for fallback projects.
- The coverage row stays `partial`. Missing inputs, not reduced precision, is the reason.
- References from test projects (`IsTestProject=true` or a test-classified path) are dropped from the fallback.

A `ProjectReference` with no type use is not an edge when the project has file edges. It is counted as unused in coverage detail, and only when the project is `complete`.

#### Snapshot and trust

Evaluation runs the project's `Directory.Build.props`, imported targets, and the props of NuGet-resolved SDKs. The Roslyn design-time build also runs prerequisite targets that write generated files (global usings, assembly info) under `obj/<Configuration>/<tfm>/`. Both execute code from the target repository. This is the same trust class as `go/packages` and cgo. See [evidence](evidence.md#net-trust-limits).

- Evaluation runs on the original tree. On `bitwarden/server` all 86 projects evaluated with `git status --ignored` clean. That is a measurement for one repository, not a guarantee.
- The helper runs on a disposable copy of the analysis root, without `.git` and `node_modules/`. The copy keeps `obj/`, `bin/`, and files matched by `exclude`, because they can be build inputs. Archfit maps paths back to the original and deletes the copy, also on error and timeout. `exclude` applies to the emitted evidence, after analysis.

#### SDK and reference packs

The image ships one SDK band, 10.0.x. Which SDK a host uses is the host's choice:

- A target `global.json` that pins an SDK this host cannot satisfy makes its projects `failed`, with a reason that names `global.json`.
- The SDK carries only its own reference pack. A `net8.0` project resolves its framework references only if the 8.0 targeting pack is installed (`Microsoft.NETCore.App.Ref` 8.x). In the image, which is offline with an empty NuGet cache, a `net8.0` target is `partial` and the reason names the missing pack. On a developer machine with that pack, it can be `complete`.
- MSBuild SDKs listed in `global.json` `msbuild-sdks` (for example `Bitwarden.Server.Sdk`, `Aspire.AppHost.Sdk`) are fetched by the SDK resolver when the network is reachable. That is SDK behavior, not an archfit restore. Offline, each unresolvable SDK costs about 11 s per evaluation, and the project is `failed` with `MSB4236`.

#### Timeouts

Evaluation runs in parallel, with a 5 minute limit per project. The helper runs under `analyzers.roslyn.timeout`. A timeout is `failed` for the run, not a silent truncation. On a repository with many NuGet-resolved SDK projects, expect a long offline run: on `bitwarden/server`, 16 unresolvable projects took 193 s to evaluate, and a full offline helper load took 676 s against 72 s with network. Raise `analyzers.roslyn.timeout` for such repositories, or run with network access and a warm NuGet cache.

#### Install the helper

The image builds the helper into `/opt/archfit/dotnet-roslyn` and sets `ARCHFIT_DOTNET_ROSLYN`. For a host CLI, build it from a source checkout and point the variable at the output:

```sh
make dotnet-roslyn
export ARCHFIT_DOTNET_ROSLYN="$PWD/.bin/dotnet-roslyn"
```

The build restores the helper's own locked dependencies. Archfit never restores a target. The helper needs the `dotnet` SDK at run time. An unset variable leaves the type graph `absent`, and `archfit doctor` reports the helper missing with this hint. An invalid path fails visibly.

#### Route rules and C# 14

The `ast-grep` C# rules find ASP.NET route attributes (`[HttpGet]`, `[HttpPost]`, `[Route]`) and `MapGet`, `MapPost`, `MapPut`, `MapDelete` calls. A file that the grammar cannot fully parse is `partial`, and the reason names up to five files with the first error line. The C# 14 `extension` block parses as an error, so any file that has one is `partial`.

#### Cobertura coverage

Use coverlet and select format `cobertura`:

```sh
dotnet test --collect "XPlat Code Coverage"
```

The report lands in `TestResults/<guid>/coverage.cobertura.xml`. Archfit reads lines. It resolves each `filename` against every `<source>` (an absolute `filename` is used as is), takes the first join under the analysis root, and makes it root-relative with `/`. A `<source>` is often `/` or a directory above the repository. Files whose every join lies outside the root, such as NuGet cache sources, are dropped and not counted as unresolved. A file that appears in several `<class>` entries or reports is merged per line, taking the highest hit count. A UTF-8 BOM is accepted.

## Analyzer tools

Enable optional tools under `analyzers`. `roslyn` is the exception: it is on by default when the helper is installed. These tools report absent or partial evidence when unavailable or when a run fails.

| Tool | Evidence | Required program |
| --- | --- | --- |
| `ast-grep` | Structural patterns and embedded syntax rules | `ast-grep` executable |
| `scip` | Language-specific symbol and edge-strength evidence | `scip-go`, `scip-typescript`, `scip-python`, or `rust-analyzer`, plus `uv` |
| `clones` | Duplicate-code evidence | `jscpd` |
| `cargo_modules` | Rust intra-crate module graph | `cargo-modules` |
| `dotnet-msbuild` | .NET project graph from MSBuild evaluation. Reported in `coverage.tools[]`; not configured under `analyzers` | `dotnet` SDK |
| `roslyn` | .NET type-level file graph and coupling strength | `dotnet` SDK and the `archfit-roslyn` helper (`ARCHFIT_DOTNET_ROSLYN`) |

SCIP indexers are language-specific. Install the matching indexer for the language under analysis. TypeScript SCIP needs a data-only TypeScript project and installed target dependencies to resolve imports. For JavaScript, provide a `tsconfig.json` with `allowJs: true`. Archfit invokes tools found on `PATH`.

The v3.2 image includes all listed tools. Optional execution is still controlled by `analyzers`;
installation alone does not enable it. Generated Rust configurations already enable cargo-modules
and SCIP. The [tool manifest](../tools/manifest.json) and dependency locks own the image/CI versions.
See the [Dockerfile](../Dockerfile) for native SDK and runtime provisioning.

Use `archfit doctor` to inspect tool discovery. A missing optional tool leaves its dependent evidence unavailable. A missing required language tool makes that language's evidence incomplete or causes the required-tool check to fail.

Tool discovery does not execute the pinned Python analyzer environment. When uv's Python downloads are disabled, provide Python 3.14 separately and read the analysis coverage to confirm that the producer ran.

## Install analyzers

The complete release image is the simplest fixed tool environment. The tool versions are:

| Tool | Image version |
| --- | --- |
| Go / Node / Python / uv | 1.27.3 / 24.22.1 / 3.14.9 / 0.13.5 |
| dependency-cruiser / TypeScript compiler | 18.7.0 / 6.0.3 |
| .NET SDK | 10.0.402 |
| ast-grep / jscpd | 0.51.0 / 5.4.2 |
| scip-go / scip-typescript / scip-python | 0.2.9 / 0.5.2 / 0.6.8 |
| Rust, Cargo, rust-src, bundled rust-analyzer | 1.99.1 |
| cargo-modules / Grimp | 0.28.0 / 3.17 |

Use the Rust component listed here, not the newer standalone rust-analyzer with a different SCIP
namespace. SCIP Python has an older semantic core: a nonempty index does not guarantee complete
Python 3.14 type/grammar support. Import-rule evidence comes from Grimp, not SCIP.

For a host CLI, use a separate owned tool directory outside every target checkout. From this
release's source checkout, install the Node and Python locks without target project hooks:

```sh
prefix="$HOME/.local/share/archfit-tools"
mkdir -p "$prefix/node"
cp tools/node/package.json tools/node/package-lock.json "$prefix/node/"
npm ci --prefix "$prefix/node" --no-audit --no-fund
uv venv --python 3.14.9 "$prefix/python"
uv pip sync --python "$prefix/python/bin/python" --require-hashes --only-binary :all: tools/python/requirements.lock
export PATH="$prefix/node/node_modules/.bin:$PATH"
export ARCHFIT_TOOL_PYTHON="$prefix/python/bin/python"
```

Provide a compatible owned Node interpreter on `PATH`; do not launch analysis through a target
package-manager script. For host Rust/Go semantic tools, install the manifest versions using the
official platform toolchains (`rust-src` is required). `scripts/ci-provision-tools.sh` is Linux
build/CI provisioning, not a portable runtime installer. Never install target dependencies as part
of the Python import producer. Runtime tool discovery does not certify the complete environment.

The image installs analyzers before use and disables npm/uv tool bootstrap. Go/Cargo target
dependency resolution may still use network access. Optional indexers/build scripts may execute
target code and send mounted source outward; no secret-free or image-digest claim removes that risk.
App reports are customer-attested measurements, not remote proof of execution.

## Supplied test coverage

Archfit reads coverage artifacts from the test job. It does not run test commands. Enable supplied coverage with the top-level `coverage` configuration. See [configuration](configuration.md) for accepted formats and the freshness sidecar. See [metrics](metrics.md) for how coverage affects metric results.

| Language | Example producer | Artifact | Format |
| --- | --- | --- | --- |
| Go | `go test ./... -coverprofile=coverage.out` | `coverage.out` | `go-coverprofile` |
| TypeScript or JavaScript | `npx c8 --reporter=lcov --reports-dir=coverage npm test` | `coverage/lcov.info` | `lcov` |
| Python | `uv run --with pytest-cov python -m pytest --cov=. --cov-report=json:coverage.json` | `coverage.json` | `coverage-py-json` |
| Rust | `cargo llvm-cov --lcov --output-path coverage.lcov` | `coverage.lcov` | `lcov` |
| .NET | `dotnet test --collect "XPlat Code Coverage"` | `TestResults/<guid>/coverage.cobertura.xml` | `cobertura` |

These commands show common producers. Keep the test runner already used by the repository when it emits a supported format. Vitest and Jest can emit LCOV directly. Rust can also use `cargo llvm-cov --json --summary-only --output-path coverage.json` with `llvm-cov-json`.

Write `<artifact>.sidecar.json` in the same CI step. The sidecar must list declared module identities and lowercase SHA256 hashes for every represented source file. `source_ref` is metadata. It does not replace source hashes. The producer must enumerate the complete set of covered files.

Coverage formats use different units. Go and Python use statements. LCOV, LLVM JSON, and Cobertura use lines. Archfit does not combine incompatible units. Unit conflicts keep testability partial.

Archfit normalizes paths under the analysis root and reports unresolved paths. Every declared module must have complete, fresh supplied coverage. Language selects the parser and unit. It does not create another completeness denominator.

## File classification

Archfit classifies source files as `Production`, `Test`, `Generated`, or `Vendor`. This affects source counts and structural metrics. Project-specific patterns can extend built-in classification through `file_class` settings. See [configuration](configuration.md).

| Language | Test patterns |
| --- | --- |
| Go | `*_test.go` |
| TypeScript and JavaScript | Any filename containing `.test.` or `.spec.`, or a `__tests__/` path segment |
| Python | `test_*.py`, `*_test.py`, or a `tests/` path segment |
| Rust | `tests.rs`, `*_test.rs`, `*_tests.rs`, or a `tests/`, `benches/`, `*_tests/`, `*-tests/` path segment |
| .NET | `*Tests.cs`, `*Test.cs`, or a `test/` or `tests/` path segment |

Generated filename checks apply across languages, before test-file patterns.
The classifier marks a filename as generated when it meets any of these conditions:

- It ends in `.pb.go`, `.d.ts`, `.d.mts`, or `.d.cts`.
- It starts with `gen_` or `mock_`.
- Its name before the final extension ends in `_gen`, `_mock`, `_moq`, `.gen`, or `_pb2`.
- It ends in `.g.cs`, `.g.i.cs`, or `.Designer.cs`, or sits under `obj/` or `bin/`.

For example, `mock_client.py`, `client_gen.rs`, and `client.gen.go` are generated files under this policy.
A header containing both `Code generated` and `DO NOT EDIT` marks a file as generated. A `<auto-generated` header does the same.
The `mocks/` and `__mocks__/` path segments also identify generated files.
Configured generated globs and mock filename patterns take precedence over test patterns.

The classifier marks files under a `vendor/` path segment as `Vendor`.
The source-count walk skips dependency directories such as `vendor/`, `node_modules/`, `obj/`, `bin/`, and the root `pkg/mod/` cache.
Skipping a directory does not assign the `Vendor` class to every file there.
Use `test_globs` and `generated_globs` for other project conventions.

Classification is file-level. An inline Rust `#[cfg(test)]` module remains part of its production file. Move tests to a separate file or narrow the rule that matches them.
