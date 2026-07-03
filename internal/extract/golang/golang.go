package golang

import (
	"context"
	"fmt"
	"go/types"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"golang.org/x/sync/errgroup"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
)

// BC integration-strength labels used for StrengthHint on Go edges.
// These match the coupling.Strength constants and the SCIP reader's RANK table.
// strengthDTO is the exception: a hint-only label (graph.StrengthHintDTO) whose
// coupling kind classify resolves from the public-boundary declaration.
const (
	strengthContract   = "contract"
	strengthModel      = "model"
	strengthFunctional = "functional"
	strengthIntrusive  = "intrusive"
	strengthDTO        = graph.StrengthHintDTO
)

const (
	statusAbsent   = "absent"
	toolGoPackages = "go/packages"
)

// goStrengthRank maps a BC integration-strength label to its coupling rank.
// contract (rank 1) is the weakest coupling; intrusive (rank 5) is the strongest.
// Used to pick the STRONGEST hint seen per (fromFile, toPkg) pair. The relative
// order of the shared labels mirrors the SCIP reader's RANK table; dto (which
// SCIP cannot detect — it needs method sets and field visibility) slots between
// contract and model: a concrete DTO reference couples tighter than an
// interface, but a leaked domain object (model) dominates a DTO.
var goStrengthRank = map[string]int{
	strengthContract:   1,
	strengthDTO:        2,
	strengthModel:      3,
	strengthFunctional: 4,
	strengthIntrusive:  5,
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

// Extract loads all Go packages for every workspace member under s.Root,
// emits nodes and edges for every import statement found in the AST, and
// returns a Coverage record.
//
// Workspace support: DiscoverMembers locates go.work (or falls back to a
// single go.mod / walk). Each member is loaded with its own packages.Config
// (Dir=memberDir), run concurrently via errgroup bounded to GOMAXPROCS.
// Results are merged deterministically (results[i] ↔ memberDirs[i]).
//
// LoadMode: NeedName | NeedFiles | NeedImports | NeedSyntax | NeedTypes |
// NeedTypesInfo | NeedModule
// (NeedTypes is required so that pkg.Fset is populated for position resolution.
// NeedTypesInfo populates pkg.TypesInfo.Uses to derive per-edge StrengthHints.
// NeedModule is required to strip the module path prefix from import paths so
// that node IDs are ScanRoot-relative and match the globs in archfit.yaml.)
//
// Strength guard: buildStrengthHints uses the member-set predicate (isFirstParty)
// so cross-member type references also produce StrengthHints. Without this,
// every Go edge in workspace mode abstains on strength and coupling_balance
// collapses even when distance is present.
func (e *GoExtractor) Extract(ctx context.Context, s scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
	if e.cfg.Mode == config.ModeOff {
		return graph.Facts{}, diagnostic.Coverage{Tool: toolGoPackages, Status: statusAbsent}, nil
	}

	// Discover workspace members (go.work → per-member dirs, or single go.mod).
	memberDirs, err := DiscoverMembers(s.Root, e.cfg.Exclusions)
	if err != nil {
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/golang: discover members: %w", err)
	}

	// Apply tools.go.modules include/exclude globs (user-facing member scoping).
	// This is a deliberate post-discovery filter: DiscoverMembers handles scope
	// exclusions (testdata, generated dirs); FilterMembers handles the user knob
	// that restricts analysis to a named subset of workspace members for large
	// workspaces where a full run exceeds acceptable wall-clock budgets.
	//
	// Scale ceiling: on a ~178-member workspace (omni), a full NeedTypesInfo load
	// takes >5 minutes. Two mitigations are available: tools.go.modules narrows
	// the member set; tools.<x>.timeout caps the per-analyzer wall-clock budget
	// (the watchdog fires before the full pipeline hangs). Use them together for
	// large workspaces.
	memberDirs = FilterMembers(memberDirs, s.Root, e.cfg.GoModuleInclude, e.cfg.GoModuleExclude)

	if len(memberDirs) == 0 {
		return graph.Facts{}, diagnostic.Coverage{Tool: toolGoPackages, Status: statusAbsent}, nil
	}

	// Load packages concurrently — one goroutine per member, bounded by GOMAXPROCS.
	// Write into results[i] by index so merge order is deterministic regardless of
	// goroutine completion order.
	type memberResult struct {
		pkgs []*packages.Package
	}
	results := make([]memberResult, len(memberDirs))

	loadMode := packages.NeedName |
		packages.NeedFiles |
		packages.NeedImports |
		packages.NeedSyntax |
		packages.NeedTypes |
		packages.NeedTypesInfo |
		packages.NeedModule

	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(max(1, runtime.GOMAXPROCS(0)))
	for i, dir := range memberDirs {
		i, dir := i, dir
		eg.Go(func() error {
			cfg := &packages.Config{
				Mode:       loadMode,
				Dir:        dir,
				Context:    egCtx,
				BuildFlags: e.cfg.BuildFlags,
			}
			pkgs, loadErr := packages.Load(cfg, "./...")
			if loadErr != nil {
				return fmt.Errorf("extract/golang: load %s: %w", dir, loadErr)
			}
			results[i] = memberResult{pkgs: pkgs}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		// go/packages.Load failed for at least one workspace member — e.g. a broken
		// go.mod or an unresolvable build constraint. This is a coverage gap, not a
		// run-level failure (the "warn-loud, don't block" contract); only an
		// explicitly required analyzer (ModeOn) hard-errors.
		if e.cfg.Mode == config.ModeOn {
			return graph.Facts{}, diagnostic.Coverage{}, err
		}
		return graph.Facts{}, diagnostic.Coverage{Tool: toolGoPackages, Status: "partial", Reason: err.Error()}, nil
	}

	// Build the module map from pkg.Module — same path family as pkg.Fset file
	// paths, avoiding symlink skew that memberDirs (os.Stat-based) can introduce
	// on macOS (/tmp → /private/tmp in temp dirs used by tests).
	//
	// modEntry holds a module path and its ScanRoot-relative dir ("." for root).
	// Entries are sorted: longest path first (correct longest-prefix matching),
	// then alpha for equal-length ties (determinism).
	type modEntry struct {
		path   string
		relDir string
	}
	seenModPath := make(map[string]struct{})
	var modEntries []modEntry
	var goModules []graph.GoModule

	for _, r := range results {
		for _, pkg := range r.pkgs {
			if pkg.Module == nil || pkg.Module.Path == "" || pkg.Module.Dir == "" {
				continue
			}
			if _, ok := seenModPath[pkg.Module.Path]; ok {
				continue
			}
			seenModPath[pkg.Module.Path] = struct{}{}
			rel, relErr := filepath.Rel(s.Root, pkg.Module.Dir)
			if relErr != nil || strings.HasPrefix(rel, "..") {
				// Module dir is outside scanRoot — skip (shouldn't happen after DiscoverMembers).
				continue
			}
			relDir := filepath.ToSlash(rel)
			modEntries = append(modEntries, modEntry{path: pkg.Module.Path, relDir: relDir})
			goModules = append(goModules, graph.GoModule{Path: pkg.Module.Path, RelDir: relDir})
		}
	}

	// Sort modEntries: longest path first for correct prefix matching; alpha tiebreak.
	sort.Slice(modEntries, func(i, j int) bool {
		li, lj := len(modEntries[i].path), len(modEntries[j].path)
		if li != lj {
			return li > lj
		}
		return modEntries[i].path < modEntries[j].path
	})
	// Sort goModules by Path for deterministic output.
	sort.Slice(goModules, func(i, j int) bool {
		return goModules[i].Path < goModules[j].Path
	})

	// stripImportPath converts a Go import path to a ScanRoot-relative path.
	// First-party imports (module path ∈ member set) are prefixed with their
	// member's relative dir; external deps are returned unchanged.
	// Single-member root package (relDir="."): ScanRoot-relative path is "" (node is
	// the scan root itself). Identical to the old stripModPath root-module behavior.
	stripImportPath := func(importPath string) string {
		for _, m := range modEntries {
			if importPath == m.path {
				if m.relDir == "." {
					return "" // root member's root package: ScanRoot-relative path is ""
				}
				return m.relDir
			}
			if strings.HasPrefix(importPath, m.path+"/") {
				suffix := importPath[len(m.path)+1:]
				if m.relDir == "." {
					return suffix
				}
				return m.relDir + "/" + suffix
			}
		}
		return importPath // external dep — unchanged
	}

	// isFirstParty reports whether importPath belongs to any loaded workspace member.
	// Replaces the old single-module isInModule predicate; cross-member type
	// references now produce StrengthHints (the critical strength guard).
	isFirstParty := func(importPath string) bool {
		for _, m := range modEntries {
			if importPath == m.path || strings.HasPrefix(importPath, m.path+"/") {
				return true
			}
		}
		return false
	}

	// Merge all packages from all member loads, deduplicating by PkgPath.
	// Iterate results in member order (results[i] ↔ memberDirs[i]) for determinism.
	seenPkg := make(map[string]struct{})
	var allPkgs []*packages.Package
	for _, r := range results {
		for _, pkg := range r.pkgs {
			if _, ok := seenPkg[pkg.PkgPath]; !ok {
				seenPkg[pkg.PkgPath] = struct{}{}
				allPkgs = append(allPkgs, pkg)
			}
		}
	}

	// Derive per-(relFile, importedPkgRelPath) StrengthHints from type info.
	// Uses isFirstParty (member-set predicate) so cross-member type references
	// also get hints — the critical strength guard for workspace mode.
	strengthHints := buildStrengthHints(allPkgs, s.Root, stripImportPath, isFirstParty, e.isExcluded)

	nodes, edges, filesSeen, unresolved := e.collectNodesEdges(
		allPkgs, s.Root, stripImportPath, strengthHints,
	)

	status := "ok"
	switch {
	case filesSeen == 0:
		// No Go source files under the scan root: go/packages is not applicable
		// here (e.g. a non-Go repo). Report absent so the coverage metric reads
		// n/a rather than a false-green 100% over an empty file set. A non-Go dir
		// makes packages.Load return a synthetic error package (unresolved>0),
		// which must not be mistaken for partial coverage.
		status = statusAbsent
	case unresolved > 0:
		status = "partial"
	}

	facts := graph.Facts{
		Nodes:      nodes,
		Edges:      edges,
		Language:   "go",
		Unresolved: unresolved,
		GoModules:  goModules,
	}
	cov := diagnostic.Coverage{
		Tool:            toolGoPackages,
		FilesSeen:       filesSeen,
		FilesApplicable: filesSeen,
		Unresolved:      unresolved,
		Status:          status,
	}
	return facts, cov, nil
}

// collectNodesEdges iterates loaded packages and emits graph nodes and edges.
// Extracted from Extract to keep Extract's cyclomatic complexity below the gate.
//
// Synthetic-error packages (Module==nil && Errors non-empty) increment unresolved
// and are skipped — they represent unresolvable patterns, never fatal.
func (e *GoExtractor) collectNodesEdges(
	pkgs []*packages.Package,
	root string,
	stripImportPath func(string) string,
	strengthHints map[string]string,
) (nodes []graph.Node, edges []graph.Edge, filesSeen, unresolved int) {
	// seenNodes deduplicates package/file nodes within this extractor.
	seenNodes := make(map[string]struct{})
	emitNode := func(n graph.Node) {
		id := n.ID()
		if _, ok := seenNodes[id]; !ok {
			seenNodes[id] = struct{}{}
			nodes = append(nodes, n)
		}
	}
	for _, pkg := range pkgs {
		if pkg.Module == nil && len(pkg.Errors) > 0 {
			unresolved++
			continue
		}
		if pkg.IllTyped || len(pkg.Errors) > 0 {
			unresolved++
		}

		pkgPath := stripImportPath(pkg.PkgPath)
		if pkgPath != "" {
			emitNode(graph.Node{Kind: graph.NodeKindPackage, Path: pkgPath, Language: graph.LangGo})
		}

		for _, f := range pkg.Syntax {
			absFile := pkg.Fset.File(f.Pos()).Name()
			relFile, err := filepath.Rel(root, absFile)
			if err != nil || strings.HasPrefix(relFile, "..") {
				continue
			}
			relFile = filepath.ToSlash(relFile)
			if e.isExcluded(relFile) {
				continue
			}
			filesSeen++
			emitNode(graph.Node{Kind: graph.NodeKindFile, Path: relFile, Language: graph.LangGo})

			for _, imp := range f.Imports {
				pos := pkg.Fset.Position(imp.Pos())
				rawImportPath := strings.Trim(imp.Path.Value, `"`)
				importPath := stripImportPath(rawImportPath)
				if e.isExcluded(importPath) {
					continue
				}

				edgeKind := graph.EdgeKindImports
				if strings.Contains(importPath, "/internal/") || strings.HasSuffix(importPath, "/internal") {
					edgeKind = graph.EdgeKindUsesInternal
				}

				locFile := pos.Filename
				if rel, relErr := filepath.Rel(root, locFile); relErr == nil && !strings.HasPrefix(rel, "..") {
					locFile = filepath.ToSlash(rel)
				}

				edges = append(edges, graph.Edge{
					From:         graph.Node{Kind: graph.NodeKindFile, Path: relFile}.ID(),
					To:           graph.Node{Kind: graph.NodeKindPackage, Path: importPath}.ID(),
					Kind:         edgeKind,
					Language:     "go",
					Confidence:   "high",
					Locations:    []graph.Location{{File: locFile, Line: pos.Line}},
					StrengthHint: strengthHints[relFile+"\x00"+importPath],
				})
			}
		}
	}
	return
}

// buildStrengthHints derives per-(relFile, importedPkgRelPath) BC integration-strength
// hints from pkg.TypesInfo.Uses:
//
//   - *types.TypeName with interface underlying → "contract"  (rank 1, weakest)
//   - pure-data DTO type or one of its fields    → "dto"       (rank 2)
//   - *types.TypeName with concrete type         → "model"     (rank 3)
//   - *types.Var, *types.Const (pure data read)  → "model"     (rank 3, book Ch7)
//   - *types.Func (function or method)           → "functional" (rank 4)
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
	dtos := newDTOIndex()
	// Pre-pass: evaluate every cross-package TypeName once so DTO FIELDS are
	// recognizable in the classification pass. A field *types.Var carries no
	// owner pointer in go/types, so a composite-literal key (UserDTO{ID: …}) or
	// a selector (u.ID) can only be tied back to its DTO through this registry.
	// Uses map iteration order is random and the field may be classified in a
	// different package than the one naming the type — hence a full pass first.
	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil {
			continue
		}
		for _, obj := range pkg.TypesInfo.Uses {
			if tn, ok := obj.(*types.TypeName); ok && tn.Pkg() != nil && isInModule(tn.Pkg().Path()) {
				dtos.isDTOType(tn)
			}
		}
	}
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
			strength := goObjectStrength(obj, dtos)

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

// goObjectStrength maps a go/types Object to its BC integration-strength label.
func goObjectStrength(obj types.Object, dtos *dtoIndex) string {
	switch tn := obj.(type) {
	case *types.TypeName:
		if types.IsInterface(tn.Type()) {
			return strengthContract
		}
		if dtos.isDTOType(tn) {
			return strengthDTO
		}
		return strengthModel
	case *types.Var:
		// A field of a pure-data DTO is the DTO's data — reading or setting it
		// (u.ID, UserDTO{ID: …}) stays dto, not the stronger model; otherwise
		// every real consumer of a DTO would outrank the dto hint away.
		if tn.IsField() && dtos.isDTOField(tn) {
			return strengthDTO
		}
		// Pure data sharing (book Ch7): reading another module's exported
		// var/field couples on its data model, not its behavior.
		return strengthModel
	case *types.Const:
		// Pure data sharing (book Ch7), same as *types.Var.
		return strengthModel
	default:
		// *types.Func (function or method) and anything unforeseen → functional.
		return strengthFunctional
	}
}

// dtoIndex memoizes pure-data-DTO decisions per named type and records the
// field objects of every type that qualifies, so a bare field reference can be
// tied back to its DTO (go/types exposes no owner pointer on a field Var).
type dtoIndex struct {
	msets  typeutil.MethodSetCache
	types  map[*types.TypeName]bool
	fields map[*types.Var]bool
}

func newDTOIndex() *dtoIndex {
	return &dtoIndex{
		types:  make(map[*types.TypeName]bool),
		fields: make(map[*types.Var]bool),
	}
}

// isDTOType reports whether tn names a pure-data DTO: an exported struct with
// at least one field, every field exported, no func- or chan-typed fields
// (behavior carriers, checked one level deep — element types of composites are
// not recursed into), and an EMPTY method set on both value and pointer
// receivers (promoted methods from embedding included). Zero-field marker
// structs (struct{} sentinels, context keys) carry no data model and are NOT
// DTOs. classify resolves the coupling kind: contract across a declared public
// boundary, model otherwise.
func (ix *dtoIndex) isDTOType(tn *types.TypeName) bool {
	if v, ok := ix.types[tn]; ok {
		return v
	}
	v := ix.computePureData(tn)
	ix.types[tn] = v
	return v
}

// isDTOField reports whether v is a field of a type isDTOType already
// qualified (the pre-pass in buildStrengthHints populates the registry).
func (ix *dtoIndex) isDTOField(v *types.Var) bool { return ix.fields[v] }

func (ix *dtoIndex) computePureData(tn *types.TypeName) bool {
	if !tn.Exported() {
		return false
	}
	named, ok := types.Unalias(tn.Type()).(*types.Named)
	if !ok {
		return false
	}
	st, ok := named.Underlying().(*types.Struct)
	if !ok || st.NumFields() == 0 {
		return false
	}
	for i := range st.NumFields() {
		f := st.Field(i)
		if !f.Exported() {
			return false
		}
		switch f.Type().Underlying().(type) {
		case *types.Signature, *types.Chan:
			return false
		}
	}
	// The pointer method set is a superset of the value method set and includes
	// promoted methods, so one lookup covers every receiver form.
	if ix.msets.MethodSet(types.NewPointer(named)).Len() != 0 {
		return false
	}
	for i := range st.NumFields() {
		ix.fields[st.Field(i)] = true
	}
	return true
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
