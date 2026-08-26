# Post-migration follow-ups

Date: 2026-08-26
Status: OPEN
Source: code review of the capability-architecture migration
(`docs/plans/complete-capability-architecture-migration.md`). Every behavioral
regression that review found is fixed on the branch; what remains here is work
that is real but larger than a review fix, plus test gaps that no longer guard
a known defect.

## 1. Collapse the config-update / config-enrich port sets

`ConfigUpdateService` (`internal/application/config_update.go`) declares eight
ports and `ConfigEnrichService` (`internal/application/config_enrich.go`) seven.
Each set has exactly one production implementation
(`cmd/archfit/config_update_adapters.go` `configUpdateAdapter`,
`cmd/archfit/config_enrich_adapters.go` `configEnrichAdapter`) and one test fake.

Four of them carry no data:

- `ConfigUpdateDiscovery.DiscoverConfigUpdate(ctx, req) error` returns only an
  error; the discovery result is stashed in adapter fields.
- `ConfigUpdateProjector.ProjectConfigUpdate(ctx)` takes no arguments and reads
  that hidden state, so `Execute` calls it twice (`config_update.go:166` and
  `:194`) to re-read after classification mutates it.
- `ConfigUpdateReviewer` and `ConfigUpdateEditor` follow the same shape.

The migration plan permits new interfaces "only at context, I/O, persistence,
provider, or process boundaries". Discovery, projection, review, and editing are
none of those, and routing them through adapter-internal mutable state means the
application owns the call order but not the data flow.

Proposed shape: keep `Configs`/`Files`/`Writer` (I/O) and `Classifier` (LLM
provider). Collapse discovery and projection into one call that RETURNS the
`ConfigUpdatePlan`, which removes both the double call and the hidden state;
fold review and editing into it or pass the plan explicitly. Same treatment for
`ConfigEnrichEditor`/`ConfigEnrichDraftStore`.

This touches the config-update golden tests, so it is its own change.

## 2. Remaining owner-local test gaps

The migration moved these behaviors to new owners without carrying their
assertions across. None currently guards a known defect — they guard the seams
this migration moved.

- `internal/relationship/analysis/summaries.go` `buildLocalCouplingSummary`
  (`:270`): `ScoredEdges`, `AbstainedEdges`, `ComplexityEdges`,
  `ComplexitySharePct`, `MeanBalance`, the offender sort, and the hard-coded
  cap of 5 are unasserted. Only non-empty/empty is checked.
- `internal/assessment/agenttask/advisory.go` `BuildAdvisoryTasks`: reached from
  `evaluation/finalize.go:63`, but the only assertion
  (`coupling_gate_test.go:186`) checks the task count and `FindingID`.
  Unasserted: gate-kind exclusion, `group_count <= 1` exclusion, `GroupMembers`
  split, `ScoreValue` parse, `TopFiles` dedup/sort/cap-8, the four constraint
  strings, `Validation` copy, and the module-less `Goal` fallback.
- `internal/relationship/analysis/advisories.go` `appendLocations` (`:71`): only
  the empty-input early return executes. This is where clone-derived locations
  merge, deduplicate, and sort into the `files[]` of `agent_tasks`.
- `internal/relationship/analysis/dynamic.go` `buildDynamicImports` (`:29`): no
  test sets `DynamicImportSites`, so grouping, the sample cap, the deterministic
  sort, and the unmapped-file module fallback never run.
- `internal/relationship/analysis/analysis.go` `cloneEvidence` (`:177`) and
  `cloneLanguage` (`:237`): the derived both-sides `file:line` map, its sort, and
  the test/generated-file fallback classification are unasserted.
- `internal/assessment/evaluation/evaluation.go` `computeVerdict` (`:131`): no
  case for a zero-value `Direction` (a `string`, so `""` silently takes the
  higher-is-better branch), none for `Delta == 0` exactly, and every subtest
  supplies one metric, so the warn-then-fail precedence is unexercised.
- `internal/extract/acquire` has no test file: only the disabled branches of the
  runtime-site, SCIP, and syntax passes execute.
- `cmd/archfit/enrich.go` `EnrichLabelsCmd.Run` (`:54`) and `runLabelEnrich`
  (`:101`) are 0%; the `application.EnrichService` they delegate to is covered.
- `cmd/archfit/check_test.go:169,204` skip when the fixture produces no advisory
  findings. If advisory generation regressed to zero, both tests would skip
  rather than fail. `TestRun_Check_MinSeverityFiltersAdvisories` also only
  asserts `critical <= unfiltered`, which passes if the filter does nothing.
- `cmd/archfit/analysis_config.go:22,38` call `config.ValidateRules`; deleting
  either call fails no test.

## 3. Accepted cost, recorded deliberately

`internal/model/report/evidence.go` restates every struct of
`internal/model/evidence` field-for-field, mapped by `ProjectReport`
(`internal/application/report.go`) plus a second hop through
`internal/assessment/evaluation/relationship_projection.go`. This is the direct
consequence of the plan's "Assessment and Relationship import no report DTOs"
plus "one pure Application-owned projector", so it is not a defect — but every
new evidence field now costs three edits and a mapper line.
`TestProjectReportCarriesEveryEvidenceBlock` exists so a dropped mapper line
fails a test instead of shipping an empty JSON block.

