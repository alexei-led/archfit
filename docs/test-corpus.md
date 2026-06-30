# archfit test corpus

Repos used to dogfood and validate `archfit analyze` (full + delta + LLM) across all
supported languages and ownership models. Cloned under `~/workspace/<name>`.

Two groups: **local dogfood repos** (small, mostly single-owner — fast inner-loop checks)
and **multi-author repos** (popular, multi-team — exercise socio-technical owner-distance,
CODEOWNERS resolution, and coupling at scale).

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

## Usage

Run each from the archfit dir so `.env` auto-loads the LLM key (see eval finding F3):

```
.bin/archfit analyze --full  --root <canonical-path> --config <cfg> --advisory --json
.bin/archfit analyze --full  --root <canonical-path> --config <cfg> --advisory --llm --markdown
.bin/archfit analyze --base <prev-minor> --full --root <canonical-path> --config <cfg> --advisory
```

The multi-author repos need per-repo module configs (owner-less, so CODEOWNERS/git fills owners)
and are heavier (SCIP on prometheus is slow). Use canonical-case `--root` paths on macOS (F4).

## Related

- Eval findings: `reports/eval-2026-06-30/00-FINDINGS.md`
- Owner-detection probe across repos: `reports/eval-2026-06-30/owner-detection-probe.md`
