package py

import (
	"fmt"
	"strings"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
)

// QueryEdge builds the facts `archfit policy can-import` judges: one import
// of target by the Python file from, spelled exactly as the extractor spells a
// grimp edge — dotted module node to dotted module node, the same edge kind,
// located at the importing file unless an exclusion glob matches it — so the
// rule pass keys its findings as it does on the extracted edge. target is a
// dotted module or a .py file. The dotted form strips a src/ prefix, as the
// module-key convention does; a package under another custom root is spelled
// differently by grimp. An importer outside the packages grimp builds wraps
// ErrNotExtracted; a target outside them (installed, stdlib, or uninstalled)
// wraps ErrNotDecidable. It runs no tool.
func QueryEdge(root string, cfg evidenceports.ExtractConfig, from, target string) (graph.Facts, error) {
	importer := moduleKey(from)
	if importer == "" {
		return graph.Facts{}, fmt.Errorf("python: %q is not a .py file", from)
	}
	imported := target
	if strings.HasSuffix(target, ".py") {
		imported = moduleKey(target)
	}
	if imported == "" {
		return graph.Facts{}, fmt.Errorf("python: %q is not a dotted module or a .py file", target)
	}
	pkgs := grimpPackages(root, cfg.PyPackage)
	if !builtBy(pkgs, importer) {
		return graph.Facts{}, fmt.Errorf("python: %s is outside the packages grimp builds (%s): %w",
			importer, strings.Join(pkgs, ", "), evidenceports.ErrNotExtracted)
	}
	if !builtBy(pkgs, imported) {
		return graph.Facts{}, fmt.Errorf("python: %s is outside the packages grimp builds (%s); grimp drops an installed or stdlib import and spells an uninstalled one external: %w",
			imported, strings.Join(pkgs, ", "), evidenceports.ErrNotDecidable)
	}
	// The extractor locates an import at the importing file unless an
	// exclusion glob matches it (pythonSourceLocations).
	var locations []graph.Location
	if !pythonLocationExcluded(from, cfg.Exclusions) {
		locations = []graph.Location{{File: from}}
	}
	return graph.Facts{
		Language: langPython,
		Nodes: []graph.Node{
			{Kind: graph.NodeKindModule, Path: importer, Language: graph.LangPython},
			{Kind: graph.NodeKindModule, Path: imported, Language: graph.LangPython},
		},
		Edges: []graph.Edge{{
			From:       "module:" + importer,
			To:         "module:" + imported,
			Kind:       ImportEdgeKind(cfg.Internal, imported),
			Language:   langPython,
			Confidence: "high",
			Locations:  locations,
		}},
	}, nil
}

// builtBy reports whether a dotted module belongs to one of the packages
// grimp builds.
func builtBy(pkgs []string, dotted string) bool {
	for _, pkg := range pkgs {
		if dotted == pkg || strings.HasPrefix(dotted, pkg+".") {
			return true
		}
	}
	return false
}

func moduleKey(file string) string {
	return graph.BuiltinConventions.Lookup(graph.LangPython).FileToModuleKey(file)
}
