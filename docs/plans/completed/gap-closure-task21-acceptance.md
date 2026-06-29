# Task 21 — final acceptance gate (gap-closure)

Verification-only task. Re-ran every Phase-1/2 correctness + determinism assertion
against the built binary on all three baseline repos, plus the full `make all`
gate and the structural invariants. All green.

Binary: `v0.3.0-22-g729717c` (built from this branch).
Repos: archfit (Go, self) · `~/Workspace/ccgram` (Python, base `HEAD~30`) ·
`~/Workspace/codegraph` (TS, base `HEAD~30`). Go delta base `HEAD~5`.

## 1. Phase-1/2 correctness assertions (17/17 PASS)

Driver: `/tmp/verify_task21.sh` (full + delta + scorecard double-run per repo).

| Assertion                                                                         | Result                              |
| --------------------------------------------------------------------------------- | ----------------------------------- |
| ccgram delta `change_locality` ≠ false-0                                          | **340** cross-module edges          |
| codegraph martin lists no node builtins (`fs`/`crypto`/`commander`/`path`/`http`) | none present                        |
| ccgram BC advisories rendered as grouped rollups                                  | **58** findings carry `group_count` |
| ccgram BC `score_value` visible                                                   | 58 findings                         |
| ccgram BC `score_band` visible                                                    | 58 findings                         |

## 2. Determinism end-to-end (PASS)

- All **9** JSON/scorecard double-runs (full/delta/scorecard × 3 repos) byte-identical.
- `config_hash` stable per repo and identical full == delta.
- Golden determinism gate `TestGolden` green; `metric_version`s unchanged.

## 3. `make all` (fmt → lint → test → build) — green

- `make lint`: 0 issues.
- `make test`: full `-race` suite, 56 packages ok, zero FAIL/panic.
- Structural invariants green: `TestArchImports` (core ring), its
  `llm_ring_unreachable_from_internal` subtest (LLM-off-gate), and `TestGolden`.

## 4. `review --llm` — skip-with-note (no API key)

`ANTHROPIC_API_KEY` absent (`archfit doctor`), so the live LLM call is skipped.
`archfit review` exits 3 with an actionable message
(`error: llm provider anthropic not configured: ANTHROPIC_API_KEY is not set …`)
and never affects `check` (LLM-off-gate invariant holds). The post-verify
entity-drop path is covered deterministically by `TestReviewCmd_Run_EntityPostCheck`
(fake provider citing a non-existent module → claim dropped).

Two defects surfaced by this gate and fixed:

- **Silent parse errors.** `cmd/archfit/main.go` returned exit 3 with no output on
  any unknown flag — manual `parser.Parse` (unlike the `kong.Parse` helper) does not
  print the error. Now prints `archfit: <err>`. Regression test
  `TestRun_UnknownFlag_NotSilent`.
- **Doc/CLI mismatch.** README prose and `docs/guide/dogfooding.md` said
  `archfit review --llm`; the command takes no `--llm` flag (the whole command is the
  off-gate LLM review, unlike `explain --llm` which has a deterministic default mode).
  Docs now say `archfit review`.

## 5. Test coverage — meets project standard

Total 77.6% of statements. Decision/core and adapter packages 80–100%
(`model/diagnostic` 100%, `model/coupling` 97.8%, `score` 89.8%, `output/jsonout`
& `output/scorecard` 100%). The sub-standard packages are data/interface-only with
no branching logic — `model/clone`, `model/signal`, `model/symbol`, `ports`,
`metrics/internal/modgraph`, and the `metricstest`/`metricstest` helpers — matching
the repo's established "cover behavior, not structs" pattern.
