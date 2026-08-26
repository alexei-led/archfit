# archfit test corpus

Repos used to dogfood and validate `archfit analyze` / `archfit check` (report,
delta, and AI summary) across all supported languages and ownership models.
Cloned under `~/workspace/<name>`.

This file is the **human-readable corpus inventory**. The operational workflow
now lives in `skills/archfit-eval/` and `scripts/eval/corpus_sweep.py`.

Two groups: **local dogfood repos** (small, mostly single-owner — fast inner-loop
checks) and **multi-author repos** (popular, multi-team — exercise socio-
technical owner-distance, CODEOWNERS resolution, and coupling at scale).

## Local dogfood repos

| Repo                 | Lang                  | Owner model                 | CODEOWNERS           | Delta base (prev minor) | Why                                                        |
| -------------------- | --------------------- | --------------------------- | -------------------- | ----------------------- | ---------------------------------------------------------- |
| archfit              | Go                    | solo (mine)                 | no                   | `v0.13.0`               | self-dogfood; gates its own architecture (`make archfit`)  |
| spotinfo             | Go                    | solo (mine)                 | no                   | `v2.2.1`                | tiny CLI; thin-graph baseline                              |
| pumba                | Go                    | solo (mine)                 | yes (`@alexei-led`)  | `1.0.6`                 | small Go, forbidden-dep rules                              |
| ccgram               | Python                | solo (mine)                 | no                   | `v4.2.0`                | richest single-owner: encapsulation + cycles fire          |
| yazi                 | Rust                  | external, single-maintainer | no                   | `v26.1.22`              | 31-crate workspace; cargo_modules off (abstain study)      |
| herdr                | Rust                  | external, single-maintainer | no                   | `v0.6.10`               | single crate; cargo_modules+scip on (94% scored)           |
| omni/scheduled-tasks | Go (monorepo subtree) | multi-team (doit)           | yes (root, 9+ teams) | commit ~5wk back        | `--root` subtree + `go.work`; CODEOWNERS-subtree case (F2) |

## Multi-author repos (cloned 2026-06-30)

Selected for: popular (>20k★), genuinely multi-author (330+ contributors), actively released
(2026), modular (workspace/monorepo), and CODEOWNERS multi-team where possible. Cloned with
`--filter=blob:none` (full history for git-log ownership + HEAD blobs for analysis).

| Repo                  | Lang   | ★   | CODEOWNERS teams | Owner-path tested    | Why useful                                           |
| --------------------- | ------ | --- | ---------------- | -------------------- | ---------------------------------------------------- |
| prometheus/prometheus | Go     | 65k | 31               | CODEOWNERS           | modular monorepo, rich team boundaries               |
| astral-sh/ruff        | Rust   | 48k | 15               | CODEOWNERS           | multi-crate cargo workspace, very active             |
| storybookjs/storybook | TS/JS  | 90k | 12               | CODEOWNERS           | large package monorepo (depcruise path)              |
| prefecthq/prefect     | Python | 23k | 5                | CODEOWNERS           | manageable multi-team Python                         |
| tokio-rs/tokio        | Rust   | 32k | none             | git-history fallback | workspace, no CODEOWNERS → tests git-owner inference |

Cloned HEAD + delta base (prev minor), as of 2026-06-30:

| Repo                  | Local path               | Latest stable | Delta base (prev minor)               |
| --------------------- | ------------------------ | ------------- | ------------------------------------- |
| prometheus/prometheus | `~/workspace/prometheus` | v3.12.0       | `v3.11.3`                             |
| astral-sh/ruff        | `~/workspace/ruff`       | 0.15.20       | `0.14.0`                              |
| storybookjs/storybook | `~/workspace/storybook`  | v10.4.6       | `v10.3.0`                             |
| prefecthq/prefect     | `~/workspace/prefect`    | 3.7.6         | `3.6.0`                               |
| tokio-rs/tokio        | `~/workspace/tokio`      | tokio-1.52.3  | `tokio-1.51.3` (per-crate tag scheme) |