## 4. Structural gaps the second review pass routed here

Each is a real weakening of a guard rather than a live defect, and each is
larger than a review fix.

- **Coverage names have two independent copies.** The emitters
  (`internal/extract/{astgrep,scip,clones,rust}`) and the consumer
  (`internal/assessment/decision.AnalyzerFamilies`) no longer share one constant
  set — the move out of `cmd/archfit` split them into non-importing packages.
  `internal/coverage_names_test.go` pins only `dependency-cruiser`, `grimp`, and
  `go/packages`. Renaming `ast-grep/syntax`, `scip`, `scip-symbols`, `jscpd`, or
  `cargo-modules` in an adapter compiles and passes, while every
  `analyze --base` task silently degrades to `unknown` origin. Extend the test
  with the same adapter-driven pattern for the other five.
- **`Config.ExtractConfigs()` hand-lists the four languages**
  (`internal/config/projection.go`) while `registry.Build` iterates the
  registry, so a fifth registered language would receive a zero `ExtractConfig`
  (`Mode ""`, no exclusions) and `TestBuildExtractorsOrder` would still pass.
  `CoverageOptions()` already iterates `registry.All()` correctly. Build the map
  the same way, or assert a key per registry ID.
- **Two dead slots on `relationship.ClassifiedEdgeSummary`**
  (`internal/relationship/advisory.go` `LLMApproved`, `VolatilityProvenance`):
  never written by `buildClassifiedSummary`, always overwritten downstream by
  `evaluation/projector.go`. Reading them off `AnalysisResult` yields zero.
  Remove them and populate only `result.ClassifiedEdgeSummary`.
- **The exported-surface gate is narrower than the criterion it enforces.**
  `internal/surface_test.go` limits `surfaceOwners` to five packages
  (`internal/assessment/decision`, `internal/assessment/result`, and
  `internal/policy` are outside it), checks only `*types.Func` rather than types
  and fields, and `identifierUses` indexes `_test.go` files — so an export used
  only by an out-of-package test counts as having a production consumer. Widen
  the owner set and exclude test files. Related surface hygiene: several
  `decision` helpers (`CompareAnalyzerEvidence`, `PartialFromDegradedPrecision`,
  `UnresolvedMagnitude`, `DegradedMagnitude`, `HasCoverageGap`) lost their only
  out-of-package consumer when `git_finding_delta.go` moved in and can be
  unexported.
- **Baseline and pinned-label validation now runs after evidence acquisition**
  (`internal/application/analysis.go` `Execute`). A malformed
  `.archfit-baseline.json` still exits 3, but only after the full extractor pass
  and its fact-cache writes; `main` rejected it before any subprocess ran. The
  error is also double-prefixed (`baseline: baseline: parse …`). Moving the load
  ahead of `Acquire` needs the resolved bundle dir, which today only
  `AnalysisContext` carries.

---

# Deferred from the architecture-state-reporting review

Date: 2026-08-26
Source: code review of `docs/plans/architecture-state-reporting.md`. Everything
mechanical that review found is fixed on the branch. What follows is real, cited,
and larger than a review fix.

## A. The comparison model is implemented only against the stored baseline

Three findings, one root cause. `evaluation.Score` builds the architecture state
(`internal/assessment/evaluation/assess.go`), and `attachBaseComparison` runs
*after* it (`internal/application/analysis.go`), so nothing computed by the base
sub-run can reach a dimension envelope.

1. **`--base` never becomes the comparison reference.** The plan freezes the
   precedence at line 425: "an explicit `--base` reference wins; otherwise the
   stored v2 baseline is used; otherwise no 'new seam' claim is made."
   `seamAnchor` (`internal/application/analysis.go`) reads only the persisted
   baseline. `scoreBaseTree` computes the base tree's seams and then drops them:
   `application.BaseEvidence` (`internal/application/base_compare.go`) carries
   finding IDs, coverage, gaps, and hashes, not `QualifyingSeamIDs`. So
   `check --base main` with `mode: fail` can never gate against `main`, and the
   output contradicts itself — root `comparison.status: comparable` beside
   `dimensions.drift: unmeasured` with reason "no comparable architecture-state
   reference is stored".
2. **The root `comparison` block never discloses a baseline reference.**
   `comparison.status` is written only by `attachBaseComparison`
   (`internal/application/report.go`). With a comparable v2 baseline and no
   `--base`, `dimensions.drift` is `measured`, new/resolved seams are published,
   and the seam gate can block — while the root block still says
   `not_requested` / `reference: none`. Two answers to "was this run compared".
3. **Per-dimension deltas are never produced.** Only `driftDimension` sets
   `Delta` (`internal/assessment/evaluation/dimensions.go`);
   `DimensionDelta.Metrics` is never populated for any dimension. The v2 baseline
   writes `DimensionSnapshot` and `HardGateFindingIDs`, and
   `application.BaselineDimension` loads them, but **no code reads them** — only
   `QualifyingSeamIDs` is consumed. Either feed the snapshots into `stateInput`
   or delete the unread fields and record the deferral.

