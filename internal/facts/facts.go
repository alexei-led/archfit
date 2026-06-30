// Package facts assembles per-module structural facts from already-collected
// data (symbol graph, file LOC). The output is a neutral evidence block — no
// risk labels, no rankings, no gates. Ranking and judgment are the
// Tranche-2 LLM's job.
package facts

import (
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/symbol"
)

// Build assembles one diagnostic.FileFact per distinct module in g.
//
// Inbound module fan-in and outbound distinct-destination counts come from
// g.Refs + g.Module. File attribution (Files, LOC) joins exactly through
// g.Path — the repo-relative defining-document path per symbol — against the
// file-keyed fileLOC map. When g.Path is absent the file-derived facts stay
// empty (no heuristic prefix joins, no fabrication).
//
// Returns an empty slice (never nil) when g is empty — no panic, no false
// zeros. The result is sorted by Module; the Files list is sorted ascending.
func Build(
	g symbol.Graph,
	fileLOC map[string]int,
) []diagnostic.FileFact {
	if g.Empty() {
		return []diagnostic.FileFact{}
	}

	// Distinct module keys and per-module defining-file sets.
	moduleSet := make(map[string]struct{}, len(g.Module))
	moduleFiles := make(map[string]map[string]struct{})
	for sym, mod := range g.Module {
		// Skip empty and scip-go test-binary pseudo-modules ("<pkg>.test"): they
		// carry loc 0 and a single file pointing into the go-build cache (outside
		// the repo), so they only pollute file_facts (F10). Their fan-in is already
		// excluded below.
		if mod == "" || strings.HasSuffix(mod, ".test") {
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
		// scip-go indexes test binaries as "<pkg>.test" modules. Exclude them
		// from inbound fan-in so the count matches blast_radius, which uses the
		// go/packages import graph loaded without Tests:true.
		isTestMod := strings.HasSuffix(fromMod, ".test")
		for to := range tos {
			if from == to {
				continue
			}
			toMod := g.Module[to]
			if toMod == "" || toMod == fromMod {
				continue
			}
			if !isTestMod {
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
	}

	// Assemble one FileFact per module key.
	out := make([]diagnostic.FileFact, 0, len(moduleSet))
	for mod := range moduleSet {
		ff := diagnostic.FileFact{
			Module:               mod,
			Files:                sortedKeys(moduleFiles[mod]),
			InboundModuleFanIn:   len(inboundSources[mod]),
			OutboundDestinations: len(outboundDests[mod]),
		}
		for _, f := range ff.Files {
			ff.LOC += fileLOC[f]
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
