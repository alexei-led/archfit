package engine

import (
	"sort"

	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/module"
	"github.com/alexei-led/archfit/internal/relationship/coupling"
	"github.com/alexei-led/archfit/internal/relationship/scoring"
)

// localCouplingOffenderCap bounds WorstOffenders per module — enough for an
// agent to act on, small enough not to bloat the JSON. Deliberate ceiling:
// raise it only if agents demonstrably need more than the five worst edges.
const localCouplingOffenderCap = 5

// buildLocalCoupling aggregates scored same-module edges into the report-only
// local_coupling block — the book Ch10 local-complexity quadrant (design note):
// same-module edges score with the book formula at the same-module distance
// rung (D=2), but report per module here and NEVER enter coupling_balance's
// denominator. Cross-module coupling and intra-module cohesion are different
// fractal levels; coupling_balance stays the cross-module number consumers
// already track. Never consumed by verdict or gate logic.
//
// Modules are keyed by the from-endpoint's module (falling back to the to-
// endpoint when the source does not resolve); edges resolving to no module are
// skipped — a same-module classification without module coverage carries no
// actionable module key.
func buildLocalCoupling(g *graph.Graph, idx coupling.Index, mm module.Map) []evidence.LocalCouplingModule {
	type agg struct {
		scored, abstained, complexity, balanceSum int
		offenders                                 []evidence.LocalCouplingEdge
	}
	byModule := make(map[string]*agg)

	for _, e := range g.Edges() {
		key := e.From + "\x00" + e.To + "\x00" + string(e.Kind)
		cl, ok := idx[key]
		if !ok || cl.Distance != coupling.DistanceSameModule {
			continue
		}
		fromPath := stripPrefix(e.From)
		toPath := stripPrefix(e.To)
		mod, resolved := mm.ModuleFor(fromPath)
		if !resolved {
			mod, resolved = mm.ModuleFor(toPath)
		}
		if !resolved {
			continue
		}
		a := byModule[mod]
		if a == nil {
			a = &agg{}
			byModule[mod] = a
		}
		if !cl.Score.Scored {
			a.abstained++
			continue
		}
		a.scored++
		a.balanceSum += cl.Score.Balance
		if scoring.LocalComplexity(cl) {
			a.complexity++
		}
		if cl.Score.Band != coupling.SeverityNone {
			off := evidence.LocalCouplingEdge{
				From:     fromPath,
				To:       toPath,
				Strength: string(cl.Strength),
				Balance:  cl.Score.Balance,
				Band:     string(cl.Score.Band),
			}
			if len(e.Locations) > 0 {
				off.File = e.Locations[0].File
				off.Line = e.Locations[0].Line
			}
			a.offenders = append(a.offenders, off)
		}
	}

	names := make([]string, 0, len(byModule))
	for name := range byModule {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]evidence.LocalCouplingModule, 0, len(names))
	for _, name := range names {
		a := byModule[name]
		m := evidence.LocalCouplingModule{
			Module:          name,
			ScoredEdges:     a.scored,
			AbstainedEdges:  a.abstained,
			ComplexityEdges: a.complexity,
		}
		if a.scored > 0 {
			m.ComplexitySharePct = 100 * a.complexity / a.scored
			m.MeanBalance = float64(a.balanceSum) / float64(a.scored)
		}
		sort.Slice(a.offenders, func(i, j int) bool {
			oi, oj := a.offenders[i], a.offenders[j]
			if oi.Balance != oj.Balance {
				return oi.Balance < oj.Balance
			}
			if oi.From != oj.From {
				return oi.From < oj.From
			}
			return oi.To < oj.To
		})
		if len(a.offenders) > localCouplingOffenderCap {
			a.offenders = a.offenders[:localCouplingOffenderCap]
		}
		m.WorstOffenders = a.offenders
		out = append(out, m)
	}
	return out
}
