---
artifact: architecture-report
schema_version: 2
rubric_version: 1
report_id: archfit-horizon3-final-review
date: 2026-08-24

target:
  repo: archfit
  scope: full
  out_of_scope: [implementation, config edits, waivers, baselines]

comparability:
  scope: full
  rubric_version: 1
  tool_coverage_level: deep

interview_context:
  system_goal: "Architecture-fitness CLI implemented as one Go modular monolith."
  quality_goals: [local change, explicit contracts, balanced coupling, deterministic analysis, stable exit codes, report/assessment separation]
  intended_units: [architecture-policy, relationship-analysis, assessment-repair, evidence-contracts, evidence-adapters, policy-config-adapter, analysis-application, report-contract, report-adapters, persistence-adapters, provider-adapters, cli-composition, architecture-tests]
  domains:
    core: [architecture policy, relationship analysis, assessment and repair]
    supporting: [evidence, application orchestration, report projection, persistence]
    generic: [provider adapters, CLI composition]
  volatile_areas: [policy language, relationship semantics, assessment rules and score]
  team_ownership: ["One owner declared for all .archfit.yaml modules: alexei-led"]
  known_pain: [cmd/archfit and internal/engine fan-out hubs, compatibility aliases, incomplete application boundary, coupling tail]
  review_scope: full
  out_of_scope: [service decomposition, team topology, production deployment changes]

system_map:
  languages: [Go]
  package_managers: [Go modules, go.work]
  units: [archfit CLI, calibrate development tool, internal Go packages, architecture tests]
  deploy_units: [single archfit binary]
  public_interfaces: [CLI commands and exit codes, report.Document, JSON/Markdown/SARIF/scorecard output, baseline and labels files]
  declared_modules: [evidence-model, relationship-analysis, architecture-model, report-contract, scan-contract, stage-views, policy-language, pipeline-contracts, assessment-repair, analysis-application, pipeline-engine, fact-adapters, config-lifecycle, baseline-store, labels-store, rendering, llm-adapter, archfit-cli, development-tools, architecture-tests]
  observed_modules: [cmd/archfit composition plus use-case and pipeline logic, internal/application report projection, internal/engine combined acquisition/classification/rules/metrics/assembly, assessment packages, relationship packages, model/evidence packages, extract adapters, output adapters, persistence adapters]
  high_risk_entrypoints: [cmd/archfit/analyze.go, cmd/archfit/pipeline_run.go, internal/engine/engine.go, internal/engine/assemble.go, internal/engine/advisories.go]
  missing_evidence: ["GitNexus and CodeGraph were refreshed against the working tree, but uncommitted changes have no historical co-change record; change-locality evidence remains partial.", "No independent team or runtime ownership topology beyond declared single owner/single binary."]

module_volatility:
  - module: architecture-policy
    classification: core
    volatility: high
    source: docs
    evidence_refs: [E1, E2]
    confidence: medium
    notes: "Current config names this capability policy-language; design explicitly classifies policy language as core/high."
  - module: relationship-analysis
    classification: core
    volatility: high
    source: docs
    evidence_refs: [E1, E2]
    confidence: high
    notes: "Approved design and .archfit.yaml agree."
  - module: assessment-repair
    classification: core
    volatility: high
    source: docs
    evidence_refs: [E1, E2]
    confidence: high
    notes: "Approved design and .archfit.yaml agree."
  - module: evidence-contracts
    classification: supporting
    volatility: medium
    source: docs
    evidence_refs: [E1, E2]
    confidence: high
    notes: "Neutral facts are supporting; archfit output reports declared medium and low values."
  - module: fact-adapters
    classification: supporting
    volatility: medium
    source: docs
    evidence_refs: [E1, E2]
    confidence: high
    notes: "External analyzers and acquisition are supporting implementation volatility."
  - module: analysis-application
    classification: supporting
    volatility: medium
    source: docs
    evidence_refs: [E1, E2]
    confidence: medium
    notes: "The target role is clear, but current code has only report projection here."
  - module: report-contract
    classification: supporting
    volatility: medium
    source: docs
    evidence_refs: [E1]
    confidence: high
    notes: "External schema is compatibility-controlled."
  - module: archfit-cli
    classification: generic
    volatility: medium
    source: docs
    evidence_refs: [E1, E2]
    confidence: high
    notes: "Composition root is generic/medium, but current command package also owns use-case logic."

