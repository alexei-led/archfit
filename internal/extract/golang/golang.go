package golang

import (
	"context"
	"fmt"
	"go/types"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
	"github.com/alexei-led/archfit/internal/view"
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
	sourceGoTypes  = "go/types"
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
	cfg view.ExtractConfig
	// Runner probes the go-toolchain version for the fact-cache key; nil
	// (tests) yields an empty version component, never an error.
	Runner toolrun.Runner
	// Cache is the per-member extractor fact cache; nil disables caching
	// (--no-cache).
	Cache *factcache.Store
	// load is the packages.Load test seam; nil = packages.Load.
	load loadFunc
}

// New returns a GoExtractor configured with the given ExtractConfig.
func New(cfg view.ExtractConfig) *GoExtractor {
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
// Strength coverage: buildStrengthHints derives a hint for EVERY resolved
// cross-package reference — first-party members, stdlib, and third-party alike.
// Cross-member hints keep workspace-mode coupling_balance measurable; external
// hints let a config-declared `external_systems:` seam (DistanceExternal, D=10)
// score with real compiler-grade strength instead of abstaining. Undeclared
// external edges are excluded from scoring by distance, so their hints are
// report-only.
func (e *GoExtractor) Extract(ctx context.Context, s scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
	if e.cfg.Mode == view.ModeOff {
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

	// Load per-member facts — from the fact cache where the member's input
	// tree is unchanged, via packages.Load otherwise (loadMemberFacts).
	mfs, err := e.loadMemberFacts(ctx, s.Root, memberDirs)
	if err != nil {
		// go/packages.Load failed for at least one workspace member — e.g. a broken
		// go.mod or an unresolvable build constraint. This is a coverage gap, not a
		// run-level failure (the "warn-loud, don't block" contract); only an
		// explicitly required analyzer (ModeOn) hard-errors.
		if e.cfg.Mode == view.ModeOn {
			return graph.Facts{}, diagnostic.Coverage{}, err
		}
		return graph.Facts{}, diagnostic.Coverage{Tool: toolGoPackages, Status: "partial", Reason: err.Error()}, nil
	}

	// Build the module map from the per-member facts (derived from pkg.Module —
	// same path family as pkg.Fset file paths, avoiding symlink skew that
	// memberDirs (os.Stat-based) can introduce on macOS).
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

	for _, mf := range mfs {
		for _, m := range mf.Modules {
			if _, ok := seenModPath[m.Path]; ok {
				continue
			}
			seenModPath[m.Path] = struct{}{}
			modEntries = append(modEntries, modEntry{path: m.Path, relDir: m.RelDir})
			goModules = append(goModules, graph.GoModule{Path: m.Path, RelDir: m.RelDir})
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

	// Merge all packages from all member facts, deduplicating by PkgPath.
	// Iterate in member order (mfs[i] ↔ memberDirs[i]) for determinism.
	seenPkg := make(map[string]struct{})
	var allPkgs []packageFacts
	for _, mf := range mfs {
		for _, pf := range mf.Packages {
			if _, ok := seenPkg[pf.PkgPath]; !ok {
				seenPkg[pf.PkgPath] = struct{}{}
				allPkgs = append(allPkgs, pf)
			}
		}
	}

	// Fold the per-member RAW strength hints (deriveRawHints — every resolved
	// target: members, stdlib, third-party) into the merged map the edge loop
	// reads: strip the imported package path (needs the full member set, hence
	// merge-time), drop excluded files, keep the strongest hint per pair.
	strengthHints := make(map[string]string)
	connascenceHints := make(map[string][]graph.ConnascenceHint)
	for _, pf := range allPkgs {
		for k, strength := range pf.Hints {
			relFile, rawPkg, ok := strings.Cut(k, "\x00")
			if !ok || e.isExcluded(relFile) {
				continue
			}
			mk := relFile + "\x00" + stripImportPath(rawPkg)
			if goStrengthRank[strengthHints[mk]] < goStrengthRank[strength] {
				strengthHints[mk] = strength
			}
		}
		for k, hints := range pf.Connascence {
			relFile, rawPkg, ok := strings.Cut(k, "\x00")
			if !ok || e.isExcluded(relFile) {
				continue
			}
			mk := relFile + "\x00" + stripImportPath(rawPkg)
			connascenceHints[mk] = appendConnascenceHints(connascenceHints[mk], hints...)
		}
	}

	nodes, edges, filesSeen, unresolved := e.collectNodesEdges(
		allPkgs, stripImportPath, strengthHints, connascenceHints,
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

// collectNodesEdges iterates the merged per-member package facts and emits
// graph nodes and edges. Extracted from Extract to keep Extract's cyclomatic
// complexity below the gate.
//
// Synthetic-error packages (Module==nil && Errors non-empty at derive time)
// increment unresolved and are skipped — unresolvable patterns, never fatal.
func (e *GoExtractor) collectNodesEdges(
	pkgs []packageFacts,
	stripImportPath func(string) string,
	strengthHints map[string]string,
	connascenceHints map[string][]graph.ConnascenceHint,
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
	for _, p := range pkgs {
		if p.Synthetic {
			unresolved++
			continue
		}
		if p.IllTyped {
			unresolved++
		}

		pkgPath := stripImportPath(p.PkgPath)
		if pkgPath != "" {
			emitNode(graph.Node{Kind: graph.NodeKindPackage, Path: pkgPath, Language: graph.LangGo})
		}

		for _, f := range p.Files {
			if e.isExcluded(f.RelFile) {
				continue
			}
			filesSeen++
			emitNode(graph.Node{Kind: graph.NodeKindFile, Path: f.RelFile, Language: graph.LangGo})

			for _, imp := range f.Imports {
				importPath := stripImportPath(imp.RawPath)
				if e.isExcluded(importPath) {
					continue
				}

				edgeKind := graph.EdgeKindImports
				if strings.Contains(importPath, "/internal/") || strings.HasSuffix(importPath, "/internal") {
					edgeKind = graph.EdgeKindUsesInternal
				}

				key := f.RelFile + "\x00" + importPath
				edges = append(edges, graph.Edge{
					From:             graph.Node{Kind: graph.NodeKindFile, Path: f.RelFile}.ID(),
					To:               graph.Node{Kind: graph.NodeKindPackage, Path: importPath}.ID(),
					Kind:             edgeKind,
					Language:         "go",
					Confidence:       "high",
					Locations:        []graph.Location{{File: imp.LocFile, Line: imp.Line}},
					StrengthHint:     strengthHints[key],
					ConnascenceHints: connascenceHints[key],
				})
			}
		}
	}
	return
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
		// A func- or chan-typed var/field is stored behavior, not data:
		// pkg.DefaultHandler() or holder.OnDone() couples on the callee's
		// behavior exactly like a *types.Func call (mirrors computePureData's
		// behavior-carrier exclusion). Interface-typed vars/fields need no case
		// here: invoking one resolves the method as a *types.Func (→ functional
		// via the default case), so the behavioral use is already captured.
		switch tn.Type().Underlying().(type) {
		case *types.Signature, *types.Chan:
			return strengthFunctional
		}
		// Pure data sharing (book Ch7): using another module's exported
		// var/field couples on its data model, not its behavior. Reads and
		// writes classify alike — assignment position is not inspected.
		return strengthModel
	case *types.Const:
		// Pure data sharing (book Ch7), same as *types.Var.
		return strengthModel
	default:
		// *types.Func (function or method) and anything unforeseen → functional.
		return strengthFunctional
	}
}

func goObjectConnascence(obj types.Object, dtos *dtoIndex) []graph.ConnascenceHint {
	hints := []graph.ConnascenceHint{{Kind: graph.ConnascenceName, Source: sourceGoTypes, Detail: "resolved symbol"}}
	switch tn := obj.(type) {
	case *types.TypeName:
		return append(hints, graph.ConnascenceHint{Kind: graph.ConnascenceType, Source: sourceGoTypes, Detail: "type name"})
	case *types.Var:
		if tn.IsField() && dtos.isDTOField(tn) {
			return append(hints, graph.ConnascenceHint{Kind: graph.ConnascenceType, Source: sourceGoTypes, Detail: "dto field"})
		}
		switch tn.Type().Underlying().(type) {
		case *types.Signature, *types.Chan:
			return append(hints, graph.ConnascenceHint{Kind: graph.ConnascenceAlgorithm, Source: sourceGoTypes, Detail: "callable value"})
		default:
			return append(hints, graph.ConnascenceHint{Kind: graph.ConnascenceMeaning, Source: sourceGoTypes, Detail: "data value"})
		}
	case *types.Const:
		return append(hints, graph.ConnascenceHint{Kind: graph.ConnascenceMeaning, Source: sourceGoTypes, Detail: "constant"})
	default:
		return append(hints, graph.ConnascenceHint{Kind: graph.ConnascenceAlgorithm, Source: sourceGoTypes, Detail: "function or method"})
	}
}

// dtoIndex memoizes pure-data-DTO decisions per named type and records the
// field objects of every type that qualifies, so a bare field reference can be
// tied back to its DTO (go/types exposes no owner pointer on a field Var).
type dtoIndex struct {
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
// at least one field, every field exported, no behavior-carrying fields (func,
// chan, or interface anywhere in the field's type structure — composite
// element types and nested struct fields are recursed into), and an EMPTY
// method set on both value and pointer receivers (promoted methods from
// embedding included). Zero-field marker structs (struct{} sentinels, context
// keys) carry no data model and are NOT DTOs. classify resolves the coupling
// kind: contract across a declared public boundary, model otherwise.
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
	seen := make(map[*types.Named]bool)
	for i := range st.NumFields() {
		f := st.Field(i)
		if !f.Exported() {
			return false
		}
		if containsBehaviorCarrier(f.Type(), seen) {
			return false
		}
	}
	// The pointer method set is a superset of the value method set and includes
	// promoted methods, so one lookup covers every receiver form. computePureData
	// runs at most once per type (isDTOType memoizes), so no method-set cache.
	if types.NewMethodSet(types.NewPointer(named)).Len() != 0 {
		return false
	}
	for i := range st.NumFields() {
		ix.fields[st.Field(i)] = true
	}
	return true
}

// containsBehaviorCarrier reports whether t carries behavior anywhere in its
// structure: a func, chan, or interface type, directly or inside composites
// (pointer, slice, array, map) and nested struct fields. A direct-type check
// alone would let `[]func()`, `map[string]chan T`, or `*Iface` fields smuggle
// behavior into a "pure data" DTO. seen breaks cycles through named types
// (e.g. Node{Next *Node}); a cyclic reference alone is not a carrier.
func containsBehaviorCarrier(t types.Type, seen map[*types.Named]bool) bool {
	if n, ok := types.Unalias(t).(*types.Named); ok {
		if seen[n] {
			return false
		}
		seen[n] = true
	}
	switch u := t.Underlying().(type) {
	case *types.Signature, *types.Chan, *types.Interface:
		return true
	case *types.Pointer:
		return containsBehaviorCarrier(u.Elem(), seen)
	case *types.Slice:
		return containsBehaviorCarrier(u.Elem(), seen)
	case *types.Array:
		return containsBehaviorCarrier(u.Elem(), seen)
	case *types.Map:
		return containsBehaviorCarrier(u.Key(), seen) || containsBehaviorCarrier(u.Elem(), seen)
	case *types.Struct:
		for i := range u.NumFields() {
			if containsBehaviorCarrier(u.Field(i).Type(), seen) {
				return true
			}
		}
	}
	return false
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
