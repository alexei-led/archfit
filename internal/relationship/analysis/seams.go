package analysis

import (
	"sort"

	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship"
	"github.com/alexei-led/archfit/v3/internal/relationship/classify"
	"github.com/alexei-led/archfit/v3/internal/relationship/labels"
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
	// Tree is the containment tree of the classified module map.
	Tree classify.Containment
	// ClonePairs are the clone facts. A pair whose modules share an import edge
	// attaches to that seam; a clone-only pair has no seam and is ignored here.
	ClonePairs []relationship.ClonePair
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
		a.add(e, classify.CohesiveRole(in.Config.Modules[e.FromModule].Role))
	}
	attachCloneFacts(acc, in.ClonePairs)
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
	qualifying                 []*relationship.Edge
	nonHighLLM                 bool
	worst                      *relationship.Edge
	worstStrength, worstDistOr int
	// qualWorst is the lowest-balance qualifying edge: when the seam qualifies it
	// is the driving edge, so strength, volatility, reason and qualification all
	// describe the same edge.
	qualWorst *relationship.Edge
	// worstClone is set when a clone fact outscores every edge: the driving fact
	// is then symmetric coupling with no import edge behind it.
	worstClone *relationship.ClonePair
	cloneFacts int
}

// qualifyingEdgeCap bounds the repair evidence a seam keeps. A seam can be
// expressed by hundreds of imports; twenty named files are enough to start the
// repair, and the full ledger stays in the edge set.
const qualifyingEdgeCap = 20

func (a *seamAccumulator) add(e *relationship.Edge, cohesive bool) {
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
	}
	if relationship.QualifiesDistributedMonolith(e.Classified.Score.Scored && e.Classified.Score.Balance > 0, e.Strength, e.Distance, e.Volatility, cohesive) {
		a.distributed = true
		a.qualifying = append(a.qualifying, e)
		if a.qualWorst == nil || lowerEdge(e, a.qualWorst) {
			a.qualWorst = e
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
	span := classify.HierarchySpan(in.Tree, a.from, a.to)
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
			Basis:             a.worstBasis(in.Tree, a.from, a.to),
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
		QualifyingEdges:     a.qualifyingEdges(),
	}
	s.VolatilityProvenance = a.volatilityProvenance(volatility)
	s.Labels, s.LabelEvidenceHash = seamLabels(in, a.from, a.to)
	s.Confidence = seamConfidence(a.scored, a.abstained, a.nonHighLLM)
	a.applyDrivingFact(&s, toDef, classify.CohesiveRole(fromDef.Role))
	return s
}

// applyDrivingFact sets the fields that describe the seam's driving fact: the
// lowest-balance qualifying edge when the seam qualifies, else the lowest-balance
// scored edge or clone fact. Strength, volatility, quadrant and hypothesis then
// all describe the same fact a reader can go and look at.
func (a *seamAccumulator) applyDrivingFact(s *relationship.Seam, toDef policy.ModuleDef, cohesive bool) {
	in := relationship.HypothesisInput{TargetPublic: len(toDef.Public) > 0, CohesiveRole: cohesive}
	switch {
	case a.qualWorst != nil:
		e := a.qualWorst
		s.Strength, s.Volatility = e.Strength, e.Volatility
		in.Strength, in.Volatility, in.Band = e.Strength, e.Volatility, e.Classified.Score.Band
	case a.worstClone != nil:
		c := a.worstClone
		s.Strength, s.Volatility = relationship.StrengthSymmetric, c.Volatility
		in.Strength, in.Volatility, in.Band, in.Clone = relationship.StrengthSymmetric, c.Volatility, c.Classified.Score.Band, true
	case a.worst != nil:
		e := a.worst
		s.Strength, s.Volatility = e.Strength, e.Volatility
		in.Strength, in.Volatility, in.Band = e.Strength, e.Volatility, e.Classified.Score.Band
	default:
		return
	}
	s.Hypothesis = relationship.BalancingHypothesis(in)
}

// volatilityProvenance reports where the seam's volatility came from. It reads
// the target, unless the target's own volatility is undeclared and the source's
// is not: functional coupling can take its high volatility from the source side.
func (a *seamAccumulator) volatilityProvenance(by map[string]classify.ModuleVolatility) string {
	to, okTo := by[a.to]
	from, okFrom := by[a.from]
	switch {
	case okTo && okFrom && to.Reported() == classify.VolatilitySourceUndeclared && from.Reported() != classify.VolatilitySourceUndeclared:
		return string(from.Reported())
	case okTo:
		return string(to.Reported())
	default:
		return ""
	}
}

