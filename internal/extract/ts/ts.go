package ts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	toolName      = "depcruise"
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

	// Build the depcruise command arguments.
	src := e.cfg.Src
	if src == "" || src == "." {
		src = "src"
	}
	args := []string{toolName, src, "--output-type", "json", "--exclude", "^node_modules"}
	if e.cfg.TSConfig != "" {
		args = append(args, "--ts-config", e.cfg.TSConfig)
	}
	// dependency-cruiser errors out when it cannot find its own config file. archfit
	// only needs the dependency graph (not depcruise's rules), so fall back to
	// --no-config (built-in defaults) for projects that ship no depcruise config.
	if !hasDepcruiseConfig(s.Root) {
		args = append(args, "--no-config")
	}

	cmd := toolrun.ToolCmd{
		Name:    launcher,
		Args:    args,
		WorkDir: s.Root,
		Timeout: runTimeout,
	}
	out, err := e.runner.Run(ctx, cmd)
	if err != nil {
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/ts: run dependency-cruiser: %w", err)
	}
	if out.ExitCode != 0 {
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/ts: dependency-cruiser exited %d: %s", out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}

	facts, cov, err := e.parseAndNormalize(out.Stdout, version)
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
}

type dcDep struct {
	Resolved        string   `json:"resolved"`
	Module          string   `json:"module"`
	DependencyTypes []string `json:"dependencyTypes"`
	Dynamic         bool     `json:"dynamic"`
	CouldNotResolve bool     `json:"couldNotResolve"`
	CoreModule      bool     `json:"coreModule"`
}

// ---------------------------------------------------------------------------
// Parse + normalise.
// ---------------------------------------------------------------------------

// parseAndNormalize parses dependency-cruiser JSON and builds graph.Facts.
func (e *Extractor) parseAndNormalize(data []byte, version string) (graph.Facts, diagnostic.Coverage, error) {
	var dc dcOutput
	if err := json.Unmarshal(data, &dc); err != nil {
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("unmarshal: %w", err)
	}

	var nodes []graph.Node
	var edges []graph.Edge
	seenNodes := make(map[string]struct{})
	unresolved := 0

	emitNode := func(n graph.Node) {
		id := n.ID()
		if _, ok := seenNodes[id]; !ok {
			seenNodes[id] = struct{}{}
			nodes = append(nodes, n)
		}
	}

	for _, mod := range dc.Modules {
		fromID := "file:" + mod.Source
		emitNode(graph.Node{Kind: graph.NodeKindFile, Path: mod.Source})

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

			// An unresolved target is not a first-party source file: it is an
			// uninstalled npm package, a path depcruise could not resolve, or a
			// node builtin it failed to tag as core (common when node_modules is
			// absent). Emit it as an external node so first-party metrics
			// (martin instability/abstractness, blast radius) exclude it, while
			// keeping the edge so dependency/fan-out counts stay complete.
			nodeKind := graph.NodeKindFile
			if dep.CouldNotResolve {
				nodeKind = graph.NodeKindExternal
			}
			toID := string(nodeKind) + ":" + toPath
			emitNode(graph.Node{Kind: nodeKind, Path: toPath})

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
				From:       fromID,
				To:         toID,
				Kind:       edgeKind,
				Language:   langTS,
				Confidence: confidence,
			})
		}
	}

	filesSeen := len(dc.Modules)
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
		Tool:            "dependency-cruiser",
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
		Tool:    "dependency-cruiser",
		Version: version,
		Status:  statusAbsent,
	}
}
