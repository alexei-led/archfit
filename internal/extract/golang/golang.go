package golang

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"golang.org/x/tools/go/packages"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
)

// GoExtractor is the native Go import extractor using go/packages.
// It is in-process (no subprocess) and satisfies the engine.Extractor interface
// structurally: Name() string and Extract(ctx, scope.Scope) (graph.Facts, diagnostic.Coverage, error).
type GoExtractor struct {
	cfg config.ExtractConfig
}

// New returns a GoExtractor configured with the given ExtractConfig.
func New(cfg config.ExtractConfig) *GoExtractor {
	return &GoExtractor{cfg: cfg}
}

// Name returns the language identifier for this extractor.
func (e *GoExtractor) Name() string {
	return "go"
}

// Extract loads all Go packages under s.Root, emits nodes and edges for every
// import statement found in the AST, and returns a Coverage record.
//
// LoadMode: NeedName | NeedFiles | NeedImports | NeedSyntax | NeedTypes
// (NeedTypes is required so that pkg.Fset is populated for position resolution).
func (e *GoExtractor) Extract(ctx context.Context, s scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedImports |
			packages.NeedSyntax |
			packages.NeedTypes,
		Dir:     s.Root,
		Context: ctx,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/golang: load packages: %w", err)
	}

	var nodes []graph.Node
	var edges []graph.Edge
	filesSeen := 0
	unresolved := 0

	// Track emitted node IDs to avoid duplicates within this extractor.
	seenNodes := make(map[string]struct{})

	emitNode := func(n graph.Node) {
		id := n.ID()
		if _, ok := seenNodes[id]; !ok {
			seenNodes[id] = struct{}{}
			nodes = append(nodes, n)
		}
	}

	for _, pkg := range pkgs {
		if pkg.IllTyped || len(pkg.Errors) > 0 {
			unresolved++
		}

		// Emit package node.
		pkgPath := pkg.PkgPath
		if pkgPath != "" {
			pkgNode := graph.Node{Kind: graph.NodeKindPackage, Path: pkgPath}
			emitNode(pkgNode)
		}

		for _, f := range pkg.Syntax {
			// Determine the repo-relative path for this file.
			absFile := pkg.Fset.File(f.Pos()).Name()
			relFile, err := filepath.Rel(s.Root, absFile)
			if err != nil || strings.HasPrefix(relFile, "..") {
				// Outside the root — skip.
				continue
			}
			// Normalise to forward slashes.
			relFile = filepath.ToSlash(relFile)

			// Check exclusions.
			if e.isExcluded(relFile) {
				continue
			}

			filesSeen++
			fileNode := graph.Node{Kind: graph.NodeKindFile, Path: relFile}
			emitNode(fileNode)

			for _, imp := range f.Imports {
				pos := pkg.Fset.Position(imp.Pos())
				importPath := strings.Trim(imp.Path.Value, `"`)

				// Check exclusions on the import target.
				if e.isExcluded(importPath) {
					continue
				}

				// Determine edge kind.
				edgeKind := graph.EdgeKindImports
				if strings.Contains(importPath, "/internal/") || strings.HasSuffix(importPath, "/internal") {
					edgeKind = graph.EdgeKindUsesInternal
				}

				// Make the location file repo-relative when possible.
				locFile := pos.Filename
				if rel, err := filepath.Rel(s.Root, locFile); err == nil && !strings.HasPrefix(rel, "..") {
					locFile = filepath.ToSlash(rel)
				}

				edge := graph.Edge{
					From:       graph.Node{Kind: graph.NodeKindFile, Path: relFile}.ID(),
					To:         graph.Node{Kind: graph.NodeKindPackage, Path: importPath}.ID(),
					Kind:       edgeKind,
					Language:   "go",
					Confidence: "high",
					Locations: []graph.Location{
						{File: locFile, Line: pos.Line},
					},
				}
				edges = append(edges, edge)
			}
		}
	}

	status := "ok"
	if unresolved > 0 {
		status = "partial"
	}

	facts := graph.Facts{
		Nodes:      nodes,
		Edges:      edges,
		Language:   "go",
		Unresolved: unresolved,
	}
	cov := diagnostic.Coverage{
		Tool:            "go/packages",
		FilesSeen:       filesSeen,
		FilesApplicable: filesSeen,
		Unresolved:      unresolved,
		Status:          status,
	}
	return facts, cov, nil
}

// isExcluded reports whether path matches any of the configured exclusion globs.
func (e *GoExtractor) isExcluded(path string) bool {
	for _, pattern := range e.cfg.Exclusions {
		matched, _ := doublestar.Match(pattern, path)
		if matched {
			return true
		}
	}
	return false
}
