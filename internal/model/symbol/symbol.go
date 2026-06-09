// Package symbol defines the domain type for per-symbol metadata extracted
// from a SCIP (or equivalent) index.
package symbol

// Graph holds per-symbol metadata extracted from a SCIP index.
//
//   - Module maps each symbol to its owning module/package path.
//   - FanIn counts the number of distinct documents that reference each symbol
//     (excluding the defining document).
//   - Refs is a symbol-to-symbol adjacency for cross-module reference edges:
//     Refs[fromSymbol] is the set of symbols that fromSymbol references in a
//     different module.
//
// Graph is empty (all maps nil/zero-length) when SCIP is off or the indexer is
// absent; metrics that require it must call naResult in that case.
type Graph struct {
	Module map[string]string
	FanIn  map[string]int
	Refs   map[string]map[string]struct{}
}

// Empty reports whether g contains no symbol data.
func (g Graph) Empty() bool {
	return len(g.Module) == 0 && len(g.FanIn) == 0 && len(g.Refs) == 0
}
