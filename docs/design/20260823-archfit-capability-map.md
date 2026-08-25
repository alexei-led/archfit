# Archfit capability architecture

Date: 2026-08-24 (design), 2026-08-26 (implementation record)
Status: IMPLEMENTED. `internal/engine`, `internal/analysispipeline`, and
`internal/view` are deleted; every context below has a physical owner in source.
The shipped module map, dependency measurements, accepted coupling, and change
recipes are recorded in [architecture-baseline.md](architecture-baseline.md).
This document remains the design rationale and the approved target it was
measured against.

## Overview

This is a review-driven redesign of Archfit as one Go modular monolith. It
replaces the provisional type-centric module map with domain-driven boundaries.
The goal is local change, explicit contracts, and balanced coupling. The Archfit
score is an acceptance signal, but never a reason to relabel volatility/distance,
add waivers, or create boundaries that do not exist in source.

Source inputs:

- `docs/reports/20260824-archfit-pr33-architecture-review.md`, findings F1-F5
  and evidence E1-E17.
- `README.md` and `docs/guide/concepts.md` for product intent.
- `.archfit.yaml` for current declared modules and gates.
- Current Go imports, CodeGraph, `go list`, architecture tests, and recent git
  co-change evidence.
- The current implementation on PR #33.

Constraints:

- One owner, one CLI process, one deployable.
- No services, queues, or network boundaries inside Archfit.
- Deterministic analysis and exit codes remain stable.
- Source changes and configuration-only score changes are reported separately.
- Domain labels are fixed before source experiments. Scores do not set labels.

## Source inputs and drift notes

Horizon 3 observed state, recorded when this design was written and kept as the
before-picture the migration was measured against:

- Relationship semantics now live under `internal/relationship/{coupling,scoring,classify,facts,labels}`.
- Findings, rule evaluation, metrics, status, score, and repair behavior live under `internal/assessment/**`.
- `report.Document` owns renderer DTOs; report/output production code does not import assessment findings.
- Production imports of both diagnostic and scan compatibility facades are zero.
- Fact-adapter acquisition and language construction sit behind `internal/extract/{acquire,registry}`.
- CLI direct module fan-out is ratcheted at 14; engine fan-out is ratcheted at 9 and currently measures 8.
- All tests and configured gates pass with zero blockers/cycles, but the deterministic coupling dimension is 46/mixed with 57 critical local edges.
- The Horizon 3 review was NO-GO: CLI use-case ownership, engine domain behavior, assessment/report aliases, and incomplete semantic gates drifted from this target. See `docs/reports/20260824-archfit-horizon3-architecture-review.md`.

Final state after the capability migration (2026-08-26): `internal/engine`,
`internal/analysispipeline`, and `internal/view` are deleted;
`application.StageExecutor` owns the stage order; the CLI holds composition and
exit translation only. `coupling_balance` is 41/`mixed` over 362 scored
cross-boundary edges — the number moved because the module map now describes
capabilities rather than the transitional engine split, not because coupling
worsened. See [architecture-baseline.md](architecture-baseline.md) for the
measured map and the balanced-coupling rationale.

## Domain model

DDD terms in this design:

- **Core**: differentiating architecture-analysis knowledge. It is expected to
  change as Archfit improves.
- **Supporting**: necessary workflow or integration capability that does not
  define Archfit's differentiation.
- **Generic**: framework or provider wiring that can be replaced without
  changing architecture-analysis semantics.

### Bounded contexts

| Bounded context | Classification | Volatility | Responsibility |
| --- | --- | --- | --- |
| Architecture Policy | Core | High | Desired modules, boundaries, ownership, deploy topology, rules, waivers, gates, and approved labels |
| Architecture Analysis | Core | High | Relationship classification, Balanced Coupling, metrics, findings, score, verdict, and repair guidance |
| Evidence Acquisition | Supporting | Medium | Neutral source facts, coverage, history, ownership observations, and language/tool adapters |
| Analysis Application | Supporting | Medium | Analyze, Check, Explain, and Enrich use cases; ordering, cancellation, and stage lifecycle |
| Report Projection | Supporting | Medium | Stable external report document and projection from internal results |
| Persistence | Supporting | Low | Baseline and approved-label persistence formats after compatibility is proven |
| Provider Integration | Generic | Medium | External analyzers, LLM providers, processes, and filesystem integrations |
| CLI Composition | Generic | Medium | Flags, command registration, concrete dependency wiring, and exit translation |

