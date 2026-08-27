package analysis

import (
	"sort"

	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/relationship/classify"
	"github.com/alexei-led/archfit/internal/relationship/coupling"
	"github.com/alexei-led/archfit/internal/relationship/labels"
)

// seamInput is everything the seam ledger needs. Every field is a value this
// stage already resolved: the ledger measures nothing new, it regroups the
// classified edges by the boundary a human would actually redesign.
type seamInput struct {
	Set             relationship.Set
	Config          classify.Config
	DeclaredModules map[string]policy.ModuleDef
	Graph           *graph.Graph
	// EvidenceHashes maps a label key to the dependency-surface hash the label
	// was approved against, as computed for this run.
	EvidenceHashes map[string]string
	// LabelEvidenceHashes maps a label key to the hash the LABEL stored, which
	// is what the seam publishes. It is a different fact from EvidenceHashes
	// above: that one is this run's evidence, and a hand-authored label with no
	// stored hash must publish nothing rather than borrow it.
	LabelEvidenceHashes map[string]string
}

// buildSeams groups the classified cross-boundary edges into one record per
// ordered module pair.
//
// The seam, not the import edge, is the unit of coupling reporting: one logical
// seam can be expressed by forty imports, and counting the imports reports it
// as forty times the risk of an identical seam written once (plan R4).
//
// Only source-graph edges between two resolved modules take part. A clone-only
// duplicated-knowledge pair has no import edge, so it is not a seam; it keeps
// its own report block. An edge whose target left the declared modules has no
// second module, so it is external dependency hygiene, not an internal seam.
func buildSeams(in seamInput) []relationship.Seam {
	acc := map[string]*seamAccumulator{}
	order := []string{}
	for i := range in.Set.Edges {
		e := &in.Set.Edges[i]
		if !seamEdge(*e) {
			continue
		}
		key := e.FromModule + "\x00" + e.ToModule
		a, ok := acc[key]
		if !ok {
			// Seed the strength rung. Its "worst so far" comparison is
			// rank-based and the zero Strength shares a rank with
			// StrengthUnknown, so a seam whose every edge abstained never
			// overwrote the zero value and published "" — a string outside the
			// strength vocabulary. (Distance cannot hit this: seamEdge filters
			// the unknown rung. Severity can, but SeverityNone IS the zero
			// value, so the zero is the right answer there.)
			a = &seamAccumulator{from: e.FromModule, to: e.ToModule, strength: relationship.StrengthUnknown}
			acc[key], order = a, append(order, key)
		}
		a.add(e)
	}
	sort.Strings(order)
	volatility := classify.VolatilityProvenanceByModule(in.Graph, in.DeclaredModules, in.Config)
	out := make([]relationship.Seam, 0, len(order))
	for _, key := range order {
		out = append(out, acc[key].seam(in, volatility))
	}
	return out
}

// seamEdge reports whether an edge belongs to an internal seam. Same-module
// edges are a different fractal level (they surface in local_coupling), and an
// unknown distance means the target is not a declared module at all.
func seamEdge(e relationship.Edge) bool {
	return e.FromModule != "" && e.ToModule != "" &&
		e.Distance != relationship.DistanceSameModule && e.Distance != relationship.DistanceUnknown
}

// seamAccumulator rolls up one ordered module pair. It keeps the worst scored
// edge rather than an average: the seam's severity, quadrant, and hypothesis
// must all describe the same concrete edge a reader can go and look at.
type seamAccumulator struct {
	from, to                   string
	edges, scored, abstained   int
	balances                   []int
	critical, highOrWorse      int
	strength                   relationship.Strength
	distance                   relationship.Distance
	volatility                 relationship.Volatility
	severity                   relationship.Severity
	distributed                bool
	nonHighLLM                 bool
	worst                      *relationship.Edge
	worstStrength, worstDistOr int
}

func (a *seamAccumulator) add(e *relationship.Edge) {
	a.edges++
	if e.Provenance.StrengthFromNonHighLLM {
		a.nonHighLLM = true
	}
	if strengthRank(e.Strength) > strengthRank(a.strength) {
		a.strength = e.Strength
	}
	if distanceRank(e.Distance) > distanceRank(a.distance) {
		a.distance = e.Distance
	}
	a.volatility = worseVolatility(a.volatility, e.Volatility)
	if severityRank(e.Classified.Score.Band) > severityRank(a.severity) {
		a.severity = e.Classified.Score.Band
	}
	if e.Classified.Score.Band == relationship.SeverityCritical {
		a.critical++
		if coupling.DistanceIsHigh(e.Distance) {
			a.distributed = true
		}
	}
	if e.Classified.Score.Band == relationship.SeverityCritical || e.Classified.Score.Band == relationship.SeverityHigh {
		a.highOrWorse++
	}
	if !e.Classified.Score.Scored || e.Classified.Score.Balance <= 0 {
		a.abstained++
		return
	}
	a.scored++
	a.balances = append(a.balances, e.Classified.Score.Balance)
	a.keepWorst(e)
}

