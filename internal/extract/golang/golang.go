package golang

import (
	"context"
	"fmt"
	"go/build"
	"go/types"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"

	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
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
	statusOK       = "ok"
	statusAbsent   = "absent"
	statusPartial  = "partial"
	toolGoPackages = "go/packages"
	sourceGoTypes  = "go/types"
)

// BuildConstraintExclusion is the go/packages coverage-reason phrase that
// discloses Go files the host build configuration left out of the load. The
// run warning finds its clause by this phrase.
const BuildConstraintExclusion = "excluded by build constraints"

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
// structurally: Name() string and Extract(ctx, scope.Scope) (graph.Facts, evidence.Coverage, error).
type GoExtractor struct {
	cfg evidenceports.ExtractConfig
	// Runner probes the go-toolchain version for the fact-cache key; nil
	// (tests) yields an empty version component, never an error.
	Runner toolrun.Runner
	// Cache is the per-member extractor fact cache; nil disables caching
	// (--no-cache).
	Cache *factcache.Store
	// load is the packages.Load test seam; nil = packages.Load.
	load loadFunc
	// versionOnce/versionValue memoize the toolchain probe; see goVersion.
	versionOnce  sync.Once
	versionValue string
}

// New returns a GoExtractor configured with the given ExtractConfig.
func New(cfg evidenceports.ExtractConfig) *GoExtractor {
	return &GoExtractor{cfg: cfg}
}

// Name returns the language identifier for this extractor.
func (e *GoExtractor) Name() string {
	return "go"
}

