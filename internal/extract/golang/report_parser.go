package golang

import (
	"fmt"
	"sort"

	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
)

// extractParsedImports keeps the report's dependency graph available without
// Go. The primary coverage row retains its name, but discloses the missing
// type precision separately from unknown imports and unreadable files.
func (e *GoExtractor) extractParsedImports(root string) (graph.Facts, evidence.Coverage, error) {
	parsed, err := ParseImports(root, e.cfg)
	if err != nil {
		return graph.Facts{}, evidence.Coverage{}, err
	}
	entries, err := memberModules(root, e.cfg)
	if err != nil {
		return graph.Facts{}, evidence.Coverage{}, err
	}
	facts := graph.Facts{Language: graph.LangGo}
	for _, m := range entries {
		facts.GoModules = append(facts.GoModules, graph.GoModule{Path: m.path, RelDir: m.relDir})
	}
	sort.Slice(facts.GoModules, func(i, j int) bool { return facts.GoModules[i].Path < facts.GoModules[j].Path })

	byFile := make(map[string][]importFacts)
	cov := evidence.Coverage{
		Tool: toolGoPackages, Status: statusAbsent,
		FilesSeen: len(parsed.Sources), FilesApplicable: len(parsed.Sources) + len(parsed.Unknown),
	}
	for _, imp := range parsed.Imports {
		target := imp.Path
		if imp.Kind != ImportExternal {
			target = imp.Package
		}
		byFile[imp.File] = append(byFile[imp.File], importFacts{RawPath: target, Line: imp.Line, LocFile: imp.File})
		if imp.Kind == ImportUnresolved {
			cov.UnresolvedInputsMissing++
			cov.Details = append(cov.Details, fmt.Sprintf("%s:%d: no Go package is read for %s", imp.File, imp.Line, imp.Path))
		}
	}
	for _, u := range parsed.Unknown {
		cov.UnresolvedInputsMissing++
		cov.Details = append(cov.Details, u.File+": "+u.Reason)
	}
	var pkgs []packageFacts
	for _, file := range parsed.Sources {
		pkgs = append(pkgs, packageFacts{
			PkgPath: packageDir(file),
			Files:   []fileFacts{{RelFile: file, Imports: byFile[file]}},
		})
	}
	facts.Nodes, facts.Edges, _, _, _ = e.collectNodesEdges(pkgs, func(path string) string { return path }, nil, nil, nil)
	if cov.FilesApplicable > 0 {
		cov.Status = statusPartial
		cov.UnresolvedPrecisionOnly = 1
		cov.Reason = "go not installed; imports read with go/parser; go/types strength unavailable"
		if cov.UnresolvedInputsMissing > 0 {
			cov.Reason += fmt.Sprintf("; %d unknown input(s)", cov.UnresolvedInputsMissing)
		}
	}
	cov.Unresolved = cov.UnresolvedInputsMissing + cov.UnresolvedPrecisionOnly
	facts.Unresolved = cov.Unresolved
	return facts, cov, nil
}
