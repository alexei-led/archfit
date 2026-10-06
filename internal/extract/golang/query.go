package golang

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/model/graph"
)

// QueryEdge builds the facts `archfit policy can-import` judges: one import
// of target by the Go file from, spelled exactly as the extractor spells it —
// a file node to a ScanRoot-relative package node, with the same edge kind —
// so the rule pass keys its findings as it does on the extracted edge. target
// is a ScanRoot-relative package dir or an import path; an import path under
// a member module is stripped to its dir, any other is kept (an external
// package). It reads the members' go.mod files and runs no Go tool.
func QueryEdge(scanRoot string, cfg evidenceports.ExtractConfig, from, target string) (graph.Facts, error) {
	if !strings.HasSuffix(from, ".go") {
		return graph.Facts{}, fmt.Errorf("go: %q is not a .go file", from)
	}
	entries, err := memberModules(scanRoot, cfg)
	if err != nil {
		return graph.Facts{}, err
	}
	to := stripModulePath(entries, strings.TrimSuffix(target, "/"))
	return graph.Facts{
		Language: graph.LangGo,
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: from, Language: graph.LangGo},
			{Kind: graph.NodeKindPackage, Path: to, Language: graph.LangGo},
		},
		Edges: []graph.Edge{{
			From:       graph.Node{Kind: graph.NodeKindFile, Path: from}.ID(),
			To:         graph.Node{Kind: graph.NodeKindPackage, Path: to}.ID(),
			Kind:       ImportEdgeKind(to),
			Language:   graph.LangGo,
			Confidence: "high",
			Locations:  []graph.Location{{File: from}},
		}},
	}, nil
}

// memberModules reads the module path of every member the extractor loads,
// with its ScanRoot-relative dir, sorted for stripModulePath.
func memberModules(scanRoot string, cfg evidenceports.ExtractConfig) ([]modEntry, error) {
	members, err := AnalysableMembers(scanRoot, cfg.Exclusions, cfg.GoModuleInclude, cfg.GoModuleExclude)
	if err != nil {
		return nil, err
	}
	root, err := canonicalDir(scanRoot)
	if err != nil {
		return nil, err
	}
	var entries []modEntry
	for _, dir := range members.Dirs {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod")) // #nosec G304 -- member dir comes from go.mod discovery
		if err != nil {
			continue
		}
		path := modfile.ModulePath(data)
		member, err := canonicalDir(dir)
		if path == "" || err != nil {
			continue
		}
		rel, err := filepath.Rel(root, member)
		if err != nil {
			continue
		}
		entries = append(entries, modEntry{path: path, relDir: filepath.ToSlash(rel)})
	}
	sortModEntries(entries)
	return entries, nil
}

// canonicalDir resolves dir to an absolute path without symlinks, so a member
// found under a symlinked spelling of the scan root still gets its relative dir.
func canonicalDir(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}
