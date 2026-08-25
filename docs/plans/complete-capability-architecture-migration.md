# Plan: Complete Archfit Capability Architecture Migration

> **Executable Ralphex plan.** Run from the repository root with:
>
> ```sh
> ralphex docs/plans/complete-capability-architecture-migration.md
> ```
>
> Ralphex executes each `### Task N:` section sequentially. Each task is an
> independently committable migration horizon and must finish green before the
> next task starts. Checkboxes appear only in executable task sections.

## Overview

Complete the capability-boundary refactor on PR #33 and leave Archfit as a
maintainable modular monolith with:

- one authoritative Architecture Policy model;
- explicit Application-owned `Prepare -> Acquire -> Relate -> Assess -> Project`
  use-case sequencing;
- neutral evidence contracts separated from per-run context;
- adjacent, strongly cohesive Relationship Analysis and Assessment/Repair
  modules with one narrow immutable integration contract;
- stable report DTOs projected at the application/output boundary;
- concrete adapters wired only at the CLI composition root;
- no `engine`, `analysispipeline`, `internal/view`, compatibility façade, or
  shared pipeline-state domain;
- behavior-focused tests colocated with the capability that owns the behavior;
- a self-consistent `.archfit.yaml`, reviewed deterministic labels, executable
  architecture gates, and an explicit post-migration baseline.

This is a consolidation plan, not another package-extraction exercise. New
interfaces are allowed only at context, I/O, persistence, provider, or process
boundaries. Within one capability, prefer concrete functions and types.

## Source artifacts

Approved target design:

- `docs/design/20260823-archfit-capability-map.md`
  - bounded contexts: Architecture Policy, Architecture Analysis, Evidence
    Acquisition, Analysis Application, Report Projection, Persistence, Provider
    Integration, and CLI Composition;
  - decisions D1-D5: modular monolith, capability ownership, no scan/pipeline
    domain, assessment/report separation, and honest volatility labels;
  - integration contracts and twelve architecture-fitness requirements;
  - self-review risks around broad snapshots, interface frameworks, strong
    Relationship/Assessment coupling, compatibility aliases, and migration size.
- `docs/design/relationship-assessment-contract.md` — immutable relationship set
  is the Assessment boundary; raw graph ownership remains upstream.
- `docs/design/model-contract-deletion.md` — deleted scan/diagnostic/ports
  compatibility contracts; `internal/view` is explicit unfinished migration
  debt.
- `docs/plans/20260823-archfit-capability-refactor.md` — previous migration
  horizons and behavior-preservation rule.

Supporting review evidence, measured against `main` at `df312e5` and PR source at
`509d50b`:

- **E1 — local complexity improved, global complexity increased.** Production
  files 211 -> 259, LoC 40,722 -> 44,577, packages 63 -> 76, internal dependency
  edges 224 -> 300, exported internal declarations 677 -> 1,061, interfaces
  17 -> 43. Functions over 150 lines fell 4 -> 2 and high-cognitive function
  density fell 8.6% -> 7.9%.
- **E2 — orchestration was redistributed, not reduced.** Main
  `cmd/archfit + internal/engine` is 11,631 LoC; PR `cmd/archfit +
  internal/application + internal/analysispipeline` is 11,653 LoC.
  `internal/analysispipeline` has 27 files, 4,145 LoC, 166 functions, 104
  exported declarations, and 35-package fan-out.
- **E3 — new owner seams lack direct behavioral coverage.** Direct package
  coverage is 0% for `internal/assessment/evaluation`, 33.3% for
  `internal/relationship/analysis`, 37.8% for `internal/analysispipeline`, and
  72.1% for `internal/application`; main's former engine had 93.2% direct
  coverage. End-to-end and golden tests pass but do not replace owner-local
  behavior tests.
- **E4 — ownership acquisition is repeated.** `resolveOwners` is called from
  `internal/analysispipeline/stage_prepare.go` and twice from
  `internal/analysispipeline/analyzer.go`; one run can repeat repository-history
  work and produce stage-local policy copies.
- **E5 — contracts remain broad or transitional.** `internal/evidence.Snapshot`
  has 23 fields and mixes neutral facts with scope, labels, config/time, and
  bundle/deployment context. `relationship.AnalysisResult` includes report-facing
  appendices. `policy.RelationshipPolicy.ClassifyConfig()` adapts back to
  `internal/view`; `internal/view` still has production consumers.
- **E6 — dependency gravity remains.** Main's diagnostic contract had 30 direct
  and 31 transitive importers. PR relationship contracts have 15 direct and 32
  transitive importers; evidence and view remain high-fan-in shared models.
- **E7 — the self-model is transitional.** `.archfit.yaml` still declares an
  `engine` layer and transitional `evidence-stage-contracts`, `stage-views`, and
  pipeline-oriented modules. Labels and module metadata must be reviewed only
  after physical boundaries settle; scores are never targets.
- **E8 — behavior compatibility is mandatory.** JSON schema
  `archfit.diagnostic.v2`, Markdown, SARIF, scorecard, console output, finding
  IDs/kinds/statuses, gate verdicts, exit codes, ordering, baseline compatibility,
  config compare, enrich schemas, and `--base` behavior must remain compatible
  unless a separate approved schema decision is recorded.

## Target architecture

Dependency direction:

