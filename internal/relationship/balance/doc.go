// Package balance gives the balance of a dependency that does not exist yet:
// a use of the public part, a use past the public part, or a copy between two
// modules. It serves `can`, `request`, the preview, and the `coupling` rule.
//
// The balance is the book formula (bc_score.v7), computed by
// scoring.BookScorer at the cross-module distance. This package never copies
// the formula or its ordinals.
//
// It uses the declared volatility of the two modules and no volatility
// cascade: with a cascade, one new dependency would change the balance of code
// the change did not touch. The volatility that applies is the target's for a
// use of the public part (contract or model strength) and the higher of the
// two for a use past it (functional or intrusive) and for a copy (symmetric).
//
// Intrusive differs from relationship/classify, which keeps the target's
// volatility for an intrusive edge. An intrusive use reaches past the public
// part, and the coupling rule lets that happen only into a stable module, so a
// volatile source counts too.
//
// Assessment may import this package. It is deliberately absent from the
// package list of TestAssessmentConsumesOnlyThePublicRelationshipContract and
// of the assessment_no_relationship_internals rule; do not add it there. Its
// signature uses the relationship aliases and plain values, so a caller never
// names a coupling type.
package balance
