# Evidence contracts

Evidence status describes completeness. It does not describe whether a metric value is good. Findings and gate results remain separate. See [results](results.md) for the public report fields and [metrics](metrics.md) for metric meaning.

## Promotion rule

Each dimension has a fixed set of required fact kinds. A producer can prove a fact not applicable only through its own applicability probe. A config opt-out does not prove that a fact does not apply.

| Status | Condition |
|---|---|
| `measured` | The dimension applies, and every required fact is complete or proved not applicable. |
| `partial` | The dimension applies, at least one required fact is complete, and another is incomplete. |
| `unmeasured` | The dimension does not apply, or no required fact was observed. |

A producer reports zero only after it completes the relevant denominator. A missing row, unavailable tool, or failed producer is not zero. Every `partial` or `unmeasured` dimension names its missing in-claim facts. Unknown facts outside the claim do not block promotion.

## Dimension contracts

Required fact names below match `internal/assessment/state.RequiredFacts`. This source contract also lists out-of-claim facts. They can appear as unknown facts without blocking promotion.

### `intent`

- **Required facts:** `declared intent inventory`, `active rule conformance`.
- **Denominator:** Active declared rules over all declared rules. Rules with `gate: off` stay visible but are not evaluated.
- **Promote when:** Policy declares at least one module or rule. The declaration inventory is observed. Every active rule has an evaluation result. A policy with no active rules proves conformance not applicable after its inventory is observed.
- **Limits:** Disabled-rule conformance is out of claim.

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
- **Denominator:** Touched declared modules over declared modules. If history finds more modules than policy declares, the larger set is the total.
- **Promote when:** At least one eligible commit exists. The bounded sample completes. Every sampled commit has complete module attribution, including observed zeroes for untouched modules.
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
- **Denominator:** The union of qualifying seam identities in the current and persisted ledgers.
- **Promote when:** A persisted state baseline contains a seam ledger and all comparison fingerprints match. Every qualifying seam identity on both sides is compared.
- **Limits:** A missing, legacy, or incompatible baseline is unmeasured, not partial. A separate `--base` comparison is out of claim. `comparison.status: not_requested` only means that the user did not request a base-tree report.

The coupling contract counts abstentions in its denominator. Same-module coupling and undeclared external dependencies are outside that denominator. See [coupling](coupling.md) for current v7 distance and seam semantics.

The testability contract needs supplied coverage. Static source classification alone does not measure executed coverage. Sidecar freshness checks file paths and source hashes. It does not authenticate the producer or prove that the artifact lists every source fact.

The operations contract requires owner provenance from a declaration or CODEOWNERS. Git author data does not satisfy this requirement. A declared deploy unit does not corroborate itself.

The complexity contract measures module graph depth and degree distributions. It does not claim function-level or cognitive complexity.

## Comparisons and references

Archfit has two comparison paths. They answer different questions.

| Reference | Purpose | Compatibility rule |
|---|---|---|
| Persisted architecture-state baseline | Compare qualifying seams over time for the `drift` dimension. | Requires the current and stored classification hash, model hash, labels hash, rubric version, and measurement profile to match. |
| `--base <ref>` | Compare the current tree with a Git base tree for findings and coverage. | Compares classification hash, model hash, labels hash, rubric version, and measurement profile. It does not compare the raw config hash because both sides use one config file. |

The comparison fingerprints are separate so reports can name each mismatch. The measurement profile contains producer identity, tool version, evidence status, and measurement settings. A missing or unsupported profile blocks comparison. A profile change is not treated as a code-only change.

Changing a module name can change seam identity. Changing labels can change strength and seam qualification. Changing classification settings can change observed relationships. Changing the rubric can change assessed meaning. Owner and deploy-unit resolution affect model identity, but do not change distance, severity, or seam qualification. Each fingerprinted change makes a numeric comparison non-comparable.

