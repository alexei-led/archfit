package relationship

// HypothesisInput is the driving fact of a seam, or the single edge behind an
// advisory. One function turns it into guidance, so the seam ledger, the
// advisories and the agent-task constraints cannot disagree.
type HypothesisInput struct {
	Strength   Strength
	Volatility Volatility
	// Band is the driving fact's severity band.
	Band Severity
	// Clone is true when the driving fact is a clone fact, which is symmetric
	// coupling with no import edge behind it.
	Clone bool
	// TargetPublic is true when the target module declares a public surface.
	TargetPublic bool
	// CohesiveRole is true when the source module is a composition root, or its
	// sources are generated or test code. A clone fact has no source side, so the
	// role never applies to it.
	CohesiveRole bool
}

// BalancingHypothesis returns the move that clears a flagged seam, or the reason
// none is needed. Every flagged seam (band medium or worse) gets a move that
// re-scores to low or better, or a non-move hypothesis; abstained facts get none.
func BalancingHypothesis(in HypothesisInput) SeamHypothesis {
	strong := in.Clone || isStrong(in.Strength)
	switch in.Band {
	case SeverityNone, SeverityLow:
		if strong {
			return SeamHypothesisAcceptLowVolatility
		}
		if in.Strength == StrengthUnknown {
			return ""
		}
		return SeamHypothesisBalanced
	}
	switch {
	case in.CohesiveRole && !in.Clone && in.Strength != StrengthIntrusive:
		return SeamHypothesisExpectedByRole
	case in.Volatility == VolatilityUndeclared || in.Volatility == VolatilityUnknown:
		return SeamHypothesisDeclareVolatility
	case in.Strength == StrengthIntrusive:
		return SeamHypothesisIntroduceContract
	case in.Clone || in.Strength == StrengthSymmetric:
		return SeamHypothesisMoveFunctionality
	case in.Strength == StrengthFunctional || in.Strength == StrengthModel:
		if in.TargetPublic {
			return SeamHypothesisIntroduceContract
		}
		return SeamHypothesisMoveFunctionality
	default:
		return ""
	}
}

func isStrong(s Strength) bool {
	return s == StrengthFunctional || s == StrengthSymmetric || s == StrengthIntrusive
}
