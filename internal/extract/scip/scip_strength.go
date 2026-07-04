package scip

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	indexTimeout  = 10 * time.Minute
	readerTimeout = 5 * time.Minute
	pyInitFile    = "__init__.py"
	pySrcDir      = "src"

	// defaultTimeout is the per-analyzer outer watchdog when analyzers.scip.timeout
	// is not configured. Generous relative to per-subprocess timeouts (index 10m +
	// reader 5m = 15m sum) to guard only against pathological hangs.
	defaultTimeout = 20 * time.Minute

	indexerPython = "scip-python"
	indexerGo     = "scip-go"
	indexerTS     = "scip-typescript"
	indexerRust   = "rust-analyzer"
	flagOutput    = "--output"
	langRust      = "rust"
	langPy        = "python"
	langTS        = "typescript"
	toolCargo     = "cargo"

	nodeModulesDir    = "node_modules"
	manifestPkgJSON   = "package.json"
	manifestPyproject = "pyproject.toml"

	// Absent-coverage reasons: why semantic strength is unavailable and the
	// actionable enable step. Static strings so a double-run stays byte-stable.
	reasonScipNoIndexer   = "no SCIP indexer found — install scip-go, scip-typescript, scip-python, or rust-analyzer for semantic integration strength"
	reasonScipNoUv        = "uv not found — install uv (https://astral.sh/uv) so archfit can read the SCIP index"
	reasonTSNoNodeModules = "install JS/TS dependencies (e.g. `npm install`) for semantic strength — scip-typescript needs node_modules to resolve cross-package imports"
	reasonTimedOut        = "SCIP analysis timed out — increase analyzers.scip.timeout or reduce the scope"
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
	if len(m) == 0 {
		// Distinguish a valid index with no cross-module edges (e.g. a single
		// package or all imports are intra-module) from a failed/empty index.
		// Parse the raw output once more to check whether any symbols were
		// indexed — if symbols exist the run succeeded, 0 edges is real (C8).
		var out readerOutput
		_ = json.Unmarshal(ro.raw, &out)
		if len(out.Symbols) == 0 {
			return m, diagnostic.Coverage{
				Tool:    toolName,
				Version: ro.indexer,
				Status:  diagnostic.StatusPartial,
				Reason:  "empty index (0 occurrences) — check path case / indexer version",
			}, nil
		}
		// Index is valid but has no cross-module edges — return OK with 0 files.
		return m, diagnostic.Coverage{
			Tool:    toolName,
			Version: ro.indexer,
			Status:  diagnostic.StatusOK,
		}, nil
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

	// Apply per-analyzer watchdog before any subprocess call. detectIndexer may
	// run `cargo metadata` for Rust virtual workspaces; that call must be
	// covered by the outer deadline. Per-subprocess timeouts (indexTimeout,
	// readerTimeout) still apply inside runner.Run; this guard catches
	// pathological hangs where a subprocess ignores its own limit.
	ctx, cancel := toolrun.WithWatchdog(ctx, a.timeout, defaultTimeout)
	defer cancel()

	indexer, pkg, lang, found, detectErr := a.detectIndexer(ctx, root)
	if !found {
		// report StatusTimedOut rather than StatusAbsent so the caller knows the
		// tool was present but the detection phase hit a time limit.
		if errors.Is(detectErr, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ro, diagnostic.Coverage{Status: diagnostic.StatusTimedOut, Reason: reasonTimedOut}, false
		}
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
	if lang == langTS && !dirExists(filepath.Join(root, nodeModulesDir)) {
		absent.Reason = reasonTSNoNodeModules
		return ro, absent, false
	}

	// Fact-cache lookup: the reader output is the durable fact. Placed AFTER
	// the environment checks above so a degraded toolchain reports identically
	// warm and cold. Key "" ⇒ cache off or key material underivable. The
	// returned Coverage matches the miss path's success return (the partial
	// template — callers construct their own final Coverage from ro).
	key := a.cacheKey(ctx, root, indexer, pkg, lang)
	if key != "" {
		if blob, hit := a.Cache.Get(toolName, key); hit {
			var ce scipCacheEntry
			if json.Unmarshal(blob, &ce) == nil && ce.Indexer == indexer {
				return pipelineResult{raw: ce.Raw, indexer: indexer},
					diagnostic.Coverage{Version: indexer, Status: diagnostic.StatusPartial}, true
			}
		}
	}

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

	// innerTimeout returns the configured per-analyzer timeout when set, else
	// the built-in constant. This lets analyzers.scip.timeout extend the per-phase
	// inner cap beyond the fixed default (e.g. 10m index, 5m reader).
	innerTimeout := func(builtin time.Duration) time.Duration {
		if a.timeout > 0 {
			return a.timeout
		}
		return builtin
	}

	// Index the project (the indexer runs in the project root, output to temp).
	idxOut, err := a.runner.Run(ctx, toolrun.ToolCmd{
		Name:    indexer,
		Args:    indexArgs(indexer, pkg, root, indexPath),
		WorkDir: root,
		Timeout: innerTimeout(indexTimeout),
	})
	if err != nil || idxOut.ExitCode != 0 {
		// Check both the inner per-subprocess deadline (err) and the outer
		// watchdog (ctx.Err()). When the configured timeout equals the inner cap
		// they may fire in either order.
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
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
		Timeout: innerTimeout(readerTimeout),
	})
	if err != nil || rdOut.ExitCode != 0 {
		// Check both the inner per-subprocess deadline (err) and the outer
		// watchdog (ctx.Err()). When the configured timeout equals the inner cap
		// they may fire in either order.
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ro, timedOut, false
		}
		return ro, partial, false
	}

	// Cache the reader output — only a usable index (symbols or edges present,
	// no reader error): an empty index usually means an indexer/environment
	// failure, and caching it would pin the degradation (fact-cache.md D3).
	// Timed-out and non-zero-exit runs returned above and are never cached.
	if key != "" && cacheableSCIP(rdOut.Stdout) {
		if blob, merr := json.Marshal(scipCacheEntry{Indexer: indexer, Raw: rdOut.Stdout}); merr == nil {
			a.Cache.Put(toolName, key, blob)
		}
	}

	return pipelineResult{raw: rdOut.Stdout, indexer: indexer}, partial, true
}