// keepWorst tracks the lowest-balance scored edge. Ties break on the higher
// strength ordinal, then the higher distance ordinal, then the edge endpoints,
// so the choice is stable across runs and independent of map iteration.
func (a *seamAccumulator) keepWorst(e *relationship.Edge) {
	s, d := e.Classified.Score.Breakdown.StrengthValue, e.Classified.Score.Breakdown.DistanceValue
	if a.worst == nil {
		a.worst, a.worstStrength, a.worstDistOr = e, s, d
		return
	}
	switch {
	case e.Classified.Score.Balance != a.worst.Classified.Score.Balance:
		if e.Classified.Score.Balance < a.worst.Classified.Score.Balance {
			a.worst, a.worstStrength, a.worstDistOr = e, s, d
		}
	case s != a.worstStrength:
		if s > a.worstStrength {
			a.worst, a.worstStrength, a.worstDistOr = e, s, d
		}
	case d != a.worstDistOr:
		if d > a.worstDistOr {
			a.worst, a.worstStrength, a.worstDistOr = e, s, d
		}
	case e.FromID+"\x00"+e.ToID < a.worst.FromID+"\x00"+a.worst.ToID:
		a.worst, a.worstStrength, a.worstDistOr = e, s, d
	}
}

func (a *seamAccumulator) seam(in seamInput, volatility map[string]classify.ModuleVolatility) relationship.Seam {
	fromDef, toDef := in.Config.Modules[a.from], in.Config.Modules[a.to]
	span := classify.HierarchySpan(a.from, a.to)
	denominator := a.scored + a.abstained
	s := relationship.Seam{
		ID:             relationship.SeamID(a.from, a.to),
		FromModule:     a.from,
		ToModule:       a.to,
		Edges:          a.edges,
		ScoredEdges:    a.scored,
		AbstainedEdges: a.abstained,
		Strength:       a.strength,
		Distance:       a.distance,
		Volatility:     a.volatility,
		Severity:       a.severity,
		RawDistance: relationship.SeamDistance{
			Level:             a.distance,
			Basis:             a.worstBasis(),
			FromOwner:         fromDef.Owner,
			ToOwner:           toDef.Owner,
			SameOwner:         fromDef.Owner != "" && fromDef.Owner == toDef.Owner,
			FromDeployUnit:    fromDef.DeployUnit,
			ToDeployUnit:      toDef.DeployUnit,
			SameDeployUnit:    fromDef.DeployUnit != "" && fromDef.DeployUnit == toDef.DeployUnit,
			BoundaryCrossings: span.BoundaryCrossings,
			SharedAncestor:    span.SharedAncestor,
		},
		Scores:              relationship.SeamScores(a.balances),
		CriticalEdges:       a.critical,
		HighOrWorseEdges:    a.highOrWorse,
		CriticalSharePct:    sharePct(a.critical, denominator),
		HighOrWorseSharePct: sharePct(a.highOrWorse, denominator),
		Quadrant:            seamQuadrant(a.worstStrength, a.worstDistOr),
		RoleExpectation:     roleExpectation(fromDef.Role),
		DistributedMonolith: a.distributed,
	}
	if vp, ok := volatility[a.to]; ok {
		s.VolatilityProvenance = string(vp.Reported())
	}
	s.Labels, s.LabelEvidenceHash = seamLabels(in, a.from, a.to)
	s.Confidence = seamConfidence(a.scored, a.abstained, a.nonHighLLM)
	s.Hypothesis = seamHypothesis(a.worst, s.RoleExpectation, a.volatility)
	return s
}

// worstBasis is the distance basis of the edge that drives the seam. An
// abstained-only seam has no scored edge to point at, so it reports no basis
// rather than a borrowed one.
func (a *seamAccumulator) worstBasis() string {
	if a.worst == nil {
		return ""
	}
	return a.worst.Classified.DistanceBasis
}

// seamLabels reports the approved label keys in effect for this seam and the
// evidence hash they were approved against. An empty result means no override
// is in effect — it never means every edge is a contract.
func seamLabels(in seamInput, from, to string) ([]string, string) {
	key := labels.Key(from, to)
	var used []string
	if _, ok := in.Config.ApprovedLabels[key]; ok {
		used = append(used, key)
	}
	if _, ok := in.Config.LLMLabels[key]; ok && len(used) == 0 {
		used = append(used, key)
	}
	if len(used) == 0 {
		return nil, ""
	}
	return used, in.LabelEvidenceHashes[key]
}

// seamConfidence reports how much of the seam was measured. An abstained edge
// is an unknown rung, not an absent one, so a seam that is mostly abstained
// cannot claim the same confidence as one that is fully scored.
func seamConfidence(scored, abstained int, nonHighLLM bool) relationship.SeamConfidence {
	switch {
	case scored == 0:
		return relationship.SeamConfidenceUnrated
	case abstained > scored:
		return relationship.SeamConfidenceLow
	case abstained > 0 || nonHighLLM:
		return relationship.SeamConfidenceMedium
	default:
		return relationship.SeamConfidenceHigh
	}
}