scores:
  boundary_integrity:
    value: 54
    band: mixed
    confidence: medium
    evidence_refs: [E1, E2, E3, E4, E5, E6]
    gaps: ["Target checks for CLI-only composition, application-only orchestration, and report-adapter import surface are not fully enforced."]
  coupling_balance:
    value: 46
    band: mixed
    confidence: high
    evidence_refs: [E7, E8, E9, E10]
    gaps: ["Archfit's 46 is one edge-weighted coupling dimension, not an overall architecture score."]
  dependency_graph_health:
    value: 67
    band: serviceable
    confidence: medium
    evidence_refs: [E7, E11, E12]
    gaps: ["GitNexus history graph is stale for working-tree changes; CodeGraph status is current but no independent historical graph was used."]
  cohesion_modularity:
    value: 49
    band: mixed
    confidence: medium
    evidence_refs: [E3, E13, E14]
    gaps: ["Engine and CLI responsibilities are still mixed; no independent complexity tool beyond LOC and deterministic blast-radius metrics."]
  change_locality:
    value: 48
    band: mixed
    confidence: medium
    evidence_refs: [E15, E16, E17]
    gaps: ["Current structural impact is indexed, but uncommitted changes have no historical co-change record."]
  architecture_fitness:
    value: 67
    band: serviceable
    confidence: medium
    evidence_refs: [E3, E4, E5, E6, E18]
    gaps: ["Several design fitness checks remain documented recommendations rather than failing automated checks."]
  analysis_confidence:
    value: 72
    band: serviceable
    confidence: medium
    evidence_refs: [E7, E11, E12, E15, E18]
    gaps: ["Historical co-change coverage for the uncommitted delta and semantic ownership of some coupling labels remain incomplete."]

findings:
  - id: F1
    title: CLI still owns volatile use-case and pipeline wiring
    severity: high
    dimension: boundary_integrity
    evidence_refs: [E1, E3, E4, E7, E9]
    narrative:
      problem: "The target says CLI composition should keep flags, wiring, and exit translation while use-case logic belongs in Analysis Application. The current cmd/archfit production package imports assessment, engine, extract, baseline, relationship, and output internals across 16 files."
      knowledge_or_boundary_leakage: "cmd/archfit/analyze.go and pipeline_run.go interpret score, decision, baseline, extractor, labels, ownership, and pipeline internals instead of delegating a typed use case."
      complexity_impact: "A rule, extractor, score, or report change can require edits in command files. The CLI is at the 14-module fan-out ceiling, so the next cross-cutting feature has no ratchet headroom."
      cascading_change_scenarios:
        - "Changing score synthesis requires cmd/archfit/analyze.go, pipeline_run.go, baseline/config-compare, and render selection review."
        - "Adding an adapter or coverage family requires command-package wiring rather than one application port/registry change."
      recommended_improvement: "Move runPipeline, score/gate/task assembly, and command-independent coverage decisions behind Analysis Application services; leave command structs, flag translation, concrete wiring, rendering selection, and exit translation in cmd."
      tradeoffs: "This is an incremental extraction of existing seams, not a new framework. Preserve the current pipeline and golden outputs while moving one use case at a time."
    recommended_action: "Extract Analyze/Check request-result services from cmd/archfit before adding another cross-cutting command."
  - id: F2
    title: Engine is a combined domain pipeline, not orchestration only
    severity: high
    dimension: cohesion_modularity
    evidence_refs: [E1, E2, E5, E6, E13, E14]
    narrative:
      problem: "The design assigns internal/engine to Analysis Application and says it should sequence stages through stable contracts. The observed Run executes extraction, rule evidence acquisition, relationship classification, rule evaluation, status assignment, metrics, advisory construction, verdict, and report-fact assembly."
      knowledge_or_boundary_leakage: "internal/engine/engine.go imports assessment, relationship, evidence, report, scope, ports, and views, and its Run method owns stages 1-9. internal/engine/assemble.go and advisories.go also contain relationship and repair/report semantics."
      complexity_impact: "The 640-line engine and 1,205-line assemble file are change hubs. A relationship or assessment change crosses the orchestration package, increasing volatility and making the engine a second domain owner."
      cascading_change_scenarios:
        - "Adding a relationship evidence type changes engine stage state, assembly, report result construction, and tests."
        - "Changing assessment verdict/task semantics requires engine changes even though the target assigns those semantics to Assessment and Repair."
      recommended_improvement: "Introduce narrow immutable RelationshipSet and AssessmentResult inputs/outputs, then move classification assembly to Relationship Analysis and rule/metric/verdict assembly to Assessment and Repair; keep engine as a thin stage coordinator."
      tradeoffs: "Keep the current single process and ports. Split by responsibility, not by service or generic interfaces."
    recommended_action: "Separate engine stage coordination from relationship and assessment behavior with characterization tests first."
  - id: F3
    title: Assessment result still depends on report-domain types and compatibility aliases
    severity: medium
    dimension: boundary_integrity
    evidence_refs: [E1, E19, E20]
    narrative:
      problem: "The target says assessment owns MetricResult, Scorecard, Verdict, Decision, tasks, and deltas, while report-contract owns JSON-tagged external views. assessment/result still imports internal/model/report and aliases MetricResult, Verdict, Direction, summaries, and deltas to report types."
      knowledge_or_boundary_leakage: "The supposed internal assessment result is coupled to the external report model at lines 7, 11, 85-113, and 135-140 of internal/assessment/result/result.go."
      complexity_impact: "A report schema change can force assessment recompilation and compatibility edits; the projection layer is explicit for findings but not for most result fields."
      cascading_change_scenarios:
        - "Changing report metric or verdict JSON shape requires internal assessment aliases to remain compatible."
        - "Adding an assessment-only field risks leaking into report.Document because the two models share types."
      recommended_improvement: "Move assessment-owned value types into assessment/repair, retain report-only DTOs in report-contract, and make ProjectReport the complete translation boundary."
      tradeoffs: "This is a larger compatibility migration than F4; preserve JSON fixtures and aliases only during a bounded migration, then delete them."
    recommended_action: "Finish the result-to-report projection by removing assessment aliases to report-domain values."
  - id: F4
    title: Report adapter still imports relationship scoring for one version constant
    severity: medium
    dimension: coupling_balance
    evidence_refs: [E8, E21, E22]
    narrative:
      problem: "Archfit identifies rendering -> relationship-analysis as a critical model-strength edge (score 2), but internal/output/jsonout.go uses relationship/coupling only for ScoreVersion."
      knowledge_or_boundary_leakage: "A renderer knows the relationship scorer package instead of consuming score metadata from report-contract or a stable assessment projection."
      complexity_impact: "A scorer package move or formula-version change can touch report adapters, although rendering should remain stable."
      cascading_change_scenarios:
        - "Moving the scorer or changing its package path requires JSON renderer changes unrelated to formatting."
        - "A report consumer can see a version constant coupled to implementation location rather than report schema metadata."
      recommended_improvement: "Place the public score-version metadata in the stable report/assessment contract and have jsonout consume that value; keep scorer implementation private to relationship analysis."
      tradeoffs: "Small source refactor with fixture verification. Do not relabel volatility, waive the edge, or baseline it."
    recommended_action: "Move ScoreVersion metadata to a stable contract and remove the renderer's relationship import."
  - id: F5
    title: Archfit dogfooding is operational but not complete as a target architecture gate
    severity: high
    dimension: architecture_fitness
    evidence_refs: [E2, E3, E4, E5, E6, E7, E18]
    narrative:
      problem: "The branch-built check runs and has zero configured gate findings, but it exits 2 because 87 advisory warnings remain. More importantly, current gates do not enforce all approved target constraints."
      knowledge_or_boundary_leakage: "The config enforces core/adapters/layer restrictions, model purity, ratchets, API ceiling, and type-leak advisory. It does not enforce CLI composition-only imports, application-only orchestration, relationship-to-assessment direction, or report-adapter-only contract imports."
      complexity_impact: "A green gate can coexist with the exact F1-F3 drift. New code can add semantic boundary leakage without tripping a configured fail rule."
      cascading_change_scenarios:
        - "A new command can import an assessment evaluator and remain gate-clean."
        - "A renderer can import relationship internals for formatting and remain gate-clean unless it hits the narrow score/decision rules."
      recommended_improvement: "Add executable import rules or package-level tests for the approved dependency direction only after the physical seams are repaired; keep advisory BC output separate from gate verdicts."
      tradeoffs: "More gates increase signal only if selectors match real modules. Add one rule per repaired seam and retain warn mode for candidate surfacing before fail."
    recommended_action: "NO-GO for claiming the full dogfooding target complete; extend fitness checks after F1-F3 repairs."

