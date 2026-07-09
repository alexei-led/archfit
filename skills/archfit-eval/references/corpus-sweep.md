# Archfit corpus sweep

Use this when evaluating archfit against the repos in `docs/test-corpus.md`.

## Scope

This is a runtime evaluation workflow. It needs shell access. It is not a
read-only document review.

## Grounding

- Ground claims in current tool output from `.bin/archfit`, not memory.
- Run from the archfit repo root so `.env` loading is consistent.
- Use the current branch binary: `.bin/archfit`.
- Keep target repos read-only. Use temp configs and temp outputs.
- Quote command exits and stderr when something fails.

## Temp-config policy

- If the target repo already has `.archfit.yaml`, copy it to a temp path.
- If it has no config, generate one to a temp path with `config init`.
- If you want AI summary on a temp config that has no `ai:` block, add the
  minimal block only to the temp copy.
- Run `config update --apply` on the temp copy, not the target repo config.
- If `config update --apply` fails, keep the copied/generated temp config,
  continue the sweep if `analyze`/`check` still work, and record the failure as a
  UX/config finding.

## Helper script

Prefer the helper script for repeated sweeps:

```sh
python3 scripts/eval/corpus_sweep.py \
  --repos spotinfo,ccgram,herdr \
  --ai-repos spotinfo,ccgram,herdr \
  --repeat-repos spotinfo
```

The script:

- uses `.bin/archfit` from the current branch
- runs from the archfit repo root
- writes temp configs and artifacts under `/tmp/archfit-corpus-eval/`
- writes a summary JSON file at `/tmp/archfit-corpus-results.json` by default
- treats config-update failures as recorded findings instead of silent skips

Use `--help` for options.

## Manual command pattern

If you are not using the helper script, the minimum useful pattern per repo is:

```sh
.bin/archfit check --json -c <temp-config> --root <repo-root>
.bin/archfit analyze --json -c <temp-config> --root <repo-root>
.bin/archfit analyze --ai-summary --markdown -c <temp-config> --root <repo-root>
```

Use AI summary only on selected representative repos. It is advisory, not gate
truth.

## Determinism check

Repeat at least one representative repo run with the same temp config:

```sh
.bin/archfit analyze --json -c <same-temp-config> --root <same-repo>
```

Compare parsed JSON, not human summaries. If it changes, report the exact field
or output difference.

## Semantic comparison rules

Choose representative repos for semantic quick sweeps. One per language is ideal;
three total is the minimum useful contrast.

Compare like-for-like:

- **Archfit should catch:** explicit dependency direction, forbidden edges,
  declared layer drift, measurable coupling hotspots, missing-tool abstentions,
  config/update friction, CLI/docs/skill drift.
- **Semantic review may catch first:** intra-module responsibility mixing,
  runtime side effects, ownership/process coupling, composition-root bloat,
  migration seams, ambiguous product/infrastructure boundaries.

Do not score archfit down for a semantic-only concern unless the current config
and deterministic model should already encode it.

## Failure handling

- If shell access is unavailable, this workflow is blocked. Do not pretend the
  sweep ran.
- If a target repo has no config, generate one to a temp path and keep going.
- If `config update --apply` fails on the temp copy, record the error and still
  try `check` / `analyze` on that temp config.
- If AI summary fails, capture the exact stderr and separate it from
  deterministic findings.
- If a repo is too heavy or too slow for the current run, mark it unverified
  instead of silently dropping it.

## Useful findings to capture

Per repo, record:

- config outcome: copied, generated, updated, update-failed
- `check` verdict and exit code
- `analyze` verdict and score band
- missing-tool or abstention signals
- AI summary success/failure and whether the narrative helped
- UX/docs issues: flags, progress messages, errors, config-update friction,
  unexpected exits, confusing output
- whether the repo seems genuinely useful as a future regression target
