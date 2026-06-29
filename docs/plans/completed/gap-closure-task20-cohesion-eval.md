# Task 20 eval — SCIP-symbol-graph cohesion (LCOM edge-density) metric

**Verdict: FAILS the eval → kept report-only (band `info`, never gates) and
disabled in archfit's own `.archfit.yaml`.** The `cohesion_lcom` metric ships
registered and available, but archfit does not dogfood it and it is not promoted
to a scorecard dimension.

## What the metric is

`internal/metrics/modularity/cohesion.go` — a Lack-of-Cohesion-of-Methods (LCOM4)
edge-density proxy over the SCIP symbol graph. For each measurable module it
builds the undirected graph of the module's definition symbols connected by
same-module references (`symbol.Graph.IntraRefs`, newly emitted by
`scip_reader.py`) and counts connected components. One component = cohesive;

> 1 component = the module bundles unrelated concerns (fragmented).

It is intentionally distinct from the removed `cohesion_spread` metric (which
measured outbound cross-module spread and failed its own gate): this proxy reasons
over **intra**-module connectivity, not external dispersion.

## The enclosing_range caveat (root cause of the failure)

SCIP indexers do not populate `enclosing_range`, so reference attribution is
**document-scoped**: within one document every definition is treated as the source
of every reference in that document. Same-document intra-module edges are therefore
over-connected, which makes a single-document module look trivially cohesive (one
component). Only cross-document intra-module structure carries a trustworthy
signal, so the metric measures only modules spanning ≥2 documents (and ≥4 symbols).

Consequence by language:

- **Go** — a module is a package spanning multiple files → measurable.
- **Python (scip-python)** — a module is keyed per source file → almost every
  module is single-document → **not measurable**.
- **TypeScript (scip-typescript)** — module keyed per file → **not measurable**
  (and additionally needs `node_modules` for the indexer to run at all).

## Live eval — improved archfit on the three baseline repos (HEAD)

SCIP on; `cohesion_lcom` enabled. Expert cohesion bands are the independent
architect-skill `cohesion_modularity` scores from
`gap-closure-phase2-comparison.md`.

| Repo (lang)     | scip-symbols | `cohesion_lcom` result                                   | expert cohesion |
| --------------- | ------------ | -------------------------------------------------------- | --------------- |
| archfit (Go)    | ok (3569)    | 1 fragmented of **36 measurable** (band info, high conf) | 70 serviceable  |
| ccgram (Python) | ok (19817)   | **n/a** — 0 measurable (single-document modules)         | 65 serviceable  |
| codegraph (TS)  | absent (0)   | **n/a** — scip absent (no `node_modules`)                | 42 mixed        |

The single Go "fragmented" module was `internal/config_test` — a Go external
test package (`_test`), i.e. test-fixture noise, not a real cohesion defect.

## Why it fails the eval

1. **Blind for 2 of 3 languages.** Python and TS — precisely the repos where the
   expert most disagreed with archfit's existing LOC-skew cohesion proxy (the gap
   this task set out to close) — return `n/a`. The metric contributes nothing
   exactly where it was needed.
2. **Not a comparable band.** Even on Go it emits a _fragmentation count_
   (1-of-36), not a 0-100 cohesion score. "35/36 packages internally connected"
   is consistent with the expert's "cohesive/serviceable", but the metric cannot
   be turned into the expert's band without an arbitrary mapping, and a single
   fragmented package is dominated by `_test`-package noise.
3. **Does not see sub-package cohesion.** The expert credits well-factored
   internal sub-packages (the documented divergence #1); a package-granularity
   component count cannot reward that.

So the SCIP-LCOM proxy does not reproduce expert cohesion judgments and is kept
report-only + disabled, matching the plan's "if it fails the eval, keep it
report-only/disabled" instruction (and, unlike `cohesion_spread`, it is retained
rather than removed because the Go-package fragmentation signal is honest where it
applies).

## Reproduce

```
# Go (cohesion enabled via an in-repo eval config so scope resolves to the repo):
#   cp .archfit.yaml .eval.yaml; flip cohesion_lcom enabled:false -> true
archfit check -c .eval.yaml --report --format json | jq '.metrics[]|select(.name=="cohesion_lcom")'
# Python / TS (metric on by default in their configs):
cd ~/Workspace/ccgram   && archfit check --report --format json | jq '.metrics[]|select(.name=="cohesion_lcom")'
cd ~/Workspace/codegraph && archfit check --report --format json | jq '.metrics[]|select(.name=="cohesion_lcom")'
```

Determinism: the metric ranks by sorted (components, symbols, module) and uses a
deterministic union-find; `TestCohesion_Deterministic` asserts byte-identical
`Display` across repeated calls. No wall-clock or map-order leakage.
