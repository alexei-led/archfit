# Wave 2 semantic evidence validation

Date: 2026-07-05
Run dir: `/tmp/archfit-wave2-task4-20260705155615`

## Scope

Task 4 validated LLM draft flows against the corpus without pinning generated suggestions into `.archfit.yaml` or `.archfit-labels.yaml`.

Scratch/temp paths used:

- `/tmp/archfit-self-init-llm-gpt-4.1-mini.yaml`
- `/tmp/archfit-self-update-llm-gpt-4.1-mini.txt`
- `/tmp/archfit-wave2-task4-20260705155615/self-scratch-openai/.archfit-labels.yaml`
- `/tmp/archfit-wave2-task4-20260705155615/corpus-llm/*`

## Deterministic controls

Commands passed before LLM draft checks:

- `make build`
- `make archfit`
- retired corpus-attribution helper

Corpus attribution before LLM commands:

| repo      | score | band  | scored | abstained | external |
| --------- | ----: | ----- | -----: | --------: | -------: |
| archfit   |    43 | mixed |    354 |         0 |      588 |
| ccgram    |    55 | mixed |    497 |        18 |        0 |
| herdr     |    29 | poor  |    686 |        18 |       24 |
| storybook |    49 | mixed |    330 |         0 |      181 |

Deterministic gate check:

- Command compared: `.bin/archfit analyze --gate --config .archfit.yaml --full`
- Before stdout SHA-256: `bad3397ecc9f382b626769dbb014d0b564910303e3135d5f986ad10bb1d0748b`
- After stdout SHA-256: `bad3397ecc9f382b626769dbb014d0b564910303e3135d5f986ad10bb1d0748b`
- Before stderr SHA-256: `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`
- After stderr SHA-256: `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`
- Result: stdout and stderr were byte-identical before/after LLM draft checks.
- `make archfit` also passed after LLM draft checks.

## LLM draft commands

Default self-check attempts:

- `.bin/archfit config init --llm --root . -o /tmp/archfit-self-init-llm.yaml` failed closed with `classify failed: classify: model response is not the required JSON array: unexpected end of JSON input`.
- `.bin/archfit config init --llm --llm-provider openai --llm-model gpt-4.1 --root . -o /tmp/archfit-self-init-llm-openai.yaml` failed closed with `classify entry "score" has invalid evidence_refs "docs/design/20260702-bc-score-v4.md"`.
- Both failures are acceptable deterministic controls: no config was mutated and invalid evidence refs were rejected.

Successful scratch/temp runs:

- `.bin/archfit config init --llm --llm-provider openai --llm-model gpt-4.1-mini --root . -o /tmp/archfit-self-init-llm-gpt-4.1-mini.yaml`
- `.bin/archfit config update --llm --llm-provider openai --llm-model gpt-4.1-mini --root . --config .archfit.yaml > /tmp/archfit-self-update-llm-gpt-4.1-mini.txt`
- `.bin/archfit config enrich abstained --root . --config /tmp/archfit-wave2-task4-20260705155615/self-scratch-openai/.archfit.yaml`
  - Result: `enrich abstained: no abstained cross-module edges — nothing to label`.
- Corpus `config update --llm --llm-provider openai --llm-model gpt-4.1-mini` on scratch config copies for:
  - Go: `archfit`
  - Python: `prefect` (`ccgram` emitted removal/path-drift output only, so `prefect` supplied Python draft samples)
  - Rust: `herdr`
  - TypeScript: `storybook`

## Spot-check results

No draft was pinned. Accept/reject means "acceptable candidate for human review/pinning".

