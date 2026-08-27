package scoring

import (
	"github.com/alexei-led/archfit/internal/relationship/coupling"
)

// AdditiveScorer implements Scorer using additive integer math:
//
//	raw = strength_val + distance_val − vol_discount, clamped to [0, 10].
//
// CheapestMove is the single adjacent dimension change that drops the band
// the most. Tie-break: prefer strength reduction over distance reduction
// over volatility change.
//
// Kept for calibration reference; DefaultScorer() now returns BookScorer.
type AdditiveScorer struct{}

// reasonAdditive labels EdgeScores produced by the additive scorer.
const reasonAdditive = "additive"

// Score computes the additive BC score for c.
func (AdditiveScorer) Score(c coupling.Classification) coupling.EdgeScore {
	// Same-module edges are not cross-boundary coupling and carry no BC risk
	// (classify.Run filters them; guard the scorer so it is correct in isolation).
	if c.Distance == coupling.DistanceSameModule {
		return coupling.EdgeScore{Value: 0, Band: ScoreBand(0), Reason: reasonAdditive}
	}
	sv := strengthOrdinal[c.Strength]
	dv := distanceOrdinal[c.Distance]
	vd := volatilityDiscount[c.Volatility]

	raw := clamp(sv+dv-vd, 0, 10)
	band := legacyScoreBand(raw)

	return coupling.EdgeScore{
		Value:  raw,
		Band:   band,
		Reason: reasonAdditive,
		Breakdown: coupling.ScoreBreakdown{
			StrengthVal: sv,
			DistanceVal: dv,
			VolDiscount: vd,
		},
		CheapestMove: additiveCheapestMove(c, band),
	}
}

// additiveCheapestMove returns the single dimension change that drops the
// band the most. Returns "" when already at none or no move improves the band.
// Tie-break: strength reduction > distance reduction > volatility change.
func additiveCheapestMove(c coupling.Classification, currentBand coupling.Severity) string {
	if currentBand == coupling.SeverityNone {
		return ""
	}

	type candidate struct {
		label string
		band  coupling.Severity
	}

	var best candidate
	bestDrop := 0

	tryMove := func(label string, modified coupling.Classification) {
		s := AdditiveScorer{}.Score(modified)
		drop := bandRank(currentBand) - bandRank(s.Band)
		if drop > bestDrop {
			bestDrop = drop
			best = candidate{label: label, band: s.Band}
		}
	}

	// Try reducing strength by one level.
	if next, ok := lowerStrength(c.Strength); ok {
		mod := c
		mod.Strength = next
		tryMove(moveReduceStrength, mod)
	}

	// Try reducing distance by one level.
	if next, ok := lowerDistance(c.Distance); ok {
		mod := c
		mod.Distance = next
		tryMove(moveReduceDistance, mod)
	}

	// Try increasing volatility discount (i.e. lower volatility = more stable).
	// An undeclared target is surfaced as "declare_volatility" instead.
	if next, ok := lowerVolatility(c.Volatility); ok {
		mod := c
		mod.Volatility = next
		tryMove(volatilityMoveLabel(c.Volatility), mod)
	}

	return best.label
}

// lowerStrength returns the next-lower coupling.Strength ordinal.
func lowerStrength(s coupling.Strength) (coupling.Strength, bool) {
	switch s {
	case coupling.StrengthIntrusive:
		return coupling.StrengthSymmetric, true
	case coupling.StrengthSymmetric:
		return coupling.StrengthFunctional, true
	case coupling.StrengthFunctional:
		return coupling.StrengthUnknown, true
	case coupling.StrengthUnknown:
		return coupling.StrengthModel, true
	case coupling.StrengthModel:
		return coupling.StrengthContract, true
	default:
		return s, false
	}
}

// lowerDistance returns the next-lower coupling.Distance ordinal.
func lowerDistance(d coupling.Distance) (coupling.Distance, bool) {
	switch d {
	case coupling.DistanceExternal:
		return coupling.DistanceCrossDeployUnit, true // bring the seam in-house
	case coupling.DistanceCrossDeployUnit:
		return coupling.DistanceCrossModuleDiffOwner, true
	case coupling.DistanceCrossModuleDiffOwner:
		return coupling.DistanceUnknown, true
	case coupling.DistanceUnknown:
		return coupling.DistanceCrossModuleSameOwner, true
	case coupling.DistanceCrossModuleSameOwner:
		return coupling.DistanceSameModule, true
	default:
		return d, false
	}
}

// lowerVolatility returns the next-lower coupling.Volatility (higher discount = more stable target).
// Undeclared and Unknown both map to Low: declaring (or resolving) a stable target
// is the change that drops the score — the move's label distinguishes the two
// (see volatilityMoveLabel), the modelled target does not.
func lowerVolatility(v coupling.Volatility) (coupling.Volatility, bool) {
	switch v {
	case coupling.VolatilityHigh:
		return coupling.VolatilityMedium, true
	case coupling.VolatilityMedium:
		return coupling.VolatilityLow, true
	case coupling.VolatilityUndeclared, coupling.VolatilityUnknown:
		return coupling.VolatilityLow, true
	default:
		return v, false
	}
}