```text
CLI Composition
    |
    v
Analysis Application ---------------> Report Contract
    |                                      ^
    +--> Architecture Policy              | projection only
    +--> Evidence Acquisition             |
    +--> Relationship Analysis ---------->+
    +--> Assessment and Repair ---------->+

Concrete Evidence / Persistence / Provider / Report adapters point inward
through owner-defined ports. Core contexts never import adapters or CLI code.
```

Approved module responsibilities:

- **Architecture Policy:** topology, module/layer semantics, ownership, deploy
  units, gates, rule definitions, waivers, approved labels, and validation.
- **Evidence Acquisition:** source/tool observations and coverage. It does not
  decide relationships, findings, scores, or verdicts.
- **Relationship Analysis:** strength, distance, volatility, connascence,
  provenance, relationship scoring, and relationship advisories.
- **Assessment and Repair:** policy evaluation, metrics, findings, statuses,
  scorecard, verdict, deltas, recommendations, and repair tasks.
- **Analysis Application:** use-case lifecycle, ordering, cancellation,
  comparison coordination, and projection orchestration.
- **Report Contract/Adapters:** stable external DTOs and rendering only.
- **CLI Composition:** flags, concrete construction, rendering selection, and
  exit translation only.

Balanced-coupling rules:

- Relationship and Assessment remain adjacent modules in one core bounded
  context. Their strong functional/model coupling is intentional and must not be
  hidden by artificial distance or an event/service boundary.
- Broad models may not cross context boundaries. Pass the smallest contract that
  carries knowledge the consumer actually needs.
- Composition-root fan-out is acceptable when it is wiring-only. Domain behavior
  in a high-fan-out package is not.
- Do not lower volatility, widen distance, split modules, add labels, or add
  waivers to improve a score. Module metadata must describe real ownership,
  deployment, domain role, and change pressure.

## Success criteria

- `internal/engine`, production `internal/analysispipeline`, and
  `internal/view` are absent.
- Application owns the exact stage order and receives explicit immutable stage
  values. No hidden analyzer state or repeated acquisition exists.
- Repository ownership/topology enrichment executes once per run and the same
  immutable `PolicySnapshot` reaches Relationship and Assessment.
- Only Relationship Analysis receives the full neutral evidence snapshot.
  Application may pass it without inspecting it; Assessment and renderers cannot
  import it or raw graph types.
- Policy owns domain semantics. `internal/config` owns YAML/schema lifecycle and
  maps to/from policy contracts without policy importing config or view DTOs.
- Relationship Analysis exposes a minimal relationship contract and optional
  analysis evidence separately. Assessment consumes only the relationship
  contract and explicit assessment signals.
- Assessment and Relationship import no report DTOs. One pure Application-owned
  projector builds `report.Document`; renderers consume only the report contract.
- CLI imports no rule, metric, classifier, scorer, status, decision, or finding
  implementation package. Concrete wiring is obvious and behavior-free.
- Every exported declaration in the migrated capability packages has a production
  consumer outside its package or is an approved stable contract. Single-use
  interfaces inside one capability are removed.
- Direct behavior tests cover the decision branches in Relationship Analysis and
  Assessment/Repair. Coverage is used as a gap signal, not a gaming target;
  expected direct statement coverage is at least 75% for Relationship Analysis
  and 80% for Assessment evaluation.
- All existing output, finding, baseline, config, enrich, comparison, and exit
  contracts remain green and deterministic.
- `.archfit.yaml` exactly models the final physical capabilities, contains no
  dead path/module/rule, has no uncovered production path, and enforces every
  approved dependency direction.
- `.archfit-labels.yaml` contains no stale, duplicate, unresolvable, speculative,
  or score-tuning entry. Every retained label maps to a real classified edge and
  has explicit provenance and confidence.
- Package and module cycles remain zero. No new architecture waiver is required.
- Architecture docs describe implemented source paths and contracts, not intended
  future state.
- A new baseline is written only after all fail gates pass, the architecture
  re-review returns GO, and every accepted remaining advisory has written
  rationale.

## Validation Commands

Run the focused commands listed per task first. Run this whole-plan suite after
all tasks and before every push:

```sh
make fmt
make lint
go vet ./...
make test
make build
make archfit
git diff --check

go test ./internal/ -run 'TestArchImports|TestModelSurfaceNoDrift'
go test ./internal/application/... ./internal/evidence/... \
  ./internal/relationship/... ./internal/assessment/... -count=1
bash scripts/tests/cli_exit_contract_test.sh
python3 internal/extract/scip/scip_reader_test.py

.bin/archfit check --config .archfit.yaml --root "$PWD"
.bin/archfit analyze --full --json --config .archfit.yaml --root "$PWD" \
  > /tmp/archfit-capability-final.json
.bin/archfit analyze --full --json --base main --config .archfit.yaml \
  --root "$PWD" > /tmp/archfit-capability-vs-main.json
```

GitNexus must match the current commit before final impact evidence:

```sh
node .gitnexus/run.cjs analyze
```

Then run the Pi GitNexus tools:

- `gitnexus_detect_changes(scope="compare", base_ref="main", repo="archfit")`
- `gitnexus_detect_changes(scope="all", repo="archfit")`

## Implementation Steps

### Task 1: Lock behavioral and architectural safety nets

Justification: E1, E3, E8; target design fitness requirements 8-10. The migration
cannot safely move ownership while the new Relationship and Assessment seams have
weak direct tests.

Files:

- `internal/assessment/evaluation/evaluation_test.go` — table-driven direct tests
  for findings, waivers, status, advisory filtering/promotion, verdict, metrics,
  delta, repair tasks, and failure/abstention paths.
