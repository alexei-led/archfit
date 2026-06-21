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
	mods, err := DiscoverRust(context.Background(), t.TempDir(), rustRunner(workspaceMetadata))
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
		if mods[i].Layer != layerCore {
			t.Errorf("module[%d].Layer = %q, want %q", i, mods[i].Layer, layerCore)
		}
	}
}

func TestDiscoverRust_SingleCrate(t *testing.T) {
	mods, err := DiscoverRust(context.Background(), t.TempDir(), rustRunner(singleCrateMetadata))
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
	mods, err := DiscoverRust(context.Background(), t.TempDir(), runner)
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
	if _, err := DiscoverRust(context.Background(), t.TempDir(), runner); err == nil {
		t.Fatal("expected error for non-zero cargo exit, got nil")
	}
}

func TestDiscoverRust_MalformedJSON_ReturnsError(t *testing.T) {
	if _, err := DiscoverRust(context.Background(), t.TempDir(), rustRunner("{not json")); err == nil {
		t.Fatal("expected error for malformed cargo metadata, got nil")
	}
}

// TestRender_RustToolMode asserts the tools.rust stanza reflects HasRust. The
// assertions bind the rust stanza to its mode (rust: → enabled: "<mode>") so a
// regression that flips the mode is caught — other languages emit "off" too, so
// a bare `enabled: "off"` check would pass even with the rust stanza wrong.
func TestRender_RustToolMode(t *testing.T) {
	on := Render(DiscoveredConfig{HasRust: true}, nil, false)
	if !strings.Contains(on, "rust:\n    enabled: \"on\"") {
		t.Errorf("HasRust=true should emit tools.rust enabled on; got:\n%s", on)
	}
	off := Render(DiscoveredConfig{HasRust: false}, nil, false)
	if !strings.Contains(off, "rust:\n    enabled: \"off\"") {
		t.Errorf("HasRust=false should emit tools.rust enabled off; got:\n%s", off)
	}
}
