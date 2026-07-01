package ts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	toolName      = "depcruise"
	coverageTool  = "dependency-cruiser" // Coverage.Tool label (the CLI subcommand is toolName).
	langTS        = "typescript"
	statusOK      = "ok"
	statusPartial = "partial"
	statusAbsent  = "absent"

	runTimeout = 5 * time.Minute
)

// Extractor is the TypeScript import extractor using dependency-cruiser.
// It satisfies the engine.Extractor interface structurally.
type Extractor struct {
	runner toolrun.Runner
	cfg    config.ExtractConfig
}

// New returns an Extractor configured with the given runner and config.
func New(runner toolrun.Runner, cfg config.ExtractConfig) *Extractor {
	return &Extractor{runner: runner, cfg: cfg}
}

// Name returns the language identifier for this extractor.
func (e *Extractor) Name() string {
	return langTS
}

// Extract detects dependency-cruiser, runs it against the project root,
// parses the JSON output, and returns a graph.Facts + diagnostic.Coverage.
//
// If mode is off, Extract returns empty Facts and an "absent" Coverage immediately.
// If mode is auto and the tool is absent or the project has no package.json,
// Extract returns empty Facts and an "absent" Coverage record — never an error.
// If mode is on and the tool is absent, Extract returns an error.
func (e *Extractor) Extract(ctx context.Context, s scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
	if e.cfg.Mode == config.ModeOff {
		return graph.Facts{}, absentCoverage(""), nil
	}

	// Applicability: requires package.json in the project root.
	pkgJSON := filepath.Join(s.Root, "package.json")
	if _, err := os.Stat(pkgJSON); os.IsNotExist(err) {
		if e.cfg.Mode == config.ModeOn {
			return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/ts: package.json not found at %s", s.Root)
		}
		return graph.Facts{}, absentCoverage(""), nil
	}

	// Detect bunx or npx.
	launcher, _, found := e.detectLauncher(ctx)
	if !found {
		if e.cfg.Mode == config.ModeOn {
			return graph.Facts{}, diagnostic.Coverage{}, errors.New("extract/ts: dependency-cruiser not found (bunx/npx unavailable); install Node and run: npm i -D dependency-cruiser")
		}
		return graph.Facts{}, absentCoverage(""), nil
	}

	// Get dependency-cruiser version.
	version := e.detectVersion(ctx, launcher)

	// Build the depcruise command arguments. "." means "scan the analysis root
	// itself" (Config.ForExtract's default since the Src-derivation fix) and is
	// used literally — only a genuinely empty value falls back to the "src"
	// convention, since silently rewriting "." to "src" broke repos whose
	// TypeScript sources live directly under the root with no src/ subdir
	// (e.g. storybookjs/storybook's "code/" layout).
	src := e.cfg.Src
	if src == "" {
		src = "src"
	}

	// Determine work directory and source entry point.
	//
	// When ScanRoot is a subtree within a Git repo (SubtreePrefix != ""), run
	// depcruise from the git root so that the root tsconfig covers cross-package
	// alias resolution and paths in the output are git-root-relative (e.g.
	// "packages/pkg-a/src/index.ts"). The --include-only flag then scopes the
	// graph to the analysis boundary. This is the "multiple roots → merged graph"
	// path: a single depcruise invocation from the git root with one tsconfig
	// covering all workspace aliases produces the cross-package dependency graph.
	//
	// When SubtreePrefix == "" (ScanRoot == GitRoot, or non-git), the behaviour is
	// byte-identical to the previous single-package path.
	workDir := s.Root
	srcArg := src
	if s.SubtreePrefix != "" {
		workDir = s.GitRoot
		srcArg = filepath.ToSlash(filepath.Join(s.SubtreePrefix, src))
	}

	args := []string{toolName, srcArg, "--output-type", "json", "--exclude", "^node_modules"}
	// Scope depcruise to the analysis subtree when running from the git root.
	// --include-only uses a JS regex against the git-root-relative module paths.
	// Omitted when SubtreePrefix is empty to preserve byte-identical output on
	// the no-subtree path.
	if s.SubtreePrefix != "" {
		// Trailing (?:/|$) ensures the pattern matches the exact prefix directory
		// and not a sibling that shares a common prefix (e.g. "packages/api" must
		// not match "packages/api-client").
		args = append(args, "--include-only", "^"+regexp.QuoteMeta(filepath.ToSlash(s.SubtreePrefix))+"(?:/|$)")
	}
	// Resolve tsconfig: an explicit config wins; otherwise auto-detect one at
	// ScanRoot. Without --ts-config, dependency-cruiser cannot resolve tsconfig
	// `paths`/`baseUrl` aliases — those imports come back unresolved and are
	// classified as external, silently dropping internal cross-module edges from
	// the coupling metrics. (--ts-config and --no-config compose.)
	// The path is returned relative to workDir so depcruise resolves it correctly
	// whether running from ScanRoot (no-subtree) or the git root (subtree).
	if tsConfig := e.resolveTSConfig(s.Root, workDir); tsConfig != "" {
		args = append(args, "--ts-config", tsConfig)
	}
	// dependency-cruiser errors out when it cannot find its own config file. archfit
	// only needs the dependency graph (not depcruise's rules), so fall back to
	// --no-config (built-in defaults) for projects that ship no depcruise config.
	if !hasDepcruiseConfig(workDir) {
		args = append(args, "--no-config")
	}

	cmd := toolrun.ToolCmd{
		Name:    launcher,
		Args:    args,
		WorkDir: workDir,
		Timeout: runTimeout,
	}
	out, err := e.runner.Run(ctx, cmd)
	if err != nil {
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/ts: run dependency-cruiser: %w", err)
	}
	if out.ExitCode != 0 {
		// dependency-cruiser exited non-zero — e.g. the source directory does not
		// exist ("Can't open 'src' for reading"), as on an npm-workspaces monorepo
		// with no top-level src/. In auto mode this is a coverage gap, not a
		// run-level failure (the "warn-loud, don't block" contract); only an
		// explicitly required analyzer (ModeOn) hard-errors.
		reason := fmt.Sprintf("dependency-cruiser exited %d: %s", out.ExitCode, strings.TrimSpace(string(out.Stderr)))
		if e.cfg.Mode == config.ModeOn {
			return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/ts: %s", reason)
		}
		return graph.Facts{}, diagnostic.Coverage{Tool: coverageTool, Version: version, Status: statusPartial, Reason: reason}, nil
	}

	facts, cov, err := e.parseAndNormalize(out.Stdout, version, s.SubtreePrefix)
	if err != nil {
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/ts: parse output: %w", err)
	}
	return facts, cov, nil
}