archfit_calibration:
  source_commands:
    - "./.bin/archfit doctor"
    - "./.bin/archfit analyze --config .archfit.yaml --root /Users/alexei/Workspace/archfit --format json > /tmp/task43-analyze.json"
    - "./.bin/archfit check --config .archfit.yaml --root /Users/alexei/Workspace/archfit --format json > /tmp/task43-check-current.json"
    - "jq summary/score/metrics/findings from /tmp/archfit-task42-check.json and /tmp/archfit-task42-delta.json"
  artifacts: [/tmp/archfit-task42-check.json, /tmp/archfit-task42-delta.json, /tmp/task43-analyze.json, /tmp/task43-check-current.json]
  confirmed:
    - "389/389 internal edges scored; 57 critical-band edges and mean coupling balance 5.1/10 map to the reported 46 mixed coupling dimension."
    - "Archfit cycle metric is 0, Go package graph has 71 packages and no duplicate dependency cycle output, and architecture import tests pass."
    - "Archfit coverage reports SCIP, SCIP symbols, go/packages, LOC, jscpd, syntax, and ast-grep passes as current for this scan; Go test and vet pass."
    - "The current check has 0 gate findings and 87 advisory findings; this is a warning result, not a clean pass."
  severity_adjusted:
    - "analysis-application/report.go -> assessment/finding is an expected translation seam, but remains model-strength coupling until a narrow AssessmentResult contract is used; treat as medium rather than archfit critical."
    - "output/jsonout.go -> relationship/coupling is a one-constant dependency; treat as medium and fix by moving metadata, not by changing labels."
    - "CLI -> assessment and engine edges are real and high-impact because command code interprets domain internals; composition-root status lowers distance but does not erase the intended-boundary drift."
  false_positive_or_noise:
    - "No claim that all 87 advisories are independent defects: grouped BC findings include expected close collaboration and one report-projection edge."
    - "The overall archfit score is not an architecture scorecard; 46 is used only as coupling_balance evidence."
  missed_by_archfit:
    - "Semantic responsibility drift: engine.Run performs domain behavior, not only orchestration."
    - "CLI composition-only intent and the completeness of assessment/report projection."
    - "Compatibility aliases that preserve report types inside assessment."
    - "GitNexus freshness relative to uncommitted changes."
  config_changes: []
  new_fitness_checks:
    - "Fail when cmd/archfit production code imports assessment evaluator, classifier, score, or decision packages outside a composition adapter allowlist."
    - "Fail when internal/engine owns rule, relationship, verdict, or report assembly after the split; keep only stage coordination imports."
    - "Fail when report adapters import relationship, assessment, evidence internals, engine, scan, or diagnostic packages; permit report-contract and format-local helpers."
    - "Fail when assessment/result imports report-domain types after the projection migration."
  labels_to_confirm:
    - "Confirm whether relationship/coupling imports from language adapters are contract vocabulary or derived relationship behavior; the current TypeScript extractor uses StrengthContract/StrengthFunctional at internal/extract/ts/ts.go:555-557."
  confidence_impact: medium