// CoverageTool returns the name this extractor stamps on its Coverage rows.
func (e *GoExtractor) CoverageTool() string {
	return toolGoPackages
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
func (e *GoExtractor) Extract(ctx context.Context, s scope.Scope) (graph.Facts, evidence.Coverage, error) {
	if e.cfg.Mode == evidenceports.ModeOff {
		return graph.Facts{}, evidence.Coverage{Tool: toolGoPackages, Status: statusAbsent}, nil
	}

	// Discover the members this run will load: go.work → per-member dirs (or a
	// single go.mod, or a walk), then the tools.go.modules include/exclude filter.
	members, err := AnalysableMembers(s.Root, e.cfg.Exclusions, e.cfg.GoModuleInclude, e.cfg.GoModuleExclude)
	if err != nil {
		return graph.Facts{}, evidence.Coverage{}, fmt.Errorf("extract/golang: discover members: %w", err)
	}
	memberDirs := members.Dirs

	if len(memberDirs) == 0 {
		return graph.Facts{}, evidence.Coverage{Tool: toolGoPackages, Status: statusAbsent}, nil
	}

	// Load per-member facts — from the fact cache where the member's input
	// tree is unchanged, via packages.Load otherwise (loadMemberFacts).
	mfs, err := e.loadMemberFacts(ctx, s.Root, memberDirs, members.GoWorkOff)
	if err != nil {
		// go/packages.Load failed for at least one workspace member — e.g. a broken
		// go.mod or an unresolvable build constraint. This is a coverage gap, not a
		// run-level failure (the "warn-loud, don't block" contract); only an
		// explicitly required analyzer (ModeOn) hard-errors.
		if e.cfg.Mode == evidenceports.ModeOn {
			return graph.Facts{}, evidence.Coverage{}, err
		}
		return graph.Facts{}, evidence.Coverage{Tool: toolGoPackages, Status: statusPartial, Reason: err.Error()}, nil
	}

	// Build the module map from the per-member facts (derived from pkg.Module —
	// same path family as pkg.Fset file paths, avoiding symlink skew that
	// memberDirs (os.Stat-based) can introduce on macOS).
	//
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

	sortModEntries(modEntries)
	// Sort goModules by Path for deterministic output.
	sort.Slice(goModules, func(i, j int) bool {
		return goModules[i].Path < goModules[j].Path
	})

	stripImportPath := func(importPath string) string { return stripModulePath(modEntries, importPath) }

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
	dataHints := make(map[string]string)
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
		for k, strength := range pf.DataHints {
			relFile, rawPkg, ok := strings.Cut(k, "\x00")
			if !ok || e.isExcluded(relFile) {
				continue
			}
			mk := relFile + "\x00" + stripImportPath(rawPkg)
			if goStrengthRank[dataHints[mk]] < goStrengthRank[strength] {
				dataHints[mk] = strength
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

	nodes, edges, filesSeen, inputsMissing, precisionOnly := e.collectNodesEdges(
		allPkgs, stripImportPath, strengthHints, dataHints, connascenceHints,
	)
	// Both conditions leave the load incomplete over the tree, so both count
	// toward Unresolved and both keep the row partial. They are NOT the same
	// failure, and the consumers that pair two runs' coverage rows (`config
	// compare`, the `--base` origin delta) decide differently on each: a load
	// that lost only go/types precision still saw every package and produced
	// every edge, so two of them rest on the same graph, while one missing input
	// can hide a whole subtree's edges. Both the prose reason (for humans) and
	// the two typed Coverage counters (for those consumers) carry the split — a
	// reason string is not a machine contract.
	cov := e.coverageForLoad(s.Root, memberDirs, filesSeen, inputsMissing, precisionOnly)
	// Files the build configuration left out are disclosed, never graded: the
	// load is complete for the configuration it ran under, so the status (and
	// with it comparability and dimension promotion) stays as measured.
	if n := e.countConstraintExcluded(allPkgs); n > 0 {
		note := fmt.Sprintf("%d Go file(s) %s (GOOS/GOARCH, build tags) were not analyzed", n, BuildConstraintExclusion)
		if cov.Reason != "" {
			note = cov.Reason + "; " + note
		}
		cov.Reason = note
	}
	cov.Version = e.goVersion(ctx)
	facts := graph.Facts{
		Nodes: nodes, Edges: edges, Language: "go", Unresolved: cov.Unresolved, GoModules: goModules,
	}
	return facts, cov, nil
}

func (e *GoExtractor) coverageForLoad(root string, members []string, filesSeen, inputsMissing, precisionOnly int) evidence.Coverage {
	filesApplicable := filesSeen
	var inventoryErr error
	if filesSeen == 0 {
		filesApplicable, inventoryErr = e.countApplicableSources(root, members)
		if filesApplicable > 0 || inventoryErr != nil {
			inputsMissing = max(inputsMissing, 1)
		}
	}
	unresolved := inputsMissing + precisionOnly
	status := statusOK
	switch {
	case filesSeen == 0 && filesApplicable == 0 && inventoryErr == nil:
		status = statusAbsent
	case unresolved > 0:
		status = statusPartial
	}

	cov := evidence.Coverage{
		Tool:                    toolGoPackages,
		FilesSeen:               filesSeen,
		FilesApplicable:         filesApplicable,
		Unresolved:              unresolved,
		UnresolvedInputsMissing: inputsMissing,
		UnresolvedPrecisionOnly: precisionOnly,
		Status:                  status,
	}
	if status == statusPartial {
		cov.Reason = goPartialReason(inputsMissing, precisionOnly)
		if inventoryErr != nil {
			cov.Reason += "; source applicability could not be determined: " + inventoryErr.Error()
		}
	}
	return cov
}

// toolchainContext is the build context of the go command the run starts:
// GOOS and GOARCH from the environment, and the -tags of GOFLAGS then of the
// build flags, the last one winning as on the go command line. getenv reads
// the environment.
func toolchainContext(getenv func(string) string, buildFlags []string) build.Context {
	buildContext := build.Default
	for key, target := range map[string]*string{"GOOS": &buildContext.GOOS, "GOARCH": &buildContext.GOARCH} {
		if value := getenv(key); value != "" {
			*target = value
		}
	}
	flags := append(strings.Fields(getenv("GOFLAGS")), buildFlags...)
	for i, flag := range flags {
		value, ok := strings.CutPrefix(flag, "-tags=")
		if flag == "-tags" && i+1 < len(flags) {
			value, ok = flags[i+1], true
		}
		if ok {
			buildContext.BuildTags = strings.FieldsFunc(strings.Trim(value, "\"'"), func(r rune) bool { return r == ',' || r == ' ' })
		}
	}
	return buildContext
}

// countApplicableSources checks applicability independently of packages.Load.
// Selected members, Go's directory exclusions, and build constraints bound the
// inventory so deliberately unbuilt source does not become a failed measurement.
func (e *GoExtractor) countApplicableSources(root string, members []string) (int, error) {
	buildContext := toolchainContext(os.Getenv, e.cfg.BuildFlags)
	count := 0
	for _, member := range members {
		err := filepath.WalkDir(member, func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, name)
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if name != member && (strings.HasPrefix(entry.Name(), ".") || strings.HasPrefix(entry.Name(), "_") || entry.Name() == "testdata" || entry.Name() == "vendor" || hasGoMod(name) || e.isExcluded(filepath.ToSlash(rel)+"/")) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") || e.isExcluded(filepath.ToSlash(rel)) {
				return nil
			}
			matched, err := buildContext.MatchFile(filepath.Dir(name), entry.Name())
			if matched {
				count++
			}
			return err
		})
		if err != nil {
			return count, err
		}
	}
	return count, nil
}

