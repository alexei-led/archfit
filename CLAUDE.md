# archfit

Architecture-fitness CLI (Go). Reads dependency facts from a target repo, checks
them against `.archfit.yaml`, emits gate violations + metrics for CI and AI agents.
Language facts come from external tools run out-of-process: `go list`,
dependency-cruiser, ast-grep, grimp, `cargo metadata`, jscpd, SCIP.

## Commands (Makefile)

- `make build` — static binary, `CGO_ENABLED=0` → `.bin/archfit`
- `make test` — `go test -race -coverprofile=coverage.out ./...` + `python3 internal/extract/scip/scip_reader_test.py` + `bash scripts/tests/cli_exit_contract_test.sh` (CI runs both non-Go steps too)
- `make lint` — `golangci-lint run -c .golangci.yaml ./...` (pinned v2.1.6)
- `make fmt` — `gofmt -s` + `goimports -local github.com/alexei-led/archfit`
- `make archfit` — dogfood architecture-drift gate: `.bin/archfit check --config .archfit.yaml`
- `make arch-lint` — architecture drift linter (alias for `make archfit`); wired into the pre-push hook
- `make archfit-report` — write `docs/reports/archfit-report.md` via `archfit analyze --markdown`
- `make mock` — regenerate moq fakes (`go generate ./...`)
- `make test-fast` — `go test -race -short ./...` (skips slow subprocess/ast-grep integration tests; for inner-loop speed)
- `make bench-gate` — cold vs warm fact-cache gate timing on this repo (reported number, not a CI assert; `scripts/bench-gate.sh`)
- `make all` — fmt → lint → test → archfit
- One test: `go test ./internal/<pkg>/ -run TestName`

## Structural gates (CI runs these explicitly — keep green)

- Import ring: `go test ./internal/ -run TestArchImports`
- Golden output: `go test ./internal/engine/ -run TestGolden` — regenerate
  deliberately and inspect the diff; output changes are never automatic.
- Dogfood gate: `make archfit` — CI runs the same target after tests/goldens. Also
  runs locally pre-push via the `arch-lint` hook in `.pre-commit-config.yaml`. The
  self-config (`.archfit.yaml`) gates its own architecture: forbidden-dependency
  ring + `forbidden_layer_direction` (fail).
  `public_api_type_leak` runs advisory (warn).

## Invariants

Enforced by `internal/arch_test.go`; extend that test when adding a boundary.

- Core ring (`classify`, `rules`, `metrics` + sub-packages, `status`, `staleness`,
  `facts`, `score`, `scope`, `syntax`) must not import `os`, `os/exec`, any YAML
  lib, or adapter packages — it decides over already-gathered facts. `score`
  synthesises the coupling_balance band from an already-computed `Diagnostic`.
  `syntax` classifies each source file (Production/Test/Generated/Vendor) and
  exposes `LookupFileClass`/`IsTestFile` used by the LOC walk and by metrics that
  filter on Production files.
- `internal/model/*` imports stdlib only (exception: `model/module` may use the
  pure doublestar glob matcher). The kernel plus `internal/view` is a frozen
  published contract: its exported surface is pinned by
  `TestModelSurfaceNoDrift` (golden: `internal/testdata/model_surface.golden`);
  regenerate deliberately with `ARCHFIT_UPDATE_SURFACE=1` and call the contract
  change out in review.
- `internal/view` holds the data-only stage contracts (ClassifyConfig,
  ExtractConfig, RuleDef, …). Stages import `view`, never `internal/config`;
  only composition roots (`cmd/*`), `internal/engine`, and
  `internal/configschema` may import `internal/config` (enforced by
  `*_no_config` rules in `.archfit.yaml`).
- Every subprocess call goes through `toolrun.Runner` (interface in
  `internal/toolrun/toolrun.go`); extractors in `internal/extract/{go,ts,py,rust}`
  are out-of-process adapters. No `exec.Command` in core code — fake the `Runner`
  in tests.
