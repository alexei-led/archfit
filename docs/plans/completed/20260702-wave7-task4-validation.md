# Wave 7 Task 4 validation

Run date: 2026-07-04.
Scratch root: `/tmp/archfit-wave7-task4`.
Binary: `.bin/archfit` built from this working tree.

All corpus commands used scratch config copies so generated labels and caches were written outside the corpus repos.

## Rust: yazi abstained labels

Command shape:

```bash
.bin/archfit config enrich abstained --config /tmp/archfit-wave7-task4/yazi/.archfit.yaml --root /Users/alexei/Workspace/yazi
```

Results:

- Pass 1 found 144 abstained edges and drafted the first capped 100 labels.
- After scratch review/pin of those 100 labels, analyze reported 166 scored and 44 abstained internal cross-boundary edges, a 79% scored fraction.
- Pass 2 drafted the remaining 44 labels.
- Scratch review result: accepted 144, rejected 0.
- Final pinned scratch analysis: 210 scored, 0 abstained, 100% scored fraction.
- Label provenance/confidence: 144 `provenance: llm`; 132 low confidence, 12 medium confidence.
- `coupling_balance`: value 58, band mixed, confidence medium.
- Evidence included `llm-provenance labels in effect: 144 (confidence lowered)` and `llm-labeled edges: 144`.

## Rust: herdr and tokio role proposals

Before-state checks:

- herdr: 646/646 internal edges were high volatility; `volatility_provenance` showed 1 declared and 178 inherited modules.
- tokio: 976/979 classified internal/same-module edges were medium volatility; `volatility_provenance` showed 5 declared, 305 inherited, 114 undeclared modules.

Patched `config update --llm` validation:

- herdr produced 178 review-only synthetic override proposals.
- herdr proposal volatility distribution: 43 high, 67 medium, 68 low.
- tokio produced 5 added support/test crate proposals and 408 review-only synthetic override proposals.
- tokio proposal volatility distribution: 14 high, 145 medium, 254 low.

Spot checks against domain judgment:

- herdr `herdr::agent_resume`: core/high/core. Agree: agent resume is a differentiating workflow.
- herdr `herdr::api::client`: supporting/low/adapter. Agree: stable API client boundary.
- herdr `herdr::api::schema::agents`: core/high/shared_model. Agree: agent schema changes with core agent integration.
- tokio `tokio::blocking`: core/medium/core. Agree: runtime blocking-task offload is central but relatively stable.
- tokio `tokio::fs::canonicalize`: supporting/low/adapter. Agree: thin async filesystem wrapper over solved OS behavior.

Agreement: 5/5.

## Python: prefect abstained labels

Command shape:

```bash
.bin/archfit config enrich abstained --config /tmp/archfit-wave7-task4/prefect/.archfit.yaml --root /Users/alexei/Workspace/prefect
```

Result: `enrich abstained: no abstained cross-module edges - nothing to label`.

No labels were drafted, so no LLM proposal could contradict grimp-derived or config-authoritative classifications.

## Go: archfit self

Command shape:

```bash
.bin/archfit config enrich abstained --config /tmp/archfit-wave7-task4/archfit/.archfit.yaml --root /Users/alexei/Workspace/archfit --no-cache
```

Result: `enrich abstained: no abstained cross-module edges - nothing to label`.

## TypeScript: storybook partial coverage

Command shape:

```bash
.bin/archfit config enrich abstained --config /tmp/archfit-wave7-task4/storybook/.archfit.yaml --root /Users/alexei/Workspace/storybook/code
```

Result: `enrich abstained: no abstained cross-module edges - nothing to label`.

This confirms unresolved-specifier coverage gaps are excluded from the abstained-label batch instead of being treated as ambiguous facts.

## Determinism and cleanliness

Two yazi analyze runs with the 144 approved scratch LLM labels were byte-identical for stdout and stderr.

Corpus repo status after validation:

- `/Users/alexei/Workspace/tokio`: clean.
- `/Users/alexei/Workspace/prefect`: clean.
- `/Users/alexei/Workspace/storybook`: clean.
- `/Users/alexei/Workspace/yazi`: unchanged pre-existing untracked archfit artifacts (`.archfit-cache/`, `.archfit.yaml`, backups, `.claude/`, `.codegraph/`, `index.scip`).
- `/Users/alexei/Workspace/herdr`: unchanged pre-existing untracked archfit artifacts (`.archfit-cache/`, `.archfit.yaml`, `.claude/`, `.codegraph/`, `index.scip`).
