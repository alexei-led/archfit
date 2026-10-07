# Glossary

This page maps the words archfit prints to the terms of _Balancing Coupling in
Software Design_ (Vlad Khononov). Chapter numbers follow the other archfit
guides. A term marked **archfit-only** has no book counterpart: it names an
archfit mechanism.

## Human terms and wire terms

Text and Markdown use the human term. JSON and SARIF use the wire term.

| Human term        | Wire term                                   | Meaning                                                                      |
| ----------------- | ------------------------------------------- | ---------------------------------------------------------------------------- |
| blocker           | gate finding (`kind: gate`, active)         | A finding that makes `check` exit `1`.                                       |
| diagnostic        | advisory finding (`kind: advisory`, active) | A finding that never blocks. It can make the verdict `needs_attention`.      |
| accepted          | baselined finding (`status: baseline`)      | A finding the stored baseline holds. It does not block.                      |
| waived            | `status: waived`                            | A finding a config waiver covers until its expiry date.                      |
| analyzer coverage | `coverage`, `coverage_gaps`                 | Which analyzers ran and what they saw. It is not test coverage.              |
| test coverage     | `coverage:` config section, `testability`   | A coverage report you supply. archfit never runs your tests.                 |
| seam              | `seams[]`                                   | One ordered module pair with at least one import edge.                       |
| seam status       | `seams[].policy`                             | What the policy says about a seam: violation, accepted, advisory, allowed, or observed. |
| next step         | none                                        | A line of the text and Markdown brief. JSON carries the facts it comes from. |

## Book terms

| archfit term                        | Book term                             | Chapter      | Notes                                                                                                         |
| ----------------------------------- | ------------------------------------- | ------------ | ------------------------------------------------------------------------------------------------------------- |
| strength (`contract` … `intrusive`) | integration strength                  | Ch7          | `contract`, `model`, `functional`, `intrusive`. `symmetric` is functional coupling that duplicates knowledge. |
| `dto`                               | contract coupling on a data structure | Ch7          | A data-only type across a declared `public:` boundary.                                                        |
| distance                            | distance                              | Ch8          | Level-relative rungs: same module (2), any module boundary (9), declared external system (10). Owner and deploy unit name the boundary only.          |
| volatility                          | volatility                            | Ch9          | Declared, or derived from `subdomain`. An undeclared volatility scores as V=10, the worst case.               |
| volatility cascade                  | volatility propagation                | Ch9          | Opt-in: `coupling.volatility_cascade`.                                                                        |
| balance score                       | balance (`max(\|S−D\|, 10−V) + 1`)    | Ch10         | Per edge, 1 to 10. Higher is better balanced.                                                                 |
| `local_coupling`                    | local complexity                      | Ch10         | Same-module edges. Reported, never gated.                                                                     |
| quadrant                            | the four coupling quadrants           | Ch10         | Per seam.                                                                                                     |
| connascence evidence                | connascence                           | Ch6          | Static evidence only. Dynamic kinds stay unmeasured.                                                          |
| distributed-monolith seam           | distributed monolith (global complexity) | Ch10         | A seam with a strong import edge (functional, intrusive or symmetric) across a module boundary into a declared high-volatility module. The only coupling gate.                                              |
| dimension                           | none                                  | archfit-only | One of nine areas the architecture state measures.                                                            |
| measured / partial / unmeasured     | none                                  | archfit-only | Whether every required fact of a dimension was observed.                                                      |
| out of claim                        | none                                  | archfit-only | A fact a dimension does not claim to measure. A NOT MEASURED line marks it "no action".                       |
| gate reference                      | none                                  | archfit-only | The stored baseline that the seam gate and the drift dimension compare against.                               |
| fingerprint                         | none                                  | archfit-only | `classification_hash`, `model_hash`, `labels_hash`, `rubric_version`. Two runs compare only when all match.           |
| agent task                          | none                                  | archfit-only | The repair contract of one blocker: goal, files, constraints, and check command.                              |

## Where each term appears

- The brief (text and Markdown) lists blockers, then next steps, then the
  dimensions. See [Commands](commands.md).
- The architecture state (`--format json`) is the contract. See
  [Concepts](concepts.md) and the
  [evidence contract](../design/evidence-contract.md).
