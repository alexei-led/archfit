package py

import (
	"errors"
	"os"
	"path"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/v3/internal/model/graph"
)

func pythonSourceLocations(root string, h helperOutput, exclusions []string) map[string]string {
	wanted := make(map[string]struct{})
	for _, edge := range h.Edges {
		wanted[edge.Importer] = struct{}{}
	}
	for _, edge := range h.UnresolvedImports {
		wanted[edge.Importer] = struct{}{}
	}
	resolved := make(map[string]string)
	convention := graph.BuiltinConventions.Lookup(graph.LangPython)
	for module := range wanted {
		file := ""
		ambiguous := false
		for _, candidate := range convention.ModuleFileCandidates(module) {
			if !filepath.IsLocal(filepath.FromSlash(candidate)) || path.Ext(candidate) != ".py" || pythonLocationExcluded(candidate, exclusions) {
				continue
			}
			info, err := os.Stat(filepath.Join(root, filepath.FromSlash(candidate)))
			if err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					ambiguous = true
				}
				continue
			}
			if !info.Mode().IsRegular() {
				continue
			}
			if file != "" {
				ambiguous = true
				break
			}
			file = candidate
		}
		if file != "" && !ambiguous {
			resolved[module] = file
		}
	}
	return resolved
}

func pythonLocationExcluded(file string, exclusions []string) bool {
	for candidate := file; candidate != "."; candidate = path.Dir(candidate) {
		for _, pattern := range exclusions {
			if matched, _ := doublestar.Match(pattern, candidate); matched {
				return true
			}
		}
	}
	return false
}

func pythonEdgeLocations(files map[string]string, edge helperEdge) []graph.Location {
	if file := files[edge.Importer]; file != "" {
		return []graph.Location{{File: file, Line: edge.Line}}
	}
	return nil
}
