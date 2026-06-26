package golang

import (
	"context"
	"fmt"
	"go/types"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"golang.org/x/tools/go/packages"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
)

// BC integration-strength labels used for StrengthHint on Go edges.
// These match the coupling.Strength constants and the SCIP reader's RANK table.
const (
	strengthContract   = "contract"
	strengthModel      = "model"
	strengthFunctional = "functional"
)

// goStrengthRank maps a BC integration-strength label to its coupling rank.
// contract (rank 1) is the weakest coupling; intrusive (rank 4) is the strongest.
// Used to pick the STRONGEST hint seen per (fromFile, toPkg) pair, mirroring the
// SCIP reader's RANK = {"contract":1, "model":2, "functional":3, "intrusive":4}.
var goStrengthRank = map[string]int{
	strengthContract:   1,
	strengthModel:      2,
	strengthFunctional: 3,
	"intrusive":        4,
}

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
// LoadMode: NeedName | NeedFiles | NeedImports | NeedSyntax | NeedTypes | NeedTypesInfo | NeedModule
// (NeedTypes is required so that pkg.Fset is populated for position resolution.
// NeedTypesInfo populates pkg.TypesInfo.Uses to derive per-edge StrengthHints.
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
			packages.NeedTypesInfo |
			packages.NeedModule,
		Dir:        s.Root,
		Context:    ctx,
		BuildFlags: e.cfg.BuildFlags,
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

	// isInModule reports whether a fully-qualified package path belongs to this module.
	isInModule := func(pkgPath string) bool {
		return modPath != "" && (pkgPath == modPath || strings.HasPrefix(pkgPath, modPath+"/"))
	}

	// Derive per-(relFile, importedPkgRelPath) StrengthHints from type info.
	// Only in-module targets are considered; external deps are excluded from
	// coupling_balance and setting hints for them would be noise.
	strengthHints := buildStrengthHints(pkgs, s.Root, stripModPath, isInModule, e.isExcluded)

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
					StrengthHint: strengthHints[relFile+"\x00"+importPath],
				}
				edges = append(edges, edge)
			}
		}
	}

	status := "ok"
	switch {
	case filesSeen == 0:
		// No Go source files under the scan root: go/packages is not applicable
		// here (e.g. a non-Go repo). Report absent so the coverage metric reads
		// n/a rather than a false-green 100% over an empty file set. A non-Go dir
		// makes packages.Load return a synthetic error package (unresolved>0),
		// which must not be mistaken for partial coverage.
		status = "absent"
	case unresolved > 0:
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

// buildStrengthHints derives per-(relFile, importedPkgRelPath) BC integration-strength
// hints from pkg.TypesInfo.Uses, mirroring the SCIP reader's classify_symbol mapping
// applied to Go:
//
//   - *types.TypeName with interface underlying → "contract"  (rank 1, weakest)
//   - *types.TypeName with concrete type         → "model"     (rank 2)
//   - *types.Func (function or method)           → "functional" (rank 3)
//   - *types.Var, *types.Const, …               → "functional" (rank 3)
//
// Go cross-package references are always to exported symbols, so "intrusive"
// (private-symbol access) never occurs. Each (fromFile, toPkg) pair accumulates
// the STRONGEST (highest-rank) hint seen.
//
// The returned map key is relFile + "\x00" + importedPkgRelPath.
// Only in-module targets are considered (isInModule guard); external deps are
// excluded from coupling_balance and adding hints for them is noise.
func buildStrengthHints(
	pkgs []*packages.Package,
	root string,
	stripModPath func(string) string,
	isInModule func(string) bool,
	isExcluded func(string) bool,
) map[string]string {
	hints := make(map[string]string)
	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil {
			continue
		}
		for ident, obj := range pkg.TypesInfo.Uses {
			// Skip same-package refs and universe symbols (builtins, nil, …).
			if obj.Pkg() == nil || obj.Pkg().Path() == pkg.PkgPath {
				continue
			}
			// Only in-module targets.
			if !isInModule(obj.Pkg().Path()) {
				continue
			}
			// Skip package-name references (import alias, not a symbol use).
			if _, isPkg := obj.(*types.PkgName); isPkg {
				continue
			}

			// Classify the symbol's BC strength per the SCIP reader mapping.
			strength := goObjectStrength(obj)

			// Locate the file containing this identifier.
			tf := pkg.Fset.File(ident.Pos())
			if tf == nil {
				continue
			}
			absFile := tf.Name()
			relFile, ferr := filepath.Rel(root, absFile)
			if ferr != nil || strings.HasPrefix(relFile, "..") {
				continue
			}
			relFile = filepath.ToSlash(relFile)
			if isExcluded(relFile) {
				continue
			}

			importedPkg := stripModPath(obj.Pkg().Path())
			k := relFile + "\x00" + importedPkg
			if goStrengthRank[hints[k]] < goStrengthRank[strength] {
				hints[k] = strength
			}
		}
	}
	return hints
}

// goObjectStrength maps a go/types Object to its BC integration-strength label,
// following the same logic as the SCIP reader's classify_symbol for Go symbols.
func goObjectStrength(obj types.Object) string {
	switch tn := obj.(type) {
	case *types.TypeName:
		if types.IsInterface(tn.Type()) {
			return strengthContract
		}
		return strengthModel
	case *types.Func:
		return strengthFunctional
	default:
		// *types.Var (field/variable), *types.Const → functional.
		return strengthFunctional
	}
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
