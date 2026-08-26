# Archfit architecture baseline

Date: 2026-08-26
Status: IMPLEMENTED — this describes the source tree as it is, not a target.
Branch: `refactor/capability-contract-boundaries` (PR #33), measured against
`main`.

This is the maintenance reference for archfit's own architecture. The design
rationale lives in [the capability map](20260823-archfit-capability-map.md);
this document records what shipped, which rules enforce it, what coupling is
deliberately accepted, and how to make a change without eroding the boundary.

## System map

Six capability layers, innermost first. `.archfit.yaml` declares them under
`layers:` and `forbidden_layer_direction` fires whenever an inner layer imports
an outer one.

| Layer | Rank | Modules | Packages |
| --- | --- | --- | --- |
| `model` | 0 | `evidence-contracts`, `report-contract` | `internal/model/{evidence,graph,symbol,fileclass,clone,pattern,report}`, `internal/evidence`, `internal/report/ports` |
| `support` | 1 | `analysis-scope` | `internal/scope` |
| `core` | 2 | `architecture-policy`, `relationship-analysis`, `assessment-repair`, `evidence-analysis` | `internal/policy`, `internal/relationship/**`, `internal/assessment/**`, `internal/syntax` |
| `application` | 3 | `analysis-application` | `internal/application` |
| `adapter` | 4 | `policy-config-adapter`, `evidence-acquisition`, `evidence-adapters`, `persistence-adapters`, `provider-adapters`, `report-adapters`, `config-lifecycle` | `internal/config`, `internal/evidence/acquisition`, `internal/extract/**`, `internal/toolrun`, `internal/evidence/ports`, `internal/{factcache,history,ownership,baseline,labels}`, `internal/llm`, `internal/output/**`, `internal/{initcfg,configschema}` |
| `cmd` | 5 | `cli-composition`, `development-tools`, `architecture-tests` | `cmd/archfit`, `cmd/calibrate`, `internal/calibrate`, `scripts/eval`, `internal`, `internal/testutil` |

```text
cmd/archfit  (flags, concrete wiring, exit translation)
     |
     v
internal/application  StageExecutor: Prepare -> Acquire -> Relate -> Assess -> Project
     |            \
     |             +--> internal/model/report   (projection target)
     +--> internal/policy         (what the architecture is supposed to be)
     +--> EvidenceStage port      (implemented by internal/evidence/acquisition)
     +--> internal/relationship   (strength, distance, volatility, scoring)
     +--> internal/assessment     (rules, metrics, findings, verdict, repair tasks)

Adapters (extract, toolrun, factcache, history, ownership, baseline, labels,
llm, output, config) point inward through ports the core defines. No core
package imports an adapter or cmd.
```

## Bounded contexts and what each owns

| Context | Owns | Never owns |
| --- | --- | --- |
| Architecture Policy (`internal/policy`) | Module and layer semantics, ownership, deploy units, gates, rule definitions, waivers, approved labels | YAML decoding, defaults, migration (that is `internal/config`) |
| Evidence Acquisition (`internal/evidence`, `internal/evidence/acquisition`, `internal/extract/**`) | Source and tool observations, coverage rows, coverage gaps | Relationships, findings, scores, verdicts |
| Relationship Analysis (`internal/relationship/**`) | Strength, distance, volatility, connascence, provenance, relationship scoring, relationship advisories | Rule evaluation, metrics, statuses, verdicts |
| Assessment and Repair (`internal/assessment/**`) | Rule and metric evaluation, findings, lifecycle status, scorecard, verdict, deltas, repair tasks | The dependency graph, report DTOs |
| Analysis Application (`internal/application`) | Stage order, the single baseline read, cancellation, base-tree comparison, report projection | Concrete adapters, YAML, subprocess execution |
| Report Contract and Adapters (`internal/model/report`, `internal/report/ports`, `internal/output/**`) | Stable external DTOs and rendering | Any domain decision |
| CLI Composition (`cmd/archfit`) | Flags, concrete construction, renderer selection, exit codes | Rules, metrics, scorers, statuses, decisions, classifiers |

## Allowed dependency direction

Measured module-to-module edges on this commit (cross-module only, edge counts
from `archfit analyze --json`, `classified_edges.by_module_pair`):

| From | To |
| --- | --- |
| `cli-composition` | `policy-config-adapter` (16), `analysis-application` (12), `provider-adapters` (10), `evidence-adapters` (8), `config-lifecycle` (6), `report-adapters` (5), `report-contract` (5), `persistence-adapters` (4), `evidence-contracts` (2), `analysis-scope` (1), `evidence-acquisition` (1), `relationship-analysis` (1) |
| `analysis-application` | `assessment-repair` (13), `relationship-analysis` (6), `report-contract` (5), `evidence-contracts` (4), `analysis-scope` (1), `architecture-policy` (1) |
| `assessment-repair` | `evidence-contracts` (23), `relationship-analysis` (17), `architecture-policy` (12), `analysis-scope` (2), `report-contract` (1) |
| `relationship-analysis` | `evidence-contracts` (18), `architecture-policy` (10), `evidence-analysis` (1) |
| `evidence-acquisition` | `evidence-contracts` (12), `evidence-adapters` (10), `persistence-adapters` (5), `analysis-scope` (3), `architecture-policy` (3), `relationship-analysis` (3), `analysis-application` (1) |
| `evidence-adapters` | `evidence-contracts` (41), `analysis-scope` (13), `persistence-adapters` (11), `evidence-analysis` (4), `architecture-policy` (2), `relationship-analysis` (2) |
| `report-adapters` | `report-contract` (14) — and nothing else |

Adapter-to-application edges (`evidence-acquisition`, `persistence-adapters`,
`policy-config-adapter` → `analysis-application`) are port implementations:
the application declares the interface, the adapter satisfies it. Outer
importing inner is the intended direction.

`assessment-repair → evidence-contracts` is 23 edges into the per-fact value
types in `internal/model/*`, never into `internal/evidence.Facts` or
`internal/model/graph`. Assessment receives `evaluation.Observations`, which
carries no graph and no classifier index, so it cannot re-derive a relationship
even by accident.

Cycles: 0 package cycles, 0 module cycles (`cycle` metric).

## Enforcement — which check catches which regression

| Invariant | Enforced by |
| --- | --- |
| Core ring imports no `os`, `os/exec`, YAML, or adapter | `TestArchImports` (`internal/arch_test.go`) |
| `internal/model/**` is stdlib-only, discovered dynamically | `TestArchImports` → `checkModelStdlibOnly` (walks every loaded `internal/model` package; no hardcoded list) |
| `internal/policy` imports stdlib, the model kernel, and one vetted pure matcher (doublestar, via `contractThirdPartyAllowed`) | `TestArchImports` → `checkPolicyContractPurity` |
| Published model surface does not drift | `TestModelSurfaceNoDrift` (golden `internal/testdata/model_surface.golden`) |
| Assessment sees no raw graph or coupling internals | `TestAssessmentProductionDoesNotImportRawGraphOrCoupling`, `assessment_no_raw_graph`, `assessment_no_coupling_internals` |
| Assessment consumes only the public relationship contract | `TestAssessmentConsumesOnlyThePublicRelationshipContract`, `assessment_no_relationship_internals` |
| Relationship never imports Assessment | `relationship_no_assessment` |
| Neither imports report DTOs | `TestDomainPackagesDoNotImportReportDTOs`, `assessment_no_report_dtos`, `relationship_no_report_dtos` |
| One report projector | `TestReportProjectionHasOneOwner`, `projector_no_domain_internals` |
| Renderers consume only the report contract | `renderer_no_{assessment,relationship,evidence}`, `output_no_{config,score,decision}`, `report_adapters_no_application` |
| Application imports no concrete adapter | `TestApplicationImportsNoConcreteAdapters` plus nine `application_no_*` rules |
| CLI imports no domain implementation | `TestCLIImportsNoDomainImplementation`, `cli_no_domain_implementation` |
| Acquisition never judges | `TestAcquisitionDelegatesAssessmentJudgment`, `acquisition_no_assessment_judgment` |
| Only composition roots import `internal/config` | the `*_no_config` rule family, `TestPolicyDoesNotImportConfig` |
| `internal/view` and `internal/analysispipeline` stay dead | `TestNoAnalysisPipelinePackage`, `TestSelfModelDeclaresNoDissolvedPackage`, guard rules `no_stage_view` and `no_analysispipeline` |
| Layer direction | `layer_inversion` (`forbidden_layer_direction`, gate `fail`) |
| The self-model describes real source | `TestSelfModel*` (`internal/selfmodel_test.go`): no dead path glob, no unowned Go package, no equal-specificity ownership tie, no dead rule, public entries real and owned, every declared layer used |
| Output/exit contracts | `TestGolden` (`internal/application`), `scripts/tests/cli_exit_contract_test.sh` |

`make archfit` runs the configured gate over archfit itself; CI runs it after
tests and goldens, and the `arch-lint` pre-push hook runs it locally.

## Balanced-coupling rationale

`.archfit.yaml` declares capabilities, not packages. Seventeen production
modules (plus `architecture-tests`) cover the 73 Go packages `go list ./...`
reports, because the unit that changes together is the capability, not the
directory.

- **Relationship and Assessment are adjacent on purpose.** Both are core, both
  are high-volatility, and they co-change. `assessment-repair →
  relationship-analysis` is 17 edges at `cross_module_same_owner` distance —
  strong coupling at short distance, which is balanced. Widening that distance
  with an event bus or a service seam would make the number look better and the
  system worse. Do not do it.
- **The score is 41/`mixed` and that is the honest read.** 363 scored
  cross-boundary edges, mean book balance 4.7/10, 70 critical-band edges, all
  at `cross_module_same_owner`. archfit has one owner and one deploy unit, so
  the distance dimension is degenerate by construction: every internal seam sits
  on the same rung and the balance formula is driven almost entirely by
  strength-vs-distance. A single-binary, sole-owner tool cannot score `strong`
  on this rubric without fabricating ownership or deploy-unit boundaries.
- **Volatility is declared from domain change pressure**, not commit count and
  not the score. Policy, relationship, and assessment are `high` because the
  rule language and the evaluation model are where the product actually
  evolves. `coupling.volatility_cascade: true` propagates it through strong
  coupling chains; 19 modules are `declared`, 7 pick up `cascade`, 0 are
  `undeclared`.
- **No waiver exists for anything this migration created**, and
  `.archfit-labels.yaml` is empty: current Go type information classifies every
  scored edge, so there is nothing to override. Abstain rather than guess.

## Accepted risks

1. **`internal/extract/{ts,py}` import `relationship/coupling` for the strength
   vocabulary** (`coupling.StrengthContract`, `StrengthIntrusive`,
   `StrengthFunctional`). Extractors emit hints in the relationship vocabulary,
   so they bind to its constants. This is a shared vocabulary, not a decision:
   no extractor calls a classifier or a scorer. Upgrade trigger — if an
   extractor ever needs more of `coupling` than the strength constants, move the
   strength enum down to `internal/model/evidence` and let both sides depend on
   the model kernel.
2. **`public_api_max` counts test declarations.** For `evidence-adapters`, 252
   of 397 counted exported declarations are `TestXxx` functions and moq fakes,
   so `syntax_api_size_ceiling` tracks module size, not published surface. It
   stays as an advisory growth ratchet at 430; the real published surface is
   gated by `TestModelSurfaceNoDrift` and the `forbidden_dependency` family.
3. **`coupling_balance` evidence lists same-module pairs among "top module
   pairs".** `by_module_pair` aggregates all 509 internal edges; the 146
   same-module ones are reported in `local_coupling` and excluded from the 363
   scored denominator. The number is correct; the label reads as if they were
   scored. Presentation only.
4. **`config update` reports `action_required` on archfit's own config with zero
   issues.** Discovery emits one module per directory (`model`, `extract`,
   `evidence`), while `.archfit.yaml` declares capabilities that span several
   directories, so the name-matching diff sees adds and removes. `Removed` is
   review-only and `--apply` never deletes a stanza; the noise is a known
   consequence of capability-grained modules and is not a config gap.

5. **`archfit baseline` is not idempotent on this repo.** Accepting the 104
   visible advisories surfaces 55 previously-grouped ones and a second round
   still does not reach a fixed point (104 → 159 → 142). `.archfit-baseline.json`
   is therefore deliberately empty: the gate passes with 0 blocking findings
   and 0 `agent_tasks` without it, so there is nothing to accept. Verified that
   the empty baseline suppresses nothing — a forbidden import injected into
   `internal/assessment/finding` still exits 1 with 2 blocking findings.

## Change recipes

**Add a metric or rule.** Implement in `internal/assessment/{metrics,rules}`.
It reads `evaluation.Observations` and the relationship contract — if it needs
the raw graph, it belongs in `internal/relationship` instead. Register the gate
knob in `internal/config`, project it through `policy.RuleConfig` or
`policy.MetricConfig`. Never import `internal/config` from the metric.

**Add a language or analyzer.** Extractor goes in `internal/extract/<lang>`,
behind `ports.Extractor`, driven by `toolrun.Runner` — never `exec.Command`.
Export an applicability function and wire it as the descriptor's
`ProjectPresent` in `internal/extract/registry/registry.go`; a probe that disagrees with its
extractor turns "we did not measure" into "there is nothing here". Give it one
coverage name of its own.

**Add an output format.** New package under `internal/output/`. It may import
`internal/model/report` and `internal/report/ports` and nothing else from
`internal/`. If it needs a field it cannot see, add the field to the report DTO
and populate it in `internal/application/report.go` — the single projector.

**Add a CLI command.** Build the stages in `cmd/archfit`, call
`application.StageExecutor.Execute`, render through a report adapter, translate
the verdict to an exit code. If the command needs a decision, the decision
belongs in `internal/assessment` or `internal/relationship`, not in `cmd`.

**Move a package.** Update the owning module's `paths:` in `.archfit.yaml` in
the same commit. `TestSelfModelCoversEveryGoPackage` fails on an unowned
package and `TestSelfModelHasNoDeadPathGlobs` fails on the glob you left
behind.

**Change the published model surface.** Regenerate with
`ARCHFIT_UPDATE_SURFACE=1 go test ./internal/ -run TestModelSurfaceNoDrift`,
inspect the golden diff, and call the contract change out in review. It is a
published contract, not an implementation detail.

**Re-baseline.** Prefer not to. The baseline is empty on purpose (accepted risk
5) and the gate passes without it. If you do re-baseline: only after
`make archfit` passes with zero blocking findings, inspect the semantic diff,
and re-run `analyze` to confirm the finding set converges — re-baselining has
previously introduced a phantom negative metric delta that turned a PASS into
exit 2. Never baseline to silence an architecture rule.
