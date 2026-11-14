package tsimports

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// ToolName is the TypeScript parser's report coverage name.
const ToolName = "tsimports"

// Extractor projects the check's resolved imports into report facts.
type Extractor struct {
	runner toolrun.Runner
	cfg    evidenceports.ExtractConfig
}

// New builds the TypeScript report extractor.
func New(runner toolrun.Runner, cfg evidenceports.ExtractConfig) *Extractor {
	return &Extractor{runner: runner, cfg: cfg}
}

// Name returns the language ID.
func (e *Extractor) Name() string { return graph.LangTypeScript }

// CoverageTool returns the report producer name.
func (e *Extractor) CoverageTool() string { return ToolName }

// Applicable uses the parser's own source inventory.
func Applicable(root string, exclude []string) bool {
	files, err := sourceFiles(os.DirFS(root), exclude)
	return err != nil || len(files) > 0
}

// Extract projects imports and discloses unread sources or resolution inputs.
func (e *Extractor) Extract(ctx context.Context, s scope.Scope) (graph.Facts, evidence.Coverage, error) {
	cov := evidence.Coverage{Tool: ToolName, Status: evidence.StatusAbsent}
	if e.cfg.Mode == evidenceports.ModeOff {
		return graph.Facts{}, cov, nil
	}
	files, inventoryErr := sourceFiles(os.DirFS(s.Root), e.cfg.Exclusions)
	cov.FilesApplicable = len(files)
	if inventoryErr == nil && len(files) == 0 {
		if e.cfg.Mode == evidenceports.ModeOn {
			return graph.Facts{}, cov, errors.New("tsimports: no TypeScript or JavaScript sources found")
		}
		return graph.Facts{}, cov, nil
	}
	parsed, err := Extract(ctx, e.runner, s.Root, Config{Exclude: e.cfg.Exclusions})
	if err != nil {
		if ctx.Err() != nil {
			return graph.Facts{}, cov, ctx.Err()
		}
		cov.Status, cov.Reason = evidence.StatusPartial, err.Error()
		if e.cfg.Mode == evidenceports.ModeOn {
			return graph.Facts{}, cov, err
		}
		return graph.Facts{}, cov, nil
	}
	cov.Status = evidence.StatusOK
	cov.FilesApplicable = len(parsed.Sources)
	out, err := e.runner.Run(ctx, toolrun.ToolCmd{Name: astGrep, Args: []string{"--version"}, WorkDir: s.Root})
	if ctx.Err() != nil {
		return graph.Facts{}, cov, ctx.Err()
	}
	if err == nil && out.ExitCode != 0 {
		err = fmt.Errorf("tsimports: ast-grep version: %s", out.Stderr)
	}
	if err != nil {
		cov.Status, cov.Reason = evidence.StatusPartial, err.Error()
		if e.cfg.Mode == evidenceports.ModeOn {
			return graph.Facts{}, cov, err
		}
		return graph.Facts{}, cov, nil
	}
	cov.FilesSeen = len(parsed.Sources)
	cov.Version = strings.TrimSpace(string(out.Stdout))
	facts := graph.Facts{Language: graph.LangTypeScript}
	for _, file := range parsed.Sources {
		facts.Nodes = append(facts.Nodes, graph.Node{Kind: graph.NodeKindFile, Path: file, Language: graph.LangTypeScript})
	}
	var gaps []string
	for _, imp := range parsed.Imports {
		target, confidence := "file:"+imp.Target, "high"
		kind := importEdgeKind(e.cfg.Internal, imp.Target)
		switch imp.Kind {
		case KindExternal:
			target, confidence, kind = "external:"+imp.Specifier, "low", graph.EdgeKindImports
		case KindUnknown:
			target, confidence, kind = "external:"+imp.Specifier, "low", graph.EdgeKindImports
			gaps = append(gaps, fmt.Sprintf("%s:%d: %s", imp.File, imp.Line, imp.Specifier))
		}
		if imp.Kind == KindFile {
			facts.Nodes = append(facts.Nodes, graph.Node{Kind: graph.NodeKindFile, Path: imp.Target, Language: graph.LangTypeScript})
		} else {
			facts.Nodes = append(facts.Nodes, graph.Node{Kind: graph.NodeKindExternal, Path: imp.Specifier, Language: graph.LangTypeScript})
		}
		facts.Edges = append(facts.Edges, graph.Edge{
			From: "file:" + imp.File, To: target, Kind: kind, Language: graph.LangTypeScript, Confidence: confidence,
			Locations:        []graph.Location{{File: imp.File, Line: imp.Line}},
			ConnascenceHints: []graph.ConnascenceHint{{Kind: graph.ConnascenceName, Source: ToolName, Detail: "import"}},
		})
	}
	for _, item := range parsed.Unknown {
		gaps = append(gaps, fmt.Sprintf("%s: %s: %s", item.File, item.Specifier, item.Reason))
	}
	cov.SpecifiersSeen = len(parsed.Imports)
	if len(gaps) > 0 {
		cov.Status, cov.Unresolved, cov.UnresolvedInputsMissing = evidence.StatusPartial, len(gaps), len(gaps)
		cov.Reason = "unresolved TypeScript imports or configuration: " + strings.Join(gaps, "; ")
		facts.Unresolved = len(gaps)
	}
	return facts, cov, nil
}

// QueryEdge spells a prospective file-to-file import without running a parser.
func QueryEdge(_ string, cfg evidenceports.ExtractConfig, from, target string) (graph.Facts, error) {
	if !slices.Contains(graph.TypeScriptSourceExtensions(), path.Ext(from)) || !slices.Contains(graph.TypeScriptSourceExtensions(), path.Ext(target)) {
		return graph.Facts{}, fmt.Errorf("typescript: %q and %q must both be source files", from, target)
	}
	return graph.Facts{Language: graph.LangTypeScript, Nodes: []graph.Node{
		{Kind: graph.NodeKindFile, Path: from, Language: graph.LangTypeScript},
		{Kind: graph.NodeKindFile, Path: target, Language: graph.LangTypeScript},
	}, Edges: []graph.Edge{{
		From: "file:" + from, To: "file:" + target, Kind: importEdgeKind(cfg.Internal, target),
		Language: graph.LangTypeScript, Confidence: "high",
	}}}, nil
}

func importEdgeKind(internal []string, target string) graph.EdgeKind {
	for _, pattern := range internal {
		if matched, _ := doublestar.Match(pattern, target); matched {
			return graph.EdgeKindUsesInternal
		}
	}
	return graph.EdgeKindImports
}