- `internal/relationship/analysis/analysis_test.go` — direct tests for
  classification, pinned-label precedence, provenance, clone-only relationships,
  runtime/connascence evidence, stale labels, and abstention.
- `internal/application/analysis_test.go` — exact stage order, cancellation,
  error short-circuit, immutable handoffs, and one-use stage values.
- `internal/analysispipeline/pipeline_owner_test.go` — temporary characterization
  for current technical behavior; delete or relocate it with Task 4.
- `cmd/archfit/analysis_characterization_test.go`,
  `cmd/archfit/pipeline_equivalence_test.go`, and
  `cmd/archfit/analyze_exit_test.go` — preserve CLI behavior and refresh/progress
  wiring.
- `internal/output/{jsonout,markdown,sarif,scorecard}/*_test.go`,
  `internal/testdata/golden.json`, and `scripts/tests/cli_exit_contract_test.sh`
  — byte/stable-schema and exit-code safety net.
- `internal/arch_test.go` and `internal/testdata/model_surface.golden` — temporary
  non-increasing import/surface ratchets for `analysispipeline`, `view`, evidence,
  relationship, and assessment contracts.
- `docs/plans/complete-capability-architecture-migration.md` — record measured
  pre-migration evidence without marking target conditions complete.

Preconditions: branch is clean; current full tests and golden fixtures pass;
GitNexus index source commit is recorded.

Postconditions: current observable behavior is characterized at owner seams;
architecture tests fail on any increase in transitional imports or contract
surface; no production behavior changes.

Fitness gate: existing gates stay green. Add temporary ratchets that fail if
production imports of `internal/view` or `internal/analysispipeline`, evidence
snapshot consumers, or exported transitional contract surface increase.

Impact commands:

- `gitnexus_impact(target="Evaluate", direction="upstream", depth=3, include_tests=true, repo="archfit")`
- `gitnexus_impact(target="Analyze", direction="upstream", depth=3, include_tests=true, repo="archfit")`
- `gitnexus_impact(target="StageExecutor.Execute", direction="upstream", depth=3, include_tests=true, repo="archfit")`
- `gitnexus_detect_changes(scope="all", repo="archfit")`
- Fallback: `git diff --name-only HEAD && go list -deps ./... >/tmp/archfit-deps-before.txt`.

Verification commands:

```sh
go test ./internal/assessment/evaluation -count=1 -cover
go test ./internal/relationship/analysis -count=1 -cover
go test ./internal/application -count=1 -cover
go test ./cmd/archfit -run 'TestAnalysis|TestAnalyze|TestPipeline|TestGolden' -count=1
go test ./internal/output/... -count=1
bash scripts/tests/cli_exit_contract_test.sh
make test-fast
make archfit
```

Manual checks:

- Review assertions for behavior, not private call order except the approved
  Application stage lifecycle.
- Confirm golden updates are absent unless they expose an already-approved
  compatibility correction.
- Confirm coverage increases through meaningful branches, not line-only tests.

- [x] Record the current GitNexus source commit, import counts, exported surface,
      direct coverage, and golden hashes in the task result.
- [x] Add table-driven Relationship Analysis tests for valid, boundary, failure,
      abstention, and label-precedence cases.
- [x] Add table-driven Assessment evaluation tests for valid, boundary, failure,
      waiver, gate, advisory, delta, verdict, and repair cases.
- [x] Strengthen Application sequencing and CLI/output characterization tests.
- [x] Add non-increasing transitional-import and contract-surface ratchets.
- [x] Run the task verification commands and commit one safety-net change.

#### Task 1 result: measured pre-migration evidence

Measured at `d8ef114` on `refactor/capability-contract-boundaries`.

GitNexus index source commit is `509d50b`; the two commits between it and
`d8ef114` touch only `docs/` and `CLAUDE.md`/`AGENTS.md`, so the index is
current for source and no re-index is required before Task 2.

Transitional production imports (AST-matched by
`TestTransitionalImportRatchet`, the same instrument that now caps them):

| Import path | Production importers | Ratchet cap |
| --- | --- | --- |
| `internal/view` | 33 | 33 |
| `internal/analysispipeline` | 9 | 9 |
| `internal/evidence` | 3 | 3 |

Exported contract surface (top-level exported objects, counted by
`TestTransitionalContractSurfaceRatchet`):

| Package | Exported declarations | Ratchet cap |
| --- | --- | --- |
| `internal/analysispipeline` | 88 | 88 |
| `internal/relationship` | 55 | 55 |
| `internal/assessment/result` | 47 | 47 |
| `internal/view` | 29 | 29 |
| `internal/evidence` | 8 | 8 |
| `internal/policy` | 8 | 8 |

Direct statement coverage at the owner seams:

| Package | Before | After |
| --- | --- | --- |
| `internal/assessment/evaluation` | 0.0% | 94.7% |
| `internal/relationship/analysis` | 33.3% | 83.2% |
| `internal/application` | 72.1% | 72.9% |

Golden and byte-identical fixture hashes (sha256), unchanged by this task:

| Artifact | Hash |
| --- | --- |
| `internal/testdata/model_surface.golden` | `4438a2d333259a594b1c421798edc19e6868b41a6c87b6f602870fc343f93dc5` |
| `internal/analysispipeline/golden_test.go` | `a6afa748a64a1a5c1dc48671aa5bc32687ad9642f35118593ed4f235ca81951c` |
| `internal/extract/golang/testdata/single-module/baseline.json` | `609be564a6e27c6dbf012d7165d83be592f3204d9fe68f749b76125627544862` |
| `internal/extract/golang/testdata/one-member-workspace/baseline.json` | `609be564a6e27c6dbf012d7165d83be592f3204d9fe68f749b76125627544862` |

