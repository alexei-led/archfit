package pipeline

import (
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/relationship/labels"
)

// PairEvidence computes the current evidence hash per module pair (keyed by
// labels.Key): HashItems over "fromPath\x00toPath\x00kind" for every
// import-graph edge whose endpoints resolve to that ordered pair. Only pairs
// in wanted are hashed (pairs of interest are few; the graph can be large).
//
// Exported because enrich (cmd) must stamp drafts with EXACTLY the hash the
// engine will later verify — one computation, two callers.
func PairEvidence(g *graph.Graph, mm policy.ModuleMap, wanted map[string]struct{}) map[string]string {
	if len(wanted) == 0 {
		return nil
	}

	items := map[string][]string{}
	for _, e := range g.Edges() {
		fromPath := stripPrefix(e.From)
		toPath := stripPrefix(e.To)
		fromMod, okF := mm.ModuleFor(fromPath)
		toMod, okT := mm.ModuleFor(toPath)
		if !okF || !okT || fromMod == toMod {
			continue
		}
		key := labels.Key(fromMod, toMod)
		if _, ok := wanted[key]; !ok {
			continue
		}
		items[key] = append(items[key], fromPath+"\x00"+toPath+"\x00"+string(e.Kind))
	}

	return hashPairEvidence(items)
}

// PairEvidenceFromRelationships computes the same label evidence hashes as
// PairEvidence from the classified relationship contract.
func PairEvidenceFromRelationships(set relationship.Set, wanted map[string]struct{}) map[string]string {
	if len(wanted) == 0 {
		return nil
	}
	items := map[string][]string{}
	for _, edge := range set.DependencyEdges() {
		if edge.FromModule == "" || edge.ToModule == "" || edge.FromModule == edge.ToModule {
			continue
		}
		key := labels.Key(edge.FromModule, edge.ToModule)
		if _, ok := wanted[key]; !ok {
			continue
		}
		items[key] = append(items[key], edge.FromPath+"\x00"+edge.ToPath+"\x00"+edge.Kind)
	}
	return hashPairEvidence(items)
}

func hashPairEvidence(items map[string][]string) map[string]string {
	evidence := make(map[string]string, len(items))
	for key, its := range items {
		evidence[key] = labels.HashItems(its)
	}
	return evidence
}
