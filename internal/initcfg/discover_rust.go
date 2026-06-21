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

// cargoMeta mirrors the subset of `cargo metadata --format-version 1` output we
// need: workspace members (the first-party crate set) and the packages list that
// resolves member IDs to crate names.
type cargoMeta struct {
	Packages         []cargoPkg `json:"packages"`
	WorkspaceMembers []string   `json:"workspace_members"`
}

type cargoPkg struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// DiscoverRust enumerates first-party crates from `cargo metadata` and returns
// one ModuleDef per workspace member. Each module's Paths glob is the crate name
// itself — Rust graph nodes are "package:<crate>" (path = crate name) and the
// change-locality file mapping also keys on the crate name, so a crate-name glob
// matches both the dependency node and the file-derived module key.
//
// Returns (nil, nil) when cargo is absent (a Cargo.toml can exist without a
// toolchain installed); a present-but-failing cargo or unparseable output is a
// real error, mirroring discoverGo's loud-on-failure behaviour.
func DiscoverRust(ctx context.Context, root string, runner toolrun.Runner) ([]ModuleDef, error) {
	if _, ok := runner.Detect(ctx, toolCargo); !ok {
		return nil, nil
	}

	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name:    toolCargo,
		Args:    []string{"metadata", "--format-version", "1", "--no-deps"},
		WorkDir: root,
	})
	if err != nil {
		return nil, fmt.Errorf("initcfg: cargo metadata: %w", err)
	}
	if out.ExitCode != 0 {
		return nil, fmt.Errorf("initcfg: cargo metadata exited %d: %s", out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}

	var meta cargoMeta
	if err := json.Unmarshal(out.Stdout, &meta); err != nil {
		return nil, fmt.Errorf("initcfg: parse cargo metadata: %w", err)
	}

	return buildRustModules(meta), nil
}

// buildRustModules resolves the workspace-member IDs to crate names and emits a
// sorted, deduplicated ModuleDef per first-party crate.
func buildRustModules(meta cargoMeta) []ModuleDef {
	memberIDs := make(map[string]struct{}, len(meta.WorkspaceMembers))
	for _, id := range meta.WorkspaceMembers {
		memberIDs[id] = struct{}{}
	}

	names := make(map[string]struct{}, len(meta.Packages))
	for _, p := range meta.Packages {
		if _, ok := memberIDs[p.ID]; ok && p.Name != "" {
			names[p.Name] = struct{}{}
		}
	}

	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	mods := make([]ModuleDef, 0, len(sorted))
	for _, name := range sorted {
		mods = append(mods, ModuleDef{
			Name:  name,
			Paths: []string{name},
			Layer: layerCore,
		})
	}
	return mods
}
