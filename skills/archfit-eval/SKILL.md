---
name: archfit-eval
description: Use to evaluate archfit itself against the book-alignment prompt, the test corpus, CLI UX, docs, and skill quality. Use when auditing archfit coverage vs Balancing Coupling, running corpus sweeps, comparing deterministic archfit output with semantic architecture review, or refreshing evaluation docs/scripts. Not for normal archfit use on a target repo; use the archfit skill.
license: Apache-2.0
compatibility: Requires repo files and shell access for build, doctor, test, and corpus commands. If command execution is unavailable, stay doc-only, say so, and lower confidence.
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

Thin skill for evaluating **archfit itself**. Keep the common-path procedure
here. Detailed review shape and corpus mechanics live in `references/` and the
helper script.

## Grounding

- Use the current branch binary: `.bin/archfit`.
- Run archfit commands from the archfit repo root so `.env` loading is stable.
- Corpus repos are read-only. Stage a candidate config outside the target and
  migrate THAT; never overwrite a target-repo config to run an evaluation.
- Run `.bin/archfit doctor` first. A missing toolchain changes what a sweep can
  prove, and blaming the code under test for it wastes the run.
- Treat config-update failures, missing tools, AI-key issues, noisy progress,
  stale docs, and unclear errors as findings — not as reasons to quietly skip.

## Routing

- Use `skills/archfit/` for normal archfit setup, config work, CI wiring, and
  single-repo finding repair.
- Use this skill for archfit self-eval, book alignment, corpus sweeps,
  deterministic-vs-semantic comparisons, and evaluation workflow maintenance.

## Read first

- `docs/book-alignment-review-prompt.md`
- `docs/test-corpus.md`
- `skills/archfit/SKILL.md`
- `references/book-alignment.md`
- `references/corpus-sweep.md`

## Workflow

1. Verify the tool you are evaluating:
   - `make build`
   - `.bin/archfit doctor`
   - `make test`
2. Run the self-check commands from `references/book-alignment.md`.
3. Test the harness itself, then run the deterministic corpus sweep:
   `python3 -m unittest discover -s scripts/eval -p '*corpus_sweep_test.py'`
   `python3 scripts/eval/corpus_sweep.py ... --strict`
   The mandatory fast matrix is one representative per supported adapter:
   spotinfo (Go), storybook (TypeScript), ccgram (Python), herdr (Rust).
4. Re-run every mandatory representative with the same candidate and compare
   `analyze --json` BYTE for byte. The v1 state carries no wall-clock or
   run-local field, so a byte difference is a determinism defect, not noise.
5. Run semantic architecture quick sweeps on representative repos. Compare only
   like-for-like: deterministic boundary drift vs semantic/runtime/ownership
   concerns.
6. Classify every anomaly — product defect, config migration defect, target
   architecture finding, optional/required tool gap, environment failure, or
   docs/UX defect. Leave nothing `unknown`.
7. Update stale docs, skills, prompts, or scripts in the same pass if the eval
   proves they are wrong.

## What v1 output looks like

`analyze --json` emits `archfit.architecture-state.v1`: a verdict, nine named
dimension envelopes, a coverage split that sums to nine, findings, agent tasks,
and the coupling seam ledger. There is **no repository score** — an eval that
reads `score.overall` is reading a field the cutover deleted, and an eval that
asks "what did it score" is asking the wrong question.

`check` exit IS the verdict: healthy 0, needs-attention 2, blocked 1, error 3.
**Expect 2, not 0, on a clean repository**: complexity, testability, and
operations report `partial` by contract in v1, and any partial dimension is
`needs_attention`. Only exit 1 means a hard gate blocked.

Every format reports the same verdict, dimension statuses, coverage split,
comparison state, and canonical finding sequence. Text, Markdown, and the
scorecard close with a finding index listing every finding ID in canonical
order; SARIF carries the state in `run.properties`.

## Output contract

Return one concise evaluation report with:

- book-alignment verdict and score
- corpus results by repo: status, config version and loadability,
  analyze/check exits and verdict, dimension statuses, coverage split, format
  parity, byte determinism
- semantic-vs-deterministic comparison notes
- UX/docs/skill findings
- exact commands run
- unverified gaps and follow-up actions

## Failure handling

- If shell access is unavailable, stop at a doc-only review, say verification
  was skipped, and lower confidence.
- If a target repo config is missing or broken, use a temp config and report
  the failure as part of UX/config findings.
- If a candidate config is not schema v2 or fails to load, keep the candidate,
  continue the run if possible, and record the config failure. Do not rewrite
  target repositories during evaluation.
- If a corpus repo is missing, record it as `unverified` with a reason. It is
  never a pass, and strict mode blocks until an owner accepts it by name.
- If AI summary fails, separate AI setup issues from deterministic gate issues.
- If semantic review finds something archfit does not, first decide whether that
  is a real archfit miss, a config-granularity choice, or intentionally
  semantic-only evidence.
