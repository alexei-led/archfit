package rust

import (
	"bufio"
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	toolCargoModules   = "cargo-modules"
	moduleGraphTimeout = 15 * time.Minute // compile-heavy; generous ceiling
	statusPartial      = "partial"
)

// runModuleGraph invokes cargo-modules per workspace member and merges the
// resulting module-level nodes and edges into facts. On any per-crate failure it
// records partial coverage and continues — never hard-errors, matching the
// SCIP/complexity absent/partial contract.
//
// Integration design: module nodes use the path "<crate>::<mod>" (double-colon,
// matching cargo-modules' own DOT node IDs). This is distinct from the crate-level
// "package:<crate>" nodes emitted by parseAndNormalize; both sets coexist in
// graph.Facts. Metrics (cycle, blast_radius, cohesion, encapsulation) operate on
// all package: nodes, so they now see module granularity for Rust when this toggle
// is on. The degenerate-graph guard in internal/score fires when there are fewer
// than two connected package: nodes; with module graph on, a single-crate repo
// has many such nodes, so the guard no longer trips.
func (e *Extractor) runModuleGraph(ctx context.Context, members []cargoPackage) ([]graph.Node, []graph.Edge, diagnostic.Coverage) {
	if len(members) == 0 {
		return nil, nil, diagnostic.Coverage{Tool: toolCargoModules, Status: statusAbsent}
	}

	// Detect cargo-modules binary.
	if _, ok := e.runner.Detect(ctx, toolCargoModules); !ok {
		return nil, nil, diagnostic.Coverage{Tool: toolCargoModules, Status: statusAbsent}
	}

	var allNodes []graph.Node
	var allEdges []graph.Edge
	seenNodes := make(map[string]struct{})
	seenEdges := make(map[string]struct{})

	emitNode := func(n graph.Node) {
		id := n.ID()
		if _, ok := seenNodes[id]; !ok {
			seenNodes[id] = struct{}{}
			allNodes = append(allNodes, n)
		}
	}
	emitEdge := func(edge graph.Edge) {
		key := edge.From + "\x00" + edge.To + "\x00" + string(edge.Kind)
		if _, ok := seenEdges[key]; !ok {
			seenEdges[key] = struct{}{}
			allEdges = append(allEdges, edge)
		}
	}

	failedCrates := 0
	for _, m := range members {
		crateDir := filepath.Dir(m.ManifestPath)
		nodes, edges, ok := e.runCargoModulesForCrate(ctx, m.Name, crateDir, m.hasLibTarget())
		if !ok {
			failedCrates++
			continue
		}
		for _, n := range nodes {
			emitNode(n)
		}
		for _, edge := range edges {
			emitEdge(edge)
		}
	}

	status := statusOK
	switch {
	case failedCrates > 0 && len(allNodes) == 0:
		status = statusAbsent
	case failedCrates > 0:
		status = statusPartial
	}

	cov := diagnostic.Coverage{
		Tool:            toolCargoModules,
		FilesSeen:       len(members),
		FilesApplicable: len(members) - failedCrates,
		Status:          status,
	}
	return allNodes, allEdges, cov
}

// hasLibTarget reports whether this package declares a lib target, used to select
// the cargo-modules invocation flag (--lib vs --bin <name>).
func (p cargoPackage) hasLibTarget() bool {
	for _, t := range p.Targets {
		for _, k := range t.Kind {
			if k == "lib" || k == "proc-macro" || k == "cdylib" || k == "staticlib" {
				return true
			}
		}
	}
	return false
}

// runCargoModulesForCrate runs cargo-modules for a single crate and parses the DOT
// output into graph nodes and edges. Returns ok=false on any failure.
func (e *Extractor) runCargoModulesForCrate(ctx context.Context, crateName, crateDir string, isLib bool) ([]graph.Node, []graph.Edge, bool) {
	// --package is required in a workspace: run from a member dir without it and
	// cargo-modules errors "Multiple packages present in workspace, please explicitly
	// select one via --package". Harmless for a single-crate project.
	args := []string{"modules", "dependencies", "--no-externs", "--package", crateName}
	if isLib {
		args = append(args, "--lib")
	} else {
		args = append(args, "--bin", crateName)
	}

	out, err := e.runner.Run(ctx, toolrun.ToolCmd{
		Name:    toolCargoModules,
		Args:    args,
		WorkDir: crateDir,
		Timeout: moduleGraphTimeout,
	})
	if err != nil || out.ExitCode != 0 {
		return nil, nil, false
	}

	nodes, edges := parseDOT(out.Stdout)
	return nodes, edges, true
}

