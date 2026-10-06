# archfit

Architecture-fitness CLI (Go). Reads dependency facts from a target repo, checks
them against `.archfit.yaml`, emits gate violations + metrics for CI and AI agents.
Language facts come from external tools run out-of-process: `go list`,
dependency-cruiser, ast-grep, grimp, `cargo metadata`, jscpd, SCIP.

## Commands (Makefile)

- `make build` — static binary, `CGO_ENABLED=0` → `.bin/archfit`
- `make test` — `go test -race -coverprofile=coverage.out ./...` + `python3 internal/extract/scip/scip_reader_test.py` + `bash scripts/tests/cli_exit_contract_test.sh` (CI runs both non-Go steps too)
- `make lint` — `golangci-lint run -c .golangci.yaml ./...` (pinned to minor v2.14 in the Makefile and CI; patch releases float)
- `make fmt` — `gofmt -s` + `goimports -local github.com/alexei-led/archfit`
- `make arch-lint` — v2 architecture-state gate: `.bin/archfit check --config .archfit.yaml`; accepts `healthy` (0) and `needs_attention` (2), and fails on `blocked` (1) or `error` (3)
- `make archfit` — compatibility alias for `make arch-lint`
- `make archfit-report` — write `docs/reports/archfit-report.md` via `archfit analyze --markdown`
- `make mock` — regenerate moq fakes (`go generate ./...`)
- `make test-fast` — `go test -race -short ./...` (skips slow subprocess/ast-grep integration tests; for inner-loop speed)
- `make bench-gate` — cold vs warm fact-cache gate timing on this repo (reported number, not a CI assert; `scripts/bench-gate.sh`)
- `make all` — fmt → lint → test → arch-lint
- One test: `go test ./internal/<pkg>/ -run TestName`

## Structural gates (CI runs these explicitly — keep green)

- Import ring: `go test ./internal/ -run TestArchImports`
- Golden output: `go test ./internal/application/ -run TestGolden` — regenerate
  deliberately and inspect the diff; output changes are never automatic.
- Erosion gates: `go test ./internal/ ./cmd/archfit/ -run TestErosion_` — the six
  named architecture-state checks (see the erosion invariant below).
- Dogfood gate: `make archfit` — CI runs the same target after tests/goldens. Also
  runs locally pre-push via the `arch-lint` hook in `.pre-commit-config.yaml`. The
  self-config (`.archfit.yaml`) gates its own architecture: forbidden-dependency
  ring + `forbidden_layer_direction` (fail).
  `public_api_type_leak` runs advisory (warn).

## Invariants

Enforced by `internal/arch_test.go`; extend that test when adding a boundary.

- Core ring (`internal/relationship/**`, `internal/assessment/**`,
  `internal/syntax`, and `internal/scope`) must not import `os`, `os/exec`, any
  YAML lib, or adapter packages — it decides over already-gathered facts. `scope`
  is the only filesystem exception: it uses `os.Stat` and symlink resolution to
  canonicalize the caller's analysis boundary. `assessment/score` synthesises the
  coupling_balance band from an already-computed `Diagnostic`; `syntax`
  classifies each source file (Production/Test/Generated/Vendor) for the LOC walk
  and Production-file metrics.
- `internal/model/*` imports stdlib only, checked by `checkModelStdlibOnly`.
  `internal/policy` (the authoritative Policy contract) additionally imports the
  model kernel and one vetted pure third-party matcher (doublestar), allowed via
  `contractThirdPartyAllowed` and checked by `checkPolicyContractPurity`.
  The model kernel is a pinned published contract:
  its exported surface is pinned by `TestModelSurfaceNoDrift` (golden:
  `internal/testdata/model_surface.golden`); regenerate deliberately with
  `ARCHFIT_UPDATE_SURFACE=1` and call the contract change out in review.
  `internal/view` is gone — its stage DTOs were returned to the capabilities that
  own them, and the `no_stage_view` rule in `.archfit.yaml` blocks a new shared
  stage-view package.
- **Stages receive projected views, never `internal/config`.** `internal/config`
  owns the YAML lifecycle and projects itself into each consumer's own contract
  (`Config.PolicySnapshot()`, `Config.RunOptions()`, `Config.CoverageOptions()`,
  `Config.AnalyzerFamilies()`, `Config.ForFileClass()`). Only composition roots
  (`cmd/*`), `internal/config` itself, and `internal/configschema` may import
  `internal/config` (enforced by `visible_to` on `policy-config-adapter` and the
  `module_dependencies` rule in `.archfit.yaml`).
- Every subprocess call goes through `toolrun.Runner` (interface in
  `internal/toolrun/toolrun.go`); extractors in `internal/extract/{go,ts,py,rust}`
  are out-of-process adapters. No `exec.Command` in core code — fake the `Runner`
  in tests. A child with a `WorkDir` gets `PWD=<abs WorkDir>` (os/exec refreshes
  PWD only when `Env` is nil): Go tools trust an inherited PWD that aliases their
  cwd, so `go list -f {{.Dir}}` echoed the shell's symlink/case-variant spelling,
  the Go main deploy unit fell outside the canonical scan root, and `model_hash`
  depended on the invocation path.