Fix shape: carry the base run's qualifying seam IDs and dimension snapshots on
`BaseEvidence`, and either move the state build after the base comparison or pass
the resolved anchor into `evaluation.Score`. One design change closes all three.

## B. `config compare` still headlines the retired repository scalar

`archfit.config-compare.v1` emits `score_delta` plus each side's full
`report.Scorecard` (`cmd/archfit/config_compare.go`), and the text renderer prints
`score: 40 (mixed) → 46 (mixed) (+6)` (`scoreCompareLine`). The plan's frozen text
(line 765) says a config/model/labels/rubric mismatch emits `non_comparable` and
**no numerical delta** — and `config compare`'s `config_hash` differs by
construction. This is the one non-legacy output where the retired averaged score
is still decision support, and it says exactly what the plan forbids.

Deferred rather than fixed in review because it is a schema change to a separate
published `v1` contract, and the plan pairs the removal with an addition —
"compare seam IDs and distributions when comparable" (Task 3) — that does not
exist yet. Do both together: drop `score_delta` and the two `Scorecard` blocks,
add seam-ID and distribution buckets beside the existing finding buckets.

## C. Test harnesses for paths whose tests were lost in the migration

Each is a real gap, none guards a known live defect:

- `internal/history/git/worktree.go` — every error return deliberately hands back
  a non-nil `cleanup` closure, and `lockWorktree`'s poll loop has a `ctx.Done()`
  arm. Neither is exercised. A regression leaks a temp worktree and an
  inter-process flock keyed on the base SHA, blocking later runs, or hangs a
  cancelled `--base` run.
- `internal/evidence/acquisition/service.go` `collectRuleEvidence` — the
  `SyntaxCfg.Enabled=true` + nil-provider guard, the provider error return, and
  the per-fact module stamping. If stamping regresses, syntax-driven rules match
  nothing and report zero violations, which is indistinguishable from a clean
  repo.
- `internal/relationship/analysis/advisories.go` `appendLocations` — only the
  empty-input early return runs. The dedup, the `CloneLocations` merge, and the
  file/line sort do not; an unstable sort there breaks byte-identity for every
  committed baseline.
- `internal/relationship/analysis/summaries.go` `buildLocalCouplingSummary` — the
  offender sort comparator, the 5-offender cap, and `ComplexitySharePct` are
  uncovered; the block is published output.
- `internal/relationship/analysis/seams.go` `keepWorst` — all three tie-break
  levels are unexercised. The final lexical tie-break is what makes seam
  representative choice deterministic.
- `internal/assessment/evaluation/advisories.go` `groupBCAdvisories` — no test
  feeds two candidates differing only in `Status`, which is the mechanic behind
  the baseline self-referentiality bug CLAUDE.md documents.

## D. Surface and vocabulary hygiene introduced by the branch

- `internal/history/git` exported eight symbols (`CleanEnv`, `WorktreesDir`,
  `WorktreeParent`, `SnapToRoot`, `SubtreeInWorktree`, `AddWorktree`,
  `RemoveWorktree`, `ResolveCommit`) with no out-of-package consumer; all were
  unexported on `main` before the move. Only `Worktree.Checkout` has one.
- `internal/model/report`'s decision view-model is write-only:
  `report.Report`, `DimReport`, `Recommendations`, `DecisionBand` + its four
  constants, `Document.Decision` (tagged `json:"-"`), and
  `application.projectDecision`/`projectRecs`. The renderers moved to
  `RenderState`; nothing reads the projection. Deleting it moves
  `internal/testdata/model_surface.golden`, so regenerate with
  `ARCHFIT_UPDATE_SURFACE=1` and call the contract change out in review.
- `internal/relationship/coupling` is half alias-façade (`Strength`, `Distance`,
  `Volatility`, `Severity` alias `internal/relationship`) and half duplicate
  vocabulary (`Location`, `Explicitness`, `ConnascenceEvidence`, `EdgeScore`,
  `ScoreBreakdown` are separate types needing a hand mapper at
  `analysis.go:303-329`, which also renames three fields for no reason). Pick one
  direction; ~216 call sites.
- `internal/assessment/state` and `internal/model/report/state.go` declare the
  identical string for ~30 state terms with no shared source, and
  `internal/model/evidence` / `internal/model/report/evidence.go` for 9 more.
  Only four pairs are pinned by `internal/surface_test.go`.
- Auxiliary coverage names (`ast-grep/syntax`, `deploy-unit`, `cargo-modules`)
  are declared as private constants in three non-importing packages plus two bare
  literals. They are a cross-package protocol: renaming one compiles and passes
  while `analyze --base` silently degrades every task to `unknown` origin.
- `internal/assessment/agenttask/agenttask.go` `pythonModuleFileCandidates` is a
  byte-identical private fork of `internal/model/graph/convention.go`. Three
  things must now stay in step with no test pinning them.
