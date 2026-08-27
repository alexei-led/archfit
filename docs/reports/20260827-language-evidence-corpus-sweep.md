# Language-evidence corpus sweep

Date: 2026-08-27
Branch: `archfit-language-evidence-improvements-470edda8`
Binary: `79f3672` (`.bin/archfit`)

## Result

The full 11-repository sweep completed in strict mode with exit **0**. Each
record passed the migration-only schema-v2/idempotence check, JSON state
validation, and the analyze/check exit contract:

- every `analyze --format=json` exited 0;
- every `check --format=json` exited 2 with `needs_attention`;
- every state document used `archfit.architecture-state.v1` and all nine
  dimensions;
- the requested format checks passed for `spotinfo` and `ccgram` (text,
  Markdown, SARIF, and scorecard), and byte determinism passed for
  `spotinfo`, `storybook`, `ccgram`, and `yazi`;
- no sweep failure or accepted-unverified record was produced.

| Repository | Language | Measured / Partial / Unmeasured | Verdict |
| --- | --- | ---: | --- |
| `spotinfo` | Go | 6 / 2 / 1 | `needs_attention` |
| `pumba` | Go | 6 / 2 / 1 | `needs_attention` |
| `omni/scheduled-tasks` | Go | 2 / 6 / 1 | `needs_attention` |
| `prometheus` | Go | 6 / 2 / 1 | `needs_attention` |
| `ccgram` | Python | 0 / 6 / 3 | `needs_attention` |
| `prefect` | Python | 0 / 6 / 3 | `needs_attention` |
| `storybook` | TypeScript | 2 / 5 / 2 | `needs_attention` |
| `yazi` | Rust | 4 / 3 / 2 | `needs_attention` |
| `herdr` | Rust | 5 / 3 / 1 | `needs_attention` |
| `ruff` | Rust | 0 / 7 / 2 | `needs_attention` |
| `tokio` | Rust | 4 / 3 / 2 | `needs_attention` |

No corpus verdict improved without an evidence gain. The sweep's independent
promotion assertion rejected measured dimensions carrying in-claim unknowns
and rejected partial dimensions without a named unknown fact.

## End-to-end reachability

The hermetic Go-only reachability fixture now reaches **Outcome A**. The test
materializes a temporary repository, creates its deterministic commit, emits a
supplied coverage profile plus version-1 source-hash sidecar, persists a
comparable baseline, and then checks the terminal state. The terminal result
was:

- verdict: `healthy`;
- `check` exit: `0`;
- `hard_gates`: `pass`;
- `unknown_dimensions`: `0`;
- active diagnostics: `0`;
- all nine dimensions: `measured`.

Two analyses before and two analyses after the baseline transition were
byte-identical. Drift became measured only after the persisted baseline was
created; the root comparison remained `not_requested`, as designed.

## Residual coverage gaps

These are disclosed gaps, not silently promoted zeros:

- The corpus repositories do not provide an enabled, attested supplied coverage
  artifact to this sweep. Consequently `testability` remains partial, with
  executed-coverage and boundary-test-semantics evidence absent. The fixture's
  supplied-coverage path is measured separately and does not imply corpus test
  coverage.
- `operations` remains partial wherever declared modules lack independently
  corroborated deploy units. Runtime topology and supply-chain inventory remain
  out of claim; committed manifests do not prove what is running.
- `drift` remains unmeasured on first-run corpus analyses because no persisted
  comparable architecture-state baseline was supplied. This is distinct from
  the fixture's baseline transition and is not a claim of drift.
- Python and TypeScript unresolved or incomplete extractor evidence keeps the
  affected structure, coupling, modularity, or intent dimensions partial or
  unmeasured. Rust analyzer gaps and partial `cargo-modules`/SCIP evidence are
  likewise disclosed in their dimension unknown facts.
- Graph complexity is measured where the declared module graph is complete,
  but cognitive complexity is intentionally out of claim. Testability does not
  claim assertion quality or meaningful boundary semantics from instrumentation
  alone.

The sweep did not write corpus source or configuration files. Several corpus
worktrees were already dirty with untracked Archfit caches/configuration or
controller artifacts; that pre-existing host state is not represented as a
successful cleanliness claim here. The complete machine-readable result is
`/tmp/archfit-corpus-results.json`, and the sweep work products are under
`/tmp/archfit-corpus-eval-task13`.
