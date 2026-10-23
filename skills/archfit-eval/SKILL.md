---
name: archfit-eval
description: Use to evaluate archfit itself against its coupling model, language coverage, corpus behavior, CLI, docs, or skills. Use when assessing book alignment, running corpus sweeps, or maintaining evaluation workflows. Not for normal archfit use on a target repository; use the archfit skill.
license: Apache-2.0
compatibility: Requires the archfit repository and shell access for build, doctor, tests, and evaluation commands. If command execution is unavailable, stay doc-only and lower confidence.
metadata:
  author: alexei-led
  tags:
    - archfit
    - evaluation
    - book-alignment
    - corpus
    - ux
---

# Archfit Evaluation

Evaluate archfit against current implementation and evidence. The evaluation
workflow lives in `docs/development.md`. Read it before running a sweep.

## Grounding

- Use the current branch binary: `.bin/archfit`.
- Run commands from the repository root so `.env` loading is stable.
- Run `.bin/archfit doctor` first. Record missing tools as coverage gaps.
- Use disposable target checkouts and candidate configs. Do not alter original
  corpus repositories or their policy files.
- Treat failures, missing tools, unclear output, and stale documentation as
  findings. Do not silently skip them.

## Workflow

1. Read `docs/development.md` and the relevant product references.
2. Build and verify the candidate with the commands in that guide.
3. Run the bounded language contracts or corpus sweep for the requested scope.
4. Compare repeated JSON output byte for byte when the sweep requests it.
5. Classify each anomaly as product, policy migration, target architecture,
   tool/environment, or documentation behavior.
6. Update the owning docs or skill when the evidence shows a defect.

The state JSON identifier is `archfit.architecture-state.v1`. The current
configuration schema is version 2. The v3 engine uses baseline schema 3 and
supports `archfit baseline --reanchor` for accepted findings. Do not infer
compatibility from the version string alone. See the [migration guide](../../docs/migration.md).

## Output

Return a concise report with the evaluation scope, verdict, evidence, commands,
unverified gaps, and follow-up actions. Cite code as `file:line` and book
references by chapter or section. Keep fresh output under `/tmp` or
`.archfit-cache/`. Keep dated measurements and old reports in `docs/archived/`.

## Failure handling

- If a target checkout is missing, record it as unverified.
- If a candidate config fails to load, record the error and continue only when
  the remaining checks can produce useful evidence.
- If an AI summary fails, separate provider setup from deterministic results.
- If semantic review finds a concern, decide whether current policy can encode
  it before calling it an archfit defect.
