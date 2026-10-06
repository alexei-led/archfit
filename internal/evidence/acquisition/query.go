package acquisition

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/extract/loc"
	"github.com/alexei-led/archfit/internal/extract/registry"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/model/graph"
)

// Query is the tool-free acquisition behind `archfit policy`. It spells one
// queried import the way the language's extractor spells it and decides the
// importing file's declared scope and production class with the predicates
// Acquire uses. It runs no tool, walks no tree, and never opens the fact cache.
type Query struct {
	// Root is the analysis root: --root, else the config directory.
	Root    string
	Options RunOptions
}

var _ application.EdgeQuery = Query{}

// Edge implements application.EdgeQuery.
func (q Query) Edge(_ context.Context, from, target string) (application.QueriedEdge, error) {
	from = q.Rel(from)
	language := registry.LanguageForFile(from)
	out := application.QueriedEdge{From: from, Language: language}
	if q.OutOfScope(from) {
		out.OutOfScope = true
		return out, nil
	}
	target = q.Rel(target)
	facts, err := registry.QueryEdge(q.Root, q.Options.Extractors, from, target)
	if errors.Is(err, registry.ErrNoQueryEdge) {
		out.Undecided = err.Error()
		return out, nil
	}
	if err != nil {
		return application.QueriedEdge{}, err
	}
	out.Graph = graph.Build([]graph.Facts{facts})
	out.Production = map[string]bool{
		from: fileclass.IsProduction(loc.ClassifyFile(q.Root, from, language, q.Options.Acquisition.FileClass)),
	}
	return out, nil
}

// OutOfScope reports whether a ScanRoot-relative file is outside the declared
// analysis scope: an effective exclude: glob matches it, or its language is
// switched off. check never sees an import from such a file.
func (q Query) OutOfScope(file string) bool {
	return outOfDeclaredScope(file, q.Options.Exclusions, disabledLanguages(q.Options.Coverage))
}

// Rel spells a path the way the report does: ScanRoot-relative with forward
// slashes. A relative path is taken as already relative to the root.
func (q Query) Rel(path string) string {
	if filepath.IsAbs(path) {
		if rel, err := filepath.Rel(q.Root, path); err == nil {
			path = rel
		}
	}
	return filepath.ToSlash(filepath.Clean(path))
}