- **Fact cache** (`internal/factcache`, adapter — core ring must not import it;
  see `docs/design/fact-cache.md`). Content-addressed extractor-fact store under
  `.archfit-cache/facts/`; stores facts, never scores. Runner-shaped analyzers
  (depcruise, grimp, cargo, SCIP, ast-grep) wrap `toolrun.Runner` in
  `factcache.Runner`; Go (`packages.Load`) and jscpd (temp-file output) use the
  store directly. Keys hash tool version + config slice + input tree; the
  input-tree hash must cover the TOOL'S real input set — config `exclude:` globs
  never filter it (the tools don't honor them; over-hash, never under-hash).
  Partial/timed-out/dirty results are never cached; a Go member whose build
  reaches source no key covers (local `replace` in a member go.mod, unkeyed
  go.work sibling) is vetoed per-member; a local `replace` in the go.work file
  itself disables the whole run's Go cache. `--no-cache` bypasses reads AND
  writes on all caches.
- **No gitnexus.** The `.gitnexus`/`.codegraph` index dirs are excluded from file
  walks (`scope.go`), but archfit does not run the tool and does not derive any
  per-module fact from it.
- **Severity source is `cl.Score.Band`** (`classify.go`, `Run`). `cl.Severity =
cl.Score.Band` after the scorer runs. `BalanceResult` is deleted — it was the
  old discrete severity table and is no longer called anywhere. Do not re-introduce
  it; the book formula (`ScoreVersion = "bc_score.v6"`) is the single severity source.
- **Coupling gate** (`coupling.gate: {min_band, max_drop}`). The synthesised
  coupling_balance score can fail the verdict: `score.Synthesize` +
  `applyCouplingGate` run INSIDE `runPipeline` (`cmd/archfit/pipeline_run.go`),
  before `agenttask.Build`, so a tripped gate escalates `diag.Verdict` and
  promotes the active BC advisories to `Kind: "gate"` (they flow into
  `agent_tasks[]` through the unchanged agenttask filter). `min_band` is a band
  floor; `max_drop` compares against the score snapshot `archfit baseline`
  stores (`baseline.ScoreSnapshot`, omitted when unmeasured). BandNA never
  gates (abstain ≠ fail). Trip reasons print to stderr from `analyze` ONLY
  (re-evaluated there via the pure `score.EvaluateCouplingGate`) — never from
  baseline/enrich/explain/`--base` scoring, which share `runPipeline`. A separate
  stale-baseline notice can also print from `analyze` when `max_drop` is skipped
  because the stored score snapshot is incompatible with this binary —
  `baseline.ScoreSnapshotMismatches` names the offending input
  (`score_version`, `rubric_version`); a snapshot written before rubric tracking
  reads as rubric `1` and still anchors.
  The score comes from `ClassifiedEdges` (pre-advisory-filter), so a trip with
  no promotable advisory (advisory off, or `coupling.min_severity` above every
  active edge) emits one synthetic `bc/coupling_gate` gate finding carrying
  the trip reasons — a FAIL verdict never ships with 0 gate findings.
  `archfit baseline` persists BC findings with their NATIVE advisory kind
  (promotion is per-run; a stored "gate" kind orphans the entry for
  `status.Assign`) and skips the synthetic trip finding entirely. The
  `couplingGateView` projection lives in cmd, NOT
  on `Config` — config (support layer) must not import score (core layer); the
  dogfood gate catches that inversion.
- **FileClass facility** (`internal/model/fileclass`, `internal/syntax/fileclass`).
  Every source file is classified as `Production | Test | Generated | Vendor` once
  during the LOC walk; the result is stored in `SizeSignals.FileClassIndex`. Use
  `syntax.LookupFileClass` for path→class lookup with fallback. Metrics that
  filter on Production files use this index and report the excluded count.
  Config override: top-level `file_class:` key (`FileClassDef`), projected via
  `Config.ForFileClass()` → `syntax.FileClassConfig`.
- **`archfit analyze --base <ref>`** flag (`cmd/archfit/worktree.go`). Creates a
  clean detached temp worktree at `<ref>`, scores both sides with the full advisory
  pipeline, and emits a dimension-by-dimension delta table. Off-gate, report-only
  (exit 0 on success, exit 3 on git/config error). Both sides use the current
  `--config`. Formats: `text` (default), `json`, `markdown`.
  The base sub-run receives the caller's EFFECTIVE head config (after `--lang` /
  `--min-severity`) and never reparses the file. `analyze.go` hands it a copy
  with an independent `Modules` map (`withIndependentModules`) — `config.Config`
  is a value but its map is shared, and `runPipeline`'s owner/deploy-unit
  backfill writes through it, so without the copy the base side would inherit
  head-tree owners and skip its own resolution.