Gate state: `make archfit` exits 0; `.bin/archfit check --config .archfit.yaml`
exits 2 (warn) both at `d8ef114` and with this task applied — unchanged.

Corrections to this task's Files list, carried forward:

- `internal/testdata/golden.json` does not exist. The golden output is an inline
  fixture in `internal/analysispipeline/golden_test.go`; the byte-identical CLI
  baselines live under `internal/extract/golang/testdata/*/baseline.json`.
- `cmd/archfit/pipeline_equivalence_test.go` does not exist. The equivalent
  safety net is `cmd/archfit/byteidentical_test.go`.
- Repair-task behavior is covered in `internal/assessment/agenttask`, not in
  `Evaluate`: `evaluation.Result` carries no repair tasks, so no repair
  assertion was forced into `evaluation_test.go`.

Characterized behavior worth flagging for later tasks (asserted as-is, not
fixed here — Task 1 changes no production code):

- `analysis.Analyze` panics on a nil `Graph`: `classify.Run` dereferences it
  before `buildSet`'s `g == nil` guard is reached. Acquisition always supplies a
  built graph, so the guard is unreachable dead defence.
- `pairEvidence` hashes endpoint *paths*, not node IDs, while
  `buildSet` keys the classifier index on node IDs. Label evidence hashes and
  classification keys are therefore built from different identifiers.
- A same-module edge is scored and reported under `local_coupling` but is never
  an advisory candidate, and runtime/connascence evidence never moves an edge's
  distance or severity. Both are pinned by direct tests now.

### Task 2: Establish authoritative Policy and neutral Evidence contracts

Justification: E4, E5, E6; target decisions D2, D3, D5; capability-map risks
"broad EvidenceSnapshot" and "compatibility aliases survive". This task removes
shared policy/view state before moving domain behavior.

Files:

- `internal/policy/policy.go` plus focused files such as
  `internal/policy/{topology,rules,labels,validation}.go` — authoritative pure
  topology, module/layer, gate, waiver, and approved-label values; immutable
  projections; no config/view imports.
- `internal/model/module/**` — move policy semantics to `internal/policy`; retain
  only genuinely neutral identifiers if needed, then delete the legacy
  architecture-model package and update the surface golden deliberately.
- `internal/config/**`, `internal/configschema/**`,
  `internal/application/pipeline/adapter_config.go`, and command config adapters
  — map YAML/schema lifecycle to `policy.PolicySnapshot`; no domain decisions in
  config DTOs.
- `internal/view/**` and all current consumers — replace transitional views with
  owner-specific Policy, Evidence, Relationship, Assessment, or Report contracts;
  delete `internal/view` when production and tests reach zero imports.
- `internal/evidence/snapshot.go` and `internal/evidence/signals.go` — split
  neutral immutable source facts from `AnalysisContext`; remove scope, bundle
  paths, config source/hash, time, policy labels, and mutable topology from the
  evidence snapshot.
- `internal/evidence/acquisition/**` and `internal/evidence/ports/**` — return
  neutral facts and coverage only; keep process/tool concerns behind ports.
- `internal/analysispipeline/stage_prepare.go` and
  `internal/analysispipeline/analyzer.go` — resolve ownership/topology once in
  Prepare, produce one immutable PolicySnapshot, and remove Relationship/Assess
  re-resolution.
- `internal/application/analysis.go` — own `AnalysisContext` and pass stage values
  without reading neutral evidence internals.
- `internal/policy/*_test.go`, `internal/config/*_test.go`,
  `internal/evidence/*_test.go`, and architecture tests — ownership, clone,
  normalization, immutability, and boundary coverage.
- `.archfit.yaml` — add failing-then-passing gates for policy/config direction,
  evidence context contamination, and view-import ratchets. Do not apply final
  module/volatility labels yet.

Preconditions: Task 1 tests and ratchets pass; output golden is unchanged.

Postconditions: one PolicySnapshot is built per run; owner resolution executes
once; Evidence contains neutral facts only; config is a lifecycle adapter;
`internal/view` and legacy policy-model compatibility types are deleted.

Fitness gate:

- Before implementation, new `policy_no_config`, `policy_no_view`,
  `evidence_no_policy`, `evidence_no_config`, and `no_stage_view` rules should
  expose current violations or ratchet counts.
- After implementation, all rules pass with zero production exception.
- `assessment_no_raw_graph` remains green.

Impact commands:

- `gitnexus_impact(target="PolicySnapshot", direction="upstream", depth=4, include_tests=true, repo="archfit")`
- `gitnexus_impact(target="ClassifyConfig", direction="upstream", depth=4, include_tests=true, repo="archfit")`
- `gitnexus_impact(target="Snapshot", direction="upstream", depth=4, include_tests=true, repo="archfit")`
- `gitnexus_impact(target="resolveOwners", direction="upstream", depth=4, include_tests=true, repo="archfit")`
- Use `gitnexus_rename(..., dry_run=true)` before every package/type rename; never
  use repository-wide text replacement for symbols.
- `gitnexus_detect_changes(scope="all", repo="archfit")`

Verification commands:

