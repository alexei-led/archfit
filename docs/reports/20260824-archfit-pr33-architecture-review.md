---
artifact: architecture-report
schema_version: 2
rubric_version: 1
report_id: archfit-pr33-domain-modularity-review
date: 2026-08-24

target:
  repo: alexei-led/archfit
  scope: full
  out_of_scope:
    - External analyzer implementation quality
    - Multi-team and multi-deploy topology because Archfit is one owner and one deployable

comparability:
  scope: full
  rubric_version: 1
  tool_coverage_level: standard

interview_context:
  system_goal: Provide deterministic architecture-fitness decisions for source repositories, centered on declared boundaries and Balanced Coupling evidence.
  quality_goals:
    - Truthful modular boundaries
    - Balanced coupling without score gaming
    - Local changes
    - Stable output contracts
    - Executable architecture fitness checks
  intended_units:
    - One Go CLI deployable
    - Domain capabilities for policy, evidence, coupling analysis, assessment, orchestration, and reporting
    - Adapters behind ports
  domains:
    core:
      - Architecture policy semantics
      - Relationship and Balanced Coupling analysis
      - Assessment, verdict, and repair guidance
    supporting:
      - Evidence acquisition
      - Pipeline orchestration
      - Reporting and persistence
      - Configuration lifecycle
    generic:
      - CLI framework
      - LLM provider adapters
      - External language tool runners
  volatile_areas:
    - Policy and module semantics
    - Coupling classification and scoring
    - Analysis pipeline
    - Language adapter coverage behavior
  team_ownership:
    - One owner, alexei-led
  known_pain:
    - Coupling score is sensitive to module and volatility configuration
    - Large CLI, engine, and evaluation modules co-change frequently
    - Report and diagnostic compatibility boundaries are incomplete
  review_scope: Full PR 33 architecture against main
  out_of_scope:
    - Splitting into services

system_map:
  languages:
    - Go 1.26
    - Python, TypeScript, and Rust fixtures and helper scripts
  package_managers:
    - Go modules
  units:
    - archfit CLI
    - calibrate development command
    - 65 Go packages
  deploy_units:
    - Static archfit binary
    - Docker image containing the same CLI and analyzer tools
  public_interfaces:
    - CLI commands and exit-code contract
    - JSON, SARIF, Markdown, console, and scorecard output schemas
    - .archfit.yaml configuration schema
    - Baseline and label files
  declared_modules:
    - evidence-model
    - coupling-model
    - pipeline-state
    - architecture-model
    - finding-model
    - report-contract
    - scan-contract
    - stage-views
    - policy-language
    - pipeline-contracts
    - evaluation-core
    - pipeline-engine
    - fact-adapters
    - config-lifecycle
    - baseline-store
    - labels-policy
    - labels-store
    - rendering
    - llm-adapter
    - archfit-cli
    - development-tools
    - architecture-tests
  observed_modules:
    - CLI composition and use-case logic
    - Analysis orchestration
    - Evidence acquisition adapters
    - Relationship classification and coupling analysis
    - Policy evaluation and metrics
    - Verdict and repair synthesis
    - Report projection and output adapters
    - Configuration, labels, and baseline lifecycle
  high_risk_entrypoints:
    - cmd/archfit/pipeline_run.go:161 runPipeline
    - internal/engine/engine.go:133 Run
    - internal/engine/assemble.go
    - cmd/archfit/registry.go
  missing_evidence:
    - GitNexus index is stale at main and was not used for current structural claims
    - No distinct runtime or deploy distance exists inside the single CLI
    - Domain volatility labels need owner confirmation

