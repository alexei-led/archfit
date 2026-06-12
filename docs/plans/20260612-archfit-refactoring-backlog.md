# archfit post-Tranche-2 — refactoring backlog + hardening + multi-repo validation

## Overview

Close the architecture refactoring backlog on archfit itself, derived from the
three-way review (`docs/plans/notes/three-way-review-comparison.md` §4):
ports extraction (F-01), engine↔metrics un-capture (M1), status→baseline
inversion (M2), `engine.Run` options struct (F-03), `cmd/archfit/main.go`
split + collector relocation (F-05, M6), volatility ordering contract (M7),
scope decoupling (F-02). Then the frontier-model enrich validation (Task 8)
and a full 4-repo validation sweep.

Every task is behavior-preserving unless stated. Determinism is the product:
double runs stay byte-identical throughout; the golden double-run test and the
CLI determinism tests stay green after every task.

## Context (from discovery)

- `engine.Run` takes 16 positional params (`internal/engine/engine.go:54`);
  lizard reports it as the top CCN function in self-scan.
- Port types live in `internal/engine/ports.go`; `extract/astgrep` and
  `extract/scip` import `internal/engine` (F-01). moq mocks live in engine
  (`extractor_moq.go`, `pattern_provider_moq.go`, `symbol_resolver_moq.go`).
  moq binary is NOT installed — mocks are moved by hand (package rename only);
  `//go:generate` directives stay accurate.
- `rules.PatternMatch{File,Line,Match}` duplicates `engine.PatternMatch`
  (6 fields); `engine.toRulesPatternMatches` silently drops fields (M4).
  All `rules.Rule.Check` impls currently ignore Evidence.
- **Findings fingerprint at FILE granularity and all rule findings are
  kind="gate"** (verified empirically: the 5 baselined `layer_inversion`
  findings are `internal/engine/{engine,ports,*_moq}.go → internal/scope`).
  Consequence: a `ports` package that imports `scope` (layer adapter, rank 4)
  must NOT be mapped to a module below rank 4, or new gate findings appear.
  Resolution: `internal/ports` stays UNMAPPED in `.archfit.yaml` (consistent
  with the existing practice for facts/staleness/labels/agenttask), and the
  scope-layer wart is fixed for real in Task 7.
- `ports` cannot be stdlib-clean (interfaces reference `config.PatternConfig`
  and `scope.Scope`) → per the decision rule, the unified `PatternMatch` value
  type goes to a new model package `internal/model/pattern` (stdlib-only),
  imported by both ports and rules.
- `MetricInput` references `fitness.Signals`; `internal/fitness` imports `os`
  (filesystem scan) so it cannot join the model ring. Resolution: the
  `Signals`/`EvidenceMap` value types move into the new `internal/model/signal`
  package; `fitness.Detect` (the scanner) stays in `internal/fitness` and
  returns `signal.Signals`.
- `status.Assign` uses exactly `base.HasFingerprint(id)` + iteration over
  `base.Accepted{Fingerprint,RuleID,Kind}` — that shapes the `AcceptedSet`
  interface. `engine` must ALSO drop its `baseline` import in Task 3, or
  reclassifying baseline→adapter creates a new engine(3)→baseline(4)
  inversion.
- `config.ApplyVolatility` mutates `ModuleDef.Volatility` in place for modules
  without explicit volatility/subdomain — that is WHY `metrics.New` must run
  first today (M7).
- Complexity collector emits no coverage record today; LOC's record is built
  in cmd. Validation repos all present: `~/Workspace/{ccgram,pumba,codegraph}`.
- `ANTHROPIC_API_KEY` is NOT set in this environment — Task 8 blocks on the
  operator; do not skip silently.
- Last `git push` failed exit 128; remote is `git@github.com:alexei-led/archfit.git`
  (SSH), `gh auth status` OK — diagnose SSH connectivity before the final push.

## Development Approach

- Feature branch `refactor/architecture-backlog`; one commit per task; merge
  to main at the end.
- Per task: `go test -count=1 ./...` green + `make lint` 0 issues before the
  next task; `gofmt` after edits.
