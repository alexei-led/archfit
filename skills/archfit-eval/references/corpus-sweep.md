# Archfit corpus sweep

Use this when evaluating archfit against the repos in `docs/test-corpus.md`.

## Scope

This is a runtime evaluation workflow. It needs shell access. It is not a
read-only document review.

## Grounding

- Ground claims in current tool output from `.bin/archfit`, not memory.
- Run from the archfit repo root so `.env` loading is consistent.
- Use the current branch binary: `.bin/archfit`.
- Keep target repos read-only. Use candidate configs staged outside them.
- Run `.bin/archfit doctor` first and read `docs/test-corpus.md` §Environment
  notes. A missing toolchain changes what the sweep can prove; attributing its
  absence to the code under test wastes the run.
- Quote command exits and stderr when something fails.

## Config policy

- If the target repo already has `.archfit.yaml`, copy it to the sweep output
  directory. That copy is the **candidate**; the target file is never written.
- If it has no config, generate one there with `config init` (which already
  emits schema v2).
- Migrate the candidate with `config update --migration-only --apply`. Do NOT
  use the full `config update --apply`: it also proposes structural module
  edits, and bundling those into a schema migration hides whether the migration
  itself is idempotent, and can silently rewrite authored settings.
- Run the migration twice. The second run must leave the file byte-identical.
- For an AI summary, copy the candidate again and add the `ai:` block to the
  COPY. An overlay is never a delivery candidate.
- A config-update failure is a recorded finding, never a silent skip.

## What a passing repo must show

`analyze --json` emits `archfit.architecture-state.v1`. There is no repository
score: a sweep reading `score.overall` or `score.overall_band` is reading fields
the v1 cutover deleted.

- `schema_version == "archfit.architecture-state.v1"`
- the nine dimension keys, in contract order: intent, structure, modularity,
  coupling, change_locality, complexity, testability, operations, drift
- every `dimensions.<d>.metrics` an ARRAY of `{name, value, unit}` records
- `coverage.measured + coverage.partial + coverage.unmeasured == 9`, and equal
  to what the nine envelopes themselves report
- the candidate at `version: 2`, idempotent on a second migration
- `check` exiting exactly what its verdict says: healthy 0, needs-attention 2,
  blocked 1, error 3

**Expect exit 2, not 0, on a clean repository.** Complexity, testability, and
operations report `partial` by contract in v1, and any partial dimension is
`needs_attention`. Only exit 1 means a hard gate blocked.

## Format parity

Every format reports the same verdict, dimension statuses, coverage split,
comparison state, and canonical finding sequence. Text, Markdown, and the
scorecard may lead with an abbreviated actionable excerpt, but each closes with
a finding index carrying every finding ID — accepted and waived included — in the
document's canonical order. Abbreviated output without that appendix fails
parity. SARIF is exempt from human LAYOUT only: the state rides in
`run.properties`, and each finding keeps its `archfit/v1` fingerprint.

## Determinism check

Repeat `analyze --json` on a representative with the same candidate and compare
BYTES:

```sh
.bin/archfit analyze --format=json -c <same-candidate> --root <same-repo> | cmp - first.json
```

The v1 state carries no wall-clock value, absolute path, or PID, so there is
nothing to exclude. A byte difference is a real defect — and because the second
run reads the fact cache the first one filled, it is also the cold-vs-warm cache
check.

## Helper script

```sh
python3 -m unittest discover -s scripts/eval -p '*corpus_sweep_test.py'
python3 scripts/eval/corpus_sweep.py --help
```

Fast four-language matrix (one representative per supported adapter):

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

Full corpus: swap in all eleven labels from `docs/test-corpus.md` and
`--ai-repos spotinfo,ccgram,herdr,storybook --max-workers 4`.

`--strict` returns 0 only when every record is `pass`, or every non-pass record
is an owner-supplied `--allow-unverified label=reason` gap. Such a gap is
recorded as `accepted_unverified` and NEVER as a pass — the final review must be
able to see it. A repo that is not cloned is `unverified` and blocks strict mode
until someone accepts it by name and reason.

The script uses `.bin/archfit` from the current branch, runs from the archfit
repo root, writes candidates and artifacts under `--output-dir`, and writes the
frozen per-repo records to `--summary-file`. Nothing it writes belongs in git.

## Manual command pattern

```sh
.bin/archfit config update --migration-only --apply -c <candidate>
.bin/archfit config update --migration-only --apply -c <candidate>   # must not change it
.bin/archfit check   --format=json -c <candidate> --root <repo-root>
.bin/archfit analyze --format=json -c <candidate> --root <repo-root>
.bin/archfit analyze --ai-summary --format=markdown -c <ai-overlay> --root <repo-root>
```

Use AI summary only on selected representative repos. It is advisory, not gate
truth: a credential or provider failure is an environment fact, while an AI-path
schema or projection defect is still a product defect.

## Classifying what a sweep surfaces

Do not leave an anomaly unclassified.

- **Product defect** — the deterministic contract is wrong. Reproduce it
  minimally, run GitNexus upstream impact on every production symbol you intend
  to edit, fix it, and add a regression test that fails without the fix.
- **Config migration defect** — the migration lost, weakened, or invented a
  setting. Same bar.
- **Target architecture finding** — the repo really does have that coupling.
  Record it. Never edit a target's modules, owners, volatility, labels,
  baselines, or rules to make output look better.
- **Optional tool gap** — an opt-in analyzer is off or its binary is missing.
  It stays an explicit coverage gap: it must not crash, disappear, or read as
  healthy. A required-tool failure keeps its configured hard behaviour.
- **Environment failure** — missing credentials or toolchain. Recorded
  separately from product defects.
- **Docs/UX defect** — the output is correct but misleads.

Re-run the affected repo after each fix, then re-run the fast matrix; re-run the
full corpus after the last fix.

## Semantic comparison rules

Choose representative repos for semantic quick sweeps. One per language is ideal;
three total is the minimum useful contrast.

Compare like-for-like:

- **Archfit should catch:** explicit dependency direction, forbidden edges,
  declared layer drift, measurable coupling seams, missing-tool abstentions,
  config/update friction, CLI/docs/skill drift.
- **Semantic review may catch first:** intra-module responsibility mixing,
  runtime side effects, ownership/process coupling, composition-root bloat,
  migration seams, ambiguous product/infrastructure boundaries.

Do not score archfit down for a semantic-only concern unless the current config
and deterministic model should already encode it.

## Failure handling

- If shell access is unavailable, this workflow is blocked. Do not pretend the
  sweep ran.
- If a target repo has no config, generate a candidate and keep going.
- If the migration fails on the candidate, record the exit and stderr and still
  try `check` / `analyze` on it.
- If AI summary fails, capture the exact stderr and separate it from
  deterministic findings.
- If a repo is missing or too heavy for the current run, record it as
  `unverified` — never silently drop it, and never call it a pass.

## Useful findings to capture

Per repo, record:

- config outcome: source, source hash, candidate hash, migrated version,
  second-migration exit and whether it changed anything
- `check` verdict and exit code, and whether they agree
- `analyze` verdict, dimension statuses, and the coverage split
- missing-tool and abstention signals, with the disclosed reason
- format parity result per format
- byte determinism on repeat
- AI summary success/failure and whether the narrative helped
- UX/docs issues: flags, progress messages, errors, migration friction,
  unexpected exits, confusing output
- whether the repo is still useful as a future regression target