// parseDOT parses cargo-modules DOT output and returns module-level nodes and edges.
//
// Only "crate" and "mod" node types are kept — struct/fn/enum/type nodes are too
// granular and would dominate the graph without adding architectural signal.
// Edges with label="owns" become EdgeKindBelongsTo (structural containment);
// edges with label="uses" become EdgeKindDependsOn (cross-module dependency).
//
// Node path convention: the DOT node ID itself (e.g. "herdr::api") is used as the
// graph node Path with NodeKindPackage, making it consistent with crate-level nodes
// from parseAndNormalize. The "::" separator matches cargo-modules' own convention.
func parseDOT(data []byte) ([]graph.Node, []graph.Edge) {
	// cargo-modules emits a node per module AND per item (struct/fn/type), with
	// "owns" edges for the module hierarchy and "uses" edges for references. The
	// dependency signal we want is module->module coupling, but "uses" edges are
	// emitted between items (e.g. demo::a::make -> demo::b::Thing). So: collect the
	// module node set, then aggregate every "uses" edge to the ENCLOSING MODULE of
	// each endpoint (longest module-node prefix), dropping self-edges and duplicates.
	// "owns" edges are hierarchy, not dependencies — they only establish module
	// membership and are never emitted as graph edges. Keeping only module endpoints
	// (the earlier behaviour) dropped every uses edge, leaving an ownership tree with
	// no coupling for the metrics or SCIP strength to measure.
	moduleNodes := make(map[string]struct{})
	var lines []string

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		lines = append(lines, line)
		if isModuleNodeLine(line) {
			if path, ok := extractDOTNodeID(line); ok {
				moduleNodes[path] = struct{}{}
			}
		}
	}

	nodes := make([]graph.Node, 0, len(moduleNodes))
	for path := range moduleNodes {
		nodes = append(nodes, graph.Node{Kind: graph.NodeKindPackage, Path: path})
	}

	seen := make(map[string]struct{})
	var edges []graph.Edge
	for _, line := range lines {
		from, to, kind, ok := extractDOTEdge(line)
		if !ok || kind != graph.EdgeKindDependsOn { // only "uses" edges are dependencies
			continue
		}
		fromMod, ok1 := moduleOf(from, moduleNodes)
		toMod, ok2 := moduleOf(to, moduleNodes)
		if !ok1 || !ok2 || fromMod == toMod {
			continue // unresolved endpoint or an intra-module reference
		}
		key := fromMod + "\x00" + toMod
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		edges = append(edges, graph.Edge{
			From:       string(graph.NodeKindPackage) + ":" + fromMod,
			To:         string(graph.NodeKindPackage) + ":" + toMod,
			Kind:       graph.EdgeKindDependsOn,
			Language:   langRust,
			Confidence: "medium", // structural; no type-resolution
		})
	}
	return nodes, edges
}

// moduleOf returns the enclosing module of a cargo-modules node path: the longest
// "::"-delimited prefix that is itself a module node. An item node like
// "demo::a::make" maps to module "demo::a"; a module node maps to itself. Returns
// ok=false when no prefix is a known module (e.g. an external crate item).
func moduleOf(path string, modules map[string]struct{}) (string, bool) {
	for p := path; p != ""; {
		if _, ok := modules[p]; ok {
			return p, true
		}
		i := strings.LastIndex(p, "::")
		if i < 0 {
			return "", false
		}
		p = p[:i]
	}
	return "", false
}

// isModuleNodeLine reports whether the DOT line is a module-level node declaration
// (type "crate" or "mod" in the trailing comment cargo-modules always emits).
func isModuleNodeLine(line string) bool {
	// cargo-modules appends a comment like: // "crate" node  or  // "mod" node
	return strings.HasSuffix(line, `"crate" node`) || strings.HasSuffix(line, `"mod" node`)
}

// extractDOTNodeID extracts the node path from a DOT node declaration line.
// Input example:  "herdr::api" [label="pub(crate) mod|api", ...]; // "mod" node
// Returns ("herdr::api", true) or ("", false) on parse failure.
func extractDOTNodeID(line string) (string, bool) {
	// The node ID is the first quoted string on the line.
	if !strings.HasPrefix(line, `"`) {
		return "", false
	}
	end := strings.Index(line[1:], `"`)
	if end < 0 {
		return "", false
	}
	path := line[1 : end+1]
	if path == "" {
		return "", false
	}
	return path, true
}

// extractDOTEdge parses a DOT edge line and returns (from, to, edgeKind, ok).
// Input example:  "herdr" -> "herdr::api" [label="owns", ...] [constraint=true]; // "owns" edge
func extractDOTEdge(line string) (string, string, graph.EdgeKind, bool) {
	// Must contain `" -> "` to be an edge declaration.
	idx := strings.Index(line, `" -> "`)
	if idx < 0 || !strings.HasPrefix(line, `"`) {
		return "", "", "", false
	}
	from := line[1:idx]

	rest := line[idx+6:] // skip `" -> "`
	end := strings.Index(rest, `"`)
	if end < 0 {
		return "", "", "", false
	}
	to := rest[:end]

	if from == "" || to == "" {
		return "", "", "", false
	}

	// Determine edge kind from label= attribute.
	switch {
	case strings.Contains(line, `label="owns"`):
		return from, to, graph.EdgeKindBelongsTo, true
	case strings.Contains(line, `label="uses"`):
		return from, to, graph.EdgeKindDependsOn, true
	default:
		return "", "", "", false // skip unknown edge types
	}
}
