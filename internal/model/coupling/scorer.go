package coupling

// Scorer computes a numeric risk score for a classified coupling edge.
// Implementations are deterministic and pure — no I/O, no global state.
// Score() is the single source of severity via ScoreBand(Balance) → cl.Score.Band.
type Scorer interface {
	Score(c Classification) EdgeScore
}

// ScoreDefinition is the canonical, user-facing definition of archfit's numeric
// BC score. Implements Vlad Khononov's published formula from _Balancing Coupling
// in Software Design_ Ch10 verbatim: balance = max(|S−D|, 10−V) + 1.
// Range 1 (distributed monolith / ball-of-mud) to 10 (frozen/contract); higher
// is better balanced. Strength, distance, and volatility ordinals are fixed per
// the book (Ch8–Ch10); changing any is a breaking metric change (bump ScoreVersion).
const ScoreDefinition = "book balance score — balance = max(|S−D|, 10−V) + 1 " +
	"(Khononov, _Balancing Coupling in Software Design_, Ch10); " +
	"range 1 (distributed monolith) to 10 (frozen/contract); higher = better balanced"

// ScoreVersion versions the BC score: its ordinals, normalisation, and the
// severity mapping derived from them. Bump it when any of those change so the
// delta baseline treats the score as a new measurement.
// v3: book-verbatim formula (Khononov Ch10) — balance = max(|S-D|, 10-V)+1,
// new ScoreBand mapping (1-2 critical → 9-10 none), BookScorer as default.
// v4: three classification fixes feeding the unchanged formula (see
// docs/design/20260702-bc-score-v4.md): Go const/var reads score Model (3)
// not Functional (8); pure-data DTOs across a declared public boundary reach
// Contract (1); declared external systems enter scoring at DistanceExternal
// (10). Scores are NOT comparable across versions.
const ScoreVersion = "bc_score.v4"

// EdgeScore is the result produced by a Scorer for one graph edge.
//
// Scored is false when the scorer abstained (strength or distance unknown).
// Balance is the book balance value 1..10 (higher = better balanced); 0 when !Scored.
// Value equals Balance for BookScorer; legacy scorers set it to their 0..10 risk integer.
// Band is the severity band derived from Balance (or Value for legacy scorers).
// Reason is a short human-readable label identifying the scorer variant.
// Breakdown holds the per-dimension contribution for diagnostics.
// CheapestMove is the single dimension change that drops the band the most;
// empty when already at band none or when no move exists.
type EdgeScore struct {
	Scored       bool // false when strength or distance is unknown → edge abstained
	Balance      int  // book balance 1..10 (0 when !Scored)
	Value        int
	Band         Severity
	Reason       string
	Breakdown    ScoreBreakdown
	CheapestMove string
}

// ScoreBreakdown records the per-dimension ordinal values used in scoring.
// All fields are the raw ordinal integers before final combination.
type ScoreBreakdown struct {
	StrengthVal   int // S ordinal (book Ch10)
	DistanceVal   int // D ordinal (book Ch8)
	VolatilityVal int // V ordinal (book Ch9); 0 for legacy scorers
	Modularity    int // |S-D|; 0 for legacy scorers
	VolDiscount   int // legacy additive/multiplicative only; 0 for BookScorer
}

// ---------------------------------------------------------------------------
// Frozen ordinal constants — Balanced Coupling (Khononov) §4.1.
//
// These ordinals encode Vlad's balance rule, NOT a literal equation he publishes
// (see ScoreDefinition). They preserve his orderings with extra numeric spread:
//   - Strength {Contract < Model < Functional < Intrusive} — knowledge shared,
//     weakest→strongest (Khononov's integration-strength ladder).
//   - Distance {same_module < cross_module_same_owner < cross_module_diff_owner
//     < cross_deploy_unit} — socio-technical separation, local→external.
//   - Volatility discount {high 0, medium 1, low 2} — low volatility neutralises
//     otherwise-unbalanced coupling; undeclared/unknown get no discount
//     (conservative: treat as potentially volatile).
// Changing any of these values is a BREAKING metric change; bump ScoreVersion.
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
// does not specify one. Changed to BookScorer (bc_score.v3): implements
// Vlad Khononov's published formula from _Balancing Coupling in Software Design_ Ch10.
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

// Cheapest-move labels for a volatility change.
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