// seamQuadrant places the seam's worst edge in the book Ch10 strength/distance
// matrix. The ladders run 1..10, so "high" is the upper half.
func seamQuadrant(strength, distance int) relationship.SeamQuadrant {
	if strength == 0 || distance == 0 {
		return ""
	}
	strong, far := strength > 5, distance > 5
	switch {
	case strong && far:
		return relationship.SeamQuadrantTight
	case strong:
		return relationship.SeamQuadrantCohesive
	case far:
		return relationship.SeamQuadrantLoose
	default:
		return relationship.SeamQuadrantLowCohesion
	}
}

// seamHypothesis names the single cheapest move for the seam.
//
// A cohesive-role source (composition_root, generated, test) is wiring by
// design: strong fan-out from it is the point of the module, so the answer is
// leave_alone and no label is needed to say so. Otherwise the scorer's own
// cheapest move wins; when it offers none but the seam is still banded, an
// undeclared target volatility is what is holding the balance down — the
// scorer treats it as the worst case — so declaring it is the honest move.
func seamHypothesis(worst *relationship.Edge, role relationship.SeamRoleExpectation, volatility relationship.Volatility) relationship.SeamHypothesis {
	if worst == nil {
		return ""
	}
	if role == relationship.SeamRoleCompositionRoot {
		return relationship.SeamHypothesisLeaveAlone
	}
	switch worst.Classified.Score.CheapestMove {
	case string(relationship.SeamHypothesisReduceStrength):
		return relationship.SeamHypothesisReduceStrength
	case string(relationship.SeamHypothesisReduceDistance):
		return relationship.SeamHypothesisReduceDistance
	}
	if worst.Classified.Score.Band != relationship.SeverityNone &&
		(volatility == relationship.VolatilityUndeclared || volatility == relationship.VolatilityUnknown) {
		return relationship.SeamHypothesisDeclareVolatility
	}
	return relationship.SeamHypothesisLeaveAlone
}

// roleExpectation projects the module's declared role onto the seam. Roles with
// no coupling expectation (generated, test, none) report none: an empty
// expectation is honest, an invented one is not.
func roleExpectation(r policy.Role) relationship.SeamRoleExpectation {
	switch r {
	case policy.RoleCompositionRoot:
		return relationship.SeamRoleCompositionRoot
	case policy.RoleAdapter:
		return relationship.SeamRoleAdapter
	case policy.RoleCore:
		return relationship.SeamRoleCore
	case policy.RoleSharedModel:
		return relationship.SeamRoleSharedModel
	default:
		return ""
	}
}

func sharePct(part, total int) int {
	if total <= 0 {
		return 0
	}
	return part * 100 / total
}

// Rung ranks used only to roll a seam up to its worst observed value. They are
// ordering helpers, never score inputs — the book ordinals live in the scorer.
var (
	strengthRanks = map[relationship.Strength]int{
		relationship.StrengthUnknown: 0, relationship.StrengthContract: 1, relationship.StrengthModel: 2,
		relationship.StrengthFunctional: 3, relationship.StrengthSymmetric: 4, relationship.StrengthIntrusive: 5,
	}
	distanceRanks = map[relationship.Distance]int{
		relationship.DistanceUnknown: 0, relationship.DistanceSameModule: 1,
		relationship.DistanceCrossModuleSameOwner: 2, relationship.DistanceCrossModuleDiffOwner: 3,
		relationship.DistanceCrossDeployUnit: 4, relationship.DistanceExternal: 5,
	}
	severityRanks = map[relationship.Severity]int{
		relationship.SeverityNone: 0, relationship.SeverityLow: 1, relationship.SeverityMedium: 2,
		relationship.SeverityHigh: 3, relationship.SeverityCritical: 4,
	}
	// volatilityRanks mirror the scorer's conservative reading: an undeclared
	// or unknown volatility scores as the worst case, so it rolls up as one.
	volatilityRanks = map[relationship.Volatility]int{
		relationship.VolatilityFrozen: 1, relationship.VolatilityLow: 3, relationship.VolatilityMedium: 6,
		relationship.VolatilityHigh: 10, relationship.VolatilityUndeclared: 10, relationship.VolatilityUnknown: 10,
	}
)

func strengthRank(s relationship.Strength) int { return strengthRanks[s] }
func distanceRank(d relationship.Distance) int { return distanceRanks[d] }
func severityRank(s relationship.Severity) int { return severityRanks[s] }

// worseVolatility keeps the worse of two rungs. At equal rank a declared value
// wins over undeclared/unknown, so "high" is reported rather than "undeclared"
// when both appear in one seam.
func worseVolatility(current, candidate relationship.Volatility) relationship.Volatility {
	cur, cand := volatilityRanks[current], volatilityRanks[candidate]
	switch {
	case cand > cur:
		return candidate
	case cand < cur:
		return current
	case current == relationship.VolatilityUndeclared || current == relationship.VolatilityUnknown:
		return candidate
	default:
		return current
	}
}
