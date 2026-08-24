package scoring

import "github.com/alexei-led/archfit/internal/relationship/coupling"

// Scorer computes a score for one classified relationship.
type Scorer interface {
	Score(coupling.Classification) coupling.EdgeScore
}

// Classification is the neutral relationship value consumed by scorers.
type Classification = coupling.Classification

// EdgeScore is the neutral score value returned by scorers.
type EdgeScore = coupling.EdgeScore

// Severity is the score band value.
type Severity = coupling.Severity

// Strength is the integration-strength value.
type Strength = coupling.Strength

// Distance is the socio-technical distance value.
type Distance = coupling.Distance

// Volatility is the target-volatility value.
type Volatility = coupling.Volatility

// ScoreBreakdown is the raw scorer-ordinal record.
type ScoreBreakdown = coupling.ScoreBreakdown

// Coupling values are re-exported for scorer implementations and calibration tests.
const (
	StrengthContract             = coupling.StrengthContract
	StrengthModel                = coupling.StrengthModel
	StrengthUnknown              = coupling.StrengthUnknown
	StrengthFunctional           = coupling.StrengthFunctional
	StrengthIntrusive            = coupling.StrengthIntrusive
	StrengthSymmetric            = coupling.StrengthSymmetric
	DistanceSameModule           = coupling.DistanceSameModule
	DistanceCrossModuleSameOwner = coupling.DistanceCrossModuleSameOwner
	DistanceUnknown              = coupling.DistanceUnknown
	DistanceCrossModuleDiffOwner = coupling.DistanceCrossModuleDiffOwner
	DistanceCrossDeployUnit      = coupling.DistanceCrossDeployUnit
	DistanceExternal             = coupling.DistanceExternal
	VolatilityLow                = coupling.VolatilityLow
	VolatilityMedium             = coupling.VolatilityMedium
	VolatilityHigh               = coupling.VolatilityHigh
	VolatilityUndeclared         = coupling.VolatilityUndeclared
	VolatilityUnknown            = coupling.VolatilityUnknown
	VolatilityFrozen             = coupling.VolatilityFrozen
	SeverityNone                 = coupling.SeverityNone
	SeverityLow                  = coupling.SeverityLow
	SeverityMedium               = coupling.SeverityMedium
	SeverityHigh                 = coupling.SeverityHigh
	SeverityCritical             = coupling.SeverityCritical
)

// ScoreDefinition is the canonical, user-facing definition of archfit's numeric
// BC score. Implements Vlad Khononov's published formula from _Balancing Coupling
// in Software Design_ Ch10 verbatim: balance = max(|S−D|, 10−V) + 1.
// Range 1 (distributed monolith / ball-of-mud) to 10 (frozen/contract); higher
// is better balanced. Strength, distance, and volatility ordinals are fixed per
// the book (Ch8–Ch10); changing any is a breaking metric change (bump ScoreVersion).
const ScoreDefinition = "book balance score — balance = max(|S−D|, 10−V) + 1 " +
	"(Khononov, _Balancing Coupling in Software Design_, Ch10); " +
	"range 1 (distributed monolith) to 10 (frozen/contract); higher = better balanced"

// ---------------------------------------------------------------------------
// Legacy calibration ordinals.
//
// BookScorer in scorer_book.go is the production scorer and owns the book-exact
// ordinals used by coupling_balance. The constants below are kept for the
// additive/multiplicative calibration scorers only; do not cite them as the
// current Balanced-Coupling scale.
// ---------------------------------------------------------------------------

const (
	strengthOrdinalContract   = 0
	strengthOrdinalModel      = 2
	strengthOrdinalUnknown    = 3
	strengthOrdinalFunctional = 5
	strengthOrdinalIntrusive  = 8
)

const (
	distanceOrdinalSameModule           = 0
	distanceOrdinalCrossModuleSameOwner = 1
	distanceOrdinalUnknown              = 2
	distanceOrdinalCrossModuleDiffOwner = 3
	distanceOrdinalCrossDeployUnit      = 5
	distanceOrdinalExternal             = 6 // declared external system: farthest rung (legacy calibration scorers only)
)

const (
	volatilityDiscountLow        = 2
	volatilityDiscountMedium     = 1
	volatilityDiscountHigh       = 0
	volatilityDiscountUndeclared = 0
	volatilityDiscountUnknown    = 0
)

// strengthOrdinal maps Strength to its risk ordinal (values frozen — see consts above).
// contract=0: declared interface, lowest coupling risk.
// model=2: concrete type import, low risk.
// unknown=3: unresolved; default to moderate risk.
// functional=5: implementation-level call.
// intrusive=8: internal/private access, highest coupling intensity.
var strengthOrdinal = map[Strength]int{
	StrengthContract:   strengthOrdinalContract,
	StrengthModel:      strengthOrdinalModel,
	StrengthUnknown:    strengthOrdinalUnknown,
	StrengthFunctional: strengthOrdinalFunctional,
	StrengthIntrusive:  strengthOrdinalIntrusive,
}

