package initcfg

import (
	"context"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/toolrun"
)

// rustRunner returns a mock runner that reports cargo present and replies to
// `cargo metadata` with the given JSON.
func rustRunner(metadataJSON string) *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, name string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, name == toolCargo
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if cmd.Name == toolCargo {
				return toolrun.Output{Stdout: []byte(metadataJSON)}, nil
			}
			return toolrun.Output{}, nil
		},
	}
}

// workspaceMetadata is a multi-crate workspace: three members plus one external
// dependency that must NOT become a module.
const workspaceMetadata = `{
  "packages": [
    {"id": "id-ripgrep", "name": "ripgrep"},
    {"id": "id-grep-cli", "name": "grep-cli"},
    {"id": "id-grep-matcher", "name": "grep-matcher"},
    {"id": "id-serde", "name": "serde"}
  ],
  "workspace_members": ["id-ripgrep", "id-grep-cli", "id-grep-matcher"]
}`

const singleCrateMetadata = `{
  "packages": [{"id": "id-just", "name": "just"}],
  "workspace_members": ["id-just"]
}`

func TestDiscoverRust_Workspace_OneModulePerMember(t *testing.T) {
	mods, _, err := DiscoverRust(context.Background(), t.TempDir(), rustRunner(workspaceMetadata))
	if err != nil {
		t.Fatalf("DiscoverRust: %v", err)
	}

	// Sorted, member crates only; external serde excluded.
	want := []string{"grep-cli", "grep-matcher", "ripgrep"}
	if len(mods) != len(want) {
		t.Fatalf("got %d modules, want %d: %v", len(mods), len(want), mods)
	}
	for i, w := range want {
		if mods[i].Name != w {
			t.Errorf("module[%d].Name = %q, want %q", i, mods[i].Name, w)
		}
		// Paths glob is the crate name so it matches the package:<crate> node.
		if len(mods[i].Paths) != 1 || mods[i].Paths[0] != w {
			t.Errorf("module[%d].Paths = %v, want [%q]", i, mods[i].Paths, w)
		}
		// No inter-crate edge, so no tier: a layer is never a name-based guess.
		if mods[i].Layer != "" {
			t.Errorf("module[%d].Layer = %q, want none without a dependency graph", i, mods[i].Layer)
		}
	}
}

// tierCLI is the outermost crate of tieredMetadata.
const tierCLI = "cli"

// tieredMetadata: cli depends on core, core on util (normal and build
// dependencies), and util dev-depends on cli for a test helper. The
// dev-dependency points back up the graph; the Rust extractor drops it by
// default, so it must not shape the tiers either.
const tieredMetadata = `{
  "packages": [
    {"id": "id-cli", "name": "cli", "dependencies": [{"name": "core", "kind": null}]},
    {"id": "id-core", "name": "core", "dependencies": [{"name": "util", "kind": "build"}]},
    {"id": "id-util", "name": "util", "dependencies": [{"name": "cli", "kind": "dev"}]}
  ],
  "workspace_members": ["id-cli", "id-core", "id-util"]
}`

// TestDiscoverRust_TiersFromDependencyGraph: layers are topological tiers of
// the normal/build crate graph, so every discovered edge points to an earlier
// layer and the starter direction rule starts with zero back-edges.
func TestDiscoverRust_TiersFromDependencyGraph(t *testing.T) {
	mods, edges, err := DiscoverRust(context.Background(), t.TempDir(), rustRunner(tieredMetadata))
	if err != nil {
		t.Fatalf("DiscoverRust: %v", err)
	}
	got := map[string]string{}
	for _, m := range mods {
		got[m.Name] = m.Layer
	}
	want := map[string]string{invModUtil: "layer-0", layerCore: "layer-1", tierCLI: "layer-2"}
	for name, layer := range want {
		if got[name] != layer {
			t.Errorf("crate %s layer = %q, want %q (all: %v)", name, got[name], layer, got)
		}
	}
	for _, e := range edges {
		if e.From == invModUtil && e.To == tierCLI {
			t.Errorf("dev-dependency util→cli became an edge: %v", edges)
		}
	}
	cfg := DiscoveredConfig{Modules: mods, Edges: edges, Layers: inferLayers(mods)}
	if n := layerBackEdgeCount(cfg); n != 0 {
		t.Errorf("layer back-edges at init = %d, want 0 (layers %v, edges %v)", n, cfg.Layers, edges)
	}
}

func TestDiscoverRust_SingleCrate(t *testing.T) {
	mods, _, err := DiscoverRust(context.Background(), t.TempDir(), rustRunner(singleCrateMetadata))
	if err != nil {
		t.Fatalf("DiscoverRust: %v", err)
	}
	if len(mods) != 1 || mods[0].Name != "just" {
		t.Fatalf("got %v, want one module 'just'", mods)
	}
}

func TestDiscoverRust_CargoAbsent_ReturnsNil(t *testing.T) {
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			t.Fatal("cargo metadata must not run when cargo is absent")
			return toolrun.Output{}, nil
		},
	}
	mods, _, err := DiscoverRust(context.Background(), t.TempDir(), runner)
	if err != nil {
		t.Fatalf("DiscoverRust: %v", err)
	}
	if mods != nil {
		t.Errorf("want nil modules when cargo absent, got %v", mods)
	}
}

func TestDiscoverRust_MetadataError_ReturnsError(t *testing.T) {
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, name string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, name == "cargo"
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{ExitCode: 101, Stderr: []byte("error: failed to parse manifest")}, nil
		},
	}
	if _, _, err := DiscoverRust(context.Background(), t.TempDir(), runner); err == nil {
		t.Fatal("expected error for non-zero cargo exit, got nil")
	}
}

func TestDiscoverRust_MalformedJSON_ReturnsError(t *testing.T) {
	if _, _, err := DiscoverRust(context.Background(), t.TempDir(), rustRunner("{not json")); err == nil {
		t.Fatal("expected error for malformed cargo metadata, got nil")
	}
}

// TestRender_RustToolMode asserts the languages.rust stanza reflects HasRust. The
// assertions bind the rust stanza to its mode (rust: → enabled: <mode>) so a
// regression that flips the mode is caught. An absent language stays at the
// config default auto, never false: a missed probe must not switch analysis off.
func TestRender_RustToolMode(t *testing.T) {
	on := Render(DiscoveredConfig{HasRust: true}, nil, false)
	if !strings.Contains(on, "rust:\n    enabled: auto") {
		t.Errorf("HasRust=true should emit languages.rust enabled auto; got:\n%s", on)
	}
	for _, want := range []string{"analyzers:\n  cargo_modules:\n    enabled: true", "  scip:\n    enabled: true"} {
		if !strings.Contains(on, want) {
			t.Errorf("HasRust=true should emit Rust deep analyzer stanza %q; got:\n%s", want, on)
		}
	}
	off := Render(DiscoveredConfig{HasRust: false}, nil, false)
	if !strings.Contains(off, "rust:\n    enabled: auto") {
		t.Errorf("HasRust=false should emit languages.rust enabled auto; got:\n%s", off)
	}
	if strings.Contains(off, "cargo_modules:\n    enabled: true") {
		t.Errorf("HasRust=false should not force cargo_modules; got:\n%s", off)
	}
}
