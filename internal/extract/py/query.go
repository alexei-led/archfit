package py

import (
	"fmt"
	"strings"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/model/graph"
)

// QueryEdge builds the facts `archfit policy can-import` judges: one import
// of target by the Python file from, spelled exactly as the extractor spells a
// grimp edge — dotted module node to dotted module node, the same edge kind,
// located at the importing file — so the rule pass keys its findings as it
// does on the extracted edge. target is a dotted module or a .py file. The
// dotted form strips a src/ prefix, as the module-key convention does; a
// package under another custom root is spelled differently by grimp. It runs
// no tool.
func QueryEdge(_ string, cfg evidenceports.ExtractConfig, from, target string) (graph.Facts, error) {
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
			Locations:  []graph.Location{{File: from}},
		}},
	}, nil
}

func moduleKey(file string) string {
	return graph.BuiltinConventions.Lookup(graph.LangPython).FileToModuleKey(file)
}
