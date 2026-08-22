# Release notes

## v1.7.0 configuration confidence

Breaking changes:

- `archfit baseline --base <ref>` removed. It never changed the saved baseline —
  a baseline always records the tree as checked out. To compare against a ref,
  use `archfit check --base <ref>` or `archfit analyze --base <ref>`. The removed
  flag now returns `archfit: unknown flag --base` with exit `3`.

New:

- `archfit config compare <candidate>` measures one source tree under two
  configurations and reports the difference. Report-only: exit `0` on success,
  exit `3` on an input or runtime error, and findings never move the exit code.
  Both sides use an empty accepted baseline, so the comparison is raw
  measurement. A higher candidate score is never reported as better.
- `archfit config update --json` emits one machine-readable review document
  (`archfit.config-review.v1`) with status `action_required`, `review_available`,
  or `no_known_issues`. `--json` with `--apply`, `--ai-classify`, or `--refresh`
  is a usage error (exit `3`) rejected before any discovery or write.
- `config update --apply` no longer deletes, comments out, or re-keys a
  configured module stanza. A configured module and a discovered module that own
  the same paths under different names are reported as `name_drift`, and
  configured modules discovery did not emit stay under `removed_modules`. Both
  are review-only and neither raises the status to `action_required`.
- `--base <ref> --json` adds a report-only `git_finding_delta` block that sorts
  the current `agent_tasks[]` into `introduced`, `pre_existing`, and
  `unknown_origin`. Missing analyzer evidence produces `unknown`, never a false
  `introduced`.

Baseline file:

- `.archfit-baseline.json` records `rubric_version` alongside the scorer version
  in its score snapshot, so `coupling.gate.max_drop` refuses to anchor against an
  incompatible snapshot instead of comparing across rubrics. Existing baselines
  without the field are read as rubric version `1`; the file changes on the next
  `archfit baseline` run.

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
