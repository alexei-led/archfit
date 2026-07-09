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
- Use temp configs and temp reports during corpus sweeps. Do not overwrite
  target-repo configs just to run an evaluation.
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
3. Run the deterministic corpus sweep with the helper script or equivalent:
   `python3 scripts/eval/corpus_sweep.py ...`
4. Re-run at least one representative repo with the same temp config and compare
   JSON output. If it changes, treat that as a determinism finding.
5. Run semantic architecture quick sweeps on representative repos. Compare only
   like-for-like: deterministic boundary drift vs semantic/runtime/ownership
   concerns.
6. Update stale docs, skills, prompts, or scripts in the same pass if the eval
   proves they are wrong.

## Output contract

Return one concise evaluation report with:

- book-alignment verdict and score
- corpus results by repo
- semantic-vs-deterministic comparison notes
- UX/docs/skill findings
- exact commands run
- unverified gaps and follow-up actions

## Failure handling

- If shell access is unavailable, stop at a doc-only review, say verification
  was skipped, and lower confidence.
- If a target repo config is missing or broken, use a temp config and report
  the failure as part of UX/config findings.
- If `config update --apply` fails on a copied config, keep the copied temp
  config, continue the run if possible, and record the update failure.
- If AI summary fails, separate AI setup issues from deterministic gate issues.
- If semantic review finds something archfit does not, first decide whether that
  is a real archfit miss, a config-granularity choice, or intentionally
  semantic-only evidence.