module_volatility:
  - module: policy-language
    classification: core
    volatility: high
    source: docs
    evidence_refs: [E1, E2, E10]
    confidence: high
    notes: Architecture policy semantics are a differentiating product capability.
  - module: evaluation-core
    classification: core
    volatility: high
    source: architect-inferred
    evidence_refs: [E9, E10, E11]
    confidence: high
    notes: Classification, metrics, scoring, and decisions change often but do not form one cohesive change vector.
  - module: coupling-model
    classification: core
    volatility: high
    source: architect-inferred
    evidence_refs: [E1, E10, E16]
    confidence: medium
    notes: Current config says medium; coupling semantics are central product behavior and inherit high volatility from evaluation.
  - module: architecture-model
    classification: core
    volatility: high
    source: architect-inferred
    evidence_refs: [E1, E2, E16]
    confidence: medium
    notes: Module, ownership, and deploy semantics are part of the core policy language, not merely supporting DTOs.
  - module: finding-model
    classification: core
    volatility: high
    source: architect-inferred
    evidence_refs: [E10, E16]
    confidence: medium
    notes: It inherits policy/evaluation volatility even when its small package changes less often.
  - module: evidence-model
    classification: supporting
    volatility: medium
    source: architect-inferred
    evidence_refs: [E4, E9, E10]
    confidence: medium
    notes: Neutral evidence contracts are supporting, but their broad shared-kernel role propagates change widely.
  - module: scan-contract
    classification: supporting
    volatility: medium
    source: architect-inferred
    evidence_refs: [E6, E7, E10, E16]
    confidence: high
    notes: Current low label is not credible while the aggregate evolves with every new product signal and is imported across the pipeline.
  - module: report-contract
    classification: supporting
    volatility: low
    source: docs
    evidence_refs: [E1, E7]
    confidence: medium
    notes: Intended stable output contract; history is split by the recent move and cannot yet corroborate low volatility.
  - module: pipeline-engine
    classification: supporting
    volatility: medium
    source: architect-inferred
    evidence_refs: [E9, E10, E11]
    confidence: high
    notes: Application orchestration, but it inherits core change through broad stage knowledge.
  - module: fact-adapters
    classification: supporting
    volatility: medium
    source: architect-inferred
    evidence_refs: [E4, E10, E13]
    confidence: high
    notes: Provider implementation volatility is real, while business-domain volatility remains supporting.
  - module: rendering
    classification: supporting
    volatility: medium
    source: architect-inferred
    evidence_refs: [E6, E7, E10]
    confidence: high
    notes: Output formats are adapters but currently bind to the broad scan aggregate.
  - module: archfit-cli
    classification: generic
    volatility: medium
    source: architect-inferred
    evidence_refs: [E4, E9, E10, E12, E13]
    confidence: high
    notes: Composition wiring is generic; command and product use-case logic make the package highly change-active.

scores:
  boundary_integrity:
    value: 58
    band: mixed
    confidence: high
    evidence_refs: [E2, E6, E7, E8, E14]
    gaps: []
  coupling_balance:
    value: 50
    band: mixed
    confidence: medium
    evidence_refs: [E4, E5, E10, E16]
    gaps:
      - Volatility labels are unconfirmed and all scored relationships use the same D=4 rung.
  dependency_graph_health:
    value: 76
    band: serviceable
    confidence: high
    evidence_refs: [E3, E4]
    gaps: []
  cohesion_modularity:
    value: 44
    band: mixed
    confidence: high
    evidence_refs: [E4, E9, E10, E11, E12]
    gaps: []
  change_locality:
    value: 48
    band: mixed
    confidence: medium
    evidence_refs: [E10]
    gaps:
      - GitNexus is stale and recent type moves split path history.
  architecture_fitness:
    value: 70
    band: serviceable
    confidence: high
    evidence_refs: [E2, E14, E15]
    gaps:
      - New evidence and scan packages are missing from a hardcoded model-purity list.
      - No fitness rule forbids output-to-scan or production diagnostic-facade imports.
  analysis_confidence:
    value: 76
    band: serviceable
    confidence: medium
    evidence_refs: []
    gaps:
      - GitNexus stale
      - Volatility labels unconfirmed

