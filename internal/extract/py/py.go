package py

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	toolGrimp     = "grimp"
	langPython    = "python"
	sourceGrimp   = "grimp"
	statusOK      = "ok"
	statusPartial = "partial"
	statusAbsent  = "absent"
	runTimeout    = 5 * time.Minute
)

// Extractor is the Python import extractor using grimp via uv or python3.12.
// It satisfies the engine.Extractor interface structurally.
type Extractor struct {
	runner toolrun.Runner
	cfg    config.ExtractConfig
	// Cache is the extractor fact cache; nil disables caching (--no-cache).
	Cache *factcache.Store
}

// pyManifestNames are the resolution-affecting manifests hashed into the
// fact-cache key alongside the .py tree: project metadata and lockfiles that
// change what grimp can import.
var pyManifestNames = []string{
	"pyproject.toml", "setup.py", "setup.cfg", "uv.lock", "requirements.txt",
}

// New returns an Extractor configured with the given runner and config.
func New(runner toolrun.Runner, cfg config.ExtractConfig) *Extractor {
	return &Extractor{runner: runner, cfg: cfg}
}

// Name returns the language identifier for this extractor.
func (e *Extractor) Name() string {
	return langPython
}

// Extract detects uv or python3.12+grimp, writes the embedded helper to a temp
// file, runs it against the project root, parses the JSON output, and returns
// graph.Facts + diagnostic.Coverage.
//
// If mode is off, Extract returns empty Facts and an "absent" Coverage immediately.
// If mode is auto and no applicable tool or Python project is found,
// Extract returns empty Facts and an "absent" Coverage — never an error.
// If mode is on and the tool is absent, Extract returns an error.
func (e *Extractor) Extract(ctx context.Context, s scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
	if e.cfg.Mode == config.ModeOff {
		return graph.Facts{}, absentCoverage(), nil
	}

	// Applicability: requires pyproject.toml, setup.py, or cfg.PyPackage directory.
	if !e.isApplicable(s.Root) {
		if e.cfg.Mode == config.ModeOn {
			return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/py: no Python project marker found at %s", s.Root)
		}
		return graph.Facts{}, absentCoverage(), nil
	}

	// Detect uv (preferred) or python3.12.
	tool, version, found := e.detectTool(ctx)
	if !found {
		if e.cfg.Mode == config.ModeOn {
			return graph.Facts{}, diagnostic.Coverage{}, errors.New("extract/py: uv or Python 3.12+ not found; install uv (https://docs.astral.sh/uv/) or Python 3.12+")
		}
		return graph.Facts{}, absentCoverage(), nil
	}

	// Write embedded helper to a temp file.
	tmp, err := os.CreateTemp("", "grimp_helper_*.py")
	if err != nil {
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/py: create temp helper: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) //nolint:errcheck

	if _, err := tmp.Write(grimpHelperSrc); err != nil {
		_ = tmp.Close()
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/py: write temp helper: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/py: close temp helper: %w", err)
	}

	// Determine the package list for grimp. An explicit PyPackage config wins;
	// otherwise discover top-level packages under ScanRoot (dirs with __init__.py).
	// If discovery finds nothing, fall back to the directory name (legacy behaviour).
	var pkgs []string
	if e.cfg.PyPackage != "" {
		pkgs = []string{e.cfg.PyPackage}
	} else {
		pkgs = discoverPackages(s.Root)
		if len(pkgs) == 0 {
			pkgs = []string{filepath.Base(s.Root)}
		}
	}

	// Build the command.
	// grimp_helper --packages pkg1 pkg2 … accepts multiple top-level package names
	// and calls grimp.build_graph(*packages). All packages must be importable from
	// a single Python environment (see discoverPackages doc for the shared-venv
	// constraint). --first-party-packages may be broader than --packages when config
	// pins analysis to one package (for example tests) while other discovered/configured
	// package roots remain first-party and must not become external-system hints.
	firstPartyPkgs := e.firstPartyPackages(s.Root, pkgs)
	helperArgs := append([]string{"--packages"}, pkgs...)
	if len(firstPartyPkgs) > 0 {
		helperArgs = append(helperArgs, "--first-party-packages")
		helperArgs = append(helperArgs, firstPartyPkgs...)
	}
	var cmd toolrun.ToolCmd
	if tool == "uv" {
		// --with grimp injects grimp into the project's venv for this run without
		// modifying pyproject.toml. --directory uses the project's environment so
		// the project's own packages (src-layout etc.) are importable.
		cmd = toolrun.ToolCmd{
			Name:    "uv",
			Args:    append([]string{"run", "--with", "grimp", "--directory", s.Root, tmpName}, helperArgs...),
			WorkDir: s.Root,
			Timeout: runTimeout,
		}
	} else {
		cmd = toolrun.ToolCmd{
			Name:    tool,
			Args:    append([]string{tmpName, "--root", s.Root}, helperArgs...),
			WorkDir: s.Root,
			Timeout: runTimeout,
		}
	}

	out, err := e.cachedRunner(s, tool, version, pkgs, firstPartyPkgs).Run(ctx, cmd)
	if err != nil {
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/py: run helper: %w", err)
	}
	if out.ExitCode != 0 {
		// The helper writes error JSON to stdout; surface that over the raw stderr
		// (which typically contains uv progress lines, not the real error).
		reason := fmt.Sprintf("helper exited %d: %s", out.ExitCode, strings.TrimSpace(string(out.Stderr)))
		var h helperOutput
		if je := json.Unmarshal(out.Stdout, &h); je == nil && h.Error != "" {
			reason = h.Error
		}
		// A helper crash is a coverage gap, not a run-level failure (the "warn-loud,
		// don't block" contract); only an explicitly required analyzer (ModeOn)
		// hard-errors.
		if e.cfg.Mode == config.ModeOn {
			return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/py: %s", reason)
		}
		return graph.Facts{}, diagnostic.Coverage{Tool: toolGrimp, Version: version, Status: statusPartial, Reason: reason}, nil
	}

	facts, cov, err := e.parseAndNormalize(out.Stdout, version)
	if err != nil {
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/py: parse output: %w", err)
	}
	return facts, cov, nil
}

