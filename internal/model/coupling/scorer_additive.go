package coupling

// AdditiveScorer implements Scorer using additive integer math:
//
//	raw = strength_val + distance_val − vol_discount, clamped to [0, 10].
//
// CheapestMove is the single adjacent dimension change that drops the band
// the most. Tie-break: prefer strength reduction over distance reduction
// over volatility change.
type AdditiveScorer struct{}

// Score computes the additive BC score for c.
func (AdditiveScorer) Score(c Classification) EdgeScore {
	sv := strengthOrdinal[c.Strength]
	dv := distanceOrdinal[c.Distance]
	vd := volatilityDiscount[c.Volatility]

	raw := clamp(sv+dv-vd, 0, 10)
	band := ScoreBand(raw)

	return EdgeScore{
		Value:  raw,
		Band:   band,
		Reason: "additive",
		Breakdown: ScoreBreakdown{
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
func additiveCheapestMove(c Classification, currentBand Severity) string {
	if currentBand == SeverityNone {
		return ""
	}

	type candidate struct {
		label string
		band  Severity
	}

	var best candidate
	bestDrop := 0

	tryMove := func(label string, modified Classification) {
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
		tryMove("reduce_strength", mod)
	}

	// Try reducing distance by one level.
	if next, ok := lowerDistance(c.Distance); ok {
		mod := c
		mod.Distance = next
		tryMove("reduce_distance", mod)
	}

	// Try increasing volatility discount (i.e. lower volatility = more stable).
	if next, ok := lowerVolatility(c.Volatility); ok {
		mod := c
		mod.Volatility = next
		tryMove("lower_volatility", mod)
	}

	return best.label
}

// lowerStrength returns the next-lower Strength ordinal.
func lowerStrength(s Strength) (Strength, bool) {
	switch s {
	case StrengthIntrusive:
		return StrengthFunctional, true
	case StrengthFunctional:
		return StrengthUnknown, true
	case StrengthUnknown:
		return StrengthModel, true
	case StrengthModel:
		return StrengthContract, true
	default:
		return s, false
	}
}

// lowerDistance returns the next-lower Distance ordinal.
func lowerDistance(d Distance) (Distance, bool) {
	switch d {
	case DistanceCrossDeployUnit:
		return DistanceCrossModuleDiffOwner, true
	case DistanceCrossModuleDiffOwner:
		return DistanceUnknown, true
	case DistanceUnknown:
		return DistanceCrossModuleSameOwner, true
	case DistanceCrossModuleSameOwner:
		return DistanceSameModule, true
	default:
		return d, false
	}
}

// lowerVolatility returns the next-lower Volatility (higher discount = more stable target).
func lowerVolatility(v Volatility) (Volatility, bool) {
	switch v {
	case VolatilityHigh:
		return VolatilityMedium, true
	case VolatilityMedium:
		return VolatilityLow, true
	case VolatilityUnknown:
		return VolatilityLow, true
	default:
		return v, false
	}
}