Architecture Analysis contains two cohesive modules:

- **Relationship Analysis** owns relationship semantics and classification.
- **Assessment and Repair** owns policy application and decisions.

Their integration is functional and volatile. They remain close inside one
bounded context instead of becoming distant services or generic shared models.

### Approved deterministic labels

These labels are approved as the target. They apply only after code has moved to
the corresponding boundary.

| Target module | Subdomain | Volatility | Deploy unit | Approval |
| --- | --- | --- | --- | --- |
| `architecture-policy` | core | high | `archfit-cli` | approved |
| `relationship-analysis` | core | high | `archfit-cli` | approved |
| `assessment-repair` | core | high | `archfit-cli` | approved |
| `evidence-contracts` | supporting | medium | `archfit-cli` | approved |
| `evidence-adapters` | supporting | medium | `archfit-cli` | approved |
| `policy-config-adapter` | supporting | medium | `archfit-cli` | approved |
| `analysis-application` | supporting | medium | `archfit-cli` | approved |
| `report-contract` | supporting | medium | `archfit-cli` | approved |
| `report-adapters` | supporting | medium | `archfit-cli` | approved |
| `persistence-adapters` | supporting | low | `archfit-cli` | approved after compatibility tests pass |
| `provider-adapters` | generic | medium | `archfit-cli` | approved |
| `cli-composition` | generic | medium | `archfit-cli` | approved |

Rejected target labels and modules:

- `scan-contract` is migration-only and medium; it is not a target bounded
  context.
- `coupling-model: medium`: coupling semantics are core/high.
- `architecture-model: supporting`: module and policy semantics are core/high.
- A separate `finding-model`: findings belong to Assessment and Repair and inherit its volatility.
- A generic shared `pipeline-state` module: stage state belongs to the
  application stage that owns it.

## Approved module map

| Module | Responsibility | Owned knowledge | Public interface | Private internals | Expected local changes |
| --- | --- | --- | --- | --- | --- |
| `architecture-policy` | Define valid intended architecture | Modules, ownership, deploy units, boundary rules, gates, waivers, approved volatility and labels | `PolicySnapshot`, validation errors, narrow `TopologyPolicy` and `GatePolicy` views | YAML representation, defaults, migration helpers | Policy schema, rule language, topology semantics |
| `relationship-analysis` | Convert evidence into classified relationships | Strength, distance, volatility, explicitness, connascence, relationship provenance, book scoring | `AnalyzeRelationships`, `RelationshipSet`, `AnalysisEvidence` | Classification heuristics, label merge, distance derivation | Coupling formula inputs, classification rules, provenance |
| `assessment-repair` | Judge analyzed architecture and produce action | Findings, metrics, scorecard, status, verdict, deltas, recommendations, repair tasks | `Assess`, `AssessmentResult`, `BaselineSnapshot` | Rule evaluators, cap logic, remediation wording | Metrics, findings, verdict, repair semantics |
| `evidence-contracts` | Represent neutral source observations | Graph, symbols, files, clones, patterns, syntax, tool coverage, raw runtime/history/ownership observations | `EvidenceSnapshot`, `SourceCoverage`, immutable fact types | Fact normalization only | New evidence families and compatibility of neutral facts |
| `evidence-adapters` | Acquire evidence from repositories and tools | Extractor implementations, tool commands, fact cache, source history and ownership readers | Context-specific acquisition ports | SCIP, ast-grep, language parsers, process details | New languages, tool versions, provider behavior |
| `policy-config-adapter` | Load, validate, initialize, and migrate policy files | YAML/schema representation and config lifecycle | Policy loader port returning `PolicySnapshot` | YAML DTOs, defaults, migrations, init/update helpers | Config format and migration behavior |
| `analysis-application` | Execute user use cases | Analyze/Check/Explain/Enrich requests, ordering, cancellation, stage-local results | Use-case services and request/result contracts | Pipeline stage state, concurrency, orchestration | Workflow and error-handling changes |
| `report-contract` | Define the versioned external report | `ReportDocument`, report section DTOs, schema version, stable JSON names | Immutable data-only report types | None | Deliberate output-schema changes |
| `report-adapters` | Render reports | Console, JSON, Markdown, SARIF, scorecard formatting | Renderer ports implemented over `ReportDocument` | Templates and format-specific helpers | Output layout and formatting |
| `persistence-adapters` | Persist comparison inputs | Baseline snapshots, approved labels, deterministic file formats | `BaselineStore`, `LabelStore` ports owned by assessment/relationship contexts | Filesystem encoding and locking | Storage compatibility and atomicity |
| `provider-adapters` | Integrate optional generic providers | LLM client and reusable process/filesystem providers | Ports owned by the calling context | Provider DTOs, retries, executable lookup | Provider changes; language analyzers remain evidence adapters |
| `cli-composition` | Expose the executable | Flags, command UX, dependency registry, exit codes | CLI commands | Concrete construction and flag translation | CLI UX and distribution wiring |
| `architecture-tests` | Enforce target dependency direction | Import rules, public surfaces, byte-identical fixtures | Test commands | Test helpers | Boundary evolution only with approved design |