evidence:
  - id: E1
    type: file
    ref: docs/design/20260823-archfit-capability-map.md:108-185
    summary: "Approved target module responsibilities, ownership migration, dependency direction, and forbidden directions."
  - id: E2
    type: file
    ref: .archfit.yaml:1-18
    summary: "Declared single-owner modular-monolith rationale, domain volatility, and score interpretation."
  - id: E3
    type: file
    ref: .archfit.yaml:330-569
    summary: "Configured fail gates cover core/adapters/layers/model purity and selected report/persistence edges; API/type checks are warn."
  - id: E4
    type: file
    ref: internal/arch_test.go:105-160
    summary: "Fan-out ratchets, diagnostic/scan import ratchets, and current thresholds."
  - id: E5
    type: file
    ref: internal/arch_test.go:191-380
    summary: "Current enforced import-ring, adapter, report, labels, model-purity, and view-kernel tests."
  - id: E6
    type: file
    ref: internal/engine/engine.go:198-390
    summary: "Run owns extraction, rule evidence, relationship analysis, rules, status, metrics, advisories, verdict, and result assembly."
  - id: E7
    type: command
    command: "./.bin/archfit analyze --config .archfit.yaml --root /Users/alexei/Workspace/archfit --format json > /tmp/task43-analyze.json; jq summary/score/metrics/tool_coverage from /tmp/task43-analyze.json"
    summary: "Current deterministic scan: coupling_balance 46 mixed; 389 scored edges; 57 critical edges; cycle 0; coverage 100%; five blast-radius hubs; 87 warnings."
  - id: E8
    type: command
    command: "jq -r '.findings[] | select(.rule_id==\"bc/imbalanced_coupling\" and (.severity==\"critical\" or .severity==\"high\")) | [.severity,.edge.from.module,.edge.from.path,.edge.to.module,.edge.to.path,.matched_by.strength,.matched_by.score_value,.matched_by.cheapest_move] | @tsv' /tmp/task43-check-current.json"
    summary: "Top calibrated coupling edges include CLI->assessment/result, engine->coupling, output/jsonout->coupling, baseline->coupling/status, and fact-adapters->coupling."
  - id: E9
    type: command
    command: "python3 module-fanout script over go list -json ./... and .archfit.yaml"
    summary: "Observed cmd/archfit fan-out is 14 modules (at the ratchet ceiling); engine resolves to 8 direct modules and passes the threshold of 9."
  - id: E10
    type: command
    command: "./.bin/archfit check --config .archfit.yaml --root /Users/alexei/Workspace/archfit --format json > /tmp/task43-check-current.json"
    summary: "Exit 2 due warnings; verdict warn; 0 gate findings; 87 advisory findings; coupling score remains 46 mixed."
  - id: E11
    type: command
    command: "go list -json ./...; go list -deps ./... | sort | uniq -d"
    summary: "71 Go packages, 617 listed imports, no duplicate dependency cycle output, no package load errors."
  - id: E12
    type: command
    command: "codegraph status /Users/alexei/Workspace/archfit"
    summary: "CodeGraph reports current index status with 7,879 nodes and 24,236 edges; used as current structural coverage, not historical change evidence."
  - id: E13
    type: command
    command: "find cmd/archfit internal/engine internal/assessment internal/relationship -name '*.go' ! -name '*_test.go' -print0 | xargs -0 wc -l | sort -nr | head -15"
    summary: "Production hotspots include internal/engine/assemble.go 1,205 LOC, relationship/classify.go 994 LOC, engine.go 640 LOC, and several 400-800 LOC command files."
  - id: E14
    type: command
    command: "jq '.metrics[] | select(.name==\"blast_radius\" or .name==\"cycle\")' /tmp/task43-analyze.json"
    summary: "Archfit reports 5 of 71 change-impact hubs and 0 import cycles."
  - id: E15
    type: command
    command: "git status --short --branch; git diff HEAD --numstat"
    summary: "Working tree has 131 changed paths, 1,042 insertions, 977 deletions; design and source are being reviewed uncommitted."
  - id: E16
    type: command
    command: "gitnexus analyze .; gitnexus status; gitnexus detect-changes --repo archfit"
    summary: "GitNexus refreshed successfully (9,242 nodes, 33,431 edges, 375 clusters, 300 flows) and mapped 125 changed files; uncommitted changes still lack historical co-change observations."
  - id: E17
    type: command
    command: "git log --follow --oneline -- internal/engine/engine.go; git log --follow --oneline -- cmd/archfit/main.go"
    summary: "Direct history confirms engine and CLI are long-lived change hubs, but cannot replace a fresh co-change graph for the current uncommitted scope."
  - id: E18
    type: command
    command: "go test ./internal/ -run 'Test(ArchImports|CompositionFanoutRatchet|DiagnosticProductionImportRatchet|ScanProductionImportRatchet|CouplingContractDoesNotImportGraph)$' -count=1; go test ./...; go vet ./...; git diff --check"
    summary: "All selected architecture tests, full Go tests, vet, and whitespace checks pass."
  - id: E19
    type: file
    ref: internal/assessment/result/result.go:1-140
    summary: "Assessment result imports report and aliases report-owned metric, verdict, summary, and delta values."
  - id: E20
    type: file
    ref: internal/application/report.go:9-45
    summary: "Projection translates assessment findings into report findings, proving partial but incomplete separation."
  - id: E21
    type: file
    ref: internal/output/jsonout/jsonout.go:58-65
    summary: "JSON renderer consumes relationship/coupling.ScoreVersion while otherwise accepting report.Document."
  - id: E22
    type: file
    ref: internal/baseline/baseline.go:49-119
    summary: "Baseline persistence imports relationship scorer metadata and report rubric metadata, illustrating a second stable-contract leak."