- **Fact cache** (`internal/factcache`, adapter — core ring must not import it;
  see `docs/design/fact-cache.md`). Content-addressed extractor-fact store under
  `.archfit-cache/facts/`; stores facts, never scores. Runner-shaped analyzers
  (depcruise, cargo, SCIP, ast-grep) wrap `toolrun.Runner` in
  `factcache.Runner`; Go (`packages.Load`) and jscpd (temp-file output) use the
  store directly. Keys hash tool version + config slice + input tree; the
  input-tree hash must cover the TOOL'S real input set — config `exclude:` globs
  never filter it (the tools don't honor them; over-hash, never under-hash).
  Partial/timed-out/dirty results are never cached; a Go member whose build
  reaches source no key covers (local `replace` in a member go.mod, unkeyed
  go.work sibling) is vetoed per-member; a local `replace` in the go.work file
  itself disables the whole run's Go cache. The shared input walker skips only
  `.archfit-cache` and `.git`; names such as `target`, `venv`, and
  `node_modules` do not prove that an analyzer ignores source there. TypeScript
  keys hash the complete supported `extends` chain and bypass the cache when a
  chain cannot be resolved. Python runs fresh because the uv launcher identity
  does not establish its transient grimp/Python environment. Cache-key work is
  bounded (20,000 entries, 64 levels, 32 MiB); cycles, unsupported scope or budget
  exhaustion never produce a partial reusable key. There is no `--no-cache` flag —
  `--refresh` re-runs the extractors and writes the fresh results back
  (`cmd/archfit/refresh_test.go` pins `--no-cache` as an exit-3 usage error).
  The Go standard-library list (`go list std`, rule-scope `GoStdlibPackages`) is cached under `facts/go-std/`, keyed ONLY on the toolchain identity `go env -json GOVERSION GOROOT GOOS GOARCH GOFLAGS GOEXPERIMENT CGO_ENABLED` probed in ScanRoot (not `GoExtractor.goVersion`, which runs without WorkDir and can name another toolchain); no input tree; only exit-0 non-empty output is cached. `config lint` (`Inventory`) passes a nil store and runs it uncached.
- **No gitnexus.** The `.gitnexus`/`.codegraph` index dirs are excluded from file
  walks (`scope.go`), but archfit does not run the tool and does not derive any
  per-module fact from it.
- **Dimension promotion is required-fact completeness, never metric quality.**
  `state.RequiredFacts` is a fixed set per dimension and `state.Promote` applies
  the shared rule in `docs/design/evidence-contract.md`: measured only when every
  in-claim fact is observed or producer-proved not applicable; partial when some
  in-claim fact is observed and another is missing; unmeasured when none is
  observed or the dimension does not apply. A measured dimension may carry only
  out-of-claim unknowns. Never infer completion from a high metric value or treat
  an absent producer as an observed zero.
- **Coverage is supplied, never executed.** Top-level `coverage:` is opt-in and
  default-disabled. `analyze`/`check` parse caller-produced Go coverprofile,
  LCOV, coverage.py JSON, or llvm-cov JSON; they never run the target's test
  command. Testability promotes only with compatible units, complete declared-
  module attribution, zero unresolved paths, and a version-1 sidecar whose
  producer-enumerated source hashes match current ScanRoot bytes. `source_ref`
  is metadata, not a freshness gate. Freshness is recomputed even on fact-cache
  hits. Duplicate equal-denominator facts use the greatest covered count as a
  lower bound; unit/denominator conflicts suppress the ratio and force partial.
  The sidecar is unsigned producer attestation: Archfit cross-binds every
  normalized covered fact path to a listed path/hash and verifies the current
  bytes. Empty/missing `sources` is unverified; omitting a covered fact path is
  stale. Archfit still cannot authenticate the producer or prove the artifact
  itself did not omit source facts; upgrade via authenticated trusted producers
  as documented in `docs/design/evidence-contract.md`.
- **`operations` claims declared topology, not runtime topology.** Every declared
  module needs an independently corroborated deploy unit plus a declared or
  CODEOWNERS-backed owner; git-author fallback is visible but does not qualify.
  Declared and corroborated deploy-unit metrics stay separate. Analyzer health,
  runtime topology, and SBOM/vulnerability state are out of claim. Within the
  out-of-claim analyzer-health metrics, only applicable rows enter
  `analyzers_reporting_coverage`; absent/disabled rows are disclosed as
  `analyzers_not_applicable` rather than overstating a single-language gap.
- **Severity source is `cl.Score.Band`** (`classify.go`, `Run`). `cl.Severity =
cl.Score.Band` after the scorer runs. `BalanceResult` is deleted — it was the
  old discrete severity table and is no longer called anywhere. Do not re-introduce
  it; the book formula (`ScoreVersion = "bc_score.v6"`) is the single severity source.
- **Coupling gate is the distributed-monolith SEAM rule**
  (`coupling.gate.distributed_monolith: {mode, max_new_seams}`, config schema
  **v2**). It counts logical seams — one ordered module pair, however many
  imports express it — not edges. `score.EvaluateSeamGate` + `applySeamGate` run
  inside `evaluation.Score` (`internal/assessment/evaluation/finalize.go`),
  before `agenttask.Build`. A seam qualifies when it has at least one active
  source-graph edge in the critical band at high distance
  (`coupling.DistanceIsHigh`); it is built from the FULL classified edge set, so
  no severity/baseline/waiver filter can hide one.
  `mode: fail` blocks ONLY on seams newly introduced against a **comparable**
  reference (all four of `config_hash`, `model_hash`, `labels_hash`,
  `rubric_version` equal); without one the gate reports the seam total, states
  that no new-seam count is claimed, and never blocks. A blocked run emits one
  `bc/coupling_gate` gate finding PER new seam, keyed `coupling-gate/<seamID>`
  and carrying the module pair. Advisory PROMOTION is gone: the scalar gate had
  to borrow findings to point at; the seam gate names its own seams. Reasons
  print to stderr from `analyze` ONLY (`AnalysisRequest.SuppressGateReasons`),
  and are EMPTY when no seam qualifies — an abstention printed on every clean
  run trains readers to ignore the line that matters.
  `internal/arch_test.go:TestSeamGateIsScoreBlind` forbids `Scorecard`/`Overall`
  in `score/gate.go`: the repository scalar must not reach the gate that
  replaced it. Self-config evidence (2026-08-26): 380 scored edges, all
  `cross_module_same_owner`, 78 critical, **0** at high distance → 0 qualifying
  seams, which is why the self-config stays `mode: warn`.
- **Config schema v2 is the only analysable schema.** `config.SchemaVersion = 2`;
  `analyze`/`check` reject every other version and unknown config keys. `config
init` emits v2 directly; owners update older configs manually before analysis.
- **Seam ledger** (`relationship.Seam`, built by `analysis.buildSeams`, carried
  on `AssessmentSignals.Seams` → `result.Result.Seams`). One record per ordered
  module pair with a stable ID (`sha256("seam.v1\x00"+from+"\x00"+to)`), the
  scored/abstained denominator, a nearest-rank score distribution
  (`ceil(p*n)-1`; p10/p90 **null** below ten samples), raw owner/deploy/structural
  distance facts beside the collapsed rung, the book Ch10 quadrant, the labels in
  effect, and a balancing hypothesis. Same-module edges (a different fractal
  level) and unresolved targets (external hygiene) are NOT seams; clone-only
  pairs are not either — they have no import edge. Seam order is by module pair,
  and the gate re-sorts by ID so a ledger reordering cannot reorder gate findings.
- **Comparison is strict on four fingerprints plus measurement profile** (`decision.CompareFingerprints`).
  `config_hash` + `model_hash` (`policy.ModelHash` over the RESOLVED module map)
  `labels_hash` (`labels.FileHash` over APPROVED entries only) +
  `rubric_version`. Any mismatch is `non_comparable` with a reason NAMING the
  drifted input — never a delta with a caveat. Model hash is load-bearing: seam
  identity comes from module NAMES, so without it a rename reads as one resolved
  seam plus one new seam and a new-seam gate blocks on a no-op refactor.
  It is taken AFTER `PolicySnapshot.WithResolvedTopology`, so a CODEOWNERS-filled
  owner or a detected deploy unit moves it — settled decision, pinned by
  `policy.TestModelHashCoversResolvedTopology`. Distance (and therefore seam
  qualification) is computed from owner and deploy unit, so hashing only the
  DECLARED map would let an owner flip re-qualify an existing seam and block a
  no-op commit; covering the resolved map makes the same event an abstention.
  Accepted cost: an ownership or deploy-unit split makes the stored baseline
  non-comparable, so `mode: fail` blocks only on code-edge changes until
  `archfit baseline` is re-run. Rationale in
  `docs/design/architecture-state-reporting.md`.
  The measurement profile adds the normalized settings hash and producer
  semantics/status/tool versions. Unknown or incompatible profile data makes
  the comparison `non_comparable` with named reasons; external producer
  versions are exact-match until equivalence is verified. Unresolved dynamic
  dependency-cruiser inputs and unsupported TypeScript config resolution are
  unknown profile inputs. The root `comparison`
  block describes `--base` and carries the current profile. The persisted
  baseline is exposed separately as `gate_reference`.
- **Labels are validated structurally** (`labels.Validate`). Self-pair, duplicate
  ordered pair, and (where the module map is in hand) undeclared endpoint are
  hard errors — an override that applies to nothing, or two answers to one
  question decided by file order, cannot produce a valid report. Shape checks run
  in `labelsio.Load`; module existence runs in `acquisition.loadLabels`, the first
  point where labels and the module map are both resolved. A STALE evidence hash
  is not an error: it disables the override and emits the `labels/stale`
  diagnostic, which can never be promoted to a gate.
- **FileClass facility** (`internal/model/fileclass`, `internal/syntax/fileclass`).
  Every source file is classified as `Production | Test | Generated | Vendor` once
  during the LOC walk; the result is stored in `SizeSignals.FileClassIndex`. Use
  `syntax.LookupFileClass` for path→class lookup with fallback. Metrics that
  filter on Production files use this index and report the excluded count.
  Config override: top-level `file_class:` key (`FileClassDef`), projected via
  `Config.ForFileClass()` → `syntax.FileClassConfig`.
  Rust test files are classified file-level only: `tests.rs`, `*_test.rs`,
  `*_tests.rs`, or a path through `tests/`, `benches/`, `*_tests/` or `*-tests/`
  (the suffix needs a separator, so `contests/` stays Production). An inline
  `#[cfg(test)]` block inside a production file is NOT separated and still counts as
  Production, so `forbidden_pattern` can fire in it. Configured `generated_globs`
  still win over the convention.
- **`archfit analyze --base <ref>`** flag. The application coordinates it
  (`StageExecutor.attachBaseComparison`, `internal/application/base_compare.go`);
  the worktree mechanics are a VCS adapter (`git.Worktree.Checkout`,
  `internal/history/git/worktree.go`). Creates a
  clean detached temp worktree at `<ref>`, scores both sides with the full advisory
  pipeline, and emits a dimension-by-dimension delta table. Off-gate, report-only
  (exit 0 on success, exit 3 on git/config error). Both sides use the current
  `--config`. Formats: `text` (default), `json`, `markdown`.
  The base sub-run receives the caller's EFFECTIVE head config (after `--lang` /
  `--min-severity`) and never reparses the file. `analyze.go` hands it a copy
  with an independent `Modules` map (`WithIndependentModules`) — `config.Config`
  is a value but its map is shared, and the run's owner/deploy-unit
  backfill writes through it, so without the copy the base side would inherit
  head-tree owners and skip its own resolution. The base sub-run is a SECOND
  acquisition service (`StageExecutor.NewBaseEvidence`), not a second call on
  the head one: no per-run state can leak between the two trees.
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
  failed extractor in `acquisition.Collect`, a rejected ast-grep rule file, an empty
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
  (`ports.Extractor`, `acquisition.Collect`). A failed `Extract` returns a zero
  Coverage, so acquisition stamps the row itself; filing it under the language
  name ("go") instead of the coverage name ("go/packages") created a phantom
  analyzer next to the real family and left the family with zero rows.