// distanceOrdinal maps Distance to its risk ordinal (values frozen — see consts above).
// same_module=0: co-located, no boundary crossed.
// cross_module_same_owner=1: nearby boundary, shared accountability.
// unknown=2: unresolved; default to cross-module risk.
// cross_module_diff_owner=3: separate accountability boundaries.
// cross_deploy_unit=5: highest deployment boundary.
var distanceOrdinal = map[Distance]int{
	DistanceSameModule:           distanceOrdinalSameModule,
	DistanceCrossModuleSameOwner: distanceOrdinalCrossModuleSameOwner,
	DistanceUnknown:              distanceOrdinalUnknown,
	DistanceCrossModuleDiffOwner: distanceOrdinalCrossModuleDiffOwner,
	DistanceCrossDeployUnit:      distanceOrdinalCrossDeployUnit,
	DistanceExternal:             distanceOrdinalExternal,
}

// volatilityDiscount maps Volatility to a discount subtracted from the raw score
// (values frozen — see consts above).
// Low volatility = higher discount (stable target reduces coupling risk).
// Unknown = no discount (conservative; treat as potentially volatile).
var volatilityDiscount = map[Volatility]int{
	VolatilityLow:        volatilityDiscountLow,
	VolatilityMedium:     volatilityDiscountMedium,
	VolatilityHigh:       volatilityDiscountHigh,
	VolatilityUndeclared: volatilityDiscountUndeclared,
	VolatilityUnknown:    volatilityDiscountUnknown,
}

// ScoreBand maps a book balance value 1..10 to a Severity band.
// Higher balance = better balanced coupling = lower (or no) finding.
// Bands: 1–2 critical · 3–4 high · 5–6 medium · 7–8 low · 9–10 none.
func ScoreBand(balance int) Severity {
	switch {
	case balance <= 2:
		return SeverityCritical
	case balance <= 4:
		return SeverityHigh
	case balance <= 6:
		return SeverityMedium
	case balance <= 8:
		return SeverityLow
	default:
		return SeverityNone
	}
}

// legacyScoreBand maps a legacy risk score [0,10] to a Severity band.
// Used only by AdditiveScorer and MultiplicativeScorer (kept for calibration).
// Lower score = lower risk = lower/no finding.
// Bands: 0–2 none · 3–4 low · 5–6 medium · 7–8 high · 9–10 critical.
func legacyScoreBand(score int) Severity {
	switch {
	case score <= 2:
		return SeverityNone
	case score <= 4:
		return SeverityLow
	case score <= 6:
		return SeverityMedium
	case score <= 8:
		return SeverityHigh
	default:
		return SeverityCritical
	}
}

// DefaultScorer returns the scorer used by classify.Run when the config
// does not specify one. BookScorer implements Vlad Khononov's published formula
// from _Balancing Coupling in Software Design_ Ch10; ScoreVersion records later
// semantic changes in the facts feeding that scorer.
func DefaultScorer() Scorer {
	return BookScorer{}
}

// clamp returns v clamped to [lo, hi].
func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Cheapest-move labels for a strength or distance change — the two Ch11
// remediation levers every scorer offers.
const (
	moveReduceStrength = "reduce_strength"
	moveReduceDistance = "reduce_distance"
)

// Cheapest-move labels for a volatility change. Offered only by the legacy
// calibration scorers (additive/multiplicative) — BookScorer never names
// volatility as a move (Ch11 sanctions strength and distance changes only).
const (
	// moveDeclareVolatility is surfaced for an undeclared target: the honest move
	// is to declare the module's subdomain/volatility so the tool can assess it
	// (and, per Khononov, a stable/low-volatility target neutralises the coupling).
	moveDeclareVolatility = "declare_volatility"
	// moveLowerVolatility is surfaced for a declared target whose volatility can be
	// reduced toward a more stable level.
	moveLowerVolatility = "lower_volatility"
)

// volatilityMoveLabel returns the cheapest-move label for a volatility change.
// An undeclared target yields moveDeclareVolatility — NOT moveLowerVolatility,
// which would imply "lower" a value the user never set.
func volatilityMoveLabel(v Volatility) string {
	if v == VolatilityUndeclared {
		return moveDeclareVolatility
	}
	return moveLowerVolatility
}

// bandRank returns the numeric rank of a Severity band (higher = worse).
func bandRank(s Severity) int {
	switch s {
	case SeverityNone:
		return 0
	case SeverityLow:
		return 1
	case SeverityMedium:
		return 2
	case SeverityHigh:
		return 3
	case SeverityCritical:
		return 4
	default:
		return 0
	}
}
