# Language-evidence corpus sweep

Date: 2026-08-28
Branch: `archfit-language-evidence-improvements-470edda8`
Binary: `823aab6` (`v2.0.0-35-g823aab6`, `.bin/archfit`)
Comparison base: `306e5658`

## Result

The current-HEAD 11-repository sweep completed in strict mode with exit **0**.
All 11 records passed the migration-only schema-v2/idempotence check, JSON state
validation, promotion contract, requested format and determinism checks, and the
analyze/check exit contract:

- every `analyze --format=json` exited 0;
- every `check --format=json` exited 2 with `needs_attention`;
- every state document used `archfit.architecture-state.v1` and all nine
  dimensions;
- requested text, Markdown, SARIF, scorecard, repeat, and byte-determinism checks
  passed;
- no sweep failure or accepted-unverified record was produced;
- no corpus repository was modified by the sweep.

| Repository | Language | Measured / Partial / Unmeasured | Metric entries | Verdict |
| --- | --- | ---: | ---: | --- |
| `spotinfo` | Go | 6 / 2 / 1 | 53 | `needs_attention` |
| `pumba` | Go | 5 / 3 / 1 | 57 | `needs_attention` |
| `omni/scheduled-tasks` | Go | 2 / 6 / 1 | 50 | `needs_attention` |
| `prometheus` | Go | 5 / 3 / 1 | 53 | `needs_attention` |
| `ccgram` | Python | 0 / 6 / 3 | 53 | `needs_attention` |
| `prefect` | Python | 0 / 6 / 3 | 49 | `needs_attention` |
| `storybook` | TypeScript | 2 / 5 / 2 | 50 | `needs_attention` |
| `yazi` | Rust | 4 / 3 / 2 | 55 | `needs_attention` |
| `herdr` | Rust | 5 / 3 / 1 | 54 | `needs_attention` |
| `ruff` | Rust | 1 / 6 / 2 | 45 | `needs_attention` |
| `tokio` | Rust | 3 / 4 / 2 | 48 | `needs_attention` |

Across the corpus, Archfit emitted 567 metric entries from 58 distinct
`dimension.metric` families. No corpus verdict improved without an evidence
gain. The sweep's independent promotion assertion rejected measured dimensions
carrying in-claim unknowns and rejected partial dimensions without a named
unknown fact.

## Before/after comparison

The same corpus was also analyzed with the pre-refactor base binary at
`306e5658`. The base completed the commands but failed the current strict
promotion contract for **11/11 repositories**, producing 15 validation failures.
Current HEAD produced **0** validation failures.

| Signal | Base `306e5658` | HEAD `823aab6` | Change |
| --- | ---: | ---: | ---: |
| Strictly valid repository records | 0 / 11 | 11 / 11 | +11 |
| Metric entries | 445 | 567 | +122 (+27.4%) |
| Distinct metric families | 43 | 58 | +15 (+34.9%) |
| Tool records | 128 | 128 | unchanged |
| Tool families | 12 | 12 | unchanged |
| Measured complexity dimensions | 0 / 11 | 6 / 11 | +6 |
| Measured operations dimensions | 0 / 11 | 1 / 11 | +1 |

The added metric families cover module-graph complexity, function-size
distributions, dependency-chain depth, module fan-in/fan-out, declared and
corroborated deploy units, analyzer reporting coverage, module deployment
coverage, and owner provenance.

The total count of `measured` dimension states decreased from 43 to 33. This is
an intentional correctness improvement, not a regression: the base marked
intent, structure, or modularity measured when no applicable producer had
completed. HEAD now reports those states as `partial` or `unmeasured` and names
the missing fact. Conversely, complexity is genuinely measured in six corpus
repositories and operations in one because those collectors now have explicit
completeness contracts.

Tool breadth did not increase: both binaries recorded the same 12 tool families
and 128 tool observations. Tool use improved semantically. HEAD binds extractor
applicability and completion to each rule and dimension, uses analyzer/deploy
corroboration in operations evidence, preserves provenance, and refuses to turn
an absent producer into a healthy zero.

## End-to-end reachability

The hermetic Go-only reachability fixture reaches **Outcome A**. The test
materializes a temporary repository, creates its deterministic commit, emits a
supplied coverage profile plus version-1 source-hash sidecar, persists a
comparable baseline, and checks the terminal state:

- verdict: `healthy`;
- `check` exit: `0`;
- `hard_gates`: `pass`;
- `unknown_dimensions`: `0`;
- active diagnostics: `0`;
- all nine dimensions: `measured`.

Two analyses before and two analyses after the baseline transition are
byte-identical. Drift becomes measured only after the persisted baseline is
created; the root comparison remains `not_requested`, as designed.

Task 3 originally classified the `complexity`, `testability`, and `operations`
blockers as B-temporary. Re-running its reachability test after Stages A/B
produces Outcome A, so none of those blockers remains. Task 3 recorded no
B-permanent outcome, and the Task 2 audit contains no owner-ratified permanent
constraint.

## Residual coverage gaps

These are disclosed gaps, not silently promoted zeros:

- The corpus repositories do not provide an enabled, attested supplied coverage
  artifact to this sweep. Consequently `testability` remains partial. The
  fixture's supplied-coverage path is measured separately and does not imply
  corpus test coverage.
- `operations` remains partial wherever declared modules lack independently
  corroborated deploy units. Runtime topology and supply-chain inventory remain
  out of claim; committed manifests do not prove what is running.
- `drift` remains unmeasured on first-run corpus analyses because no persisted
  comparable architecture-state baseline was supplied.
- Python and TypeScript unresolved or incomplete extractor evidence keeps the
  affected structure, coupling, modularity, or intent dimensions partial or
  unmeasured. Rust analyzer gaps and partial `cargo-modules`/SCIP evidence are
  likewise disclosed in their dimension unknown facts.
- Graph complexity is measured where the declared module graph is complete, but
  cognitive complexity remains out of claim. Testability does not claim
  assertion quality or meaningful boundary semantics from instrumentation
  alone.

## Reproduction artifacts

Current HEAD:

- summary: `/tmp/archfit-corpus-results-head.json`;
- work products: `/tmp/archfit-corpus-eval-head`.

Pre-refactor comparison:

- summary: `/tmp/archfit-corpus-results-base.json`;
- work products: `/tmp/archfit-corpus-eval-base`.
