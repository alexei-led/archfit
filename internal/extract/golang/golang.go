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
// If mode is off, Extract returns empty Facts and an "absent" Coverage immediately.
// LoadMode: NeedName | NeedFiles | NeedImports | NeedSyntax | NeedTypes | NeedModule
// (NeedTypes is required so that pkg.Fset is populated for position resolution.
// NeedModule is required to strip the module path prefix from import paths so that
// node IDs are repo-relative and match the glob patterns in archfit.yaml.)
func (e *GoExtractor) Extract(ctx context.Context, s scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
	if e.cfg.Mode == config.ModeOff {
		return graph.Facts{}, diagnostic.Coverage{Tool: "go/packages", Status: "absent"}, nil
	}

	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedImports |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedModule,
		Dir:     s.Root,
		Context: ctx,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/golang: load packages: %w", err)
	}

	// Determine the module path prefix from the first loaded package so we can
	// strip it from import paths and produce repo-relative node IDs.
	var modPath string
	for _, pkg := range pkgs {
		if pkg.Module != nil && pkg.Module.Path != "" {
			modPath = pkg.Module.Path
			break
		}
	}

	// stripModPath converts a fully-qualified Go import path to a repo-relative
	// path by removing the module prefix (e.g. "example.com/myapp/pkg/a" →
	// "pkg/a"). Paths that do not belong to this module (stdlib, external) are
	// returned unchanged so callers can still apply exclusion rules.
	stripModPath := func(importPath string) string {
		if modPath == "" {
			return importPath
		}
		if importPath == modPath {
			return "."
		}
		if strings.HasPrefix(importPath, modPath+"/") {
			return importPath[len(modPath)+1:]
		}
		return importPath
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

		// Emit package node with repo-relative path.
		pkgPath := stripModPath(pkg.PkgPath)
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
				rawImportPath := strings.Trim(imp.Path.Value, `"`)

				// Strip module prefix to get a repo-relative path for matching.
				importPath := stripModPath(rawImportPath)

				// Check exclusions on the import target.
				if e.isExcluded(importPath) {
					continue
				}

				// Determine edge kind based on the repo-relative path.
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