findings:
  - id: F1
    title: Report boundary still exposes the full scan aggregate
    severity: high
    dimension: boundary_integrity
    evidence_refs: [E6, E7, E8]
    narrative:
      problem: The renderer port and output adapters consume scan.Diagnostic instead of a completed report projection.
      knowledge_or_boundary_leakage: Extraction coverage, findings, runtime evidence, coupling summaries, deltas, tasks, and verdict data cross one broad transport seam.
      complexity_impact: Output changes can depend on pipeline internals, and adding one scan signal expands the model visible to every renderer.
      cascading_change_scenarios:
        - Adding a new evidence family changes scan, engine assembly, JSON, Markdown, tests, and compatibility aliases.
        - Versioning the JSON schema can force changes into decision and rendering code that should depend on narrower views.
      recommended_improvement: Introduce one immutable ReportDocument projection and make renderer and baseline ports consume it; drive production diagnostic imports to zero.
      tradeoffs: Adds an explicit mapping step and golden-output migration; preserve the compatibility facade only at external or test boundaries.
    recommended_action: Complete the report projection before further module splitting.
  - id: F2
    title: Type-centric model modules do not define cohesive bounded contexts
    severity: high
    dimension: cohesion_modularity
    evidence_refs: [E2, E4, E9, E10]
    narrative:
      problem: The map separates finding, pipeline-state, architecture, coupling, scan, and report data shapes while evaluation-core still groups many unrelated behaviors.
      knowledge_or_boundary_leakage: Shared data packages become an anemic model archipelago, while classification, rules, metrics, status, score, decision, and repair logic remain one broad context.
      complexity_impact: The module map adds edges without giving one capability ownership of its model and behavior.
      cascading_change_scenarios:
        - A coupling-policy change touches coupling model, classification, metrics, score, engine, CLI, and reports.
        - A finding lifecycle change touches finding model, status, decision, agent tasks, scan, and renderers.
      recommended_improvement: Organize around policy, relationship analysis, assessment/repair, application orchestration, and report projection; keep model and behavior together inside each context.
      tradeoffs: Requires translations at context boundaries and may initially lower the Archfit score by exposing honest high-volatility edges.
    recommended_action: Approve a domain-context target map before changing more module labels.
  - id: F3
    title: CLI and pipeline orchestration are oversized change hubs
    severity: high
    dimension: change_locality
    evidence_refs: [E4, E9, E10, E11, E12, E13]
    narrative:
      problem: cmd/archfit has 18 module fan-out and 8775 production LOC; runPipeline is 344 lines. Engine Run is 240 lines and engine fan-out is 11.
      knowledge_or_boundary_leakage: Composition wiring, application use cases, coverage policy, git delta handling, report projection, and adapter details meet in the same packages.
      complexity_impact: Feature work crosses CLI, engine, evaluation, adapters, and reporting; the strongest co-change pair is evaluation-core with pipeline-engine at 74 commits.
      cascading_change_scenarios:
        - Adding an analyzer changes registry, coverage policy, pipeline assembly, doctor/install behavior, engine inputs, and tests.
        - Adding an output signal changes engine assembly, CLI enrichment, scan, and several renderers.
      recommended_improvement: Keep concrete adapter construction in cmd, but move Analyze/Check/Enrich use cases into application services with narrow stage inputs and outputs.
      tradeoffs: Avoid a framework of one-interface-per-function; extract only stable use-case seams with demonstrated multiple callers or change vectors.
    recommended_action: Reduce CLI fan-out to 10-14 and engine fan-out to 7-9 through application facades.
  - id: F4
    title: Volatility labels make the coupling score configuration-sensitive
    severity: medium
    dimension: coupling_balance
    evidence_refs: [E5, E16]
    narrative:
      problem: The declared run scores 54, but the native source delta under the PR configuration is 55 to 54, and plausible DDD volatility corrections lower the score to 41-50.
      knowledge_or_boundary_leakage: Supporting/low labels hide inferred volatility carried from core policy and evaluation behavior through shared scan and model contracts.
      complexity_impact: A score increase can be mistaken for source improvement, leading refactoring toward favorable labels instead of lower knowledge sharing.
      cascading_change_scenarios:
        - Relabeling scan-contract from low to high changes critical edges from 14 to 39 without source changes.
        - Marking broad core-facing models high moves the score near the pre-PR value without changing imports.
      recommended_improvement: Confirm domain labels with the owner, report source and configuration deltas separately, and hold labels fixed during source experiments.
      tradeoffs: The headline score may fall; the measurement becomes more credible and comparable.
    recommended_action: Establish an approved volatility ledger before using score movement as acceptance evidence.
  - id: F5
    title: Architecture fitness checks lag the new contract map
    severity: medium
    dimension: architecture_fitness
    evidence_refs: [E2, E14, E15]
    narrative:
      problem: The hardcoded model-purity list omits new evidence and scan packages, and no check enforces the intended output-to-report boundary or diagnostic-facade retirement.
      knowledge_or_boundary_leakage: Design intent exists in docs but is not executable for the highest-priority new seams.
      complexity_impact: The repository can pass its architecture gate while renderers still consume scan.Diagnostic and production code continues importing compatibility aliases.
      cascading_change_scenarios:
        - A future adapter import can enter evidence or scan without TestArchImports seeing it.
        - Compatibility imports can grow while the model-surface golden preserves them.
      recommended_improvement: Make model coverage pattern-based; forbid output and renderer ports from scan; allow diagnostic only in compatibility tests.
      tradeoffs: New gates must be introduced after migration or they will block the current branch immediately.
    recommended_action: Add boundary fitness checks in the same changes that complete each migration.

