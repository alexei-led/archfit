package initcfg

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/toolrun"
)

// Rust toolchain constants.
const (
	// markerCargoToml is the Rust project-marker filename gating Rust discovery.
	markerCargoToml = "Cargo.toml"
	toolCargo       = "cargo"
)

// cargoMeta mirrors the subset of `cargo metadata --format-version 1 --no-deps`
// output we need: workspace members (the first-party crate set) and the packages
// list. With --no-deps each member still carries its declared dependencies array,
// which we use to build inter-crate edges without touching the full resolve graph
// (and without risking a Cargo.lock write).
type cargoMeta struct {
	Packages         []cargoPkg `json:"packages"`
	WorkspaceMembers []string   `json:"workspace_members"`
}

type cargoPkg struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Dependencies []cargoDep `json:"dependencies"`
}

// cargoDep is one declared dependency from a crate's Cargo.toml.
type cargoDep struct {
	Name string `json:"name"`
	// Kind is null for a normal dependency, "dev", or "build".
	Kind *string `json:"kind"`
}

// cargoDepKindDev marks a dev-dependency in cargo metadata.
const cargoDepKindDev = "dev"

// DiscoverRust enumerates first-party crates from `cargo metadata` and returns
// one ModuleDef per workspace member. Each module's Paths glob is the crate name
// itself — Rust graph nodes are "package:<crate>" (path = crate name) and the
// change-locality file mapping also keys on the crate name, so a crate-name glob
// matches both the dependency node and the file-derived module key.
//
// Returns (nil, nil, nil) when cargo is absent (a Cargo.toml can exist without a
// toolchain installed); a present-but-failing cargo or unparseable output is a
// real error, mirroring discoverGo's loud-on-failure behaviour.
func DiscoverRust(ctx context.Context, root string, runner toolrun.Runner) ([]ModuleDef, []ModuleEdge, error) {
	if _, ok := runner.Detect(ctx, toolCargo); !ok {
		return nil, nil, nil
	}

	// --no-deps limits the package set to first-party workspace members; each
	// member still lists its declared dependencies array, which we use to build
	// inter-crate edges without touching the full resolve graph (and without
	// risking a Cargo.lock write in the target repo).
	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name:    toolCargo,
		Args:    []string{"metadata", "--format-version", "1", "--no-deps"},
		WorkDir: root,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("initcfg: cargo metadata: %w", err)
	}
	if out.ExitCode != 0 {
		return nil, nil, fmt.Errorf("initcfg: cargo metadata exited %d: %s", out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}

	var meta cargoMeta
	if err := json.Unmarshal(out.Stdout, &meta); err != nil {
		return nil, nil, fmt.Errorf("initcfg: parse cargo metadata: %w", err)
	}

	mods, edges := buildRustModules(meta)
	return mods, edges, nil
}

// buildRustModules resolves the workspace-member IDs to crate names and emits a
// sorted, deduplicated ModuleDef per first-party crate, plus inter-crate edges
// from the resolve graph (when present).
func buildRustModules(meta cargoMeta) ([]ModuleDef, []ModuleEdge) {
	memberIDs := make(map[string]struct{}, len(meta.WorkspaceMembers))
	for _, id := range meta.WorkspaceMembers {
		memberIDs[id] = struct{}{}
	}

	memberNames := make(map[string]struct{}, len(meta.WorkspaceMembers))
	for _, p := range meta.Packages {
		if _, ok := memberIDs[p.ID]; ok && p.Name != "" {
			memberNames[p.Name] = struct{}{}
		}
	}

	sorted := make([]string, 0, len(memberNames))
	for n := range memberNames {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	// Derive inter-crate edges from each member's declared dependencies array.
	// With --no-deps cargo still populates dependencies[], so we can build
	// first-party edges without the resolve graph (and without Cargo.lock writes).
	// Matches the approach in internal/extract/rust/rust.go parseAndNormalize.
	seen := make(map[string]struct{})
	var edges []ModuleEdge
	for _, p := range meta.Packages {
		if _, isMember := memberIDs[p.ID]; !isMember {
			continue
		}
		if p.Name == "" {
			continue
		}
		for _, dep := range p.Dependencies {
			if _, toIsMember := memberNames[dep.Name]; !toIsMember {
				continue // skip external dependencies
			}
			if dep.Kind != nil && *dep.Kind == cargoDepKindDev {
				// The Rust extractor drops dev-dependencies by default, and a
				// dev-dependency may point back up the graph (a test-helper
				// crate depending on its dependents), which no tier order holds.
				continue
			}
			if dep.Name == p.Name {
				continue
			}
			key := p.Name + "\x00" + dep.Name
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			edges = append(edges, ModuleEdge{From: p.Name, To: dep.Name})
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})

	// Layers are topological tiers of the crate dependency graph, not a guess
	// from a name: a crate sits one tier above the deepest crate it depends on.
	// Normal and build dependencies cannot form a cycle in cargo, so every
	// current edge points to an earlier tier and the direction rule starts
	// clean; it then catches a new dependency that inverts the observed order.
	// Without edges there are no tiers, and Render asks for layers instead.
	layerAssign := topoLayerAssign(sorted, edges)

	mods := make([]ModuleDef, 0, len(sorted))
	for _, name := range sorted {
		mods = append(mods, ModuleDef{
			Name:  name,
			Paths: []string{name},
			Layer: layerAssign[name],
		})
	}

	return mods, edges
}
