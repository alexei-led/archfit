package py

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/relationship/coupling"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const (
	toolGrimp           = "grimp"
	grimpRequirement    = "grimp==3.17"
	pythonVersion       = "3.14"
	langPython          = "python"
	sourceGrimp         = "grimp"
	statusOK            = "ok"
	statusPartial       = "partial"
	statusAbsent        = "absent"
	runTimeout          = 5 * time.Minute
	sourceGapSummaryMax = 3
	sourceGapDetailMax  = 100
)

// Extractor is the Python import extractor using grimp in an isolated tool environment.
// It satisfies the engine.Extractor interface structurally.
type Extractor struct {
	runner toolrun.Runner
	cfg    evidenceports.ExtractConfig
}

// New returns an Extractor configured with the given runner and config.
func New(runner toolrun.Runner, cfg evidenceports.ExtractConfig) *Extractor {
	return &Extractor{runner: runner, cfg: cfg}
}

// Name returns the language identifier for this extractor.
func (e *Extractor) Name() string {
	return langPython
}

// CoverageTool returns the name this extractor stamps on its Coverage rows.
func (e *Extractor) CoverageTool() string {
	return toolGrimp
}

// Extract selects the provisioned interpreter or detects uv/python3.14+grimp,
// writes the embedded helper outside the target, runs it against the project
// root, parses the JSON output, and returns
// graph.Facts + evidence.Coverage.
//
// If mode is off, Extract returns empty Facts and an "absent" Coverage immediately.
// If mode is auto and no applicable tool or Python project is found,
// Extract returns empty Facts and an "absent" Coverage — never an error.
// If mode is on and the tool is absent, Extract returns an error.
func (e *Extractor) Extract(ctx context.Context, s scope.Scope) (graph.Facts, evidence.Coverage, error) {
	if e.cfg.Mode == evidenceports.ModeOff {
		return graph.Facts{}, absentCoverage(), nil
	}

	// Applicability: requires pyproject.toml, setup.py, or cfg.PyPackage directory.
	if !Applicable(s.Root, e.cfg.PyPackage) {
		if e.cfg.Mode == evidenceports.ModeOn {
			return graph.Facts{}, evidence.Coverage{}, fmt.Errorf("extract/py: no Python project marker found at %s", s.Root)
		}
		return graph.Facts{}, absentCoverage(), nil
	}

	// An explicit owned runtime is authoritative: invalid configuration must not
	// silently fall back to uv or a Python discovered from the target environment.
	var tool, version string
	var found bool
	if python, configured := os.LookupEnv(toolPythonEnv); configured {
		var err error
		version, err = e.provisionedPython(ctx, s.Root, python)
		if err != nil {
			return graph.Facts{}, evidence.Coverage{}, fmt.Errorf("extract/py: %w", err)
		}
		tool, found = python, true
	} else {
		tool, version, found = e.detectTool(ctx)
	}
	if !found {
		if e.cfg.Mode == evidenceports.ModeOn {
			return graph.Facts{}, evidence.Coverage{}, errors.New("extract/py: uv or Python 3.14+ not found; install uv (https://docs.astral.sh/uv/) or Python 3.14+")
		}
		return graph.Facts{}, absentCoverage(), nil
	}

	// Write embedded helper to a temp file.
	tmp, err := os.CreateTemp("", "grimp_helper_*.py")
	if err != nil {
		return graph.Facts{}, evidence.Coverage{}, fmt.Errorf("extract/py: create temp helper: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) //nolint:errcheck

	if _, err := tmp.Write(grimpHelperSrc); err != nil {
		_ = tmp.Close()
		return graph.Facts{}, evidence.Coverage{}, fmt.Errorf("extract/py: write temp helper: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return graph.Facts{}, evidence.Coverage{}, fmt.Errorf("extract/py: close temp helper: %w", err)
	}

	// Determine the package list for grimp. An explicit PyPackage config wins;
	// otherwise discover top-level packages under ScanRoot (dirs with __init__.py).
	// If discovery finds nothing, fall back to the directory name (legacy behaviour).
	pkgs := grimpPackages(s.Root, e.cfg.PyPackage)

	// Build the command.
	// grimp_helper --packages pkg1 pkg2 … accepts multiple top-level package names
	// and calls grimp.build_graph(*packages). Source roots must be visible to one
	// invocation (see grimpPackages). --first-party-packages may be broader when config
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
		// No project setup or inherited Python import path may execute target code.
		// The helper exposes only checked source roots after loading the analyzer.
		// --no-config only disables discovery; override UV_CONFIG_FILE explicitly.
		cmd = toolrun.ToolCmd{
			Name:    "uv",
			Args:    append([]string{"run", "--no-config", "--config-file", os.DevNull, "--no-env-file", "--no-project", "--isolated", "--python", pythonVersion, "--with", grimpRequirement, "--directory", filepath.Dir(tmpName), "python", "-I", tmpName, "--root", s.Root}, helperArgs...),
			WorkDir: filepath.Dir(tmpName),
			Env:     isolatedPythonEnv(),
			Timeout: runTimeout,
		}
	} else {
		cmd = toolrun.ToolCmd{
			Name:    tool,
			Args:    append([]string{"-I", tmpName, "--root", s.Root}, helperArgs...),
			WorkDir: filepath.Dir(tmpName),
			Env:     isolatedPythonEnv(),
			Timeout: runTimeout,
		}
	}

	// Python fact caching is intentionally bypassed. The effective environment
	// used by uv (including the resolved grimp and Python versions) is outside
	// the repository tree and cannot be identified reliably before lookup.
	out, err := e.runner.Run(ctx, cmd)
	if err != nil {
		return graph.Facts{}, evidence.Coverage{}, fmt.Errorf("extract/py: run helper: %w", err)
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
		if e.cfg.Mode == evidenceports.ModeOn {
			return graph.Facts{}, evidence.Coverage{}, fmt.Errorf("extract/py: %s", reason)
		}
		return graph.Facts{}, evidence.Coverage{Tool: toolGrimp, Version: version, Status: statusPartial, Reason: reason}, nil
	}

	facts, cov, err := e.parseAndNormalize(out.Stdout, s.Root)
	if err != nil {
		return graph.Facts{}, evidence.Coverage{}, fmt.Errorf("extract/py: parse output: %w", err)
	}
	return facts, cov, nil
}

// Applicable reports whether this extractor has a Python project to analyse at
// root: a pyproject.toml or setup.py in the root itself, or the configured
// languages.python.package directory. setup.cfg is NOT a marker — the extractor
// reports absent over a repo that carries only that.
//
// Exported so the CLI's coverage probe can answer "is this language present?"
// with the extractor's own code instead of a parallel marker list. A probe that
// disagreed turned "the extractor never looked" into "there is nothing here",
// the one absent shape both `analyze --base` and `config compare` read as safely
// comparable — and in the other direction hid a configured package dir behind
// "no Python here".
//
// pkg is evidenceports.ExtractConfig.PyPackage (empty when unset).
func Applicable(root, pkg string) bool {
	for _, marker := range []string{"pyproject.toml", "setup.py"} {
		if _, err := os.Stat(filepath.Join(root, marker)); err == nil {
			return true
		}
	}
	if pkg != "" {
		if _, err := os.Stat(filepath.Join(root, pkg)); err == nil {
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
// near-empty import graph. The helper binds source through static package specs
// (see grimp_helper._ensure_importable), without adding it to sys.path.
//
// All discovered package source roots must be visible to one grimp invocation.
// The helper supports root/package and root/src/package layouts, not separate
// service roots. Runtime dependency installation is not needed.
// grimpPackages is the package list grimp builds: the configured package, else
// the discovered top-level packages, else the root directory's name. Only
// modules of these packages become importers.
func grimpPackages(root, configured string) []string {
	if configured != "" {
		return []string{configured}
	}
	if pkgs := discoverPackages(root); len(pkgs) > 0 {
		return pkgs
	}
	return []string{filepath.Base(root)}
}

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

// detectTool tries uv then Python 3.14+. Returns (name, version, true) on success.
func (e *Extractor) detectTool(ctx context.Context) (string, string, bool) {
	if info, ok := e.runner.Detect(ctx, "uv"); ok {
		ver := e.toolVersion(ctx, info.Name, []string{"--version"})
		return "uv", ver, true
	}
	for _, name := range []string{"python3.14", "python3.15", "python3", "python"} {
		if info, ok := e.runner.Detect(ctx, name); ok {
			ver := e.toolVersion(ctx, info.Name, []string{"--version"})
			if python3Plus(ver, 14) {
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
	ProducerVersion   string         `json:"producer_version"`
	Modules           []string       `json:"modules"`
	Sources           []helperSource `json:"sources"`
	Edges             []helperEdge   `json:"edges"`
	Unresolved        int            `json:"unresolved"`
	UnresolvedImports []helperEdge   `json:"unresolved_imports,omitempty"`
	ExternalImports   []helperEdge   `json:"external_imports,omitempty"`
	Error             string         `json:"error,omitempty"`
}

type helperSource struct {
	File   string `json:"file"`
	Module string `json:"module"`
	Parsed bool   `json:"parsed"`
}

type helperEdge struct {
	Importer     string `json:"importer"`
	Imported     string `json:"imported"`
	Line         int    `json:"line"`
	LineContents string `json:"line_contents"`
	Literal      bool   `json:"literal,omitempty"`
	File         string `json:"file,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

// ---------------------------------------------------------------------------
// Parse + normalise.
// ---------------------------------------------------------------------------

// parseAndNormalize parses grimp_helper JSON and builds graph.Facts.
func (e *Extractor) parseAndNormalize(data []byte, root string) (graph.Facts, evidence.Coverage, error) {
	var h helperOutput
	if err := json.Unmarshal(data, &h); err != nil {
		return graph.Facts{}, evidence.Coverage{}, fmt.Errorf("unmarshal: %w", err)
	}
	if h.Error != "" {
		return graph.Facts{}, evidence.Coverage{}, fmt.Errorf("grimp: %s", h.Error)
	}
	// Only a located, explicitly excluded file proves a gap out of scope.
	// Unlocated walk/detail failures remain missing evidence.
	kept := h.UnresolvedImports[:0]
	for _, gap := range h.UnresolvedImports {
		if h.Unresolved > 0 && e.excludedSource(gap.File) {
			h.Unresolved--
			continue
		}
		kept = append(kept, gap)
	}
	h.UnresolvedImports = kept
	filesSeen, filesApplicable := 0, 0
	excludedModules, includedModules := map[string]bool{}, map[string]bool{}
	for _, source := range h.Sources {
		if e.excludedSource(source.File) {
			excludedModules[source.Module] = true
			continue
		}
		includedModules[source.Module] = true
		filesApplicable++
		if source.Parsed {
			filesSeen++
		}
	}
	for module := range includedModules {
		delete(excludedModules, module)
	}

	var nodes []graph.Node
	var edges []graph.Edge
	seenNodes := make(map[string]struct{})
	locationFiles := pythonSourceLocations(root, h, e.cfg.Exclusions)

	emitNode := func(dotted string) {
		id := "module:" + dotted
		if _, ok := seenNodes[id]; !ok {
			seenNodes[id] = struct{}{}
			nodes = append(nodes, graph.Node{Kind: graph.NodeKindModule, Path: dotted, Language: graph.LangPython})
		}
	}

	for _, module := range h.Modules {
		if !excludedModules[module] {
			emitNode(module)
		}
	}
	for _, he := range h.Edges {
		if excludedModules[he.Importer] {
			continue
		}
		emitNode(he.Importer)
		if !he.Literal && !excludedModules[he.Imported] {
			emitNode(he.Imported)
		}

		edgeKind := ImportEdgeKind(e.cfg.Internal, he.Imported)

		// Strength hint: intrusive is assigned when the edge reaches into PEP 8-private
		// internals — either via a private module name ("pkg._internal") or via an
		// imported private symbol ("from pkg import _sym"). Both are structural evidence;
		// no naming-heuristic guessing (PascalCase/snake_case) is applied — non-intrusive
		// edges stay abstaining until Task 15 (scip-python symbol kinds).
		//
		// Config public/internal globs still take precedence in classify.
		// We never emit a "contract" hint — grimp resolves imports to the defining
		// submodule, so a public-API signal cannot be established here.
		privateImport := !he.Literal && (isPrivatePythonModule(he.Imported) || hasPrivateSymbolImport(he.LineContents))
		confidence := "high"
		if he.Literal {
			confidence = "low"
		}
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
			Confidence:       confidence,
			StrengthHint:     strengthHint,
			ConnascenceHints: connascenceHints,
			Locations:        pythonEdgeLocations(locationFiles, he),
		})
	}
	for _, he := range h.ExternalImports {
		if excludedModules[he.Importer] {
			continue
		}
		emitNode(he.Importer)
		edges = append(edges, graph.Edge{
			From:       "module:" + he.Importer,
			To:         "external:" + he.Imported,
			Kind:       graph.EdgeKindImports,
			Language:   langPython,
			Confidence: "low",
			Locations:  pythonEdgeLocations(locationFiles, he),
		})
	}

	covStatus := statusOK
	covReason := ""
	if h.Unresolved > 0 {
		covStatus = statusPartial
		covReason = grimpUnresolvedReason(h.Unresolved, h.UnresolvedImports)
	}
	cov := evidence.Coverage{
		Tool:                    toolGrimp,
		Version:                 h.ProducerVersion,
		FilesSeen:               filesSeen,
		FilesApplicable:         filesApplicable,
		Unresolved:              h.Unresolved,
		UnresolvedInputsMissing: h.Unresolved,
		Status:                  covStatus,
		Reason:                  covReason,
	}
	facts := graph.Facts{
		Nodes:      nodes,
		Edges:      edges,
		Language:   langPython,
		Unresolved: h.Unresolved,
	}
	return facts, cov, nil
}

func (e *Extractor) excludedSource(file string) bool {
	return file != "" && filepath.IsLocal(filepath.FromSlash(file)) && filepath.Ext(file) == ".py" && pythonLocationExcluded(file, e.cfg.Exclusions)
}

func grimpUnresolvedReason(unresolved int, gaps []helperEdge) string {
	details := make(map[string]struct{})
	for _, gap := range gaps {
		detail := gap.File
		if detail == "" {
			detail = gap.Imported
		}
		if gap.Reason != "" {
			detail += ": " + gap.Reason
		}
		if detail != "" {
			runes := []rune(detail)
			if len(runes) > sourceGapDetailMax {
				detail = string(runes[:sourceGapDetailMax]) + "…"
			}
			details[detail] = struct{}{}
		}
	}
	parts := make([]string, 0, len(details))
	for detail := range details {
		parts = append(parts, detail)
	}
	sort.Strings(parts)
	if len(parts) > sourceGapSummaryMax {
		parts = append(parts[:sourceGapSummaryMax], "…")
	}
	reason := fmt.Sprintf("%d import/source gaps", unresolved)
	if len(parts) > 0 {
		reason += ": " + strings.Join(parts, "; ")
	}
	return reason + " — check selected source roots, readability and interpreter syntax support"
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

// ImportEdgeKind is the kind of an import edge to the dotted module
// imported: uses_internal when a configured internal glob matches it, else
// imports. The extractor and `archfit policy can-import` share it, so a
// queried edge keys its findings exactly as the extracted one does.
//
// Python internal: globs are written in DOTTED module form (e.g.
// "myapp.b._internal.*"), the same form used by paths: and by
// classify.classifyStrength. Earlier this converted the module name to slash form,
// which silently disagreed with classifyStrength (it matches the dotted path), so a
// glob could set the uses_internal edge kind without setting strength=intrusive, or
// vice versa. Matching the dotted form here keeps edge-kind and strength consistent.
func ImportEdgeKind(internal []string, imported string) graph.EdgeKind {
	for _, pattern := range internal {
		if matched, _ := doublestar.Match(pattern, imported); matched {
			return graph.EdgeKindUsesInternal
		}
	}
	return graph.EdgeKindImports
}

// absentCoverage returns a Coverage record indicating the tool was not found.
func absentCoverage() evidence.Coverage {
	return evidence.Coverage{
		Tool:   toolGrimp,
		Status: statusAbsent,
	}
}
