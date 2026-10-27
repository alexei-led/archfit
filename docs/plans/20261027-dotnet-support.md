# .NET language support

## Overview

Add modern cross-platform .NET (C#, SDK-style projects, .NET 8+, Linux and macOS) as a fifth
analyzed language, at the depth Rust has today and with compiler-resolved coupling strength.

Delivered in this plan (phase 1, all of it):
- project graph from MSBuild evaluation: `ProjectReference`, per target framework, `.slnx`/`.sln` membership
- type-level file graph and coupling strength from a bundled Roslyn helper
- intrusive hints from compiler-enforced accessibility (`internal` use across assemblies, D3), feeding the existing `encapsulation` metric
- .NET SDK in the main image, pinned and checksum-verified like Go, Node, Python and Rust
- C# LOC, generated and test-file classification, ast-grep route rules, clone detection through jscpd
- Cobertura coverage parsing (coverlet output)
- config init, config update, doctor, and the language contracts harness
- acceptance run on `bitwarden/server`

Cut, not deferred (no metric consumes them, or they need a build):
- `obj/project.assets.json` transitive packages: no metric reads transitive packages.
- Aspire AppHost deploy units: needs a build for the manifest. Revisit with the `deployunit` owner.
- CA1506 coupling counts: counts only, no edges, needs a full build.
- scip-dotnet (thin maintenance, restores by default, indexes one TFM; the helper already yields strength hints), language servers (heavy stdio drivers), NDepend (commercial), Metrics.exe (Windows only).

Reverse any cut by asking; it is a one-line change to this plan.

## Context (from discovery)

Measured on `bitwarden/server` at `0850efddd`, cloned to `~/Workspace/bitwarden-server`:

| Fact | Value | Consequence |
| --- | --- | --- |
| Project files | 86 `.csproj`, all `net10.0` from `Directory.Build.props` | Single-TFM testbed; multi-TFM still required for other repos |
| Solution | one `.slnx`, no `.sln` | `.slnx` parser is required |
| Central Package Management | no `Directory.Packages.props` | Package versions come from each csproj |
| `global.json` | SDK `10.0.103` `latestFeature`; `msbuild-sdks` pins `Bitwarden.Server.Sdk` 1.8.0 and `Aspire.AppHost.Sdk` 13.3.4 | SDK pin must be satisfiable by the image SDK; NuGet-resolved SDKs need network |
| Offline `dotnet msbuild -getItem:ProjectReference` | 70 of 86 evaluate; 16 fail with `MSB4236` (15 `Bitwarden.Server.Sdk`, 1 `Aspire.AppHost.Sdk`) | "No restore needed" holds for SDK-only projects, not for the repo; failures must be `partial` with the SDK named |
| `InternalsVisibleTo` | `src/Core/AssemblyInfo.cs` has 4 attributes; `src/Core/Core.csproj` has 2 items (`Infrastructure.IntegrationTest`, `Pam`) | Declarations live in both places; the intrusive hint uses symbol accessibility instead (D3) |
| Evaluation side effects | `git status --ignored` clean after evaluating all 86 projects | Evaluation writes nothing into the target. Roslyn design-time build is still unverified |
| CODEOWNERS | 93 rules | Ownership input is real for the testbed |

### Spike results (Task 1, 2026-10-27)

Measured on the host (macOS arm64, Homebrew SDK 10.0.402, Roslyn `Microsoft.CodeAnalysis.Workspaces.MSBuild` 5.6.0, `Microsoft.Build.Locator` 1.11.2, ast-grep 0.51.0).

| Spike | Result |
| --- | --- |
| Roslyn load, offline (empty `NUGET_PACKAGES`, network blocked) | `OpenSolutionAsync` opens the `.slnx` directly. 86 C# projects: 10 `complete`, 60 `partial`, 16 `failed` (the 16 `MSB4236` projects). Symbols: 1,019,701 resolved, 1,457,568 unresolved, 7 `Inaccessible` candidates. 676 s wall-clock, of which 627 s is loading; max RSS 2.1 GB |
| Roslyn load, networked | The SDK resolver fetched only `Bitwarden.Server.Sdk` 1.8.0 and `Aspire.AppHost.Sdk` 13.3.4 (6.6 MB). 10 `complete`, 76 `partial`, 0 `failed`. 72 s wall-clock, max RSS 2.2 GB |
| Offline cost | Each failed SDK resolution costs about 11 s (NuGet retries per evaluation, also when a failed project is re-evaluated as a reference). The extractor and the helper need a timeout that covers this; the image contract runs offline |
| Snapshot writes | Only under `<project>/obj/<Configuration>/<tfm>/`: `*.GlobalUsings.g.cs`, `*.AssemblyInfo.cs`, `*.AssemblyInfoInputs.cache`, `.NETCoreApp,Version=v10.0.AssemblyAttributes.cs`, `*.GeneratedMSBuildEditorConfig.editorconfig`, plus `*.RazorAssemblyInfo.cs` for Razor projects. 350 files offline, 436 networked. Nothing in `bin/`, no restore files. Outside the snapshot: NuGet creates `$DOTNET_CLI_HOME/.nuget/NuGet/NuGet.Config` |
| Original clone | `git status --porcelain --ignored` identical (empty) before, after the offline run, and after the networked run |
| Cross-assembly `internal` use | Networked: 2,575 uses, all resolve directly through the generated IVT attribute (the design-time build writes `InternalsVisibleTo` from csproj items into `AssemblyInfo.cs`). 0 real internal uses went through `CandidateReason.Inaccessible`; the 20 `Inaccessible` candidates were `protected` members and internals of compiled assemblies |
| Evaluation, networked | All 16 `MSB4236` projects evaluate: 9 s cold, 11 s warm. Offline the same 16 fail in 193 s. `Microsoft.Build.Traversal` and `Microsoft.Build.Sql` in `global.json` are never fetched (no project uses them) |
| MSBuild JSON | Two or more `-getProperty`/`-getItem` flags print `{"Properties":{...},"Items":{...}}`; a single `-getProperty` prints the bare value. Unset properties are `""`. A multi-TFM project has `TargetFramework` `""` and `TargetFrameworks` `"net8.0;net10.0"`. Item `Identity` is as written (relative); use `FullPath` |
| Cobertura (coverlet.collector 6.0.0) | One `<source>`, the common ancestor of all instrumented files: `/` or a directory above the repo, not the repo root. `filename` is relative to it, forward slashes. One file appears in many `<class>` entries (nested and generated classes). Reports include files outside the repo (NuGet cache `Program.cs`). The file starts with a UTF-8 BOM. Output lands in `TestResults/<guid>/coverage.cobertura.xml` |
| ast-grep `csharp` | 6,134 files in 4.6 s; 6 files have `ERROR` nodes (23 nodes): the C# 14 `extension` block (2 files, 18 nodes), `#if` splitting a statement (2), an interpolated string with escaped braces (1), `partial` as a variable name (1). Attribute patterns like `[HttpGet]` do not parse; `kind: attribute` with a name regex does. 830 route attributes in 120 files, 78 `Map*` calls in 26 files |
| SDK archive | Pin `10.0.402` (same as the host; satisfies `10.0.103` + `latestFeature`). linux-x64: 240.3 MB compressed, 651 MB extracted, sha256 `a131496fe7dac21dceea13354e50a3636126fac05d6e5ccf5fb250829c6e9b6a`. linux-arm64: 232.7 MB, 683 MB, sha256 `5c33483bf8af611524301afe0200fef2b44bfd93d2cf792da5404ca73e7bb8cb`. Upstream publishes sha512 only (both matched); sha256 was computed from the download. The archive unpacks flat (no top-level directory). `libicu72` is in bookworm main |
| Helper build | Referencing `Microsoft.Build`/`Microsoft.Build.Framework` needs `ExcludeAssets="runtime" PrivateAssets="all"` (`MSBL001`). An apphost run against a non-default SDK location needs `DOTNET_ROOT` |

Related code:
- Language touchpoints: [`docs/archived/plans/erosion-prevention/language-extensibility.md`](../archived/plans/erosion-prevention/language-extensibility.md) lists 14 steps. This plan follows them, in Task 7.
- Rust adapter as the template: `internal/extract/rust/` (crate-level first, optional module graph second).
- Python helper pattern: `internal/extract/py/embed.go` and `grimp_helper.py`. The Roslyn helper is a process, not an embedded script, because it needs NuGet packages.
- Tool pins: `tools/manifest.json`, `scripts/provision_tools.py`, `Dockerfile`.
- Measurement profile: `internal/evidence/acquisition/measurement.go`. Absent language rows are skipped when the tree has no matching files (`profileNotApplicable`). Task 7 adds a regression test for that.
- `docs/plans/current.md` lists "JVM and .NET language adapters" as an uncommitted proposal. Task 12 updates it.

## Design decisions

Reviewed with Codex in three rounds on 2026-10-27 (go for handoff). Task 1 spikes remain gates: a measured result that contradicts a decision changes the decision here first.

**D1. One scored edge set per project: file edges, or project edges as fallback.**
- The classifier scores every edge it receives (`internal/relationship/classify/classify.go` `Run`), and a project node can still get strength from a policy glob or a label. Emitting both levels for the same project would score the same dependency twice, and the project copy would miss file-level `public:`/`internal:` selectors.
- So, per source project: when the Roslyn helper loaded it, emit its file → file edges and no `ProjectReference` edges. When the helper is absent or failed for that project, emit its `ProjectReference` edges only if MSBuild evaluation supplied them (strength unknown; the scorer abstains unless policy or a label supplies strength). Never both for one source project.
- A broken MSBuild SDK or unsatisfied `global.json` usually breaks evaluation too. Then the project has no edges from either producer: it is reported as missing inventory (`failed`, reason names the SDK or file), and the run is `partial`. No fallback is promised for it.
- Project nodes stay in the graph for topology and coverage. A `ProjectReference` with no type use is not a dependency edge when file edges exist for that project; the helper reports it as an unused reference count in coverage detail.
- The Roslyn type graph is on by default when the helper is installed (the image ships it). `analyzers.roslyn.enabled: false` forces the project-only graph.
- Fallback edges from test projects (`IsTestProject=true`, or a test-classified project path) are dropped by the extractor, as the Rust extractor drops dev-dependencies. `productionSource` (`internal/assessment/rules/rules_dependency.go`) counts unattributed edges as production, so a test-project edge would otherwise enter production cycle checks.
- Fallback limits, disclosed in [languages](../languages.md): file-specific selectors do not match fallback edges, edge counts become project pairs for failed projects, and the coverage row stays `partial` (missing inputs, not reduced precision).
- No new edge kind and no relationship-layer change. Rust has the same latent double-scoring shape (crate and cargo-modules edges); that is out of scope and recorded in Post-Completion.

**D2. File and path vocabulary, not namespaces.**
- Node IDs are repo-relative slash paths, the same shape as Go and TypeScript. Separator is `/`. Project nodes use the `.csproj` path.
- Module binding goes through declared module globs, as for Go and TypeScript. `config init` writes one module per evaluated project directory, so default policy matches project boundaries.
- `NodeConvention.FileToModuleKey` stays nil (path pass-through). It is filesystem-free and has no project context, so it cannot express project membership; linked files and shared directories would make any directory rule wrong.
- A file compiled into more than one project (linked `Compile Include`) is one node. Its edges are the union across projects. The helper reports linked files outside their project directory as a coverage detail, not as partial.
- Namespaces are not graph nodes. A namespace is a compile-time name, and graph nodes must be places.

**D3. Strength hints from compiler symbols.** The extractor emits a `StrengthHint` per edge, as Python and TypeScript do. It does not classify. `internal/relationship` decides strength, and the [coupling](../coupling.md) precedence is unchanged: an `internal:` policy match is authoritative `intrusive`, a `public:` match is at least `contract`, approved labels pin, and hints fill the rest. Strongest hint wins per file pair.

| Reference from file A to a symbol in file B | Hint |
| --- | --- |
| Implements or uses an interface or abstract type | `contract` |
| Uses a concrete type, field, property value, or constant as data only (no behavior called) | `model` |
| Uses a record or struct with no behavior as data only | `dto` |
| Calls a method, property, event, or constructor of a concrete type | `functional` |
| Inherits a concrete (non-abstract) class | `functional`, never `contract` |
| Uses a symbol whose accessibility is `internal` or `private protected`, declared in another assembly | `intrusive` |

Two channels, aggregated independently per file pair (strongest use wins in each):
- `StrengthHint` carries every row above. `resolveStrength` reads it when no policy glob or label decides (`internal/relationship/classify/classify.go`), so a data-only use of a concrete class without any `public:` selector classifies as `model`.
- `DataStrengthHint` carries only the non-callable rows (`model`, `dto`), as the Go extractor does (`internal/extract/golang/golang.go`). It is what lifts a `public:` floor from `contract` to `model` (`internal/relationship/classify/classify_test.go`, the public-floor cases).

The `intrusive` hint comes from the referenced symbol's accessibility, not from IVT discovery. A design-time compilation can lack the generated IVT attribute; the helper then accepts Roslyn's inaccessible candidate symbol (`CandidateReason.Inaccessible`) as the resolution, but only when the candidate is `internal` or `private protected` and declared in source of another project in the workspace. Task 1 measured that the fallback never fired on bitwarden (the design-time build generated every IVT attribute) and that unfiltered `Inaccessible` candidates are `protected` members or metadata internals, which would be false intrusive hints. IVT declarations (csproj `InternalsVisibleTo` items and `[assembly: InternalsVisibleTo]` attributes) are reported as coverage detail only. A declaration is not a usage edge.

The `intrusive` hint is the C# analogue of Python's private-import hint (`internal/extract/py/py.go`): a structural, config-independent signal that the compiler enforces. It feeds the existing `encapsulation` metric; no new metric.

Importing `internal/relationship/coupling` for the hint constants joins the accepted boundary already granted to `ts` and `py` in [architecture](../architecture.md#accepted-boundaries). Task 7 adds `internal/extract/dotnet` to that list and to the adapter `public:` list in `.archfit.yaml`.

**D4. Roslyn helper.**
- Source in `tools/dotnet/archfit-roslyn/`, a C# console app on `net10.0`, framework-dependent.
- Uses `Microsoft.CodeAnalysis.Workspaces.MSBuild` through `MSBuildWorkspace`. The workspace starts an out-of-process BuildHost, which needs an installed SDK.
- Dependencies pinned with `packages.lock.json` and restored in locked mode. Image build and CI restore it; the product never restores a target.
- Output: one JSON document on stdout with `schema_version`, per-project status, and per-file edges with strength. Errors go to stderr, not into partial-valid JSON.
- Built into `/opt/archfit/dotnet-roslyn` in the image. The Go side reads `ARCHFIT_DOTNET_ROSLYN` (absolute path to the built directory). An invalid path fails visibly, like `ARCHFIT_TOOL_PYTHON`.
- Local CLI: `make dotnet-roslyn` builds the helper. Without the env var, `doctor` reports it missing with the install hint, and the type graph is `absent`.
- Rejected: embedding the helper source and building it at first run. That needs NuGet on the user's machine and a build cache, for no gain over a make target.

**D5. No restore requested by archfit; per-project status from diagnostics.**
- archfit never runs `dotnet restore` and never asks the helper to. Evaluation itself can still download NuGet-resolved MSBuild SDKs (`global.json` `msbuild-sdks`) through the SDK resolver when the network is reachable. That is the SDK's behavior, not an archfit restore; the coverage detail names each SDK the resolver had to fetch or failed to find.
- Roslyn keeps going after errors, so "loaded" does not mean "complete". The helper classifies each project from workspace and compilation diagnostics:

| Status | Condition | Edges emitted |
| --- | --- | --- |
| `complete` | project loaded, no unresolved-symbol diagnostics (`CS0246`, `CS0234`, `CS0103`, `CS0012`, and similar) | file edges |
| `partial` | project loaded, some symbols unresolved (missing NuGet packages or reference packs) | file edges for resolved symbols; coverage `partial` with counts |
| `failed` | project did not load in the helper | none from the helper; the extractor falls back to `ProjectReference` edges only if MSBuild evaluation supplied them (D1) |

- "Did not load" is measured, not assumed (Task 1): MSBuildWorkspace keeps a project whose evaluation failed in the solution as an empty project (0 documents). So `failed` means 0 source documents, with the workspace failure for that project path as the reason. A workspace failure with documents loaded is `partial`. Workspace diagnostics are attributed to a project by the project file path in the diagnostic only when that path is the failing project, never by any path in the message (the "project reference without a matching metadata reference" warning names the referenced project). Non-C# projects in a solution (`.sqlproj`) are skipped and counted in coverage detail.
- Offline, each unresolvable NuGet-resolved SDK costs about 11 s per evaluation. The helper and the evaluation loop run under the configured analyzer timeout; a timeout is `failed` for the run, not silent truncation.
- A `var` declaration resolves to the inferred type. It adds no separate edge beyond the per-file-pair dedup.

- Unresolved symbols are counted, never turned into invented external nodes. Resolved metadata symbols (BCL, restored packages) become external nodes named by assembly.
- A `ProjectReference` is reported unused only when the project is `complete`.
- Unresolved symbols mean missing candidates, not lower precision: an unresolved reference may hide a cross-boundary edge. So:
  - any `partial` or `failed` project keeps the coupling dimension from `measured`. It blocks the existing `FactExtractorResolutionWithinCeiling` fact, which today only TypeScript's `score.TSUnresolvedPartial` blocks (`internal/assessment/evaluation/dimensions.go` `couplingDimension`). No new fact.
  - Roslyn partial runs are not comparable: `comparablePartialProducer` (`internal/assessment/decision/measurement.go`) stays false for `roslyn`.
  - Roslyn joins `FamilyOptions` and `AnalyzerFamilies` (`internal/assessment/decision/origin.go`) as its own analyzer family, so `--base` pairs it.
- Producer identity: the helper prints `archfit-roslyn <version>; Microsoft.CodeAnalysis <version>; SDK <version>` for `--version`. That string is the coverage row `Version`, and `knownProducerVersion` accepts it.

**D6. Target code runs; the original stays read-only.**
- MSBuild evaluation runs `Directory.Build.props`, imported targets, and NuGet-resolved SDK props. Evaluation of all 86 bitwarden projects left `git status --ignored` clean, so `dotnet msbuild -getItem` runs on the original tree.
- The Roslyn design-time build writes. It runs `Compile`/`CoreCompile` with `SkipCompilerExecution=true`, which skips the compiler but not prerequisite targets: `GenerateGlobalUsings` and assembly-info generation write into `IntermediateOutputPath` (bitwarden enables implicit usings in `Directory.Build.props`). Task 1 measured five files per project under `obj/<Configuration>/<tfm>/` (global usings, assembly info and its cache, the TFM attribute file, the generated editorconfig) plus Razor assembly info, and nothing else.
- So the helper runs on a disposable writable snapshot: copy the analysis root to a per-run temp directory, skipping only `.git` and `node_modules/`, analyze it, map paths back by prefix, and delete it. This is the same move the image contract makes (`scripts/image_contracts.py` copies `/trusted` to `/tmp/repo`).
- The snapshot keeps every build input: `obj/` (supplied `project.assets.json`, NuGet imports), `bin/` (`HintPath` references), and files matched by `exclude`. Removing an excluded file could change the compiled program (an excluded generated file or project that included code binds to). `exclude` applies to emitted evidence, after analysis, as for other languages.
- "Evaluation left bitwarden clean" is a measured fact for that repo, not a guarantee. Evaluation runs on the original because it is read-only in intent and was measured clean; the read-only image case (Task 10) is the guard.
- Rejected: redirecting `BaseIntermediateOutputPath`. It also moves `MSBuildProjectExtensionsPath`, which hides supplied `project.assets.json`, and one global value collides across projects.
- The trust statement goes into [evidence](../evidence.md) and [languages](../languages.md): this is the same trust class as `go/packages` and cgo.

**D7. SDK in the image, one version.**
- Official Linux SDK archive `10.0.402`, pinned in `tools/manifest.json` for `amd64` and `arm64`, with `sha256`, provisioned in the builder stage like Go and Node. Upstream publishes sha512 only; the manifest sha256 was computed from the verified download (Task 1). The archive unpacks flat, so provisioning moves the unpack directory itself.
- One SDK band (10.0.x). A target `global.json` that pins a band this SDK cannot satisfy yields `failed` for its projects, with the reason naming `global.json`. Shipping two SDKs is rejected: size, and no evidence that repos need it.
- The SDK carries only its own reference pack. A `net8.0` target offline resolves no framework references and becomes `partial` with the missing pack named. This depends on the host: a developer machine with the 8.0 targeting pack installed resolves it. So the `partial` expectation is asserted only in the image contract, where the environment is fixed (one SDK, empty `NUGET_PACKAGES`, no network). Host tests assert `complete`, or `partial` naming `Microsoft.NETCore.App.Ref` 8.x, nothing else.
- Multi-targeting: project status is the worst status across its TFMs; edges are the union across TFMs.
- Runtime needs ICU for culture-sensitive behavior. Install `libicu72` (bookworm) rather than `InvariantGlobalization`, so comparisons match what the SDK does on a developer machine.
- Environment: `DOTNET_ROOT`, `DOTNET_CLI_HOME=/tmp/archfit-dotnet`, `DOTNET_CLI_TELEMETRY_OPTOUT=1`, `DOTNET_NOLOGO=1`, `DOTNET_SKIP_FIRST_TIME_EXPERIENCE=1`, `MSBUILDDISABLENODEREUSE=1`, `NUGET_PACKAGES=/tmp/archfit-nuget`.
- Measure the image size delta during provisioning. The Linux SDK is 651 MB (amd64) and 683 MB (arm64) extracted, about 235 MB compressed (Task 1).

**D8. Test and generated classification.**
- Test files: `*Tests.cs`, `*Test.cs`, or a `test`/`tests` path segment. Matches the existing rule shape.
- Generated files: `*.g.cs`, `*.g.i.cs`, `*.Designer.cs`, files under `obj/` or `bin/`, and a `<auto-generated` header. The header sniff is shared with all languages and affects only files that carry it.
- `obj/` and `bin/` are skipped in the walk, like `target/` for Cargo.

**D9. Coverage.** Coverlet Cobertura (`dotnet test --collect "XPlat Code Coverage"`) is the one format both coverlet and `dotnet-coverage` emit. Unit: lines, same as LCOV and LLVM. Paths are normalized from `<sources>` plus `filename`: join each `<source>` with `filename` (an absolute `filename` is used as is), take the first join that lies under the analysis root, make it root-relative with `/`, and drop files outside the root. A `<source>` can be `/` or a directory above the repo. Lines are merged per file across `<class>` entries and reports (max hits per line). Strip a UTF-8 BOM. The sidecar contract is unchanged.

**D10. Config.** `languages.dotnet` selects solutions or projects (default: the first `.slnx`/`.sln` at root, else all `*.csproj` outside `obj/`, `bin/`, `exclude`). `analyzers.roslyn` defaults to on when `ARCHFIT_DOTNET_ROSLYN` names an installed helper (the image sets it), matching D1; `enabled: false` forces the project-only graph. `config init` and `config update` write the explicit setting when a solution or csproj exists, like `cargo_modules` for Rust.

**D11. Delivery.** The Roslyn helper is built and tested with the same SDK as the image. CI provisions the SDK from the same manifest pin, through the existing Actions cache. No separate CI version.

## Development Approach

- **Testing approach**: Regular (code first, then tests), with characterization fixtures recorded from real tool output.
- Complete each task fully before the next. Small, focused changes.
- Every task lists its tests. Tests cover success and failure paths: missing SDK, failing `global.json`, `MSB4236`, unresolved symbols, malformed XML, unit mismatch.
- Regression guards: a tree with no `.cs` files produces the same measurement profile before and after the .NET row lands.
- All tests pass before starting the next task.
- Maintain backward compatibility: no change to existing language output on trees without .NET.
- Update this plan when scope changes.

## Testing Strategy

- **Unit tests**: per task, table-driven for the parsers (`.slnx`, `.sln`, MSBuild JSON, Cobertura) and the strength mapping.
- **Helper tests**: xUnit project under `tools/dotnet/archfit-roslyn/tests`, run by `make dotnet-roslyn-test`. Fixture solutions cover cross-project edges, IVT, interface contracts, a global using, a generated file, and a project with an unresolvable NuGet package.
- **Fixtures**: `internal/testdata/dotnet/` (Task 3): seven projects covering success, partial (`net8.0`, and `Unresolved` with a nonexistent package), failure (unresolvable SDK), and a test project. Recorded helper output is the golden for the Go side.
- **Language contracts**: `scripts/eval/language_contracts.py` gets a .NET case that runs the real SDK and helper.
- **Image contract**: `scripts/image_contracts.py` runs the .NET case under the default and an arbitrary numeric UID with read-only inputs.
- **Acceptance**: `bitwarden/server` in a disposable worktree (Task 13). Not a test in CI; the numbers are recorded in the plan.
- **E2E**: none. archfit has no UI.

## Progress Tracking

- Mark completed items `[x]` immediately.
- Blockers: ⚠️ prefix. Discovered tasks: ➕ prefix.
- Spike results are written back into this plan (Task 1) before implementation tasks start.

## What Goes Where

- **Implementation Steps** (`[ ]`): code, tests, and documentation in this repository.
- **Post-Completion** (no checkboxes): actions outside this repository.

## Implementation Steps

### Task 1: Verify spikes that gate the design

- [x] run a minimal Roslyn `MSBuildWorkspace` loader on a snapshot copy of `bitwarden/server` with no restore; record per-project status counts (`complete`, `partial`, `failed`) and resolved vs unresolved symbol counts
- [x] diff the snapshot after the run; record which targets wrote which files (expected: global usings, assembly info under `obj/`)
- [x] confirm the original clone is untouched (`git status --porcelain --ignored`)
- [x] run the 16 NuGet-SDK projects' evaluation with network reachable; record which SDKs the resolver fetched and the wall-clock time
- [x] verify coverlet Cobertura `filename` and `<sources>` paths on one bitwarden test project; record the normalization rule
- [x] verify `ast-grep` `csharp` on the bitwarden tree: count `ERROR` nodes per file
- [x] verify the Linux SDK archive URL, checksum, and size for amd64 and arm64
- [x] write results into the Context and Design decisions of this plan; ⚠️ any decision that changes

### Task 2: Pin and provision the SDK

- [x] add a `dotnet` version and per-architecture archive artifacts (URL, `sha256`) to `tools/manifest.json`
- [x] extend `scripts/provision_tools.py` `sdks` to install the .NET SDK; reject a checksum mismatch
- [x] extend `scripts/ci-provision-tools.sh` and the CI cache key so CI uses the same pin
- [x] write tests in `scripts/tests` for checksum mismatch and unknown architecture
- [x] run `make tool-locks`; it must pass before the next task

### Task 3: Shared .NET fixture

- [x] create `internal/testdata/dotnet/` with a `.slnx` solution: `Domain`; `Application` (`ProjectReference` to Domain, an interface, a DTO record); `Infrastructure` (implements the Application interface, inherits a concrete Domain class, uses a Domain `internal` member through `InternalsVisibleTo`); a `Legacy` project with `<TargetFrameworks>net10.0;net8.0</TargetFrameworks>` and a `ProjectReference` to `Domain` only under the `net10.0` condition (a `net8.0` reference to the `net10.0`-only `Domain` is rejected by the design-time build, measured in Task 4); a `Broken` project with an unresolvable MSBuild SDK; a `Tests` project with `IsTestProject=true` referencing `Broken`
- [x] add a global using, a linked `Compile Include` file, and a generated `*.g.cs`
- [x] write a short README in the fixture listing the expected status and edges per project; review by hand
- [x] no code yet; check the fixture with `dotnet msbuild -getItem` and record the expected evaluation result per project

### Task 4: Roslyn helper

- [x] create `tools/dotnet/archfit-roslyn/` with a `net10.0` console project and `packages.lock.json`
- [x] implement project loading through `MSBuildWorkspace`; classify each project `complete`, `partial`, or `failed` from workspace and compilation diagnostics (D5)
- [x] implement the output contract: `schema_version`, projects with status and reason, per-file edges with hint, data hint, and reference kind; errors on stderr
- [x] implement the D3 hint mapping as one pure function, separate from workspace code
- [x] implement edge collection: symbol → declaring file, dedup per file pair, strongest hint wins; emit `data_strength_hint` separately
- [x] resolve inaccessible candidate symbols (`CandidateReason.Inaccessible`) so cross-assembly `internal` use yields the intrusive hint without the generated IVT attribute
- [x] count unresolved symbols per project; do not invent nodes for them
- [x] report per project: unused `ProjectReference` count (only when `complete`), IVT declarations, linked files outside the project directory
- [x] add `make dotnet-roslyn` and `make dotnet-roslyn-test`
- [x] implement `--version` printing the producer identity string (D5)
- [x] write xUnit tests: hint table (every D3 row in `StrengthHint`; concrete inheritance is `functional`, not `contract`; data-only concrete use is `model`); data hints in the separate channel for a public concrete model and a DTO record; `internal` use without the generated IVT attribute is intrusive; unresolved symbols counted and not emitted; generated file marked generated; status per fixture project matches the fixture README
- [x] run the helper tests; they must pass before the next task

**Lane results (Tasks 2, 3, 4, 8, 9; 2026-10-27).** Facts later tasks depend on:
- Task 2: `install_dotnet` moves the flat archive to `<prefix>/dotnet`; `install_sdks` rejects an unknown architecture. CI exports `DOTNET_ROOT` and adds `<prefix>/dotnet` to `PATH`; the cache key already hashes the manifest and both scripts. The `Dockerfile` (ENV, `libicu72`) is Task 10.
- Task 3/4 fixture, measured offline on the host: `Domain`, `Application`, `Infrastructure`, `Legacy@net10.0` `complete`; `Legacy@net8.0` `partial` (31 unresolved, reason names `Microsoft.NETCore.App.Ref` 8.0); `Unresolved` `partial` (3 unresolved, no edges); `Broken` `failed` (`MSB4236`, SDK named); `Tests` `partial`, not `failed` (its document loads; the workspace failure names `Broken`). Offline helper run: 13-14 s. The `InternalsVisibleTo` item must name the assembly.
- Task 4 helper contract (`schema_version` 1) extends Technical Details: top-level `skipped_projects` (non-C# projects); per project `linked_files`, `ivt_declarations: [{assembly, declared_in}]`, `unused_references` null unless `complete`; one entry per (project, TFM), a failed project has `tfm` `""`; edges carry `external` (then `to` is an assembly name) and `reference` in `implement|inherit|call|data_use|type_use`. Folding TFMs (worst status, union of edges) is the Go side's job (Task 6). An unsatisfiable `global.json` makes the helper exit 1 with a stderr line naming `global.json` and no JSON. Invocation: `dotnet <ARCHFIT_DOTNET_ROSLYN>/archfit-roslyn.dll <root> <solution | project...>`.
- Task 4 behavior: workspace failures attach to a project by the first quoted project path (failure kind only), so a failure marks every TFM of that project. Documents under `obj/` or `bin/` and symbols declared outside the root produce no edges. A property with a body counts as a call; an auto-property is data. Known gap: invoking a delegate stored in a field counts as data use.
- Task 8: the ast-grep language id is `csharp`; the rules are embedded in the astgrep package, so Task 7 only passes `csharp`. A file with `ERROR` nodes makes the syntax coverage row `partial`, its reason naming up to five files with the first error line.
- Task 9: format `cobertura` (`.xml`, root element `<coverage>`), parser version `coverage-parser.cobertura.v1`. A file whose every join lies outside the root is dropped without counting as unresolved. ➕ The config format vocabulary lives outside the coverage package: `internal/config/config.go` allowed formats and error text, the `internal/config/types.go` jsonschema enum, `internal/configschema/schema_test.go`, and the schema; Task 7 adds `cobertura` there and Task 12 regenerates the schema.

**Review round 1 and 2 (revmux, Tasks 2, 4, 8, 9; 2026-10-27).** Round 1 (comprehensive, 4/4 sources) found 5 issues, all fixed with tests: `record struct` was never a DTO; CS1061/CS0117 did not make a project `partial`; indexer uses were ignored; the C# route regex took `Name = "..."` or a `MapGroup` prefix as the template; `install_dotnet` nested the SDK over an existing `prefix/dotnet` (Go and Node had the same flaw, now one `replace_directory` helper). Round 2 (final, 2/2 sources) found nothing.

**Task 5 results.** `internal/extract/dotnet` (`New(runner, cfg, targets)`, `Applicable`, `Evaluation`, `ProjectEvaluation.FallbackReferences`). Evaluations run in parallel (4 workers, 5 min each) because an unresolvable SDK costs about 11 s offline. Test projects are detected from `IsTestProject` only; offline, projects that rely on the Test SDK props in `obj/*.nuget.g.props` may evaluate as `""`, so Task 7's path classifier (D8) must back it up. Timeout is project `failed` plus coverage `timed out`. Unexported placeholders for Task 7: `langDotNet`, `nodeKindProject` (no project node kind exists in `graph` yet), `coverageTool`. `ExtractConfig` has no dotnet fields, so the configured targets go in as `targets`. No fact cache yet (Rust has one). Reason strings: `MSB4236: SDK '<name>' not resolvable offline; run with NuGet access and a warm cache`; `global.json requests SDK X, which is not installed; install a matching SDK or relax the pin`; `target framework <tfm>: <reason>`.

### Task 5: Project graph extractor

- [x] create `internal/extract/dotnet` with an applicability function used by `Extract` and by the registry probe
- [x] parse `.slnx` (XML `<Project Path>`) and `.sln` (`Project(...)` lines) into project paths; test both, including a solution folder and a missing path
- [x] run `dotnet msbuild <project> -getItem:ProjectReference,PackageReference -getProperty:TargetFramework,TargetFrameworks,IsTestProject` once per project and once per TFM from `TargetFrameworks`, on the original tree; parse the JSON
- [x] map a failing evaluation to `failed` with the reason: `MSB4236` names the SDK, a `global.json` mismatch names the file, other errors keep the first MSBuild error line
- [x] emit project nodes and per-TFM-deduplicated `ProjectReference` facts; drop references from test projects
- [x] write table-driven tests with recorded MSBuild JSON: one TFM, two TFMs, missing reference, `MSB4236`, unsatisfied SDK pin, test project
- [x] write an integration test (skipped without `dotnet` on `PATH`) that evaluates the fixture's `Legacy` project with the real SDK and finds the `net10.0`-only reference through the per-TFM loop
- [x] write a test that a root with no `.csproj` or solution yields `absent`, not `partial`
- [x] run the package tests; they must pass before the next task

### Task 6: Type graph, snapshot, and fallback

- [ ] create the per-run snapshot (D6): copy the root minus `.git` and `node_modules/`; keep `obj/`, `bin/`, and excluded files; apply `exclude` to emitted evidence; delete the snapshot after the run, also on error and timeout
- [ ] run the helper on the snapshot through `toolrun.Runner` with `ARCHFIT_DOTNET_ROSLYN`; an unset variable yields `absent` with the install hint; an invalid path fails visibly
- [ ] parse the helper JSON; check `schema_version`; map snapshot paths back to the original root
- [ ] per source project: `complete` or `partial` emits file edges; `failed` or helper absent emits that project's `ProjectReference` edges; never both (D1)
- [ ] write tests with recorded helper output for the fixture: edges, hints, data hints, partial counts, no fallback edge from `Tests`, unsupported `schema_version` rejected
- [ ] write two separate fallback tests: helper failed but evaluation succeeded emits `ProjectReference` edges; both failed (`Broken`) emits no edges and reports missing inventory
- [ ] write a snapshot test: a file matched by `exclude` that included source binds to still resolves, and its own edges are not emitted
- [ ] write a test that a data-only use of a concrete class field, with no `public:` selector, classifies as `model`
- [ ] write a test that one dependency is scored once: a project with file edges emits no project edge to the same target
- [ ] write a classify-level test: a `public:` selector on target files classifies the file edge as `contract`, and no `functional` project edge exists beside it
- [ ] write a test that the snapshot is removed after success, helper error, and timeout, and that the original root is never written
- [ ] run the package tests; they must pass before the next task

### Task 7: Language wiring (the 14 touchpoints)

- [ ] add the `.NET` descriptor to `languages` in `internal/extract/registry/registry.go`, appended after Rust; never reorder
- [ ] add `LangDotNet` in `internal/model/graph/graph.go` and a `BuiltinConventions` entry: separator `/`, extensions `.cs`, priority 4, nil file-to-module mapping (D2)
- [ ] add the coverage names (`dotnet-msbuild`, `roslyn`, `cobertura`) to `MeasurementContract` in `internal/model/evidence/measurement.go`
- [ ] keep Roslyn partial runs non-comparable: do not add `roslyn` to `comparablePartialProducer`; add a test that a partial Roslyn producer makes the run non-comparable
- [ ] make a `partial` or `failed` Roslyn row block `FactExtractorResolutionWithinCeiling` in `couplingDimension`, next to `score.TSUnresolvedPartial`
- [ ] write a promotion test: fully rated resolved edges plus one unresolved Roslyn reference leave the coupling dimension `partial`
- [ ] add Roslyn to `FamilyOptions` and `AnalyzerFamilies` (`internal/assessment/decision/origin.go`) and wire it from config through application options; test that `--base` pairs the Roslyn family
- [ ] accept the helper identity string in `knownProducerVersion`; test known and empty versions
- [ ] add the `dotnet` config case to `internal/config/tools.go` (`ToolMode`, `ToolGate`, `SetToolMode`) and `analyzers.roslyn`
- [ ] extend `primaryToolForLanguage` in `internal/assessment/evaluation/dimensions.go`
- [ ] add `Presence.DotNet` and `languagePresence` in `cmd/archfit/init.go`; add `internal/initcfg/discover_dotnet.go` that writes one module per evaluated project directory
- [ ] add `.cs` to the LOC walk (`internal/extract/loc/loc.go`), skip `obj/` and `bin/`, and add the `cloneLanguage` entry (`internal/relationship/analysis/analysis.go`)
- [ ] add the test-file rule (`internal/syntax/testfile.go`) and generated rules (`internal/syntax/fileclass.go`, including the `<auto-generated` header)
- [ ] add the agent-task file resolver entry (`internal/assessment/agenttask`) and the doctor tool rows (`dotnet`, `archfit-roslyn` via `ARCHFIT_DOTNET_ROSLYN`)
- [ ] add `internal/extract/dotnet` to the adapter `public:` list in `.archfit.yaml`; run `archfit policy can-import internal/extract/dotnet/dotnet.go internal/relationship/coupling` before adding the import
- [ ] write tests for each new switch case: `.cs` recognized, `obj/` skipped, `*.g.cs` generated, `*Tests.cs` test, `<auto-generated` header generated
- [ ] write a regression test: a tree with no `.cs` files gives the same measurement profile before and after the row lands
- [ ] regenerate `internal/testdata/model_surface.golden` with `ARCHFIT_UPDATE_SURFACE=1` for the new exported identifiers (`LangDotNet` and any coverage constants); review the diff
- [ ] run `go test ./internal/...`; it must pass before the next task

### Task 8: ast-grep C# rules

- [x] add `internal/extract/astgrep/rules/csharp.yml` with route declarations: `[HttpGet]`, `[HttpPost]`, `[Route]` attributes and `MapGet`, `MapPost`, `MapPut`, `MapDelete` invocations
- [x] mark a file `partial` when its tree has `ERROR` nodes; record the reason per file (rules use `kind: attribute` with a name regex and `kind: invocation_expression` on `Map*`; `[...]` patterns do not parse, Task 1)
- [x] write tests on a fixture file: each route form, a C# 14 `extension` block produces `partial`, a plain class produces no route
- [x] run the astgrep package tests; they must pass before the next task

### Task 9: Cobertura coverage

- [x] add a `cobertura` parser in `internal/extract/coverage/`, with the path normalization from Task 1
- [x] register the format in `detect.go` and `normalize.go`; units are lines
- [x] write table-driven tests: a valid report, a missing `<sources>` entry, a unit conflict with a Go profile, malformed XML, and a file outside the root
- [x] run the coverage package tests; they must pass before the next task

### Task 10: Image integration

- [ ] update the `Dockerfile` builder to provision the SDK and build the Roslyn helper into `/opt/archfit/dotnet-roslyn`; set the D7 environment variables and `ARCHFIT_DOTNET_ROSLYN`; install `libicu72` in the runtime stage
- [ ] extend `scripts/image_contracts.py`: run real extraction on the .NET fixture under the default and an arbitrary numeric UID
- [ ] add an image contract case that analyzes the fixture directly from the read-only `/trusted` mount, without the harness copy, and passes
- [ ] assert the deterministic image statuses for the fixture: `Legacy` `net8.0` target `partial` naming the 8.0 reference pack, `Broken` `failed` naming its SDK
- [ ] record the image size delta in this plan; ⚠️ if it exceeds 600 MB compressed
- [ ] run the image contract; it must pass before the next task

### Task 11: Language contracts and architecture tests

- [ ] add a `dotnet` case to `scripts/eval/language_contracts.py` that runs the real producers on the fixture and asserts statuses, edges, hints, and partial reasons
- [ ] add architecture tests so `internal/extract/dotnet` runs tools only through `toolrun.Runner` and imports no concrete adapter outside its package
- [ ] run `make language-contracts`; it must pass before the next task

### Task 12: Schemas and documentation

- [ ] run `make schema` and review the diff for the new `languages.dotnet` and `analyzers.roslyn` fields
- [ ] confirm `TestModelSurfaceNoDrift` passes without regeneration (Task 7 owns the surface change)
- [ ] update `docs/languages.md`: support table, Before-you-run table, a .NET section covering D1 fallback limits, D5 statuses, D6 snapshot and trust, the analyzer table, and the helper install section
- [ ] update `docs/configuration.md` with `languages.dotnet` and `analyzers.roslyn`
- [ ] update `docs/evidence.md` with the .NET trust limits (target code runs in a snapshot; NuGet-resolved SDKs; `global.json` pin; missing reference packs)
- [ ] update `docs/development.md` (`make dotnet-roslyn`, the .NET language contract) and `docs/architecture.md` (the helper is an adapter artifact; add `internal/extract/dotnet` to Accepted boundaries; the extractor emits hints only)
- [ ] update `docs/plans/current.md` to remove .NET from the proposals list and point to this plan
- [ ] add a `bitwarden` label to `scripts/eval/corpus_sweep.py` with its acceptance role, and a test for the label in `scripts/eval`
- [ ] run `make all` and `archfit check --format agent`; follow `next_action`

### Task 13: Acceptance on bitwarden/server

- [ ] create a disposable worktree of `~/Workspace/bitwarden-server` for analysis; the original stays untouched
- [ ] run `archfit config init` on the worktree; check the generated `.archfit.yaml` has one module per project directory and the Roslyn analyzer on
- [ ] run `archfit analyze` offline and record: project statuses, fallback projects and why, the intrusive edges, the total edge count, wall-clock time, peak memory
- [ ] run the same analysis with the network reachable; record which SDKs the resolver fetched and the difference
- [ ] confirm the original clone is clean after both runs (`git status --porcelain --ignored`)
- [ ] record the numbers in Post-Completion; ⚠️ any metric that reads `n/a` where the evidence says it should be measured

### Task 14: Verify acceptance criteria

- [ ] verify every requirement in Overview is implemented or listed in Cut
- [ ] verify D5 statuses and D6 read-only behavior with tests or recorded runs
- [ ] run the full Go test suite, the helper tests, the language contracts, the image contract, and `make tool-locks`
- [ ] run `archfit check --format agent` on this repo and follow its `next_action`
- [ ] verify test coverage on the new packages meets the project standard

### Task 15: Final documentation review

- [ ] re-read `docs/languages.md` and `docs/evidence.md` against the implemented behavior; fix any claim the code does not back
- [ ] record in this plan any decision that implementation changed

## Technical Details

- Project graph node: `{id: "src/Core/Core.csproj", kind: project, module: "src/Core"}`.
- Type graph edge: `{from: "src/Core/Auth/Login.cs", to: "src/Domain/User.cs", strength: "model", reference: "record_use"}`.
- Helper JSON: `{schema_version, projects: [{path, tfm, status: complete|partial|failed, reason, unresolved_symbols, unused_references, ivt_declarations, files: [{path, generated, edges: [{to, hint, data_hint, reference}]}]}]}`.
- Partial reason strings name the cause and the fix: `"MSB4236: SDK 'Bitwarden.Server.Sdk' not resolvable offline; run with NuGet access and a warm cache"`.

## Post-Completion

*Items requiring action outside this repository. No checkboxes.*

**Acceptance numbers** (filled in Task 13): projects measured offline, projects measured networked, edge counts, runtime, memory.

**Manual verification**:
- Run `archfit analyze` on an Aspire repository (one NuGet SDK in `global.json`) and check the partial reason.
- Run on a multi-target library to check the per-TFM dedup.
- Check the image on a native arm64 host (emulated amd64 is not proof, per the release contract).

**Out-of-scope finding**:
- Rust emits crate `depends_on` edges and cargo-modules module edges for the same dependency, and the classifier scores both. Check whether crate edges reach strength through policy or labels; if so, apply the D1 rule to Rust in a separate change.

**External system updates**:
- The App and Action pins need a new image digest and their own adoption tests.
- `bitwarden/server` results are local measurements; do not publish them as a bitwarden statement.
