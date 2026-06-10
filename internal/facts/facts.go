// Package facts assembles per-file structural facts from already-collected data
// (symbol graph, file LOC, co-change history). The output is a neutral evidence
// block — no risk labels, no rankings, no gates. Ranking and judgment are the
// Tranche-2 LLM's job.
package facts

import (
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/symbol"
)

// maxCoChangePartners is the maximum number of co-change partners kept per file.
const maxCoChangePartners = 5

// FileFact holds per-file structural facts derived from collected data.
//
// File and Module are identical at this level of granularity: both hold the
// dotted/slash-path module key as it appears in symbol.Graph.Module (e.g.
// "internal/a"). Future tasks may add a BlastRadius field (Task 5, gitnexus).
type FileFact struct {
	// File is the module key (slash-path package path from symbol.Graph.Module).
	File string
	// Module mirrors File — same key, kept separate to support future enrichment
	// (e.g. a display path distinct from the graph key).
	Module string
	// InboundModuleFanIn is the count of distinct OTHER modules whose symbols
	// reference at least one symbol in this file's module. High values indicate
	// this file is widely imported. Config modules score high here too — the LLM
	// distinguishes read-only config from mutable state.
	InboundModuleFanIn int
	// OutboundDestinations is the count of distinct destination modules this
	// file's symbols reference across module boundaries. Computed at raw module
	// granularity — no parent-package collapse. A high value indicates a low-
	// cohesion grab-bag (e.g. directory_callbacks ~ 46 destinations in ccgram).
	OutboundDestinations int
	// LOC is the total source lines of code for all files whose repo-relative
	// slash-path starts with this module's path prefix. Summed from fileLOC.
	//
	// Join note: symbol.Graph.Module values are slash-path package dirs
	// (e.g. "internal/a"); fileLOC keys are slash-path file paths
	// (e.g. "internal/a/a.go"). We sum all fileLOC entries whose key has the
	// module path as a path-component prefix. Limitation: if a fileLOC key is
	// an absolute OS path rather than a repo-relative slash path, the join will
	// miss and LOC will be 0 — document the mismatch in the calling site.
	LOC int
	// CoChangePartners lists the file paths most frequently co-changed with this
	// module's files, sorted by commit count descending then path ascending, up
	// to maxCoChangePartners entries.
	//
	// Join note: coChange keys are repo-relative slash-path file pairs from git
	// log. We match coChange entries where either pair element has this module's
	// path as a prefix. If the module path format diverges from the git file
	// paths, the list will be empty — no fabrication.
	CoChangePartners []string
}