- goconst (3+ repeated literals incl. tests) and gocyclo (threshold 30) are
  active — extract constants proactively in new/edited test files.
- `check` stays LLM-free; arch ring tests are only ever EXTENDED.
- Self-scan checkpoints: after Tasks 1, 3, 4, 5, 7 run
  `./.bin/archfit check --full --advisory --report` and confirm no new gate
  findings.

## Testing Strategy

- Task 1: arch test gains (a) `internal/engine` in the core-forbidden list and
  (b) a NEW `adapters_no_engine_import` subtest over the real adapter prefixes
  (toolrun, extract/, history/, output/) — added in the SAME commit as the
  move, proving it would have caught the old extract→engine imports.
- Task 2: arch test `modelPkgs` += `internal/model/signal`, `internal/model/pattern`.
- Task 3: status tests run against a tiny in-memory `AcceptedSet` fake, not
  `baseline.Baseline`; `internal/baseline` joins `adapterPrefixes`.
- Task 4: byte-identical self-scan JSON before/after the refactor (determinism
  proof), `Run` CCN < 28 via self-scan complexity metric.
- Task 5: new packages get table-driven tests (loc: fixture walk; complexity:
  RunnerMock with canned/malformed lizard CSV, ported from
  `complexity_parse_test.go`); CLI tests green unchanged.
- Task 6: a test that calls `ApplyVolatility` BEFORE `metrics.New` and proves
  risk_hub still sees explicit-config volatility only.
- Task 7: scope tests use a fake resolver (no git); CLI `--base` delta test green.
- Task 8 + runbook: see Validation tasks below; results recorded in
  `docs/plans/notes/refactoring-backlog-validation.md`.

## Progress Tracking

- Mark completed items `[x]` immediately when done.
- New tasks: `+` prefix. Blockers: `WARN` prefix. Deviations recorded inline.

## Implementation Steps

### Task 1: extract port types to `internal/ports` + unified PatternMatch (F-01, M4)

**Files:** create `internal/ports/ports.go` (+moved moq files),
`internal/model/pattern/pattern.go` (+test); modify `internal/engine/engine.go`,
`internal/engine/*_test.go`, `internal/rules/rules.go` (+test),
`internal/extract/astgrep`, `internal/extract/scip`, `cmd/archfit/*.go`,
`internal/arch_test.go`, `.archfit.yaml`.

- [x] `internal/model/pattern`: unified `pattern.Match` (File, Pattern, Text, Node, Line, Column); stdlib-only
- [x] `internal/ports`: Extractor, PatternProvider, SymbolResolver, Renderer, Nop impls moved from engine; moq mocks moved (package rename; go:generate directives updated); clean move — no aliases left in engine
- [x] `rules.Evidence` uses `[]pattern.Match`; `rules.PatternMatch` deleted; `engine.toRulesPatternMatches` deleted
- [x] arch test: core-forbidden list += `internal/engine`; new `adapters_no_engine_import` subtest (toolrun, extract/, history/, output/ must not import engine); modelPkgs += `internal/model/pattern` — pre-move failure captured (extract/astgrep + extract/scip flagged)
- [x] `.archfit.yaml`: `forbidden_dependency` rule `internal/extract/** → internal/engine/**` (gate: fail)
- [x] tests green + lint 0 + self-scan no new gate findings (verdict pass, 0 gate, double-run byte-identical); commit e8561b5 — DEVIATION: reverted ~290 lines of out-of-scope rule tests a subagent added; Task 1 stays a pure move

### Task 2: un-capture engine from metrics via `internal/model/signal` (M1)

**Files:** create `internal/model/signal/signal.go`; modify `internal/metrics/*`
(aliases), `internal/fitness/fitness.go`, `internal/engine/engine.go`,
`cmd/archfit/*.go`, `internal/arch_test.go`.

