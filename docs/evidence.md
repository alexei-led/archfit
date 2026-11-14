# Evidence contracts

Evidence status describes completeness. It does not describe whether a metric value is good. Report measurements and repository rule errors remain separate. See [results](results.md) for the public report fields and [metrics](metrics.md) for metric meaning.

## Promotion rule

Each dimension has a fixed set of required fact kinds. A producer can prove a fact not applicable only through its own applicability probe. A config opt-out does not prove that a fact does not apply.

| Status | Condition |
|---|---|
| `measured` | The dimension applies, and every required fact is complete or proved not applicable. |
| `partial` | The dimension applies, at least one required fact is complete, and another is incomplete. |
| `unmeasured` | The dimension does not apply, or no required fact was observed. |

A producer reports zero only after it completes the relevant denominator. A missing row, unavailable tool, or failed producer is not zero. Every `partial` or `unmeasured` dimension names its missing in-claim facts. Unknown facts outside the claim do not block promotion.

## Code the change adds that no analysis reads

`archfit check` reads the merge base and head in one run. It counts errors by rule, modules, and symbol or literal. A new entry, or a higher count, belongs to the change. Moving or renaming code alone does not establish a new error. Missing facts give `unknown`, not a clean count. An expired exception fails every change while the error remains.

The policy's exclusions and language settings apply to both trees. They are protected policy inputs, not a way for a worker to accept unread code. See [what the check reads](#what-the-check-reads) for the readers and their limits.

## Dimension contracts

Required fact names below match `internal/assessment/state.RequiredFacts`. This source contract also lists out-of-claim facts. They can appear as unknown facts without blocking promotion.

### `intent`

- **Required facts:** `declared intent inventory`.
- **Denominator:** Declared modules.
- **Promote when:** The policy declares at least one module. Module and layer counts are policy facts, not proof that code follows the rules.
- **Limits:** Rule errors and exceptions belong to `repository`, not to a second report evaluator.

### `structure`

- **Required facts:** `primary dependency inventory`, `internal edge classification`.
- **Denominator:** Internally resolved edges over all discovered dependency edges.
- **Promote when:** At least one primary extractor applies. Every applicable enabled extractor completes its inventory. Every edge inside the declared module map has module, direction, and layer classification. A completed empty inventory is observed zero.
- **Limits:** External edge direction and layer are out of claim. A first-party target that produces no graph node can appear as a library edge. The map report covers walked but unowned source separately.

### `modularity`

- **Required facts:** `declared module inventory`, `module boundary attribution`, `module graph shape`.
- **Denominator:** Declared modules with a public surface over all declared modules. This ratio does not decide status.
- **Promote when:** At least one module is declared. Every first-party graph node has a module or an explicit outside-map result. Every applicable graph fact is observed or proved not applicable by its metric producer.
- **Limits:** Inferred public surface is out of claim. Zero declared public surfaces is an observed policy fact.

### `coupling`

- **Required facts:** `coupling candidate inventory`, `coupling strength`, `coupling distance`, `coupling volatility`, `extractor resolution within ceiling`.
- **Denominator:** Scored cross-boundary candidates over scored plus abstained cross-boundary candidates. Same-module and undeclared-external edges are outside this denominator.
- **Promote when:** An in-scope candidate exists. Candidate inventory is observed. Every denominator candidate has strength and distance. Each scored candidate has rated volatility. TypeScript unresolved specifiers are at most 10 percent, or the TypeScript probe proves that TypeScript does not apply.
- **Limits:** Same-module coupling and undeclared-external edges are out of claim. Unknown strength or distance, undeclared volatility, and TypeScript resolution over the ceiling leave the dimension partial. See [coupling](coupling.md) for v7 semantics.

### `change_locality`

- **Required facts:** `eligible commit sample`, `commit-to-module attribution`.
- **Current result:** `unmeasured`. The report collects no co-change history or module touch counts.
- **Limits:** Essential versus accidental volatility is out of claim. Commit frequency cannot distinguish domain change from design churn.

### `complexity`

- **Required facts:** `declared module graph`, `dependency chain depth`, `module fan-in distribution`, `module fan-out distribution`.
- **Denominator:** Declared modules with chain-depth and degree values over all declared modules.
- **Promote when:** Production source exists in a declared module. All four graph fact kinds are complete over every declared module. A completed edgeless graph records zero depths and degrees.
- **Limits:** This claim covers architecture-level graph complexity. File size, function length, and cognitive complexity are out of claim. Chain depth counts edges between strongly connected components. Edges within one component add no depth.