// lowerEdge reports whether e outranks cur as the driving edge: the lower
// balance wins, then the higher strength ordinal, then the higher distance
// ordinal, then the endpoint IDs, so the choice is stable across runs.
func lowerEdge(e, cur *relationship.Edge) bool {
	eb, cb := e.Classified.Score.Balance, cur.Classified.Score.Balance
	if eb != cb {
		return eb < cb
	}
	es, cs := e.Classified.Score.Breakdown.StrengthValue, cur.Classified.Score.Breakdown.StrengthValue
	if es != cs {
		return es > cs
	}
	ed, cd := e.Classified.Score.Breakdown.DistanceValue, cur.Classified.Score.Breakdown.DistanceValue
	if ed != cd {
		return ed > cd
	}
	return e.FromID+"\x00"+e.ToID < cur.FromID+"\x00"+cur.ToID
}

// attachCloneFacts adds each connected clone pair to one seam. A pair between A
// and B attaches to A→B or B→A, whichever exists; when both exist, to the seam
// whose ID sorts first. A clone fact scores like an edge (symmetric, D=9, the
// worse volatility of the pair) and can set the seam's severity and hypothesis.
// It counts in scored edges, never in edges, and it never makes a seam qualify.
func attachCloneFacts(acc map[string]*seamAccumulator, pairs []relationship.ClonePair) {
	for i := range pairs {
		p := &pairs[i]
		if !p.Connected {
			continue
		}
		fwd, rev := acc[p.FromModule+"\x00"+p.ToModule], acc[p.ToModule+"\x00"+p.FromModule]
		target := fwd
		switch {
		case fwd == nil:
			target = rev
		case rev != nil && relationship.SeamID(rev.from, rev.to) < relationship.SeamID(fwd.from, fwd.to):
			target = rev
		}
		if target != nil {
			target.addClone(p)
		}
	}
}

func (a *seamAccumulator) addClone(p *relationship.ClonePair) {
	a.cloneFacts++
	sc := p.Classified.Score
	if !sc.Scored || sc.Balance <= 0 {
		a.abstained++
		return
	}
	a.scored++
	a.balances = append(a.balances, sc.Balance)
	if severityRank(sc.Band) > severityRank(a.severity) {
		a.severity = sc.Band
	}
	if sc.Band == relationship.SeverityCritical {
		a.critical++
	}
	if sc.Band == relationship.SeverityCritical || sc.Band == relationship.SeverityHigh {
		a.highOrWorse++
	}
	// A clone fact is symmetric (S=9): at an equal balance it outranks an edge of
	// lower strength, as lowerEdge does between edges.
	if a.worst == nil || sc.Balance < a.worst.Classified.Score.Balance ||
		(sc.Balance == a.worst.Classified.Score.Balance && sc.Breakdown.StrengthValue > a.worstStrength) {
		a.worstClone = p
		a.worstStrength, a.worstDistOr = sc.Breakdown.StrengthValue, sc.Breakdown.DistanceValue
	}
}

// qualifyingEdges returns the distributed-monolith edges in endpoint-ID order,
// capped. The order is the edge identity, not a severity rank: every one of
// them qualifies the seam equally, and a stable order keeps the repair evidence
// identical across runs over the same tree.
func (a *seamAccumulator) qualifyingEdges() []relationship.Edge {
	if len(a.qualifying) == 0 {
		return nil
	}
	sort.Slice(a.qualifying, func(i, j int) bool {
		x, y := a.qualifying[i], a.qualifying[j]
		if x.FromID != y.FromID {
			return x.FromID < y.FromID
		}
		if x.ToID != y.ToID {
			return x.ToID < y.ToID
		}
		return x.Kind < y.Kind
	})
	out := make([]relationship.Edge, 0, min(len(a.qualifying), qualifyingEdgeCap))
	for _, e := range a.qualifying[:min(len(a.qualifying), qualifyingEdgeCap)] {
		out = append(out, *e)
	}
	return out
}

// systemContainer names the container of two top-level modules: the system.
const systemContainer = "system"

// worstBasis is the boundary of the edge that drives the seam, joined to the
// container both sides share: "<boundary>@<container>". An abstained-only seam
// has no scored edge to point at, so it reports no basis rather than a borrowed
// one.
func (a *seamAccumulator) worstBasis(tree classify.Containment, from, to string) string {
	if a.worst == nil {
		return ""
	}
	container := tree.Container(from, to)
	if container == "" {
		container = systemContainer
	}
	return a.worst.Classified.DistanceBasis + "@" + container
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
		relationship.DistanceCrossModule: 2, relationship.DistanceCrossModuleDiffOwner: 3,
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