archfit_calibration:
  source_commands:
    - /tmp/archfit-pr33 analyze --config .archfit.yaml --root $PWD --format json
    - /tmp/archfit-pr33 analyze --config .archfit.yaml --root $PWD --base main --format json
    - /tmp/archfit-pr33 check --config .archfit.yaml --root $PWD --format json
  artifacts:
    - docs/reports/archfit-report.md
    - /tmp/archfit-pr33-full.json
    - /tmp/archfit-pr33-delta.json
    - /tmp/archfit-pr33-check.json
  confirmed:
    - 429 scored edges and zero abstained edges
    - No blocking findings
    - No module dependency cycles
    - Dominant measured pairs are CLI to adapters, adapters to evidence, and evaluation to evidence
  severity_adjusted:
    - All scored distances are same-owner and same-deploy D=4; critical arithmetic is local structural risk, not distributed-monolith risk.
    - CLI to fact-adapters is expected composition-root coupling and is not the first repair target.
    - More medium warnings after the PR reflect more explicit boundaries, not a simple regression.
  false_positive_or_noise:
    - The full 40 to 54 comparison mixes source and module-map changes; native base comparison under one configuration is 55 to 54.
  missed_by_archfit:
    - Renderers consume the full scan aggregate.
    - Production compatibility imports remain.
    - The module map is type-centric rather than capability-cohesive.
    - Fitness lists omit new model packages.
    - Large orchestration functions and cross-module co-change remain.
  config_changes:
    - Confirm scan-contract volatility; low is not currently credible.
    - Confirm coupling-model, architecture-model, and finding-model as core or inferred-high.
    - Keep score experiments on one fixed approved configuration.
  new_fitness_checks:
    - Output adapters and renderer ports cannot import model/scan.
    - Production packages cannot import model/diagnostic.
    - All internal/model packages are covered automatically by purity checks.
    - CLI may construct adapters but application policy lives outside cmd.
  labels_to_confirm:
    - scan-contract medium or high
    - coupling-model core/high
    - architecture-model core/high
    - finding-model inferred-high
    - evidence-model core versus supporting
  confidence_impact: medium

