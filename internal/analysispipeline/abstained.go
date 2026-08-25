package pipeline

import (
	"sort"

	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/module"
	"github.com/alexei-led/archfit/internal/relationship/coupling"
)

// AbstainedSample identifies one source location supporting an abstained edge.
type AbstainedSample struct {
	FromPath, ToPath string
	File             string
	Line             int
}

// AbstainedPair groups abstained edges by ordered module pair.
type AbstainedPair struct {
	From, To  string
	EdgeCount int
	Samples   []AbstainedSample
}

// SelectAbstainedPairs selects unknown-strength cross-module edges for review.
// It owns graph/classification interpretation; callers receive prompt DTOs.
func SelectAbstainedPairs(g *graph.Graph, idx coupling.Index, mm module.Map, approved map[string]struct{}, edgeCap, sampleCap int) (pairs []AbstainedPair, total int) {
	if g == nil {
		return nil, 0
	}
	byKey := map[string]*AbstainedPair{}
	included := 0
	for _, e := range g.Edges() {
		cl, ok := idx[e.From+"\x00"+e.To+"\x00"+string(e.Kind)]
		if !ok || cl.Strength != coupling.StrengthUnknown || cl.Distance == coupling.DistanceSameModule || cl.Distance == coupling.DistanceUnknown || cl.Distance == "" {
			continue
		}
		fromPath, toPath := graph.NodePath(e.From), graph.NodePath(e.To)
		fromMod, okF := mm.ModuleFor(fromPath)
		toMod, okT := mm.ModuleFor(toPath)
		if !okF || !okT || fromMod == toMod {
			continue
		}
		key := fromMod + "\x00" + toMod
		if _, ok := approved[key]; ok {
			continue
		}
		total++
		if included >= edgeCap {
			continue
		}
		included++
		p := byKey[key]
		if p == nil {
			p = &AbstainedPair{From: fromMod, To: toMod}
			byKey[key] = p
		}
		p.EdgeCount++
		if len(p.Samples) < sampleCap {
			s := AbstainedSample{FromPath: fromPath, ToPath: toPath}
			if len(e.Locations) > 0 {
				s.File, s.Line = e.Locations[0].File, e.Locations[0].Line
			}
			p.Samples = append(p.Samples, s)
		}
	}
	pairs = make([]AbstainedPair, 0, len(byKey))
	for _, p := range byKey {
		pairs = append(pairs, *p)
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].From != pairs[j].From {
			return pairs[i].From < pairs[j].From
		}
		return pairs[i].To < pairs[j].To
	})
	return pairs, total
}

// SelectAbstainedRelationshipPairs applies the same selection to captured
// application evidence, avoiding a second graph/domain representation.
func SelectAbstainedRelationshipPairs(set application.EnrichmentEvidence, approved map[string]struct{}, edgeCap, sampleCap int) (pairs []AbstainedPair, total int) {
	byKey := map[string]*AbstainedPair{}
	included := 0
	for _, edge := range set.Edges {
		if edge.Strength != "unknown" || edge.Distance == "same_module" || edge.Distance == "unknown" || edge.Distance == "" || edge.FromModule == "" || edge.ToModule == "" || edge.FromModule == edge.ToModule {
			continue
		}
		key := EnrichmentLabelKey(edge.FromModule, edge.ToModule)
		if _, ok := approved[key]; ok {
			continue
		}
		total++
		if included >= edgeCap {
			continue
		}
		included++
		p := byKey[key]
		if p == nil {
			p = &AbstainedPair{From: edge.FromModule, To: edge.ToModule}
			byKey[key] = p
		}
		p.EdgeCount++
		if len(p.Samples) < sampleCap {
			s := AbstainedSample{FromPath: edge.FromPath, ToPath: edge.ToPath}
			if len(edge.Locations) > 0 {
				s.File, s.Line = edge.Locations[0].File, edge.Locations[0].Line
			}
			p.Samples = append(p.Samples, s)
		}
	}
	pairs = make([]AbstainedPair, 0, len(byKey))
	for _, p := range byKey {
		pairs = append(pairs, *p)
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].From != pairs[j].From {
			return pairs[i].From < pairs[j].From
		}
		return pairs[i].To < pairs[j].To
	})
	return pairs, total
}
