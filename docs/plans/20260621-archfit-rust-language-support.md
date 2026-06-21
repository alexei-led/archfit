# archfit: Rust support + language/tool abstraction refactor

> **Executable ralphex plan.** Run with `ralphex` from the repo root. ralphex executes
> each `### Task` sequentially in an isolated subagent; every task must end green
> (`make build` + `make test` + `make lint`) before the next starts. ralphex moves the
> plan to `docs/plans/completed/` on completion.
>
> Source: three Explore passes + three design passes (2026-06-21). User-locked scope:
> crate-level Rust via `cargo metadata` + opt-in `rust-analyzer scip` strength; full
> refactor (god-module splits + language abstraction) before Rust; e2e against
> `BurntSushi/ripgrep` (multi-crate) and `casey/just` (single-crate).

## Overview

Add Rust as a first-class language, but first make adding _any_ language cheap and
correct. Today one clean seam exists (`ports.Extractor`, `internal/ports/ports.go:30-38`)
but language-specific logic is hardcoded across ~25 sites and **leaks into core-ring
decision packages** that `internal/arch_test.go` forbids from importing adapters.

Two problems fixed, then Rust lands as the proof:

1. **Core-ring leakage** — `classify/distance_structure.go:76` splits module names on a
   slash-vs-dot heuristic; `metrics/boundary/change_locality.go` hardcodes Python
   `.py`/`.pyi`/`__init__.py` candidates; `metrics/internal/modgraph/modgraph.go:62`
   switches on `"python"/"go"/default-TS`; `score.go` lists tool names; `model/graph`
   has a `languagePriority` switch. A pure `NodeConvention` value-type in the model ring
   makes these declarative and importable by core.
2. **Composition-root sprawl** — a hardcoded extractor slice + parallel per-language maps
   in `cmd/archfit/pipeline.go`, `doctor.go`, `install.go`. A `LanguageDescriptor`
   registry in `cmd` collapses them to one ordered table.

Outcome: "add a language" = one registry row + one `internal/extract/<lang>` package.
Rust ships crate-granular (`package:<crate>` nodes, `depends_on` edges from
`cargo metadata`) with opt-in `rust-analyzer scip` strength, wired exactly like the
existing scip-go/scip-typescript/scip-python indexers.

## Context (from discovery)

Verified seam — the entire language contract (`internal/ports/ports.go:30-38`):
`Name() string` + `Extract(ctx, scope.Scope) (graph.Facts, diagnostic.Coverage, error)`.
Missing toolchain returns empty `Facts` + `Coverage{Status:"absent"}`, **never an error**.

| Area                                    | Location                                               | Note                                    |
| --------------------------------------- | ------------------------------------------------------ | --------------------------------------- |
| Extractor slice (hardcoded)             | `cmd/archfit/pipeline.go:97-102`                       | → registry-driven                       |
| Coverage/tool maps                      | `cmd/archfit/pipeline.go:285-314`                      | → derived from registry                 |
| CLI alias switch                        | `cmd/archfit/pipeline.go:431-441` `canonicalLang`      | → `languageByAlias`                     |
| install switch + `--lang` enum          | `cmd/archfit/install.go:23-30`                         | → registry + guard test                 |
| doctor tool list                        | `cmd/archfit/doctor.go:17-37`                          | language tools from registry            |
| lang constants + `ForExtract` py branch | `internal/config/config.go:34-36,664-674`              | + `LangRust`, Rust branch               |
| module-name split heuristic             | `internal/classify/distance_structure.go:76-81`        | → `NodeConvention.ModuleSegments`       |
| Python file candidates                  | `internal/metrics/boundary/change_locality.go`         | → `NodeConvention.ModuleFileCandidates` |
| `FileToModuleKey(file,lang)` switch     | `internal/metrics/internal/modgraph/modgraph.go:62-83` | → `NodeConvention.FileToModuleKey`      |
| `languagePriority` switch + Lang consts | `internal/model/graph/graph.go:95-113`                 | → `NodeConvention.Priority`             |
| `primaryExtractors` literals            | `internal/score/score.go:464-470`                      | → injected via `RunInput`               |
| SCIP indexer detect                     | `internal/extract/scip/scip_strength.go:215-233`       | + `rust-analyzer` arm                   |
| language detection                      | `internal/initcfg/initcfg.go` `Discover*`              | + `discover_rust.go`                    |

