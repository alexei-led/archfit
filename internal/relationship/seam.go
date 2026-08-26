package relationship

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"sort"
)

// seamIDDomain is the domain separator in the seam identity hash. It is part of
// the frozen seam contract: changing it re-keys every seam and makes every
// stored seam delta non-comparable.
const seamIDDomain = "seam.v1"

// SeamID is the stable identity of one logical ordered module pair:
// sha256("seam.v1\x00" + fromModule + "\x00" + toModule).
//
// It is deliberately derived from module names only. A module rename therefore
// produces a different seam, which is why a module-map hash mismatch makes a
// seam comparison non-comparable instead of reporting a rename as one resolved
// seam plus one new seam.
func SeamID(fromModule, toModule string) string {
	sum := sha256.Sum256([]byte(seamIDDomain + "\x00" + fromModule + "\x00" + toModule))
	return hex.EncodeToString(sum[:])
}

// SeamQuadrant names the book Ch10 strength/distance quadrant a seam sits in.
// It is a shape, not a score: cohesive and loose are both healthy, low_cohesion
// and tight are both worth explaining.
type SeamQuadrant string

// Seam quadrants. High strength close together is cohesion; low strength far
// apart is decoupling; the two mismatched corners are the interesting ones.
const (
	// SeamQuadrantCohesive is high strength at short distance — components that
	// share meaning and live near each other.
	SeamQuadrantCohesive SeamQuadrant = "cohesive"
	// SeamQuadrantLoose is low strength at long distance — a contract-shaped
	// dependency across a real boundary.
	SeamQuadrantLoose SeamQuadrant = "loose"
	// SeamQuadrantLowCohesion is low strength at short distance — neighbours
	// that share almost nothing, the book's local-complexity corner.
	SeamQuadrantLowCohesion SeamQuadrant = "low_cohesion"
	// SeamQuadrantTight is high strength at long distance — the global
	// complexity corner and the only quadrant a distributed monolith lives in.
	SeamQuadrantTight SeamQuadrant = "tight"
)

// SeamHypothesis is the single cheapest design move for a seam. It is a
// hypothesis, not an instruction: the tool measures the seam, a human decides
// whether the move is worth making.
type SeamHypothesis string

// Seam balancing hypotheses.
const (
	SeamHypothesisReduceStrength    SeamHypothesis = "reduce_strength"
	SeamHypothesisReduceDistance    SeamHypothesis = "reduce_distance"
	SeamHypothesisDeclareVolatility SeamHypothesis = "declare_volatility"
	SeamHypothesisLeaveAlone        SeamHypothesis = "leave_alone"
)

// SeamRoleExpectation is what the source module's declared role says this seam
// should look like. It is derived from the existing module `role:` field — there
// is no second role vocabulary and no strength label may stand in for one.
type SeamRoleExpectation string

// Seam role expectations. An empty value means the source module declared no
// role, so no expectation can be claimed.
const (
	SeamRoleCompositionRoot SeamRoleExpectation = "composition_root"
	SeamRoleAdapter         SeamRoleExpectation = "adapter"
	SeamRoleCore            SeamRoleExpectation = "core"
	SeamRoleSharedModel     SeamRoleExpectation = "shared_model"
)

// SeamConfidence is how much of the seam was actually measured. It never
// changes the seam's severity — it says how much to trust it.
type SeamConfidence string

// Seam confidence rungs.
const (
	SeamConfidenceHigh    SeamConfidence = "high"
	SeamConfidenceMedium  SeamConfidence = "medium"
	SeamConfidenceLow     SeamConfidence = "low"
	SeamConfidenceUnrated SeamConfidence = "unrated"
)

// SeamDistance is the raw distance evidence behind a seam's collapsed rung.
// The rung alone cannot be audited: cross_module_same_owner is produced both by
// two modules that genuinely share an owner and by a repository where ownership
// is degenerate. Reporting the facts beside the level makes the difference
// visible without changing the ordinal formula.
type SeamDistance struct {
	// Level is the coarse book rung the scorer consumed.
	Level Distance
	// Basis names the deterministic signal that selected Level.
	Basis string
	// FromOwner and ToOwner are the resolved owners, empty when undeclared.
	FromOwner string
	ToOwner   string
	// SameOwner is true only when both owners are declared and equal. Two
	// undeclared owners are not evidence of shared ownership.
	SameOwner bool
	// FromDeployUnit and ToDeployUnit are the resolved deploy units.
	FromDeployUnit string
	ToDeployUnit   string
	// SameDeployUnit is true only when both units are declared and equal.
	SameDeployUnit bool
	// BoundaryCrossings and SharedAncestor are the raw structural span between
	// the two module names (relationship/classify.HierarchySpan).
	BoundaryCrossings int
	SharedAncestor    int
	// There is deliberately no runtime-topology field. V1 observes no runtime
	// topology, and a permanently-false field would read as "no runtime
	// boundary here" rather than "nobody looked"; the operations dimension
	// reports the gap as unmeasured instead.
}

