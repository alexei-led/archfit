// Package facts assembles per-module structural facts from already-collected
// data (symbol graph, file LOC, co-change history, optional SCIP dependant count).
// The output is a neutral evidence block — no risk labels, no rankings, no
// gates. Ranking and judgment are the Tranche-2 LLM's job.
package facts

import (
	"sort"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/symbol"
)

// maxCoChangePartners is the maximum number of co-change partners kept per module.
const maxCoChangePartners = 5

// Build assembles one diagnostic.FileFact per distinct module in g.
//
// Inbound module fan-in and outbound distinct-destination counts come from
// g.Refs + g.Module. File attribution (Files, LOC, CoChangePartners) joins
// exactly through g.Path — the repo-relative defining-document path per symbol —
// against the file-keyed fileLOC and coChange maps. When g.Path is absent the
// file-derived facts stay empty (no heuristic prefix joins, no fabrication).
//
// symbolDependants (repo-relative file path → distinct dependant-file count,
// derived from the SCIP symbol graph) enriches matching facts when non-empty:
// a module's count is the MAX over its defining files' counts. Uncovered
// modules keep a nil SymbolDependants.
//
// Returns an empty slice (never nil) when g is empty — no panic, no false
// zeros. The result is sorted by Module; all nested lists carry a total order
// (Files ascending; CoChangePartners by count descending then path ascending).
func Build(
	g symbol.Graph,
	fileLOC map[string]int,
	coChange map[[2]string]int,
	symbolDependants map[string]int,
) []diagnostic.FileFact {
	if g.Empty() {
		return []diagnostic.FileFact{}
	}

	// Distinct module keys and per-module defining-file sets.
	moduleSet := make(map[string]struct{}, len(g.Module))
	moduleFiles := make(map[string]map[string]struct{})
	for sym, mod := range g.Module {
		if mod == "" {
			continue
		}
		moduleSet[mod] = struct{}{}
		if p := g.Path[sym]; p != "" {
			if moduleFiles[mod] == nil {
				moduleFiles[mod] = make(map[string]struct{})
			}
			moduleFiles[mod][p] = struct{}{}
		}
	}
	if len(moduleSet) == 0 {
		return []diagnostic.FileFact{}
	}

	// Inbound fan-in and outbound destinations in one pass over Refs.
	//
	// inboundSources[toMod] = set of distinct fromMod that reference toMod.
	// outboundDests[fromMod] = set of distinct toMod that fromMod references.
	inboundSources := make(map[string]map[string]struct{})
	outboundDests := make(map[string]map[string]struct{})
	for from, tos := range g.Refs {
		fromMod := g.Module[from]
		if fromMod == "" {
			continue
		}
		for to := range tos {
			if from == to {
				continue
			}
			toMod := g.Module[to]
			if toMod == "" || toMod == fromMod {
				continue
			}
			if outboundDests[fromMod] == nil {
				outboundDests[fromMod] = make(map[string]struct{})
			}
			outboundDests[fromMod][toMod] = struct{}{}

			if inboundSources[toMod] == nil {
				inboundSources[toMod] = make(map[string]struct{})
			}
			inboundSources[toMod][fromMod] = struct{}{}
		}
	}

	// Owner index: defining file path → module. Exact keys, no prefix logic.
	fileOwner := make(map[string]string)
	for mod, files := range moduleFiles {
		for f := range files {
			fileOwner[f] = mod
		}
	}

	// Co-change partners: for each module, count commits shared with files
	// OUTSIDE the module (an own-file partner is trivially co-changed and
	// carries no cross-module signal).
	rawPartners := make(map[string]map[string]int)
	addPartner := func(mod, partner string, count int) {
		if fileOwner[partner] == mod {
			return
		}
		if rawPartners[mod] == nil {
			rawPartners[mod] = make(map[string]int)
		}
		rawPartners[mod][partner] += count
	}
	for pair, count := range coChange {
		a, b := pair[0], pair[1]
		if mod := fileOwner[a]; mod != "" {
			addPartner(mod, b, count)
		}
		if mod := fileOwner[b]; mod != "" {
			addPartner(mod, a, count)
		}
	}

	// Assemble one FileFact per module key.
	out := make([]diagnostic.FileFact, 0, len(moduleSet))
	for mod := range moduleSet {
		ff := diagnostic.FileFact{
			Module:               mod,
			Files:                sortedKeys(moduleFiles[mod]),
			InboundModuleFanIn:   len(inboundSources[mod]),
			OutboundDestinations: len(outboundDests[mod]),
			CoChangePartners:     topPartners(rawPartners[mod]),
		}
		for _, f := range ff.Files {
			ff.LOC += fileLOC[f]
		}
		covered := false
		impact := 0
		for _, f := range ff.Files {
			if v, ok := symbolDependants[f]; ok {
				covered = true
				if v > impact {
					impact = v
				}
			}
		}
		if covered {
			ff.SymbolDependants = &impact
		}
		out = append(out, ff)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Module < out[j].Module })
	return out
}

// sortedKeys returns the keys of set in ascending order; empty (non-nil) slice
// for an empty/nil set.
func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// topPartners returns up to maxCoChangePartners partner paths sorted by commit
// count descending, then path ascending. Empty (non-nil) slice when partners
// is empty.
func topPartners(partners map[string]int) []string {
	type partnerCount struct {
		path  string
		count int
	}
	pcs := make([]partnerCount, 0, len(partners))
	for path, cnt := range partners {
		pcs = append(pcs, partnerCount{path: path, count: cnt})
	}
	sort.Slice(pcs, func(i, j int) bool {
		if pcs[i].count != pcs[j].count {
			return pcs[i].count > pcs[j].count
		}
		return pcs[i].path < pcs[j].path
	})
	if len(pcs) > maxCoChangePartners {
		pcs = pcs[:maxCoChangePartners]
	}
	names := make([]string, len(pcs))
	for i, pc := range pcs {
		names[i] = pc.path
	}
	return names
}
