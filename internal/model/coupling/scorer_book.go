package coupling

// BookScorer implements Vlad Khononov's published formula from
// _Balancing Coupling in Software Design_ Ch10.
//
// balance = max(|S-D|, 10-V) + 1, range 1..10, higher = better balanced.
//
// When strength or distance is unknown, the edge is abstained (Scored=false).
// Same-module edges score at the same-module rung (D=2): high strength close
// together is cohesion (high balance), low strength close together is the
// book's Ch10 local-complexity quadrant (low balance — a ball-of-mud signal).
// classify.Run keeps same-module scores out of the advisory pipeline and
// coupling_balance; they surface only in the local_coupling report block
// (cross-module coupling and intra-module cohesion are different fractal levels).
// Undeclared/unknown volatility is treated conservatively as V=10 (worst case).
type BookScorer struct{}

// Book ordinals — verbatim from Khononov Ch8–Ch10.
// Changing these values is a BREAKING metric change; bump ScoreVersion.

// Strength ordinals (Ch10): lower = weaker/safer coupling.
const (
	bookStrengthContract   = 1
	bookStrengthModel      = 3
	bookStrengthFunctional = 8
	bookStrengthSymmetric  = 9
	bookStrengthIntrusive  = 10
)

// Distance ordinals (Ch8): lower = closer/safer.
// bookDistanceExternal is the ladder's far end (book Ch10 Example 1,
// cross-vendor integration) — reserved for config-declared external systems.
const (
	bookDistanceSameModule           = 2
	bookDistanceCrossModuleSameOwner = 4
	bookDistanceCrossModuleDiffOwner = 7
	bookDistanceCrossDeployUnit      = 9
	bookDistanceExternal             = 10
)

// Volatility ordinals (Ch9): lower = more stable = safer.
// Undeclared and unknown are conservative worst-case (10).
const (
	bookVolatilityLow        = 3  // low / supporting / generic
	bookVolatilityMedium     = 6  // medium
	bookVolatilityHigh       = 10 // high / core
	bookVolatilityUndeclared = 10 // cannot confirm stability → worst case
	bookVolatilityUnknown    = 10 // cannot confirm stability → worst case
)

// bookStrengthOrdinal maps Strength to its book Ch10 ordinal.
// StrengthUnknown is absent — unknown strength causes abstention.
var bookStrengthOrdinal = map[Strength]int{
	StrengthContract:   bookStrengthContract,
	StrengthModel:      bookStrengthModel,
	StrengthFunctional: bookStrengthFunctional,
	StrengthSymmetric:  bookStrengthSymmetric,
	StrengthIntrusive:  bookStrengthIntrusive,
}

// bookDistanceOrdinal maps Distance to its book Ch8 ordinal.
// DistanceUnknown is absent — unknown distance causes abstention.
var bookDistanceOrdinal = map[Distance]int{
	DistanceSameModule:           bookDistanceSameModule,
	DistanceCrossModuleSameOwner: bookDistanceCrossModuleSameOwner,
	DistanceCrossModuleDiffOwner: bookDistanceCrossModuleDiffOwner,
	DistanceCrossDeployUnit:      bookDistanceCrossDeployUnit,
	DistanceExternal:             bookDistanceExternal,
}

// bookVolatilityFrozen is V=1 for frozen/legacy systems (most stable).
const bookVolatilityFrozen = 1

// bookVolatilityOrdinal maps Volatility to its book Ch9 ordinal.
var bookVolatilityOrdinal = map[Volatility]int{
	VolatilityFrozen:     bookVolatilityFrozen,
	VolatilityLow:        bookVolatilityLow,
	VolatilityMedium:     bookVolatilityMedium,
	VolatilityHigh:       bookVolatilityHigh,
	VolatilityUndeclared: bookVolatilityUndeclared,
	VolatilityUnknown:    bookVolatilityUnknown,
}

const reasonBook = "book"

// LocalComplexity reports whether cl sits in the book Ch10 local-complexity
// quadrant: low integration strength (contract/model — the low half of the
// strength ladder) at same-module distance. Components that share little
// meaning but live together — low cohesion, the "big ball of mud" corner.
// Consumed by the local_coupling report block only; never by advisories,
// coupling_balance, or the gate.
func LocalComplexity(cl Classification) bool {
	return cl.Distance == DistanceSameModule &&
		(cl.Strength == StrengthContract || cl.Strength == StrengthModel)
}