Architecture ring invariants to preserve (`internal/arch_test.go`, `TestArchImports`):
core ring (`classify`, `rules`, `metrics/**`, `status`, `staleness`, `facts`, `score`,
`scope`) must not import `os`/`os/exec`/YAML/adapters/`internal/llm`; `model/**` is
stdlib-only (so `NodeConvention` is safe there); every subprocess goes through
`toolrun.Runner`; LLM only in `cmd/`.

Templates to copy: `internal/extract/ts/ts.go` (subprocess + JSON), `internal/extract/py/py.go`
(absent-coverage shape, `parseAndNormalize`).

## Development Approach

- **Testing approach**: Regular (code first, then tests), matching the repo's
  `go test -race` convention — tests are a **required deliverable of every task**.
- One logical unit per task; small focused changes; backward compatible (new config /
  `RunInput` / `Diagnostic` fields are additive + `omitempty`).
- **CRITICAL — Phases 1-2 are behavior-preserving.** The golden double-run
  (`go test ./internal/engine/ -run TestGolden`) must stay **byte-identical**; the
  Go/TS/Python `NodeConvention` entries are exact copies of today's heuristics. Phase 3
  adds Rust only in `cmd` wiring + a new extractor package — the engine golden test
  builds its own `RunInput.Extractors`, so it stays zero-diff. If any golden fixture
  gains a `cargo` absent-coverage record, regenerate **deliberately**, inspecting the diff.
- **CRITICAL — every task ends with `make test` + `make lint` clean** before the next.
  Re-run `go test ./internal/ -run TestArchImports` explicitly after Tasks 4-7 and 10-11.
- Update this plan as scope shifts (➕ new task, ⚠️ blocker).

## Testing Strategy

- **Unit tests**: required per task; table-driven where there's an input/output matrix.
  `NodeConvention` and the Rust extractor (fake `toolrun.Runner`) get dedicated tables.
- **Golden / determinism**: `internal/engine/golden_test.go` double-run byte-identical;
  regenerate only deliberately when output schema changes.
- **Ring invariants**: `go test ./internal/ -run TestArchImports` gates every task that
  adds imports; new `TestBuiltinConventionsCoverage` keeps the registry honest.
- **No e2e/UI** harness in-repo; CLI behaviour checks live under `cmd/archfit`. Real-repo
  Rust runs (ripgrep, just) are in the final validation task and Post-Completion.

## Progress Tracking

- Mark completed items with `[x]` immediately when done.
- Add newly discovered tasks with ➕ prefix; blockers with ⚠️ prefix.
- Keep this plan in sync with actual work.

## What Goes Where

- **Implementation Steps** (`[ ]`): code, tests, docs, and the automatable archfit runs.
- **Post-Completion** (no checkboxes): real-repo manual inspection that needs a Rust
  toolchain + network, and the optional `rust-analyzer` strength check.

## Implementation Steps

### Task 1: Split `internal/config/config.go` (inert, same package)

- [x] move types/consts into new same-package files: `tools.go` (`ToolConfig`/`ToolsConfig`/
      `ToolMode`/`GateMode`, `LangGo/LangTypeScript/LangPython`, `ToolScip/...`, `LLMConfig`),
      `modules.go` (`ModuleDef`/`RuleDef`/`ExceptionDef`/`ModuleRole`/`ModuleMap`),
      `views.go` (all `For*()` methods + `Lint`), `types.go` (`MetricsConfig`/`OutputsConfig`/
      `PatternDef`/`ExtractConfig`/...); keep `config.go` = `Config`/`Load`/`Default`/`validate`
- [x] no signature changes, no sub-packages (avoids circular imports) — pure relocation
- [x] run `go build ./... && go test ./internal/config/ -v` — green
- [x] run `go test ./internal/ -run TestArchImports` — ring unchanged
- [x] run `make test` + `make lint` — must pass before Task 2

### Task 2: Split `internal/engine/engine.go` (inert, same package)

- [x] move into new same-package files: `advisories.go` (`collectAdvisories`,
      `groupBCAdvisories`, `rollupFinding`, `mergeLocations`, `severityFor`,
      `bcAdvisoryWhy/RiskClause`, `couplingAdvisoryID`, `staleLabelID`), `labels.go`
      (`applyPinnedLabels`, exported `PairEvidence`, `buildClonePairSet`), `assemble.go`
      (`buildDynamicImports`, `deltaReport`, `countActive`, `stripPrefix`, `pathDir`)
- [x] keep `engine.go` = `Run`/`RunInput`/`Mode`/`extract`/`resolveEvidence`/`enrichEdges`
- [x] `go build ./...` — confirm `cmd/archfit/enrich.go` still resolves `engine.PairEvidence`
- [x] run `go test ./internal/engine/ -run TestGolden` — **byte-identical**
- [x] run `make test` + `make lint` — must pass before Task 3