// detectLauncher tries bunx then npx. Returns ("", "", false) if neither is available.
func (e *Extractor) detectLauncher(ctx context.Context) (string, string, bool) {
	for _, tool := range []string{"bunx", "npx"} {
		if info, ok := e.runner.Detect(ctx, tool); ok {
			return tool, info.Path, true
		}
	}
	return "", "", false
}

// tsConfigNames are the conventional tsconfig file names archfit auto-detects at
// the project root when no explicit TSConfig is configured. Ordered by preference:
// a concrete tsconfig.json before a base-only tsconfig.base.json.
var tsConfigNames = []string{"tsconfig.json", "tsconfig.base.json"}

// resolveTSConfig returns the tsconfig path to pass to dependency-cruiser: the
// explicitly configured path if set, otherwise the first conventional tsconfig
// found at scanRoot (so `paths`/`baseUrl` aliases resolve). Returns "" when none
// is configured or present. The returned path is relative to workDir (the
// depcruise run directory) so dependency-cruiser resolves it correctly whether
// running from scanRoot (no-subtree) or the git root (subtree).
func (e *Extractor) resolveTSConfig(scanRoot, workDir string) string {
	if e.cfg.TSConfig != "" {
		return e.cfg.TSConfig
	}
	for _, name := range tsConfigNames {
		full := filepath.Join(scanRoot, name)
		if _, err := os.Stat(full); err == nil {
			rel, err := filepath.Rel(workDir, full)
			if err == nil {
				return filepath.ToSlash(rel)
			}
			return full // absolute fallback
		}
	}
	// Subtree mode: workDir is the git root, which may hold a root-level tsconfig
	// that covers all packages. Fall back to searching workDir so path-alias edges
	// (tsconfig.paths/baseUrl) are not silently dropped from coupling_balance.
	if workDir != scanRoot {
		for _, name := range tsConfigNames {
			full := filepath.Join(workDir, name)
			if _, err := os.Stat(full); err == nil {
				rel, err := filepath.Rel(workDir, full)
				if err == nil {
					return filepath.ToSlash(rel)
				}
				return full // absolute fallback
			}
		}
	}
	return ""
}