evidence:
  - id: E1
    type: file
    ref: README.md:11-196
    summary: Defines Archfit as one architecture-fitness CLI, its Balanced Coupling semantics, and intended report-contract flow.
  - id: E2
    type: file
    ref: .archfit.yaml:44-590
    summary: Declares 22 modules, volatility/subdomain labels, dependency gates, and architecture fitness rules.
  - id: E3
    type: command
    command: go list -json ./... plus module-SCC analysis
    summary: Found 65 Go packages, 22 observed modules, no unscoped packages, and zero module dependency cycles.
  - id: E4
    type: command
    command: codegraph status . && codegraph explore runPipeline engine.Run score.Synthesize decision.Build analyzeRender
    summary: Current index covered 414 files, 7144 nodes, and 21951 edges; confirmed pipeline flow and high-fan-out hubs.
  - id: E5
    type: command
    command: /tmp/archfit-pr33 analyze --config .archfit.yaml --root $PWD --format json and --base main
    summary: Declared score is 54 with 429 scored edges and 14 critical; native source delta under PR config is 55 to 54.
  - id: E6
    type: file
    ref: internal/ports/ports.go:164-171
    summary: Renderer port accepts scan.Diagnostic rather than the intended report projection.
  - id: E7
    type: file
    ref: internal/model/scan/scan.go:68-180
    summary: Scan aggregate combines metrics, findings, evidence, coverage, runtime facts, deltas, tasks, and summary data.
  - id: E8
    type: command
    command: rg -n 'internal/model/diagnostic|diagnostic\.' --glob '*.go' --glob '!**/*_test.go' cmd internal
    summary: Nineteen production files still import the diagnostic compatibility facade.
  - id: E9
    type: command
    command: go/parser production LOC and function-length analysis
    summary: CLI is 8775 LOC, evaluation-core 6309, engine 2926; runPipeline is 344 lines and engine.Run is 240 lines.
  - id: E10
    type: command
    command: git log --since=180.days --format=@@%H --name-only -- cmd internal .archfit.yaml README.md docs
    summary: 468-commit window; strongest co-change is evaluation-core to pipeline-engine 74, followed by CLI to evaluation 65 and CLI to engine 57.
  - id: E11
    type: file
    ref: internal/engine/engine.go:133-365
    summary: One method performs extraction, matching, classification, rules, metrics, status, verdict, advisory assembly, and scan construction.
  - id: E12
    type: file
    ref: cmd/archfit/pipeline_run.go:161-504
    summary: CLI application flow owns cache, history, extraction wiring, base comparison, score, coverage, and gate coordination.
  - id: E13
    type: file
    ref: cmd/archfit/registry.go:1-205
    summary: Composition root directly constructs all language adapters; this is expected but contributes to CLI fan-out.
  - id: E14
    type: file
    ref: internal/arch_test.go:66-91
    summary: Hardcoded model package list excludes newly introduced evidence and scan packages.
  - id: E15
    type: file
    ref: internal/model_surface_test.go:1-74
    summary: Golden surface check is executable but its low-volatility/shared-kernel rationale conflicts with several current module labels and preserves compatibility aliases.
  - id: E16
    type: command
    command: Temporary-config Archfit sensitivity runs for scan/core/evidence volatility and DDD candidate maps
    summary: Plausible corrected labels lower score to 41-50; vertical-context grouping without source translation scores 41-52, proving config-only context maps are not a fix.
  - id: E17
    type: interview
    summary: Vlad independent review recommended a modular monolith with evidence acquisition, classification, policy evaluation, decision/repair, orchestration, and report projection contexts; services were rejected.