// cachedRunner wraps the runner in a fact-cache decorator for the grimp
// helper invocation (fact-cache.md D5 seam 1). Returns the plain runner when
// the cache is off or key material cannot be derived — never fails the run.
//
// The helper command's argv embeds a random temp path (the extracted helper
// script), so EntryArgs replaces it with the deterministic invocation
// identity: tool + package list. The helper source itself is key material —
// editing grimp_helper.py must invalidate — so its hash rides the config
// slice. Key inputs otherwise: uv version, the ExtractConfig view, the scan
// root, local virtualenv package metadata when present, and the content hash
// of every .py + manifest file under s.Root. The python3.12 fallback is left
// uncached because its global/site environment is outside repo key material.
func (e *Extractor) cachedRunner(s scope.Scope, tool, version string, pkgs, firstPartyPkgs []string) toolrun.Runner {
	if e.Cache == nil {
		return e.runner
	}
	if tool != "uv" {
		return e.runner
	}
	helperSum := sha256.Sum256(grimpHelperSrc)
	resolverStateHash, err := pyResolverStateHash(s.Root)
	if err != nil {
		return e.runner
	}
	cfgHash, err := factcache.HashJSON(struct {
		Cfg               config.ExtractConfig
		Root              string
		HelperHash        string
		ResolverStateHash string
	}{e.cfg, s.Root, hex.EncodeToString(helperSum[:]), resolverStateHash})
	if err != nil {
		return e.runner
	}
	// Config exclusions deliberately do NOT filter the hash: grimp imports the
	// package regardless of `exclude:` globs, so editing an excluded-but-imported
	// file must still invalidate. ListInputs prunes only known non-input cache
	// directories, matching what grimp never reads; exclusion-config changes invalidate via
	// cfgHash (e.cfg embeds Exclusions).
	files := factcache.ListInputs(s.Root, factcache.MatchExts([]string{".py"}, pyManifestNames), nil)
	treeHash, err := factcache.HashTree(s.Root, files)
	if err != nil {
		return e.runner
	}
	return &factcache.Runner{
		Inner:     e.runner,
		Store:     e.Cache,
		Analyzer:  langPython,
		Key:       factcache.Key(langPython, version, cfgHash, treeHash),
		Cacheable: cacheableGrimp,
		EntryArgs: append(append(append([]string{tool, "--packages"}, pkgs...), "--first-party-packages"), firstPartyPkgs...),
	}
}

func pyResolverStateHash(root string) (string, error) {
	files := pyResolverInputs(root)
	if len(files) == 0 {
		return "", nil
	}
	return factcache.HashTree(root, files)
}

func pyResolverInputs(root string) []string {
	var files []string
	for _, dir := range []string{".venv", "venv"} {
		base := filepath.Join(root, dir)
		if _, err := os.Stat(base); err != nil {
			continue
		}
		_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			switch filepath.Base(path) {
			case "METADATA", "RECORD", "direct_url.json", "pyvenv.cfg":
				rel, err := filepath.Rel(root, path)
				if err == nil {
					files = append(files, filepath.ToSlash(rel))
				}
			}
			return nil
		})
	}
	return files
}

