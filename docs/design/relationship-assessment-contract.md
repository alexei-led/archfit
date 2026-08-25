# Relationship → assessment contract

This seam narrows the metric/evidence path from the extractor `graph.Graph` plus `coupling.Index` pair to `relationship.Set`.

## Contract

`internal/relationship.Set` is owned by the relationship layer and carries only classified relationship facts required downstream:

- `Node`: participant identity, structural module key, language, and first-party flag.
- `Edge`: endpoint paths/IDs, configured module labels, kind/language, strength, distance, volatility, severity.
- `Provenance`: classification key and report-only derivation flags (distance basis, LLM/connascence/clone provenance).

`internal/assessment/signals.CommonInput` now gives metrics `relationship.Set`, `CoverageView`, changed files, symbol inputs, findings, and metric baselines. Assessment rules, staleness, finding, score, and agenttask packages also consume `relationship.Set`/`relationship.Node`/`relationship.Edge`; no production file under `internal/assessment` imports `model/graph` or `relationship/coupling`.

## Remaining raw-Graph consumers

Graph acquisition and these downstream consumers intentionally remain raw-graph based until their own seams are migrated:

- `internal/analysispipeline/collectAdvisories` and advisory helpers: coupling advisory emission still walks raw graph edges with `coupling.Index` (the engine owns the raw-graph seam; rules and staleness already receive the built `relationship.Set`).
- `internal/analysispipeline` report assembly helpers: distance config candidates, volatility provenance, local coupling, classified-edge summaries, dynamic reports, and file facts still use raw graph/symbol facts.
- `internal/relationship/classify`: relationship classification itself still consumes `graph.Graph`; graph acquisition was deliberately not rewritten in this task.