```sh
go test ./internal/policy ./internal/config/... ./internal/evidence/... \
  ./internal/application/... ./internal/analysispipeline/... -count=1
go test ./internal/ -run 'TestArchImports|TestModelSurfaceNoDrift' -count=1
! rg -n 'internal/view' --glob '*.go' --glob '!**/*_test.go'
! test -d internal/view
# Two lines: the helper's declaration and its single call site in Prepare.
# Anything above two is a second ownership resolution in the same run.
rg -n 'resolveOwners\(' internal | tee /tmp/resolve-owners.txt
test "$(wc -l </tmp/resolve-owners.txt | tr -d ' ')" -eq 2
make test-fast
make archfit
```

Manual checks:

- Policy values describe domain meaning, not YAML field layout.
- Evidence values are observations; no finding, severity, verdict, policy, path
  lifecycle, or rendering decision leaks into them.
- Approved labels remain policy input; label persistence remains an adapter.
- No volatility or distance metadata changes are made for score improvement.

- [x] Run and report impact analysis for PolicySnapshot, ClassifyConfig, Snapshot,
      resolveOwners, and every renamed symbol; warn on HIGH/CRITICAL impact.
      (`resolveOwners` CRITICAL — 3 direct callers, 5 processes; `evidence.Snapshot`
      HIGH — 3 processes. Both blast radii are confined to `internal/analysispipeline`,
      the package Task 4 dissolves. `PolicySnapshot`/`ClassifyConfig` resolve as
      types, so GitNexus reports UNKNOWN rather than a call-graph radius.)
- [x] Introduce pure authoritative Policy values and update config/schema adapters.
- [x] Split EvidenceFacts from AnalysisContext and update acquisition ports.
- [x] Resolve ownership/topology once and pass one cloned/immutable PolicySnapshot.
- [x] Migrate all production and test consumers from `internal/view`; delete it.
- [x] Move or delete legacy module/policy compatibility types and update the
      approved contract golden.
- [x] Add and pass policy/evidence/import fitness gates.
- [x] Run task verification, confirm byte-identical outputs, and commit the
      independently complete Policy/Evidence migration.

### Task 3: Complete the Relationship and Assessment domain model

Justification: E3, E5, E6; target decisions D2 and D4; approved
Relationship-to-Assessment contract. Relationship and Assessment are strongly
coupled core modules and must be close, explicit, and independently testable—not
merged into orchestration and not separated by generic DTOs.

Files:

- `internal/relationship/relationship.go` and
  `internal/relationship/advisory.go` — expose a minimal immutable
  `RelationshipSet` plus a separate `AnalysisEvidence`/provenance result for
  Application report projection.
- `internal/relationship/analysis/**` — own relationship orchestration and produce
  the two explicit outputs from neutral evidence and policy.
- `internal/relationship/{classify,scoring,coupling,facts,labels}/**` — keep public
  value contracts minimal; move implementation-only behavior below
  `internal/relationship/internal/**` when it has no approved cross-context
  consumer.
- `internal/assessment/evaluation/**` — consume RelationshipSet, explicit
  assessment signals, policy, baseline, and time; own status, advisory/gate
  lifecycle, verdict, scorecard, deltas, recommendations, and repair tasks.
- `internal/assessment/{rules,metrics,score,status,staleness,decision,agenttask,
  finding,result,signals}/**` — consolidate domain values under Assessment,
  remove report aliases and duplicated DTOs, and privatize implementation-only
  packages without creating a common framework.
- `internal/application/report.go` — the only pure projection from Policy,
  Evidence appendices, Relationship AnalysisEvidence, and AssessmentResult to
  `report.Document`.
- `internal/model/report/**` and `internal/report/ports/**` — stable external DTO
  contract only; no domain behavior or aliases.
- `internal/analysispipeline/{advisories,report_projection,stages,pipeline_run}.go`
  — move remaining relationship/assessment/report behavior to its owner; leave
  only temporary stage delegation for Task 4.
- Owner-local tests and `internal/arch_test.go` — direct behavior, import
  direction, immutability, public-surface, and report-projection coverage.
- `.archfit.yaml` — add failing-then-passing gates for Relationship -> Assessment,
  domain -> Report DTO, Assessment -> raw Graph/Evidence, and projector -> domain
  internals.

Preconditions: Task 2 removed view/config leakage and established neutral evidence
and authoritative policy.

Postconditions: Relationship owns classification/provenance; Assessment owns all
judgment/action; Assessment receives no raw graph/full snapshot; report projection
is behavior-free; no analysispipeline file owns domain behavior.

Fitness gate:

- Existing `assessment_no_raw_graph`, `assessment_no_coupling_internals`,
  `relationship_no_assessment`, `assessment_no_report_dtos`, and
  `relationship_no_report_dtos` remain zero.
- Add gates restricting Assessment to the public relationship contract and
  restricting report adapters to `report.Document`.
- New gates fail before movement if analysispipeline still calls status,
  staleness, finding, rule, metric, score, decision, or advisory implementations;
  they pass after movement.

Impact commands:

- `gitnexus_impact(target="AnalysisResult", direction="upstream", depth=4, include_tests=true, repo="archfit")`
- `gitnexus_impact(target="Evaluate", direction="upstream", depth=4, include_tests=true, repo="archfit")`
- `gitnexus_impact(target="Document", direction="upstream", depth=4, include_tests=true, repo="archfit")`
- `gitnexus_impact(target="collectAdvisories", direction="upstream", depth=4, include_tests=true, repo="archfit")`
- Use GitNexus dry-run rename for moved public types; then
  `gitnexus_detect_changes(scope="all", repo="archfit")`.