All modules have one owner and one deploy unit. Distance is code/package distance,
not team or service distance.

**As implemented**, `.archfit.yaml` declares five modules the approved table did
not name separately, each because a real package boundary exists:
`analysis-scope` (`internal/scope`, support layer), `evidence-analysis`
(`internal/syntax`), `evidence-acquisition` (`internal/evidence/acquisition`,
the concrete stage service split from the neutral contract),
`config-lifecycle` (`internal/initcfg`, `internal/configschema`), and
`development-tools` (`cmd/calibrate`, `internal/calibrate`, `scripts/eval`).
The transitional `evidence-stage-contracts` module was merged back into
`evidence-contracts`: after the migration `internal/evidence` holds one
immutable `Facts` value importing only `internal/model/*`, so a separate
core-layer stanza for one pure value package would have been package mirroring.
`internal/model/report` was kept, not removed: it is the versioned external
report contract, and `assessment_no_report_dtos` / `relationship_no_report_dtos`
keep domain packages out of it.

## Type and code ownership migration

| Current location or construct | Target owner | Decision |
| --- | --- | --- |
| `internal/model/module` and pure module/rule/gate values in `internal/config` | Architecture Policy | Move semantics out of YAML/config lifecycle code |
| `internal/assessment/rules` rule definitions | Architecture Policy | Policy describes what must hold |
| `internal/assessment/rules` evaluators | Assessment and Repair | Evaluation produces findings |
| `internal/relationship/coupling`, `internal/relationship/scoring`, `internal/relationship/classify`, relationship-derived `internal/relationship/facts` | Relationship Analysis | Coupling values are a data-only context contract; scorer and classification behavior are private relationship-analysis packages |
| Strength-label approval and merge behavior | Relationship Analysis | Label storage remains an adapter |
| `internal/assessment/{finding,metrics,score,status,staleness,decision,agenttask}` | Assessment and Repair | One owner for judgment and action; the transitional finding-model module is removed |
| `internal/model/graph`, symbol, fileclass, clone, pattern | Evidence Contracts | Neutral facts only |
| Raw `Coverage`, `SyntaxFact`, `DynamicImport`, `RuntimeAsync*`, `DeprecatedDep` | Evidence Contracts | Produced by adapters, no verdict semantics |
| `DistanceContext`, `LocalCoupling*`, `ConnascenceReport`, classified volatility evidence | Relationship Analysis | These are derived relationship knowledge, not raw evidence |
| `MetricResult`, `Scorecard`, `Verdict`, `Decision`, `Recommendation`, task and delta domain values | Assessment and Repair | Remove domain values from the external report package |
| JSON-tagged external views and schema version | Report Contract | Projection DTOs only |
| `internal/assessment/signals` | Assessment and Repair | Stage-local metric inputs; no shared pipeline-state module |
| `internal/assessment/result.Result` | Assessment and Repair | Internal assessment result; project to `report.Document` at the output edge |
| `internal/model/report.Diagnostic` | No target module | Compatibility alias only; production imports are zero |
| `internal/model/report` | No target module | Remove after production imports reach zero; compatibility tests may temporarily retain it |
| `internal/analysispipeline` orchestration | Analysis Application | Ordering and cancellation only |
| `internal/application/config_enrich.go`, `config_update.go` | Analysis Application | Own config target selection, review merge, and draft/review/apply sequencing behind ports |
| `cmd/archfit/config_enrich_adapters.go`, `config_update_adapters.go` | CLI Composition | Keep concrete config/initcfg, LLM, filesystem, prompt, and render adapters at the edge |
| `cmd/archfit` use-case logic | Analysis Application | Keep only composition and command UX in `cmd` |
| `internal/output` | Report Adapters | Consume `ReportDocument` only |
| `internal/application/report.go` | Report Projection | Map `assessment.Result` to `report.Document` |

