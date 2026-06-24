// Package syntax provides pure role derivation and graph-node join over
// SyntaxFacts gathered by the ast-grep extractor. It is a core-ring package:
// no os, no subprocess, no YAML, no adapter imports.
package syntax

import (
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
)

// RoleHit records one role assignment resolved onto a graph node.
type RoleHit struct {
	Role       string // handler|service|repository|domain
	Confidence string // high|medium|low
	Evidence   string // free-text from DeriveRoles
}

// NodeRoleIndex maps graph node IDs → their resolved roles, built once per run.
// The zero value is unusable; use BuildNodeRoleIndex.
type NodeRoleIndex struct {
	// nodeRoles maps node ID → deduplicated, stably-sorted []RoleHit.
	nodeRoles map[string][]RoleHit
}

// RolesFor returns the roles attached to nodeID, or nil if none.
// The returned slice is deduplicated and sorted by (role, confidence).
func (idx *NodeRoleIndex) RolesFor(nodeID string) []RoleHit {
	return idx.nodeRoles[nodeID]
}

// BuildNodeRoleIndex constructs a NodeRoleIndex from a set of SyntaxFacts (with
// roles already derived by DeriveRoles) and the dependency graph. Only facts
// that have a non-empty Role are joined.
//
// Per-language file→node resolution (verified against extractor output):
//
//   - TypeScript: node is "file:<path>"; identity mapping (node path == fact.File).
//   - Go: node is "package:<importPath>"; the Go extractor uses the stripped module
//     import path as the package node path, which in a standard layout equals the
//     repo-relative directory. goFileToModuleKey (from graph.BuiltinConventions)
//     returns that directory; we look for "package:<dir>" in the graph's node set.
//   - Python: node is "module:<dotted>"; the Python extractor keys nodes on the
//     dotted module name (e.g. "billing.service") while SyntaxFact.File is a
//     slash path. pythonFileToModuleKey converts slash→dotted, then we look for
//     "module:<dotted>".
//   - Rust: node is "package:<crate::mod>" (from cargo-modules DOT output or
//     cargo metadata). graph.RustFileToModuleKey(file, crateRoots) maps a .rs
//     file to "<crate>::<mod>" so we look for "package:<crate::mod>". Falls back
//     to crate-level lookup when crateRoots is absent.
//
// This is the single place language-specific node resolution lives; rules call
// RolesFor(nodeID) and stay language-agnostic.
func BuildNodeRoleIndex(g *graph.Graph, facts []diagnostic.SyntaxFact) *NodeRoleIndex {
	// Build a set of valid node IDs for fast existence checks.
	nodeSet := make(map[string]struct{}, len(g.Nodes()))
	for _, n := range g.Nodes() {
		nodeSet[n.ID()] = struct{}{}
	}

	// Fetch crate roots once for Rust resolution.
	crateRoots := g.CrateRoots()

	// Accumulate role hits per node; allow duplicates during accumulation, dedup at end.
	accumulator := make(map[string][]RoleHit)

	for _, f := range facts {
		if f.Role == "" {
			continue // no role derived — skip
		}
		hit := RoleHit{
			Role:       f.Role,
			Confidence: f.RoleConf,
			Evidence:   f.Evidence,
		}
		for _, nodeID := range resolveFileToNodes(f, nodeSet, crateRoots) {
			accumulator[nodeID] = append(accumulator[nodeID], hit)
		}
	}

	// Dedup and sort each node's hits for determinism.
	nodeRoles := make(map[string][]RoleHit, len(accumulator))
	for nodeID, hits := range accumulator {
		nodeRoles[nodeID] = deduplicateHits(hits)
	}

	return &NodeRoleIndex{nodeRoles: nodeRoles}
}

