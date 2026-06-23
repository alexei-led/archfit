package coupling

import "math"

// MultiplicativeScorer implements archfit's numeric BC score: a deterministic
// implementation of Vlad Khononov's qualitative maintenance-effort heuristic
// (Effort ∝ Strength × Distance × Volatility), NOT a literal equation he
// publishes — see ScoreDefinition. It encodes the balance rule: low volatility
// neutralises (V_norm down-weights), and cohesion (high strength + low distance)
// scores low because the continuous-XOR R_mod term is small when one dimension
// is high and the other low.
//
// Formula:
//
//	S_norm = strength_val / 8.0
//	D_norm = distance_val / 5.0
//	V_norm = {low:0.2, med:0.6, high:1.0, unk:0.5}
//	R_mod  = 1 − |S_norm − D_norm|   (continuous XOR: modular quadrants score low)
//	R_edge = R_mod × V_norm
//	score  = round(10 × R_edge), clamped [0, 10]
//
// Intrusive floor: intrusive strength always scores at least 3 (band low),
// regardless of V — reconciles BC's "intrusive always fragile" with the gate.
//
// Kept for calibration reference; DefaultScorer() now returns BookScorer.
type MultiplicativeScorer struct{}

// Frozen normalisation constants — Balanced Coupling (Khononov) §4.1 / BC-pure formula.
// Changing any of these values is a BREAKING metric change; bump *.v2 instead.
const (
	volatilityNormLow        = 0.2
	volatilityNormMedium     = 0.6
	volatilityNormHigh       = 1.0
	volatilityNormUndeclared = 0.5
	volatilityNormUnknown    = 0.5
)

// maxStrengthOrdinal is the normalisation denominator for strength (intrusive=8).
const maxStrengthOrdinal = 8.0

// maxDistanceOrdinal is the normalisation denominator for distance (cross_deploy=5).
const maxDistanceOrdinal = 5.0

// intrusiveFloor is the minimum score for any intrusive-strength edge (band low).
const intrusiveFloor = 3

// volatilityNorm maps Volatility to its [0,1] weight for the BC-pure formula
// (values frozen — see consts above).
// Low volatility suppresses the coupling risk (stable target), high amplifies it.
var volatilityNorm = map[Volatility]float64{
	VolatilityLow:        volatilityNormLow,
	VolatilityMedium:     volatilityNormMedium,
	VolatilityHigh:       volatilityNormHigh,
	VolatilityUndeclared: volatilityNormUndeclared,
	VolatilityUnknown:    volatilityNormUnknown,
}

// reasonMultiplicative labels EdgeScores produced by the multiplicative scorer.
const reasonMultiplicative = "multiplicative"

// Score computes the multiplicative BC score for c.
func (MultiplicativeScorer) Score(c Classification) EdgeScore {
	// Same-module edges are not cross-boundary coupling and carry no BC risk.
	// classify.Run already filters them, but guard the scorer so it is correct in
	// isolation: without this, (low-strength, same_module, high_vol) would score
	// critical via rMod = 1 - |0 - 0| = 1.0.
	if c.Distance == DistanceSameModule {
		return EdgeScore{Value: 0, Band: ScoreBand(0), Reason: reasonMultiplicative}
	}
	sv := strengthOrdinal[c.Strength]
	dv := distanceOrdinal[c.Distance]
	vd := volatilityDiscount[c.Volatility] // kept in Breakdown for consistency

	sNorm := float64(sv) / maxStrengthOrdinal
	dNorm := float64(dv) / maxDistanceOrdinal
	vNorm := volatilityNorm[c.Volatility]

	rMod := 1.0 - math.Abs(sNorm-dNorm)
	rEdge := rMod * vNorm
	raw := int(math.Round(10.0 * rEdge))
	raw = clamp(raw, 0, 10)

	// Intrusive floor: intrusive strength always triggers at least band low.
	if c.Strength == StrengthIntrusive && raw < intrusiveFloor {
		raw = intrusiveFloor
	}

	band := legacyScoreBand(raw)

	return EdgeScore{
		Value:  raw,
		Band:   band,
		Reason: reasonMultiplicative,
		Breakdown: ScoreBreakdown{
			StrengthVal: sv,
			DistanceVal: dv,
			VolDiscount: vd,
		},
		CheapestMove: multiplicativeCheapestMove(c, band),
	}
}

// multiplicativeCheapestMove returns the single dimension change that drops the
// band the most. Returns "" when already at none or no move improves the band.
// Tie-break: strength reduction > distance reduction > volatility change.
func multiplicativeCheapestMove(c Classification, currentBand Severity) string {
	if currentBand == SeverityNone {
		return ""
	}

	bestDrop := 0
	bestLabel := ""

	tryMove := func(label string, modified Classification) {
		s := MultiplicativeScorer{}.Score(modified)
		drop := bandRank(currentBand) - bandRank(s.Band)
		if drop > bestDrop {
			bestDrop = drop
			bestLabel = label
		}
	}

	if next, ok := lowerStrength(c.Strength); ok {
		mod := c
		mod.Strength = next
		tryMove("reduce_strength", mod)
	}
	if next, ok := lowerDistance(c.Distance); ok {
		mod := c
		mod.Distance = next
		tryMove("reduce_distance", mod)
	}
	if next, ok := lowerVolatility(c.Volatility); ok {
		mod := c
		mod.Volatility = next
		tryMove(volatilityMoveLabel(c.Volatility), mod)
	}

	return bestLabel
}
