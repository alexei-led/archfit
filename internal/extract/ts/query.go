package ts

import (
	"fmt"
	"path"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
)

// QueryEdge builds the facts `archfit policy can-import` judges: one import
// of the first-party file target by the file from, both ScanRoot-relative,
// spelled exactly as the extractor spells a resolved dependency-cruiser edge —
// file node to file node, the same edge kind, and no location — so the rule
// pass keys its findings as it does on the extracted edge. It runs no tool.
func QueryEdge(_ string, cfg evidenceports.ExtractConfig, from, target string) (graph.Facts, error) {
	if !isSource(from) || !isSource(target) {
		return graph.Facts{}, fmt.Errorf("typescript: %q and %q must both be source files", from, target)
	}
	return graph.Facts{
		Language: langTS,
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: from, Language: graph.LangTypeScript},
			{Kind: graph.NodeKindFile, Path: target, Language: graph.LangTypeScript},
		},
		Edges: []graph.Edge{{
			From:       graph.Node{Kind: graph.NodeKindFile, Path: from}.ID(),
			To:         graph.Node{Kind: graph.NodeKindFile, Path: target}.ID(),
			Kind:       ImportEdgeKind(cfg.Internal, target),
			Language:   langTS,
			Confidence: "high",
		}},
	}, nil
}

func isSource(file string) bool {
	ext := path.Ext(file)
	for _, want := range graph.TypeScriptSourceExtensions() {
		if ext == want {
			return true
		}
	}
	return false
}