// scipCacheEntry is the stored fact-cache envelope: the reader JSON plus the
// indexer that produced it (echoed into Coverage.Version on a hit).
type scipCacheEntry struct {
	Indexer string `json:"indexer"`
	Raw     []byte `json:"raw"`
}

// cacheableSCIP reports whether the reader output is a usable fact worth
// caching: it parses, reports no error, and indexed something.
func cacheableSCIP(stdout []byte) bool {
	var out readerOutput
	if json.Unmarshal(stdout, &out) != nil || out.Error != "" {
		return false
	}
	return len(out.Symbols) > 0 || len(out.Edges) > 0
}

// scipLangInputs maps the detected language to its input scope for the
// fact-cache key: source extensions plus resolution-affecting manifests.
// The indexers resolve symbols against installed dependencies (node_modules,
// site-packages), so lockfiles stand in for that environment — a
// lockfile-only bump must invalidate (over-hash, never under-hash).
var scipLangInputs = map[string]struct {
	exts      []string
	basenames []string
}{
	"go": {[]string{".go"}, []string{"go.mod", "go.sum"}},
	langTS: {[]string{".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs"}, []string{
		manifestPkgJSON, "tsconfig.json", "tsconfig.base.json",
		"package-lock.json", "yarn.lock", "pnpm-lock.yaml", "bun.lock", "bun.lockb",
	}},
	langPy:   {[]string{".py"}, []string{manifestPyproject, "setup.py", "setup.cfg", "uv.lock", "requirements.txt"}},
	langRust: {[]string{".rs"}, []string{"Cargo.toml", "Cargo.lock"}},
}

// cacheKey derives the fact-cache key for one index+read pass, or "" when the
// cache is off or key material cannot be derived (never an error). Key
// inputs: indexer name+version probe, the reader/proto sources (editing
// scip_reader.py must invalidate), the package/lang the reader filtered on,
// the scan root, and the content hash of the detected language's source tree.
func (a *Adapter) cacheKey(ctx context.Context, root, indexer, pkg, lang string) string {
	if a.Cache == nil {
		return ""
	}
	in, ok := scipLangInputs[lang]
	if !ok {
		return ""
	}
	readerSum := sha256.Sum256(scipReaderSrc)
	protoSum := sha256.Sum256(scipProtoSrc)
	cfgHash, err := factcache.HashJSON(struct {
		Root, Pkg, Lang, Reader, Proto string
	}{root, pkg, lang, hex.EncodeToString(readerSum[:]), hex.EncodeToString(protoSum[:])})
	if err != nil {
		return ""
	}
	files := factcache.ListInputs(root, factcache.MatchExts(in.exts, in.basenames), nil)
	treeHash, err := factcache.HashTree(root, files)
	if err != nil {
		return ""
	}
	return factcache.Key(toolName, indexer+"\x00"+a.indexerVersion(ctx, indexer), cfgHash, treeHash)
}

