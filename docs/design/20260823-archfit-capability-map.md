# Archfit capability map

Date: 2026-08-23
Status: IMPLEMENTED IN WORKTREE

This map is based on package imports, recent co-change history, public-surface
size, and the current architecture report. It is not a score-optimization plan.
The map is implemented in the current worktree. The score change is a
measurement of the resulting boundaries, not the acceptance criterion by itself.

## Evidence

The current run reports:

- 102 modules in the source graph.
- 429 scored cross-boundary edges; 0 abstained.
- Mean book balance: 5.86/10; normalized `coupling_balance`: 54/100.
- Balance drivers: `strength_distance=79`, `tie=92`, `volatility=258`.
- Critical edges fell from 123 to 14.
- All scored edges use `cross_module_same_owner` distance.
- Largest scored module pairs are now `archfit-cli -> fact-adapters=34`,
  `fact-adapters -> evidence-model=27`, and
  `evaluation-core -> evidence-model=23`.
- `evaluation-core` has 331 exported declarations in the current configuration.

`go list` reports the largest package-level module edges:

| From | To | Import edges |
| --- | --- | ---: |
| evaluation-core | evidence-model | 23 |
| fact-adapters | evidence-model | 27 |
| fact-adapters | pipeline-contracts | 14 |
| pipeline-contracts | evidence-model | 13 |
| pipeline-engine | evidence-model | 13 |
| evaluation-core | report-contract | 12 |
| evaluation-core | stage-views | 11 |
| evaluation-core | finding-model | 10 |
| archfit-cli | scan-contract | 20 |

Recent history also shows distinct change vectors inside `evaluation-core`:

| Package | Commits touched in the last 180 days |
| --- | ---: |
| `metrics` | 69 |
| `classify` | 62 |
| `score` | 41 |
| `rules` | 24 |
| `agenttask` | 15 |
| `status` | 10 |
| `facts` | 6 |
| `decision` | 4 |
| `staleness` | 4 |

The source refactor moved extractor-facing facts into `internal/model/evidence`,
score/report contracts into `internal/model/report`, and the aggregate into
`internal/model/scan`. `internal/model/diagnostic` now provides compatibility
aliases only. The module map assigns coupling, signal, architecture, and finding
contracts to separate capability modules with medium volatility.

The strongest package co-change pairs are `classify-score` (18),
`metrics-score` (13), `classify-metrics` (12), `metrics-rules` (12), and
`classify-rules` (10). `decision` and `staleness` have much lower co-change
with the evaluation cluster. This supports a split between fact/policy
calculation and verdict/report projection. It does not support splitting
`classify`, `metrics`, and `score` only because the headline score is low.

## Target map

The target keeps stable package seams where they already match the change
vector. New module names describe ownership. They do not require one package
per module.

| Capability | Initial package ownership | Responsibility | Allowed inward dependencies |
| --- | --- | --- | --- |
| `evidence-model` | `internal/model/graph`, `symbol`, `fileclass`, `clone`, `pattern` | Source facts and extractor-neutral evidence types | Standard library |
| `coupling-model` | `internal/model/coupling` | Strength, distance, volatility, classification, and book scoring types | Evidence model only when required by a fact type |
| `pipeline-state` | `internal/model/signal` | Signal carriers between extraction and metrics | Evidence, coupling, finding, and report contracts |
| `architecture-model` | `internal/model/module` | Module-map and ownership values | Evidence and pipeline contracts |
| `finding-model` | `internal/model/finding` | Finding lifecycle and repair contracts | Coupling and report contracts |
| `report-contract` | `internal/model/report` | Stable scorecard, decision, persistence, and output view models | Data-only; no engine, extractor, or renderer imports |
| `scan-contract` | `internal/model/scan`, compatibility `diagnostic` | Stable top-level scan aggregate and legacy facade | Evidence, finding, and report contracts |
| `pipeline-contracts` | `internal/ports`, `internal/scope`, `internal/syntax` | Extractor, resolver, syntax, and scope ports | Evidence and stage views; not output adapters |
| `stage-views` | `internal/view` | Stable stage configuration and policy views | Data-only model packages |
| `fact-derivation` | `internal/classify`, `internal/facts` | Convert extracted facts into classified coupling and derived evidence | Evidence model, coupling model, pipeline contracts, stage views |
| `policy-evaluation` | `internal/rules`, `internal/metrics`, `internal/score` | Apply architecture rules and compute metric results | Evaluation model, coupling model, policy language, stage views |
| `verdict` | `internal/status`, `internal/decision`, `internal/agenttask` | Join findings and metrics into verdicts and repair tasks | Report contract, finding model, policy language |
| `pipeline-state` | extracted state types from `internal/engine` | Stage results and assembly state; no policy decisions | Evidence model, evaluation model, report contract, stage views |
| `pipeline-engine` | `internal/engine` orchestration | Sequence extraction, derivation, policy evaluation, and report assembly | Ports, fact derivation, policy evaluation, verdict, adapters through ports only |
| `fact-adapters` | `internal/extract`, `toolrun`, `factcache`, `history`, `ownership` | External tools and source-control facts | Pipeline contracts, evidence model, stage views |
| `rendering` | `internal/output` | Human and machine projections of `report-contract` | Report contract only for completed reports |
| `archfit-cli` | `cmd/archfit` | Composition roots and command UX | Pipeline engine, config lifecycle, rendering, stores, adapters |

## Boundary decisions

1. `report-contract` is the stable output boundary. Renderers must not consume
   `diagnostic.Diagnostic` after the migration. Diagnostic assembly stays inside
   the pipeline until the report projection is complete.
2. `scan-contract` is not the report boundary. The scan aggregate carries
   extraction and gate input; `report-contract` carries the stable output view.
3. `fact-derivation` and `policy-evaluation` are separate capabilities even
   when one run calls them in sequence. The former derives facts; the latter
   applies rules and metrics.
4. `verdict` owns decision wording and repair-task projection. It must not own
   extractor or renderer details.
5. `pipeline-engine` owns ordering and cancellation only. Stage data moves to
   `pipeline-state` so the orchestrator does not become a second domain model.
6. `cmd/archfit` remains a composition root. Command-specific lifecycle code
   moves behind existing config, store, adapter, or engine seams instead of
   adding more policy to the CLI package.

## Non-goals

- Do not split a package only to change the coupling score.
- Do not add labels or baselines to hide current findings.
- Do not change distance or volatility values as part of this refactor.
- Do not make `coupling_balance` an overall architecture score.