// hasDepcruiseConfig reports whether a dependency-cruiser config file exists at root.
func hasDepcruiseConfig(root string) bool {
	for _, name := range []string{
		".dependency-cruiser.cjs", ".dependency-cruiser.js",
		".dependency-cruiser.mjs", ".dependency-cruiser.json",
	} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return true
		}
	}
	return false
}

// detectVersion runs `<launcher> depcruise --version` and returns the version string.
// Returns empty string on any failure (non-fatal).
func (e *Extractor) detectVersion(ctx context.Context, launcher string) string {
	out, err := e.runner.Run(ctx, toolrun.ToolCmd{
		Name:    launcher,
		Args:    []string{toolName, "--version"},
		Timeout: 30 * time.Second,
	})
	if err != nil || out.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(string(out.Stdout))
}

// ---------------------------------------------------------------------------
// JSON parsing types for dependency-cruiser output.
// ---------------------------------------------------------------------------

type dcOutput struct {
	Modules []dcModule `json:"modules"`
}

type dcModule struct {
	Source       string  `json:"source"`
	Dependencies []dcDep `json:"dependencies"`
	Deps         []dcDep `json:"deps"` // alternate key name used by some builds
	// Module-level resolution flags. dependency-cruiser lists every resolved
	// target as its own module entry, so node builtins (coreModule) and
	// uninstalled npm packages (couldNotResolve) appear here as sources — not
	// just as dependencies. They must not become first-party file nodes.
	CouldNotResolve bool `json:"couldNotResolve"`
	CoreModule      bool `json:"coreModule"`
}

type dcDep struct {
	Resolved        string   `json:"resolved"`
	Module          string   `json:"module"`
	DependencyTypes []string `json:"dependencyTypes"`
	Dynamic         bool     `json:"dynamic"`
	CouldNotResolve bool     `json:"couldNotResolve"`
	CoreModule      bool     `json:"coreModule"`
	// PreCompilationOnly is dependency-cruiser's flag for an import that does not
	// survive to runtime — i.e. an `import type {…}`. It is the boolean twin of
	// the "type-only" dependencyTypes entry; either marks a type-only edge.
	PreCompilationOnly bool `json:"preCompilationOnly"`
}

// dependency-cruiser dependencyTypes entries archfit reasons about.
const (
	depTypeTypeOnly          = "type-only"
	depTypePreCompilationOny = "pre-compilation-only"
)

// isTypeOnly reports whether a dependency exists only before compilation —
// a TypeScript `import type {…}` that is erased from the emitted JS. Detected
// via the boolean flag or either type-only dependencyTypes entry, for
// robustness across dependency-cruiser versions.
func (d dcDep) isTypeOnly() bool {
	if d.PreCompilationOnly {
		return true
	}
	for _, t := range d.DependencyTypes {
		if t == depTypeTypeOnly || t == depTypePreCompilationOny {
			return true
		}
	}
	return false
}

// strengthHint maps a dependency to a Balanced Coupling integration-strength
// hint (Vlad Khononov's ladder, weakest→strongest: Contract→Model→Functional→
// Intrusive). A type-only edge shares only the type shape and vanishes at
// runtime → Contract (weakest). A value/runtime edge — including a dynamic
// `import()` — binds to the target's exported names and signatures → Functional
// (connascence of name). archfit cannot tell Model from Functional from the
// import alone; SCIP symbol resolution refines that later in the pipeline, and
// config public/internal globs still win in classify. The hint is a fallback.
func (d dcDep) strengthHint() string {
	if d.isTypeOnly() {
		return string(coupling.StrengthContract)
	}
	return string(coupling.StrengthFunctional)
}

// ---------------------------------------------------------------------------
// Parse + normalise.
// ---------------------------------------------------------------------------