### `testability`

The [metrics reference](metrics.md) explains how supplied coverage differs from analyzer coverage.

- **Required facts:** `production source inventory`, `supplied coverage units`, `coverage path resolution`, `coverage module attribution`, `coverage freshness`.
- **Denominator:** Covered declared modules over all declared modules, plus covered units over total units within one compatible unit family. Statement and line units are never averaged together.
- **Promote when:** Supported production source exists. Every declared module has valid coverage. All paths and modules resolve. Unit kinds and totals are compatible. Every configured source has a valid non-empty sidecar, each parsed coverage path appears in its `sources` map, and all listed hashes match current source bytes.
- **Limits:** Missing coverage is partial when production source exists. Static source classification alone does not measure executed coverage. Assertion quality, mutation resistance, and boundary-test meaning are out of claim. Sidecars are unsigned, so Archfit cannot authenticate a producer or detect coordinated edits to an artifact and its sidecar.
- **Aggregate limit:** Duplicate facts for the same file, unit, and total keep the greatest covered count. This is a lower bound. Conflicting units or totals make coverage partial and suppress the ratio.

### `operations`

- **Required facts:** `declared operational topology`, `corroborated deploy unit`, `owner provenance`, `declared-to-corroborated topology reconciliation`.
- **Denominator:** Modules with both a corroborated deploy unit and a qualifying owner over all declared modules.
- **Promote when:** At least one module is declared. Each module has independent deploy-unit evidence, declared or CODEOWNERS owner provenance, and a reconciliation result. A declared deploy unit cannot corroborate itself.
- **Limits:** Git author fallback does not satisfy owner provenance. Runtime topology, supply-chain state, and analyzer health are out of claim.

### `drift`

- **Required facts:** `admissible persisted reference`, `complete two-sided seam identity`.
- **Current result:** `unmeasured`. The report has no stored reference and does not compare seam inventories across runs.
- **Limits:** This legacy report dimension is not the repository drift that schema 3 `archfit report` lists for owners. The check's same-run change counting does not promote it.

The coupling contract counts abstentions in its denominator. Same-module coupling and undeclared external dependencies are outside that denominator. See [coupling](coupling.md) for current v7 distance and seam semantics.

The testability contract needs supplied coverage. Static source classification alone does not measure executed coverage. Sidecar freshness checks file paths and source hashes. It does not authenticate the producer or prove that the artifact lists every source fact.

The operations contract requires owner provenance from a declaration or CODEOWNERS. Git author data does not satisfy this requirement. A declared deploy unit does not corroborate itself.

The complexity contract measures module graph depth and degree distributions. It does not claim function-level or cognitive complexity.

## Comparisons and references

