# Relationship → assessment contract

This seam narrows the metric/evidence path from the extractor `graph.Graph` plus `coupling.Index` pair to `relationship.Set`.

## Contract

`internal/relationship.Set` is owned by the relationship layer and carries only classified relationship facts required downstream:

- `Node`: participant identity, structural module key, language, and first-party flag.
- `Edge`: endpoint paths/IDs, configured module labels, kind/language, strength, distance, volatility, severity.
- `Provenance`: classification key and report-only derivation flags (distance basis, LLM/connascence/clone provenance).

`internal/assessment/signals.CommonInput` now gives metrics `relationship.Set`, `CoverageView`, changed files, symbol inputs, findings, and metric baselines. Assessment rules, staleness, finding, score, and agenttask packages also consume `relationship.Set`/`relationship.Node`/`relationship.Edge`; no production file under `internal/assessment` imports `model/graph` or `relationship/coupling`.

## Raw-graph ownership

`graph.Graph` stops at relationship analysis. `evidence.Facts` carries it into
`analysis.Analyze`; everything downstream receives `relationship.Set`.

- `internal/relationship/classify` and `internal/relationship/analysis` are the
  only production consumers of `graph.Graph`. Coupling advisory emission, local
  coupling, classified-edge summaries, distance-config candidates, volatility
  provenance, and dynamic reports all live behind `analysis.Analyze` and are
  returned on `relationship.AnalysisResult`.
- Assessment receives `evaluation.Observations`, which has no graph and no
  classifier index (`application.observationsOf`), so it cannot re-derive a
  relationship even by accident. `assessment_no_raw_graph` and
  `assessment_no_coupling_internals` gate this at `fail`, and
  `TestAssessmentProductionDoesNotImportRawGraphOrCoupling` proves it in Go.
- `internal/analysispipeline` is deleted. The `no_analysispipeline` rule blocks
  a replacement orchestration hub from reappearing.

Status: IMPLEMENTED. This is the shipped contract, not a migration target.
