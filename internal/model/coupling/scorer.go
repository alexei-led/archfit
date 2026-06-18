package coupling

// Scorer computes a numeric risk score for a classified coupling edge.
// Implementations are deterministic and pure — no I/O, no global state.
// The score augments, never replaces, the existing BalanceResult severity bands.
type Scorer interface {
	Score(c Classification) EdgeScore
}

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
// Frozen ordinal tables (BC measurement engine v2 spec §4.1).
// These are the canonical, non-negotiable mappings. Changing them is a
// breaking change to the metric — bump the scorer version instead.
// ---------------------------------------------------------------------------

// strengthOrdinal maps Strength to its risk ordinal.
// contract=0: declared interface, lowest coupling risk.
// model=2: concrete type import, low risk.
// unknown=3: unresolved; default to moderate risk.
// functional=5: implementation-level call.
// intrusive=8: internal/private access, highest coupling intensity.
var strengthOrdinal = map[Strength]int{
	StrengthContract:   0,
	StrengthModel:      2,
	StrengthUnknown:    3,
	StrengthFunctional: 5,
	StrengthIntrusive:  8,
}

// distanceOrdinal maps Distance to its risk ordinal.
// same_module=0: co-located, no boundary crossed.
// cross_module_same_owner=1: nearby boundary, shared accountability.
// unknown=2: unresolved; default to cross-module risk.
// cross_module_diff_owner=3: separate accountability boundaries.
// cross_deploy_unit=5: highest deployment boundary.
var distanceOrdinal = map[Distance]int{
	DistanceSameModule:           0,
	DistanceCrossModuleSameOwner: 1,
	DistanceUnknown:              2,
	DistanceCrossModuleDiffOwner: 3,
	DistanceCrossDeployUnit:      5,
}

// volatilityDiscount maps Volatility to a discount subtracted from the raw score.
// Low volatility = higher discount (stable target reduces coupling risk).
// Unknown = no discount (conservative; treat as potentially volatile).
var volatilityDiscount = map[Volatility]int{
	VolatilityLow:     2,
	VolatilityMedium:  1,
	VolatilityHigh:    0,
	VolatilityUnknown: 0,
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

// LegacyShim is a Scorer that wraps BalanceResult to reproduce the post-Task-1
// severity bands, keeping golden output stable until calibration (Task 16).
// Value is set to the mid-point of each band so downstream consumers can
// treat it as a coarse integer; Reason is "legacy"; CheapestMove is empty.
type LegacyShim struct{}

// bandMidpoint maps a Severity band to the mid-point integer score in that band.
// none→0, low→3, medium→5, high→7, critical→9.
var bandMidpoint = map[Severity]int{
	SeverityNone:     0,
	SeverityLow:      3,
	SeverityMedium:   5,
	SeverityHigh:     7,
	SeverityCritical: 9,
}

// Score implements Scorer using the legacy BalanceResult formula.
func (LegacyShim) Score(c Classification) EdgeScore {
	sev := BalanceResult(c)
	val := bandMidpoint[sev]
	return EdgeScore{
		Value:  val,
		Band:   sev,
		Reason: "legacy",
		Breakdown: ScoreBreakdown{
			StrengthVal: strengthOrdinal[c.Strength],
			DistanceVal: distanceOrdinal[c.Distance],
			VolDiscount: volatilityDiscount[c.Volatility],
		},
		CheapestMove: "",
	}
}

// DefaultScorer returns the scorer used by classify.Run when the config
// does not specify one. Returns LegacyShim to preserve golden-output stability
// until calibration (Task 16) selects the production scorer.
func DefaultScorer() Scorer {
	return LegacyShim{}
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