tool_coverage:
  - dimension: discovery
    tools_used: [fd/find, rg, git status, git diff, targeted file reads]
    tools_skipped: []
    tools_missing: []
    tools_failed: []
    confidence_impact: none
  - dimension: structural
    tools_used: [go list -json, internal architecture tests, CodeGraph status, LOC aggregation]
    tools_skipped: [ast-grep independent replay]
    tools_missing: []
    tools_failed: []
    confidence_impact: low
  - dimension: semantic
    tools_used: [go test, go vet, go/packages through architecture tests]
    tools_skipped: [full LSP reference walk]
    tools_missing: []
    tools_failed: []
    confidence_impact: low
  - dimension: dependency
    tools_used: [archfit SCIP/go/packages, go list, CodeGraph]
    tools_skipped: [dependency-cruiser and grimp because Go-only target]
    tools_missing: []
    tools_failed: []
    confidence_impact: none
  - dimension: change
    tools_used: [git status, git diff stat, git log --follow, GitNexus analyze/status/detect-changes]
    tools_skipped: []
    tools_missing: []
    tools_failed: ["An initial GitNexus detect-changes call omitted the repo selector; the corrected archfit-scoped call succeeded."]
    confidence_impact: medium
  - dimension: operational
    tools_used: [archfit doctor, deploy-unit coverage in archfit]
    tools_skipped: [service/deployment topology scan beyond single binary]
    tools_missing: ["No separate deployment unit; not applicable to this modular monolith."]
    tools_failed: []
    confidence_impact: low
  - dimension: report
    tools_used: [git diff --check]
    tools_skipped: [architect-validate-report unavailable in this runtime]
    tools_missing: [architect-validate-report]
    tools_failed: []
    confidence_impact: none

---

# Architecture report: archfit

## Executive summary

**Verdict: NO-GO for the claim that the approved target architecture and dogfooding target are complete.** The branch is buildable and operationally self-checking: full Go tests, vet, architecture tests, and the branch-built Archfit scan pass without configured gate findings. It is not architecturally complete. The CLI still owns use-case/pipeline wiring, the engine still owns relationship and assessment behavior, assessment results still alias report-domain types, and the configured dogfood gates do not enforce those semantic boundaries. The deterministic coupling dimension is 46/mixed with high confidence, based on 389/389 classified internal edges; it is not an overall architecture score. Review confidence is medium-high: CodeGraph and GitNexus are refreshed for current structure, while uncommitted changes still lack historical co-change observations.

## System map

Observed flow:

```text
cmd/archfit (flags, use cases, wiring, rendering, exit codes)
  -> internal/engine (extract -> classify -> assess -> assemble)
       -> internal/extract/* and internal/toolrun through ports
       -> internal/relationship/*
       -> internal/assessment/*
       -> internal/model/report and internal/view
  -> internal/application/report.go -> internal/model/report.Document
  -> internal/output/{console,jsonout,markdown,sarif,scorecard}
  -> baseline/labels persistence
```

The intended flow separates CLI Composition, Analysis Application, Relationship Analysis, Assessment and Repair, Report Projection, and Report Adapters (E1). The observed flow has the same package names but not the same responsibility boundaries.

## Intended architecture

The current design document is approved for implementation and explicitly says it is intent, not implementation truth (E1). It requires one owner, one process, one deployable; narrow contracts; application orchestration; report projection; no relationship-to-assessment dependency; and CLI composition without domain interpretation. `.archfit.yaml` agrees on single-owner modules, core/supporting/generic classification, and layer direction (E2). The key disagreement is not between documents: it is between those intended boundaries and current code.

## Observed architecture

Positive evidence:

- `internal/model/report.Document` is now a separate renderer input and `internal/application/report.go` performs an explicit finding/task projection (E20).
- Production imports of `internal/model/diagnostic` are zero; architecture ratchet tests pass (E4, E5, E18).
- Core ring, model purity, labels I/O isolation, report-contract no-finding import, and CLI/engine fan-out thresholds are enforced and currently pass (E5, E18).
- Go dependency loading is healthy: 71 packages, no package-load errors, no cycles detected by the selected checks (E11, E14).

Remaining drift:

- `cmd/archfit` production files import 14 declared modules, including assessment implementation, relationship implementation, engine, extractors, persistence, and renderers (E7, E9).
- `internal/engine.Run` performs all nine pipeline stages, including domain classification, rule evaluation, metrics, advisories, verdict, and result assembly (E6).
- `internal/assessment/result` is an assessment/report hybrid through direct report imports and aliases (E19).
- `internal/output/jsonout` and `internal/baseline` still bind to relationship scorer metadata (E21, E22).
- The largest production hotspots remain engine assembly and command files (E13), and archfit reports five of 71 change-impact hubs (E14).

## Score map

- **boundary_integrity: 54, mixed, medium confidence.** Ring rules pass, but CLI/application/engine/report semantic boundaries remain partly aspirational.
- **coupling_balance: 46, mixed, high confidence.** 389 edges are classified and scored; the tail has 57 critical edges. This is one edge-weighted dimension only.
- **dependency_graph_health: 67, serviceable, medium confidence.** No cycles and healthy package loading; hubs and stale history coverage prevent a stronger claim.
- **cohesion_modularity: 49, mixed, medium confidence.** Engine and command packages mix orchestration, domain behavior, adapters, and projection; large hotspots remain.
- **change_locality: 48, mixed, medium confidence.** Refreshed graph impact and direct history show broad CLI/engine change scope; uncommitted changes still lack historical co-change observations.
- **architecture_fitness: 67, serviceable, medium confidence.** Real failing import/ratchet checks exist, but several approved target constraints have no executable gate.
- **analysis_confidence: 72, serviceable, medium confidence.** Current static, semantic, deterministic, CodeGraph, and GitNexus structural evidence is broad; historical co-change evidence for the uncommitted delta remains the material gap.

## Key findings

### F1 — High: CLI still owns volatile use-case and pipeline wiring

See E1, E3, E4, E7, E9. The CLI is exactly at the 14-module fan-out ceiling and imports assessment, relationship, engine, extract, persistence, and output internals. The smallest safe repair is to extract typed Analyze/Check/Explain/Enrich services into Analysis Application and leave only flag translation, concrete wiring, renderer choice, and exit translation in `cmd/archfit`. Do this incrementally with existing golden and exit-code tests.

### F2 — High: Engine is a combined domain pipeline

See E1, E2, E5, E6, E13, E14. `internal/engine` is not orchestration only. It owns extraction, classification, rule semantics, metrics, advisory wording, verdict, and result assembly. Split by existing stage seams: relationship analysis returns an immutable RelationshipSet; assessment returns AssessmentResult; engine sequences them. Do not create services or a generic interface framework.

### F3 — Medium: Assessment/report separation is incomplete

See E1, E19, E20. Projection exists for findings and tasks, but assessment/result aliases report-owned metric, verdict, summary, and delta values. Complete the explicit projection, preserve report fixtures, then delete migration aliases. This is real boundary drift, not a formatting concern.

### F4 — Medium: One-constant renderer coupling

See E8, E21. `jsonout` imports relationship/coupling only for `ScoreVersion`. Move score-version metadata to a stable contract. This is the smallest source refactor that improves coupling balance without changing volatility, distance labels, waivers, or baselines.

### F5 — High: Dogfooding target is not complete