// cacheableGrimp vetoes caching output the extractor would report as partial
// (fact-cache.md D3): a non-zero exit, a helper-reported error, or unresolved
// imports. Unresolved imports usually mean the environment is missing a
// dependency — state the cache key cannot see — so caching them would make
// the degradation sticky across a `pip install`.
func cacheableGrimp(out toolrun.Output) bool {
	if out.ExitCode != 0 {
		return false
	}
	var h helperOutput
	if json.Unmarshal(out.Stdout, &h) != nil {
		return false
	}
	return h.Error == "" && h.Unresolved == 0
}

// isApplicable returns true if s.Root contains a Python project marker.
func (e *Extractor) isApplicable(root string) bool {
	for _, marker := range []string{"pyproject.toml", "setup.py"} {
		if _, err := os.Stat(filepath.Join(root, marker)); err == nil {
			return true
		}
	}
	if e.cfg.PyPackage != "" {
		if _, err := os.Stat(filepath.Join(root, e.cfg.PyPackage)); err == nil {
			return true
		}
	}
	return false
}

// discoverPackages returns the sorted list of importable Python package names
// found for root — directories that contain an __init__.py file. Used when no
// explicit PyPackage is configured.
//
// SRC-LAYOUT (PEP 517/518): when root/src/ contains packages, those are returned
// in preference to top-level ones. A src-layout project (e.g. prefect: the package
// lives at src/prefect/) has NO package dir directly under root — a top-level
// __init__.py dir there is typically a stray (tests/, scripts/, benches/), not the
// source. Analysing the stray package instead of the real one silently yields a
// near-empty import graph. The grimp helper adds root/src to sys.path
// (see grimp_helper._ensure_importable), so these names resolve at import time.
//
// SHARED-VENV CONSTRAINT: all discovered packages are passed to a single
// grimp.build_graph call and must therefore be co-importable from one Python
// environment. In a monorepo where each service has its own virtualenv
// (e.g. ~42 isolated services), cross-service coupling cannot be measured
// in one run. This is a grimp limitation; archfit does not promise
// cross-service Python analysis in that setup.
func discoverPackages(root string) []string {
	if srcPkgs := packagesUnder(filepath.Join(root, "src")); len(srcPkgs) > 0 {
		return srcPkgs // src-layout: prefer the real source packages over top-level strays
	}
	return packagesUnder(root)
}

// packagesUnder returns the sorted names of immediate subdirectories of dir that
// contain an __init__.py (i.e. importable Python packages). Returns nil if dir is
// unreadable or absent.
func (e *Extractor) firstPartyPackages(root string, pkgs []string) []string {
	seen := make(map[string]struct{}, len(pkgs)+len(e.cfg.Paths))
	for _, pkg := range pkgs {
		addPythonPackageRoot(seen, pkg)
	}
	for _, pkg := range discoverPackages(root) {
		addPythonPackageRoot(seen, pkg)
	}
	for _, pattern := range e.cfg.Paths {
		if root, ok := pythonPackageRootFromPattern(pattern); ok {
			seen[root] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for pkg := range seen {
		out = append(out, pkg)
	}
	sort.Strings(out)
	return out
}

func addPythonPackageRoot(seen map[string]struct{}, pkg string) {
	if pythonPackageRoot(pkg) {
		seen[pkg] = struct{}{}
	}
}

func pythonPackageRootFromPattern(pattern string) (string, bool) {
	if strings.Contains(pattern, "/") {
		return "", false
	}
	root, _, _ := strings.Cut(pattern, ".")
	if !pythonPackageRoot(root) {
		return "", false
	}
	return root, true
}

func pythonPackageRoot(pkg string) bool {
	if pkg == "" {
		return false
	}
	for i, r := range pkg {
		switch {
		case r == '_':
			continue
		case r >= 'a' && r <= 'z':
			continue
		case r >= 'A' && r <= 'Z':
			continue
		case i > 0 && r >= '0' && r <= '9':
			continue
		default:
			return false
		}
	}
	return true
}

func packagesUnder(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var pkgs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, entry.Name(), "__init__.py")); err == nil {
			pkgs = append(pkgs, entry.Name())
		}
	}
	sort.Strings(pkgs)
	return pkgs
}