// Build assembles one FileFact per distinct file/module in g, using fileLOC for
// line counts and coChange for co-change partners. Returns an empty slice (never
// nil) when g is empty — no panic, no false zeros.
//
// The returned slice is sorted by File (total order, deterministic). CoChange
// partner lists are sorted by commit count descending, then partner path
// ascending (deterministic tie-break).
func Build(g symbol.Graph, fileLOC map[string]int, coChange map[[2]string]int) []FileFact {
	if g.Empty() {
		return []FileFact{}
	}

	// Collect the distinct module keys that appear in the graph.
	moduleSet := make(map[string]struct{}, len(g.Module))
	for _, mod := range g.Module {
		if mod != "" {
			moduleSet[mod] = struct{}{}
		}
	}
	if len(moduleSet) == 0 {
		return []FileFact{}
	}

	// Compute inbound fan-in and outbound destinations in one pass over Refs.
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
			// outbound: fromMod -> toMod
			if outboundDests[fromMod] == nil {
				outboundDests[fromMod] = make(map[string]struct{})
			}
			outboundDests[fromMod][toMod] = struct{}{}

			// inbound: toMod <- fromMod
			if inboundSources[toMod] == nil {
				inboundSources[toMod] = make(map[string]struct{})
			}
			inboundSources[toMod][fromMod] = struct{}{}
		}
	}

	// Build LOC index: sum fileLOC entries whose slash-path key is under the
	// module dir. We match on path-component prefix to avoid "internal/ab"
	// accidentally matching "internal/a".
	locByModule := make(map[string]int, len(moduleSet))
	for filePath, lines := range fileLOC {
		for mod := range moduleSet {
			if hasModulePrefix(filePath, mod) {
				locByModule[mod] += lines
				break // a file belongs to at most one module prefix match
			}
		}
	}

	// Build co-change index: for each module, collect (partner, count) pairs
	// from coChange entries where either side of the pair is under this module.
	//
	// partnerCounts[thisMod][partnerPath] = commit count
	type partnerCount struct {
		path  string
		count int
	}
	// Collect raw partner counts per module.
	rawPartners := make(map[string]map[string]int)
	for pair, count := range coChange {
		a, b := pair[0], pair[1]
		aMod := moduleForPath(a, moduleSet)
		bMod := moduleForPath(b, moduleSet)
		if aMod == "" && bMod == "" {
			continue
		}
		if aMod != "" {
			if rawPartners[aMod] == nil {
				rawPartners[aMod] = make(map[string]int)
			}
			rawPartners[aMod][b] += count
		}
		if bMod != "" {
			if rawPartners[bMod] == nil {
				rawPartners[bMod] = make(map[string]int)
			}
			rawPartners[bMod][a] += count
		}
	}

	// Assemble sorted top-N partner lists.
	coChangePartners := make(map[string][]string, len(rawPartners))
	for mod, partners := range rawPartners {
		pcs := make([]partnerCount, 0, len(partners))
		for path, cnt := range partners {
			pcs = append(pcs, partnerCount{path: path, count: cnt})
		}
		sort.Slice(pcs, func(i, j int) bool {
			if pcs[i].count != pcs[j].count {
				return pcs[i].count > pcs[j].count // highest count first
			}
			return pcs[i].path < pcs[j].path // alpha tie-break
		})
		top := pcs
		if len(top) > maxCoChangePartners {
			top = top[:maxCoChangePartners]
		}
		names := make([]string, len(top))
		for i, pc := range top {
			names[i] = pc.path
		}
		coChangePartners[mod] = names
	}

	// Assemble one FileFact per module key.
	facts := make([]FileFact, 0, len(moduleSet))
	for mod := range moduleSet {
		ff := FileFact{
			File:                 mod,
			Module:               mod,
			InboundModuleFanIn:   len(inboundSources[mod]),
			OutboundDestinations: len(outboundDests[mod]),
			LOC:                  locByModule[mod],
			CoChangePartners:     coChangePartners[mod],
		}
		if ff.CoChangePartners == nil {
			ff.CoChangePartners = []string{}
		}
		facts = append(facts, ff)
	}

	// Sort by File for deterministic total order.
	sort.Slice(facts, func(i, j int) bool {
		return facts[i].File < facts[j].File
	})
	return facts
}

// hasModulePrefix reports whether filePath (slash-separated) is a file under
// the directory named by modPath. It matches on path-component boundaries to
// prevent "internal/ab/foo.go" matching module "internal/a".
//
// Examples:
//
//	hasModulePrefix("internal/a/a.go", "internal/a")   → true
//	hasModulePrefix("internal/ab/x.go", "internal/a")  → false
//	hasModulePrefix("internal/a", "internal/a")         → true  (exact match)
func hasModulePrefix(filePath, modPath string) bool {
	if filePath == modPath {
		return true
	}
	return strings.HasPrefix(filePath, modPath+"/")
}

// moduleForPath returns the module key (from moduleSet) that owns filePath via
// path-component prefix match, or "" if none matches. When multiple modules
// could match (e.g. nested packages), the longest prefix wins.
func moduleForPath(filePath string, moduleSet map[string]struct{}) string {
	best := ""
	for mod := range moduleSet {
		if hasModulePrefix(filePath, mod) {
			if len(mod) > len(best) {
				best = mod
			}
		}
	}
	return best
}