| language   | repo      | inspected | accepted | rejected | notes                                                                                                               |
| ---------- | --------- | --------: | -------: | -------: | ------------------------------------------------------------------------------------------------------------------- |
| Go         | archfit   |         5 |        5 |        0 | Good evidence refs from API/comment/doc IDs; no sampled false positives.                                            |
| Python     | prefect   |         6 |        5 |        1 | Useful domain guesses, but one weak layer/role sample needs human review.                                           |
| Rust       | herdr     |         6 |        4 |        2 | Good Rust module decomposition, but inherited config-only evidence caused overconfident `deterministic_fact` bases. |
| TypeScript | storybook |         8 |        6 |        2 | Useful package-level suggestions; one truncated rationale and one weak volatility claim rejected.                   |
| Total      | corpus    |        25 |       20 |        5 | Draft quality is useful for review, not safe for automatic pinning.                                                 |

Accepted Go samples:

- `agenttask`: accepted as supporting/low; cites `api:agenttask` and `comment:internal/agenttask/agenttask.go`.
- `baseline`: accepted as supporting/low; cites `api:baseline` and `comment:internal/baseline/baseline.go`.
- `calibrate`: accepted as supporting/medium; cites command and package comments.
- `classify`: accepted as core/high with role `core`; cites classifier API/comment evidence.
- `cmd_archfit`: accepted as generic composition root; cites CLI comments and `doc:README.md`.

Accepted Python samples:

- `assets`: accepted as core/medium; paths include asset domain files.
- `blocks`: accepted as core/high; paths include central block domain files.
- `cli`: accepted as generic/high command adapter.
- `client`: accepted as supporting API client adapter.
- `locking`: accepted as generic/low solved infrastructure.

Rejected Python sample:

- `analytics`: rejected pending review. The classification as `supporting` is plausible, but `layer: core` and the rationale read as generic data-analysis inference rather than Prefect-specific architecture intent.

Accepted Rust samples:

- `herdr-api-client`: accepted as adapter/high for API client boundary.
- `herdr-api-event_hub`: accepted as core service/high.
- `herdr-api-schema`: accepted as supporting shared model/medium.
- `herdr-api-schema-agents`: accepted as supporting shared model/medium.

Rejected Rust samples:

- `herdr-agent_resume`: rejected pending review because it marks a per-module judgment as `deterministic_fact` while citing only the scratch config.
- `herdr-api`: rejected pending review for the same config-only `deterministic_fact` overconfidence.

Accepted TypeScript samples:

- `cli-sb`: accepted as delivery adapter/supporting.
- `cli-storybook`: accepted as delivery adapter/supporting.
- `codemod`: accepted as generic/high code-transformation module.
- `core-webpack`: accepted as core/medium build integration.
- `csf-plugin`: accepted as supporting plugin module.
- `react-dom-shim`: accepted as adapter/generic.

Rejected TypeScript samples:

- `create-storybook`: rejected because the rationale was truncated (`paths: lib/cre`).
- `eslint-plugin`: rejected pending review because `volatility: high` was asserted from `api:eslint-plugin` alone.

## Key false positives and controls

- Some models emit invalid evidence refs, e.g. bare `docs/design/20260702-bc-score-v4.md` instead of a valid evidence ID. The parser rejected this and left config unchanged.
- Rust synthetic module suggestions can overuse config-only evidence and call semantic guesses `deterministic_fact`. These should remain review-only until backed by doc/API/code evidence.
- Package-level TypeScript suggestions can infer volatility from package kind alone. Require human review before pinning.
- Truncated rationales should be rejected even when the structural fields look plausible.

## Worktree state

Checked after validation:

- `archfit`: no tracked modifications from validation. Pre-existing untracked `.pi/` remains outside the commit.
- `ccgram`: clean.
- `prefect`: clean.
- `storybook`: clean.
- `herdr`: no tracked modifications. Pre-existing untracked files remain: `.archfit.yaml` (mtime 2026-07-02 14:35:09), `.claude/` (mtime 2026-06-21 13:17:06), `.codegraph/` (mtime 2026-07-05 11:10:10), `index.scip` (mtime 2026-06-26 12:56:30). Generated `.archfit-cache/` was removed after validation.
