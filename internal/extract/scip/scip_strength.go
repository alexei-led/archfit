package scip

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	indexTimeout  = 10 * time.Minute
	readerTimeout = 5 * time.Minute
	pyInitFile    = "__init__.py"
	pySrcDir      = "src"

	// defaultTimeout is the per-analyzer outer watchdog when tools.scip.timeout
	// is not configured. Generous relative to per-subprocess timeouts (index 10m +
	// reader 5m = 15m sum) to guard only against pathological hangs.
	defaultTimeout = 20 * time.Minute

	indexerPython = "scip-python"
	indexerGo     = "scip-go"
	indexerTS     = "scip-typescript"
	indexerRust   = "rust-analyzer"
	flagOutput    = "--output"
	langRust      = "rust"
	toolCargo     = "cargo"

	nodeModulesDir = "node_modules"

	// Absent-coverage reasons: why semantic strength is unavailable and the
	// actionable enable step. Static strings so a double-run stays byte-stable.
	reasonScipNoIndexer   = "no SCIP indexer found — install scip-go, scip-typescript, scip-python, or rust-analyzer for semantic integration strength"
	reasonScipNoUv        = "uv not found — install uv (https://astral.sh/uv) so archfit can read the SCIP index"
	reasonTSNoNodeModules = "install JS/TS dependencies (e.g. `npm install`) for semantic strength — scip-typescript needs node_modules to resolve cross-package imports"
	reasonTimedOut        = "SCIP analysis timed out — increase tools.scip.timeout or reduce the scope"
)

// readerOutput mirrors the JSON emitted by scip_reader.py.
type readerOutput struct {
	Edges []struct {
		From     string `json:"from"`
		To       string `json:"to"`
		Strength string `json:"strength"`
	} `json:"edges"`
	Symbols []struct {
		Symbol string `json:"symbol"`
		Path   string `json:"path"`
		Module string `json:"module"`
		FanIn  int    `json:"fan_in"`
	} `json:"symbols"`
	SymbolRefs []struct {
		FromSymbol string `json:"from_symbol"`
		ToSymbol   string `json:"to_symbol"`
	} `json:"symbol_refs"`
	IntraRefs []struct {
		FromSymbol string `json:"from_symbol"`
		ToSymbol   string `json:"to_symbol"`
	} `json:"intra_refs"`
	Error string `json:"error,omitempty"`
}

// Strengths runs a SCIP indexer over the project, reads the index, and returns
// symbol-level integration strength per cross-module edge, keyed by
// "<fromModulePath>\x00<toModulePath>". A missing toolchain (no indexer, no uv) or
// any non-fatal failure yields an empty map with an absent/partial coverage record,
// never an error — strength enrichment is best-effort on top of the import graph.
func (a *Adapter) Strengths(ctx context.Context, s scope.Scope) (map[string]string, diagnostic.Coverage, error) {
	ro, partial, ok := a.runSCIPPipeline(ctx, s.Root, toolName)
	if !ok {
		return nil, partial, nil
	}
	m, perr := parseReaderEdges(ro.raw)
	if perr != nil {
		return nil, partial, nil
	}
	return m, diagnostic.Coverage{
		Tool:            toolName,
		Version:         ro.indexer,
		FilesSeen:       len(m),
		FilesApplicable: len(m),
		Status:          diagnostic.StatusOK,
	}, nil
}

// pipelineResult holds the raw reader output and metadata from runSCIPPipeline.
type pipelineResult struct {
	raw     []byte
	indexer string
}

// pipeCacheEntry memoizes one pipeline execution per project root so that
// Strengths and Symbols share a single index+read pass instead of indexing the
// repo twice per run. cov.Tool is a template — rewrapped per caller.
type pipeCacheEntry struct {
	ro  pipelineResult
	cov diagnostic.Coverage
	ok  bool
}

// runSCIPPipeline runs the detect → index → read pipeline shared by Strengths and
// Symbols, memoized per root for the Adapter's lifetime (one engine run). On any
// non-fatal failure it returns ok=false and a partial/absent coverage record. The
// caller is responsible for parsing ro.raw into its own typed result and
// constructing a final Coverage with the correct tool name.
func (a *Adapter) runSCIPPipeline(ctx context.Context, root, covTool string) (pipelineResult, diagnostic.Coverage, bool) {
	a.pipeMu.Lock()
	entry, hit := a.pipeCache[root]
	a.pipeMu.Unlock()
	if !hit {
		entry.ro, entry.cov, entry.ok = a.runSCIPPipelineUncached(ctx, root)
		a.pipeMu.Lock()
		if a.pipeCache == nil {
			a.pipeCache = make(map[string]pipeCacheEntry, 1)
		}
		a.pipeCache[root] = entry
		a.pipeMu.Unlock()
	}
	cov := entry.cov
	cov.Tool = covTool
	return entry.ro, cov, entry.ok
}

