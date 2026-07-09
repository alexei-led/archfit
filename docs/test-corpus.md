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
- Use temp configs outside the target repos.
- If a repo already has `.archfit.yaml`, copy it and update the temp copy.
- If `config update --apply` fails on the temp copy, keep going if `check` /
  `analyze` still work, and record the failure as a config/UX finding.
- Re-run at least one representative repo with the same temp config and compare
  parsed JSON to check determinism.
- Use AI summary on selected representative repos, not necessarily every repo.

## Helper script

For repeated sweeps, prefer:

```sh
python3 scripts/eval/corpus_sweep.py --help
```

Example:

```sh
python3 scripts/eval/corpus_sweep.py \
  --repos spotinfo,pumba,ccgram,herdr,storybook \
  --ai-repos spotinfo,ccgram,herdr \
  --repeat-repos spotinfo
```

The script:

- uses `.bin/archfit` from the current branch
- runs from the archfit repo root
- writes temp configs and artifacts under `/tmp/archfit-corpus-eval/`
- writes a summary JSON file at `/tmp/archfit-corpus-results.json` by default

## Manual command pattern

If you are not using the helper script, the minimum useful pattern per repo is:

```sh
.bin/archfit check --json -c <temp-config> --root <canonical-path>
.bin/archfit analyze --json -c <temp-config> --root <canonical-path>
.bin/archfit analyze --ai-summary --markdown -c <temp-config> --root <canonical-path>
```

The multi-author repos are heavier (SCIP on prometheus is slow). Use canonical-
case `--root` paths on macOS.

## Related

- `skills/archfit-eval/`
- `docs/book-alignment-review-prompt.md`