- **Applicability is decided by the extractor, never by a marker list**
  (`LanguageDescriptor.ProjectPresent`, `internal/extract/registry/registry.go` — the row's doc
  comment is the contract; probes are projected by `Config.CoverageOptions()` and
  read in `internal/evidence/acquisition/coverage.go`).
  Every language answers "is this language present under root?" by calling its OWN
  exported applicability function — `golang.AnalysableMembers`, `ts.Applicable`,
  `py.Applicable`, `rust.Applicable` — and that same function is what the
  extractor's `Extract` calls to decide whether to run. There is no
  `ProjectMarkers []string` and no marker-list fallback: a new language MUST
  supply a `ProjectPresent` that delegates to its extractor, never a hand-rolled
  list of filenames. `config init`/`config update` take presence from the same
  probes (`languagePresence` in `cmd/archfit/init.go` → `initcfg.Presence`; Go
  members from `registry.GoMembers`), write a present language `enabled: true`
  (Rust: `auto`, so a missing cargo is a coverage gap, not a run failure —
  `initcfg.languageMode`) and an absent one `auto`, never `false`. **A probe that disagrees with its extractor turns "we did not
  measure" into "there is nothing here"**, and gapless `absent` on a primary row
  is the ONE shape both pairing paths read as "language not present" — so
  `analyze --base` and `config compare` drop the analyzer and report confidence
  neither side earned (a fabricated `introduced`; a fully-comparable grade over a
  language nothing looked at). Both directions have shipped as bugs: a marker the
  extractor ignores (`tsconfig.json` with no `package.json`, `setup.cfg`, a
  `go.mod` the `languages.go.modules` filter removes) fabricates presence; a
  marker it accepts but a list omits (`languages.python.package`, a sub-crate
  `languages.rust.manifest`, a `go.work` member a walk cannot reach) fabricates
  absence. The exclusion set is merged EXACTLY ONCE, in
  `Config.RunOptions()` (`internal/config/projection.go`), before
  `Config.CoverageOptions()` projects the probes `buildCoverageGaps` reads;
  `scope.MergeExclusions` is NOT idempotent (it consumes `!` re-includes), so a
  second merge re-seeds defaults the user removed and the probe then skips trees
  the extractors analysed.
- **A rule's producer scope is narrowed by SELECTOR VOCABULARY, never by
  analyzer availability** (`evaluation.restrictToTargetVocabulary`,
  `policy.ModuleMap.SelectorLanguages`). Rule selectors match graph node IDs,
  and those are spelled per language — slash paths for Go/TS, dotted IDs for
  Python, `crate::mod` for Rust (`NodeConvention.ModuleSegmentSep` is the single
  source; a new language declares its vocabulary there, not here). A selector
  the language cannot spell can never match one of its nodes, so that language
  leaves the rule's scope. The source scope is an extension scan over the
  `from:` glob, and a glob picks up whatever sits there: archfit ships Python
  helper scripts it runs through uv, so `from: internal/**` put python in
  scope for rules whose `to:` is a Go package path, and grimp's legitimate
  absence then marked them unevaluated — 8 of 60 rules, holding `intent` at
  `partial` permanently. `restrictToTargetVocabulary` itself uses only
  availability-INDEPENDENT facts: an absent producer for a language whose extractor
  says the language IS present still leaves the rule unevaluated, which is what keeps
  a missing analyzer honest. Extractor APPLICABILITY is a different axis and is
  applied once, at the inventory (the rule-scope bullets below). Module-wide rules (`public_api_max`, `forbidden_layer_direction`,
  `module_cycle`) keep the full module scope on purpose — a helper script inside
  a declared module is a real member of its API accounting and of its edges.
- **Internal-access rules decide from declared surfaces, rule-side**
  (`rules.internalTarget` over `policy.ModuleMap.MatchesInternal`).
  `public_api_only`/`internal_api_access`: a `public:` glob of the target's own
  module never fires; any module's `internal:` glob fires; only an undeclared
  target falls back to the extractor's `uses_internal` kind (Go `/internal/`
  segment, TS/Python internal-glob match). Never move this into an extractor:
  finding fingerprints hash the edge kind, so changing Go edge kinds re-keys
  every accepted finding on those edges, under every rule.
- **`cycle` is node-level; `module_cycle` is the declared-module check.** Go
  edges run file -> package, so `cycle` is structurally 0 on Go.
  `module_cycle` runs Tarjan (`relationship.Set.ModuleCycles`, sharing
  `stronglyConnected` with `Set.Cycles`) over DECLARED modules only (`mm.Has`;
  synthetic crate::mod and go.work members stay out) and emits one
  `module_dependency` finding per ordered pair inside an SCC, keyed by
  `finding.NewKeyed(rule, kind, from, to)` with empty endpoint paths.
  `resolveEvidence` fills a module only when it is empty and has a path. Its
  `why` names the pair and the cycle size only (a capped cycle also says how many pairs it reports); the node-level `cycle` why caps
  its member list (`boundedMemberList`, 300 bytes) and the module-cycle goal
  collapses a long member list to its size (`agenttask.cycleMembers`). The App
  rejects a `why` over 500 characters, so no finding text may grow with the graph.
- **Module allowlists** (`modules.<m>.depends_on` / `visible_to`, enforced by
  ONE `module_dependencies` rule; predicate `policy.ModuleMap.DeniedDependency`,
  which `policy can-import` must reuse). Absent key = unconstrained, `[]` =
  nothing allowed (goccy keeps nil vs empty; `cloneTopology` preserves it).
  Endpoints resolve against the DECLARED map (`ModuleForNode`), never the
  augmented `e.FromModule`: a node only a synthetic module owns is unowned. A
  target no declared module owns is out of scope; an unowned importer is denied
  by every `visible_to` and keyed by its package (`importingPackage`; a Go file
  goes through `ModuleForFile`, as in classify, so a package-dir `paths:` glob
  owns it). `resolveEvidence` never back-fills a `module_dependency` finding's
  empty module.
  One finding per denied ordered module pair (`finding.NewKeyed(rule, kind,
  from, to)`, `matched_by.violates`), production edges only, as for
  `module_cycle`. Scope = languages of the modules that declare a list
  (`allowlistRuleScope`); no list anywhere is `selector matches nothing: …`.
  `ModelHash` ignores both keys (`TestModelHashIgnoresAllowlists`). The self-config
  parity proof for the 21 retired deny rules is
  `internal/selfmodel_allowlist_test.go`.
- **Module selectors and rule rationale** (`policy.ModuleMap.SelectsModule`,
  the one parser: `layer:<name>`, `role:<role>`, else a doublestar glob over
  DECLARED module names — synthetic modules are never selected). Allowlist
  entries and `forbidden_dependency`'s `from_module`/`to_module` read it. Each
  side takes exactly one of glob / module selector (`validateForbiddenDependencyDef`);
  `from_module`/`to_module` on any other type is a config error. Path-only
  `forbidden_dependency` keeps its original edge loop and `finding.New` IDs
  (byte-identical); the module branch (`checkModules`) skips same-module edges
  and keys `NewKeyed(rule, "module_dependency", "module:"|"path:"+side…)`. A
  module side is judged like `moduleRuleScope` judges a module
  (`moduleSideState`): live when a selected module owns analysed source or a
  module-graph node; empty (`selector matches nothing: from_module|to_module
  …`; lint `unknown_module` when nothing is selected, else `dead_selector`)
  when nothing is selected or every selected module provably owns nothing (no
  owned file, and only explicit source-file paths or `crate::mod` paths the
  graph lacks); unanalysed-only gets the unanalysed reason; anything the
  inventory cannot judge (a directory glob with no file, a Rust package name
  without crate roots) abstains with the generic reason. A `to_module` scope
  that cannot be established is never replaced by the from side. Module-selector tasks carry no `declarations`. `rationale`/
  `alternatives`/`docs` are applied once, in `rules.gatedRule` (`explain`;
  docs end both the why and the constraint),
  never enter IDs; the rationale rides `finding.Finding.Rationale`
  (`json:"-"`) into the task constraints as `rationale: …`; SARIF carries it
  in the message (the why) and `allowed_alternatives` as a result property
  only when declared.