// runSCIPPipelineUncached executes the actual detect → index → read pipeline.
// The returned Coverage carries Status/Version only; Tool is set by the wrapper.
func (a *Adapter) runSCIPPipelineUncached(ctx context.Context, root string) (ro pipelineResult, cov diagnostic.Coverage, ok bool) {
	absent := diagnostic.Coverage{Status: diagnostic.StatusAbsent}

	indexer, pkg, lang, found := a.detectIndexer(ctx, root)
	if !found {
		absent.Reason = scipAbsentReason(root)
		return ro, absent, false
	}
	// The reader runs via uv (PEP 723 inline deps: protobuf + grpcio-tools).
	if _, found := a.runner.Detect(ctx, "uv"); !found {
		absent.Reason = reasonScipNoUv
		return ro, absent, false
	}
	// scip-typescript resolves imports through node_modules; without installed
	// deps it indexes nothing useful, so surface the fix instead of silently
	// returning empty (the codegraph baseline: indexer present, deps absent).
	if lang == "typescript" && !dirExists(filepath.Join(root, nodeModulesDir)) {
		absent.Reason = reasonTSNoNodeModules
		return ro, absent, false
	}

	// Apply per-analyzer watchdog before any subprocess call. The outer timeout
	// caps the full index+read pipeline. Per-subprocess timeouts (indexTimeout,
	// readerTimeout) still apply inside runner.Run; this guard catches pathological
	// hangs where a subprocess ignores its own limit.
	ctx, cancel := toolrun.WithWatchdog(ctx, a.timeout, defaultTimeout)
	defer cancel()

	timedOut := diagnostic.Coverage{Version: indexer, Status: diagnostic.StatusTimedOut, Reason: reasonTimedOut}

	tmp, err := os.MkdirTemp("", "archfit-scip-")
	if err != nil {
		return ro, absent, false
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	protoPath := filepath.Join(tmp, "scip.proto")
	readerPath := filepath.Join(tmp, "scip_reader.py")
	indexPath := filepath.Join(tmp, "index.scip")
	if os.WriteFile(protoPath, scipProtoSrc, 0o600) != nil ||
		os.WriteFile(readerPath, scipReaderSrc, 0o600) != nil {
		return ro, absent, false
	}

	partial := diagnostic.Coverage{Version: indexer, Status: diagnostic.StatusPartial}

	// Index the project (the indexer runs in the project root, output to temp).
	idxOut, err := a.runner.Run(ctx, toolrun.ToolCmd{
		Name:    indexer,
		Args:    indexArgs(indexer, pkg, root, indexPath),
		WorkDir: root,
		Timeout: indexTimeout,
	})
	if err != nil || idxOut.ExitCode != 0 {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ro, timedOut, false
		}
		return ro, partial, false
	}
	if _, statErr := os.Stat(indexPath); statErr != nil {
		return ro, partial, false
	}

	// Read: uv run scip_reader.py --proto <p> --index <i> --package <pkg> --lang <lang>
	rdOut, err := a.runner.Run(ctx, toolrun.ToolCmd{
		Name:    "uv",
		Args:    []string{"run", readerPath, "--proto", protoPath, "--index", indexPath, "--package", pkg, "--lang", lang},
		WorkDir: tmp,
		Timeout: readerTimeout,
	})
	if err != nil || rdOut.ExitCode != 0 {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ro, timedOut, false
		}
		return ro, partial, false
	}

	return pipelineResult{raw: rdOut.Stdout, indexer: indexer}, partial, true
}

// parseReaderEdges parses scip_reader.py JSON into a strength map keyed by
// "<fromModulePath>\x00<toModulePath>" — the same key the engine builds from a
// graph edge's stripped from/to paths. A helper-reported error fails the parse.
func parseReaderEdges(stdout []byte) (map[string]string, error) {
	var ro readerOutput
	if err := json.Unmarshal(stdout, &ro); err != nil {
		return nil, err
	}
	if ro.Error != "" {
		return nil, errReader(ro.Error)
	}
	m := make(map[string]string, len(ro.Edges))
	for _, e := range ro.Edges {
		m[e.From+"\x00"+e.To] = e.Strength
	}
	return m, nil
}

// errReader is a helper error type carrying the reader's reported message.
type errReader string

func (e errReader) Error() string { return "scip reader: " + string(e) }

// detectIndexer picks a SCIP indexer for the project's language and the root
// package/module name used for internal-symbol filtering and the reader --lang.
// One reader handles all three (the .scip format is language-agnostic); only the
// indexer binary and the root identifier differ.
func (a *Adapter) detectIndexer(ctx context.Context, root string) (indexer, pkg, lang string, ok bool) {
	if fileExists(filepath.Join(root, "pyproject.toml")) || fileExists(filepath.Join(root, "setup.py")) {
		if _, found := a.runner.Detect(ctx, indexerPython); found {
			if p := detectPyPackage(root); p != "" {
				return indexerPython, p, "python", true
			}
		}
	}
	if fileExists(filepath.Join(root, "go.mod")) {
		if _, found := a.runner.Detect(ctx, indexerGo); found {
			if m := goModulePath(root); m != "" {
				return indexerGo, m, "go", true
			}
		}
	}
	if fileExists(filepath.Join(root, "package.json")) {
		if _, found := a.runner.Detect(ctx, indexerTS); found {
			if n := npmPackageName(root); n != "" {
				return indexerTS, n, "typescript", true
			}
		}
	}
	if fileExists(filepath.Join(root, "Cargo.toml")) {
		if _, found := a.runner.Detect(ctx, indexerRust); found {
			if n := cargoPackageName(root); n != "" {
				// Single-package crate: pass the name directly.
				return indexerRust, n, langRust, true
			}
			// Virtual workspace ([workspace] with no [package]): enumerate members
			// via cargo metadata and pass them comma-joined so the reader can build
			// the _is_internal membership set.
			if members := a.cargoWorkspaceMembers(ctx, root); len(members) > 0 {
				return indexerRust, strings.Join(members, ","), langRust, true
			}
		}
	}
	return "", "", "", false
}