## Dependency direction

```text
CLI Composition
  -> Analysis Application
       -> Architecture Policy
       -> Evidence Acquisition ports
       -> Relationship Analysis
       -> Assessment and Repair
       -> Report Projection

Evidence Adapters -> Evidence Acquisition ports/contracts
Policy Config Adapter -> Architecture Policy
Relationship Analysis -> Evidence Contracts + narrow Policy views
Assessment and Repair -> RelationshipSet + narrow Policy views
Report Projection -> AssessmentResult + selected evidence appendix
Report Adapters -> ReportDocument
Persistence Adapters -> BaselineSnapshot or ApprovedLabelSet
Provider Adapters -> context-owned ports
```

Forbidden directions:

- Core contexts must not import CLI, renderer, store, process, LLM, or extractor
  implementations.
- Relationship Analysis must not import findings, verdicts, report DTOs, or
  output adapters.
- Assessment and Repair must not import extractors or renderers.
- Report Adapters must not import scan, evidence internals, relationship
  internals, assessment internals, or diagnostic compatibility aliases.
- CLI may construct adapters but must not interpret policy, classification, or
  assessment internals.
- No production package may import `internal/model/report` after migration.

## Integration contracts

### CLI Composition -> Analysis Application

- Strength: contract. CLI shares request, result, and exit-status vocabulary.
- Distance: separate packages, same owner/process/deploy.
- Volatility: CLI generic/medium; application supporting/medium.
- Balanced: yes when commands translate flags and delegate one use case.
- Contract: `Analyze`, `Check`, `Explain`, and `Enrich` services with typed
  request/result values.
- Private knowledge: pipeline stages, domain rules, report projection, and
  provider details stay out of command handlers.
- Failure modes: context cancellation, invalid request, unavailable optional
  tool, gate result, and internal failure remain distinct.

### Analysis Application -> Evidence Acquisition

- Strength: contract.
- Distance: separate modules in one process and owner.
- Volatility: supporting/medium on both sides.
- Balanced: yes; acquisition varies behind a stable evidence contract.
- Contract: acquisition port returns `EvidenceSnapshot` and `SourceCoverage`.
- Private knowledge: tool commands, parsers, cache, and provider DTOs remain in
  adapters.
- Failure modes: absent, disabled, partial, timed out, and invalid evidence are
  explicit data, not hidden logs.

### Relationship Analysis -> Evidence Contracts

- Strength: model. Relationship analysis consumes neutral facts.
- Distance: adjacent modules in one bounded workflow.
- Volatility: core/high relationship semantics; supporting/medium evidence.
- Balanced: acceptable only if the evidence contract stays neutral and narrow.
- Contract: immutable `EvidenceSnapshot` views scoped to required facts.
- Balancing move: lower strength by translating raw evidence before policy and
  assessment. Do not pass the whole evidence aggregate onward.

### Relationship Analysis -> Architecture Policy

- Strength: contract/model through topology and approved labels.
- Distance: separate core contexts, same owner/deploy.
- Volatility: both core/high.
- Balanced: only through narrow views.
- Contract: `TopologyPolicy`, `VolatilityPolicy`, and approved relationship-label
  values. Relationship analysis cannot depend on YAML config structures.
- Balancing move: lower strength; do not merge the contexts because desired
  architecture and observed relationships have distinct language and changes.