### Task 3: Split `internal/initcfg/initcfg.go` (inert, same package)

- [x] move per-language detection into `discover_go.go`/`discover_ts.go`/`discover_py.go`
      and shared helpers into `helpers.go`; keep `Discover()` + `Render()` in `initcfg.go`;
      leave `yamledit.go` untouched
- [x] no signature changes — pure relocation
- [x] run `go build ./... && go test ./internal/initcfg/ -v` — green
- [x] run `make test` + `make lint` — must pass before Task 4

### Task 4: Add `NodeConvention` registry to `internal/model/graph` (model ring)

- [x] new `internal/model/graph/convention.go` (imports stdlib `strings` only):
      `NodeConvention{Language, ModuleSegmentSep, FileExtensions, Priority,
moduleFileCandidatesFn, fileToModuleKeyFn}` + methods `ModuleSegments`,
      `ModuleFileCandidates`, `FileToModuleKey`; `ConventionRegistry` map +
      `Lookup(lang)` (zero-value default sep `/`, low priority)
- [x] populate `BuiltinConventions` with `go`/`typescript`/`python`/`rust` — Go/TS/Python
      entries are **exact copies** of current heuristics (slash; dot + Python candidates;
      priorities 0/1/2); `rust`: sep `/`, ext `.rs`, priority 3
- [x] write table tests: `ModuleSegments` (go/ts slash, py dot, rust crate-dir),
      `ModuleFileCandidates` (python triple; nil for file-node langs), `FileToModuleKey`
      (go dir-collapse, py dotted, ts passthrough), `Lookup` unknown→default
- [x] no callers yet — `go build ./... && go test ./internal/model/graph/ -v` green
- [x] run `go test ./internal/ -run TestArchImports` + `make test` + `make lint`

### Task 5: Rewire core-ring leak sites to `NodeConvention`

- [x] `model/graph/graph.go` `languagePriority()` → `BuiltinConventions.Lookup(lang).Priority`
- [x] `classify/distance_structure.go`: `moduleSegments(mod)` → `moduleSegments(mod, lang)`;
      thread `e.Language` from the edge into `codeStructureDistance`
- [x] `metrics/boundary/change_locality.go`: `nodeFileCandidates(n)` takes a looked-up
      `graph.NodeConvention` (via `modgraph.DominantLanguage(g)`) → `conv.ModuleFileCandidates`
- [x] `metrics/internal/modgraph/modgraph.go`: `FileToModuleKey(file,lang)` →
      `BuiltinConventions.Lookup(lang).FileToModuleKey(file)`; leave `ModuleKey` `.go`-collapse
      as-is (Rust/Py module IDs pass through — verified)
- [x] update/extend the affected packages' unit tests for the new signatures
- [x] run `go test ./internal/engine/ -run TestGolden` — **byte-identical** + `TestArchImports`
- [x] run `make test` + `make lint` — must pass before Task 6

### Task 6: Inject primary-extractor tool names via `RunInput` (remove score.go literals)

- [x] add `PrimaryExtractorTools []string` to `engine.RunInput`; pass it through to
      `score` synthesis in place of the `primaryExtractors` package var in `score.go`
- [x] `cmd/archfit/pipeline.go`: set the field from the (Task 7) registry helper; until
      then a temporary literal `["go/packages","dependency-cruiser","grimp"]` keeps output stable
- [x] update `score` + `engine` tests for the injected field; assert empty-slice fallback
- [x] run `go test ./internal/engine/ -run TestGolden` — **byte-identical**
- [x] run `make test` + `make lint` — must pass before Task 7

### Task 7: Add `cmd/archfit/registry.go` language registry; rewire pipeline/doctor/install

- [ ] new `cmd/archfit/registry.go`: `LanguageDescriptor{ID, Aliases, ProjectMarkers,
NewExtractor(toolrun.Runner, config.ExtractConfig) ports.Extractor, PrimaryTool,
InstallHint, DoctorTools, SCIPIndexer}` + ordered `languageRegistry` (go/ts/py) +
      `buildExtractors`, `languageByAlias`, `primaryExtractorTools`
- [ ] `pipeline.go`: slice → `buildExtractors(deps.Runner, cfg)`; build `coverageToolConfigKey` + `toolAffectedMetrics` by ranging the registry (lizard/jscpd/gitnexus stay literal);
      `RunInput.PrimaryExtractorTools = primaryExtractorTools()`; `canonicalLang` → `languageByAlias`
