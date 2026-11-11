package balance

import (
	"github.com/alexei-led/archfit/v3/internal/relationship/coupling"
	"github.com/alexei-led/archfit/v3/internal/relationship/scoring"
)

// Of returns the balance of module source reaching module target with
// strength s, from their declared volatilities. A use of the public part is
// contract strength, a use past it functional, a copy symmetric. ok is false
// when the scorer abstains (unknown strength): a missing balance is not 0.
func Of(s coupling.Strength, source, target coupling.Volatility) (balance int, ok bool) {
	score := scoreAt(s, target)
	switch s {
	case coupling.StrengthContract, coupling.StrengthModel:
	default:
		// The scorer's own ordinals pick the higher volatility; none are copied here.
		if fromSource := scoreAt(s, source); fromSource.Breakdown.VolatilityVal > score.Breakdown.VolatilityVal {
			score = fromSource
		}
	}
	return score.Balance, score.Scored
}

func scoreAt(s coupling.Strength, v coupling.Volatility) coupling.EdgeScore {
	return scoring.BookScorer{}.Score(coupling.Classification{
		Strength:   s,
		Distance:   coupling.DistanceCrossModule,
		Volatility: v,
	})
}