### Relationship Analysis -> Assessment and Repair

- Strength: functional. Assessment semantics depend on classified relationship
  meaning.
- Distance: adjacent modules inside the Architecture Analysis bounded context.
- Volatility: core/high on both sides.
- Balanced: yes because high-strength collaboration remains close.
- Contract: immutable `RelationshipSet` with provenance and scorer breakdown.
- Private knowledge: classification heuristics stay in Relationship Analysis;
  finding/rule semantics stay in Assessment.
- Failure modes: abstained and low-confidence relationships remain explicit.

### Assessment and Repair -> Report Projection

- Strength: contract/model.
- Distance: core to supporting adapter boundary, same process.
- Volatility: assessment high; report schema medium and compatibility-controlled.
- Balanced: yes when projection translates domain results into stable DTOs.
- Contract: `AssessmentResult`, selected evidence appendix, and pure
  `ProjectReport` function returning `ReportDocument`.
- Private knowledge: cap logic, rule evaluators, and remediation templates do
  not cross into renderers.

### Report Projection -> Report Adapters

- Strength: contract.
- Distance: adapter boundary in one process.
- Volatility: supporting/medium; schema version controls change.
- Balanced: yes.
- Contract: immutable `ReportDocument` only.
- Failure modes: unsupported schema version and write failure. Rendering cannot
  alter verdicts or scores.

### Core contexts -> Persistence and Provider Adapters

- Strength: contract.
- Distance: ports-and-adapters boundary, same owner/deploy.
- Volatility: core high, adapters low or provider-medium.
- Balanced: yes when ports use context-owned values.
- Contracts: `BaselineStore`, `LabelStore`, tool/process runner, and optional LLM
  advisory port.
- Private knowledge: file layout and provider DTOs remain outside core.

## Key flows

### Analyze

1. CLI translates flags into `AnalyzeRequest`.
2. Application loads `PolicySnapshot` through the policy config adapter.
3. Application acquires `EvidenceSnapshot` through acquisition ports.
4. Relationship Analysis produces `RelationshipSet` and analysis evidence.
5. Assessment produces `AssessmentResult` and optional comparison result.
6. Report Projection creates `ReportDocument`.
7. A report adapter renders the document.
8. CLI translates the result into the documented exit code.

Local-change expectation:

- New language support stays in evidence adapters and contracts.
- New coupling semantics stay in Relationship Analysis.
- New rule, metric, verdict, or repair behavior stays in Assessment and Policy.
- New output layout stays in Report Projection and adapters.

### Check

Check reuses Analyze through Assessment. The application selects gate behavior
and exit translation; it does not duplicate analysis. Renderers cannot influence
gate results.

### Explain

Explain loads or produces an `AssessmentResult`, selects one finding and its
relationship evidence, and projects a stable explanation view. Optional LLM
narration is advisory and consumes a context-owned input contract.

### Baseline comparison

Assessment owns `BaselineSnapshot` semantics. A persistence adapter reads and
writes the file. Comparison produces assessment deltas before report projection.
The baseline file does not store renderer-specific DTOs.

## Module test specifications

### Architecture Policy

Behavior tests:

- Valid policy input produces an immutable `PolicySnapshot` with resolved module,
  rule, gate, waiver, ownership, deploy, and label semantics.
- Invalid topology, unknown module references, overlapping paths, and invalid
  labels fail with deterministic validation errors.

Contract tests:

- YAML loading and config migrations map to the same policy snapshot across
  supported config versions.
- Policy views expose only the topology or gate data required by a consumer.

Boundary tests:

- Policy code imports no extractor, renderer, process, LLM, or filesystem
  implementation.
- Core policy types contain no YAML-library or CLI types.

### Relationship Analysis

Behavior tests:

- Table-driven strength, distance, volatility, explicitness, connascence, and
  book-score cases cover valid, unknown, and abstained relationships.
- Approved labels override heuristics only under documented provenance rules.
- Same input and policy produce byte-identical relationship sets.

Contract tests:

- `EvidenceSnapshot` and narrow policy views map to `RelationshipSet` without
  findings, verdicts, or output DTOs.
- Provenance and score breakdown survive projection to assessment inputs.