- [ ] `doctor.go`: append `lang.DoctorTools` from registry (shared git/sg/uv stay literal);
      `install.go`: `Run()` dispatches via `languageByAlias` (keep kong `enum` tag string)
- [ ] add `TestBuiltinConventionsCoverage` (every canonical lang id has a convention) and
      `TestLangAliasesInInstallEnum` (registry aliases ⊆ kong enum) to `internal/arch_test.go`
- [ ] **preserve extractor order** go→ts→py (golden depends on graph-merge order)
- [ ] run `go test ./internal/engine/ -run TestGolden` + `TestArchImports` — green;
      `archfit doctor` lists the same tools; `make test` + `make lint` before Task 8

### Task 8: Add `internal/extract/rust` extractor (`cargo metadata`)

- [ ] new package `internal/extract/rust/{doc.go,rust.go,rust_test.go}` modeled on `ts.go`:
      `New(runner, cfg)`, `Name() "rust"`, `Extract`; applicability = `Cargo.toml` at `s.Root`
- [ ] command `cargo metadata --format-version 1 --no-deps` (+`--features`, +`--manifest-path`
      when set), `WorkDir s.Root`; version via `cargo --version`
- [ ] `parseAndNormalize`: emit `package:<crate>` (`NodeKindPackage`) per workspace member,
      `external:<crate>` (`NodeKindExternal`) for registry deps; `Edge{Kind:EdgeKindDependsOn,
Language:"rust", Confidence:"high", Locations:[{File:"Cargo.toml"}]}`; skip dev-deps unless
      `IncludeDevDeps`; `Coverage{Tool:"cargo", FilesSeen=FilesApplicable=len(members), Status:"ok"}`
- [ ] mode behavior copied from `ts.go`: Off / Auto-without-marker / Auto-without-`cargo` →
      empty Facts + absent coverage, no error; On-without-marker / On-without-binary /
      non-zero exit → error
- [ ] write table tests with a fake `toolrun.Runner`: multi-crate workspace fixture JSON,
      single-crate fixture (`FilesSeen=1`, no edges, no error), absent-cargo, mode matrix
- [ ] run `go test ./internal/extract/rust/ -v` + `make test` + `make lint` before Task 9

### Task 9: Config — Rust language key, view fields, and `ForExtract` branch

- [ ] `internal/config/tools.go`: add `LangRust = "rust"`; add to `Default()` tools map (`ModeAuto`)
- [ ] `internal/config/types.go`: add `ExtractConfig` fields `CargoManifest string`,
      `CargoFeatures []string`, `IncludeDevDeps bool`; add top-level YAML fields
      `rust_manifest`/`rust_features`/`rust_include_dev_deps` to `Config`
- [ ] `internal/config/views.go` `ForExtract`: add a `lang == LangRust` branch mapping the
      new fields into the view (mirror the existing `LangPython` PyPackage branch)
- [ ] write/extend config tests: `ForExtract("rust")` populates the Rust fields; default mode
- [ ] run `go test ./internal/config/ -v` + `make test` + `make lint` before Task 10

### Task 10: Register Rust in registry + doctor + initcfg

- [ ] `cmd/archfit/registry.go`: append the Rust `LanguageDescriptor` (`ID:"rust"`,
      `Aliases:["rs"]`, `ProjectMarkers:["Cargo.toml"]`, `NewExtractor: rust.New`,
      `PrimaryTool:"cargo"`, `InstallHint:"https://rustup.rs"`, `SCIPIndexer:"rust-analyzer"`,
      doctor tools `{"cargo",...}`+`{"rust-analyzer",...}`); add `rust` to install kong `enum`
- [ ] `internal/initcfg/discover_rust.go`: `DiscoverRust(root)` → crate modules from
      `cargo metadata` member manifest dirs; add `HasRust` to `DiscoverResult`, call in
      `Discover()`, emit `tools.rust` in `Render()`
- [ ] write tests: `languageByAlias("rs")`→rust, `buildExtractors` includes rust, `DiscoverRust`
      on a fixture; confirm `TestBuiltinConventionsCoverage` + `TestLangAliasesInInstallEnum` pass
- [ ] run `go test ./internal/ -run TestArchImports` + `TestGolden` (zero-diff) + `make test` + `make lint`

### Task 11: Wire `rust-analyzer scip` strength (opt-in, gated)

