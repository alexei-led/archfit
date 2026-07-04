package golang

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/types"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/mod/modfile"
	"golang.org/x/sync/errgroup"
	"golang.org/x/tools/go/packages"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// goAnalyzer is the fact-cache subdirectory for per-member Go facts.
const goAnalyzer = "go"

// loadFunc is the packages.Load seam: tests inject a fake to count per-member
// loads; nil means the real packages.Load (fact-cache.md D5 seam 2 —
// go/packages shells out internally, so the Runner decorator cannot see it).
type loadFunc func(cfg *packages.Config, patterns ...string) ([]*packages.Package, error)

// memberFacts is the serialized per-workspace-member fact set — everything the
// merge step needs from one member's packages.Load, with RAW import paths
// (module-path stripping needs the full member set, so it happens at merge).
// One cache entry per member, so editing one member re-loads only that member.
type memberFacts struct {
	Modules  []graph.GoModule `json:"modules,omitempty"`
	Packages []packageFacts   `json:"packages,omitempty"`
}

type packageFacts struct {
	PkgPath string `json:"pkg_path"`
	// Synthetic mirrors the pkg.Module==nil && len(pkg.Errors)>0 case: an
	// unresolvable pattern counted as unresolved and otherwise skipped.
	Synthetic bool `json:"synthetic,omitempty"`
	// IllTyped mirrors pkg.IllTyped || len(pkg.Errors)>0.
	IllTyped bool        `json:"ill_typed,omitempty"`
	Files    []fileFacts `json:"files,omitempty"`
	// Hints maps relFile+"\x00"+rawImportedPkgPath to the strongest BC
	// integration-strength label seen (buildStrengthHints, pre-strip).
	Hints map[string]string `json:"hints,omitempty"`
}

type fileFacts struct {
	RelFile string        `json:"rel_file"`
	Imports []importFacts `json:"imports,omitempty"`
}

type importFacts struct {
	RawPath string `json:"raw_path"`
	Line    int    `json:"line"`
	LocFile string `json:"loc_file"`
}