// parseAndNormalize parses dependency-cruiser JSON and builds graph.Facts.
func (e *Extractor) parseAndNormalize(data []byte, version, subtreePrefix string) (graph.Facts, diagnostic.Coverage, error) {
	var dc dcOutput
	if err := json.Unmarshal(data, &dc); err != nil {
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("unmarshal: %w", err)
	}
	// normPath strips the git-root-relative prefix from paths emitted by
	// depcruise when running from the git root (subtree mode). This converts
	// git-root-relative paths like "packages/api/src/x.ts" to ScanRoot-relative
	// "src/x.ts" so node IDs, matchesInternal globs, and downstream classify all
	// work correctly against config patterns.
	// No-op when subtreePrefix is "" — preserves byte-identical output on the
	// non-subtree path.
	normPath := func(path string) string {
		if subtreePrefix == "" {
			return path
		}
		prefix := subtreePrefix + "/"
		if strings.HasPrefix(path, prefix) {
			return path[len(prefix):]
		}
		return path // external/npm paths pass through unchanged
	}

	var nodes []graph.Node
	var edges []graph.Edge
	seenNodes := make(map[string]struct{})
	unresolved := 0
	filesSeen := 0 // first-party source files only (excludes core/unresolved module entries)

	emitNode := func(n graph.Node) {
		id := n.ID()
		if _, ok := seenNodes[id]; !ok {
			seenNodes[id] = struct{}{}
			nodes = append(nodes, n)
		}
	}

	for _, mod := range dc.Modules {
		// A node builtin (fs, path, crypto, …) is never a first-party source
		// file: skip its module entry entirely so it is not treated as a first-party
		// module. Its inbound edges are already dropped on the dependency side
		// (dep.CoreModule).
		if mod.CoreModule {
			continue
		}
		// An uninstalled npm package (no node_modules) is reported as an
		// unresolvable source entry. Record it as external (not first-party) so
		// metrics exclude it, but keep the node so edges TO it stay consistent.
		// Do NOT count it here: depcruise also lists the package as an unresolved
		// dependency of every file that imports it, and those edges below are the
		// authoritative unresolved count. Counting the module entry too would
		// double-count the same package.
		srcPath := normPath(mod.Source)
		if mod.CouldNotResolve {
			emitNode(graph.Node{Kind: graph.NodeKindExternal, Path: srcPath, Language: graph.LangTypeScript})
			continue
		}

		fromID := "file:" + srcPath
		emitNode(graph.Node{Kind: graph.NodeKindFile, Path: srcPath, Language: graph.LangTypeScript})
		filesSeen++

		// Merge dependencies and deps (alternate key).
		deps := mod.Dependencies
		deps = append(deps, mod.Deps...)

		for _, dep := range deps {
			// Skip Node.js core modules (fs, path, etc.).
			if dep.CoreModule {
				continue
			}

			// Resolve the target path: prefer resolved, fall back to module.
			toPath := dep.Resolved
			if toPath == "" {
				toPath = dep.Module
			}
			toPath = normPath(toPath)

			// An unresolved target is not a first-party source file: it is an
			// uninstalled npm package, a path depcruise could not resolve, or a
			// node builtin it failed to tag as core (common when node_modules is
			// absent). Emit it as an external node so first-party metrics
			// (blast_radius, coupling_balance) exclude it, while keeping the edge
			// so dependency/fan-out counts stay complete.
			nodeKind := graph.NodeKindFile
			if dep.CouldNotResolve {
				nodeKind = graph.NodeKindExternal
			}
			toID := string(nodeKind) + ":" + toPath
			emitNode(graph.Node{Kind: nodeKind, Path: toPath, Language: graph.LangTypeScript})

			// Determine edge kind. External targets are never internal.
			edgeKind := graph.EdgeKindImports
			if nodeKind == graph.NodeKindFile && e.matchesInternal(toPath) {
				edgeKind = graph.EdgeKindUsesInternal
			}

			// Determine confidence.
			confidence := "high"
			if dep.CouldNotResolve {
				confidence = "low"
				unresolved++
			}

			edges = append(edges, graph.Edge{
				From:         fromID,
				To:           toID,
				Kind:         edgeKind,
				Language:     langTS,
				Confidence:   confidence,
				StrengthHint: dep.strengthHint(),
			})
		}
	}

	status := statusOK
	if unresolved > 0 {
		status = statusPartial
	}

	facts := graph.Facts{
		Nodes:      nodes,
		Edges:      edges,
		Language:   langTS,
		Unresolved: unresolved,
	}
	cov := diagnostic.Coverage{
		Tool:            coverageTool,
		Version:         version,
		FilesSeen:       filesSeen,
		FilesApplicable: filesSeen,
		Unresolved:      unresolved,
		Status:          status,
	}
	return facts, cov, nil
}

// matchesInternal reports whether path matches any of the configured internal globs.
func (e *Extractor) matchesInternal(path string) bool {
	for _, pattern := range e.cfg.Internal {
		if matched, _ := doublestar.Match(pattern, path); matched {
			return true
		}
	}
	return false
}

// absentCoverage returns a Coverage record indicating the tool was not found.
func absentCoverage(version string) diagnostic.Coverage {
	return diagnostic.Coverage{
		Tool:    coverageTool,
		Version: version,
		Status:  statusAbsent,
	}
}