Refine the exact prev-minor patch at delta-run time (`git -C <repo> tag --sort=-creatordate`).

## Workflow rules

- Run from the archfit repo root so `.env` auto-loads the AI key consistently.
- Corpus repos are **read-only**. Copy `.archfit.yaml` out and work on the copy;
  nothing in a sweep may write into a target tree.
- Every config in the corpus was authored at schema v1. Migrate the copy with
  `config update --migration-only --apply`, never the full `config update
--apply`: the full form also proposes structural module edits, and bundling
  those into a schema migration hides whether the migration itself settles.
- The migration must be byte-idempotent. A second `--migration-only --apply`
  that rewrites the file is a defect, not noise.
- A config-update failure is a finding, never a silent skip.
- Re-run every mandatory representative with the same candidate and compare
  `analyze --json` BYTE for byte. The v1 state carries no wall-clock or
  run-local field, so there is nothing to exclude from the comparison.
- Use the AI summary on selected representatives only, and always over a
  separate overlay copy of the candidate — never over the candidate itself.

## v1 acceptance contract

`analyze --json` emits `archfit.architecture-state.v1`. There is no repository
score to parse; a sweep that reads `score.overall` is reading a field the
cutover deleted. What a passing repo must show:

| Fact       | Where                    | Requirement                                                                      |
| ---------- | ------------------------ | -------------------------------------------------------------------------------- |
| schema     | `schema_version`         | `archfit.architecture-state.v1`                                                  |
| verdict    | `verdict`                | `healthy`, `needs_attention`, or `blocked`                                       |
| dimensions | `dimensions`             | the nine keys, in contract order                                                 |
| metrics    | `dimensions.<d>.metrics` | an ARRAY of `{name, value, unit}` records                                        |
| coverage   | `coverage`               | `measured + partial + unmeasured == 9`, and equal to what the nine envelopes say |
| config     | migrated candidate       | `version: 2`, idempotent on a second migration                                   |
| exit       | `check`                  | healthy 0, needs-attention 2, blocked 1, error 3                                 |

**Expect exit 2, not 0, on a healthy repository.** Complexity, testability, and
operations report `partial` by contract in v1, and any partial dimension is
`needs_attention`. Only exit 1 means a hard gate blocked.

Every format reports the same verdict, dimension statuses, coverage split,
comparison state, and canonical finding sequence. Text, Markdown, and the
scorecard may lead with an abbreviated actionable excerpt, but each closes with
a finding index carrying every finding ID — including accepted and waived ones —
in the document's canonical order. SARIF is exempt from layout parity only: the
state rides in `run.properties` and each finding keeps its `archfit/v1`
fingerprint.

## Classifying what a sweep surfaces

Do not leave an anomaly unclassified.

- **Product defect** — the deterministic contract is wrong. Fix it, with a
  minimal reproduction and a regression test that fails without the fix.
- **Config migration defect** — the migration lost, weakened, or invented a
  setting. Same bar as a product defect.
- **Target architecture finding** — the repo really does have that coupling.
  Record it; never change a target's modules, owners, volatility, labels,
  baselines, or rules to make the output look better.
- **Optional tool gap** — an opt-in analyzer is off or its binary is missing.
  It stays an explicit coverage gap: it must not crash, disappear, or read as
  healthy. A required-tool failure keeps its configured hard behaviour.
- **Environment failure** — missing credentials, a missing toolchain. Recorded
  separately. AI credential failures never fail strict mode; an AI-path
  schema or projection defect still does.
- **Docs/UX defect** — the output is correct but misleads.

## Helper script

```sh
python3 scripts/eval/corpus_sweep.py --help
python3 -m unittest discover -s scripts/eval -p '*corpus_sweep_test.py'
```

Mandatory four-language fast matrix — one representative per supported adapter:

```sh
python3 scripts/eval/corpus_sweep.py \
  --repos spotinfo,storybook,ccgram,herdr \
  --ai-repos '' --migration-only \
  --repeat-repos spotinfo,storybook,ccgram,herdr \
  --format-repos spotinfo,storybook,ccgram,herdr \
  --strict --max-workers 1 \
  --output-dir /tmp/archfit-corpus-eval-fast \
  --summary-file /tmp/archfit-corpus-results-fast.json
```

Full corpus — all eleven labels:

```sh
python3 scripts/eval/corpus_sweep.py \
  --repos spotinfo,pumba,omni/scheduled-tasks,prometheus,ccgram,prefect,storybook,yazi,herdr,ruff,tokio \
  --ai-repos spotinfo,ccgram,herdr,storybook --migration-only \
  --repeat-repos spotinfo,storybook,ccgram,herdr \
  --format-repos spotinfo,storybook,ccgram,herdr \
  --strict --max-workers 4 \
  --output-dir /tmp/archfit-corpus-eval \
  --summary-file /tmp/archfit-corpus-results.json
```

`--strict` returns 0 only when every record is `pass`, or every non-pass record
is an owner-supplied `--allow-unverified label=reason` gap. An allowed gap is
recorded as `accepted_unverified` and NEVER as a pass: the final review has to
be able to see the hole. A repo that is simply not cloned is `unverified` and
blocks strict mode until someone accepts it by name and reason.

Per-repo record shape (frozen; absent values are explicit `null` or `[]`):

```
{label, root, language, status, failures[], unverified{reason},
 config{source, source_config_sha256, candidate_sha256, target_head, version,
        update_exit, second_update_exit, second_update_changed},
 analyze{exit, schema_version, verdict, dimension_keys},
 check{exit, verdict},
 formats{json, text, markdown, sarif, scorecard},
 determinism{json_byte_identical},
 ai{requested, exit}}
```

The script uses `.bin/archfit` from the current branch, runs from the archfit
repo root, writes candidates and artifacts under the output directory, and
writes the summary to the summary file. Nothing it writes belongs in git.

## Delivering a migrated config

Only the three owner-controlled dogfood repos — `spotinfo`, `pumba`, `ccgram` —
ever receive a config back, and only as a separate, owner-gated pass after the
sweep and the full repository validation have both passed. External corpus repos
are never edited. The candidate delivered is the immutable
`--migration-only` output; an AI overlay is never eligible. Require a clean
worktree, the exact swept HEAD, and the recorded source-config hash; stage all
three before replacing any; restore every original if post-delivery validation
fails.

## Manual command pattern

If you are not using the helper script, the minimum useful pattern per repo is:

```sh
.bin/archfit config update --migration-only --apply -c <temp-config>
.bin/archfit check --format=json -c <temp-config> --root <canonical-path>
.bin/archfit analyze --format=json -c <temp-config> --root <canonical-path>
.bin/archfit analyze --ai-summary --format=markdown -c <temp-overlay> --root <canonical-path>
```

The multi-author repos are heavier (SCIP on prometheus is slow). Use canonical-
case `--root` paths on macOS.

## Environment notes

`archfit doctor` first. A missing optional toolchain changes what a sweep can
prove, and silently attributing its absence to the code under test wastes a
sweep:

- **No Rust toolchain** (`cargo`, `rust-analyzer`, `cargo-modules`) — the four
  Rust repos report `cargo: partial` and `cargo-modules: absent`, and their
  structure/coupling/drift envelopes are `unmeasured`. That is the contract
  working, not a regression; the run still produces a valid v1 state.
- **A sandbox that denies Go module-cache writes** degrades `go/packages` and
  `scip-go` into partial coverage, which moves bands and confidence. Run sweeps
  with module-cache writes permitted, and check the `go/packages` coverage row
  before believing any Go anomaly.
- **Third-party imports are expected to be unresolved.** grimp reporting
  `telegram`, `structlog`, and `httpx` unresolved on ccgram is grimp declining
  to resolve site-packages, not a broken config.

## Related

- `skills/archfit-eval/`
- `docs/book-alignment-review-prompt.md`
