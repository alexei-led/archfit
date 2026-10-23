# Coupling model evaluation

Use this reference when assessing archfit against _Balancing Coupling in
Software Design_. Use [`docs/development.md`](../../../docs/development.md) for
the evaluation workflow and [`docs/coupling.md`](../../../docs/coupling.md) for
the current product contract.

## Review method

Read the local book at `.book/9780137353576.epub`. Check the chapters on
connascence, integration strength, distance, volatility, balance, refactoring,
and practical use. Cite chapter or section numbers. Do not copy book text into
reports.

For every claimed mismatch:

1. State the book rule or scale.
2. Trace archfit's implementation and tests with `file:line` evidence.
3. Decide whether archfit implements the rule, adapts it, partially measures
   it, or leaves it to human judgment.
4. Reproduce behavior with a focused fixture or controlled command when possible.
5. Separate a deterministic product gap from semantic-only judgment, missing
   tool evidence, and policy choice.

## Current coupling behavior to verify

- The balance formula is `max(|S − D|, 10 − V) + 1`.
- The shipped v7 model assigns D=9 to every module boundary. Ownership labels
  name boundaries. They do not change distance or severity.
- `coupling_balance` is diagnostic. The distributed-monolith seam gate checks
  qualifying seams against a comparable baseline.
- Missing strength or distance evidence abstains. It does not become a passing
  score.

Check these statements against current code, tests, and the coupling reference.
If the implementation changes, update the owning product documentation in the
same change.
