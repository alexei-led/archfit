package coupling

// Scorer computes a numeric risk score for a classified coupling edge.
// Implementations are deterministic and pure — no I/O, no global state.
// The score augments, never replaces, the existing BalanceResult severity bands.
type Scorer interface {
	Score(c Classification) EdgeScore
}

// ScoreDefinition is the canonical, user-facing definition of archfit's numeric
// BC score. It is deliberately explicit that the formula is archfit's OWN
// deterministic implementation of Vlad Khononov's qualitative Balanced Coupling
// heuristic — Khononov publishes no literal equation. coupling.dev frames a
// single qualitative "formula": maintenance effort rises monotonically with
// integration strength, distance, and volatility. archfit operationalises that
// as Effort ∝ Strength × Distance × Volatility over frozen ordinals.
const ScoreDefinition = "maintenance-effort proxy — Effort ∝ Strength × Distance × Volatility; " +
	"archfit's deterministic implementation of Vlad Khononov's qualitative Balanced Coupling " +
	"heuristic (low volatility neutralises; cohesion = high strength + low distance is healthy, " +
	"not flagged), NOT a literal published equation"

// ScoreVersion versions the BC score: its ordinals, normalisation, and the
// severity mapping derived from them. Bump it when any of those change so the
// delta baseline treats the score as a new measurement. Held at v2: the
// Balanced Coupling measurement engine v2 (2026-06-18) plus the volatility
// undeclared/unknown split and balance-rule attribution (Task 10).
const ScoreVersion = "bc_score.v2"

// EdgeScore is the result produced by a Scorer for one graph edge.
// Value is the final integer score in [0, 10].
// Band is the severity band derived from Value.
// Reason is a short human-readable label identifying the scorer variant.
// Breakdown holds the per-dimension contribution for diagnostics.
// CheapestMove is the single dimension change that drops the band the most;
// empty when already at band none or when no move exists.
type EdgeScore struct {
	Value        int
	Band         Severity
	Reason       string
	Breakdown    ScoreBreakdown
	CheapestMove string
}

// ScoreBreakdown records the per-dimension ordinal values used in scoring.
// All fields are the raw ordinal integers before final combination.
type ScoreBreakdown struct {
	StrengthVal int
	DistanceVal int
	VolDiscount int
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

// ScoreBand maps a numeric score in [0, 10] to a Severity band.
// Bands: 0–2 none · 3–4 low · 5–6 medium · 7–8 high · 9–10 critical.
func ScoreBand(score int) Severity {
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
// does not specify one. Locked to MultiplicativeScorer per Task 16 calibration
// decision (2026-06-18): 150 edges on archfit, 105 agree, rate=0.70 vs AdditiveScorer.
func DefaultScorer() Scorer {
	return MultiplicativeScorer{}
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