- **`git_finding_delta`** (`cmd/archfit/git_finding_delta.go`) — report-only JSON
  block emitted only with `--base`, classifying the CURRENT `agent_tasks[]` as
  `introduced` / `pre_existing` / `unknown` origin. Pointer + `omitempty`, so a
  run without `--base` stays byte-identical; never changes the verdict, the exit
  code, or text/markdown/scorecard/SARIF output. Matching uses stable finding IDs
  only (lifecycle labels and gate/advisory promotion ignored; base `status=fixed`
  entries dropped). An unmatched task is `introduced` ONLY when every ACTIVE
  finding-producing analyzer family compared equivalently — otherwise `unknown`,
  never a fabricated new task. One family = one `ToolCoverage` name: the
  per-language primaries, the `ast-grep` pattern pass, the `ast-grep/syntax`
  pass, and opt-in `scip`/`scip-symbols`/`jscpd`/`cargo-modules`. Pairing rules:
  equal statuses always pair; the only cross-status pair is `ok` against a
  gapless-`absent` PRIMARY, which means the language is not in that tree. A
  gapless `absent` on a NON-primary analyzer is evidence about the tool, not the
  tree, so it pairs only with itself — symmetric absence is safe (neither side
  produced findings), asymmetric absence is not. A timeout, a missing row, and a
  duplicate row are all unavailable evidence.
  Isolation: only base finding IDs, coverage rows/gaps, and the config hash
  cross over — `runScoreSide` projects the base Diagnostic to `baseEvidence` at
  the source, because base agent tasks carry paths and a validation command
  rooted in a temp worktree that is deleted on return.
  `comparison_status: comparable` can still ship with non-empty
  `comparison_reasons`: the status reports task placement, the reasons report
  evidence.
  **Shared with `config compare`'s `decision.gradeTool` (keep these three in
  step):** one row per tool per side, so a repeated coverage name is unpairable;
  the coverage-gap condition is PER SIDE, so a gap on one side only is an
  asymmetry; symmetric absence never blocks, gapped or not. Symmetric
  `absent`-WITH-a-gap (an enabled analyzer whose tool is not installed) pairs on
  BOTH paths — `comparable_with_gaps` in `decision.gradeTool`,
  `familyPairedDegradedAbsent` in `pairFamily`, always with a disclosed reason.
  Failing it made `--base` permanently all-`unknown` for a whole class of
  environments, including archfit's own runtime image (no Rust toolchain) on any
  repo carrying a `Cargo.toml`. The two paths deliberately DIFFER once, because
  `--base` compares two trees and `config compare` compares one: `ok` vs
  gapless-`absent` primary is comparable for `--base` (a language appearing is
  expected) but `not_comparable` for `config compare` (a status change on one
  tree is config-caused).
- **`partial` means two different things and the TOOL NAME separates them, not
  `Coverage.Unresolved`** (`decision.PartialFromUnresolvedSpecifiers`, the single
  predicate both pairing paths call). dependency-cruiser and grimp mark a
  COMPLETED run partial as soon as one import specifier anywhere fails to resolve
  — the normal steady state on any TS/Python repo, which
  `score.tsUnresolvedRatioCeiling` already tolerates to 10%. `go/packages` ALSO
  sets `Unresolved`, but there it counts whole packages it SKIPPED because they
  failed to load (`collectNodesEdges`, synthetic-error packages), which is the
  "did not finish" meaning and must never grade comparable — so `Unresolved > 0`
  is not a completion marker and never was. Every remaining partial producer (a
  failed extractor in `engine.extract`, a rejected ast-grep rule file, an empty
  SCIP index, a failed jscpd run) leaves `Unresolved` at zero. A SYMMETRIC
  unresolved-specifier partial pairs (`comparable_with_gaps` in
  `decision.gradeTool`, `familyPairedDegradedUnresolved` in `pairFamily`) and
  always discloses a reason CARRYING BOTH MAGNITUDES — the rule is
  magnitude-blind, so `3 unresolved` and `5000/6000 unresolved` must be
  distinguishable in the output. Treating all partial as unusable made both
  features permanently inert on TypeScript and Python. When adding a
  specifier-granular extractor, add its coverage name to
  `PartialFromUnresolvedSpecifiers`.
- **Extractor-failure coverage rows use `CoverageTool()`, not `Name()`**
  (`ports.Extractor`, `engine.extract`). A failed `Extract` returns a zero
  Coverage, so the engine stamps the row itself; filing it under the language
  name ("go") instead of the coverage name ("go/packages") created a phantom
  analyzer next to the real family and left the family with zero rows.
- **Coverage-gap suppression follows the analyzer's REAL discovery**
  (`primaryToolProjectProbe`, `cmd/archfit/pipeline_coverage.go`). A missing
  project marker suppresses the gap, and a gapless `absent` primary row is the
  one shape both pairing paths read as "language not present". Root-only marker
  checks are correct for dependency-cruiser, grimp, and cargo (all resolve from a
  root manifest) but wrong for Go, which walks for nested `go.mod` dirs — so Go
  probes the tree (`goProjectPresent`, pruning dependency/build dirs). Without
  it, a real go/packages failure in a `services/api/go.mod` repo was
  indistinguishable from "no Go here" and could manufacture a false `introduced`.
  The probe must match what the analyzer ACTUALLY does, not a rough stand-in:
  Go reuses the extractor's own `golang.FilterMembers`, so a
  `languages.go.modules` include/exclude that empties the member set never reads
  as a missing toolchain; Rust reads `languages.rust.manifest`
  (`rustProjectPresent`, shared with the cargo-modules row), so a configured
  sub-crate manifest is applicable with no root `Cargo.toml`. `buildCoverageGaps`
  reads `cfg.Exclude` AS GIVEN — `runPipeline` merges it once at setup and
  `scope.MergeExclusions` is NOT idempotent (it consumes `!` re-includes), so a
  second merge re-seeds defaults the user removed and hides markers the
  extractors saw.
