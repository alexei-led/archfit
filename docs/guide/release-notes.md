# Release notes

Release notes for all versions are maintained on GitHub Releases:

<https://github.com/alexei-led/archfit/releases>

The canonical changelog, migration notes, and per-version details are there.

## Next release note draft

Use this entry in the next annotated tag message:

```text
LLM semantic labels and module-role review are now available as an off-gate
workflow.

- `archfit config enrich labels` drafts `.archfit-labels.yaml` entries for
  weak static strength classifications; `archfit config enrich abstained`
  targets cross-module edges whose strength was unknown after all static
  sources and includes source snippets plus model-reported confidence.
- Approved labels are deterministic committed YAML. Drafts are inert, stale
  evidence hashes are ignored with a `labels/stale` advisory, and
  `provenance: llm` labels with medium/low confidence lower
  `coupling_balance` confidence by one band.
- Strength precedence is explicit:
  config-authoritative > compiler-grade > SCIP > approved LLM label >
  heuristic > abstain. LLM labels fill unknown cells; they do not override
  static classifications.
- `archfit config update --llm` proposes review-only module roles
  (`core|supporting|generic`) and derived volatility, including synthetic
  module keys, so repos with many generated module declarations can review
  differentiated domain volatility without putting model calls on the gate.
```
