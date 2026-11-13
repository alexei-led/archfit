package pyimports

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// ToolName is the Python parser's report coverage name.
const ToolName = "pyimports"

// Extractor projects the same resolved imports as the check into report facts.
type Extractor struct {
	runner toolrun.Runner
	cfg    evidenceports.ExtractConfig
}

// New builds the Python report extractor. It never starts Python or reads its environment.
func New(runner toolrun.Runner, cfg evidenceports.ExtractConfig) *Extractor {
	return &Extractor{runner: runner, cfg: cfg}
}

func (e *Extractor) Name() string { return graph.LangPython }

func (e *Extractor) CoverageTool() string { return ToolName }

// Applicable uses the parser's own package discovery, including configured packages.
func Applicable(root, pkg string) bool {
	read, _, err := packages(os.DirFS(root), pkg)
	return err != nil || len(read) > 0
}

func (e *Extractor) Extract(ctx context.Context, s scope.Scope) (graph.Facts, evidence.Coverage, error) {
	cov := evidence.Coverage{Tool: ToolName, Status: evidence.StatusAbsent}
	if e.cfg.Mode == evidenceports.ModeOff {
		return graph.Facts{}, cov, nil
	}
	if !Applicable(s.Root, e.cfg.PyPackage) {
		if e.cfg.Mode == evidenceports.ModeOn {
			return graph.Facts{}, cov, errors.New("pyimports: no Python package found")
		}
		return graph.Facts{}, cov, nil
	}
	parsed, err := Extract(ctx, e.runner, s.Root, Config{Package: e.cfg.PyPackage, Exclude: e.cfg.Exclusions})
	if err != nil {
		if ctx.Err() != nil {
			return graph.Facts{}, cov, ctx.Err()
		}
		cov.Reason = err.Error()
		if e.cfg.Mode == evidenceports.ModeOn {
			return graph.Facts{}, cov, err
		}
		if !errors.Is(err, ErrNoAstGrep) {
			cov.Status = evidence.StatusPartial
		}
		return graph.Facts{}, cov, nil
	}
	cov.Status = evidence.StatusOK
	cov.FilesSeen, cov.FilesApplicable = len(parsed.Sources), len(parsed.Sources)
	if len(parsed.Sources) > 0 {
		out, err := e.runner.Run(ctx, toolrun.ToolCmd{Name: astGrep, Args: []string{"--version"}, WorkDir: s.Root})
		if ctx.Err() != nil {
			return graph.Facts{}, cov, ctx.Err()
		}
		if err == nil && out.ExitCode != 0 {
			err = fmt.Errorf("pyimports: ast-grep version: %s", out.Stderr)
		}
		if err != nil {
			cov.Status, cov.Reason = evidence.StatusPartial, err.Error()
			if e.cfg.Mode == evidenceports.ModeOn {
				return graph.Facts{}, cov, err
			}
			return graph.Facts{}, cov, nil
		}
		cov.Version = strings.TrimSpace(string(out.Stdout))
	}
	facts := graph.Facts{Language: graph.LangPython}
	for _, file := range parsed.Sources {
		facts.Nodes = append(facts.Nodes, graph.Node{Kind: graph.NodeKindModule, Path: parsed.Modules[file], Language: graph.LangPython})
	}
	var gaps []string
	for _, imp := range parsed.Imports {
		target, confidence := "module:"+imp.Module, "high"
		kind := importEdgeKind(e.cfg.Internal, imp.Module)
		switch imp.Kind {
		case KindExternal:
			target, confidence, kind = "external:"+imp.Module, "low", graph.EdgeKindImports
		case KindUnknown:
			confidence = "low"
			gaps = append(gaps, fmt.Sprintf("%s:%d: %s", imp.File, imp.Line, imp.Module))
		}
		facts.Edges = append(facts.Edges, graph.Edge{
			From: "module:" + parsed.Modules[imp.File], To: target,
			Kind: kind, Language: graph.LangPython, Confidence: confidence,
			Locations:        []graph.Location{{File: imp.File, Line: imp.Line}},
			ConnascenceHints: []graph.ConnascenceHint{{Kind: graph.ConnascenceName, Source: ToolName, Detail: "dotted import"}},
		})
	}
	if len(gaps) > 0 {
		cov.Status, cov.Unresolved, cov.UnresolvedInputsMissing = evidence.StatusPartial, len(gaps), len(gaps)
		cov.Reason = "unresolved Python imports: " + strings.Join(gaps, "; ")
		facts.Unresolved = len(gaps)
	}
	return facts, cov, nil
}

// QueryEdge spells a prospective import using the same discovered module inventory.
// It runs no parser; imports outside the selected inventory remain undecidable.
func QueryEdge(root string, cfg evidenceports.ExtractConfig, from, target string) (graph.Facts, error) {
	if excludedSource(cfg.Exclusions, from) {
		return graph.Facts{}, fmt.Errorf("python: %s is excluded: %w", from, evidenceports.ErrNotExtracted)
	}
	fsys := os.DirFS(root)
	read, _, err := packages(fsys, cfg.PyPackage)
	if err != nil {
		return graph.Facts{}, err
	}
	modules := map[string]string{}
	for _, pkg := range read {
		files, err := sourceFiles(fsys, pkg)
		if err != nil {
			return graph.Facts{}, err
		}
		maps.Copy(modules, files)
	}
	moduleOf := map[string]string{}
	for mod, file := range modules {
		moduleOf[file] = mod
	}
	importer := moduleOf[from]
	imported := target
	if strings.HasSuffix(target, pyExt) {
		imported = moduleOf[target]
	}
	if importer == "" || modules[imported] == "" {
		return graph.Facts{}, fmt.Errorf("python: %s or %s is outside the selected modules: %w", from, target, evidenceports.ErrNotDecidable)
	}
	return graph.Facts{
		Language: graph.LangPython,
		Nodes: []graph.Node{
			{Kind: graph.NodeKindModule, Path: importer, Language: graph.LangPython},
			{Kind: graph.NodeKindModule, Path: imported, Language: graph.LangPython},
		},
		Edges: []graph.Edge{{
			From: "module:" + importer, To: "module:" + imported,
			Kind: importEdgeKind(cfg.Internal, imported), Language: graph.LangPython, Confidence: "high",
			Locations: []graph.Location{{File: from}},
		}},
	}, nil
}

func importEdgeKind(internal []string, module string) graph.EdgeKind {
	for _, pattern := range internal {
		if matched, _ := doublestar.Match(pattern, module); matched {
			return graph.EdgeKindUsesInternal
		}
	}
	return graph.EdgeKindImports
}