- [x] `signal.MetricInput`, `signal.ChangeHistory`, `signal.ComplexityFunc`, `signal.Signals`+`EvidenceMap` (moved from fitness) — stdlib+model only
- [x] `metrics` keeps permanent type aliases (`type MetricInput = signal.MetricInput` etc., the os.FileInfo pattern) so the 13 metric files stay untouched; engine + cmd construct `signal.*` directly — a new MetricInput field then churns only signal + its producer, not metrics
- [x] `fitness.Detect` returns `signal.Signals`; arch_fitness metric reads `signal.Signals`
- [x] arch test: modelPkgs += `internal/model/signal`
- [x] golden test byte-identical; tests green + lint 0; self-scan pass/0 gate, double-run identical; commit 4e3a2d7

### Task 3: invert status→baseline + reclassify baseline layer (M2)

**Files:** modify `internal/status/status.go` (+test), `internal/baseline/baseline.go`
(+test), `internal/engine/engine.go` (+tests), `cmd/archfit/*.go`,
`internal/arch_test.go`, `.archfit.yaml`.

- [x] `status.AcceptedEntry` + `status.AcceptedSet{HasFingerprint, Entries}` shaped from Assign's actual usage; `Assign` takes `AcceptedSet`
- [x] `baseline.Baseline.Entries()` satisfies the interface (baseline imports status — outward-in, legal)
- [x] engine drops the baseline import: Run takes `accepted status.AcceptedSet` + `baseMetrics diagnostic.MetricSnapshot` (prevents a new engine→baseline inversion on reclassify)
- [x] status tests use an in-memory fake (`fakeAccepted`); engine tests green
- [x] `.archfit.yaml`: baseline layer support→adapter; arch test: `adapterPrefixes` += `internal/baseline`; self-scan: no new layer_inversion (verdict pass, 0 gate; 4 of 5 old engine→scope inversions now report fixed — their from-files moved to ports in Task 1)
- [x] tests green + lint 0; double-run identical; commit

### Task 4: `engine.Run` options struct + CCN reduction (F-03)

**Files:** modify `internal/engine/engine.go`, `internal/engine/engine_test.go`,
`internal/engine/golden_test.go`, `cmd/archfit/*.go` (runPipeline).