// resolveFileToNodes maps one SyntaxFact's file to the graph node IDs that
// should receive its role. Returns zero, one, or more node IDs.
//
// Per-language logic is isolated here so the rest of BuildNodeRoleIndex and all
// rule code stay language-agnostic.
func resolveFileToNodes(f diagnostic.SyntaxFact, nodeSet map[string]struct{}, crateRoots []graph.CrateRoot) []string {
	file := f.File
	lang := f.Language

	switch lang {
	case graph.LangTypeScript:
		// TS graph nodes are "file:<path>" — direct identity.
		nodeID := string(graph.NodeKindFile) + ":" + file
		if _, ok := nodeSet[nodeID]; ok {
			return []string{nodeID}
		}
		return nil

	case graph.LangGo:
		// Go file nodes are "file:<path>"; package nodes are "package:<importPath>".
		// The import path (after stripping the module prefix) equals the directory
		// in a standard Go module layout. goFileToModuleKey returns that directory.
		// We attach roles to the package node because rules operate on package edges.
		pkgPath := graph.BuiltinConventions.Lookup(graph.LangGo).FileToModuleKey(file)
		if pkgPath == "" {
			return nil
		}
		pkgNodeID := string(graph.NodeKindPackage) + ":" + pkgPath
		if _, ok := nodeSet[pkgNodeID]; ok {
			return []string{pkgNodeID}
		}
		return nil

	case graph.LangPython:
		// Python graph nodes are "module:<dotted>" (e.g. "module:billing.service").
		// SyntaxFact.File is slash-relative (e.g. "billing/service.py").
		// BuiltinConventions.FileToModuleKey converts slash→dotted.
		dottedKey := graph.BuiltinConventions.Lookup(graph.LangPython).FileToModuleKey(file)
		if dottedKey == "" {
			return nil
		}
		nodeID := string(graph.NodeKindModule) + ":" + dottedKey
		if _, ok := nodeSet[nodeID]; ok {
			return []string{nodeID}
		}
		return nil

	case graph.LangRust:
		// Rust graph nodes are "package:<crate::mod>" (from cargo-modules or cargo
		// metadata). graph.RustFileToModuleKey maps a .rs file to "<crate>::<mod>"
		// using the extractor-supplied CrateRoots; absent CrateRoots it returns "".
		moduleKey := graph.RustFileToModuleKey(file, crateRoots)
		if moduleKey == "" {
			// crateRoots absent (no cargo-modules) — try the crate-level convention
			// which uses directory heuristics.
			moduleKey = graph.BuiltinConventions.Lookup(graph.LangRust).FileToModuleKey(file)
		}
		if moduleKey == "" {
			return nil
		}
		nodeID := string(graph.NodeKindPackage) + ":" + moduleKey
		if _, ok := nodeSet[nodeID]; ok {
			return []string{nodeID}
		}
		return nil

	default:
		// Unknown language: try file identity, then passthrough convention.
		// This handles future language additions gracefully.
		fileNodeID := string(graph.NodeKindFile) + ":" + file
		if _, ok := nodeSet[fileNodeID]; ok {
			return []string{fileNodeID}
		}
		// Passthrough via convention (identity for unknown langs).
		key := graph.BuiltinConventions.Lookup(lang).FileToModuleKey(file)
		if key == "" || key == file {
			return nil
		}
		moduleNodeID := string(graph.NodeKindModule) + ":" + key
		if _, ok := nodeSet[moduleNodeID]; ok {
			return []string{moduleNodeID}
		}
		return nil
	}
}

// deduplicateHits removes duplicate (role, confidence) pairs and returns a
// stably-sorted slice: primary sort by role, secondary by confidence tier
// (high < medium < low — higher confidence first).
func deduplicateHits(hits []RoleHit) []RoleHit {
	seen := make(map[string]struct{}, len(hits))
	var out []RoleHit
	for _, h := range hits {
		key := h.Role + "\x00" + h.Confidence
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, h)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Role != out[j].Role {
			return out[i].Role < out[j].Role
		}
		return confidenceOrd(out[i].Confidence) < confidenceOrd(out[j].Confidence)
	})
	return out
}

// confidenceOrd maps a confidence string to a sort ordinal (lower = higher priority).
func confidenceOrd(c string) int {
	switch strings.ToLower(c) {
	case ConfHigh:
		return 0
	case ConfMedium:
		return 1
	case ConfLow:
		return 2
	default:
		return 3
	}
}