// SeamScoreDistribution is the per-seam spread of book balance scores. A mean
// alone hides a concentrated tail, which is exactly the failure the repository
// scalar had, so the whole distribution travels with the seam.
type SeamScoreDistribution struct {
	// N is the number of scored edges the distribution was computed over.
	N int
	// Min, Median, and Max are nearest-rank order statistics over the sorted
	// scores. Median uses the same ceil(p*n)-1 rule as the deciles.
	Min    int
	Median int
	Max    int
	// P10 and P90 are nil when N < 10: a decile over fewer than ten samples is
	// an invented number, not a measurement.
	P10 *int
	P90 *int
	// Mean is the arithmetic mean of the scored balances.
	Mean float64
}

// Seam is one logical ordered module pair and everything measured about it.
//
// The seam, not the import edge, is the unit of coupling reporting. R4 in the
// migration plan is the reason: one logical seam can be expressed by forty
// imports, so an edge count reads as forty times the risk of an identical seam
// expressed once.
type Seam struct {
	ID         string
	FromModule string
	ToModule   string

	// Edges is the observed source-graph edge count for this seam;
	// ScoredEdges + AbstainedEdges is its measurement denominator. An abstained
	// edge is a real seam edge whose rung is unknown, so it stays in the
	// denominator rather than disappearing.
	Edges          int
	ScoredEdges    int
	AbstainedEdges int

	// Strength, Distance, and Volatility are the seam's worst observed rungs —
	// the ones that drive its severity. Per-edge detail stays on the edges.
	Strength   Strength
	Distance   Distance
	Volatility Volatility
	// VolatilityProvenance names where the target's volatility came from
	// (declared, inherited, cascade, undeclared).
	VolatilityProvenance string

	// Severity is the worst active underlying edge band. It is never a second
	// repository scalar and is never averaged across seams.
	Severity Severity

	RawDistance SeamDistance
	Quadrant    SeamQuadrant
	Scores      SeamScoreDistribution

	CriticalEdges       int
	HighOrWorseEdges    int
	CriticalSharePct    int
	HighOrWorseSharePct int

	// Labels holds the approved label keys applied to this seam, and
	// LabelEvidenceHash the evidence hash they were approved against. Empty
	// labels means "no override", never "every edge is a contract".
	Labels            []string
	LabelEvidenceHash string
	Confidence        SeamConfidence

	RoleExpectation SeamRoleExpectation
	Hypothesis      SeamHypothesis

	// DistributedMonolith marks a seam with at least one active edge in the
	// critical band at high raw distance (different owner or different deploy
	// unit). It is the seam fact the distributed-monolith policy counts; the
	// policy decision itself lives in assessment.
	DistributedMonolith bool
}

// SeamScores computes the nearest-rank distribution over balance scores.
// Percentiles use ceil(p*n)-1 against the ascending sort, and p10/p90 abstain
// below ten samples. The input slice is sorted in place.
func SeamScores(scores []int) SeamScoreDistribution {
	if len(scores) == 0 {
		return SeamScoreDistribution{}
	}
	sort.Ints(scores)
	n := len(scores)
	sum := 0
	for _, s := range scores {
		sum += s
	}
	d := SeamScoreDistribution{
		N:      n,
		Min:    scores[0],
		Max:    scores[n-1],
		Median: scores[nearestRank(0.5, n)],
		Mean:   float64(sum) / float64(n),
	}
	if n >= 10 {
		p10, p90 := scores[nearestRank(0.10, n)], scores[nearestRank(0.90, n)]
		d.P10, d.P90 = &p10, &p90
	}
	return d
}

// nearestRank is the frozen percentile index: ceil(p*n)-1, clamped into range.
func nearestRank(p float64, n int) int {
	i := int(math.Ceil(p*float64(n))) - 1
	if i < 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	return i
}
