// Package symbol defines the domain type for per-symbol metadata extracted
// from a SCIP (or equivalent) index.
package symbol

// Graph holds per-symbol metadata extracted from a SCIP index.
//
//   - Module maps each symbol to its owning module/package path.
//   - Path maps each symbol to the repo-relative slash-path of its defining
//     document (e.g. "src/ccgram/tui/polling_state.py"). Unlike Module — which
//     is a language-shaped key (dotted for Python, package dir for Go) — Path
//     joins exactly against file-keyed data such as FileLOC and CoChange.
//   - FanIn counts the number of distinct documents that reference each symbol
//     (excluding the defining document).
//   - Refs is a symbol-to-symbol adjacency for cross-module reference edges:
//     Refs[fromSymbol] is the set of symbols that fromSymbol references in a
//     different module.
//   - IntraRefs is the same adjacency restricted to same-module edges
//     (fromSymbol and the referenced symbol share a module). It is the source
//     for the report-only cohesion (LCOM edge-density) proxy. Because SCIP
//     indexers do not populate enclosing_range, attribution is document-scoped:
//     within one document every definition is treated as the source of every
//     reference in that document, so same-document edges are over-connected.
//     The cohesion proxy therefore only trusts cross-document intra-module
//     structure (multi-document modules); single-document modules — the common
//     Python/TS shape — are not measurable. See internal/metrics/modularity/cohesion.go.
//
// Graph is empty (all maps nil/zero-length) when SCIP is off or the indexer is
// absent; metrics that require it must call naResult in that case.
type Graph struct {
	Module    map[string]string
	Path      map[string]string
	FanIn     map[string]int
	Refs      map[string]map[string]struct{}
	IntraRefs map[string]map[string]struct{}
}

// Empty reports whether g contains no symbol data.
func (g Graph) Empty() bool {
	return len(g.Module) == 0 && len(g.FanIn) == 0 && len(g.Refs) == 0
}

// DependantsFromSymbolGraph computes a per-file distinct-dependant count from
// the SCIP symbol graph. For each from→to edge in g.Refs, with fa=g.Path[from]
// and fb=g.Path[to], when fa and fb are both non-empty and fa!=fb, fa is
// counted as a distinct dependant of fb. Returns file→count or nil for an
// empty graph.
//
// Semantics: per source file F, count distinct other files whose symbols
// reference symbols defined in F. Derived in-process from the SCIP graph —
// no external tool required.
func DependantsFromSymbolGraph(g Graph) map[string]int {
	if g.Empty() {
		return nil
	}
	// dependantSet[fb] = set of distinct files fa that reference symbols in fb
	dependantSet := make(map[string]map[string]struct{})
	for from, tos := range g.Refs {
		fa := g.Path[from]
		if fa == "" {
			continue
		}
		for to := range tos {
			fb := g.Path[to]
			if fb == "" || fa == fb {
				continue
			}
			if dependantSet[fb] == nil {
				dependantSet[fb] = make(map[string]struct{})
			}
			dependantSet[fb][fa] = struct{}{}
		}
	}
	if len(dependantSet) == 0 {
		return nil
	}
	out := make(map[string]int, len(dependantSet))
	for fb, set := range dependantSet {
		out[fb] = len(set)
	}
	return out
}
