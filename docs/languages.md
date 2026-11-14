# Languages and tools

Archfit builds a dependency graph from Go, TypeScript and JavaScript, Python, Rust, and .NET projects. Optional report adapters use project tools for more detail.

Language adapters run when project markers and required tools are present. Set `languages.<language>.enabled: true` to require an adapter, or `false` to skip it. See [configuration](configuration.md) for all options.

## Language support

| Language | Project marker | Required tool | Graph detail | Missing or incomplete input |
| --- | --- | --- | --- | --- |
| Go | `go.mod` or a supported `go.work` workspace | None for imports; optional `go` for report strength hints | File-to-package imports | Without `go`, the report reads imports with the built-in parser and discloses missing type precision. Unknown inputs remain partial. |
| TypeScript and JavaScript | Source files | `ast-grep` | File-to-file imports | Unresolved first-party imports, unread configuration, and parser failures produce incomplete evidence. |
| Python | A flat or `src/` package with `__init__.py`, or `languages.python.package` | `ast-grep` | Dotted module imports | Without a package, the adapter is absent. Unresolved first-party imports and parser failures produce incomplete evidence. |
| Rust | Root `Cargo.toml`, or configured `languages.rust.manifest` | `cargo` | Crate dependencies | Cargo errors produce partial evidence. A one-crate graph has no internal crate edges. |
| .NET (C#, SDK-style projects, net8.0 and later) | `languages.dotnet.projects`, else a `.slnx` or `.sln` at the root, else any `.csproj` outside `obj/`, `bin/`, and `exclude` | `dotnet` SDK; the Roslyn helper for the type graph | Project graph from MSBuild evaluation; file-to-file type references with compiler-resolved strength hints; `internal` use across assemblies as intrusive hints | Projects that do not evaluate or load are reported `failed` or `partial` with the cause. Unresolved symbols keep coupling from `measured`. See [.NET](#net). |

Optional tools add syntax evidence, clone evidence, or more precise coupling evidence. Their absence does not stop the analysis. See [metrics](metrics.md) for metric meaning and limits.

## Before you run

The TypeScript and Python check and report use `ast-grep` with a built-in resolver, not the project's runtime or installed dependencies. Other report adapters can use project tooling. Archfit reports what it could not read; it never installs dependencies into your repository.

| Language | Do this first | Configure when auto-detection is wrong |
| --- | --- | --- |
| Go | a `go.mod` (or a supported `go.work`); install `go` and resolve dependencies only for typed report detail | `languages.go.modules` to select workspace members, `exclude` for generated trees |
| TypeScript / JavaScript | `git` and `ast-grep`; project dependencies are not needed for imports | `exclude` for generated code |
| Python | `git` and `ast-grep`; no Python interpreter or installed project dependencies | `languages.python.package` to select one package instead of discovering flat or `src/` packages |
| Rust | a root `Cargo.toml` and `cargo` | `languages.rust.manifest`, `languages.rust.features`, `languages.rust.include_dev_deps` |
| .NET | the `dotnet` SDK on `PATH`, and `ARCHFIT_DOTNET_ROSLYN` set to the absolute path of the built Roslyn helper, outside the target (`make dotnet-roslyn`). Restore NuGet packages (`dotnet restore`) in the environment that runs archfit; archfit never restores, and unrestored package symbols leave projects `partial`. Network is optional, but without it MSBuild SDKs resolved through NuGet cannot load | `languages.dotnet.projects` to select solutions or projects, `analyzers.roslyn.enabled: false` for the project-only graph (map and report only; .NET rule gates need the helper, which the image ships) |

The shipped image contains Node, Python, Rust, the .NET SDK with the Roslyn helper, ast-grep and cargo-modules, at the versions pinned in `tools/manifest.json`. Go is build-only: the final image has no Go executable or SDK. Its Go check and report read imports with the built-in parser; optional typed Go report detail needs a host installation of `go`.

## When evidence is absent, disabled, or partial

Every analyzer reports one status per tool in `coverage.tools[]`. The status tells you what is left for you to do:

| Status | Meaning | What to do |
| --- | --- | --- |
| `ok` | the tool ran and its evidence was complete | nothing |
| `absent` | the tool does not apply to this repository's languages, or its project marker is missing | check the marker for that language |
| `disabled` | the analyzer is opt-in and your config did not enable it | set `analyzers.<name>.enabled: true` when you want that evidence (`coverage.enabled: true` for supplied coverage) |
| `partial` | the tool ran, but its evidence is incomplete | read `reason`: it names the missing input and the command or key that fixes it |
| `timed out` | the pass exceeded its budget | raise `analyzers.<name>.timeout`, or narrow the scope |

The partial results people hit first:

- **TypeScript / JavaScript.** A `tsimports` partial row names unresolved imports or unread `tsconfig` inputs. Fix the import or make `extends` a relative path inside the repository. Package-based `extends` is not read, even if dependencies are installed; affected bare imports remain unknown.
- **.NET.** `N of M projects partial, K failed, U unresolved symbols` on the `roslyn` row means packages or reference packs are missing: restore the projects in the environment that runs archfit, or install the targeting pack the reason names. `MSB4236` on the `dotnet-msbuild` row means an MSBuild SDK resolved through NuGet could not be fetched, whether `global.json` `msbuild-sdks` lists it or the project pins it inline (`<Project Sdk="Aspire.AppHost.Sdk/13.6.0">`): run with NuGet access. `NETSDK1147: workload X not installed` means a MAUI, Android, or iOS project needs an SDK workload: run `dotnet workload install X` on the host, or exclude the project. `NETSDK1178` means the workload packs do not exist for this host's OS: analyze on a host that has them, or exclude the project.

### Go

`archfit check` always reads Go imports with the built-in standard parser. It never runs the typed loader or the Go toolchain, even when `go` is installed, and needs no installed module dependencies.

`archfit report` uses `go/packages` when `go` is on `PATH`, adding strength hints from `go/types`. Without `go`, it uses the same parser as the check and still emits file-to-package imports and workspace module information. The `go/packages` coverage row then discloses the parser fallback and missing type precision; it does not claim typed strength. An installed toolchain that fails to load packages still produces a load error or partial evidence, not a parser retry.

Both paths read `go.work` when present, otherwise a root `go.mod`, then search below the analysis root for modules. They respect `GOOS`, `GOARCH`, build constraints, and tags in `GOFLAGS`. Run archfit under each platform or tag set that matters. `_test.go` files are not read for imports.

The parser reports unparseable source and unresolved first-party imports as unknown input. A source read failure returns an error. If `CGO_ENABLED` is unset, it also reports cgo-dependent file selection as unknown rather than guessing. The typed loader maps preprocessed cgo imports back to the original file; if preprocessing fails, it reads imports from that file and marks the load partial.

Use `languages.go.modules` to select workspace members. Use `exclude` for files that must not count, such as ignored generators. See [configuration](configuration.md).

### TypeScript and JavaScript

Both `archfit check` and `archfit report` use `ast-grep` and the built-in `tsimports` resolver. Neither needs Node, a TypeScript compiler, `node_modules`, or a root `package.json`. The report's `tsimports` coverage row records the parser version. Type-only imports count.

The resolver reads relative paths, supported `tsconfig` paths and baseUrl, package imports, and workspace exports. It uses the nearest `tsconfig.json`, or the root `tsconfig.base.json` when no `tsconfig.json` exists. It does not read package-based `extends`, configuration outside the root, or `node_modules`. An unread config or unresolved first-party import stays unknown, with a location; it cannot establish complete coverage. Unknown imports can hide edges and cap report confidence. Project dependency-cruiser configuration is not executed. See [evidence](evidence.md) for limits.

### Python

Both `archfit check` and `archfit report` read Python imports through `ast-grep` and the built-in `pyimports` resolver. They need `git` and `ast-grep`, not Python, `uv`, grimp, or installed project dependencies. The report's `pyimports` coverage row records the parser version. Target Python environments, initializers and Grimp caches are not used or executed.

Leave `languages.python.package` unset to discover top-level packages containing `__init__.py`; a `src/` layout wins over a flat layout. Configure a package to select it explicitly, including a namespace root without `__init__.py`. A configured package that does not exist is missing evidence, not an absent language. Packaging metadata alone does not select source. Loose scripts outside selected packages remain unscanned.

**Namespace-package change in 4.0:** directories without `__init__.py` inside selected packages now count as namespace-package modules (PEP 420). The old grimp adapter skipped them. This adds their imports to both the check and report; the equivalence corpus added 142 dependencies on prefect (design appendix D.3).

Report module identities are dotted names, such as `myapp.service`, and every import keeps its source file and line. The check uses resolved file paths. Relative imports and `from package import name` use the same resolver: `package.name` is selected when it is a module, otherwise `package`. Imports inside functions and `try/except` count; imports guarded by `TYPE_CHECKING` do not.

Standard-library and third-party imports are external. A first-party module without a file, or a relative import above its package, is unresolved: the report keeps a low-confidence dotted edge and partial coverage with the file, line and module; the check emits an unknown item. Neither path silently drops it. Excluded files are not read, but can still resolve as targets. Selecting one package does not prove that other packages were scanned. Unscanned source remains missing evidence, not proof of a clean tree.

The report does not infer symbol/type strength from import spelling. Duplicate bodies for one dotted module, such as `app.py` and `app/__init__.py`, do not prove both source files were scanned.

CI checks the src-layout `testdata/python314` regression and verifies that grimp is absent from the image.

### Rust

Archfit runs `cargo metadata --format-version 1 --no-deps`. Each workspace member becomes a crate node. Registry dependencies become external nodes. Dev-dependencies are skipped unless `languages.rust.include_dev_deps` is `true`.

The default graph is crate-level. One crate alone has no internal dependency edges. Set `languages.rust.manifest` to use a non-root manifest or to require Rust analysis when no root manifest exists. Set `languages.rust.features` to pass Cargo features to metadata discovery.

The optional `cargo-modules` tool adds intra-crate module nodes and `uses` edges. Missing tools create coverage gaps.

Rust selectors use package names from `Cargo.toml`, such as `grep-cli`. With cargo-modules,
module names use target crate identifiers, such as `grep_cli::args`; custom lib/bin target names
can differ from the package name. Cover both vocabularies when the policy uses both graph levels.
Bench/example-only targets can lack a supported module graph; read the named coverage gap.

### .NET

Archfit selects projects from `languages.dotnet.projects`, else the first `.slnx` or `.sln` at the root (by file name), else every `.csproj` outside `obj/`, `bin/`, `.git/`, `node_modules/`, and `exclude`. A configured solution filter (`.slnf`, such as the `eShop.Web.slnf` a CI job builds) selects the projects it lists from its base solution; archfit never picks a filter on its own, and a filter whose base solution is missing makes the `dotnet-msbuild` row partial. It evaluates each project on the original tree with `dotnet msbuild -getItem -getProperty`: once, then once per entry of `TargetFrameworks`, because a conditional reference appears only in the per-framework run. The Roslyn helper (`archfit-roslyn`) then loads the projects on a snapshot without compiling and emits file-to-file type references. Archfit never runs `dotnet restore` and never asks the helper to. Non-C# projects in a solution, such as `.sqlproj`, are skipped and counted in coverage detail.

Node IDs are root-relative slash paths, as for Go and TypeScript. A file edge is located at the first use of the target in the source file (the earliest across projects and frameworks), as a Go or Python edge is located at its import; a fallback project edge is located at the `ProjectReference` `Include` line of the file that declares it, 0 when no `Include` names the target literally. Project nodes use the `.csproj` path. Namespaces are not graph nodes. Symbols from referenced assemblies (BCL, restored packages) become `external:<assembly>` nodes; a rule selector names the assembly, not the node ID. Unresolved symbols are counted, never turned into nodes. Files under `obj/` or `bin/` and symbols declared outside the root produce no edges. A file compiled into several projects through a linked `Compile Include` is one node with the union of its edges; it has no module unless a module glob covers it.

A dependency rule's `to:` matches an external node by its assembly name: `Microsoft.EntityFrameworkCore*`, `MediatR.Contracts`, or a brace glob such as `{Microsoft.EntityFrameworkCore*,Microsoft.AspNetCore*}`. The assembly name is usually the NuGet package ID, but not the namespace a `using` names: `using MediatR;` resolves to the `MediatR.Contracts` assembly when only that package is referenced. A dotted target is an external dependency ban; its last segment is never read as a file extension. A `to:` spelled with the node's `external:` prefix matches nothing, and `config lint` reports it: write `to: Npgsql`, not `to: external:Npgsql`.

Declare modules with globs. `archfit config init` writes one module per project directory, named after the project file, and drops a module whose directory holds no production source (for example a project under `test/`). It writes `languages.dotnet.enabled: auto` and `analyzers.roslyn.enabled: true`. `archfit config update --apply` adds `analyzers.roslyn.enabled: true` when the setting is implicit.

Coupling strength comes from compiler symbols. The extractor emits hints and does not classify; `internal/relationship` decides strength with the precedence in [coupling](coupling.md).

| Reference from file A to a symbol in file B | Hint |
| --- | --- |
| Implements or uses an interface, an abstract class, or an abstract member | `contract` |
| Names a concrete type, or reads or writes its field, constant, or auto-property | `model` |
| Names a source record or struct that has data members and no behavior, or reads its members | `dto` |
| Calls a method, event, constructor, indexer, or a property with a body on a concrete type | `functional` |
| Inherits a concrete class | `functional` |
| Uses a symbol that is, or is nested in, `internal` or `private protected`, declared in another assembly | `intrusive` |

The strongest use wins per file pair. Data uses (`model`, `dto`) also travel in a separate data channel, which lifts a `public:` target from `contract` to `model` for a concrete type. An `intrusive` hint lifts a `public:` target to `intrusive`: an `internal` constructor in a file under a `public:` glob is still not part of the published surface. A `dto` stays `contract` across a `public:` glob and reads `model` elsewhere. Known gap: invoking a delegate stored in a field counts as a data use.

Only compiler-resolved type references become edges. Published-language coupling between services is not measured: integration-event records duplicated per service, a `.proto` file that another project compiles directly (eShop `WebApp` compiles `Basket.API`'s), and HTTP, gRPC, or YARP calls resolved at runtime produce no type edge. See [coupling](coupling.md#copies-and-limits).

The `intrusive` hint feeds the `encapsulation` metric. `InternalsVisibleTo` declarations are reported as coverage detail only; a declaration is not a usage edge.

#### Project status

The helper classifies each project and target framework from workspace and compilation diagnostics, and one target framework's outcome never decides another's. Archfit folds the frameworks: a project is `complete` only when every framework is, `failed` only when every framework failed, and `partial` otherwise. Its file edges are the union of the frameworks that loaded. The union drops which framework contributed an edge: an edge is reported for its project, not for `net8.0` or `net10.0`, because the graph has one node per file.

| Status | Condition | Edges |
| --- | --- | --- |
| `complete` | The framework loaded its documents, every name resolved, and the compilation has no error. | File edges. |
| `partial` | The framework loaded its documents, but a name is unresolved (missing NuGet packages or reference packs) or the compilation has an error (the file keeps only the edges it resolved). | File edges for resolved symbols. Coverage names the counts. |
| `failed` | The framework loaded no source documents: its evaluation produced no `Compile` items, or the workspace could not add it. | None from the helper. See the fallback below. |

`failed` is measured, not assumed: MSBuildWorkspace keeps a project whose evaluation failed as an empty project. A framework that loaded documents is measured by its own compilation; a workspace failure for the project file is reported as that framework's cause in the `roslyn` row, but it does not hold a framework whose compilation is clean, because the failure names the project, not the framework its evaluation belongs to. A failure that loses evidence leaves no documents, which is `failed`.

A multi-targeted project also has a framework-less evaluation, and MSBuild's cross-targeting build for it defines no `Compile` target, so it loads no documents. The helper omits it: it measures nothing, and its failure says nothing about the frameworks, which load separately. A project whose evaluation fails before its frameworks are known is reported `failed` with that cause.

Two causes keep a framework `partial` with their own reason instead of a restore. A file whose symbol walk threw, which an incomplete reference set does (Roslyn's nullable analysis over error types), is named with the exception; the project's other files keep their edges. A project whose `.razor` files the design-time compilation does not generate (the Razor source generator runs inside a build) names Razor and states that those files' edges are unmeasured; list the other projects in `languages.dotnet.projects` to drop it, or accept the gap.

The `dotnet-msbuild` row names the evaluation cause and fix, for example `MSB4236: SDK 'Bitwarden.Server.Sdk' not resolvable offline; run with NuGet access and a warm cache` or `global.json requests SDK X, which is not installed; install a matching SDK or relax the pin`. The `roslyn` row reads `N of M projects partial, K failed, U unresolved symbols` and names every such project with its cause, as the `dotnet-msbuild` row names every project that did not evaluate. The report keeps one shortened line per row; the run prints the whole reason on stderr (`warning: roslyn is partial; full analyzer output ...`). Both rows also carry a `details` list, one line per project: the `dotnet-msbuild` row gives its status and the frameworks MSBuild evaluated, the `roslyn` row its status, frameworks, distinct files and unresolved count. The table stays readable in JSON when the reason is shortened.

When the helper itself fails, for example on an unsatisfiable `global.json`, it writes one `archfit-roslyn: …` line to stderr and exits 1, or 2 for an unexpected error (`ARCHFIT_ROSLYN_DEBUG=1` adds the trace). The `roslyn` row is then `partial` with `archfit-roslyn failed: <that line>`, and every project falls back to its project graph. A project whose MSBuild evaluation also failed has no project edges to fall back to; the row says how many projects fall back, or that none does.

#### Fallback to the project graph

One source project contributes one scored edge set. When the helper loaded a project, only its file edges are emitted. When the helper is absent, disabled, failed, or timed out for a project, and MSBuild evaluation supplied its `ProjectReference` items, only those project edges are emitted. A project that fails both has no edges and is reported as missing inventory.

Fallback limits:

- Project edges have unknown strength. The scorer abstains unless a policy glob supplies strength.
- Selectors match a project edge by the `.csproj` path: a directory glob such as `src/Core/**` matches it, a glob for files below it (`src/Core/Api/**`) does not.
- Edge counts become project pairs for fallback projects.
- Incomplete file edges keep report dimensions partial or unmeasured. Coverage reasons name each project's cause. Repository errors and unread places use parser-based check facts, not Roslyn. See [evidence](evidence.md#net-trust-limits).
- The `roslyn` row reads `partial` when the helper failed for a project or the whole run, `timed out` on a timeout, `absent` with a coverage gap when `ARCHFIT_DOTNET_ROSLYN` is unset, and `disabled` without a gap when switched off. A `partial` or `timed out` row counts as missing inputs, not reduced precision.
- Edges from test projects are dropped, both fallback and file edges, as Rust drops dev-dependencies. Their files stay as nodes. A test project is one that evaluates `IsTestProject=true`, or whose project path has a `test/` or `tests/` segment (the .NET test-file rule in [file classification](#file-classification)): offline, the test SDK props under `obj/` may not be imported and `IsTestProject` evaluates empty. A project outside such a segment and without the property, such as `src/Foo.Tests/Foo.Tests.csproj`, keeps its edges.

A `ProjectReference` with no type use is not an edge when the project has file edges. It is counted as unused in coverage detail, and only when the project is `complete`. Only direct references count: the `ProjectReference` items MSBuild evaluated for the project, not the transitive references the SDK adds during the design-time build. A reference with `ReferenceOutputAssembly=false` or `IsAspireProjectResource=true` (an Aspire AppHost resource) compiles against nothing, so it is never unused.

Razor is the case measured on eShop: `src/WebApp` and `src/WebhookClient` are the only two partial projects of the web solution, both for the source generator that runs inside a build. Any generator that runs only as a build target behaves the same, such as Protobuf in a project that does not commit the generated files. Archfit does not run a build, so it names the generator instead of guessing at a restore; the way out is to shrink the measured project set with `languages.dotnet.projects` and accept those edges as unmeasured, or to analyze a tree where the generated sources already exist.

#### Snapshot and trust

Evaluation runs the project's `Directory.Build.props`, imported targets, and the props of NuGet-resolved SDKs. The Roslyn design-time build also runs prerequisite targets that write generated files (global usings, assembly info) under `obj/<Configuration>/<tfm>/`. Both execute code from the target repository. This is the same trust class as `go/packages` and cgo. See [evidence](evidence.md#net-trust-limits).

- Every `dotnet` call archfit makes sets `DOTNET_CLI_TELEMETRY_OPTOUT=1`, `DOTNET_NOLOGO=1`, `DOTNET_SKIP_FIRST_TIME_EXPERIENCE=1`, `DOTNET_GENERATE_ASPNET_CERTIFICATE=false`, `DOTNET_ADD_GLOBAL_TOOLS_TO_PATH=false`, and `MSBUILDDISABLENODEREUSE=1`: no telemetry, no ASP.NET development certificate or tool-path first-run state, and no MSBuild node left running. NuGet still writes its default user `NuGet.Config` under `DOTNET_CLI_HOME` when none exists.
- Evaluation runs on the original tree. On `bitwarden/server` all 86 projects evaluated with `git status --ignored` clean. That is a measurement for one repository, not a guarantee.
- The helper runs on a disposable copy of the analysis root in the system temp directory. The copy skips `.git` and `node_modules` at any depth and keeps `obj/`, `bin/`, and files matched by `exclude`, because they can be build inputs. With `UseArtifactsOutput` or `ArtifactsPath`, restore output lives in `artifacts/obj/<project>/`; the copy keeps it too, so a restored project stays `complete` (measured on `jasontaylordev/CleanArchitecture`, which sets `ArtifactsPath`). `exclude` applies to the emitted evidence, after analysis.
- The copy also takes the build inputs (`Directory.Build.props`, `Directory.Build.targets`, `Directory.Build.rsp`, `Directory.Packages.props`, `global.json`, `NuGet.config`) of every directory between the analysis root and the repository root. Build inputs above the repository root are not copied; the `roslyn` detail names them when any exist. A project that would inherit one from above the repository root is `partial` and its reason names the file(s), because the snapshot compiles without whatever it sets: its file edges are not proven complete, so the affected report measurements stay incomplete. A build input in the repository tree that MSBuild would read first shadows the ancestor file, except `NuGet.config`, which layers. A file that a build input imports from outside the copied tree, such as `../build/common.props` in a subtree scan, is not copied.
- A symlink that points inside the analysis root is redirected into the copy. One that points elsewhere is not reproduced: the copy drops it, so the design-time build creates a real path inside the copy instead of writing through the link into the original tree. The `roslyn` reason names every dropped link, and the project whose directory holds it is `partial`; a dropped link no project directory holds makes the row `partial`.
- Archfit maps paths back to the original and deletes the copy on success, error, timeout, and cancellation.
- With `--base`, the base tree is a Git checkout of tracked files, so it has no restore outputs. Each base project takes the restore outputs of the project at the same path in the working tree: the files directly in its `MSBuildProjectExtensionsPath` (`project.assets.json`, the `*.nuget.g.props` and `*.nuget.g.targets` imports), never the build output below it. They are copied into the base copy only; the working tree is read, never written. A project with no restore outputs in the working tree at that path stays without them, and the `roslyn` reason names it and the fix (`dotnet restore` in the working tree). The borrowed outputs describe the working tree's packages, so archfit checks that each borrowing project restores the same way: it generates NuGet's restore graph for the base copy (`dotnet msbuild -t:GenerateRestoreGraphFile`, no restore) and compares it with the `<project>.nuget.dgspec.json` the working tree's restore wrote, for the project and every project it references. Compared are package references, central package versions (`Directory.Packages.props`), package sources and the central-management switches; `global.json`, `NuGet.config`, `Directory.Packages.props`, `Directory.Build.props`, and `Directory.Build.targets` from the project's directory up to the root are compared byte for byte. Paths and machine-bound fields (packages folder, user config paths, audit settings, SDK install path) are not. A project whose inputs differ, or cannot be compared (no `.dgspec.json`, a restore made at another path, a graph that does not generate), is `partial`, and the `roslyn` reason names it, so the comparison is non-comparable; a version bump or `global.json` change between base and head does that for every affected project. Project references are not compared: the helper reads them from the base project file. The `roslyn` reason also says how many projects reused restore outputs. Go modules, Python environments, and the cargo registry need no copy: both trees read the same cache. A base worktree sits inside the repository, so `node_modules` resolves from the working tree.

#### SDK and reference packs

The image ships one SDK band, 10.0.x. Which SDK a host uses is the host's choice:

- A target `global.json` that pins an SDK this host cannot satisfy makes its projects `failed`, with a reason that names `global.json`.
- The SDK carries only its own reference pack. A `net8.0` project resolves its framework references only if the 8.0 targeting pack is installed (`Microsoft.NETCore.App.Ref` 8.x). In the image contract (offline, one SDK, empty NuGet cache), a `net8.0` target is `partial` and the reason names the missing pack. On a developer machine with that pack, it can be `complete`.
- MSBuild SDKs listed in `global.json` `msbuild-sdks` (for example `Bitwarden.Server.Sdk`), or pinned with a version in the project's `Sdk` attribute (for example `Sdk="Aspire.AppHost.Sdk/13.6.0"`), are fetched by the SDK resolver when the network is reachable. That is SDK behavior, not an archfit restore. Offline, each unresolvable SDK costs about 11 s per evaluation, and the project is `failed` with `MSB4236`.
- To reproduce a cold offline machine, empty both NuGet caches, not only `NUGET_PACKAGES`. The HTTP cache keeps downloaded `.nupkg` files and serves them offline, so an SDK fetched once still resolves. Point `NUGET_PACKAGES` and `NUGET_HTTP_CACHE_PATH` at empty directories. On Linux and macOS the HTTP cache defaults to `~/.local/share/NuGet/http-cache`, or sits under `DOTNET_CLI_HOME` when that is set; `dotnet nuget locals all --list` prints both paths.

#### Timeouts

Evaluation runs four projects at a time, with a 5 minute limit per `dotnet msbuild` call. A project whose evaluation times out is `failed`, and the `dotnet-msbuild` row reads `timed out`. The snapshot copy and the helper share `analyzers.roslyn.timeout`, 20 minutes by default. A helper timeout reads `timed out` on the `roslyn` row and every project falls back to its project graph; it is never a silent truncation. On a repository with many NuGet-resolved SDK projects, expect a long offline run: on `bitwarden/server`, 16 unresolvable projects took 193 s to evaluate, and a full offline `archfit analyze` took 749 s against 89 s with network. Raise `analyzers.roslyn.timeout` for larger repositories, or run with network access and a warm NuGet cache.

#### Install the helper

The image builds the helper into `/opt/archfit/dotnet-roslyn` and sets `ARCHFIT_DOTNET_ROSLYN`. For a host CLI, build it from a source checkout and point the variable at the output:

```sh
make dotnet-roslyn
export ARCHFIT_DOTNET_ROSLYN="$PWD/.bin/dotnet-roslyn"
```

The build restores the helper's own locked dependencies. Archfit never restores a target. The helper runs as `dotnet <dir>/archfit-roslyn.dll` and needs an SDK that satisfies the target's `global.json`. Its `--version` line, `archfit-roslyn <version>; Microsoft.CodeAnalysis <version>; SDK <version>`, is the `roslyn` row version.

`ARCHFIT_DOTNET_ROSLYN` must be an absolute directory outside the analysis root that holds `archfit-roslyn.dll`. To analyze the archfit checkout itself, copy `.bin/dotnet-roslyn` outside it first.

| Setting | `archfit doctor` | Analysis |
| --- | --- | --- |
| Unset | `missing`, with this hint | `roslyn` row `absent`, with a coverage gap for `coupling_balance` and `encapsulation`; project graph only, so file-level report measurements are incomplete |
| Relative, or without `archfit-roslyn.dll` | `invalid`; doctor exits non-zero | No .NET graph; the `dotnet-msbuild` row is `partial` and names the variable |
| Inside the analysis root | not checked: doctor has no target | No .NET graph; the `dotnet-msbuild` row is `partial` and names the variable |
| Any value, with `analyzers.roslyn.enabled: false` | as above | `roslyn` row `disabled`, no gap; project graph only, so file-level report measurements are incomplete |

For a found SDK and helper, `archfit doctor` prints the SDK version and the helper's `--version` line. It runs both in the current directory, as analysis runs them in the target, so a `global.json` there selects the SDK. When that pin cannot be satisfied, both rows are `invalid`, the reason names the `global.json` and the requested SDK, and doctor exits non-zero. Run doctor from the repository root to check its pin.

#### Route rules and C# 14

With `analyzers.syntax` enabled, the ast-grep C# rules find ASP.NET route attributes (`[HttpGet]`, `[HttpPost]`, `[HttpPut]`, `[HttpDelete]`, `[HttpPatch]`, `[Route]`) and minimal API registrations: `MapGet`, `MapPost`, `MapPut`, `MapDelete`, `MapPatch`, `MapMethods`, `MapFallback`, and `MapIdentityApi<TUser>`. `MapGroup` is a prefix, not a route. Feature endpoints such as `MapHealthChecks`, `MapHub`, `MapControllers`, and `MapOpenApi` are not counted. A registration without a literal path template reports no route: bare `[HttpGet]`, `[HttpGet("")]`, `[HttpGet(Name = ...)]`, `[Route]` without an argument, `MapFallback()` without a pattern, and `MapIdentityApi<TUser>()`. An empty template adds no path segment of its own — under attribute routing the class `[Route]` is the route, and that attribute is reported as its own route fact at its declaration line — and a verb name is not a path. A `Map*` call inside a method with the same verb name is a forwarding wrapper, such as an extension `MapGet` that calls the framework `MapGet`; its callers are the routes, so the call in the wrapper is not counted. The same pass records methods, constructors, and local functions that have a body, for the function-length metrics in the complexity dimension. A method's extent starts at its first attribute line. Lambdas and property accessors are not counted, as arrow functions are not in TypeScript. A route a test, generated, or vendored file registers is a test fixture or codegen artifact, not a production entry point, so it is not reported; declaration facts from those files are. C# function facts are not public API facts: visibility depends on the enclosing types and `InternalsVisibleTo`, which a syntax match cannot see. A file whose syntax tree has `ERROR` nodes makes the `ast-grep/syntax` row `partial`; the reason names up to five files with the first error line. Files matched by `exclude` do not count. The C# 14 `extension` block parses as an error, so any non-excluded file that has one makes the row `partial`. So does a `#if` directive that splits a statement, such as `#if !WINDOWS` around one call of a fluent chain (eShop `src/ClientApp/MauiProgram.cs`): the grammar does not preprocess, and which branch compiles depends on the build's symbols. Routes elsewhere in that file and in other files are still reported. Archfit cannot fix either case; exclude such a file when it holds no routes, or accept the `partial` row.

#### Cobertura coverage

Use coverlet and select format `cobertura`:

```sh
dotnet test --collect "XPlat Code Coverage"
```

The report lands in `TestResults/<guid>/coverage.cobertura.xml`. `coverage.sources[].path` is a fixed path, so copy the report to a stable location in the same CI step. `format: auto` detects it from the `.xml` extension and the `<coverage>` root element. Archfit reads lines. It joins each `filename` with every `<source>` (an absolute `filename` is used as is) and takes the first join that resolves under the analysis root, root-relative with `/`. A `<source>` is often `/` or a directory above the repository. A join inside the root that does not resolve counts as unresolved. Files whose every join lies outside the root, such as NuGet cache sources, are dropped and not counted as unresolved. When no file of the report lies inside the root, the report was produced in another checkout: every file counts as unresolved, and the coverage row reason names one of the paths, so the other root is visible. A file that appears in several `<class>` entries of one report is merged per line, taking the highest hit count. A file covered by two reports counts once in the coverage row. A UTF-8 BOM is accepted.

A project with no coverable lines, such as one that holds only constants, gets no `<class>` entry from coverlet. Its module stays in the "missing coverage for declared modules" list and testability stays partial. It is not counted as 0% covered. Go cover profiles behave the same way for a package with no statements.

## Analyzer tools

Enable optional tools under `analyzers`. `roslyn` is the exception: it is on unless `enabled: false`, and runs when `ARCHFIT_DOTNET_ROSLYN` names the helper. These tools report absent or partial evidence when unavailable or when a run fails.

| Tool | Evidence | Required program |
| --- | --- | --- |
| `ast-grep` | Structural patterns and embedded syntax rules | `ast-grep` executable |
| `cargo_modules` | Rust intra-crate module graph | `cargo-modules` |
| `dotnet-msbuild` | .NET project graph from MSBuild evaluation. Reported in `coverage.tools[]`; configured by `languages.dotnet`, not under `analyzers` | `dotnet` SDK |
| `roslyn` | .NET type-level file graph and coupling strength | `dotnet` SDK and the `archfit-roslyn` helper (`ARCHFIT_DOTNET_ROSLYN`) |

The release image includes the listed tools. Optional execution is controlled by `analyzers`. The [tool manifest](../tools/manifest.json) and dependency locks own image and CI versions. See the [Dockerfile](../Dockerfile) for provisioning.

Use `archfit doctor` to inspect tool discovery. A missing optional tool leaves its dependent evidence unavailable. A missing required language tool makes that language's evidence incomplete or causes the required-tool check to fail.

## Install analyzers

The complete release image is the simplest fixed tool environment. The tool versions are:

| Tool | Image version |
| --- | --- |
| Node / Python / uv | 24.22.1 / 3.14.9 / 0.13.5 |
| Go (build-only; absent from the final image) | 1.27.4 |
| .NET SDK | 10.0.402 |
| archfit-roslyn / Microsoft.CodeAnalysis | 1.1.0 / 5.6.0 |
| ast-grep | 0.51.0 |
| Rust, Cargo, rust-src | 1.99.1 |
| cargo-modules | 0.28.0 |

For a host CLI, use a separate owned tool directory outside every target checkout. From this
release's source checkout, `make host-tools` installs the Node lock without target
project hooks. It uses `~/.local/share/archfit-tools` unless you set `HOST_TOOLS_DIR`:

```sh
make host-tools
prefix="$HOME/.local/share/archfit-tools"
export PATH="$prefix/node/node_modules/.bin:$PATH"
```

That `PATH` line puts the locked `sg` ahead of your own. Link only the tools you need if you prefer not to change `PATH`.

For host Rust/Go semantic tools, install the manifest versions using the
official platform toolchains (`rust-src` is required). `scripts/ci-provision-tools.sh` is Linux
build/CI provisioning, not a portable runtime installer. Never install target dependencies as part
of the Python import producer. Runtime tool discovery does not certify the complete environment.

The image installs analyzers before use and disables npm/uv tool bootstrap. Cargo target
dependency resolution may still use network access. Host reports using the typed Go loader may
also resolve dependencies over the network. Optional indexers/build scripts may execute
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
| .NET | `*Tests.cs`, `*Test.cs`, or a `test/` or `tests/` path segment in any case, such as `Tests/`. A project directory such as `Foo.Tests/` is not a test segment; add a `file_class.test_globs` entry such as `**/*.Tests/**`. |

Generated filename checks apply across languages, before test-file patterns.
The classifier marks a filename as generated when it meets any of these conditions:

- It ends in `.pb.go`, `.d.ts`, `.d.mts`, or `.d.cts`.
- It starts with `gen_` or `mock_`.
- Its name before the final extension ends in `_gen`, `_mock`, `_moq`, `.gen`, or `_pb2`.
- It ends in `.g.cs`, `.g.i.cs`, or `.Designer.cs`, in any case.

For example, `mock_client.py`, `client_gen.rs`, and `client.gen.go` are generated files under this policy.
A header containing both `Code generated` and `DO NOT EDIT` marks a file as generated in any language. A `<auto-generated` header does the same for C# only: other languages do not use that marker, and a file that merely carries the literal — a generator template, or a test that mentions it — is not generated. Archfit reads the first 512 bytes.
A C# file under an `obj/` or `bin/` directory, at any depth, is generated; that includes `artifacts/obj/` and `artifacts/bin/` under `UseArtifactsOutput`. Other languages keep source under `bin/`.
The `mocks/` and `__mocks__/` path segments also identify generated files. For .NET, `mocks/` matches in any case, such as `Mocks/`.
Configured generated globs and mock filename patterns take precedence over test patterns.

The classifier marks files under a `vendor/` path segment as `Vendor`.
The source-count walk skips dependency directories such as `vendor/`, `node_modules/`, and the root `pkg/mod/` cache, and C# files under `obj/` or `bin/`.
Skipping a directory does not assign the `Vendor` class to every file there.
Use `test_globs` and `generated_globs` for other project conventions.

Classification is file-level. An inline Rust `#[cfg(test)]` module remains part of its production file. Move tests to a separate file or narrow the rule that matches them.
