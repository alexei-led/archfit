package pipeline

import (
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/relationship"
)

func runtimeModules(in []relationship.RuntimeSignal) []evidence.RuntimeAsyncModule {
	out := make([]evidence.RuntimeAsyncModule, 0, len(in))
	for _, v := range in {
		out = append(out, evidence.RuntimeAsyncModule{Module: v.Module, IntegrationKind: v.IntegrationKind, Count: v.Count, Confidence: v.Confidence})
	}
	return out
}
func runtimeEdges(in []relationship.RuntimeRelationship) []evidence.RuntimeAsyncEdge {
	out := make([]evidence.RuntimeAsyncEdge, 0, len(in))
	for _, e := range in {
		sites := make([]evidence.RuntimeAsyncSite, 0, len(e.Sites))
		for _, s := range e.Sites {
			sites = append(sites, evidence.RuntimeAsyncSite{File: s.File, Line: s.Line, Library: s.Library, IntegrationKind: s.IntegrationKind, Language: s.Language})
		}
		out = append(out, evidence.RuntimeAsyncEdge{FromModule: e.FromModule, Target: e.Target, IntegrationKind: e.IntegrationKind, Count: e.Count, Confidence: e.Confidence, Sites: sites})
	}
	return out
}