Verification commands:

```sh
go test ./internal/relationship/... -count=1 -coverprofile=/tmp/relationship.cover
go test ./internal/assessment/... -count=1 -coverprofile=/tmp/assessment.cover
go tool cover -func=/tmp/relationship.cover | tail -1
go tool cover -func=/tmp/assessment.cover | tail -1
go test ./internal/application ./internal/model/report ./internal/output/... -count=1
! rg -n 'internal/model/report' internal/relationship internal/assessment --glob '*.go'
! rg -n 'internal/model/graph|internal/evidence' internal/assessment --glob '*.go'
! rg -n 'assessment/(status|staleness|finding|rules|metrics|score|decision|agenttask)' \
    internal/analysispipeline --glob '*.go'
go test ./internal/ -run TestArchImports -count=1
make test-fast
make archfit
```

Manual checks:

- Relationship/Assessment integration stays a direct in-process call. Do not add
  events, serialization, or service boundaries.
- AnalysisEvidence contains provenance/report evidence, not assessment decisions.
- AssessmentResult contains domain decisions, not JSON/YAML/rendering concerns.
- Any remaining exported type has a named owning context and real consumer.

- [ ] Run and report impact analysis for relationship, assessment, report, and
      advisory symbols; stop for unreviewed HIGH/CRITICAL scope.
- [ ] Split RelationshipSet from AnalysisEvidence and migrate consumers.
- [ ] Move all relationship semantics and advisories from analysispipeline to
      Relationship Analysis.
- [ ] Move all status, staleness, findings, scoring, verdict, recommendation, and
      repair semantics to Assessment/Repair.
- [ ] Privatize implementation packages and remove report/domain aliases without
      adding shared/common packages.
- [ ] Make Application report projection the single domain-to-report conversion.
- [ ] Add direct table-driven tests until decision branches and failure paths are
      locally protected; record coverage and uncovered risk.
- [ ] Add and pass relationship/assessment/report architecture gates.
- [ ] Run task verification, confirm output and baseline compatibility, and commit
      the independently complete domain-model migration.

### Task 4: Dissolve analysispipeline and simplify Application/composition

Justification: E1, E2, E6; target decisions D1 and D3; target risk "interface
framework". After Tasks 2-3, analysispipeline should contain only delegation and
wiring. Keeping it would preserve a second owner for application flow.

Files:

- `internal/application/analysis.go`, `baseline.go`, `compare.go`, `explain.go`,
  `enrich.go`, `config_enrich.go`, and `config_update.go` — cohesive use-case
  services, request/result contracts, cancellation/error handling, and explicit
  stage lifecycle.
- `internal/application/ports.go` or owner-local port files — retain only
  PolicyLoader/Preparer, EvidenceAcquirer, RelationshipAnalyzer, Assessor,
  ReportProjector, BaselineStore, LabelStore, VCS/worktree, clock, and provider
  boundaries actually required by use cases. Remove duplicate single-method
  interfaces that represent internal helpers rather than boundaries.
- `internal/evidence/acquisition/**`, `internal/relationship/analysis/**`, and
  `internal/assessment/evaluation/**` — concrete stage services implementing
  Application-owned ports without hidden cross-stage state.
- `internal/analysispipeline/**` — relocate remaining acquisition, coverage,
  comparison, worktree, delta, and projection code to its owning application,
  evidence, assessment, persistence, or VCS adapter; delete the package.
- `cmd/archfit/{main,analyze,check,baseline,explain,config_compare,enrich,
  enrich_abstained,update}.go` and focused wiring files — flags, concrete
  construction, registry selection, renderer selection, and exit translation
  only. High fan-out is allowed only in explicit wiring code.
- `internal/baseline/**`, `internal/labels/**`, ownership/history/VCS adapters,
  and report adapters — implement owner-defined ports; no use-case decisions.
- `internal/application/*_test.go`, command characterization tests, adapter
  contract tests, and architecture tests — lifecycle, errors, comparison,
  persistence, and composition direction.
- `.github/workflows/ci.yaml`, `Makefile`, and test commands — update golden and
  package paths after analysispipeline deletion; fix the current `scip-go`
  installation module path so CI is green.
- `CLAUDE.md` invariants — update only after production paths are final in this
  task; detailed architecture docs are finalized in Task 5.

Preconditions: Tasks 2-3 leave no domain behavior in analysispipeline; all
characterization, owner-local, and architecture tests pass.

Postconditions: Application is the only use-case sequencer; owner services are
stateless per run or receive explicit immutable values; analysispipeline and
engine packages are absent; CLI composition is behavior-free; CI references
current owners.

Fitness gate:

- Add `no_analysispipeline`, `application_no_concrete_adapters`,
  `cli_no_domain_implementation`, `core_no_cmd`, and `adapter_no_cli` gates.
- Public-surface tests reject unused exports and migration aliases.
- Package/module cycle checks remain zero.

Impact commands:

- `gitnexus_impact(target="Analyzer", direction="upstream", depth=5, include_tests=true, repo="archfit")`
- `gitnexus_impact(target="StageExecutor.Execute", direction="upstream", depth=5, include_tests=true, repo="archfit")`
- `gitnexus_impact(target="runScan", direction="upstream", depth=5, include_tests=true, repo="archfit")`
- `gitnexus_impact(target="runPipeline", direction="upstream", depth=5, include_tests=true, repo="archfit")`
- Use GitNexus dry-run rename/move for every retained public symbol; then
  `gitnexus_detect_changes(scope="all", repo="archfit")`.