// cargoWorkspaceMembers runs `cargo metadata --no-deps --format-version 1` in root
// and returns the names of all workspace member packages. Returns nil on any failure.
// Used for virtual workspaces (no [package] in root Cargo.toml) so detectIndexer
// can build a comma-separated package list for the SCIP reader.
func (a *Adapter) cargoWorkspaceMembers(ctx context.Context, root string) []string {
	out, err := a.runner.Run(ctx, toolrun.ToolCmd{
		Name:    toolCargo,
		Args:    []string{"metadata", "--no-deps", "--format-version", "1"},
		WorkDir: root,
		Timeout: 60 * time.Second,
	})
	if err != nil || out.ExitCode != 0 {
		return nil
	}
	var meta struct {
		Packages []struct {
			Name string `json:"name"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(out.Stdout, &meta); err != nil {
		return nil
	}
	// With --no-deps, `packages` already contains ONLY workspace members (external
	// dependencies are excluded), so every package name is a member. This avoids
	// parsing the workspace_members package-IDs, whose format changed in cargo 1.96
	// to "path+file:///…/<name>#<version>" (no spaces) — the old space-split parser
	// produced an empty set there, which silently disabled SCIP on every workspace.
	names := make([]string, 0, len(meta.Packages))
	for _, p := range meta.Packages {
		names = append(names, p.Name)
	}
	return names
}

// indexArgs returns the per-indexer command arguments to write an index to out.
func indexArgs(indexer, pkg, root, out string) []string {
	switch indexer {
	case indexerGo:
		// scip-go runs in the module root and auto-detects the module.
		return []string{flagOutput, out}
	case indexerTS:
		return []string{"index", flagOutput, out}
	case indexerRust:
		// rust-analyzer scip REQUIRES the project path as a positional arg; with only
		// --output it exits 0 in milliseconds and writes nothing. WorkDir is the root,
		// so pass ".". (Omitting it was why Rust SCIP silently produced no index.)
		return []string{"scip", ".", flagOutput, out}
	default: // scip-python
		return []string{"index", "--project-name", pkg, "--cwd", root, flagOutput, out, "--quiet"}
	}
}

// goModulePath reads the module path from go.mod ("module github.com/x/y" → that path).
func goModulePath(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod")) // #nosec G304 -- root is the repository root already chosen by discovery
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if rest, found := strings.CutPrefix(line, "module "); found {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// cargoPackageName reads the package name from Cargo.toml's [package] table
// (name = "x" → "x"). A virtual-workspace manifest (no [package]) yields "",
// which makes detectIndexer skip rust-analyzer — strength stays graph-only.
func cargoPackageName(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "Cargo.toml")) //nolint:gosec
	if err != nil {
		return ""
	}
	inPackage := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inPackage = line == "[package]"
			continue
		}
		if !inPackage {
			continue
		}
		if key, val, found := strings.Cut(line, "="); found && strings.TrimSpace(key) == "name" {
			return strings.Trim(strings.TrimSpace(val), `"'`)
		}
	}
	return ""
}

// npmPackageName reads the "name" field from package.json.
func npmPackageName(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "package.json")) // #nosec G304 -- root is the repository root already chosen by discovery
	if err != nil {
		return ""
	}
	var pkg struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return ""
	}
	return pkg.Name
}

// detectPyPackage returns the first top-level Python package directory name found
// at root or root/src (matching initcfg's discovery).
func detectPyPackage(root string) string {
	for _, dir := range []string{root, filepath.Join(root, pySrcDir)} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "_") {
				continue
			}
			if fileExists(filepath.Join(dir, e.Name(), pyInitFile)) {
				return e.Name()
			}
		}
	}
	return ""
}

// fileExists reports whether path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// dirExists reports whether path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// scipAbsentReason picks the most actionable reason when no SCIP indexer was
// detected. A TS project with no installed deps is the dominant, most common
// blocker (the codegraph baseline); otherwise a generic install-an-indexer hint.
func scipAbsentReason(root string) string {
	if fileExists(filepath.Join(root, "package.json")) && !dirExists(filepath.Join(root, nodeModulesDir)) {
		return reasonTSNoNodeModules
	}
	return reasonScipNoIndexer
}