`--base` does not provide the persisted reference for `drift`. Without a comparable persisted baseline, drift is `unmeasured`, even when `--base` runs. `comparison.status: not_requested` means only that the user did not request a separate base-tree report.

The raw config hash identifies the config file. It is not a comparability fingerprint. Comments, waivers, rules, and `reviewed_at` can change raw bytes without changing the facts used by a numeric comparison.

## Fact cache

The fact cache stores extractor facts. It does not store relationship classifications, assessment results, scores, or verdicts. Those are recomputed on each run.

Each extractor key is a SHA-256 digest of NUL-separated values:

```text
schema version, analyzer name, tool version, analyzer config hash, input tree hash
```

The input tree hash covers sorted relative paths and file content hashes. Hashing uses content rather than modification times. Archfit skips `.git` and `.archfit-cache` during input discovery. It stops cache-key discovery after 20,000 filesystem entries or 64 directory levels. It also refuses an input hash over 32 MiB. In each case, the extractor runs without a cache key. Hitting the entry cap is also reported on stderr (`warning: fact cache skipped: input tree has more than 20000 filesystem entries`), because a healthy tree over the cap would otherwise re-extract on every run with nothing saying why. The cache uses `.archfit-cache/facts/<analyzer>/<key>.json`. LLM responses use the separate `.archfit-cache/llm` directory.

The fact cache is best effort. A missing, unreadable, or corrupt entry is a cache miss. A failed write does not fail analysis. A hit updates the entry time for eviction. The cache caps fact data at 1 GiB and evicts oldest entries until usage is at most 768 MiB.

`--refresh` bypasses cache reads and writes successful fresh facts. Process errors and timeouts are not cached. Extractors can reject successful process output when it is partial. Go caches only clean module loads. TypeScript rejects dependency-cruiser output with unresolved imports. Other extractors decide whether a non-zero exit contains usable facts.

The key has limits that vary by analyzer:

- Python extraction does not use the fact cache. It validates an explicit installed tool environment when configured; otherwise the isolated local uv environment is resolved outside the repository. No target cache or virtualenv establishes freshness.
- TypeScript keys supported data-only `tsconfig` inputs, Node dependency metadata, owned runtime/compiler identity and fixed judge options. Project dependency-cruiser configuration is ignored. Incomplete source inventories and unusable graphs are not cached.
- Go keys each workspace member with Go toolchain version, Go environment, build settings, module files, and the member's transitive workspace dependency inputs. The Go environment includes `GOOS`, `GOARCH`, `CGO_ENABLED`, build flags, and toolchain options. The Go toolchain version identifies standard-library facts.
- Go bypasses cache for a member whose dependency closure uses a local `replace` or an unkeyed workspace module. A local replacement in `go.work` disables Go caching for the full run.

The cache-key schema changes when serialized facts or their derivation semantics change. A cache entry never raises the evidence trust ceiling. It changes extraction cost, not evidence completeness or provenance.

Delete `.archfit-cache` to remove cached facts and LLM responses. Cache contents are disposable. A cache miss causes extraction to run again.

## Analyzer trust boundary

A required import rule cannot pass from missing compiler/source evidence. CLI `needs_attention`
with unmeasured hard gates is not a successful measurement; the App must keep such required-rule
evidence actionable. A tool version or a nonempty index alone does not establish completeness.
Rust target attribution uses Cargo metadata and preserves ambiguity on live and cached results.

Image digests bind installed files, not remote execution. Reports remain self-reported/customer-
attested. Target code can run in optional indexers/build tools; container network access and
mounted source remain part of the operator's trust boundary. See [Languages and tools](languages.md).

## .NET trust limits

