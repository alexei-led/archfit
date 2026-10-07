package scoring

import (
	"github.com/alexei-led/archfit/internal/relationship/coupling"
)

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

// coupling.Strength ordinals (Ch10): lower = weaker/safer coupling.
const (
	bookStrengthContract   = 1
	bookStrengthModel      = 3
	bookStrengthFunctional = 8
	bookStrengthSymmetric  = 9
	bookStrengthIntrusive  = 10
)

// coupling.Distance ordinals (Ch8): lower = closer/safer.
// bookDistanceExternal is the ladder's far end (book Ch10 Example 1,
// cross-vendor integration) — reserved for config-declared external systems.
//
// Distance is level-relative: any module boundary is the far end of the
// in-house ladder (Ch10 puts in-house services at 9 and vendors at 10), so
// owner and deploy unit name the boundary without moving the rung.
const (
	bookDistanceSameModule = 2
	bookDistanceBoundary   = 9
	bookDistanceExternal   = 10
)

// coupling.Volatility ordinals (Ch9): lower = more stable = safer.
// Undeclared and unknown are conservative worst-case (10).
const (
	bookVolatilityLow        = 3  // low / supporting / generic
	bookVolatilityMedium     = 6  // medium
	bookVolatilityHigh       = 10 // high / core
	bookVolatilityUndeclared = 10 // cannot confirm stability → worst case
	bookVolatilityUnknown    = 10 // cannot confirm stability → worst case
)

// bookStrengthOrdinal maps coupling.Strength to its book Ch10 ordinal.
// coupling.StrengthUnknown is absent — unknown strength causes abstention.
var bookStrengthOrdinal = map[coupling.Strength]int{
	coupling.StrengthContract:   bookStrengthContract,
	coupling.StrengthModel:      bookStrengthModel,
	coupling.StrengthFunctional: bookStrengthFunctional,
	coupling.StrengthSymmetric:  bookStrengthSymmetric,
	coupling.StrengthIntrusive:  bookStrengthIntrusive,
}

// bookDistanceOrdinal maps coupling.Distance to its book Ch8 ordinal.
// coupling.DistanceUnknown is absent — unknown distance causes abstention.
var bookDistanceOrdinal = map[coupling.Distance]int{
	coupling.DistanceSameModule:           bookDistanceSameModule,
	coupling.DistanceCrossModule:          bookDistanceBoundary,
	coupling.DistanceCrossModuleDiffOwner: bookDistanceBoundary,
	coupling.DistanceCrossDeployUnit:      bookDistanceBoundary,
	coupling.DistanceExternal:             bookDistanceExternal,
}

// bookVolatilityFrozen is V=1 for frozen/legacy systems (most stable).
const bookVolatilityFrozen = 1

// bookVolatilityOrdinal maps coupling.Volatility to its book Ch9 ordinal.
var bookVolatilityOrdinal = map[coupling.Volatility]int{
	coupling.VolatilityFrozen:     bookVolatilityFrozen,
	coupling.VolatilityLow:        bookVolatilityLow,
	coupling.VolatilityMedium:     bookVolatilityMedium,
	coupling.VolatilityHigh:       bookVolatilityHigh,
	coupling.VolatilityUndeclared: bookVolatilityUndeclared,
	coupling.VolatilityUnknown:    bookVolatilityUnknown,
}

const reasonBook = "book"

// LocalComplexity reports whether cl sits in the book Ch10 local-complexity
// quadrant: low integration strength (contract/model — the low half of the
// strength ladder) at same-module distance. Components that share little
// meaning but live together — low cohesion, the "big ball of mud" corner.
// Consumed by the local_coupling report block only; never by advisories,
// coupling_balance, or the gate.
func LocalComplexity(cl coupling.Classification) bool {
	return cl.Distance == coupling.DistanceSameModule &&
		(cl.Strength == coupling.StrengthContract || cl.Strength == coupling.StrengthModel)
}

// Score computes the book balance score for c.
func (BookScorer) Score(c coupling.Classification) coupling.EdgeScore {
	// Abstain when strength or distance is unknown — no book ordinal exists.
	s, sOK := bookStrengthOrdinal[c.Strength]
	d, dOK := bookDistanceOrdinal[c.Distance]
	if !sOK || !dOK {
		return coupling.EdgeScore{Scored: false, Reason: reasonBook}
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

	return coupling.EdgeScore{
		Scored:  true,
		Balance: balance,
		Value:   balance,
		Band:    band,
		Reason:  reasonBook,
		Breakdown: coupling.ScoreBreakdown{
			StrengthVal:   s,
			DistanceVal:   d,
			VolatilityVal: v,
			Modularity:    modularity,
		},
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