// indexerVersion probes `<indexer> --version`. Best-effort: "" on any failure
// (some indexers may not support the flag; the indexer NAME is always in the
// key, so a probe gap only weakens upgrade invalidation, never correctness of
// content keying).
func (a *Adapter) indexerVersion(ctx context.Context, indexer string) string {
	out, err := a.runner.Run(ctx, toolrun.ToolCmd{
		Name:    indexer,
		Args:    []string{"--version"},
		Timeout: 30 * time.Second,
	})
	if err != nil || out.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(string(out.Stdout))
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
// Returns a non-nil err only when a cargo metadata timeout fires (inner cap or
// outer ctx), so the caller can surface StatusTimedOut rather than StatusAbsent.
func (a *Adapter) detectIndexer(ctx context.Context, root string) (indexer, pkg, lang string, ok bool, err error) {
	if fileExists(filepath.Join(root, manifestPyproject)) || fileExists(filepath.Join(root, "setup.py")) {
		if _, found := a.runner.Detect(ctx, indexerPython); found {
			if p := detectPyPackage(root); p != "" {
				return indexerPython, p, langPy, true, nil
			}
		}
	}
	if fileExists(filepath.Join(root, "go.mod")) {
		if _, found := a.runner.Detect(ctx, indexerGo); found {
			if m := goModulePath(root); m != "" {
				return indexerGo, m, "go", true, nil
			}
		}
	}
	if fileExists(filepath.Join(root, manifestPkgJSON)) {
		if _, found := a.runner.Detect(ctx, indexerTS); found {
			if n := npmPackageName(root); n != "" {
				return indexerTS, n, langTS, true, nil
			}
		}
	}
	if fileExists(filepath.Join(root, "Cargo.toml")) {
		if _, found := a.runner.Detect(ctx, indexerRust); found {
			if n := cargoPackageName(root); n != "" {
				// Single-package crate: pass the name directly.
				return indexerRust, n, langRust, true, nil
			}
			// Virtual workspace ([workspace] with no [package]): enumerate members
			// via cargo metadata and pass them comma-joined so the reader can build
			// the _is_internal membership set.
			members, membersErr := a.cargoWorkspaceMembers(ctx, root)
			if errors.Is(membersErr, context.DeadlineExceeded) {
				return "", "", "", false, context.DeadlineExceeded
			}
			if len(members) > 0 {
				return indexerRust, strings.Join(members, ","), langRust, true, nil
			}
		}
	}
	return "", "", "", false, nil
}

// cargoWorkspaceMembers runs `cargo metadata --no-deps --format-version 1` in root
// and returns the names of all workspace member packages. Returns nil on any failure.
// Returns context.DeadlineExceeded when the inner cap fires (so detectIndexer can
// propagate StatusTimedOut rather than silently degrading to StatusAbsent).
// The inner cap honours analyzers.scip.timeout when set; floor is 60 s.
// Used for virtual workspaces (no [package] in root Cargo.toml) so detectIndexer
// can build a comma-separated package list for the SCIP reader.
func (a *Adapter) cargoWorkspaceMembers(ctx context.Context, root string) ([]string, error) {
	// Floor: 60 s. Honour configured timeout so a larger value can extend detection.
	inner := 60 * time.Second
	if a.timeout > 0 {
		inner = a.timeout
	}
	out, err := a.runner.Run(ctx, toolrun.ToolCmd{
		Name:    toolCargo,
		Args:    []string{"metadata", "--no-deps", "--format-version", "1"},
		WorkDir: root,
		Timeout: inner,
	})
	if err != nil || out.ExitCode != 0 {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, context.DeadlineExceeded
		}
		return nil, nil
	}
	var meta struct {
		Packages []struct {
			Name string `json:"name"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(out.Stdout, &meta); err != nil {
		return nil, nil
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
	return names, nil
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
	data, err := os.ReadFile(filepath.Join(root, manifestPkgJSON)) // #nosec G304 -- root is the repository root already chosen by discovery
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

// detectPyPackage returns the first top-level Python package directory name found.
// For src-layout projects, root/src wins over top-level stray packages.
func detectPyPackage(root string) string {
	for _, dir := range []string{filepath.Join(root, pySrcDir), root} {
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
	if fileExists(filepath.Join(root, manifestPkgJSON)) && !dirExists(filepath.Join(root, nodeModulesDir)) {
		return reasonTSNoNodeModules
	}
	return reasonScipNoIndexer
}