Boundary tests:

- Relationship Analysis imports no assessment, report, renderer, store, CLI, or
  concrete tool adapter.

### Assessment and Repair

Behavior tests:

- Rule, metric, cap, status, verdict, delta, recommendation, and repair-task
  matrices cover pass, warn, fail, unknown, and partial-coverage paths.
- Existing score formula and cap disclosure regressions remain table-driven.
- Findings and tasks are deterministic for identical relationship and policy
  inputs.

Contract tests:

- `RelationshipSet` plus policy views produce `AssessmentResult` without raw
  extractor or renderer knowledge.
- `BaselineSnapshot` round-trips through the baseline-store port.

Boundary tests:

- Assessment imports no extractor, process, LLM implementation, report adapter,
  or CLI package.

### Evidence Acquisition

Behavior tests:

- Each language adapter produces neutral facts and explicit coverage states for
  ok, partial, absent, disabled, and timed-out runs.
- Tool failure, cancellation, malformed output, and cache hit/miss behavior are
  characterized at adapter boundaries.

Contract tests:

- All adapters satisfy the acquisition port and fixture corpus.
- Evidence contracts remain independent of findings, scorecards, verdicts, and
  render DTOs.

Boundary tests:

- Provider DTOs and command-line details do not escape adapters.

### Analysis Application

Behavior tests:

- Analyze, Check, Explain, and Enrich execute stages in the approved order.
- Cancellation and stage errors stop downstream work and preserve documented
  exit semantics.
- Check reuses Analyze/Assessment rather than duplicating policy behavior.

Contract tests:

- Command handlers translate flags into use-case requests and results into exit
  codes without inspecting domain internals.
- Test fakes implement ports at actual system boundaries only.

Boundary tests:

- Application imports context ports and contracts, not concrete extractors,
  renderers, stores, or provider clients.

### Report Projection and Adapters

Behavior tests:

- Projection produces a complete, versioned `ReportDocument` from assessment
  result and selected evidence appendix.
- Console, JSON, Markdown, SARIF, and scorecard outputs remain byte-identical for
  current fixtures unless a schema change is explicitly approved.

Contract tests:

- Every renderer accepts `ReportDocument` only.
- Unsupported report schema versions and write failures are explicit.
- Rendering cannot change score, verdict, findings, or exit status.

Boundary tests:

- Report adapters import no scan, diagnostic, evidence internals, relationship
  internals, assessment internals, or CLI package.

### Persistence and Provider Adapters

Behavior tests:

- Baseline and approved-label writes are atomic and deterministic.
- Optional provider failure remains advisory unless the calling context declares
  it required.

Contract tests:

- Stores accept context-owned snapshots or label sets, never report documents or
  provider DTOs.
- Provider adapters map external errors into context-owned error categories.

## Architecture-fitness checks

All twelve are implemented and green on the final source. Each entry names the
check that enforces it; the full invariant-to-check table is in
[architecture-baseline.md](architecture-baseline.md).

1. Discover all `internal/model/**` or successor domain packages automatically;
   no hardcoded incomplete purity list. — `checkModelPurity` walks every loaded
   `internal/model` package (`TestArchImports`).
2. `internal/output/**` and renderer ports may import only `report-contract` and
   format-local helpers for completed reports. — `renderer_no_assessment`,
   `renderer_no_relationship`, `renderer_no_evidence`, `output_no_config`,
   `output_no_score`, `output_no_decision`, `report_adapters_no_application`.
3. No DOMAIN package imports `internal/model/report`. — `assessment_no_report_dtos`,
   `relationship_no_report_dtos`, `TestDomainPackagesDoNotImportReportDTOs`.
   Revised from the original wording: `internal/model/report` was kept as the
   versioned external contract, so the rule is that only the application
   projector and the renderers may touch it, not that nothing may.
4. Core policy, relationship, and assessment modules cannot import adapters,
   CLI, renderers, stores, process runners, or LLM implementations. —
   `TestArchImports`, `core_no_toolrun`, `core_no_extract`, `internal_no_llm`,
   `internal_no_labelsio`, `core_no_cmd`, the `*_no_config` family.