- **A language switched off over a language that IS PRESENT reports `disabled`,
  never `absent`** (`markDisabledPrimaries`, `cmd/archfit/pipeline_coverage.go`,
  applied to `diag.ToolCoverage` before `buildCoverageGaps`). Extractors encode
  `ModeOff` as `StatusAbsent`, which both pairing paths read as "this language is
  not in the tree" and drop from the comparison — so two configs that BOTH
  disabled Go over a Go repo graded fully comparable while neither had looked.
  Rewriting the row leaves gapless-`absent` with exactly ONE cause (markers
  missing), which is what `decision.gradeTool` and `normalizeCoverage` already
  assume. Two conditions are load-bearing, both narrowing:
  `primaryDisabledByConfig` (mode off AND no explicit `gate:` — a pinned gate
  keeps `absent` so its gap and `--require-tools` still fire; it is also the
  single predicate behind the gap suppression), and `primaryLanguagePresent`,
  which runs the SAME probe `buildCoverageGaps` suppresses on. Without the
  presence probe the rewrite is the mirror image of the bug: a repo with no
  TypeScript is told TypeScript analysis is switched off, and
  `python: {enabled: false}` on a Go-only repo grades `not_comparable` against a
  config that merely left python unset. An empty root cannot be probed and
  answers "present" — disclose the opt-out rather than hide it, matching
  `buildCoverageGaps`' empty-root behaviour.
- **One coverage name per analyzer.** `internal/extract/astgrep` drives one
  binary for two passes and they report under two names: `ast-grep` (patterns)
  and `ast-grep/syntax` (`syntaxToolName`). Both consumers that pair coverage
  rows — `pairFamily` (git origin delta) and `decision.gradeTool` (config
  compare) — read a repeated name as an unpairable duplicate and grade the pair
  unavailable/`not_comparable`. Never give two analyzers one coverage name.