// goPartialReason states which incomplete-load condition earned the partial row.
// This is the human half of the split; the machine half is the two typed
// Coverage counters set beside it. The two conditions need different fixes, so
// the row has to say which occurred rather than leaving the reader to assume.
func goPartialReason(inputsMissing, precisionOnly int) string {
	switch {
	case precisionOnly == 0:
		return fmt.Sprintf("%d package(s) did not load completely; some imports are missing from the graph", inputsMissing)
	case inputsMissing == 0:
		return fmt.Sprintf("%d package(s) did not type-check; imports are complete, go/types strength is not", precisionOnly)
	default:
		return fmt.Sprintf("%d package(s) left imports missing from the graph and %d did not type-check (strength degraded)",
			inputsMissing, precisionOnly)
	}
}

// collectNodesEdges iterates the merged per-member package facts and emits
// graph nodes and edges. Extracted from Extract to keep Extract's cyclomatic
// complexity below the gate.
//
// Two DIFFERENT incomplete-load conditions are counted separately, split by
// what the incompleteness COST rather than by how the load failed. That is the
// question every consumer of the count actually asks — can this run hide an edge
// another run reports? — and one number for both meant it could not be asked:
//
//   - inputsMissing — part of the package's input set never reached the graph.
//     Either its facts were dropped entirely (synthetic-error packages:
//     Module==nil && Errors non-empty at derive time, no node and no edges), or
//     they were emitted with an import that did not resolve or a file that did
//     not parse (packageFacts.InputsMissing). Both leave edges absent, so both
//     count here — a comparison must never rest on this run.
//   - precisionOnly — the package's facts are ALL emitted (nodes and every
//     import edge) and only type checking did not complete, so the go/types
//     StrengthHints this extractor exists to provide are missing or wrong for
//     it. IllTyped propagates from any dependency, so one bad package can mark a
//     swath — and a package marked purely by propagation is safe to count here,
//     because whatever package caused it is itself counted for its own reason.
func (e *GoExtractor) collectNodesEdges(
	pkgs []packageFacts,
	stripImportPath func(string) string,
	strengthHints, dataHints map[string]string,
	connascenceHints map[string][]graph.ConnascenceHint,
) (nodes []graph.Node, edges []graph.Edge, filesSeen, inputsMissing, precisionOnly int) {
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
			inputsMissing++
			continue
		}
		if p.IllTyped {
			if p.InputsMissing {
				inputsMissing++
			} else {
				precisionOnly++
			}
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

				edgeKind := ImportEdgeKind(importPath)

				key := f.RelFile + "\x00" + importPath
				edges = append(edges, graph.Edge{
					From:             graph.Node{Kind: graph.NodeKindFile, Path: f.RelFile}.ID(),
					To:               graph.Node{Kind: graph.NodeKindPackage, Path: importPath}.ID(),
					Kind:             edgeKind,
					Language:         "go",
					Confidence:       "high",
					Locations:        []graph.Location{{File: imp.LocFile, Line: imp.Line}},
					StrengthHint:     strengthHints[key],
					DataStrengthHint: dataHints[key],
					ConnascenceHints: connascenceHints[key],
				})
			}
		}
	}
	return
}

