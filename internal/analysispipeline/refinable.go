package pipeline

import (
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/relationship/coupling"
	"github.com/alexei-led/archfit/internal/relationship/labels"
)

// RefinablePair is a data-only candidate for semantic coupling-label review.
type RefinablePair struct {
	From, To    string
	Strength    string
	EdgeCount   int
	SamplePaths []string
}

// SelectRefinablePairs selects unresolved or heuristic-strength module pairs
// for semantic review. It keeps graph and coupling interpretation in the
// relationship pipeline; callers receive only prompt-ready data.
func SelectRefinablePairs(g *graph.Graph, idx coupling.Index, mm policy.ModuleMap, approved map[string]struct{}) []RefinablePair {
	if g == nil {
		return nil
	}
	type aggregate struct {
		strength string
		count    int
		samples  []string
	}
	pairs := map[string]*aggregate{}
	for _, e := range g.Edges() {
		cl, ok := idx[e.From+"\x00"+e.To+"\x00"+string(e.Kind)]
		if !ok || (cl.Strength != coupling.StrengthFunctional && cl.Strength != coupling.StrengthModel && cl.Strength != coupling.StrengthUnknown) {
			continue
		}
		if cl.Distance == coupling.DistanceSameModule || cl.Distance == coupling.DistanceUnknown || cl.Distance == "" {
			continue
		}
		fromPath, toPath := graph.NodePath(e.From), graph.NodePath(e.To)
		fromMod, okF := mm.ModuleFor(fromPath)
		toMod, okT := mm.ModuleFor(toPath)
		if !okF || !okT || fromMod == toMod {
			continue
		}
		key := labels.Key(fromMod, toMod)
		if _, ok := approved[key]; ok {
			continue
		}
		a := pairs[key]
		if a == nil {
			a = &aggregate{strength: string(cl.Strength)}
			pairs[key] = a
		}
		a.count++
		if len(a.samples) < 5 {
			a.samples = append(a.samples, fromPath+" -> "+toPath)
		}
	}
	out := make([]RefinablePair, 0, len(pairs))
	for key, a := range pairs {
		from, to, _ := strings.Cut(key, "\x00")
		sort.Strings(a.samples)
		out = append(out, RefinablePair{From: from, To: to, Strength: a.strength, EdgeCount: a.count, SamplePaths: a.samples})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out
}

// SelectRefinableRelationshipPairs is the relationship-contract equivalent of
// SelectRefinablePairs for callers that already have the classified relationship
// stage result and should not recapture the raw graph through metrics.
func SelectRefinableRelationshipPairs(set relationship.Set, approved map[string]struct{}) []RefinablePair {
	type aggregate struct {
		strength string
		count    int
		samples  []string
	}
	pairs := map[string]*aggregate{}
	for _, edge := range set.DependencyEdges() {
		if edge.Strength != relationship.StrengthFunctional && edge.Strength != relationship.StrengthModel && edge.Strength != relationship.StrengthUnknown {
			continue
		}
		if edge.Distance == relationship.DistanceSameModule || edge.Distance == relationship.DistanceUnknown || edge.Distance == "" {
			continue
		}
		if edge.FromModule == "" || edge.ToModule == "" || edge.FromModule == edge.ToModule {
			continue
		}
		key := labels.Key(edge.FromModule, edge.ToModule)
		if _, ok := approved[key]; ok {
			continue
		}
		a := pairs[key]
		if a == nil {
			a = &aggregate{strength: string(edge.Strength)}
			pairs[key] = a
		}
		a.count++
		if len(a.samples) < 5 {
			a.samples = append(a.samples, edge.FromPath+" -> "+edge.ToPath)
		}
	}
	out := make([]RefinablePair, 0, len(pairs))
	for key, a := range pairs {
		from, to, _ := strings.Cut(key, "\x00")
		sort.Strings(a.samples)
		out = append(out, RefinablePair{From: from, To: to, Strength: a.strength, EdgeCount: a.count, SamplePaths: a.samples})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out
}