Verification commands:

```sh
! test -d internal/analysispipeline
! test -d internal/engine
! rg -n 'internal/(analysispipeline|engine)' --glob '*.go' --glob '*.yaml' \
    --glob '*.md' --glob '!docs/archived/**' --glob '!docs/plans/**'
! rg -n 'internal/assessment/(rules|metrics|score|status|decision|finding)' \
    cmd/archfit --glob '*.go'
go test ./internal/application/... ./internal/evidence/... \
  ./internal/relationship/... ./internal/assessment/... ./cmd/archfit -count=1
go test ./internal/ -run 'TestArchImports|TestModelSurfaceNoDrift' -count=1
bash scripts/tests/cli_exit_contract_test.sh
make all
```

Manual checks:

- Trace Analyze, Check, Explain, Baseline, Compare, Enrich, and Config Update from
  CLI to owner service. Each decision has one obvious owner.
- Inspect every remaining interface. Keep it only when it isolates a real
  context/I/O/provider boundary or has multiple meaningful implementations.
- Inspect composition fan-out by content: imports used only to construct values
  are acceptable; branching domain logic is not.
- Confirm no new `common`, `shared`, `manager`, `engine`, or `pipeline` domain
  package replaced the deleted hub.

- [ ] Run and report HIGH/CRITICAL impact for Analyzer, stage execution, scan, and
      pipeline flows before editing.
- [ ] Move each remaining analysispipeline responsibility to its approved owner.
- [ ] Reduce Application ports and exported DTOs to approved context boundaries;
      remove migration aliases and single-use internal interfaces.
- [ ] Make owner stage implementations explicit and per-run-state safe.
- [ ] Reduce command files to flags, construction, renderer selection, and exit
      translation; document any composition-only fan-out exception.
- [ ] Delete `internal/analysispipeline` and every production/test/CI reference.
- [ ] Update CI golden paths and fix the external `scip-go` installer path.
- [ ] Add and pass final application/composition/import/surface gates.
- [ ] Run task verification, confirm byte-identical outputs, and commit the
      independently complete orchestration migration.

### Task 5: Finalize Archfit model, labels, tests, docs, and baseline

Justification: E7, E8; target decision D5 and fitness requirements 1-12. The
architecture is not complete until its executable model describes the physical
code, labels are evidence-backed, CI protects it, and maintenance documentation
matches reality.

Files:

- `.archfit.yaml` — replace transitional modules with the final capability map;
  remove `engine` layer and all dead/catch-all path rules; use exact owners,
  deploy unit, roles, layers, subdomains, volatility, public paths, and forbidden
  dependencies that reflect implemented code.
- `.archfit-labels.yaml` — audit every label against a real classified edge;
  remove stale/duplicate/speculative entries; retain only reviewed deterministic
  or explicitly approved labels with correct provenance/confidence.
- `.archfit-baseline.json` — regenerate only after GO re-review and explicit
  review of every remaining active finding; never use baseline to hide an
  architecture-rule failure.
- `internal/arch_test.go`, contract surface goldens, config tests, and any
  architecture scripts — enforce dynamic model purity discovery, owner boundary
  imports, module/path coverage, no cycles, no dead rules, label resolvability,
  and report/exit contracts.
- `docs/design/20260823-archfit-capability-map.md` — change target/future language
  to implemented architecture; record final package/module map, contracts,
  accepted coupling, and design decisions.
- `docs/design/relationship-assessment-contract.md` and
  `docs/design/model-contract-deletion.md` — update final contracts and deleted
  migration debt.
- `docs/design/architecture-baseline.md` — record the baseline commit, system
  map, allowed dependency direction, bounded contexts, balanced-coupling
  rationale, known accepted risks, and change recipes.
- `README.md`, `CONTRIBUTING.md`, `CLAUDE.md`, and `AGENTS.md` — update current
  commands, package ownership, invariants, GitNexus metadata, and contributor
  guidance; remove stale engine/view/pipeline references outside archived history.
- `docs/plans/complete-capability-architecture-migration.md` — record final
  evidence and handoff. Move to `docs/plans/completed/` only after the PR merges.

Preconditions: Tasks 1-4 pass; production tree contains no engine,
analysispipeline, or view compatibility packages; branch-built binary is current.

Postconditions: source, Archfit module map, labels, tests, CI, docs, and baseline
agree; final architecture review returns GO; PR checks are green; maintenance
rules are executable.

Fitness gate:

- Every new forbidden-dependency rule first demonstrates the intended violation
  against a temporary fixture or pre-fix commit, then passes on final source.
- `.bin/archfit check` has zero configuration gaps, uncovered production paths,
  dead modules, dead path globs, dead rules, and fail-gate findings.
- No waivers are added for migration-created architecture violations.
- Displayed score is recorded but never used as acceptance evidence.

Impact commands:

- `gitnexus_detect_changes(scope="compare", base_ref="main", repo="archfit")`
- `gitnexus_detect_changes(scope="all", repo="archfit")`
- `gitnexus_query(query="Analyze Check Explain Baseline Compare Enrich stage flow", task_context="Final architecture re-review", goal="Confirm implemented owner flows", repo="archfit")`
- Fallback: `git diff --stat main...HEAD`, `git diff --name-status main...HEAD`,
  `go list -f '{{.ImportPath}} {{join .Imports " "}}' ./...`, and direct git
  history evidence.