See E2-E7 and E18. Configured dogfooding is operational, but a check result of `warn`, exit 2, and 87 advisories is not a clean target completion signal. More importantly, semantic target constraints are not gated: CLI composition-only imports, engine orchestration-only behavior, relationship/assessment direction, and report-adapter-only imports. Add gates after the corresponding physical seams exist. Do not hide current findings with waivers or baselines.

## Coupling review

| Relationship | Strength | Distance | Volatility | Balance and judgment | Smallest move |
|---|---|---|---|---|---|
| `cmd/archfit` -> `assessment-repair` (`analyze.go`, `pipeline_run.go`) | model/functional; E8 | Separate packages, same owner/process/deploy; code distance is cross-module | High on assessment; medium on CLI | Unbalanced tail; archfit critical edges are real because CLI interprets assessment internals | Lower strength by moving use-case/score assembly to application services |
| `internal/engine` -> `relationship-analysis` (`engine.go:21-24`) | model; E8 | Separate capability modules, same owner/process/deploy | High relationship semantics; medium engine | Critical tail; engine owns relationship orchestration and assembly | Lower strength with RelationshipSet contract; keep close runtime collaboration |
| `internal/engine` -> `assessment-repair` (`engine.go:9-14`) | model/functional; E8 | Separate capability modules, same owner/process/deploy | High assessment semantics; medium engine | Critical/high tail; engine is a second assessment owner | Lower strength with AssessmentResult contract |
| `internal/application/report.go` -> `assessment/finding` | model; E8, E20 | Projection package to core package, same owner/process/deploy | High assessment, medium report | Archfit critical is severity-adjusted to medium because translation is expected; contract is still too broad | Translate from narrow AssessmentResult and keep finding internals private |
| `internal/output/jsonout` -> `relationship/coupling` | model by import classifier; E8, E21 | Renderer to core package, same owner/process/deploy | High scorer implementation, medium report | Archfit critical is severity-adjusted to medium: one metadata constant, not a distributed-monolith seam | Move ScoreVersion to report/assessment contract |
| `internal/extract/ts` -> `relationship/coupling` | model vocabulary; E8, E22 | Adapter to core package, same owner/process/deploy | Medium adapter, high relationship semantics | Structural leak; strength labels are being emitted from an adapter | Move neutral strength-hint vocabulary to evidence-contracts, or expose a narrow adapter-owned enum mapping |
| `internal/baseline` -> `relationship/coupling` and `assessment/status` | model; E8, E22 | Persistence adapter to core packages, same owner/process/deploy | Low persistence, high core | Medium latent risk; storage format binds to scorer/status implementation | Define persistence-owned snapshot/accepted-entry DTOs and translate at the boundary |

All relationships are assessed at Go package/capability level. Ownership is intentionally same-owner in the design and config; runtime/deploy distance is one process/binary. No distance or volatility relabeling is recommended.

## Archfit calibration

The deterministic scan is useful and current for the working tree. It scores 389 internal edges, all classified, with 57 critical-band edges; it reports 0 cycles and 100% primary coverage for the applicable Go target. The score 46 is retained only as the `coupling_balance` dimension. The 87 warnings are mostly grouped BC advisories plus a syntax API ceiling warning; they are not 87 independent architecture blockers.

The strongest calibrations are F1/F2/F3: Archfit sees imports and edge scores but not intended responsibility. It therefore cannot detect that `engine.Run` is a domain hub or that assessment/report aliases undermine the projection boundary. Conversely, expected projection edges and the one-constant renderer dependency are real but severity-adjusted. No config changes, waivers, baselines, or volatility/distance relabeling are recommended.

## Boundary violations and fitness status

Existing enforced checks: core-ring adapter bans; model purity; view kernel restriction; labels I/O isolation; adapter-to-engine ban; report contract no-finding import; diagnostic and scan import ratchets; CLI/engine fan-out ratchets; layer inversion; full Go architecture tests. These all pass (E5, E18).

Recommended but not yet enforced: CLI composition-only import allowlist; engine orchestration-only dependency/ownership check; report adapter import allowlist; relationship/assessment direction; no assessment-to-report-domain imports. Documentation alone does not raise architecture fitness (E1, E3, E5).

## Change locality and hotspots

The uncommitted review scope is 131 changed paths (+1,042/-977), including staged rename entries and additional unstaged modifications. This is broad for a capability-boundary migration but expected for the current tasks. Direct `git log --follow` shows engine and CLI as persistent hubs. GitNexus was refreshed and mapped the working-tree impact, but uncommitted changes have no historical co-change record; that remains a coverage gap and prevents a strong locality claim. The current local coupling risk is concentrated in command orchestration, engine assembly, and shared evidence/report types.

## Smallest real refactors to improve balance