- [ ] `internal/extract/scip/scip_strength.go`: `const indexerRust = "rust-analyzer"`; Rust arm
      in `detectIndexer` (`Cargo.toml` → `rust-analyzer`, project name via a small
      `cargoPackageName(root)` TOML-name scan mirroring the `go.mod` parse)
- [ ] `indexArgs` case → `["scip","--output",out]`; extend `reasonScipNoIndexer` to mention `rust-analyzer`
- [ ] graceful no-op when binary absent (same as scip-go/scip-typescript/scip-python)
- [ ] write tests: `detectIndexer` picks `rust-analyzer` on a Cargo fixture; absent-binary path
      returns empty strengths + absent coverage (no error)
- [ ] run `go test ./internal/extract/scip/ -v` + `TestArchImports` + `make test` + `make lint`

### Task 12: Docs — Rust language guide + extractor doc

- [ ] `docs/guide/languages.md`: add a Rust section (requires `cargo`; optional `rust-analyzer`
      for strength; crate-granularity + its ceiling)
- [ ] `internal/extract/rust/doc.go`: state the accepted tradeoff — crate-level graph;
      single-crate repos yield one node; intra-crate module edges out of scope (SCIP = upgrade path)
- [ ] update root `README.md`/CLAUDE.md language list if Rust belongs there
- [ ] run `make lint` (markdown/links if checked) + `make test` before Task 13

### Task 13: Verify acceptance criteria + end-to-end Rust runs

- [ ] run full `make all` (fmt → lint → test → build); `go test ./internal/engine/ -run TestGolden`
      byte-identical; `go test ./internal/ -run TestArchImports` green; coverage ≥ repo standard
- [ ] e2e ripgrep: clone `github.com/BurntSushi/ripgrep`, `.bin/archfit init` then
      `.bin/archfit check --root <ripgrep>` → assert `package:` nodes for `ripgrep`+`crates/grep-*`+`ignore`,
      crate-to-crate `depends_on` edges, `external:` deps, coverage `cargo` ok
- [ ] e2e single-crate: clone `github.com/casey/just`, `.bin/archfit check --root <just>` →
      exactly one `package:just` node, no intra-workspace edges, `FilesSeen=1`, **exit 0 / no error**
- [ ] `.bin/archfit doctor` lists `cargo`+`rust-analyzer`; non-zero exit only when a required tool missing
- [ ] _(if cargo absent in the runner, mark these e2e items blocked ⚠️ and move them to Post-Completion)_

## Technical Details

- **`NodeConvention`** (model ring, pure data + pure funcs): `ModuleSegmentSep` removes the
  slash-vs-dot heuristic; `moduleFileCandidatesFn` carries Python's `.py`/`.pyi`/`__init__.py`
  mapping; `fileToModuleKeyFn` carries the per-language git-file→module-key mapping; `Priority`
  replaces `languagePriority`. Core packages call `BuiltinConventions.Lookup(lang)` — no switches.
- **Rust `FileToModuleKey`** maps a `.rs` git path to its crate via a directory heuristic
  (`crates/<name>` / workspace-member prefix). Precise crate boundaries need member paths the
  core ring lacks (no FS) — documented ceiling; change-coupling is a secondary metric.
- **`cargo metadata` parse**: `cargoMetadata{Packages,WorkspaceMembers,WorkspaceRoot}` /
  `cargoPackage{ID,Name,ManifestPath,Source,Dependencies}` / `cargoDependency{Name,Source,Kind}`.
  `WorkspaceMembers` defines first-party set; deps with `Kind==nil`(normal)/`"build"` become
  edges (members→`package:`, else `external:`); dev-deps skipped unless `IncludeDevDeps`.
- **Extractor order** stays go→ts→py→rust (graph-merge dedup tiebreak = `Priority`).

## Post-Completion

_Items requiring a Rust toolchain / network / human review — no checkboxes, informational only._

**Manual verification** — local steps:

- If `cargo`/`rust-analyzer` are unavailable in the ralphex runner, perform the ripgrep +
  casey/just e2e runs locally and confirm the node/edge shapes and exit codes above.
- With `tools.scip.enabled: on` on ripgrep, confirm the `rust-analyzer scip` strength pass
  runs (and no-ops cleanly when the binary is absent).
- Inspect any deliberate golden regeneration diff if a fixture gained a `cargo` coverage record.

**Follow-ups** — out of scope, note only:

- Intra-crate module-level edges (ast-grep `use`-scan / cargo-modules) if single-crate
  precision is later required.
- Generalize the Rust crate-boundary heuristic into member-path data passed to metrics.