Verification commands:

```sh
make fmt
make lint
go vet ./...
make test
make build
make archfit
git diff --check

node .gitnexus/run.cjs clean
node .gitnexus/run.cjs analyze

.bin/archfit config update --json --config .archfit.yaml > /tmp/config-review.json
.bin/archfit check --config .archfit.yaml --root "$PWD"
.bin/archfit analyze --full --json --config .archfit.yaml --root "$PWD" \
  > /tmp/archfit-final.json
.bin/archfit analyze --full --json --base main --config .archfit.yaml \
  --root "$PWD" > /tmp/archfit-vs-main.json

go test ./internal/ -run 'TestArchImports|TestModelSurfaceNoDrift' -count=1
go test ./internal/application/... ./internal/evidence/... \
  ./internal/relationship/... ./internal/assessment/... -count=1
bash scripts/tests/cli_exit_contract_test.sh
python3 internal/extract/scip/scip_reader_test.py

git status --short
git diff --check
```

Manual checks:

- Review each `.archfit.yaml` module stanza against actual source ownership and
  reasons-to-change. Do not mirror every Go package into a module.
- Review every volatility label from domain change pressure, not commit count or
  desired score. All modules share one owner/deploy unit unless physical facts
  prove otherwise.
- Review every `.archfit-labels.yaml` edge against source/type evidence and
  provenance. Abstain instead of guessing.
- Inspect generated JSON/Markdown/SARIF/scorecard/console output and comparison
  deltas for schema, ordering, paths, statuses, and exit codes.
- Run a scoped `architecture-review` against the approved capability map. Require
  an explicit GO and list residual risks without converting them to waivers.
- Review active advisories before baseline creation. Fix architecture issues;
  baseline only accepted non-erosion findings with written rationale.

- [ ] Build the final physical package/dependency map and compare it with the
      approved capability map.
- [ ] Rewrite `.archfit.yaml` to the final capability modules, layers, metadata,
      public surfaces, and dependency gates; remove transitional/dead entries.
- [ ] Add tests proving complete module coverage, no dead paths/rules, correct
      layer direction, no cycles, and approved contract surfaces.
- [ ] Audit `.archfit-labels.yaml` edge by edge; remove stale/speculative labels
      and add only evidence-backed approved labels.
- [ ] Run full validation and record source-change, module-map, and label effects
      separately from the displayed score.
- [ ] Refresh GitNexus and record compare/all change-flow evidence.
- [ ] Update architecture, contributor, command, and agent documentation to the
      implemented state; create the maintenance baseline document.
- [ ] Run a scoped architecture re-review and repair every blocking finding.
- [ ] Regenerate `.archfit-baseline.json` only after re-review GO and manual
      advisory acceptance; verify baseline does not suppress architecture gates.
- [ ] Run Ralphex review with
      `ralphex --review docs/plans/complete-capability-architecture-migration.md`.
- [ ] Confirm GitHub PR #33 lint, test, build, security, and architecture checks
      are green; commit and push the final migration.

## Acceptance criteria

- All success criteria above are proven by source, tests, Archfit output, and
  re-review evidence.
- Every task produced one independently revertible commit and left all narrower
  and whole-task checks green.
- All user-visible behavior remains compatible or has a separately approved and
  documented schema decision.
- The final architecture has explicit context ownership without shared-state,
  generic-pipeline, or compatibility hubs.
- Strong Relationship/Assessment coupling remains local and direct; distant
  boundaries use narrow contracts and owner-defined ports.
- Source packages, `.archfit.yaml`, `.archfit-labels.yaml`, architecture tests,
  docs, and baseline describe the same architecture.
- No result depends on score tuning, weakened gates, fabricated labels, lowered
  volatility, artificial distance, blanket waivers, or stale baselines.
- PR #33 is green and ready for merge.
- After merge, move this plan to
  `docs/plans/completed/20260825-complete-capability-architecture-migration.md`.

## Safety notes

The total PR blast radius is CRITICAL: hundreds of symbols and more than one
hundred files participate in changed execution flows. Every production symbol
change requires upstream GitNexus impact analysis before editing, and HIGH or
CRITICAL results must be reported before proceeding.

No irreversible data migration is planned. Baseline and label formats must stay
backward compatible. Before regenerating either file, preserve the prior version
in git and inspect the semantic diff. Never use `--no-verify`, force-push, or
waivers to bypass failing checks.

Rollback one task commit at a time. Do not keep old and new production paths in
parallel beyond the task that migrates all callers; temporary adapters must have
a same-task deletion condition.

Ralphex or another mutator executes this approved plan. Each task must stop on
behavior drift, unexplained output changes, unresolved coverage loss, dead
Archfit rules, or a boundary that requires score/label manipulation to pass.

## Re-review

After Task 5, run a scoped architecture review against
`docs/design/20260823-archfit-capability-map.md` covering:

- DDD ownership and ubiquitous language;
- modularity, cohesion, and dependency direction;
- Balanced Coupling strength/distance/volatility at each context seam;
- change locality for policy, analyzer, relationship, rule/verdict, renderer,
  and new-CLI-use-case scenarios;
- direct testability and failure-path coverage;
- Archfit module/label/gate honesty;
- remaining cognitive workload and contract surface.

Do not baseline or merge until that review returns GO.
