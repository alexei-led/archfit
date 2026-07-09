# Release notes

## v2.0 CLI redesign

Breaking changes:

- `archfit check` replaces `archfit analyze --gate` and `archfit --gate` for CI
  and agent validation.
- Removed flags:
  - `--gate` → use `archfit check`
  - `--full` → full scan is now the default
  - `--advisory` → advisories are on by default; use `--no-advisories` to hide them
  - `--no-config` → initialize config first with `archfit config init --root .`
- Renamed flags:
  - `--severity` → `--min-severity`
  - `analyze --llm` → `analyze --ai-summary`
  - `explain --llm` → `explain --ai-summary`
  - `config init --llm` → `config init --ai-classify`
  - `config update --llm` → `config update --ai-classify`
  - `--llm-provider` → `--ai-provider`
  - `--llm-model` → `--ai-model`
  - `--no-cache` → `--refresh`

`--refresh` bypasses cache reads but still writes fresh results back to cache.

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
  config-authoritative > compiler-grade > SCIP/heuristic static facts >
  approved LLM label > abstain. LLM labels fill unknown cells; they do not
  override static classifications.
- `archfit config update --ai-classify` proposes review-only module subdomains
  (`core|supporting|generic`), derived volatility, layer suggestions, and
  optional architectural roles, including synthetic module keys, so repos with
  many generated module declarations can review differentiated domain
  volatility without putting model calls on the gate.
```