tool_coverage:
  - dimension: discovery
    tools_used:
      - rg
      - find
      - go list
    tools_skipped: []
    tools_missing: []
    tools_failed: []
    confidence_impact: none
  - dimension: structural
    tools_used:
      - go/parser
      - Archfit syntax facts
      - CodeGraph
    tools_skipped:
      - ast-grep direct queries because Go AST and current CodeGraph covered the selected claims
    tools_missing: []
    tools_failed: []
    confidence_impact: none
  - dimension: semantic
    tools_used:
      - CodeGraph current index
      - go/types through project architecture tests
    tools_skipped: []
    tools_missing: []
    tools_failed: []
    confidence_impact: none
  - dimension: dependency
    tools_used:
      - go list
      - CodeGraph
      - Archfit
    tools_skipped: []
    tools_missing: []
    tools_failed: []
    confidence_impact: none
  - dimension: change
    tools_used:
      - git log direct co-change analysis
    tools_skipped: []
    tools_missing: []
    tools_failed:
      - GitNexus index stale at main commit df312e5
    confidence_impact: medium
  - dimension: operational
    tools_used:
      - Dockerfile inspection
      - GitHub Actions workflow inspection
      - Makefile release targets
    tools_skipped:
      - Live deployment topology because the product is a distributable CLI
    tools_missing: []
    tools_failed: []
    confidence_impact: low
  - dimension: report
    tools_used:
      - YAML parser
      - scoped Markdown checks
    tools_skipped:
      - Mermaid rendering because the report uses a plain-text map
    tools_missing:
      - architect-validate-report command not installed
    tools_failed: []
    confidence_impact: low
---

# Architecture report: Archfit PR #33

## Executive summary

PR #33 is a useful contract-extraction step, but it does not yet implement the DDD architecture implied by its module map. The dependency graph is acyclic and the core ring is well guarded. The main risks are cohesion and change locality: a type-centric model archipelago surrounds a broad evaluation core, while the CLI and engine remain the dominant change hubs.

The declared Archfit coupling score rises from 40 to 54, but that number is not a source-only improvement. With the PR configuration held constant, Archfit reports 55 to 54. Plausible domain-volatility corrections lower the current score to 41-50. Source and configuration effects must be reported separately.

Vlad's independent review reached the same target direction: retain one modular monolith and organize around domain capabilities plus application services. Do not split into services.

## Interview context

Archfit is one owner, one CLI, and one deployable. Modularity should reduce the cost and spread of changes inside that deployable. Cross-package coupling is not automatically bad; high-strength relationships should be close, and distant relationships should cross narrow stable contracts.

The review treats design documents as intent. Code, imports, current graph data, and history decide whether that intent is implemented.

## System map

Observed execution:

```text
cmd/archfit
  -> runPipeline
  -> engine.Run
       -> evidence acquisition through ports
       -> relationship classification
       -> rules and metrics
       -> status and verdict
       -> scan aggregate assembly
  -> score.Synthesize
  -> decision.Build
  -> renderers
```

The repository has 65 Go packages and 22 declared modules. Go and module graphs have no cycles. There is one binary and one Dockerized distribution of the same CLI.

Largest observed units:

- `cmd/archfit`: 8775 production LOC, fan-out 18.
- `evaluation-core`: 6309 LOC.
- `pipeline-engine`: 2926 LOC, fan-out 11.
- `fact-adapters`: 8858 LOC across 16 packages.

## Intended architecture

The intended architecture is a hexagonal modular monolith: core analysis and policy do not import tool/process/output adapters, and command packages compose concrete adapters. The PR adds evidence, report, scan, coupling, finding, architecture, and pipeline-state modules.

The observed drift is that those model packages are separated by data shape, not owned with the behavior that changes them. The report contract exists, but renderers still consume the scan aggregate.

## Module volatility

The main label corrections to approve are:

- `coupling-model`: core/high, not medium.
- `architecture-model`: core/high, not supporting/medium.
- `finding-model`: inferred high while it changes with policy/evaluation behavior.
- `scan-contract`: at least medium until split; low is contradicted by its broad responsibility and change propagation.
- `evidence-model`: confirm core versus supporting. Its values are neutral, but its shared-kernel role creates high fan-in.

New-path churn undercounts moved types. Domain role is therefore the primary evidence; history only corroborates.

## Observed architecture

Healthy observations:

- No import or module cycles.
- Core-to-adapter direction is executable through tests and Archfit gates.
- LLM code remains off the deterministic gate path.
- Fact adapters already use ports and neutral evidence values.
- Byte-identical and model-surface tests protect external behavior.