// detectTool tries uv then Python 3.12+. Returns (name, version, true) on success.
func (e *Extractor) detectTool(ctx context.Context) (string, string, bool) {
	if info, ok := e.runner.Detect(ctx, "uv"); ok {
		ver := e.toolVersion(ctx, info.Name, []string{"--version"})
		return "uv", ver, true
	}
	for _, name := range []string{"python3.14", "python3.13", "python3.12", "python3", "python"} {
		if info, ok := e.runner.Detect(ctx, name); ok {
			ver := e.toolVersion(ctx, info.Name, []string{"--version"})
			if python3Plus(ver, 12) {
				return info.Name, ver, true
			}
		}
	}
	return "", "", false
}

// python3Plus reports whether the version string describes Python 3.minor where minor ≥ minMinor.
func python3Plus(version string, minMinor int) bool {
	var major, minor int
	if n, _ := fmt.Sscanf(version, "Python %d.%d", &major, &minor); n < 2 {
		return false
	}
	return major == 3 && minor >= minMinor
}

// toolVersion runs tool with versionArgs and returns the trimmed stdout.
// Returns empty string on any failure (non-fatal).
func (e *Extractor) toolVersion(ctx context.Context, tool string, args []string) string {
	out, err := e.runner.Run(ctx, toolrun.ToolCmd{
		Name:    tool,
		Args:    args,
		Timeout: 30 * time.Second,
	})
	if err != nil || out.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(string(out.Stdout))
}

// ---------------------------------------------------------------------------
// JSON parsing types for grimp_helper output.
// ---------------------------------------------------------------------------

type helperOutput struct {
	Edges             []helperEdge `json:"edges"`
	Unresolved        int          `json:"unresolved"`
	UnresolvedImports []helperEdge `json:"unresolved_imports,omitempty"`
	Error             string       `json:"error,omitempty"`
}

type helperEdge struct {
	Importer     string `json:"importer"`
	Imported     string `json:"imported"`
	Line         int    `json:"line"`
	LineContents string `json:"line_contents"`
}

// ---------------------------------------------------------------------------
// Parse + normalise.
// ---------------------------------------------------------------------------

// parseAndNormalize parses grimp_helper JSON and builds graph.Facts.
func (e *Extractor) parseAndNormalize(data []byte, version string) (graph.Facts, diagnostic.Coverage, error) {
	var h helperOutput
	if err := json.Unmarshal(data, &h); err != nil {
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("unmarshal: %w", err)
	}
	if h.Error != "" {
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("grimp: %s", h.Error)
	}

	var nodes []graph.Node
	var edges []graph.Edge
	seenNodes := make(map[string]struct{})

	emitNode := func(dotted string) {
		id := "module:" + dotted
		if _, ok := seenNodes[id]; !ok {
			seenNodes[id] = struct{}{}
			nodes = append(nodes, graph.Node{Kind: graph.NodeKindModule, Path: dotted, Language: graph.LangPython})
		}
	}

	for _, he := range h.Edges {
		emitNode(he.Importer)
		emitNode(he.Imported)

		edgeKind := graph.EdgeKindImports
		if e.matchesInternal(he.Imported) {
			edgeKind = graph.EdgeKindUsesInternal
		}

		// Location file: dotted module name converted to path (dots → slashes).
		locFile := strings.ReplaceAll(he.Importer, ".", "/")

		// Strength hint: intrusive is assigned when the edge reaches into PEP 8-private
		// internals — either via a private module name ("pkg._internal") or via an
		// imported private symbol ("from pkg import _sym"). Both are structural evidence;
		// no naming-heuristic guessing (PascalCase/snake_case) is applied — non-intrusive
		// edges stay abstaining until Task 15 (scip-python symbol kinds).
		//
		// Config public/internal globs still take precedence in classify.
		// We never emit a "contract" hint — grimp resolves imports to the defining
		// submodule, so a public-API signal cannot be established here.
		privateImport := isPrivatePythonModule(he.Imported) || hasPrivateSymbolImport(he.LineContents)
		strengthHint := ""
		if privateImport {
			strengthHint = string(coupling.StrengthIntrusive)
		}
		connascenceDetail := "dotted import"
		if privateImport {
			connascenceDetail = "private import"
		}
		connascenceHints := []graph.ConnascenceHint{{Kind: graph.ConnascenceName, Source: sourceGrimp, Detail: connascenceDetail}}

		edges = append(edges, graph.Edge{
			From:             "module:" + he.Importer,
			To:               "module:" + he.Imported,
			Kind:             edgeKind,
			Language:         langPython,
			Confidence:       "high",
			StrengthHint:     strengthHint,
			ConnascenceHints: connascenceHints,
			Locations: []graph.Location{
				{File: locFile, Line: he.Line},
			},
		})
	}
	for _, he := range h.UnresolvedImports {
		emitNode(he.Importer)
		locFile := strings.ReplaceAll(he.Importer, ".", "/")
		edges = append(edges, graph.Edge{
			From:       "module:" + he.Importer,
			To:         "external:" + he.Imported,
			Kind:       graph.EdgeKindImports,
			Language:   langPython,
			Confidence: "low",
			Locations: []graph.Location{
				{File: locFile, Line: he.Line},
			},
		})
	}

	filesSeen := len(seenNodes)
	covStatus := statusOK
	if h.Unresolved > 0 {
		covStatus = statusPartial
	}
	cov := diagnostic.Coverage{
		Tool:            toolGrimp,
		Version:         version,
		FilesSeen:       filesSeen,
		FilesApplicable: filesSeen,
		Unresolved:      h.Unresolved,
		Status:          covStatus,
	}
	facts := graph.Facts{
		Nodes:      nodes,
		Edges:      edges,
		Language:   langPython,
		Unresolved: h.Unresolved,
	}
	return facts, cov, nil
}