5. Relationship Analysis cannot import Assessment and Repair. Assessment may
   import the immutable relationship contract only. — `relationship_no_assessment`,
   `assessment_no_relationship_internals`,
   `TestAssessmentConsumesOnlyThePublicRelationshipContract`.
6. Application orchestration cannot import concrete adapters. — nine
   `application_no_*` rules, `TestApplicationImportsNoConcreteAdapters`.
7. CLI composition may import concrete adapters but cannot import internal rule,
   metric, classifier, score, or decision implementation packages. —
   `cli_no_domain_implementation`, `TestCLIImportsNoDomainImplementation`.
8. Module and package dependency graphs remain acyclic. — `cycle` metric = 0.
9. Public surface snapshots cover the approved contracts, not temporary aliases.
   — `TestModelSurfaceNoDrift`, `TestTransitionalContractSurfaceRatchet`.
10. Byte-identical output fixtures protect all supported formats and exit codes.
    — `TestGolden`, `scripts/tests/cli_exit_contract_test.sh`.
11. Archfit dogfooding records fixed-config source delta separately from module
    map, volatility, or distance changes. — recorded in the migration plan's
    final-evidence section.
12. `.archfit.yaml` labels change only after the corresponding physical module
    boundary exists and the design ledger is updated. — `.archfit-labels.yaml`
    is empty with a written abstain rationale; the self-model is gated by
    `TestSelfModel*` (`internal/selfmodel_test.go`).

Required whole-design checks:

```sh
make lint
make test
make build
make all
go vet ./...
git diff --check
# Use the branch-built binary, not an older installed release.
/tmp/archfit-pr33 check --config .archfit.yaml --root "$PWD" --format json
/tmp/archfit-pr33 analyze --config .archfit.yaml --root "$PWD" --base main --format json
codegraph status .
```

A refreshed GitNexus run is optional until its index matches the implementation
commit. Direct git history is the fallback and its coverage gap must be recorded.

## Design decisions and trade-offs

### D1: Keep a modular monolith

- Chosen because: one owner, one deployable, and close runtime collaboration.
- Rejected: services or process boundaries.
- Trade-off: module discipline must be enforced in tests rather than by network
  boundaries.
- Revisit when: independent ownership and deployment are real requirements.

### D2: Organize core behavior by bounded context, not data shape

- Chosen because: model and behavior share invariants and change vectors.
- Rejected: one module per DTO category.
- Trade-off: some packages move and context translation becomes explicit.
- Revisit when: history shows a proposed context has two independent change
  vectors after implementation.

### D3: Dissolve scan and pipeline-state as stable domain modules

- Chosen because: they are transport aggregates with no independent business
  rules.
- Rejected: preserving them as global shared kernels.
- Trade-off: application stage results and report projection need explicit
  mappings.
- Revisit when: an externally versioned scan API becomes a product requirement.

### D4: Separate assessment from report projection

- Chosen because: domain verdicts and output compatibility change for different
  reasons.
- Rejected: renderers consuming the whole scan or assessment aggregate.
- Trade-off: one deterministic projection layer and additional golden tests.
- Revisit when: only one output format remains and the external schema is
  intentionally identical to the assessment model.

### D5: Fix volatility labels before measuring source changes

- Chosen because: score sensitivity can mask or invent improvement.
- Rule: each experiment records fixed-config source delta separately from module
  map or label delta.
- Trade-off: an honest score can fall while architecture improves.

## Non-goals

- No package split solely to change `coupling_balance`.
- No new baseline or waiver to hide findings.
- No services, plugin framework, event bus, or generic interface layer.
- No requirement that every target module maps to one Go package.
- No promise that the raw Archfit score increases in every implementation step.

## Self-review