// Score computes the book balance score for c.
func (BookScorer) Score(c Classification) EdgeScore {
	// Abstain when strength or distance is unknown — no book ordinal exists.
	s, sOK := bookStrengthOrdinal[c.Strength]
	d, dOK := bookDistanceOrdinal[c.Distance]
	if !sOK || !dOK {
		return EdgeScore{Scored: false, Reason: reasonBook}
	}

	// Volatility: undeclared/unknown → worst-case 10 (conservative).
	v, vOK := bookVolatilityOrdinal[c.Volatility]
	if !vOK {
		v = bookVolatilityUnknown
	}

	// Book Ch10 formula.
	modularity := abs(s - d)
	volRescue := 10 - v
	balance := max2(modularity, volRescue) + 1
	balance = clamp(balance, 1, 10)

	band := ScoreBand(balance)

	return EdgeScore{
		Scored:  true,
		Balance: balance,
		Value:   balance,
		Band:    band,
		Reason:  reasonBook,
		Breakdown: ScoreBreakdown{
			StrengthVal:   s,
			DistanceVal:   d,
			VolatilityVal: v,
			Modularity:    modularity,
		},
		CheapestMove: bookCheapestMove(c, band),
	}
}

// bookCheapestMove returns the single dimension change that raises balance the
// most (i.e. drops the severity band the most). Tie-break: strength > distance > volatility.
func bookCheapestMove(c Classification, currentBand Severity) string {
	if currentBand == SeverityNone {
		return ""
	}

	bestDrop := 0
	bestLabel := ""

	tryMove := func(label string, modified Classification) {
		got := BookScorer{}.Score(modified)
		if !got.Scored {
			return
		}
		drop := bandRank(currentBand) - bandRank(got.Band)
		if drop > bestDrop {
			bestDrop = drop
			bestLabel = label
		}
	}

	if next, ok := bookLowerStrength(c.Strength); ok {
		mod := c
		mod.Strength = next
		tryMove("reduce_strength", mod)
	}
	if next, ok := bookLowerDistance(c.Distance); ok {
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

// bookLowerStrength is like lowerStrength but skips StrengthUnknown.
// StrengthUnknown causes BookScorer to abstain, so tryMove would silently drop
// the suggestion; this ladder jumps directly from Functional to Model.
func bookLowerStrength(s Strength) (Strength, bool) {
	switch s {
	case StrengthIntrusive:
		return StrengthSymmetric, true
	case StrengthSymmetric:
		return StrengthFunctional, true
	case StrengthFunctional:
		return StrengthModel, true // skip StrengthUnknown
	case StrengthModel:
		return StrengthContract, true
	default:
		return s, false
	}
}

// bookLowerDistance is like lowerDistance but skips DistanceUnknown.
// DistanceUnknown causes BookScorer to abstain, so tryMove would silently drop
// the suggestion; this ladder jumps directly from CrossModuleDiffOwner to CrossModuleSameOwner.
// DistanceCrossModuleSameOwner is the terminal rung: the next step down is
// DistanceSameModule, which is not a distance reduction but a module merge — a
// design change that moves the edge out of cross-module coupling entirely (its
// score would report in local_coupling, not coupling_balance), so it is not
// offered as a "reduce_distance" remediation.
func bookLowerDistance(d Distance) (Distance, bool) {
	switch d {
	case DistanceExternal:
		return DistanceCrossDeployUnit, true // bring the seam in-house
	case DistanceCrossDeployUnit:
		return DistanceCrossModuleDiffOwner, true
	case DistanceCrossModuleDiffOwner:
		return DistanceCrossModuleSameOwner, true // skip DistanceUnknown
	// DistanceCrossModuleSameOwner is terminal — further reduction collapses to cohesion
	default:
		return d, false
	}
}

// abs returns the absolute value of x.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// max2 returns the larger of a and b.
func max2(a, b int) int {
	if a > b {
		return a
	}
	return b
}