// isPrivatePythonModule reports whether a dotted module name targets a PEP 8
// "internal use" module — any path segment with a single leading underscore
// (e.g. "pkg._internal", "pkg.sub._impl"). Dunder segments (__init__, __main__)
// are package/runtime machinery, not private internals, so they do not count.
func isPrivatePythonModule(dotted string) bool {
	for _, seg := range strings.Split(dotted, ".") {
		if strings.HasPrefix(seg, "_") && !isDunder(seg) {
			return true
		}
	}
	return false
}

// isDunder reports whether a path segment is a __dunder__ name.
func isDunder(seg string) bool {
	return len(seg) >= 4 && strings.HasPrefix(seg, "__") && strings.HasSuffix(seg, "__")
}

// hasPrivateSymbolImport reports whether a Python "from … import …" statement
// imports at least one PEP 8-private symbol (single leading underscore, not dunder).
// Handles multiple imports ("from x import a, _b, c") and aliases ("from x import _sym as s").
// For plain "import x" form there is no symbol name — returns false (module-level rule applies).
//
// Ceiling: multi-line parenthesized imports where line_contents captures only the
// opening physical line ("from x import (\n") are not detected — the function abstains
// safely. Upgrade path: Task 15 (scip-python) resolves individual symbol kinds precisely.
func hasPrivateSymbolImport(line string) bool {
	line = strings.TrimSpace(line)
	// Must be a "from … import …" statement.
	const fromPfx = "from "
	const importKW = " import "
	if !strings.HasPrefix(line, fromPfx) {
		return false
	}
	idx := strings.Index(line, importKW)
	if idx < 0 {
		return false
	}
	symbols := line[idx+len(importKW):]
	// Strip trailing inline comment.
	if i := strings.Index(symbols, "#"); i >= 0 {
		symbols = symbols[:i]
	}
	// Strip surrounding parentheses (single-line form: "from x import (a, _b)").
	symbols = strings.Trim(symbols, " ()")
	for _, sym := range strings.Split(symbols, ",") {
		// Take the original name before any "as alias"; the alias is just a local
		// binding and does not reflect access to a private internal.
		name := sym
		if parts := strings.SplitN(sym, " as ", 2); len(parts) == 2 {
			name = parts[0]
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if strings.HasPrefix(name, "_") && !isDunder(name) {
			return true
		}
	}
	return false
}

// matchesInternal reports whether the dotted module name matches any internal glob.
//
// Python internal: globs are written in DOTTED module form (e.g.
// "myapp.b._internal.*"), the same form used by paths: and by
// classify.classifyStrength. Earlier this converted the module name to slash form,
// which silently disagreed with classifyStrength (it matches the dotted path), so a
// glob could set the uses_internal edge kind without setting strength=intrusive, or
// vice versa. Matching the dotted form here keeps edge-kind and strength consistent.
func (e *Extractor) matchesInternal(dotted string) bool {
	for _, pattern := range e.cfg.Internal {
		if matched, _ := doublestar.Match(pattern, dotted); matched {
			return true
		}
	}
	return false
}

// absentCoverage returns a Coverage record indicating the tool was not found.
func absentCoverage() diagnostic.Coverage {
	return diagnostic.Coverage{
		Tool:   toolGrimp,
		Status: statusAbsent,
	}
}
