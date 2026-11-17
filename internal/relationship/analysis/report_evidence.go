package analysis

import (
	"sort"

	"github.com/alexei-led/archfit/v4/internal/model/evidence"
	"github.com/alexei-led/archfit/v4/internal/policy"
	"github.com/alexei-led/archfit/v4/internal/relationship"
)

func reportRelationships(set relationship.Set, modules map[string]policy.ModuleDef) evidence.ReportRelationships {
	out := evidence.ReportRelationships{ModuleInventoryHash: policy.ModelHash(modules), TotalEdges: len(set.Edges), Pairs: []evidence.ReportPair{}, Components: []evidence.ReportComponent{}}
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		m := modules[name]
		c := evidence.ReportComponent{Name: name, Public: evidence.ReportPathsFact{Origin: evidence.ReportUnknown, Refs: []evidence.ReportRef{}},
			Subdomain:  evidence.ReportStringFact{Origin: evidence.ReportUnknown, Refs: []evidence.ReportRef{}},
			Volatility: evidence.ReportStringFact{Origin: evidence.ReportUnknown, Refs: []evidence.ReportRef{}},
			Unknowns:   []evidence.ReportUnknownFact{{Fact: "component_origin", Reason: "effective classifier input has no retained declaration ancestry", Owner: "relationship/analysis"}}}
		if m.Public != nil {
			public := append([]string{}, m.Public...)
			c.Public.Effective = &public
		}
		if m.Volatility != "" {
			c.Volatility.Effective = new(m.Volatility)
		}
		if m.Subdomain != "" {
			c.Subdomain.Effective = new(m.Subdomain)
		}
		out.Components = append(out.Components, c)
	}
	pairs := map[string]*evidence.ReportPair{}
	for _, e := range set.Edges {
		if !seamEdge(e) {
			continue
		}
		out.PairEdges++
		id := relationship.SeamID(e.FromModule, e.ToModule)
		pair := pairs[id]
		if pair == nil {
			pair = &evidence.ReportPair{PairID: id, Observations: []evidence.ReportObservation{}, Unknowns: []evidence.ReportUnknownFact{}}
			pairs[id] = pair
		}
		observation := evidence.ReportObservation{FromNode: e.FromID, ToNode: e.ToID, EdgeKind: e.Kind, Language: e.Language, Sites: []evidence.ReportSite{}, Connascence: []evidence.ReportConnascence{}}
		seen := map[evidence.ReportSite]bool{}
		for _, location := range e.Locations {
			site := evidence.ReportSite{Path: location.File, Line: location.Line}
			if !seen[site] {
				observation.Sites = append(observation.Sites, site)
				seen[site] = true
			}
		}
		sort.Slice(observation.Sites, func(i, j int) bool {
			a, b := observation.Sites[i], observation.Sites[j]
			if a.Path != b.Path {
				return a.Path < b.Path
			}
			return a.Line < b.Line
		})
		if len(observation.Sites) == 0 {
			pair.Unknowns = append(pair.Unknowns, evidence.ReportUnknownFact{Fact: "sites", Reason: "retained edge has no source locations: " + e.FromID + " → " + e.ToID, Owner: "extractor"})
		}
		for _, item := range e.Classified.Connascence {
			observation.Connascence = append(observation.Connascence, evidence.ReportConnascence{Kind: item.Kind, Source: item.Source, Detail: item.Detail})
		}
		pair.Observations = append(pair.Observations, observation)
	}
	ids := make([]string, 0, len(pairs))
	for id := range pairs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		pair := pairs[id]
		sort.SliceStable(pair.Observations, func(i, j int) bool {
			a, b := pair.Observations[i], pair.Observations[j]
			if a.FromNode != b.FromNode {
				return a.FromNode < b.FromNode
			}
			if a.ToNode != b.ToNode {
				return a.ToNode < b.ToNode
			}
			if a.EdgeKind != b.EdgeKind {
				return a.EdgeKind < b.EdgeKind
			}
			return a.Language < b.Language
		})
		out.Pairs = append(out.Pairs, *pair)
	}
	return out
}