The check compares the merge base and head under one policy and one engine in the same run. Its `--base <ref>` selects the target branch; it does not load an earlier report. See [commands](commands.md#check) for target selection and [results](results.md) for added errors and repository drift.

The report measures one tree. It has no measurement profile, comparability verdict, finding origin, or base-tree comparison block. Tool versions and availability remain visible as measurement and coverage evidence. Missing evidence still prevents promotion; removing a profile does not make an incomplete inventory complete.

Report identity retains the policy digest, policy ref, base commit, head tree digest, and engine version when known. It carries no measurement-profile hash. A policy digest identifies policy bytes, not evidence completeness.

## What the check reads

Schema 3 `archfit check` reads three kinds of fact from each tree: which file imports which package, file or library; the source files; and the string literals, which only the `duplication` rule needs. It reads them from two trees in one run: the merge base and the head. The head is the working tree as a commit of every change would hold it: edits and new files count, files git ignores do not. Both trees are written from git objects to a temporary directory and read there, so the head resolves imports against the same files the base and CI see, and the working tree is never changed.

| Language | Read with | A first-party import targets |
|---|---|---|
| Go | The standard parser, built in. `go.mod` and `go.work` give the module path. | The package directory (`.` for the root package) |
| TypeScript, JavaScript | `ast-grep`, and a built-in resolver for relative paths, `tsconfig` `paths` and workspace packages | The file |
| Python | `ast-grep`, and a built-in resolver for packages and the `src` layout | The file |
| Rust | `cargo metadata`: crates only | The crate directory; the site is the dependency's line in `Cargo.toml` |
| C# | MSBuild project references | The project directory; the site is the reference's line in the project file |
| Literals | The standard parser for Go, `ast-grep` for the other languages | — |

The check never runs a language toolchain for Go, TypeScript or Python, `node_modules`, dependency-cruiser, grimp, the typed Go loader, SCIP indexers or `jscpd`, and no optional analyzer: the facts are the same with every `analyzers` key on or off. A Go repository needs only `git`. TypeScript and Python need `git` and `ast-grep`. Rust needs `cargo` and C# the .NET SDK, plus `ast-grep` for their literals.

A language is read where its files exist, unless `languages.<id>.enabled` is `false`; `true` reads the same as `auto`. Python files outside any package (loose scripts) are not a Python project: they give no facts and need no `ast-grep`.

One exclusion rule covers every extractor: the policy's `exclude`, plus `.git`, `node_modules` and `vendor` at any depth. A language whose files are all excluded is not read and needs no tool. The Go parser also follows the go command: it skips `testdata`, directories that start with `_` or `.`, `_test.go` files, nested modules the run does not load, and files the build constraints exclude. The Go literals come from every `.go` file the rule keeps, test files included. A Go file the go command never builds (under `testdata`, or a `_` or `.` path) that does not parse is skipped without an `unknown` item: a parser's fixture may be broken on purpose.

Facts are cached under the tree's git id in `.archfit-cache/facts/checkfacts/`. A tree read once is not read again. The key also holds the engine version, the `ast-grep` version, the policy settings the facts depend on, and the Go build settings (`GOOS`, `GOARCH`, `GOFLAGS`, `CGO_ENABLED`, the go env file). Facts read with `cargo` or `dotnet` are served only while that tool has the same version. Facts with a `tool` or `base` item are not cached: installing or fixing the tool does not change the tree, so the next run reads again. A build without a version reads without the cache.

Missing evidence is not zero. Each thing the check needs and cannot read is an `unknown` item with a cause and the step that restores it:

| Cause | When | Next |
|---|---|---|
| `import` | A first-party import that names no file or package | fix the import |
| `import` | A file that does not parse, or a `tsconfig` that does not parse | fix the syntax error |
| `import` | A `tsconfig` `extends` the resolver does not read (a package, `node_modules`, outside the repository) | make the extends a relative path inside the repository |
| `import` | A Go file whose build depends on cgo while `CGO_ENABLED` is not set | set `CGO_ENABLED` to 0 or 1 |
| `tool` | `ast-grep`, `cargo` or `dotnet` is not installed where its language has files; the item is about the whole tree | install the tool |
| `base` | The merge base cannot be read | run `git fetch`, then `archfit check` again |

## Fact cache

The fact cache stores extractor facts. It does not store relationship classifications, assessment results, scores, or verdicts. Those are recomputed on each run.

Each extractor key is a SHA-256 digest of NUL-separated values:

```text
schema version, analyzer name, tool version, analyzer config hash, input tree hash
```

The input tree hash covers sorted relative paths and file content hashes. Hashing uses content rather than modification times. Archfit skips `.git` and `.archfit-cache` during input discovery. It stops cache-key discovery after 20,000 filesystem entries or 64 directory levels. It also refuses an input hash over 32 MiB. In each case, the extractor runs without a cache key. Hitting the entry cap is also reported on stderr (`warning: fact cache skipped: input tree has more than 20000 filesystem entries`), because a healthy tree over the cap would otherwise re-extract on every run with nothing saying why. The cache uses `.archfit-cache/facts/<analyzer>/<key>.json`.

The fact cache is best effort. A missing, unreadable, or corrupt entry is a cache miss. A failed write does not fail analysis. A hit updates the entry time for eviction. The cache caps fact data at 1 GiB and evicts oldest entries until usage is at most 768 MiB.

`--refresh` bypasses cache reads and writes successful fresh facts. Process errors and timeouts are not cached. Extractors can reject successful process output when it is partial. Go caches only clean module loads. Other extractors decide whether a non-zero exit contains usable facts.

The key has limits that vary by analyzer:

- The check's facts are keyed by git tree id, not by an input tree hash: see [what the check reads](#what-the-check-reads).
- TypeScript and Python report import extraction do not use the fact cache or target runtimes. The check caches its facts by git tree id.
- Go keys each workspace member with Go toolchain version, Go environment, build settings, module files, and the member's transitive workspace dependency inputs. The Go environment includes `GOOS`, `GOARCH`, `CGO_ENABLED`, build flags, and toolchain options. The Go toolchain version identifies standard-library facts.
- Go bypasses cache for a member whose dependency closure uses a local `replace` or an unkeyed workspace module. A local replacement in `go.work` disables Go caching for the full run.

The cache-key schema changes when serialized facts or their derivation semantics change. A cache entry never raises the evidence trust ceiling. It changes extraction cost, not evidence completeness or provenance.

Delete `.archfit-cache` to remove cached facts. Cache contents are disposable. A cache miss causes extraction to run again.

## Analyzer trust boundary

Missing parser facts give the check `unknown`, not `pass`. Missing compiler or source evidence keeps the report's affected measurements partial or unmeasured. Repository errors and unread places come from the parser-based check path. A tool version or a nonempty index alone does not establish completeness. Rust target attribution uses Cargo metadata and preserves ambiguity on live and cached results.

Image digests bind installed files, not remote execution. Reports remain self-reported/customer-
attested. Target code can run in optional indexers/build tools; container network access and
mounted source remain part of the operator's trust boundary. See [Languages and tools](languages.md).

## .NET trust limits

- **Target code runs.** MSBuild evaluation executes `Directory.Build.props`, imported targets, and SDK props. The Roslyn design-time build also writes generated files under `obj/`. Evaluation runs on the original tree. The helper runs on a disposable snapshot of the analysis root plus the ancestor build inputs up to the repository root, deleted after the run. A build input above the repository root is not in the snapshot: a project that would inherit one is `partial` and names it, so its file edges are not complete evidence. A symlink whose target lies outside the analysis root is dropped from the snapshot rather than reproduced, so the build cannot write through it into the original tree; the `roslyn` reason names every dropped link, and the project holding it is `partial`. The image contract analyzes the .NET fixture from a read-only mount and checks the tree is unchanged. This is the same trust class as `go/packages` and cgo. Evaluation leaving `bitwarden/server` clean is a measurement, not a guarantee.
- **Helper location.** `ARCHFIT_DOTNET_ROSLYN` must name an absolute directory outside the analysis root, so the target cannot supply the helper. An invalid value drops the .NET graph and makes `dotnet-msbuild` partial; it never falls back silently.
- **NuGet-resolved SDKs.** The SDK resolver fetches `msbuild-sdks` from `global.json` when the network is reachable. Archfit does not restore packages. Offline, the affected projects are `failed` with the SDK named.
- **`global.json` pin.** A pin that the installed SDK cannot satisfy makes the projects `failed`. The reason names `global.json`.
- **Missing reference packs.** A target framework without its reference pack on the host (for example `net8.0` against a 10.0 SDK) leaves the project `partial`, with the pack named. Results depend on the host's installed packs.
- **Unresolved symbols are missing candidates.** An unresolved reference can hide a cross-boundary edge, so it is not reduced precision. Any `partial` or `failed` project, an evaluated project the helper did not report, or a failed or timed-out helper run keeps the `coupling` dimension from `measured` by blocking `extractor resolution within ceiling`, as TypeScript unresolved specifiers do above the ceiling.
- **Incomplete projects limit report measurements.** A project's file edges are complete only when the helper loaded it `complete`. Per-file gaps keep `structure`, `modularity`, and `complexity` from `measured`, even when the project-level `dotnet-msbuild` row is `ok`. Reasons name `roslyn`, the projects, and their causes. The report runs no version 2 rules. Repository errors and unread places come from the same parser-based facts as the check, not from Roslyn.

- **Producer identity.** The coverage row version is the helper's `archfit-roslyn <version>; Microsoft.CodeAnalysis <version>; SDK <version>` string.

See [languages](languages.md#net) for statuses and fallback.

## Upgrade limits

- Coverage sidecars are unsigned. Add authenticated producer evidence only when the product needs to prove producer identity or resist coordinated artifact and sidecar edits.
- Duplicate coverage aggregates use a lower bound when their unit and total agree. Different units or totals make coverage partial. Add per-unit identities when real multi-profile data needs an exact union.
- Fact-cache eviction walks the cache tree when it crosses the size cap. Add an index only if profiling shows that this walk affects analysis time.