- **`module_cycle` is production-only** (`rules.productionSource`). An edge counts when its importing file (a file node, or an import site with a source extension) is production:
  - a walked file: its in-scope FileClass decides;
  - a file declared out of scope: never;
  - a file the LOC walk skipped (a dot directory, `target/`, `mocks/`): its path-only class with the configured `file_class` globs (`Observations.UnwalkedSourceProduction`, computed in acquisition);
  - a file with no class at all: it counts.
  An edge with no source-file attribution (a Rust crate dependency at `Cargo.toml`, a `crate::mod` edge) counts. The strongly connected component is computed over production edges, so finding IDs (rule, module pair) are unchanged.
- **`module_cycle` is bounded per strongly-connected component** (`maxModuleCyclePairs = 200`, `rules_dependency.go`): the first pairs in (from, to) order are kept, so kept IDs never move; every finding carries `matched_by.cycle_pairs_total`, and a capped cycle's `why` says how many pairs it reports. Pairs past the cap surface as new once reported ones are fixed. Module-cycle and seam-gate agent tasks carry no `declarations` (`agenttask.Build`): on ccgram they were 882 KB of a 1.29 MB report.
- **`forbidden_pattern` is the only consumer of `rules[].patterns`.** It fires
  on production files in the LOC inventory (`FileClassIndex` minus
  `OutOfScopeFiles`, `evaluation.inScopeFileClasses`; the LOC walk and the `sg`
  scan ignore `exclude:` and switched-off languages) under `from:`
  (path or convention selector — the same matcher its producer scope uses), is
  evaluated only when the `ast-grep` pattern row is ok, and never puts matched
  source text in a finding (the text is hashed into the ID only). A pattern run
  `sg` rejects (exit > 1) makes that row partial, never "no match". Patterns on
  other rule types still run and warn at Prepare; `ForPatterns` keeps
  collecting every rule's patterns because they feed the settings hash.
- **The rule-scope source inventory is the DECLARED analysis scope**
  (`acquisition.declaredOutOfScope` → `Observations.OutOfScopeFiles`, skipped by
  `evaluation.sourceInventoryFiles`). A walked source file leaves every rule's
  scope when an effective `exclude:` glob matches it or its language is switched
  off (`primaryDisabledByConfig`: `enabled: false` and no explicit `gate:`). The
  exclusion set is `RunOptions.Exclusions` — merged once, never re-merged. This
  is declared scope, not analyzer availability, and not "what the extractors
  read": dependency-cruiser ignores `exclude:`. Metrics and file classes still
  count the files. Vocabulary narrowing alone did not keep exit 0 reachable:
  #40 added `internal/extract/ts/config_snapshot.cjs`, and the `.cjs` plus the
  `.py` helpers held 7 fail-gated rules (`layer_inversion` included, through
  module-wide scope) waiting for dependency-cruiser and grimp, although
  `.archfit.yaml` switches TypeScript and Python off. With the declared-scope
  inventory the self-check reaches `hard_gates: pass`.