// loadMemberFacts returns each member's fact set — from the cache where the
// member's key hits, via packages.Load otherwise (fact-cache.md D5 seam 2).
// keys == nil ⇒ cache off (nil store, --no-cache, or underivable key
// material) — every member loads, nothing is read or written. keys[i] == ""
// ⇒ that single member is vetoed (memberKeys: local replace or unkeyed
// go.work sibling) — it loads fresh and is never read or written. Loads run
// concurrently, one goroutine per member bounded by GOMAXPROCS, writing
// mfs[i] by index so merge order is deterministic regardless of goroutine
// completion order.
func (e *GoExtractor) loadMemberFacts(ctx context.Context, root string, memberDirs []string) ([]memberFacts, error) {
	mfs := make([]memberFacts, len(memberDirs))
	cached := make([]bool, len(memberDirs))
	var keys []string
	if e.Cache != nil {
		keys = e.memberKeys(ctx, root, memberDirs)
	}
	for i := 0; keys != nil && i < len(memberDirs); i++ {
		if keys[i] == "" {
			continue
		}
		if blob, ok := e.Cache.Get(goAnalyzer, keys[i]); ok {
			var mf memberFacts
			if json.Unmarshal(blob, &mf) == nil {
				mfs[i], cached[i] = mf, true
			}
		}
	}

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
		if cached[i] {
			continue
		}
		eg.Go(func() error {
			cfg := &packages.Config{
				Mode:       loadMode,
				Dir:        dir,
				Context:    egCtx,
				BuildFlags: e.cfg.BuildFlags,
			}
			pkgs, loadErr := e.loadPackages(cfg, "./...")
			if loadErr != nil {
				return fmt.Errorf("extract/golang: load %s: %w", dir, loadErr)
			}
			mf, clean := deriveMemberFacts(pkgs, root)
			mfs[i] = mf
			// Only clean loads are cached: a member with load errors would pin
			// its partial facts (fact-cache.md D3). Store.Put is atomic;
			// concurrent members write distinct keys.
			if keys != nil && keys[i] != "" && clean {
				if blob, merr := json.Marshal(mf); merr == nil {
					e.Cache.Put(goAnalyzer, keys[i], blob)
				}
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return mfs, nil
}

// loadPackages is the packages.Load seam (see loadFunc).
func (e *GoExtractor) loadPackages(cfg *packages.Config, patterns ...string) ([]*packages.Package, error) {
	if e.load != nil {
		return e.load(cfg, patterns...)
	}
	return packages.Load(cfg, patterns...)
}

// deriveMemberFacts converts one member's loaded packages into the
// serializable fact set. clean reports whether every package loaded without
// errors — only clean members are cached (fact-cache.md D3: partial results
// are never written; a cached degradation would be sticky).
func deriveMemberFacts(pkgs []*packages.Package, root string) (mf memberFacts, clean bool) {
	clean = true
	seenMod := make(map[string]struct{})
	for _, pkg := range pkgs {
		if pkg.Module == nil || pkg.Module.Path == "" || pkg.Module.Dir == "" {
			continue
		}
		if _, ok := seenMod[pkg.Module.Path]; ok {
			continue
		}
		seenMod[pkg.Module.Path] = struct{}{}
		rel, err := filepath.Rel(root, pkg.Module.Dir)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue // module dir outside scanRoot — mirrors the pre-cache skip
		}
		mf.Modules = append(mf.Modules, graph.GoModule{Path: pkg.Module.Path, RelDir: filepath.ToSlash(rel)})
	}

	// DTO pre-pass over the whole member load, mirroring buildStrengthHints:
	// field references can only be tied to their DTO through this registry, and
	// the owning type may be classified in a different package of the same load.
	dtos := newDTOIndex()
	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil {
			continue
		}
		for _, obj := range pkg.TypesInfo.Uses {
			if tn, ok := obj.(*types.TypeName); ok && tn.Pkg() != nil {
				dtos.isDTOType(tn)
			}
		}
	}

	for _, pkg := range pkgs {
		pf := packageFacts{
			PkgPath:   pkg.PkgPath,
			Synthetic: pkg.Module == nil && len(pkg.Errors) > 0,
			IllTyped:  pkg.IllTyped || len(pkg.Errors) > 0,
		}
		if pf.Synthetic || pf.IllTyped {
			clean = false
		}
		if !pf.Synthetic {
			pf.Files = deriveFileFacts(pkg, root)
			pf.Hints = deriveRawHints(pkg, dtos, root)
		}
		mf.Packages = append(mf.Packages, pf)
	}
	return mf, clean
}

// deriveFileFacts extracts the per-file import facts collectFromFacts needs.
// Exclusion filtering stays in the merge step so cached facts match what the
// pre-cache collectNodesEdges saw at the same stage.
func deriveFileFacts(pkg *packages.Package, root string) []fileFacts {
	var files []fileFacts
	for _, f := range pkg.Syntax {
		absFile := pkg.Fset.File(f.Pos()).Name()
		relFile, err := filepath.Rel(root, absFile)
		if err != nil || strings.HasPrefix(relFile, "..") {
			continue
		}
		ff := fileFacts{RelFile: filepath.ToSlash(relFile)}
		for _, imp := range f.Imports {
			pos := pkg.Fset.Position(imp.Pos())
			locFile := pos.Filename
			if rel, relErr := filepath.Rel(root, locFile); relErr == nil && !strings.HasPrefix(rel, "..") {
				locFile = filepath.ToSlash(rel)
			}
			ff.Imports = append(ff.Imports, importFacts{
				RawPath: strings.Trim(imp.Path.Value, `"`),
				Line:    pos.Line,
				LocFile: locFile,
			})
		}
		files = append(files, ff)
	}
	return files
}

// deriveRawHints derives per-(relFile, rawImportedPkgPath) BC
// integration-strength hints from pkg.TypesInfo.Uses:
//
//   - *types.TypeName with interface underlying → "contract"  (rank 1, weakest)
//   - pure-data DTO type or one of its fields    → "dto"       (rank 2)
//   - *types.TypeName with concrete type         → "model"     (rank 3)
//   - *types.Var, *types.Const (pure data use — reads and writes classify
//     alike)                                     → "model"     (rank 3, book Ch7)
//   - *types.Var with func/chan underlying (stored behavior) → "functional"
//   - *types.Func (function or method)           → "functional" (rank 4)
//
// Go cross-package references are always to exported symbols, so "intrusive"
// (private-symbol access) never occurs. Each (fromFile, toPkg) pair keeps the
// STRONGEST (highest-rank) hint seen. Every resolved target gets a hint —
// first-party members AND external (stdlib/third-party) packages — so a
// config-declared `external_systems:` seam scores at DistanceExternal instead
// of abstaining; on undeclared external edges the hint is inert.
//
// Imported package paths stay RAW and no exclusion filter runs here — module
// stripping needs the full member set and exclusions are config, so both are
// applied at merge time in Extract.
func deriveRawHints(pkg *packages.Package, dtos *dtoIndex, root string) map[string]string {
	if pkg.TypesInfo == nil {
		return nil
	}
	hints := make(map[string]string)
	for ident, obj := range pkg.TypesInfo.Uses {
		if obj.Pkg() == nil || obj.Pkg().Path() == pkg.PkgPath {
			continue
		}
		if _, isPkg := obj.(*types.PkgName); isPkg {
			continue
		}
		strength := goObjectStrength(obj, dtos)
		tf := pkg.Fset.File(ident.Pos())
		if tf == nil {
			continue
		}
		relFile, err := filepath.Rel(root, tf.Name())
		if err != nil || strings.HasPrefix(relFile, "..") {
			continue
		}
		k := filepath.ToSlash(relFile) + "\x00" + obj.Pkg().Path()
		if goStrengthRank[hints[k]] < goStrengthRank[strength] {
			hints[k] = strength
		}
	}
	if len(hints) == 0 {
		return nil
	}
	return hints
}

// goListHashExcludes are the input-tree-hash exclusions faithful to
// `go list ./...` semantics: the go tool never loads packages under testdata,
// so edits there cannot affect a member's facts. vendor/ is deliberately
// HASHED: with vendor/modules.txt present the build defaults to -mod=vendor
// and type-checks dependency source from vendor/, so a hand-patched vendored
// file must invalidate (for -mod=mod repos that is a spurious invalidation —
// over-hash, never under-hash). Config `exclude:` globs are deliberately NOT
// applied — packages.Load still type-checks files they match (exclusion
// filtering happens at merge time), so editing an excluded file must still
// invalidate; exclusion-config changes invalidate via cfgHash (e.cfg embeds
// Exclusions).
var goListHashExcludes = []string{"**/testdata/**"}

// memberKeys derives one fact-cache key per member, or nil when key material
// cannot be derived (cache disabled for this run — never an error). A key of
// "" vetoes caching for that single member: its build reaches local source
// the input-tree hash cannot see (a replace-to-directory in its go.mod, or a
// require satisfied by a go.work member outside memberDirs — excluded,
// config-filtered, or above ScanRoot). Veto, not fold: hashing a foreign tree
// would need its own walk/rel-path scheme; upgrade trigger is warm-run misses
// on repos that keep local replaces permanently.
//
// Key inputs per member: go-toolchain version, build-affecting go env, the
// ExtractConfig view + scan root + go.work content, and the content hash of
// the member's OWN input tree UNION the trees of every member it
// (transitively) requires — a workspace member compiles against its sibling
// dependencies' source, so their edits must invalidate its facts too.
// Members nobody depends on invalidate alone ("edit one member file → only
// that member re-loads").
func (e *GoExtractor) memberKeys(ctx context.Context, scanRoot string, memberDirs []string) []string {
	cfgHash, err := factcache.HashJSON(struct {
		Cfg    config.ExtractConfig
		Root   string
		GoWork string
		Env    map[string]string
	}{e.cfg, scanRoot, hashGoWork(scanRoot), e.goCacheEnv(ctx)})
	if err != nil {
		return nil
	}
	goVer := e.goVersion(ctx)

	type memberInfo struct {
		modPath      string
		requires     map[string]struct{}
		files        []string // scanRoot-relative slash paths
		localReplace bool     // go.mod replaces a module with a local directory
	}
	infos := make([]memberInfo, len(memberDirs))
	for i, dir := range memberDirs {
		data, rerr := os.ReadFile(filepath.Join(dir, "go.mod")) //nolint:gosec // member dir from DiscoverMembers
		if rerr != nil {
			return nil
		}
		mod, perr := modfile.Parse("go.mod", data, nil)
		if perr != nil || mod.Module == nil {
			return nil
		}
		infos[i].modPath = mod.Module.Mod.Path
		infos[i].requires = make(map[string]struct{}, len(mod.Require))
		for _, r := range mod.Require {
			infos[i].requires[r.Mod.Path] = struct{}{}
		}
		infos[i].localReplace = hasLocalReplace(mod.Replace)
		infos[i].files = memberInputFiles(scanRoot, dir, memberDirs)
	}
	unkeyed, workLocalReplace := unkeyedWorkspaceMods(scanRoot, memberDirs)
	if workLocalReplace {
		return nil // go.work replaces a module with a local dir no key can see
	}

	// Transitive closure over intra-workspace requires; a member whose closure
	// touches unkeyed local source gets the "" veto key.
	modIdx := make(map[string]int, len(infos))
	for i, inf := range infos {
		modIdx[inf.modPath] = i
	}
	keys := make([]string, len(memberDirs))
	for i := range infos {
		seen := make(map[int]struct{})
		veto := false
		var visit func(int)
		visit = func(j int) {
			if _, ok := seen[j]; ok {
				return
			}
			seen[j] = struct{}{}
			if infos[j].localReplace {
				veto = true
			}
			for req := range infos[j].requires {
				if k, ok := modIdx[req]; ok {
					visit(k)
				} else if _, unk := unkeyed[req]; unk {
					veto = true
				}
			}
		}
		visit(i)
		if veto {
			continue // keys[i] stays "" — this member runs uncached
		}
		var files []string
		for j := range seen {
			files = append(files, infos[j].files...)
		}
		treeHash, herr := factcache.HashTree(scanRoot, files)
		if herr != nil {
			return nil
		}
		keys[i] = factcache.Key(goAnalyzer, goVer, cfgHash, treeHash)
	}
	return keys
}

// hasLocalReplace reports whether any replace directive points at a local
// directory (modfile convention: a filesystem-path replacement has an empty
// New.Version). Such a build compiles against source the input-tree hash
// cannot see.
func hasLocalReplace(replaces []*modfile.Replace) bool {
	for _, r := range replaces {
		if r.New.Version == "" {
			return true
		}
	}
	return false
}

// unkeyedWorkspaceMods returns the module paths of go.work members that are
// NOT in memberDirs (above ScanRoot, exclusion-matched, or filtered by
// tools.go.modules include/exclude), plus whether go.work itself carries a
// local-directory replace. packages.Load in workspace mode still compiles
// against those members' source, which no per-member key covers. Best-effort:
// a missing/unparsable go.work or member go.mod contributes nothing.
func unkeyedWorkspaceMods(scanRoot string, memberDirs []string) (map[string]struct{}, bool) {
	path, found := findGoWork(scanRoot)
	if !found {
		return nil, false
	}
	data, err := os.ReadFile(path) //nolint:gosec // path from findGoWork
	if err != nil {
		return nil, false
	}
	wf, err := modfile.ParseWork(path, data, nil)
	if err != nil {
		return nil, false
	}
	keyed := make(map[string]struct{}, len(memberDirs))
	for _, d := range memberDirs {
		keyed[filepath.Clean(d)] = struct{}{}
	}
	workDir := filepath.Dir(path)
	var out map[string]struct{}
	for _, u := range wf.Use {
		dir := filepath.Clean(filepath.Join(workDir, filepath.FromSlash(u.Path)))
		if _, ok := keyed[dir]; ok {
			continue
		}
		modData, rerr := os.ReadFile(filepath.Join(dir, "go.mod"))
		if rerr != nil {
			continue
		}
		mod, perr := modfile.Parse("go.mod", modData, nil)
		if perr != nil || mod.Module == nil {
			continue
		}
		if out == nil {
			out = make(map[string]struct{})
		}
		out[mod.Module.Mod.Path] = struct{}{}
	}
	return out, hasLocalReplace(wf.Replace)
}

// goBuildExts are the source extensions the go tool treats as package input:
// .go plus the cgo/asm/swig companion files (go/build's file-type list). A
// cgo member's type info depends on its .c/.h sources, so they must be in the
// key — over-hash, never under-hash.
var goBuildExts = []string{
	".go", ".c", ".cc", ".cxx", ".cpp", ".m", ".h", ".hh", ".hpp", ".hxx",
	".s", ".S", ".sx", ".f", ".F", ".for", ".f90", ".swig", ".swigcxx", ".syso",
}

// memberInputFiles enumerates one member's input tree as scanRoot-relative
// slash paths: its go-build source files plus go.mod/go.sum, excluding files
// owned by a NESTED member (go list ./... stops at nested modules, so those
// files cannot affect this member's load) and goListHashExcludes (dirs the go
// tool never loads).
func memberInputFiles(scanRoot, memberDir string, memberDirs []string) []string {
	var nested []string
	for _, other := range memberDirs {
		if other == memberDir {
			continue
		}
		if rel, err := filepath.Rel(memberDir, other); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			nested = append(nested, filepath.ToSlash(rel)+"/")
		}
	}
	files := factcache.ListInputs(memberDir, factcache.MatchExts(goBuildExts, []string{"go.mod", "go.sum"}), goListHashExcludes)
	var out []string
	for _, f := range files {
		underNested := false
		for _, n := range nested {
			if strings.HasPrefix(f, n) {
				underNested = true
				break
			}
		}
		if underNested {
			continue
		}
		abs := filepath.Join(memberDir, filepath.FromSlash(f))
		rel, err := filepath.Rel(scanRoot, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		out = append(out, filepath.ToSlash(rel))
	}
	return out
}

// hashGoWork returns the sha256 of the go.work + go.work.sum contents that
// govern scanRoot, or "" when no go.work exists. The file may live ABOVE
// scanRoot (findGoWork walks up), so it cannot ride the input-tree hash.
func hashGoWork(scanRoot string) string {
	path, found := findGoWork(scanRoot)
	if !found {
		return ""
	}
	h := sha256.New()
	for _, p := range []string{path, path + ".sum"} {
		if data, err := os.ReadFile(p); err == nil { //nolint:gosec // path from findGoWork
			h.Write(data)
			h.Write([]byte{0})
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// goVersion probes the go toolchain version through the injected Runner —
// go/packages shells out to the PATH toolchain, so runtime.Version() (the
// version archfit was BUILT with) would be the wrong key input. Best-effort:
// "" when no Runner is wired (tests) or the probe fails.
func (e *GoExtractor) goVersion(ctx context.Context) string {
	if e.Runner == nil {
		return ""
	}
	out, err := e.Runner.Run(ctx, toolrun.ToolCmd{
		Name:    "go",
		Args:    []string{"version"},
		Timeout: 30 * time.Second,
	})
	if err != nil || out.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(string(out.Stdout))
}

var goCacheEnvKeys = []string{
	"GOOS",
	"GOARCH",
	"CGO_ENABLED",
	"GOFLAGS",
	"GOWORK",
	"GOEXPERIMENT",
	"GO111MODULE",
	"GOTOOLCHAIN",
}

func (e *GoExtractor) goCacheEnv(ctx context.Context) map[string]string {
	if e.Runner != nil {
		args := append([]string{"env", "-json"}, goCacheEnvKeys...)
		out, err := e.Runner.Run(ctx, toolrun.ToolCmd{
			Name:    "go",
			Args:    args,
			Timeout: 30 * time.Second,
		})
		if err == nil && out.ExitCode == 0 {
			var env map[string]string
			if json.Unmarshal(out.Stdout, &env) == nil {
				return env
			}
		}
	}
	env := make(map[string]string, len(goCacheEnvKeys))
	for _, key := range goCacheEnvKeys {
		env[key] = os.Getenv(key)
	}
	env["GOOS"] = runtime.GOOS
	env["GOARCH"] = runtime.GOARCH
	return env
}