// modEntry holds a module path and its ScanRoot-relative dir ("." for root).
type modEntry struct {
	path   string
	relDir string
}

// sortModEntries orders entries longest path first, for correct
// longest-prefix matching, then alphabetically for determinism.
func sortModEntries(entries []modEntry) {
	sort.Slice(entries, func(i, j int) bool {
		li, lj := len(entries[i].path), len(entries[j].path)
		if li != lj {
			return li > lj
		}
		return entries[i].path < entries[j].path
	})
}

// stripModulePath converts a Go import path to a ScanRoot-relative path.
// First-party imports (module path ∈ member set) are prefixed with their
// member's relative dir; external deps are returned unchanged.
// Single-member root package (relDir="."): ScanRoot-relative path is "" (node is
// the scan root itself). Identical to the old stripModPath root-module behavior.
// entries must be sorted by sortModEntries.
func stripModulePath(entries []modEntry, importPath string) string {
	for _, m := range entries {
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

// ImportEdgeKind is the kind of an import edge to the ScanRoot-relative
// import path: uses_internal through a Go internal/ segment, else imports. It
// is the one predicate the extractor and `archfit policy can-import` share, so
// a queried edge keys its findings exactly as the extracted one does.
func ImportEdgeKind(importPath string) graph.EdgeKind {
	if strings.Contains(importPath, "/internal/") || strings.HasSuffix(importPath, "/internal") {
		return graph.EdgeKindUsesInternal
	}
	return graph.EdgeKindImports
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
	case *types.Func:
		// A method called through an interface is a contract call; package-level
		// functions and concrete-receiver methods stay functional.
		if sig, ok := tn.Type().(*types.Signature); ok && sig.Recv() != nil && types.IsInterface(sig.Recv().Type()) {
			return strengthContract
		}
		return strengthFunctional
	default:
		// Anything unforeseen → functional.
		return strengthFunctional
	}
}

// goObjectDataStrength is the strongest NON-CALLABLE use an object represents,
// or "" when the use is purely callable. Type names, data vars/fields and
// consts classify as in goObjectStrength; a method on a concrete receiver is
// model evidence (it needs the concrete type); package-level functions,
// interface-method calls and func/chan-valued vars never contribute.
func goObjectDataStrength(obj types.Object, dtos *dtoIndex) string {
	switch tn := obj.(type) {
	case *types.Func:
		if sig, ok := tn.Type().(*types.Signature); ok && sig.Recv() != nil && !types.IsInterface(sig.Recv().Type()) {
			return strengthModel
		}
		return ""
	case *types.TypeName, *types.Var, *types.Const:
		if s := goObjectStrength(obj, dtos); s != strengthFunctional {
			return s
		}
	}
	return ""
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

// countConstraintExcluded counts the distinct Go files the build configuration
// left out of the loaded packages, minus config exclusions (an excluded file is
// out of scope on every platform).
//
// Ceiling: a directory whose every file is excluded is not counted, because
// `go list ./...` does not list it at all. Upgrade to a member-tree walk for
// directories the load never reported if whole platform-only packages turn up.
func (e *GoExtractor) countConstraintExcluded(pkgs []packageFacts) int {
	seen := make(map[string]struct{})
	for _, p := range pkgs {
		for _, f := range p.IgnoredFiles {
			if !e.isExcluded(f) {
				seen[f] = struct{}{}
			}
		}
	}
	return len(seen)
}

// isExcluded reports whether path matches any of the configured exclusion globs.
func (e *GoExtractor) isExcluded(path string) bool {
	return excluded(e.cfg.Exclusions, path)
}
