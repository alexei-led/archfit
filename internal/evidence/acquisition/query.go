package acquisition

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/alexei-led/archfit/internal/application"
	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/extract/loc"
	"github.com/alexei-led/archfit/internal/extract/registry"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// Query is the tool-free acquisition behind `archfit policy`. It spells one
// queried import the way the language's extractor spells it, drops it where
// the extractor drops it, and classifies the importing file with the
// predicates Acquire uses. It runs no analyzer, walks no tree, and never opens
// the fact cache.
type Query struct {
	// Root is the canonical analysis root, resolved as check resolves it.
	Root    string
	Options RunOptions
}

var _ application.EdgeQuery = Query{}

// NewQuery resolves the analysis root exactly as check does — --root, else
// the git toplevel, else the config directory — so the query spells nodes
// relative to the same root. The resolution asks git for the toplevel; it is
// the one process the query starts.
func NewQuery(ctx context.Context, configPath, root string, opts RunOptions, runner toolrun.Runner) (Query, error) {
	sc := opts.Scope
	sc.WorkDir, sc.Root, sc.Full = scanDir(root, filepath.Dir(configPath)), root, true
	resolved, err := scope.Resolve(ctx, sc, gitResolver{workDir: sc.WorkDir, runner: runner})
	if err != nil {
		return Query{}, err
	}
	return Query{Root: resolved.Root, Options: opts}, nil
}

// Edge implements application.EdgeQuery.
func (q Query) Edge(_ context.Context, from, target string) (application.QueriedEdge, error) {
	from = q.Rel(from)
	language := registry.LanguageForFile(from)
	out := application.QueriedEdge{From: from, Language: language}
	if language != "" && q.Options.Extractors[language].Mode == evidenceports.ModeOff {
		out.NotExtracted = language + " analysis is switched off, so check never reads this import"
		return out, nil
	}
	facts, err := registry.QueryEdge(q.Root, q.Options.Extractors, from, q.Rel(target))
	switch {
	case errors.Is(err, registry.ErrNoQueryEdge):
		out.Undecided = err.Error()
		return out, nil
	case errors.Is(err, registry.ErrNotExtracted):
		out.NotExtracted = err.Error()
		return out, nil
	case err != nil:
		return application.QueriedEdge{}, err
	}
	out.Graph = graph.Build([]graph.Facts{facts})
	out.Production = map[string]bool{
		from: !outOfDeclaredScope(from, q.Options.Exclusions, disabledLanguages(q.Options.Coverage)) &&
			fileclass.IsProduction(loc.ClassifyFile(q.Root, from, language, q.Options.Acquisition.FileClass)),
	}
	return out, nil
}

// OutOfScope reports whether a root-relative file is outside the declared
// analysis scope: an effective exclude: glob matches it, or its language is
// switched off. Rule scope and metrics leave it out; dependency-cruiser and
// grimp still extract its imports.
func (q Query) OutOfScope(file string) bool {
	return outOfDeclaredScope(file, q.Options.Exclusions, disabledLanguages(q.Options.Coverage))
}

// Rel spells a path the way the report does: root-relative with forward
// slashes. A relative path is taken as already relative to the root; an
// absolute one is made relative to it, through symlinks when the plain
// spelling falls outside.
func (q Query) Rel(path string) string {
	if filepath.IsAbs(path) {
		if rel, ok := relWithin(q.Root, path); ok {
			path = rel
		} else if resolved, err := filepath.EvalSymlinks(path); err == nil {
			if rel, ok := relWithin(q.Root, resolved); ok {
				path = rel
			}
		}
	}
	return filepath.ToSlash(filepath.Clean(path))
}

func relWithin(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}