- **Target code runs.** MSBuild evaluation executes `Directory.Build.props`, imported targets, and SDK props. The Roslyn design-time build also writes generated files under `obj/`. Evaluation runs on the original tree. The helper runs on a disposable snapshot of the analysis root plus the ancestor build inputs up to the repository root, deleted after the run. Build inputs above the repository root are not in the snapshot; the `roslyn` detail names them. The image contract analyzes the .NET fixture from a read-only mount and checks the tree is unchanged. This is the same trust class as `go/packages` and cgo. Evaluation leaving `bitwarden/server` clean is a measurement, not a guarantee.
- **Helper location.** `ARCHFIT_DOTNET_ROSLYN` must name an absolute directory outside the analysis root, so the target cannot supply the helper. An invalid value drops the .NET graph and makes `dotnet-msbuild` partial; it never falls back silently.
- **NuGet-resolved SDKs.** The SDK resolver fetches `msbuild-sdks` from `global.json` when the network is reachable. Archfit does not restore packages. Offline, the affected projects are `failed` with the SDK named.
- **`global.json` pin.** A pin that the installed SDK cannot satisfy makes the projects `failed`. The reason names `global.json`.
- **Missing reference packs.** A target framework without its reference pack on the host (for example `net8.0` against a 10.0 SDK) leaves the project `partial`, with the pack named. Results depend on the host's installed packs.
- **Unresolved symbols are missing candidates.** An unresolved reference can hide a cross-boundary edge, so it is not reduced precision. Any `partial` or `failed` project, an evaluated project the helper did not report, or a failed or timed-out helper run keeps the `coupling` dimension from `measured` by blocking `extractor resolution within ceiling`, as TypeScript unresolved specifiers do above the ceiling.
- **Rules over incomplete projects are unevaluated.** A project's file edges are complete only when the helper loaded it `complete`. A required rule whose dependency source reaches a file of an incomplete project is unevaluated: its `from:` side, the modules a module rule reads, or every file for `cycle`. Incomplete means `partial`, `failed`, not reported, or any project when the helper produced no report. The reason names `roslyn`, each project, and that project's own cause. A rule whose scope is established and that produced a finding is not listed: the finding is its result, and the report must not both publish a rule's violations and say the rule was not evaluated. A rule whose scope the configuration does not establish stays listed even when it produced a finding: that finding comes from a file set the declaration did not define. Its blocker still fails the gate, so the verdict is unchanged. A rule scoped to complete projects stays evaluated, even when the `dotnet-msbuild` or `roslyn` row is `partial`. `ProjectReference` fallback edges do not make a project complete: they match no selector below the project directory, and they omit projects used through a transitive `ProjectReference`. The same files keep `structure`, `modularity`, and `complexity` from `measured`. This follows the Python precedent: unanalysed source in a rule's scope holds that rule, not every rule.
- **Partial runs are not comparable.** A Roslyn `partial` producer makes a `--base` comparison non-comparable. `roslyn` is its own analyzer family, so `--base` pairs it, and `config compare` grades its row the same way. The base checkout has no restore outputs of its own; it reuses the working tree's for projects at the same path (see [languages](languages.md#snapshot-and-trust)). Reuse is trusted only when NuGet's restore graph for the base project matches the one the working tree was restored from: a base project with different package references, central versions, sources, `global.json`, `NuGet.config`, or `Directory.*` build files, or one the working tree has not restored, stays `partial`, and `origin_reasons` quotes the base row's reason that names it. A restore graph records the path its tree was restored at, so outputs copied from a tree that has moved are reconciled by the project path they record; one whose recorded path cannot be reconciled stays `partial` with that cause, not a package-input one.
- **Producer identity.** The coverage row version is the helper's `archfit-roslyn <version>; Microsoft.CodeAnalysis <version>; SDK <version>` string.

See [languages](languages.md#net) for statuses and fallback.

## Upgrade limits

- Coverage sidecars are unsigned. Add authenticated producer evidence only when the product needs to prove producer identity or resist coordinated artifact and sidecar edits.
- Duplicate coverage aggregates use a lower bound when their unit and total agree. Different units or totals make coverage partial. Add per-unit identities when real multi-profile data needs an exact union.
- Fact-cache eviction walks the cache tree when it crosses the size cap. Add an index only if profiling shows that this walk affects analysis time.