- **`archfit config compare <candidate>`** (`cmd/archfit/config_compare.go`,
  pure decision in `internal/decision.CompareConfigs`). Two full pipelines over
  ONE source tree, report-only: exit 0 on success, exit 3 on an input or runtime
  error; findings never move the exit code. Both sides use an EMPTY accepted
  baseline (never reads `.archfit-baseline.json` — it records findings accepted
  under the current config, so applying it would silence the candidate's
  findings by the current config's history), share the CURRENT config's bundle
  directory (pinned labels, fact cache) and one `EvaluatedAt`; only
  `ConfigSource` differs, so a candidate file outside the repo cannot move the
  analysis boundary. Config, baseline, labels, candidate, and policy files stay
  byte-identical; normal fact-cache reads/writes are the only filesystem effect.
  Finding buckets are `current_only`/`candidate_only`/`both` — never
  introduced/resolved, because alternative configurations have no time order —
  and no output ever says the candidate is better.
- **`archfit config update --json`** emits `archfit.config-review.v1`. The
  non-obvious part: `--json` with `--apply`, `--ai-classify`, or `--refresh` is
  a usage error (exit 3) rejected BEFORE discovery, tool calls, cache access, or
  any write. Schema in `docs/guide/commands.md`.
- **`config update` never destroys a configured module stanza.** `DiffModules`
  matches config keys to discovery keys by NAME, and the conventions need not
  agree — this repo configures `internal/agenttask` while discovery emits
  `agenttask` over the identical path set. `DiffModules` runs the name-drift
  pass ITSELF (`resolveNameDrift`, unexported — there is no two-step call a
  consumer can get wrong), reclassifying each 1:1 add/remove pair with an equal
  normalized path set as `NameDrift`. On top of that, `Removed` is review-only:
  `initcfg.HasModuleEdits` (module stanzas), `initcfg.HasPendingEdits`
  (`HasModuleEdits` plus settings — the single source for "would `--apply` write
  anything"), and `buildUpdateEdits` all exclude it, so `--apply`
  writes only added modules, path drift, and settings. Deleting or re-keying a
  stanza discards its `owner`/`subdomain`/`volatility`/`layer`/`public`, so both
  stay human decisions and the report says so (NAME DRIFT / UNMATCHED sections,
  `review_available` status). Before this, `config update --apply` on archfit's
  own config commented out all 44 modules and added 31 bare stanzas, and the
  status read `action_required` with 0 real issues.
  Two consequences of that name-only matching:
  (1) `candidateConfigForUpdate` (`cmd/archfit/update.go`) — the "config after
  `--apply`" projection the deploy-unit and distance suggestion builders read —
  resolves each discovered module through the drift pairs FIRST, so a drifted
  module enters under its CONFIG name carrying the config's metadata. Keying on
  the discovered name dropped `owner`/`deploy_unit`/`subdomain` and proposed
  fields the config already sets, under a module name `.archfit.yaml` does not
  contain. The config name cannot collide: it comes from `Removed`, which holds
  only names discovery did not emit.
  (2) `--apply` discloses the review-only half on BOTH branches. The write branch
  gates on `initcfg.HasReviewItems` and prints `initcfg.RenderAppliedReview`,
  which reuses `writeUnappliedModuleSections` + `writeModuleGapSections` — the
  same helpers `RenderUpdateReport` uses. Gating on `HasReviewSuggestions` there
  hid module gaps, name drift, unmatched and pathless stanzas exactly when apply
  had an edit to make.
- **`runContext`** (`cmd/archfit/pipeline_run.go`) replaces `runPipeline`'s
  positional path arguments. Each field selects one thing, and a caller that
  leaves one zero silently gets head-tree state on a base or candidate run:
  `ConfigSource` → config hash + validation command; `BundleDir` → pinned labels
  + fact-cache location; `ScanRoot` → scope + on-disk path resolution;
  `EvaluatedAt` → the single instant waiver expiry and staleness age against
  (zero samples `time.Now()` on read). Per-run values: normal analysis = current
  path / current config dir / current tree / persisted baseline; git base =
  same, with the base worktree as ScanRoot and an empty baseline; compare
  current and compare candidate = the common tree, the current config dir, one
  shared `EvaluatedAt`, empty baselines, and only `ConfigSource` differing.
- **Owner inheritance for auto-registered synthetic submodules**
  (`classify.AugmentModulesFromGraph`, `AugmentGoWorkspaceModules`): propagates
  `owner` from the nearest config-declared ancestor module to each synthetic module.
  Fixes inter-submodule edges defaulting to `cross_module_different_owner` (D=10)
  on single-team repos with many cargo-modules or Go workspace members.
- **SCIP empty-index reports `partial`/`warn`** (`internal/extract/scip/scip_strength.go`).
  When the resolved edge map is empty (`len(m)==0`), `Coverage.Status` is set
  to `StatusPartial` with reason
  `"empty index (0 occurrences) — check path case / indexer version"`. Previously
  this reported `ok`, silently hiding a SCIP indexer failure.
- **scanRoot vs gitRoot decoupling.** `Scope.Root` = ScanRoot (the analysis
  boundary; all extractors walk this tree). `Scope.GitRoot` = `git rev-parse
--show-toplevel` (git ops only). `Scope.SubtreePrefix = rel(GitRoot, Root)`.
  `--root` absent ⇒ ScanRoot=GitRoot, prefix="" ⇒ byte-identical. Non-git full
  mode proceeds with `GitRoot=""` (history empty); delta mode without git is a
  hard error.
  **macOS APFS case-variant `--root` (Task 25, fixed):** `snapScanRoot` in
  `internal/scope/scope.go` uses `os.SameFile` (device+inode) to snap a
  case-variant scan root to the git root's canonical path, so
  `/users/…/repo` and `/Users/…/repo` resolve to the same scope on
  case-insensitive APFS. `filepath.EvalSymlinks` still handles symlinks;
  `snapScanRoot` handles the case-mismatch that EvalSymlinks cannot fix.
  `caseInsensitiveSubtreePrefix` (same file) extends the fix to a case-variant
  _ancestor_ when `--root` is a subtree below gitRoot (walks scanRoot's
  ancestors via `os.SameFile` to locate gitRoot), so CODEOWNERS
  `SubtreePrefix` derivation survives a lowercase `--root` path. Degraded
  owner resolution (`owner_source=codeowners_no_match` or `git_timeout`) is
  disclosed on stderr via `ownerDegradationWarning` — never a silent fallback.
- **Go workspace loading.** Member discovery: `go.work` at or above ScanRoot
  (parsed in-process via `golang.org/x/mod/modfile`) → filter to members inside
  ScanRoot and not exclusion-matched; else single `go.mod`; else walk for `go.mod`
  dirs. Per-member `packages.Load({Dir: memberDir}, "./...")` concurrent
  (bounded to GOMAXPROCS). Per-package strip via `pkg.Module.Dir`. First-party =
  target `Module.Path` ∈ loaded member set. 1 surviving member = today's single-
  module path (byte-identical). `classify.AugmentGoWorkspaceModules` auto-registers
  each member as a synthetic module when **≥2** members were loaded and the
  member's `RelDir != "."` and no config module already covers it — mirrors the
  Rust `::` gate. `languages.go.modules.include/exclude` scopes which members load.
- **Per-analyzer timeout.** `analyzers.<x>.timeout` (Go duration string, e.g. `"5m"`)
  caps `scip` and `clones` (jscpd) subprocess runs. On timeout the result is
  dropped; dependent metrics report `n/a (timed out)`; the run continues on the
  verdict from the remaining analyzers.
- **Go edge strength** comes from `go/packages` type info (`NeedTypesInfo`): the
  resolved object kind (interface→contract, pure-data DTO struct or its
  fields→dto, concrete type→model, const/var use→model, func or func/chan-valued
  var→functional) is compiler-grade ground truth, so SCIP does **not** override
  it — `enrichEdges`
  keeps the Go type-info hint and uses SCIP strength only where type-info is absent
  (empty hint). SCIP-go is a coarser subprocess re-derivation that collapses
  imports to a blanket `functional`; letting it override flattened
  `coupling_balance`'s strength signal. For TS/Py/Rust (heuristic extractor hints)
  SCIP **does** override — it is their precision upgrade. Unclassified edges stay
  `unknown` (abstain-not-fake). A public-glob match is a not-intrusive _floor_
  whose kind the hint refines (classify.go); an internal-glob match is
  authoritative intrusive. The `dto` hint (rank 2, between contract and model)
  resolves to Contract only across a config-declared `public:` boundary — it is
  Go-only; Python/TS extraction can't see object kinds, Rust gets const/static
  precision via rust-analyzer SCIP terms instead (`docs/design/bc-measurement-v4.md`).
- **Python module globs are DOTTED, not file paths.** grimp emits dotted node IDs
  (`prefect.states`); `paths:`/`public:`/`internal:` and rule `from:`/`to:` all match the
  dotted node ID via `doublestar.Match`. Write `prefect.**`, NOT `src/prefect/**` — a slash
  glob silently matches nothing → every Python edge classifies external → 0 scored →
  `coupling_balance` n/a. Locked by `config_test.go:TestModuleFor_PythonDottedGlobs`.
  `pythonModuleFileCandidates` (`internal/model/graph/convention.go`) emits `src/`-prefixed
  candidates alongside the flat ones, mirroring `pythonFileToModuleKey`'s `src/`-stripping —
  keep the two symmetric or src-layout repos silently fail dotted-ID → file resolution.
- **agent_tasks `files[]` exist on disk (`agenttask.PathResolver`).** Every candidate
  (edge endpoints, locations, module keys) resolves index-first against the LOC walk's
  `FileClassIndex` with an injected `onDisk` os.Stat backstop (the LOC walk skips `mocks/`,
  `target/`, `venv/` which extractor exclusions do not) — resolve-or-drop, never a bare
  config key or dotted/`::` id. Rust `crate::mod` probes `<dir>/src/<mod>.rs|/mod.rs`
  then the crate dir (`src` for a root crate whose `CrateRoot.Dir` is `""`); the
  last-resort module root from `config.ModuleRootDirs` (dotted prefix for Python globs)
  goes through the same resolver. agenttask itself never touches the filesystem — the
  composition root (`cmd`) owns the `onDisk` closure.
- **TS coverage honesty: one unresolved ratio.** `score.tsUnresolvedRatioCeiling` (10%)
  caps `coupling_balance` confidence using `Unresolved/SpecifiersSeen` — the SAME
  specifier-denominator ratio the dependency-cruiser `Coverage.Reason` string and the
  `analyze` stderr warning disclose. Never reintroduce a `FilesSeen` denominator: the
  cap and the disclosure would contradict each other on the same run.
- **`coupling_balance` reports `n/a` (band `score.BandNA`) when unmeasured — never a
  fabricated number.** Zero scored cross-boundary edges (empty module map, non-matching
  globs, empty SCIP index, all-external) or a degenerate (<2 connected modules, e.g.
  single-crate Rust) graph → overall score renders `n/a`, not a mid-band 50/60 sentinel
  (`score.go` `finalize`/`Synthesize`, `score_boundary_coupling.go`). `finalize` early-returns
  for `BandNA` (skips clamp/cap/`bandFor`); delta builders and `decideBand` treat an n/a side
  as unknown (no phantom delta, not NEEDS_ATTENTION). The legacy nil-summary path keeps the
  non-penalising 60 (calibration-only, unreachable from engine.go).
- Parse config once into typed views; pass a package its view, not the whole config.
- LLM SDKs (`anthropic-sdk-go`, `openai-go`) are off-gate: only `config enrich`,
  `config init --ai-classify`, `config update --ai-classify`, `analyze --ai-summary`, and `explain --ai-summary`
  touch them, never the gate. Enforced structurally — `arch_test.go` forbids any
  `internal/*` package from importing `internal/llm`, so the LLM commands live in
  `cmd`.
- `gate:` is wired for **all rule types** (`off` skips, `warn` is advisory/non-blocking,
  `fail`/unset is blocking; exception: `public_api_change` defaults to `warn` when unset). An unknown `type` value is a config error.
  `metrics.<name>.gate` follows the same convention: a worsening baseline delta
  blocks when `gate` is unset. `MetricEntry.Enabled` is a `*bool` so a knob-only
  entry (`{gate: warn}`) stays enabled — only explicit `enabled: false` disables
  the metric (`metrics.New`). `coupling_balance` gates via the `coupling.gate:`
  block, not `metrics:` — see the coupling-gate invariant above.

## Coupling scorer — key design facts

`ScoreVersion = "bc_score.v6"` (`internal/model/coupling/scorer.go`).
Formula: `balance = max(|S−D|, 10−V) + 1` (Khononov Ch10 verbatim).
Ordinals frozen as named constants — changing any is a breaking metric change.

**Abstain-not-fake:** when strength OR distance is `unknown`, the edge is
unscored (`EdgeScore.Scored = false`). No invented ordinals. Genuine internal
edges with unknown strength stay in the `abstained` bucket (lowers
`coupling_balance` confidence); external/library edges (`DistanceUnknown`) are
excluded from the denominator entirely (counted in `classified_edges.external`).

**External edges excluded from `coupling_balance` — unless declared:** edges
whose target is not a declared module are NOT internal coupling seams; their
count surfaces in `classified_edges.external` and the `coupling_balance`
evidence string. Exception (introduced in bc_score.v4): a target matching a
config-declared `external_systems:` entry gets the frozen `DistanceExternal = 10` rung
(`distance_basis: declared_external`, book Ch10 Example 1) and ENTERS scoring
with the entry's volatility (default low); those count in
`classified_edges.declared_external`. The match is gated on the target's own
module resolution — a module-resolved target is never re-labelled external,
even when the edge's source is unresolved.

**Symmetric from clones:** when `analyzers.clones` detects a cross-module clone
pair, the edge strength is upgraded to `StrengthSymmetric` (S=9) only when the
edge's strength is still `functional` or `unknown` — config-authoritative
`contract`/`intrusive`, type-info `model`/`dto`, and approved pinned labels are
never overridden.

**`bc/duplicated_knowledge` (clone pair without an import edge):**
`classify.CloneOnlyPairs` builds each cross-module clone pair whose modules share
NO import edge (StrengthSymmetric, module-pair distance, worst-of-pair
volatility). Default `coupling.duplicated_knowledge: score` includes the pair in
`ClassifiedEdges` and `coupling_balance` as a score-bearing coupling fact; it
may also surface as a `bc/duplicated_knowledge` advisory after severity/baseline/
waiver filtering. `advisory` preserves the v4 behavior: advisory-only, held out
of the headline score. It is never promoted by the coupling gate (promotion
matches `RuleIDBCImbalanced` only). Ceiling: a pair WITH an edge is owned by the
symmetric-upgrade path above, so clone evidence on a contract/model/
intrusive-strength edge surfaces nowhere — deliberate.

**Same-module edges are scored but report-only (`local_coupling`):** classify
scores same-module edges at the book's D=2 rung; they surface in the
`local_coupling` JSON block (Ch10 local-complexity quadrant, per-module worst
offenders) but keep `SeverityNone`, stay out of `coupling_balance`'s
denominator (`assemble.go` early-continue), and never become advisories or
gate findings — fractal-level separation.

**Volatility provenance disclosure:** `classified_edges.volatility_provenance`
counts modules by volatility source (`declared`/`inherited`/`cascade`/
`undeclared`) and rides the `coupling_balance` evidence line, so a
uniform-by-inheritance repo (one declared ancestor fanned out to N synthetic
submodules) is not mistaken for N measured judgments.

**Provenance lowers confidence:** approved labels in `.archfit-labels.yaml` with
`provenance: llm` and `confidence` below `high` lower `coupling_balance`
confidence by one band. `provenance: human` and `provenance: tool` do not.

**Opt-in volatility cascade:** `coupling.volatility_cascade: true` in
`.archfit.yaml` enables a deterministic fixpoint propagation pass (book Ch9): a
module strongly coupled (`functional`/`symmetric`/`intrusive`) to a
high-effective-volatility module inherits `high`, and that can propagate through
strong-coupling chains. Clone-only pairs are excluded. archfit's own self-config
has this enabled.

**Runtime async is report-only:** `runtime_async` JSON field records async-bridge
evidence per module. The runtime/dynamic evidence detectors skip test files and
`testdata/` fixtures so examples do not become architecture-review signals. Never
annotates graph edges, never affects distance or balance score, never gates. This
is a deliberate design decision — do not wire async detection into distance.

**SCIP semantic overlay is report-only** (`internal/engine/semantic_overlay.go`,
`enrichEdges`). `semantic_strength_overlay.by_language` counts, per language,
how many heuristic extractor edges SCIP strength actually refined
(`candidate_edges`/`applied`/`missed` + before/after buckets). Only TS/Python/Rust
are tracked — Go strength is compiler-grade `go/types` and SCIP never overrides
it, so Go edges are excluded (the `e.Language == graph.LangGo && StrengthHint != ""`
early-`continue` runs before overlay tracking). Never consumed by
`coupling_balance`, findings, baselines, or gates. The block is omitted when SCIP
is absent/disabled/timed out or when no TS/Python/Rust candidate edges exist. If
SCIP returns `StatusOK` or `StatusPartial`, candidate languages still appear when
the strength map is empty: `applied=0`, `missed=candidate_edges`. Use SCIP coverage
status as the run signal; do not add a duplicate config-derived enable flag.

## Release (tag-triggered — never release manually)

`git tag -a vX.Y.Z -m … && git push origin vX.Y.Z` → `release.yaml` builds binaries +
multi-arch image, pushes `ghcr.io/alexei-led/archfit:<tag>` + `:latest`, and its
`release` job runs `gh release create` itself. A second `gh release create` (or a
release tool) collides on the tag and fails the job.

## Runtime image

`Dockerfile` is `debian:bookworm-slim` (glibc; musl broke ast-grep) — one image with
Go SDK, git, Node 24, dependency-cruiser, ast-grep (`sg`), uv, python3; non-root.
The Rust toolchain (`cargo`, `rust-analyzer`) is **not** bundled — Rust analysis
reports `n/a` (never fails) in the image; run on a host with cargo or extend it.
`archfit doctor` checks tools; `sg` must resolve to ast-grep, not util-linux. Build
amd64 in CI, not local emulation.

## Rust analysis granularity

Rust crate facts are **crate-level**: `cargo metadata` makes one graph node per
workspace member. The scorer caps a **degenerate graph** (<2 connected modules,
e.g. a single crate) at `mixed` — it never scores `strong` on a one-node graph (see
`internal/score`, `degenerateGraph`). Opt-in `analyzers.cargo_modules.enabled`
adds an **intra-crate module graph** (`<crate>::<mod>` nodes + aggregated `uses`
edges), so single-crate projects get real cycle/blast-radius/cohesion signal.
Opt-in `analyzers.scip.enabled` runs rust-analyzer SCIP, which produces a correct
`<crate>::<mod>` strength map and attaches `StrengthHint` to those module edges. The
engine then registers auto-discovered module nodes as modules
(`classify.AugmentModulesFromGraph`, gated on the `::` separator so Go/TS/Python are
untouched) so distance/volatility classify and the strength is consumed — verified on
herdr: `coupling_balance` measures (was n/a). `encapsulation`
stays `n/a` for typical Rust by design: it scores only contract/intrusive edges, and
Rust's module privacy makes cross-module _intrusive_ edges rare. With all three on
(`languages.rust` + `analyzers.cargo_modules` + `analyzers.scip`) a single-crate
Rust project gets full module-level coupling analysis.

**Rust deep-analysis config is auto-generated.** For a project with a root
`Cargo.toml`, `config init` emits the `analyzers.cargo_modules`/`analyzers.scip`
stanza enabled, and `config update --apply` rewrites an existing config's Rust
stanza to `languages.rust.enabled: auto` + both analyzers on
(`cmd/archfit/rust_config_update.go`, `needsRustDeepAnalysisConfig` /
`ensureRustDeepAnalysisConfig`). Explicit `languages.rust.enabled: false` opts
out and is preserved. This is a deliberate default (single-crate Rust degenerates
to one node without it) — the sections above describing cargo_modules/scip as
manual opt-in still hold for non-generated configs. The line-based editor is used
here (not `initcfg.ApplyEdits`, whose sealed `Edit` types only cover module
stanzas, not top-level `languages:`/`analyzers:` sections).

The extractor carries crate roots (`graph.CrateRoot`, repo-relative src dir + crate
name from cargo metadata) on the graph. Rust facts remain crate-level for per-file
metrics (size, churn); the per-file module-key resolver (`RustFileToModuleKey`,
`modgraph.ModuleKeyResolver`) was removed as dead code — it was built but never
wired to any metric.

## Layout

`cmd/archfit` (kong CLI) · `internal/` decision core + adapters · `docs/design`
(current decisions — 5 files) · `docs/guide` (user docs) · `docs/spec` (spec) ·
`docs/plans` (open plans only; shipped plans move to `docs/plans/completed/`).

**Skip `docs/archived/`** — superseded design docs, completed plans, plan notes,
research artifacts, and analysis notes. Only read when explicitly debugging
history or looking up a completed plan by name.
