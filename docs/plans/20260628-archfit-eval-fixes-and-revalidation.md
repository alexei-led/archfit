# archfit Eval Fixes, Metric-Correctness & Re-validation

## Overview

Fix every issue surfaced by the 2026-06-28 seven-repo meta-evaluation
(`reports/eval-2026-06-28/00-FINDINGS.md`), correct the metric false-positives
(especially test/mock/generated code inflating production-health metrics), add the
missing version-diff capability, then **re-validate an ideally-configured archfit**
(all helper tools + opt-in metrics enabled per repo) across the seven repos in full,
delta, and LLM modes — confirming each fix moved its metric in the predicted direction.

**Problem it solves:** the eval proved archfit (a) miscomputes distance for flat-named
single-team repos (false "tight coupling"), (b) corrupts metrics under macOS path-case
mismatch, (c) carries two disagreeing severity models, (d) counts test/mock/generated
code as production risk, (e) cannot compare architecture version-over-version, and (f)
ships ~half its metrics off-by-default with silent coverage gaps.

**Code-map corrections that change the fixes (verified against source, not the report's prose):**

- `panic_density` **already** excludes `*_test.go` (`internal/metrics/modularity/panic_density.go:43` → `syntax.IsTestFile`). pumba's 208 panics are in `mocks/`+`mock_*.go`, which Go's `IsTestFile` (suffix `_test.go` only) misses → real fix is **generated/mock detection**, not "exclude tests".
- The fan-in `.test` inconsistency is **not** a `go/packages` artifact (loaded without `Tests:true`, `internal/extract/golang/golang.go:133` — no `.test` nodes ever). It comes from the **SCIP symbol graph** (`scip-go` indexes tests) at `internal/facts/facts.go:130`; `blast_radius` uses the package import graph (`modgraph.FirstPartyModules`) and excludes it. Fix targets the SCIP refs path.
- gocyclo-absent **already** falls back to the ast-grep proxy for Go (`internal/extract/complexity/complexity.go:118-120`) — the bug is the misleading "install gocyclo" message, not a missing fallback.
- P3 diverges in **both** directions: `BalanceResult` can say `None` where book says `Medium` (S=9 symmetric, low distance) and `Critical` where book says `High` (S=9, high distance). Switching to book-severity both **adds and removes** gate findings → baseline/goldens must be regenerated.

## Context (from discovery)

**Findings → task coverage** (every eval finding maps to a task):

| Eval finding                                                                  | Task  | Verified anchor                                                                                                                   |
| ----------------------------------------------------------------------------- | ----- | --------------------------------------------------------------------------------------------------------------------------------- |
| P4 explain ignores `--root`                                                   | 1     | `cmd/archfit/explain.go:18-23,41` (`runPipeline(...,"",...)`)                                                                     |
| P2 macOS case-path corruption                                                 | 2     | `scope.go:204-222`, `rust.go:345,351`, `scip_strength.go:198,352-367`                                                             |
| P1 flat-name distance = false DiffOwner                                       | 3     | `internal/classify/distance_structure.go:45-47`                                                                                   |
| P3 two severity models disagree                                               | 4     | `classify.go:326` (`BalanceResult`) vs `scorer_book.go:78`+`scorer.go:139` (`ScoreBand`)                                          |
| Mock/test/generated false positives                                           | 5–8   | classifier + `panic_density.go:43`, `functional_candidates.go:38`/`clone.go:24`, `structural_weight.go:33`, `complexity` backends |
| Fan-in counts `.test` (spotinfo 3 vs 2)                                       | 9     | `facts.go:130`, `scip/symbols.go:50-53`                                                                                           |
| P6 SCIP empty index reported `ok`                                             | 10    | `scip_strength.go:84-89` (len(m)==0 still StatusOK)                                                                               |
| P7 jscpd clusters empty for Rust                                              | 11    | `clones.go:102-114,172-177`                                                                                                       |
| P12 syntax opt-in silent; #18 lizardExcludes; #17 gocyclo msg; #23 wording    | 12    | `pipeline_run.go:241-250`, `complexity.go:47-60,118-120`, `pipeline_coverage.go`                                                  |
| file_extraction_coverage>1.0; cohesion_lcom TS IntraRefs; change_locality n/a | 13    | metric + `cohesion.go:79`; scip-python 3.14 (doc)                                                                                 |
| #21 LLM org-distance hallucination; #22 strength hallucination; conflict-flag | 14    | `explain.go:99-103`, `review.go:266-349,492-560`                                                                                  |
| P10 synthetic submodules lack owner                                           | 15    | `classify.go:130-148,171-197`                                                                                                     |
| P9 delta "no baseline" misleads                                               | 16    | `check.go:70`, `baseline/baseline.go:70-78`                                                                                       |
| P11 observed-layer vs role divergence                                         | 17    | `rules.go:73,179-186`, `rules_dependency.go:97+`, `modules.go:154-163`                                                            |
| P8 no native version diff                                                     | 18    | new `cmd/archfit/diff.go`                                                                                                         |
| Re-validation (controlled binary-only + ideal-config showcase)                | 19–22 | per-repo ideal configs + held-constant diff vs eval JSON                                                                          |

**Safety gates** (keep green; regenerate deliberately on scoring changes):

- Import ring: `go test ./internal/ -run TestArchImports`.
- Determinism: `go test ./internal/engine/ -run TestGolden` (double-run byte-identity; **no snapshot file to update** — `internal/engine/golden_test.go`).
- Dogfood: `make archfit` (`archfit check --config .archfit.yaml --full`) gated by `.archfit-baseline.json`. Tasks 3, 4, 15, 17 change scoring → run `make build && make archfit`, inspect the finding diff, restamp `.archfit-baseline.json`, commit it with the code. **Before restamping, confirm each newly-accepted finding _follows from_ the severity/distance change and is not an unrelated regression** (cf. the re-baseline gotcha: a phantom negative metric delta with 0 findings flips PASS→WARN/exit-2).

**Patterns/conventions:** Go, `CGO_ENABLED=0`; subprocess only via `toolrun.Runner` (fake in tests); core ring must not import `os`/`os/exec`/YAML/adapters; parse config once into typed views; LLM SDKs off-gate (only `enrich`/`autopilot`/`explain`/`review`); table-driven tests; `make test` = `go test -race ./...`.

## Development Approach

- **Testing approach: Regular** (code then tests) with `fixing-code` discipline: reproduce the wrong value on a fixture, apply the minimal patch, add a regression test that fails pre-patch.
- Complete each task fully (code + tests green) before the next.
- **Every task includes new/updated tests as separate checklist items** (success + edge/error).
- **All tests pass before starting the next task.** Behavior-changing tasks also keep `make archfit` green (restamp baseline deliberately).
- Smallest correct change; match surrounding style; no speculative abstraction.
- Update this plan (`[x]`, ➕ new, ⚠️ blocker) as work proceeds.

## Testing Strategy

- **Unit tests** required per task — table-driven where an input/output matrix exists (distance ordinals, severity by (S,D,V), file-class by language).
- **Integration tests** for subprocess adapters (jscpd clusters, SCIP empty-index) using a faked `toolrun.Runner` or a real-tool gated test (`-short` skips).
- **No UI / e2e** (CLI project).
- **Re-validation (Tasks 19–21)** is the system-level acceptance test: predicted metric movements on the 7 real repos.

## Progress Tracking

- Mark `[x]` immediately when done; add ➕ for discovered tasks; ⚠️ for blockers.
- Keep plan in sync with actual work.

## What Goes Where

- **Implementation Steps** (`[ ]`): code, tests, config authoring, harness runs the agent can automate.
- **Post-Completion** (no checkboxes): release tagging, committing target-repo config diffs, anything needing the maintainer.

## Implementation Steps

### Task 1: P4 — `explain` honors `--root`

- [x] add `Root string` field with path tag to `ExplainCmd` (`cmd/archfit/explain.go:18-23`), mirroring `CheckCmd`/`ScanCmd`
- [x] pass `c.Root` (not `""`) as the root arg to `runPipeline` (`explain.go:41`)
- [x] write test: `explain <fp> --root <subdir>` resolves the same finding IDs as `check --root <subdir>` on a monorepo-style fixture (two services under one git root)
- [x] write test: `explain` without `--root` is unchanged (defaults to config dir) — back-compat
- [x] run `go test ./cmd/... ./internal/...` — must pass before Task 2

### Task 2: P2 — canonicalize scan-root & git-root (macOS path-case)

- [x] add `canonicalPath(p string) string` helper in `internal/scope` using `filepath.EvalSymlinks` with graceful fallback to `filepath.Abs` on error (handles non-existent/edge paths)
- [x] apply it to `scanRoot` in `resolveScanRoot` (`internal/scope/scope.go:204-213`) and to `gitRoot` before `subtreePrefix` (`scope.go:217-222`) so both sides are canonical before `filepath.Rel`
- [x] defensively `EvalSymlinks` `rootAbs` in `crateRoots()` (`internal/extract/rust/rust.go:345`) so it matches `cargo metadata` canonical `ManifestPath` casing; confirm `s.Root` flowing to SCIP `WorkDir`/`--cwd` (`scip_strength.go:198,366`) is the canonical form
- [x] write test: `canonicalPath` resolves a symlinked temp dir to its real path (Linux-testable; documents the macOS case behavior)
- [x] write test: `subtreePrefix` returns the correct subtree (not `""`) when given a case/symlink-variant scanRoot under gitRoot
- [x] run `go test ./internal/scope/... ./internal/extract/rust/...` — must pass before Task 3

### Task 3: P1 — flat single-segment distance → `CrossModuleSameOwner`

- [x] `internal/classify/distance_structure.go:46`: return `coupling.DistanceCrossModuleSameOwner` (not `DiffOwner`) for the two-single-segment-names case; rewrite the `:32-35` comment to state the BC rationale (degenerate-owner path guarantees one owner → honest floor is same-owner cross-module; labeling it different-owner converts high cohesion into false tight coupling)
- [x] reconcile the `.archfit.yaml` comment the eval flagged as "factually wrong" so it matches the corrected behavior (search self-config for the single-owner/`cross_module_same_owner` note)
- [x] before flipping: `git blame`/grep for any existing test pinning `codeStructureDistance(flat,flat)==DiffOwner` (the early-return looks deliberate — "advisory flood" history) and update it here; confirm codegraph/spotinfo edges are genuinely flat single-segment (`extraction`,`mcp`,`spot`). Frame as a **misclassification fix, not an ordinal retune**: nested far-apart names still classify `DiffOwner`, and archfit's own self-repo uses nested module names so this does not flatter its own score
- [x] write table-driven `codeStructureDistance` tests: two flat names → SameOwner; nested siblings → SameOwner (unchanged); different subtrees → DiffOwner (unchanged); Python dotted names parallel cases
- [x] write classify-level test: a flat-named single-team `functional`+`high`-volatility edge scores `Medium` (balance 6, plan said 5 — corrected; S=9,D=4,V=10 → max(5,0)+1=6) not `Critical`
- [x] `make build && make archfit`; inspect finding diff; restamp `.archfit-baseline.json` deliberately; commit baseline with code
- [x] run `go test ./internal/classify/... ./internal/engine/...` + `make archfit` — must pass before Task 4

### Task 4: P3 — derive `Severity` from the book formula (retire `BalanceResult` as severity source)

- [x] `internal/classify/classify.go`: moved `cl.Severity = cl.Score.Band` into `Run` after scorer runs (not `ScoreBand(cl.Score.Balance)` which would stamp 0-balance edges as Critical); removed `BalanceResult` call from `classify()`
- [x] keep `coupling.BalanceResult` only if used elsewhere; otherwise delete it and its now-dead tests — deleted `BalanceResult` + `strengthIsHigh` (dead after deletion); deleted `TestBalanceResult`
- [x] verify Case A (S=9 symmetric, SameOwner, V=high) now emits a `Medium` advisory where it previously emitted none, and Case B (S=9, DiffOwner, V=high) emits `High` not `Critical`; ensure these stay advisory unless a rule gates them
- [x] write table-driven severity tests over (S,D,V) incl `StrengthSymmetric`: assert `Severity == ScoreBand(balance)` for both divergence cases — `TestScoreBand_Severity` in `coupling_test.go`
- [x] write test: `explain` and `check` report identical severity for the same fingerprint (closes ccgram `2c5200c6` mismatch) — `TestRun_Explain_SeverityMatchesCheck` in `main_test.go`
- [x] `make build && make archfit`; restamp `.archfit-baseline.json`; confirm `TestGolden` determinism still passes — baseline has 0 findings before and after; no restamp needed; `TestGolden` double-run passes
- [x] run `go test ./internal/...` + `make archfit` — must pass before Task 5

### Task 5: Shared `FileClass` classifier (auto-detect + config override)

- [x] add `internal/syntax/fileclass` (or extend `internal/syntax`): `Classify(lang, path string, header []byte) FileClass` returning `Production | Test | Generated | Vendor` — implemented as `syntax.ClassifyFile` in `internal/syntax/fileclass.go`; type in new `internal/model/fileclass` package
- [x] Generated detection: `// Code generated .* DO NOT EDIT` header regex (covers moq + Go stdlib), `*.pb.go`, `*_gen.go`/`*.gen.*`, `_pb2.py`; mock heuristics `mock_*.go`/`*_mock.go`/`mocks/` → Generated. Test detection: reuse/extend `syntax.IsTestFile` (`internal/syntax/testfile.go:17`) per language (+ Rust `tests/`, best-effort `#[cfg(test)]`)
- [x] config override: new top-level `file_class:` key (`FileClassDef`) with `generated_globs`, `test_globs`, `mock_frameworks`; projected via `Config.ForFileClass()` → `FileClassView`; auto-detection runs first, config patterns extend it. Note: plan said `tools.file_class` but `ToolsConfig` is `map[string]ToolConfig` — used top-level key instead (correct design).
- [x] stamp the result onto facts so metrics filter without re-reading: `loc.Run` now returns a parallel `map[string]fileclass.FileClass` covering ALL source files (including test/generated); stored in `SizeSignals.FileClassIndex`; `syntax.LookupFileClass` provides path→FileClass lookup with fallback for paths not in the index (e.g. files in loc's `skipDirs`). Note: `FileFact.Class` is per-module, not per-file — delivering per-file map is the correct grain for tasks 6-9.
- [x] write table-driven tests per language for each class incl moq header-sniff; config override adds a custom mock pattern and reclassifies a file
- [x] run `go test ./internal/syntax/... ./internal/extract/loc/...` — must pass before Task 6

### Task 6: `panic_density` — exclude Generated/Mock, segregate the count

- [x] `internal/metrics/modularity/panic_density.go:43`: filter on `FileClass` (skip `Test` AND `Generated`) instead of only `syntax.IsTestFile`; tally excluded panics
- [x] surface the excluded count in the metric evidence string ("N panics in test/generated excluded") so nothing is hidden
- [x] write test: a fixture with `mock_*.go` panic sites → production `panic_density` excludes them, excluded count reported (reproduces pumba 208→~0 production)
- [x] write test: a real production panic is still counted
- [x] run `go test ./internal/metrics/...` — must pass before Task 7

### Task 7: `functional_candidates` / jscpd — drop test/generated clone pairs

- [x] post-filter `clone.ModulePairs` (`internal/model/clone/clone.go:24`) / `functional_candidates.go:38` to drop pairs where either file's `FileClass` is `Test`/`Generated` (source of truth)
- [x] also pass coarse `--ignore` globs (`*_test.*`, `mock_*`, `*.pb.go`, generated dirs) to the jscpd invocation (`internal/extract/clones/clones.go:107-109`) for scan-time speed
- [x] write test: clone pairs among `_test.go`/`mock_*.go` are excluded; a genuine production cross-module clone is retained (reproduces pumba 13→1)
- [x] write test: `cohesion_modularity` no longer drops from test/mock clone noise — cohesion score reads from `functional_candidates` value which is now filtered; no independent re-derivation
- [x] run `go test ./internal/...` — must pass before Task 8

### Task 8: `structural_weight` & `complexity` — exclude Generated consistently

- [x] `structural_weight`/`file_structural_weight` (`internal/metrics/modularity/structural_weight.go:33`): exclude `Generated` files (loc already drops `mock_`/`test_`; add header-based generated via `FileClass`)
- [x] complexity: make `gocyclo` (`internal/extract/complexity/gocyclo.go`) and the ast-grep `proxy` (`proxy.go:81`) exclude `Test`+`Generated`, matching `lizardExcludes` (`complexity.go:57-60`) so all three backends agree
- [x] write test: a generated `*.pb.go` god-file is excluded from `structural_weight`; a real god-file still flagged
- [x] write test: complexity hotspots exclude generated/test functions across backends
- [x] run `go test ./internal/...` — must pass before Task 9

### Task 9: `inbound_module_fanin` — drop test packages from SCIP refs

- [ ] `internal/facts/facts.go:130` (or `internal/extract/scip/symbols.go:50-53`): when building `inboundSources`, exclude referencing modules that are test packages (`.test` suffix, or referencing file `FileClass==Test`) so fan-in matches `blast_radius`/`instability`
- [ ] write test: a SCIP-refs fixture with a `cmd/x.test` importer → `inbound_module_fanin` excludes it (reproduces spotinfo 3→2, consistent with blast_radius)
- [ ] run `go test ./internal/facts/... ./internal/metrics/...` — must pass before Task 10

### Task 10: P6 — SCIP empty index → warn, not `ok`

- [ ] `internal/extract/scip/scip_strength.go:84-89`: when resolved edges `len(m)==0` (or index file size < ~1KB), set `Coverage.Status = StatusPartial` with reason "empty index (0 occurrences) — check path case / indexer version"
- [ ] write test: an empty-index run yields `scip: warn` coverage (not `ok`) and lowers confidence visibly
- [ ] run `go test ./internal/extract/scip/...` — must pass before Task 11

### Task 11: P7 — jscpd clusters empty for Rust

- [ ] reproduce: confirm whether `internal/extract/clones/clones.go:102-114` invokes jscpd with a format/language set that includes Rust (`.rs`); jscpd needs `--format` or relies on extension detection
- [ ] fix the invocation so `.rs` (and other first-party langs) are scanned; verify `parseJscpdReport` (`clones.go:172-177`) populates `Duplicates`/clusters
- [ ] write integration test (faked or real-tool, `-short`-gated): a known Rust clone fixture yields non-empty clusters → `functional_candidates` measures (not n/a)
- [ ] run `go test ./internal/extract/clones/...` — must pass before Task 12

### Task 12: Tool-coverage honesty (P12, #17, #18, #23)

- [ ] add explicit `Coverage{Status: StatusDisabled, Reason:"opt-in: tools.syntax.enabled"}` rows when syntax/scip passes are skipped (`cmd/archfit/pipeline_run.go:241-250`) so they read "skipped", not absent/missing
- [ ] #18: feed config `exclude:` globs + `scope.DefaultExclusions` into `lizardExcludes` (`complexity.go:57-60`) instead of the hardcoded-only slice
- [ ] #17: rewrite the "install gocyclo" message (`complexity.go:48`) to state the ast-grep proxy already covers Go when gocyclo is absent
- [ ] #23: surface `lizard: not configured` vs `lizard: absent` distinctly in tool_coverage / review `analysis_confidence` (`complexity.go:47-49`, `pipeline_coverage.go`)
- [ ] write tests: skipped-pass coverage row present; config exclusions reach lizard; messages updated
- [ ] run `go test ./internal/... ./cmd/...` — must pass before Task 13

### Task 13: `file_extraction_coverage`>1.0, `cohesion_lcom` TS wiring, `change_locality` n/a

- [ ] `file_extraction_coverage`: align numerator/denominator scope so the ratio ≤ 1.0 — count only first-party source files in the SCIP numerator (exclude `.d.ts`/`node_modules` type decls) (locate metric; codegraph 1.02→≤1.0)
- [ ] `cohesion_lcom` TS: investigate empty `g.IntraRefs` despite SCIP 5832 symbols (`cohesion.go:79` gate); wire scip-typescript occurrence→flat-module-key resolution so intra-module refs populate
- [ ] `change_locality` (metric, not dimension): diagnose n/a while `change_coupling`/`change_amplification` compute from the same git history (yazi); fix or document with a precise reason string
- [ ] detection+warning for scip-python vs Python 3.14 gap (65-byte empty index) — emit an actionable coverage note (no version pin in code)
- [ ] write tests where a fixture exists; document tool-version gaps with reason strings otherwise
- [ ] run `go test ./internal/...` — must pass before Task 14
- ➕ split this task during execution if any sub-item proves large

### Task 14: LLM fidelity — distance honesty + strength-claim verification

- [ ] `cmd/archfit/explain.go:99-103`: render `distance_basis` (code_structure vs ownership) and append a `(degenerate_owner_map)` qualifier to the distance label when the code-structure fallback was used, so single-owner repos are never framed as cross-team (note: P1 already removes the false `DiffOwner`, so this is the belt-and-suspenders fix)
- [ ] `internal/llm` / `cmd/archfit/review.go:266-349` `postVerify`: cross-check narrative strength claims (intrusive/functional/model/contract) against actual finding strengths; drop/flag unsupported claims (closes herdr "intrusive" hallucination)
- [ ] flag config-vs-LLM-suggestion conflicts (e.g. review suggests `core` where config says `supporting`) instead of silently emitting
- [ ] write test: `postVerify` drops an injected unsupported "intrusive" claim; counts it in the dropped tally
- [ ] write test: explain output includes `distance_basis` and the degenerate-owner qualifier on a flat-name finding
- [ ] run `go test ./internal/llm/... ./cmd/...` — must pass before Task 15

### Task 15: P10 — owner inheritance for auto-registered submodules

- [ ] `internal/classify/classify.go:148` (`AugmentModulesFromGraph`) and `:197` (`AugmentGoWorkspaceModules`): propagate `owner` from the nearest config-declared ancestor module to each synthetic module
- [ ] write test: a cargo-modules submodule under a crate with `owner: X` inherits `X` → inter-submodule edges classify `cross_module_same_owner`, not `different_owner` (reproduces herdr fix)
- [ ] `make build && make archfit`; restamp baseline only if self-config is affected (likely not)
- [ ] run `go test ./internal/classify/...` — must pass before Task 16

### Task 16: P9 — delta "no baseline found" warning

- [ ] `cmd/archfit/check.go` after `baseline.Load` (`check.go:70`): when `--base` is given and the baseline file is absent/empty (`baseline/baseline.go:70-78`), print to stderr "no baseline found at <ref> — all N findings are untracked; run `archfit baseline` to enable drift detection"
- [ ] write test: missing-baseline delta run emits the warning and still exits on real verdict only
- [ ] run `go test ./cmd/...` — must pass before Task 17

### Task 17: P11 — observed-layer vs role divergence finding (new rule)

- [ ] add rule type `layer_role_divergence` (`internal/rules/rules.go:73`): compute each module's observed topological rank from the import DAG, compare to the rank implied by its declared `role`/`layer`, emit a `warn` finding when the delta exceeds a threshold (default 3)
- [ ] reuse `layerRank` (`rules.go:179-186`) / `ModuleMap.LayerFor` (`config/modules.go:154-163`); add config knobs (enabled, threshold) parsed into the rule view
- [ ] write test: a config module placed at a high observed layer (yazi `yazi-config` at rank 11) emits the finding; an aligned module does not
- [ ] `make build && make archfit`; restamp baseline if self-config triggers it
- [ ] run `go test ./internal/rules/...` + `make archfit` — must pass before Task 18

### Task 18: P8 — native `archfit diff <ref>` scorecard comparison

- [ ] add `DiffCmd` (`cmd/archfit/diff.go`): `archfit diff <base-ref> [--config --root --format text|json|markdown]`; create a clean detached worktree at `<base-ref>` in an `os.MkdirTemp` dir, run `score` on base + HEAD (both via the canonical-path resolution from Task 2), emit a structured before/after metric/dimension delta table, then remove the worktree (deferred cleanup even on error)
- [ ] handle monorepo subtree via `--root`; non-git or missing ref → clear exit-3 error; never mutate the user's working tree
- [ ] write test: diff between two fixture commit states emits a delta table and cleans up the worktree
- [ ] write test: non-git root and bad ref produce graceful errors
- [ ] run `go test ./cmd/...` — must pass before Task 19

### Task 19: Author ideal per-repo configs (all tools + opt-in metrics)

- [ ] for each repo (spotinfo, pumba, ccgram, codegraph, herdr, yazi, omni/scheduled-tasks) write/refresh `.archfit.yaml` enabling `tools.scip`, `tools.cargo-modules` (Rust), `tools.complexity`(+backend), `tools.clones`, `tools.syntax.enabled`, `metrics.risk_hub`, `metrics.functional_candidates`, `volatility_cascade_enabled`; correct modules/layers/roles/owners; service-scoped config for omni; save under `reports/eval-2026-06-28/ideal-configs/`
- [ ] run `archfit doctor` on the host; record which analyzers are present (gate the revalidation acceptance to available tools)
- [ ] verify each config parses: `archfit score --config <cfg> --root <repo>` exits cleanly
- [ ] run the parse checks — must pass before Task 20

### Task 20: Held-constant revalidation (binary-only controlled experiment)

- [ ] rebuild: `make build`
- [ ] re-run the FIXED binary on the eval's EXACT inputs — the SAME configs and the SAME (lowercase `/Users/alexei/workspace/...`) `--root` paths used in the eval — for every repo: `score`, `check --full --advisory --format json`, `check --base <old-ref> --advisory --format json`; capture under `reports/eval-2026-06-28/revalidation-controlled/`
- [ ] diff each output against the original eval JSON still on disk in `reports/eval-2026-06-28/` — only the binary changed, so every delta is attributable to a specific code fix. Keeping the lowercase path keeps the **P2 trigger LIVE**, so `hidden_coupling` recovering from 0 is real proof, not a vacuous canonical-path run
- [ ] record the per-metric binary-only deltas; this controlled diff — NOT the subagents' estimates — is the acceptance ground truth
- [ ] (system run; no unit test) — proceed to Task 21

### Task 21: Ideal-config best-achievable showcase (full + delta + LLM)

- [ ] per repo with the Task-19 ideal config + rebuilt binary, run and capture under `reports/eval-2026-06-28/revalidation-ideal/`: `scan`, `score`, `check --full`, `check --base <old-ref>` (delta), `review` (LLM, key from `.env`), `explain <top-fp> [--llm]`
- [ ] true version diff via `archfit diff <old-ref>` (Task 18) for single-repo targets; omni stays delta-only
- [ ] reuse a workflow/parallel harness; heavy Rust/omni throttled (≤3-4 concurrent) to avoid per-analyzer timeouts
- [ ] keep this SEPARATE from the controlled diff so the two variables (code vs config) never blur — this run shows the achievable ceiling with all tools/opt-in metrics on
- [ ] (system run; no unit test) — proceed to Task 22

### Task 22: Acceptance — verify each fix against the controlled diff

- [ ] write `reports/eval-2026-06-28/REVALIDATION.md`: a before→after table + pass/fail per fix, anchored to the Task-20 **binary-only deltas** (not the report's estimates):
      P2 — herdr `hidden_coupling` recovers from 0 (nonzero) and `structural_weight` from n/a on the SAME lowercase path ·
      P1 — codegraph/spotinfo `coupling_balance` rises (flat-name edges no longer `DiffOwner`) ·
      P3 — `explain` severity == `check` severity; S=9 edges band per the book formula ·
      Tasks 6-9 — pumba `panic_density` production ≈0 (+excluded count), `functional_candidates` drops test/mock pairs, spotinfo `inbound_module_fanin` 2 not 3 ·
      P6 — ccgram empty SCIP → `warn` · P10 — herdr submodule edges same-owner · omni `explain --root` scopes to the service · P8 — `archfit diff` emits a real version delta
- [ ] for any fix whose controlled delta does NOT match its prediction, open a ➕ follow-up task with the diagnosis (do not hand-wave a pass)
- [ ] cross-check the ideal-config run (Task 21) as the achievable ceiling; note any metric still n/a due to a genuine tool/version gap
- [ ] run `go test ./...` — suite still green after revalidation work

### Task 23: Verify acceptance criteria (suite, lint, dogfood, determinism)

- [ ] `make all` (fmt → lint → test → archfit) green
- [ ] `go test ./internal/ -run TestArchImports` and `go test ./internal/engine/ -run TestGolden` pass
- [ ] `.archfit-baseline.json` reflects intended scoring changes only (diff reviewed)
- [ ] coverage meets project standard; `make lint` clean

### Task 24: Documentation

- [ ] update `docs/guide` (languages/metrics: file-class handling & segregated test/generated counts; tools opt-in; new `archfit diff`); update `cmd` help text
- [ ] update root `CLAUDE.md` invariants if any changed (severity source, file-class, diff subcommand)
- [ ] append a "fixes → findings" cross-reference to `reports/eval-2026-06-28/00-FINDINGS.md` linking each P-item to its task/commit

## Technical Details

**Mock/test/generated handling (the metric-correctness core).** A single `FileClass`
facility (`Production | Test | Generated | Vendor`) is the source of truth, computed once
during the `loc` walk and stamped on `FileFacts`. Auto-detection: generated-code header
sniff (`// Code generated … DO NOT EDIT`, the moq marker), generated filename patterns
(`*.pb.go`, `*_gen.*`, `_pb2.py`), mock patterns (`mock_*.go`, `*_mock.go`, `mocks/`), and
per-language test conventions (extending `syntax.IsTestFile`). A config view
(`tools.file_class`) lets users add patterns for custom mock frameworks and fine-tune.
**Policy = segregate, not hide:** production-health metrics (`panic_density`,
`functional_candidates`, `structural_weight`, `complexity`) exclude `Test`+`Generated`
from the score but report the excluded count as evidence; `test_density` deliberately keeps
tests. This fixes pumba (panic 208→~0, clones 13→1) without erasing legitimate test signal.

**Severity unification.** `BookScorer.Score().Balance → ScoreBand()` becomes the single
severity source (`classify.go:326`), replacing the discrete `BalanceResult` quadrant table.
This is the "adopt the book formula verbatim" principle applied to severity, and it makes
`check` and `explain` agree. Because it changes findings in both directions, the dogfood
baseline is restamped in Task 4.

**Distance correctness (P1).** In the degenerate-owner path the honest classification of
two same-owner modules is `cross_module_same_owner` (ordinal 4), never `different_owner`
(7). The change is one return value plus the comment; it removes the false "tight coupling"
on every flat-named single-team Go/TS repo and the downstream LLM "cross-team" hallucination.

**Path canonicalization (P2).** Normalize the scan root once at `resolveScanRoot` via
`EvalSymlinks` (with `gitRoot` likewise) so `subtreePrefix`, SCIP `WorkDir/--cwd`, and
`crateRoots` all receive a path whose case/symlinks match `git`/`cargo` output. Linux CI is
unaffected (case-sensitive); the fix targets the macOS developer loop.

## Post-Completion

**Maintainer actions (no checkboxes):**

- The six target repos' `.archfit.yaml` were aligned during the eval (untracked) and ideal
  configs are authored in Task 19 — decide which to commit into each target repo vs keep
  under `reports/eval-2026-06-28/ideal-configs/`.
- Release: tag `vX.Y.Z` after merge to trigger `release.yaml` (never `gh release create` manually).
- LLM revalidation (Task 20) consumes Anthropic credits via `.env`; run when ready to spend.
- Re-confirm the macOS path-case fix on an actual case-variant checkout (CI cannot reproduce it).