1. Move `coupling.ScoreVersion` metadata to `internal/model/report` or an assessment-owned stable score contract; update `jsonout` and baseline callers. This removes implementation-package edges without relabeling.
2. Move neutral strength-hint constants used by language extractors to evidence-contracts; relationship analysis maps them to its private scoring vocabulary. This lowers adapter-to-core knowledge strength.
3. Extract one application service for Analyze/Check around the existing `runPipeline`; move only request/result and score/task assembly first. This reduces CLI fan-out and semantic leakage without changing the pipeline.
4. Split engine result assembly into `relationship-analysis` and `assessment-repair` functions returning narrow immutable contracts. Keep orchestration in engine until both seams are characterized.
5. Replace assessment/result aliases to report types with explicit assessment values and one `ProjectReport` mapping. Preserve all output fixtures before deleting aliases.

Do not split modules solely to improve a number. Do not change volatility or distance labels. Do not add waivers or baselines. Do not rewrite the monolith as services.

## GO/NO-GO and next recommendation

- **GO:** build/test/verification posture. `go test ./...`, `go vet ./...`, selected architecture ratchets, and `git diff --check` pass. Configured Archfit gates report zero gate findings.
- **NO-GO:** final architecture completion and dogfooding-complete claim. F1-F5 remain.
- **Next primary skill:** `architecture-plan`, using this approved design and findings F1-F5, with incremental implementation steps and postconditions. After implementation, run a comparable `architecture-review` with a refreshed GitNexus index or direct-history coverage explicitly recorded.

## Evidence appendix

See frontmatter E1-E22. The most important rerunnable commands are E7-E12 and E18. No production source or repository config was modified by this review.

## Tool coverage and gaps

Local discovery, Go dependency loading, CodeGraph status, Archfit deterministic analysis, architecture tests, full tests, vet, and diff checks were used. GitNexus and CodeGraph were refreshed and used for current structural impact; historical co-change claims remain limited for the uncommitted delta. Dependency-cruiser/grimp were not run because the target production system is Go-only. No fresh historical graph was available, so change-locality confidence is low.

```acceptance-report
{
  "criteriaSatisfied": [
    {
      "id": "criterion-1",
      "status": "satisfied",
      "evidence": "The report contains findings F1-F5 with severity, concrete file paths, line references, deterministic command evidence, residual risks, and an explicit GO/NO-GO judgment."
    }
  ],
  "changedFiles": [],
  "testsAddedOrUpdated": [],
  "commandsRun": [
    {
      "command": "go test ./...",
      "result": "passed",
      "summary": "All Go packages passed."
    },
    {
      "command": "go vet ./...",
      "result": "passed",
      "summary": "No vet findings."
    },
    {
      "command": "go test ./internal/ -run 'Test(ArchImports|CompositionFanoutRatchet|DiagnosticProductionImportRatchet|ScanProductionImportRatchet|CouplingContractDoesNotImportGraph)$' -count=1",
      "result": "passed",
      "summary": "Architecture and ratchet tests passed."
    },
    {
      "command": "./.bin/archfit check --config .archfit.yaml --root /Users/alexei/Workspace/archfit --format json",
      "result": "failed",
      "summary": "Exit 2 because 87 advisory warnings were emitted; gate_findings is 0 and verdict is warn."
    },
    {
      "command": "git diff --check",
      "result": "passed",
      "summary": "No whitespace errors."
    },
    {
      "command": "gitnexus status",
      "result": "passed",
      "summary": "GitNexus was refreshed and mapped current working-tree impact; historical co-change evidence remains unavailable until changes are committed."
    }
  ],
  "validationOutput": [
    "Archfit analyze: coupling_balance 46/mixed, 389 scored internal edges, 57 critical-band edges, cycle 0, 100% applicable coverage, 5 blast-radius hubs.",
    "Current branch has 131 changed paths; the independent review made no source changes."
  ],
  "residualRisks": [
    "CLI remains at the 14-module fan-out ceiling and still imports domain internals.",
    "internal/engine remains a combined acquisition/classification/assessment/assembly hub.",
    "Assessment result aliases report-domain values.",
    "Dogfooding gates do not yet enforce all approved semantic boundaries.",
    "Uncommitted changes have current structural impact coverage but no historical co-change record."
  ],
  "noStagedFiles": false,
  "diffSummary": "Independent review was read-only. Existing branch contains 131 changed paths across the implementation and documentation refactor.",
  "reviewFindings": [
    "high: cmd/archfit/analyze.go:12-24 and cmd/archfit/pipeline_run.go:12-28 - CLI owns use-case and adapter wiring across 14 modules.",
    "high: internal/engine/engine.go:198-390 - engine owns extraction, relationship, assessment, verdict, advisory, and result assembly rather than orchestration only.",
    "medium: internal/assessment/result/result.go:4-140 - assessment result aliases report-domain types.",
    "medium: internal/output/jsonout/jsonout.go:7-8,58-65 - renderer imports relationship scorer for one version constant.",
    "high: .archfit.yaml:330-569 - configured dogfood gates omit several approved semantic boundary constraints; check emits 87 warnings and exits 2."
  ],
  "manualNotes": "Final target status is NO-GO despite operational GO for tests and configured gates. Next primary skill is architecture-plan."
}
```