Risk observations:

- `scan.Diagnostic` remains the broad pipeline/report dependency.
- Nineteen production files use the diagnostic facade.
- `runPipeline` and `engine.Run` span too many stages.
- `evaluation-core` still combines several change vectors.
- New module boundaries lack matching fitness rules.

## Score map

| Dimension | Score | Band | Confidence | Reason |
| --- | ---: | --- | --- | --- |
| boundary_integrity | 58 | mixed | high | Strong core/adapter rules, incomplete report/scan and compatibility boundaries. |
| coupling_balance | 50 | mixed | medium | Several local functional/model seams; score is sensitive to unconfirmed volatility labels. |
| dependency_graph_health | 76 | serviceable | high | No cycles and clear inward dependency direction, with large hubs remaining. |
| cohesion_modularity | 44 | mixed | high | Large CLI/engine/evaluation units and type-centric shared models. |
| change_locality | 48 | mixed | medium | Strong co-change across declared module boundaries; GitNexus is stale. |
| architecture_fitness | 70 | serviceable | high | Good executable rings and gates, but new contracts are not enforced. |
| analysis_confidence | 76 | serviceable | medium | Standard direct evidence; volatility and refreshed GitNexus remain gaps. |

These are Architect rubric scores. They are not Archfit's single `coupling_balance` dimension.

## Key findings

### F1 — Report boundary still exposes the full scan aggregate

The renderer port accepts `scan.Diagnostic`. This makes reporting aware of extraction, evaluation, runtime evidence, tasks, and deltas. Complete one `ReportDocument` projection and make output and persistence consume it.

### F2 — Type-centric modules do not define cohesive bounded contexts

The map separates DTO categories while keeping much of the behavior in `evaluation-core`. Models should live with the domain capability that owns their invariants. Prefer policy, relationship analysis, assessment/repair, application orchestration, and report projection.

### F3 — CLI and pipeline orchestration remain change hubs

Composition-root coupling to concrete adapters is expected. Product use-case policy in `cmd` is not. Move Analyze, Check, and Enrich orchestration behind application services. Keep only flag translation and concrete dependency construction in the command package.

### F4 — Volatility labels overstate score improvement

Declared score 54 is plausible only under the current labels. The same source under corrected core/inferred volatility scores 41-50. Approve a volatility ledger and hold it fixed during source experiments.

### F5 — Fitness checks lag the new map

Make purity checks pattern-based. Add rules for output-to-report, no production diagnostic imports, and command/application separation as each migration lands.

## Coupling review

### CLI to fact adapters

- Strength: functional, estimated 8. Registry constructs concrete adapters and includes Rust-specific lookup.
- Distance: code/package 2-4; same owner, runtime, and deploy distance 1.
- Volatility: supporting/provider medium, estimated 5.
- Balance: about 7. High strength is counterbalanced by proximity.
- Severity: low.
- Move: keep composition wiring; optionally concentrate it behind one adapter registry. Do not prioritize for score.

### Evaluation core to pipeline engine

- Strength: functional, estimated 8. Engine sequencing changes with evaluation stages.
- Distance: package/module 4; owner/runtime/deploy 1.
- Volatility: core/high inherited, estimated 9.
- Balance: about 5.
- Severity: medium-high local change-spread risk.
- Move: introduce stage input/output contracts and application services; do not create services.

### Evaluation core to evidence model

- Strength: broad model sharing, estimated 3.
- Distance: package/module 4; same owner/deploy.
- Volatility: core/inferred high, estimated 9.
- Balance: about 2.
- Severity: high low-cohesion risk.
- Move: translate neutral evidence into narrow relationship and policy inputs before evaluation.

### Fact adapters to evidence model

- Strength: model/contract, estimated 3.
- Distance: package/module 4; same owner/deploy.
- Volatility: supporting medium-low, estimated 4.
- Balance: about 7.
- Severity: low.
- Move: keep. This is the correct adapter contract.