- **A language switched off over a language that IS PRESENT reports `disabled`,
  never `absent`** (`markDisabledPrimaries`, `internal/evidence/acquisition/coverage.go`,
  applied to `diag.ToolCoverage` before `buildCoverageGaps`). Extractors encode
  `ModeOff` as `StatusAbsent`, which both pairing paths read as "this language is
  not in the tree" and drop from the comparison — so two configs that BOTH
  disabled Go over a Go repo graded fully comparable while neither had looked.
  Rewriting the row leaves gapless-`absent` with exactly ONE cause (the
  extractor's own applicability probe says the language is not in the tree),
  which is what `decision.gradeTool` and `normalizeCoverage` already assume. Two conditions are load-bearing, both narrowing:
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
  rows — `pairFamily` (task origin) and `decision.gradeTool` (config
  compare) — read a repeated name as an unpairable duplicate and grade the pair
  unavailable/`not_comparable`. Never give two analyzers one coverage name.
- **`archfit config compare <candidate>`** (`cmd/archfit/config_compare.go`,
  pure decision in `internal/assessment/decision.CompareConfigs`). Two full pipelines over
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
  agree — discovery emits one key per directory, while `.archfit.yaml` declares
  capability modules that span several (`assessment-repair` over
  `internal/assessment/**`); see accepted risk 4 in
  `docs/design/architecture-baseline.md`. `DiffModules` runs the name-drift
  pass ITSELF (`resolveNameDrift`, unexported — there is no two-step call a
  consumer can get wrong), reclassifying each 1:1 add/remove pair with an equal
  normalized path set as `NameDrift`. It then runs the ownership pass
  (`resolveOwnership`): a discovered module whose every `Sources` entry (Go
  package dirs; every Python dotted package and module in the subtree) the
  configured map owns under
  most-specific matching (`ModuleMap.ModuleFor`, injected from cmd) is
  `Covered`, not `Added`, and its owning stanzas leave `Removed` and are
  field-checked. TypeScript/Rust modules carry no sources and stay name-matched. On top of that, `Removed` is review-only:
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
  (1) `candidateConfigForUpdate` (`cmd/archfit/config_update_adapters.go`) — the "config after
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
- **Onboarding proposes only what check can see** (`internal/initcfg/inventory.go`, `cmd/archfit/init.go:languagePresence`). `config init` and `config update` read the rule-scope source inventory through the same reader as `config lint` (`acquisition.Inventory`, then `evaluation.SourceInventory`, passed in `initcfg.Presence.Sources`). Discovery keeps only Go/Python/TS modules that own a production file under `moduleRuleScope`'s ownership: most-specific `ModuleFor` on the path, then on the selector. It drops mocks/, generated-only, test-only and default-excluded trees, `public:` entries that name no production node, and edges to dropped modules. Rust crates are kept unjudged, because their selectors need cargo metadata. A nil `Sources` is a discovery unit-test seam only; production always supplies it and fails loudly when the read fails. A starter rule gets `gate: fail` only when `DiscoveredConfig.ImportGraphComplete`. Otherwise it gets `gate: warn` with a `# Why:` line from `GraphGap`. The graph is incomplete when there are Python or TS modules, when Rust is present, or when a Go module owns non-Go source (prometheus `web/ui`). Init infers no layer from any directory or package name. Only a Rust workspace gets layers: topological tiers of the normal/build crate graph, with dev-dependencies excluded as in the extractor, so the starter direction rule starts with zero back-edges.
- **`AnalysisRequest` + `AnalysisContext`** (`internal/application/analysis.go`)
  carry the per-run path and time inputs. `AnalysisRequest` is what the caller
  asks for; `AnalysisContext` is what acquisition resolved, and every later stage
  reads it instead of re-deriving anything. A caller that leaves a request field
  zero silently gets the service default, which is head-tree state on a base or
  candidate run: `ConfigSource` → config hash + validation command; `BundleDir` →
  pinned labels + fact-cache location; `Root` → scope + on-disk path resolution;
  `EvaluatedAt` → the single instant waiver expiry and staleness age are measured
  against (zero samples `time.Now()` once, in `Acquire`). Per-run values: normal
  analysis = current path / current config dir / current tree / persisted
  baseline; git base = same, with the base worktree as Root and
  `EmptyBaseline: true`; compare current and compare candidate = the common tree,
  the current config dir, one shared `EvaluatedAt`, empty baselines, and only
  `ConfigSource` differing.
- **The stage sequence has one owner.** `application.StageExecutor.Execute` runs
  Prepare → Acquire → Relate → Assess → Score → (optional) base comparison, in
  that order, for analyze, check, baseline, explain, enrich, and config compare.
  Only Prepare and Acquire are ports (`config.Preparer`,
  `acquisition.Service`): they validate policy, walk the tree, and run external
  tools. Relationship analysis (`analysis.Analyze`) and assessment
  (`evaluation.Assess`/`Score`) are pure decisions the application calls
  directly — a port for either would only hide the call site. Persistence
  crosses as ports: `BaselineLoader`, `BaselineWriter`, `EnrichmentLabelStore`,
  `WorktreeProvider`.
- **Only relationship analysis sees the graph.** `evidence.Facts` carries it to
  `Relate`; assessment receives `evaluation.Observations`, which has no graph and
  no classifier index, so it cannot re-derive a relationship even by accident
  (`application.observationsOf`). Coverage marking, coverage gaps, config
  warnings, and git-history volatility corroboration are resolved ONCE in
  `Acquire` and ride `AnalysisContext`; assessment attaches them. Rule and metric
  evaluation deliberately reads the RAW coverage rows, not the marked copy, so a
  config opt-out can never move a measured metric.
- **Owner inheritance for auto-registered synthetic submodules**
  (`classify.AugmentModulesFromGraph`, `AugmentGoWorkspaceModules`): propagates
  `owner` from the nearest config-declared ancestor module to each synthetic module.
  Fixes inter-submodule edges defaulting to `cross_module_different_owner` (D=7)
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
  mode proceeds with `GitRoot=""` (history empty) and ScanRoot = the canonical
  ABSOLUTE `--root` or config directory (`canonicalPath` absolutizes before
  resolving symlinks; a relative `.` root dropped every Go fact and deploy
  unit); delta mode without git is a hard error.
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
- **Go files excluded by build constraints are disclosed, never graded.** The
  load sees only the host GOOS/GOARCH and tags, so `_windows.go` and tag-gated
  files are never parsed. `deriveIgnoredFiles` keeps the non-test `.go` files of
  `pkg.IgnoredFiles` (cached with the member facts — fact-cache schema `3`), and
  the merge counts them minus config exclusions into the go/packages `Reason`
  (`golang.BuildConstraintExclusion`) plus ONE stderr warning
  (`goBuildConstraintWarning`). Status, `Unresolved`, the measurement profile and
  dimension promotion do not move: the load is complete for its configuration.
  Ceiling: a directory whose every file is excluded is dropped by `go list ./...`
  and not counted.
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
- **Rule scope follows extractor applicability** (`acquisition.unanalysedFiles` → `Observations.UnanalysedFiles`).
  - A walked file is out of the scope of every rule that reads dependency edges, module-wide rules included, in two cases:
    - Its language's primary row is gapless `absent` (`primaryAbsentFromTree`). That is the SAME predicate `buildCoverageGaps` suppresses the gap on: the extractor's own probe says the language is not present, and no explicit warn/fail `languages.<id>.gate` demands it.
    - It is a Rust file outside every cargo workspace member, once cargo metadata has named the members.
  - `forbidden_pattern` keeps these files; it reads the ast-grep pass.
  - Consequence: TypeScript under `web/ui/` with no root `package.json`, and `.go` files with no `go.mod`, leave rule scope; their coverage rows already call the language absent. Before this, prometheus' starter `module_cycle` waited forever for dependency-cruiser.
  - A dependency selector that matches only such files is listed `selector matches only source no dependency producer analyses: <side> <glob>`, guard or not. It never carries the `selector matches nothing:` prefix, which the App reads as a policy defect.
  - `moduleRuleScope` skips a module whose walked files are all out of scope or unanalysed, instead of returning Unknown.
- **Rust `crate::mod` scope reads the module graph** (`Observations.RustModuleNodes`, the cargo-modules nodes).
  - A `crate::mod` selector or module path under a crate the graph covers is decidable: live or dead.
  - Under a loaded crate the graph does not cover, it is undecidable: an absent module graph is never an empty one.
  - Under a crate that is not loaded, it is dead.
  - Crate roots are compared in library spelling (`my-core` → `my_core`).
- **A guard holds while either selector matches nothing**, whatever the producer status (`vacuityScope`). Once the guarded path exists again, the guard is an ordinary rule.
- **TypeScript never ingests installed code.** depcruise runs with `--exclude (^|/)node_modules/` (root and subtree mode), and `parseAndNormalize` drops node_modules sources and resolved targets at any depth, which makes a cache replay safe. The fact-cache source hash still excludes only the root `node_modules` (over-hash). `exclude` is not part of the TS measurement profile.
- **agent_tasks `files[]` exist on disk (`agenttask.PathResolver`).** Every candidate
  (edge endpoints, locations, module keys) resolves index-first against the LOC walk's
  `FileClassIndex`, with an injected `onDisk` os.Stat backstop. The LOC walk skips
  `mocks/`, `target/` and `venv/`, which extractor exclusions do not. The rule is
  resolve-or-drop: never a bare config key or a dotted/`::` id.
  - Rust crates have two spellings, and `CrateRootDirs` is keyed by both: the package
    name (`yazi-shared`, used by crate-level nodes and selectors) and the crate
    identifier `graph.CrateRoot.Crate` (`yazi_shared`, or a binary target's own name
    such as `yazi`), which starts cargo-modules node IDs. The crate identifier comes
    from the target cargo-modules graphs (`cargoPackage.crateIdentifier`: lib, else
    first bin, with `-` turned into `_`). A package name wins a collision.
  - A bare crate spelling resolves to the crate dir (`src` for a root crate).
  - `crate::mod` probes `<dir>/src/<mod>.rs|/mod.rs`, then the crate dir.
  - With no resolved evidence, the last-resort module root (`config.ModuleRootDirs`)
    stands in for a public_api_* module, a coupling-gate seam, and a module-pair
    (`module_dependency`, i.e. module_cycle) finding, source module first.
  - agenttask never touches the filesystem; the composition root owns `onDisk`.
  - A `bc/coupling_gate` finding carries only a module pair, so `agenttask.Build` takes the seam
    ledger and resolves the paths of up to 20 qualifying edges (`relationship.Seam.QualifyingEdges`
    → `result.Seam.QualifyingPaths`, `json:"-"`, so `seams[]` on the wire is unchanged), falling
    back to the source, then target, module root. Seam-gate and module-cycle tasks carry no
    `declarations`.
- **Rules resolve Rust nodes to the declared crate module
  (`policy.ModuleMap.ModuleForNode`).** Rules see the declared module map, never the
  augmented one. A crate declared by package name cannot glob-match its own
  cargo-modules nodes (`yazi_shared::url::buf`), which made same-crate edges
  module-blind and ignored the owner's `public:`.
  - Acquisition computes `ModuleMap.CrateOwners(graph.CrateRoots())`: package name,
    then crate identifier, then a file in the crate dir; a root crate is never
    claimed by its dir. The result rides `evaluation.Observations.CrateOwners`,
    because assessment may not import `model/graph`.
  - `Assess` attaches it with `WithCrateOwners`.
  - `ModuleForNode(path, language)` is `ModuleFor` first, then a Rust-only fallback
    on the segment before `::`, so a Python or Go node that shares a crate's name
    never takes it.
  - `sameModule`, the `MatchesInternal` owner, `LayerFor`, and
    `new_cross_module_dependency` all use it. `ModuleFor` itself is unchanged
    (CRITICAL fan-in).
  - An explicit `crate::mod` `paths:` glob still wins.
  - `classify.ancestorByKey` (synthetic-module attribute inheritance) still matches
    module KEYS only; that is a known gap.
- **Agent repair contracts are policy-aware and replayable.** Each task carries
  `repair_kind: code_change|needs_owner_decision`; forbidden-dependency goals
  cannot route through a public API, `forbidden_dependency`,
  `forbidden_layer_direction`, `cycle`, `module_cycle`, `module_dependencies`,
  and `new_cross_module_dependency` constraints never list the target module's
  public surface (`agenttask.forbidsTarget`: a public route keeps the
  violation), and new-cross-module goals cannot use baseline capture as a
  repair. One task per active gate finding: an import that
  breaks two rules is two findings and two tasks. Validation replays `--base`,
  `--lang`, and `--require-tools`; it omits `--refresh` so cache control cannot
  change the result.
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
  non-penalising 60 (calibration-only, unreachable from the analysis stages).
- **The self-model is executable** (`internal/selfmodel_test.go`, run with
  `go test ./internal/ -run TestSelfModel`). `.archfit.yaml` must describe the
  physical tree: no dead path glob, no Go package without an owning module, no
  equal-specificity ownership tie (the catch-all shadowing bug), no rule aimed at
  a path that does not exist, no `public:` entry outside its own module or
  naming no package, no undeclared layer, and no declared layer without a module.
  The rule, public-surface, layer, and ownership-tie checks ARE the production
  `archfit config lint` predicates (`evaluation.LintPolicy` over
  `acquisition.Inventory`), so the engine's config is held to exactly what users
  get. Two rules match nothing ON PURPOSE and declare `guard: true`:
  `no_stage_view` and `no_analysispipeline` block the dissolved packages from
  returning. The `guardRules` table names what each guards; the test fails if
  either guard is deleted, loses `guard: true`, or starts matching real source
  (`guard_matches_source`), and if any other rule declares `guard: true` without
  an entry. Moving a package means updating the owning `paths:` in the same commit.
- **A rule selector that matches nothing is never conformance**
  (`evaluation.selectorInventory.vacuousSelector`, one predicate for check and
  `config lint`). It applies to the selectors of `forbidden_dependency`,
  `public_api_only`, and `internal_api_access`, and to the `from:` of
  `forbidden_pattern` (`selectorRuleTypes`), over the declared-scope
  inventory plus node selectors. The dependency rules match edge endpoints in
  each language's own vocabulary (`selectorInventory.matches`): a target is the
  module node (Go package dir, TS file, Python dotted module, Rust crate), a
  source is the importing file for Go/TS and the module node for Python/Rust;
  `forbidden_pattern` matches file path or node selector. Vacuous: a `from:`
  that matches no in-scope source; a `to:` spelled as first-party source (empty
  literal prefix, or a leading segment, cut in the selector's own vocabulary,
  that the inventory's paths or selectors start with; a dotted leading segment
  of a slash selector is a Go import-path domain, never first-party) that
  matches nothing; and any selector spelled with the Go module path, a leading
  `!`/`./`/`../`/`/`, or `!(` extglob. A `to:` naming no first-party root (`os`, `github.com/...`)
  is an external ban, never vacuous; nor is a `to:` whose literal prefix names a
  Go standard-library package or its parent path on segment boundaries
  (`database/sql` beside a top-level `database/`): `Observations.GoStdlibPackages`
  comes from `go list std` through `toolrun` in `acquisition.ruleScopeObservations`,
  only when a Go module exists, `internal`/`vendor` dropped, nil on failure (the
  first-party judgment then applies). Lint and check share the predicate and the
  inventory build, but check alone has Rust crate roots, and `check --lang`
  switches a language back on. The inventory abstains, keeping the generic
  "rule scope cannot be established" reason, for an unsupported file type, for
  a language whose node identities are unknown (Rust without crate roots), and
  for a `crate::mod` selector the cargo-modules graph cannot decide (see the
  `crate::mod` scope bullet). A Go selector spelled with a
  loaded module path is dead even for a nested `go.mod` the run does not load:
  the Go extractor strips every import under a loaded module path to its
  scan-root-relative dir (pinned by
  `TestExtract_NestedUnloadedModuleImportIsScanRootRelative`).
  A vacuous fail-gated rule is listed in `decision.unevaluated_required_rules`
  with the reason `selector matches nothing: <from|to> <glob>` (the App keys a
  policy-defect reason off that prefix); a warn-gated one is a config warning
  (`evaluation.PolicyWarnings`, noted by acquisition); both stay out of intent's
  evaluated count. `guard: true` exempts a rule: a vacuous source side makes it
  not applicable, a vacuous target side is ignored. Never add a "the rule
  fired, so it is live" shortcut: lint cannot see findings, and the two would
  disagree. `archfit config lint` exits 1 on an error diagnostic and 3 on an
  unreadable config; unknown `volatility`/`subdomain` (vocabulary owned by
  `policy.ModuleDef.UnknownVolatility`/`UnknownSubdomain`, pinned to classify)
  and undeclared layers still load in schema v2 and surface as lint errors and
  check config warnings.
- **The primary output is `archfit.architecture-state.v1`** (`internal/model/report/state.go`,
  rendered by `internal/output/jsonout.Renderer`). `--format json` emits the
  state AT THE DOCUMENT ROOT — `verdict`, `decision`, `comparison`, `measurement`,
  the nine `dimensions` keys, `coverage`, `findings`, `agent_tasks`, `seams` — with
  no repository scalar. Text, Markdown, SARIF, and scorecard all report the
  same verdict, dimension statuses, coverage split, and finding IDs
  (`cmd/archfit.TestFormatMatrix_CrossFormatParity`,
  `TestFormatMatrix_SarifCarriesTheState`); SARIF is exempt from human LAYOUT
  parity only — the state rides in `run.properties` and finding identity (ruleId,
  ruleIndex, `archfit/v1` fingerprint) is unchanged by the cutover.
- **Report free text is bounded once, at projection** (`boundReportText` → `reportText`, `internal/application/report_text.go`, called last in `application.ProjectReport`). A strict state consumer (the archfit App) rejects any string with a control character or U+2028/U+2029, caps free text at 500 runes and an agent task's goal/constraints at 4096, and one bad string invalidates the whole report. Tool stderr and error chains reach coverage reasons verbatim (the ts/py/rust/ast-grep extractors, `acquisition.Collect`'s `err.Error()`), and joins carry them into unevaluated-rule reasons and dimension unknowns, so the bound lives at the single projection every command and format passes through — never at an extractor. Text already one line within the bound is byte-identical; otherwise ANSI CSI is dropped, whitespace/control runs collapse to one space, and the leading text is kept, cut at 400 runes (task text 3600) with `…`. Identity material — IDs, hashes, paths, tool versions, the measurement profile, validation commands — is never rewritten. The raw text goes to stderr only (`discloseRawCoverageReasons` in `StageExecutor.Execute`, rows the sanitizer changes; written directly, not via acquisition's `note()`, which would feed it back into ConfigWarnings). Contract: `cmd/archfit/report_text_contract_test.go` + `reporttest.AppTextViolations`.
- **`--format agent` is a digest, decided in the renderer**
  (`internal/output/agentout`, `archfit.agent-result.v1`, schema
  `archfit.agent-result.schema.json`). `next_action` is decided ONCE, in
  `agentout.decide`, from the report contract only (a report adapter may not
  import assessment): active gate tasks + origin, unevaluated required rules
  (`selector matches nothing:` → `ask_owner`, any other reason →
  `restore_evidence`), coverage gaps with gate `fail`, and the ratchet twin of
  `console.ratchetRegressions`. A blocked verdict never maps to `none`. Scope is
  origin only: a repair is out of scope only when every grouped task is
  `pre_existing`. The 8 KB budget runs after the decision and never moves it;
  it shortens free text and caps repair lists (counting what it cut), never
  an ID, path, edge, or `validate`. `edit` is the source side read from finding
  locations and the source node, never a path compared with the target node
  (Python/Rust node IDs are not file paths); an edge with neither is empty,
  and only a module-pair finding lists every task file. The format is
  exempt from layout parity, like SARIF; `TestFormatMatrix_AgentDigestCarriesTheState`
  holds it to the verdict and every active gate finding. The state gets no
  agent-only field.
- **`archfit policy can-import` is `check`'s evaluator on one edge**
  (`application.PolicyQueryService`, `evaluation.JudgeEdge`). The edge is
  spelled by the extractor's own `QueryEdge` (`registry.QueryEdge`: go/ts/py;
  Rust has none and answers `not_decided`) with the extractor's own edge-kind
  predicate (`ImportEdgeKind`), so node IDs and finding IDs equal the extracted
  ones. It goes through `analysis.Analyze`, the shared rule pass
  (`evaluation.checkRules`), `status.Assign` with the persisted baseline and
  waivers, and `agenttask.Build` for the repair text. The importing file's
  production class comes from `acquisition.Query` with Acquire's own
  predicates (`outOfDeclaredScope`, `loc.ClassifyFile`: the walk's class for a
  walked file, the path-only class otherwise). The root is resolved as check
  resolves it (`scope.Resolve`: --root, git toplevel, config dir; one
  `git rev-parse`). No analyzer, no fact cache. `QueryEdge` drops what the
  extractor drops (`ErrNotExtracted` → `unconstrained`: the descriptor's own
  `ProjectPresent` finds no project; Go `_test.go`,
  build-constrained under the toolchain's env (`toolchainContext`, shared with
  `countApplicableSources`, plus the go env file; unset `CGO_ENABLED` with a
  cgo-tagged file is `not_decided`; a file that imports "C" is not extracted
  with `CGO_ENABLED=0` and `not_decided` otherwise: preprocessed cgo syntax
  comes from the build cache, which `deriveFileFacts` skips, but a failed
  preprocess falls back to the original file),
  excluded file or target, unloaded member; a switched-off
  language) and abstains where only the tool knows (`ErrNotDecidable` →
  `not_decided`: Rust, a Python target outside the packages grimp builds;
  an importer outside them is `ErrNotExtracted`, from `py.grimpPackages`, the
  extractor's own package list). Edge modules for the
  answer: `rules.DeclaredEndpoints` for allowlists, the edge's own declared
  modules and `rules.ProductionEdge` for `module_cycle`, the augmented ones (go.work members via
  `Facts.GoModules`) for the seam gate. Metric ratchets are not evaluated.
  Answer order: `denied` > `not_decided` (a fail-gated
  `module_cycle` on a cross-module edge, `cycle` off Go, the seam gate in
  `mode: fail`) > `allowed` (an allowlist that names the pair, or the layer
  order) > `unconstrained`; an edge whose only violations are accepted debt is
  `unconstrained`, never `allowed`. `cmd/archfit.TestPolicyCanImportAgreesWithCheck`
  pins the agreement in both directions.
- **Hooks map `next_action` onto a host protocol** (`cmd/archfit/hook.go`). `hook
  claude` (Stop/SubagentStop JSON on stdin, config resolved against the event
  `cwd`, a tree clean but for `.archfit-cache` skips) exits 2 with
  `agentout.Brief` on stderr only when `blocksChange`: `repair`/`ask_owner`
  AND an in-scope repair (a dead selector or a ratchet is not scoped to the
  change, so it is a `systemMessage`); `stop_hook_active` → `systemMessage`;
  any archfit error fails open (exit 0 + `systemMessage`); malformed stdin is
  1. `hook git` exits 1/0/3 on the same `blocksChange` and judges the files on
  disk. Both run `executeScan` in process with `--base HEAD`
  and the pipeline's stderr discarded. These exit codes are the host protocol,
  never the engine verdict.
- **`AGENTS.md` carries a generated block** (`archfit agents-md`, markers
  `<!-- archfit:start/end -->`). `TestAgentsMDRepositoryBlockIsCurrent` fails
  when `.archfit.yaml` changes without `archfit agents-md --write`; example
  configs are pinned by `cmd/archfit/testdata/agents-md`. The skill ships in
  the binary (`skills/skills.go`, `//go:embed archfit`) and installs with
  `archfit skill install`.
- **`check` exit code IS the state verdict** (`application.outcomeFor`):
  `healthy` → 0, `needs_attention` → 2, `blocked` → 1, error → 3. Nothing else
  participates — a required-analyzer gate and a failing hard rule both reach the
  verdict through the state's own hard-gate result, so a second condition at the
  application layer could only disagree with the report the same run printed.
  Exit 0 is reachable when all nine dimensions are measured, hard gates pass,
  and no active diagnostic remains. Missing supplied coverage, operational
  corroboration, or a comparable persisted baseline honestly produces exit 2;
  none is a permanent dimension status. `make archfit` accepts 0 or 2; only 1
  fails it. A coupling advisory is a diagnostic and can never reach exit 1.
  An applicable fail-gated rule with incomplete producer evidence, or with a
  selector that matches nothing, is listed in
  `decision.unevaluated_required_rules`; without another blocker it sets
  `hard_gates: unmeasured` and remains exit 2. Required-rule evidence is read
  from these fields, never inferred from finding prose.
- **Six named erosion gates** hold the architecture-state contract against decay
  back into the averaged score it replaced. Each has ONE executable owner and a
  PAIRED fixture proving it fires on a violating input — a structural rule nobody
  has watched fail is a rule nobody knows still works.
  `no_scalar_decision` + `no_dead_archfit_rule` live in `internal/erosion_test.go`
  (which carries the name→owner table); `dimension_status_required`,
  `config_hash_required`, `label_evidence_required`, and `baseline_idempotent`
  live in `cmd/archfit/erosion_test.go` and run the real command over a fixture
  repo. `no_scalar_decision` scopes `internal/application/analysis.go` to
  `outcomeFor`/`seamAnchor`, NOT the whole file: `AnalysisResult` still CARRIES
  `score.Scorecard` for config comparison and AI review. Keeping that internal
  diagnostic is not the same defect as deciding from it. The scoped rule FAILS if its
  target function is renamed away, so it cannot silently check nothing.
  `label_evidence_required` is the one check whose positive case is vacuous today
  (`.archfit-labels.yaml` is `labels: []`) — its fixtures are what prove it works.
- **`measurement` is a property of the tree, never of the run**
  (`report.StateMeasurement`, populated in `application.projectArchitectureState`).
  Exactly four fields: `source_ref`, `history_depth`, `history_window`,
  `tool_versions`, pinned by `TestMeasurementCarriesOnlyDeterministicFields`. One
  wall-clock timestamp, absolute path, or PID here retires the byte-identity
  contract every format baseline depends on. A full run reports
  `source_ref: worktree` — it measures files on disk, and naming a commit would
  claim the bytes equal it even on a dirty tree; only a delta run, which really
  diffed against a resolved SHA, publishes one. A run that scanned no history
  records `history_window: unavailable` with depth 0 rather than leaving both
  blank, so "no history here" stays distinguishable from "the scan was never
  wired up".
- **The four comparability fingerprints live ONLY in the root `comparison`
  block** (`TestFingerprintsLiveOnlyInTheComparisonBlock` walks the serialised
  wire form). A second copy is a second answer to "may these two runs be
  compared", and the copies drift. `labels_hash` is `omitempty` and absent when
  no label is approved — empty compares equal to empty, so two unlabelled repos
  stay comparable; a malformed labels file is exit 3 long before a report
  exists. The measurement profile also lives under `comparison` and records the
  normalized settings hash plus producer semantics/status/tool versions; an
  unknown or incompatible profile is `non_comparable` with named reasons. A
  seam's `label_evidence_hash` is a DIFFERENT fact (the edges behind one label)
  and stays on the seam. The persisted baseline is exposed separately as
  `gate_reference`; it is not the report-only `--base` comparison.
- **`change_locality`'s denominator is the DECLARED module set**, not the touched
  count (`changeLocalityDimension`). Observed-over-observed is a tautology: it
  reported 100% coverage on a window that reached one module out of forty.
- **Baseline schema v2** (`internal/baseline`, `SchemaVersion =
"archfit.baseline.v2"`). Stores accepted findings, the metric snapshot, and the
  architecture-state reference: the four comparison fingerprints (`config_hash`,
  `model_hash`, `labels_hash`, `rubric_version`) and the measurement profile
  travelling with the facts they qualify — hard-gate finding IDs,
  distributed-monolith seam IDs, and the nine dimension snapshots. NO repository
  scalar is written. Older schemas are
  rejected; changing only `schema_version` cannot synthesize the missing state
  reference. Preserve the old file for owner review and regenerate deliberately
  with `archfit baseline` after reviewing current findings.
  `seamAnchor` (`internal/application/analysis.go`) is comparable only when
  `decision.CompareFingerprints` finds all four equal, and the SAME anchor feeds
  both the seam gate and the drift dimension, so the two cannot disagree about
  whether a comparison was admissible.
- **`archfit baseline` capture is a pure function of tree + config**
  (`BaselineService.Execute` runs with `EmptyBaseline: true`). Reading the file it
  was about to overwrite made the capture self-referential: BC advisories roll up
  per `(module pair, strength, distance, volatility, STATUS)`, so accepting a
  group's representative split the group on the next run, exposed its siblings as
  new representatives, and wrote a different file every time. Three captures on
  this repo produced 108, 164, then 148 accepted entries and never settled
  (`cmd/archfit.TestRun_Baseline_IsIdempotent`).
  Capture also skips findings covered by temporary waivers, including expired
  waivers, and prints the count; it never silently turns temporary exceptions
  into permanent accepted debt. A profile mismatch or incomplete seam snapshot
  requires review, not a blanket re-baseline.
- Parse config once into typed views; pass a package its view, not the whole config.
- LLM SDKs (`anthropic-sdk-go`, `openai-go`) are off-gate: only `config enrich`,
  `config init --ai-classify`, `config update --ai-classify`, `analyze --ai-summary`, and `explain --ai-summary`
  touch them, never the gate. Enforced structurally — `arch_test.go` forbids any
  `internal/*` package from importing `internal/llm`, so the LLM commands live in
  `cmd`.
- `gate:` is wired for **all rule types** (`off` skips, `warn` is advisory/non-blocking,
  `fail`/unset is blocking; exceptions: `public_api_change` and
  `public_api_type_leak` default to `warn` when unset). An unknown `type` value is a config error.
  `metrics.<name>.gate` follows the same convention: a worsening baseline delta
  blocks when `gate` is unset. A tripped ratchet produces NO finding, so it
  reaches the verdict the same way the required-tool gate does — through
  `evaluation.blockingMetricRegressions` into the state's hard-gate result
  (`buildState`), never through the finding populations. It also raises the
  owning dimension's `gate` to `fail`, routed by the envelope's own metric list.
  Asserting only `evaluation.Result.Verdict` cannot see this: nothing reads that
  verdict for the exit code. The state carries no ratchet field, so text and
  Markdown name the ratchet (`METRIC RATCHET` / `## Metric ratchet`,
  `ratchetRegressions`, twin helpers in console and markdown) from the
  Document's metric deltas, and only when the contract proves it, with verdict
  blocked. With zero active blockers and no coverage gap gating `fail`, they
  list every metric that worsened against the accepted baseline. Otherwise
  (`ratchetProvenDimensions`) they list the worsened metrics of each failing
  dimension with no hard-gate finding ref — for `operations`, also no failing
  analyzer gate; a ratchet beside a hard-gate finding in its own dimension stays
  unnamed. Thresholds are not in the contract, so a worsened metric inside its
  threshold is listed too; the label says "worsened", never "tripped".
  `MetricEntry.Enabled` is a `*bool` so a knob-only
  entry (`{gate: warn}`) stays enabled — only explicit `enabled: false` disables
  the metric (`metrics.New`). `coupling_balance` does not gate at all — the only
  coupling gate is `coupling.gate.distributed_monolith`; see the coupling-gate
  invariant above.

## Coupling scorer — key design facts

`ScoreVersion = "bc_score.v6"` (`internal/model/report/report.go`) — it is part
of the published report contract; `internal/relationship/analysis` carries a
private `relationshipScoreVersion` mirror that must stay in step.
The formula implementation lives in `internal/relationship/scoring/scorer_book.go`:
`balance = max(|S−D|, 10−V) + 1` (Khononov Ch10 verbatim).
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

**SCIP semantic overlay is report-only** (`internal/evidence/acquisition/semantic_overlay.go`,
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
`internal/assessment/score`, `degenerateGraph`). Opt-in `analyzers.cargo_modules.enabled`
adds an **intra-crate module graph** (`<crate>::<mod>` nodes + aggregated `uses`
edges), so single-crate projects get real cycle/blast-radius/cohesion signal.
Opt-in `analyzers.scip.enabled` runs rust-analyzer SCIP, which produces a correct
`<crate>::<mod>` strength map and attaches `StrengthHint` to those module edges.
Relationship analysis then registers auto-discovered module nodes as modules
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
wired to any metric. `graph.CrateRoot` gains `Crate string`; `internal/testdata/model_surface.golden` was regenerated.

Cargo dependency edges are located in the member's own `Cargo.toml`, at the line
that declares the dependency in the table of its kind
(`internal/extract/rust/manifest.go`, a conservative line scan that honours
`rename`, `[target.<cfg>.…]`, `[x.dependencies.<key>]` and skips
`[workspace.dependencies]`). A declaration it cannot place keeps line 0, and a
member outside the scan root gets no location. Locations never enter finding IDs
or label hashes.

## Layout

`cmd/archfit` (kong CLI) · `internal/` decision core + adapters · `docs/design`
(current decisions) · `docs/guide` (user docs) · `docs/spec` (spec) ·
`docs/plans` (open plans only; shipped plans move to `docs/plans/completed/`).

**Architecture reference:** `docs/design/architecture-baseline.md` — the shipped
capability map, layer ranks, measured module dependencies, which check enforces
which invariant, accepted coupling, and change recipes. Read it before moving a
package, adding a metric/language/output format, or touching `.archfit.yaml`.
`docs/design/20260823-archfit-capability-map.md` is the design rationale behind
it.

**Skip `docs/archived/`** — superseded design docs, completed plans, plan notes,
research artifacts, and analysis notes. Only read when explicitly debugging
history or looking up a completed plan by name.

<!-- gitnexus:start -->

## GitNexus — Code Intelligence

This project is indexed by GitNexus as **archfit** (14346 symbols, 42693 relationships, 592 execution flows).

> Index stale? Run `node .gitnexus/run.cjs analyze --index-only` from the project root — it auto-selects an available runner. No `.gitnexus/run.cjs` yet? Bootstrap with `npx`, `bunx`, or `pnpm dlx` — e.g. `bunx gitnexus@latest analyze` (npm 11 npx crash; #1939).

## Always Do

- **MUST run impact analysis before editing.** Use `impact({target: "symbolName", direction: "upstream"})` (MCP) or `node .gitnexus/run.cjs impact "symbolName" --direction upstream --repo .` (CLI fallback); report callers, processes, and risk. Never substitute grep for graph analysis.
- **MUST analyze graph changes before committing.** Use `detect_changes({scope: "all"})` (MCP) or `node .gitnexus/run.cjs detect-changes --scope all --repo .` (CLI fallback). `partial: true` or `truncated: true` is not a clean check — a zero means unseen, not unaffected; re-run it. For regression review: `detect_changes({scope: "compare", base_ref: "main"})` or `node .gitnexus/run.cjs detect-changes --scope compare --base-ref "main" --repo .`.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- **MUST treat `risk: UNKNOWN` as unresolved, not as low.** An empty caller set is not evidence the symbol is unused — it can also mean the callers are not resolvable by the index (plain-object property access, dynamic dispatch, cross-language calls). `impact` pairs `UNKNOWN` with a `riskNote` saying so. Confirm with a text search before treating the symbol as safe to change or delete; do not proceed on the strength of a zero.
- When exploring unfamiliar code, use `query({search_query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `context({name: "symbolName"})`.
- For security review, `explain({target: "fileOrSymbol"})` lists taint findings (source→sink flows; needs `analyze --pdg`).

## Never Do

- NEVER edit a function, class, or method before MCP/CLI impact analysis.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis, and never read `UNKNOWN` as an all-clear — it means the walk could not answer, which is the one verdict that requires confirming by other means.
- NEVER rename symbols with find-and-replace — use `rename` which understands the call graph.
- NEVER commit before MCP/CLI graph change analysis.

## Resources

| Resource                                 | Use for                                  |
| ---------------------------------------- | ---------------------------------------- |
| `gitnexus://repo/archfit/context`        | Codebase overview, check index freshness |
| `gitnexus://repo/archfit/clusters`       | All functional areas                     |
| `gitnexus://repo/archfit/processes`      | All execution flows                      |
| `gitnexus://repo/archfit/process/{name}` | Step-by-step execution trace             |

## CLI

| Task                                         | Read this skill file                               |
| -------------------------------------------- | -------------------------------------------------- |
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus-exploring/SKILL.md`       |
| Blast radius / "What breaks if I change X?"  | `.claude/skills/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?"             | `.claude/skills/gitnexus-debugging/SKILL.md`       |
| Rename / extract / split / refactor          | `.claude/skills/gitnexus-refactoring/SKILL.md`     |
| Tools, resources, schema reference           | `.claude/skills/gitnexus-guide/SKILL.md`           |
| Index, status, clean, wiki CLI commands      | `.claude/skills/gitnexus-cli/SKILL.md`             |

<!-- gitnexus:end -->