| Issue | Severity | Rationale | Resolution |
| --- | --- | --- | --- |
| A broad `EvidenceSnapshot` could become another shared-kernel hub | High | Relationship, assessment, reporting, and CLI could all depend on neutral raw facts | Only Relationship Analysis consumes the full snapshot. Other contexts receive narrow relationship, assessment, or appendix views. Add an import/contract fitness check. |
| Report DTOs could duplicate domain models and drift | Medium | Projection deliberately separates compatibility from core semantics | Keep one pure projector, byte-identical golden tests, and explicit schema versioning. Do not copy domain behavior into DTOs. |
| Context boundaries could create an interface framework | Medium | Ports-and-adapters can add indirection without reducing knowledge sharing | Add ports only at I/O/provider boundaries or between approved contexts. Internal functions inside a context remain concrete. |
| Policy config lifecycle was absent from the first target table | Medium | YAML/schema migration changes for different reasons than policy semantics | Added `policy-config-adapter`; it maps external representation to `PolicySnapshot`. |
| Relationship and assessment are strongly coupled | High | Both are core/high and co-change, so separating them too far would be unbalanced | Keep them as adjacent modules inside the Architecture Analysis bounded context with one immutable `RelationshipSet` contract. Never split them into services. |
| The migration can become a big-bang rewrite | High | CLI, engine, scan, report, and evaluation currently meet in one flow | Use two execution horizons with a mandatory re-review after report/compatibility repair. Every task remains independently committable and behavior-preserving. |
| Fixed domain labels can lower the displayed score | Low | Current score benefits from disputed low/medium volatility labels | Accept the lower score. Source, module-map, and label deltas are reported independently. |
| Compatibility aliases may survive indefinitely | Medium | Golden surfaces can accidentally preserve migration debt | Add a non-increasing import ratchet, then a zero-production-import fitness rule and delete the facade when the ratchet reaches zero. |

No critical design issue remains unresolved. High risks have explicit sequencing or
fitness controls.

## Open risks

Resolved during implementation:

- Baseline and approved-label file compatibility — characterized before the
  persistence ports moved; both formats are unchanged.
- Output schema compatibility — proven across JSON, Markdown, SARIF, console,
  and scorecard by the golden and exit-contract suites.
- GitNexus staleness — the index was refreshed against the final commit.
- Evidence/relationship provenance — carried on `relationship.Provenance`; no
  raw extractor type crosses the seam.

Still open, tracked as accepted risks in
[architecture-baseline.md](architecture-baseline.md):

- Context ownership and dependency direction still require a design update
  before Go package names move.
- `internal/extract/{ts,py}` bind to the `relationship/coupling` strength
  constants for the hints they emit. Shared vocabulary, not a decision; the
  upgrade trigger is recorded.

## Handoff

COMPLETE. The plan this design handed off to
(`docs/plans/complete-capability-architecture-migration.md`) finished all five
tasks. The record below is kept as the migration's acceptance ledger.

Implementation uses the existing two-horizon backlog:

- Horizon 1: tasks #22-#25 characterize output behavior, complete the report
  boundary, retire diagnostic production imports, and run a scoped re-review.
- Horizon 2: tasks #26-#30 extract application services and stage contracts,
  establish relationship and assessment modules, align the module map and
  fitness checks, and run the final review.

Implementation rules:

- An engineer or mutator executes the plan; the architecture-design task edits
  no production source.
- Each task is independently committable and behavior-preserving unless it
  carries an explicitly approved schema change.
- Horizon 2 cannot start until task #25 approves the Horizon 1 implementation.
- `.archfit.yaml` target labels are applied only after the matching physical
  source boundary exists.
- Use the branch-built Archfit binary for fixed-configuration comparisons.
- GitNexus impact evidence is required only after its index matches the working
  commit. Until then use CodeGraph plus `git diff --name-only` and direct history.

Acceptance signals:

- Production diagnostic imports: 19 to 0.
- Renderer and renderer-port imports of scan: nonzero to 0.
- CLI module fan-out: 18 to at most 14, or composition-only exceptions are
  documented.
- `internal/engine`, `internal/analysispipeline`, and `internal/view`: deleted.
- Package and module cycles remain 0. — measured 0.
- All output fixtures and exit-code contracts pass.
- Core modules import no adapters.
- The final review reports source, module-map, and label score effects separately.
- Boundary-integrity and cohesion scores improve from the review baseline; a raw
  Archfit score increase is not required.

Rollback:

- Revert one horizon task at a time; no data migration is irreversible.
- Keep old file-format readers until byte-identical compatibility tests pass.
- Do not keep both old and new domain paths beyond the task that migrates all
  production callers; temporary aliases are migration-only.