### Rendering to scan contract

- Strength: broad model, estimated 3.
- Distance: package/module 4.
- Volatility: declared low gives balance 8; inferred medium/high gives 4-6.
- Severity: high boundary concern even when numeric balance is rescued by low volatility.
- Move: lower strength through a stable report document and remove scan from the renderer port.

## Archfit calibration

Archfit is useful for edge coverage and ranking, but the current score must be calibrated:

- Full branch-local comparison: 40 to 54.
- Fixed PR configuration source comparison: 55 to 54.
- Scan set to high: 49.
- Core model labels set high: 50.
- Scan plus core model labels high: 45.
- Broad core/inferred-high interpretation: 41.

The most credible current score range is therefore 41-50 until labels are approved. A target source refactor should be evaluated with labels held fixed.

## Boundary violations

No configured blocking dependency violation is active. The important unenforced drifts are:

- Renderers and renderer port depend on scan rather than report.
- Production diagnostic-facade imports remain.
- New model packages are missing from one hardcoded purity list.
- CLI owns application policy beyond composition.

## Change locality and hotspots

In a 468-commit, 180-day window:

- evaluation-core with pipeline-engine: 74 co-change commits;
- CLI with evaluation-core: 65;
- CLI with pipeline-engine: 57;
- CLI with fact adapters: 55;
- evaluation-core with fact adapters: 54;
- evaluation-core with policy language: 49.

This supports Candidate B: domain kernel plus application services. It does not support splitting every data type into a separate module.

## Recommendations

### Recommended target context map

```text
CLI composition
  -> Application use cases
       -> Evidence acquisition ports -> adapters
       -> Relationship analysis
            owns coupling model, classification, and relationship facts
       -> Architecture policy
            owns module/rule/gate semantics; YAML is an adapter
       -> Assessment and repair
            owns metrics, findings, score, status, verdict, recommendations
       -> Report projection
            produces immutable ReportDocument
                 -> renderers
                 -> baseline persistence
```

Keep one modular monolith. Reject service extraction.

### Options discussed with Vlad

1. Keep current map: lowest migration cost, insufficient cohesion.
2. Domain kernel plus application services: recommended.
3. Product-feature vertical slices: useful later, but risks duplicate pipelines now.
4. Strict ports-and-adapters: use as dependency direction, not as an interface framework.
5. Separate services: reject for one owner and one deployable.

### Estimated improvement

Source-first estimates, with configuration held fixed:

- Report projection and diagnostic retirement:
  - production diagnostic imports 19 to 0;
  - Archfit approximately +2 to +8;
  - boundary-integrity score approximately +8 to +15.
- Application facade and stage contracts:
  - CLI fan-out 18 to 10-14;
  - engine fan-out 11 to 7-9;
  - CLI LOC reduction 10-25%;
  - Archfit approximately -2 to +6 because new honest contract edges can lower the score.
- Cohesive relationship-analysis and assessment contexts:
  - evaluation-core split into 2-3 owned capabilities;
  - cohesion score approximately 44 to 60-70;
  - change-locality score can reach 55-65 only after history demonstrates local changes.

Do not sum Archfit ranges. A realistic honest coupling target is 50-62 after approved volatility corrections and source changes. Under the current favorable labels it may display 58-65. Structural acceptance criteria take precedence.

## Evidence appendix

Evidence IDs E1-E17 are defined in frontmatter. All commands are rerunnable from the repository root. GitNexus evidence was excluded because its index is stale for this branch.

## Tool coverage and gaps

Coverage is standard:

- Current: Go package graph, CodeGraph, Archfit, direct source reads, Go AST size analysis, git co-change, architecture tests.
- Stale: GitNexus.
- Missing: authoritative `architect-validate-report` binary.
- Limited: runtime/deploy evidence because Archfit is one CLI deployable.

## Next skill

Use `architecture-design`. The target contexts, ownership of data types, translation contracts, and fitness checks need explicit approval before implementation sequencing.
