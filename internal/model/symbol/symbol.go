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