- [x] Shape: `engine.Run(ctx, RunInput{...})` — ctx stays positional (Go convention), everything else in the struct; stage helpers split out of Run's body (`extract`, `resolveEvidence`, `collectAdvisories`)
- [x] capture self-scan JSON BEFORE, re-run AFTER — DEVIATION: literal byte-identity is impossible for a self-referential scan (the refactor changes engine.go's own LOC/CCN facts). Verified instead: finding IDs+statuses identical, verdict pass→pass, summary identical; only self-measurement values moved (hidden_coupling 46→45 — caused by the Task 3 COMMIT entering co-change history, not by this refactor). Double-run byte-identical holds.
- [x] self-scan complexity: `Run` CCN 28→17; new helpers peak at 7; repo max is facts.Build at 26 < 30 gocyclo threshold
- [x] tests green + lint 0; commit

### Task 5: split `cmd/archfit/main.go` + collectors into extract (F-05, M6)

**Files:** create `cmd/archfit/{check,baseline,explain,doctor,install,init,pipeline}.go`,
`internal/extract/loc/` (+test), `internal/extract/complexity/` (+test);
delete `cmd/archfit/{sourceloc,complexity}.go`; modify main.go, enrich.go.

- [ ] main.go keeps `main`, `Run`, `cli`, `appDeps`, `exitError` (+versionFlag/exitCode); commands split by concern; Scan lives with check.go; no behavior change
- [ ] `loc.Run(root) (map[string]int, diagnostic.Coverage, error)` — coverage record moves inside (was built in cmd)
- [ ] `complexity.Run(ctx, runner, root) ([]signal.ComplexityFunc, diagnostic.Coverage, error)` — clones-style coverage with zero file counts (status only) so the coverage metric value does not shift
- [ ] table-driven tests per Testing Strategy; CLI tests green unchanged
- [ ] structural_weight self-scan: cmd/archfit below god-module threshold (record the number)
- [ ] tests green + lint 0; commit

### Task 6: kill the `metrics.New`/`ApplyVolatility` ordering contract (M7)

**Files:** modify `internal/config/volatility.go` (+test),
`internal/config/config.go`, `internal/metrics/metrics.go` (+test), cmd callers.

- [ ] structural fix: `ApplyVolatility` STOPS mutating `ModuleDef.Volatility`; derived values land in a separate store on Config; effective volatility = explicit-else-derived; existing consumers (ForClassify path) read effective — planned DEVIATION from the brief's suggested `metrics.New(cfg, vol)` signature: two-store is stronger (no caller can get it wrong) and keeps `metrics.New(cfg)` stable. `newRiskHubMetric` keeps reading pristine `def.Volatility` — provably explicit-only regardless of call order
- [ ] wrong-order test: ApplyVolatility before metrics.New → risk_hub unchanged (explicit-only)
- [ ] tests green + lint 0; commit

### Task 7: scope decoupling + scope layer reclassification (F-02)

**Files:** modify `internal/scope/scope.go` (+test), `cmd/archfit/pipeline.go`,
`.archfit.yaml`.

- [ ] `scope.Resolve` takes an injected resolver (RepoRoot/HeadRef/Changed); concrete git calls move to cmd next to `git.History`; scope drops `history/git` + `toolrun` imports
- [ ] scope tests use a fake resolver; git-only fixtures removed if any exist only for git; CLI `--base` delta test green
- [ ] `.archfit.yaml`: scope layer adapter→support (honest once scope is config+value types only); the 5 baselined engine→scope `layer_inversion` findings become fixed; stale "Phase 2: move Scope to model" comment updated; re-baseline to drop the 5 stale entries
- [ ] self-scan: no new findings; tests green + lint 0; commit

### Task 8: frontier-model enrich + approve flow on ccgram

**WARN: blocked on `ANTHROPIC_API_KEY` (not set here) — ask the operator; do
not skip silently.**

- [ ] flip ccgram `tools.llm` → anthropic/claude-opus-4-8; `archfit enrich`; diff drafts vs the local-model run recorded in `notes/tranche2-enrich-validation.md` (56 drafts)
- [ ] approve 2–3 clearly-correct labels (e.g. `session_state→window_state: model`); run `check`; verify classification changes (BC advisories shift)
- [ ] byte-identical double run with labels present; touch a file in the pair → `labels/stale` advisory fires on next full run
- [ ] restore ccgram config afterwards; append results to `notes/tranche2-enrich-validation.md`; commit

### Task 9: validation runbook (gate for the whole plan)

**Files:** create `docs/plans/notes/refactoring-backlog-validation.md`.

- [ ] build `.bin/archfit` from the final branch
- [ ] archfit/ccgram/pumba/codegraph × {double-run byte-identical + exit 0 + no stderr panics; SARIF schema-validated via `uvx check-jsonschema` against sarif-2.1.0; baseline→check parity (verdict unchanged, zero phantom deltas)}
- [ ] archfit: verdict pass; arch tests green; Run CCN reduced (Task 4); cmd structural_weight reduced (Task 5); facts ≈ 90+ modules
- [ ] ccgram: 383+ facts; 3 gate findings → 3 agent_tasks with correct validation commands; labels-file checks from Task 8
- [ ] pumba: scip-go temporarily on → facts populate; record count; restore config after (no borrowed-config edits left behind)
- [ ] codegraph: scip-typescript; if the TS indexer path fails, record a coverage-gap finding, not a silent skip
- [ ] archfit delta mode: `check --base main~5` → change_locality sane non-n/a value
- [ ] full suite green, lint 0, push (diagnose exit-128 remote/auth first: `git remote -v`, `gh auth status`, `ssh -T git@github.com`)

## Post-Completion

- Move this plan to `docs/plans/completed/`; mark §4 backlog items closed in
  `three-way-review-comparison.md`; update project memory.
- Still deferred (unchanged): MCP server, GitHub Action wrapper, plugin
  protocol, `map_staleness`, LLM subdomain/volatility drafting.
